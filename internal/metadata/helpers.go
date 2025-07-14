package metadata

import (
	"fmt"
	"go/ast"
	"go/token"
	"strings"
)

// getTypeName extracts a type name from an AST expression
func getTypeName(expr ast.Expr) string {
	if expr == nil {
		return ""
	}

	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return "*" + getTypeName(t.X)
	case *ast.SelectorExpr:
		if x, ok := t.X.(*ast.Ident); ok {
			return x.Name + "." + t.Sel.Name
		}
		return t.Sel.Name
	case *ast.ArrayType:
		return "[]" + getTypeName(t.Elt)
	case *ast.MapType:
		return "map[" + getTypeName(t.Key) + "]" + getTypeName(t.Value)
	case *ast.InterfaceType:
		return "interface{}"
	case *ast.StructType:
		return "struct{}"
	case *ast.FuncType:
		return "func"
	case *ast.ChanType:
		if t.Dir == ast.SEND {
			return "chan<- " + getTypeName(t.Value)
		} else if t.Dir == ast.RECV {
			return "<-chan " + getTypeName(t.Value)
		}
		return "chan " + getTypeName(t.Value)
	}
	return ""
}

// getPosition returns a string representation of a position
func getPosition(pos token.Pos, fset *token.FileSet) string {
	if !pos.IsValid() || fset == nil {
		return ""
	}
	return fset.Position(pos).String()
}

// getFuncPosition returns the position of a function declaration
func getFuncPosition(fn *ast.FuncDecl, fset *token.FileSet) string {
	if fn == nil {
		return ""
	}
	return getPosition(fn.Pos(), fset)
}

// getVarPosition returns the position of a variable identifier
func getVarPosition(ident *ast.Ident, fset *token.FileSet) string {
	if ident == nil {
		return ""
	}
	return getPosition(ident.Pos(), fset)
}

// getComments extracts comments from AST nodes
func getComments(node ast.Node) string {
	if node == nil {
		return ""
	}

	switch n := node.(type) {
	case *ast.FuncDecl:
		if n.Doc != nil {
			return strings.TrimSpace(n.Doc.Text())
		}
	case *ast.GenDecl:
		if n.Doc != nil {
			return strings.TrimSpace(n.Doc.Text())
		}
	case *ast.Field:
		var comments []string
		if n.Doc != nil {
			comments = append(comments, strings.TrimSpace(n.Doc.Text()))
		}
		if n.Comment != nil {
			comments = append(comments, strings.TrimSpace(n.Comment.Text()))
		}
		return strings.Join(comments, "\n")
	case *ast.ValueSpec:
		if n.Doc != nil {
			return strings.TrimSpace(n.Doc.Text())
		}
	}
	return ""
}

// getFieldTag extracts the tag from a struct field
func getFieldTag(field *ast.Field) string {
	if field == nil || field.Tag == nil {
		return ""
	}
	// Remove quotes from the tag
	tag := field.Tag.Value
	if len(tag) >= 2 && tag[0] == '`' && tag[len(tag)-1] == '`' {
		return tag[1 : len(tag)-1]
	}
	if len(tag) >= 2 && tag[0] == '"' && tag[len(tag)-1] == '"' {
		return tag[1 : len(tag)-1]
	}
	return tag
}

// isExported checks if a name is exported (starts with uppercase)
func isExported(name string) bool {
	if len(name) == 0 {
		return false
	}
	return name[0] >= 'A' && name[0] <= 'Z'
}

// getScope determines the scope of a name
func getScope(name string) string {
	if isExported(name) {
		return "exported"
	}
	return "unexported"
}

// formatSignature creates a readable function signature string
func formatSignature(fn *ast.FuncType) string {
	if fn == nil {
		return "func()"
	}

	var params []string
	if fn.Params != nil {
		for _, field := range fn.Params.List {
			typeStr := getTypeName(field.Type)
			if len(field.Names) == 0 {
				params = append(params, typeStr)
			} else {
				for range field.Names {
					params = append(params, typeStr)
				}
			}
		}
	}

	var results []string
	if fn.Results != nil {
		for _, field := range fn.Results.List {
			typeStr := getTypeName(field.Type)
			if len(field.Names) == 0 {
				results = append(results, typeStr)
			} else {
				for range field.Names {
					results = append(results, typeStr)
				}
			}
		}
	}

	signature := fmt.Sprintf("func(%s)", strings.Join(params, ", "))
	if len(results) > 0 {
		if len(results) == 1 {
			signature += " " + results[0]
		} else {
			signature += fmt.Sprintf(" (%s)", strings.Join(results, ", "))
		}
	}

	return signature
}

// getImportPath extracts the import path from an import spec
func getImportPath(imp *ast.ImportSpec) string {
	if imp == nil || imp.Path == nil {
		return ""
	}
	return strings.Trim(imp.Path.Value, `"`)
}

// getImportAlias extracts the alias from an import spec
func getImportAlias(imp *ast.ImportSpec) string {
	if imp == nil || imp.Name == nil {
		return ""
	}
	return imp.Name.Name
}
