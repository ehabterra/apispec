# Framework-Agnostic Authentication / Security Detection — Design

Status: proposed
Scope: detect authentication requirements from Go source and emit OpenAPI
`securitySchemes` (catalog) and per-operation `security` (which paths are
protected, and by what).

## 1. Principles

1. **Framework-agnostic.** Auth is detected through the same config-driven
   pattern system that already powers route/mount/request/response/param
   extraction. No router is special-cased in core logic. chi, echo, gin, fiber,
   gorilla/mux and net/http are expressed purely as default config presets,
   exactly like `DefaultEchoConfig()` etc.
2. **Exhaustive wiring coverage.** Every realistic way to attach auth must be
   expressible (see §3). If a new framework or idiom appears, it is added by
   config, not by changing the engine.
3. **Two orthogonal concerns.** "Where does auth apply?" (scope) and "Which
   scheme is it?" (identity) are separated so a small config covers a large
   matrix of cases (§4).
4. **Reuse existing propagation.** Security context flows down the tracker tree
   the same way `mountPath` / `mountTags` / `mountDynParams` already do in
   `extractor.go::traverseForRoutesWithVisited`.
5. **Graceful + honest.** Unrecognized middleware is reported (not silently
   dropped); config can always override or force a route public.

## 2. What exists today (baseline)

- OpenAPI types complete: `SecurityScheme`, `OAuthFlows/Flow`,
  `SecurityRequirement`, `Operation.Security`, `Components.SecuritySchemes`
  (`internal/spec/openapi.go`).
- Config holds **global** `Security` and `SecuritySchemes`
  (`internal/spec/config.go:420-421`); the mapper copies them to the spec root
  only (`internal/spec/mapper.go:136-147`).
- `Operation.Security` is defined but **never populated**.
- No middleware is detected anywhere; `MountInfo`/`RouteInfo` carry no auth.

## 3. The wiring matrix (must all be covered)

| Granularity | chi | echo | gin | fiber | gorilla/mux | net/http |
|---|---|---|---|---|---|---|
| Global | `r.Use(mw)` at root | `e.Use(mw)` | `r.Use(mw)` | `app.Use(mw)` | `r.Use(mw)` | wrap top handler |
| Group / subtree | `r.Group(func(r){ r.Use(mw); … })`, `r.Route(…)` | `g := e.Group("/x", mw)`, `g.Use(mw)` | `g := r.Group("/x", mw…)` | `g := app.Group("/x", mw)` | `sub := r.PathPrefix("/x").Subrouter(); sub.Use(mw)` | per-mux wrap |
| Per-route inline | `r.With(mw).Get(…)` | `e.GET("/x", h, mw…)` (variadic after handler) | `r.GET("/x", mw, h)` (chain before handler) | `app.Get("/x", mw, h)` | wrap handler arg | `mux.Handle("/x", mw(h))` |
| Handler-wrap | `r.Get("/x", Auth(h))` | `e.GET("/x", Auth(h))` | — | — | `r.Handle("/x", Auth(h))` | `Auth(http.HandlerFunc(h))` |
| Config/manual | global `security` + per-function override (all frameworks) |

**Identity sources** (what value is the middleware): a user-defined function
(`authMiddleware`, `h.RequireAuth`), a library constructor call
(`jwt.New(...)`, `echojwt.WithConfig(...)`, `middleware.BasicAuth(...)`,
`ginjwt.New(...)`), or a variable assigned from either.

## 4. Core model: Scope × Identity

### 4a. `SecurityPattern` — scope detection (mirrors `MountPattern`)

Recognizes a *middleware-application call* and says **where** its middleware
applies. Lives under `FrameworkConfig.SecurityPatterns`.

