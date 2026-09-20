# Contributing to APISpec

**APISpec** analyzes your Go code and automatically generates OpenAPI 3.1 specifications (YAML or JSON). It detects routes for popular frameworks (Gin, Echo, Chi, Fiber, Gorilla Mux, `net/http`), follows call graphs to final handlers, and infers request/response types from real code.

Thank you for your interest in contributing to APISpec! Your contributions, feedback, and help are greatly appreciated.

## Getting Started

### Prerequisites

- Go 1.26+
- Git
- Basic understanding of Go AST and OpenAPI 3.1 specification (helpful but not required)

### Development Setup

1. **Fork and clone the repository**

   ```bash
   git clone https://github.com/your-username/apispec.git
   cd apispec
   ```

2. **Install dependencies**

   ```bash
   make deps
   ```

3. **Build the project**

   ```bash
   make build
   ```

4. **Run tests**

   ```bash
   make test
   ```

### Project layout

```text
apispec/
├── cmd/
│   ├── apispec/       # CLI generator
│   ├── apispecui/     # Browser UI + spec preview
│   └── apidiag/       # Paginated call-graph server
├── generator/         # High-level generator interface
├── internal/
│   ├── core/          # Framework detection & shared logic
│   ├── diagserver/    # Shared call-graph HTTP server (used by apidiag + apispecui)
│   ├── engine/        # Processing engine
│   ├── metadata/      # AST analysis & metadata extraction
│   └── spec/          # OpenAPI generation & mapping
├── pkg/patterns/      # Public pattern helpers
├── spec/              # Public spec package (configs, types)
├── testdata/          # Example projects used in tests
├── scripts/           # Build & utility scripts
└── docs/              # Long-form documentation
```

The binaries (`apispec`, `apispecui`, `apidiag`) are gitignored build artifacts.
After switching branches, rebuild *and restart* a running `apispecui` — it keeps
the code it started with, and a browser refresh will not pick up a new build.

## Before Making Changes

Before writing a feature or fixing an issue, please:

1. **Check existing issues** on GitHub to see if your idea or bug is already being discussed
2. **Add comments** to relevant issues if you want to contribute or have questions
3. **Create a new issue** to discuss your proposal if it doesn't exist yet - I'd love to hear your ideas!
4. **Wait for feedback** before starting implementation (if possible) to avoid duplicate work

This helps us work together effectively and ensures contributions align with the project's direction.

## Making Changes

### Workflow

1. **Create a feature branch**
   ```bash
   git checkout -b feature/your-feature-name
   # or
   git checkout -b fix/issue-description
   ```

2. **Make your changes**
   - Write clean, well-documented code
   - Follow Go coding standards
   - Add tests when possible

3. **Run tests and linting**
   ```bash
   make test          # Run all tests
   make coverage      # Check test coverage
   make lint          # Run linting checks
   make fmt           # Format code
   ```

4. **Commit your changes**
   ```bash
   git commit -m "Add: brief description of your changes"
   ```

5. **Push and create a Pull Request**
   ```bash
   git push origin feature/your-feature-name
   ```

## Code Standards

- **Write tests when possible**: Tests are helpful, but don't worry if you're not sure how to test something - we can figure it out together
- **Follow Go conventions**: Use `gofmt`, `go vet`, and follow standard Go style
- **Document public APIs**: Add comments for exported functions and types
- **Keep it simple**: Write clear, readable code
- **Don't worry about perfection**: If something isn't quite right, we can iterate on it together. Please add a TODO comment for incomplete parts so we can address them later.

## Testing

- Run tests: `make test` — this is what CI runs and the source of truth
- Check coverage: `make coverage`
- Run specific tests: `go test ./internal/spec -v -run "Test.*Comprehensive"`
- Add test cases in `testdata/` for framework-specific features
- Refresh the coverage badge if coverage moves: `make update-badge`

### Required: compare the fixtures before you submit

`make test` proves the pipeline still *works*. It does not prove the output did
not *change*: the fixture tests in `generator/` are deliberately structural —
expected routes present, no dangling `$ref`s, no unresolved placeholders — so
that schema evolution doesn't churn them. Everything between "it still runs" and
"it produces the same document" is caught by `scripts/compare-spec.sh`, which
regenerates a spec for every project under `testdata/` and diffs it key by key.

**Run it for any change that can move the output** — anything in `internal/`,
`generator/`, `spec/`, or a framework config. Doc-only and tooling-only changes
can skip it.

There is no committed baseline to compare against: fixture `openapi*.yaml` and
`used-config.yaml` files are gitignored build artifacts. So generate the
baseline from the code *without* your change, then compare:

```bash
# 1. baseline, from a tree without your change (pick an unused version number)
git stash                       # or: git checkout main
scripts/compare-spec.sh -g -v 900

# 2. bring your change back and compare against it
git stash pop                   # or: git checkout your-branch
scripts/compare-spec.sh -v 900
```

