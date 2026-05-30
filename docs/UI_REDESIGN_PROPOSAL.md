# apispecui Redesign — Design Proposal ("the imagination")

> Status: **proposal / for review**. No code yet. This document is the
> Phase 0 deliverable: it fixes the *what* and *how it should feel*
> before we touch the implementation. Read the mockups, push back, and
> once it's approved we implement in the phases at the end.

## Goals (from the request)

1. **Easy + fancy for first-time use** — proper onboarding, inline
   documentation, examples.
2. **apidiag loaded separately** — not inside an iframe.
3. **New "API Analysis & Insight" view** — maximize use of `metadata`
   and the tracker tree; diagrams for both *general* API insight and
   *per-endpoint* insight. (apidiag today is too hard to use.)
4. **Load config file from a location dialog** — like the open-project
   dialog.
5. **Open-project dialog** — fixed default size, resizable, scrollable.
6. **Left panel** — easier, fancier, with its own vertical scroll.
7. **Export to AI** — turn detected issues into a fix-ready prompt
   bundle (trace + source + config excerpt) so they're easy to resolve
   (§3e).

Plus a parallel workstream: **raise test coverage** (currently 49.3%).

---

## 0. Frontend stack: decision = Preact + htm (comparison kept)

**Chosen: Preact + htm, no build step** (details in the Decision box
below). This section keeps the comparison that led there. The trade-off
is specific to *this* project — a **Go CLI tool** whose UI ships inside
the binary via `go:embed`, installable with `go install` and no other
toolchain.

| Dimension | Vanilla (modular ESM) | Preact + htm (no build) | React + Vite (build step) |
|---|---|---|---|
| Toolchain to ship | **none** | **none** (ESM CDN / vendored) | npm + `vite build` in CI/release |
| `go install` still "just works" | ✅ | ✅ | ⚠️ needs prebuilt `dist/` committed or built |
| Component model / reactivity | hand-rolled | ✅ components + hooks | ✅ full ecosystem |
| Fit for the complex **insight view** | ok (more manual state) | **good** | best |
| Bundle size added | 0 | ~4 KB (preact+htm) | ~45 KB+ (react-dom) |
| Maintainability as UI grows | medium | good | best |
| Risk to release pipeline | none | none | **real** (build step, node version, lockfile) |
| Matches repo philosophy ("no toolchain") | ✅ | ✅ | ✗ |

### Decision (chosen) — Preact + htm

> **Decided: Preact + htm, no build step.** Components + hooks for the
> whole UI (not just the insight view), imported as a vendored ~4 KB ES
> module — `import { html } from './vendor/htm/preact.module.js'`. Still
> served from `embed.FS`; `go install` keeps working with zero
> toolchain. This gives us the React-style authoring model the insight
> view benefits from, without putting npm/Vite in the release pipeline.

### Recommendation (rationale kept for the record)

The original recommendation was vanilla-modular with Preact reserved for
the insight view; we've upgraded to Preact+htm everywhere since the
authoring win is uniform and the cost (a vendored module) is the same.

Reasoning:

- The single biggest cost of React here isn't React — it's **adding a
  Node build step to a Go tool's release**. Today anyone can
  `go install .../cmd/apispecui@latest` and run it. A Vite build means
  either committing a generated `dist/` (ugly diffs, drift) or wiring
  npm into the release/CI. That's a permanent tax on a project whose
  whole pitch is "point it at your module, done."
- React's value is concentrated in **stateful, component-heavy** UI. Of
  our surface, only the **insight view** is genuinely that. The config
  form is already form-driven; the viewers are iframes to
  Swagger/Redoc/Scalar; the diagram is Cytoscape.
- **Preact + htm** (htm = JSX-less template literals) gives ~90% of the
  React authoring experience as a **4 KB ES module import — no build
  step**. It's the components-and-hooks model without betraying the
  zero-toolchain rule — drop-in via a single
  `import { html, render } from './vendor/htm/preact.module.js'`.

So **the proposal below is written for Preact + htm** as the UI
framework throughout (shell, config, insight, dialogs), all served from
`embed.FS` with no build step. If you'd later want to commit to
React+Vite instead, the only structural change is adding
`cmd/apispecui/web/` (a Vite project) and a `make ui` step before
`go build` — say the word and I'll re-scope.

---

## 1. Information architecture (the shell)

Today everything is one screen: a 440px config sidebar + an iframe
viewer, with the call graph stuffed into the same iframe. We split the
app into **four top-level modes** on a left **icon rail**, so first-time
users see a clear map instead of a wall of tabs.

