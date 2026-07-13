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
	"go/parser"
	"go/token"
	"testing"
)

// returnTypeMeta builds a small metadata world for the return-type resolvers:
// a package-level variable, a function with assignments (concrete and chained),
// a struct type with a field, and a func-typed variable for signature
// extraction.
func returnTypeMeta() *Metadata {
	sp := NewStringPool()
	m := &Metadata{
		StringPool: sp,
		Packages: map[string]*Package{
			"app": {
				Files: map[string]*File{
					"app.go": {
						Variables: map[string]*Variable{
							"globalUser": {Name: sp.Get("globalUser"), Type: sp.Get("User")},
							"mk":         {Name: sp.Get("mk"), Type: sp.Get("func(int) User")},
						},
						Types: map[string]*Type{
							"User": {
								Name: sp.Get("User"),
								Kind: sp.Get("struct"),
								Fields: []Field{
									{Name: sp.Get("Name"), Type: sp.Get("string")},
								},
							},
						},
						Functions: map[string]*Function{},
					},
				},
			},
		},
	}

	fn := &Function{Name: sp.Get("handler"), AssignmentMap: map[string][]Assignment{}}
	// Direct concrete-type assignment.
	fn.AssignmentMap["u"] = []Assignment{{
		VariableName: sp.Get("u"),
		ConcreteType: sp.Get("User"),
		Value:        CallArgument{Kind: -1, Name: -1, Value: -1, Raw: -1, Pkg: -1, Type: -1, Position: -1, ResolvedType: -1, GenericTypeName: -1, Meta: m},
	}}
	// Chained assignment: no concrete type, value is the ident "u", so
	// resolution must recurse through the assignment map.
	chainedVal := CallArgument{Kind: sp.Get(KindIdent), Name: sp.Get("u"), Value: -1, Raw: -1, Pkg: -1, Type: -1, Position: -1, ResolvedType: -1, GenericTypeName: -1, Meta: m}
	fn.AssignmentMap["chained"] = []Assignment{{
		VariableName: sp.Get("chained"),
		ConcreteType: -1,
		Value:        chainedVal,
	}}
	m.Packages["app"].Files["app.go"].Functions["handler"] = fn
	return m
}

func arg(m *Metadata, kind string) *CallArgument {
	a := NewCallArgument(m)
	if kind != "" {
		a.SetKind(kind)
	}
	return a
}

