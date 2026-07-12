package spec

import (
	"fmt"
	"slices"
	"strings"

	"github.com/ehabterra/apispec/internal/metadata"
	"github.com/ehabterra/apispec/internal/typemodel"
)

// ContextProviderImpl implements ContextProvider
type ContextProviderImpl struct {
	meta *metadata.Metadata
}

// NewContextProvider creates a new context provider
func NewContextProvider(meta *metadata.Metadata) *ContextProviderImpl {
	return &ContextProviderImpl{
		meta: meta,
	}
}

// GetString gets a string from the string pool
func (c *ContextProviderImpl) GetString(idx int) string {
	if c.meta == nil || c.meta.StringPool == nil {
		return ""
	}
	return c.meta.StringPool.GetString(idx)
}

// GetCalleeInfo gets callee information from a node
func (c *ContextProviderImpl) GetCalleeInfo(node TrackerNodeInterface) (name, pkg, recvType string) {
	if node == nil || node.GetEdge() == nil {
		return "", "", ""
	}
	edge := node.GetEdge()
	return c.GetString(edge.Callee.Name), c.GetString(edge.Callee.Pkg), c.GetString(edge.Callee.RecvType)
}

// GetArgumentInfo gets argument information as a string
func (c *ContextProviderImpl) GetArgumentInfo(arg *metadata.CallArgument) string {
	return c.callArgToString(arg, nil)
}

