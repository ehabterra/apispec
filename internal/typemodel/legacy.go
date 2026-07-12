package typemodel

import "strings"

// This file holds the transitional string views: exact behavioral ports of
// the string-type helpers that used to live in internal/spec, consolidated
// here so every parser of the string encoding has one home. They are kept
// byte-compatible — quirks included and documented — so the spec layer could
// delegate to them with zero output drift. Each names its structured
// replacement; as consumers migrate to TypeRef these views shrink and
// eventually disappear (see docs/TYPE_MODEL.md).

// Parts is the flat package/type/arguments view of a string-encoded type
// name, as consumed by the spec layer's resolution and schema mapping.
//
// Transitional: new code should use Parse and consume TypeRef fields.
type Parts struct {
	PkgName      string
	TypeName     string
	GenericTypes []string
}

// ParseParts splits a string-encoded type name into Parts. Exact port of the
// spec layer's TypeParts. Quirks preserved from the original:
//
//   - A bracketed generic whose base is unqualified (Container[T]) stays
//     opaque: the brackets remain in TypeName and no arguments are split.
//   - Exactly one leading "*" or "[]" marker is stripped from PkgName; other
//     wrapper syntax (map[…], chan) is not understood and leaks into PkgName.
//   - In the legacy pkg-->Type-->Arg form, trailing segments become
//     arguments only when no bracket supplied them.
//
// Structured replacement: Parse + TypeRef{Pkg, Name, Args}.
func ParseParts(typeName string) Parts {
	parts := Parts{}

	// Peel off a generic argument list first so every qualified form is
	// handled uniformly: the internal form (pkg-->Type[Arg]) from
	// composite-literal rendering AND the go/types form (pkg.Type[pkg.Arg])
	// that inferred instantiations and nested field types produce — including
	// nested (Page[Box[User]]) and multi-argument (Pair[K,V]) brackets. A bare
	// unqualified generic (Container[T], no package) stays opaque, matching
	// the prior behavior for local/unresolvable type names.
	base := typeName
	if open := strings.Index(typeName, "["); open >= 0 && strings.HasSuffix(typeName, "]") {
		b := typeName[:open]
		if strings.Contains(b, Sep) || strings.Contains(b, ".") {
			base = b
			parts.GenericTypes = SplitArgs(typeName[open+1 : len(typeName)-1])
		}
	}

	baseParts := strings.Split(base, Sep)
	if len(baseParts) >= 2 {
		parts.PkgName = baseParts[0]
		parts.TypeName = baseParts[1]
		// pkg-->Type-->Arg form (no brackets): trailing segments are the args.
		if len(parts.GenericTypes) == 0 && len(baseParts) > 2 {
			parts.GenericTypes = baseParts[2:]
		}
	} else if lastSep := strings.LastIndex(base, "."); lastSep > 0 {
		parts.PkgName = base[:lastSep]
		parts.TypeName = base[lastSep+1:]
	} else {
		parts.TypeName = base
	}

	parts.PkgName = strings.TrimPrefix(parts.PkgName, "*")
	parts.PkgName = strings.TrimPrefix(parts.PkgName, "[]")

	return parts
}

// SplitArgs splits the contents of a generic bracket (`K,V` /
// `T any, U comparable`) on top-level commas, keeping commas inside nested
// brackets intact, and trims surrounding whitespace from each argument. An
// empty input yields no arguments. Exact port of the spec layer's
// splitGenericArgs; also used by the structured parser.
func SplitArgs(s string) []string {
	var (
		result []string
		cur    strings.Builder
		depth  int
	)
	for _, ch := range s {
		switch ch {
		case '[':
			depth++
			cur.WriteRune(ch)
		case ']':
			depth--
			cur.WriteRune(ch)
		case ',':
			if depth == 0 {
				result = append(result, strings.TrimSpace(cur.String()))
				cur.Reset()
				continue
			}
			cur.WriteRune(ch)
		default:
			cur.WriteRune(ch)
		}
	}
	if strings.TrimSpace(cur.String()) != "" {
		result = append(result, strings.TrimSpace(cur.String()))
	}
	return result
}

// NormalizeInstance, ArgPackage, SimplifyArg, and SimpleName were removed in
// phase 2: their consumers now go through the structured layer
// (Canonicalize / Parse + renderers). See docs/TYPE_MODEL.md.
