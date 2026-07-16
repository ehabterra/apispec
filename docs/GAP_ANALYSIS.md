# apispec — What's Important to Cover (Gap Analysis for Discussion)

*Generated 2026-07-07 from a full-repo sweep: architecture map, TODO/limitation hunt, test-coverage audit, and GitHub issue/PR history. For discussion — priorities are proposals, not decisions.*

## TL;DR — proposed priorities

| # | Item | Why it matters | Effort (guess) |
|---|------|----------------|----------------|
| 1 | ~~**Deterministic output** (string pool, call-graph edge order, fiber/generic/operationId flips)~~ ✅ DONE 2026-07-07 — guarded by `TestGenerateMetadataDeterministic`/`TestGenerateDeterministic`; verified stable across repeated runs (see §1.1) | Blocks reliable golden testing, clean diffs, reproducible CI | Medium |
| 2 | ~~**Wire existing `testdata/` scenarios into `go test`**~~ ✅ DONE 2026-07-08 — all 11 orphaned fixtures now covered (see §3.1) | Regressions in whole frameworks currently only caught by manual `compare-spec.sh` | Low–Medium |
| 3 | ~~**Fix `internal/spec/tests/*.yaml` fixture hygiene**~~ ✅ DONE 2026-07-12 — promoted to real golden files: no gitignore contradiction, a test run no longer dirties the tree, no absolute temp paths, writes gated behind `-update` (see §3.2) | Constant dirty-tree noise; non-portable; hides real changes | Low |
| 4 | ~~**Mux path params → handler params**~~ ✅ DONE 2026-07-08 — `mux.Vars(r)["id"]` now wired (see §2.1) | Only framework in the support matrix with a hole in a core column | Medium |
| 5 | ~~**Generic types (parametric structs)**~~ ✅ DONE 2026-07-12 — `Page[User]`-style envelopes resolved when the instantiation is visible at the encode site (see §2.2) | README-declared partial; generics are mainstream Go now | High |
| 6 | ~~**Interface resolution**~~ ✅ DONE 2026-07-12 — interface-typed **response bodies** resolve to the concrete type across every common form: written assignment/declaration, return-through-interface, embedded-DI dispatch, and interface (named or `interface{}`) parameters; ambiguous cases keep the interface (PR #110, see §2.3) | README-declared gap + `docs/INTERFACE_RESOLUTION.md` future-work list | High |
| 7 | ~~**Robustness regression fixtures for large/cyclic projects**~~ ✅ DONE 2026-07-08 — #10/#14 (recursive_types) + #20 (dense_graph) fixtures added; one open limitation noted (see §4) | Worst historical failures (hang, stack overflow, truncated output) have no regression tests | Medium |
| 8 | ~~**Structured typed type-model**~~ ✅ **DONE 2026-07-13** (PRs #111/#112/#115/#116, all four phases) — `internal/typemodel.TypeRef` is the single parse path; metadata records carry types via the memoized `TypeRefOf` accessor; the mapper's wrapper dispatch is Kind-based; the transitional legacy views are deleted. Fixed en route: slice-of-instantiation collapse, lost array lengths, lost multi-param-generic methods. Two documented string-surgery islands remain by design (naming behavior — see docs/TYPE_MODEL.md) | Removes the separator/encoding fragility that keeps biting generics (`-->`/`.`/`[]` conventions) | High |
| 9 | ~~**Control-flow awareness for method dispatch**~~ ✅ **DONE 2026-07-12** (narrow version — see §2 item 8) — `switch r.Method` / `if r.Method ==` handlers split per verb; a general CFG for other conditionals remains open (§7.1) | Closes the "HTTP method via switch/if" gap | High |

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
- `internal/metadata/metadata.go:1598` — `assignment.ReturnIndex = 0` hardcoded: multi-return assignments always bind the first return value. Wrong tracing for `h, err := factory()` style code paths. **Verified 2026-07-16: not observable in output** — `u, err := build()` and even the adversarial error-first `err, u := build2()` both resolve the response to the concrete `User`, so response resolution uses the assigned variable's actual type, not a naive index-0 binding. No issue filed; the hardcode may still matter for provenance depth (`§4.1` in `INSIGHT_ROADMAP.md`) but produces no wrong spec today.
- `internal/spec/tracker.go:1213` — functional-options parameter tracing uses a self-described "confusing, may not handle all cases" reverse-link workaround (relates to closed issue #38).
- Non-string map keys silently fall back to bare `object` (`schema_mapper.go:84`, `mapper.go:2030`) — fine as behavior, but could warrant a diagnostic.
- Many bare `object` fallbacks in `mapper.go` when a type can't be resolved — the insight report catches some of these; consider making "unresolved → object" count a first-class quality metric.

## 2. Declared feature gaps (README §Partial/Not-yet + docs)

Ordered by proposed value:

1. ~~**Gorilla Mux path params** detected but not wired into handler params~~ ✅ **DONE 2026-07-08** (§2.1 below).
2. ~~**Generic types** (parametric structs)~~ ✅ **DONE 2026-07-12** (§2.2 below). Function generics already worked; `Page[T]`-style response envelopes returned directly at the encode site now resolve to per-instantiation schemas.
3. ~~**Interface resolution**~~ ✅ **DONE 2026-07-12** (§2.3 below, PR #110). Interface-typed **response bodies** resolve to the concrete type across every common form (written assignment/declaration, return-through-interface, embedded-DI dispatch, interface parameters — named and `interface{}`); ambiguous cases keep the interface. Not specifically addressed: interface-typed **request** bodies (`Decode(&v)` into an interface) and cross-package multi-implementation disambiguation — separate, rarer concerns.
4. **Handler-factory pattern, part 2** — ✅ the request-body-via-wrapper variable-decoder case is done (issue #153, PR #152); part 1 (closure-returning routes) was already done. What remains under this heading is only the `interface{}`/`any`-erasing helper case below (recovering a route's real `TResponse` when the payload is genuinely bound to `any` at the encode site). **→ reproduced & filed as [#163](https://github.com/ehabterra/apispec/issues/163).**
5. **Router passed as function parameter** (not via Mount) isn't traversed — known fiber `/products` gap; deferred feature.
6. **`dive` validator tag** (array-element validation) — README-declared "planned". **→ reproduced & filed as [#165](https://github.com/ehabterra/apispec/issues/165).**
7. **Same path + same status, different schemas** — not supported (would need `oneOf` merge).
8. ~~**HTTP method chosen via switch/if** around `net/http` Handle~~ ✅ **DONE 2026-07-12** — a handler that branches on `r.Method` (switch or if-chain) is split into one operation per verb, request/response attributed per branch by source position (`internal/metadata/method_dispatch.go`, `internal/spec/method_dispatch.go`, `testdata/method_switch/`). This is the narrow, targeted version of §7.1's control-flow idea (an AST pattern-detector + line-range scoping, not a general CFG).
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
> - **Multi-argument, nested, and inferred instantiations** all resolve:
>   - `TypeParts` was reworked to peel off the bracketed argument list first and split it (`splitGenericArgs`, respecting nested brackets), so both the internal form (`pkg-->Type[Arg]`) and the go/types dotted form (`pkg.Type[pkg.Arg]`) parse uniformly — that dotted form is what an **inferred** instantiation's body type (a generic constructor's return type) and a nested field type look like.
>   - `genericInstantiationName` renders type arguments recursively, so a **nested** generic (`Envelope[Page[User]]`) keeps its inner instantiation (`Page[User]`) instead of collapsing to the declaration.
>   - `normalizeGenericInstanceName` folds the go/types form of an **inferred** instantiation into the internal form with simple argument names, so `NewEnvelope(product)` → `Envelope[Product]` keys to the same clean component as a written `Envelope[Product]{}` (no full package path embedded in the name, no duplicate schema).
> - Guarded by `TestTestdata_GenericStructs` (`Page[User]`/`Page[Product]` distinct, `Items`→`[]$ref`, `Envelope[User].data`→`$ref User`, `Pair[User, Product]` two-param, `Envelope[Page[User]]` nested, `NewEnvelope(product)` inferred, `User`/`Product` emitted, no `_T-any` placeholder), `TestTypeParts_Comprehensive` / `TestNormalizeGenericInstanceName` unit cases, and a structural row in `TestTestdata_Frameworks`. Fixture: `testdata/generic_structs/`.
>
> **Known limitation (deferred, related to §2 item 4).** When the concrete argument only exists behind a helper that erases it to `interface{}`/`any` — e.g. the existing `testdata/generic` fixture's `respondWithSuccess(w, data any)` writing `APIResponse[any]{Data: data}` — the payload still renders as a generic object. `T` is genuinely bound to `interface{}` at the encode site; recovering the route's real `TResponse` requires interprocedural type-argument threading through the helper boundary (the same shape as handler-factory part 2 / wrapper specialisation). That fixture's output improved as a side effect (the junk `APIResponse_T-any` + dangling `T-any` component became a clean `APIResponse_any` with an inline `object` `data`).

### 2.3 Interface-typed response bodies — ✅ DONE 2026-07-12

> When a handler encodes an interface-typed variable (`var a Animal = Dog{}; json.NewEncoder(w).Encode(a)`), the schema documented the empty `Animal` interface instead of the concrete `Dog`. It now resolves to the concrete type when statically traceable.
>
> Root cause was twofold: (1) `var a Iface = Concrete{}` is a `DeclStmt`, not an `AssignStmt`, so `processFunctions` never captured the concrete right-hand side; (2) the response resolver only checked the call edge's `AssignmentMap`, not the enclosing handler's.
>
> Fix (metadata + spec, gated to keep blast radius small):
> - `metadata.processFunctions` now synthesizes an assignment from each var-declaration-with-initializer (`DeclStmt` → `ValueSpec`) and reuses `processAssignment`, so `var a Animal = Dog{}` records `a`'s concrete type like any other assignment.
> - `spec.ResponsePatternMatcherImpl.resolveTypeOrigin`, when the body type resolves to a **known interface** (checked via the type's metadata `Kind`), looks up the enclosing handler's assignments (via `edge.Callee.Meta` + `findFunctionByName`) and prefers a single concrete (non-interface) type. **More than one concrete type across branches ⇒ ambiguous ⇒ keep the interface** (honest over wrong).
> - Covers `var a Animal = Dog{}`, `var a Animal; a = Dog{}`, and an intermediate concrete var; the embedded-interface DI handler pattern already resolved.
> - Guarded by `testdata/interface_response` + `TestTestdata_InterfaceResponse` (`/dog`→Dog, `/cat`→Cat, `/either`→Animal for the ambiguous case) + a structural row in `TestTestdata_Frameworks`. Full suite + lint clean; no golden drift.
>
> **Also done:** a concrete returned through a function typed to return the interface (`Encode(makeAnimal())` where `makeAnimal() Animal { return Dog{} }`) resolves via `concreteFromCalleeReturn`, which traces the callee's `ReturnVars` (single concrete → use it; ambiguous → keep interface). Fixture route `/made`.
>
> **Also done — interface parameters (named + `interface{}`).** A value passed into a helper via an interface parameter (`writeAnimal(w, v Animal); Encode(v)` with `writeAnimal(w, Dog{})`) resolves via `concreteFromParamBinding`, which walks up to the edge whose callee is the enclosing function (not the immediate parent, whose own same-named parameter shadowed the lookup) and reads that edge's `ParamArgMap`. Fixture route `/passed`.
>
> **Remaining (separate concerns, not this gap):** interface-typed **request** bodies (`Decode(&v)` into an interface) — **reproduced & filed as [#164](https://github.com/ehabterra/apispec/issues/164)** (response resolution traces the concrete, request resolution does not — an asymmetry); and disambiguating when several concrete types genuinely implement the interface at one site.

## 3. Testing gaps

### 3.1 Orphaned testdata scenarios (cheap, high-value) — ✅ DONE 2026-07-08

> Status: the 11 previously-untested fixtures are now wired into `go test`. `generator/testdata_frameworks_test.go` (`TestTestdata_Frameworks`) covers `chi`, `fiber`, `gin`, `mux`, `generic` — structural assertions (expected routes + methods present, no dangling `$ref`, no unresolved placeholders), loading each fixture's committed `used-config.yaml` when present so it matches the `compare-spec.sh` snapshot. `generator/testdata_auth_test.go` (`TestTestdata_AuthPresets`) covers the 6 auth fixtures (`auth_chi_with`, `auth_echo_group`, `auth_fiber_group`, `auth_gin_perroute`, `auth_mux_subrouter`, `auth_nethttp_wrap`) — each asserts the golang-jwt import auto-applies `bearerAuth` to the guarded route and leaves the sibling open route untouched, one row per framework wiring style (route/subtree/per-route/router/wrapper scope).

Original finding (kept for context): 27 scenario dirs existed under `testdata/`, but automated tests only exercised ~14. **No `go test` references at all** for the 11 dirs above; they were only reachable via the manual `scripts/compare-spec.sh` flow. The structural-assertion style already used in `generator/testdata_smoke_test.go` (paths present, no dangling `$ref`, no unresolved placeholders) was the right template — now extended over these dirs.

### 3.2 Fixture hygiene: `internal/spec/tests/*.yaml` — ✅ DONE 2026-07-12

> Status: resolved via option (c). The `internal/spec/tests/*.yaml` are now **real golden files**: `TestGenerateMetadata` builds each source module in `t.TempDir()` and compares the produced metadata byte-for-byte against the committed golden, writing only under `-update`. The `.gitignore` contradiction is gone (no rule for `internal/spec/tests/`), a `go test` run no longer dirties the tree, CI no longer needs `git restore`, and no absolute/temp machine paths are embedded. Verified on `main`.

Original finding (kept for context): these were not golden files — the test *wrote* them on every run as dev-inspection artifacts. They were tracked **and** listed in `.gitignore`, so every `go test` dirtied the tree (CI's coverage workflow ran `git restore internal/spec/tests/` to cope) and they embedded the absolute temp path of the last machine to run tests.

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

## 5. Housekeeping (quick wins) — ✅ DONE 2026-07-12

Swept and verified on `main`:

- ~~Delete `README.md.bak`~~ ✅ — it's gitignored (a `sed -i.bak` byproduct of the badge script), not tracked; no repo change needed.
- ~~Remove committed root-level artifacts (`coverage*.out/txt/html`, `profiles*/`, `playground_optimized.go`)~~ ✅ — none are tracked.
- ~~Resolve the tracked-vs-gitignored contradiction for `internal/spec/tests/`~~ ✅ — done (see §3.2).
- ~~`docs/AUTH_DETECTION_DESIGN.md` phase-3/phase-4 contradiction~~ ✅ — reconciled (PR #109).
- ~~Verify the package READMEs linked from the main README exist~~ ✅ — all linked docs/READMEs present (`cmd/apispec`, `cmd/apidiag`, `internal/spec`, `internal/metadata`, `docs/*`), plus `CONTRIBUTING.md` / `CODE_OF_CONDUCT.md`.

## 6. Suggested sequencing

1. ~~**Now:** finish determinism (1.1) + fixture hygiene (3.2)~~ ✅ DONE.
2. ~~**Next:** wire orphaned testdata scenarios (3.1) + robustness fixtures (4)~~ ✅ DONE 2026-07-08 (one open cyclic-graph limitation noted in §4).
3. **Then features by value:** ~~mux path params~~ ✅ → ~~generic types~~ ✅ → ~~method-via-switch/if (§2 item 8)~~ ✅ → ~~interface resolution~~ ✅ → **handler-factory part 2** (next) → router-as-param → `dive`.
4. **Continuous:** apispecui test baseline, PR-level coverage gate, housekeeping ✅.

### Delivered this session (2026-07-12)

- **Generic types (#5)** — beyond the flat encode-site case: multi-parameter (`Pair[K,V]`), nested (`Envelope[Page[User]]`), compiler-**inferred** (`NewEnvelope(x)`), struct-field, and request bodies; all key to one shared component. Fixture `testdata/generic_structs`.
- **Method-via-switch/if (§2 item 8 / #9)** — verb-less `r.Method`-dispatch handlers split per method with per-branch request/response attribution (PR #105, merged). Fixture `testdata/method_switch`.
- **Interface resolution (#6, §2.3)** — interface-typed response bodies resolve to the concrete type across every common form (written assignment/declaration, return-through-interface, embedded-DI dispatch, named + `interface{}` parameters), with an ambiguity guard (PR #110). Fixture `testdata/interface_response`.
- **Determinism (#1), fixture hygiene (#3), housekeeping (§5)** — confirmed done; AUTH doc reconcile (PR #109).
- **Coverage badge** — moved off the blocked direct-push-to-protected-main; badge job now pushes with a bypass-capable PAT (`BADGE_TOKEN`), `[skip ci]` guards re-trigger (PR #108, merged).

**Open / next:** handler-factory part 2 (request-body-via-wrapper, §2 item 4), or a bounded win — `dive` validator tag, or apispecui/cold-spot test coverage (§3.3). The type-model (#8) is complete through phase 4 (PR #116): metadata records carry `TypeRef` via the memoized `Metadata.TypeRefOf`, the mapper dispatch is Kind-based, and the legacy string views are deleted; only two documented naming-behavior islands remain (mapper map branch, argument-renderer qualification tail).

### Delivered 2026-07-13

- **Structured type-model (#8), phases 1–3** — `internal/typemodel`: `TypeRef` + `Parse` (all string encodings, never fails) + renderers with round-trip guarantees + `FromExpr` AST-boundary constructor (PR #111); spec consumers migrated onto it — canonicalizer, instantiation-name builders, enum/trace/lookup sites — with the legacy views shrunk to `ParseParts`+`SplitArgs` (PR #112); lookup + AST boundaries unified — `typeByName(pkg, name)`, `getTypeName` renders via `FromExpr` (PR #115). Bugs fixed en route, each fixture/test-covered: slice-of-instantiation composite literals collapsed onto `_T-any` (`testdata/generic_structs` `/batch`); fixed-size array fields lost their length (`[4]byte`→`[]byte`); methods on multi-parameter generic types were lost entirely (IndexListExpr receiver stringified to `""`). Zero golden drift throughout. Phase-2 assumption corrected in phase 3: metadata `Types` keys were already bare names; the bracketed form is only the methods-table convention.
- **License headers** — every non-fixture Go file carries the full Apache header with its creation year (PR #114, merged); required for new files per CLAUDE.md.
- **CLAUDE.md** — repo architecture map, invariants, and conventions for agents/contributors (PR #113, merged).

### Delivered 2026-07-14/15 (pre-v0.5.0 session)

- **Multi-framework detection & pattern merging (§7.2)** — `DetectAll` returns every framework present (first-seen order, `net/http` fallback); the engine merges a scoped secondary view (`SecondaryView` keeps only patterns with a receiver scope; `MergeFrameworkConfigs` first-occurrence-wins; `HTTPSecondaryConfig` for stdlib) so a chi-router-under-a-`net/http`-mux or a gin+mux binary gets both frameworks' patterns. Auto-detect path only — user configs are never augmented. Proven zero-drift on the suite + two real projects; fixtures `testdata/mixed_chi_nethttp`, `testdata/mixed_gin_mux` (PR #135, merged). README "Mixed / multi-framework projects" section documents the ✅/⚠️ scope.
- **chi `Method()` / `Handle()` registration (§ new)** — `r.Method(http.MethodGet, path, h)` and `r.Handle(path, h)` now register; metadata records go/types constant string values so `http.MethodGet` resolves (PR #134, merged). Fixture `testdata/chi_method_handle`.
- **Performance regressions fixed** — chain-key quadratic alloc → `chainInterner` (PR #132); `DetectAll` full-parse → `parser.ImportsOnly` + early-exit, 146→53ms (PR #140); merged-pattern route-matcher linear scan → per-edge memo, mapping 2.5–3.0s→1.8–1.9s (PR #141). All merged, byte-identical output.
- **Insight-trace completeness (#34)** — the handler-scoped trace keeps stdlib callees as natural leaves (`calleeIsBuiltin`), so `*url.Values.Get` / `*http.Request.Query` query-param nodes appear in the apispecui graph; missing them under-counted metrics (PR #142, merged). `docs/DEBUGGING.md` "my route is missing" guide + issue templates (`.github/ISSUE_TEMPLATE/*`) round out #34.
- **Known-gap pinning fixtures** — `testdata/cli_action_routes` (#143 composite-literal `Action` field, gitea urfave/cli shape) and `testdata/status_via_constructor` (#144 constructor-field status) with flip-when-fixed structural tests (PR #145, merged).
- **Open enhancement issues filed:** #138 cross-framework mount composition (chi under `net/http` loses the mount prefix); #143 composite-literal function-field roots; #144 status-code-through-constructor-field provenance.

### Found 2026-07-15 — real-world validation against a hand-maintained spec (chi cost-control API, ~50 ops)

Regenerated a private production chi API and diffed against its hand-authored OpenAPI. **Route discovery, success schemas (from real Go return types), and success status codes are excellent.** Four gaps surfaced; one is a **v0.5.0 release blocker**.

- **🔴 BLOCKER — ✅ FIXED 2026-07-16 (PR #147 by an external contributor, issue #146).** The default (lazy) tracker dropped routes registered on a receiver variable when a middleware reassigned an identically-named variable. On this app the lazy tracker emitted **22 of 50 operations**; the legacy (eager) tracker emitted all **43 paths / 50 ops**. Root cause: a middleware invoked via `r.Use(mw(...))` whose body reassigns the request variable `r` (the canonical `r = r.WithContext(ctx)` idiom) records that internal `r` (`*http.Request`) in the call edge's `AssignmentMap`. `Metadata.BuildAssignmentRelationships` then pairs that callee-body assignment with the *caller's* `r chi.Router` (same name, different scope), and `LazyTree.buildRelations` **claimed** the caller's `r.Get`/`r.Group` registrations onto the middleware producer, where they were never re-emitted (dropped) or re-homed under the wrong prefix (`/api/v1/tenant` → `/api/v1/auth/tenant`). Regression: v0.4.0 had no LazyTree (eager only); making LazyTree the default after v0.4.0 introduced the drop. **Fix (PR #147):** `internal/spec/lazytree.go` `buildRelations` gates the assignment-claim on `rel.Assignment.Func == rel.Edge.Caller.Name` (the assignment must live in the claiming caller scope), rejecting the cross-scope name collision — plus a claim-map unit test. Independently validated: lazy now matches the eager tracker (43 paths); full suite passes with zero golden drift; a 245-path real project is byte-identical; no perf regression. #147's original `chi_receiver_name_collision` fixture is too shallow to reproduce end-to-end (its structural test passes even without the fix), so a follow-up commit on #147 adds `testdata/chi_middleware_recv_shadow` + `TestTestdata_ChiMiddlewareRecvShadow` — installs the middleware as a call with nested-group density and **fails pre-fix** — and hardens the relation unit test's pooled fields to `-1` (resolving the CodeRabbit comment). (My duplicate PR #148 and the standalone fixture PR #150 were closed in favor of doing it all on #147.)
- **⚠️ Error status codes collapse to `default` — ✅ FIXED 2026-07-16 (issue #144), in two parts:**
  - **Constructor-field shape (PR #149, merged).** `RespondWithError(w, NewAPIError(msg, 401))` → `w.WriteHeader(err.Code)` (status is a selector on a parameter) now resolves via `statusFromConstructorField`: it follows the selector base variable through the wrapper parameter to its constructor assignment, matches the return composite-literal field (`Code ← code`), and reads that parameter's actual argument.
  - **Nested-helper shape (PR #151).** The far more common `respondError(w, http.StatusX, …)` → `respondJSON(w, status, …)` → `w.WriteHeader(status)` threads the status through **two** parameter hops (three via a shared error→status mapper), but the resolver walked only **one** hop — so on the motivating chi API **23 of ~50 ops** were `default`. Fix: swap the single-hop `traceArgViaParent` for the existing multi-hop `resolveArgThroughParams`, a strict superset that never changes an already-resolved status. Result: 23 `default` → 0, each verified against its handler's branches (`GET /auth/me` → `200/401/500`; `POST /auth/login` → `200/400/401/403/429/500`, matching its `writeAuthError` switch). Fixture `status_via_helper_chain` (fails pre-change). A shared error mapper attributes all its switch-branch statuses to each caller — conservative but statically reachable; precise per-branch scoping (§7.3) remains a separate, harder refinement.
- **⚠️ Request bodies not captured (only 2 `requestBody`, both generic `object`) — ✅ FIXED 2026-07-16 (issue #153, PR #152).** The app decodes via a `decodeJSON(w, r, &req)` wrapper that assigns the decoder to a local (`dec := json.NewDecoder(r.Body); dec.Decode(dst)`). The `dec` variable re-homes `dec.Decode` under the `json.NewDecoder` producer node in the LazyTree, so `argViaParent`'s single immediate-parent `ParamArgMap` lookup could not reach the wrapper's `dst` parameter and the concrete request type was lost — an *inline* decoder already worked, so only some bodies resolved. Fix: `argViaParent` keeps its immediate-parent fast path and adds an ancestor-walk fallback for the edge whose callee **is** the enclosing function (the same walk `concreteFromParamBinding` uses for interface params). Result on the motivating chi API: **25/25 request bodies now `$ref` a concrete schema** (was 0 through this wrapper). Fixture `testdata/request_body_var_decoder`; zero golden drift; the 245-path real project is byte-identical.
- **ℹ️ Verbose component/operationId names** — `github_com_org_repo_internal_httpapi_userDTO`. Fine for Swagger UI, poor for FE TypeScript codegen. Addressable via `typeMapping`/`overrides` today; a name-shortening pass would be a nice polish.

**Verdict on the tool's readiness:** all four gaps found on this app are now fixed and merged/in-flight, each drift-clean against the full suite and a 245-path real project (byte-identical): the lazy-tracker route drop (issue #146, PR #147), constructor-field status (issue #144, PR #149), nested-helper multi-hop status (PR #151, merged), and wrapper-decoded request bodies (issue #153, PR #152). On the motivating chi API the lazy default now recovers all 49 routes, 0 spurious `default` statuses (only `/docs`), and 25/25 request bodies `$ref` a concrete schema — 0 dangling refs, 0 unresolved placeholders. Remaining non-blocking cosmetic gap: verbose component names. One honest fidelity caveat (future work, §7.3): a shared error→status mapper attributes all of its switch-branch statuses to every caller — conservative and statically reachable, but per-branch scoping would be more precise. **v0.5.0 is unblocked.**

### Found 2026-07-16 — second real-world validation (modular chi microservice gateway, ~83 ops)

Regenerated a second private app (multi-module chi gateway) and audited its 21 `default` responses. Two distinct causes, one fixed, one filed:

- **✅ FIXED (PR #154, a #144 follow-up) — constructor-field status lost through a handler-factory closure.** The #144 resolver worked for plain-function handlers but collapsed to `default` when the handler was a closure returned by a method (`func (h *handler) Cancel() http.HandlerFunc { return func(w, r){ e := NewLmdError(msg, http.StatusBadRequest); RespondWithError(w, e) } }`). The `e :=` assignment is recorded on the *enclosing method's* AssignmentMap, and methods live in `Type.Methods` keyed by receiver, not `file.Functions` — which `findFunctionByName` never searched, so the constructor provenance was never found. Fix: new `callerAssignmentMap`/`methodAssignmentMap` resolve the enclosing function **or method** via the edge's `ParentFunction` (which already carries the receiver). Additive by construction (only former `default`s change); fixture `status_via_constructor_closure`; zero drift; 245-path project byte-identical. Recovered **26 previously-missing `400`s** on this app.
- **⏳ REMAINING (filed) — status assigned across switch branches, then passed to the constructor.** The shared `ePaymentError(w, err)` mapper does `switch { … statusCode = http.StatusNotFound … statusCode = http.StatusBadRequest … }; RespondWithError(w, NewLmdError(msg, statusCode))`. The status argument is a *variable* whose value depends on the branch, not a literal — so `constructorArgForParam` resolves to the `statusCode` ident and `MapStatusCode` can't pin one code, leaving `default` on the 18 endpoints that error only through this mapper (and dropping their `404`). Resolving it means fanning one WriteHeader out into the **set** of branch-assigned statuses ({404, 400, 500}) — a one→many change to the response-recording loop, distinct from every single-status path today. This is the concrete instance of the §7.3 shared-error-mapper caveat; tracked as issue #155. The other 3 `default`s (`GET /`, `/metrics`, an empty stub `GetOrderHandler`) are handlers that write nothing — `default` is the honest answer.

### Verified & filed 2026-07-16 — open-gap reproduction sweep (issues #163–#172)

Every still-open item from this doc and the history was **reproduced on current `main` with an explicit minimal fixture** (`example.com/*` stdlib net/http repros, run through the CLI) before filing — so these are confirmed, not stale. Ten issues filed; three candidates were verified as **not** gaps and closed out here.

**Filed (confirmed reproducing):**

| Issue | Gap | GAP ref | Kind |
|---|---|---|---|
| [#163](https://github.com/ehabterra/apispec/issues/163) | Response type erased through an `any`/`interface{}` helper parameter (`APIResponse[any]{Data: data}` → `data: {}`) | §2 item 4, §2.2 | enhancement |
| [#164](https://github.com/ehabterra/apispec/issues/164) | Interface-typed **request** body not resolved to concrete (responses do; requests don't — asymmetry) | §2.3 | enhancement |
| [#165](https://github.com/ehabterra/apispec/issues/165) | `dive` validator tag ignored — post-`dive` constraints not applied to slice elements | §2 item 6 | enhancement |
| [#166](https://github.com/ehabterra/apispec/issues/166) | Struct-level `validate` tag (blank marker field) silently dropped | §7.4 | enhancement |
| [#167](https://github.com/ehabterra/apispec/issues/167) | `requestBody.required` never emitted; string `min`/`max` → no `minLength`/`maxLength` | §7.3 | enhancement |
| [#168](https://github.com/ehabterra/apispec/issues/168) | Handler doc comment not mapped to operation `summary`/`description` | §7.4 | enhancement |
| [#169](https://github.com/ehabterra/apispec/issues/169) | Bodyless 204/304 emit an invalid empty `content` block | §7.3 | bug |
| [#170](https://github.com/ehabterra/apispec/issues/170) | Value encoded to a non-`ResponseWriter` `io.Writer` wrongly emitted as the response (no write-destination gating) | §7.3 | bug |
| [#171](https://github.com/ehabterra/apispec/issues/171) | `r.FormValue` params emitted with an invalid `in: form` location | §7.2 | bug |
| [#172](https://github.com/ehabterra/apispec/issues/172) | Repo hygiene: `.golangci.yml` / `docs/CONFIGURATION.md` / `CHANGELOG.md` all missing | §7.4 | documentation |

**Verified NOT a gap (no issue filed):**
- **Multi-return `ReturnIndex = 0`** (§1.2) — response resolves correctly even error-first; no wrong output. See the §1.2 note.
- **Router passed as a function parameter** (net/http) — `registerUsers(mux *http.ServeMux)` routes *are* discovered. (The historical fiber `/products` case, §2 item 5, is framework-specific and was not re-tested here — it needs a fiber fixture.)
- **Inlined HTML → template files** (§7.4) — only a test file contains inline HTML; apispecui already uses external assets. Folded into #172's description as already-done.

Not re-tested (need a framework fixture or are design-scale, left as documented notes rather than filed): §2 item 5 router-as-param **fiber** case, §2 item 7 same-path/same-status `oneOf` merge, §1.2 speculative-middleware unknown-auth diagnostic, §7.1 general CFG. Existing open issues [#138](https://github.com/ehabterra/apispec/issues/138) / [#143](https://github.com/ehabterra/apispec/issues/143) / [#155](https://github.com/ehabterra/apispec/issues/155) already cover their respective gaps.

## 7. Capability-gap ideas to consider

*Added 2026-07-12. These are **conceptual capability gaps** — directions worth designing independently, not a to-do list to copy from anywhere. Each entry names the idea, why it matters for this codebase, and where in **our** code it would land. Treat every item as a clean-room design prompt: decide if it's worth doing, then design and implement it from first principles. Some may already exist here in a different shape — verify against current code first.*

### 7.1 Architecture ideas (High effort, high leverage)

- **A structured, typed type-model instead of string-encoded types.** ✅ **Phases 1–3 done 2026-07-13** (PRs #111/#112/#115; plan and status in `docs/TYPE_MODEL.md`): `internal/typemodel.TypeRef` is the first-class descriptor (package, name, type args, pointer/slice/map wrappers as fields) with one parser for every legacy encoding and one renderer per boundary; the spec layer's parsing/building helpers and metadata's `getTypeName` all flow through it. **Remaining (phase 4):** metadata records (`CallArgument`, `Field`, assignments/returns) carry the `TypeRef` itself, string pool as serialization; then the transitional `ParseParts`/`SplitArgs` views are deleted.
- **Control-flow awareness for method/branch dispatch.** ✅ **Partially done 2026-07-12** — the specific `r.Method`-dispatch case (§2 item 8) now splits into per-method operations via a *targeted* AST detector (`detectMethodDispatch`) + line-range response scoping, deliberately **not** a general CFG. What remains as the broader idea: a reusable control-flow/branch model that also handles non-method conditionals (e.g. status set in one `if` arm, body in another), same-status-different-body branches, and dispatch inside receiver-method handlers. That general layer is a good candidate to design from scratch (or align with a contributor) rather than growing the narrow detector.

### 7.2 Detection & inference ideas (Medium effort)

- **Multi-framework detection & pattern merging.** `engine.go` detects a single framework and switches on it, so a project that mixes styles (e.g. a chi router mounted under a `net/http` mux, or a gin group embedded elsewhere) only gets one framework's patterns applied. **Idea:** let detection return the *set* of frameworks present and merge their pattern sets (with a defined precedence) before extraction. Lands in `internal/core` detection + config merge.
- **Fold more request-shaping sources into params/bodies.** Areas where param/field inference could be broadened and consolidated (some logic today is spread across `mapper.go`/`extractor.go`): variable-bound `r.FormValue`/`r.FormFile` → query/form params, converter-typed params (`strconv`-style conversions that reveal the param type), and field-level schema inference as a cohesive step. **Idea:** treat these as first-class inference inputs rather than ad-hoc cases. **Note:** `r.FormValue` *is* detected today, but emitted with an invalid `in: form` location — **filed as [#171](https://github.com/ehabterra/apispec/issues/171)** (a spec-conformance bug: valid locations are query/header/path/cookie).

### 7.3 Spec-correctness ideas (Medium; several are small and independently landable)

- **Bodyless status codes.** 1xx / 204 / 304 responses must carry **no** response content — audit that we never emit an empty/placeholder schema for them. **→ reproduced & filed as [#169](https://github.com/ehabterra/apispec/issues/169)** (204/304 emit an invalid empty `content: {application/json: {}}`).
- **Decode data-source rigor.** A decode call should only count as a request body when its source argument provably traces to the request body; audit our `requestContext` disambiguation for gaps.
- **Write-destination gating.** A write should only become a response when its destination is the response writer, not an arbitrary `io.Writer` passed around. **→ reproduced & filed as [#170](https://github.com/ehabterra/apispec/issues/170)** (a value encoded to a `bytes.Buffer` is wrongly emitted as the response).
- **Conditional-status fan-out scoping.** When a status variable is reassigned across branches, only attribute the statuses reachable from the actual call site (avoid over-emitting responses). **→ tracked as [#155](https://github.com/ehabterra/apispec/issues/155).**
- **Helper-internal error-path filtering.** Don't attribute a shared helper's internal error-fallback write to every caller's response schema.
- **JSON DTO format inference + `requestBody.required`.** Richer `format` inference from field types/tags, and marking request bodies required when appropriate. **→ reproduced & filed as [#167](https://github.com/ehabterra/apispec/issues/167)** (`requestBody.required` never emitted; string `min`/`max` don't map to `minLength`/`maxLength`).

### 7.4 Smaller ideas & housekeeping (Low–Medium effort)

- **Doc comments → `summary`/`description`.** Ensure a handler's Go doc comment reliably becomes the operation `summary`/`description` (we extract some — verify coverage). **→ reproduced & filed as [#168](https://github.com/ehabterra/apispec/issues/168)** (doc comment not consumed at all — no `summary`/`description`).
- **Struct-level validation.** Support whole-struct constraints (e.g. a `validate` tag on a blank marker field) beyond per-field validation. **→ reproduced & filed as [#166](https://github.com/ehabterra/apispec/issues/166)** (blank-field `validate` tag silently dropped).
- **Repo hygiene worth having regardless:** a committed `.golangci.yml` (pin the lint config that CI already runs), a `docs/CONFIGURATION.md` reference, a `CHANGELOG.md`, and moving inlined HTML (diagram/UI templates) into template files. **→ filed as [#172](https://github.com/ehabterra/apispec/issues/172)** (`.golangci.yml`/`docs/CONFIGURATION.md`/`CHANGELOG.md` confirmed missing; the inlined-HTML sub-item is already effectively done — only a test file contains inline HTML, apispecui uses external assets).

### 7.5 Strengths to protect (not gaps)

For balance: auth/security detection, envelope specialisation, and handler-factory support are already well-developed here, and the **lazy/eager tracker redesign, the SSA+VTA resolved call graph, and the generics work (§2.2)** are distinctive strengths. Any of the ideas above should be pursued without regressing these.