// callArgToString converts a call argument to a string representation
func (c *ContextProviderImpl) callArgToString(arg *metadata.CallArgument, sep *string) string {
	// Use provided separator or default
	separator := "."
	if sep != nil && *sep != "" {
		separator = *sep
	}

	switch arg.GetKind() {
	case metadata.KindLiteral:
		// Remove quotes from string literals
		return strings.Trim(arg.GetValue(), "\"")

	case metadata.KindKeyValue:
		return ""

	case metadata.KindMapType:
		if arg.X != nil && arg.Fun != nil {
			return fmt.Sprintf("map[%s]%s", c.callArgToString(arg.X, nil), c.callArgToString(arg.Fun, nil))
		}
		return "map"

	case metadata.KindUnary:
		// Handle unary expressions (e.g., *X)
		if arg.X != nil {
			return "*" + c.callArgToString(arg.X, nil)
		}
		return "*"
	case metadata.KindArrayType:
		// Handle index expressions (e.g., arr[i])
		if arg.X != nil {
			return "[]" + c.callArgToString(arg.X, nil)
		}
		return "[]"
	case metadata.KindIndex:
		// Handle index expressions (e.g., arr[i])
		if arg.X != nil {
			return "*" + c.callArgToString(arg.X, nil)
		}
		return "*"
	case metadata.KindCompositeLit:
		if arg.X != nil {
			// A composite literal's type expression can be a generic
			// instantiation (Page[User]{...} / Pair[K,V]{...}), possibly
			// wrapped in a slice constructor ([]Envelope[User]{...}). Render
			// it carrying the concrete type arguments — `Base[User]` — so the
			// schema layer can substitute them into the parametric struct's
			// fields, instead of falling back to the bare declaration
			// (Page[T any]) and collapsing every instantiation together.
			if name := c.instantiationName(arg.X); name != "" {
				return name
			}
			return c.callArgToString(arg.X, nil)
		}
		return ""

	case metadata.KindIdent, metadata.KindFuncLit:
		// Try to resolve as a constant value from metadata
		if pkg, exists := c.meta.Packages[arg.GetPkg()]; exists {
			for _, file := range pkg.Files {
				if variable, exists := file.Variables[arg.GetName()]; exists && c.GetString(variable.Tok) == "const" {
					return strings.Trim(c.GetString(variable.Value), "\"")
				}
			}
		}
		// If not a function type, build a qualified type string
		if !strings.HasPrefix(arg.GetType(), "func(") && !strings.HasPrefix(arg.GetType(), "func[") {
			if arg.GetType() != "" {
				// Check if this is a built-in Go type that doesn't need package prefix
				builtinTypes := []string{
					"string", "int", "int8", "int16", "int32", "int64",
					"uint", "uint8", "uint16", "uint32", "uint64",
					"float32", "float64", "bool", "byte", "rune",
					"error", "interface{}", "any",
				}

				// Check for map types (built-in)
				if strings.HasPrefix(arg.GetType(), "map[") {
					return arg.GetType()
				}

				// Check for slice types with built-in element types
				if after, ok := strings.CutPrefix(arg.GetType(), "[]"); ok {
					elementType := after
					elementType = strings.TrimPrefix(elementType, "*")
					if slices.Contains(builtinTypes, elementType) {
						return arg.GetType()
					}
				}

				// Check for pointer types with built-in base types
				if strings.HasPrefix(arg.GetType(), "*") {
					baseType := strings.TrimPrefix(arg.GetType(), "*")
					for _, builtin := range builtinTypes {
						if baseType == builtin {
							return arg.GetType()
						}
					}
				}

				// Check if it's a built-in type
				if slices.Contains(builtinTypes, arg.GetType()) {
					return arg.GetType()
				}

				// If we have a package and type, process as custom type
				if arg.GetPkg() != "" {
					// Remove slice, pointer, and redundant package prefixes
					argType := strings.TrimPrefix(arg.GetType(), "[]")
					argType = strings.TrimPrefix(argType, "*")
					argType = strings.TrimPrefix(argType, arg.GetPkg()+separator)

					// Add only if the pkg is deattached from the type
					if !strings.Contains(argType, "/") {
						// Re-add package prefix
						argType = arg.GetPkg() + TypeSep + argType
					}

					// If original type was a slice, add [] prefix
					if strings.HasPrefix(arg.GetType(), "[]") {
						argType = "[]" + argType
					}
					return argType
				}

				// If no package but has type, return as is
				return arg.GetType()
			}
		}

		var argName string

		if arg.GetType() == "" && strings.HasSuffix(DefaultPackageName(arg.GetPkg()), arg.GetName()) {
			argName = DefaultPackageName(arg.GetPkg())
		} else if arg.GetType() != "" {
			argName = DefaultPackageName(arg.GetPkg()) + "." + arg.GetName()
		} else {
			argName = DefaultPackageName(arg.GetPkg()) + "/" + arg.GetName()
		}

		// Fallback to variable name
		return argName

	case metadata.KindSelector:
		// Handle selector expressions (e.g., pkg.X.Sel)
		if arg.X != nil {
			var pkgKey string

			if arg.X.Type == -1 && !strings.HasSuffix(arg.X.GetPkg(), arg.X.GetName()) {
				pkgKey = arg.X.GetPkg() + "/" + arg.X.GetName()
			} else {
				pkgKey = arg.X.GetPkg()
			}

			if pkg, exists := c.meta.Packages[pkgKey]; exists {
				for _, file := range pkg.Files {
					var selName string

					if arg.X.GetType() != "" {
						selName = arg.X.GetType() + "." + selName
					} else {
						selName = arg.Sel.GetName()
					}
					if variable, exists := file.Variables[selName]; exists {
						return strings.Trim(c.GetString(variable.Value), "\"")
					}
				}
			}
			xResult := c.callArgToString(arg.X, strPtr("/"))
			if xResult != "" {
				return xResult + "." + arg.Sel.GetName()
			}
		}
		return arg.Sel.GetName()

	case metadata.KindCall:
		// Handle function call expressions
		if arg.Fun != nil {
			argName := c.callArgToString(arg.Fun, nil)
			if arg.GetPkg() != "" {
				argName = arg.GetPkg() + separator + arg.GetName()
			}

			// if resolvedType := arg.GetResolvedType(); resolvedType != "" {
			// 	argName = resolvedType
			// }

			typeParams := arg.TypeParams()
			if len(typeParams) > 0 {
				// typeParams is a map[paramName]concreteType; iterate by sorted
				// key so the rendered type-argument list is deterministic
				// (otherwise HandleRequest[A, B] vs [B, A] flips between runs).
				names := make([]string, 0, len(typeParams))
				for name := range typeParams {
					names = append(names, name)
				}
				slices.Sort(names)
				typParam := "["
				for _, name := range names {
					typParam += typeParams[name] + ", "
				}
				typParam = typParam[:len(typParam)-2] + "]"
				argName += typParam
			}

			return argName
		}
		return "call(...)"

	case metadata.KindTypeConversion:
		// Handle type conversions like []byte("value")
		if arg.Fun != nil {
			// For type conversions, we want to get the target type
			targetType := c.callArgToString(arg.Fun, nil)

			return targetType
		}
		return ""

	case metadata.KindInterfaceType:
		// interface{}
		return "interface{}"
	case metadata.KindRaw:
		// Raw string value
		return arg.GetRaw()
	}
	// Fallback for unknown kinds
	return ""
}