```
┌──────────────────────────────────────────────────────────────────────────┐
│  ▦ apispec     Project: ~/work/lmd-core  [📁]   ● ready   [ Generate ▸ ]   │  ← top bar
├──┬───────────────────────────────────────────────────────────────────────┤
│  │                                                                         │
│⚡│   ┌─────────────────────────┐   ┌───────────────────────────────────┐  │
│  │   │  (mode-specific left    │   │                                   │  │
│⚙ │   │   panel — own scroll)   │   │      (main content area)          │  │
│  │   │                         │   │                                   │  │
│◷ │   │                         │   │                                   │  │
│  │   │                         │   │                                   │  │
│⌕ │   └─────────────────────────┘   └───────────────────────────────────┘  │
│  │                                                                         │
│? │                                                                         │
└──┴───────────────────────────────────────────────────────────────────────┘
 rail
```

Icon rail (top→bottom):

| Glyph | Mode | What it is |
|---|---|---|
| ⚡ | **Start / Spec** | Onboarding for first run; after generate, the spec preview (Swagger/Redoc/Scalar). |
| ⚙ | **Configure** | The config form (today's left panel), reorganized. |
| ◷ | **Insight** | The NEW analysis dashboard + per-endpoint insight. |
| ⌕ | **Call graph** | Opens apidiag (`/diagram`) **in a new tab**. |
| ? | **Docs / Help** | Inline documentation, examples, integrate guide. |

- The rail is always visible; the **left panel content changes per mode**
  and has `overflow-y:auto` (its own scrollbar), independent of the main
  area.
- Rail is collapsible (⟨/⟩) to widen content on small screens.

---

## 2. First-time experience (Start mode)

Before any spec exists, **Start** mode shows a guided, friendly empty
state instead of a blank iframe.

```
┌───────────────────────────────────────────────────────────────────┐
│   Generate an OpenAPI spec from your Go code — in 3 steps           │
│                                                                     │
│   ①  Pick your project            ②  Confirm framework             │
│      ┌───────────────────────┐       Detected: chi  ▾  (auto)       │
│      │ ~/work/lmd-core   [📁] │       66 routes found               │
│      └───────────────────────┘                                      │
│                                                                     │
│   ③  Generate                                                       │
│      [ Generate spec ▸ ]    …or load an existing config  [⤓ Load]   │
│                                                                     │
│   ┌─ What APISpec does ────────────────────────────────────────┐   │
│   │ Walks your call graph from route registration to the real  │   │
│   │ handler and infers request/response types from actual code │   │
│   │ — struct tags, literals, generics. No annotations needed.  │   │
│   │ ▸ See a worked example   ▸ Supported frameworks            │   │
│   └────────────────────────────────────────────────────────────┘   │
└───────────────────────────────────────────────────────────────────┘
```

- Inline expandable "What APISpec does" + a worked example (the
  `testdata/` order/customer sample rendered as a mini before/after).
- Tooltips (`?` chips) on every config concept on first run; dismissable
  "got it" so power users aren't nagged.
- After a successful generate, Start mode becomes the **spec preview**
  (Swagger/Redoc/Scalar switcher — unchanged behavior, still iframes,
  since those are third-party viewers).

---

## 3. ◷ Insight mode — the centerpiece ("imagination")

The whole point: **apidiag dumps the raw call graph (thousands of nodes,
no route context) — that's why it's hard.** Insight mode is
**API-centric**: it starts from your *routes*, explains *how apispec
resolved each one*, and **grades each endpoint with the feasible
data-flow metrics** (§3d) — all with diagrams scoped to a single
endpoint.

Two sub-views, toggled at the top: **Overview** (whole API, with
aggregate metrics) and **Endpoint** (one route, with its metrics + trace
diagram).

> **Display convention — `{max}+`.** Several metrics (call-path count,
> fan-out, propagation depth, chain depth) are bounded by the engine's
> `TrackerLimits`. When traversal hit the cap, we **never show a
> misleading exact number** — we show `{max}+` (e.g. `1000+` paths,
> `15+` hops) and tag it "limit reached." The insight backend returns a
> `truncated: true` flag per metric so the UI can render the `+` and a
> tooltip ("raise --max-nodes / --max-recursion-depth to resolve fully").

### 3a. Insight ▸ Overview (general API insight)

```
┌─ Insight ───────────────────────────  [ Overview ] [ Endpoint ]  ──┐
│                                                                     │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────────────────┐   │
│  │ 66       │ │ 5        │ │ 92%      │ │ Resolution health     │   │
│  │ routes   │ │ tags     │ │ resolved │ │ ███████████████░░ 92% │   │
│  └──────────┘ └──────────┘ └──────────┘ └──────────────────────┘   │
│                                                                     │
│  Methods                    Status codes            Content types   │
│  GET  ████████████ 38       200 ███████ 60          json ███████ 64 │
│  POST ██████ 18             400 ███ 22              form  █ 2        │
│  PUT  ██ 6                  404 ██ 14                                │
│  DEL  █ 4                   500 █ 9                                  │
│                                                                     │
│  ┌─ Needs attention ──────────────────────────  [ ⤴ Export to AI ]┐ │
│  │ ⚠ 3 routes have unresolved response types                       │ │
│  │     POST /payment/sodexo  →  data: object (type not found)  [⤴] │ │
│  │ ⚠ 2 routes synthesized dynamic path params                      │ │
│  │ ⚠ 1 route has no response body detected                         │ │
│  │ ⓘ 4 wrapper/envelope responses specialised (data $ref)          │ │
│  └──────────────────────────────────────────────────────────────────┘│
│                                                                     │
│  Endpoint complexity (heuristic, §3d)     Hotspots (worst-graded)   │
│  A ████████ 31   B ██████ 22              ▸ POST /payment/charge  D  │
│  C ███ 9         D █ 4                     ▸ GET  /order/search   D  │
│                                            ▸ POST /payment/sodexo D  │
│  Aggregate flow:  avg param-depth 3.4 · max 15+ (limit)             │
│                   max mutations 7 · pointer:value 38:62             │
│                                                                     │
│  Routes by tag (click a row → Endpoint view)                        │
│  ▸ /payment   24 routes   ███████████                               │
│  ▸ /auth      18 routes   ████████                                  │
│  ▸ /order     14 routes   ██████                                    │
│                                                                     │
│  Call-graph stats:  79 pkgs · 5,352 edges · deepest handler chain 11│
│  Most-referenced types: LmdResponse(31) · Address(12) · Phone(9)    │
└─────────────────────────────────────────────────────────────────────┘
```

Every number is **derived from data we already compute** (no new
analysis engine). Mapping to sources (from the metadata/RouteInfo
catalog):

| Card / chart | Source |
|---|---|
| routes, by method, by tag, by mount | `[]RouteInfo` (`.Method`, `.Tags`, `.MountPath`) |
| status-code & content-type histograms | `RouteInfo.Response[*].StatusCode/ContentType`, `Request.ContentType` |
| **resolution health %** | share of routes with no unresolved/placeholder/dynamic-param schema (we already detect `unresolvedExternalPlaceholder`, `DynamicParams`) |
| **complexity grades + hotspots** | per-endpoint efficiency score (§3d) bucketed A–D; "hotspots" = worst-graded routes |
| **aggregate flow** | avg/max of the Tier-1 per-endpoint metrics (§3d); `15+` shows the `{max}+` convention when a route hit `TrackerLimits` |
| "needs attention" list | routes with `Request==nil`, empty `Response`, `DynamicParams != nil`, schemas containing placeholders; wrapper specialisations (`allOf`+`data`) |
| call-graph stats | `len(meta.Packages)`, `len(meta.CallGraph)`, deepest tracker-tree path |
| most-referenced types | histogram over `RouteInfo.UsedTypes` / `$ref` targets |

The "Needs attention" panel and the complexity hotspots are the real
value: they turn the dangling-ref class of bug (the one we just fixed)
and over-complex endpoints into a **visible, navigable** list instead of
a Redoc crash or a guess.

### 3b. Insight ▸ Endpoint (per-endpoint insight)

Pick a route (search/typeahead or click from Overview). This is the
replacement for "open apidiag and squint at 3,000 nodes."

```
┌─ Insight ▸ Endpoint ───────────────────────  [ Overview ] [ Endpoint ]┐
│  GET  /payment/transactions/{actionID}                 tag: /payment   │
│  handler: PaymentHandler.ListTransactionsByActionID                    │
│  payment/http/handlers/transaction.go:21                               │
│                                                                        │
│  ┌─ Request ──────────────┐  ┌─ Parameters ───────────────────────┐   │
│  │ (none — GET)           │  │ actionID   path   string   required │   │
│  └────────────────────────┘  └─────────────────────────────────────┘  │
│                                                                        │
│  ┌─ Responses ─────────────────────────────────────────────────────┐  │
│  │ 200  application/json   allOf[ LmdResponse, {data: ListTxnResp} ] │  │
│  │      ↳ resolved at common.RespondWithSuccess(...)  transaction.go:51 │
│  │      ↳ payload recovered via wrapper specialisation:              │  │
│  │          mappedResp  ► NewEnvelope(data)  ► Envelope.Data          │  │
│  │ default  application/json   LmdHTTPError                          │  │
│  └───────────────────────────────────────────────────────────────────┘│
│                                                                        │
│  ┌─ Resolution trace (this endpoint only) ──────────────────────────┐ │
│  │                                                                   │ │
│  │   ListTransactionsByActionID ●                                    │ │
│  │        │ calls                                                    │ │
│  │        ▼                                                          │ │
│  │   uc.ListTransactionsByActionID ○ (returns []EPaymentTransaction) │ │
│  │        │                                                          │ │
│  │        ▼                                                          │ │
│  │   RespondWithSuccess ◆  ── writes 200 ──┐                         │ │
│  │        │                                │ data arg = mappedResp   │ │
│  │        ▼                                ▼                         │ │
│  │   json.Encode ◆                  dtos.ListTransactionResponse ◉   │ │
│  │                                  (= the data $ref)                │ │
│  │                                                                   │ │
│  │   ● handler  ○ callee  ◆ response sink  ◉ resolved payload type   │ │
│  └───────────────────────────────────────────────────────────────────┘│
│                                                                        │
│  ┌─ Metrics — Complexity B (heuristic, §3d) ────────────────────────┐ │
│  │  call fan-out ...... 3.2 avg / 11 max        ███░░  moderate      │ │
│  │  call-paths ........ 18                        ██░░░  ok          │ │
│  │  param depth ....... 4 hops                    ██░░░  ok          │ │
│  │  max mutations ..... mappedResp ×4             ███░░  watch       │ │
│  │  pointer:value ..... 9 : 14                    ░░░░░  info        │ │
│  │  unresolved types .. 0                          ░░░░░  clean      │ │
│  │  ⚑ Taint (opt-in, Phase 6): no request→sink without a sanitizer  │ │
│  └───────────────────────────────────────────────────────────────────┘│
│  ⚠ Warnings: none.  ⓘ Transactions []any → items: object  [⤴ to AI] │
└────────────────────────────────────────────────────────────────────────┘
```

(A deeply-nested endpoint would instead read e.g. `call-paths 1000+`,
`param depth 15+` — the `{max}+` convention — with a "limit reached" tag.)

### 3e. Export to AI — resolve issues fast

Most "needs attention" items have a small fix — but the fix is often *not
obvious*: is it a code change, or a missing `externalTypes` /
`typeMapping` / function `override` in the apispec config? Because we
already hold the **resolution trace**, the **source positions**, and the
**expected-vs-actual**, we can hand an AI a complete, self-contained
repro and ask it for the smallest fix.

**Two scopes** (mirroring the views): a per-issue/per-endpoint export
(the `[⤴]` on a row / the Endpoint `[⤴ to AI]`) and a whole-API export
(`[⤴ Export to AI]` on the Overview panel — bundles every open issue).

**What the bundle contains** (generated locally; nothing is sent
anywhere by us — see privacy note):

```markdown
# apispec — issue report (1 of 3)
Route:   POST /payment/sodexo        Handler: PaymentHandler.Sodexo
Source:  payment/http/handlers/sodexo.go:42

## Problem
Response payload type could not be resolved → `data` rendered as a
generic `object`. apispec expected a named struct for the 200 body.

## How apispec resolved it (trace)
PaymentHandler.Sodexo  (sodexo.go:42)
  → common.RespondWithSuccess(w, msg, resp, 200)   (sodexo.go:51)
  → resp = mapToGeneric(x)   — return type seen as `interface{}`  ◉ unresolved

## Relevant code (pulled from your source via positions)
```go
// payment/http/handlers/sodexo.go:42-58
func (h PaymentHandler) Sodexo(...) { ... resp := mapToGeneric(x); RespondWithSuccess(w, "ok", resp, 200) }
// payment/http/mapper.go:NN
func mapToGeneric(...) interface{} { ... }
```

## Current apispec config (excerpt)
```yaml
externalTypes: []
typeMapping: []
```

## What I need
Suggest the **smallest** fix, and give it to me verbatim. Either:
- (A) Go code: a concrete return type for `mapToGeneric`, or
- (B) apispec config: the exact `externalTypes` / `typeMapping` /
      `overrides` YAML entry to add.
Explain which you chose and why.
```

This is the payoff of the whole insight model: the same data that
*explains* an issue also *exports* it as a fix-ready prompt. For the
config-fix path it's near-closed-loop — the AI's YAML can be pasted
straight into the Full-YAML editor (§4) and re-generated.

**Formats & delivery:**
- **Copy to clipboard** (default) and **Download `.md`**.
- Optional **JSON** variant for programmatic/MCP use (issue + trace +
  snippets + config excerpt as structured data).
- Backend: `GET /api/insight/export?scope=all` and
  `?scope=endpoint&method=..&path=..&format=md|json`. It reads source
  snippets from disk using the metadata `Position` strings.

> **Privacy note (important).** The bundle **includes snippets of your
> source code and your config**. We only *generate and copy/download* it
> locally — we never transmit it. But when *you* paste it into a hosted
> AI, that source leaves your machine and may be retained by that
> service. The UI states this on the button and offers a **"redact
> identifiers"** toggle (replace package/type/func names with stable
> placeholders) for sensitive codebases. A future local-model / MCP path
> could keep it fully on-device.

