package spec

import (
	"regexp"
	"strings"
	"testing"

	"github.com/ehabterra/apispec/internal/metadata"
)

// TestMapMetadataToOpenAPI_Comprehensive tests the main mapping function with various scenarios
func TestMapMetadataToOpenAPI_Comprehensive(t *testing.T) {
	tests := []struct {
		name        string
		description string
		testFunc    func(t *testing.T)
	}{
		{"nil_tree", "Should handle nil tracker tree", testMapMetadataToOpenAPI_NilTree},
		{"empty_routes", "Should handle empty routes", testMapMetadataToOpenAPI_EmptyRoutes},
		{"with_config_info", "Should use config info when present", testMapMetadataToOpenAPI_WithConfigInfo},
		{"with_security_schemes", "Should include security schemes", testMapMetadataToOpenAPI_WithSecuritySchemes},
		{"with_external_docs", "Should include external docs", testMapMetadataToOpenAPI_WithExternalDocs},
		{"with_servers", "Should include servers", testMapMetadataToOpenAPI_WithServers},
		{"with_tags", "Should include tags", testMapMetadataToOpenAPI_WithTags},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.testFunc(t)
		})
	}
}

func testMapMetadataToOpenAPI_NilTree(t *testing.T) {
	cfg := DefaultAPISpecConfig()
	genCfg := GeneratorConfig{
		OpenAPIVersion: "3.0.3",
		Title:          "Test API",
		APIVersion:     "1.0.0",
	}

	// This should panic because NewExtractor doesn't handle nil tree
	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected panic when tree is nil")
		}
	}()

	_, err := MapMetadataToOpenAPI(nil, cfg, genCfg)
	if err != nil {
		t.Errorf("Expected error for nil metadata, got: %v", err)
	}
}

func testMapMetadataToOpenAPI_EmptyRoutes(t *testing.T) {
	// Create a mock tree that returns empty routes
	mockTree := &MockTrackerTree{
		meta: &metadata.Metadata{
			StringPool: metadata.NewStringPool(),
			Packages:   make(map[string]*metadata.Package),
		},
	}

	cfg := DefaultAPISpecConfig()
	genCfg := GeneratorConfig{
		OpenAPIVersion: "3.0.3",
		Title:          "Test API",
		APIVersion:     "1.0.0",
	}

	spec, err := MapMetadataToOpenAPI(mockTree, cfg, genCfg)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if spec == nil {
		t.Fatal("Expected non-nil spec")
	}

	if len(spec.Paths) != 0 {
		t.Errorf("Expected empty paths, got %d", len(spec.Paths))
	}
}

func testMapMetadataToOpenAPI_WithConfigInfo(t *testing.T) {
	mockTree := &MockTrackerTree{
		meta: &metadata.Metadata{
			StringPool: metadata.NewStringPool(),
			Packages:   make(map[string]*metadata.Package),
		},
	}

	cfg := &APISpecConfig{
		Info: Info{
			Title:       "Config Title",
			Description: "Config Description",
			Version:     "2.0.0",
		},
	}

	genCfg := GeneratorConfig{
		OpenAPIVersion: "3.0.3",
		Title:          "Generator Title", // Should be overridden
		APIVersion:     "1.0.0",           // Should be overridden
	}

	spec, err := MapMetadataToOpenAPI(mockTree, cfg, genCfg)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if spec.Info.Title != "Config Title" {
		t.Errorf("Expected title 'Config Title', got %s", spec.Info.Title)
	}

	if spec.Info.Description != "Config Description" {
		t.Errorf("Expected description 'Config Description', got %s", spec.Info.Description)
	}

	if spec.Info.Version != "2.0.0" {
		t.Errorf("Expected version '2.0.0', got %s", spec.Info.Version)
	}
}

func testMapMetadataToOpenAPI_WithSecuritySchemes(t *testing.T) {
	mockTree := &MockTrackerTree{
		meta: &metadata.Metadata{
			StringPool: metadata.NewStringPool(),
			Packages:   make(map[string]*metadata.Package),
		},
	}

	cfg := &APISpecConfig{
		SecuritySchemes: map[string]SecurityScheme{
			"bearerAuth": {
				Type:         "http",
				Scheme:       "bearer",
				BearerFormat: "JWT",
			},
		},
	}

	genCfg := GeneratorConfig{
		OpenAPIVersion: "3.0.3",
		Title:          "Test API",
		APIVersion:     "1.0.0",
	}

	spec, err := MapMetadataToOpenAPI(mockTree, cfg, genCfg)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if spec.Components.SecuritySchemes == nil {
		t.Fatal("Expected security schemes to be set")
	}

	if len(spec.Components.SecuritySchemes) != 1 {
		t.Errorf("Expected 1 security scheme, got %d", len(spec.Components.SecuritySchemes))
	}

	if spec.Components.SecuritySchemes["bearerAuth"].Type != "http" {
		t.Errorf("Expected security scheme type 'http', got %s", spec.Components.SecuritySchemes["bearerAuth"].Type)
	}
}

func testMapMetadataToOpenAPI_WithExternalDocs(t *testing.T) {
	mockTree := &MockTrackerTree{
		meta: &metadata.Metadata{
			StringPool: metadata.NewStringPool(),
			Packages:   make(map[string]*metadata.Package),
		},
	}

	cfg := &APISpecConfig{
		ExternalDocs: &ExternalDocumentation{
			Description: "API Documentation",
			URL:         "https://example.com/docs",
		},
	}

	genCfg := GeneratorConfig{
		OpenAPIVersion: "3.0.3",
		Title:          "Test API",
		APIVersion:     "1.0.0",
	}

	spec, err := MapMetadataToOpenAPI(mockTree, cfg, genCfg)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if spec.ExternalDocs == nil {
		t.Fatal("Expected external docs to be set")
	}

	if spec.ExternalDocs.Description != "API Documentation" {
		t.Errorf("Expected external docs description 'API Documentation', got %s", spec.ExternalDocs.Description)
	}

	if spec.ExternalDocs.URL != "https://example.com/docs" {
		t.Errorf("Expected external docs URL 'https://example.com/docs', got %s", spec.ExternalDocs.URL)
	}
}

func testMapMetadataToOpenAPI_WithServers(t *testing.T) {
	mockTree := &MockTrackerTree{
		meta: &metadata.Metadata{
			StringPool: metadata.NewStringPool(),
			Packages:   make(map[string]*metadata.Package),
		},
	}

	cfg := &APISpecConfig{
		Servers: []Server{
			{
				URL:         "https://api.example.com/v1",
				Description: "Production server",
			},
		},
	}

	genCfg := GeneratorConfig{
		OpenAPIVersion: "3.0.3",
		Title:          "Test API",
		APIVersion:     "1.0.0",
	}

	spec, err := MapMetadataToOpenAPI(mockTree, cfg, genCfg)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if len(spec.Servers) != 1 {
		t.Errorf("Expected 1 server, got %d", len(spec.Servers))
	}

	if spec.Servers[0].URL != "https://api.example.com/v1" {
		t.Errorf("Expected server URL 'https://api.example.com/v1', got %s", spec.Servers[0].URL)
	}
}

