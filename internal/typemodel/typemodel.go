// Package typemodel is the structured type model for apispec: a first-class,
// parseable descriptor (TypeRef) for Go types, replacing the string-encoded
// conventions — `pkg-->Type` separators, dotted qualifiers, `*`/`[]`/`map[…]`
// prefixes, and bracketed generic argument lists — that the metadata and spec
// layers previously parsed with hand-rolled, duplicated string code.
//
// The package has two layers:
//
//   - The structured core (TypeRef, Parse, FromExpr, and the renderers) — the
//     target model. New code should build and consume TypeRef values and only
//     render to a string at an output boundary (component naming,
//     serialization).
//   - Transitional string views (ParseParts, NormalizeInstance, SimplifyArg,
//     SimpleName, ArgPackage, SplitArgs in legacy.go) — exact behavioral ports
//     of the helpers that used to live in internal/spec, kept byte-compatible
//     so consumers can migrate to the structured core one at a time with zero
//     output drift. Each documents its quirks and its structured replacement.
//
// See docs/TYPE_MODEL.md for the migration plan.
package typemodel

import "strings"

// Sep separates a package path from a type or function name in the internal
// string encoding (`pkg-->Type`). This is the single source of truth;
// internal/spec's TypeSep aliases it.
const Sep = "-->"

// Kind discriminates the shape of a TypeRef.
type Kind uint8

const (
	// KindNamed is a named, builtin, or otherwise opaque type, possibly a
	// generic instantiation: pkg.Type[Args].
	KindNamed Kind = iota
	// KindPointer is *Elem.
	KindPointer
	// KindSlice is []Elem.
	KindSlice
	// KindArray is [Len]Elem.
	KindArray
	// KindMap is map[Key]Elem.
	KindMap
	// KindChan is a channel of Elem; Dir qualifies the direction.
	KindChan
)

// ChanDir is a channel direction.
type ChanDir uint8

const (
	// SendRecv is a bidirectional channel (chan T).
	SendRecv ChanDir = iota
	// SendOnly is a send-only channel (chan<- T).
	SendOnly
	// RecvOnly is a receive-only channel (<-chan T).
	RecvOnly
)

// TypeRef is a structured reference to a Go type as written at a use site: a
// possibly package-qualified named type (with generic arguments), wrapped in
// any number of pointer/slice/array/map/chan constructors. It carries type
// *identity and shape* — not the type's definition (fields, methods), which
// stays in metadata.Type.
//
// Field usage by Kind:
//
//	KindNamed:   Pkg, Name, Args, Constraint
//	KindPointer: Elem
//	KindSlice:   Elem
//	KindArray:   Len, Elem
//	KindMap:     Key, Elem
//	KindChan:    Dir, Elem
type TypeRef struct {
	Kind Kind

	// Pkg is the import path qualifying Name; empty for builtins, local
	// names, and type parameters.
	Pkg string
	// Name is the simple type name. Opaque or unparseable input is preserved
	// here verbatim so rendering never loses information.
	Name string
	// Args are the generic type arguments of an instantiation (Page[User]),
	// or the declared parameters of a generic declaration (Page[T any], where
	// each arg carries a Constraint).
	Args []*TypeRef
	// Constraint is the declared constraint of a declaration-form type
	// parameter ("any" in "T any"); empty for instantiation arguments.
	Constraint string

	// Key is the map key type (KindMap only).
	Key *TypeRef
	// Elem is the wrapped element type (pointer/slice/array/chan element,
	// map value).
	Elem *TypeRef
	// Len is the literal array-length text (KindArray only).
	Len string
	// Dir is the channel direction (KindChan only).
	Dir ChanDir

	// raw is the exact input substring this ref was parsed from; empty when
	// the ref was constructed programmatically.
	raw string
}

// Parse builds a TypeRef from any of the string encodings in use across the
// codebase: the internal form (pkg-->Type, including the legacy
// pkg-->Type-->Arg argument encoding), the go/types dotted form
// (pkg.Type[pkg.Arg]), and Go type syntax wrappers (*T, []T, [N]T,
// map[K]V, chan T). Parsing never fails: input it cannot model is preserved
// opaquely in Name and rendered back verbatim.
func Parse(s string) *TypeRef {
	return parse(strings.TrimSpace(s))
}