Why this is *not* apidiag:

- It's the **tracker-tree subtree rooted at this one handler**, not the
  whole graph — a handful of nodes, not thousands.
- Nodes are **annotated with meaning**: which call wrote the status, which
  argument became the request body, where a type was recovered or left
  unresolved. (All available: `TrackerNode` + `CallGraphEdge.ParamArgMap`
  + the response/request matchers already tag these.)
- It directly answers the question users actually have: *"why did my
  endpoint get this schema?"* — the same question that produced your
  `ListTransactionResponse` report.

Diagram rendering: reuse **Cytoscape.js** (already vendored for apidiag),
but feed it a *route-scoped, annotated* subgraph from a new endpoint
endpoint (below), with a simple top-down layout. Small graphs → no
pagination, no pan-and-pray.

### 3c. Backend for Insight (small, reuses existing analysis)

The engine already holds `[]RouteInfo`, the `TrackerTree`, and
`Metadata` after a generate. We add two read-only JSON endpoints that
project that state — **no new analysis pass**:

- `GET /api/insight/overview` → counts, histograms, health, "needs
  attention" list, top types, call-graph stats.
- `GET /api/insight/endpoint?method=GET&path=/payment/...` → the route's
  request/response/params, the resolution annotations, and the
  route-scoped subgraph (Cytoscape nodes/edges) for the trace diagram.

