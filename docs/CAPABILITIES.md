# What APISpec understands

The detailed capability reference: which Go and routing shapes APISpec resolves,
with worked examples. For the short version see the
[README](../README.md#framework-support); for the shapes that do **not** resolve, see
[LIMITATIONS.md](LIMITATIONS.md).

APISpec aims for practical coverage of real-world Go services — no annotations,
no code changes, everything read from the AST, `go/types` and the call graph.

## Contents

- [Routing & handlers](#routing--handlers)
- [Requests](#requests)
- [Responses](#responses)
- [Types & schemas](#types--schemas)
- [Auth](#auth)
- [Mixed / multi-framework projects](#mixed--multi-framework-projects)
- [Examples](#examples)

## Routing & handlers

- Route groups, sub-routers, mounts and prefixes, including dependency-injected groups.
- Function literals (anonymous handlers) and handler factories — a route registered as a *call* that returns the framework's handler type (`g.POST("/users", h.Create())`), including when dispatched through an interface implemented in another package.
- **House routers and house contexts, detected automatically** — a project that puts its own type in front of the framework is documented with no configuration. See [Automatic wrapper detection](CONFIGURATION.md#automatic-wrapper-detection).
- Go 1.22 `net/http.ServeMux` method-aware routing — `mux.HandleFunc("GET /users/{id}", …)` splits into method + path, `{id}` wildcards become path parameters, and ServeMux-only syntax (`{path...}`, `{$}`) is normalised to OpenAPI templating.
- Method dispatch in the handler — one handler registered without a verb that branches on `r.Method` is split into one operation per HTTP method, each with its own body, responses and operationId. Every handler shape splits: a plain function, a closure written at the registration site, and a method (pointer or value receiver, in any package). A body written a call deeper (`case http.MethodGet: h.Get(w, r)`) is attributed to the arm that reached it, and a registration that named its verb stays one operation, scoped to the matching arm.
- **CLI-dispatched services** — routing code hanging off a command library's callback (`&cli.Command{Action: runWeb}`, `&cobra.Command{RunE: runServe}`) has no call edge from your code. APISpec treats those fields as entrypoints and documents everything below them. Presets ship for urfave/cli v1/v2/v3, spf13/cobra and peterbourgon/ff; a house dispatcher declares its own via [`entrypointPatterns`](CONFIGURATION.md#entrypoints-cli-dispatched-services).
- Route registration behind a func-typed struct field invoked in-module — package-level command vars, cross-package function values, method values and inline closures.

## Requests

- Request bodies from `json.Decode`/`Unmarshal`, framework binders, and custom wrapper helpers (`util.ReadRequest(c, &dto)` → `ctx.Bind(dto)`), traced through the wrapper's parameters.
- Form and file-upload bodies — a file part or an explicit `ParseMultipartForm` makes it `multipart/form-data` with the file as `string`/`binary`; plain form values alone stay `application/x-www-form-urlencoded`. Form reads on `GET` are documented as query parameters instead.
- Parameters in path, query, header, cookie and form position.
- Which decoder calls count as request bodies is configurable — see [Request body source disambiguation](CONFIGURATION.md#request-body-source-disambiguation).

## Responses

- Conditional status codes — when a status variable is reassigned across branches with distinct HTTP codes, one response per status is emitted, sharing the body schema.
- Wrapper/envelope specialisation — when a payload flows through a shared helper whose field is `interface{}`/`any`, the concrete per-route type is recovered from the call site and emitted as an `allOf` of the base envelope plus a `data` override.
- Map-literal envelopes — a response written as a map composite literal with constant string keys is documented as an object with those `properties`, each value resolved to its own type instead of collapsing to `additionalProperties`.
- Interface-typed bodies resolved to the concrete type statically assigned, returned, or bound at the call site — including embedded-interface handler dispatch. When several concrete types are possible on different branches the interface is kept rather than guessed.

## Types & schemas

- Import and type aliases, resolved to underlying primitives.
- Enum resolution from constants, `enum` tags, or `oneof` validator tags — attributed per declared type, never borrowed between two types sharing an underlying type.
- Assignment & alias tracking: `:=`, `=`, multi-assign, tuple returns, alias chains, latest-wins shadowing. Registration **paths** are read through the same tracking (`p := "/users"`, an alias chain, a variable prefix with a literal tail), through a type conversion (`r.Mount(string(prefix), sub)`), and — for a prefix a helper takes as a parameter — from the helper's call sites. Always subject to agreement: two branches assigning different paths, or two callers passing different prefixes, is reported as unresolvable rather than documented at one of them. A write that the registration cannot reach — below it in straight-line code — does not count against that agreement, while one below it *inside a loop* does, since a later iteration carries it back.
- Composite literals, maps, slices, fixed-size and variable-length arrays (`[16]byte`, `[5]int`, `[...]int`), pointers and automatic dereferencing, selectors and nested field access.
- Struct fields, embedded fields, and tag-based metadata (`json`, `xml`, `form`, `validate`, …).
- Inline (anonymous) struct types as request/response bodies and as nested fields — captured structurally from `go/types`, so the inline schema shows real properties and resolves named field types to `$ref`s.
- Generics on functions and parametric types — `Page[User]` and `Page[Product]` get distinct schemas, and written, multi-parameter, nested and compiler-*inferred* instantiations all resolve.
- Function-local named types used as bodies, emitted as real component schemas rather than dangling `$ref`s.
- External package types resolved to underlying primitives, with `externalTypes` for custom overrides.
- `go-playground/validator` tags mapped to OpenAPI constraints, routed by field type (`min` on a string → `minLength`, on a number → `minimum`, on a slice → `minItems`), with `dive` applying post-`dive` rules to elements.
- Go doc comments on handlers → operation `summary`/`description`; on types and fields → schema and property `description`.
- CGO packages can be skipped to avoid build errors.

## Auth

Authentication and security detection is framework-agnostic and config-driven —
see [Security & authentication detection](#security--authentication-detection).
Protected routes get a per-operation `security` requirement and the scheme is
registered under `components.securitySchemes`; explicitly-public routes render
`security: []`. The credential itself is documented **once**: a header parameter
is dropped when a scheme on that operation consumes that header, so the
middleware's `Authorization` read does not also become an argument the client is
told to supply. A header the handler reads for its own purposes — or one on an
operation whose apiKey travels in a query parameter — is kept. An **apiKey** scheme is shaped from the middleware's own
configuration — echo's `KeyAuthConfig.KeyLookup` and fiber's
`keyauth.Config.KeyLookup` — so `"query:api_key"` is documented as
`in: query, name: api_key` rather than the library's default header. Two groups
configured differently become two schemes, and two key middlewares on one scope
become two required credentials in the same requirement. A lookup built at
runtime keeps the default and says so on stderr, as does a scheme you defined
yourself that the code contradicts — your definition is kept.

## Mixed / multi-framework projects

One binary often serves more than one routing surface — a framework API next to
plain `net/http` ops endpoints, a gin API beside a gorilla/mux admin router, or
a half-migrated codebase. APISpec handles this automatically:

- **All recognised frameworks are detected** (import scan), not just the first
  one. The first-seen framework is the *primary* — its defaults and info apply.
- **Every additional framework's patterns are merged in**, restricted to its
  receiver-scoped patterns, so each framework's registrations are documented.
- **The stdlib `net/http` surface is always layered underneath** — it never
  appears in `go.mod`, so it can't be "detected" as a second framework; instead
  a receiver-scoped subset of its config is always merged.

Receiver scoping is what makes the merge safe: a pattern like *`Handle` on
`*mux.Router`* can never claim another framework's calls, so it is inert unless
that framework is actually routing. Unscoped patterns are *not* merged from
secondaries, because gin's `Handle(method, path, h)` and mux's `Handle(path, h)`
would each misparse the other's calls.

| Scenario | Supported? |
|---|---|
| Framework API + plain `net/http` ServeMux endpoints in one binary | ✅ both documented |
| Two frameworks side by side (e.g. gin API + mux admin router) | ✅ both documented, correct verbs |
| Raw `*http.Request` reads (headers, query, `PathValue`) inside framework handlers | ✅ documented as parameters |
| A framework router **mounted under** a `net/http` mux (`root.Handle("/api/", http.StripPrefix("/api", chiRouter))`) | ✅ the mount prefix composes across the boundary (`/api/users`) |
| Mounts wired through a *secondary* framework's own `Mount`-style calls | ✅ those patterns are receiver-scoped in their home configs, so they survive the merge |
| Which framework is *primary* changing the output (it is decided by file-walk order) | ✅ it doesn't — every framework keeps its own patterns and type mappings whichever one leads, pinned by a rename-invariance test that compares the two specs in full |
| A user-supplied `--config` | framework **patterns** are never auto-augmented — what you write is what matches. Library *presets* still apply on top (auth-scheme mappings by import, and CLI entrypoint fields) |

## Examples

Each block below is a code shape APISpec resolves, with the Go it reads and the
OpenAPI it emits.

<details>
<summary><strong>Type alias and enum resolution</strong></summary>

```go
type AllowedUserType string

const (
    UserTypeAdmin    AllowedUserType = "admin"
    UserTypeCustomer AllowedUserType = "user"
)

type Permission struct {
    AllowedUserTypes []domain.AllowedUserType // → []string in the schema
}

type UserID *int64
type User struct {
    ID UserID // → integer / int64
}
```

A field's enum is built from the constants declared with **that** type. Two types sharing an underlying type (`type Status string` and `type Band string`) are different types, so one never lends its values to the other — and where the values genuinely cannot be attributed to one type, no enum is emitted rather than a guess.

</details>

<details>
<summary><strong>Array support</strong></summary>

```go
type User struct {
    ID     [16]byte   // string, format: byte, maxLength: 16
    Scores [5]int     // array, minItems/maxItems: 5
    Tags   [10]string // array, minItems/maxItems: 10
}

type Config struct {
    Values [...]int   // array, no size constraint
}
```

</details>

<details>
<summary><strong>Validator tag support</strong></summary>

| Validator tag        | OpenAPI mapping                       |
|----------------------|---------------------------------------|
| `required`           | `required: true`                      |
| `omitempty`          | `required: false`                     |
| `min=N`              | `minimum: N`                          |
| `max=N`              | `maximum: N`                          |
| `len=N`              | `minLength: N, maxLength: N`          |
| `email`              | `format: email`                       |
| `url`                | `format: uri`                         |
| `uuid`               | `format: uuid`                        |
| `oneof=a b`          | `enum: [a, b]`                        |
| `alphanum`           | `pattern: "^[a-zA-Z0-9]+$"`           |
| `alpha`              | `pattern: "^[a-zA-Z]+$"`              |
| `numeric`            | `pattern: "^[0-9]+$"`                 |
| `containsany=chars`  | `pattern: ".*[chars].*"`              |
| `e164`               | `pattern: "^\\+[1-9]\\d{1,14}$"`      |
| `dive`               | rules after it apply to the **elements** (`items.*`) |

Struct-level (cross-field) rules on a blank marker field (`` _ struct{} `validate:"gtefield=Min"` ``) surface as a schema `description` note.

</details>

<details>
<summary><strong>Go 1.22 <code>net/http.ServeMux</code> method-aware routing</strong></summary>

```go
mux := http.NewServeMux()
mux.HandleFunc("GET /users/{id}", getUser) // method + wildcard
mux.HandleFunc("POST /users", createUser)

func getUser(w http.ResponseWriter, r *http.Request) {
    id := r.PathValue("id") // → path parameter "id"
    // ...
}
```

`GET /users/{id}` becomes a `GET` operation on `/users/{id}` with an `id` path
parameter, and `POST /users` becomes a `POST` with its request body inferred as
usual. ServeMux-only syntax is normalised: trailing wildcards `{path...}`
collapse to `{path}` and the `{$}` end-of-path anchor is dropped.

</details>

<details>
<summary><strong>Inline anonymous struct request / response bodies</strong></summary>

```go
func createOrder(w http.ResponseWriter, r *http.Request) {
    var req struct {
        Items []itemReq `json:"items"`
    }
    if err := render.DecodeJSON(r.Body, &req); err != nil {
        http.Error(w, "invalid JSON", http.StatusBadRequest)
        return
    }
    // ...
}
```

APISpec captures the local `struct{...}` type structurally from `go/types`,
emits an inline `object` schema on the `requestBody`, and promotes named field
types (`itemReq` here) to their own components with `$ref`. Nested anonymous
structs stay inlined.

</details>

<details>
<summary><strong>Wrapper / envelope response specialisation</strong></summary>

Many services wrap every response in a shared envelope whose payload field is
`interface{}` — so the concrete type is only knowable at the call site:

```go
type Envelope struct {
    Message string      `json:"message"`
    Data    interface{} `json:"data"`
    Code    int         `json:"code"`
}

func RespondWithSuccess(w http.ResponseWriter, message string, data interface{}, code int) {
    _ = json.NewEncoder(w).Encode(NewEnvelope(message, data, code))
}

func listOrders(w http.ResponseWriter, r *http.Request) {
    common.RespondWithSuccess(w, "ok", orders.Order{...}, http.StatusOK)
}
```

APISpec follows the assignment + constructor + parameter chain to recover the
caller-site payload type, then composes a per-route schema:

```yaml
allOf:
  - $ref: '#/components/schemas/Envelope'   # base wrapper (message, code, …)
  - type: object
    properties:
      data:
        $ref: '#/components/schemas/Order'   # recovered per-route payload
```

Only genuinely generic fields (`interface{}`/`any`) are overridden; concrete
fields like `message`/`code` keep rendering from the base schema. The recovered
payload type is always registered as a component, so the `data` `$ref` never
dangles.

</details>

<!-- markdownlint-disable MD033 -->
<a id="security--authentication-detection"></a>
<!-- markdownlint-enable MD033 -->

<details>
<summary><strong>Security / authentication detection</strong></summary>

APISpec detects auth middleware and marks the routes it protects, framework-agnostically. Detection has two halves, both config-driven:

- **Scope** (`framework.securityPatterns`) — recognises *how* middleware is applied and how far it reaches: `router` (chi/echo/gin/mux `Use`), `subtree` (group/route closures), `route` (chi `With`, per-route middleware args), and `wrapper` (a handler wrapped by an auth function).
- **Identity → scheme** (`securityMappings`) — resolves *which middleware* to one or more OpenAPI security requirements, by function name, package, and/or receiver type.

```go
r := chi.NewRouter()
r.Get("/health", health)              // open

r.Group(func(r chi.Router) {
    r.Use(jwtauth.Verifier(tokenAuth)) // subtree-wide auth
    r.Get("/me", me)                   // → security: [{ bearerAuth: [] }]
})

r.With(authMiddleware).Get("/admin", admin) // per-route chain → protected
```

Common JWT/auth libraries are recognised with **zero config** via an import detector (echo-jwt, appleboy/gin-jwt, gofiber/contrib/jwt, golang-jwt validation calls, and more). Explicitly-public routes (skipper / `AllowUnauthenticated` style middleware) render `security: []`.

When a custom middleware can't be mapped to a scheme automatically, `apispec` **warns and lists** the unresolved middleware; `apispecui` surfaces the same list with a picker to assign a scheme interactively. To map one yourself:

```yaml
securityMappings:
  - functionNameRegex: ^authMiddleware$
    recvTypeRegex: Handler           # optional: method-value middleware
    schemes:
      - { bearerAuth: [] }           # entries here are ANDed
  # An apiKey middleware that carries WHERE the key travels in its own config:
  # the scheme's in/name are read from that field rather than assumed.
  - functionNameRegex: ^KeyAuthWithConfig$
    pkgRegex: ^example\.com/auth$
    schemes:
      - { apiKeyAuth: [] }
    lookupField: KeyLookup           # "<header|query|cookie>:<name>"
    lookupArgIndex: 0                # default: the first argument
  # OR alternatives (any one satisfies):
  - functionNameRegex: ^New$
    pkgRegex: github\.com/golang-jwt/.*
    schemesAnyOf:
      - [ { bearerAuth: [] } ]
      - [ { apiKeyAuth: [] } ]
  # Mark a middleware as making routes explicitly public:
  - functionNameRegex: ^AllowPublic$
    public: true
  # Mark a middleware as known non-auth so it's not reported as unresolved
  # (logging, CORS, recovery, request-id, …). Mutually exclusive with
  # schemes, schemesAnyOf, and public:
  - functionNameRegex: ^(Logger|Recoverer|RequestID)$
    pkgRegex: github\.com/go-chi/chi/v5/middleware
    skip: true
```

Well-known non-auth middleware from the major frameworks' own middleware packages (chi, echo, gin/gin-contrib, fiber, gorilla/handlers) is **skipped automatically** by import-gated presets, so the unresolved list stays focused on middleware that's genuinely yours to map. In `apispecui`, each item in the unresolved list also has a one-click **Skip** button.

</details>