func parse(s string) *TypeRef {
	t := &TypeRef{raw: s}

	switch {
	case s == "":
		return t
	case strings.HasPrefix(s, "*"):
		t.Kind = KindPointer
		t.Elem = parse(s[1:])
		return t
	case strings.HasPrefix(s, "[]"):
		t.Kind = KindSlice
		t.Elem = parse(s[2:])
		return t
	case strings.HasPrefix(s, "map["):
		if key, elem, ok := splitMapKey(s[len("map["):]); ok {
			t.Kind = KindMap
			t.Key = parse(key)
			t.Elem = parse(elem)
			return t
		}
	case strings.HasPrefix(s, "chan<- "):
		t.Kind = KindChan
		t.Dir = SendOnly
		t.Elem = parse(s[len("chan<- "):])
		return t
	case strings.HasPrefix(s, "<-chan "):
		t.Kind = KindChan
		t.Dir = RecvOnly
		t.Elem = parse(s[len("<-chan "):])
		return t
	case strings.HasPrefix(s, "chan "):
		t.Kind = KindChan
		t.Elem = parse(s[len("chan "):])
		return t
	case strings.HasPrefix(s, "["):
		// Array with a length: [N]Elem. The length is a constant expression
		// and may itself contain brackets ([len([3]int{})]byte), so find the
		// matching close bracket, not the first one. Anything bracketed that
		// doesn't close before more input falls through to the opaque named
		// fallback.
		if i := matchingBracket(s); i > 1 && i < len(s)-1 {
			t.Kind = KindArray
			t.Len = s[1:i]
			t.Elem = parse(s[i+1:])
			return t
		}
	}
	parseNamed(s, t)
	return t
}

// matchingBracket returns the index of the "]" closing the "[" at s[0], or -1
// when the bracket never closes.
func matchingBracket(s string) int {
	depth := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// splitMapKey splits "K]V" (the remainder of "map[K]V") at the bracket that
// closes the key, respecting nested brackets inside K.
func splitMapKey(s string) (key, elem string, ok bool) {
	depth := 1
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				return s[:i], s[i+1:], i+1 < len(s)
			}
		}
	}
	return "", "", false
}

// parseNamed fills t as a named type: an optional bracketed generic argument
// list peeled off the end, then the base split into package qualifier and
// simple name (internal Sep form or dotted form).
func parseNamed(s string, t *TypeRef) {
	t.Kind = KindNamed
	base := s
	if open := strings.Index(s, "["); open > 0 && strings.HasSuffix(s, "]") {
		base = s[:open]
		for _, a := range SplitArgs(s[open+1 : len(s)-1]) {
			t.Args = append(t.Args, parseArg(a))
		}
	}
	if pkg, rest, ok := strings.Cut(base, Sep); ok {
		t.Pkg = pkg
		segs := strings.Split(rest, Sep)
		t.Name = segs[0]
		// Legacy pkg-->Type-->Arg encoding (no brackets): trailing segments
		// are the type arguments. When a bracket already supplied the
		// arguments, extra separator segments are dropped — matching the
		// legacy parser.
		if len(t.Args) == 0 {
			for _, a := range segs[1:] {
				t.Args = append(t.Args, parseArg(a))
			}
		}
		return
	}
	if dot := strings.LastIndex(base, "."); dot > 0 {
		t.Pkg = base[:dot]
		t.Name = base[dot+1:]
		return
	}
	t.Name = base
}

// parseArg parses one generic argument: either a declaration-form type
// parameter ("T any") or a type reference.
func parseArg(s string) *TypeRef {
	if name, constraint, ok := declArg(s); ok {
		return &TypeRef{Kind: KindNamed, Name: name, Constraint: constraint, raw: s}
	}
	return parse(s)
}

// declArg recognizes a declaration-form type parameter ("T any",
// "K comparable", "T constraints.Ordered"): a bare identifier followed by its
// constraint. Composite types that contain spaces (chan/map/func/interface
// forms) are not declarations.
func declArg(s string) (name, constraint string, ok bool) {
	sp := strings.IndexByte(s, ' ')
	if sp <= 0 {
		return "", "", false
	}
	head := s[:sp]
	switch head {
	case "chan", "func", "struct", "map", "interface":
		return "", "", false
	}
	if !isIdent(head) {
		return "", "", false
	}
	return head, strings.TrimSpace(s[sp+1:]), true
}

// isIdent reports whether s is a plain Go identifier.
func isIdent(s string) bool {
	for i, r := range s {
		switch {
		case r == '_', 'a' <= r && r <= 'z', 'A' <= r && r <= 'Z':
		case i > 0 && '0' <= r && r <= '9':
		default:
			return false
		}
	}
	return s != ""
}

type renderMode uint8

const (
	renderDotted renderMode = iota
	renderInternal
	renderSimple
)

