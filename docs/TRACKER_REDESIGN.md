# Tracker Tree Redesign — Diagnosis & Roadmap

> Architectural analysis of why the tracker tree resists reachability-style
> features (mux path params, middleware/security look-through), what the
> structural problem is, and the recommended path forward.
> Written 2026-07-09, based on the state of `internal/spec/tracker.go` at that time.

---

## 1. The core problem: call-graph construction performed as tree surgery

The tree is built top-down from `main` to every leaf, and along the way it is
**reordered** — based on interface, type, generics, param, variable,
assignment, and chain-call resolutions — to discover the *real* path. That
sentence names the problem exactly: the build is doing **two different jobs
at once**, and the first is expressed through the second's data structure.

- **Job A — call-target resolution (edge *discovery*).** The syntactic call
  graph does not contain the real edges:
  - an interface method call has no concrete target;
  - a handler stored in a variable / param / struct field has no edge to its
    body;
  - a generic function has no concrete instantiation;
  - a chained call's receiver has no link to its producer.

  The top-down build *discovers* these targets — interface resolution,
  `TraceVariableOrigin`, assignment linking, `attachReturnedClosureBody`,
  chain processing — and records each discovery **by rewiring the tree**
  (reparenting, appending children, hijacking `Parent`).

- **Job B — path materialization** for per-route value/shape tracing
  (request bodies, responses, generic specialization).

Because Job A's output is expressed as mutations of Job B's structure, every
pathology in §2 follows. And there is a second, independent defect:

### 1.1 Resolution is a fixpoint problem; the build runs a fixed pass order

Resolving one edge can enable resolving another: a router passed as a
parameter, whose method registers a handler stored in a field, assigned via a
functional option — each link is only discoverable after the previous one.
The build instead runs a **fixed number of passes in a fixed order**:

```
NewTrackerNode (top-down build)
  → traverseTree(assignmentNodes)
  → traverseTree(variableNodes)
  → processChainRelationships
```

A fixed pass order is a **truncated fixpoint**. The known gaps are precisely
the cases that need one more iteration:

| Known gap | Symptom |
|---|---|
| Functional options | the reverse-`Parent` TODO at `tracker.go` ~1222 (`assignmentNode.Parent = argNode`) |
| Handler factory (`h.Create()` returning a closure) | needed its own dedicated hook, `attachReturnedClosureBody` |

(Historical note: the fiber `/products` "router passed as function parameter"
case was once thought to be such a gap, but was a stale `used-config.yaml` —
the tracker *does* traverse routers passed as func args. It still illustrates
fragility: correctness there depends on regex config + traversal order, where
a resolved graph makes it a plain edge.)

Each new wiring style currently becomes a new *tree hook*; under a fixpoint
formulation it becomes a new *rule* (§3).

---

## 2. Secondary diagnosis: one spanning tree storing five relations by mutation

The tracker tree is semantically a **multi-relation graph** — it encodes:

1. call structure (caller → callee),
2. assignment/dataflow links,
3. parameter bindings (arg at call site → param in callee),
4. chain order (`NewEncoder(w).Encode(v)`),
5. interface/generic specialization,

— all forced into a **single tree shape with one `Parent` pointer per node**,
where each relation is applied by *destructively rewiring* that shared shape.

### Evidence (all in `internal/spec/tracker.go`)

| Symptom | Location | What it shows |
|---|---|---|
| `AddChild` detaches a node from its previous parent | `AddChild`/`detachChild` (~239–274) | Whichever pass runs last (assignments, params, chains) *owns* a node's ancestry; other relations are overwritten |
| Chain wiring appends to `Children` **without** setting `Parent` | `processChainRelationships` (~588–634) | Documented workaround: doing it properly would steal the node from its call-site parent and break `traceArgViaParent` |
| `assignmentNode.Parent = argNode` reverse link | TODO (~1222–1228) | No place for a second relationship, so `Parent` is hijacked backwards; the comment admits it's confusing |
| Shared nodes across all routes via `nodeMap` (keyed by edge ID) | throughout | Per-route facts must be encoded in the linkage of shared *mutable* nodes — why the global-seen-set experiment broke WrappedResponse tracing |
| Last-write-wins `assignmentIndex`, "most recent parent = last in slice" | (~437–447, ~1076–1081) | Determinism sorts make the winner *stable*, but semantics are still "whoever wrote last wins," not "the right one wins" |
| `MaxNodesPerTree`/`nodesBuilt`, `MaxRecursionDepth`, `MaxChildrenPerNode`, depth-50/20-children search caps | (~316–324, ~1354–1380, ~1483–1488, ~679, ~690) | Eager path materialization is exponential; every safety brake is load-bearing for wall-clock time, and every brake makes coverage **best-effort** |

