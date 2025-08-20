package spec

import (
	"testing"

	"github.com/ehabterra/swagen/internal/metadata"
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
				ParamArgMap: map[string]metadata.CallArgument{
					"user": {
						Meta: nil, // Will be set after metadata is fully created
						Kind: stringPool.Get(metadata.KindIdent),
						Name: stringPool.Get("user"),
						Type: stringPool.Get("User"),
					},
				},
			},
		},
	}

	// Now set the Meta field for the CallArgument in ParamArgMap
	userArg := meta.CallGraph[0].ParamArgMap["user"]
	userArg.Meta = meta
	meta.CallGraph[0].ParamArgMap["user"] = userArg

	cfg := DefaultSwagenConfig()
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
			expected: "User",
		},
		{
			name: "resolve type parameter",
			arg: metadata.CallArgument{
				Meta: meta,
				Kind: stringPool.Get(metadata.KindIdent),
				Name: stringPool.Get("T"),
			},
			context: &TrackerNode{
				CallGraphEdge: &meta.CallGraph[0],
			},
			expected: "string",
		},
		{
			name: "resolve parameter mapping",
			arg: metadata.CallArgument{
				Meta: meta,
				Kind: stringPool.Get(metadata.KindIdent),
				Name: stringPool.Get("user"),
			},
			context: &TrackerNode{
				CallGraphEdge: &meta.CallGraph[0],
			},
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
			expected: "map[string]int",
		},
		{
			name: "resolve interface type",
			arg: metadata.CallArgument{
				Meta: meta,
				Kind: stringPool.Get(metadata.KindInterfaceType),
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
	cfg := DefaultSwagenConfig()
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
	cfg := DefaultSwagenConfig()
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
	cfg := DefaultSwagenConfig()
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