These live in a small `internal/insight` package (recommended — testable,
feeds the coverage workstream). The route-scoped subgraph reuses the
**tracker tree** that `apidiag`/`diagserver` already build and already
turn into Cytoscape nodes (`visualization.go` →
`OrderTrackerTreeNodesDepthFirst`); insight just *filters* that to one
route's subtree and annotates it, rather than dumping the whole graph.

### 3d. Endpoint metrics — feasibility & how they're computed

These are the metrics surfaced in the Overview ("complexity grades",
"aggregate flow") and in the Endpoint metrics panel above. They're part
of the centerpiece — this subsection is just the honest accounting of
*which* are real and *how* each is computed.

You proposed a rich set of data-flow / parameter / taint metrics. They're
exciting, but their feasibility is decided by **one fact about our
engine**:

> **We build on `go/ast` + `go/types` (`types.Info`), not `go/ssa`.** The
> tracker tree is a **call graph with best-effort argument/assignment
> tracking** — *not* a control-flow graph (CFG) and *not* SSA. There are
> no basic blocks, no def-use chains, no liveness, no dominance, and no
> compiler escape analysis.

So metrics about **call-graph shape and argument propagation** are real;
metrics that need **intra-function control flow, liveness, or the
compiler** are heuristics at best. Here's the honest tiering. (Mutation
count is the standout freebie: `Function.AssignmentMap[var]` is already a
*slice* of assignments — `len()` is the overwrite count.)

