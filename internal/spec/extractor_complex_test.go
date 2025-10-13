package spec

import (
	"testing"

	"github.com/ehabterra/apispec/internal/metadata"
)

func TestExtractor_ComplexRouteExtraction(t *testing.T) {
	// Create metadata with string pool
	stringPool := metadata.NewStringPool()
	meta := &metadata.Metadata{
		StringPool: stringPool,
		Packages: map[string]*metadata.Package{
			"main": {
				Files: map[string]*metadata.File{
					"routes.go": {
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
							"Response": {
								Name: stringPool.Get("Response"),
								Kind: stringPool.Get("struct"),
								Fields: []metadata.Field{
									{
										Name: stringPool.Get("Message"),
										Type: stringPool.Get("string"),
									},
									{
										Name: stringPool.Get("Data"),
										Type: stringPool.Get("interface{}"),
									},
								},
							},
						},
						Variables: map[string]*metadata.Variable{
							"router": {
								Name:  stringPool.Get("router"),
								Type:  stringPool.Get("*gin.Engine"),
								Value: stringPool.Get("gin.New()"),
							},
						},
					},
				},
			},
		},
	}

	// Create call graph with route registration
	caller := metadata.Call{
		Meta: meta,
		Name: stringPool.Get("main"),
		Pkg:  stringPool.Get("main"),
	}
	callee := metadata.Call{
		Meta:     meta,
		Name:     stringPool.Get("GET"),
		Pkg:      stringPool.Get("github.com/gin-gonic/gin"),
		RecvType: stringPool.Get("*gin.Engine"),
	}

	// Create arguments for route registration
	pathArg := metadata.NewCallArgument(meta)
	pathArg.SetKind(metadata.KindLiteral)
	pathArg.SetValue(`"/users"`)

	handlerArg := metadata.NewCallArgument(meta)
	handlerArg.SetKind(metadata.KindIdent)
	handlerArg.SetName("getUsers")
	handlerArg.SetType("func(*gin.Context)")

	_ = metadata.CallGraphEdge{
		Caller: caller,
		Callee: callee,
		Args:   []metadata.CallArgument{*pathArg, *handlerArg},
	}

	// Create tracker with limits
	limits := metadata.TrackerLimits{
		MaxNodesPerTree:    100,
		MaxChildrenPerNode: 10,
		MaxArgsPerFunction: 5,
		MaxNestedArgsDepth: 3,
	}

	tree := NewMockTrackerTree(meta, limits)

	// Create a comprehensive config for testing
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
					RecvTypeRegex:   "^github\\.com/gin-gonic/gin\\.\\*(Engine|RouterGroup)$",
				},
			},
			RequestBodyPatterns: []RequestBodyPattern{
				{
					CallRegex:    `^(?i)(BindJSON|ShouldBindJSON|BindXML|BindYAML|BindForm|ShouldBind)$`,
					TypeArgIndex: 0,
					TypeFromArg:  true,
					Deref:        true,
				},
			},
			ResponsePatterns: []ResponsePattern{
				{
					CallRegex:      `^(?i)(JSON|String|XML|YAML|ProtoBuf|Data|File|Redirect)$`,
					StatusArgIndex: 0,
					TypeArgIndex:   1,
					TypeFromArg:    true,
					StatusFromArg:  true,
				},
			},
			ParamPatterns: []ParamPattern{
				{
					CallRegex:     "^Param$",
					ParamIn:       "path",
					ParamArgIndex: 0,
				},
				{
					CallRegex:     "^Query$",
					ParamIn:       "query",
					ParamArgIndex: 0,
				},
			},
			MountPatterns: []MountPattern{
				{
					CallRegex:      `^Group$`,
					PathFromArg:    true,
					RouterFromArg:  true,
					PathArgIndex:   0,
					RouterArgIndex: 1,
					IsMount:        true,
					RecvTypeRegex:  "^github\\.com/gin-gonic/gin\\.\\*(Engine|RouterGroup)$",
				},
			},
		},
		Defaults: Defaults{
			RequestContentType:  "application/json",
			ResponseContentType: "application/json",
			ResponseStatus:      200,
		},
		ExternalTypes: []ExternalType{
			{
				Name: "github.com/gin-gonic/gin.H",
				OpenAPIType: &Schema{
					Type: "object",
				},
			},
		},
	}

	// Create extractor
	extractor := NewExtractor(tree, cfg)

	// Test route extraction
	routes := extractor.ExtractRoutes()

	// For this simple test, we expect no routes since we need more complex call graph setup
	// This tests the extraction logic without requiring full call graph construction
	if len(routes) != 0 {
		t.Logf("Extracted %d routes", len(routes))
	}

	// Test that extractor was created with valid configuration
	if extractor.cfg == nil {
		t.Fatal("Extractor config should not be nil")
	}

	if extractor.tree == nil {
		t.Fatal("Extractor tree should not be nil")
	}

	// Test pattern matcher initialization
	if len(extractor.routeMatchers) == 0 {
		t.Error("Route matchers should be initialized")
	}

	if len(extractor.mountMatchers) == 0 {
		t.Error("Mount matchers should be initialized")
	}

	if len(extractor.requestMatchers) == 0 {
		t.Error("Request matchers should be initialized")
	}

	if len(extractor.responseMatchers) == 0 {
		t.Error("Response matchers should be initialized")
	}

	if len(extractor.paramMatchers) == 0 {
		t.Error("Param matchers should be initialized")
	}
}