```go
type SecurityPattern struct {
    // Match the application call (same matcher fields as other patterns).
    CallRegex         string // e.g. `^Use$`, `^With$`, `^Group$`, `^(GET|POST|…)$`
    FunctionNameRegex string
    RecvType          string
    RecvTypeRegex     string // e.g. chi.*Mux / echo.*(Echo|Group)

    // Scope: how far the matched middleware reaches.
    //   "router"  — applies to routes registered on the SAME receiver var,
    //               in the same scope, AFTER this call (chi/echo/gin/mux Use).
    //   "subtree" — applies to everything in the mounted subtree
    //               (Group/Route closures, echo/gin/fiber Group(mw…)).
    //   "route"   — applies to this single route registration call
    //               (chi With, echo/gin/fiber per-route middleware args).
    //   "wrapper" — the handler argument is wrapped by an auth function;
    //               the wrapping call's identity is the middleware.
    Scope string `yaml:"scope"`

    // Where the middleware value(s) live on the matched call.
    MiddlewareArgIndex int  `yaml:"middlewareArgIndex"` // first middleware arg
    MiddlewareVariadic bool `yaml:"middlewareVariadic"` // collect args from index..end
    MiddlewareFromRecv bool `yaml:"middlewareFromRecv"` // value is the receiver (rare)
    HandlerArgIndex    int  `yaml:"handlerArgIndex"`    // for scope=wrapper / route

    // Package/type filtering (same as other patterns).
    CallerPkgPatterns, CalleePkgPatterns,
    CallerRecvTypePatterns, CalleeRecvTypePatterns []string
}
```

### 4b. `SecurityMapping` — identity resolution (framework-agnostic)

Maps a middleware *value* (resolved to a function/constructor/selector) to one
or more OpenAPI security requirements. Top-level config so it is shared across
frameworks.

```go
type SecurityMapping struct {
    // Match the resolved middleware identity.
    FunctionNameRegex string // e.g. `^authMiddleware$`, `^RequireAuth$`, `^New$`
    PkgRegex          string // e.g. `github.com/golang-jwt/.*`, `.*/middleware`
    RecvTypeRegex     string // for method-value middleware (h.authMiddleware)

    // Resulting requirement(s). Each entry is one scheme name + scopes;
    // multiple entries in Schemes => AND (all required). Use SchemesAnyOf for OR.
    Schemes     []SecurityRequirement `yaml:"schemes"`     // AND
    SchemesAnyOf [][]SecurityRequirement `yaml:"schemesAnyOf,omitempty"` // OR groups

    // If true, a match means "this scope is explicitly PUBLIC" (clears
    // inherited security) — e.g. a `middleware.Skipper` / `AllowUnauthenticated`.
    Public bool `yaml:"public,omitempty"`
}
```

`securitySchemes` (existing) remains the catalog the mapping names refer to.

**Decision — built-in recognition stays engine-agnostic.** Well-known
libraries are recognized via *default config presets* plus a *detector*, never
hardcoded in the engine. Concretely: a set of bundled `SecurityMapping` presets
(grouped by library, e.g. `golang-jwt`, `echojwt`, `gin-jwt`, `basic-auth`,
`api-key`) ship as data and are merged into the active config (after the
framework preset, before user config so the user always wins). The engine only
ever reads `cfg.SecurityMappings` / `cfg.Framework.SecurityPatterns` — it has no
knowledge of any specific library. A "detector" step selects which library
preset bundles to merge based on the project's imports (already available in
metadata package import lists), so projects get zero-config detection without
the engine special-casing anything. Users override or extend via their own
config.

## 5. Pipeline integration

```
metadata  → tracker tree → extractor traversal → RouteInfo.Security → mapper → openapi Operation.Security
(call edges,   (Use/Group/        (NEW: SecurityPattern matchers +      (NEW field)   (NEW: set op.Security;
 recv var,      With/route          security propagation, mirroring                    collect used schemes)
 args)          nodes already        mountTags)
                present?)
```

### Step 0 — metadata (verify first; likely no change)

`SecurityPattern` matchers run on tracker nodes, so middleware-application
calls and their function-value args must reach the tracker tree. We confirmed:
- `Group` is already matched as a mount (chi/echo configs) → those calls are in
  the tree.
- The call graph records `callee_recv_var_name` (e.g. `r`) per edge — the key
  for `router`-scope correlation (§5b).
- `authMiddleware` appears in metadata as a referenced function + call arg.

**Spike — DONE (✓ no metadata change needed).** Verified on
`testdata/complex_chi_router` (decoded the pooled `metadata.yaml` call graph):

- **`Use` edges survive**, both inside a `Group(func(rg){…})` closure
  (`rg.Use(h.authMiddleware)` at handler.go:37) and at the root
  (`r.Use(...)` ×7 in main.go). `Group`/`Mount` edges are present too.
- **Middleware identity is fully recoverable** from the `CallArgument`:
  - selector `h.authMiddleware` → `kind=selector`, `sel.name="authMiddleware"`,
    `sel.pkg="…/handler"`, `receiver_type="Handler"`, func type carried.
  - constructor `middleware.Timeout(…)` → `kind=call`, `fun.sel.name="Timeout"`,
    `fun.pkg="…/chi/v5/middleware"`.
  - bare `customMiddleware` → `kind=ident`, `name="customMiddleware"`,
    `pkg="complex-chi-router"`.
  All three forms expose name + pkg (+ recv type), enough to drive
  `SecurityMapping` regexes.
