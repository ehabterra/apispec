// Copyright 2026 Ehab Terra
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
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

const covmetaHelperSrc = `package p

// FooDoc is the doc for Foo.
func Foo() {}

// GenDoc documents the type block.
type Bar struct {
	// FieldDoc documents ID.
	ID int ` + "`json:\"id\"`" + ` // FieldLine trails ID.
	Untagged string
}

var (
	// ValDoc documents the value.
	X = 1
)
`

// covmetaParse parses a standalone Go source file for helper-node tests.
func covmetaParse(t *testing.T, src string) (*ast.File, *token.FileSet) {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "p.go", src, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return file, fset
}

// covmetaFindNodes walks a file collecting the AST nodes the helper tests need.
func covmetaFindNodes(file *ast.File) (fn *ast.FuncDecl, typeDecl *ast.GenDecl, varSpec *ast.ValueSpec, taggedField, untaggedField *ast.Field) {
	ast.Inspect(file, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.FuncDecl:
			if node.Name.Name == "Foo" {
				fn = node
			}
		case *ast.GenDecl:
			if node.Tok == token.TYPE {
				typeDecl = node
			}
		case *ast.ValueSpec:
			varSpec = node
		case *ast.Field:
			if len(node.Names) == 1 {
				switch node.Names[0].Name {
				case "ID":
					taggedField = node
				case "Untagged":
					untaggedField = node
				}
			}
		}
		return true
	})
	return
}

func TestCovmetaGetComments(t *testing.T) {
	file, _ := covmetaParse(t, covmetaHelperSrc)
	fn, typeDecl, varSpec, taggedField, _ := covmetaFindNodes(file)

	if got := getComments(fn); got != "FooDoc is the doc for Foo." {
		t.Errorf("FuncDecl doc = %q", got)
	}
	if got := getComments(typeDecl); got != "GenDoc documents the type block." {
		t.Errorf("GenDecl doc = %q", got)
	}
	if got := getComments(varSpec); got != "ValDoc documents the value." {
		t.Errorf("ValueSpec doc = %q", got)
	}
	// Field carries both a leading doc and a trailing line comment.
	got := getComments(taggedField)
	if !strings.Contains(got, "FieldDoc documents ID.") || !strings.Contains(got, "FieldLine trails ID.") {
		t.Errorf("Field comments = %q", got)
	}
	// nil node and an unhandled node type both yield the empty string.
	if getComments(nil) != "" {
		t.Errorf("nil node should have no comments")
	}
	if getComments(&ast.Ident{Name: "x"}) != "" {
		t.Errorf("unhandled node type should have no comments")
	}
}

func TestCovmetaGetFieldTag(t *testing.T) {
	file, _ := covmetaParse(t, covmetaHelperSrc)
	_, _, _, taggedField, untaggedField := covmetaFindNodes(file)

	if got := getFieldTag(taggedField); got != `json:"id"` {
		t.Errorf("backtick tag = %q", got)
	}
	if got := getFieldTag(untaggedField); got != "" {
		t.Errorf("field without tag should be empty, got %q", got)
	}
	if got := getFieldTag(nil); got != "" {
		t.Errorf("nil field should be empty, got %q", got)
	}
	// A double-quoted tag literal is unwrapped by the quote branch.
	quoted := &ast.Field{Tag: &ast.BasicLit{Kind: token.STRING, Value: `"json:\"q\""`}}
	if got := getFieldTag(quoted); got != `json:\"q\"` {
		t.Errorf("quoted tag = %q", got)
	}
	// A tag literal wrapped in neither backtick nor quote falls through
	// unchanged (the parser never emits this, but the branch exists).
	raw := &ast.Field{Tag: &ast.BasicLit{Kind: token.STRING, Value: "raw"}}
	if got := getFieldTag(raw); got != "raw" {
		t.Errorf("raw tag = %q", got)
	}
}

