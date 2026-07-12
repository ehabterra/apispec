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

package typemodel

import (
	"reflect"
	"testing"
)

// TestParse_Structure locks in the structured decomposition of every string
// encoding in use: internal Sep form, dotted go/types form, wrapper syntax,
// and generic argument lists (nested, multi-argument, declaration-form).
func TestParse_Structure(t *testing.T) {
	tests := []struct {
		input    string
		kind     Kind
		corePkg  string
		coreName string
		coreArgs int
	}{
		{"string", KindNamed, "", "string", 0},
		{"main-->User", KindNamed, "main", "User", 0},
		{"github.com/x/y.User", KindNamed, "github.com/x/y", "User", 0},
		{"*main-->User", KindPointer, "main", "User", 0},
		{"[]*github.com/x/y.User", KindSlice, "github.com/x/y", "User", 0},
		{"[4]byte", KindArray, "", "byte", 0},
		{"map[string][]*pkg.User", KindMap, "", "", 0},
		{"chan int", KindChan, "", "int", 0},
		{"chan<- int", KindChan, "", "int", 0},
		{"<-chan int", KindChan, "", "int", 0},
		{"pkg-->Page[User]", KindNamed, "pkg", "Page", 1},
		{"pkg-->Pair[User, Product]", KindNamed, "pkg", "Pair", 2},
		{"pkg.Envelope[pkg.Page[pkg.User]]", KindNamed, "pkg", "Envelope", 1},
		{"pkg-->Type-->T", KindNamed, "pkg", "Type", 1}, // legacy arg encoding
		{"Page[T any]", KindNamed, "", "Page", 1},
		{"interface{}", KindNamed, "", "interface{}", 0},
		{"Container[T]", KindNamed, "", "Container", 1},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			ref := Parse(tt.input)
			if ref.Kind != tt.kind {
				t.Fatalf("Kind = %d, want %d", ref.Kind, tt.kind)
			}
			core := ref.Core()
			if tt.kind == KindMap {
				return // a map has no single core; checked separately below
			}
			if core.Pkg != tt.corePkg || core.Name != tt.coreName || len(core.Args) != tt.coreArgs {
				t.Errorf("core = {Pkg:%q Name:%q Args:%d}, want {Pkg:%q Name:%q Args:%d}",
					core.Pkg, core.Name, len(core.Args), tt.corePkg, tt.coreName, tt.coreArgs)
			}
		})
	}
}

// TestParse_MapAndDecl covers structure that the flat table above can't
// express: map key/value decomposition and declaration-form parameters.
func TestParse_MapAndDecl(t *testing.T) {
	m := Parse("map[string][]*pkg.User")
	if m.Kind != KindMap || m.Key.Name != "string" {
		t.Fatalf("map key = %+v", m.Key)
	}
	if m.Elem.Kind != KindSlice || m.Elem.Elem.Kind != KindPointer {
		t.Fatalf("map value wrappers = %+v", m.Elem)
	}
	if core := m.Elem.Core(); core.Pkg != "pkg" || core.Name != "User" {
		t.Fatalf("map value core = %+v", core)
	}

	d := Parse("Page[T any, U comparable]")
	if len(d.Args) != 2 {
		t.Fatalf("args = %d, want 2", len(d.Args))
	}
	if d.Args[0].Name != "T" || d.Args[0].Constraint != "any" {
		t.Errorf("arg0 = %+v, want T any", d.Args[0])
	}
	if d.Args[1].Name != "U" || d.Args[1].Constraint != "comparable" {
		t.Errorf("arg1 = %+v, want U comparable", d.Args[1])
	}
}

// TestRender covers the three renderers, including the forms that motivated
// the structured model: wrapped generics and qualifier normalization.
func TestRender(t *testing.T) {
	tests := []struct {
		input    string
		dotted   string
		internal string
		simple   string
	}{
		{"string", "string", "string", "string"},
		{"main-->User", "main.User", "main-->User", "User"},
		{"github.com/x/y.User", "github.com/x/y.User", "github.com/x/y-->User", "User"},
		{"*pkg.User", "*pkg.User", "*pkg-->User", "*User"},
		{"[]*pkg.User", "[]*pkg.User", "[]*pkg-->User", "[]*User"},
		{"map[string]pkg.User", "map[string]pkg.User", "map[string]pkg-->User", "map[string]User"},
		{"pkg.Envelope[pkg.Product]", "pkg.Envelope[pkg.Product]", "pkg-->Envelope[Product]", "Envelope[Product]"},
		{"pkg.Pair[pkg.User, pkg.Product]", "pkg.Pair[pkg.User, pkg.Product]", "pkg-->Pair[User, Product]", "Pair[User, Product]"},
		{"pkg.Envelope[pkg.Page[pkg.User]]", "pkg.Envelope[pkg.Page[pkg.User]]", "pkg-->Envelope[Page[User]]", "Envelope[Page[User]]"},
		// Wrapped generic instantiation — the legacy string views mangled this.
		{"[]pkg.Page[pkg.User]", "[]pkg.Page[pkg.User]", "[]pkg-->Page[User]", "[]Page[User]"},
		{"interface{}", "interface{}", "interface{}", "any"},
		{"Page[interface{}]", "Page[interface{}]", "Page[any]", "Page[any]"},
		{"Page[T any]", "Page[T any]", "Page[T any]", "Page[T any]"},
		{"chan<- int", "chan<- int", "chan<- int", "chan<- int"},
		{"[4]byte", "[4]byte", "[4]byte", "[4]byte"},
		// An array length is a constant expression and may itself contain
		// brackets; the matching close bracket ends the length.
		{"[len([3]int{})]byte", "[len([3]int{})]byte", "[len([3]int{})]byte", "[len([3]int{})]byte"},
		// A function-type argument is one argument, commas and all.
		{"Box[func(int, string)]", "Box[func(int, string)]", "Box[func(int, string)]", "Box[func(int, string)]"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			ref := Parse(tt.input)
			if got := ref.String(); got != tt.dotted {
				t.Errorf("String() = %q, want %q", got, tt.dotted)
			}
			if got := ref.Internal(); got != tt.internal {
				t.Errorf("Internal() = %q, want %q", got, tt.internal)
			}
			if got := ref.Simple(); got != tt.simple {
				t.Errorf("Simple() = %q, want %q", got, tt.simple)
			}
		})
	}
}

