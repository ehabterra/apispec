package spec

import (
	"strings"

	"github.com/ehabterra/apispec/internal/metadata"
)

// resolveSelectorFieldType returns the Go type string of a selector expression
// like `obj.Field` or `a.b.Field`. The pattern matchers historically converted
// such expressions into a dotted chain string (e.g. "APIError.Message"), then
// fed that into the schema mapper — producing broken $ref pointers to types
// that don't exist. This resolver looks up the field's declared type via
// metadata instead.
//
// The returned string is the field's underlying Go type (e.g. "string",
// "*github.com/foo/bar.SomeStruct"). Empty string means "could not resolve",
// in which case callers should fall back to their previous behaviour.
func resolveSelectorFieldType(arg *metadata.CallArgument, cp ContextProvider) string {
	if arg == nil || arg.GetKind() != metadata.KindSelector {
		return ""
	}

	// Fast path: metadata sometimes records the resolved type of the selector
	// expression itself.
	if t := arg.GetResolvedType(); t != "" {
		return t
	}
	if t := arg.GetType(); t != "" {
		return t
	}

	impl, ok := cp.(*ContextProviderImpl)
	if !ok || impl.meta == nil {
		return ""
	}
	meta := impl.meta

	// Determine the type of the base expression (the LHS of the selector).
	baseType := selectorBaseType(arg.X, cp)
	if baseType == "" {
		return ""
	}
	pkg, typeName := splitPkgType(stripPointer(baseType))
	if typeName == "" {
		return ""
	}

	fieldName := ""
	if arg.Sel != nil {
		fieldName = arg.Sel.GetName()
	}
	if fieldName == "" {
		return ""
	}

	if t := lookupFieldType(meta, pkg, typeName, fieldName); t != "" {
		return t
	}
	return ""
}

// selectorBaseType returns the declared type of the LHS of a selector. The
// LHS can be an ident (e.g. `api`), a deeper selector (e.g. `req.User`), or
// a method call (e.g. `req.User()`). Returns empty when undetermined.
func selectorBaseType(x *metadata.CallArgument, cp ContextProvider) string {
	if x == nil {
		return ""
	}
	switch x.GetKind() {
	case metadata.KindIdent:
		if t := x.GetResolvedType(); t != "" {
			return t
		}
		return x.GetType()
	case metadata.KindSelector:
		// a.b.c — recurse to resolve the inner selector's type.
		return resolveSelectorFieldType(x, cp)
	case metadata.KindCall:
		// foo.Bar() — its return type would ideally come from
		// ResolvedType. We don't try to dive into function bodies here.
		if t := x.GetResolvedType(); t != "" {
			return t
		}
		return x.GetType()
	case metadata.KindUnary, metadata.KindStar:
		if x.X != nil {
			return selectorBaseType(x.X, cp)
		}
	}
	return ""
}

// stripPointer trims a leading "*" from a Go type string.
func stripPointer(t string) string {
	return strings.TrimPrefix(t, "*")
}

// splitPkgType splits a fully-qualified type "github.com/foo/bar.Type" into
// ("github.com/foo/bar", "Type"). Package paths contain "/" but the last "."
// always separates pkg path from type name (because Go identifiers can't
// contain ".").
func splitPkgType(t string) (pkg, typeName string) {
	i := strings.LastIndex(t, ".")
	if i < 0 {
		return "", t
	}
	return t[:i], t[i+1:]
}

// lookupFieldType returns the declared type of the named field on the named
// type in the named package, or empty when the field can't be found.
//
// Looks in both package-level Types and per-file Types because the metadata
// has historically populated both views. Falls back to embedded types when
// the field isn't directly declared — but only one level deep, to avoid
// pathological recursion.
func lookupFieldType(meta *metadata.Metadata, pkg, typeName, fieldName string) string {
	if meta == nil {
		return ""
	}
	t := findType(meta, pkg, typeName)
	if t == nil {
		return ""
	}
	for _, f := range t.Fields {
		if meta.StringPool.GetString(f.Name) == fieldName {
			return meta.StringPool.GetString(f.Type)
		}
	}
	// Try embedded fields — one level deep.
	for _, embedIdx := range t.Embeds {
		embedName := meta.StringPool.GetString(embedIdx)
		ePkg, eType := splitPkgType(stripPointer(embedName))
		if eType == "" {
			continue
		}
		if r := lookupFieldType(meta, ePkg, eType, fieldName); r != "" {
			return r
		}
	}
	return ""
}

// constIdentDeclaredType returns the declared Go type of an ident that
// refers to a `const` declaration. When the ident is not a const (or its
// package/variable cannot be located), it returns the empty string and
// callers should fall back to their previous behaviour.
//
// This exists because the context-provider's renderer for const idents
// returns the constant's *value* (e.g. the body of an embedded HTML
// string), which leaks into schemas as a $ref to a nonexistent type.
func constIdentDeclaredType(arg *metadata.CallArgument, cp ContextProvider) string {
	if arg == nil || arg.GetKind() != metadata.KindIdent {
		return ""
	}
	impl, ok := cp.(*ContextProviderImpl)
	if !ok || impl.meta == nil {
		return ""
	}
	pkg, ok2 := impl.meta.Packages[arg.GetPkg()]
	if !ok2 {
		return ""
	}
	name := arg.GetName()
	for _, file := range pkg.Files {
		v, ok := file.Variables[name]
		if !ok {
			continue
		}
		if impl.GetString(v.Tok) != "const" {
			return ""
		}
		if t := impl.GetString(v.Type); t != "" {
			return t
		}
		if t := impl.GetString(v.ResolvedType); t != "" {
			return t
		}
		return ""
	}
	return ""
}

// findType resolves (pkg, typeName) to a Type entry. Checks package-level
// Types first, then per-file Types as a fallback.
func findType(meta *metadata.Metadata, pkg, typeName string) *metadata.Type {
	if pkg == "" || typeName == "" {
		return nil
	}
	p, ok := meta.Packages[pkg]
	if !ok {
		return nil
	}
	if t, ok := p.Types[typeName]; ok && t != nil {
		return t
	}
	for _, file := range p.Files {
		if t, ok := file.Types[typeName]; ok && t != nil {
			return t
		}
	}
	return nil
}