### The consequence

Completeness-sensitive queries — *"does the handler reach `mux.Vars` at
all?"*, middleware detection, security detection — cannot use a structure
that legitimately truncates, because it can't answer **"no"** reliably.
Three features independently growing their own `meta.Callers` walks
(`lookThroughMiddleware`, security detection, mux `recoverAccessorKeys`) is
the smell confirming the **structure**, not the features, is the problem.

The tree *is* good at what it materializes: per-route value/shape tracing for
request bodies and responses. It is structurally incapable of completeness or
of holding more than one relationship per node.

### One-line summary

> The tracker's "reordering" is **on-the-fly call-graph construction
> performed as tree surgery**. A single-parent tree physically can't hold N
> relationships, and a fixed pass order can't complete a fixpoint — so every
> new wiring style either overwrites an old relation or grows a hack. The
> fix: discover edges in a dedicated **resolution phase** producing a
> *resolved call graph*; make the tree a thin lazy view of it.

---

## 3. The centerpiece fix: a resolved call graph (on-the-fly construction)

This is the standard solution in program analysis — *on-the-fly call-graph
construction*, as done by Soot/WALA-style frameworks and pointer analyses
(CHA → RTA → VTA / Andersen-style refinement, k-CFA). Instead of discovering
call targets while building a tree, run a **worklist fixpoint** over
relations and emit a **resolved call graph**:

```
ResolvedEdge {
    Caller    FuncCtx   // function + relevant bindings
    Site      Position  // the call site
    Callee    FuncCtx   // concrete target + bindings
    Via       Kind      // direct | interface-dispatch | func-value |
                        // generic-instantiation | chain
}
```

Each resolution kind that today rewires the tree becomes an **edge-producing
rule**:

| Today (tree surgery) | Rule in the resolution phase |
|---|---|
| Interface resolution map + `ResolveInterfaceFromMetadata` | *dispatch rule:* receiver's concrete-type set (from assignments/composites) → emit edge(s) to each implementation, `Via: interface-dispatch` |
| `variableNodes` / `TraceVariableOrigin` linking | *func-value rule:* function values propagate through assignments, params, returns, fields; a call through a variable that a func value flows into → emit edge, `Via: func-value` |
| Generic `filterChildren` / `TypeParamMap` | *instantiation rule:* edge carries canonical relevant bindings; `Decode[User]` and `Decode[Order]` are distinct `FuncCtx`s |
| `attachReturnedClosureBody` (handler factory) | *return-value rule:* a call whose result is a func literal/value → emit edge from the registration site to the closure body |
| `processChainRelationships` | not an edge problem at all — a plain relation `chains(prev, next)`, kept as a table |

**Worklist fixpoint:** seed with syntactic (direct) edges; each new edge can
expose new flows (a resolved dispatch reveals an assignment that carries a
handler, which resolves another call-through…); iterate until no rule fires.
Termination is guaranteed: finitely many functions × bounded contexts, and
the edge set only grows (monotone). This is exactly the iteration the fixed
pass order truncates today — router-as-param and functional options stop
being special cases and become ordinary rule applications on iteration 2+.

**Payoff:** the "real path" lives *in the edges*, discovered once, globally,
deterministically. Everything downstream — DAG algorithms (§4), summaries
(§5), the lazy tree (§6) — operates on the resolved graph and never needs to
reorder anything. New frameworks / wiring styles = new rules (mostly
config-driven, matching the existing pattern-based design), not new tree
hooks.

### 3.1 Don't hand-roll it: `golang.org/x/tools` SSA + VTA (already a dependency)

apispec already depends on `golang.org/x/tools` and loads via `go/packages`
with full syntax + types. `go/ssa` and `go/callgraph/vta` live in that same
module and consume exactly what `packages.Load` produces — **zero new
dependencies**. VTA (Variable Type Analysis) is the maintained, production
resolved-call-graph builder for Go (it powers govulncheck).

| §3 rule | x/tools equivalent |
|---|---|
| interface-dispatch | VTA propagates concrete types through assignments/params/returns → edges to actual implementers |
| func-value | function values through variables/params/fields/returns → resolved at call-through sites |
| return-value (handler factory) | closures are first-class in SSA (`MakeClosure`); returned handler = `*ssa.Function` with real edges → `attachReturnedClosureBody` disappears |
| generic-instantiation | `ssa.InstantiateGenerics` builder mode monomorphizes: `Decode[User]` / `Decode[Order]` become distinct functions — `FuncCtx` at IR level |
| worklist fixpoint | VTA's internals; never written by hand |