Each pass builds `apispec` once; the two together take a few minutes over the
full fixture set. A clean run ends with:

```text
RESULT: no drift across all paths (status sets, keys, and values match).
```

The script exits non-zero and prints the offending keys otherwise. It reports
three kinds of drift, all of which fail:

- **STATUS CHANGES** — an operation gained or lost a response status. This is
  the one that catches a route quietly degrading to `default`.
- **MISSING** — a key in the baseline is absent from the new spec.
- **CHANGED** — a key exists in both with a different value (a `$ref`
  retargeted, a type flipped, a `format` dropped).

Added keys are informational and shown only with `-a`, so a change that only
*adds* passes by default.

Two things worth knowing when you read the output:

- **The MISSING list includes keys that merely MOVED.** A schema that gains a
  wrapper (`$ref` → `allOf: [$ref, …]`) reports every affected `$ref` path as
  missing, and a renamed path reports its whole subtree. Before concluding
  something was lost, count the thing itself — references to the component,
  operations, responses — rather than reading the key diff.
- **A refactor ships with zero drift.** If your change is meant to be
  behaviour-preserving and the comparison is not clean, that is a finding, not
  noise. A deliberate behaviour change is fine — say so in the PR, explain each
  drifted key, and add a fixture that covers the new behaviour.

By default the project set is every project under `testdata/`, plus any paths
listed in `scripts/compare-spec.paths` (gitignored, for running the comparison
against real services you have locally). Paths that don't exist are skipped, so
without that file you get the fixture set.

Useful flags: `-a` to also list added keys, `-k` to keep the generated spec for
inspection, `--bin PATH` to reuse a built binary, and a path argument to limit
the run to one project (`scripts/compare-spec.sh -v 900 testdata/gin`). If you
change the comparator itself, `scripts/compare-spec.sh --self-test` checks its
key semantics.

Finally, make sure the run left nothing behind: `git status` should be clean.
Generated specs and configs under `testdata/` are gitignored, but a binary built
inside a fixture is not — `TestNoCompiledBinariesUnderTestdata` reads git state,
so it passes locally against an unstaged binary and fails only in CI.

## Adding Framework Support

To add support for a new web framework:

1. **Add a registry entry** in `internal/core/frameworks.go` — the single source for detection patterns, dependency analysis and the UI picker
2. **Add the default configuration** in `internal/spec/config_<framework>.go` (each framework lives in its own file alongside `config.go`) and map it in `internal/spec/framework_config.go`
3. **Add a fixture project** under `testdata/<framework>/` and a corresponding test case
4. **Update documentation**: the support matrix in `README.md` and, if the framework needs config, [`docs/CONFIGURATION.md`](docs/CONFIGURATION.md)

Nothing else needs editing: the detector, the dependency analyser and the UI all
project the registry, and `TestFrameworkConfigsCoverRegistry` fails if step 2's
mapping is missed.

Step 2 is the one part that can be deferred. A registry entry with
`HasDefaultConfig: false` (as `fasthttp` has today) is classified during
dependency analysis — so its packages are included in the walk and its imports
are treated as external — but extracts no routes, and the project is analysed
with the `net/http` surface instead. That is a useful half-step, not a
destination: leave a comment on the entry saying so.

If you're unsure about any step, feel free to ask questions or create a draft PR - I'm happy to help!

## Submitting Changes

1. Ensure all tests pass (`make test`)
2. **Compare the fixtures** (`scripts/compare-spec.sh`) and confirm the output
   drift is zero, or explain every drifted key in the PR — see
   [Required: compare the fixtures before you submit](#required-compare-the-fixtures-before-you-submit)
3. Run linting (`make lint`) - if it fails, don't worry, we can fix it together
4. Check `git status` is clean — no generated specs, configs or binaries left behind
5. Update documentation if needed
6. Create a Pull Request with a clear description
7. Reference any related issues

**Note**: PRs don't need to be perfect. If you're stuck or unsure about something, feel free to open a draft PR and ask for help. Collaboration and feedback help us all improve!

## Questions?

If you have questions, need help, or want to discuss something:

- Open an issue on GitHub
- Comment on existing issues
- Don't hesitate to ask - your questions and contributions help make this project better!

## License

By contributing to APISpec, you agree that your contributions will be licensed under the Apache License 2.0, as stated in the [LICENSE](LICENSE) file.

## Code of Conduct

APISpec is committed to fostering a welcoming and inclusive community. Please read and follow our [Code of Conduct](CODE_OF_CONDUCT.md). All contributors are expected to uphold a respectful and collaborative environment.

## A Note from the Maintainer

I'm relatively new to open-source contribution, and I value your help and collaboration. If you notice something that could be improved, have suggestions, or want to help in any way, please reach out. Your contributions, feedback, and expertise are what make this project better for everyone.

Thank you for contributing! 🎉