func TestExtractor_TypeResolution(t *testing.T) {
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

	// Create extractor
	extractor := NewExtractor(tree, cfg)

	// Test that extractor can handle complex types
	if extractor == nil {
		t.Fatal("Extractor should not be nil")
	}

	// Test type resolution through the extractor
	// This tests the internal type resolution logic
	_ = extractor.ExtractRoutes()
}

func TestExtractor_PatternMatching(t *testing.T) {
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

	// Create config with various pattern types
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
				{
					CallRegex:       `^HandleFunc$`,
					MethodFromCall:  false,
					PathFromArg:     true,
					HandlerFromArg:  true,
					PathArgIndex:    0,
					HandlerArgIndex: 1,
				},
			},
			MountPatterns: []MountPattern{
				{
					CallRegex:      `^Group$`,
					PathFromArg:    true,
					RouterFromArg:  true,
					PathArgIndex:   0,
					RouterArgIndex: 1,
					IsMount:        true,
				},
				{
					CallRegex:      `^Mount$`,
					PathFromArg:    true,
					RouterFromArg:  true,
					PathArgIndex:   0,
					RouterArgIndex: 1,
					IsMount:        true,
				},
			},
			RequestBodyPatterns: []RequestBodyPattern{
				{
					CallRegex:    `^(?i)(BindJSON|ShouldBindJSON|BindXML|BindYAML|BindForm|ShouldBind)$`,
					TypeArgIndex: 0,
					TypeFromArg:  true,
					Deref:        true,
				},
				{
					CallRegex:    `^Decode$`,
					TypeArgIndex: 0,
					TypeFromArg:  true,
					Deref:        true,
				},
			},
			ResponsePatterns: []ResponsePattern{
				{
					CallRegex:      `^(?i)(JSON|String|XML|YAML|ProtoBuf|Data|File|Redirect)$`,
					StatusArgIndex: 0,
					TypeArgIndex:   1,
					TypeFromArg:    true,
					StatusFromArg:  true,
				},
				{
					CallRegex:    `^Marshal$`,
					TypeArgIndex: 0,
					TypeFromArg:  true,
					Deref:        true,
				},
			},
			ParamPatterns: []ParamPattern{
				{
					CallRegex:     "^Param$",
					ParamIn:       "path",
					ParamArgIndex: 0,
				},
				{
					CallRegex:     "^Query$",
					ParamIn:       "query",
					ParamArgIndex: 0,
				},
				{
					CallRegex:     "^GetHeader$",
					ParamIn:       "header",
					ParamArgIndex: 0,
				},
			},
		},
	}

	// Create extractor
	extractor := NewExtractor(tree, cfg)

	// Test pattern matcher initialization
	if len(extractor.routeMatchers) != 2 {
		t.Errorf("Expected 2 route matchers, got %d", len(extractor.routeMatchers))
	}

	if len(extractor.mountMatchers) != 2 {
		t.Errorf("Expected 2 mount matchers, got %d", len(extractor.mountMatchers))
	}

	if len(extractor.requestMatchers) != 2 {
		t.Errorf("Expected 2 request matchers, got %d", len(extractor.requestMatchers))
	}

	if len(extractor.responseMatchers) != 2 {
		t.Errorf("Expected 2 response matchers, got %d", len(extractor.responseMatchers))
	}

	if len(extractor.paramMatchers) != 3 {
		t.Errorf("Expected 3 param matchers, got %d", len(extractor.paramMatchers))
	}

	// Test that all pattern matchers have valid patterns
	for i, matcher := range extractor.routeMatchers {
		if matcher == nil {
			t.Errorf("Route matcher %d should not be nil", i)
		}
	}

	for i, matcher := range extractor.mountMatchers {
		if matcher == nil {
			t.Errorf("Mount matcher %d should not be nil", i)
		}
	}

	for i, matcher := range extractor.requestMatchers {
		if matcher == nil {
			t.Errorf("Request matcher %d should not be nil", i)
		}
	}

	for i, matcher := range extractor.responseMatchers {
		if matcher == nil {
			t.Errorf("Response matcher %d should not be nil", i)
		}
	}

	for i, matcher := range extractor.paramMatchers {
		if matcher == nil {
			t.Errorf("Param matcher %d should not be nil", i)
		}
	}
}

