package metadata

import (
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"testing"

	"github.com/ehabterra/apispec/internal/typemodel"
)

// TestFromExpr_MatchesGetTypeName is the boundary contract for the structured
// type model: for every type-expression shape the legacy string stringifier
// (getTypeName) understands, typemodel.FromExpr must produce a TypeRef whose
// dotted rendering is byte-identical — so migrating a getTypeName call site to
// FromExpr(...).String() cannot change metadata output. The two documented
// divergences (below) are cases where the flat string *dropped* information
// and the structured form keeps it.
func TestFromExpr_MatchesGetTypeName(t *testing.T) {
	src := `package p

import "net/http"

type Box[T any] struct{ V T }
type Pair[K any, V any] struct{ K K; V V }
type Local struct{ N int }

var (
	a int
	b *string
	c []Local
	d map[string][]*int
	e chan int
	f chan<- int
	g <-chan int
	h interface{}
	i struct{ X int }
	j func(int) string
	k Box[int]
	l Pair[int, string]
	m [4]byte
	n http.Handler
	o *http.Request
	p2 map[Local][]Box[Local]
)
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "p.go", src, 0)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	info := &types.Info{
		Types: map[ast.Expr]types.TypeAndValue{},
		Defs:  map[*ast.Ident]types.Object{},
		Uses:  map[*ast.Ident]types.Object{},
	}
	conf := types.Config{Importer: importer.Default()}
	if _, err := conf.Check("p", fset, []*ast.File{file}, info); err != nil {
		t.Fatalf("typecheck: %v", err)
	}

	// Vars whose flat-string form loses information; the structured form must
	// keep it instead of matching.
	better := map[string]string{
		// getTypeName renders every array as a slice, dropping the length.
		"m": "[4]byte",
		// getTypeName has no IndexListExpr case at all: a multi-argument
		// generic instantiation stringified to "".
		"l": "Pair[int, string]",
	}

	checked := 0
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.VAR {
			continue
		}
		for _, s := range gen.Specs {
			vspec, ok := s.(*ast.ValueSpec)
			if !ok || vspec.Type == nil {
				continue
			}
			name := vspec.Names[0].Name
			got := typemodel.FromExpr(vspec.Type, info).String()
			if want, diverges := better[name]; diverges {
				if got != want {
					t.Errorf("var %s: FromExpr = %q, want the information-preserving %q", name, got, want)
				}
			} else if legacy := getTypeName(vspec.Type, info); got != legacy {
				t.Errorf("var %s: FromExpr = %q, getTypeName = %q — boundary forms must agree", name, got, legacy)
			}
			checked++
		}
	}
	if checked < 16 {
		t.Fatalf("only %d vars checked; corpus incomplete", checked)
	}
}
