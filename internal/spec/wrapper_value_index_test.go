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

// valueIndexOf maps a VALUE role — a response body, a decode destination —
// only to an argument that IS a parameter, where indexOf settles for one that
// mentions it. `http.Error(w, "bad "+p, 400)` with `p := req.URL.Path` mentions
// `req`; mapping its body there documented *http.Request as the response of
// every caller (issue #659).
//
// The expressions are built by hand because the kinds that matter — a type
// conversion above all — are only told apart with type information; the
// locals they name come from parsed source so the assignment hops are real.
func TestParamIndexValueIndexOf(t *testing.T) {
	src := `package p

type R struct{}

func (r *R) M(w W, v any, code int, req *Req) {
	alias := v
	twice := alias
	both := v
	both = code
	same := v
	same = alias
	built := req.URL
	mixed := v
	mixed = "fallback"
	loopA := loopB
	loopB := loopA
}
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

	arg := func(kind, name string) *metadata.CallArgument {
		a := metadata.NewCallArgument(meta)
		a.SetKind(kind)
		if name != "" {
			a.SetName(name)
		}
		return a
	}
	ident := func(name string) *metadata.CallArgument { return arg(metadata.KindIdent, name) }
	wrap := func(kind, value string, x *metadata.CallArgument) *metadata.CallArgument {
		a := arg(kind, "")
		if value != "" {
			a.SetValue(value)
		}
		a.X = x
		return a
	}
	selector := func(x *metadata.CallArgument, sel string) *metadata.CallArgument {
		a := wrap(metadata.KindSelector, "", x)
		a.Sel = ident(sel)
		return a
	}
	call := func(kind string, fun *metadata.CallArgument, args ...*metadata.CallArgument) *metadata.CallArgument {
		a := arg(kind, "")
		a.Fun = fun
		a.Args = args
		return a
	}
	literal := arg(metadata.KindLiteral, "")
	literal.SetValue(`"bad "`)
	concat := wrap(metadata.KindBinary, "+", literal)
	concat.Fun = ident("v")
	assert := wrap(metadata.KindTypeAssert, "", ident("v"))
	assert.Fun = ident("T")

	const (
		v    = 1
		code = 2
		none = -1
	)
	cases := []struct {
		name string
		arg  *metadata.CallArgument
		want int
	}{
		{"the parameter itself", ident("v"), v},
		{"another parameter", ident("code"), code},
		{"its address", wrap(metadata.KindUnary, "&", ident("v")), v},
		{"its dereference", wrap(metadata.KindStar, "", ident("v")), v},
		{"parenthesised", wrap(metadata.KindParen, "", ident("v")), v},
		{"a type assertion of it", assert, v},
		{"a conversion of it", call(metadata.KindTypeConversion, ident("string"), ident("v")), v},
		{"through a local", ident("alias"), v},
		{"through two locals, by address", wrap(metadata.KindUnary, "&", ident("twice")), v},
		{"every assignment the same parameter", ident("same"), v},

		{"a field of it", selector(ident("req"), "URL"), none},
		{"a local holding a field of it", ident("built"), none},
		{"a string built from it", concat, none},
		{"a call over it", call(metadata.KindCall, ident("sink"), ident("code")), none},
		{"a negation is a new value", wrap(metadata.KindUnary, "-", ident("code")), none},
		{"an index into it", wrap(metadata.KindIndex, "", ident("v")), none},
		{"assigned two different parameters", ident("both"), none},
		{"one assignment builds its own value", ident("mixed"), none},
		{"a cycle of locals", ident("loopA"), none},
		{"a name the method never binds", ident("ghost"), none},
		{"no argument", nil, none},
	}
	idx := newParamIndex(meta)
	w := &wrapperMethod{pkg: "p", recvType: "*R", name: "M"}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := idx.valueIndexOf(w, tc.arg)
			if tc.want == none {
				if ok {
					t.Errorf("valueIndexOf mapped to parameter %d, want no mapping", got)
				}
				return
			}
			if !ok || got != tc.want {
				t.Errorf("valueIndexOf = (%d, %v), want (%d, true)", got, ok, tc.want)
			}
		})
	}

	// The roles that stay permissive still see through what valueIndexOf
	// rejects: indexOf maps the built string to the parameter it mentions.
	if got, ok := idx.indexOf(w, concat); !ok || got != v {
		t.Errorf("indexOf(\"bad \"+v) = (%d, %v), want (%d, true) — the path and handler roles rely on it", got, ok, v)
	}
	// A method that does not exist has nothing to map to.
	if _, ok := idx.valueIndexOf(&wrapperMethod{pkg: "p", recvType: "*R", name: "Nope"}, ident("v")); ok {
		t.Error("an unknown method mapped a value role")
	}
}
