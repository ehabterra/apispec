package metadata

import (
	"fmt"
	"go/ast"
	"go/token"
	"strings"
)

const (
	defaultScopeExported   = "exported"
	defaultScopeUnexported = "unexported"
)

// getTypeName extracts a type name from an AST expression
func getTypeName(nd ast.Node) string {
	if nd == nil {
		return ""
	}

	switch t := nd.(type) {
	case *ast.TypeSpec:
		var list string

		if t.TypeParams != nil {
			list = getTypeName(t.TypeParams)
		}

		return t.Name.Name + list
	case *ast.FieldList:
		var result []byte
		for _, item := range t.List {
			for _, name := range item.Names {
				result = append(result, []byte(name.Name+", ")...)
			}
		}
		return fmt.Sprintf("[%s]", strings.TrimSuffix(string(result), ", "))
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return "*" + getTypeName(t.X)
	case *ast.IndexExpr:
		return fmt.Sprintf("%s[%s]", getTypeName(t.X), getTypeName(t.Index))
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
		// For nested struct types, we'll return a placeholder
		// The actual structure will be captured in the Field.NestedType
		return "struct{}"
	case *ast.FuncType:
		return "func"
	case *ast.ChanType:
		switch t.Dir {
		case ast.SEND:
			return "chan<- " + getTypeName(t.Value)
		case ast.RECV:
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
		return defaultScopeExported
	}
	return defaultScopeUnexported
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
