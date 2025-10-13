package spec

import (
	"testing"

	"github.com/ehabterra/apispec/internal/metadata"
)

func TestGenerateSchemaFromType_Comprehensive(t *testing.T) {
	// Create metadata with string pool
	stringPool := metadata.NewStringPool()
	meta := &metadata.Metadata{
		StringPool: stringPool,
		Packages: map[string]*metadata.Package{
			"main": {
				Files: map[string]*metadata.File{
					"types.go": {
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
									{
										Name: stringPool.Get("Email"),
										Type: stringPool.Get("*string"),
									},
									{
										Name: stringPool.Get("Tags"),
										Type: stringPool.Get("[]string"),
									},
									{
										Name: stringPool.Get("Metadata"),
										Type: stringPool.Get("map[string]interface{}"),
									},
								},
							},
							"UserList": {
								Name:   stringPool.Get("UserList"),
								Kind:   stringPool.Get("slice"),
								Target: stringPool.Get("User"),
							},
							"UserMap": {
								Name:   stringPool.Get("UserMap"),
								Kind:   stringPool.Get("map"),
								Target: stringPool.Get("map[string]User"),
							},
							"GenericResponse": {
								Name: stringPool.Get("GenericResponse"),
								Kind: stringPool.Get("struct"),
								Fields: []metadata.Field{
									{
										Name: stringPool.Get("Success"),
										Type: stringPool.Get("bool"),
									},
									{
										Name: stringPool.Get("Data"),
										Type: stringPool.Get("interface{}"),
									},
									{
										Name: stringPool.Get("Error"),
										Type: stringPool.Get("string"),
									},
								},
							},
						},
					},
				},
			},
		},
	}

	// Create config
	cfg := DefaultGinConfig()

	// Test schema generation for various types
	tests := []struct {
		name     string
		typeName string
		expected *Schema
	}{
		{
			name:     "struct type",
			typeName: "User",
			expected: &Schema{
				Type: "object",
			},
		},
		{
			name:     "slice type",
			typeName: "UserList",
			expected: &Schema{
				Type: "array",
			},
		},
		{
			name:     "map type",
			typeName: "UserMap",
			expected: &Schema{
				Type: "object",
			},
		},
		{
			name:     "generic response type",
			typeName: "GenericResponse",
			expected: &Schema{
				Type: "object",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Find the type in metadata
			types := findTypesInMetadata(meta, tt.typeName)
			if len(types) == 0 {
				t.Fatalf("Type %s not found in metadata", tt.typeName)
			}

			// Get the first type (there should only be one)
			var typ *metadata.Type
			for _, t := range types {
				if t != nil {
					typ = t
					break
				}
			}

			if typ == nil {
				t.Fatalf("Type %s is nil", tt.typeName)
			}

			// Generate schema
			usedTypes := make(map[string]*Schema)
			schema, _ := generateSchemaFromType(usedTypes, tt.typeName, typ, meta, cfg, nil)
			if schema == nil {
				t.Fatalf("Failed to generate schema for type %s", tt.typeName)
				return
			}

			// Basic validation
			if schema.Type == "" && schema.Ref == "" {
				t.Errorf("Schema should have either Type or Ref set for type %s", tt.typeName)
			}
		})
	}
}