**Setup:** `ssautil.AllPackages(pkgs, ssa.InstantiateGenerics)` from the
engine's existing `packages.Load` result, then `vta.CallGraph(...)`.
Cheaper siblings behind the same `callgraph.Graph` API: `rta` (needs `main`
as root — apispec always has one) and `cha` (coarsest/fastest). **Avoid
`go/pointer`** — deprecated; VTA is its official replacement.

**Compatibility bridge:** `token.Pos`. SSA shares the `FileSet` from the
same load, so every `*ssa.Function` / call instruction joins back to AST
metadata by position. One load, two views:

- **Keep the metadata layer** for framework pattern matching, literal
  extraction (paths, `mux.Vars` keys, tags), and `go/types` schema
  derivation — the domain value no OSS tool provides.
- **Replace the resolution machinery**: interface-resolution map,
  `variableNodes` linking, `TraceVariableOrigin` rewiring,
  `attachReturnedClosureBody`, `maxWrapperLookThroughDepth` walks — all
  become queries over the VTA graph.
- (Optional, much later) value tracing itself could move to SSA def-use
  chains (`Value.Referrers()`), a more principled substrate than argument
  tracing — don't couple it to the first step.

**Performance:** SSA build ≈ type-check cost (linear, already paid for the
load); VTA near-linear in practice on very large graphs (govulncheck runs it
daily) — vs. today's worst-case-exponential tree materialization behind five
safety limits.

