# APISpec documentation

Start with the [main README](../README.md) for the overview and quick start.
This directory holds the long-form documentation.

## Using APISpec

| Doc | What's in it |
|-----|--------------|
| [INSTALLATION.md](INSTALLATION.md) | Every installation method, per platform |
| [TOOLS.md](TOOLS.md) | `apispec`, `apispecui`, `apidiag` — all flags and HTTP endpoints |
| [CONFIGURATION.md](CONFIGURATION.md) | Field-by-field configuration reference |
| [CAPABILITIES.md](CAPABILITIES.md) | Every code shape APISpec resolves, with worked examples |
| [LIMITATIONS.md](LIMITATIONS.md) | What it can't see, what it doesn't state, and the guardrails to add |
| [DEBUGGING.md](DEBUGGING.md) | **My route is missing** — the debugging path |
| [PERFORMANCE.md](PERFORMANCE.md) | Limits, profiling and measured trade-offs |

## How it works

| Doc | What's in it |
|-----|--------------|
| [PIPELINE.md](PIPELINE.md) | The analysis pipeline, stage by stage |
| [TYPE_MODEL.md](TYPE_MODEL.md) | The structured type model — how Go types become schemas |
| [AUTH_DETECTION_DESIGN.md](AUTH_DETECTION_DESIGN.md) | The security/auth detection model |
| [INTERFACE_RESOLUTION.md](INTERFACE_RESOLUTION.md) | Interface and return-value resolution |
| [TRACKER_REDESIGN.md](TRACKER_REDESIGN.md) | The tracker redesign: rationale and measurements |
| [TRACKER_TREE_USAGE.md](TRACKER_TREE_USAGE.md) | Using TrackerTree for call-graph analysis |
| [CYTOGRAPHE_README.md](CYTOGRAPHE_README.md) | The call-graph visualization |
| [ANALYSIS_THEORY_VS_PRACTICE.md](ANALYSIS_THEORY_VS_PRACTICE.md) | Why the analysis is shaped the way it is |

## Package documentation

- [`../internal/metadata/README.md`](../internal/metadata/README.md) — metadata package
- [`../internal/spec/README.md`](../internal/spec/README.md) — spec generation package
- [`../cmd/apispec/README.md`](../cmd/apispec/README.md) — CLI
- [`../cmd/apidiag/README.md`](../cmd/apidiag/README.md) — diagram server

## Project

- [Contributing](../CONTRIBUTING.md) — development setup, project layout, adding a framework
- [RELEASE_WORKFLOW.md](RELEASE_WORKFLOW.md) — the release process
- [Changelog](../CHANGELOG.md)
- [License](../LICENSE) — Apache 2.0
