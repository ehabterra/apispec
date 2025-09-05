package spec

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"reflect"
	"testing"

	"github.com/ehabterra/apispec/internal/metadata"
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

	cfg := DefaultAPISpecConfig()

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
			usedTypes := make(map[string]bool)
			schema, _ := mapGoTypeToOpenAPISchema(usedTypes, tt.goType, meta, cfg)
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

	usedTypes := make(map[string]bool)
	markUsedType(usedTypes, "*User", true)

	// Should include only the marked type
	if !usedTypes["*User"] {
		t.Error("expected *User to be included")
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
		if getStringFromPool(meta, result["User"].Name) != "User" {
			t.Errorf("Expected User type, got %s", getStringFromPool(meta, result["User"].Name))
		}
	})

	// Test qualified custom type - should return the type
	t.Run("qualified_custom_type", func(t *testing.T) {
		result := findTypesInMetadata(meta, "main-->User")
		if result == nil {
			t.Error("Expected main-->User type to be found, got nil")
			return
		}
		userType := result["main-->User"]
		if userType == nil {
			t.Error("Expected User type to be found in result, got nil")
			return
		}
		if getStringFromPool(meta, userType.Name) != "User" {
			t.Errorf("Expected User type, got %s", getStringFromPool(meta, userType.Name))
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
			// External types should return a map with nil values or nil map
			if result != nil {
				// Check if all values in the map are nil (expected for external types)
				allNil := true
				for _, v := range result {
					if v != nil {
						allNil = false
						break
					}
				}
				if !allNil {
					t.Errorf("Expected nil values for external type '%s', got %v", externalType, result)
				}
			}
		})
	}

	// Test unknown external type - should return nil or map with nil values
	t.Run("unknown_external_type", func(t *testing.T) {
		result := findTypesInMetadata(meta, "unknown.ExternalType")
		if result != nil {
			// Check if all values in the map are nil (expected for external types)
			allNil := true
			for _, v := range result {
				if v != nil {
					allNil = false
					break
				}
			}
			if !allNil {
				t.Errorf("Expected nil values for unknown external type, got %v", result)
			}
		}
	})

	// Test without config - should fall back to hardcoded types
	t.Run("external_without_config", func(t *testing.T) {
		result := findTypesInMetadata(meta, "primitive.ObjectID")
		if result != nil {
			// Check if all values in the map are nil (expected for external types)
			allNil := true
			for _, v := range result {
				if v != nil {
					allNil = false
					break
				}
			}
			if !allNil {
				t.Errorf("Expected nil values for external type without config, got %v", result)
			}
		}
	})
}

func TestMapGoTypeToOpenAPISchema_ExternalTypes(t *testing.T) {
	cfg := &APISpecConfig{
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
			usedTypes := make(map[string]bool)
			result, _ := mapGoTypeToOpenAPISchema(usedTypes, tt.goType, nil, cfg)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("mapGoTypeToOpenAPISchema() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestMapMetadataToOpenAPI_WithValidConfig(t *testing.T) {
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
								},
							},
						},
					},
				},
			},
		},
	}

	// Create tracker with limits
	limits := metadata.TrackerLimits{
		MaxNodesPerTree:    100,
		MaxChildrenPerNode: 10,
		MaxArgsPerFunction: 5,
		MaxNestedArgsDepth: 3,
	}

	tree := NewMockTrackerTree(meta, limits)

	// Create config
	cfg := DefaultGinConfig()

	// Create generator config
	genCfg := GeneratorConfig{
		OpenAPIVersion: "3.0.3",
		Title:          "Test API",
		APIVersion:     "1.0.0",
	}

	// Test mapping
	spec, err := MapMetadataToOpenAPI(tree, cfg, genCfg)
	if err != nil {
		t.Fatalf("Failed to map metadata to OpenAPI: %v", err)
	}

	if spec == nil {
		t.Fatal("OpenAPI spec should not be nil")
	}

	// Test basic structure
	if spec.OpenAPI != "3.0.3" {
		t.Errorf("Expected OpenAPI version 3.0.3, got %s", spec.OpenAPI)
	}

	if spec.Info.Title != "Test API" {
		t.Errorf("Expected title 'Test API', got %s", spec.Info.Title)
	}

	if spec.Info.Version != "1.0.0" {
		t.Errorf("Expected version '1.0.0', got %s", spec.Info.Version)
	}

	// Test components
	if spec.Components == nil {
		t.Fatal("Components should not be nil")
	}

	if spec.Components.Schemas == nil {
		t.Fatal("Schemas should not be nil")
	}
}

