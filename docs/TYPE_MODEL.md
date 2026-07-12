# The structured type model (`internal/typemodel`)

## Why

Until now a Go type traveled through apispec as a **string** with hand-rolled
encoding conventions layered on top of each other:

- the internal package/type separator `pkg-->Type` (`TypeSep`),
- the go/types dotted form `pkg.Type` (with `/` inside import paths),
- wrapper markers as prefixes: `*T`, `[]T`, `[N]T`, `map[K]V`, `chan T`,
- bracketed generic argument lists (`Page[User]`, nested and multi-argument),
  plus a legacy `pkg-->Type-->Arg` argument encoding.

Every layer that needed the package, the simple name, or a type argument
re-parsed that string with its own prefix-trimming and separator-splitting
code (`TypeParts`, `normalizeGenericInstanceName`, `simplifyGenericArg`,
`simpleGenericArgName`, `genericArgPackage`, ~70 further ad-hoc
`strings.TrimPrefix`/`HasPrefix` sites in `internal/spec` alone). That
encoding is the root cause of a recurring class of bugs — generic base/arg
qualification, request-vs-response naming, nested/inferred instantiation
forms — because every re-parse is an opportunity to disagree with the writer.

`internal/typemodel` replaces this with a first-class descriptor, **TypeRef**:

```go
type TypeRef struct {
    Kind Kind      // Named | Pointer | Slice | Array | Map | Chan
    Pkg  string    // import path ("" = builtin/local/type-parameter)
    Name string    // simple type name (opaque input preserved verbatim)
    Args []*TypeRef // generic arguments (instantiation or declaration form)
    Constraint string // "any" in "T any" (declaration-form parameters)
    Key, Elem *TypeRef // map key; pointer/slice/array/chan/map element
    Len  string    // array length text
    Dir  ChanDir   // channel direction
}
```

Structure is carried in **fields, not string fragments**, and is rendered to a
string only at an output boundary:

- `Parse(s)` — one parser for every encoding above; never fails (unmodelable
  input stays opaque in `Name` and renders back verbatim).
- `FromExpr(expr, info)` — the constructor at the AST/go/types boundary, the
  structured counterpart of `metadata.getTypeName`. Already fixes two gaps the
  flat string could not represent: multi-argument generic instantiations
  (`IndexListExpr`) and array lengths.
- `String()` — go/types dotted form (round-trips dotted input).
- `Internal()` — canonical component-key form `pkg-->Type[SimpleArgs]`
  (round-trips internal-form input).
- `Simple()` — unqualified form; normalizes `interface{}` → `any`.

## Migration strategy (strangler, zero-drift steps)

The type model is the single largest architectural change in the codebase
(GAP #8), so it lands in phases, each independently shippable and each
verified against the golden fixtures:

**Phase 1 — foundation (this change).**
- `internal/typemodel` exists with the structured core, full unit coverage,
  and round-trip guarantees.
- A boundary differential test
  (`internal/metadata/typeref_boundary_test.go`) proves
  `FromExpr(...).String()` is byte-identical to `getTypeName` on every shape
  the legacy stringifier understands, so call sites can migrate mechanically.
- Every legacy string helper from `internal/spec` moved into
  `typemodel/legacy.go` as an **exact port** (`ParseParts`,
  `NormalizeInstance`, `SimplifyArg`, `SimpleName`, `ArgPackage`,
  `SplitArgs`); the spec layer delegates to them. There is now exactly one
  home for string-encoding knowledge. Quirks are preserved *and documented*
  on each function (e.g. `ParseParts` leaks `map[...]` into `PkgName`;
  `SimplifyArg` drops `*`/`[]` markers).
- Zero output drift: all golden fixtures, determinism tests, and framework
  tests unchanged.

**Phase 2 — migrate consumers, kill quirks deliberately.**
One consumer (or one quirk) per PR, so any golden diff is reviewable and
intentional:
- `internal/spec` call sites of `TypeParts(...)` consume `Parse(...)` fields
  directly; the ~70 ad-hoc prefix-trim sites collapse onto `Core()`/renderers.
- `genericInstantiationName`/`renderGenericArg` in the context provider build
  a `TypeRef` and render `Internal()` instead of concatenating brackets.
- Known legacy mangles become fixes with fixture coverage: wrapped generic
  instantiations (`[]pkg.Page[pkg.User]`), map-typed generic arguments,
  wrapper markers dropped from argument names.

**Phase 3 — metadata carries structure.**
- `metadata.getTypeName` call sites move to `FromExpr(...).String()` (proven
  byte-identical by the boundary test), then the interesting ones keep the
  `TypeRef` instead of the string — `CallArgument`, `Field`, assignment and
  return records — with the string pool retained as the serialization format.

**End state.** Strings exist only at two boundaries: metadata serialization
(string pool) and OpenAPI component naming (sanitizer over `Internal()`).
`typemodel/legacy.go` shrinks to nothing and is deleted.

## Ground rules

- A phase lands only with the full suite green; a golden diff must be
  explained by the quirk it deliberately removes.
- New detection/resolution code must not parse type strings — build or accept
  a `TypeRef`. If a helper you need is missing, add it to `typemodel` with
  unit tests.
- The legacy views must not gain new callers.
