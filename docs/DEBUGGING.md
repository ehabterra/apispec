# My route is missing — how to debug

apispec finds routes by statically tracing the call graph from `main()` to a
route-registration call it recognises, then walking into the handler. When a
route is missing from the generated spec, one of four things happened:

1. the **registration call never made it into the metadata** (package not
   loaded, file excluded, build tags),
2. the **call path from `main()` to the registration is broken** for the
   tracer (reflection, data-driven route tables, unresolved indirection),
3. **no pattern matched the registration** (unsupported wiring style, wrong
   framework config), or
4. the walk was **truncated by a safety limit** on a very large graph.

Each stage of the pipeline has an inspectable artifact. Work through them in
order — each step tells you which of the four cases you are in.

## Step 0 — read the stage log

Every run prints per-stage progress:

```
[engine] loaded 40 packages in 663ms
[engine] framework dependencies analysed (61 pkgs) in 3ms
[engine] metadata generated (7933 call edges, 40 pkgs) in 2.3s
[engine] tracker tree ready (lazy) in 1ms
[engine] spec mapped (245 paths) in 1.1s
```

- **Suspiciously few packages loaded?** Your route may live in a package that
  wasn't loaded (separate module, build tags, cgo). See `--skip-cgo`,
  `--include-package`, and make sure you point `--dir` at the module root.
- **Fewer paths than expected?** Continue below.
- A stderr warning like `MaxNodesPerTree limit (50000) reached, truncating
  lazy expansion` means case 4: raise `--max-nodes`.

## Step 1 — check which config actually ran

```bash
apispec -d . -o openapi.yaml --output-config used-config.yaml
```

`used-config.yaml` is the **effective** config after framework auto-detection
(plus the always-merged, receiver-scoped net/http patterns). Check:

- Is the detected framework the one registering your route? A project that
  imports several router packages gets one primary framework config.
- Do the `routePatterns` cover the call you use to register the route?
  Patterns match on the **method name and receiver type** — e.g. chi's
  `r.Get(...)` and `r.Method(...)` match different patterns. If your wiring
  style has no pattern, that's case 3: you can extend `used-config.yaml` by
  hand and re-run with `--config used-config.yaml` to confirm, then open an
  issue with the wiring style.

## Step 2 — is the registration in the metadata at all?

```bash
apispec -d . -o openapi.yaml --write-metadata        # writes metadata.yaml
# very large projects: add --split-metadata (-s) to shard it
```

`metadata.yaml` (at the module root) is everything apispec extracted before
any spec decision: functions, types, string-pooled call-graph edges. Search it
for your handler and for the registration call:

- **Handler function absent** → case 1. The package or file wasn't analysed:
  check `--include-*`/`--exclude-*` flags, `--auto-exclude-tests`/`-mocks`,
  build tags, and whether the code is generated into a directory you exclude.
- **Handler present but no call edge from your router-setup function to the
  registration** → case 2. Typical culprits:
  - routes registered from a **data-driven table** (`for _, r := range routes
    { mux.Handle(r.path, r.h) }`) — the path/method are runtime values;
  - registration behind **reflection** or code generation at runtime;
  - the router reaching the registration through an **interface** whose
    implementation apispec could not resolve (it resolves declared
    implementations, but stays honest when several are possible).
- **Everything present** → the tracker/extractor side; continue.

## Step 3 — look at the call graph visually

```bash
apispec -d . -o openapi.yaml --diagram diagram.html
# huge graphs: add --paginated-diagram (and --diagram-page-size 50 for 3000+ edges)
```

Open `diagram.html` and find your route-registration call. Follow the chain
back toward `main()`. A missing link in the middle (the registration node is
there but disconnected from the entry point) is case 2 — apispec only walks
routes it can reach from the roots. Wrapper functions, routers passed as
function parameters and handler factories are supported; if your link breaks
at a construct not listed there, that's the minimal repro to report.

## Step 4 — per-route drill-down with the insight report

For routes that ARE in the spec but look wrong (missing body, params,
responses), use the web UI:

```bash
apispecui -d .          # serves on localhost:8088
```

Every route gets an **insight report**: the resolved handler and its position
(`handlerFound: false` means the spec has the route but the handler body
could not be located in the call graph — bodies/responses will be missing),
a step-by-step trace of the walk (tracker- or callgraph-backed), detected
issues, and a Markdown export you can attach to a bug report as-is.

## Common causes, quickest fixes

| Symptom | Likely cause | Fix / workaround |
|---|---|---|
| Whole package of routes missing | package not loaded or excluded | check include/exclude flags, `--skip-cgo`, module root |
| One wiring style missing (e.g. `r.Method(...)`) | no route pattern for it | extend config (Step 1), report the style |
| Routes behind `for … range routeTable` missing | runtime values, statically unknowable | register statically, or accept the gap |
| Verb-less registrations show POST | historic default for unknown method | handlers that `switch r.Method` split automatically; otherwise the default applies |
| Deep/dense project: some routes missing + truncation warning | node budget hit | raise `--max-nodes` |
| Route present, body/params empty | handler not located / binding style unrecognised | insight report (Step 4), check `handlerFound` |
| Path shows `{someFunc}` placeholder | path built by a function call | statically unknowable; name comes from the called function |

## What to include in a bug report

1. apispec version (`apispec --version`) and the framework used
2. `used-config.yaml` (Step 1)
3. The relevant `metadata.yaml` excerpt, or the whole file if small (Step 2)
4. The insight report Markdown export for a nearby working route (Step 4)
5. A minimal `main.go` reproducing the wiring style — the fixtures in
   `testdata/` are good templates

With those four artifacts the gap is usually diagnosable without access to
your code base.