func testMapMetadataToOpenAPI_WithTags(t *testing.T) {
	mockTree := &MockTrackerTree{
		meta: &metadata.Metadata{
			StringPool: metadata.NewStringPool(),
			Packages:   make(map[string]*metadata.Package),
		},
	}

	cfg := &APISpecConfig{
		Tags: []Tag{
			{
				Name:        "users",
				Description: "User management operations",
			},
		},
	}

	genCfg := GeneratorConfig{
		OpenAPIVersion: "3.0.3",
		Title:          "Test API",
		APIVersion:     "1.0.0",
	}

	spec, err := MapMetadataToOpenAPI(mockTree, cfg, genCfg)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if len(spec.Tags) != 1 {
		t.Errorf("Expected 1 tag, got %d", len(spec.Tags))
	}

	if spec.Tags[0].Name != "users" {
		t.Errorf("Expected tag name 'users', got %s", spec.Tags[0].Name)
	}
}

// TestBuildPathsFromRoutes_Comprehensive tests path building with various scenarios
func TestBuildPathsFromRoutes_Comprehensive(t *testing.T) {
	tests := []struct {
		name        string
		description string
		testFunc    func(t *testing.T)
	}{
		{"empty_routes", "Should handle empty routes", testBuildPathsFromRoutes_Empty},
		{"single_route", "Should handle single route", testBuildPathsFromRoutes_Single},
		{"multiple_routes_same_path", "Should handle multiple routes on same path", testBuildPathsFromRoutes_MultipleSamePath},
		{"path_with_params", "Should convert path parameters", testBuildPathsFromRoutes_WithParams},
		{"all_http_methods", "Should handle all HTTP methods", testBuildPathsFromRoutes_AllMethods},
		{"with_request_body", "Should handle request body", testBuildPathsFromRoutes_WithRequestBody},
		{"with_parameters", "Should handle parameters", testBuildPathsFromRoutes_WithParameters},
		{"with_responses", "Should handle responses", testBuildPathsFromRoutes_WithResponses},
		{"with_package_prefix", "Should handle package prefix", testBuildPathsFromRoutes_WithPackagePrefix},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.testFunc(t)
		})
	}
}

func testBuildPathsFromRoutes_Empty(t *testing.T) {
	routes := []RouteInfo{}
	paths := buildPathsFromRoutes(routes)

	if len(paths) != 0 {
		t.Errorf("Expected empty paths, got %d", len(paths))
	}
}

func testBuildPathsFromRoutes_Single(t *testing.T) {
	routes := []RouteInfo{
		{
			Path:     "/users",
			Method:   "GET",
			Function: "GetUsers",
			Package:  "main",
		},
	}

	paths := buildPathsFromRoutes(routes)

	if len(paths) != 1 {
		t.Errorf("Expected 1 path, got %d", len(paths))
	}

	pathItem, exists := paths["/users"]
	if !exists {
		t.Fatal("Expected path '/users' to exist")
	}

	if pathItem.Get == nil {
		t.Fatal("Expected GET operation to exist")
	}

	if pathItem.Get.OperationID != "main.GetUsers" {
		t.Errorf("Expected operation ID 'main.GetUsers', got %s", pathItem.Get.OperationID)
	}
}

func testBuildPathsFromRoutes_MultipleSamePath(t *testing.T) {
	routes := []RouteInfo{
		{
			Path:     "/users",
			Method:   "GET",
			Function: "GetUsers",
			Package:  "main",
		},
		{
			Path:     "/users",
			Method:   "POST",
			Function: "CreateUser",
			Package:  "main",
		},
	}

	paths := buildPathsFromRoutes(routes)

	if len(paths) != 1 {
		t.Errorf("Expected 1 path, got %d", len(paths))
	}

	pathItem, exists := paths["/users"]
	if !exists {
		t.Fatal("Expected path '/users' to exist")
	}

	if pathItem.Get == nil {
		t.Fatal("Expected GET operation to exist")
	}

	if pathItem.Post == nil {
		t.Fatal("Expected POST operation to exist")
	}
}

func testBuildPathsFromRoutes_WithParams(t *testing.T) {
	routes := []RouteInfo{
		{
			Path:     "/users/:id",
			Method:   "GET",
			Function: "GetUser",
			Package:  "main",
		},
	}

	paths := buildPathsFromRoutes(routes)

	pathItem, exists := paths["/users/{id}"]
	if !exists {
		t.Fatal("Expected path '/users/{id}' to exist")
	}

	if pathItem.Get == nil {
		t.Fatal("Expected GET operation to exist")
	}
}

func testBuildPathsFromRoutes_AllMethods(t *testing.T) {
	methods := []string{"GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS", "HEAD"}
	routes := make([]RouteInfo, len(methods))

	for i, method := range methods {
		routes[i] = RouteInfo{
			Path:     "/test",
			Method:   method,
			Function: method + "Test",
			Package:  "main",
		}
	}

	paths := buildPathsFromRoutes(routes)

	pathItem, exists := paths["/test"]
	if !exists {
		t.Fatal("Expected path '/test' to exist")
	}

	if pathItem.Get == nil {
		t.Error("Expected GET operation to exist")
	}
	if pathItem.Post == nil {
		t.Error("Expected POST operation to exist")
	}
	if pathItem.Put == nil {
		t.Error("Expected PUT operation to exist")
	}
	if pathItem.Delete == nil {
		t.Error("Expected DELETE operation to exist")
	}
	if pathItem.Patch == nil {
		t.Error("Expected PATCH operation to exist")
	}
	if pathItem.Options == nil {
		t.Error("Expected OPTIONS operation to exist")
	}
	if pathItem.Head == nil {
		t.Error("Expected HEAD operation to exist")
	}
}

func testBuildPathsFromRoutes_WithRequestBody(t *testing.T) {
	routes := []RouteInfo{
		{
			Path:     "/users",
			Method:   "POST",
			Function: "CreateUser",
			Package:  "main",
			Request: &RequestInfo{
				ContentType: "application/json",
				Schema:      &Schema{Type: "object"},
			},
		},
	}

	paths := buildPathsFromRoutes(routes)

	pathItem, exists := paths["/users"]
	if !exists {
		t.Fatal("Expected path '/users' to exist")
	}

	if pathItem.Post == nil {
		t.Fatal("Expected POST operation to exist")
	}

	if pathItem.Post.RequestBody == nil {
		t.Fatal("Expected request body to exist")
	}

	if pathItem.Post.RequestBody.Content["application/json"].Schema.Type != "object" {
		t.Errorf("Expected schema type 'object', got %s", pathItem.Post.RequestBody.Content["application/json"].Schema.Type)
	}
}

