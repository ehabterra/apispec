# CLAUDE.md

Guidance for AI coding agents (and new contributors) working on **apispec** —
a static-analysis tool that reads Go source and generates OpenAPI 3.1 specs.

## Architecture in one pass

Pipeline (each stage consumes the previous one's output):

1. **`internal/metadata`** — loads packages (AST + go/types) and records
   *facts*: functions, types, assignments, call-graph edges, string-pooled.
   No spec decisions happen here.
2. **`internal/spec` tracker** (`tracker.go`, `lazy_tree.go`) — expands the
   call graph from route-registration sites into a tracker tree
   (LazyTree is the default engine).
3. **`internal/spec` extractor** (`extractor.go`, `pattern_matchers.go`) —
   walks the tree with **config-driven patterns** (per-framework
   `config_<framework>.go`) to find routes, request/response bodies, params,
   security.
4. **`internal/spec` mapper** (`mapper.go`, `schema_mapper.go`) — resolves Go
   types to OpenAPI schemas/components.
5. **`internal/engine`** — orchestrates 1–4; **`internal/core`** detects the
   framework; **`generator/`** is the public library API; **`cmd/apispec`**
   the CLI, **`cmd/apispecui`** the web UI.

Design docs live in `docs/` — most importantly `TYPE_MODEL.md` (structured
type model + migration phases), `TRACKER_REDESIGN.md`,
`AUTH_DETECTION_DESIGN.md`, `INTERFACE_RESOLUTION.md`.

## Commands

```bash
make test            # go test ./...
make lint            # golangci-lint (CI runs this + vet + gofmt)
make fmt
make coverage
go test ./internal/spec -run TestName -v          # one test
go test ./internal/metadata -run TestGenerateMetadata -update
                     # rewrite the metadata golden files (internal/spec/tests/*.yaml)
scripts/compare-spec.sh                            # regenerate/diff fixture snapshots
```

## Profiling & performance

```bash
./apispec --dir <project> --cpu-profile --mem-profile --profile-dir profiles
                     # also: --block-profile --mutex-profile --trace-profile
go tool pprof -top ./apispec profiles/cpu.prof
go tool pprof -list 'FuncName' -sample_index=alloc_space ./apispec profiles/mem.prof
make metrics-generate     # per-stage metrics JSON (--custom-metrics --metrics-path)
make metrics-view         # interactive metrics viewer (scripts/view_metrics.sh)
```

- First-line diagnosis is the per-stage `[engine]` log lines (loaded /
  metadata generated / tracker tree / spec mapped). With LazyTree, tree
  expansion happens *inside* the "spec mapped" stage — a bigger mapping
  number alone is moved work, not necessarily a regression.
- Benchmark on a large real project, never on `testdata/` fixtures — they
  are so small that `go/packages` load noise dominates. A/B by building
  binaries from two `git worktree`s of the versions under comparison.
- The extraction walk visits every tracker node: any per-child O(depth)
  work (string concatenation, capped-cap slice appends) goes quadratic over
  deep graphs and shows up as GC dominance (`runtime.scanObject`,
  `runtime.madvise`) rather than as the guilty frame. Chain identity is
  interned to int handles (`chainInterner`, extractor.go) for exactly this
  reason — extend the interner rather than reintroducing key strings.

## Golden rules (hard-won invariants — do not relearn these)

1. **Determinism is a feature.** Any map iteration whose order can reach the
   output (spec, metadata YAML, component names, operationIds) must be
   sorted. This was the root cause of a long-standing flaky-output bug;
   `TestGenerateMetadataDeterministic` / `TestGenerateDeterministic` guard it.
2. **Never parse type strings.** Build or accept a `typemodel.TypeRef`
   (`internal/typemodel`); pooled type strings parse once via the memoized
   `Metadata.TypeRefOf` / `CallArgument.TypeRef()` (shared refs — `Clone()`
   before mutating); render to a string only at an output boundary. The
   transitional string views are deleted — do not reintroduce them. See
   `docs/TYPE_MODEL.md`.
3. **Metadata type-name conventions.** The `Types` map keys a declaration by
   its bare name (`"Page"`, parameters in `Type.TypeParams`); the bracketed
   `"Page[T]"` form exists only as the methods-table key (matching how a
   generic receiver renders). Two string-surgery islands are deliberate
   naming behavior, not parsing debt: the mapper's map branch and the
   argument renderer's qualification tail (see `docs/TYPE_MODEL.md`).
4. **Layering: metadata records facts, spec decides schemas.** External-type
   registry, config overrides, and marshaler-based decisions belong in the
   spec layer, never at metadata time (collapsing types early loses formats —
   e.g. the historical `uuid.UUID` regression). Inline (non-`$ref`) types
   must skip every `$ref` fast-path or refs dangle.
5. **Detection must be framework-agnostic and config-driven.** Any new
   detection capability (auth, params, bodies) has to work for all supported
   frameworks (gin, echo, chi, fiber, gorilla/mux, net/http) and all wiring
   styles (router-level, group, per-route, wrapper, var-assigned) via the
   pattern configs — never hardcoded for the framework that prompted it.
6. **Bound tracker work by cumulative cost, not depth.** Tree expansion over
   dense/cyclic graphs is exponential along distinct paths while stack depth
   stays small; caps must count total nodes built (`tree.nodesBuilt`), not
   recursion-stack size. Fixtures `testdata/dense_graph`, `cyclic_graph`,
   `recursive_types` guard this.
7. **Honest over wrong.** When resolution is ambiguous (two concrete types
   assigned to one interface, a type argument erased to `any`), keep the
   honest general type; never guess one concrete option.
8. **HTTP-method precedence is a chain — don't short it.** Verb sources
   resolve in order: explicit verb in the registration pattern
   (`"GET /users"`, `MethodFromPath` → sets `MethodExplicit`) →
   verb-carrying call (`.Methods("GET")`, `router.GET`) →
   `switch r.Method` dispatch splitting (`splitMethodDispatchRoutes`,
   fires **only** on non-explicit routes) → handler-name inference
   (opt-in per config via `MethodExtraction`; matches whole camelCase
   words via `splitNameWords` — "get" must never match inside "widget") →
   the `ExtractRoute` POST default. `MethodExplicit` suppresses dispatch
   splitting; the POST default gates name inference off for plain
   registrations but does *not* block dispatch splitting
   (`testdata/method_switch` guards this).
9. **Fix the root cause or file an issue — never a workaround.** When a
   feature is blocked by a real precision gap in a lower layer (assignment,
   parameter, variable, type, chain, generics, or interface resolution;
   metadata facts; the tracker), do **not** paper over it in the upper layer
   with a heuristic — a type-name denylist/allowlist, a hardcoded special
   case, or a "good enough" guess. Fix the actual limitation, or if that's out
   of scope, **open a GitHub issue** describing the root cause (with a
   reproduction) and mark the dependent work blocked on it. A heuristic that
   trades one wrong answer for another (a false negative for a false positive)
   is not progress. This is the operational form of #7: when resolution is
   genuinely blocked, the honest move is to report the gap, not to guess around
   it. (Origin: issue #178 — `Type.Implements` omitting stdlib interfaces
   blocked the #170 write-destination gate; the denylist attempt was reverted
   and the metadata gap was filed instead.)

## Testing conventions

- **Fixture projects** live in `testdata/<name>/` (a `main.go` + `go.mod`).
  Every fixture is wired into `go test` via structural tests in `generator/`
  (`testdata_*_test.go`): expected routes/methods present, no dangling
  `$ref`s, no unresolved placeholders — structural on purpose, so schema
  evolution doesn't churn them but a dropped route fails loud.
- `used-config.yaml` and `openapi*.yaml` under fixtures are **gitignored**
  compare artifacts (`scripts/compare-spec.sh`); the structural tests are the
  CI source of truth.
- **Metadata goldens**: `internal/spec/tests/*.yaml` are byte-compared;
  regenerate only via `-update` (never by hand, never as a side effect).
- **Refactors ship with zero output drift**: the full suite (goldens,
  determinism, all framework fixtures) must pass unchanged. A behavior change
  must be a deliberate fix with its own fixture coverage — one reviewable
  change at a time.
- New feature ⇒ new fixture + structural test + (if resolution logic) unit
  tests at the layer that changed.
- **Coverage is ratcheted.** CI runs `scripts/check-coverage.sh` against
  `scripts/coverage-floor.txt`; measure with `-coverpkg` over the library
  packages and `-count=1` — per-package numbers under-report ~14 pts
  (the `generator/` fixture suites exercise the pipeline cross-package)
  and a stale test cache corrupts merged profiles. Bump the floor in its
  own PR after the raising change merges.
- **Before testing a 0%-coverage function, check its callers** — 0% usually
  means dead code (delete it, don't test it). Caveat: the `deadcode` tool
  (RTA) treats every exported `Metadata` method as reachable because of
  yaml reflection — confirm dead exported methods by grep, not by tool.

## Code & PR conventions

- Go 1.26+; `gofmt`, `go vet`, `golangci-lint` all clean before pushing —
  CI enforces them plus a 3-OS build and `go test -race`.
- Branches: `feature/<name>` / `fix/<name>`; `main` is protected (PRs only).
  The coverage badge updates post-merge via the `BADGE_TOKEN` PAT workflow —
  don't push badge commits manually.
- CodeRabbit reviews PRs. **Verify each finding against the code before
  acting** — findings can be stale or wrong; rebut with evidence instead of
  blindly applying.
- Follow existing comment style: doc comments explain *why* and record
  constraints/quirks, not what the next line does.
- Adding framework support: `internal/core/detector.go` → new
  `internal/spec/config_<framework>.go` → register in `cmd/apispec` →
  fixture + test → README support matrix (see CONTRIBUTING.md).
- **Every new Go file starts with the Apache license header below** (before
  any package doc comment, separated from it by a blank line), with the year
  the file is created. Fixture projects (`testdata/`, `test_cgo_mixed/`) are
  exempt.

  ```go
  // Copyright <current year> Ehab Terra
  //
  // Licensed under the Apache License, Version 2.0 (the "License");
  // you may not use this file except in compliance with the License.
  // You may obtain a copy of the License at
  //
  //     http://www.apache.org/licenses/LICENSE-2.0
  //
  // Unless required by applicable law or agreed to in writing, software
  // distributed under the License is distributed on an "AS IS" BASIS,
  // WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
  // See the License for the specific language governing permissions and
  // limitations under the License.
  ```

## Things never to do

- Never commit references to private/client project names in code, fixtures,
  docs, or commit messages.
- Never hand-edit generated artifacts (goldens, snapshots, coverage badge).
- Never let a `go test` run dirty the working tree (that class of bug was
  deliberately eliminated; keep test writes inside `t.TempDir()`).
