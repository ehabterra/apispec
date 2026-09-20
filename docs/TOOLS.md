# The tools

APISpec ships three binaries that share the same analysis engine. The README
covers [which one to reach for](../README.md#the-three-binaries); this is the
full reference, including every CLI flag and the `apispecui` HTTP surface.

| Binary       | Purpose                                                                | Entry point                  |
|--------------|------------------------------------------------------------------------|------------------------------|
| `apispec`    | Generate an OpenAPI 3.1 spec from a Go module                          | `cmd/apispec`                |
| `apispecui`  | Browser UI: configure APISpec, preview the spec, *and* explore the call graph at `/diagram` | `cmd/apispecui` |
| `apidiag`    | Standalone interactive call-graph server (same engine, headless)       | `cmd/apidiag`                |

## `apispec` — CLI generator

The main generator. Auto-detects the framework, loads a default config (overridable with `--config`), and writes an OpenAPI spec.

```bash
# Basic
apispec --output openapi.yaml

# Generate metadata for debugging
apispec --output openapi.yaml --write-metadata

# Limit tuning for very large projects
apispec --output openapi.yaml --max-nodes 100000 --max-children 1000

# Performance profiling
apispec --output openapi.yaml --cpu-profile --mem-profile
```

CLI flags always override values from a config file, and a positional argument
overrides `--dir` (`apispec ./api -o spec.yaml`).

### Full flag reference

| Flag                        | Shorthand | Description                                            | Default                         |
|-----------------------------|-----------|--------------------------------------------------------|---------------------------------|
| `--output`                  | `-o`      | Output path for the OpenAPI spec                       | `openapi.json`                  |
| `--dir`                     | `-d`      | Directory to parse                                     | `.`                             |
| `--title`                   | `-t`      | API title                                              | `Generated API`                 |
| `--api-version`             | `-v`      | API version                                            | `1.0.0`                         |
| `--description`             | `-D`      | API description                                        | `""`                            |
| `--terms`                   | `-T`      | Terms of service URL                                   | `""`                            |
| `--contact-name`            | `-N`      | Contact name                                           | `Ehab`                          |
| `--contact-url`             | `-U`      | Contact URL                                            | `https://ehabterra.github.io/`  |
| `--contact-email`           | `-E`      | Contact email                                          | `ehabterra@hotmail.com`         |
| `--license-name`            | `-L`      | License name                                           | `""`                            |
| `--license-url`             | `-lu`     | License URL                                            | `""`                            |
| `--openapi-version`         | `-O`      | OpenAPI spec version                                   | `3.1.1`                         |
| `--config`                  | `-c`      | Path to custom config YAML                             | `""`                            |
| `--output-config`           | `-oc`     | Write the effective config to a YAML file              | `""`                            |
| `--write-metadata`          | `-w`      | Write `metadata.yaml` to disk                          | `false`                         |
| `--split-metadata`          | `-s`      | Write metadata as multiple files                       | `false`                         |
| `--diagram`                 | `-g`      | Write call-graph HTML to this path                     | `""`                            |
| `--paginated-diagram`       | `-pd`     | Use paginated rendering for the diagram                | `false`                         |
| `--diagram-page-size`       | `-dps`    | Nodes per page in paginated diagram (50–500)           | `100`                           |
| `--max-nodes`               | `-mn`     | Max nodes in the walk that finds route registrations   | `50000`                         |
| `--max-nodes-per-route`     |           | Max nodes expanded below one route registration (lazy engine) | `1000000`                |
| `--max-children`            | `-mc`     | Max children per node                                  | `500`                           |
| `--max-args`                | `-ma`     | Max arguments per function                             | `100`                           |
| `--max-nested-args`         | `-md`     | Max depth for nested arguments                         | `100`                           |
| `--max-recursion-depth`     | `-mrd`    | Max recursion depth (anti-loop)                        | `10`                            |
| `--max-instances-per-key`   |           | Max copies of one callee within an instance scope (lazy engine) | `75`                   |
| `--skip-cgo`                |           | Skip CGO packages                                      | `true`                          |
| `--include-file`            |           | Include files matching pattern (repeatable)            | `""`                            |
| `--include-package`         |           | Include packages matching pattern (repeatable)         | `""`                            |
| `--include-function`        |           | Include functions matching pattern (repeatable)        | `""`                            |
| `--include-type`            |           | Include types matching pattern (repeatable)            | `""`                            |
| `--exclude-file`            |           | Exclude files matching pattern (repeatable)            | `""`                            |
| `--exclude-package`         |           | Exclude packages matching pattern (repeatable)         | `""`                            |
| `--exclude-function`        |           | Exclude functions matching pattern (repeatable)        | `""`                            |
| `--exclude-type`            |           | Exclude types matching pattern (repeatable)            | `""`                            |
| `--analyze-framework-dependencies` | `-afd` | Walk into framework packages during analysis     | `true`                          |
| `--auto-include-framework-packages` | `-aifp` | Auto-include known framework packages          | `true`                          |
| `--auto-exclude-tests`      | `-aet`    | Skip `*_test.go` files                                 | `true`                          |
| `--auto-exclude-mocks`      | `-aem`    | Skip mock files                                        | `true`                          |
| `--cpu-profile`             |           | Enable CPU profiling                                   | `false`                         |
| `--mem-profile`             |           | Enable memory profiling                                | `false`                         |
| `--block-profile`           |           | Enable block profiling                                 | `false`                         |
| `--mutex-profile`           |           | Enable mutex profiling                                 | `false`                         |
| `--trace-profile`           |           | Enable trace profiling                                 | `false`                         |
| `--custom-metrics`          |           | Enable custom metrics collection                       | `false`                         |
| `--profile-dir`             |           | Directory for profiling output                         | `profiles`                      |
| `--cpu-profile-path`        |           | CPU profile filename (within `--profile-dir`)          | `cpu.prof`                      |
| `--mem-profile-path`        |           | Memory profile filename                                | `mem.prof`                      |
| `--block-profile-path`      |           | Block profile filename                                 | `block.prof`                    |
| `--mutex-profile-path`      |           | Mutex profile filename                                 | `mutex.prof`                    |
| `--trace-profile-path`      |           | Trace filename                                         | `trace.out`                     |
| `--metrics-path`            |           | Custom metrics filename                                | `metrics.json`                  |
| `--strict[=categories]`     |           | Exit `3` on a quality shortfall instead of exiting `0`  | off                             |
| `--verbose`                 | `-vb`     | Verbose output (derived patterns, entrypoints, skips)  | `false`                         |
| `--version`                 | `-V`      | Print version and exit                                 | `false`                         |

### `--strict`

Fails a run whose spec came out incomplete, for CI — mostly by promoting
warnings the run already prints. Bare `--strict` gates on everything; a value
(attached with `=`, as with any Go bool flag) narrows it:
`--strict=security,paths`.

| Category | Fails when | What it costs the document |
|---|---|---|
| `security`   | auth middleware matched no `securityMappings` entry | the endpoints behind it are documented as **public** |
| `paths`      | a registration's path is built at runtime, or nothing matched at all | the endpoint is **missing entirely** |
| `schemas`    | a response came out with an empty schema, or a `$ref` had no component | the operation is there, its **shape is not** |
| `truncation` | `--max-nodes` or `--max-nodes-per-route` cut the walk short | the operation reads as finished with **fewer params/responses** than the code has |
| `packages`   | in-module packages failed to load or parse | whatever they registered is absent, by an **unknown amount** |

`security` is the one category that can fire with no warning above it: the
stderr line stays quiet when the config declares no `securityMappings` at all,
since auth detection is then effectively off and it would be noise on every
run. The gate does not exempt that case — it is the worst one.

Exit `3` rather than `1`, so a script can distinguish "apispec could not run"
from "apispec ran and the result is below the bar". `--strict=false` turns the
gate off again, for a wrapper that cannot edit the command it inherits.

The spec is written either way and is byte-identical to a non-strict run: the
flag decides an exit code, never a document.

See also: [`cmd/apispec/README.md`](../cmd/apispec/README.md).

## `apispecui` — Browser-based config & preview

`apispecui` is a small local web server that lets you configure APISpec interactively, generate a spec on demand, immediately preview it through embedded **Swagger UI**, **Redoc**, or **Scalar** viewers, *and* explore the project's call graph at `/diagram`.

```bash
# Install it the same way as apispec (see the README for install methods)
brew install ehabterra/tap/apispecui
# or: go install github.com/ehabterra/apispec/cmd/apispecui@latest

apispecui --dir ./my-go-project

# Open http://localhost:8088 — config UI
# Open http://localhost:8088/diagram — call-graph visualization
```

Flags: `--host` (default `localhost`), `--port` (default `8088`), `--dir`/`-d` (project root, default `.`), `--config`/`-c` (initial config), `--verbose`.

Two things worth knowing before a first run on a large project:

- **Analysis engine** edits the expansion limits (see [PERFORMANCE.md](PERFORMANCE.md)), with the engine's defaults shown as placeholders. A project whose call tree is bigger than the default budget needs them raised.
- When expansion does stop early, the result says so — *"Expansion stopped at the 50000-node limit, so routes beyond that point are missing"* — so a truncated run doesn't read as a complete one with a short route list.

**Insight** (the ◷ tab) reports how the spec was produced, not just what is in it: which frameworks were detected and which one's patterns lead, what the CLI entry-point gate decided, and — per status code — how many responses describe their fields, are a free-form object, were found with an unresolved type, or document no body at all. That last split is the useful one: an empty body at `200` means the write was never followed, while an unresolved type means it was found and needs a type mapping.

**Framework selection matches the CLI.** The UI composes the same multi-framework config the CLI does — the detected primary, every other detected framework merged in receiver-scoped, and the `net/http` surface underneath — so a mixed project documents the same routes either way. The selector chooses which framework *leads*; the rest still merge under it, and the form lists them ("Also detected: gin").

Key endpoints (`cmd/apispecui/main.go` registers more — config load/save, generation progress and cancel, project browsing, health):

| Path                        | Purpose                                                |
|-----------------------------|--------------------------------------------------------|
| `/`                         | Configuration UI                                       |
| `/swagger`                  | Swagger UI preview                                     |
| `/redoc`                    | Redoc preview                                          |
| `/scalar`                   | Scalar preview                                         |
| `/diagram`                  | Interactive call-graph / tracker-tree visualization    |
| `/api/spec.json`            | Last-generated spec (JSON)                             |
| `/api/spec.yaml`            | Last-generated spec (YAML)                             |
| `/api/config.yaml`          | Current effective config                               |
| `/api/generate` (POST)      | Trigger spec generation with the current config        |
| `/api/detect` (GET)         | Detected frameworks + a pre-filled config for the project |
| `/api/insight/overview`     | Whole-API insight report for the last-generated spec   |
| `/api/insight/endpoint`     | Per-route insight report (`?method=&path=`, optional `?trace=tracker\|callgraph`) |
| `/api/insight/source`       | Source window around a trace position (`?pos=file:line`), restricted to the analyzed module, GOROOT and the module cache |
| `/api/insight/export`       | The report as Markdown, or `?format=json` (`?scope=endpoint` for one route, `?redact=1` to redact paths) |
| `/api/diagram/*`            | Paginated diagram API (same surface as `apidiag`)      |

## `apidiag` — Interactive call-graph server (standalone)

The same diagram server, packaged as its own binary. Use it when you want a dedicated graph explorer without the config UI, or to run it on its own host/port. Internally both binaries share `internal/diagserver`.

```bash
go install github.com/ehabterra/apispec/cmd/apidiag@latest
apidiag --dir ./my-go-project --port 8080
# Open http://localhost:8080
```

Features include package/function/file filtering, multiple export formats (SVG, PNG, PDF, JSON), and a JSON HTTP API for programmatic access.

See [`cmd/apidiag/README.md`](../cmd/apidiag/README.md) for full documentation and a [demo video](https://youtu.be/UshBJ5-ayzA).