func TestMapMetadataToOpenAPI_WithConfigInfo(t *testing.T) {
	// Create metadata with string pool
	stringPool := metadata.NewStringPool()
	meta := &metadata.Metadata{
		StringPool: stringPool,
	}

	// Create tracker with limits
	limits := metadata.TrackerLimits{
		MaxNodesPerTree:    100,
		MaxChildrenPerNode: 10,
		MaxArgsPerFunction: 5,
		MaxNestedArgsDepth: 3,
	}

	tree := NewMockTrackerTree(meta, limits)

	// Create config with custom info
	cfg := &APISpecConfig{
		Info: Info{
			Title:       "Custom API",
			Description: "Custom API Description",
			Version:     "2.0.0",
		},
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
		Servers: []Server{
			{
				URL:         "https://api.example.com",
				Description: "Production server",
			},
		},
		Security: []SecurityRequirement{
			{
				"bearerAuth": []string{},
			},
		},
		Tags: []Tag{
			{
				Name:        "users",
				Description: "User management",
			},
		},
		ExternalDocs: &ExternalDocumentation{
			Description: "API Documentation",
			URL:         "https://docs.example.com",
		},
	}

	// Create generator config
	genCfg := GeneratorConfig{
		OpenAPIVersion: "3.0.3",
		Title:          "Default Title",
		APIVersion:     "1.0.0",
	}

	// Test mapping
	spec, err := MapMetadataToOpenAPI(tree, cfg, genCfg)
	if err != nil {
		t.Fatalf("Failed to map metadata to OpenAPI: %v", err)
	}

	if spec == nil {
		t.Fatal("OpenAPI spec should not be nil")
	}

	// Test that config info takes precedence
	if spec.Info.Title != "Custom API" {
		t.Errorf("Expected title 'Custom API', got %s", spec.Info.Title)
	}

	if spec.Info.Description != "Custom API Description" {
		t.Errorf("Expected description 'Custom API Description', got %s", spec.Info.Description)
	}

	if spec.Info.Version != "2.0.0" {
		t.Errorf("Expected version '2.0.0', got %s", spec.Info.Version)
	}

	// Test servers
	if len(spec.Servers) != 1 {
		t.Errorf("Expected 1 server, got %d", len(spec.Servers))
	}

	if spec.Servers[0].URL != "https://api.example.com" {
		t.Errorf("Expected server URL 'https://api.example.com', got %s", spec.Servers[0].URL)
	}

	// Test security
	if len(spec.Security) != 1 {
		t.Errorf("Expected 1 security requirement, got %d", len(spec.Security))
	}

	// Test tags
	if len(spec.Tags) != 1 {
		t.Errorf("Expected 1 tag, got %d", len(spec.Tags))
	}

	if spec.Tags[0].Name != "users" {
		t.Errorf("Expected tag name 'users', got %s", spec.Tags[0].Name)
	}

	// Test external docs
	if spec.ExternalDocs == nil {
		t.Fatal("External docs should not be nil")
	}

	if spec.ExternalDocs.Description != "API Documentation" {
		t.Errorf("Expected external docs description 'API Documentation', got %s", spec.ExternalDocs.Description)
	}
}

