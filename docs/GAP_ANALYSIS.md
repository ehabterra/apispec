# apispec — What's Important to Cover (Gap Analysis for Discussion)

*Generated 2026-07-07 from a full-repo sweep: architecture map, TODO/limitation hunt, test-coverage audit, and GitHub issue/PR history. For discussion — priorities are proposals, not decisions.*

## TL;DR — proposed priorities

| # | Item | Why it matters | Effort (guess) |
|---|------|----------------|----------------|
| 1 | **Deterministic output** (string pool, call-graph edge order, fiber/generic/operationId flips) | Blocks reliable golden testing, clean diffs, reproducible CI | Medium |
| 2 | ~~**Wire existing `testdata/` scenarios into `go test`**~~ ✅ DONE 2026-07-08 — all 11 orphaned fixtures now covered (see §3.1) | Regressions in whole frameworks currently only caught by manual `compare-spec.sh` | Low–Medium |
| 3 | **Fix `internal/spec/tests/*.yaml` fixture hygiene** (tracked-but-gitignored, mutated by every test run, embed absolute temp paths) | Constant dirty-tree noise; non-portable; hides real changes | Low |
| 4 | ~~**Mux path params → handler params**~~ ✅ DONE 2026-07-08 — `mux.Vars(r)["id"]` now wired (see §2.1) | Only framework in the support matrix with a hole in a core column | Medium |
| 5 | ~~**Generic types (parametric structs)**~~ ✅ DONE 2026-07-12 — `Page[User]`-style envelopes resolved when the instantiation is visible at the encode site (see §2.2) | README-declared partial; generics are mainstream Go now | High |
| 6 | **Interface-typed param resolution** | README-declared gap + `docs/INTERFACE_RESOLUTION.md` future-work list | High |
| 7 | ~~**Robustness regression fixtures for large/cyclic projects**~~ ✅ DONE 2026-07-08 — #10/#14 (recursive_types) + #20 (dense_graph) fixtures added; one open limitation noted (see §4) | Worst historical failures (hang, stack overflow, truncated output) have no regression tests | Medium |

---

## 1. Correctness & determinism (highest leverage)

### 1.1 Nondeterministic spec/metadata output — ✅ FIXED 2026-07-07

> Status: root causes were unsorted map iteration (string-pool interning order in `GenerateMetadataWithLogger`, `analyzeInterfaceImplementations` append order, raw-`token.Pos` anon-struct keys, `detectFrameworkType` first-match-wins, `generateSchemas` inline-vs-$ref order, first-match-wins package lookups). Guarded by `TestGenerateMetadataDeterministic` and `TestGenerateDeterministic`. Historical fiber/generic flips verified gone across 8 CLI runs.
- Observed on this branch: `internal/spec/tests/multipackage.yaml` diff (466 lines) is **pure reordering** — `string_pool` entries and call-graph `caller/callee` blocks shuffle between runs with identical content. Same for `example.yaml` type ordering.
- Known flips (from prior sessions): fiber responses, generic response resolution, operationId assignment vary between runs.
- Coverage percentages also fluctuate ±1–2% between identical runs — another symptom of order-dependent traversal.
- **Impact:** golden-file testing (item 2) is impractical until this is fixed; spec diffs between releases are noisy; users see churn in committed `openapi.yaml` files.
- **Direction:** sort at serialization boundaries (string pool, edges, paths, schemas) rather than chasing map-iteration order at every site. PR #93 (stop re-sorting tracker maps) and #85 (generic determinism) already started this — finish the sweep.

