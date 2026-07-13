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
	"testing"

	"github.com/ehabterra/apispec/internal/metadata"
)

// fieldResolverMeta builds metadata with a package holding two types: Outer
// (package-level Types, with a plain field and an embedded Inner) and Inner
// (file-level Types, with the embedded field), plus a const and a var for
// constIdentDeclaredType.
func fieldResolverMeta() *metadata.Metadata {
	sp := metadata.NewStringPool()
	m := &metadata.Metadata{
		StringPool: sp,
		Packages: map[string]*metadata.Package{
			"example.com/app": {
				Types: map[string]*metadata.Type{
					"Outer": {
						Name: sp.Get("Outer"),
						Fields: []metadata.Field{
							{Name: sp.Get("Message"), Type: sp.Get("string")},
						},
						Embeds: []int{sp.Get("example.com/app.Inner")},
					},
				},
				Files: map[string]*metadata.File{
					"app.go": {
						Types: map[string]*metadata.Type{
							"Inner": {
								Name: sp.Get("Inner"),
								Fields: []metadata.Field{
									{Name: sp.Get("Code"), Type: sp.Get("int")},
								},
							},
						},
						Variables: map[string]*metadata.Variable{
							"StatusOK": {
								Name: sp.Get("StatusOK"),
								Tok:  sp.Get("const"),
								Type: sp.Get("Status"),
							},
							"tmplBody": {
								Name:         sp.Get("tmplBody"),
								Tok:          sp.Get("const"),
								Type:         -1,
								ResolvedType: sp.Get("string"),
							},
							"mutable": {
								Name: sp.Get("mutable"),
								Tok:  sp.Get("var"),
								Type: sp.Get("int"),
							},
						},
					},
				},
			},
		},
	}
	return m
}

func selArg(m *metadata.Metadata, baseType, field string) *metadata.CallArgument {
	a := metadata.NewCallArgument(m)
	a.SetKind(metadata.KindSelector)
	x := metadata.NewCallArgument(m)
	x.SetKind(metadata.KindIdent)
	x.SetName("obj")
	x.SetType(baseType)
	sel := metadata.NewCallArgument(m)
	sel.SetKind(metadata.KindIdent)
	sel.SetName(field)
	a.X, a.Sel = x, sel
	return a
}

func TestResolveSelectorFieldType(t *testing.T) {
	m := fieldResolverMeta()
	cp := NewContextProvider(m)

	t.Run("nil and non-selector args", func(t *testing.T) {
		if got := resolveSelectorFieldType(nil, cp); got != "" {
			t.Errorf("nil arg = %q", got)
		}
		a := metadata.NewCallArgument(m)
		a.SetKind(metadata.KindIdent)
		if got := resolveSelectorFieldType(a, cp); got != "" {
			t.Errorf("non-selector = %q", got)
		}
	})

	t.Run("fast path prefers recorded types", func(t *testing.T) {
		a := selArg(m, "example.com/app.Outer", "Message")
		a.SetResolvedType("string")
		if got := resolveSelectorFieldType(a, cp); got != "string" {
			t.Errorf("resolved-type fast path = %q", got)
		}
		b := selArg(m, "example.com/app.Outer", "Message")
		b.SetType("string")
		if got := resolveSelectorFieldType(b, cp); got != "string" {
			t.Errorf("type fast path = %q", got)
		}
	})

	t.Run("metadata lookup on package-level type", func(t *testing.T) {
		a := selArg(m, "example.com/app.Outer", "Message")
		if got := resolveSelectorFieldType(a, cp); got != "string" {
			t.Errorf("field lookup = %q, want string", got)
		}
	})

	t.Run("pointer base and embedded field", func(t *testing.T) {
		// Code lives on Inner, embedded in Outer — one level of embedding.
		a := selArg(m, "*example.com/app.Outer", "Code")
		if got := resolveSelectorFieldType(a, cp); got != "int" {
			t.Errorf("embedded field via pointer base = %q, want int", got)
		}
	})

	t.Run("file-level type lookup", func(t *testing.T) {
		a := selArg(m, "example.com/app.Inner", "Code")
		if got := resolveSelectorFieldType(a, cp); got != "int" {
			t.Errorf("file-level type = %q, want int", got)
		}
	})

	t.Run("unresolvable shapes", func(t *testing.T) {
		if got := resolveSelectorFieldType(selArg(m, "example.com/app.Outer", "NoSuchField"), cp); got != "" {
			t.Errorf("missing field = %q", got)
		}
		if got := resolveSelectorFieldType(selArg(m, "example.com/nowhere.Gone", "F"), cp); got != "" {
			t.Errorf("missing package = %q", got)
		}
		noSel := selArg(m, "example.com/app.Outer", "x")
		noSel.Sel = nil
		if got := resolveSelectorFieldType(noSel, cp); got != "" {
			t.Errorf("nil Sel = %q", got)
		}
		noBase := selArg(m, "", "Message")
		if got := resolveSelectorFieldType(noBase, cp); got != "" {
			t.Errorf("untyped base = %q", got)
		}
	})
}

