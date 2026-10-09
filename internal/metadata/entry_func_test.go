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
	"testing"
)

// TestIsEntryFunc pins which functions run with no caller (issue #580): main,
// and a package init — but not a METHOD named init, which is called like any
// other function and gets its assignments recorded through that call.
func TestIsEntryFunc(t *testing.T) {
	cases := []struct {
		name, recv string
		want       bool
	}{
		{"main", "", true},
		{"init", "", true},
		{"init#2", "", true},
		{"init", "*Server", false},
		{"init#2", "*Server", false},
		{"setup", "", false},
		{"Init", "", false},
	}
	for _, tc := range cases {
		if got := isEntryFunc(tc.name, tc.recv); got != tc.want {
			t.Errorf("isEntryFunc(%q, %q) = %v, want %v", tc.name, tc.recv, got, tc.want)
		}
	}
}

// TestBuildInitNames pins the init numbering: per package, in sorted file order
// then declaration order, the first keeping the plain name (review of #583).
// Two inits in one file, a third in a later file, a method named init (not a
// package init) and a second package whose numbering starts over.
func TestBuildInitNames(t *testing.T) {
	fset := token.NewFileSet()
	parse := func(name, src string) *ast.File {
		f, err := parser.ParseFile(fset, name, src, 0)
		if err != nil {
			t.Fatal(err)
		}
		return f
	}
	a := parse("a.go", "package p\ntype T struct{}\nfunc (T) init() {}\nfunc init() {}\nfunc init() {}\n")
	b := parse("b.go", "package p\nfunc init() {}\n")
	q := parse("q.go", "package q\nfunc init() {}\n")
	pkgs := map[string]map[string]*ast.File{
		"p": {"b.go": b, "a.go": a},
		"q": {"q.go": q},
	}
	m := &Metadata{initNames: buildInitNames(pkgs)}

	var got []string
	for _, f := range []*ast.File{a, b, q} {
		for _, d := range f.Decls {
			if fn, ok := d.(*ast.FuncDecl); ok {
				got = append(got, m.declName(fn))
			}
		}
	}
	want := []string{"init", "init", "init#1", "init#2", "init"}
	if len(got) != len(want) {
		t.Fatalf("names = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("names = %v, want %v", got, want)
			break
		}
	}
}