### Tier 1 — reliable, cheap, ship in v1

| Metric | How (data we already have) |
|---|---|
| **Variable mutation count** | `len(fn.AssignmentMap[varName])` — direct. Flag vars assigned ≥N times. |
| **Call fan-out (branching factor)** | avg `len(node.Children)` over the route's tracker subtree. *Reframed honestly*: this is **call/argument fan-out**, not if/else branching (we don't model intra-fn control flow). |
| **Call-path count (path "explosion")** | count root→leaf paths in the route's tracker subtree (memoized DFS, bounded by existing `TrackerLimits`). *Reframed*: **execution-path fan-out through calls**, a proxy for "hard to reason about," not literal CFG paths. |
| **Parameter propagation depth** | follow a route input through `CallGraphEdge.ParamArgMap` chains down the tree; report max hops. This is *literally the walk the extractor already does* for body/response resolution. |
| **Pointer vs value ratio** | over the subtree's args, count `CallArgument.Type`/`ResolvedType` starting with `*` vs not. Descriptive. |
| **Handler/middleware chain depth** | `CallGraphEdge.ChainDepth` (already populated for fluent chains). |
| **Unresolved-type count** | args/fields with empty `ResolvedType` or schemas carrying the `unresolvedExternalPlaceholder`. Already detectable. |