func TestDetermineResolvedTypeFromReturnVar(t *testing.T) {
	m := returnTypeMeta()

	tests := []struct {
		name string
		mk   func() *CallArgument
		want string
	}{
		{
			name: "ident resolves package variable",
			mk: func() *CallArgument {
				a := arg(m, KindIdent)
				a.SetName("globalUser")
				return a
			},
			want: "User",
		},
		{
			name: "ident resolves concrete assignment",
			mk: func() *CallArgument {
				a := arg(m, KindIdent)
				a.SetName("u")
				return a
			},
			want: "User",
		},
		{
			name: "ident recurses through chained assignment",
			mk: func() *CallArgument {
				a := arg(m, KindIdent)
				a.SetName("chained")
				return a
			},
			want: "User",
		},
		{
			name: "ident falls back to its own name",
			mk: func() *CallArgument {
				a := arg(m, KindIdent)
				a.SetName("mystery")
				return a
			},
			want: "mystery",
		},
		{
			name: "selector resolves struct field type",
			mk: func() *CallArgument {
				a := arg(m, KindSelector)
				x := arg(m, KindIdent)
				x.SetName("globalUser")
				sel := arg(m, KindIdent)
				sel.SetName("Name")
				a.X, a.Sel = x, sel
				return a
			},
			want: "string",
		},
		{
			name: "selector falls back to concatenation",
			mk: func() *CallArgument {
				a := arg(m, KindSelector)
				x := arg(m, KindIdent)
				x.SetName("globalUser")
				sel := arg(m, KindIdent)
				sel.SetName("Missing")
				a.X, a.Sel = x, sel
				return a
			},
			want: "User.Missing",
		},
		{
			name: "selector without parts uses own type",
			mk: func() *CallArgument {
				a := arg(m, KindSelector)
				a.SetType("Fallback")
				return a
			},
			want: "Fallback",
		},
		{
			name: "call without Fun is opaque func",
			mk: func() *CallArgument {
				return arg(m, KindCall)
			},
			want: "func()",
		},
		{
			name: "call extracts return type from func-typed variable",
			mk: func() *CallArgument {
				a := arg(m, KindCall)
				fun := arg(m, KindIdent)
				fun.SetName("mk")
				a.Fun = fun
				return a
			},
			want: "User",
		},
		{
			name: "call passes through non-func type",
			mk: func() *CallArgument {
				a := arg(m, KindCall)
				fun := arg(m, KindIdent)
				fun.SetName("globalUser")
				a.Fun = fun
				return a
			},
			want: "User",
		},
		{
			name: "composite literal resolves via X",
			mk: func() *CallArgument {
				a := arg(m, KindCompositeLit)
				x := arg(m, KindIdent)
				x.SetName("globalUser")
				a.X = x
				return a
			},
			want: "User",
		},
		{
			name: "composite literal without X uses own type",
			mk: func() *CallArgument {
				a := arg(m, KindCompositeLit)
				a.SetType("User")
				return a
			},
			want: "User",
		},
		{
			name: "unary adds pointer",
			mk: func() *CallArgument {
				a := arg(m, KindUnary)
				x := arg(m, KindIdent)
				x.SetName("globalUser")
				a.X = x
				return a
			},
			want: "*User",
		},
		{
			name: "star dereferences pointer type",
			mk: func() *CallArgument {
				a := arg(m, KindStar)
				a.SetType("*User")
				x := arg(m, KindIdent)
				x.SetName("ptr")
				a.X = x
				return a
			},
			want: "ptr", // base "ptr" has no pointer prefix to cut
		},
		{
			name: "unary without X uses own type",
			mk: func() *CallArgument {
				a := arg(m, KindUnary)
				a.SetType("*User")
				return a
			},
			want: "*User",
		},
		{
			name: "literal returns its type",
			mk: func() *CallArgument {
				a := arg(m, KindLiteral)
				a.SetType("string")
				return a
			},
			want: "string",
		},
		{
			name: "unknown kind falls back to type field",
			mk: func() *CallArgument {
				a := arg(m, KindRaw)
				a.SetType("whatever")
				return a
			},
			want: "whatever",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := m.determineResolvedTypeFromReturnVar(tt.mk(), "app", "handler"); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// TestAnalyzeAssignmentValue covers the expression-kind dispatch with no type
// info (the info-nil branches every expression kind falls back to).
func TestAnalyzeAssignmentValue(t *testing.T) {
	m := returnTypeMeta()
	fset := token.NewFileSet()

	parse := func(src string) (pkg string, got *CallArgument) {
		t.Helper()
		expr, err := parser.ParseExpr(src)
		if err != nil {
			t.Fatalf("parse %q: %v", src, err)
		}
		return analyzeAssignmentValue(expr, nil, "handler", "app", m, fset)
	}

	if pkg, got := analyzeAssignmentValue(nil, nil, "handler", "app", m, fset); pkg != "app" || got != nil {
		t.Errorf("nil expr = (%q, %v), want (app, nil)", pkg, got)
	}

	// Identifier: traced through the metadata (no origin here, nil type).
	if pkg, _ := parse("someVar"); pkg == "" {
		t.Error("ident should keep a package")
	}

	// Selector with ident base takes the trace path.
	if pkg, _ := parse("pkg.Field"); pkg == "" {
		t.Error("selector should keep a package")
	}

	// Direct and method calls take the call branches.
	if pkg, _ := parse("newUser()"); pkg == "" {
		t.Error("call should keep a package")
	}
	if pkg, _ := parse("svc.Create()"); pkg == "" {
		t.Error("method call should keep a package")
	}

	// Type assertion yields the asserted type.
	if _, got := parse("v.(User)"); got == nil || got.GetName() != "User" {
		t.Errorf("type assertion arg = %+v, want User ident", got)
	}

	// Star expression recurses into the base.
	if pkg, _ := parse("*ptr"); pkg == "" {
		t.Error("star should keep a package")
	}

	// Composite literal reports its type.
	if _, got := parse("User{}"); got == nil || got.GetName() != "User" {
		t.Errorf("composite literal arg = %+v, want User ident", got)
	}

	// Anything else falls through to ExprToCallArgument.
	if _, got := parse("1 + 2"); got == nil {
		t.Error("binary expr should produce a call argument")
	}
}
