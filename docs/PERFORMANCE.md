# Performance, limits, and profiling

APISpec bounds its own analysis so that a deep or cyclic call graph becomes a
truncation warning instead of a hang. This page explains what each bound does,
how the analysis is bounded, and how to profile a slow run.

## Analysis engine

The [tracker tree](PIPELINE.md) — the expansion of each route down to its real
handler and the calls inside it — is built **lazily**: subtrees are expanded on
demand, only along the paths a query actually touches. Cost scales with what is
*reachable from routes* rather than with the size of the codebase, so it tends
to win on large projects where much of the code never participates in routing,
and it degrades gracefully on dense or cyclic graphs (a cumulative budget, then
leaf stubs) instead of expanding exponentially.

There used to be a second, eager engine behind `--legacy-tracker`, which
materialised the whole tree up front. It was removed in v0.5.9 (issue #425)
after being measured against the default on three real services: identical
output on two, and on the third it documented **194 of 280 routes** — 31%
missing, with no warning — while running 1.6× slower. Across the fixture suite
it resolved four wiring shapes incorrectly. Its one remaining advantage was
memory on small projects, which is not worth carrying a second engine that
silently drops a third of an API.

If the analysis is missing something, please
[open an issue](https://github.com/ehabterra/apispec/issues).

## Limits

Expansion is bounded by node budgets plus an internal per-scope instance cap:

| Parameter            | Default  | CLI flag                | Bounds                                                                     |
|----------------------|----------|-------------------------|----------------------------------------------------------------------------|
| Max nodes / tree     | 50,000   | `--max-nodes`           | distinct callees materialized by the walk that *finds* route registrations |
| Max nodes / route    | 1,000,000 | `--max-nodes-per-route` | nodes expanded *below* one registration                                   |
| Max children / node  | 500      | `--max-children`        | children materialized per node                                             |
| Max args / function  | 100      | `--max-args`            | arguments walked per call                                                   |
| Max nested arg depth | 100      | `--max-nested-args`     | depth of nested argument expressions                                        |
| Max recursion depth  | 10       | `--max-recursion-depth` | recursion into repeated callees                                             |
| Max instances / key  | 100      | `--max-instances-per-key` | copies of one callee within an instance scope                             |

### Two node budgets, not one

The lazy engine's node budget is **two budgets, and they are independent**. That
split is what keeps truncation local. With one global budget the walk is
depth-first, so whatever expands first spends it and every route not yet
*reached* is lost outright — on a ~900-route project the allowance was gone
inside configuration and logging packages, and the run documented **12 paths**.
Bounding a route's detail separately, and charging its keys only to its own
allowance, took the same project to **640**.

The two flags therefore fail in different ways, and the warnings say which
happened:

- `--max-nodes` spent → routes are **missing**. Raise it.
- `--max-nodes-per-route` spent → a named route is **less detailed** (a schema
  or body may be absent), and no other route is affected. Raise it, or ignore
  it for endpoints you don't need in full.

### The per-scope instance cap

Instead of the recursion-depth / nested-args caps, the lazy engine bounds copies
of one callee **within an instance scope**: it keeps a copy of a shared helper
per route so per-route value tracing stays accurate, but cuts the combinatorial
copies a call diamond inside a single handler would otherwise create.

The scope is the nearest argument ancestor. For a route registered directly — or
inside a group closure (`r.Route("/x", func(r chi.Router) {…})`) — that is the
**handler**, not the closure; `testdata/group_closure_instances` pins this, and
it is why a group of 15 routes sharing one responder keeps every body down to a
cap of 1.

But how high that ancestor sits depends on how the app is wired, which is what
makes a fixed number unsafe. On the 374-route service below, the first scope to
run out was the argument node of a constructor call in the composition root —
above every handler it reaches, so those handlers share one allowance and the
routes that lose their bodies are decided by expansion order rather than by
anything about themselves. Raising `--max-instances-per-key` is the remedy until
the cap is scoped per route
([#224](https://github.com/ehabterra/apispec/issues/224)); it is a real trade,
measured across several services:

| `--max-instances-per-key` | success bodies (330-route service) | time (107-route service) | 374-route service | 163-route service | this repo (34 routes) |
|---|---|---|---|---|---|
| 5   | 77 / 391  | — | — | — | — |
| 25 (previous default) | 391 / 391 | 66s | 9 bodies empty, 8s | identical spec, 7s | 1 body empty, 8.6s |
| 40  | 391 / 391 | 82s | — | — | — |
| 100 (default) | 391 / 391 | — | 0 bodies empty, 9s | identical spec, 13s | 0 bodies empty, 9.2s |

The last three columns are one measurement session on one machine. Absolute wall
clock varies with hardware, and so does the gap between two rows — what carries
across machines is the **ratio**: the 163-route service's 13s / 7s = 1.8× is the
comparable figure, not the "+6s". "Empty" means a response that rendered as
`application/json: {}` — content present, schema missing.

The default is 100 rather than 25 because of how 25 failed rather than how
often: on the 374-route service, adding three handlers in an unrelated feature
pushed a shared response helper past 25 copies and silently removed the response
body of an endpoint nobody had touched. The threshold moves when you edit
elsewhere, so no project can tell whether it is safe.

**The cost is uneven, and on some projects it is large.** Medium projects (~20
paths) show no measurable change. The 374-route service pays about 1.1× for the
nine bodies it gains, and this repo about 1.07× for one. But a 163-route service
produced a byte-identical spec and took **1.8× as long** (7s → 13s) — it pays
the whole cost for nothing, because its cap fires 3.6M times inside
error-formatting call diamonds that no response body depends on. If your spec
does not change when you set `--max-instances-per-key 25`, set it: the lower cap
is safe *for the code as it stands today*, which is exactly the guarantee the
default gives up in exchange for not depending on where the next endpoint is
added.

Raise it above 100 when success responses are still missing bodies on a project
with very large route groups.

### Truncation warnings

When a limit is reached, APISpec logs a clear warning:

```text
Warning: MaxNodesPerTree limit (50000) reached, truncating tree at node example.com/pkg.Function
Warning: MaxChildrenPerNode limit (500) reached for node example.com/pkg.Function, truncating children
Warning: MaxRecursionDepth limit (10) reached for node example.com/pkg.Function
```

## Profiling

```bash
apispec -d ./my-project --cpu-profile --mem-profile --trace-profile --custom-metrics
go tool pprof profiles/cpu.prof
go tool pprof profiles/mem.prof
go tool trace   profiles/trace.out
```

Supported: CPU, memory, block, mutex, trace, and custom metrics
(`--custom-metrics` writes `metrics.json`).