### Tier 2 — feasible **heuristic**, label clearly (v1 lite / v2)

| Metric | Approach + caveat |
|---|---|
| **Assignment lifespan** | line-distance between a var's first assignment `Position` and its last referencing call-arg `Position` *within the same function*. Caveat: uses inside unmodeled expressions are missed → lifespan can read short. Surface as "approx." |
| **Re-assignment / copy overhead** | count assignments whose `Lhs`/`Value` are struct-typed idents/fields (struct→struct copies, field-by-field fills like the `mappedResp.Transactions = append(...)` pattern). Heuristic; informative, not exact. |
| **Unsanitized data paths (taint)** | model **source** = request-derived values (we already identify body/params), **sink** = calls whose callee matches a configurable regex (db/exec), **sanitizer** = configurable regex; flag a source that reaches a sink via `ParamArgMap` chains without crossing a sanitizer. Natural extension of the existing **pattern-config** system. **Caveat: AST-level taint is best-effort — false negatives through unmodeled flows.** Opt-in, clearly labeled "heuristic." |
| **Context leakage (secrets→log/error)** | same taint machinery; source = fields/vars matching a secret-name regex (or a `secret` struct tag), sink = logger/error-response calls. Same caveats. |

### Tier 3 — needs SSA / CFG / compiler — **defer or out of scope**

| Metric | Why it's not honest on our model |
|---|---|
| **Dead assignments** | needs liveness (def-use + CFG). Our "latest-wins" `AssignmentMap` can hint at *shadowing*, but true dead-store detection would mislead. |
| **Unused parameters** | same liveness problem → false positives where a param is used in an unmodeled expression. |
| **Escape-analysis predictor** | real escape analysis is the **compiler's** (`go build -gcflags=-m`). We could *optionally* shell out to it and parse the output as a separate, opt-in mechanism (v2+), but we can't infer it from the AST. |
| **True control-flow path explosion** | requires a CFG per function. Our call-path count is a *proxy*, and we label it as such rather than claiming CFG semantics. |

### The "Efficiency Score" (descriptive, not a verdict)

The **Tier-1** signals fold into a single per-endpoint grade (A–D),
rendered in the Overview complexity distribution/hotspots and in the
Endpoint metrics panel (mockups in §3a/§3b) — a weighted blend of
normalized *call-path fan-out*, *parameter propagation depth*, *max
mutation count*, and *unresolved-type count*. Any metric that hit
`TrackerLimits` contributes at its `{max}+` value and flags the grade as
a lower bound ("≥B"). It must be framed as a **heuristic
readability/complexity indicator**, never a correctness or performance
guarantee — anything stronger would over-claim given an AST (not SSA)
foundation.

**Proposed split:** Tier 1 + the efficiency score in the insight v1
(Phase 3/4). Tier 2 taint as an **opt-in, config-driven** feature in a
follow-up (Phase 6) — it's the highest-value Tier-2 item and slots
cleanly into the existing pattern config. Tier 3 explicitly out of scope
unless we later add an SSA pass (a much larger investment).

---

## 4. ⚙ Configure mode (left panel redesign)

Keep the powerful config, make it approachable.

- Replace the dense 14-tab strip with **grouped, collapsible sections**
  in a scrollable left panel (its own `overflow-y:auto`):

```
┌─ Configure ─────────────────┐
│ ▾ Basics                    │
│    API info                 │
│    Servers                  │
│    Security · Tags          │
│ ▸ Schemas                   │
│ ▸ Detection (framework)     │
│ ▸ Advanced                  │
│ ─────────────────────────── │
│ ⤓ Load config file…         │   ← new (see §6)
│ ⛁ Full YAML                 │
│ ↧ Save config.yaml          │
└─────────────────────────────┘
```

- Each section gets a one-line description + a `?` linking to Docs mode.
- "Detection" (framework patterns) is the scary part today → add a
  plain-language intro card and collapse the regex editors by default
  behind "Advanced pattern editing."
- Visual refresh via a **design-token CSS file** (`tokens.css`) extending
  the existing `:root` palette (`--bg/--panel/--accent/...`) with
  spacing, radius, shadow, and semantic tokens — so the whole app is
  consistent and re-themeable.

---

## 5. Open-project dialog — fixed size, resizable, scrollable

Current browse modal is fixed 640px / 78vh, **not resizable**.