func TestCovmetaIsExported(t *testing.T) {
	cases := map[string]bool{
		"Exported":   true,
		"unexported": false,
		"":           false,
		"_private":   false,
	}
	for name, want := range cases {
		if got := isExported(name); got != want {
			t.Errorf("isExported(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestCovmetaGetImportPath(t *testing.T) {
	file, _ := covmetaParse(t, "package p\n\nimport foo \"example.com/bar\"\n")
	var imp *ast.ImportSpec
	ast.Inspect(file, func(n ast.Node) bool {
		if is, ok := n.(*ast.ImportSpec); ok {
			imp = is
		}
		return true
	})
	if got := getImportPath(imp); got != "example.com/bar" {
		t.Errorf("import path = %q", got)
	}
	if got := getImportAlias(imp); got != "foo" {
		t.Errorf("import alias = %q", got)
	}
	if got := getImportPath(nil); got != "" {
		t.Errorf("nil import path should be empty, got %q", got)
	}
	// A spec with a nil Path yields the empty string.
	if got := getImportPath(&ast.ImportSpec{}); got != "" {
		t.Errorf("import with nil Path should be empty, got %q", got)
	}
}

func TestCovmetaGetPositions(t *testing.T) {
	file, fset := covmetaParse(t, covmetaHelperSrc)
	fn, _, varSpec, _, _ := covmetaFindNodes(file)

	if got := getFuncPosition(fn, fset); got == "" {
		t.Error("func position should be non-empty")
	}
	if got := getFuncPosition(nil, fset); got != "" {
		t.Errorf("nil func position should be empty, got %q", got)
	}

	ident := varSpec.Names[0]
	if got := getVarPosition(ident, fset); got == "" {
		t.Error("var position should be non-empty")
	}
	if got := getVarPosition(nil, fset); got != "" {
		t.Errorf("nil var position should be empty, got %q", got)
	}
}

// TestCovmetaExtractParamsInference exercises the inferred-type-argument
// branch of extractParamsAndTypeParams that reads type args from
// info.Instances (uncovered by the synthetic sweep tests, which leave
// Instances empty), plus the IndexExpr ident-base and bare-SelectorExpr
// funcObj-resolution branches.
func TestCovmetaExtractParamsInference(t *testing.T) {
	src := `package p

import "fmt"

func G[T any](x T)     {}
func GG[T, U any](x T, y U) {}

func h(n int) {
	G(n)
	G[string]("a")
	GG(n, "b")
	fmt.Println(n)
}
`
	file, info, _ := sweepTypeCheck(t, src)
	m := sweepMeta()

	// Collect the call expressions from the type-checked AST so their
	// identifiers key into info.Instances/Uses.
	var calls []*ast.CallExpr
	ast.Inspect(file, func(nn ast.Node) bool {
		if c, ok := nn.(*ast.CallExpr); ok {
			calls = append(calls, c)
		}
		return true
	})

	byName := func(c *ast.CallExpr) string {
		switch fun := c.Fun.(type) {
		case *ast.Ident:
			return fun.Name
		case *ast.IndexExpr:
			if id, ok := fun.X.(*ast.Ident); ok {
				return id.Name
			}
		case *ast.SelectorExpr:
			return fun.Sel.Name
		}
		return ""
	}

	got := map[string]map[string]string{}
	for _, c := range calls {
		tpm := map[string]string{}
		args := make([]*CallArgument, len(c.Args))
		for i := range args {
			args[i] = arg(m, KindIdent)
		}
		extractParamsAndTypeParams(c, info, args, map[string]CallArgument{}, tpm)
		got[byName(c)] = tpm
	}

	if got["G"]["T"] == "" {
		t.Errorf("G call: expected inferred type param, got %v", got["G"])
	}
	if got["GG"]["T"] == "" || got["GG"]["U"] == "" {
		t.Errorf("GG call: expected two inferred type params, got %v", got["GG"])
	}
	// fmt.Println resolves a funcObj but has no type params; it must not panic
	// and must add no type-param entries.
	if len(got["Println"]) != 0 {
		t.Errorf("Println should yield no type params, got %v", got["Println"])
	}
}

func TestCovmetaKeyStrings(t *testing.T) {
	ak := AssignmentKey{Name: "n", Pkg: "pkg", Type: "T", Container: "c"}
	if got := ak.String(); got != "pkgTnc" {
		t.Errorf("AssignmentKey.String() = %q, want %q", got, "pkgTnc")
	}
	ik := InterfaceResolutionKey{InterfaceType: "I", StructType: "S", Pkg: "pkg"}
	if got := ik.String(); got != "pkgSI" {
		t.Errorf("InterfaceResolutionKey.String() = %q, want %q", got, "pkgSI")
	}
}