// String renders the go/types-style dotted form (pkg.Type[pkg.Arg]) with all
// qualifiers kept. For well-formed dotted input, Parse(s).String() == s.
func (t *TypeRef) String() string { return t.render(renderDotted) }

// Internal renders the canonical internal component-key form: the base
// qualified with Sep and every generic argument reduced to its simple form
// (pkg-->Page[User]) so the bracketed list stays free of qualifiers. For
// input already in that form, Parse(s).Internal() == s.
func (t *TypeRef) Internal() string { return t.render(renderInternal) }

// Simple renders the fully unqualified form (Page[User]), normalizing the
// empty interface to "any" (braces are illegal in an OpenAPI component name).
func (t *TypeRef) Simple() string { return t.render(renderSimple) }

func (t *TypeRef) render(m renderMode) string {
	if t == nil {
		return ""
	}
	switch t.Kind {
	case KindPointer:
		return "*" + t.Elem.render(m)
	case KindSlice:
		return "[]" + t.Elem.render(m)
	case KindArray:
		return "[" + t.Len + "]" + t.Elem.render(m)
	case KindMap:
		return "map[" + t.Key.render(m) + "]" + t.Elem.render(m)
	case KindChan:
		switch t.Dir {
		case SendOnly:
			return "chan<- " + t.Elem.render(m)
		case RecvOnly:
			return "<-chan " + t.Elem.render(m)
		}
		return "chan " + t.Elem.render(m)
	}

	name := t.Name
	if m == renderSimple && (name == "interface{}" || name == "interface {}") {
		name = "any"
	}
	if t.Constraint != "" {
		name += " " + t.Constraint
	}
	base := name
	if t.Pkg != "" && m != renderSimple {
		if m == renderInternal {
			base = t.Pkg + Sep + name
		} else {
			base = t.Pkg + "." + name
		}
	}
	if len(t.Args) == 0 {
		return base
	}
	args := make([]string, len(t.Args))
	for i, a := range t.Args {
		if m == renderDotted {
			args[i] = a.render(renderDotted)
		} else {
			// Internal and simple forms keep arguments simple so the
			// bracketed list parses cleanly and sanitizes to a valid
			// component name.
			args[i] = a.render(renderSimple)
		}
	}
	return base + "[" + strings.Join(args, ", ") + "]"
}

// Canonicalize renders a generic instantiation in the canonical internal
// component-key form (pkg-->Type[SimpleArgs]) regardless of which encoding it
// arrived in: the go/types dotted form an inferred instantiation renders as,
// the internal form, or either wrapped in pointer/slice constructors (which
// the legacy string view mangled). An unqualified base borrows the package of
// the first qualified argument — an envelope and its payload almost always
// co-locate — so a request-body var form (Page[github.com/acme/svc.User])
// keys to the same component as the encode-site form.
//
// Returned unchanged: non-instantiations, bare unqualified generics with no
// qualified argument to borrow from (Container[T]), and map types (a map is
// not a component-key candidate).
func Canonicalize(s string) string {
	if !strings.Contains(s, "[") || !strings.HasSuffix(s, "]") {
		return s
	}
	ref := Parse(s)
	core := ref.Core()
	if core == nil || core.Kind != KindNamed || len(core.Args) == 0 {
		return s
	}
	if core.Pkg == "" {
		for _, a := range core.Args {
			if c := a.Core(); c != nil && c.Pkg != "" {
				core.Pkg = c.Pkg
				break
			}
		}
		if core.Pkg == "" {
			return s
		}
	}
	return ref.Internal()
}

// Raw returns the exact input this ref was parsed from, or "" if it was
// constructed programmatically.
func (t *TypeRef) Raw() string {
	if t == nil {
		return ""
	}
	return t.raw
}

// Core unwraps pointer/slice/array/chan constructors and returns the
// innermost element ref (a map is returned as-is: its element is not "the"
// core type).
func (t *TypeRef) Core() *TypeRef {
	for t != nil {
		switch t.Kind {
		case KindPointer, KindSlice, KindArray, KindChan:
			t = t.Elem
		default:
			return t
		}
	}
	return nil
}

// IsNamed reports whether the ref itself (not its core) is a named type.
func (t *TypeRef) IsNamed() bool { return t != nil && t.Kind == KindNamed }

// IsGeneric reports whether the core type carries generic arguments.
func (t *TypeRef) IsGeneric() bool {
	c := t.Core()
	return c != nil && c.Kind == KindNamed && len(c.Args) > 0
}

// Qualified reports whether the core type carries a package qualifier.
func (t *TypeRef) Qualified() bool {
	c := t.Core()
	return c != nil && c.Pkg != ""
}
