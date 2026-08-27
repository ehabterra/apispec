// Copyright 2025 Ehab Terra
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package metadata

import (
	"fmt"
	"go/ast"
	"go/types"
	"strings"

	"github.com/ehabterra/apispec/internal/typemodel"
)

const (
	defaultScopeExported   = "exported"
	defaultScopeUnexported = "unexported"
)

// getTypeName extracts a type name from an AST node. Type expressions render
// through the structured type model (typemodel.FromExpr), so every layer
// parses and prints types through one code path; only the two non-expression
// node shapes stay here:
//
//   - *ast.TypeSpec renders with its declared parameter-NAME list (Page[T]) —
//     the methods-table key convention, matching how a generic method
//     receiver expression renders (allTypeMethods is keyed by both).
//   - *ast.FieldList renders that bracketed name list.
//
// Note the Types map itself is keyed by the bare tspec.Name.Name, not this
// bracketed form.
func getTypeName(nd ast.Node, info *types.Info) string {
	switch t := nd.(type) {
	case nil:
		return ""
	case *ast.TypeSpec:
		var list string

		if t.TypeParams != nil {
			list = getTypeName(t.TypeParams, info)
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
	}
	if e, ok := nd.(ast.Expr); ok {
		return typemodel.FromExpr(e, info).String()
	}
	return ""
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
	case *ast.TypeSpec:
		// Only the GROUPED form carries its doc here — `type ( // C \n T struct{} )`.
		// For the ordinary `// C \n type T struct{}` the parser attaches the
		// comment to the enclosing GenDecl and leaves this nil, which is why
		// typeComments consults both.
		if n.Doc != nil {
			return strings.TrimSpace(n.Doc.Text())
		}
		if n.Comment != nil {
			return strings.TrimSpace(n.Comment.Text())
		}
	}
	return ""
}

// typeComments returns the doc comment of a type declaration.
//
// Go's parser attaches the comment to whichever node "owns" it, and that
// differs between the two ways of writing the same declaration: for
// `// C` + `type T struct{}` it lands on the GenDecl, while inside a
// `type ( ... )` group it lands on the TypeSpec. Reading only one of them
// records nothing for the common form, which is why every generated schema was
// undocumented (issue #366).
func typeComments(gd *ast.GenDecl, tspec *ast.TypeSpec) string {
	if c := getComments(tspec); c != "" {
		return c
	}
	// A grouped declaration's own doc describes the GROUP, not any one type, so
	// it is not borrowed for a spec that has none of its own.
	if gd != nil && len(gd.Specs) == 1 {
		return getComments(gd)
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
