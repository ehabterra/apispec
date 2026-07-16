# Insight Dashboard Roadmap — From "What" to "Why"

> A plan to enrich the apispecui **Insight** dashboard (`cmd/apispecui`,
> `internal/insight`) with resolution-reasoning insights: how a type / route /
> status / body was actually resolved, and — when it wasn't — exactly where the
> chain broke. The organizing idea is to surface the **tracker-tree facts**
> (assignments, interface decisions, parameter bindings, fluent chains) that the
> pipeline already computes but the dashboard does not show.
>
> Written 2026-07-16, based on the state of `internal/insight/*`,
> `internal/diagserver/*`, `internal/spec/visualization.go`, and
> `cmd/apispecui/assets/js/*` at that time. Companion to `GAP_ANALYSIS.md`
> (correctness gaps) — this doc is about **observability of the pipeline's
> reasoning**, not about resolving more types.

---

## 1. The core reframe: the dashboard shows *what*, not *why*

Today the Insight dashboard answers two questions well:

- **"What did we produce?"** — the Overview is almost entirely derived from the
  generated OpenAPI spec (`insight.BuildOverview`, `internal/insight/overview.go:138`):
  route/method/status/content histograms, a resolution-health score, a
  "needs attention" issue list, component/type stats, security posture.
- **"How big is the graph?"** — the only metadata-derived block is
  `CallGraphStats` (`overview.go:412`): package/function/edge counts, edge
  composition, fan-in hot functions, busiest packages.

It does **not** answer the question every user of a static-analysis tool
actually asks: **"*Why* did this resolve the way it did — and where did it
break?"** That question is what turned every recent correctness investigation
(the lazy-tracker route drop / #146, the status `default`s / #144·#155, the
wrapper-decoded request bodies / #153) into a manual trace of `AssignmentMap`,
`ParamArgMap`, and `ChainParent`. The dashboard should make that trace
self-service.

The insight is structural: **the call graph shows *who calls whom*; the tracker
tree shows *how a value/type/route was resolved*.** The call graph is
well-covered by the current UI. The tracker tree — the structure apispec
actually walks to build the spec — and its facts are the untapped value.

## 2. What already exists (build on this, do not rebuild)

Three capabilities are already present and materially reduce the work:

1. **The per-endpoint resolution trace already walks the tracker tree.**
   `EndpointReport.Trace` (`internal/insight/metrics.go:90`) is built by
   `analyzeFromTrackerTree` (`metrics.go:270`) by default, and it already marks
   interface→concrete hops with `TraceNode.Resolved` (`metrics.go:71`) — the
   `⟐ impl` badge in the UI (`cmd/apispecui/assets/js/charts.js`). The substrate
   for "why" is there; it is under-labelled, not missing.

2. **The diagram server already has a full `tracker-tree` mode** with popups for
   argument type, resolved type, root assignments, generics, and per-call-path
   parameter/generic values — `CytoscapeNodeData` fields `ArgType`,
   `ArgResolvedType`, `RootAssignments`, `Generics`, and `CallPathInfo`
   (`internal/spec/visualization.go:57-98`). **But apispecui hard-codes
   `DiagramType:"call-graph"`** (`cmd/apispecui/main.go:311`), so the "Call
   graph ↗" tab never renders the tree or any of those resolution facts. A mode
   toggle unlocks work that is already written.

3. **The insight layer already holds the full `*metadata.Metadata`** (both
   `/api/insight/overview` and `/api/insight/endpoint` cache `s.currentMeta`,
   `cmd/apispecui/main.go`). It consumes only a thin slice today, so most new
   insights are **new reads over data already in hand**, not new analysis.

## 3. The facts we compute but do not surface

Every item below is present and rich in `internal/metadata`, serialized in the
metadata goldens, and read by *no* dashboard endpoint (some are read only by the
separate diagram viewer). Source: the metadata-fact inventory + insight
data-model audit.

| Fact | Struct (file:line) | Dashboard today | Diagram viewer |
|---|---|---|---|
| Assignment provenance | `Assignment` (`internal/metadata/types.go:518`); `AssignmentMap` on `Function`/`Method`/`CallGraphEdge` (`types.go:470`, `449`, `981`) | **No** | count-only (`RootAssignments`) |
| Interface impl set | `Type.ImplementedBy` / `Implements` / `Embeds` (`types.go:403-408`) | badge only (`Resolved`) | via trace |
| Param→arg binding | `CallGraphEdge.ParamArgMap` (`types.go:984`) | **No** | yes (popup) |
| Generic instantiation | `CallGraphEdge.TypeParamMap` (`types.go:985`) | **No** | yes (popup) |
| Fluent-chain shape | `CallGraphEdge.ChainParent`/`ChainRoot` (`types.go:991-992`) | depth scalar only | indirect |
| Verb dispatch arms | `Function.MethodDispatch` / `MethodBranch` (`types.go:476-482`) | **No** | **No** |
| Return-value origins | `Method`/`Function.ReturnVars` (`types.go:446-467`) | **No** | **No** |
| External-type facts | `Metadata.ExternalTypes` / `ExternalTypeFact` (`types.go:169-191`) | string-marker only | **No** |
| Go signatures | `SignatureStr` / `Signature` (`types.go:434-457`) | **No** (uses spec schema summary) | partial |

The four highest-value untapped facts — the ones this roadmap is built around —
are **assignments, interface impl sets, param/generic bindings, and chain
shape**, i.e. exactly the categories named in the original request.

## 4. The insights to add

The unifying deliverable is a **Resolution ("Why") view**: the tracker tree for
an endpoint, where every node explains *how* it resolved and every failure is a
clickable dead-end with a jump-to-source. The four fact categories are four
overlays on that view; §4.5 adds whole-API aggregates.

### 4.1 Value provenance (assignments)

**Show:** for a resolved (or *unresolved*) request body / response schema /
status, the assignment chain that produced it —
`var req X` → `decodeJSON(w, r, &req)` → param `dst` → `dec.Decode(dst)`.
Render as an ordered provenance list per node, with each hop's `file:line`.

**Why it's crucial:** this is exactly the hand-trace behind #153 (request body
lost through a variable-receiver decoder). It converts "the body is a generic
`object`" into "resolution stopped *here*, at this assignment," which the user
can act on.

