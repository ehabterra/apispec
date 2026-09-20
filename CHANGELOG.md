# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- **`--strict`: a shortfall can now fail the build.** Every condition it gates
  on was already detected, and all but one case already printed — what was
  missing is a consequence. In a 16-second CI log a `[security] … not mapped to
  a security scheme` line scrolls past, and its effect is invisible in the spec
  diff to anyone not already looking for it: an operation whose `security` block
  was dropped reads exactly like an operation that genuinely has none. The same
  goes for a route that lost its path, a response that lost its schema, and a
  package that never loaded.

  Five categories, gated together (`--strict`) or separately
  (`--strict=security,paths`, value attached with `=`): `security` (endpoints
  documented as public), `paths` (endpoints missing entirely), `schemas` (the
  operation without its shape), `truncation` (an operation that reads as
  finished and is not), `packages` (a shortfall of unknown size). Separate
  because they are not equally tolerable — a project may accept a truncated
  route while refusing to let an endpoint lose its documented authentication.

  Exit code **3**, not 1: a CI script has to be able to tell "apispec could not
  run" from "apispec ran and the result is below the bar", since the second is
  a spec to look at and the first is a build to fix. The spec is still written,
  and is byte-identical to a non-strict run — the flag decides an exit code,
  never a document — so a failing job can publish the artifact and diff it.

  One deliberate difference from the warning it promotes: the `security` gate
  fires even when no `securityMappings` are configured at all. The stderr line
  stays quiet there because auth detection is effectively off and it would be
  noise on every run; a gate that was asked for is not noise, and a project
  with auth middleware and no mappings is the worst case of this finding, not
  an exempt one. (#297)

- **`schema.nullableWhenNil`: a field the encoder writes as `null` says so.**
  A field whose zero value `encoding/json` writes as null — a nil pointer,
  slice, map or interface — with no `omitempty` is always written, and written
  as `null`. Documenting it as `type: array` claimed a shape the API does not
  guarantee, so a client validating against the document rejected a response
  the server legitimately sent.

  Not pointers alone, which is where this started: a nil `[]T` encodes as
  `null` exactly as a nil `*T` does, and `Items []Item` is on every list
  response there is, while `*[]string` is a double indirection almost nobody
  writes. A string and a fixed-size array cannot be nil and are never widened —
  `[]T` and `[2]T` differ on exactly this and both start with `[`, so the type
  is parsed rather than prefix-matched.

  Encoded as `anyOf: [{…}, {type: "null"}]` rather than
  `type: [string, "null"]`, because that is the only form a `$ref` can take —
  a `$ref` may carry no sibling keywords — and one shape for both saves a
  generated client from handling two. The union wraps the field, so a nil
  `[]string` admits null as an array while its items do not.

  The other half of `requiredFromJSONTags`, and they compose: the same field is
  always PRESENT and sometimes NULL. Enabling `required` WITHOUT this is the
  one combination worse than neither, since the document then insists a field
  is always there and never null while the server sends null.

  Opt-in partly because of a convention it cannot see: many Go services
  deliberately return `[]T{}` rather than nil, so `null` never appears on them
  and widening every array would be noise. (#368)

- **`schema.requiredFromJSONTags`: `required` derived from what encoding/json
  does.** A field with no `omitempty` and no `omitzero` is written on every
  encode, so it is always on the wire — and none of that reached the document.
  On a 452-schema service, **2,233 of 2,900 properties are always sent and not
  one said so**, so a generated TypeScript client null-checked every field and
  got no signal on the ones that really can be absent.

  Presence only: whether a value may be `null` is a separate statement and
  belongs to #368. A `*T` with no `omitempty` is always PRESENT — written as
  `null` — so it is required here and nullable there. A field promoted through
  an embedded POINTER is never required, because encoding/json writes nothing
  at all for a nil embed, which is the one thing the field's own tag cannot
  tell you, and a type that declares `MarshalJSON` contributes none at all
  because its declared fields are not the shape that reaches the wire.
  `validate:"required"` is merged with it rather than replaced.

  **Off by default, and opt-in rather than inferred**, because it states what
  the SERVER SENDS. That is exactly right for a response and an over-claim for
  a request body — a client is not bound by the server's struct tags, and the
  server decodes a payload that omits the field perfectly happily. A type used
  as both therefore declares more than the server enforces, which is a trade a
  project takes on knowingly. (#516)

### Added

- **A streamed response body is documented, with the media type the handler
  declares.** Response detection recognises a VALUE being encoded, so a handler
  that streams — a CSV writer, `io.Copy` from a file, any library writing to the
  response writer — had nothing for it to see, and the operation documented no
  success at all. On the reporting service that was 21 downloads and a CSV
  export, the export claiming an endpoint that can only fail.

  The signal is the handler's own `Content-Type`, not which writer it used.
  Enumerating writers does not scale — nine patterns still missed excelize,
  archive/zip and any house streamer — and could only guess the media type,
  where the header states it: a PDF download now documents `application/pdf`
  rather than a per-writer `application/octet-stream`. That closes the
  remaining half of #354 by construction.

  The spellings are config and per framework
  (`responseContext.contentTypeWrites`): net/http's `w.Header().Set`, gin's
  `c.Header`, fiber's `c.Set`, with echo reaching it through net/http — and a
  project with a house context declares its own. The body's schema says bytes
  rather than naming a Go type that was never encoded, and a declaration whose
  value is decided at runtime documents nothing rather than guessing. (#517)

### Fixed

- **The unmapped-middleware warning is about auth again.** Every middleware in a
  `Use`/group/per-route slot was announced as *"auth middleware not mapped to a
  security scheme"* — and on a normal service most of it is logging, recovery,
  CORS, compression, rate limiting and timeouts. A real project reported 6 such
  warnings where 2 were real, and 16 on defaults; a warning that is mostly false
  trains people past the true one, and the true one is the most important thing
  apispec prints, because an auth middleware nobody mapped means **its routes
  are documented as public**. Recognised libraries were already skipped; a
  project's own middleware matched nothing, so all of it was reported.

  The split is now made on what the middleware's body DOES, never on its name —
  which would be the guess golden rule #9 forbids. Two signals count, and either
  is sufficient on its own: it READS a credential, or it REFUSES with 401/403.
  Neither covers every authentication shape, and each catches what the other
  misses — session auth redirects to a
  login page and never writes 401, a middleware that returns an error for a
  shared renderer decides its status elsewhere, and no name table can know a
  house credential like `X-Acme-Request-Signature`. Refusing on its own is still
  not a signal: a rate limiter refuses too, with 429.

  Both signals come from what the CALL does, not from a value appearing
  somewhere in it: a credential name counts only as the argument of a by-name
  read, and a refusal status only where the framework's own response patterns
  say a status is written. Otherwise `log.Println("Authorization")`, a proxy
  middleware SETTING an outbound `Authorization` header, and `observe(403)` all
  read as authentication.

  Nothing is dropped. Middleware that shows neither signal is listed under
  `--verbose` and exposed on `SecurityDiagnostics`, so a project whose auth
  middleware fetches its credential somewhere the walk does not reach can still
  find it — the strict rule costs visibility, not the answer. The warning also
  states the consequence rather than only the condition. New
  `framework.credentialReads` is the table it decides by — the credential names,
  the calls that ARE a read whatever they are passed, and the refusal statuses —
  config-driven for a house credential, and the same surface issue #359 needs.
  (#520)


- **A `--config` file is MERGED over the detected framework's configuration,
  key by key.** It used to replace it wholesale, so a file setting only `info:`
  or `naming:` — the first thing anyone writes, and what the README suggests for
  readable operation ids — carried no route patterns and documented **zero paths
  while exiting 0**. Replacing was wrong in a subtler way too: a config that did
  name `routePatterns` silently lost the response, parameter and security
  patterns it never mentioned, and a `framework:` block describing only a
  request context lost everything else in it. A config now states what it wants
  to change and inherits the rest, at every level of nesting; map keys such as
  `securitySchemes` add to the detected ones rather than displacing them.

  Emptying a part is said out loud, because an omitted key and one written
  empty are otherwise the same value:

  ```yaml
  framework:
    routePatterns: []   # this project registers no routes my way
  ```

  `LoadAPISpecConfig` is unchanged for library callers — a file still parses on
  its own terms — and `LoadAPISpecConfigOnto` is the layering the CLI does.
  (#524)

- **The empty-spec diagnostic names the real cause.** It reported "with gin
  patterns in effect" from the *detected* framework — a fact independent of
  whether those patterns survived into the config that ran — so it asserted they
  were in effect at the exact moment a config had replaced them, and the
  follow-up line sent the reader looking for an unsupported router they did not
  have. It now reports the patterns actually in effect, and when there are none
  it says so instead of offering advice that cannot apply. (#524)

- **A serializer contributes a response only when its bytes reach the writer.**
  `json.Marshal(v)` takes a value and returns bytes, so nothing about the call
  says where those bytes go — and a response pattern naming one is therefore
  ungateable by `requireResponseDestination`, which has no writer argument to
  vet and no writer receiver to resolve. The shipped default never has to ask:
  it anchors on the write sink and traces back through
  `responseContext.bodyTransforms`, so a marshal no sink reaches is never found.
  A pattern the USER writes got none of that, and documented the body of every
  outbound request in the handler's call graph: on one real service, a mail
  provider's payload struct, an ERP credential struct and `context.Context`,
  56 spurious blocks in all. Such a match is now answered forwards instead —
  the result has to reach the response writer, in the function the call is
  written in. A project carrying the pattern gets the same document as one
  without it, so it no longer has to track each release's defaults to get a
  correct spec. (#519, follow-up to #294)

- **An operation declares only the path parameters its own template binds.**
  Parameters are attributed to a HANDLER, and a handler can be mounted at more
  than one template — `/codes/{codeId}/thing` and
  `/groups/{id}/codes/{codeId}/thing` in the same router. It reads the union of
  their names, so both were emitted on both operations, and the shorter one
  declared an `id` its path has nowhere to bind. That is not merely imprecise:
  OpenAPI requires every `in: path` parameter to appear in the template, so the
  operation is invalid — `redocly lint` reports `path-parameters-defined`, and a
  generated client gets a required argument it cannot place. Query, header and
  cookie parameters are untouched, being unbound to the template by nature.
  (#514)

- **A path variable no route declares is reported for every framework, not
  just gorilla/mux.** The diagnostic was driven by the map-key recovery, which
  only mux needs; everywhere else the name reached the route as a resolved
  parameter and nothing checked it, so a misspelled `chi.URLParam(r, "teamID")`
  was emitted as a real path parameter in silence. It is now dropped and
  reported. Conversely the warning no longer fires for a handler mounted at
  several templates, where each route legitimately lacks its sibling's names —
  it said "likely typo" about correct code on every such handler. (#514)

- **`json.RawMessage` documents any JSON value, not a string.** It is bytes
  copied into the document verbatim — object, array, number, string or null —
  and the marshaler fallback called it a string, which is the one answer that is
  almost never right: a client validating against the spec rejects every real
  payload, and a generated TypeScript client types the field as `string` and
  casts. It is a standard-library type with a fully known contract, so it joins
  the built-in registry next to `time.Time` and `uuid.UUID` rather than needing
  a `typeMapping` in every project, and it carries no "assumed string" note
  because nothing was assumed. The pointer and slice forms follow. A marshaler
  whose JSON form *is* knowable still resolves precisely. (#518)

- **An unconstrained schema is inlined instead of named.** A schema that
  constrains nothing has no content to share, so promoting it to a component
  published an empty definition and a `$ref` pointing at it. Schemas are now
  judged inline-or-named by whether they constrain anything, of which
  "primitive-shaped" was only the first half. (#518)

- **A third-party client's reply is no longer documented as the handler's
  request body.** A decoder wrapper is recognised by shape — a method forwarding
  its own parameter into a decode — and an outbound HTTP client has that shape
  too:

  ```go
  func (c *Ctx) Bind(dst any) error     { return json.NewDecoder(c.Req.Body).Decode(dst) }
  func (c *client) fetch(out any) error { return json.NewDecoder(resp.Body).Decode(out) }
  ```

  Only where the bytes come from tells them apart, which is what a request
  pattern's `requireRequestSource` asks at extraction time — and derivation
  never asked it, so the second produced a pattern as readily as the first.
  Every handler calling it then documented the provider's type, *replacing* the
  handler's own body rather than appearing beside it, with nothing on stderr.
  Derivation now resolves the inner decode's source: a method that reads the
  request derives as before, a method that reads what it is HANDED carries the
  check to its call sites (so the same helper given `r.Body` documents a body
  and given a file does not), and a method that reads anything else derives
  nothing. (#513)

- **A request body read into bytes before being unmarshalled is found.**
  `data, _ := io.ReadAll(r.Body)` followed by `json.Unmarshal(data, &v)` is an
  ordinary way to write a handler, and the source check could not see past the
  read — so it answered "not the request" exactly as it does for an outbound
  response, and the operation documented no body at all. A new
  `requestContext.bodyReaders` names the calls that turn a reader into bytes
  (`io.ReadAll`, `io/ioutil.ReadAll`), the mirror of
  `responseContext.bodyTransforms` on the way out; a decode of those bytes asks
  about the reader they came from. Serializer-level, so every framework shares
  them. Found through #513, and wrong on its own well before it: on gitea it
  recovers `POST /restore_repo`, which reads its body this way.

- **A variable assigned in a method body is visible to the resolvers that read
  it.** The canonical call-site assignment lookup reached a method's scope only
  through `ParentFunction`, i.e. only for a closure declared inside one, because
  methods live in `Type.Methods` rather than in the file's function table. For a
  call written directly in a method body, every local assigned in that body was
  invisible, and each resolver reading the lookup fell back to its "cannot tell"
  answer. (#513)

- **The request the chain names is found past the root.** A project's own
  context holds the request in a field, so `c.Req.Body` has a root typed
  `*Ctx`; reading the root's type alone answered "reads nothing from the
  request" for every project that owns a context type. Every prefix of the chain
  is now offered its turn as the request, driven by the configured
  `requestContext` alone, so a house context wrapping any framework's is covered
  by construction. (#513)

### Documentation

- **The README is now a getting-started document, not the manual.** It opens
  with a fit table — what APISpec is good at (internal type sources, CI drift
  checks, documenting an undocumented service), what needs review first
  (published contracts), and what it is the wrong tool for (spec-first
  workflows, runtime-assembled routes) — so a reader can decide in a minute.
  It also carries a worked before/after (Go in, OpenAPI out) and a triage table
  for a missing route. The reference material moved out, in full, to
  `docs/CAPABILITIES.md` (every shape resolved, with examples),
  `docs/LIMITATIONS.md` (what it cannot see, what it does not state, and the
  guardrails to add), `docs/TOOLS.md` (all flags and the `apispecui` HTTP API),
  `docs/CONFIGURATION.md` (wrapper detection, entrypoints, `requestContext` and
  doc-comment descriptions joined the reference there) and `CONTRIBUTING.md`
  (project layout, build and test).

- **`--config` semantics corrected.** `docs/CONFIGURATION.md` said a supplied
  config was merged *on top of* the detected framework defaults. It is not: the
  file replaces them, so a config carrying only a `naming` or `info` block
  matches no routes and documents nothing — filed as #524. Both the README and
  the reference now state that, and point at `--output-config` as the way to
  build a config.


## [0.5.9] - 2026-09-18

Routes that were silently missing. Three separate defects each dropped
registrations without a word — a builder's chains collapsing onto one route, a
nested registration overwriting a path the outer one had resolved, and a
re-extraction wiping the route outright — and together they cost **89 endpoints
on a ~900-route project (899 → 988 paths)**. The same project now documents the
same path set at every `--max-instances-per-key` setting, where it previously
differed by ten, so the tuning limit decides a route's detail rather than which
endpoints exist. What the limits *do* cost is now reported per endpoint instead
of as a count. Two allocation fixes take the run back to v0.5.8's wall clock
while carrying those extra routes.

### Added

- **The document says which endpoints an expansion limit cost.** Two reports
  gave a count and one example, so the endpoints they affected were
  undiscoverable: finding that `/{username}/{reponame}/compare` had quietly lost
  its `sort` and `template` query parameters took a four-point sweep of
  `--max-instances-per-key`, a diff of two specs and a control run. The two
  limits needed **opposite** treatments. The per-route node budget cuts a handful
  of subtrees, so they are now named. The instance cap fires in nearly every
  scope by design — listing them produced 1,219 entries on a 970-path project and
  buried the six that mattered — so what it *costs* is measured on the document
  instead: responses that rendered as `application/json: {}`. The common answer
  is the useful one, and it was previously impossible to get:
  `instance cap (75) dropped 25267970 call copies — no operation lost a response
  schema`. Exposed on `SecurityDiagnostics`, `Engine` and `Generator` so a CI
  gate can read it without parsing logs. (#296, #503)

- **An encode into a buffer that is flushed to the writer is a response.** The
  write-destination gate resolves a destination *backwards*, which answers where
  a value came from — and a buffer's answer is always "a buffer", for the one
  that becomes the response and the one that is thrown away alike. The question
  they differ on is forward: does anything take these bytes to a writer?
  `ResponseContext.BufferSinks` names the calls that do (`buf.WriteTo(w)`,
  `io.Copy(w, buf)`), config-driven and serializer-level so every net/http-family
  framework shares them. A resolved non-writer still fails, so a buffer flushed
  to another buffer or to a file stays undocumented. (#471)

- **A route constraint is read for what it says about a path parameter.** A
  purely numeric mux constraint (`{id:[0-9]+}`) now produces `type: integer`
  rather than an untyped string. (#345)

- **`naming`: choose how operationIds and component names are spelled.** Both
  default to the fully-qualified Go symbol, which is collision-free and
  reproducible — and also reproduces the module path, the internal package
  layout and unexported handler names in a document that is usually served over
  HTTP, in identifiers long enough that a generated client needs an alias for
  every type. `schemaNames: short` gives `LineInput`; `operationId:
  receiver-method` gives `estimateHandler.updateLine`, and `method-path` gives
  `putEstimatesByIdLine`, which carries no Go symbol at all. When short names
  collide — two packages with a `Components` type is ordinary — **every member
  of the group is qualified**, with the shortest package-path suffix that tells
  them apart, so no name wins for reasons a reader cannot see. The default is
  unchanged in every respect: a project that says nothing about naming gets the
  same document, byte for byte. (#298)

### Changed

- **`--max-instances-per-key` defaults to 75, down from 100.** The measurement
  that raised it does not reproduce: the nine empty response bodies it was based
  on were a *bug*, not a budget — a nested registration overwrote a resolved path
  with a placeholder, so a deeper walk produced worse output, and a re-extraction
  then wiped the route outright (both fixed below). Re-measured on the same
  metric across five projects, the count is **0 at every cap setting**, including
  on the service the raise was made for. Swept 25/50/75/100: nothing anywhere
  gains from 75 → 100, while 100 costs +34% wall on a 970-path project and +84%
  on another for a byte-identical document. It is not 25 because that is where
  two large services measurably lose parameters. A higher cap can also be
  actively worse: copies are charged against `--max-nodes-per-route`, so raising
  it truncates more route subtrees. (#224, #502)

- **Homebrew accepts any Go on PATH instead of installing its own keg.**
  `depends_on "go"` can only be satisfied by Homebrew's own build, so
  `brew install` pulled down the newest Go — hundreds of megabytes — onto
  machines that already had a working toolchain, and then did not use it: apispec
  shells out to whatever `go` PATH resolves to. The tap formulae now declare a
  requirement satisfied by `which("go")`, still fatal, so a machine with no Go is
  stopped at install time with a message rather than on the first run. (#492)

### Removed

- **The eager tracker tree and `--legacy-tracker` are gone.** The flag read like
  a safe fallback and was not one: measured against the default on three real
  services it produced identical output on two, and on the third it documented
  **194 of 280 routes** — 31% missing, with no warning — while running 1.6×
  slower; across the fixture suite it resolved four wiring shapes incorrectly.
  Deprecated in 0.5.8, removed here: `internal/spec/tracker.go` (~2000 lines),
  the flag, `EngineConfig.UseLazyTracker`, the UI's "Analysis engine" selector,
  and the five cross-engine parity tests, whose oracle no longer exists — every
  fixture involved keeps its own structural test, so the behaviour stays covered.
  Nothing in the output changes: all 108 fixtures and a 894-path real project
  generate byte-identically. If the analysis is missing something, please report
  it rather than reaching for an engine that documented less. (#425, closes #410
  and #286)

### Fixed

- **A builder's chains are separate routes, not one.** A builder registers
  every route it makes from one call site, so `r.Combo("/alpha").Get(a)` and
  `r.Combo("/beta").Get(b)` reach the same node with the same key and the same
  empty mount path. Both gates that dedupe the route walk keyed on exactly that
  and dropped every chain after the first — silently: no placeholder, no warning,
  just a document that stated one path confidently and omitted the others. On a
  970-path project this recovers **18 endpoints** — `/user/repos`,
  `/repositories/{id}`, an issue's reactions, deadline and dependencies — with
  none lost and no measurable time cost. (#465)

- **A nested registration no longer overwrites a path the outer one resolved.**
  Route extraction re-reads a route from its children, because that is how a
  chain-style route gets its path. A router *wrapper* descends into the same
  walk, and what it reaches is the same route one hop further from the literal —
  whose path operand is a local assigned from a call, which resolves to nothing.
  That placeholder replaced the caller's own `/assets/site-manifest.json`, and
  the route was dropped. Worse, it also defeated the diagnostic meant to catch
  it: because the placeholder *replaced* a resolved path, the route counted as
  documented and never reached the unresolved-registration report. Which
  endpoints this cost even depended on `--max-instances-per-key`, since a lower
  cap truncated the walk before it could overwrite anything. (#494)

- **A re-extraction fills a route in; it does not wipe it.** `ExtractRoute`
  replaced the whole `RouteInfo` whenever `File` or `Package` was empty, which
  conflates "not initialised yet" with "initialised but short one field" — so the
  second read discarded everything the first had resolved, before the guard above
  could defend it. With both fixed, a 970-path project documents the **same path
  set at every cap setting**, where it previously differed by ten and then two:
  the cap decides a route's detail again rather than which endpoints exist.
  (#498)

- **`Handle` is matched on net/http's receiver, not on the name alone.** The
  `^Handle$` route pattern carried no receiver constraint, unlike the
  `^HandleFunc$` pattern beside it — and `Handle` is what a house router calls
  its own registration method. Argument 0 of `Combo.Handle(listItems)` is the
  *handler*, and it was read as the path, so the route was documented at
  `{listItems}` while the real registration inside the method was never reached.
  It also ends a silence: that shape previously produced no path, no placeholder
  and no diagnostic at all. (#506)

- **An embedded struct contributes its fields.** A struct that embeds another
  documented none of the embedded fields, so any response whose type embeds a
  shared `Base`, `Meta` or `Envelope` was under-documented by exactly what the
  embed carries — silently. The rules are `encoding/json`'s rather than an
  approximation: an untagged embed promotes at one greater depth, a tagged embed
  is an ordinary field, an embedded non-struct contributes one field named for
  its type, and where names collide the shallowest wins — with a per-path visited
  set, so two embeds reaching one type are two candidates at equal depth and Go
  sends neither. Field order is recorded and honoured, because an embed declared
  first promotes its fields ahead of the outer ones. (#166, #487)

- **A status can carry more than one representation.** A handler that answers
  one status as JSON and another as XML had its alternates overwritten rather
  than merged. (#354, #470)

- **A wrapper's status parameter is not its response body.** A helper taking a
  status code had that argument read as the body type, and the status/body role
  collision resolved differently depending on which role arrived first. (#416,
  #484, #485)

- **Parameter accessors are covered evenly across frameworks.** The
  per-framework lists had drifted, so the same parameter source was detected on
  one router and silently dropped on the next — fiber had no header pattern at
  all, so no fiber handler could ever produce an `in: header` parameter. gin
  gains Cookie, GetQuery, QueryArray/QueryMap, GetPostForm, PostFormArray/Map and
  Params.ByName; fiber gains its header accessor and Queries; net/http gains
  Header.Values. Two knobs carry the fidelity: a multi-value accessor becomes an
  array of the single-value type, and a documented fallback
  (`DefaultQuery("page", "1")`) becomes `schema.default` and clears `required`.
  (#355, #365)

- **A ServeMux pattern's host is not part of the path.** Go 1.22 patterns are
  `[METHOD ][HOST]/[PATH]`, so a host folded into the path and emitted an
  endpoint no client calls. Hosts are read from the config (`hosts:`, plus any
  configured server URL) and never guessed, because a pattern that lost its
  leading slash is indistinguishable from one carrying a host. (#356)

- **A generic adapter's response type resolves from its instantiation**, and
  constructors survive on both sides of a type-parameter substitution. (#367,
  #477)

- **A package qualifier is evidence, so a miss does not answer with a twin.**
  A lookup that failed on the qualified name fell back to a same-named type from
  another package. (#447)

- **A handler forwarded through a wrapper parameter is followed**, including
  alternating parameter and wrapper hops, and the frame must be an invocation of
  the argument's own function rather than one that merely shares a parameter
  name. (#466, #467, #468)

- **A receiver's type identity includes its package.** `one.Combo` and
  `two.Combo` are both "Combo", and the bare-name collision is what put a
  migration's throwaway struct into a real schema. (#464)

- **A package's declared name is recorded**, and qualifiers resolve with it
  rather than with the directory name. (#458)

- **The verb matcher no longer matches inside a word.** A substring match made
  "get" fire inside "widget", which golden rule #8 forbids. (#283)

- **Security alternatives intersect, a user-defined scheme is kept, and derived
  keys are collision-safe.** Two api-key middlewares on one scope reading
  different places — a tenant key beside a user key — documented the first and
  silently dropped the second; once a scheme asserts a location that is a
  confidently wrong answer rather than a vague one. (#439, #441)

- **apispecui: the Configure tab renders again.** One escaped quote in a help
  string blanked the whole tab. Inside a JavaScript template literal `\"` is not
  an escape — it resolves to a plain `"` — so the attribute value ended early and
  the rest of the sentence was parsed as markup, where every bare `/` popped the
  element stack. The error surfaced at the template's last line with no mention
  of the string that caused it. A guard now forbids the sequence across every UI
  script, because nothing in the Go suite parses that markup: a whole tab can go
  blank with CI green. (#499)

- **The unanchored-response advisory names a remedy that applies.** It told the
  reader to add `recvType` or `requireResponseDestination` — but `json.Marshal`
  has no receiver and is handed no writer, so neither can work. It now names
  `calleePkgPatterns` and says that a serializer carrying no destination at all
  is one to drop. (#500)

- **A path held on a builder's receiver is resolved, not just reported.** The
  builder shape gives the path once, to a constructor, and reads it back off the
  receiver for each verb — `r.Combo("/items").Get(list).Post(create)` — so the
  registration itself has no path to read. #463 stopped the argument being
  rendered as a Go symbol, which left an honest `{pattern}`; this resolves what
  it stood for, by following the call chain back to the call that BUILT the
  receiver and reading the field out of the literal it returns. On a real
  project that recovered **33 endpoints** (899 paths → 932), turning collapsed
  placeholders into real routes like `/-/admin/auths/new` and
  `/notifications/threads/{id}`. Nothing is matched by name: a candidate call
  qualifies only by returning a composite literal of the receiver's type
  declaring that field, positional literals are matched against the struct's
  declared field order, and the value must resolve to a constant or to the
  argument bound to the parameter the literal stores — anything else stays a
  placeholder. (#461)

- **A path is never a rendered Go symbol.** When a path argument was a shape the
  resolver had no case for — a selector such as `c.pattern` or `settings.Path` —
  the argument was *rendered*, and rendering a selector yields a Go symbol. One
  real project had **60 paths** reading
  `/-/admin/auths/gitea.dev/modules/web.Combo.pattern`: endpoints that do not
  exist, with nothing to warn a reader, so a consumer would call a URL that
  404s. The internal type separator leaked too (`/recvfield-->Config.Path`).
  Such an argument now goes through the same value-resolution ladder as any
  other path, and becomes a declared `{placeholder}` when nothing resolves — the
  route stays addressable and visibly incomplete instead of fabricated. A path
  written as a literal is unaffected, which is 2389 of 2605 path arguments on
  that project. (#461)

- **operationIds are unique, and never prose.** An `operationId` identifies an
  operation and must be unique — a client generator turns it into a method name
  — but the fully-qualified handler symbol cannot carry that, because a shared
  middleware or wrapper genuinely *is* the resolved handler for many routes. One
  real project had **1109 operations under 700 ids**, `reqToken` alone
  accounting for 46, and 153 operations whose id was the string `invalid type`
  (go/types rendering a handler whose type did not check). A duplicated or
  unusable id is now replaced — for **every** route holding it, not the
  runners-up — by the operation's own method-and-path identity; an id that is
  unique and usable is left exactly as it was. On that project 617 of 1109 ids
  are untouched and the remaining 492 are repaired. A wrapped route and the same
  handler registered directly therefore no longer share an id: attribution is
  what the operation says (summary, request schema, responses), not its name.
  (#459)

- **One Go type now produces one component, and the right one.** A response type
  recovered from a call carried the package's *name* (`api.Issue`) while
  metadata's own type strings carry the import *path* (`example.com/api.Issue`),
  so the same type became two components — and the short-named one resolved to a
  **different type entirely**: its qualifier matches no package, so the lookup
  fell back to a bare-name scan and returned the first same-named declaration in
  package order. On gitea that was a migration's function-local
  `type User struct`, so `GET /user/` documented a User with **1 of its 22
  fields**; seven operations were affected and 31 duplicate or orphan schemas
  were emitted. A package-name qualifier is now resolved to its import path
  when exactly one package declares that name and the type; anything ambiguous
  is left untouched rather than guessed. Metadata now records each package's
  declared name, because the import path does not carry it — a module major
  version puts the package in a directory named `v2`, and `gopkg.in/yaml.v3`
  declares `package yaml` — so reading the name off the path left every
  versioned dependency broken. (#457)

- **A parameter name held in a local variable is documented, not dropped.**
  `key := "X-Request-ID"; r.Header.Get(key)` produced no parameter at all, while
  the same name written as a literal produced one — so a header or query
  parameter went missing with no warning depending only on how the name was
  spelled. Parameter names now go through the same value-resolution ladder as a
  registration path, which also resolves a name behind a type conversion.
  Constants were never affected (a package constant, a package variable and a
  constant in another package all resolved already). Unresolvable stays
  unresolved: a name the code rewrites on a branch, or one that cannot be
  evaluated, is left out rather than guessed, and a parameter is never emitted
  with an empty name. A field of a package-level struct value is still not
  resolved (#455). (#453)

- **apispecui: a symlink inside the analyzed module could no longer be used to
  read files outside it.** `GET /api/insight/source` serves source only from the
  analyzed module, GOROOT, or the module cache, but the check was lexical: for a
  link at `<module>/leak.go` pointing anywhere on disk, the relative path never
  left the module *as a string*, so the request was allowed and the linked
  file's contents were returned — reported under the in-module path. Two
  defects compounded, because the guard also validated one path while
  `SourceSnippet` re-derived and opened another. Containment is now decided on
  symlink-resolved paths on both sides, the resolved path is the one read, and a
  path that cannot be resolved fails closed instead of falling through to the
  lexical comparison. Resolving the roots as well matters: a module under a
  symlinked path (on macOS `/tmp` is `/private/tmp`) would otherwise be refused
  its own source. `apispecui` binds to localhost, so this needed a symlink in a
  repo the developer chose to analyze — plausible for an untrusted checkout,
  which is what the tool is for. (#424)

- **A metadata field that records no string now reads as absent, not as an
  arbitrary one.** Pooled strings are stored by index, and 0 was a valid index —
  the first string interned in a run — so any record that left an index field
  unset pointed at that string instead of at nothing. That is what published 211
  junk schema `description` values (#448), and it was silent by construction:
  the junk is plausible prose, and it changed with interning order, so it read
  as version drift rather than as a bug. Slot 0 of the pool is now reserved for
  the empty string, so the Go zero value means "no string" by construction.
  Every pool index shifts by one, which regenerates the metadata goldens; all
  109 fixture specs are byte-identical. (#449)

- **A parameter with no name is no longer emitted.** OpenAPI requires `name`,
  and a parameter without one cannot be sent, matched or validated. One was
  reaching a real project's spec, readable only because the unset name index
  resolved to a pooled string (`name: func_type`) — reserving pool slot 0
  exposed it. A `$ref` parameter is unaffected, since its name lives in the
  component it points at. Why a parameter is recorded without a name is tracked
  separately in #452. (#449)

- **An inline struct's schema no longer publishes an interned string as its doc
  comment.** An anonymous-struct request body carried a `description` that was
  never a comment: the synthetic type recorded no `Comments` index, and the zero
  value is a *valid* pool index — the first string interned in that run — so the
  body and every property of it were described as `literal`, `func_type`, or the
  type's own internal key. One real service published 211 of them. Because the
  text is "whatever was pooled first", it also moved with pooling order, so an
  unchanged project drifted between versions. Inline structs now say they have
  no comment; documented named types are unaffected. (#448)

- **A write the registration never sees no longer makes its path ambiguous.**
  Path variables are traced by agreement — every assignment visible at the call
  site has to agree, so an unreadable one is reported rather than guessed
  (#431) — but "visible" counted every write to that name anywhere in the
  function, including ones *below* the registration. `p := "/first";
  register(p); p = "/second"` therefore looked ambiguous and kept a placeholder.
  Writes that cannot reach the call are now excluded, using the control-flow
  regions the response pairing already relies on: source order alone would be
  wrong, because in `for { register(p); p = next(p) }` the write below the call
  is live on every iteration after the first, so that one is still counted and
  the path stays approximate. Half of #436; distinguishing two *bindings* of one
  name (an inner block's `p := …`) still needs a metadata fact that is not
  recorded, and `testdata/variable_path` pins it. (#436)

- **The call-graph diagram is deterministic.** Two generations of an unchanged
  project emitted the same `--diagram` file with a pair of node labels swapped,
  because a node's parameter labels were collected by ranging the call edge's
  parameter map. They are now collected in sorted order. The spec has been
  deterministic since #340 precisely so it can be committed and diffed in CI;
  the diagram is an output too, and a project that commits `diagram.html` was
  getting a spurious diff on every run. Content is unchanged — the same labels,
  in a stable order. (#443)

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
  and the handler are written by different hands, and the requirement list's
  ALTERNATIVES are intersected: where a client can authenticate one way that
  consumes the header and another way that does not, the parameter stays.
  (#412)

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

### Performance

- **The response-call matcher is memoized, recovering a 45% regression.** It
  runs the configured response patterns over a call's name, and the lazy tree
  asks it per child spec — 15.7M nodes on a 970-path project, carrying a few
  thousand distinct names — with nothing caching the answer. On that project:
  **291s → 160s wall, 1.302T → 0.620T instructions retired**, output
  byte-identical. That was the whole of the regression the response budget had
  introduced. (#496)

- **The instance-cap report no longer renders its own label 31 million times.**
  `noteInstanceTruncation` took the scope as a rendered string, so every caller
  built it — a concatenation of two interned keys — on each refused copy, to
  produce one line of output naming the first. That was **6.89 GB, 42% of
  everything the run allocated**. It now takes the two handles and renders once.
  (#491)

## [0.5.8] - 2026-08-28

Released without a changelog entry; this section is written from the commit
range for the record.

Response fidelity and route discovery. A body written without `WriteHeader` is
documented as `200`, a response with no body carries no `content` block, and a
status write is carried by every body it dominates. Router groups created inline
keep their prefix, catch-all routes lose the router's wildcard, and a project
that does not build says so instead of reporting success over an empty document.
`--legacy-tracker` is deprecated here and removed in 0.5.9.

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

[Unreleased]: https://github.com/ehabterra/apispec/compare/v0.5.9...HEAD
[0.5.9]: https://github.com/ehabterra/apispec/compare/v0.5.8...v0.5.9
[0.5.8]: https://github.com/ehabterra/apispec/compare/v0.5.7...v0.5.8
[0.5.7]: https://github.com/ehabterra/apispec/compare/v0.5.6...v0.5.7
[0.5.6]: https://github.com/ehabterra/apispec/compare/v0.5.5...v0.5.6
[0.5.5]: https://github.com/ehabterra/apispec/compare/v0.5.4...v0.5.5
[0.5.4]: https://github.com/ehabterra/apispec/compare/v0.5.3...v0.5.4
[0.5.3]: https://github.com/ehabterra/apispec/compare/v0.5.2...v0.5.3
[0.5.2]: https://github.com/ehabterra/apispec/compare/v0.5.1...v0.5.2
[0.5.1]: https://github.com/ehabterra/apispec/compare/v0.5.0...v0.5.1
[0.5.0]: https://github.com/ehabterra/apispec/compare/v0.4.0...v0.5.0
[0.4.0]: https://github.com/ehabterra/apispec/releases/tag/v0.4.0
