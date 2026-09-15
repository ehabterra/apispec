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

package spec

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	"github.com/ehabterra/apispec/internal/metadata"
)

// `...T` and `[]T` are a different fact, and issue #416 turned on whether
// metadata keeps them apart. It does — the parameter's CallArgument carries
// KindEllipsis where a slice parameter carries KindArrayType — which is what
// made the fix possible without the type-model change the issue anticipated.
//
// Pinned at this level because it is a metadata guarantee the derivation now
// depends on: if the ellipsis kind were ever folded into the slice kind, the
// wrapper would silently go back to attributing operations to middleware.
func TestParamIndexIsVariadic(t *testing.T) {
	src := `package p

type H func()

type R struct{}

// Variadic: the chain, with the handler last.
func (r *R) Get(path string, handlers ...H) {}

// A genuine slice parameter, which is NOT variadic.
func (r *R) Post(path string, handlers []H) {}

// No chain at all.
func (r *R) Head(path string, handler H) {}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "p.go", src, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	meta := metadata.GenerateMetadata(
		map[string]map[string]*ast.File{"p": {"p.go": file}},
		nil, map[string]string{"p": "p"}, fset,
	)

	idx := newParamIndex(meta)
	cases := []struct {
		method string
		pos    int
		want   bool
	}{
		{"Get", 1, true},   // ...H
		{"Get", 0, false},  // the path, alongside it
		{"Post", 1, false}, // []H — the case that must not be confused with it
		{"Head", 1, false}, // a single handler
		// Out of range answers false rather than panicking: a derived pattern
		// can name a position the signature does not have.
		{"Get", 9, false},
		{"Get", -1, false},
	}
	for _, tc := range cases {
		w := &wrapperMethod{pkg: "p", recvType: "*R", name: tc.method}
		if got := idx.isVariadic(w, tc.pos); got != tc.want {
			t.Errorf("isVariadic(%s, %d) = %v, want %v", tc.method, tc.pos, got, tc.want)
		}
	}

	// A method that does not exist has no parameters to report on, and the
	// answer is cached without blowing up.
	missing := &wrapperMethod{pkg: "p", recvType: "*R", name: "Nope"}
	if idx.isVariadic(missing, 0) {
		t.Error("an unknown method reported a variadic parameter")
	}
	if idx.isVariadic(missing, 0) {
		t.Error("the cached answer for an unknown method changed")
	}
}