**Data:** `AssignmentMap` (`types.go:470/449/981`), `Assignment{VariableName,
Value, CalleeFunc, CalleePkg, ReturnIndex}` (`types.go:518`);
`BuildAssignmentRelationships` (`internal/metadata/metadata.go:466`) and
`traceVariableOriginHelper` (`internal/metadata/analysis.go:506`) already do the
walk internally.

**Prerequisite:** `Assignment.ReturnIndex` is hardcoded to `0`
(`metadata.go:1428`), so multi-return provenance (`h, err := factory()`)
mis-binds to the first return. Fix this before the provenance panel can be
*correct* for multi-return producers. Small change; own PR + fixture.

**Effort:** medium (new insight-layer plumbing + a UI panel). Prerequisite is
small but real.

### 4.2 Interface decisions

**Show:** at each interface→concrete hop, *which* implementation was chosen and
the **alternatives** (`ImplementedBy`). When the set has >1 concrete impl and
the type was kept general (erased to `any`), **flag it**: "resolved to `any` —
2 concrete types assigned to this interface."

**Why it's crucial:** it surfaces golden-rule-#7 honesty *at the exact point it
cost precision*, so the user can disambiguate via config instead of guessing why
a payload is generic. It also makes the existing `⟐ impl` badge explain itself.

**Data:** `Type.ImplementedBy` is a slice that honestly keeps *all* impls
(`types.go:403`), so the ambiguity flag is a straight read — no new resolution
logic. Forward `Implements` and `Embeds` (`types.go:405-408`) enable an
interface-inventory card (§4.5).

**Effort:** low–medium (data is a direct read; UI is a panel + a badge state).

### 4.3 Parameter / generic type-flow

**Show:** an overlay on the endpoint trace where edges are labelled param→arg
(`ParamArgMap`), so the user can watch a concrete type thread from a call site
through wrapper parameters to the encode/decode site; plus the concrete generic
instantiation (`TypeParamMap`) on each node.

**Why it's crucial:** the param binding *is* the request-body/status resolution
mechanism (the multi-hop `resolveArgThroughParams` path). Making it visible
turns a black box into an inspectable data-flow.

**Data:** `CallGraphEdge.ParamArgMap` / `TypeParamMap` (`types.go:984-985`).
**The diagram viewer already extracts both** — `extractParameterInfo` reads
`ParamArgMap` (`visualization.go:660`) and `TypeParamMap` feeds the generics
popup (`visualization.go:406`) — so extraction is proven trivial; this is
lifting it into the dashboard trace (or reusing the tracker-tree diagram mode).

**Effort:** low (extraction exists; wire to the dashboard).

### 4.4 Registration chains

**Show:** the fluent chain that decides route/middleware/security scope —
`r.With(mw).Get(...)`, `r.Group(func(r){ r.Use(auth); r.Get(...) })` — as a
small chain diagram per route (`ChainParent` → `ChainRoot`).

**Why it's crucial:** the SecurityCard shows the *outcome* (protected / public /
no-auth) but never *why*. Chain shape is how you debug "why is this route
unsecured / mis-prefixed" — the #146 class of bug — and how a reviewer verifies
that a group's middleware actually reaches its routes.

**Data:** `CallGraphEdge.ChainParent`/`ChainRoot` (`types.go:991-992`); the
LazyTree already indexes chain relations (`chainChildren`,
`internal/spec/lazytree.go:66`). Only the scalar `ChainDepth` reaches the
dashboard today.