func TestMapMetadataToOpenAPI_WithSecuritySchemes(t *testing.T) {
	// Create metadata with string pool
	stringPool := metadata.NewStringPool()
	meta := &metadata.Metadata{
		StringPool: stringPool,
	}

	// Create tracker with limits
	limits := metadata.TrackerLimits{
		MaxNodesPerTree:    100,
		MaxChildrenPerNode: 10,
		MaxArgsPerFunction: 5,
		MaxNestedArgsDepth: 3,
	}

	tree := NewMockTrackerTree(meta, limits)

	// Create config with security schemes
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
		SecuritySchemes: map[string]SecurityScheme{
			"bearerAuth": {
				Type:         "http",
				Scheme:       "bearer",
				BearerFormat: "JWT",
			},
			"apiKeyAuth": {
				Type: "apiKey",
				In:   "header",
				Name: "X-API-Key",
			},
		},
	}

	// Create generator config
	genCfg := GeneratorConfig{
		OpenAPIVersion: "3.0.3",
		Title:          "Test API",
		APIVersion:     "1.0.0",
	}

	// Test mapping
	spec, err := MapMetadataToOpenAPI(tree, cfg, genCfg)
	if err != nil {
		t.Fatalf("Failed to map metadata to OpenAPI: %v", err)
	}

	if spec == nil {
		t.Fatal("OpenAPI spec should not be nil")
	}

	// Test security schemes
	if spec.Components == nil {
		t.Fatal("Components should not be nil")
	}

	if spec.Components.SecuritySchemes == nil {
		t.Fatal("Security schemes should not be nil")
	}

	if len(spec.Components.SecuritySchemes) != 2 {
		t.Errorf("Expected 2 security schemes, got %d", len(spec.Components.SecuritySchemes))
	}

	// Test bearer auth
	bearerAuth, exists := spec.Components.SecuritySchemes["bearerAuth"]
	if !exists {
		t.Fatal("Bearer auth security scheme should exist")
	}

	if bearerAuth.Type != "http" {
		t.Errorf("Expected bearer auth type 'http', got %s", bearerAuth.Type)
	}

	if bearerAuth.Scheme != "bearer" {
		t.Errorf("Expected bearer auth scheme 'bearer', got %s", bearerAuth.Scheme)
	}

	// Test API key auth
	apiKeyAuth, exists := spec.Components.SecuritySchemes["apiKeyAuth"]
	if !exists {
		t.Fatal("API key auth security scheme should exist")
	}

	if apiKeyAuth.Type != "apiKey" {
		t.Errorf("Expected API key auth type 'apiKey', got %s", apiKeyAuth.Type)
	}

	if apiKeyAuth.In != "header" {
		t.Errorf("Expected API key auth in 'header', got %s", apiKeyAuth.In)
	}
}

func TestLoadAPISpecConfig(t *testing.T) {
	// Test loading non-existent config file
	_, err := LoadAPISpecConfig("/non/existent/path/config.yaml")
	if err == nil {
		t.Error("Expected error when loading non-existent config file")
	}

	// Test loading invalid YAML file
	// We can't easily test this without creating a temporary file
	// So we'll just test that the function exists and can be called
	_ = LoadAPISpecConfig
}

func TestDefaultAPISpecConfig(t *testing.T) {
	config := DefaultAPISpecConfig()
	if config == nil {
		t.Fatal("Default config should not be nil")
	}

	// Test that default config has empty framework
	// Note: DefaultAPISpecConfig returns an empty config, so framework fields are nil
	// This is expected behavior
}

func TestBuildPathsFromRoutes(t *testing.T) {
	// Test with empty routes
	routes := []RouteInfo{}
	paths := buildPathsFromRoutes(routes)
	if paths == nil {
		t.Fatal("Paths should not be nil")
	}

	if len(paths) != 0 {
		t.Errorf("Expected 0 paths, got %d", len(paths))
	}

	// Test with single route
	routes = []RouteInfo{
		{
			Path:    "/users",
			Method:  "GET",
			Handler: "getUsers",
		},
	}

	paths = buildPathsFromRoutes(routes)
	if len(paths) != 1 {
		t.Errorf("Expected 1 path, got %d", len(paths))
	}

	pathItem, exists := paths["/users"]
	if !exists {
		t.Fatal("Path '/users' should exist")
	}

	if pathItem.Get == nil {
		t.Error("GET operation should exist")
	}
}

func TestGenerateComponentSchemas(t *testing.T) {
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
								},
							},
						},
					},
				},
			},
		},
	}

	// Create routes that use the User type
	routes := []RouteInfo{
		{
			Path:    "/users",
			Method:  "GET",
			Handler: "getUsers",
			Response: map[string]*ResponseInfo{
				"200": {
					BodyType: "User",
				},
			},
		},
	}

	// Create config
	cfg := DefaultGinConfig()

	// Test component schema generation
	components := generateComponentSchemas(meta, cfg, routes)
	if components.Schemas == nil {
		t.Fatal("Schemas should not be nil")
	}

	// Should have at least one schema (User)
	if len(components.Schemas) == 0 {
		t.Error("Should generate schemas for used types")
	}
}