func testBuildPathsFromRoutes_WithParameters(t *testing.T) {
	routes := []RouteInfo{
		{
			Path:     "/users",
			Method:   "GET",
			Function: "GetUsers",
			Package:  "main",
			Params: []Parameter{
				{
					Name:     "limit",
					In:       "query",
					Required: false,
					Schema:   &Schema{Type: "integer"},
				},
			},
		},
	}

	paths := buildPathsFromRoutes(routes)

	pathItem, exists := paths["/users"]
	if !exists {
		t.Fatal("Expected path '/users' to exist")
	}

	if pathItem.Get == nil {
		t.Fatal("Expected GET operation to exist")
	}

	if len(pathItem.Get.Parameters) != 1 {
		t.Errorf("Expected 1 parameter, got %d", len(pathItem.Get.Parameters))
	}

	if pathItem.Get.Parameters[0].Name != "limit" {
		t.Errorf("Expected parameter name 'limit', got %s", pathItem.Get.Parameters[0].Name)
	}
}

func testBuildPathsFromRoutes_WithResponses(t *testing.T) {
	routes := []RouteInfo{
		{
			Path:     "/users",
			Method:   "GET",
			Function: "GetUsers",
			Package:  "main",
			Response: map[string]*ResponseInfo{
				"200": {
					StatusCode:  200,
					ContentType: "application/json",
					Schema:      &Schema{Type: "array"},
				},
			},
		},
	}

	paths := buildPathsFromRoutes(routes)

	pathItem, exists := paths["/users"]
	if !exists {
		t.Fatal("Expected path '/users' to exist")
	}

	if pathItem.Get == nil {
		t.Fatal("Expected GET operation to exist")
	}

	if len(pathItem.Get.Responses) != 1 {
		t.Errorf("Expected 1 response, got %d", len(pathItem.Get.Responses))
	}

	response, exists := pathItem.Get.Responses["200"]
	if !exists {
		t.Fatal("Expected response '200' to exist")
	}

	if response.Content["application/json"].Schema.Type != "array" {
		t.Errorf("Expected schema type 'array', got %s", response.Content["application/json"].Schema.Type)
	}
}

func testBuildPathsFromRoutes_WithPackagePrefix(t *testing.T) {
	routes := []RouteInfo{
		{
			Path:     "/users",
			Method:   "GET",
			Function: "GetUsers",
			Package:  "api.v1",
		},
	}

	paths := buildPathsFromRoutes(routes)

	pathItem, exists := paths["/users"]
	if !exists {
		t.Fatal("Expected path '/users' to exist")
	}

	if pathItem.Get == nil {
		t.Fatal("Expected GET operation to exist")
	}

	if pathItem.Get.OperationID != "api.v1.GetUsers" {
		t.Errorf("Expected operation ID 'api.v1.GetUsers', got %s", pathItem.Get.OperationID)
	}
}

// TestConvertPathToOpenAPI_Comprehensive tests path conversion
func TestConvertPathToOpenAPI_Comprehensive(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"/users", "/users"},
		{"/users/:id", "/users/{id}"},
		{"/users/:id/posts/:postId", "/users/{id}/posts/{postId}"},
		{"/users/:userId/comments/:commentId", "/users/{userId}/comments/{commentId}"},
		{"/api/v1/users/:id", "/api/v1/users/{id}"},
		{"/users/:id/posts/:postId/comments/:commentId", "/users/{id}/posts/{postId}/comments/{commentId}"},
		{"/:param", "/{param}"},
		{"/:param1/:param2", "/{param1}/{param2}"},
		{"/users/:id/posts", "/users/{id}/posts"},
		{"/posts/:id", "/posts/{id}"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := convertPathToOpenAPI(tt.input)
			if result != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, result)
			}
		})
	}
}

// TestEnsureAllPathParams_Comprehensive tests path parameter handling
func TestEnsureAllPathParams_Comprehensive(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		params   []Parameter
		expected int // expected number of parameters
	}{
		{
			name:     "no_params_needed",
			path:     "/users",
			params:   []Parameter{},
			expected: 0,
		},
		{
			name:     "missing_path_param",
			path:     "/users/{id}",
			params:   []Parameter{},
			expected: 1,
		},
		{
			name: "existing_path_param",
			path: "/users/{id}",
			params: []Parameter{
				{Name: "id", In: "path", Required: true},
			},
			expected: 1,
		},
		{
			name: "mixed_params",
			path: "/users/{id}/posts/{postId}",
			params: []Parameter{
				{Name: "id", In: "path", Required: true},
			},
			expected: 2,
		},
		{
			name: "query_param_ignored",
			path: "/users/{id}",
			params: []Parameter{
				{Name: "limit", In: "query", Required: false},
			},
			expected: 2, // 1 existing + 1 missing path param
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ensureAllPathParams(tt.path, tt.params)
			if len(result) != tt.expected {
				t.Errorf("Expected %d parameters, got %d", tt.expected, len(result))
			}

			// Check that all path parameters are present
			pathParams := make(map[string]bool)
			for _, p := range result {
				if p.In == "path" {
					pathParams[p.Name] = true
				}
			}

			// Extract expected path parameters from the path
			re := regexp.MustCompile(`\{([a-zA-Z_][a-zA-Z0-9_]*)\}`)
			matches := re.FindAllStringSubmatch(tt.path, -1)
			for _, match := range matches {
				paramName := match[1]
				if !pathParams[paramName] {
					t.Errorf("Expected path parameter '%s' to be present", paramName)
				}
			}
		})
	}
}

// TestDeduplicateParameters_Comprehensive tests parameter deduplication
func TestDeduplicateParameters_Comprehensive(t *testing.T) {
	tests := []struct {
		name     string
		params   []Parameter
		expected int
	}{
		{
			name:     "empty_params",
			params:   []Parameter{},
			expected: 0,
		},
		{
			name: "no_duplicates",
			params: []Parameter{
				{Name: "id", In: "path"},
				{Name: "limit", In: "query"},
			},
			expected: 2,
		},
		{
			name: "duplicate_name_different_in",
			params: []Parameter{
				{Name: "id", In: "path"},
				{Name: "id", In: "query"},
			},
			expected: 2,
		},
		{
			name: "duplicate_name_same_in",
			params: []Parameter{
				{Name: "id", In: "path"},
				{Name: "id", In: "path"},
			},
			expected: 1,
		},
		{
			name: "multiple_duplicates",
			params: []Parameter{
				{Name: "id", In: "path"},
				{Name: "id", In: "path"},
				{Name: "limit", In: "query"},
				{Name: "limit", In: "query"},
				{Name: "offset", In: "query"},
			},
			expected: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := deduplicateParameters(tt.params)
			if len(result) != tt.expected {
				t.Errorf("Expected %d parameters, got %d", tt.expected, len(result))
			}
		})
	}
}