**Caveats:** VTA over-approximates (spurious edges on many-implementer
interfaces — fine for reachability/summaries; filter with assignment-based
resolution where precision matters); reflection-based routing stays
invisible (same as today); `go/ssa` isn't under the Go 1 compatibility
promise but is the de-facto standard substrate (staticcheck, govulncheck,
golangci-lint). Ruled out: CodeQL (not embeddable, licensing), Infer (no Go
frontend), Soufflé/Datalog engines (external toolchain), `honnef.co/go/tools/ir`
(x/tools callgraph algorithms don't accept it).

**Validation spike — DONE 2026-07-09, both hypotheses confirmed.**
Spike code: `internal/spike/vta_spike_test.go` (test-only, not wired into
the product).

- `TestVTA_MuxAccessorReachability` (`testdata/mux_path_params`): VTA
  reproduces all four facts the hand-rolled `handlerReachesAccessor`
  computes — `getProduct`/`getTag` reach `mux.Vars` directly, `getOrder`
  reaches it **through the `pathVar` helper** (path recovered:
  `getOrder → pathVar → mux.Vars`), `getItem` does not reach it.
  No depth bound needed. Timings: load 1.4s, SSA 0.5s, VTA 1.1s
  (17k call-graph nodes, whole program incl. gorilla/mux).
- `TestVTA_FiberRouterAsParam` (`testdata/fiber`): the
  router-as-function-parameter wiring (which the tracker handles today, but
  via config-regex + traversal order — historically fragile, see the §1.1
  note) is recoverable **directly and trivially** from SSA + VTA. From
  `main`, VTA reaches `products.Routes(r fiber.Router)`; inside it, every
  registration is an interface invoke whose method, literal path, and
  handler are recoverable from SSA: `{Get / ListProducts} {Post /
  CreateProduct} {Get /:id GetProduct}`. Timings: load 1.7s, SSA 0.8s, VTA
  1.8s (21.7k nodes, whole program incl. fiber v2).

Notes from the spike: whole-program SSA+VTA lands in ~2–4s on these
fixtures with dependencies included — acceptable, and dep syntax loading
(`NeedDeps|NeedSyntax`) is the dominant cost; variadic handler args
(`...fiber.Handler`) need a small SSA walk (alloc → index-addr → store →
slice) to recover the stored function — see `funcsStoredIn` in the spike.

**Performance comparison vs. current pipeline — measured 2026-07-09**
(`TestComparePerf` in `internal/spike/perf_compare_test.go`; single-run
wall-clock, engine default limits):

| fixture | current: load+meta | tree | **total** | vta-light: load | ssa | vta | **total** |
|---|---|---|---|---|---|---|---|
| mux_path_params | 224ms | 0ms | **224ms** | 248ms | 3ms | 10ms | **261ms** |
| fiber | 311ms | 2ms | **312ms** | 285ms | 3ms | 19ms | **308ms** |
| dense_graph | 327ms | 39ms | **365ms** | 254ms | 3ms | 10ms | **267ms** |
| cyclic_graph | 248ms | **1.105s** ⚠ | **1.354s** | 246ms | 5ms | 13ms | **264ms** |

- **vta-light** = deps from export data, no bodies — the realistic
  integration mode (module functions get SSA; calls into deps resolve to
  declared stubs, which is all reachability needs; verified: `getOrder →
  mux.Vars` still resolves). SSA+VTA analysis itself is **3–24ms** —
  effectively free; package loading dominates both pipelines equally.
- ⚠ On `cyclic_graph`, the tracker tree hit `MaxNodesPerTree` (50,000) —
  1.1s to produce a **truncated** result from 116 call edges, vs. VTA's
  13ms for a **complete** one. On the stress fixture the tree is ~80×
  slower per-analysis *and* incomplete. That asymmetry (analysis cost that
  explodes with graph shape vs. one that stays linear) is the practical
  version of §4's unfolding argument.
- Full dep-bodies mode (2.5–4.3s) is only needed to analyze *inside*
  frameworks; not required for the reachability/summaries family. Caveat:
  light mode can lose interface flows that pass *through* dependency code
  (callback stored by the framework, invoked later) — acceptable for
  module-side registration/reachability queries, revisit per feature.

---

## 4. The DAG framing — why the blow-up exists at all

The **resolved** call graph of §3 is a DAG (plus cycles from recursion): each
edge stored once. The tracker tree is the **unfolding** of that DAG into
paths — a function called from 10 places gets 10 physical subtrees, and
shared callees multiply again underneath. The exponential is not a traversal
bug; it is the mathematical cost of turning a DAG into a tree. Every limit
(`MaxNodesPerTree`, `MaxRecursionDepth`, `MaxChildrenPerNode`) is a tax on
that one decision.

**Guiding principle: store the DAG, never materialize the unfolding.** The
"tree" consumers see should be a *view* computed on demand.

Two standard DAG techniques apply directly:

### 4.1 SCC condensation (Tarjan / Kosaraju)

Collapse each strongly-connected component (mutually/self-recursive
functions) into one super-node. The condensed graph is a **true DAG — no
cycles by construction**. This replaces `MaxRecursionDepth` as a correctness
mechanism: recursion isn't "cut off at depth N," it's "this SCC is one
analysis unit." One `O(V+E)` pass. Cheap enough to recompute as the
resolution fixpoint (§3) adds edges — or just compute once after the
fixpoint converges.

### 4.2 Reverse topological order

On the condensed DAG, process functions **bottom-up**: leaves first, then
callers. By the time you reach a function, everything it calls is already
analyzed — each function analyzed **exactly once**. This is the foundation
for summaries (§5).

---

## 5. Function summaries — the highest-value query pattern

How industrial analyzers (Meta's Infer, compilers' interprocedural passes)
scale. It formalizes what apispec has **already built three times ad hoc**:
`lookThroughMiddleware`, security detection, and mux `recoverAccessorKeys`
are all hand-rolled summary computations.

For each function, precompute a small record of domain facts:

```go
type FnSummary struct {
    ReachesAccessor  map[string]bool   // e.g. "mux.Vars", "chi.URLParam"
    PathVarKeysRead  []string          // literal keys, incl. via helpers
    ResponseShapes   []ShapeRef        // what it writes/encodes
    ReturnsHandler   *HandlerRef       // handler-factory pattern
    PropagatesParams map[int]ParamFlow // "my param 1 flows into mux.Vars key"
}
```

Computed bottom-up in reverse topo order **over the resolved graph** (§3);
composition at call sites is trivial:

- `pathVar(r, key)`'s summary says *"my 2nd argument is used as a path-var key"*.
- When `getOrder` calls `pathVar(r, "id")`, composing summaries yields
  `KeysRead = {"id"}`.
- Helper indirection falls out **for free, at any nesting depth, with no
  depth limit** — each function is summarized once regardless of caller count.

**Cycles:** within an SCC, iterate to fixpoint (summaries only grow; domains
are finite → guaranteed termination), or start recursive members at ⊥ (empty
summary) — which is what today's bounded walk approximates.

**This retires `maxWrapperLookThroughDepth` entirely**: the depth-6 bound
exists only because each query re-walks from its query point; summaries walk
the whole graph once.

---

## 6. Lazy cursor — the tree as a computed view

**Feasibility check (verified):** all consumers — extractor, pattern
matchers, type resolver, context provider — already go through
`TrackerNodeInterface` / `TrackerTreeInterface` (`interfaces.go:176–224`),
and `mock_tracker.go` proves a second implementation works. A lazy
implementation is a **drop-in experiment, not a rewrite**.

```go
type LazyNode struct {
    edgeID   string      // which resolved edge this is
    ctx      *BindingCtx // type-param bindings, arg origins for THIS path
    tree     *LazyTree
    children []*LazyNode // nil = not yet computed
}

func (n *LazyNode) GetChildren() []TrackerNodeInterface {
    if n.children == nil {
        n.children = n.tree.expand(n.edgeID, n.ctx) // resolved graph + relations
    }
    return asInterfaces(n.children)
}
```

Key properties:

- **No reordering exists, by construction.** Expansion reads the *resolved*
  graph (§3), where interface/func-value/generic/factory targets are already
  edges. The tree never has to discover anything.
- **Identity = (edgeID, context), not a mutable object.** Today the same
  `Encode` node is shared and mutated by every route; here each route's
  cursor over `Encode` is a distinct lightweight pair carrying its *own*
  context. Per-route isolation stops being a fragile invariant (the
  `processChainRelationships` non-parenting hack) and becomes free — context
  is passed down, never stored in shared state.
- **`Parent` is trivially correct** — it's whoever expanded you. No
  rewiring, so no relation ever fights another for the pointer.
- **Pay only for what matchers visit.** Route extraction touches a shallow
  slice; body/response tracing goes deep on a few paths. The eager tree
  pre-builds everything to serve both.
- **Cycles:** ancestor-set on the cursor's own path (or SCC condensation) —
  visiting an edge already on *your own path* returns a leaf. This is
  per-path state on the cursor — exactly what the global-seen-set experiment
  got wrong by making it global.

---

## 7. Memoization keyed by (edgeID, relevant bindings)

Laziness bounds *time per query*, but two routes visiting the same subtree
still expand it twice. Memoization fixes that — with one subtlety that makes
or breaks it: **the key must be the *relevant* context only.**

```
key = (edgeID, canonical(bindings ∩ freeTypeParams(callee)))
```

— only the type parameters the callee's signature actually mentions, sorted
into a canonical string.

- **Over-keying** (full path, all bindings) → every path is a distinct key →
  zero sharing → you've rebuilt the eager tree with extra steps.
- **Under-keying** (edgeID alone) → `Decode[User]` and `Decode[Order]` merge
  → wrong schemas.

The existing `filterChildren` / `TypeParams()` machinery already computes
exactly the right discriminator — it just applies it by filtering a
materialized tree instead of keying a cache.

The memo is **pure** here: metadata is immutable after build and generation
is single-shot → no invalidation problem, shareable across all routes. This
is the "summary edges" idea from the IFDS / functional interprocedural
analysis literature (Sharir–Pnueli; Reps–Horwitz–Sagiv): analyze a procedure
once per *relevant* context, reuse everywhere.

**Layering:** memoized expansion caches *structure* (children); summaries
(§5) cache *facts*. Summaries are coarser, cheaper, and answer most queries
without any expansion; the lazy cursor is the fallback for value-shape
tracing that genuinely needs paths.

---

## 8. Supporting practices

### 8.1 Base facts vs. derived facts (the Datalog discipline)

Treat metadata as immutable base relations —

```
calls(caller, callee, site)          // syntactic, direct only
assigns(var, origin, fn)
binds(param, arg, site)
chains(prev, next)
implements(iface, concrete)
```

— derived facts (the resolved edges of §3, summaries of §5) are computed by
*rules over relations*, iterated to fixpoint, **never stored back by
mutating a shared structure**. No Datalog engine needed; only the
discipline. Every determinism bug fixed so far (map-order sorts,
last-write-wins `assignmentIndex`, "most recent parent") came from derived
facts being written into shared mutable structure — where write *order*
becomes semantics. Derived facts from sorted base tables via monotone rules
are deterministic by construction (fixpoints are order-independent).

### 8.2 Query API over structure access

Consumers currently walk `Children` and pattern-match. Wrapping the common
questions —

```go
OriginOf(arg)               // where did this value come from?
CalleesOf(fn)               // what does this function call? (resolved)
ReachedBy(fn, accessor)     // does fn transitively reach the accessor?
```

— behind methods on the tree/metadata lets implementations swap
(eager → lazy → summary-backed) per query without touching matchers. The
existing interface layer abstracts *nodes*; this abstracts *questions*.

### 8.3 Immutable-after-build + freeze

Once the resolution fixpoint converges, nothing mutates shared state except
memo inserts (idempotent). Also unlocks safe per-route parallelism later.

### 8.4 Bounded ≠ silent

Wherever a cap genuinely remains (memo size, pathological SCCs, dispatch
fan-out), keep the `warnOnce` pattern — a truncated analysis that *says so*
is a diagnostic; one that doesn't is a wrong spec.

### 8.5 Migration safety

Golden fixtures + determinism tests already exist. Run both implementations
side-by-side behind `TrackerTreeInterface` and diff outputs across all of
`testdata/`. That is the whole regression story — hence: build the new
pieces as **new** implementations rather than modifying `TrackerTree` in
place.

---

## 9. Roadmap — suggested order of attack

Each step lands independently and is verifiable against fixtures before the
next.

| # | Step | Where | Payoff |
|---|---|---|---|
| 1 | **SCC condensation + reverse topo order** | `internal/metadata` | Small, self-contained; kills depth limits as correctness mechanism; foundation for everything below |
| 2 | **Resolved call graph** — via x/tools SSA + VTA (§3.1), not hand-rolled | `internal/metadata` (new file/pkg, ~100–200 lines of glue) | The centerpiece (§3): subsumes interface-resolution map, `variableNodes` linking, `attachReturnedClosureBody`; fixes router-as-param and functional-options gaps; zero new dependencies |
| 3 | **Function summaries** for the reachability family (middleware, security, mux accessors), over the resolved graph | `internal/metadata` or new package | Unifies three ad-hoc walks; deletes `maxWrapperLookThroughDepth` |
| 4 | **`LazyTree` behind `TrackerTreeInterface`**, unfolding the resolved graph, memoized on `(edgeID, relevant bindings)`, validated by fixture diffing | `internal/spec` | Retires `MaxNodesPerTree` as a correctness hazard; per-route isolation for free; **no reordering anywhere** |
| 5 | **Relations cleanup** (chain/assignment links as tables) | falls out of step 4 | Deletes `processChainRelationships` hack and the reverse-`Parent` TODO |

### Progress

- **Step 1 — DONE 2026-07-09** (`internal/metadata/scc.go`): iterative
  Tarjan, callees-first `Components`, condensed `DAG`, `Recursive` flags;
  deterministic. cyclic_graph: 116 edges → 21 components, one 14-function
  recursion cluster. Not yet consumed by production code (step 3 is the
  first consumer).
- **Step 2 — DONE 2026-07-09** (`internal/callgraph` + engine wiring):
  `callgraph.Build(pkgs)` runs SSA (`InstantiateGenerics`) + VTA over the
  engine's own package load (mode extended with
  `NeedCompiledGoFiles|NeedTypesSizes`). `FunctionID` formats functions
  exactly like metadata `BaseID` (verified: every module-local metadata
  caller joins), generic instances collapse to their origin, closures are
  SSA-named (`pkg.parent$1`). `Reaches`/`ReachesID` walk with no depth
  bound. Wired behind `EngineConfig.ResolveCallGraph` (default off,
  exposed via `Engine.GetResolvedCallGraph`) — feeding nothing until
  step 3.