- Give the modal a sensible **default size** and make it **resizable**
  via the native CSS `resize: both` on the modal box + `min-width/height`
  + `max-width/height: 92vw/90vh`, with the listing area
  `overflow:auto`.
- Persist the chosen size in `localStorage` so it reopens as the user
  left it.
- Keep keyboard nav (↑ parent, Enter to open) and the "projects with
  go.mod first" sort.

```
┌─ 📁 Open project ───────────────────────────┐
│ /Users/ehab/work                  [ Go ]     │
│ ┌──────────────────────────────────────────┐│
│ │ ↑ ..                                      ││  ← scrolls
│ │ 🟢 lmd-core         (go.mod)              ││
│ │ 📁 scratch                                ││
│ │ …                                         ││
│ └──────────────────────────────────────────┘│
│ [↑ Parent]            [Cancel] [Open folder] │
│                                          ◢   │  ← resize grip
└──────────────────────────────────────────────┘
```

---

## 6. Load **and save** config file — location dialog

Mirror the open-project flow, but for files — in both directions.

**Load config from a location:**

- New action **"⤓ Load config file…"** opens the same browse modal in
  **file mode**: it lists directories *and* `*.yaml` / `*.yml` files,
  files selectable.
- On choose → `GET /api/load-config?path=/abs/file.yaml` reads the file,
  parses it through the existing config parser (same one behind
  `/api/parse-config`), and **populates the form** + Full-YAML editor.
  Bad YAML surfaces inline, no crash.

**Save config to a specific location (new):**

- New action **"↧ Save config as…"** opens the same dialog in
  **save mode**: pick a directory, type a filename (default
  `apispec.yaml`), confirm.
- On confirm → `POST /api/save-config { path, yaml }` renders the current
  form to YAML (reusing `/api/render-config`) and writes it to the chosen
  path. We **confirm before overwriting** an existing file and refuse to
  write outside the allowed roots (same `validateProjectDir` guard the
  browse endpoint already uses), so the server can't be coerced into
  writing arbitrary locations.
- The existing "Save config.yaml" download button stays for the
  quick-download case; this adds explicit "write to disk here."

Both reuse `/api/browse` with a `&files=yaml` filter, so the backend
additions are small and share one dialog component (`browse.js`, with a
`mode: 'project' | 'open-file' | 'save-file'`).

---

## 7. apidiag — separate, not embedded

- The ⌕ rail item and the old "Call Graph" button **open `/diagram` in a
  new browser tab** (`target=_blank`, `rel=noopener`), removing it from
  the shared iframe entirely.
- apidiag itself is unchanged for now (still the power-user raw graph);
  the **Insight view** becomes the recommended, friendly path. We can
  later cross-link "open this endpoint in apidiag" from the Endpoint
  view.

---

## 8. Proposed file layout (Preact + htm, no build step)

All `js/*` modules are **Preact + htm** (components + hooks), imported as
ES modules; no build step, no JSX (htm tagged templates).

```
cmd/apispecui/assets/
  index.html              ← shell host: <script type="module" src="js/app.js">
  css/
    tokens.css            ← palette + spacing/radius/shadow tokens
    base.css              ← resets, typography, layout primitives
    components.css        ← buttons, cards, modals, badges, charts
  js/
    app.js                ← bootstrap: mounts <App/>, rail/mode router, shared signals/store
    api.js                ← fetch wrappers for /api/*
    config.js             ← <Configure/> mode (form ↔ YAML) component
    spec.js               ← <Spec/> Start/preview mode (viewer switching)
    insight.js            ← <Insight/> (overview + endpoint + metrics) components
    browse.js             ← <BrowseDialog/> (project + config open/save modes)
    docs.js               ← <Docs/> help, integrate guide, onboarding
    components/            ← shared Preact components
      charts.js            ← tiny SVG bar/donut/gauge (no dep)
      ui.js                ← Button, Card, Modal, Badge, etc.
  vendor/
    preact.module.js      ← Preact (vendored ESM, ~4 KB)
    htm.module.js         ← htm (JSX-less templates, ~1 KB)
    cytoscape.min.js      ← already used by apidiag; reused for insight
```

State is shared via Preact **signals** (or a small context store) in
`app.js` — no external state library.

Backend:

```
internal/insight/          ← NEW, unit-testable
  overview.go              ← []RouteInfo + meta → OverviewReport
  endpoint.go              ← one route → EndpointReport + scoped subgraph
  metrics.go               ← Tier-1 metrics + efficiency score
  export.go                ← issue → AI prompt bundle (md/json) + source pull
  (later) taint.go         ← Tier-2 opt-in taint (Phase 6)
```