// TestBuildResponses_Comprehensive tests response building
func TestBuildResponses_Comprehensive(t *testing.T) {
	tests := []struct {
		name     string
		respInfo map[string]*ResponseInfo
		expected int
	}{
		{
			name:     "nil_responses",
			respInfo: nil,
			expected: 1, // Default 200 response
		},
		{
			name:     "empty_responses",
			respInfo: map[string]*ResponseInfo{},
			expected: 0, // No default response for empty map
		},
		{
			name: "single_response",
			respInfo: map[string]*ResponseInfo{
				"200": {
					StatusCode:  200,
					ContentType: "application/json",
					Schema:      &Schema{Type: "object"},
				},
			},
			expected: 1,
		},
		{
			name: "multiple_responses",
			respInfo: map[string]*ResponseInfo{
				"200": {
					StatusCode:  200,
					ContentType: "application/json",
					Schema:      &Schema{Type: "object"},
				},
				"404": {
					StatusCode:  404,
					ContentType: "application/json",
					Schema:      &Schema{Type: "object"},
				},
			},
			expected: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildResponses(tt.respInfo)
			if len(result) != tt.expected {
				t.Errorf("Expected %d responses, got %d", tt.expected, len(result))
			}

			if tt.respInfo == nil {
				// Check default response
				if response, exists := result["200"]; !exists {
					t.Error("Expected default 200 response")
				} else if response.Description != "Success" {
					t.Errorf("Expected description 'Success', got %s", response.Description)
				}
			}
		})
	}
}

// TestSetOperationOnPathItem_Comprehensive tests operation setting
func TestSetOperationOnPathItem_Comprehensive(t *testing.T) {
	methods := []string{"GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS", "HEAD", "INVALID"}

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			item := &PathItem{}
			operation := &Operation{OperationID: method + "Test"}

			setOperationOnPathItem(item, method, operation)

			switch strings.ToUpper(method) {
			case "GET":
				if item.Get != operation {
					t.Error("Expected GET operation to be set")
				}
			case "POST":
				if item.Post != operation {
					t.Error("Expected POST operation to be set")
				}
			case "PUT":
				if item.Put != operation {
					t.Error("Expected PUT operation to be set")
				}
			case "DELETE":
				if item.Delete != operation {
					t.Error("Expected DELETE operation to be set")
				}
			case "PATCH":
				if item.Patch != operation {
					t.Error("Expected PATCH operation to be set")
				}
			case "OPTIONS":
				if item.Options != operation {
					t.Error("Expected OPTIONS operation to be set")
				}
			case "HEAD":
				if item.Head != operation {
					t.Error("Expected HEAD operation to be set")
				}
			default:
				// Invalid method should not set any operation
				if item.Get != nil || item.Post != nil || item.Put != nil || item.Delete != nil ||
					item.Patch != nil || item.Options != nil || item.Head != nil {
					t.Error("Expected no operation to be set for invalid method")
				}
			}
		})
	}
}

// TestMapGoTypeToOpenAPISchema_Comprehensive tests type mapping with various scenarios
func TestMapGoTypeToOpenAPISchema_Comprehensive(t *testing.T) {
	tests := []struct {
		name        string
		description string
		testFunc    func(t *testing.T)
	}{
		{"primitive_types", "Should handle all primitive types", testMapGoTypeToOpenAPISchema_PrimitiveTypes},
		{"pointer_types", "Should handle pointer types", testMapGoTypeToOpenAPISchema_PointerTypes},
		{"slice_types", "Should handle slice types", testMapGoTypeToOpenAPISchema_SliceTypes},
		{"map_types", "Should handle map types", testMapGoTypeToOpenAPISchema_MapTypes},
		{"custom_types", "Should handle custom types", testMapGoTypeToOpenAPISchema_CustomTypes},
		{"external_types", "Should handle external types", testMapGoTypeToOpenAPISchema_ExternalTypes},
		{"type_mappings", "Should handle type mappings", testMapGoTypeToOpenAPISchema_TypeMappings},
		{"nil_metadata", "Should handle nil metadata", testMapGoTypeToOpenAPISchema_NilMetadata},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.testFunc(t)
		})
	}
}

func testMapGoTypeToOpenAPISchema_PrimitiveTypes(t *testing.T) {
	cfg := DefaultAPISpecConfig()
	usedTypes := make(map[string]*Schema)

	primitiveTests := []struct {
		goType       string
		expectedType string
	}{
		{"string", "string"},
		{"int", "integer"},
		{"int8", "integer"},
		{"int16", "integer"},
		{"int32", "integer"},
		{"int64", "integer"},
		{"uint", "integer"},
		{"uint8", "integer"},
		{"uint16", "integer"},
		{"uint32", "integer"},
		{"uint64", "integer"},
		{"byte", "integer"},
		{"float32", "number"},
		{"float64", "number"},
		{"bool", "boolean"},
		{"time.Time", "string"},
		{"[]byte", "array"},
		{"[]string", "array"},
		{"[]time.Time", "array"},
		{"[]int", "array"},
		{"interface{}", "object"},
		{"struct{}", "object"},
		{"any", "object"},
	}

	for _, tt := range primitiveTests {
		t.Run(tt.goType, func(t *testing.T) {
			schema, _ := mapGoTypeToOpenAPISchema(usedTypes, tt.goType, nil, cfg)
			if schema.Type != tt.expectedType {
				t.Errorf("Expected type %s for %s, got %s", tt.expectedType, tt.goType, schema.Type)
			}
		})
	}
}

func testMapGoTypeToOpenAPISchema_PointerTypes(t *testing.T) {
	cfg := DefaultAPISpecConfig()
	usedTypes := make(map[string]*Schema)

	pointerTests := []struct {
		goType       string
		expectedType string
	}{
		{"*string", "string"},
		{"*int", "integer"},
		{"*bool", "boolean"},
		{"*time.Time", "string"},
	}

	for _, tt := range pointerTests {
		t.Run(tt.goType, func(t *testing.T) {
			schema, _ := mapGoTypeToOpenAPISchema(usedTypes, tt.goType, nil, cfg)
			if schema.Type != tt.expectedType {
				t.Errorf("Expected type %s for %s, got %s", tt.expectedType, tt.goType, schema.Type)
			}
		})
	}
}

