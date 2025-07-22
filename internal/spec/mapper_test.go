package spec

import (
	"reflect"
	"testing"

	"github.com/ehabterra/swagen/internal/metadata"
)

func TestMapGoTypeToOpenAPISchema_PointerTypes(t *testing.T) {
	// Create a simple metadata with a User struct
	stringPool := metadata.NewStringPool()
	meta := &metadata.Metadata{
		StringPool: stringPool,
		Packages: map[string]*metadata.Package{
			"main": {
				Files: map[string]*metadata.File{
					"user.go": {
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
	}

	cfg := DefaultSwagenConfig()

	tests := []struct {
		name     string
		goType   string
		expected string
	}{
		{
			name:     "pointer to struct",
			goType:   "*User",
			expected: "object", // Should generate inline object schema
		},
		{
			name:     "pointer to string",
			goType:   "*string",
			expected: "string",
		},
		{
			name:     "pointer to int",
			goType:   "*int",
			expected: "integer",
		},
		{
			name:     "pointer to slice",
			goType:   "*[]string",
			expected: "array",
		},
		{
			name:     "slice of pointers",
			goType:   "[]*User",
			expected: "array",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schema := mapGoTypeToOpenAPISchema(tt.goType, meta, cfg)
			if schema.Type != tt.expected {
				t.Errorf("expected type %s, got %s", tt.expected, schema.Type)
			}

			// For pointer to struct, verify it generates inline properties
			if tt.goType == "*User" {
				if schema.Properties == nil {
					t.Error("expected properties for *User, got nil")
				}
				if len(schema.Properties) != 2 {
					t.Errorf("expected 2 properties for User, got %d", len(schema.Properties))
				}
				if schema.Properties["Name"] == nil {
					t.Error("expected Name property")
				}
				if schema.Properties["Age"] == nil {
					t.Error("expected Age property")
				}
			}

			// For slice of pointers, verify it generates array with proper items
			if tt.goType == "[]*User" {
				if schema.Items == nil {
					t.Error("expected items for array, got nil")
				}
				if schema.Items.Type != "object" {
					t.Errorf("expected object items for []*User, got %s", schema.Items.Type)
				}
			}
		})
	}
}

func TestAddTypeAndDependenciesWithMetadata_PointerTypes(t *testing.T) {
	// Create metadata with nested structs
	stringPool := metadata.NewStringPool()
	meta := &metadata.Metadata{
		StringPool: stringPool,
		Packages: map[string]*metadata.Package{
			"main": {
				Files: map[string]*metadata.File{
					"models.go": {
						Types: map[string]*metadata.Type{
							"User": {
								Name: stringPool.Get("User"),
								Kind: stringPool.Get("struct"),
								Fields: []metadata.Field{
									{
										Name: stringPool.Get("Profile"),
										Type: stringPool.Get("*Profile"),
									},
								},
							},
							"Profile": {
								Name: stringPool.Get("Profile"),
								Kind: stringPool.Get("struct"),
								Fields: []metadata.Field{
									{
										Name: stringPool.Get("Bio"),
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

	usedTypes := make(map[string]bool)
	addTypeAndDependenciesWithMetadata("*User", usedTypes, meta, nil)

	// Should include both User and Profile
	if !usedTypes["*User"] {
		t.Error("expected *User to be included")
	}
	if !usedTypes["*Profile"] {
		t.Error("expected *Profile to be included")
	}
	if !usedTypes["Profile"] {
		t.Error("expected Profile to be included")
	}
}

func TestFindTypeInMetadata_ExcludesPrimitiveTypes(t *testing.T) {
	// Create a simple metadata with a custom type
	stringPool := metadata.NewStringPool()
	meta := &metadata.Metadata{
		StringPool: stringPool,
		Packages: map[string]*metadata.Package{
			"main": {
				Files: map[string]*metadata.File{
					"user.go": {
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

	// Test primitive types - should return nil
	primitiveTypes := []string{
		"string", "int", "int32", "uint", "float64", "bool", "byte",
		"interface{}", "any", "error",
		"*string", "*int", "*bool", // pointer to primitives
		"[]string", "[]int", "[]bool", // slice of primitives
		"map[string]int", "map[string]string", // map of primitives
	}

	for _, primitiveType := range primitiveTypes {
		t.Run("primitive_"+primitiveType, func(t *testing.T) {
			result := findTypesInMetadata(meta, primitiveType)
			if result != nil {
				t.Errorf("Expected nil for primitive type '%s', got %v", primitiveType, result)
			}
		})
	}

	// Test custom type - should return the type
	t.Run("custom_type", func(t *testing.T) {
		result := findTypesInMetadata(meta, "User")
		if result == nil {
			t.Error("Expected User type to be found, got nil")
			return
		}
		if getStringFromPool(meta, result.Name) != "User" {
			t.Errorf("Expected User type, got %s", getStringFromPool(meta, result.Name))
		}
	})

	// Test qualified custom type - should return the type
	t.Run("qualified_custom_type", func(t *testing.T) {
		result := findTypesInMetadata(meta, "main-->User")
		if result == nil {
			t.Error("Expected main-->User type to be found, got nil")
			return
		}
		if getStringFromPool(meta, result.Name) != "User" {
			t.Errorf("Expected User type, got %s", getStringFromPool(meta, result.Name))
		}
	})
}

func TestFindTypeInMetadata_HandlesExternalTypes(t *testing.T) {
	// Create a simple metadata
	stringPool := metadata.NewStringPool()
	meta := &metadata.Metadata{
		StringPool: stringPool,
		Packages:   make(map[string]*metadata.Package),
	}

	// Test external types - should return nil (not found in metadata, but recognized as external)
	externalTypes := []string{
		"primitive.ObjectID",
		"primitive.DateTime",
		"*primitive.ObjectID",  // pointer to external type
		"[]primitive.ObjectID", // slice of external type
	}

	for _, externalType := range externalTypes {
		t.Run("external_"+externalType, func(t *testing.T) {
			result := findTypesInMetadata(meta, externalType)
			if result != nil {
				t.Errorf("Expected nil for external type '%s', got %v", externalType, result)
			}
		})
	}

	// Test unknown external type - should return nil
	t.Run("unknown_external_type", func(t *testing.T) {
		result := findTypesInMetadata(meta, "unknown.ExternalType")
		if result != nil {
			t.Errorf("Expected nil for unknown external type, got %v", result)
		}
	})

	// Test without config - should fall back to hardcoded types
	t.Run("external_without_config", func(t *testing.T) {
		result := findTypesInMetadata(meta, "primitive.ObjectID")
		if result != nil {
			t.Errorf("Expected nil for external type without config, got %v", result)
		}
	})
}

func TestMapGoTypeToOpenAPISchema_ExternalTypes(t *testing.T) {
	cfg := &SwagenConfig{
		ExternalTypes: []ExternalType{
			{
				Name: "primitive.ObjectID",
				OpenAPIType: &Schema{
					Type:   "string",
					Format: "objectid",
				},
			},
			{
				Name: "primitive.DateTime",
				OpenAPIType: &Schema{
					Type:   "string",
					Format: "date-time",
				},
			},
		},
	}

	tests := []struct {
		name     string
		goType   string
		expected *Schema
	}{
		{
			name:   "external_objectid",
			goType: "primitive.ObjectID",
			expected: &Schema{
				Ref: "#/components/schemas/primitive.ObjectID",
			},
		},
		{
			name:   "external_datetime",
			goType: "primitive.DateTime",
			expected: &Schema{
				Ref: "#/components/schemas/primitive.DateTime",
			},
		},
		{
			name:   "pointer_to_external",
			goType: "*primitive.ObjectID",
			expected: &Schema{
				Ref: "#/components/schemas/primitive.ObjectID",
			},
		},
		{
			name:   "slice_of_external",
			goType: "[]primitive.ObjectID",
			expected: &Schema{
				Type: "array",
				Items: &Schema{
					Ref: "#/components/schemas/primitive.ObjectID",
				},
			},
		},
		{
			name:   "unknown_external",
			goType: "unknown.ExternalType",
			expected: &Schema{
				Ref: "#/components/schemas/unknown.ExternalType",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mapGoTypeToOpenAPISchema(tt.goType, nil, cfg)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("mapGoTypeToOpenAPISchema() = %v, want %v", result, tt.expected)
			}
		})
	}
}
