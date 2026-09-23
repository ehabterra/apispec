# APISpec: Generate OpenAPI from Go code

[![Coverage](https://img.shields.io/badge/coverage-96.0%25-brightgreen.svg)](https://github.com/ehabterra/apispec)
[![Go Version](https://img.shields.io/badge/go-1.26+-00ADD8?style=flat&logo=go)](https://go.dev/)
[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](https://github.com/ehabterra/apispec/blob/main/LICENSE)
[![GitHub Actions](https://img.shields.io/github/actions/workflow/status/ehabterra/apispec/ci.yml?branch=main&label=CI&logo=github)](https://github.com/ehabterra/apispec/actions/workflows/ci.yml)
[![Tests](https://img.shields.io/github/actions/workflow/status/ehabterra/apispec/test.yml?branch=main&label=Tests&logo=github)](https://github.com/ehabterra/apispec/actions/workflows/test.yml)
[![Lint](https://img.shields.io/github/actions/workflow/status/ehabterra/apispec/lint.yml?branch=main&label=Lint&logo=github)](https://github.com/ehabterra/apispec/actions/workflows/lint.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/ehabterra/apispec.svg)](https://pkg.go.dev/github.com/ehabterra/apispec)
[![GitHub release](https://img.shields.io/github/v/release/ehabterra/apispec?include_prereleases&sort=semver)](https://github.com/ehabterra/apispec/releases)

<!-- markdownlint-disable MD033 -->
<div align="center">
  <img src="logo.png" alt="APISpec Logo" width="200">
</div>
<!-- markdownlint-enable MD033 -->

**Point APISpec at a Go module and get an OpenAPI 3.1 spec.** It detects your
router, follows the call graph from every route registration into the real
handler, and reads parameters, request bodies and response schemas from the
types the compiler already resolved.

No annotations to write. No code to change. Nothing is executed — it is static
analysis, so the spec can be regenerated in CI on every commit.

```bash
brew install ehabterra/tap/apispec
cd my-go-service && apispec -o openapi.yaml
```

**Routers:** Gin · Echo · Chi · Fiber · Gorilla Mux · `net/http` — including
binaries that serve several of them, and projects that put their own router type
in front of one.

📖 **[apispec.ehabterra.com](https://apispec.ehabterra.com)** — installation, CLI
reference, CI drift checking, and a per-framework walkthrough showing the spec
APISpec actually produces. Coming from an annotation-based tool? See
[APISpec vs swaggo/swag](https://apispec.ehabterra.com/vs/swaggo/).

![apispecui — generate an OpenAPI 3.1 spec from Go source, no annotations](docs/demo.gif)

▶︎ **[Full 2-minute walkthrough on YouTube](https://youtu.be/PEG8gDXeOGE)** —
install → generate → explore → configure → insight.

---

## Is APISpec right for your project?

A generated spec gets used for very different jobs, and APISpec is not equally
good at all of them. The honest breakdown, so you can decide in a minute rather
than after an afternoon:

| You want to… | Fit |
|---|---|
| Generate **types/clients for your own frontend, mobile app or services** | ✅ **This is the sweet spot.** Routes, methods and request/response types come from the compiler's own type information. |
| **Catch drift in CI** — fail the build when code and spec disagree | ✅ Output is deterministic; an unchanged project regenerates byte-identically, so any diff is real. |
| **Document an existing service** that has no spec at all | ✅ Nothing to annotate — point it at the module and read the result. Set [`naming`](docs/CONFIGURATION.md#naming) for readable operation ids. |
| Publish a **contract for partners or third-party SDKs** | ⚠️ **Review it first.** The error contract, field optionality and some media types are under-stated — see [LIMITATIONS.md](docs/LIMITATIONS.md#what-apispec-does-not-state). Treat the output as a draft to curate. |
| Work **spec-first** — the spec is the source of truth, the code follows | ❌ Wrong tool by design. APISpec derives the spec *from* the code. Use oapi-codegen or ogen; APISpec can still document the server they generate. |
| Document routes assembled at **runtime** (route tables, config files, plugins) | ❌ Statically unknowable. APISpec reports these rather than guessing — [`UnresolvedPaths()`](docs/LIMITATIONS.md#what-apispec-cannot-see) names each one. |

> [!IMPORTANT]
> **By default APISpec exits `0` even when it could not resolve everything.**
> Unresolved paths, truncated expansion and unmapped auth middleware are
> reported on stderr and through the Go API, but they fail the command only
> under [`--strict`](#fail-the-build-when-the-spec-comes-out-incomplete). If the
> spec matters, turn it on in CI and diff the committed spec.
> [Guardrails worth having →](docs/LIMITATIONS.md#guardrails-worth-having)

## Quick start

### Install

```bash
# Homebrew (macOS/Linux) — nothing to compile
brew install ehabterra/tap/apispec      # the CLI
brew install ehabterra/tap/apispecui    # the web UI

# or with Go (make sure $HOME/go/bin is on PATH)
go install github.com/ehabterra/apispec/cmd/apispec@latest
```

Pre-built binaries for six platforms, an install script and build-from-source
instructions are in [docs/INSTALLATION.md](docs/INSTALLATION.md).

### Generate

Run from anywhere inside your Go module:

```bash
apispec -o openapi.yaml           # YAML, framework auto-detected
apispec -o openapi.json           # JSON
apispec ./cmd/api -o openapi.yaml # or point at a subdirectory
```

### Fail the build when the spec comes out incomplete

By default a shortfall is a warning on stderr at most, and the run exits `0`.
In CI those lines scroll past, and their effect is invisible in the spec diff:
an operation whose `security` block was dropped reads exactly like an operation
that genuinely has none. `--strict` makes the run exit **3** instead — distinct
from `1`, so a script can tell "apispec could not run" from "apispec ran and
the result is below the bar".

```bash
apispec -o openapi.yaml --strict                      # gate on everything
apispec -o openapi.yaml --strict=security,paths       # only these (attach with =)
```

| Category | Fails when | What it costs the document |
|---|---|---|
| `security` | auth middleware matched no `securityMappings` entry | the endpoints behind it are documented as **public** |
| `paths` | a registration's path is built at runtime, or nothing matched | the endpoint is **missing entirely** |
| `schemas` | a response came out with an empty schema, or a `$ref` had no component | the operation is there, its **shape is not** |
| `truncation` | an expansion budget (`--max-nodes`, `--max-nodes-per-route`) cut the walk short | the operation reads as finished with **fewer params/responses** than the code has |
| `packages` | in-module packages failed to load or parse | whatever they registered is absent, by an **unknown amount** |

The spec is written either way, and is byte-identical to a non-strict run —
`--strict` decides an exit code, never a document — so a failing job can still
publish the artifact and diff it.

One case has no warning to promote: `security` also fires when the config
declares no `securityMappings` at all. The stderr line stays quiet there
because auth detection is effectively off and it would be noise on every run,
but that is the *worst* case of this finding — every guarded endpoint is
public in the document — not an exempt one.

### What comes out

Given ordinary Gin code with no annotations:

```go
// Get a user by ID
func GetUser(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	// ...
	c.JSON(http.StatusOK, user)
}

r.GET("/users/:id", GetUser)
```

APISpec produces:

```yaml
/users/{id}:
  get:
    summary: Get a user by ID        # from the doc comment
    operationId: getUsersById
    parameters:
      - name: id                     # from c.Param("id")
        in: path
        required: true
        schema: { type: string }
    responses:
      "200":                         # from c.JSON(http.StatusOK, user)
        content:
          application/json:
            schema: { $ref: '#/components/schemas/User' }
      "400":
        content:
          application/json:
            schema: { $ref: '#/components/schemas/H' }
```

Both status codes, the path parameter, the summary and the `User` schema come
from the code itself — the handler was reached by following `r.GET`'s handler
argument, not by matching a comment. (Names are shown with the `naming` block
from the next section; by default they are fully-qualified Go symbols.)

### Make the names readable

By default an `operationId` is the fully-qualified Go symbol
(`github.com/acme/api/internal/httpapi.GetUser`) and a schema is that symbol with
separators swapped. Those never collide, which is why they are the default — but
they leak your module layout and make generated clients unwieldy. Choose shorter
styles on the command line:

```bash
apispec --operation-id method-path --schema-names short -o openapi.yaml
```

or keep them in a config file, which is layered over the detected framework's
patterns — a file holding only this block is enough:

```yaml
naming:
  operationId: method-path   # full (default) | receiver-method | method-path
  schemaNames: short         # full (default) | short
```

A flag overrides the file. Short schema names are qualified only where two types
share a name, and then every member of that group is qualified, so no name wins
for a reason you cannot see.

## Framework support

| Framework | Routes & methods | Path params | Groups / mounting | Request body | Responses | Auth |
|---|:-:|:-:|:-:|:-:|:-:|:-:|
| **[Gin](https://apispec.ehabterra.com/gin-openapi-generator/)** | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| **[Echo](https://apispec.ehabterra.com/echo-openapi-generator/)** | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| **[Chi](https://apispec.ehabterra.com/chi-openapi-generator/)** | ✅ | ✅ | ✅ (incl. `render`) | ✅ | ✅ | ✅ |
| **[Fiber](https://apispec.ehabterra.com/fiber-openapi-generator/)** | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| **[Gorilla Mux](https://apispec.ehabterra.com/gorilla-mux-openapi-generator/)** | ✅ | ✅ | ✅ (`PathPrefix`, `Subrouter`) | ✅ | ✅ | ✅ |
| **[`net/http`](https://apispec.ehabterra.com/net-http-openapi-generator/)** | ✅ (Go 1.22 method patterns) | ✅ (`{id}`, `r.PathValue`) | basic | ✅ | ✅ | ✅ |

Beyond the matrix, and with no configuration: **your own router and context
types** (`func (r *Router) Get(...)`, `ctx.JSON/Bind/Query`), **mixed
multi-framework binaries**, **CLI-dispatched servers** (cobra, urfave/cli),
`switch r.Method` dispatch, framework error sentinels a handler returns
(`return echo.ErrForbidden` documents a 403), generics, embedded and inline
structs, enums from constants, and `go-playground/validator` constraints.

📖 The full list, with worked examples: **[docs/CAPABILITIES.md](docs/CAPABILITIES.md)**
· What it cannot see: **[docs/LIMITATIONS.md](docs/LIMITATIONS.md)**

## A route is missing — what now?

APISpec logs a per-stage summary on every run, so start there. The common cases:

| Symptom | Likely cause | Fix |
|---|---|---|
| **0 paths**, "no route registrations matched" | unsupported router, an unmatched wiring style, or an over-eager `--exclude-*` | [DEBUGGING.md step 1](docs/DEBUGGING.md) |
| A whole package's routes missing | package not loaded or filtered out | check `--include-*`/`--exclude-*`, `--skip-cgo`, module root |
| Some routes missing **+ a truncation warning** | the walk's node budget is spent | raise `--max-nodes` |
| Route present, **body/params empty** | the handler was located but the binding style wasn't recognised | insight report ([step 4](docs/DEBUGGING.md)) |
| Path contains `{someVar}` / `{someFunc}` | part of the path is built at runtime | statically unknowable — the rest of the path is real |
| Routes from `for … range routeTable` missing | runtime values | named on stderr; register statically or accept the gap |
| Everything empty right after adding `--config` | your config explicitly empties a list (`routePatterns: []`) | drop that key to inherit the detected patterns |

📖 **[docs/DEBUGGING.md](docs/DEBUGGING.md)** walks through the effective config,
the metadata dump, the call-graph diagram and the per-route insight report — and
what to include if you file an issue.

## The three binaries

| Binary | What it's for |
|---|---|
| **`apispec`** | The generator. Auto-detects the framework, writes YAML or JSON. |
| **`apispecui`** | Local web UI: configure interactively, preview through Swagger UI / Redoc / Scalar, browse the call graph, and read the **Insight** report that explains *why* a response has no body. [Tour below ↓](#the-web-ui) |
| **`apidiag`** | The call-graph explorer on its own, for when you only want the diagram. |

```bash
apispecui --dir ./my-go-project     # http://localhost:8088
apidiag   --dir ./my-go-project     # http://localhost:8080
```

📖 Every flag, and the `apispecui` HTTP API: **[docs/TOOLS.md](docs/TOOLS.md)**

### The web UI

`apispecui` has one view per job, on the left rail:

| View | What it is for |
|---|---|
| **⚡ Start / Spec** | Pick the project, press **Generate**, and read the result in Swagger UI, Redoc or Scalar. The status line says how the run went: path count, skipped packages, a truncated walk, and the strict verdict. |
| **⚙ Configure** | Every config key and run option, in five groups. The **Jump to** list and the filter box get you to one setting fast. See below. |
| **◷ Insight** | Explains how the spec came out. **Overview** covers the whole API; **Endpoint** covers one route, with its call trace and a complexity grade. |
| **⌕ Call graph** | Opens the interactive call-graph and tracker-tree explorer (`/diagram`). |

**Configure** is ordered by the question each group answers:

1. **Document**: title, external docs, servers, tags, default media types.
2. **Types, naming & overrides**: operationId/schema naming, doc-comment
   descriptions, schema shape, type mappings, external types, per-handler
   overrides.
3. **Security**: schemes, plus which middleware applies them. It opens by
   itself when a run finds middleware it could not map.
4. **Analysis & scope**: package selection, include/exclude filters,
   virtual hosts, expansion limits, strict mode.
5. **Detection (advanced)**: the framework pattern lists. The detected
   framework's presets fill these, so most projects never open this group.

Changes feed the next **Generate** directly. **Save config as…** writes them
to an `apispec.yaml` the CLI reads.

**Insight ▸ Overview** has three layers, each answering one question:

- **Brief**: is the spec in good shape? One line, the health score, and the
  basic facts: framework, routes, operations and schemas.
- **Needs attention**: what should I do? One ranked list covering broken
  references, unresolved types, defaulted statuses, unmapped auth middleware
  and failing gated strict checks. Each row expands to the routes it
  affects, and a route opens in the Endpoint view. A row that config can
  fix has a button to the right Configure group.
- **At a glance**: how does each facet look? Seven tiles, all read the same
  way (label, value, bar, caption), in two rows. *How complete is the spec*
  holds Resolution, Response bodies, Coverage and the Quality
  gate, which is the `--strict` check counted on every run. *What the API is*
  holds Security, API shape and How it was read. Select a tile to open its
  detail in a side drawer; ← and → step through the tiles without closing it.

**Export to AI** packages the issues, the trace and the handler source as
Markdown for an assistant. Identifiers can be redacted.

## Configuration

**Most projects need none.** APISpec detects the framework and loads its bundled
patterns. Reach for a config file when you want to:

| Goal | Key |
|---|---|
| Set the title/version/contact in the document | [`info`](docs/CONFIGURATION.md#info) (or `--title`, `--api-version`, …) |
| Get readable operation ids and schema names | [`naming`](docs/CONFIGURATION.md#naming) |
| Pin how a Go type renders (`uuid.UUID`, `time.Time`, a house enum) | [`typeMapping`](docs/CONFIGURATION.md#typemapping) / [`externalTypes`](docs/CONFIGURATION.md#externaltypes) |
| Keep internal packages or ops endpoints out of the spec | [`include` / `exclude`](docs/CONFIGURATION.md#include--exclude) |
| Map a house auth middleware to a security scheme | [`securityMappings`](docs/CONFIGURATION.md#security-security-securityschemes-securitymappings) |
| Fix a summary, description or response by hand | [`overrides`](docs/CONFIGURATION.md#overrides) |
| Teach it a bespoke router, wrapper or decoder | [`framework`](docs/CONFIGURATION.md#framework-advanced) |

A config is layered over the detected defaults, so write only what you want to
change. `apispec --output-config used-config.yaml` shows the complete
configuration that ran, which is where to copy a block from.

📖 Field-by-field reference: **[docs/CONFIGURATION.md](docs/CONFIGURATION.md)**

## Use it as a library

```go
import (
    "os"

    "github.com/ehabterra/apispec/generator"
    "github.com/ehabterra/apispec/spec"
    "gopkg.in/yaml.v3"
)

func main() {
    cfg := spec.DefaultGinConfig() // or spec.LoadAPISpecConfig("apispec.yaml")
    gen := generator.NewGenerator(cfg)

    openapi, err := gen.GenerateFromDirectory("./your-project")
    if err != nil {
        panic(err)
    }
    if unresolved := gen.UnresolvedPaths(); len(unresolved) > 0 {
        // routes whose path is only knowable at runtime — fail your build here
    }
    data, _ := yaml.Marshal(openapi)
    _ = os.WriteFile("openapi.yaml", data, 0644)
}
```

📖 [pkg.go.dev reference](https://pkg.go.dev/github.com/ehabterra/apispec)

## Limits and performance

APISpec bounds its own analysis, so a deep or cyclic call graph produces a
truncation warning instead of a hang. The defaults suit most projects; very
large ones need the node budgets raised.

| Parameter | Default | Flag | When it runs out |
|---|---|---|---|
| Max nodes / tree | 50,000 | `--max-nodes` | routes go **missing** — raise it |
| Max nodes / route | 1,000,000 | `--max-nodes-per-route` | one named route is **less detailed** |
| Max children / node | 500 | `--max-children` | a node's children are truncated |
| Max instances / key | 75 | `--max-instances-per-key` | a shared helper's copies are capped, so some response bodies may be empty |

Every truncation is logged with the node it happened at, so a short spec is never
silent. The engine is lazy — subtrees are expanded only along the paths a query
actually touches.

📖 The reasoning behind each limit, measured trade-offs and profiling:
**[docs/PERFORMANCE.md](docs/PERFORMANCE.md)**

## How it works

Ten stages, each consuming the previous one's output: locate the module → load
and type-check with `go/packages` → detect the framework → merge configuration →
generate metadata (types, functions, call edges, assignments) → expand the
tracker tree from each route registration → match patterns to find methods,
paths, params, bodies and responses → map Go types to OpenAPI schemas → serialize
deterministically → emit diagrams, the effective config and diagnostics.

📖 Stage by stage, with the data each one produces:
**[docs/PIPELINE.md](docs/PIPELINE.md)**

## Documentation

| Doc | What's in it |
|---|---|
| [docs/CAPABILITIES.md](docs/CAPABILITIES.md) | Every routing, request, response and type shape APISpec resolves, with examples |
| [docs/LIMITATIONS.md](docs/LIMITATIONS.md) | What it can't see, what it doesn't state, and the guardrails to add |
| [docs/DEBUGGING.md](docs/DEBUGGING.md) | **My route is missing** — the debugging path |
| [docs/CONFIGURATION.md](docs/CONFIGURATION.md) | Field-by-field configuration reference |
| [docs/TOOLS.md](docs/TOOLS.md) | `apispec`, `apispecui`, `apidiag` — all flags and endpoints |
| [docs/INSTALLATION.md](docs/INSTALLATION.md) | Every installation method, per platform |
| [docs/PERFORMANCE.md](docs/PERFORMANCE.md) | Limits, profiling and measured trade-offs |
| [docs/PIPELINE.md](docs/PIPELINE.md) | The analysis pipeline, stage by stage |

Design notes and internals live in [docs/](docs/) — `TYPE_MODEL.md`,
`AUTH_DETECTION_DESIGN.md`, `INTERFACE_RESOLUTION.md`, `TRACKER_TREE_USAGE.md`.

## Contributing

Issues and PRs are welcome — a minimal `main.go` reproducing a wiring style that
APISpec misses is the single most useful thing you can send, because it becomes
a test fixture. See [CONTRIBUTING.md](CONTRIBUTING.md) for the development setup,
project layout, and how to add a framework, plus
[CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md).

## Forks & derivatives

APISpec is Apache-2.0, so anyone is free to build on it — and someone has:

- **[antst/go-apispec](https://github.com/antst/go-apispec)** — a fork of this project by Anton
  Starikov, also Apache-2.0, with a substantially reworked analysis pipeline. Credit to him for
  taking the idea further and spending the time to make it his own; that is exactly what the
  licence is for.

Building on APISpec yourself? Open an issue or a discussion — downstream projects are welcome here.

## License

Apache License 2.0 — see [LICENSE](LICENSE).
