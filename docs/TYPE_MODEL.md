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

**Phase 1 — foundation.** *(landed)*
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

**Phase 2 — migrate consumers, kill quirks deliberately.** *(landed)*
- The component-key canonicalizer is structured: `Canonicalize` (in
  `typemodel`) replaces the legacy `NormalizeInstance` string view; unlike it,
  instantiations wrapped in pointer/slice constructors
  (`[]pkg.Page[pkg.User]`) canonicalize correctly instead of being mangled,
  and maps pass through untouched.
- The context provider *builds* instantiation names as `TypeRef` values
  rendered via `Internal()` (`genericInstantiationName`/`genericArgRef`/
  `genericBaseRef`) instead of concatenating bracket strings — and now
  recognizes a composite literal whose type is a slice of an instantiation
  (`[]Envelope[User]{…}`), which previously collapsed onto the declaration
  placeholder (`Envelope_T-any`). Fixture: `testdata/generic_structs` route
  `/batch`.
- Pkg/simple-name-only consumers (`detectEnumFromConstants` and its callers,
  `traceGenericOrigin`) consume `Parse(...).Core()` fields directly.
- Deleted from `legacy.go` (no callers remain): `NormalizeInstance`,
  `ArgPackage`, `SimplifyArg`, `SimpleName`.
- Everything feeding `typeByName`/metadata type lookups stayed on
  `ParseParts` in this phase, pending the lookup-side migration below.

**Phase 3 — the lookup boundary and the AST boundary unify.** *(landed)*
- *Correction discovered here:* the metadata `Types` map was **already keyed
  by the bare declared name** (`tspec.Name.Name`; a generic declaration is
  stored as `"Page"`, its parameters in `Type.TypeParams`) — the bracketed
  `getTypeName(tspec)` form (`"Page[T]"`) is only the *methods-table* key
  convention, matching how a generic method-receiver expression renders. The
  phase-2 worry that the `ParseParts` opaque quirk was load-bearing for
  lookups was wrong: the quirk merely made unqualified generic names *miss*.
- All lookup sites migrated to the structured core: `typeByName` now takes
  `(pkg, name)` and every caller (`findTypesInMetadata`,
  `generateStructSchema`'s key parsing incl. the concrete-vs-declaration
  argument split via `Constraint`, `isInterfaceTypeName`,
  `lookupWrapperType`) parses with `typemodel.Parse(...).Core()`.
- `metadata.getTypeName`'s expression cases delegate to
  `FromExpr(...).String()` — one render path for every layer. Two shapes the
  old stringifier lost are now recorded correctly, proven by
  `TestMetadata_BoundaryShapeImprovements`:
  - fixed-size array fields keep their length (`[4]byte`, feeding the
    mapper's existing maxItems/maxLength handling) — previously `[]byte`;
  - methods on multi-parameter generic types attach (an `IndexListExpr`
    receiver like `Pair[K, V]` previously stringified to `""`, filing the
    method under the empty key and losing it).
  `TypeSpec`/`FieldList` (the methods-table naming convention) are the only
  shapes still rendered locally.
- Zero drift on all goldens and fixtures.

**Phase 4 — metadata carries structure; the legacy layer dies.** *(landed)*
- Metadata records carry their types structurally via a memoized accessor:
  `Metadata.TypeRefOf(poolID)` parses each pooled type string at most once
  (thread-safe, following the existing cache pattern), with
  `CallArgument.TypeRef()`/`ResolvedTypeRef()` riding on it. The string pool
  remains the serialization format — no metadata YAML change. Cached refs are
  shared and immutable; `TypeRef.Clone()` exists for mutating consumers.
- The mapper's core wrapper dispatch (`mapGoTypeToOpenAPISchema`) switches on
  the parsed `Kind` for pointer/array/slice and recurses on the exact element
  substring (`Raw()`) — byte-identical, and no longer confused by nested
  brackets in array lengths. The **map branch stays string-based on
  purpose**: its `Contains("map[")` trigger and preceding-qualifier glue
  encode naming behavior beyond parsing; it migrates when component naming
  moves onto `Internal()` rendering.
- The argument renderer's builtin detection runs on the argument's memoized
  structured type (`builtinPassThrough(arg.TypeRef())`); its custom-type
  qualification tail deliberately stays string surgery for the same
  component-naming reason.
- **The transitional layer is gone**: `legacy.go` deleted (`ParseParts` and
  spec's `TypeParts`/`Parts` had no production callers left; `SplitArgs`
  folded into the parser as `splitArgs`).

**Remaining (deliberate, small).** Two string-surgery islands are kept until
component naming itself is revisited (they *are* the naming behavior):
the mapper's map branch and the argument renderer's qualification tail. The
remaining scattered `*`/`[]` prefix-trims in `internal/spec` collapse onto
`Core()`/renderers as their surrounding code is touched.

**End state (reached for parsing).** Types are parsed in exactly one place;
strings persist only at the serialization boundary (string pool) and in the
two documented naming islands above.

## Ground rules

- A phase lands only with the full suite green; a golden diff must be
  explained by the quirk it deliberately removes.
- New detection/resolution code must not parse type strings — build or accept
  a `TypeRef`. If a helper you need is missing, add it to `typemodel` with
  unit tests.
- The legacy views must not gain new callers.
