// Copyright 2025 Ehab Terra
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

func TestTypeResolver_ResolveType(t *testing.T) {
	// Create test metadata
	stringPool := metadata.NewStringPool()

	// Create metadata without the circular reference first
	meta := &metadata.Metadata{
		StringPool: stringPool,
		Packages: map[string]*metadata.Package{
			"main": {
				Files: map[string]*metadata.File{
					"test.go": {
						Variables: map[string]*metadata.Variable{
							"user": {
								Name: stringPool.Get("user"),
								Type: stringPool.Get("User"),
							},
						},
						Types: map[string]*metadata.Type{
							"User": {
								Name: stringPool.Get("User"),
								Kind: stringPool.Get("struct"),
								Fields: []metadata.Field{
									{
										Name: stringPool.Get("Name"),
										Type: stringPool.Get("string"),
									},
									{
										Name: stringPool.Get("Age"),
										Type: stringPool.Get("int"),
									},
								},
							},
						},
					},
				},
			},
		},
		CallGraph: []metadata.CallGraphEdge{
			{
				Caller: metadata.Call{
					Name: stringPool.Get("main"),
					Pkg:  stringPool.Get("main"),
				},
				Callee: metadata.Call{
					Name: stringPool.Get("handler"),
					Pkg:  stringPool.Get("main"),
				},
				TypeParamMap: map[string]string{
					"T": "string",
				},
			},
		},
	}

	cfg := DefaultAPISpecConfig()
	schemaMapper := NewSchemaMapper(cfg)
	resolver := NewTypeResolver(meta, cfg, schemaMapper)

	tests := []struct {
		name     string
		arg      metadata.CallArgument
		context  *TrackerNode
		expected string
	}{
		{
			name: "resolve identifier with direct type",
			arg: metadata.CallArgument{
				Meta: meta,
				Kind: stringPool.Get(metadata.KindIdent),
				Name: stringPool.Get("user"),
				Type: stringPool.Get("User"),
			},
			context: func() *TrackerNode {
				node := &TrackerNode{}
				node.CallGraphEdge = &meta.CallGraph[0]
				return node
			}(),
			expected: "User",
		},
		{
			name: "resolve identifier from metadata",
			arg: metadata.CallArgument{
				Meta: meta,
				Kind: stringPool.Get(metadata.KindIdent),
				Name: stringPool.Get("user"),
				Pkg:  stringPool.Get("main"),
				Type: -1, // Explicitly set to -1 to force metadata lookup
			},
			context: func() *TrackerNode {
				node := &TrackerNode{}
				node.CallGraphEdge = &meta.CallGraph[0]
				return node
			}(),
			expected: "User",
		},
		{
			name: "resolve type parameter",
			arg: metadata.CallArgument{
				Meta: meta,
				Kind: stringPool.Get(metadata.KindIdent),
				Name: stringPool.Get("T"),
			},
			context: func() *TrackerNode {
				node := &TrackerNode{}
				node.CallGraphEdge = &meta.CallGraph[0]
				node.typeParamMap = map[string]string{
					"T": "string",
				}
				return node
			}(),
			expected: "string",
		},
		{
			name: "resolve parameter mapping",
			arg: metadata.CallArgument{
				Meta: meta,
				Kind: stringPool.Get(metadata.KindIdent),
				Name: stringPool.Get("user"),
			},
			context: func() *TrackerNode {
				node := &TrackerNode{}
				node.CallGraphEdge = &meta.CallGraph[0]
				return node
			}(),
			expected: "User",
		},
		{
			name: "resolve selector expression",
			arg: metadata.CallArgument{
				Meta: meta,
				Kind: stringPool.Get(metadata.KindSelector),
				Sel: &metadata.CallArgument{
					Meta: meta,
					Kind: stringPool.Get(metadata.KindIdent),
					Name: stringPool.Get("Name"),
					Type: stringPool.Get(""),
				},
				X: &metadata.CallArgument{
					Meta: meta,
					Kind: stringPool.Get(metadata.KindIdent),
					Name: stringPool.Get("user"),
					Type: stringPool.Get("User"),
				},
			},
			context: &TrackerNode{
				CallGraphEdge: &meta.CallGraph[0],
			},
			expected: "string",
		},
		{
			name: "resolve unary expression",
			arg: metadata.CallArgument{
				Meta: meta,
				Kind: stringPool.Get(metadata.KindUnary),
				X: &metadata.CallArgument{
					Meta: meta,
					Kind: stringPool.Get(metadata.KindIdent),
					Name: stringPool.Get("ptr"),
					Type: stringPool.Get("*User"),
				},
			},
			context: &TrackerNode{
				CallGraphEdge: &meta.CallGraph[0],
			},
			expected: "User",
		},
		{
			name: "resolve map type",
			arg: metadata.CallArgument{
				Meta: meta,
				Kind: stringPool.Get(metadata.KindMapType),
				X: &metadata.CallArgument{
					Meta: meta,
					Kind: stringPool.Get(metadata.KindIdent),
					Name: stringPool.Get("key"),
					Type: stringPool.Get("string"),
				},
				Fun: &metadata.CallArgument{
					Meta: meta,
					Kind: stringPool.Get(metadata.KindIdent),
					Name: stringPool.Get("value"),
					Type: stringPool.Get("int"),
				},
			},
			context: func() *TrackerNode {
				node := &TrackerNode{}
				node.CallGraphEdge = &meta.CallGraph[0]
				return node
			}(),
			expected: "map[string]int",
		},
		{
			name: "resolve interface type",
			arg: metadata.CallArgument{
				Meta: meta,
				Kind: stringPool.Get(metadata.KindInterfaceType),
			},
			context: &TrackerNode{
				CallGraphEdge: &meta.CallGraph[0],
			},
			expected: "interface{}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := resolver.ResolveType(tt.arg, tt.context)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestTypeResolver_MapToOpenAPISchema(t *testing.T) {
	meta := &metadata.Metadata{}
	cfg := DefaultAPISpecConfig()
	schemaMapper := NewSchemaMapper(cfg)
	resolver := NewTypeResolver(meta, cfg, schemaMapper)

	tests := []struct {
		name     string
		goType   string
		expected string
	}{
		{
			name:     "string type",
			goType:   "string",
			expected: "string",
		},
		{
			name:     "int type",
			goType:   "int",
			expected: "integer",
		},
		{
			name:     "bool type",
			goType:   "bool",
			expected: "boolean",
		},
		{
			name:     "slice type",
			goType:   "[]string",
			expected: "array",
		},
		{
			name:     "map type",
			goType:   "map[string]int",
			expected: "object",
		},
		{
			name:     "pointer type",
			goType:   "*User",
			expected: "", // Custom types return empty schema (will be handled by ref)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schema := resolver.MapToOpenAPISchema(tt.goType)
			if schema.Type != tt.expected {
				t.Errorf("expected type %s, got %s", tt.expected, schema.Type)
			}
		})
	}
}

func TestTypeResolver_ResolveGenericType(t *testing.T) {
	meta := &metadata.Metadata{}
	cfg := DefaultAPISpecConfig()
	schemaMapper := NewSchemaMapper(cfg)
	resolver := NewTypeResolver(meta, cfg, schemaMapper)

	tests := []struct {
		name        string
		genericType string
		typeParams  map[string]string
		expected    string
	}{
		{
			name:        "resolve generic with string parameter",
			genericType: "Container[T]",
			typeParams:  map[string]string{"T": "string"},
			expected:    "Container[string]",
		},
		{
			name:        "resolve generic with multiple parameters",
			genericType: "Map[K,V]",
			typeParams:  map[string]string{"K": "string", "V": "int"},
			expected:    "Map[string,int]",
		},
		{
			name:        "resolve generic with ordered parameters",
			genericType: "Map[T,U]",
			typeParams:  map[string]string{"T": "string", "U": "int"},
			expected:    "Map[string,int]",
		},
		{
			name:        "resolve nested generic",
			genericType: "Container[List[T]]",
			typeParams:  map[string]string{"T": "string"},
			expected:    "Container[List[string]]",
		},
		{
			name:        "resolve complex nested generic",
			genericType: "Map[K,List[V]]",
			typeParams:  map[string]string{"K": "string", "V": "int"},
			expected:    "Map[string,List[int]]",
		},
		{
			name:        "resolve simple nested with V parameter",
			genericType: "List[V]",
			typeParams:  map[string]string{"V": "int"},
			expected:    "List[int]",
		},
		{
			name:        "resolve nested with K and V parameters",
			genericType: "Map[K,List[V]]",
			typeParams:  map[string]string{"K": "string", "V": "int"},
			expected:    "Map[string,List[int]]",
		},
		{
			name:        "resolve simple nested generic",
			genericType: "Container[List[T]]",
			typeParams:  map[string]string{"T": "string"},
			expected:    "Container[List[string]]",
		},
		{
			name:        "empty generic parameters",
			genericType: "Container[]",
			typeParams:  map[string]string{},
			expected:    "Container",
		},
		{
			name:        "no type parameters",
			genericType: "Container",
			typeParams:  map[string]string{},
			expected:    "Container",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := resolver.ResolveGenericType(tt.genericType, tt.typeParams)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestTypeResolver_ExtractTypeParameters(t *testing.T) {
	meta := &metadata.Metadata{}
	cfg := DefaultAPISpecConfig()
	schemaMapper := NewSchemaMapper(cfg)
	resolver := NewTypeResolver(meta, cfg, schemaMapper)

	tests := []struct {
		name        string
		genericType string
		expected    map[string]string
	}{
		{
			name:        "extract single type parameter",
			genericType: "Container[T]",
			expected:    map[string]string{"T": "T"},
		},
		{
			name:        "extract multiple type parameters",
			genericType: "Map[K,V]",
			expected:    map[string]string{"T": "K", "U": "V"},
		},
		{
			name:        "extract nested type parameters",
			genericType: "Container[List[T]]",
			expected:    map[string]string{"T": "List[T]"},
		},
		{
			name:        "extract complex nested parameters",
			genericType: "Map[K,List[V]]",
			expected:    map[string]string{"T": "K", "U": "List[V]"},
		},
		{
			name:        "extract with constraints",
			genericType: "Container[T comparable]",
			expected:    map[string]string{"T": "T comparable"},
		},
		{
			name:        "no type parameters",
			genericType: "Container",
			expected:    map[string]string{},
		},
		{
			name:        "empty type parameters",
			genericType: "Container[]",
			expected:    map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := resolver.ExtractTypeParameters(tt.genericType)
			if len(result) != len(tt.expected) {
				t.Errorf("expected %d parameters, got %d", len(tt.expected), len(result))
			}

			// Check that we have the expected number of parameters
			expectedCount := len(tt.expected)
			actualCount := len(result)
			if expectedCount != actualCount {
				t.Errorf("expected %d parameters, got %d", expectedCount, actualCount)
			}
		})
	}
}

// TestTypeResolver_ArgumentKinds covers the resolveTypeFromArgument dispatch
// for every argument kind, via ResolveType with a nil context (the fallback
// path when no call-graph edge is available).
func TestTypeResolver_ArgumentKinds(t *testing.T) {
	stringPool := metadata.NewStringPool()
	meta := &metadata.Metadata{StringPool: stringPool}
	cfg := DefaultAPISpecConfig()
	resolver := NewTypeResolver(meta, cfg, NewSchemaMapper(cfg))

	ident := func(name, typ string) *metadata.CallArgument {
		a := metadata.NewCallArgument(meta)
		a.SetKind(metadata.KindIdent)
		a.SetName(name)
		if typ != "" {
			a.SetType(typ)
		}
		return a
	}

	tests := []struct {
		name     string
		arg      *metadata.CallArgument
		expected string
	}{
		{
			name: "call with nil Fun",
			arg: func() *metadata.CallArgument {
				a := metadata.NewCallArgument(meta)
				a.SetKind(metadata.KindCall)
				return a
			}(),
			expected: "func()",
		},
		{
			name: "call extracts return type from func signature",
			arg: func() *metadata.CallArgument {
				a := metadata.NewCallArgument(meta)
				a.SetKind(metadata.KindCall)
				a.Fun = ident("newUser", "func(int) User")
				return a
			}(),
			expected: "User",
		},
		{
			name: "call with non-func type passes it through",
			arg: func() *metadata.CallArgument {
				a := metadata.NewCallArgument(meta)
				a.SetKind(metadata.KindCall)
				a.Fun = ident("u", "User")
				return a
			}(),
			expected: "User",
		},
		{
			name: "type conversion returns target type",
			arg: func() *metadata.CallArgument {
				a := metadata.NewCallArgument(meta)
				a.SetKind(metadata.KindTypeConversion)
				a.Fun = ident("UserID", "UserID")
				return a
			}(),
			expected: "UserID",
		},
		{
			name: "type conversion with nil Fun is empty",
			arg: func() *metadata.CallArgument {
				a := metadata.NewCallArgument(meta)
				a.SetKind(metadata.KindTypeConversion)
				return a
			}(),
			expected: "",
		},
		{
			name: "composite literal resolves base type",
			arg: func() *metadata.CallArgument {
				a := metadata.NewCallArgument(meta)
				a.SetKind(metadata.KindCompositeLit)
				a.X = ident("User", "User")
				return a
			}(),
			expected: "User",
		},
		{
			name: "composite literal with nil X falls back to kind",
			arg: func() *metadata.CallArgument {
				a := metadata.NewCallArgument(meta)
				a.SetKind(metadata.KindCompositeLit)
				return a
			}(),
			expected: metadata.KindCompositeLit,
		},
		{
			name: "index into slice yields element type",
			arg: func() *metadata.CallArgument {
				a := metadata.NewCallArgument(meta)
				a.SetKind(metadata.KindIndex)
				a.X = ident("users", "[]User")
				return a
			}(),
			expected: "User",
		},
		{
			name: "index into map yields value type",
			arg: func() *metadata.CallArgument {
				a := metadata.NewCallArgument(meta)
				a.SetKind(metadata.KindIndex)
				a.X = ident("byName", "map[string]User")
				return a
			}(),
			expected: "User",
		},
		{
			name: "index into non-container passes base through",
			arg: func() *metadata.CallArgument {
				a := metadata.NewCallArgument(meta)
				a.SetKind(metadata.KindIndex)
				a.X = ident("v", "Vector")
				return a
			}(),
			expected: "Vector",
		},
		{
			name: "index with nil X falls back to kind",
			arg: func() *metadata.CallArgument {
				a := metadata.NewCallArgument(meta)
				a.SetKind(metadata.KindIndex)
				return a
			}(),
			expected: metadata.KindIndex,
		},
		{
			name: "unary adds pointer to non-pointer base",
			arg: func() *metadata.CallArgument {
				a := metadata.NewCallArgument(meta)
				a.SetKind(metadata.KindUnary)
				a.X = ident("u", "User")
				return a
			}(),
			expected: "*User",
		},
		{
			name: "unary with nil X prefixes own type",
			arg: func() *metadata.CallArgument {
				a := metadata.NewCallArgument(meta)
				a.SetKind(metadata.KindUnary)
				a.SetType("User")
				return a
			}(),
			expected: "*User",
		},
		{
			name: "star dereferences pointer base",
			arg: func() *metadata.CallArgument {
				a := metadata.NewCallArgument(meta)
				a.SetKind(metadata.KindStar)
				a.X = ident("p", "*User")
				return a
			}(),
			expected: "User",
		},
		{
			name: "map type with missing halves falls back",
			arg: func() *metadata.CallArgument {
				a := metadata.NewCallArgument(meta)
				a.SetKind(metadata.KindMapType)
				return a
			}(),
			expected: "map",
		},
		{
			name: "literal uses its type",
			arg: func() *metadata.CallArgument {
				a := metadata.NewCallArgument(meta)
				a.SetKind(metadata.KindLiteral)
				a.SetType("string")
				return a
			}(),
			expected: "string",
		},
		{
			name: "raw uses raw text",
			arg: func() *metadata.CallArgument {
				a := metadata.NewCallArgument(meta)
				a.SetKind(metadata.KindRaw)
				a.SetRaw("some raw expr")
				return a
			}(),
			expected: "some raw expr",
		},
		{
			name: "selector without X uses Sel name",
			arg: func() *metadata.CallArgument {
				a := metadata.NewCallArgument(meta)
				a.SetKind(metadata.KindSelector)
				a.Sel = ident("Field", "")
				return a
			}(),
			expected: "Field",
		},
		{
			name: "selector without field metadata concatenates",
			arg: func() *metadata.CallArgument {
				a := metadata.NewCallArgument(meta)
				a.SetKind(metadata.KindSelector)
				a.X = ident("u", "Unknown")
				a.Sel = ident("Field", "")
				return a
			}(),
			expected: "Unknown.Field",
		},
		{
			name: "ident without type or metadata falls back to name",
			arg: func() *metadata.CallArgument {
				a := metadata.NewCallArgument(meta)
				a.SetKind(metadata.KindIdent)
				a.SetName("mystery")
				return a
			}(),
			expected: "mystery",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolver.ResolveType(*tt.arg, nil); got != tt.expected {
				t.Errorf("ResolveType = %q, want %q", got, tt.expected)
			}
		})
	}
}

// TestTypeResolver_HelperNilSafety pins the nil-context/nil-pool guards.
func TestTypeResolver_HelperNilSafety(t *testing.T) {
	cfg := DefaultAPISpecConfig()
	resolver := NewTypeResolver(nil, cfg, NewSchemaMapper(cfg))

	if got := resolver.getString(3); got != "" {
		t.Errorf("getString with nil meta = %q, want empty", got)
	}
	if got := resolver.getCallerName(nil); got != "" {
		t.Errorf("getCallerName(nil) = %q, want empty", got)
	}
	if got := resolver.getCallerPkg(nil); got != "" {
		t.Errorf("getCallerPkg(nil) = %q, want empty", got)
	}
	// Node with a nil edge takes the same guard path.
	node := &TrackerNode{}
	if got := resolver.getCallerName(node); got != "" {
		t.Errorf("getCallerName(edgeless node) = %q, want empty", got)
	}
	if got := resolver.getCallerPkg(node); got != "" {
		t.Errorf("getCallerPkg(edgeless node) = %q, want empty", got)
	}
}

// TestTypeResolver_GenericHelpers covers the generic-name helpers directly:
// parameter-name extraction, concrete-type lookup, and generated parameter
// naming (including the wrap past 'Z', which used to produce '[').
func TestTypeResolver_GenericHelpers(t *testing.T) {
	cfg := DefaultAPISpecConfig()
	resolver := NewTypeResolver(nil, cfg, NewSchemaMapper(cfg))

	t.Run("extractParameterName", func(t *testing.T) {
		cases := []struct{ in, want string }{
			{"K", "K"},
			{"v", "v"},                 // single letter, any case
			{"List[V]", "V"},           // nested parameter
			{"Map[K, List[V]]", "V"},   // recursion descends to the innermost bracket
			{"T comparable", "T"},      // first word is the parameter
			{"main.User", "main.User"}, // not a parameter shape: unchanged
			{"some words here", "some words here"},
		}
		for _, c := range cases {
			if got := resolver.extractParameterName(c.in); got != c.want {
				t.Errorf("extractParameterName(%q) = %q, want %q", c.in, got, c.want)
			}
		}
	})

	t.Run("findConcreteTypeByName", func(t *testing.T) {
		params := map[string]string{"T": "string", "V": "int"}
		if got := resolver.findConcreteTypeByName("T", params); got != "string" {
			t.Errorf("exact match = %q, want string", got)
		}
		if got := resolver.findConcreteTypeByName("List[V]", params); got != "int" {
			t.Errorf("extracted match = %q, want int", got)
		}
		if got := resolver.findConcreteTypeByName("Q", params); got != "" {
			t.Errorf("miss = %q, want empty", got)
		}
	})

	t.Run("generateParameterName", func(t *testing.T) {
		cases := []struct {
			index int
			want  string
		}{
			{0, "T"}, {1, "U"}, {6, "Z"},
			{7, "T1"}, {8, "U1"}, {13, "Z1"}, {14, "T2"},
		}
		for _, c := range cases {
			if got := resolver.generateParameterName(c.index); got != c.want {
				t.Errorf("generateParameterName(%d) = %q, want %q", c.index, got, c.want)
			}
		}
	})
}

// TestTypeResolver_ResolveGenericType_Edges covers the bracket-cleanup
// branches ResolveGenericType takes when no or empty parameters are present.
func TestTypeResolver_ResolveGenericType_Edges(t *testing.T) {
	cfg := DefaultAPISpecConfig()
	resolver := NewTypeResolver(nil, cfg, NewSchemaMapper(cfg))

	cases := []struct {
		name    string
		generic string
		params  map[string]string
		want    string
	}{
		{"no params keeps instantiated form", "Page[User]", nil, "Page[User]"},
		{"no params cleans empty brackets", "Page[]", nil, "Page"},
		{"no params plain type unchanged", "User", nil, "User"},
		{"empty param string with params map", "Page[]", map[string]string{"T": "User"}, "Page"},
		{"plain type with params map unchanged", "User", map[string]string{"T": "User"}, "User"},
		{"nested generic recursion", "List[Box[T]]", map[string]string{"T": "int"}, "List[Box[int]]"},
		{"unknown parameter kept", "Pair[T,Q]", map[string]string{"T": "string"}, "Pair[string,Q]"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := resolver.ResolveGenericType(c.generic, c.params); got != c.want {
				t.Errorf("ResolveGenericType(%q) = %q, want %q", c.generic, got, c.want)
			}
		})
	}
}