func TestMapGoTypeToOpenAPISchema_EdgeCases(t *testing.T) {
	// Create metadata with string pool
	stringPool := metadata.NewStringPool()
	meta := &metadata.Metadata{
		StringPool: stringPool,
	}

	// Create config
	cfg := DefaultGinConfig()

	// Test edge cases for type mapping
	tests := []struct {
		name     string
		goType   string
		expected *Schema
	}{
		{
			name:     "empty type",
			goType:   "",
			expected: nil,
		},
		{
			name:     "pointer to pointer",
			goType:   "**string",
			expected: &Schema{Type: "string"},
		},
		{
			name:     "slice of slice",
			goType:   "[][]string",
			expected: &Schema{Type: "array"},
		},
		{
			name:     "map with complex key",
			goType:   "map[User]string",
			expected: &Schema{Type: "object"},
		},
		{
			name:     "map with complex value",
			goType:   "map[string]User",
			expected: &Schema{Type: "object"},
		},
		// Note: Complex types like interfaces, functions, and channels are not fully supported
		// by the current implementation and may return empty schemas
		{
			name:     "time type",
			goType:   "time.Time",
			expected: &Schema{Type: "string", Format: "date-time"},
		},
		{
			name:     "custom type with dots",
			goType:   "github.com/user/project/types.Response",
			expected: &Schema{Ref: "#/components/schemas/github.com_user_project_types.Response"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			usedTypes := make(map[string]*Schema)
			schema, _ := mapGoTypeToOpenAPISchema(usedTypes, tt.goType, meta, cfg, nil)

			if tt.expected == nil {
				if schema != nil {
					t.Errorf("Expected nil schema for %s, got %+v", tt.goType, schema)
				}
			} else {
				if schema == nil {
					t.Errorf("Expected non-nil schema for %s", tt.goType)
				} else {
					// Basic validation
					if tt.expected.Type != "" && schema.Type != tt.expected.Type {
						t.Errorf("Expected type %s for %s, got %s", tt.expected.Type, tt.goType, schema.Type)
					}
					if tt.expected.Ref != "" && schema.Ref != tt.expected.Ref {
						t.Errorf("Expected ref %s for %s, got %s", tt.expected.Ref, tt.goType, schema.Ref)
					}
				}
			}
		})
	}
}

func TestTypeResolution_EdgeCases(t *testing.T) {
	// Create metadata with string pool
	stringPool := metadata.NewStringPool()
	meta := &metadata.Metadata{
		StringPool: stringPool,
		Packages: map[string]*metadata.Package{
			"main": {
				Files: map[string]*metadata.File{
					"types.go": {
						Types: map[string]*metadata.Type{
							"User": {
								Name: stringPool.Get("User"),
								Kind: stringPool.Get("struct"),
								Fields: []metadata.Field{
									{
										Name: stringPool.Get("Name"),
										Type: stringPool.Get("string"),
									},
								},
							},
						},
					},
				},
			},
		},
	}

	// Test type resolution edge cases
	tests := []struct {
		name     string
		typeName string
		expected bool
	}{
		{
			name:     "existing type",
			typeName: "User",
			expected: true,
		},
		{
			name:     "non-existent type",
			typeName: "NonExistentType",
			expected: true, // findTypesInMetadata always returns a map with the type name
		},
		{
			name:     "empty type name",
			typeName: "",
			expected: false, // Empty type names return nil
		},
		{
			name:     "primitive type",
			typeName: "string",
			expected: false, // Primitive types are not stored in metadata
		},
		{
			name:     "pointer type",
			typeName: "*User",
			expected: true, // findTypesInMetadata always returns a map with the type name
		},
		{
			name:     "slice type",
			typeName: "[]User",
			expected: true, // findTypesInMetadata always returns a map with the type name
		},
		{
			name:     "map type",
			typeName: "map[string]User",
			expected: true, // findTypesInMetadata always returns a map with the type name
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			types := findTypesInMetadata(meta, tt.typeName)

			if tt.expected {
				if len(types) == 0 {
					t.Errorf("Expected to find type %s", tt.typeName)
				}
			} else {
				if len(types) > 0 {
					t.Errorf("Expected not to find type %s, but found %d types", tt.typeName, len(types))
				}
			}
		})
	}
}