**Effort:** medium (chain reconstruction for display + a small diagram).

### 4.5 Whole-API aggregates (Overview)

These lift the per-endpoint facts into API-level signals:

- **Resolution funnel + failure taxonomy.** Replace the flat "needs attention"
  list with *categorized* causes: external type, interface-erased-to-`any`,
  **status defaulted (dynamic/switch)**, wrapper-not-matched,
  request-body-unresolved — each linking to the exact nodes. A "status
  defaulted" bucket would have surfaced the #155 case at a glance instead of
  eyeballing 21 `default`s. Generalizes every gap we file.
- **Resolution confidence per endpoint** (beside the complexity grade): did
  body / status / params resolve via a *literal*, via *N-hop param threading*,
  or *default/fallback*? A "defaulted status" / "body = object" / "synthesized
  param" badge is the single highest-signal addition.
- **Interface inventory:** interfaces by implementation count, unimplemented
  interfaces, and ambiguous (multi-impl) interfaces — from `ImplementedBy` /
  `Implements`.
- **Verb-dispatch explainer:** where a `switch r.Method` split one handler into
  several operations (`MethodDispatch`, `types.go:476`) — read by no UI today —
  answers "where did this extra operation come from?" (golden-rule #8).
- **Generation diff / regression view.** Compare two runs (lazy vs legacy, or
  before/after a code change): which routes / statuses / bodies changed. This is
  the A/B done by hand in every validation session; valuable as a feature and as
  a release gate.

## 5. Phasing

Sequenced by value-to-effort, cheapest-first. Each phase is independently
shippable and testable.

**Phase 0 — unlock what exists (cheap, high value)**
- Add **resolution-confidence badges** to the endpoint report (defaulted status,
  generic-object body, synthesized param) — small `internal/insight` additions.
- **Failure taxonomy** on the Overview (re-bucket issues already collected in
  `collectOperationIssues` + a few metadata-derived causes).
- (The endpoint view already exposes a tracker-tree ↔ call-graph toggle
  (`insight.js` trace-source segment), so the tree substrate is already
  user-reachable there; wiring the separate `/diagram` page's `tracker-tree`
  mode (`cmd/apispecui/main.go:311`) is optional and lower priority.)

**Phase 1 — the differentiators**
- **Param/generic type-flow overlay** (§4.3) — extraction already exists.
- **Interface-decision panel** with alternatives + ambiguity flag (§4.2).

**Phase 2 — deeper provenance**
- Fix `Assignment.ReturnIndex` (§4.1 prerequisite), then the **value-provenance
  panel** (§4.1).
- **Verb-dispatch explainer** and **interface inventory** (§4.5).

**Phase 3 — bigger bets**
- **Registration-chain inspector** (§4.4).
- **Generation diff** view (§4.5).

## 6. Prerequisites & known data limitations

- **`Assignment.ReturnIndex == 0` (hardcoded, `metadata.go:1428`).** Multi-return
  provenance is lossy; must be fixed before §4.1 is correct.
- **Tracker tree is transient.** LazyTree nodes are expanded on demand and not
  serialized (`internal/spec/lazytree.go`); insights that need the tree build it
  via `cachedTrackerTree` (`internal/insight/metrics.go:624`), as the endpoint
  metrics already do. No new persistence is required, but the cost is a tree
  build per request — keep the existing cache.
- **Determinism.** Any new insight that ranges maps whose order can reach the
  API response must sort (golden rule #1). The existing insight code already
  does this (`sortedCounts`, stable issue sort); match it.
- **Layering.** Insights are read-only projections; keep `internal/insight`
  side-effect free (its package doc states this) so it stays unit-testable with
  hand-built inputs.

## 7. What not to do

- **Do not add more call-graph-shaped charts** (fan-in/fan-out variants). That
  dimension is well-covered; the marginal value is entirely in resolution
  reasoning, not more graph statistics.
- **Do not collapse the honest general type** to show a single "winner" when an
  interface has multiple impls. Surface the ambiguity (§4.2) — do not hide it
  (golden rule #7).
- **Do not duplicate the diagram viewer.** Where the diagram already extracts a
  fact (param bindings, generics), reuse that extraction rather than reimplement.

## 8. Open questions

- **Rendering model for the "Why" view:** extend the existing layered
  `TraceDiagram` (`charts.js`) with per-node provenance panels, or render the
  tracker structure as a literal collapsible tree widget? The trace diagram is
  proven and interactive; a tree widget may read more naturally for deep chains.
  (To be decided with the maintainer.)
- **Scope of the generation diff:** same-project two-run diff only, or persist a
  baseline spec for release-gate comparison?
- **Redaction:** the export flow already redacts identifiers
  (`internal/insight/export.go`); provenance panels expose more source
  structure, so confirm the redaction story extends to them before shipping any
  share/export of the "Why" view.