- **Step 3 — DONE 2026-07-09, first behavior-affecting change**
  (`internal/spec/reachability.go`): `maxWrapperLookThroughDepth` is
  **deleted**. The mux accessor query (`handlerReachesAccessor`) now uses
  `reachSet` — one bottom-up pass over the step-1 SCC condensation
  computing, per pattern, every function that transitively reaches a
  matching call; cached per pattern, no depth bound (first production
  consumer of `BuildCallGraphSCC`). Wrapper middleware look-through
  (`expandMiddlewareRefs`) now uses `middlewareMatchesThrough` — memoized
  per function, unbounded, still following closure-internal edges via
  `parentFnIndex` (which the call-graph SCC does not order — hence
  demand-driven memoization there instead of the bottom-up pass).
  Equivalence proven by the full golden-fixture suite; improvement locked
  in by new tests (12-deep helper chain and a recursion cluster both
  resolve; both were invisible under the old cap). NOTE: these summaries
  still run over the *metadata* (syntactic) graph — switching them to the
  resolved graph (step 2's) is deferred until the resolved graph is on by
  default, since it changes which indirect calls resolve.
- **Step 4 — IN PROGRESS 2026-07-09** (`internal/spec/lazytree.go` +
  `internal/spike/lazytree_diff_test.go`): `LazyTree`/`LazyNode` implement
  `TrackerTreeInterface`/`TrackerNodeInterface` as an on-demand unfolding.
  Nodes are per-path (one true parent, per-route isolation free); cycles
  cut by an ancestor-key check on the node's own path; traversals visit
  each key once globally (linear, not exponential); per-function edge
  lists memoized. The mutation overlays became **query-time relations**
  built once in `buildRelations`: `chainChildren` (chained calls),
  `receiverChildren` (calls on a variable, listed under its producer —
  with `claimed` edges removed from the plain caller expansion, mirroring
  the eager detach), and **param bindings** (router-passed-to-helper:
  callee's calls on the param hang under the argument's producer). Plus
  method-value handler resolution (`h.GetUsers` → method base key),
  interface→implementer fan-out (`ImplementedBy`), and closure expansion
  via `ParentFunctions`.
  **Parity (side-by-side full-mapper diff, `TestLazyTreeParity`): 10/12
  fixtures byte-identical** — mux, mux_path_params, chi, gin, echo,
  echo_handler_factory, fiber, servemux, wrapped_response,
  helper_response_body. The harness now ASSERTS identity for these (any
  regression fails the test). Two knownDiff entries remain, both
  understood:
  - `another_chi_router`: the same sub-router is mounted under two
    servers (goroutine closures both declare `r := chi.NewRouter()`);
    producer attribution is last-write-wins in BOTH trees and they pick
    different winners (eager `/api`, lazy `/ws/v1`). Converging means
    replicating the eager pass ordering — or fixing both to emit both
    mounts.
  - `complex_chi_router`: **LazyTree resolves more than eager** — DELETE
    `/api/user/{id}` 400 carries the `ErrorResponse` schema that is
    plainly in the handler code; eager emits it schema-less. Byte-parity
    here would mean reproducing an eager deficiency.

  Key mechanisms that got the last five fixtures (worth knowing for
  maintenance):
  - handler-factory args (`h.Create()`): a call-arg whose `Fun` is a
    selector resolves through the same method-key + implementer fan-out
    as method values, then `ParentFunctions` reaches the returned
    closure's body;
  - **chain children carry the CALL-SITE parent** (`processChainRelationships`'s
    rule, now explicit): `.Methods("GET")`/`.Use(mw)` stay visible under
    the chain parent for matchers, but `NewEncoder(w).Encode(v)` traces
    `v` through the enclosing call's `ParamArgMap` — parenting the chain
    copy under the chain parent had stolen the 200 slot with an
    untraceable body while the real body fell to `default`.

  **Default-tree status (2026-07-09, end of day): EAGER, lazy opt-in via
  `EngineConfig.UseLazyTracker`.** The default was briefly switched to
  lazy after the 10/12 fixture milestone, then runs against large
  codebases showed the fixture suite under-represents production wiring
  styles (such projects emitted only a fraction of their routes). The
  acceptance bar: **lazy must cover every wiring style the legacy tree
  supports.** Meters: `TestLazyTreeParity` (fixtures, asserts identity)
  and `TestTreeParityDirs` (env-gated `APISPEC_PARITY_DIRS`, diffs both
  trees on any local codebase).

  Fixed along the way (all kept): `MaxNodesPerTree` as cumulative node
  budget (cyclic_graph 60s→0.5s); interface-VALUE dispatch fan-out via
  `ImplementedBy` (functional_options routes); `assignIndex` — a
  byte-for-byte mirror of the eager `assignmentIndex` (same key
  composition incl. the selector-Lhs container override) consumed with
  eager-identical `TraceVariableOrigin` lookups at argument expansion,
  plus `producerArgs` step-through for option calls → `router_mount_options`
  now byte-identical (14-fixture harness: 11 identical, 3 knownDiff).

  ### Systematic tracker.go → lazytree.go mechanism inventory

  | # | eager mechanism | lazy status |
  |---|---|---|
  | 1 | roots (main) + RootAssignmentMap | ✅ |
  | 2 | Callers expansion + skips + MaxChildren | ✅ |
  | 3 | generics IsSubset filter | ✅ |
  | 4 | recursion/node budgets | ✅ (cumulative budget) |
  | 5 | arg classification + nested call-arg expansion | ✅ |
  | 6 | chain relationships | ✅ (call-site parent rule) |
  | 7 | CalleeVarName receiver linking | ✅ (receiverChildren + claimed) |
  | 8 | variableNodes / ParamArgMap bindings | ✅ partial (param producers) |
  | 9 | ParentFunctions closure fallback | ✅ |
  | 10 | interface-method callee → ImplementedBy | ✅ |
  | 11 | method-value / factory-call args | ✅ |
  | 12 | attachReturnedClosureBody | ✅ (via 9+10) |
  | 13 | assignmentIndex exact-key producer links | ✅ (assignIndex) |
  | 14 | interfaceResolutionMap receiver resolution | ❌ not used in lazy |
  | 15 | eager traverseTree merge passes (order semantics) | ❌ by design (another_chi winner差) |

  ### Root cause of the large-codebase path losses — FOUND AND FIXED

  Both large-codebase failures had ONE root cause, not per-idiom gaps:
  **per-path node copies exploded through business-layer call diamonds**,
  draining `MaxNodesPerTree` before traversal reached later router wiring
  (the budget-exhaustion warning fired deep inside entity-mapping code).
  The eager tree never hits this because it shares node objects globally,
  so its cap counts graph-sized work; the lazy tree was counting
  path-sized work. Fix, mirroring eager semantics without shared mutable
  nodes:

  - `maxInstancesPerKey` (10): at most N per-path copies of the same
    callee ID; beyond that, further paths stop descending — the role the
    eager per-ID recursion cap plays. (Sharing the first instance instead
    was tried and rejected: it makes the tree cyclic and the extractor's
    recursion is unguarded.)
  - The node budget now counts **distinct keys** (first instances), the
    same unit as the eager shared-node cap.

  Result: full **path parity on both large local codebases** (66/66 and
  245/245, zero missing/extra).

  **Content-level investigation (2026-07-10):** two more root causes
  found and fixed —
  - the `existsInArgs` child-skip used `meta.Args` (keyed by
    position-stripped base ID), so one `foo(q.Get("x"))` anywhere
    suppressed every `Values.Get` call site project-wide, losing query
    params and response fragments; the lazy tree now indexes exact
    argument instance IDs and skips only true duplicates;
  - receiver-variable claiming was keyed by bare function name, colliding
    same-named methods (ten `list` methods each with `q`); now keyed by
    the caller's full BaseID.

  After those, ALL remaining content diffs (5 + 10 across the two
  codebases) are one class: the eager tree emits extra
  "status could not be determined" default fragments (junk text/plain
  strings, or request DTOs as response bodies) that the lazy tree
  correctly omits or resolves to a consistent domain type — lazy
  equal-or-better in every case, same category as complex_chi_router.

  **Performance (2026-07-10):** lazy analysis on the larger fixture
  codebase went 19.6s → 10.0s via three memoizations (TraceVariableOrigin
  per (var, caller); ExtractGenericTypes per key; GetTypeParamMap per
  node). Remaining gap vs eager (~2.4s) is the per-route subtree
  re-expansion that per-route value tracing pays for; the second codebase
  runs 2.1s lazy vs 0.7s eager. Profile-verified: the residual cost is
  extractor descent + pattern matching, not tree construction (1ms).

  **The lazy tree is now the production default**
  (`DefaultEngineConfig.UseLazyTracker = true`); `UseLazyTracker: false`
  selects the eager tree, and NewEngine deliberately does not merge this
  bool (an unconditional merge had made the escape hatch dead). Meters:
  `TestLazyTreeParity` (fixtures) and `TestTreeParityDirs`
  (`APISPEC_PARITY_DIRS`, per-codebase missing/extra/content-diff).
  Still open: `functional_options` and `another_chi_router` knownDiffs.

### Interim rule (codify now, costs nothing)

Until the redesign lands, route each query class to the right structure and
write it into `internal/spec/README.md`:

- **Boolean / reachability / completeness-sensitive** queries
  (middleware, security, "does handler reach `mux.Vars`") →
  `meta.Callers` bounded walks.
- **Value / shape** queries (request bodies, response schemas, generic
  specialization) → tracker tree.

This is descriptive of current reality and stops the next feature from
fighting the tree again.

---

## 10. Glossary

- **On-the-fly call-graph construction:** building the call graph *during*
  analysis, discovering indirect-call targets (interface dispatch, function
  values) as dataflow facts become available; the standard fix for "the
  syntactic graph doesn't contain the real edges." (CHA/RTA/VTA and k-CFA
  are points on this spectrum, cheapest → most precise.)
- **Worklist algorithm:** process items (edges, facts) from a queue; each
  new derivation may enqueue more work; terminates when the queue drains —
  the mechanical form of a fixpoint.
- **Fixpoint iteration:** re-running rules until results stop changing;
  terminates when the fact domain is finite and growth is monotone. A fixed
  pass order is a *truncated* fixpoint.
- **SCC (strongly-connected component):** maximal set of nodes each
  reachable from every other — i.e., a recursion cluster. Condensing SCCs
  yields an acyclic graph.
- **Reverse topological order:** visiting order where every callee precedes
  its callers; enables single-pass bottom-up analysis.
- **Function summary:** precomputed record of a function's externally
  visible facts, composed at call sites instead of re-analyzing the body.
- **IFDS / summary edges (Sharir–Pnueli; Reps–Horwitz–Sagiv):** classic
  framework for context-sensitive interprocedural analysis via per-procedure
  reusable summaries.
- **Context sensitivity:** distinguishing analyses of the same function by
  the context that affects the result (here: relevant type-param bindings) —
  and *only* by that.
- **Unfolding:** expanding a DAG into the tree of all its paths; the source
  of the exponential blow-up the eager tracker pays.
