# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- `.golangci.yml` pinning the linter set (the golangci-lint v2 `standard` set)
  so local `make lint` and CI agree and version bumps can't silently change the
  rules. ([#172](https://github.com/ehabterra/apispec/issues/172))
- `docs/CONFIGURATION.md` — field-by-field configuration reference.
  ([#172](https://github.com/ehabterra/apispec/issues/172))
- This `CHANGELOG.md`. ([#172](https://github.com/ehabterra/apispec/issues/172))

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