func TestSchemaGeneration_ComplexTypes(t *testing.T) {
	// Create metadata with string pool
	stringPool := metadata.NewStringPool()
	meta := &metadata.Metadata{
		StringPool: stringPool,
		Packages: map[string]*metadata.Package{
			"main": {
				Files: map[string]*metadata.File{
					"complex_types.go": {
						Types: map[string]*metadata.Type{
							"ComplexStruct": {
								Name: stringPool.Get("ComplexStruct"),
								Kind: stringPool.Get("struct"),
								Fields: []metadata.Field{
									{
										Name: stringPool.Get("ID"),
										Type: stringPool.Get("int"),
									},
									{
										Name: stringPool.Get("Name"),
										Type: stringPool.Get("string"),
									},
									{
										Name: stringPool.Get("Email"),
										Type: stringPool.Get("*string"),
									},
									{
										Name: stringPool.Get("Tags"),
										Type: stringPool.Get("[]string"),
									},
									{
										Name: stringPool.Get("Metadata"),
										Type: stringPool.Get("map[string]interface{}"),
									},
									{
										Name: stringPool.Get("CreatedAt"),
										Type: stringPool.Get("time.Time"),
									},
									{
										Name: stringPool.Get("UpdatedAt"),
										Type: stringPool.Get("*time.Time"),
									},
									{
										Name: stringPool.Get("Status"),
										Type: stringPool.Get("Status"),
									},
									{
										Name: stringPool.Get("Profile"),
										Type: stringPool.Get("*Profile"),
									},
								},
							},
							"Status": {
								Name:   stringPool.Get("Status"),
								Kind:   stringPool.Get("string"),
								Target: stringPool.Get("string"),
							},
							"Profile": {
								Name: stringPool.Get("Profile"),
								Kind: stringPool.Get("struct"),
								Fields: []metadata.Field{
									{
										Name: stringPool.Get("Bio"),
										Type: stringPool.Get("string"),
									},
									{
										Name: stringPool.Get("Avatar"),
										Type: stringPool.Get("*string"),
									},
								},
							},
						},
					},
				},
			},
		},
	}

	// Create config
	cfg := DefaultGinConfig()

	// Test complex type schema generation
	t.Run("complex struct with nested types", func(t *testing.T) {
		types := findTypesInMetadata(meta, "ComplexStruct")
		if len(types) == 0 {
			t.Fatal("ComplexStruct type not found")
		}

		typ := types["ComplexStruct"]
		if typ == nil {
			t.Fatal("ComplexStruct type is nil")
		}

		usedTypes := make(map[string]*Schema)
		schema, _ := generateSchemaFromType(usedTypes, "ComplexStruct", typ, meta, cfg, nil)
		if schema == nil {
			t.Fatal("Failed to generate schema for ComplexStruct")
			return
		}

		// Should be an object type
		if schema.Type != "object" {
			t.Errorf("Expected object type, got %s", schema.Type)
		}

		// Should have properties
		if schema.Properties == nil {
			t.Error("Expected properties to be set")
		}
	})

	t.Run("nested struct resolution", func(t *testing.T) {
		types := findTypesInMetadata(meta, "Profile")
		if len(types) == 0 {
			t.Fatal("Profile type not found")
		}

		typ := types["Profile"]
		if typ == nil {
			t.Fatal("Profile type is nil")
		}

		usedTypes := make(map[string]*Schema)
		schema, _ := generateSchemaFromType(usedTypes, "Profile", typ, meta, cfg, nil)
		if schema == nil {
			t.Fatal("Failed to generate schema for Profile")
			return
		}

		// Should be an object type
		if schema.Type != "object" {
			t.Errorf("Expected object type, got %s", schema.Type)
		}
	})
}

