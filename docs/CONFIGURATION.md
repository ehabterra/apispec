# Configuration reference

APISpec is driven by a YAML configuration file. For most projects the bundled
per-framework defaults are enough and no config is needed — pass `--config`
only when you want to add OpenAPI metadata, map custom types, or teach the
resolver about a framework/wiring style the defaults don't cover.

This document is the field-by-field reference. For a task-oriented introduction
with worked examples, see the [Configuration section of the
README](../README.md#configuration).

## How config is loaded and merged

- **No `--config`** — APISpec detects the framework and loads its built-in
  default config (`internal/spec/config_<framework>.go`).
- **`--config path.yaml`** — your file is loaded *on top of* the detected
  defaults. You only need to specify the keys you want to add or change; the
  framework patterns you omit still apply.
- **CLI flags win.** Values such as `--title`, `--api-version`, and
  `--description` override the corresponding config-file values.
- **Inspect the effective config.** `apispec --output-config used-config.yaml`
  (or `-oc`) writes the fully merged config that was actually used, which is the
  best starting point for a custom file.

```bash
apispec --config apispec.yaml --output openapi.yaml
apispec --output-config used-config.yaml     # dump the effective config
```

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
| `security` | list | Document-level security requirements. |
| `securitySchemes` | map | OpenAPI `securitySchemes` definitions. |
| `securityMappings` | list | Map detected auth middleware to a scheme. |
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

## Security: `security`, `securitySchemes`, `securityMappings`

Most auth setups are detected with **no config** (see the README
[Security & authentication detection](../README.md#security--authentication-detection)
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
```

`securityMappings` is framework-agnostic and works together with
`framework.securityPatterns` (which describes *scope* — router / subtree / route
/ wrapper). See [`AUTH_DETECTION_DESIGN.md`](AUTH_DETECTION_DESIGN.md) for the
full model.

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
      paramIn: path        # path | query | header | cookie
    - callRegex: ^Query$
      paramIn: query
  requestContext:          # disambiguate generic decoders (json.Decode, etc.)
    typeRegexes:
      - ^net/http\.\*Request$
    bodyAccessors:
      - ^Body$
```

Sub-keys of `framework`:

| Key | Purpose |
|-----|---------|
| `routePatterns` | How routes are registered (method/path/handler extraction). |
| `requestBodyPatterns` | Calls that bind a request body to a Go type. |
| `responsePatterns` | Calls that write a response (status + body type). |
| `paramPatterns` | Calls that read a parameter, and its `in:` location. |
| `mountPatterns` | Sub-router mounting (path-prefix composition). |
| `securityPatterns` | Where/how auth middleware is applied (scope). |
| `requestContext` | Which receivers/accessors mark a "request body" source. |

Because these patterns are numerous and framework-specific, the authoritative
reference is the in-repo default configs (`internal/spec/config_*.go`) and the
struct definitions with doc comments in `internal/spec/config.go`. The quickest
way to author a custom pattern is to dump the effective config with
`--output-config` and edit the relevant block.

---

## See also

- [README → Configuration](../README.md#configuration) — examples and quick start
- [`TYPE_MODEL.md`](TYPE_MODEL.md) — how Go types become OpenAPI schemas
- [`AUTH_DETECTION_DESIGN.md`](AUTH_DETECTION_DESIGN.md) — security detection model
- [`INTERFACE_RESOLUTION.md`](INTERFACE_RESOLUTION.md) — interface/return resolution