- **Scope linkage is recoverable**: the `Group` call's arg is a `func_lit` whose
  ID equals the `caller` of the `Use`/`Mount` edges inside it — so the
  subtree-scope set is "edges whose caller is the matched closure". `recvType`
  distinguishes inner (`chi.Router`) vs root (`*Mux`); source `position`
  gives ordering for `router`-scope "applies to routes registered after".

Conclusion: `SecurityPattern` matchers can run directly on existing tracker
nodes. No call-graph/metadata-builder change is required.

Note (`router` scope): `callee_recv_var_name` is populated on some edges but was
empty for these chi closure edges, so receiver correlation should key on
(caller function/closure id, callee `recvType`) + source-position ordering
rather than relying solely on the recv-var name.

### Step 1 — new pattern matcher

Add `SecurityPatternMatcher` (file `pattern_matchers.go`) and
`e.securityMatchers` in the `Extractor`, initialized in
`initializePatternMatchers()` from `cfg.Framework.SecurityPatterns`, exactly
like the existing matchers. It exposes:
`MatchNode(node) bool`, `Scope() string`, and
`ExtractMiddleware(node) []middlewareRef` where a `middlewareRef` is the
resolved identity (name/pkg/recvType) of each middleware value, plus (for
wrapper/route scope) the handler arg.

### Step 2 — identity resolver

`resolveSecurity(refs []middlewareRef) ([]SecurityRequirement, isPublic)` walks
`cfg.SecurityMappings`, matching each ref; merges results (AND semantics by
default; `schemesAnyOf` produces alternative requirement objects). Unmatched
middleware → recorded in a `[]string` "unresolved middleware" diagnostic list.

### Step 3 — propagate through traversal

Add a `mountSecurity []SecurityRequirement` parameter alongside
`mountTags`/`mountDynParams` in `traverseForRoutesWithVisited`,
`handleMountNode`, `handleRouteNode`, `handleRouterAssignment`:

- **scope=subtree** (Group(mw…), Group/Route closures): resolve schemes and
  pass them into the subtree via `mountSecurity` (accumulate, dedup) — same
  shape as how `mountPath`/`newTags` are pushed to children.
- **scope=router** (bare `Use`): record `(recvVar, schemes)` in a per-scope map
  keyed by the receiver var name. When a sibling route node with the same
  `callee_recv_var_name` is handled afterward, fold those schemes into its
  security. (Order: process a parent's children in source order — positions are
  available — so `Use` precedes the routes it guards.)
- **scope=route** (chi `With`, echo/gin/fiber per-route mw): attach resolved
  schemes only to that one route.
- **scope=wrapper**: the handler arg is the wrapped handler; the wrapping call's
  identity resolves the scheme and attaches to that route.

`Public` mappings clear inherited `mountSecurity` for the affected scope/route.

### Step 4 — RouteInfo + mapper

```go
// RouteInfo (extractor.go)
Security []SecurityRequirement // nil => inherit global; []{} => explicitly public
```

`handleRouteNode` sets `routeInfo.Security = merge(mountSecurity, routeLevel…)`.

In `mapper.go::buildPathsFromRoutes`, set `op.Security = route.Security` when
non-nil. Semantics:
- `nil` → omit (operation inherits the document-level `security`).
- empty non-nil (`[]`) → emit `security: []` (explicitly public, overrides global).
- non-empty → emit the requirements.

Also collect every scheme name referenced by any operation; verify each exists
in `cfg.SecuritySchemes`; if a default-preset mapping fired, auto-add its
catalog entry to `spec.Components.SecuritySchemes`. Warn on dangling names.

## 6. Default presets + detector (zero-config for common stacks)

Two layers, both pure data merged into the active config (engine stays agnostic):

- **Framework scope presets** — add a `SecurityPatterns` block to each
  `config_*.go` (chi/echo/gin/fiber/mux/http), describing the framework's
  Use/Group/With/per-route/wrap idioms.
- **Library identity presets + detector** — bundled `SecurityMapping` groups per
  auth library. A detector inspects the project's import paths (from metadata
  package lists) and merges the matching groups. Merge order:
  `framework preset → library presets → user config` (user always wins).

