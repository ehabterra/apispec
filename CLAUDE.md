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

## Golden rules (hard-won invariants — do not relearn these)

1. **Determinism is a feature.** Any map iteration whose order can reach the
   output (spec, metadata YAML, component names, operationIds) must be
   sorted. This was the root cause of a long-standing flaky-output bug;
   `TestGenerateMetadataDeterministic` / `TestGenerateDeterministic` guard it.
2. **Never parse type strings in new code.** Build or accept a
   `typemodel.TypeRef` (`internal/typemodel`); render to a string only at an
   output boundary. The transitional legacy views (`ParseParts`, `SplitArgs`)
   must not gain new callers. See `docs/TYPE_MODEL.md`.
3. **The `typeByName` lookup invariant.** Metadata keys a generic declaration
   *with* its parameter brackets (`"Page[T]"`). Everything feeding
   `typeByName`/metadata type lookups therefore stays on `ParseParts` until
   metadata and spec migrate to `TypeRef` together (phase 3 of the type-model
   plan). Do not "fix" the opaque-generic quirk in isolation.
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