func testMapGoTypeToOpenAPISchema_SliceTypes(t *testing.T) {
	cfg := DefaultAPISpecConfig()
	usedTypes := make(map[string]*Schema)

	sliceTests := []struct {
		goType            string
		expectedType      string
		expectedItemsType string
	}{
		{"[]string", "array", "string"},
		{"[]int", "array", "integer"},
		{"[]bool", "array", "boolean"},
		{"[]*User", "array", ""},
	}

	for _, tt := range sliceTests {
		t.Run(tt.goType, func(t *testing.T) {
			schema, _ := mapGoTypeToOpenAPISchema(usedTypes, tt.goType, nil, cfg)
			if schema.Type != tt.expectedType {
				t.Errorf("Expected type %s for %s, got %s", tt.expectedType, tt.goType, schema.Type)
			}
			if schema.Items == nil {
				t.Errorf("Expected items schema for %s", tt.goType)
			} else if schema.Items.Type != tt.expectedItemsType {
				t.Errorf("Expected items type %s for %s, got %s", tt.expectedItemsType, tt.goType, schema.Items.Type)
			}
		})
	}
}

func testMapGoTypeToOpenAPISchema_MapTypes(t *testing.T) {
	cfg := DefaultAPISpecConfig()
	usedTypes := make(map[string]*Schema)

	mapTests := []struct {
		goType                  string
		expectedType            string
		expectedAdditionalProps bool
	}{
		{"map[string]string", "object", true},
		{"map[string]int", "object", true},
		{"map[string]bool", "object", true},
		{"map[int]string", "object", false}, // Non-string keys not supported
	}

	for _, tt := range mapTests {
		t.Run(tt.goType, func(t *testing.T) {
			schema, _ := mapGoTypeToOpenAPISchema(usedTypes, tt.goType, nil, cfg)
			if schema.Type != tt.expectedType {
				t.Errorf("Expected type %s for %s, got %s", tt.expectedType, tt.goType, schema.Type)
			}
			if tt.expectedAdditionalProps && schema.AdditionalProperties == nil {
				t.Errorf("Expected additional properties for %s", tt.goType)
			}
		})
	}
}

func testMapGoTypeToOpenAPISchema_CustomTypes(t *testing.T) {
	cfg := DefaultAPISpecConfig()
	usedTypes := make(map[string]*Schema)

	// Create metadata with a custom type
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

	schema, _ := mapGoTypeToOpenAPISchema(usedTypes, "User", meta, cfg)
	if schema.Type != "object" {
		t.Errorf("Expected type 'object' for custom type, got %s", schema.Type)
	}
}

func testMapGoTypeToOpenAPISchema_ExternalTypes(t *testing.T) {
	cfg := &APISpecConfig{
		ExternalTypes: []ExternalType{
			{
				Name: "CustomType",
				OpenAPIType: &Schema{
					Type: "string",
					Enum: []interface{}{"value1", "value2"},
				},
			},
		},
	}
	usedTypes := make(map[string]*Schema)

	_, schemas := mapGoTypeToOpenAPISchema(usedTypes, "CustomType", nil, cfg)
	// External types are added to schemas map, not returned directly
	if externalSchema, exists := schemas["CustomType"]; exists {
		if externalSchema.Type != "string" {
			t.Errorf("Expected type 'string' for external type, got %s", externalSchema.Type)
		}
		if len(externalSchema.Enum) != 2 {
			t.Errorf("Expected 2 enum values, got %d", len(externalSchema.Enum))
		}
	} else {
		t.Error("Expected external type to be in schemas map")
	}
}

func testMapGoTypeToOpenAPISchema_TypeMappings(t *testing.T) {
	cfg := &APISpecConfig{
		TypeMapping: []TypeMapping{
			{
				GoType: "CustomType",
				OpenAPIType: &Schema{
					Type:   "integer",
					Format: "int64",
				},
			},
		},
	}
	usedTypes := make(map[string]*Schema)

	schema, _ := mapGoTypeToOpenAPISchema(usedTypes, "CustomType", nil, cfg)
	if schema.Type != "integer" {
		t.Errorf("Expected type 'integer' for mapped type, got %s", schema.Type)
	}
	if schema.Format != "int64" {
		t.Errorf("Expected format 'int64' for mapped type, got %s", schema.Format)
	}
}

func testMapGoTypeToOpenAPISchema_NilMetadata(t *testing.T) {
	cfg := DefaultAPISpecConfig()
	usedTypes := make(map[string]*Schema)

	// Test with nil metadata
	schema, _ := mapGoTypeToOpenAPISchema(usedTypes, "CustomType", nil, cfg)
	if schema == nil {
		t.Error("Expected non-nil schema")
		return
	}

	if schema.Ref == "" {
		t.Error("Expected reference schema for unknown type")
	}
}

// TestGenerateSchemaFromType_Comprehensive_Extended tests schema generation from metadata types
func TestGenerateSchemaFromType_Comprehensive_Extended(t *testing.T) {
	tests := []struct {
		name        string
		description string
		testFunc    func(t *testing.T)
	}{
		{"struct_type", "Should generate struct schema", testGenerateSchemaFromType_Struct},
		{"interface_type", "Should generate interface schema", testGenerateSchemaFromType_Interface},
		{"alias_type", "Should generate alias schema", testGenerateSchemaFromType_Alias},
		{"external_type", "Should handle external types", testGenerateSchemaFromType_External},
		{"nil_type", "Should handle nil type", testGenerateSchemaFromType_Nil},
		{"with_generics", "Should handle generic types", testGenerateSchemaFromType_WithGenerics},
		{"with_nested_types", "Should handle nested types", testGenerateSchemaFromType_WithNestedTypes},
		{"with_json_tags", "Should handle JSON tags", testGenerateSchemaFromType_WithJSONTags},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.testFunc(t)
		})
	}
}

func testGenerateSchemaFromType_Struct(t *testing.T) {
	cfg := DefaultAPISpecConfig()
	usedTypes := make(map[string]*Schema)

	stringPool := metadata.NewStringPool()
	typ := &metadata.Type{
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
	}

	meta := &metadata.Metadata{StringPool: stringPool}

	schema, _ := generateSchemaFromType(usedTypes, "User", typ, meta, cfg)
	if schema.Type != "object" {
		t.Errorf("Expected type 'object', got %s", schema.Type)
	}
	if len(schema.Properties) != 2 {
		t.Errorf("Expected 2 properties, got %d", len(schema.Properties))
	}
}

func testGenerateSchemaFromType_Interface(t *testing.T) {
	cfg := DefaultAPISpecConfig()
	usedTypes := make(map[string]*Schema)

	stringPool := metadata.NewStringPool()
	typ := &metadata.Type{
		Name: stringPool.Get("Handler"),
		Kind: stringPool.Get("interface"),
	}

	meta := &metadata.Metadata{StringPool: stringPool}

	schema, _ := generateSchemaFromType(usedTypes, "Handler", typ, meta, cfg)
	if schema.Type != "object" {
		t.Errorf("Expected type 'object', got %s", schema.Type)
	}
}

func testGenerateSchemaFromType_Alias(t *testing.T) {
	cfg := DefaultAPISpecConfig()
	usedTypes := make(map[string]*Schema)

	stringPool := metadata.NewStringPool()
	typ := &metadata.Type{
		Name:   stringPool.Get("UserID"),
		Kind:   stringPool.Get("alias"),
		Target: stringPool.Get("string"),
	}

	meta := &metadata.Metadata{StringPool: stringPool}

	schema, _ := generateSchemaFromType(usedTypes, "UserID", typ, meta, cfg)
	if schema.Type != "string" {
		t.Errorf("Expected type 'string', got %s", schema.Type)
	}
}

