# Limits and fit

Where APISpec is the right tool, where it needs a second pair of eyes, and
where it is the wrong tool. The capability side of the same story is
[CAPABILITIES.md](CAPABILITIES.md).

APISpec reads your code; it never runs it. Everything below follows from that
one fact: a route your compiler can see, APISpec can usually see too — a route
that only exists once a value is computed at runtime, it cannot.

## Is APISpec the right tool for the job?

"Production-ready" isn't one question here, because a generated spec gets used
for several different jobs and APISpec is not equally good at all of them.

| Job | Fit | Why |
|-----|-----|-----|
| **An internal type source** — generate clients/types for your own frontend, mobile app or services | ✅ **Yes** | Route, method, request-type and response-type inference is driven by the compiler's own type information. Where it is imperfect it is usually *loose* (a wider type, a superset of statuses) rather than *wrong*. |
| **A drift check in CI** — fail the build when the spec and the code disagree | ✅ **Yes** | Output is deterministic: an unchanged project regenerates byte-identically, so a diff is signal rather than noise. |
| **Documentation for humans** — an always-current reference for an existing service | ✅ **Yes**, with `naming:` set | The default operationIds and schema names are fully-qualified Go symbols; set [`naming`](CONFIGURATION.md#naming) for readable ones. |
| **A published contract** — partner integrations, third-party SDKs, contract testing | ⚠️ **Not without review** | The error contract, field optionality and some media types are under-stated (see [What APISpec does not state](#what-apispec-does-not-state)). Treat the output as a draft to curate, not as the contract. |
| **A spec-first workflow** — the spec is the source of truth and the code follows | ❌ **Wrong tool** | APISpec derives the spec *from* the code. If you want the spec to lead, use a spec-first generator (oapi-codegen, ogen) — APISpec can still document the server it produces. |
| **A non-Go service, or Go without static routing** | ❌ **Wrong tool** | Routes assembled from runtime tables or config files are reported as unresolved rather than guessed at. |

**Run it unattended?** Only behind a check. APISpec logs what it could not
resolve, but it still exits `0` — there is no `--strict` mode that turns a
warning into a non-zero exit. If the spec matters, read stderr, count your
routes, and diff the spec in CI. See [Guardrails worth having](#guardrails-worth-having).

## What APISpec does not state

These are gaps in the *contract*, not in the type inference — the schemas are
right as far as they go, they just say less than your code knows.

- **Field optionality is off by default.** A struct field is marked `required`
  when a `go-playground/validator` tag says so. `encoding/json` knows more than
  that — a field with no `omitempty` is always written, and a nil pointer, slice
  or map is written as `null` — and two opt-in settings say so:
  [`schema.requiredFromJSONTags`](CONFIGURATION.md#schema) and
  [`schema.nullableWhenNil`](CONFIGURATION.md#schema). Both are off unless you
  ask, because they describe what the SERVER SENDS: exactly right for a
  response, and an over-claim for a request body, since a client is not bound by
  your struct tags. Leave them off and a strict client generator will make
  almost everything optional.
- **The error-status list is a superset, and this is the ceiling.** Every status
  written anywhere in a handler's reachable call graph becomes a response, so an
  operation lists the statuses that EXIST behind it, not the ones a client must
  handle for that call. A shared error mapper is the usual cause:

  ```go
  func writeError(w http.ResponseWriter, err error) {
      switch {
      case errors.Is(err, ErrNotFound): respond(w, 404, …)
      case errors.Is(err, ErrInUse):    respond(w, 409, …)   // a list call can never reach this
      case errors.Is(err, ErrInvalid):  respond(w, 422, …)
      default:                          respond(w, 500, …)
      }
  }
  ```

  Every handler calling `writeError` reaches every arm, so every arm is
  documented. Measured on one service, **225 of 277 GET operations carried a
  `409 Conflict`** they cannot return, and the set `400 404 409 422 500`
  appeared on 318 operations.

  This is **sound but not precise**: each status documented really can be
  written by reachable code, and nothing tracks which sentinel errors a given
  service call can actually return. Narrowing it needs error-value flow analysis
  — which `error` values reach which `errors.Is` arm, interprocedurally, through
  wrapping — a different class of analysis from call-graph inference, so treat
  it as the expected ceiling rather than a defect awaiting a fix. The practical
  consequence: a generated client's response union is wider than reality, never
  narrower. Curate it with [`overrides`](CONFIGURATION.md#overrides) if you
  publish the document as a contract.
- **Responses written by middleware are not attributed to the route.** An auth
  middleware's `401`, a rate limiter's `429` or a recovery handler's `500` are
  written outside the handler, so they do not appear on the operations they
  protect. Add them with [`overrides`](CONFIGURATION.md#overrides) or a
  document-level convention.
- **A media type must be a constant.** The renderer or encoder decides it
  where there is one — `c.XML(...)`, an `xml`/`yaml` `Encoder`, `http.Error`,
  the framework's `String`/`HTML`/`ProtoBuf` helpers — and a handler that
  STREAMS instead declares it:
  `w.Header().Set("Content-Type", "application/pdf")` is read, including when
  the value is a constant, and documents the download. A value assembled at
  runtime (`mime.TypeByExtension(ext)`) is not, and the operation keeps the
  default rather than guessing; set it with
  [`overrides`](CONFIGURATION.md#overrides) or a per-pattern
  `defaultContentType`. Which calls count as a declaration is configurable per
  framework (`responseContext.contentTypeWrites`), so a house context can be
  added. A declaration is only read when it can be shown to be about the
  response rather than an outbound request, which takes
  `responseContext.writerTypeRegexes`; every supported framework declares
  those, but a **custom config that replaces the response context without them
  documents no streamed body at all**.

## What APISpec cannot see

- **Two bodies under one status that never state that status.** Alternative
  bodies on branches that *do* write their status compose into an `anyOf`; two
  that merely fall back to the framework's implicit status keep the first one
  and drop the rest. Within an `r.Method` split, an `anyOf` that two arms share
  is documented on **both** operations rather than divided between them.
- Only `go-playground/validator`-style `validate:` tags are read; Gin/Echo
  `binding:` tags and comparison validators (`gt`/`gte`/`lt`/`lte`) are not yet
  mapped.
- **A path that exists only as a runtime value.** Routes registered from a
  **table** (`for _, r := range routes { adapter.Add(r.Method, r.Path, r.Handler) }`)
  or by a house router chained through a returned object
  (`Combo("/x").Get(h).Post(h)`) cannot be located, so they are **reported and
  left out** rather than documented at the placeholder standing in for the
  expression. Each one names the registration site on stderr, and
  `Generator.UnresolvedPaths()` returns the list, so the gap is countable
  instead of silent. A path that is only *partly* unresolved keeps its
  operation, with the placeholder flagged: an unresolved prefix
  (`/{mountPoint}/clear`), an unresolved segment before a literal tail
  (`/{dynamicBase}/dyn`), and an unreadable tail under a prefix that IS known
  (`/repo/{owner}/{name}/info/lfs/{path}` — a catch-all seen through a wrapper)
  are all real endpoints, documented approximately.
- **A payload erased inside a generic envelope** — a helper that re-wraps the
  value as `APIResponse[any]{Data: data}` documents `data` as an open object
  ([#163](https://github.com/ehabterra/apispec/issues/163)).
  A plain `any`/`interface{}` **parameter** is not affected: that resolves, and
  keeps resolving when one helper serves several routes with different types.
  A cross-package type used as a **type argument** also loses its package in the
  component name (`app_Page_Product`, where the same type returned directly is
  `app_inner_Product`).
- Command libraries that dispatch through a **factory map** (`mitchellh/cli`,
  `hashicorp/cli`) or **reflection-invoked methods** (`alecthomas/kong`) — the
  command body is never reached from your code, so nothing it registers is
  documented.

- **Conditional registration** — routes added behind a runtime condition
  (a feature flag read from config, a plugin registry populated at start-up) are
  generally not resolved.
- Command libraries that dispatch through a **factory map** (`mitchellh/cli`,
  `hashicorp/cli`) or **reflection-invoked methods** (`alecthomas/kong`) — the
  command body is never reached from your code, so nothing it registers is
  documented.

Everything in this list is *reported*, not silently dropped: unresolved paths
name their registration site on stderr and come back from
`Generator.UnresolvedPaths()`.

## Guardrails worth having

Because there is no strict mode yet, wire the checks yourself. A useful set:

1. **Count the routes.** Assert the number of paths in the generated spec against
   a floor you ratchet upward, so a truncated or degraded run fails loudly.
2. **Fail on unresolved paths.** Grep stderr, or call
   `Generator.UnresolvedPaths()` from a small Go check and exit non-zero when it
   is non-empty.
3. **Diff the spec in CI.** Regenerate and `git diff --exit-code` the committed
   spec: output is deterministic, so any diff is a real change.
4. **Read the Insight report** (`apispecui`, the ◷ tab) after a version bump. It
   splits responses per status into *described*, *free-form object*, *unresolved
   type* and *no body*, which is the fastest way to see whether a release moved
   anything.
5. **Pin the version.** Defaults (naming, patterns, limits) change between
   releases; pin APISpec in CI and review the spec diff when you upgrade.

## Something missing that should not be?

Open an issue with a minimal reproduction — a `main.go` showing the wiring is
usually enough, and it becomes a test fixture:
<https://github.com/ehabterra/apispec/issues>.
