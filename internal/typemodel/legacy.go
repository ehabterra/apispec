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

// NormalizeInstance rewrites a generic instantiation rendered in the go/types
// form (pkg.Type[pkg.Arg], produced by inferred instantiations whose body
// type comes from the call's return type) into the internal form
// pkg-->Type[Arg] with arguments reduced to their simple names — so an
// inferred Envelope[Product] keys to the same clean component as a written
// one instead of embedding the full package path of each argument. Names
// already in the internal form, and bare unqualified generics (Container[T]),
// are returned unchanged. Exact port of the spec layer's
// normalizeGenericInstanceName.
//
// Structured replacement: Parse + TypeRef.Internal() (which also handles
// wrapped instantiations like []pkg.Page[pkg.User] that this view mangles).
func NormalizeInstance(s string) string {
	open := strings.Index(s, "[")
	if open < 0 || !strings.HasSuffix(s, "]") {
		return s
	}
	base := s[:open]
	args := SplitArgs(s[open+1 : len(s)-1])

	// Request-body var types can render with an unqualified base but a
	// package-qualified argument (`Page[github.com/acme/svc.User]`). Qualify
	// the base from the first qualified argument's package so it keys to the
	// same component as the encode-site form (`…svc-->Page[User]`) — an
	// envelope and its payload almost always co-locate. If nothing qualifies
	// the base it's a bare local generic (`Container[T]`); leave it opaque.
	if !strings.Contains(base, Sep) && !strings.Contains(base, ".") {
		pkg := ""
		for _, a := range args {
			if p := ArgPackage(a); p != "" {
				pkg = p
				break
			}
		}
		if pkg == "" {
			return s
		}
		base = pkg + Sep + base
	}

	for i, a := range args {
		args[i] = SimplifyArg(a)
	}
	// Convert a dotted base (pkg.Type) to the internal pkg-->Type form; a base
	// already in Sep form is left as-is.
	if !strings.Contains(base, Sep) {
		if dot := strings.LastIndex(base, "."); dot >= 0 {
			base = base[:dot] + Sep + base[dot+1:]
		}
	}
	// Join with ", " (not a bare comma): the schema-name sanitizer maps ", "
	// to "-", so a multi-argument instantiation yields a valid component name.
	return base + "[" + strings.Join(args, ", ") + "]"
}

// ArgPackage returns the package qualifier of a rendered type argument
// (`github.com/acme/svc.User` -> `github.com/acme/svc`, `pkg-->User` ->
// `pkg`), or "" when the argument is unqualified. Only the segment before any
// nested bracket is considered, so `pkg.Page[pkg.User]` yields `pkg`. Exact
// port of the spec layer's genericArgPackage.
//
// Structured replacement: Parse(a).Core().Pkg.
func ArgPackage(arg string) string {
	arg = strings.TrimPrefix(arg, "[]")
	arg = strings.TrimPrefix(arg, "*")
	if i := strings.LastIndex(arg, Sep); i >= 0 {
		return arg[:i]
	}
	head := arg
	if b := strings.Index(arg, "["); b >= 0 {
		head = arg[:b]
	}
	if dot := strings.LastIndex(head, "."); dot >= 0 {
		return head[:dot]
	}
	return ""
}

// SimplifyArg reduces a type argument to its simple base name, recursing into
// nested instantiations (pkg.Page[pkg.User] -> Page[User]) so the enclosing
// type's package can be re-glued during field resolution. Exact port of the
// spec layer's simplifyGenericArg.
//
// Structured replacement: Parse(a).Simple() (which preserves the "*"/"[]"
// wrapper markers this view drops via SimpleName).
func SimplifyArg(a string) string {
	if open := strings.Index(a, "["); open >= 0 && strings.HasSuffix(a, "]") {
		inner := SplitArgs(a[open+1 : len(a)-1])
		for i, x := range inner {
			inner[i] = SimplifyArg(x)
		}
		return SimpleName(a[:open]) + "[" + strings.Join(inner, ", ") + "]"
	}
	return SimpleName(a)
}

// SimpleName reduces a rendered type name to its simple type name, stripping
// package qualifiers (which use Sep, "/", or ".") and one leading
// pointer/slice marker. The empty interface normalizes to "any" ("{"/"}" are
// illegal in an OpenAPI component name). Exact port of the spec layer's
// simpleGenericArgName. Quirks preserved: wrapper markers are dropped, and a
// qualifier anywhere in the string is stripped up to its last separator (so
// map[string]pkg.User collapses to "User").
//
// Structured replacement: Parse(s).Simple().
func SimpleName(s string) string {
	s = strings.TrimPrefix(s, "[]")
	s = strings.TrimPrefix(s, "*")
	for _, sepr := range []string{Sep, "/", "."} {
		if i := strings.LastIndex(s, sepr); i >= 0 {
			s = s[i+len(sepr):]
		}
	}
	if s == "interface{}" || s == "interface {}" {
		return "any"
	}
	return s
}