func testGenerateSchemaFromType_External(t *testing.T) {
	cfg := &APISpecConfig{
		ExternalTypes: []ExternalType{
			{
				Name: "ExternalType",
				OpenAPIType: &Schema{
					Type: "string",
				},
			},
		},
	}
	usedTypes := make(map[string]*Schema)

	stringPool := metadata.NewStringPool()
	typ := &metadata.Type{
		Name: stringPool.Get("ExternalType"),
		Kind: stringPool.Get("struct"),
	}

	meta := &metadata.Metadata{StringPool: stringPool}

	schema, _ := generateSchemaFromType(usedTypes, "ExternalType", typ, meta, cfg)
	if schema.Type != "string" {
		t.Errorf("Expected type 'string', got %s", schema.Type)
	}
}

func testGenerateSchemaFromType_Nil(t *testing.T) {
	cfg := DefaultAPISpecConfig()
	usedTypes := make(map[string]*Schema)

	meta := &metadata.Metadata{StringPool: metadata.NewStringPool()}

	// This should panic because typ is nil
	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected panic when typ is nil")
		}
	}()

	generateSchemaFromType(usedTypes, "Test", nil, meta, cfg)
}

func testGenerateSchemaFromType_WithGenerics(t *testing.T) {
	cfg := DefaultAPISpecConfig()
	usedTypes := make(map[string]*Schema)

	stringPool := metadata.NewStringPool()
	typ := &metadata.Type{
		Name: stringPool.Get("Container"),
		Kind: stringPool.Get("struct"),
		Fields: []metadata.Field{
			{
				Name: stringPool.Get("Data"),
				Type: stringPool.Get("T"),
			},
		},
	}

	meta := &metadata.Metadata{StringPool: stringPool}

	schema, _ := generateSchemaFromType(usedTypes, "Container-T", typ, meta, cfg)
	if schema.Type != "object" {
		t.Errorf("Expected type 'object', got %s", schema.Type)
	}
}

func testGenerateSchemaFromType_WithNestedTypes(t *testing.T) {
	cfg := DefaultAPISpecConfig()
	usedTypes := make(map[string]*Schema)

	stringPool := metadata.NewStringPool()
	nestedType := &metadata.Type{
		Name: stringPool.Get("Address"),
		Kind: stringPool.Get("struct"),
		Fields: []metadata.Field{
			{
				Name: stringPool.Get("Street"),
				Type: stringPool.Get("string"),
			},
		},
	}

	typ := &metadata.Type{
		Name: stringPool.Get("User"),
		Kind: stringPool.Get("struct"),
		Fields: []metadata.Field{
			{
				Name:       stringPool.Get("Address"),
				Type:       stringPool.Get("Address"),
				NestedType: nestedType,
			},
		},
	}

	meta := &metadata.Metadata{StringPool: stringPool}

	schema, _ := generateSchemaFromType(usedTypes, "User", typ, meta, cfg)
	if schema.Type != "object" {
		t.Errorf("Expected type 'object', got %s", schema.Type)
	}
	if len(schema.Properties) != 1 {
		t.Errorf("Expected 1 property, got %d", len(schema.Properties))
	}
}

func testGenerateSchemaFromType_WithJSONTags(t *testing.T) {
	cfg := DefaultAPISpecConfig()
	usedTypes := make(map[string]*Schema)

	stringPool := metadata.NewStringPool()
	typ := &metadata.Type{
		Name: stringPool.Get("User"),
		Kind: stringPool.Get("struct"),
		Fields: []metadata.Field{
			{
				Name: stringPool.Get("Name"),
				Type: stringPool.Get("string"),
				Tag:  stringPool.Get(`json:"user_name"`),
			},
		},
	}

	meta := &metadata.Metadata{StringPool: stringPool}

	schema, _ := generateSchemaFromType(usedTypes, "User", typ, meta, cfg)
	if schema.Type != "object" {
		t.Errorf("Expected type 'object', got %s", schema.Type)
	}
	if schema.Properties["user_name"] == nil {
		t.Error("Expected property 'user_name' from JSON tag")
	}
}

// TestResolveUnderlyingType_Comprehensive tests underlying type resolution
func TestResolveUnderlyingType_Comprehensive(t *testing.T) {
	tests := []struct {
		name        string
		description string
		testFunc    func(t *testing.T)
	}{
		{"alias_type", "Should resolve alias to underlying type", testResolveUnderlyingType_Alias},
		{"non_alias_type", "Should return empty for non-alias", testResolveUnderlyingType_NonAlias},
		{"nil_metadata", "Should handle nil metadata", testResolveUnderlyingType_NilMetadata},
		{"with_prefixes", "Should handle type prefixes", testResolveUnderlyingType_WithPrefixes},
		{"not_found", "Should return empty for not found", testResolveUnderlyingType_NotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.testFunc(t)
		})
	}
}

func testResolveUnderlyingType_Alias(t *testing.T) {
	stringPool := metadata.NewStringPool()
	meta := &metadata.Metadata{
		StringPool: stringPool,
		Packages: map[string]*metadata.Package{
			"main": {
				Files: map[string]*metadata.File{
					"types.go": {
						Types: map[string]*metadata.Type{
							"UserID": {
								Name:   stringPool.Get("UserID"),
								Kind:   stringPool.Get("alias"),
								Target: stringPool.Get("string"),
							},
						},
					},
				},
			},
		},
	}

	result := resolveUnderlyingType("UserID", meta)
	if result != "string" {
		t.Errorf("Expected 'string', got %s", result)
	}
}

func testResolveUnderlyingType_NonAlias(t *testing.T) {
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
							},
						},
					},
				},
			},
		},
	}

	result := resolveUnderlyingType("User", meta)
	if result != "" {
		t.Errorf("Expected empty string, got %s", result)
	}
}

func testResolveUnderlyingType_NilMetadata(t *testing.T) {
	result := resolveUnderlyingType("UserID", nil)
	if result != "" {
		t.Errorf("Expected empty string for nil metadata, got %s", result)
	}
}

func testResolveUnderlyingType_WithPrefixes(t *testing.T) {
	stringPool := metadata.NewStringPool()
	meta := &metadata.Metadata{
		StringPool: stringPool,
		Packages: map[string]*metadata.Package{
			"main": {
				Files: map[string]*metadata.File{
					"types.go": {
						Types: map[string]*metadata.Type{
							"UserID": {
								Name:   stringPool.Get("UserID"),
								Kind:   stringPool.Get("alias"),
								Target: stringPool.Get("string"),
							},
						},
					},
				},
			},
		},
	}

	// Test with array prefix
	result := resolveUnderlyingType("[]UserID", meta)
	if result != "[]string" {
		t.Errorf("Expected '[]string', got %s", result)
	}

	// Test with pointer prefix
	result = resolveUnderlyingType("*UserID", meta)
	if result != "*string" {
		t.Errorf("Expected '*string', got %s", result)
	}
}