func TestSelectorBaseType(t *testing.T) {
	m := fieldResolverMeta()
	cp := NewContextProvider(m)

	if got := selectorBaseType(nil, cp); got != "" {
		t.Errorf("nil = %q", got)
	}

	ident := metadata.NewCallArgument(m)
	ident.SetKind(metadata.KindIdent)
	ident.SetResolvedType("example.com/app.Outer")
	if got := selectorBaseType(ident, cp); got != "example.com/app.Outer" {
		t.Errorf("ident resolved type = %q", got)
	}

	call := metadata.NewCallArgument(m)
	call.SetKind(metadata.KindCall)
	call.SetType("example.com/app.Inner")
	if got := selectorBaseType(call, cp); got != "example.com/app.Inner" {
		t.Errorf("call type = %q", got)
	}

	// &obj.Message — unary wraps a selector; recursion resolves the field.
	unary := metadata.NewCallArgument(m)
	unary.SetKind(metadata.KindUnary)
	unary.X = selArg(m, "example.com/app.Outer", "Message")
	if got := selectorBaseType(unary, cp); got != "string" {
		t.Errorf("unary over selector = %q, want string", got)
	}

	bare := metadata.NewCallArgument(m)
	bare.SetKind(metadata.KindUnary)
	if got := selectorBaseType(bare, cp); got != "" {
		t.Errorf("unary without X = %q", got)
	}

	lit := metadata.NewCallArgument(m)
	lit.SetKind(metadata.KindLiteral)
	if got := selectorBaseType(lit, cp); got != "" {
		t.Errorf("unsupported kind = %q", got)
	}
}

func TestConstIdentDeclaredType(t *testing.T) {
	m := fieldResolverMeta()
	cp := NewContextProvider(m)

	mk := func(name string) *metadata.CallArgument {
		a := metadata.NewCallArgument(m)
		a.SetKind(metadata.KindIdent)
		a.SetName(name)
		a.SetPkg("example.com/app")
		return a
	}

	if got := constIdentDeclaredType(mk("StatusOK"), cp); got != "Status" {
		t.Errorf("const with declared type = %q, want Status", got)
	}
	if got := constIdentDeclaredType(mk("tmplBody"), cp); got != "string" {
		t.Errorf("const with resolved type only = %q, want string", got)
	}
	if got := constIdentDeclaredType(mk("mutable"), cp); got != "" {
		t.Errorf("var (non-const) = %q, want empty", got)
	}
	if got := constIdentDeclaredType(mk("unknownName"), cp); got != "" {
		t.Errorf("unknown ident = %q, want empty", got)
	}

	other := mk("StatusOK")
	other.SetPkg("example.com/other")
	if got := constIdentDeclaredType(other, cp); got != "" {
		t.Errorf("unknown package = %q, want empty", got)
	}

	notIdent := metadata.NewCallArgument(m)
	notIdent.SetKind(metadata.KindSelector)
	if got := constIdentDeclaredType(notIdent, cp); got != "" {
		t.Errorf("non-ident = %q, want empty", got)
	}
	if got := constIdentDeclaredType(nil, cp); got != "" {
		t.Errorf("nil = %q, want empty", got)
	}
}

// TestBodySourceIdentType covers identType's fallback ladder: resolved type,
// declared type, call-site assignments, and the empty fallthrough.
func TestBodySourceIdentType(t *testing.T) {
	m := fieldResolverMeta()
	cp := NewContextProvider(m)
	r := &bodySourceResolver{contextProvider: cp}

	if got := r.identType(nil, nil); got != "" {
		t.Errorf("nil arg = %q", got)
	}

	a := metadata.NewCallArgument(m)
	a.SetKind(metadata.KindIdent)
	a.SetName("c")
	a.SetResolvedType("*gin.Context")
	if got := r.identType(a, nil); got != "*gin.Context" {
		t.Errorf("resolved type = %q", got)
	}

	b := metadata.NewCallArgument(m)
	b.SetKind(metadata.KindIdent)
	b.SetName("c")
	b.SetType("echo.Context")
	if got := r.identType(b, nil); got != "echo.Context" {
		t.Errorf("declared type = %q", got)
	}

	// Type recovered from the call site's assignment map.
	sp := m.StringPool
	edge := &metadata.CallGraphEdge{
		Caller: metadata.Call{Meta: m, Name: sp.Get("handler"), Pkg: sp.Get("example.com/app")},
		Callee: metadata.Call{Meta: m, Name: sp.Get("decode"), Pkg: sp.Get("example.com/app")},
		AssignmentMap: map[string][]metadata.Assignment{
			"body": {{ConcreteType: sp.Get("io.ReadCloser")}},
		},
	}
	c := metadata.NewCallArgument(m)
	c.SetKind(metadata.KindIdent)
	c.SetName("body")
	if got := r.identType(c, edge); got != "io.ReadCloser" {
		t.Errorf("assignment type = %q, want io.ReadCloser", got)
	}

	// Unknown ident with no assignments falls through the trace to empty.
	d := metadata.NewCallArgument(m)
	d.SetKind(metadata.KindIdent)
	d.SetName("nothingKnown")
	if got := r.identType(d, edge); got != "" {
		t.Errorf("unknown ident = %q, want empty", got)
	}
}
