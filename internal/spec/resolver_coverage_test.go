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
	"strings"
	"testing"

	"github.com/ehabterra/apispec/internal/metadata"
)

// covspecArg builds a CallArgument of a given kind with optional name/type/pkg.
func covspecArg(meta *metadata.Metadata, kind, name, typ, pkg string) *metadata.CallArgument {
	a := metadata.NewCallArgument(meta)
	a.SetKind(kind)
	if name != "" {
		a.SetName(name)
	}
	if typ != "" {
		a.SetType(typ)
	}
	if pkg != "" {
		a.SetPkg(pkg)
	}
	return a
}

// TestCovspecCallArgToStringKinds drives callArgToString across the argument
// kinds and sub-branches that existing tests do not reach.
func TestCovspecCallArgToStringKinds(t *testing.T) {
	meta := newTestMeta()
	cp := NewContextProvider(meta)

	t.Run("literal strips quotes", func(t *testing.T) {
		a := covspecArg(meta, metadata.KindLiteral, "", "", "")
		a.SetValue(`"hello"`)
		if got := cp.callArgToString(a, nil); got != "hello" {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("keyvalue empty", func(t *testing.T) {
		a := covspecArg(meta, metadata.KindKeyValue, "", "", "")
		if got := cp.callArgToString(a, nil); got != "" {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("map type", func(t *testing.T) {
		a := covspecArg(meta, metadata.KindMapType, "", "", "")
		a.X = covspecArg(meta, metadata.KindLiteral, "", "", "")
		a.X.SetValue("string")
		a.Fun = covspecArg(meta, metadata.KindLiteral, "", "", "")
		a.Fun.SetValue("int")
		if got := cp.callArgToString(a, nil); got != "map[string]int" {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("map type without children", func(t *testing.T) {
		a := covspecArg(meta, metadata.KindMapType, "", "", "")
		if got := cp.callArgToString(a, nil); got != "map" {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("unary with X", func(t *testing.T) {
		a := covspecArg(meta, metadata.KindUnary, "", "", "")
		a.X = covspecArg(meta, metadata.KindLiteral, "", "", "")
		a.X.SetValue("T")
		if got := cp.callArgToString(a, nil); got != "*T" {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("unary without X", func(t *testing.T) {
		a := covspecArg(meta, metadata.KindUnary, "", "", "")
		if got := cp.callArgToString(a, nil); got != "*" {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("array type with X", func(t *testing.T) {
		a := covspecArg(meta, metadata.KindArrayType, "", "", "")
		a.X = covspecArg(meta, metadata.KindLiteral, "", "", "")
		a.X.SetValue("byte")
		if got := cp.callArgToString(a, nil); got != "[]byte" {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("array type without X", func(t *testing.T) {
		a := covspecArg(meta, metadata.KindArrayType, "", "", "")
		if got := cp.callArgToString(a, nil); got != "[]" {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("index with X", func(t *testing.T) {
		a := covspecArg(meta, metadata.KindIndex, "", "", "")
		a.X = covspecArg(meta, metadata.KindLiteral, "", "", "")
		a.X.SetValue("T")
		if got := cp.callArgToString(a, nil); got != "*T" {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("index without X", func(t *testing.T) {
		a := covspecArg(meta, metadata.KindIndex, "", "", "")
		if got := cp.callArgToString(a, nil); got != "*" {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("composite lit nil X", func(t *testing.T) {
		a := covspecArg(meta, metadata.KindCompositeLit, "", "", "")
		if got := cp.callArgToString(a, nil); got != "" {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("composite lit non-generic X", func(t *testing.T) {
		a := covspecArg(meta, metadata.KindCompositeLit, "", "", "")
		a.X = covspecArg(meta, metadata.KindIdent, "User", "User", "")
		if got := cp.callArgToString(a, nil); got != "User" {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("builtin passthrough", func(t *testing.T) {
		a := covspecArg(meta, metadata.KindIdent, "n", "int", "")
		if got := cp.callArgToString(a, nil); got != "int" {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("custom type with pkg", func(t *testing.T) {
		a := covspecArg(meta, metadata.KindIdent, "u", "mypkg.User", "mypkg")
		got := cp.callArgToString(a, nil)
		if !strings.Contains(got, "User") {
			t.Fatalf("expected qualified User, got %q", got)
		}
	})

	t.Run("slice custom type with pkg", func(t *testing.T) {
		a := covspecArg(meta, metadata.KindIdent, "us", "[]mypkg.User", "mypkg")
		got := cp.callArgToString(a, nil)
		if !strings.HasPrefix(got, "[]") || !strings.Contains(got, "User") {
			t.Fatalf("expected []...User, got %q", got)
		}
	})

	t.Run("ident no pkg with type", func(t *testing.T) {
		a := covspecArg(meta, metadata.KindIdent, "x", "some/nested/Thing", "")
		got := cp.callArgToString(a, nil)
		if got != "some/nested/Thing" {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("ident fallback name no type", func(t *testing.T) {
		a := covspecArg(meta, metadata.KindIdent, "myVar", "", "some/pkg")
		got := cp.callArgToString(a, nil)
		if !strings.Contains(got, "myVar") {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("call with type params", func(t *testing.T) {
		a := covspecArg(meta, metadata.KindCall, "HandleRequest", "", "mypkg")
		a.Fun = covspecArg(meta, metadata.KindIdent, "HandleRequest", "", "mypkg")
		a.TypeParamMap = map[string]string{"B": "Bee", "A": "Ay"}
		got := cp.callArgToString(a, nil)
		if !strings.Contains(got, "[Ay, Bee]") {
			t.Fatalf("expected sorted type args, got %q", got)
		}
	})

	t.Run("call nil fun", func(t *testing.T) {
		a := covspecArg(meta, metadata.KindCall, "", "", "")
		if got := cp.callArgToString(a, nil); got != "call(...)" {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("type conversion", func(t *testing.T) {
		a := covspecArg(meta, metadata.KindTypeConversion, "", "", "")
		a.Fun = covspecArg(meta, metadata.KindLiteral, "", "", "")
		a.Fun.SetValue("[]byte")
		if got := cp.callArgToString(a, nil); got != "[]byte" {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("type conversion nil fun", func(t *testing.T) {
		a := covspecArg(meta, metadata.KindTypeConversion, "", "", "")
		if got := cp.callArgToString(a, nil); got != "" {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("interface type", func(t *testing.T) {
		a := covspecArg(meta, metadata.KindInterfaceType, "", "", "")
		if got := cp.callArgToString(a, nil); got != "interface{}" {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("raw", func(t *testing.T) {
		a := covspecArg(meta, metadata.KindRaw, "", "", "")
		a.SetRaw("rawvalue")
		if got := cp.callArgToString(a, nil); got != "rawvalue" {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("selector fallback", func(t *testing.T) {
		x := covspecArg(meta, metadata.KindIdent, "obj", "", "unknownpkg")
		sel := covspecArg(meta, metadata.KindIdent, "Field", "", "")
		a := covspecArg(meta, metadata.KindSelector, "", "", "")
		a.X = x
		a.Sel = sel
		got := cp.callArgToString(a, nil)
		if !strings.Contains(got, "Field") {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("selector nil X returns sel name", func(t *testing.T) {
		sel := covspecArg(meta, metadata.KindIdent, "Field", "", "")
		a := covspecArg(meta, metadata.KindSelector, "", "", "")
		a.Sel = sel
		if got := cp.callArgToString(a, nil); got != "Field" {
			t.Fatalf("got %q", got)
		}
	})
}

// TestCovspecCallArgToStringConst covers const resolution from package metadata.
func TestCovspecCallArgToStringConst(t *testing.T) {
	meta := newTestMeta()
	sp := meta.StringPool
	variable := &metadata.Variable{
		Name:  sp.Get("MyConst"),
		Tok:   sp.Get("const"),
		Value: sp.Get(`"const-value"`),
	}
	meta.Packages = map[string]*metadata.Package{
		"cpkg": {
			Files: map[string]*metadata.File{
				"a.go": {
					Variables: map[string]*metadata.Variable{"MyConst": variable},
				},
			},
		},
	}
	cp := NewContextProvider(meta)

	a := covspecArg(meta, metadata.KindIdent, "MyConst", "", "cpkg")
	if got := cp.callArgToString(a, nil); got != "const-value" {
		t.Fatalf("expected const-value, got %q", got)
	}
}

// TestCovspecGenericInstantiation drives the composite-literal generic
// instantiation rendering path (genericInstantiationName / genericArgRef /
// genericBaseRef) via callArgToString.
func TestCovspecGenericInstantiation(t *testing.T) {
	meta := newTestMeta()
	cp := NewContextProvider(meta)

	// Page[User]: composite whose X is a KindIndex (base=Page, arg=User).
	t.Run("single type arg", func(t *testing.T) {
		idx := covspecArg(meta, metadata.KindIndex, "", "", "")
		idx.X = covspecArg(meta, metadata.KindIdent, "Page", "Page", "")
		idx.Fun = covspecArg(meta, metadata.KindIdent, "User", "User", "")
		comp := covspecArg(meta, metadata.KindCompositeLit, "", "", "")
		comp.X = idx
		if got := cp.callArgToString(comp, nil); got != "Page[User]" {
			t.Fatalf("got %q", got)
		}
	})

	// Pair[K,V]: composite whose X is a KindIndexList with two args.
	t.Run("index list two args", func(t *testing.T) {
		idxl := covspecArg(meta, metadata.KindIndexList, "", "", "")
		idxl.X = covspecArg(meta, metadata.KindIdent, "Pair", "Pair", "")
		idxl.Args = []*metadata.CallArgument{
			covspecArg(meta, metadata.KindIdent, "K", "K", ""),
			covspecArg(meta, metadata.KindIdent, "V", "V", ""),
		}
		comp := covspecArg(meta, metadata.KindCompositeLit, "", "", "")
		comp.X = idxl
		got := cp.callArgToString(comp, nil)
		if !strings.HasPrefix(got, "Pair[") || !strings.Contains(got, "K") || !strings.Contains(got, "V") {
			t.Fatalf("got %q", got)
		}
	})

	// []Page[User]: composite whose X is KindArrayType wrapping a KindIndex.
	t.Run("slice of instantiation", func(t *testing.T) {
		idx := covspecArg(meta, metadata.KindIndex, "", "", "")
		idx.X = covspecArg(meta, metadata.KindIdent, "Page", "Page", "")
		idx.Fun = covspecArg(meta, metadata.KindIdent, "User", "User", "")
		arr := covspecArg(meta, metadata.KindArrayType, "", "", "")
		arr.X = idx
		comp := covspecArg(meta, metadata.KindCompositeLit, "", "", "")
		comp.X = arr
		if got := cp.callArgToString(comp, nil); got != "[]Page[User]" {
			t.Fatalf("got %q", got)
		}
	})

	// Envelope[Page[User]]: nested generic argument (genericArgRef KindIndex).
	t.Run("nested generic arg", func(t *testing.T) {
		inner := covspecArg(meta, metadata.KindIndex, "", "", "")
		inner.X = covspecArg(meta, metadata.KindIdent, "Page", "Page", "")
		inner.Fun = covspecArg(meta, metadata.KindIdent, "User", "User", "")

		outer := covspecArg(meta, metadata.KindIndex, "", "", "")
		outer.X = covspecArg(meta, metadata.KindIdent, "Envelope", "Envelope", "")
		outer.Fun = inner

		comp := covspecArg(meta, metadata.KindCompositeLit, "", "", "")
		comp.X = outer
		got := cp.callArgToString(comp, nil)
		if !strings.Contains(got, "Envelope[") || !strings.Contains(got, "Page[User]") {
			t.Fatalf("got %q", got)
		}
	})

	// Envelope[Pair[K,V]]: nested generic argument via KindIndexList.
	t.Run("nested index-list arg", func(t *testing.T) {
		innerList := covspecArg(meta, metadata.KindIndexList, "", "", "")
		innerList.X = covspecArg(meta, metadata.KindIdent, "Pair", "Pair", "")
		innerList.Args = []*metadata.CallArgument{
			covspecArg(meta, metadata.KindIdent, "K", "K", ""),
			covspecArg(meta, metadata.KindIdent, "V", "V", ""),
		}

		outer := covspecArg(meta, metadata.KindIndex, "", "", "")
		outer.X = covspecArg(meta, metadata.KindIdent, "Envelope", "Envelope", "")
		outer.Fun = innerList

		comp := covspecArg(meta, metadata.KindCompositeLit, "", "", "")
		comp.X = outer
		got := cp.callArgToString(comp, nil)
		if !strings.Contains(got, "Envelope[") || !strings.Contains(got, "Pair[") {
			t.Fatalf("got %q", got)
		}
	})
}

// TestCovspecResolveGenericTypeExtra covers the ResolveGenericType branches
// that the existing table test does not reach.
func TestCovspecResolveGenericTypeExtra(t *testing.T) {
	meta := &metadata.Metadata{}
	cfg := DefaultAPISpecConfig()
	resolver := NewTypeResolver(meta, cfg, NewSchemaMapper(cfg))

	cases := []struct {
		name        string
		genericType string
		typeParams  map[string]string
		want        string
	}{
		// len(typeParams)==0 with a real parameter present -> returns as-is.
		{"empty params keep bracketed", "Container[T]", map[string]string{}, "Container[T]"},
		// len(typeParams)==0, base empty (leading bracket) -> returns as-is.
		{"empty params no base", "[]", map[string]string{}, "[]"},
		// typeParams provided but base empty -> returns as-is.
		{"params but no base", "[T]", map[string]string{"T": "int"}, "[T]"},
		// param name with no concrete match keeps the original parameter.
		{"unmatched param kept", "Box[Q]", map[string]string{"T": "int"}, "Box[Q]"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := resolver.ResolveGenericType(tc.genericType, tc.typeParams); got != tc.want {
				t.Fatalf("ResolveGenericType(%q) = %q, want %q", tc.genericType, got, tc.want)
			}
		})
	}
}

// TestCovspecResolveIdentType covers resolveIdentType's package-variable lookup
// and name fallback (arg.Type == -1).
func TestCovspecResolveIdentType(t *testing.T) {
	meta := newTestMeta()
	sp := meta.StringPool
	variable := &metadata.Variable{
		Name: sp.Get("user"),
		Type: sp.Get("mypkg.User"),
	}
	meta.Packages = map[string]*metadata.Package{
		"mypkg": {
			Files: map[string]*metadata.File{
				"a.go": {
					Variables: map[string]*metadata.Variable{"user": variable},
				},
			},
		},
	}
	cfg := DefaultAPISpecConfig()
	resolver := NewTypeResolver(meta, cfg, NewSchemaMapper(cfg))

	t.Run("resolves via package variable", func(t *testing.T) {
		a := metadata.NewCallArgument(meta)
		a.SetKind(metadata.KindIdent)
		a.SetName("user")
		a.SetPkg("mypkg")
		// Type left as -1 so the variable lookup fires.
		if got := resolver.resolveIdentType(*a); got != "mypkg.User" {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("falls back to name when not found", func(t *testing.T) {
		a := metadata.NewCallArgument(meta)
		a.SetKind(metadata.KindIdent)
		a.SetName("missing")
		a.SetPkg("nosuchpkg")
		if got := resolver.resolveIdentType(*a); got != "missing" {
			t.Fatalf("got %q", got)
		}
	})
}
