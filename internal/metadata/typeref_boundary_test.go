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
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"testing"

	"github.com/ehabterra/apispec/internal/typemodel"
)

// TestBoundaryTypeNames characterizes the AST-boundary type rendering.
// getTypeName's expression cases delegate to typemodel.FromExpr (phase 3 of
// docs/TYPE_MODEL.md), so this pins the exact string every shape records into
// metadata — including the two shapes the pre-typemodel stringifier lost:
// array lengths ([4]byte used to record "[]byte") and multi-argument generic
// instantiations (IndexListExpr used to record "").
func TestBoundaryTypeNames(t *testing.T) {
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

	want := map[string]string{
		"a":  "int",
		"b":  "*string",
		"c":  "[]Local",
		"d":  "map[string][]*int",
		"e":  "chan int",
		"f":  "chan<- int",
		"g":  "<-chan int",
		"h":  "interface{}",
		"i":  "struct{}",
		"j":  "func",
		"k":  "Box[int]",
		"l":  "Pair[int, string]",
		"m":  "[4]byte",
		"n":  "net/http.Handler",
		"o":  "*net/http.Request",
		"p2": "map[Local][]Box[Local]",
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
			expected, known := want[name]
			if !known {
				t.Errorf("var %s: no expectation in the table; add one", name)
				continue
			}
			if got := getTypeName(vspec.Type, info); got != expected {
				t.Errorf("var %s: getTypeName = %q, want %q", name, got, expected)
			}
			// The delegation invariant: getTypeName IS the structured render.
			if got, direct := getTypeName(vspec.Type, info), typemodel.FromExpr(vspec.Type, info).String(); got != direct {
				t.Errorf("var %s: getTypeName = %q but FromExpr.String() = %q — delegation broken", name, got, direct)
			}
			checked++
		}
	}
	if checked != len(want) {
		t.Fatalf("checked %d vars, want table has %d; corpus and table drifted", checked, len(want))
	}
}
