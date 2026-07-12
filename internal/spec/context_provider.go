package spec

import (
	"fmt"
	"slices"
	"strings"

	"github.com/ehabterra/apispec/internal/metadata"
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
			// instantiation (Page[User]{...} / Pair[K,V]{...}). Render it
			// carrying the concrete type arguments — `Base[User]` — so the
			// schema layer can substitute them into the parametric struct's
			// fields, instead of falling back to the bare declaration
			// (Page[T any]) and collapsing every instantiation together.
			switch arg.X.GetKind() {
			case metadata.KindIndex:
				if arg.X.X != nil && arg.X.Fun != nil {
					return c.genericInstantiationName(arg.X.X, []*metadata.CallArgument{arg.X.Fun})
				}
			case metadata.KindIndexList:
				if arg.X.X != nil && len(arg.X.Args) > 0 {
					return c.genericInstantiationName(arg.X.X, arg.X.Args)
				}
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

// genericInstantiationName renders a generic type instantiation as
// `Base[Arg1,Arg2]` (literal brackets), preserving the concrete type
// arguments so the schema layer resolves each instantiation to its own
// component (Page[User] vs Page[Product]) rather than collapsing onto the bare
// declaration. Base keeps its package qualifier (e.g. pkg-->Page); each type
// argument is reduced to its simple type name so the resulting bracketed form
// parses cleanly through TypeParts and sanitizes to a readable component name.
// Cross-package type arguments are reduced to their base name (a known v1
// limitation) rather than dropped.
func (c *ContextProviderImpl) genericInstantiationName(base *metadata.CallArgument, typeArgs []*metadata.CallArgument) string {
	baseStr := c.callArgToString(base, nil)
	if baseStr == "" {
		return ""
	}
	// The base ident renders with its own declared type parameters
	// (Page[T any]); drop them — we re-append the concrete arguments below.
	if i := strings.Index(baseStr, "["); i >= 0 {
		baseStr = baseStr[:i]
	}
	args := make([]string, 0, len(typeArgs))
	for _, a := range typeArgs {
		if a == nil {
			continue
		}
		if name := simpleGenericArgName(c.callArgToString(a, nil)); name != "" {
			args = append(args, name)
		}
	}
	if len(args) == 0 {
		return baseStr
	}
	return baseStr + "[" + strings.Join(args, ",") + "]"
}

// simpleGenericArgName reduces a rendered type-argument string to its simple
// type name, stripping package qualifiers (which use TypeSep, "/", or ".") and
// any leading pointer/slice markers. Keeping the bracketed argument free of
// TypeSep is what lets TypeParts recover it via its single-generic bracket
// fallback.
func simpleGenericArgName(s string) string {
	s = strings.TrimPrefix(s, "[]")
	s = strings.TrimPrefix(s, "*")
	for _, sepr := range []string{TypeSep, "/", "."} {
		if i := strings.LastIndex(s, sepr); i >= 0 {
			s = s[i+len(sepr):]
		}
	}
	// The empty interface is a valid type argument (APIResponse[interface{}])
	// but "{" / "}" are illegal in an OpenAPI component name; normalize it to
	// the equivalent "any" so the instantiation key stays valid.
	if s == "interface{}" || s == "interface {}" {
		return "any"
	}
	return s
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
