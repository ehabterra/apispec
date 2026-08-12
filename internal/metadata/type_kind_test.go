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

// TestProcessTypeKindRecordsUnderlyingType covers what a declaration is
// recorded as. The kinds were already right; what was missing is the TARGET for
// anything that is not a bare ident, without which a consumer has nothing to
// build a schema from and every named container looks like an opaque object
// (issue #333).
func TestProcessTypeKindRecordsUnderlyingType(t *testing.T) {
	const src = `package p

type (
	Point   [2]int64
	IDs     []string
	Lookup  map[string]int
	Count   int
	Nested  []Point
	Ptr     *Point
	Ch      chan int
	Handler func(int) error
	Rec     struct{ A string }
	Iface   interface{ M() }
)
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "p.go", src, 0)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	meta := &Metadata{StringPool: NewStringPool()}
	// Target is a pooled index whose zero value doubles as "unset" (yaml
	// omitempty), and index 0 is a real string. Burn index 0 on a sentinel so
	// an unset Target cannot be mistaken for a recorded one below.
	_ = meta.StringPool.Get("<unset-sentinel>")

	got := map[string]struct{ kind, target string }{}

	for _, decl := range file.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, s := range gd.Specs {
			tspec, ok := s.(*ast.TypeSpec)
			if !ok {
				continue
			}
			typ := &Type{Name: meta.StringPool.Get(tspec.Name.Name)}
			processTypeKind(tspec, nil, "p", fset, typ, map[string]*Type{}, meta)

			target := ""
			if typ.Target != 0 {
				target = meta.StringPool.GetString(typ.Target)
			}
			got[tspec.Name.Name] = struct{ kind, target string }{
				kind:   meta.StringPool.GetString(typ.Kind),
				target: target,
			}
		}
	}

	want := map[string]struct{ kind, target string }{
		// The three the type switch names explicitly.
		"Rec":   {"struct", ""},
		"Iface": {"interface", ""},
		"Count": {"alias", "int"},

		// Everything else is "other" — they are none of the three above, and
		// calling them aliases would misdescribe defined types — but the
		// underlying type must be recorded.
		"Point":  {"other", "[2]int64"},
		"IDs":    {"other", "[]string"},
		"Lookup": {"other", "map[string]int"},
		"Nested": {"other", "[]Point"},
		"Ptr":    {"other", "*Point"},
		"Ch":     {"other", "chan int"},
		// getTypeName renders a func type as bare "func". Recording that is
		// still better than recording nothing — the kind is right and the
		// mapper has something to dispatch on — but it is why a func-typed
		// declaration cannot produce a useful schema.
		"Handler": {"other", "func"},
	}

	for name, w := range want {
		g, ok := got[name]
		if !ok {
			t.Errorf("%s: not recorded", name)
			continue
		}
		if g.kind != w.kind {
			t.Errorf("%s: kind = %q, want %q", name, g.kind, w.kind)
		}
		if g.target != w.target {
			t.Errorf("%s: target = %q, want %q", name, g.target, w.target)
		}
	}

	// The target of a named container is what the declaration says, verbatim.
	// Consumers qualify it themselves — metadata records the fact, it does not
	// decide how the name resolves (golden rule #4).
	if g := got["Nested"]; g.target != "[]Point" {
		t.Errorf("Nested target = %q; it should be recorded as written, unqualified", g.target)
	}
}
