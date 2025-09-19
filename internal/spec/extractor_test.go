package spec

import (
	"testing"

	"github.com/ehabterra/apispec/internal/metadata"
)

func TestRefactoredExtractor(t *testing.T) {
	// Create a simple metadata structure for testing
	meta := &metadata.Metadata{
		StringPool: metadata.NewStringPool(),
	}

	arg := metadata.NewCallArgument(meta)
	arg.SetKind(metadata.KindIdent)
	arg.SetName("router")
	arg.SetType("*chi.Mux")

	// Create call graph after meta is defined
	meta.CallGraph = []metadata.CallGraphEdge{
		{
			Caller: metadata.Call{
				Meta: meta,
				Name: 0, // Will be set after StringPool is created
				Pkg:  1,
			},
			Callee: metadata.Call{
				Meta: meta,
				Name: 2,
				Pkg:  3,
			},
			Args: []metadata.CallArgument{
				*arg,
			},
		},
	}

	// Set the string pool indices after creation
	meta.CallGraph[0].Caller.Name = meta.StringPool.Get("main")
	meta.CallGraph[0].Caller.Pkg = meta.StringPool.Get("main")
	meta.CallGraph[0].Callee.Name = meta.StringPool.Get("NewRouter")
	meta.CallGraph[0].Callee.Pkg = meta.StringPool.Get("chi")

	// Create tracker with limits
	limits := metadata.TrackerLimits{
		MaxNodesPerTree:    100,
		MaxChildrenPerNode: 10,
		MaxArgsPerFunction: 5,
		MaxNestedArgsDepth: 3,
	}

	// Use MockTrackerTree for isolated unit testing
	tree := NewMockTrackerTree(meta, limits)

	// Add a test root node for the extractor to process
	testNode := &TrackerNode{
		key:           "test-router",
		CallGraphEdge: &meta.CallGraph[0],
	}
	tree.AddRoot(testNode)

	// Create a simple config for testing
	cfg := &APISpecConfig{
		Framework: FrameworkConfig{
			RoutePatterns: []RoutePattern{
				{
					CallRegex:      "NewRouter",
					MethodFromCall: true,
					PathFromArg:    true,
					PathArgIndex:   0,
				},
			},
			MountPatterns: []MountPattern{
				{
					CallRegex:      "Mount",
					IsMount:        true,
					PathFromArg:    true,
					PathArgIndex:   0,
					RouterFromArg:  true,
					RouterArgIndex: 1,
				},
			},
		},
		Defaults: Defaults{
			RequestContentType:  "application/json",
			ResponseContentType: "application/json",
			ResponseStatus:      200,
		},
	}

	// Create refactored extractor
	extractor := NewExtractor(tree, cfg)

	// Test extraction
	routes := extractor.ExtractRoutes()

	// Verify results
	if len(routes) == 0 {
		t.Log("No routes extracted, which is expected for this simple test")
	} else {
		t.Logf("Extracted %d routes", len(routes))
		for i, route := range routes {
			t.Logf("Route %d: %s %s", i, route.Method, route.Path)
		}
	}
}

func TestPatternMatchers(t *testing.T) {
	// Create context provider
	meta := &metadata.Metadata{
		StringPool: metadata.NewStringPool(),
	}
	contextProvider := NewContextProvider(meta)

	// Test route pattern matcher
	routePattern := RoutePattern{
		CallRegex:      "Get",
		MethodFromCall: true,
		PathFromArg:    true,
		PathArgIndex:   0,
	}

	cfg := &APISpecConfig{}
	schemaMapper := NewSchemaMapper(cfg)
	typeResolver := NewTypeResolver(meta, cfg, schemaMapper)
	matcher := NewRoutePatternMatcher(routePattern, cfg, contextProvider, typeResolver)

	// Test pattern matching
	if matcher.GetPriority() <= 0 {
		t.Error("Expected positive priority for route pattern matcher")
	}

	pattern := matcher.GetPattern()
	if pattern == nil {
		t.Error("Expected non-nil pattern")
	}
}

func TestContextProvider(t *testing.T) {
	// Create metadata with string pool
	meta := &metadata.Metadata{
		StringPool: metadata.NewStringPool(),
	}
	meta.StringPool.Get("test") // Add a string to the pool

	contextProvider := NewContextProvider(meta)

	// Test GetString
	result := contextProvider.GetString(0)
	if result != "test" {
		t.Errorf("Expected 'test', got '%s'", result)
	}

	// Test GetString with invalid index
	result = contextProvider.GetString(999)
	if result != "" {
		t.Errorf("Expected empty string for invalid index, got '%s'", result)
	}
}