### 1.2 Silent degradations worth surfacing or fixing
- `internal/spec/extractor.go:515` — speculative middleware (`auth(h)`) using an **unknown auth library is silently ignored** — no warning, no diagnostic. Contradicts the unresolved-security diagnostics philosophy used elsewhere; should at least emit a diagnostic.
- `internal/metadata/metadata.go:1598` — `assignment.ReturnIndex = 0` hardcoded: multi-return assignments always bind the first return value. Wrong tracing for `h, err := factory()` style code paths.
- `internal/spec/tracker.go:1213` — functional-options parameter tracing uses a self-described "confusing, may not handle all cases" reverse-link workaround (relates to closed issue #38).
- Non-string map keys silently fall back to bare `object` (`schema_mapper.go:84`, `mapper.go:2030`) — fine as behavior, but could warrant a diagnostic.
- Many bare `object` fallbacks in `mapper.go` when a type can't be resolved — the insight report catches some of these; consider making "unresolved → object" count a first-class quality metric.

## 2. Declared feature gaps (README §Partial/Not-yet + docs)

Ordered by proposed value:

1. ~~**Gorilla Mux path params** detected but not wired into handler params~~ ✅ **DONE 2026-07-08** (§2.1 below).
2. ~~**Generic types** (parametric structs)~~ ✅ **DONE 2026-07-12** (§2.2 below). Function generics already worked; `Page[T]`-style response envelopes returned directly at the encode site now resolve to per-instantiation schemas.
3. **Interface-typed parameters** not resolved to concrete types. `docs/INTERFACE_RESOLUTION.md` lists the future work: automatic discovery, cross-package resolution, generic interfaces.
4. **Handler-factory pattern, part 2** — request-body-via-wrapper still pending (part 1, closure-returning routes, is done).
5. **Router passed as function parameter** (not via Mount) isn't traversed — known fiber `/products` gap; deferred feature.
6. **`dive` validator tag** (array-element validation) — README-declared "planned".
7. **Same path + same status, different schemas** — not supported (would need `oneOf` merge).
8. **HTTP method chosen via switch/if** around `net/http` Handle — not detected.
9. Conditional/dynamic runtime route registration — explicitly out of scope; keep it documented rather than attempt it.

Cross-cutting principle (worth restating in CONTRIBUTING or docs): **auth/security detection must stay framework-agnostic and config-driven** — every new detection feature should cover all six frameworks and all wiring styles (router-level, group, per-route, wrapper, var-assigned), not just the framework that prompted it.

### 2.1 Gorilla Mux path params — ✅ DONE 2026-07-08

> The support matrix is now fully green. Root cause: mux exposes path vars as a **map** (`vars := mux.Vars(r); id := vars["id"]`), so the name is a map key, not a call argument — the arg-index `ParamPattern` mechanism (which works for `chi.URLParam(r,"id")`, `c.Param("id")`, `r.PathValue("id")`) couldn't reach it. Two visible symptoms: a bogus `net/http.Request` path param (the `Vars` call's request arg misread as a name) and the real `{id}` left flagged *"present in path but not found in the code"*.
>
> Fix (config-driven, keeps the engine framework-agnostic): added a `ParamPattern.NameFromMapKey` flag and set it on mux's `Vars` pattern. Per route, `completeMapKeyPathParams` emits one clean path parameter per `{placeholder}` in the route path **when the handler reaches the accessor** — determined by call-graph reachability (`handlerReachesAccessor`, bounded depth), so direct, inline (`mux.Vars(r)["id"]`), and **helper-wrapped** access (`id := readParam(r, "id")` where `readParam` wraps `mux.Vars`) all resolve. Names are authoritative from the path template (robust to every access form — assignment, blank `_ =`, inline call arg, dynamic key in a helper). Routes whose handler never reaches `Vars` still fall through to the warned synthesis, matching chi/gin/etc.
>
> Two follow-on fixes landed with it:
> - **Regex-constrained params** (`{id:[0-9]+}`, mux/chi): `convertPathToOpenAPI` now strips the regex to `{id}` (OpenAPI paths can't carry a regex — previously the param was dropped entirely and the path was invalid) and surfaces the constraint as a schema `pattern`.
> - **Key-mismatch diagnostic**: reachability wires `{id}` clean whenever the handler *reaches* `mux.Vars`, but it can't tell whether the code reads the *right* key. `recordPathVarKeyMismatches` recovers the literal keys actually read — via the assignment tracker (`vars := mux.Vars(r); vars["id"]`, tagged `CalleeFunc`/`CalleePkg`) and inline `mux.Vars(r)["id"]` — and reports any key with no matching placeholder (e.g. `mux.Vars(r)["userId"]` on `/users/{id}`, an always-empty read). Surfaced as a `[path-params]` CLI warning and programmatically via `Engine.GetPathParamMismatches()` / `Generator.PathParamMismatches()` (for the UI). Dynamic keys and keys passed into helpers aren't recovered — the diagnostic errs toward zero false positives.
> - Guarded by `TestTestdata_MuxPathParams` (direct), `TestTestdata_MuxAdvancedPathParams` (regex + helper indirection + unread-placeholder-stays-warned), and `TestTestdata_MuxPathParamKeyMismatch` (typo key). Other frameworks' path params verified unchanged; no golden drift.

### 2.2 Generic types (parametric structs) — ✅ DONE 2026-07-12

> A response envelope instantiated with concrete arguments at the encode site — `json.NewEncoder(w).Encode(Page[User]{…})` — now resolves to its own component with the type argument substituted into the parametric fields, instead of collapsing every instantiation onto a single placeholder (`Page_T-any`) whose payload pointed at an empty `T-any` object.
>
> Root cause: the concrete argument *was* captured in metadata (a composite literal's type expression `Page[User]` is an `IndexExpr` whose index child is the `User` ident), but the spec layer's `callArgToString` rendered the index node as the bare **declaration** `Page[T any]` and dropped the argument. So `Page[User]` and `Page[Product]` both stringified to `Page[T any]` and shared one schema; the payload field (`Data T` / `Items []T`) mapped to a `T-any` stand-in and the concrete structs were never emitted.
>
> Fix (spec layer + one metadata field, contained blast radius):
> - `metadata.processTypeSpec` now records a generic type's declared parameter names (`Type.TypeParams`, e.g. `["T"]`) — previously only functions carried this.
> - `context_provider.callArgToString` renders a composite-literal generic instantiation as `Base[Arg1,…]` carrying the **concrete** arguments (stripping the base's own declaration brackets; reducing each argument to its simple type name so the bracketed form parses through `TypeParts` and sanitizes to a valid component name; `interface{}` → `any` so no illegal `{}` leaks into a `$ref`).
> - `mapper.generateStructSchema` zips the declared parameter names (`typ.TypeParams`) positionally with the concrete arguments off the key and substitutes them into each field, preserving slice/pointer markers (`Items []T` → `[]User`, `Data T` → `User`). `findTypesInMetadata` skips the concrete argument as a top-level entry (it is emitted through the parametric struct's field resolution) so a single `goType` still maps to exactly one non-nil type — avoiding a non-deterministic shadow where `Page[User]` could resolve to `User`.
> - Guarded by `TestTestdata_GenericStructs` (`Page[User]`/`Page[Product]` distinct, `Items`→`[]$ref`, `Envelope[User].data`→`$ref User`, `User`/`Product` emitted, no `_T-any` placeholder) + a structural row in `TestTestdata_Frameworks`. Fixture: `testdata/generic_structs/`.
>
> **Known limitation (deferred, related to §2 item 4).** When the concrete argument only exists behind a helper that erases it to `interface{}`/`any` — e.g. the existing `testdata/generic` fixture's `respondWithSuccess(w, data any)` writing `APIResponse[any]{Data: data}` — the payload still renders as a generic object. `T` is genuinely bound to `interface{}` at the encode site; recovering the route's real `TResponse` requires interprocedural type-argument threading through the helper boundary (the same shape as handler-factory part 2 / wrapper specialisation). That fixture's output improved as a side effect (the junk `APIResponse_T-any` + dangling `T-any` component became a clean `APIResponse_any` with an inline `object` `data`).

## 3. Testing gaps

### 3.1 Orphaned testdata scenarios (cheap, high-value) — ✅ DONE 2026-07-08

> Status: the 11 previously-untested fixtures are now wired into `go test`. `generator/testdata_frameworks_test.go` (`TestTestdata_Frameworks`) covers `chi`, `fiber`, `gin`, `mux`, `generic` — structural assertions (expected routes + methods present, no dangling `$ref`, no unresolved placeholders), loading each fixture's committed `used-config.yaml` when present so it matches the `compare-spec.sh` snapshot. `generator/testdata_auth_test.go` (`TestTestdata_AuthPresets`) covers the 6 auth fixtures (`auth_chi_with`, `auth_echo_group`, `auth_fiber_group`, `auth_gin_perroute`, `auth_mux_subrouter`, `auth_nethttp_wrap`) — each asserts the golang-jwt import auto-applies `bearerAuth` to the guarded route and leaves the sibling open route untouched, one row per framework wiring style (route/subtree/per-route/router/wrapper scope).

Original finding (kept for context): 27 scenario dirs existed under `testdata/`, but automated tests only exercised ~14. **No `go test` references at all** for the 11 dirs above; they were only reachable via the manual `scripts/compare-spec.sh` flow. The structural-assertion style already used in `generator/testdata_smoke_test.go` (paths present, no dangling `$ref`, no unresolved placeholders) was the right template — now extended over these dirs.

### 3.2 Fixture hygiene: `internal/spec/tests/*.yaml`
These are not golden files — `metadata_test.go:802` *writes* them on every run as dev-inspection artifacts. They're tracked in git but also listed in `.gitignore`, so every `go test` run dirties the tree (CI's coverage workflow literally runs `git restore internal/spec/tests/` to cope). They also embed the absolute temp path of the machine that last ran tests. Options: (a) `git rm --cached` them and keep them purely local, (b) write them to `t.TempDir()`, or (c) sanitize paths + sort output and promote them to real golden files once 1.1 lands.

### 3.3 Coverage cold spots
- `cmd/apispecui` — **zero tests** for 1,845 lines (config editor, preview, diagram endpoints). Recent feature work (#91, #75/#76) keeps landing here untested. Even handler-level HTTP tests would help.
- `internal/engine` 58.3% — the orchestrator; package filtering/framework-dependency logic had a past silent-ignore bug (`engine.go:298`), which argues for more filter tests.
- `internal/metadata` 64.1% — the AST layer, second-largest package.
- `internal/spec/tracker_test.go:1236,1360` — assertion-light ("verify the code path exists") tests on the most complex file in the repo.

### 3.4 CI gates
PRs get build (3 OSes) + `go test -race` + lint/vet/gofmt — good. But the only coverage threshold (45%, in `scripts/update-coverage-badge.sh`) runs **post-merge on main** as part of the badge job. Consider running the same library-scoped check as a PR gate, and raising the floor toward the actual ~70%.

## 4. Robustness (lessons from closed issues)

All 8 historical issues are closed and there are zero open issues/PRs, but the worst failure modes have no regression fixtures:

- **#10 stack overflow** / **#14 truncated output** — ✅ DONE 2026-07-08. `testdata/recursive_types` + `TestTestdata_RecursiveTypes` (`generator/testdata_robustness_test.go`) exercises three cycle shapes: a directly self-referential `TreeNode` (via both `*T` and `[]*T`), a mutually-recursive `Category`↔`Product`, and a three-hop `Graph`→`Edge`→`Node`→`Graph`. Asserts every cycle closes as a `$ref` to a registered component (no infinite inline expansion, no dangling ref) — a stack-overflow/hang regression means the test never returns.
- **#20 hang on scan** (Echo + swaggo, 23 endpoints, limits set) — ✅ DONE 2026-07-08 (realistic scale). `testdata/dense_graph` + `TestTestdata_DenseGraphBounded` models the shape (25 handlers fanning into a shared service→repo→leaf layer, dense fan-in, bounded depth) and asserts generation finishes within a generous 60s wall-clock budget (local: ~0.4s). A regression that reintroduces unbounded traversal for realistic graphs trips the timeout instead of hanging CI.
  - ✅ **Also fixed — pathological dense *cyclic* graphs (2026-07-08).** The root cause was in `NewTrackerNode`: the `MaxNodesPerTree` safety brake gated on `len(visited)`, but `visited` is a per-path recursion-*stack* counter (incremented on enter, decremented on exit), so its size is the current stack depth, never the total work. A dense/cyclic graph re-expands shared callees along exponentially many distinct paths while keeping stack depth small, so the brake never fired and generation ran effectively forever. Fixed by adding a cumulative `tree.nodesBuilt` counter and gating the cap on that instead; once the cap is hit every further call returns a leaf stub and the recursion unwinds cheaply. `testdata/cyclic_graph` + `TestTestdata_CyclicGraphBounded` locks it in (12 strongly-connected functions with modular back-edges; was >45s / no truncation, now ~1.3s with a single truncation warning, all 12 routes still recovered). Every real fixture builds ≤1.9k nodes, far under the 50k cap, so no existing output changes.
- **#34 debuggability** — a user couldn't tell *why* routes were missed in a real project. The insight report and diagnostics exist; a documented "my route is missing — how to debug" section (using `--write-metadata`, the diagram, insight output) would close the loop.

## 5. Housekeeping (quick wins)

- Delete `README.md.bak` (stale 33k copy).
- Remove committed root-level artifacts: `coverage*.out/txt/html`, `profiles*/`, and decide the fate of `playground_optimized.go` (0% coverage, gitignore-patterned but present).
- Resolve the tracked-vs-gitignored contradiction for `internal/spec/tests/` (see 3.2).
- `docs/AUTH_DETECTION_DESIGN.md:319` says a phase is "not yet called from traversal" while marking it DONE below — reconcile the doc.
- Verify the package READMEs linked from the main README (`cmd/apispec/README.md`, `internal/spec/README.md`, etc.) actually exist.

## 6. Suggested sequencing

1. **Now (this branch / next):** finish determinism (1.1) — it unblocks everything test-shaped; fixture hygiene (3.2) rides along.
2. **Next:** ~~wire orphaned testdata scenarios into smoke tests (3.1)~~ ✅ DONE 2026-07-08 + ~~robustness fixtures (4)~~ ✅ DONE 2026-07-08 (one open cyclic-graph limitation noted in §4) — locks in current behavior before feature work.
3. **Then features by value:** ~~mux path params~~ ✅ → ~~generic types~~ ✅ → interface resolution → handler-factory part 2 → router-as-param → `dive`.
4. **Continuous:** apispecui test baseline, PR-level coverage gate, housekeeping.