Framework `SecurityPatterns` cover, e.g.:

- **chi:** `Use`/`With` (router/route), `Group`/`Route` (subtree).
- **echo:** `Use` (router), `Group` mw args (subtree), per-route variadic mw (route).
- **gin:** `Use` (router), `Group` mw args (subtree), per-route leading mw (route).
- **fiber:** `Use` (router), `Group` mw arg (subtree), per-route mw (route).
- **mux:** `Use` (router), `Subrouter` (subtree), wrap (wrapper).
- **net/http:** wrap (wrapper).

Shared mappings (catalog + identity) for well-known auth middleware:
`golang-jwt`/`echojwt`/`gin-jwt` → `bearerAuth` (http/bearer, JWT);
`middleware.BasicAuth` → `basicAuth` (http/basic);
API-key header middlewares → `apiKeyAuth` (apiKey/header). Users override or
extend via config; project-specific middleware (e.g. `authMiddleware`) is mapped
by the user in `securityMappings`.

## 7. Edge cases & semantics

- **AND vs OR**: multiple schemes on one requirement object = AND; multiple
  requirement objects = OR. `schemesAnyOf` builds the OR form.
- **Accumulation**: nested groups accumulate (group A bearer + group B apiKey →
  both). Dedup identical requirements.
- **Public override**: a `Public` mapping or explicit per-route override yields
  `security: []` so a public login route under an authed group is correct.
- **Unknown middleware** (decision): a detected middleware with no matching
  mapping is **never guessed**. The route keeps its inherited security (or none)
  and the middleware identity is added to a diagnostics list.
  - **apispec (CLI):** warn and list the unresolved middleware (function, pkg,
    and the routes/paths it guards) so the user can add a `securityMappings`
    entry. Nothing is emitted for it.
  - **apispecui:** the same list is returned to the UI, which offers an
    interactive picker to assign each unresolved middleware to an existing
    scheme (or define a new one). The selection is written back into the
    generated config's `securityMappings`, so the next run resolves it
    automatically. (Mirrors how the UI already round-trips `securitySchemes` via
    `DetectResponse`/`GenerateRequest` in `cmd/apispecui/main.go`.)
- **Idempotent multi-mount**: a sub-router mounted at two prefixes inherits the
  security of each mount independently (mirrors existing per-prefix routing).

## 8. Output example

```yaml
components:
  securitySchemes:
    bearerAuth: { type: http, scheme: bearer, bearerFormat: JWT }
paths:
  /users/{id}:
    get:
      security:
        - bearerAuth: []
    /login:
      post:
        security: []          # explicit public, overrides a global default
```

## 9. Phased implementation plan

1. **Spike**: ✓ DONE — `Use`/`Group` edges + middleware-arg identity + scope
   linkage all reach the tracker tree; no metadata change needed (see §5 Step 0).
2. ✓ DONE — Config structs `SecurityPattern` (+ scope constants) and
   `SecurityMapping`, wired into `FrameworkConfig.SecurityPatterns` /
   `APISpecConfig.SecurityMappings`, with `validateSecurityConfig()` enforced in
   `LoadAPISpecConfig` and YAML round-trip + validation unit tests
   (`internal/spec/security_config_test.go`). Inert until phase 3.
3. `SecurityPatternMatcher` + identity resolver (`resolveSecurity`).
4. Traversal propagation (`mountSecurity`) + `RouteInfo.Security`.
5. Mapper: populate `Operation.Security`, reconcile catalog, diagnostics.
6. Default presets per framework + import-based library detector/mappings.
7. UI: surface detected schemes; interactive picker to map unresolved
   middleware → scheme, persisted into the generated config.

## 10. Testing

- New `testdata/` fixtures per wiring style: `auth_chi_group`, `auth_chi_with`,
  `auth_echo_group_mw`, `auth_gin_perroute`, `auth_fiber_group`,
  `auth_mux_subrouter`, `auth_nethttp_wrap`, plus a public-override case.
- Snapshot each with `scripts/compare-spec.sh --generate`, then guard with
  `scripts/compare-spec.sh -v <N>` (the auto-discovered set already picks up new
  `testdata/*` dirs).
- Unit tests: matcher scope resolution, AND/OR/public merge, catalog
  reconciliation, dangling-scheme warning.
- Re-run the full external corpus (lmd-core etc.) to confirm no regressions and
  to see real-world protected paths light up.
```
