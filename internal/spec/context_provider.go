package spec

import (
	"fmt"
	"slices"
	"strings"

	"github.com/ehabterra/swagen/internal/metadata"
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

// GetCallerInfo gets caller information from a node
func (c *ContextProviderImpl) GetCallerInfo(node *TrackerNode) (name, pkg string) {
	if node == nil || node.CallGraphEdge == nil {
		return "", ""
	}
	return c.GetString(node.CallGraphEdge.Caller.Name), c.GetString(node.CallGraphEdge.Caller.Pkg)
}

// GetCalleeInfo gets callee information from a node
func (c *ContextProviderImpl) GetCalleeInfo(node *TrackerNode) (name, pkg, recvType string) {
	if node == nil || node.CallGraphEdge == nil {
		return "", "", ""
	}
	return c.GetString(node.CallGraphEdge.Callee.Name), c.GetString(node.CallGraphEdge.Callee.Pkg), c.GetString(node.CallGraphEdge.Callee.RecvType)
}

// GetArgumentInfo gets argument information as a string
func (c *ContextProviderImpl) GetArgumentInfo(arg metadata.CallArgument) string {
	return c.callArgToString(arg, nil)
}

// callArgToString converts a call argument to a string representation
func (c *ContextProviderImpl) callArgToString(arg metadata.CallArgument, sep *string) string {
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
			return fmt.Sprintf("map[%s]%s", c.callArgToString(*arg.X, nil), c.callArgToString(*arg.Fun, nil))
		}
		return "map"

	case metadata.KindUnary:
		// Handle unary expressions (e.g., *X)
		if arg.X != nil {
			return "*" + c.callArgToString(*arg.X, nil)
		}
		return "*"
	case metadata.KindIndex:
		// Handle index expressions (e.g., arr[i])
		if arg.X != nil {
			return "*" + c.callArgToString(*arg.X, nil)
		}
		return "*"
	case metadata.KindCompositeLit:
		if arg.X != nil {
			return c.callArgToString(*arg.X, nil)
		}
		return ""

	case metadata.KindIdent:
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
				for _, builtin := range builtinTypes {
					if arg.GetType() == builtin {
						return arg.GetType()
					}
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
		} else {
			argName = DefaultPackageName(arg.GetPkg()) + "/" + arg.GetName()
		}

		// Fallback to variable name
		return argName

	case metadata.KindSelector:
		// Handle selector expressions (e.g., pkg.X.Sel)
		if arg.X != nil {
			var pkgKey string

			if arg.X.GetType() == "" && strings.HasSuffix(arg.X.GetPkg(), arg.X.GetName()) {
				pkgKey = arg.X.GetPkg()
			} else {
				pkgKey = arg.X.GetPkg() + "/" + arg.X.GetName()
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
			xResult := c.callArgToString(*arg.X, strPtr("/"))
			if xResult != "" {
				return xResult + "." + arg.Sel.GetName()
			}
		}
		return arg.Sel.GetName()

	case metadata.KindCall:
		// Handle function call expressions
		if arg.Fun != nil {
			argName := c.callArgToString(*arg.Fun, nil)
			if arg.GetPkg() != "" {
				argName = arg.GetPkg() + separator + arg.GetName()
			}

			typeParams := arg.TypeParams()
			if len(typeParams) > 0 {
				typParam := "["
				for _, val := range typeParams {
					typParam += val + ", "
				}
				typParam = typParam[:len(typParam)-2] + "]"
				argName += typParam
			}

			return argName
		}
		return "call(...)"

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
