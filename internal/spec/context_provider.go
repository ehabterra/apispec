package spec

import (
	"fmt"
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

	switch arg.Kind {
	case "literal":
		// Remove quotes from string literals
		return strings.Trim(arg.Value, "\"")

	case "key_value":
		return ""

	case "map_type":
		if arg.X != nil && arg.Fun != nil {
			return fmt.Sprintf("map[%s]%s", c.callArgToString(*arg.X, nil), c.callArgToString(*arg.Fun, nil))
		}
		return "map"

	case "unary":
		// Handle unary expressions (e.g., *X)
		if arg.X != nil {
			return "*" + c.callArgToString(*arg.X, nil)
		}
		return "*"
	case "index":
		// Handle index expressions (e.g., arr[i])
		if arg.X != nil {
			return "*" + c.callArgToString(*arg.X, nil)
		}
		return "*"
	case "composite_lit":
		if arg.X != nil {
			return c.callArgToString(*arg.X, nil)
		}
		return ""

	case "ident":
		if arg.Pkg == "net/http" && strings.HasPrefix(arg.Name, "Status") {
			return arg.Pkg + separator + arg.Name
		}

		// Try to resolve as a constant value from metadata
		if pkg, exists := c.meta.Packages[arg.Pkg]; exists {
			for _, file := range pkg.Files {
				if variable, exists := file.Variables[arg.Name]; exists && c.GetString(variable.Tok) == "const" {
					return strings.Trim(c.GetString(variable.Value), "\"")
				}
			}
		}
		// If not a function type, build a qualified type string
		if !strings.HasPrefix(arg.Type, "func(") && !strings.HasPrefix(arg.Type, "func[") {
			if arg.Type != "" {
				// Check if this is a built-in Go type that doesn't need package prefix
				builtinTypes := []string{
					"string", "int", "int8", "int16", "int32", "int64",
					"uint", "uint8", "uint16", "uint32", "uint64",
					"float32", "float64", "bool", "byte", "rune",
					"error", "interface{}", "any",
				}

				// Check for map types (built-in)
				if strings.HasPrefix(arg.Type, "map[") {
					return arg.Type
				}

				// Check for slice types with built-in element types
				if strings.HasPrefix(arg.Type, "[]") {
					elementType := strings.TrimPrefix(arg.Type, "[]")
					elementType = strings.TrimPrefix(elementType, "*")
					for _, builtin := range builtinTypes {
						if elementType == builtin {
							return arg.Type
						}
					}
				}

				// Check for pointer types with built-in base types
				if strings.HasPrefix(arg.Type, "*") {
					baseType := strings.TrimPrefix(arg.Type, "*")
					for _, builtin := range builtinTypes {
						if baseType == builtin {
							return arg.Type
						}
					}
				}

				// Check if it's a built-in type
				for _, builtin := range builtinTypes {
					if arg.Type == builtin {
						return arg.Type
					}
				}

				// If we have a package and type, process as custom type
				if arg.Pkg != "" {
					// Remove slice, pointer, and redundant package prefixes
					argType := strings.TrimPrefix(arg.Type, "[]")
					argType = strings.TrimPrefix(argType, "*")
					argType = strings.TrimPrefix(argType, arg.Pkg+separator)

					// Add only if the pkg is deattached from the type
					if !strings.Contains(argType, "/") {
						// Re-add package prefix
						argType = arg.Pkg + TypeSep + argType
					}

					// If original type was a slice, add [] prefix
					if strings.HasPrefix(arg.Type, "[]") {
						argType = "[]" + argType
					}
					return argType
				}

				// If no package but has type, return as is
				return arg.Type
			}
		}

		argName := arg.Name
		if arg.Pkg != "" {
			argName = arg.Pkg + separator + arg.Name
		}

		// Fallback to variable name
		return argName

	case "selector":
		// Handle selector expressions (e.g., pkg.X.Sel)
		if arg.X != nil {
			pkgKey := arg.X.Pkg + "/" + arg.X.Name
			if pkg, exists := c.meta.Packages[pkgKey]; exists {
				for _, file := range pkg.Files {
					if variable, exists := file.Variables[arg.Sel]; exists {
						return strings.Trim(c.GetString(variable.Value), "\"")
					}
				}
			}
			xResult := c.callArgToString(*arg.X, strPtr("/"))
			if xResult != "" {
				return xResult + "." + arg.Sel
			}
		}
		return arg.Sel

	case "call":
		// Handle function call expressions
		if arg.Fun != nil {
			argName := c.callArgToString(*arg.Fun, nil)
			if arg.Pkg != "" {
				argName = arg.Pkg + separator + arg.Name
			}

			if len(arg.TypeParamMap) > 0 {
				typParam := "["
				for _, val := range arg.TypeParamMap {
					typParam += val + ", "
				}
				typParam = typParam[:len(typParam)-2] + "]"
				argName += typParam
			}

			return argName
		}
		return "call(...)"

	case "interface_type":
		// interface{}
		return "interface{}"
	case "raw":
		// Raw string value
		return arg.Raw
	}
	// Fallback for unknown kinds
	return ""
}

// strPtr returns a pointer to the given string (helper for separator passing)
func strPtr(s string) *string { return &s }