// instantiationName renders a composite literal's type expression when it is
// a generic instantiation — directly (Page[User]{...}, Pair[K,V]{...}) or
// wrapped in slice constructors ([]Envelope[User]{...}) — and "" otherwise.
// Preserving the concrete arguments through the wrapper is what keeps a
// slice-of-instantiation element from collapsing onto the declaration
// placeholder (Envelope[T any]).
func (c *ContextProviderImpl) instantiationName(x *metadata.CallArgument) string {
	switch x.GetKind() {
	case metadata.KindIndex:
		if x.X != nil && x.Fun != nil {
			return c.genericInstantiationName(x.X, []*metadata.CallArgument{x.Fun})
		}
	case metadata.KindIndexList:
		if x.X != nil && len(x.Args) > 0 {
			return c.genericInstantiationName(x.X, x.Args)
		}
	case metadata.KindArrayType:
		if x.X != nil {
			if inner := c.instantiationName(x.X); inner != "" {
				return "[]" + inner
			}
		}
	}
	return ""
}

// genericInstantiationName renders a generic type instantiation in the
// canonical internal component-key form (pkg-->Page[User]), preserving the
// concrete type arguments so the schema layer resolves each instantiation to
// its own component (Page[User] vs Page[Product]) rather than collapsing onto
// the bare declaration. The instantiation is built as a structured TypeRef —
// the base's declared parameters (Page[T any]) replaced by the concrete
// arguments — and rendered via Internal(), which keeps the base's package
// qualifier and reduces every argument to its simple form (the component-name
// convention; ", " joins so the sanitizer yields …Pair_User-Product with no
// raw comma).
func (c *ContextProviderImpl) genericInstantiationName(base *metadata.CallArgument, typeArgs []*metadata.CallArgument) string {
	baseStr := c.callArgToString(base, nil)
	if baseStr == "" {
		return ""
	}
	ref := typemodel.Parse(baseStr)
	core := ref.Core()
	core.Args = nil
	for _, a := range typeArgs {
		if a == nil {
			continue
		}
		if arg := c.genericArgRef(a); arg != nil {
			core.Args = append(core.Args, arg)
		}
	}
	if len(core.Args) == 0 {
		// No renderable arguments: fall back to the base with its declared
		// parameter brackets stripped, preserving its original encoding.
		if i := strings.Index(baseStr, "["); i >= 0 {
			return baseStr[:i]
		}
		return baseStr
	}
	return ref.Internal()
}

// genericArgRef builds the TypeRef of one type argument of a generic
// instantiation. A nested generic argument (the Page[User] in
// Envelope[Page[User]]) recurses with its own concrete arguments attached;
// leaves parse the rendered argument. The structured ref keeps the argument's
// qualifier and wrappers — Internal() reduces arguments to their simple form
// at render time, so a nested instantiation still re-parses cleanly once the
// enclosing type's package is glued back on during field resolution.
func (c *ContextProviderImpl) genericArgRef(a *metadata.CallArgument) *typemodel.TypeRef {
	switch a.GetKind() {
	case metadata.KindIndex:
		if a.X != nil && a.Fun != nil {
			if inner := c.genericArgRef(a.Fun); inner != nil {
				if base := c.genericBaseRef(a.X); base != nil {
					base.Args = []*typemodel.TypeRef{inner}
					return base
				}
			}
		}
	case metadata.KindIndexList:
		if a.X != nil && len(a.Args) > 0 {
			inners := make([]*typemodel.TypeRef, 0, len(a.Args))
			for _, sub := range a.Args {
				if sub == nil {
					continue
				}
				if r := c.genericArgRef(sub); r != nil {
					inners = append(inners, r)
				}
			}
			if len(inners) > 0 {
				if base := c.genericBaseRef(a.X); base != nil {
					base.Args = inners
					return base
				}
			}
		}
	}
	s := c.callArgToString(a, nil)
	if s == "" {
		return nil
	}
	return typemodel.Parse(s)
}

// genericBaseRef parses a generic base expression to its named core with the
// declared type-parameter brackets dropped (the concrete arguments replace
// them).
func (c *ContextProviderImpl) genericBaseRef(a *metadata.CallArgument) *typemodel.TypeRef {
	s := c.callArgToString(a, nil)
	if s == "" {
		return nil
	}
	base := typemodel.Parse(s).Core()
	if base == nil || base.Name == "" {
		return nil
	}
	base.Args = nil
	return base
}

// DefaultPackageName returns the default package name for an package path (last non-version segment)
func DefaultPackageName(pkgPath string) string {
	parts := strings.Split(pkgPath, "/")
	if len(parts) == 0 {
		return ""
	}
	last := parts[len(parts)-1]
	// If last is a version (e.g., v5), use the one before it
	if len(parts) > 1 && strings.HasPrefix(last, "v") && len(last) > 1 && last[1] >= '0' && last[1] <= '9' {
		return pkgPath[:len(pkgPath)-len(last)-1]
	}
	return pkgPath
}

// strPtr returns a pointer to the given string (helper for separator passing)
func strPtr(s string) *string { return &s }
