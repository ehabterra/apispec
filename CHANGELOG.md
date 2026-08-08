# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.5.6] - 2026-08-08

The largest correctness release so far. On a ~900-route project the documented
surface goes from **12 paths to 640**.

### Changed

- **`--max-nodes` now bounds only route DISCOVERY.** The node budget is two
  budgets: `--max-nodes` bounds the walk that finds route registrations, and the
  new `--max-nodes-per-route` (default 1,000,000) bounds the detail expanded
  below each one. They fail differently and the warnings say which happened —
  spending the discovery budget means routes are **missing**; spending a route's
  budget means one **named** route is less detailed and no other route is
  affected. Existing flags keep working, but anyone who tuned `--max-nodes` is
  now tuning something narrower. (#291, #264)
- **Deep-route projects trade wall clock for coverage.** The per-route default is
  set by what it must not cost: at 20,000 three real projects silently lost
  request bodies and response schemas they had always documented. On a
  ~900-route project the default buys 12 → 640 paths for 46s → 166s; a project
  with no deep route pays nothing measurable. (#291)
- **Specs will change on upgrade.** Every change below adds or corrects
  documented content, so expect a reviewable diff: enum members appear, map
  envelopes gain properties, and bodies that were never responses disappear.

### Fixed

- **Route discovery no longer competes with route detail.** One global,
  depth-first node budget meant whatever expanded first spent it, so improving
  the call graph made the spec *worse*. The budgets are now independent: keys
  discovered inside a route's subtree are no longer charged to the walk that
  finds the next route. (#291, #264)
- **An enum is all of a type's constants**, not its largest `const` block. A
  32-value type was documented with 6 — and *which* 6 changed when a constant was
  added elsewhere, because the winner was the biggest block with ties to the
  earliest. Enum values are also deduplicated, since unioning blocks can bring
  together two constants sharing a value. (#292)
- **Map-literal envelopes keep their shape.** `map[string]any{"items": rows}` was
  emitted as `additionalProperties: {type: object}` — all the *type* can say —
  losing both the key and the payload's own component. Constant string keys
  become `properties` with each value resolved; a computed key leaves
  `additionalProperties` alongside the keys that did resolve, and runtime-built
  or non-string-keyed maps are unchanged. (#299, #295)
- **A `Mount` written inside a helper keeps its prefix.** The prefix reaches
  nested routes by tree containment, and with the mount one function deeper the
  sub-router was built at the call site — so the routes were documented at paths
  nothing serves. A prefix that is itself a parameter is still unresolved.
  (#304, #275)
- **A response body is no longer chosen by its type NAME.** `strings.Contains(name, "error")`
  decided which of two competing bodies filled the `default` slot, and the loser
  was dropped: it missed `ProblemDetails` and claimed `ErrorBudgetReport`.
  (#293, #287)
- **The net/http response catch-all is anchored on the writer.** It matched any
  call named `JSON`/`String`/`Data`/`File`/`Redirect` in any reached package and
  documented its second argument as the response — a default-config defect.
  `requireResponseDestination` gains `destFromAnyArg` for helpers with no agreed
  signature. (#305, #302)
- **A route's request body must come from code the route runs.** Two routes
  sharing a decode helper could be documented with each other's DTO. (#271, #269)
- **A concatenated registration path is folded, not lost.** `r.Post(opts.BaseURL+"/things", h)`
  — the shape every oapi-codegen server registers with — documented no path.
  (#277, #274)
- Declarations whose name reads like a test double are no longer erased from
  metadata. (#288)
- **apispecui was missing the per-route budget entirely** — not in the request
  limits, defaults, resolver or panel, so it could not be changed from the UI.
  Both node budgets are relabelled by the symptom they produce. (#306)
- The release workflow no longer rewrites `GO_VERSION` in `scripts/release.sh`:
  `VERSION="` is a substring of `GO_VERSION="`, and an unanchored `sed …/g`
  baked the release version into the reported Go version. (#307)

### Added

- **`--max-nodes-per-route`** (CLI and UI), plus per-route truncation reporting
  that names the route it cut short. (#291, #306)
- **Config lint for unanchored response patterns.** A pattern that extracts a
  body type but is anchored to nothing matches a bare call name anywhere in the
  call graph — on one real project a `^Marshal$` pattern documented a third-party
  provider's *request* struct as the response of 16 operations. Reported at
  config load; advisory, and no shipped preset trips it. (#303, #294)
- `docs/CONFIGURATION.md` gains an "Anchoring a response pattern" section, and
  `docs/INSTALLATION.md` now documents the pre-built per-platform binaries with
  verified commands and checksums. (#303, #307)

### Security

- The release workflow passed the pushed tag straight into `run:` blocks with
  `${{ }}`, so a tag containing shell metacharacters could execute on the runner.
  Untrusted values now arrive through `env:`. (#307)

### Performance

- Struct fields reordered so the hot ones stop paying for padding. (#278)

### Removed

- The dead `TypeResolver` and its plumbing. (#289)
- `inferMethodFromContext`, an unreachable mux-specific verb guess. (#290)

## [0.5.5] - 2026-07-30

### Added

- Expansion reporting now covers the work the tree actually does, not only the
  budget it spends. (#258, #247)
- A receiver-scoped pattern matches either receiver form. (#263, #260)

### Fixed

- The per-scope instance cap no longer truncates in silence — it counts refused
  copies and names the first scope and key. (#262, #224)
- A package qualifier belongs on a name, not on a container. (#261, #259)

### Reverted

- Both parts of the per-route node budget (#265, #266), which regressed real
  projects in two independent ways. Re-landed correctly in 0.5.6. (#268, #264)

## [0.5.4] - 2026-07-28

### Added

- Route registration is followed through func-typed struct fields. (#219, #143)
- Routes behind a CLI dispatcher resolve via entrypoint patterns. (#222, #220)
- `multipart/form-data` request bodies are documented. (#218, #207)
- Expansion limits are configurable from the UI, and a truncated run says so.
  (#234, #233)
- A project's own router and context patterns are derived instead of required.
  (#236, #235)
- The Insight dashboard shows frameworks, CLI entry points and per-status body
  resolution. (#227)

### Fixed

- Mount prefixes compose across framework boundaries. (#213, #138)
- The generated spec is independent of framework file order. (#217, #212)
- An enum is built from the constants of that type, deterministically. (#230, #229)
- A closure's identity is module-relative, so the spec is reproducible across
  machines. (#231, #216)
- Framework-specific patterns are scoped so secondary frameworks keep theirs.
  (#215, #211)
- A house router in front of the framework documents its real routes. (#232, #221)
- The declared caller/callee pattern filters actually filter. (#239, #238)
- The UI composes the same multi-framework config the engine does. (#223)

### Performance

- Positions are interned once per location rather than once per record.
  (#228, #226)

## [0.5.3] - 2026-07-22

### Added

- An ambiguous interface body maps to `oneOf`. (#210, #201)

### Fixed

- Operation summaries are sourced from method handlers' doc comments. (#205, #168)
- Handler-value routes are traced into their concrete method. (#206, #204, #178)
- Interface-typed request bodies resolve to the concrete type. (#208, #164)

## [0.5.2] - 2026-07-20

### Added

- Handler Go doc comments are mapped to the operation: the first line becomes
  `summary` and the remaining lines become `description`.
  ([#168](https://github.com/ehabterra/apispec/issues/168))
- Validator `dive` tag support — post-`dive` rules now constrain slice/map
  **elements** (`items.minimum`/`maximum`/…) while the rules before `dive`
  constrain the container.
  ([#165](https://github.com/ehabterra/apispec/issues/165))
- Struct-level (cross-field) validation expressed on a blank marker field
  (`_ struct{} \`validate:"gtefield=Min"\``) is surfaced as a note on the schema
  `description` instead of being silently dropped (OpenAPI has no native
  cross-field rule). ([#166](https://github.com/ehabterra/apispec/issues/166))
- Response status resolved through an error mapper's struct field and through
  cross-package error constructors/mappers.
  ([#187](https://github.com/ehabterra/apispec/issues/187),
  [#192](https://github.com/ehabterra/apispec/issues/192),
  [#155](https://github.com/ehabterra/apispec/issues/155))
- `.golangci.yml` pinning the linter set (the golangci-lint v2 `standard` set)
  so local `make lint` and CI agree and version bumps can't silently change the
  rules. ([#172](https://github.com/ehabterra/apispec/issues/172))
- `docs/CONFIGURATION.md` — field-by-field configuration reference.
  ([#172](https://github.com/ehabterra/apispec/issues/172))
- This `CHANGELOG.md`. ([#172](https://github.com/ehabterra/apispec/issues/172))

### Fixed

- Response over-detection: response detection is now anchored on the write to the
  response writer (`w.Write`/encoder-bound-to-`w`) and traces the written bytes
  back to their `json.Marshal` source. A `json.Marshal` whose result never
  reaches the writer — e.g. a downstream HTTP client's outbound-request marshal —
  is no longer emitted as a spurious `default` response.
  ([#195](https://github.com/ehabterra/apispec/issues/195))
- String `min`/`max` validator tags now map to `minLength`/`maxLength` (they
  constrain length in go-playground/validator), and slice `min`/`max` to
  `minItems`/`maxItems`, instead of being dropped or mis-applied as numeric
  `minimum`/`maximum`. ([#167](https://github.com/ehabterra/apispec/issues/167))
- A detected (decoded) JSON request body is now marked `required: true`.
  ([#167](https://github.com/ehabterra/apispec/issues/167))
- Response schema is gated by write-destination provenance, so a value encoded
  to a non-writer sink (a `bytes.Buffer`, a hash) is not treated as the response.
  ([#170](https://github.com/ehabterra/apispec/issues/170))
- Response value types resolve through two or more helper hops.
  ([#180](https://github.com/ehabterra/apispec/issues/180))
- `r.FormValue`-style reads resolve to a valid OpenAPI parameter location
  (query for GET/HEAD/DELETE, form body for POST/PUT/PATCH).
  ([#171](https://github.com/ehabterra/apispec/issues/171))

## [0.5.1] - 2026-07-17

### Fixed

- Bodyless status codes (1xx, 204, 205, 304) are no longer emitted with an
  invalid empty `content` block; the `content` block is omitted entirely per the
  OpenAPI spec. ([#169](https://github.com/ehabterra/apispec/issues/169))

## [0.5.0] - 2026-07-16

### Added

- Insight endpoint interface-decision reporting and an overview redesign.

### Fixed

- Lazy tracker no longer drops receiver-registered routes when middleware
  reassigns an `r`-named variable. ([#146](https://github.com/ehabterra/apispec/issues/146))
- Request bodies decoded through a `dec := json.NewDecoder(r.Body); dec.Decode(&dst)`
  wrapper now resolve to a `$ref`. ([#153](https://github.com/ehabterra/apispec/issues/153))
- Status codes threaded through helper chains, constructor fields, and
  constructor closures now resolve to concrete responses instead of `default`.
- chi `Method`/`Handle` route registration is now recognised.
- HTTP-method name inference matches whole camelCase words only ("get" no longer
  matches inside "widget").
- `[]byte` fields map to `{type: string, format: byte}`.

### Changed

- Route-matcher edge memoisation and imports-only detector pass for faster
  analysis on large projects.
- Coverage ratcheted to ~95% with a CI floor check.

## [0.4.0] - 2026-07-09

Baseline release. Static-analysis OpenAPI 3.1 generation for gin, echo, chi,
fiber, gorilla/mux, and net/http, with framework-agnostic auth detection, a
structured type model, and the `apispecui`/`apidiag` companion tools.

[Unreleased]: https://github.com/ehabterra/apispec/compare/v0.5.6...HEAD
[0.5.6]: https://github.com/ehabterra/apispec/compare/v0.5.5...v0.5.6
[0.5.5]: https://github.com/ehabterra/apispec/compare/v0.5.4...v0.5.5
[0.5.4]: https://github.com/ehabterra/apispec/compare/v0.5.3...v0.5.4
[0.5.3]: https://github.com/ehabterra/apispec/compare/v0.5.2...v0.5.3
[0.5.2]: https://github.com/ehabterra/apispec/compare/v0.5.1...v0.5.2
[0.5.1]: https://github.com/ehabterra/apispec/compare/v0.5.0...v0.5.1
[0.5.0]: https://github.com/ehabterra/apispec/compare/v0.4.0...v0.5.0
[0.4.0]: https://github.com/ehabterra/apispec/releases/tag/v0.4.0