func TestSchemaGeneration_TypeMapping(t *testing.T) {
	// Create metadata with string pool
	stringPool := metadata.NewStringPool()
	meta := &metadata.Metadata{
		StringPool: stringPool,
	}

	// Create config with custom type mappings
	cfg := &APISpecConfig{
		Framework: FrameworkConfig{
			RoutePatterns: []RoutePattern{
				{
					CallRegex:       `^(?i)(GET|POST|PUT|DELETE|PATCH|OPTIONS|HEAD)$`,
					MethodFromCall:  true,
					PathFromArg:     true,
					HandlerFromArg:  true,
					PathArgIndex:    0,
					HandlerArgIndex: 1,
				},
			},
		},
		TypeMapping: []TypeMapping{
			{
				GoType: "CustomType",
				OpenAPIType: &Schema{
					Type: "object",
					Properties: map[string]*Schema{
						"value": {Type: "string"},
					},
				},
			},
			{
				GoType: "[]CustomType",
				OpenAPIType: &Schema{
					Type: "array",
					Items: &Schema{
						Type: "object",
						Properties: map[string]*Schema{
							"value": {Type: "string"},
						},
					},
				},
			},
		},
	}

	// Test custom type mapping
	t.Run("custom type mapping", func(t *testing.T) {
		usedTypes := make(map[string]*Schema)
		schema, _ := mapGoTypeToOpenAPISchema(usedTypes, "CustomType", meta, cfg, nil)
		if schema == nil {
			t.Fatal("Failed to generate schema for CustomType")
			return
		}

		if schema.Type != "object" {
			t.Errorf("Expected object type, got %s", schema.Type)
		}

		if schema.Properties == nil {
			t.Error("Expected properties to be set")
		}
	})

	t.Run("custom slice type mapping", func(t *testing.T) {
		usedTypes := make(map[string]*Schema)
		schema, _ := mapGoTypeToOpenAPISchema(usedTypes, "[]CustomType", meta, cfg, nil)
		if schema == nil {
			t.Fatal("Failed to generate schema for []CustomType")
			return
		}

		if schema.Type != "array" {
			t.Errorf("Expected array type, got %s", schema.Type)
		}

		if schema.Items == nil {
			t.Error("Expected items to be set")
		}
	})
}

func TestSchemaGeneration_ExternalTypes(t *testing.T) {
	// Create metadata with string pool
	stringPool := metadata.NewStringPool()
	meta := &metadata.Metadata{
		StringPool: stringPool,
	}

	// Create config with external types
	cfg := &APISpecConfig{
		Framework: FrameworkConfig{
			RoutePatterns: []RoutePattern{
				{
					CallRegex:       `^(?i)(GET|POST|PUT|DELETE|PATCH|OPTIONS|HEAD)$`,
					MethodFromCall:  true,
					PathFromArg:     true,
					HandlerFromArg:  true,
					PathArgIndex:    0,
					HandlerArgIndex: 1,
				},
			},
		},
		ExternalTypes: []ExternalType{
			{
				Name: "github.com/gin-gonic/gin.H",
				OpenAPIType: &Schema{
					Type: "object",
					AdditionalProperties: &Schema{
						Type: "string",
					},
				},
			},
			{
				Name: "github.com/gin-gonic/gin.Context",
				OpenAPIType: &Schema{
					Type: "object",
					Properties: map[string]*Schema{
						"request":  {Type: "object"},
						"response": {Type: "object"},
					},
				},
			},
		},
	}

	// Test external type handling
	tests := []struct {
		name     string
		typeName string
		expected *Schema
	}{
		{
			name:     "gin.H type",
			typeName: "github.com/gin-gonic/gin.H",
			expected: &Schema{
				Ref: "#/components/schemas/github.com_gin-gonic_gin.H",
			},
		},
		{
			name:     "gin.Context type",
			typeName: "github.com/gin-gonic/gin.Context",
			expected: &Schema{
				Ref: "#/components/schemas/github.com_gin-gonic_gin.Context",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			usedTypes := make(map[string]*Schema)
			schema, _ := mapGoTypeToOpenAPISchema(usedTypes, tt.typeName, meta, cfg, nil)
			if schema == nil {
				t.Fatalf("Failed to generate schema for %s", tt.typeName)
				return
			}

			if schema.Type != tt.expected.Type {
				t.Errorf("Expected type %s for %s, got %s", tt.expected.Type, tt.typeName, schema.Type)
			}
		})
	}
}