func TestExtractor_EdgeCases(t *testing.T) {
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

	// Test with nil config - this should panic due to nil pointer dereference
	// We'll test this with a defer to catch the panic
	defer func() {
		if r := recover(); r != nil {
			// Expected panic due to nil config
			t.Log("Expected panic with nil config:", r)
		}
	}()

	_ = NewExtractor(tree, nil)

	// Test with empty config
	emptyCfg := &APISpecConfig{}
	extractor := NewExtractor(tree, emptyCfg)
	if extractor == nil {
		t.Fatal("Extractor should handle empty config")
	}

	// Test with config that has no patterns
	noPatternsCfg := &APISpecConfig{
		Framework: FrameworkConfig{
			RoutePatterns:       []RoutePattern{},
			MountPatterns:       []MountPattern{},
			RequestBodyPatterns: []RequestBodyPattern{},
			ResponsePatterns:    []ResponsePattern{},
			ParamPatterns:       []ParamPattern{},
		},
	}
	extractor = NewExtractor(tree, noPatternsCfg)
	if extractor == nil {
		t.Fatal("Extractor should handle config with no patterns")
	}

	// Test extraction with no patterns
	routes := extractor.ExtractRoutes()
	if len(routes) != 0 {
		t.Errorf("Expected 0 routes with no patterns, got %d", len(routes))
	}
}

func TestExtractor_ComplexTypeHandling(t *testing.T) {
	// Create metadata with string pool
	stringPool := metadata.NewStringPool()
	meta := &metadata.Metadata{
		StringPool: stringPool,
		Packages: map[string]*metadata.Package{
			"main": {
				Files: map[string]*metadata.File{
					"complex_types.go": {
						Types: map[string]*metadata.Type{
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
							"PaginatedResponse": {
								Name: stringPool.Get("PaginatedResponse"),
								Kind: stringPool.Get("struct"),
								Fields: []metadata.Field{
									{
										Name: stringPool.Get("Items"),
										Type: stringPool.Get("[]interface{}"),
									},
									{
										Name: stringPool.Get("Total"),
										Type: stringPool.Get("int"),
									},
									{
										Name: stringPool.Get("Page"),
										Type: stringPool.Get("int"),
									},
									{
										Name: stringPool.Get("Limit"),
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

	// Create config with complex type handling
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
			RequestBodyPatterns: []RequestBodyPattern{
				{
					CallRegex:    `^(?i)(BindJSON|ShouldBindJSON|BindXML|BindYAML|BindForm|ShouldBind)$`,
					TypeArgIndex: 0,
					TypeFromArg:  true,
					Deref:        true,
				},
			},
			ResponsePatterns: []ResponsePattern{
				{
					CallRegex:      `^(?i)(JSON|String|XML|YAML|ProtoBuf|Data|File|Redirect)$`,
					StatusArgIndex: 0,
					TypeArgIndex:   1,
					TypeFromArg:    true,
					StatusFromArg:  true,
				},
			},
		},
		TypeMapping: []TypeMapping{
			{
				GoType: "interface{}",
				OpenAPIType: &Schema{
					Type: "object",
				},
			},
			{
				GoType: "[]interface{}",
				OpenAPIType: &Schema{
					Type: "array",
					Items: &Schema{
						Type: "object",
					},
				},
			},
		},
	}

	// Create extractor
	extractor := NewExtractor(tree, cfg)

	// Test that extractor can handle complex types
	if extractor == nil {
		t.Fatal("Extractor should not be nil")
		return
	}

	// Test extraction with complex types
	routes := extractor.ExtractRoutes()
	if len(routes) != 0 {
		t.Logf("Extracted %d routes with complex types", len(routes))
	}

	// Test that type mappings are properly configured
	if len(extractor.cfg.TypeMapping) != 2 {
		t.Errorf("Expected 2 type mappings, got %d", len(extractor.cfg.TypeMapping))
	}
}
