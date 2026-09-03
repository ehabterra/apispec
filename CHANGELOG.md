# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- **Type and field doc comments become schema descriptions.** A documented Go
  struct now produces a documented schema: the type's doc comment becomes the
  schema `description`, and each field's comment (doc block or trailing line
  comment) becomes its property's. Applies to every type kind, not just structs.
  Text is kept **verbatim**, leading identifier included — Go's naming
  convention is not reliable enough to edit automatically, and a wrong edit is
  worse than a slightly redundant sentence. A `json:"-"` field stays absent; a
  comment never resurrects a field the encoder skips. `excludeTypeComments: true`
  turns it off for projects that treat internal comments as private. (#366)

### Changed

- CI builds and tests on the latest 1.26.x rather than a pinned patch release.

### Deprecated

- **`--legacy-tracker` (the eager tracker tree) is deprecated and will be
  removed in a future release.** It reads like a safe fallback and is not one:
  on a real ~280-route service it documents **194 routes** — 31% missing, with
  no warning — and runs 1.6x slower; across the fixture suite it resolves four
  wiring shapes incorrectly. Selecting it now prints a deprecation warning, and
  the CLI, README and UI describe it accurately instead of offering it as a
  comparison/escape hatch. If the default engine is missing something, report it
  rather than switching — switching will usually document *fewer* routes. (#410)

### Fixed

- **A credential is no longer documented twice.** The header read a security
  scheme was derived from was also emitted as an ordinary header parameter, so
  one `c.GetHeader("Authorization")` inside a middleware produced both
  `security: [bearerAuth]` and an `Authorization` parameter — different
  contracts in OpenAPI, so a generated client grew a second, manually-supplied
  argument beside the one the scheme drives. A header parameter is now dropped
  when a security scheme **on that same operation** consumes that header:
  `http`, `oauth2` and `openIdConnect` schemes consume `Authorization`, and an
  `apiKey` scheme consumes the header it names — which is knowable since #370.
  Everything else keeps its parameter, and that is the point: an apiKey that
  travels in a query parameter or a cookie consumes no header, an operation
  with no security has none to consume, and an explicitly public operation
  overrides the document's. Matching is case-insensitive, since the middleware
  and the handler are written by different hands. (#412)

- **An apiKey scheme is documented where the credential actually travels.**
  `schemeAPIKey` was a constant — `in: header, name: Authorization` — so every
  API-key middleware was documented at echo's and fiber's *default* lookup, and
  a project that configures one (`KeyLookup: "query:api_key"`,
  `"cookie:token"`, `"header:X-API-Key"`) got a spec that reads as
  authoritative while sending the key to a place the server never looks. The
  scheme is now shaped from the middleware's own configuration, declared by the
  mapping rather than hardcoded per library (`lookupField`, `lookupArgIndex`),
  so a house middleware can describe itself the same way. Two groups configured
  differently become two schemes; a group left at the default keeps the default,
  including when another group configures one; and a lookup that is built at
  runtime — or names a source OpenAPI has no apiKey location for, such as a form
  field — keeps the default and is reported on stderr rather than presented as
  observed. Two api-key middlewares on one scope stay two credentials: both
  appear in the operation's requirement (an AND), where collapsing them by
  middleware identity documented the first and dropped the second. A scheme the
  user defined themselves is never reshaped; where it contradicts the code, the
  disagreement is reported. (#370)

- **A path wrapped in a type conversion no longer documents a phantom
  endpoint.** `r.Mount(string(prefix), sub)` rendered as the conversion's
  *target type*, so a path named `/string/…` appeared in the document: literal
  in appearance, carrying no warning, matching nothing. A conversion changes the
  type and never the string inside it, so it is now looked through — metadata
  records it as its own kind, so this reads a fact rather than guessing a shape.
  Getting the value out also needed the parameter under the conversion, which
  the tracker cannot bind for this shape (the walk reaches such a registration
  under a *different* helper's frame), so a last resolution step reads a
  parameter from the call sites of the function the registration is written in —
  only when every caller passes the same thing, so a helper mounted at two
  prefixes keeps its placeholder rather than adopting one. `mount_via_helper`
  now documents all four of its mount forms, including `/named/things`. (#433)
- **A registration path held in a variable is now read, instead of being
  reported as unknowable.** `p := "/users"` two lines above the registration is
  a statically-known path, and it was treated as unreadable: left out of the
  document (since the change below), or — when the whole path was one variable —
  rendered as the variable's *type*, so `/string` appeared as if it were an
  endpoint. A path argument that is a bare identifier now goes through the same
  resolution ladder as one operand of a concatenation, plus the variable's own
  assignments: a literal, an alias chain, a variable prefix with a literal tail,
  and a value assembled from parts that all resolve. Only when every assignment
  visible at the call site agrees — two branches assigning different paths stays
  reported, because picking one would document an endpoint the server may not
  serve, and so does a path assigned from a call. This also resolves a prefix
  passed to a mount helper as a parameter (`mountAt(prefix, r, sub)`), which had
  its own change-detector test. Measured on a real project: six phantom
  `/…/string` paths became flagged placeholders, with no route gained or lost.
  (#431)
- **A registration whose path is built at runtime is reported instead of
  documented at a placeholder path.** A route table
  (`mux.HandleFunc(rt.Method+" "+rt.Path, rt.Handler)`) was emitted as an
  operation at `/{Method} {Path}`, and a house router that carries its pattern
  on a returned object (`Combo("/x").Get(h).Post(h)`) at `/{pattern}` — paths no
  request can match, which fail a spec-lint gate, generate client methods for
  endpoints that do not exist, and keep the path count plausible while the real
  routes are missing. Such a registration is now named on stderr with its source
  position and returned by `Generator.UnresolvedPaths()`, and the "0 paths"
  message says which of the two things happened: nothing matched, or everything
  that matched builds its path at runtime. A *partly* resolved path is
  unaffected — an unresolved mount prefix, an unresolved segment before a
  literal tail, and an unreadable tail under a prefix that IS known (a catch-all
  seen through a wrapper) all still get their operation, with the placeholder
  flagged; measured on a real project, judging on the route's own path alone
  deleted three such operations. For the chained-wrapper shape this also makes two decisions agree: wrapper derivation
  already declined it as "incomplete, not applied", and the framework call
  inside the wrapper is no longer documented behind its back. A path held
  entirely in a local variable is now reported rather than documented under the
  variable's name; tracing it is #431. (#428)
- **A `switch r.Method` written in a method is now split into one operation per
  verb.** With #382 (closures) this completes the set: every handler shape —
  plain function, closure, and method with either receiver kind, in any package
  — splits. A method's dispatch had nowhere to live: `processFunctions` skips
  any declaration with a receiver, so `detectMethodDispatch` never ran for one,
  and `metadata.Method` had no field to hold the arms nor the line range to
  scope them with. It now carries both, and the spec layer resolves a method
  handler through the per-type methods table instead of giving up when
  `findFunctionByName` returns nothing. (#427)
- **A `switch r.Method` written in a closure is now split into one operation per
  verb.** The split worked for a named handler but not for the shape
  `http.HandleFunc("/x", func(w, r) { switch r.Method { … } })`: a function
  literal has no `Function` record, so its arms were folded into the enclosing
  declaration's — mixed with every other closure's — and unreachable from the
  route, which fell to the POST default carrying *every* arm's responses. So
  `GET /x` was undocumented while `POST /x` advertised a body only the GET arm
  writes. The dispatch of a closure is now recorded under the closure's own
  identity, with the range that scopes its arms; closures registered from a
  method are covered too. Attribution also follows the **call chain** rather
  than only the response statement, so arms that delegate
  (`case http.MethodGet: h.Get(w, r)`) get their bodies instead of losing them,
  and a registration that named its verb (`mux.HandleFunc("GET /x", h)`) stops
  documenting the arms the router never sends it. (#382)
- **A route registered with per-route middleware documented the middleware, not
  the handler.** gin and fiber take their handler chain variadically
  (`r.GET(path, mw, handler)`), so the endpoint handler is the *last* argument;
  a fixed argument position held the first middleware and the operation's
  identity was built from it — the middleware's name as the `operationId` and
  its doc comment as the summary. Not shared with echo, which puts the handler
  first and the middleware after, so this is per-framework rather than a global
  "last argument wins". Repaired four operations on a real fiber service.
  (#386)
- **A middleware wrapping the handler at the registration site replaced it.**
  The wrapped-call form (`mux.Handle(p, mw(http.HandlerFunc(h)))`) is now peeled
  to the handler underneath. (#364)
- **A router group created inline in a call argument lost its prefix.**
  `RegisterRouter(v1.Group("/mod"))` has no assignment to key on, so the
  callee's registrations hung outside the group and were documented at the root
  — where two modules registering the same relative path collapse into one, so
  an endpoint disappeared rather than merely moving. (#407)
- **A catch-all route kept the router's wildcard in the path.** `/scheduler*`
  was emitted verbatim, which no OpenAPI consumer can match; it now becomes a
  path parameter. (#403)
- **Fiber route constraints leaked into the path template.** `:id<int>` left the
  `<int>` tail in the emitted path, producing a key no request can match. The
  constraint is stripped; mapping it onto the parameter's type is still open.
  (#357)
- **A project that does not build now says so.** A package that failed to
  *parse* was dropped silently and the run reported success over a thin spec —
  the report existed but only reached the verbose logger. The reason now
  distinguishes "does not parse" (a syntax error in your own source) from "does
  not type-check" (often a missing generated file), and names the file and line.
  (#237)
- **An unmatched router no longer reports success over an empty document.**
  (#379)
- **A literal `nil` response body is documented as `type: "null"`** rather than
  as an unconstrained `{}`. (#404)
- **A response with no body carries no `content` block**, instead of
  `content: {application/json: {}}`. (#393)
- **A body written without `WriteHeader` is documented as `200`**, not
  `default`. (#369)
- **A status write is carried by every body it dominates**, so a second body
  under one status no longer falls through to `default`. (#391, #389)
- **A field with no schema mapping is no longer emitted as a null property**,
  which crashed ReDoc. (#395)
- **Spec output is deterministic on projects large enough to truncate.**
  Memoized first-match scans over the file and type maps could resolve
  differently between runs. (#340)
- **Insight metrics and the tracker-tree diagram describe the tree the spec was
  built from.** Both constructed an eager tree unconditionally while generation
  has used the lazy one by default for several releases, so on a real service
  the diagram was drawn from a tree missing a third of the routes. (#410)

### Performance

- **`LazyNode` is 72 bytes instead of 80**, one Go size class smaller. Node keys
  are interned to `int32` handles, which also turns the cycle check's ancestor
  walk and the per-scope instance counters into integer comparisons. Measured on
  a ~900-path project: mapping **-7.5%**, peak RSS **-4.4%**, output
  byte-identical.
- The extraction walk grows a node's child slice instead of sizing it from the
  plan, bounds the scan for a statement's enclosing frame, and finds a
  constructor's call edge through the caller index.

## [0.5.7] - 2026-08-15

Speed and completeness. The walk that builds the spec no longer visits code that
cannot contribute to it, which on the ~900-route project from 0.5.6 takes the
documented surface from **640 paths to 900** while halving peak memory. Four
separate causes of dangling `$ref`s are fixed, and a final pass now guarantees
the document resolves.

### Added

- **`$ref`s that the document cannot satisfy are repaired and reported.** A
  single unresolvable reference makes Swagger UI refuse the whole document, and
  the four causes fixed this cycle were all found the same way — by loading the
  output into a viewer, which is not a check anyone runs against their own
  project. Generation now ends by making the document internally consistent: a
  missing target is **repaired, not dropped**, since removing the reference
  would silently change an operation's shape. The report names the **Go** type,
  not the mangled component name, because that is what tells you which
  dependency to register under `externalTypes`. (#327)
- **Homebrew tap, and install instructions that install.** `brew install` now
  works for both binaries — `apispecui` is built, released and distributed the
  same way as `apispec` rather than being source-only. (#309, #335)

### Changed

- **`--max-instances-per-key` now defaults to 100 (was 25).** The reason is how
  25 failed, not how often: on a 374-route service, adding three handlers in an
  unrelated feature pushed a shared response helper past 25 copies and silently
  removed the response body of an endpoint nobody had touched. A threshold that
  moves when you edit somewhere else cannot be verified safe by any project. At
  100 that service documents all nine bodies it was missing, for about 1.1× the
  run time; this repo's own spec gains one body it had been missing. (#224)
- **Some projects pay for that and gain nothing.** The cost is uneven: medium
  projects show no measurable change, but a 163-route service emits a
  byte-identical spec and takes 1.8× as long (7s → 13s), because its instance
  cap fires millions of times inside error-formatting call diamonds that no
  response body depends on. If your spec is unchanged at
  `--max-instances-per-key 25`, set it explicitly — you give up only the
  guarantee that the number stays safe as the code grows. (#224)
- **Specs gain content on upgrade, so expect a reviewable diff.** Skipping
  subtrees that cannot contribute frees the per-route and instance budgets for
  the code that can, so responses and bodies that were being truncated away now
  appear. On this repo six of 36 routes were hitting the per-route limit and are
  no longer; on the ~900-route project the endpoint count rises by 260. Nothing
  is removed — the change is additive wherever it is not byte-identical. (#318)

### Fixed

- **A sixth framework would have been detected only by accident.** The supported
  set was written out six times in six shapes, one of them a bare
  `knownFrameworks = 5` bounding the detector's file walk — an unlinked
  restatement of a `switch` twenty lines below. Adding a sixth framework makes
  the early exit fire at five and abandon the walk, so the new framework
  resolves only in projects that happen not to import five others. It compiles,
  every test passes, and the spec just comes out thinner. There is now one
  registry. (#285)
- **The standard library was being treated as your project code.** For any
  domain-hosted module — essentially every real Go project — project-root
  inference produced no root at all and fell through to a heuristic that accepts
  any two-segment path whose first segment has no dot. Running against this
  repo, that classified `net/http`, `go/types`, `encoding/json` and nine others
  as project packages and appended them to `IncludePackages`. Packages are now
  classified by module path. (#282)
- **The components a route's `$ref`s point at are kept.** Five callers took the
  schema from `mapGoTypeToOpenAPISchema` and discarded the components map that
  went with it, so the references survived and their targets did not — always
  for types declared outside the analysed module, which have no metadata entry.
  A route reached through two traversal contexts also dropped one extraction's
  recorded types on merge; those are now unioned, since there is no "better"
  answer between two records of what was referenced. (#325)
- **Fixed-size arrays and untyped constants are no longer registered as
  components.** `[2]int64` reached the mapper carrying a package it was never
  in, defeating a ref gate that tests the raw key for `[]` or `map[`, and became
  a component named `_2int64`; `ok: true` in a map-literal response became one
  named `untyped-bool`. The gate now judges the parsed core, and both shapes are
  normalised on entry — an untyped constant becoming the type the Go spec fixes
  as its default, so `true` is a `bool` by derivation rather than by guess.
  (#326)
- **A type is re-qualified only when it carries no package of its own.** (#329)
- **Named container types record their underlying type.** `processTypeKind`
  recorded a target only for `*ast.Ident`, so a named map or slice reached the
  spec layer with nothing to build from and fell through to an opaque object.
  (#333)
- **The UI could crash with `fatal error: concurrent map writes`.** Two insight
  requests in flight at once — two browser tabs, or one tab firing the endpoint
  and export requests together — walked the same metadata and tracker tree
  simultaneously. Analysis memoizes as it walks (identifier caches, type-param
  maps, expansion plans), so a "read" writes, and the resulting map corruption
  killed the whole server process rather than failing one request. Insight
  analysis is now serialized. The CLI was never affected: it is single-threaded
  and does not pay for the fix.
- **Homebrew installs a working binary.** Go is a runtime dependency of the
  formula, not a build-time one. (#312)

### Performance

- **The expansion no longer builds subtrees that nothing can read.** The tracker
  materialises one node per path, so a callee reached along many paths is
  rebuilt once per path — measured on a 163-route service, 12,882 distinct
  callees unfolded into 7,147,505 nodes, of which **97.6% were in subtrees that
  matched no pattern at all**. Whether a subtree can contain anything a matcher
  accepts is a property of the call edge rather than of the path taken to reach
  it, so the answer is computed once over the plan graph — thousands of
  identities — instead of over the unfolded tree's millions. Nodes on the way to
  a match are still built, so provenance is unaffected.

  | | before | after |
  |---|---|---|
  | 163-route service, mapping stage | 6.06s | **2.67s** |
  | ↳ nodes materialised | 7,147,505 | **405,490** |
  | ↳ peak RSS | 1145 MB | **522 MB** |
  | ~900-route project, peak RSS | 8.24 GB | **4.86 GB** |
  | this repo, mapping stage | 6.58s | **0.58s** |

  (#318)
- **Function lookups are indexed instead of scanning every package.** Resolving
  a bare name sorted every package key and scanned every file of every package,
  per call, on a path the response-destination resolver reaches once per
  candidate per path. On a 163-route service that was 15% of CPU and 130MB of
  allocation on its own: **12.45s → 8.03s** end to end, spec byte-identical.
  (#322)
- **The extraction chain is carried on the stack instead of interned.** 2.1M
  chains were being retained for the life of each route walk to serve 610
  lookups of at most five frames. **−25% allocation, −16% wall clock.** (#319)
- **Lookup ordering is resolved once rather than per call**, and the
  enclosing-function-literal walk is pruned — **37% faster end to end** on the
  measured project. (#322, #225)

### Known issues

- **On a project large enough to truncate, output is not yet stable run to
  run.** The same command can document a route's responses in one run and omit
  them in the next: a truncation decision is broken by map-iteration order,
  which Go randomises per process. It predates this release, but is more visible
  now that far more content is documented and therefore sits near a budget
  boundary. Projects that do not exhaust a budget are unaffected and remain
  byte-identical between runs. (#340)

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

[Unreleased]: https://github.com/ehabterra/apispec/compare/v0.5.7...HEAD
[0.5.7]: https://github.com/ehabterra/apispec/compare/v0.5.6...v0.5.7
[0.5.6]: https://github.com/ehabterra/apispec/compare/v0.5.5...v0.5.6
[0.5.5]: https://github.com/ehabterra/apispec/compare/v0.5.4...v0.5.5
[0.5.4]: https://github.com/ehabterra/apispec/compare/v0.5.3...v0.5.4
[0.5.3]: https://github.com/ehabterra/apispec/compare/v0.5.2...v0.5.3
[0.5.2]: https://github.com/ehabterra/apispec/compare/v0.5.1...v0.5.2
[0.5.1]: https://github.com/ehabterra/apispec/compare/v0.5.0...v0.5.1
[0.5.0]: https://github.com/ehabterra/apispec/compare/v0.4.0...v0.5.0
[0.4.0]: https://github.com/ehabterra/apispec/releases/tag/v0.4.0