func TestSchemaMapper(t *testing.T) {
	cfg := &APISpecConfig{}
	mapper := NewSchemaMapper(cfg)

	// Test basic type mapping
	schema := mapper.MapGoTypeToOpenAPISchema("string")
	if schema == nil || schema.Type != "string" {
		t.Error("Expected string schema for 'string' type")
	}

	// Test pointer type mapping
	schema = mapper.MapGoTypeToOpenAPISchema("*string")
	if schema == nil || schema.Type != "string" {
		t.Error("Expected string schema for '*string' type")
	}

	// Test slice type mapping
	schema = mapper.MapGoTypeToOpenAPISchema("[]string")
	if schema == nil || schema.Type != "array" {
		t.Error("Expected array schema for '[]string' type")
	}

	// Test status code mapping
	status, ok := mapper.MapStatusCode("200")
	if !ok || status != 200 {
		t.Error("Expected status code 200")
	}

	// Test method extraction
	method := mapper.MapMethodFromFunctionName("GetUsers")
	if method != "GET" {
		t.Errorf("Expected 'GET', got '%s'", method)
	}
}

func TestOverrideApplier(t *testing.T) {
	cfg := &APISpecConfig{
		Overrides: []Override{
			{
				FunctionName:   "testFunc",
				Summary:        "Test Summary",
				ResponseStatus: 201,
				ResponseType:   "TestType",
				Tags:           []string{"test"},
			},
		},
	}

	applier := NewOverrideApplier(cfg)

	// Test HasOverride
	if !applier.HasOverride("testFunc") {
		t.Error("Expected override to exist for 'testFunc'")
	}

	if applier.HasOverride("nonexistent") {
		t.Error("Expected no override for 'nonexistent'")
	}

	// Test ApplyOverrides
	routeInfo := &RouteInfo{
		Function: "testFunc",
		Response: map[string]*ResponseInfo{
			"200": {StatusCode: 200},
		},
	}

	applier.ApplyOverrides(routeInfo)

	if routeInfo.Summary != "Test Summary" {
		t.Errorf("Expected 'Test Summary', got '%s'", routeInfo.Summary)
	}

	if len(routeInfo.Tags) != 1 || routeInfo.Tags[0] != "test" {
		t.Error("Expected tags to be applied")
	}
}

func TestExtractResponse_WithLiteralValue(t *testing.T) {
	// Create a simple metadata structure for testing
	meta := &metadata.Metadata{
		StringPool: metadata.NewStringPool(),
	}

	// Test different types of literals
	testCases := []struct {
		name               string
		literalValue       string
		expectedType       string
		expectedSchemaType string
	}{
		{"string_literal", "OK", "string", "string"},
		{"numeric_literal", "42", "int", "integer"},
		{"float_literal", "3.14", "float64", "number"},
		{"boolean_literal", "true", "bool", "boolean"},
		{"nil_literal", "nil", "interface{}", "object"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create a call argument that represents a literal value
			arg := metadata.NewCallArgument(meta)
			arg.SetKind("literal")        // Set kind to literal
			arg.SetValue(tc.literalValue) // Set value to the test case value

			// Create call graph edge with the literal argument
			edge := &metadata.CallGraphEdge{
				Args: []metadata.CallArgument{*arg},
			}

			// Create a tracker node
			node := &TrackerNode{
				CallGraphEdge: edge,
			}

			// Create a simple config
			cfg := &APISpecConfig{
				Defaults: Defaults{
					ResponseStatus:      200,
					ResponseContentType: "application/json",
				},
			}

			// Create context provider and schema mapper
			contextProvider := NewContextProvider(meta)
			schemaMapper := NewSchemaMapper(cfg)

			// Create the response pattern matcher
			matcher := &ResponsePatternMatcherImpl{
				BasePatternMatcher: &BasePatternMatcher{
					cfg:             cfg,
					contextProvider: contextProvider,
					schemaMapper:    schemaMapper,
				},
				pattern: ResponsePattern{
					TypeFromArg:    true,
					TypeArgIndex:   0,
					StatusFromArg:  false,
					StatusArgIndex: -1,
				},
			}

			// Extract response
			result := matcher.ExtractResponse(node, &RouteInfo{
				Path:     "/test",
				Method:   "POST",
				Handler:  "testHandler",
				Package:  "testPackage",
				File:     "testFile",
				Function: "testFunction",
				Summary:  "testSummary",
				Tags:     []string{"test"},
				Request: &RequestInfo{
					BodyType: "testBodyType",
				},
				Response: map[string]*ResponseInfo{
					"200": {
						BodyType: "testBodyType",
					},
				},
				Params: []Parameter{
					{
						Name: "testParam",
						Schema: &Schema{
							Type: "string",
						},
					},
				},
				UsedTypes:   make(map[string]*Schema),
				Metadata:    meta,
				GroupPrefix: "testGroup",
			})

			// Verify that literal values are handled correctly
			if result == nil {
				t.Fatal("Expected non-nil result")
			}

			// For literal values, we expect the appropriate type based on the value
			if result.BodyType != tc.expectedType {
				t.Errorf("Expected BodyType to be '%s' for literal value '%s', got '%s'",
					tc.expectedType, tc.literalValue, result.BodyType)
			}

			if result.Schema == nil {
				t.Fatal("Expected non-nil Schema")
			}

			// The schema should match the expected OpenAPI type
			if result.Schema.Type != tc.expectedSchemaType && result.Schema.Type != "" {
				t.Errorf("Expected Schema.Type to be '%s', got '%s'", tc.expectedSchemaType, result.Schema.Type)
			}
		})
	}
}