Served straight from `embed.FS`; `main.go` gains a static handler for
`/assets/*` plus new endpoints: `/api/insight/overview`,
`/api/insight/endpoint`, `/api/insight/export`, `/api/load-config`,
`/api/save-config`.

---

## 9. Phased implementation plan

| Phase | Scope | Deliverable |
|---|---|---|
| **0** | *this doc* | Approved design + stack decision (**Preact+htm**) |
| **1** | Shell + theme on **Preact+htm**: `index.html`, rail, mode router, `tokens.css`/`components.css`, left-panel scroll, **resizable browse dialog**, **apidiag → new tab** | New look, no behavior lost; quick visible wins |
| **2** | Configure mode port: 14-tab form → grouped collapsible sections; onboarding/Start mode; Docs mode; **load + save config to a location** (`/api/load-config`, `/api/save-config`) | First-time-friendly config, file I/O |
| **3** | **Insight backend** (`internal/insight` + `/api/insight/overview`) with unit tests; **Insight Overview** UI incl. Tier-1 metrics + efficiency score; **whole-API "Export to AI"** | General API insight dashboard |
| **4** | **Insight Endpoint** UI + route-scoped Cytoscape trace (`/api/insight/endpoint`) + per-endpoint metrics panel; **per-endpoint/per-issue "Export to AI"** (`/api/insight/export`, md/json, redact toggle) | Per-endpoint insight + fix-ready exports |
| **5** | **Coverage push** (parallelizable): `internal/spec` (field_resolver, type_resolver, mapper helpers, wrapper_specialisation), `internal/metadata` (dependency_analyzer, call-graph traversal), `internal/engine` include/exclude; target 49% → 65%+ | Coverage raised; `internal/insight` adds testable surface |
| **6** *(opt)* | **Tier-2 taint** (`internal/insight/taint.go`): config-driven source/sink/sanitizer + secret-leak, clearly labeled heuristic | Security insight (opt-in) |

Phases 1, 5 are low-risk and can land first/independently if you want
momentum. Phases 3–4 are the heart of the "insight" ask. Phase 6 is
optional and gated on the Tier-2 confidence caveats.

---

## 10. Coverage workstream (summary; detail in Phase 5)

From the coverage analysis (49.3% total), prioritized:

- **internal/spec (46.6%)** — `field_resolver.go` (0%, pure string
  helpers — cheap), `type_resolver.go` resolvers, `mapper.go`
  `extractValidationConstraints`/`extractConstantValue`,
  `wrapper_specialisation.go` (new; mostly via the `testdata/wrapped_response`
  fixture we just added + targeted unit tests).
- **internal/metadata (59.2%)** — `dependency_analyzer.go` (entire file
  0%; pure helpers `contains`/`findCommonPrefix`/`isTestMockPackage`
  first, then a small multi-package fixture), call-graph traversal in
  `metadata.go` (hand-built `Metadata` graphs).
- **internal/engine (51.8%)** — `matchesPattern`/`shouldIncludePackage`/
  `shouldIncludeFile` (pure, table-driven tests).
- Building `internal/insight` as a package (not inline in `cmd`) means
  the new feature *adds* easily-tested surface rather than untested
  `cmd` glue.

Approach order: pure-function unit tests (cheapest) → constructed-metadata
tests → fixture-driven generation tests.

---

## Decisions locked

- ✅ **Stack**: Preact + htm, no build step (§0).
- ✅ **apidiag**: opens in a new browser tab (§7).
- ✅ **Config file**: both **load from** and **save to** a chosen
  location (§6).
- ✅ **Insight backend**: `internal/insight` package (testable).
- ✅ **Sequencing**: design-first (this doc) → phases 1–6.

## Open questions for you

1. **v1 insight depth**: Tier-1 metrics + efficiency score in v1 (Phase
   3/4), Tier-2 taint deferred to the optional Phase 6 — good? Or do you
   want taint pulled into v1?
2. **Efficiency-score framing**: agreed it must be labeled a *heuristic
   complexity/readability indicator* (not a perf/correctness verdict),
   given the AST-not-SSA foundation? If you want true escape analysis or
   CFG path counts later, that's a separate SSA investment (Tier 3).
3. **Data-flow overlay** (how one param/field flows through calls,
   highlighted on the trace diagram): in or out for v1? It's a moderate
   add on top of the route-scoped trace; I'd lean v2.
4. Anything in the mockups (esp. §3a/§3b and the metrics strip) that
   doesn't match your mental picture — now's the moment to redraw it.