func testResolveUnderlyingType_NotFound(t *testing.T) {
	stringPool := metadata.NewStringPool()
	meta := &metadata.Metadata{
		StringPool: stringPool,
		Packages:   make(map[string]*metadata.Package),
	}

	result := resolveUnderlyingType("NonExistentType", meta)
	if result != "" {
		t.Errorf("Expected empty string for non-existent type, got %s", result)
	}
}

// TestMarkUsedType_Comprehensive tests type marking functionality
func TestMarkUsedType_Comprehensive(t *testing.T) {
	tests := []struct {
		name        string
		description string
		testFunc    func(t *testing.T)
	}{
		{"basic_marking", "Should mark type as used", testMarkUsedType_Basic},
		{"pointer_type", "Should handle pointer types", testMarkUsedType_Pointer},
		{"already_marked", "Should return true if already marked", testMarkUsedType_AlreadyMarked},
		{"different_values", "Should handle different mark values", testMarkUsedType_DifferentValues},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.testFunc(t)
		})
	}
}

func testMarkUsedType_Basic(t *testing.T) {
	usedTypes := make(map[string]*Schema)

	result := markUsedType(usedTypes, "User", &Schema{Type: "object"})
	if result {
		t.Error("Expected false for first marking")
	}
	if usedTypes["User"] == nil {
		t.Error("Expected User to be marked as used")
	}
}

func testMarkUsedType_Pointer(t *testing.T) {
	usedTypes := make(map[string]*Schema)

	result := markUsedType(usedTypes, "*User", &Schema{Type: "object"})
	if result {
		t.Error("Expected false for first marking")
	}
	if usedTypes["*User"] == nil {
		t.Error("Expected *User to be marked as used")
	}
	if usedTypes["User"] == nil {
		t.Error("Expected User to be marked as used")
	}
}

func testMarkUsedType_AlreadyMarked(t *testing.T) {
	usedTypes := make(map[string]*Schema)
	usedTypes["User"] = &Schema{Type: "object"}

	result := markUsedType(usedTypes, "User", &Schema{Type: "object"})
	if !result {
		t.Error("Expected true for already marked type")
	}
}

func testMarkUsedType_DifferentValues(t *testing.T) {
	usedTypes := make(map[string]*Schema)

	// Mark with true
	result1 := markUsedType(usedTypes, "User", &Schema{Type: "object"})
	if result1 {
		t.Error("Expected false for first marking")
	}
	if usedTypes["User"] == nil {
		t.Error("Expected User to be marked as true")
	}

	// Mark with false
	result2 := markUsedType(usedTypes, "User", &Schema{Type: "object"})
	if !result2 {
		t.Error("Expected true for already marked type")
	}
	// Value should remain true (first marking takes precedence)
	if usedTypes["User"] == nil {
		t.Error("Expected User to remain marked as true")
	}
}

// TestIsPrimitiveType_Comprehensive tests primitive type detection
func TestIsPrimitiveType_Comprehensive(t *testing.T) {
	tests := []struct {
		name        string
		description string
		testFunc    func(t *testing.T)
	}{
		{"basic_primitives", "Should detect basic primitives", testIsPrimitiveType_BasicPrimitives},
		{"pointer_primitives", "Should detect pointer primitives", testIsPrimitiveType_PointerPrimitives},
		{"slice_primitives", "Should detect slice primitives", testIsPrimitiveType_SlicePrimitives},
		{"map_primitives", "Should detect map primitives", testIsPrimitiveType_MapPrimitives},
		{"custom_types", "Should not detect custom types", testIsPrimitiveType_CustomTypes},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.testFunc(t)
		})
	}
}

func testIsPrimitiveType_BasicPrimitives(t *testing.T) {
	primitives := []string{
		"string", "int", "int8", "int16", "int32", "int64",
		"uint", "uint8", "uint16", "uint32", "uint64",
		"float32", "float64", "bool", "byte", "rune",
		"error", "interface{}", "struct{}", "any",
		"complex64", "complex128",
	}

	for _, primitive := range primitives {
		if !isPrimitiveType(primitive) {
			t.Errorf("Expected %s to be primitive", primitive)
		}
	}
}

func testIsPrimitiveType_PointerPrimitives(t *testing.T) {
	pointerPrimitives := []string{
		"*string", "*int", "*bool", "*float64",
	}

	for _, primitive := range pointerPrimitives {
		if !isPrimitiveType(primitive) {
			t.Errorf("Expected %s to be primitive", primitive)
		}
	}
}

func testIsPrimitiveType_SlicePrimitives(t *testing.T) {
	slicePrimitives := []string{
		"[]string", "[]int", "[]bool", "[]float64",
	}

	for _, primitive := range slicePrimitives {
		if !isPrimitiveType(primitive) {
			t.Errorf("Expected %s to be primitive", primitive)
		}
	}
}

func testIsPrimitiveType_MapPrimitives(t *testing.T) {
	mapPrimitives := []string{
		"map[string]string", "map[string]int", "map[string]bool",
	}

	for _, primitive := range mapPrimitives {
		if !isPrimitiveType(primitive) {
			t.Errorf("Expected %s to be primitive", primitive)
		}
	}
}

func testIsPrimitiveType_CustomTypes(t *testing.T) {
	customTypes := []string{
		"User", "UserID", "CustomType", "MyStruct",
	}

	for _, customType := range customTypes {
		if isPrimitiveType(customType) {
			t.Errorf("Expected %s to not be primitive", customType)
		}
	}
}

// TestExtractJSONName_Comprehensive tests JSON name extraction
func TestExtractJSONName_Comprehensive(t *testing.T) {
	tests := []struct {
		tag      string
		expected string
	}{
		{"", ""},
		{`json:"name"`, "name"},
		{`json:"user_name"`, "user_name"},
		{`json:"name,omitempty"`, "name"},
		{`json:"-"`, ""},
		{`json:"name,omitempty,string"`, "name"},
		{`other:"value"`, ""},
		{`json:"name" other:"value"`, "name"},
		{`other:"value" json:"name"`, "name"},
	}

	for _, tt := range tests {
		t.Run(tt.tag, func(t *testing.T) {
			result := extractJSONName(tt.tag)
			if result != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, result)
			}
		})
	}
}

