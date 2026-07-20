# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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

[Unreleased]: https://github.com/ehabterra/apispec/compare/v0.5.1...HEAD
[0.5.1]: https://github.com/ehabterra/apispec/compare/v0.5.0...v0.5.1
[0.5.0]: https://github.com/ehabterra/apispec/compare/v0.4.0...v0.5.0
[0.4.0]: https://github.com/ehabterra/apispec/releases/tag/v0.4.0