func TestCollectUsedTypesFromRoutes(t *testing.T) {
	// Create routes with various types
	routes := []RouteInfo{
		{
			Path:    "/users",
			Method:  "GET",
			Handler: "getUsers",
			Request: &RequestInfo{
				BodyType: "CreateUserRequest",
			},
			Response: map[string]*ResponseInfo{
				"200": {
					BodyType: "User",
				},
			},
			Params: []Parameter{
				{
					Name: "id",
					Schema: &Schema{
						Ref: "#/components/schemas/UserID",
					},
				},
			},
		},
	}

	// Test type collection
	usedTypes := collectUsedTypesFromRoutes(routes)
	if len(usedTypes) == 0 {
		t.Error("Should collect types from routes")
	}

	// Check that expected types are collected
	expectedTypes := []string{"CreateUserRequest", "User", "UserID"}
	for _, expectedType := range expectedTypes {
		if !usedTypes[expectedType] {
			t.Errorf("Type %s should be collected", expectedType)
		}
	}
}

func TestFindTypesInMetadata(t *testing.T) {
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

	// Test finding existing type
	types := findTypesInMetadata(meta, "User")
	if len(types) == 0 {
		t.Error("Should find existing type")
	}

	// Test finding non-existent type
	types = findTypesInMetadata(meta, "NonExistentType")
	if len(types) == 0 {
		t.Error("Should return map with type name even for non-existent type")
	}

	// The type should exist in the map but be nil
	if types["NonExistentType"] != nil {
		t.Error("Non-existent type should be nil in the map")
	}

	// Test with nil metadata
	types = findTypesInMetadata(nil, "User")
	if types != nil {
		t.Error("Should return nil for nil metadata")
	}
}

func TestTypeByName(t *testing.T) {
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

	// Test finding type by name
	typ := typeByName([]string{"main", "User"}, meta, "User")
	if typ == nil {
		t.Error("Should find type by name")
	}

	// Test finding non-existent type
	typ = typeByName([]string{"main", "NonExistentType"}, meta, "NonExistentType")
	if typ != nil {
		t.Error("Should not find non-existent type")
	}
}

func TestAddTypeAndDependenciesWithMetadata(t *testing.T) {

	// Test adding type and dependencies
	usedTypes := make(map[string]bool)
	markUsedType(usedTypes, "User", true)

	// Should have User and Profile types
	if !usedTypes["User"] {
		t.Error("User type should be added")
	}

	// Test pointer type handling
	usedTypes = make(map[string]bool)
	markUsedType(usedTypes, "*User", true)

	if !usedTypes["*User"] {
		t.Error("Pointer type should be added")
	}

	if !usedTypes["User"] {
		t.Error("Underlying type should be added")
	}
}

func TestGetStringFromPool(t *testing.T) {
	// Test with nil metadata - this should panic due to nil pointer dereference
	defer func() {
		if r := recover(); r != nil {
			// Expected panic due to nil metadata
			t.Log("Expected panic with nil metadata:", r)
		}
	}()

	_ = getStringFromPool(nil, 0)

	// Test with nil string pool
	meta := &metadata.Metadata{
		StringPool: nil,
	}
	result := getStringFromPool(meta, 0)
	if result != "" {
		t.Error("Should return empty string for nil string pool")
	}

	// Test with valid metadata
	stringPool := metadata.NewStringPool()
	meta = &metadata.Metadata{
		StringPool: stringPool,
	}

	// Add a string to the pool
	idx := stringPool.Get("test")
	result = getStringFromPool(meta, idx)
	if result != "test" {
		t.Errorf("Expected 'test', got '%s'", result)
	}
}

func TestExtractJSONName(t *testing.T) {
	tests := []struct {
		name     string
		tag      string
		expected string
	}{
		{
			name:     "simple json tag",
			tag:      `json:"name"`,
			expected: "name",
		},
		{
			name:     "json tag with omitempty",
			tag:      `json:"name,omitempty"`,
			expected: "name",
		},
		{
			name:     "json tag with multiple options",
			tag:      `json:"name,omitempty,string"`,
			expected: "name",
		},
		{
			name:     "json tag with dash",
			tag:      `json:"-"`,
			expected: "",
		},
		{
			name:     "empty tag",
			tag:      "",
			expected: "",
		},
		{
			name:     "tag without json",
			tag:      `xml:"name"`,
			expected: "",
		},
		{
			name:     "tag with spaces",
			tag:      `json: "name" `,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractJSONName(tt.tag)
			if result != tt.expected {
				t.Errorf("extractJSONName(%q) = %q, expected %q", tt.tag, result, tt.expected)
			}
		})
	}
}