// TestTypeParts_Comprehensive tests type parts parsing
func TestTypeParts_Comprehensive(t *testing.T) {
	tests := []struct {
		input                string
		expectedPkgName      string
		expectedTypeName     string
		expectedGenericTypes []string
	}{
		{"string", "", "string", nil},
		{"main-->User", "main", "User", nil},
		{"pkg-->Type-->T", "pkg", "Type", []string{"T"}},
		{"Container[T]", "", "Container[T]", nil},
		{"Container[T, U]", "", "Container[T, U]", nil},
		{"pkg-->Container[T]", "pkg", "Container", []string{"T"}},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := TypeParts(tt.input)
			if result.PkgName != tt.expectedPkgName {
				t.Errorf("Expected PkgName to be %s, got %s", tt.expectedPkgName, result.PkgName)
			}
			if result.TypeName != tt.expectedTypeName {
				t.Errorf("Expected TypeName to be %s, got %s", tt.expectedTypeName, result.TypeName)
			}
			if len(result.GenericTypes) != len(tt.expectedGenericTypes) {
				t.Errorf("Expected %d generic types, got %d", len(tt.expectedGenericTypes), len(result.GenericTypes))
			}
			for i, expected := range tt.expectedGenericTypes {
				if i < len(result.GenericTypes) && result.GenericTypes[i] != expected {
					t.Errorf("Expected generic type %d to be %s, got %s", i, expected, result.GenericTypes[i])
				}
			}
		})
	}
}

// TestFindTypesInMetadata_Comprehensive tests type finding in metadata
func TestFindTypesInMetadata_Comprehensive(t *testing.T) {
	tests := []struct {
		name        string
		description string
		testFunc    func(t *testing.T)
	}{
		{"primitive_type", "Should return nil for primitives", testFindTypesInMetadata_Primitive},
		{"nil_metadata", "Should handle nil metadata", testFindTypesInMetadata_NilMetadata},
		{"found_type", "Should find existing type", testFindTypesInMetadata_FoundType},
		{"not_found", "Should return empty for not found", testFindTypesInMetadata_NotFound},
		{"generic_type", "Should handle generic types", testFindTypesInMetadata_GenericType},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.testFunc(t)
		})
	}
}

func testFindTypesInMetadata_Primitive(t *testing.T) {
	stringPool := metadata.NewStringPool()
	meta := &metadata.Metadata{StringPool: stringPool}

	result := findTypesInMetadata(meta, "string")
	if result != nil {
		t.Error("Expected nil for primitive type")
	}
}

func testFindTypesInMetadata_NilMetadata(t *testing.T) {
	result := findTypesInMetadata(nil, "User")
	if result != nil {
		t.Error("Expected nil for nil metadata")
	}
}

func testFindTypesInMetadata_FoundType(t *testing.T) {
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
							},
						},
					},
				},
			},
		},
	}

	result := findTypesInMetadata(meta, "User")
	if len(result) == 0 {
		t.Error("Expected to find User type")
	}
	if result["User"] == nil {
		t.Error("Expected User type to be present")
	}
}

func testFindTypesInMetadata_NotFound(t *testing.T) {
	stringPool := metadata.NewStringPool()
	meta := &metadata.Metadata{
		StringPool: stringPool,
		Packages:   make(map[string]*metadata.Package),
	}

	result := findTypesInMetadata(meta, "NonExistent")
	if len(result) == 0 {
		t.Error("Expected non-empty result for non-existent type (should contain the type name as key)")
	}
}

func testFindTypesInMetadata_GenericType(t *testing.T) {
	stringPool := metadata.NewStringPool()
	meta := &metadata.Metadata{
		StringPool: stringPool,
		Packages: map[string]*metadata.Package{
			"main": {
				Files: map[string]*metadata.File{
					"types.go": {
						Types: map[string]*metadata.Type{
							"Container": {
								Name: stringPool.Get("Container"),
								Kind: stringPool.Get("struct"),
							},
						},
					},
				},
			},
		},
	}

	result := findTypesInMetadata(meta, "Container-T")
	if len(result) == 0 {
		t.Error("Expected to find generic type")
	}
	// The function should return the type name as a key, even if the type doesn't exist
	if _, exists := result["Container-T"]; !exists {
		t.Error("Expected Container-T type name to be present as key")
	}
}

// TestCollectUsedTypesFromRoutes_Comprehensive tests used type collection
func TestCollectUsedTypesFromRoutes_Comprehensive(t *testing.T) {
	tests := []struct {
		name        string
		description string
		testFunc    func(t *testing.T)
	}{
		{"empty_routes", "Should handle empty routes", testCollectUsedTypesFromRoutes_Empty},
		{"with_request_body", "Should collect request body types", testCollectUsedTypesFromRoutes_WithRequestBody},
		{"with_response_types", "Should collect response types", testCollectUsedTypesFromRoutes_WithResponseTypes},
		{"with_parameters", "Should collect parameter types", testCollectUsedTypesFromRoutes_WithParameters},
		{"mixed_types", "Should collect all types", testCollectUsedTypesFromRoutes_MixedTypes},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.testFunc(t)
		})
	}
}

func testCollectUsedTypesFromRoutes_Empty(t *testing.T) {
	routes := []RouteInfo{}
	result := collectUsedTypesFromRoutes(routes)
	if len(result) != 0 {
		t.Errorf("Expected empty result, got %d types", len(result))
	}
}

func testCollectUsedTypesFromRoutes_WithRequestBody(t *testing.T) {
	routes := []RouteInfo{
		{
			Request: &RequestInfo{
				BodyType: "User",
			},
		},
	}

	result := collectUsedTypesFromRoutes(routes)
	if _, exists := result["User"]; !exists {
		t.Error("Expected User type to be collected")
	}
}

func testCollectUsedTypesFromRoutes_WithResponseTypes(t *testing.T) {
	routes := []RouteInfo{
		{
			Response: map[string]*ResponseInfo{
				"200": {
					BodyType: "User",
				},
			},
		},
	}

	result := collectUsedTypesFromRoutes(routes)
	if _, exists := result["User"]; !exists {
		t.Error("Expected User type to be collected")
	}
}

func testCollectUsedTypesFromRoutes_WithParameters(t *testing.T) {
	routes := []RouteInfo{
		{
			Params: []Parameter{
				{
					Schema: &Schema{
						Ref: "#/components/schemas/User",
					},
				},
			},
		},
	}

	result := collectUsedTypesFromRoutes(routes)
	if _, exists := result["User"]; !exists {
		t.Error("Expected User type to be collected")
	}
}

func testCollectUsedTypesFromRoutes_MixedTypes(t *testing.T) {
	routes := []RouteInfo{
		{
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
					Schema: &Schema{
						Ref: "#/components/schemas/UserID",
					},
				},
			},
		},
	}

	result := collectUsedTypesFromRoutes(routes)
	expectedTypes := []string{"CreateUserRequest", "User", "UserID"}
	for _, expectedType := range expectedTypes {
		if _, exists := result[expectedType]; !exists {
			t.Errorf("Expected %s type to be collected", expectedType)
		}
	}
}