// TestCanonicalize covers the component-key canonicalizer. The first block
// mirrors the historical normalize table (behavior unchanged); the second
// covers what the structured model fixes: instantiations wrapped in
// pointer/slice constructors, which the legacy string view mangled.
func TestCanonicalize(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		// go/types dotted form (inferred) -> internal form, clean args.
		{"pkg.Envelope[pkg.Product]", "pkg-->Envelope[Product]"},
		{"m/x.Page[m/x.User]", "m/x-->Page[User]"},
		{"pkg.Pair[pkg.User, pkg.Product]", "pkg-->Pair[User, Product]"},
		{"pkg.Envelope[pkg.Page[pkg.User]]", "pkg-->Envelope[Page[User]]"},
		// Unqualified base borrows the first qualified argument's package.
		{"Page[github.com/acme/svc.User]", "github.com/acme/svc-->Page[User]"},
		{"Page[pkg.User]", "pkg-->Page[User]"},
		// Already-internal, non-generic, and bare-unqualified pass through.
		{"pkg-->Envelope[User]", "pkg-->Envelope[User]"},
		{"pkg.User", "pkg.User"},
		{"Container[T]", "Container[T]"},
		{"string", "string"},

		// Structured improvements over the legacy view:
		{"[]pkg.Page[pkg.User]", "[]pkg-->Page[User]"},
		{"*pkg.Page[pkg.User]", "*pkg-->Page[User]"},
		{"[]*m/x.Envelope[m/x.Page[m/x.User]]", "[]*m/x-->Envelope[Page[User]]"},
		// Maps are not component-key candidates: pass through untouched.
		{"map[string]pkg.Page[pkg.User]", "map[string]pkg.Page[pkg.User]"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := Canonicalize(tt.input); got != tt.want {
				t.Errorf("Canonicalize(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestRoundTrip: well-formed dotted input survives Parse+String unchanged,
// and internal-form input survives Parse+Internal unchanged — the property
// that makes TypeRef safe to thread through code that today passes strings.
func TestRoundTrip(t *testing.T) {
	dotted := []string{
		"string", "*int", "[]byte", "[8]byte",
		"pkg.User", "*pkg.User", "[]*pkg.User",
		"map[string]pkg.User", "map[pkg.Key][]*pkg.Val",
		"chan int", "chan<- pkg.Event", "<-chan pkg.Event",
		"pkg.Page[pkg.User]", "pkg.Pair[pkg.User, pkg.Product]",
		"pkg.Envelope[pkg.Page[pkg.User]]",
		"github.com/acme/svc.User",
	}
	for _, s := range dotted {
		if got := Parse(s).String(); got != s {
			t.Errorf("Parse(%q).String() = %q; not a round-trip", s, got)
		}
	}
	internal := []string{
		"main-->User", "*main-->User", "[]main-->User",
		"pkg-->Page[User]", "pkg-->Pair[User, Product]",
		"pkg-->Envelope[Page[User]]",
	}
	for _, s := range internal {
		if got := Parse(s).Internal(); got != s {
			t.Errorf("Parse(%q).Internal() = %q; not a round-trip", s, got)
		}
	}
}

// TestParse_OpaqueFallbacks: junk and half-formed input must never panic and
// must render back verbatim (dotted mode), so a failed parse can't corrupt a
// pipeline that today just carries the string through.
func TestParse_OpaqueFallbacks(t *testing.T) {
	for _, s := range []string{
		"", "Container[T", "map[string", "func", "struct{}", "call(...)",
	} {
		ref := Parse(s)
		if got := ref.String(); got != s {
			t.Errorf("Parse(%q).String() = %q, want verbatim", s, got)
		}
	}
}

// TestHelpers covers the small predicate/accessor surface.
func TestHelpers(t *testing.T) {
	ref := Parse("[]*pkg.Page[pkg.User]")
	if !ref.IsGeneric() || !ref.Qualified() || ref.IsNamed() {
		t.Errorf("predicates = generic:%v qualified:%v named:%v, want true/true/false",
			ref.IsGeneric(), ref.Qualified(), ref.IsNamed())
	}
	if ref.Raw() != "[]*pkg.Page[pkg.User]" {
		t.Errorf("Raw() = %q", ref.Raw())
	}
	if core := ref.Core(); core.Name != "Page" {
		t.Errorf("Core().Name = %q, want Page", core.Name)
	}
	var nilRef *TypeRef
	if nilRef.String() != "" || nilRef.Core() != nil || nilRef.IsGeneric() {
		t.Error("nil receiver must be safe")
	}
	if !reflect.DeepEqual(Parse("  pkg.User  "), Parse("pkg.User")) {
		t.Error("surrounding whitespace must be trimmed")
	}
}