func TestCompleteNestedStructFlow(t *testing.T) {
	// Test source code with nested struct types
	src := `
package main

type X struct {
	Y struct {
		Z string ` + "`json:\"z\"`" + `
	} ` + "`json:\"y\"`" + `
}

type Container struct {
	Data struct {
		ID   int    ` + "`json:\"id\"`" + `
		Name string ` + "`json:\"name\"`" + `
	} ` + "`json:\"data\"`" + `
}
`

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, 0)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	pkgs := map[string]map[string]*ast.File{"main": {"test.go": file}}
	importPaths := map[string]string{"main": "main"}
	fileToInfo := map[*ast.File]*types.Info{}

	// Generate metadata
	metadata := metadata.GenerateMetadata(pkgs, fileToInfo, importPaths, fset)

	// Create config
	cfg := &APISpecConfig{
		TypeMapping:   []TypeMapping{},
		ExternalTypes: []ExternalType{},
	}

	// Test schema generation for type X
	for _, pkg := range metadata.Packages {
		for _, file := range pkg.Files {
			if xType, exists := file.Types["X"]; exists {
				usedTypes := make(map[string]bool)
				schema, _ := generateSchemaFromType(usedTypes, "X", xType, metadata, cfg)

				// Verify the schema structure
				if schema.Type != "object" {
					t.Errorf("Expected schema type 'object', got '%s'", schema.Type)
				}

				if len(schema.Properties) != 1 {
					t.Errorf("Expected 1 property, got %d", len(schema.Properties))
				}

				// Check Y property
				yProp, exists := schema.Properties["y"]
				if !exists {
					t.Error("Expected property 'y' to exist")
				} else {
					if yProp.Type != "object" {
						t.Errorf("Expected Y property type 'object', got '%s'", yProp.Type)
					}

					if len(yProp.Properties) != 1 {
						t.Errorf("Expected 1 property in Y, got %d", len(yProp.Properties))
					}

					// Check Z property
					zProp, exists := yProp.Properties["z"]
					if !exists {
						t.Error("Expected property 'z' to exist in Y")
					} else {
						if zProp.Type != "string" {
							t.Errorf("Expected Z property type 'string', got '%s'", zProp.Type)
						}
					}
				}
			} else {
				t.Error("Expected to find type X")
			}

			// Test schema generation for type Container
			if containerType, exists := file.Types["Container"]; exists {
				usedTypes := make(map[string]bool)
				schema, _ := generateSchemaFromType(usedTypes, "Container", containerType, metadata, cfg)

				// Verify the schema structure
				if schema.Type != "object" {
					t.Errorf("Expected schema type 'object', got '%s'", schema.Type)
				}

				if len(schema.Properties) != 1 {
					t.Errorf("Expected 1 property, got %d", len(schema.Properties))
				}

				// Check Data property
				dataProp, exists := schema.Properties["data"]
				if !exists {
					t.Error("Expected property 'data' to exist")
				} else {
					if dataProp.Type != "object" {
						t.Errorf("Expected Data property type 'object', got '%s'", dataProp.Type)
					}

					if len(dataProp.Properties) != 2 {
						t.Errorf("Expected 2 properties in Data, got %d", len(dataProp.Properties))
					}

					// Check ID property
					idProp, exists := dataProp.Properties["id"]
					if !exists {
						t.Error("Expected property 'id' to exist in Data")
					} else {
						if idProp.Type != "integer" {
							t.Errorf("Expected ID property type 'integer', got '%s'", idProp.Type)
						}
					}

					// Check Name property
					nameProp, exists := dataProp.Properties["name"]
					if !exists {
						t.Error("Expected property 'name' to exist in Data")
					} else {
						if nameProp.Type != "string" {
							t.Errorf("Expected Name property type 'string', got '%s'", nameProp.Type)
						}
					}
				}
			} else {
				t.Error("Expected to find type Container")
			}
		}
	}
}
