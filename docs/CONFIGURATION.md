# Configuration reference

APISpec is driven by a YAML configuration file. For most projects the bundled
per-framework defaults are enough and no config is needed — pass `--config`
only when you want to add OpenAPI metadata, map custom types, or teach the
resolver about a framework/wiring style the defaults don't cover.

This document is the field-by-field reference. Start from the [minimal
example](#minimal-example-gin) below, or from the effective config that
`--output-config` dumps for your project.

## How config is loaded and merged

- **No `--config`** — APISpec detects the framework and loads its built-in
  default config (`internal/spec/config_<framework>.go`).
- **`--config path.yaml`** — your file is used **instead of** the detected
  framework defaults, not merged on top of them. What you write is what matches:
  a config with no `framework:` block has no route patterns, and the run
  documents nothing. This is deliberate for the pattern system — gin's
  `Handle(method, path, h)` and mux's `Handle(path, h)` would misparse each
  other's calls — but it does mean **a custom config must carry the framework
  patterns too**.
- **So always start from `--output-config`.** `apispec --output-config
  apispec.yaml` (or `-oc`) writes the complete effective configuration that
  actually ran — detected framework patterns, auto-applied presets and derived
  wrappers included. Edit *that* file rather than writing one from scratch.
- **Presets still apply on top of your config.** Import-gated security/auth
  mappings, CLI entrypoint fields and derived router-wrapper patterns are added
  to whatever you supply; your own entries take precedence.
- **CLI flags win.** Values such as `--title`, `--api-version`, and
  `--description` override the corresponding config-file values.

```bash
apispec --output-config apispec.yaml --output openapi.yaml   # 1. dump what ran
$EDITOR apispec.yaml                                         # 2. edit it
apispec --config apispec.yaml --output openapi.yaml          # 3. use it
```

## Minimal example (Gin)

A hand-written config has to carry its own `framework` block — this is what the
smallest useful one looks like. In practice you will want the richer set that
`--output-config` dumps; this is here to show the shape.

```yaml
info:
  title: My API
  version: 1.0.0
  description: A comprehensive API for user management

framework:
  routePatterns:
    - callRegex: ^(?i)(GET|POST|PUT|DELETE|PATCH|OPTIONS|HEAD)$
      recvTypeRegex: ^github\.com/gin-gonic/gin\.\*(Engine|RouterGroup)$
      handlerArgIndex: 1
      methodFromCall: true
      pathFromArg: true
      handlerFromArg: true
    # A registrar whose verb travels as an ARGUMENT, and may name several:
    # `Methods("GET,POST", "/search", h)` registers both.
    - callRegex: ^Methods$
      recvTypeRegex: ^example\.com/app\.\*?Router$
      methodFromArg: true
      methodArgIndex: 0
      pathFromArg: true
      pathArgIndex: 1
      handlerFromArg: true
      handlerArgIndex: 2
  requestBodyPatterns:
    - callRegex: ^(?i)(BindJSON|ShouldBindJSON|BindXML|BindYAML|BindForm|ShouldBind)$
      typeFromArg: true
      deref: true
  responsePatterns:
    - callRegex: ^(?i)(JSON|String|XML|YAML|ProtoBuf|Data|File|Redirect)$
      typeArgIndex: 1
      statusFromArg: true
      typeFromArg: true
  paramPatterns:
    - callRegex: ^Param$
      paramIn: path
    - callRegex: ^Query$
      paramIn: query
    - callRegex: ^GetHeader$
      paramIn: header
```

---

## Top-level keys

| Key | Type | Purpose |
|-----|------|---------|
| `info` | object | OpenAPI document metadata (title, version, contact, license). |
| `servers` | list | OpenAPI `servers` entries. |
| `tags` | list | OpenAPI `tags` definitions. |
| `externalDocs` | object | OpenAPI `externalDocs` block. |
| `typeMapping` | list | Map a Go type to a fixed OpenAPI schema. |
| `externalTypes` | list | Give a package/external type a custom schema. |
| `overrides` | list | Per-handler summary/description/response overrides. |
| `include` / `exclude` | object | Filter which files/packages/functions/types are analysed. |
| `defaults` | object | Fallback content types and response status. |
| `naming` | object | How operationIds and component names are spelled. |
| `security` | list | Document-level security requirements. |
| `securitySchemes` | map | OpenAPI `securitySchemes` definitions. |
| `securityMappings` | list | Map detected auth middleware to a scheme. |
| `excludeTypeComments` | bool | Keep Go doc comments out of schema `description`s. |
| `framework` | object | Framework detection/extraction patterns (advanced). |

---

## `info`

OpenAPI document metadata. Also settable via CLI flags (`--title`,
`--api-version`, `--description`, `--terms`).

```yaml
info:
  title: My API
  version: 1.0.0
  description: User management service
  termsOfService: https://example.com/terms
  contact:
    name: API Team
    url: https://example.com/support
    email: api@example.com
  license:
    name: Apache 2.0
    url: https://www.apache.org/licenses/LICENSE-2.0
```

| Field | Type | Notes |
|-------|------|-------|
| `title` | string | API title. |
| `version` | string | API version (required by OpenAPI). |
| `description` | string | Longer description. |
| `termsOfService` | string | URL. |
| `contact` | object | `name`, `url`, `email`. |
| `license` | object | `name`, `url`. |

## `servers`

```yaml
servers:
  - url: https://api.example.com/v1
    description: Production
  - url: http://localhost:8080
    description: Local
```

| Field | Type | Notes |
|-------|------|-------|
| `url` | string | Server base URL (required). |
| `description` | string | Human-readable label. |
| `variables` | map | OpenAPI server-variable substitutions. |

## `typeMapping`

Replace a Go type — wherever it appears — with a fixed OpenAPI schema. Use this
for well-known value types and for domain enums.

```yaml
typeMapping:
  - goType: time.Time
    openapiType: { type: string, format: date-time }
  - goType: uuid.UUID
    openapiType: { type: string, format: uuid }
  - goType: domain.UserStatus
    openapiType:
      type: string
      enum: [active, inactive, pending]
```

| Field | Type | Notes |
|-------|------|-------|
| `goType` | string | Go type name to match (as rendered by the analyser, e.g. `time.Time`). |
| `openapiType` | schema | The OpenAPI schema to emit for it. |

## `externalTypes`

External package types are usually resolved automatically. Declare an
`externalTypes` entry only when a third-party type needs a custom schema (for
example one whose fields aren't exported, or that marshals to a scalar).

```yaml
externalTypes:
  - name: github.com/gin-gonic/gin.H
    description: Generic JSON object
    openapiType:
      type: object
      additionalProperties: true
  - name: go.mongodb.org/mongo-driver/bson/primitive.ObjectID
    openapiType: { type: string }
```

| Field | Type | Notes |
|-------|------|-------|
| `name` | string | Fully-qualified type name (`pkgpath.TypeName`). |
| `openapiType` | schema | Schema to emit for the type. |
| `description` | string | Optional; copied into the schema. |

> Layering note: type-to-schema decisions like these live in the spec layer, not
> at metadata time — collapsing a type too early loses format information. See
> [`TYPE_MODEL.md`](TYPE_MODEL.md).

## `overrides`

Manual, per-handler overrides applied by function name. Useful when static
analysis can't recover a summary or the intended success response.

```yaml
overrides:
  - functionName: GetUser
    summary: Fetch a user by ID
    description: Returns the user record for the given ID.
    responseStatus: 200
    responseType: models.User
    tags: [users]
```

| Field | Type | Notes |
|-------|------|-------|
| `functionName` | string | Handler function name to match. |
| `summary` | string | Operation summary. |
| `description` | string | Operation description. |
| `responseStatus` | int | Force a success status code. |
| `responseType` | string | Force the success response Go type. |
| `tags` | list | Operation tags. |

## `include` / `exclude`

Gitignore-style filters that restrict what is analysed. `exclude` takes
precedence over `include`; empty lists mean "match everything".

```yaml
include:
  packages:
    - github.com/your-org/service/internal/api/**
exclude:
  files:
    - "**/*_test.go"
  functions:
    - "^debug.*"
```

Each of `include` and `exclude` accepts `files`, `packages`, `functions`, and
`types` lists.

## `defaults`

Fallbacks used when a request/response content type or status can't be inferred.

```yaml
defaults:
  requestContentType: application/json
  responseContentType: application/json
  responseStatus: 200
```

| Field | Type | Notes |
|-------|------|-------|
| `requestContentType` | string | Default request body media type. |
| `responseContentType` | string | Default response media type. |
| `responseStatus` | int | Default success status when none is detected. |

## `naming`

By default every `operationId` is the fully-qualified Go symbol and every
component name is that symbol with separators replaced. Those names are
collision-free and reproducible, which is why they are the default — but they
also reproduce the module path, the internal package layout and unexported
handler names in a document that is usually served over HTTP, and they make
80-character identifiers in a generated client.

```yaml
naming:
  operationId: full          # full (default) | receiver-method | method-path
  schemaNames: full          # full (default) | short
```

| Field | Value | Result |
|-------|-------|--------|
| `schemaNames` | `full` (default) | `github_com_acme_api_internal_estimate_LineInput` |
| | `short` | `LineInput` |
| `operationId` | `full` (default) | `github.com/acme/api/internal/httpapi.estimateHandler.updateLine` |
| | `receiver-method` | `estimateHandler.updateLine` |
| | `method-path` | `putEstimatesByIdLine` |

An unknown value logs a warning and keeps `full`, so a typo cannot silently
change the names your consumers depend on.

### Collisions

Two packages with a `Components` type are ordinary, and short names collide.
When they do, **every member of the colliding group is qualified** — never just
one of them — with the shortest suffix of its package path that tells them
apart, extended a segment at a time:

```yaml
billing_Components      # from internal/billing
estimate_Components     # from internal/estimate
LineInput               # unique, so it stays bare
```

Letting one `Components` keep the bare name would make the winner depend on
nothing a reader can see. Groups are resolved in sorted order, so the result is
reproducible run to run.

`method-path` needs no *package* qualification, but it is not collision-free
either: a method and path pair is unique in OpenAPI, while the identifier
derived from it is not — every non-alphanumeric character is dropped, so
`/a-b`, `/a/b` and `/aB` all read as `getAB`. A collision there takes a numeric
suffix (`getAB2`), assigned in sorted path order. These ids are also longer
than a handler name — `deleteReposByOwnerByRepoIssuesByIndex` — which is the
trade for carrying no Go symbol at all. `receiver-method` keeps them short, and
falls back to the method-path form when a handler serves more than one route.

A component whose type is a pointer or slice is left fully qualified: such a
key is an artifact rather than a type anyone references, and shortening it
would collide with the component for the type itself.

### Trying it on your project

A `-c` config **replaces** the framework preset rather than merging with it, so
write the effective config out first and edit that:

```bash
apispec --dir . --output-config used-config.yaml   # what apispec composed
# add a `naming:` block to used-config.yaml
apispec --dir . -c used-config.yaml -o openapi.yaml
```

## Security: `security`, `securitySchemes`, `securityMappings`

Most auth setups are detected with **no config** (see the README
[Security & authentication detection](CAPABILITIES.md#security--authentication-detection)
section). Add config only for custom middleware.

```yaml
# Document-level requirement (applies to all operations unless overridden)
security:
  - bearerAuth: []

# Scheme definitions (only needed for schemes not auto-registered)
securitySchemes:
  bearerAuth:
    type: http
    scheme: bearer
    bearerFormat: JWT

# Map a detected middleware identity to a scheme
securityMappings:
  - functionNameRegex: ^authMiddleware$
    schemes:
      - { bearerAuth: [] }

  # An apiKey middleware that says in its own configuration WHERE the key
  # travels. Without this the scheme can only be documented at the library
  # default, which is wrong for any project that configures one (#370).
  - functionNameRegex: ^KeyAuthWithConfig$
    pkgRegex: ^example\.com/auth$
    schemes:
      - { apiKeyAuth: [] }
    lookupField: KeyLookup     # field holding "<header|query|cookie>:<name>"
    lookupArgIndex: 0          # which argument holds the config (default 0)
```

`securityMappings` is framework-agnostic and works together with
`framework.securityPatterns` (which describes *scope* — router / subtree / route
/ wrapper). See [`AUTH_DETECTION_DESIGN.md`](AUTH_DETECTION_DESIGN.md) for the
full model.

### `lookupField` / `lookupArgIndex`

`lookupField` names the configuration field that states where an API key is
read from, in the grammar echo and fiber share (`"query:api_key"`,
`"cookie:token"`, `"header:X-API-Key"`). It is read at the call site, so:

- a middleware left unconfigured keeps the library default;
- two scopes configured differently become **two** schemes, named after the
  location and key (`apiKeyAuthQueryApiKey`), so neither claims to be the
  project's single answer;
- a value built at runtime, or a source OpenAPI cannot express (a form field),
  keeps the default and is reported on stderr rather than presented as observed.

The presets for echo's `KeyAuth`/`KeyAuthWithConfig` and fiber's `keyauth.New`
already declare it; this is for a house middleware that carries the same kind of
configuration.

## `framework` (advanced)

The `framework` block holds the pattern system that drives route, request-body,
response, parameter, mount, and security detection. The bundled defaults cover
gin, echo, chi, fiber, gorilla/mux, and net/http; you normally extend this only
to support a bespoke wrapper or an unsupported framework.

```yaml
framework:
  routePatterns:
    - callRegex: ^(?i)(GET|POST|PUT|DELETE|PATCH|OPTIONS|HEAD)$
      recvTypeRegex: ^github\.com/gin-gonic/gin\.\*(Engine|RouterGroup)$
      handlerArgIndex: 1
      methodFromCall: true
      pathFromArg: true
      handlerFromArg: true
  requestBodyPatterns:
    - callRegex: ^(?i)(BindJSON|ShouldBindJSON|ShouldBind)$
      typeFromArg: true
      deref: true
  responsePatterns:
    - callRegex: ^(?i)(JSON|XML|String)$
      typeArgIndex: 1
      statusFromArg: true
      typeFromArg: true
  paramPatterns:
    - callRegex: ^Param$
      # path | query | header | cookie, plus two requestBody facts:
      # formFile (an uploaded file part) and multipart (the body IS multipart)
      paramIn: path
    - callRegex: ^Query$
      paramIn: query
    - callRegex: ^Get$
      paramIn: header
      recvType: net/http.Header
      # net/http.Header is the header map of the request AND the response, so a
      # read only documents a parameter by provenance: w.Header().Get(k) and
      # c.Response().Header().Get(k) read headers the server SENDS. An origin
      # that cannot be resolved keeps the parameter.
      excludeRecvOriginRegex: ^\*?net/http\.\*?ResponseWriter$
  requestContext:          # disambiguate generic decoders (json.Decode, etc.)
    typeRegexes:
      - ^net/http\.\*Request$
    bodyAccessors:
      - ^Body$
  responseContext:         # which types ARE the response writer (see below)
    writerTypeRegexes:
      - ^net/http\.ResponseWriter$
```

Sub-keys of `framework`:

| Key | Purpose |
|-----|---------|
| `routePatterns` | How routes are registered (method/path/handler extraction). |
| `requestBodyPatterns` | Calls that bind a request body to a Go type. |
| `responsePatterns` | Calls that write a response (status + body type). **Anchor any pattern that extracts a body type** — see below. |
| `paramPatterns` | Calls that read a parameter, and its `in:` location. |
| `mountPatterns` | Sub-router mounting (path-prefix composition). |
| `securityPatterns` | Where/how auth middleware is applied (scope). |
| `entrypointPatterns` | Struct fields holding a function a library calls back (a CLI `Action`/`RunE`), so routes registered there are reachable. Presets apply from your imports. |
| `handlerInterfaceMethods` | Method names that make a type a handler (`ServeHTTP`), so a route registered with a handler *value* is followed into it. |
| `requestContext` | Which receivers/accessors mark a "request body" source. Its `typeRegexes` are also what a param pattern with `requireRequestOrigin` checks against: a query key is read off the request only when its `url.Values` traces to a value of one of these types, so `u.Query().Get(k)` on a URL parsed from configuration is not a parameter. |
| `responseContext` | Which types are the response *writer*, for response patterns gated on write destination (`requireResponseDestination`). An **empty `writerTypeRegexes` disables that gate**, and with it every streamed body — the `Content-Type` declaration is then unplaceable, so nothing is claimed. Parameter reads state their own exclusion per pattern (`excludeRecvOriginRegex`). |

### Anchoring a response pattern

A `responsePattern` matches by **call name**. If it also sets `typeFromArg` and is
anchored to nothing else, it documents that call's argument as the endpoint's
response body *wherever the call appears in the handler's call graph* — including
where a client marshals a body for an **outbound** request. The endpoint then
carries a response it can never return, and nothing in the spec indicates why.

```yaml
# Wrong: matches every json.Marshal reached from the handler, including the one
# an HTTP client uses to build its own request body.
- callRegex: ^Marshal$
  typeFromArg: true
```

Any one of these anchors it — pick whichever describes the real constraint:

| anchor | use when |
|---|---|
| `recvType` / `recvTypeRegex` | the call is made **on** the response writer or a framework renderer |
| `requireResponseDestination` | it is a generic encoder whose destination must trace to the response writer (`json.NewEncoder(w).Encode(v)`, with `destFromReceiver`) |
| `callerPkgPatterns` / `calleePkgPatterns` | only calls made in, or landing in, particular packages count |
| `functionNameRegex` | the enclosing function identifies it |

APISpec reports unanchored patterns at config load:

```
[config] 1 response pattern(s) match a bare call name anywhere in the call graph
and may document an outbound request body as a response: responsePatterns[7]
(callRegex "^Marshal$") — scope with recvType/recvTypeRegex, or set
requireResponseDestination
```

It is advisory, not an error: a project whose serializer really is only reached
from a response path is entitled to keep the pattern. Every shipped preset is
anchored, so a default run never reports this.

> A serializer that *returns* bytes (`json.Marshal`) has no destination for
> `requireResponseDestination` to check. The supported way to document
> `b, _ := json.Marshal(v); w.Write(b)` is not a `Marshal` response pattern at
> all — it is `responseContext.bodyTransforms`, which traces the marshalled bytes
> to the write on the response writer.

### Raw bytes and their media type

A byte slice names no media type, so a call that writes one verbatim needs to be
told where the media type comes from. Two pattern fields say so:

| field | use when |
|---|---|
| `rawBody` | the call writes its argument's bytes as-is (`w.Write(b)`). On a status where the handler also declares a `Content-Type` header, the declaration describes the bytes — `200 application/pdf`, binary — instead of the JSON default rendering them as a base64 string. Bytes traced back through `bodyTransforms` (`json.Marshal`) are not raw: they document the marshalled type. |
| `contentTypeFromArg` + `contentTypeArgIndex` | the call states its media type in an argument, as gin's `c.Data(code, contentType, data)` and echo's `c.Blob(code, contentType, b)` do. The argument's constant value is the response's media type, and raw bytes under one no serializer describes are documented as binary. |

```yaml
# A house renderer: Send(status int, mediaType string, body []byte)
- callRegex: ^Send$
  recvTypeRegex: ^example\.com/app/web\.\*Context$
  statusFromArg: true
  statusArgIndex: 0
  contentTypeFromArg: true
  contentTypeArgIndex: 1
  typeFromArg: true
  typeArgIndex: 2
  rawBody: true
```

### Scoping a pattern to where the call is made

Every pattern above also accepts four filters, shared by all six pattern types:

| Field | Matches against |
|-------|-----------------|
| `callerPkgPatterns` | the package of the function containing the call |
| `callerRecvTypePatterns` | the type whose method contains the call |
| `calleePkgPatterns` | the package being called into |
| `calleeRecvTypePatterns` | the owner type of the call (the list form of `recvTypeRegex`) |

Each is a list of regexes; **any** entry admits the call, all four are ANDed with
each other, and an empty list constrains nothing. A function with no receiver is
addressed by its package, the same convention `recvTypeRegex` uses.

The caller side answers a question nothing else can: two packages may register
routes with the *identical* call, and only where the call is made separates them.

```yaml
framework:
  routePatterns:
    - callRegex: ^(?i)(Get|Post|Put|Delete)$
      recvTypeRegex: ^github\.com/go-chi/chi(/v\d)?\.\*?(Router|Mux)$
      methodFromCall: true
      pathFromArg: true
      handlerFromArg: true
      handlerArgIndex: 1
      # Document the public surface; the operator endpoints registered the same
      # way from internal/debugroutes stay out of the spec.
      callerPkgPatterns:
        - /internal/api$
```

These are **include** filters — there is no "everything except", because Go's
regexp engine has no negative lookahead. A pattern that must avoid one caller is
written by naming the callers it wants.

Because these patterns are numerous and framework-specific, the authoritative
reference is the in-repo default configs (`internal/spec/config_*.go`) and the
struct definitions with doc comments in `internal/spec/config.go`. The quickest
way to author a custom pattern is to dump the effective config with
`--output-config` and edit the relevant block.

### Returned error sentinels

A handler that returns a framework's error *value*, as in
`return echo.ErrForbidden`, gets its response from the framework's error handler.
A sentinel is a package variable, not a call, so no response pattern can match
it. `errorSentinels` describes these values instead:

```yaml
framework:
  errorSentinels:
    - pkgRegex: ^github\.com/labstack/echo(/v\d+)?$        # required: the declaring package
      typeRegex: ^\*github\.com/labstack/echo(/v\d+)?\.HTTPError$
      nameRegex: ^Err(\w+)$     # first group is a status name: ErrNotFound → 404
      bodyFromValue: true       # body is the sentinel's own type...
      deref: true               # ...without the pointer
    - pkgRegex: ^github\.com/gofiber/fiber(/v\d+)?$
      typeRegex: ^\*github\.com/gofiber/fiber(/v\d+)?\.Error$
      nameRegex: ^Err(\w+)$
      bodyType: string          # or a fixed body type
      contentType: text/plain; charset=utf-8
```

Rules:

- **Status from the name.** The captured name is looked up as a `net/http`
  status name. A capture that already starts with `Status` is used as it is, so
  `ErrStatusRequestEntityTooLarge` resolves to 413.
- **No guessing.** A name that names no status, such as `ErrValidatorNotRegistered`,
  is skipped.
- **Scoped matching.** The package regex is required, so your application's own
  `ErrForbidden` is never claimed. The type regex keeps a plain `errors.New`
  sentinel in the same package out.
- **Handler returns only.** apispec reads only the handler's own `return`
  statements.
- **Existing statuses win.** If the route already documents a status, the
  sentinel doesn't replace it.

echo and fiber ship with these entries. Other supported frameworks have no
returned-sentinel convention.

---

## Schema descriptions from Go doc comments

Doc comments on the types and struct fields your handlers exchange become the
`description` of the matching schema and property. This is on by default; turn it
off when internal comments should not reach a published spec:

```yaml
excludeTypeComments: true
```

```go
// Item is a catalogue item.
type Item struct {
	// ID is the unique identifier of the item.
	ID    string  `json:"id"`
	Price float64 `json:"price"` // trailing comments are collected too
}
```

```yaml
components:
  schemas:
    myapp_Item:
      type: object
      description: Item is a catalogue item.
      properties:
        id:
          type: string
          description: ID is the unique identifier of the item.
        price:
          type: number
          description: trailing comments are collected too
```

Text is kept **verbatim**, including the leading identifier Go convention puts
there. A `json:"-"` field stays absent — a comment never resurrects a field the
encoder skips. Applies to every type kind: structs, interfaces, aliases and
named container types.

---

## Entrypoints (CLI-dispatched services)

A function parked in a struct field and called back by a library has no call edge
from your code, so nothing reaches the routes it registers. `entrypointPatterns`
names those fields. Presets for urfave/cli, cobra and ffcli apply automatically
from your imports — you only need this for a **house dispatcher**:

```yaml
framework:
  entrypointPatterns:
    # "a function stored in Cmd.Handle is invoked by something outside this
    #  module — root it if nothing else reaches it"
    - fieldRegex: ^Handle$
      recvTypeRegex: ^example\.com/internal/cli\.Cmd$
```

The owner type is matched as metadata renders it (`example.com/internal/cli.Cmd`),
and nothing is needed from the owning package — which is why this works for types
declared in a third-party library that APISpec never analyses. Leaving the owner
unconstrained is treated as a misconfiguration rather than a wildcard, since it
would claim every same-named field in the project.

Only entrypoints that are otherwise unreachable *and* whose subtree actually
registers routes are rooted, so a CLI with 50 subcommands pays for the one that
serves HTTP. Run with `--verbose` to see what it did:

```text
Entrypoints: 53 declared, 1 rooted (0 already reachable, 52 register no routes)
```

---

## Automatic wrapper detection

Plenty of projects do not call the framework directly. They put their own router in front of it, and answer through their own context:

```go
func (r *Router) Get(pattern string, h ...any) { r.Methods("GET", pattern, h...) }
func (r *Router) Methods(methods, pattern string, h ...any) {
	r.chiRouter.Method(methods, r.getPattern(pattern), unwrap(h))     // the framework call is in HERE
}

func (c *Ctx) JSON(status int, body any) { c.Resp.WriteHeader(status); json.NewEncoder(c.Resp).Encode(body) }
func (c *Ctx) Bind(dst any) error       { return json.NewDecoder(c.Req.Body).Decode(dst) }
```

The framework's own patterns cannot see any of it: by the time the chi call happens, the path and handler are the wrapper's *parameters*, not literals. APISpec derives the patterns instead, from one fact — **a method of a project type that forwards its own parameters into a call APISpec already recognises**:

| written as | derived as |
|---|---|
| `Get(pattern, h...)` → `Methods("GET", …)` → chi | a route pattern, verb from the method name |
| `Methods(verb, pattern, h...)` | a route pattern, verb from the argument (`GET,POST` registers both) |
| `Group(prefix, func(){…})` | a mount pattern — the prefix applies to everything inside |
| `Ctx.JSON(status, body)` | a response pattern, status and body merged from the two calls it makes |
| `Ctx.Bind(dst)` | a request-body pattern |
| `Ctx.Query(name)` | a parameter pattern, location taken from what it reads |

Derivation is transitive (verb methods → one registrar → the framework, and a context that encodes through the project's own json package), and it follows a value through a call, an index or a chain of assignments — so a router that unwraps a variadic `...any` resolves like one that forwards directly.

What it deliberately does **not** do is guess:

- a method that names its route with literals (`func (s *Server) routes() { r.Get("/users", h) }`) is a registration, not a way of registering, and derives nothing;
- a plain function is skipped — there is no type to scope a pattern to;
- a dependency's method is skipped — describing it would not document your project;
- a derivation that cannot resolve every role it needs is **reported but not applied**, because a pattern missing its path produces routes at the wrong path.

Everything derived is listed with `--verbose` and lands in `--output-config`, so it can be reviewed, pinned into a config file, or corrected:

```text
Router wrappers: example.com/app.*Router [Delete Get Post Put] route via example.com/app.Methods (applied);
                 example.com/app.*Router [Group] mount via prefix held by example.com/app.*Router (applied);
                 example.com/app.*Ctx [JSON] response via net/http.WriteHeader (applied);
                 example.com/app.*Combo [Get] route via example.com/app.Get (incomplete, not applied)
```

---

## Request body source disambiguation

Generic decoders like `json.Decode`, `json.Unmarshal`, and `render.DecodeJSON` are used both for request bodies *and* for unrelated decoding (config files, internal payloads). The `requestContext` block tells APISpec which receivers represent a request context and which method names yield the body. A decoder call is classified as a request-body decoder only when its source argument can be traced — through selectors, idents, assignments, and parameter boundaries — back to a body accessor on a request-context root.

```yaml
framework:
  requestContext:
    typeRegexes:
      - ^net/http\.\*Request$
      - ^github\.com/gin-gonic/gin\.\*Context$
    bodyAccessors:
      - ^Body$
      - ^GetRawData$
```

When omitted, APISpec falls back to its prior receiver-only matching, so existing configs keep working unchanged.

---

## See also

- [README](../README.md) — quick start and what APISpec does without any config
- [`CAPABILITIES.md`](CAPABILITIES.md) — which code shapes are resolved
- [`LIMITATIONS.md`](LIMITATIONS.md) — which are not, and what to do about it
- [`TYPE_MODEL.md`](TYPE_MODEL.md) — how Go types become OpenAPI schemas
- [`AUTH_DETECTION_DESIGN.md`](AUTH_DETECTION_DESIGN.md) — security detection model
- [`INTERFACE_RESOLUTION.md`](INTERFACE_RESOLUTION.md) — interface/return resolution
