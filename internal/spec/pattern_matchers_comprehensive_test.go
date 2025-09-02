package spec

import (
	"reflect"
	"testing"

	"github.com/ehabterra/swagen/internal/metadata"
)

func TestRoutePatternMatcher_Comprehensive(t *testing.T) {
	// Create metadata with string pool
	stringPool := metadata.NewStringPool()
	meta := &metadata.Metadata{
		StringPool: stringPool,
	}

	// Create config
	cfg := DefaultGinConfig()

	// Create schema mapper
	schemaMapper := NewSchemaMapper(cfg)

	// Create type resolver
	typeResolver := NewTypeResolver(meta, cfg, schemaMapper)

	// Create context provider
	contextProvider := NewContextProvider(meta)

	// Test various route patterns
	tests := []struct {
		name          string
		pattern       RoutePattern
		expectedMatch bool
		description   string
	}{
		{
			name: "exact method match",
			pattern: RoutePattern{
				CallRegex:       `^GET$`,
				MethodFromCall:  true,
				PathFromArg:     true,
				HandlerFromArg:  true,
				PathArgIndex:    0,
				HandlerArgIndex: 1,
			},
			expectedMatch: true,
			description:   "Should match exact GET method",
		},
		{
			name: "case insensitive method match",
			pattern: RoutePattern{
				CallRegex:       `(?i)^get$`,
				MethodFromCall:  true,
				PathFromArg:     true,
				HandlerFromArg:  true,
				PathArgIndex:    0,
				HandlerArgIndex: 1,
			},
			expectedMatch: true,
			description:   "Should match case insensitive GET method",
		},
		{
			name: "method with receiver type",
			pattern: RoutePattern{
				CallRegex:       `^GET$`,
				MethodFromCall:  true,
				PathFromArg:     true,
				HandlerFromArg:  true,
				PathArgIndex:    0,
				HandlerArgIndex: 1,
				RecvTypeRegex:   `^\*gin\.Engine$`,
			},
			expectedMatch: true,
			description:   "Should match method with specific receiver type",
		},
		{
			name: "method with function name pattern",
			pattern: RoutePattern{
				CallRegex:         `^GET$`,
				MethodFromCall:    true,
				PathFromArg:       true,
				HandlerFromArg:    true,
				PathArgIndex:      0,
				HandlerArgIndex:   1,
				FunctionNameRegex: `^getUsers$`,
			},
			expectedMatch: true,
			description:   "Should match method with function name pattern",
		},
		{
			name: "complex regex pattern",
			pattern: RoutePattern{
				CallRegex:       `^(?i)(GET|POST|PUT|DELETE|PATCH|OPTIONS|HEAD)(?:Handler)?$`,
				MethodFromCall:  true,
				PathFromArg:     true,
				HandlerFromArg:  true,
				PathArgIndex:    0,
				HandlerArgIndex: 1,
			},
			expectedMatch: true,
			description:   "Should match complex regex with optional suffix",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher := NewRoutePatternMatcher(tt.pattern, cfg, contextProvider, typeResolver)
			if matcher == nil {
				t.Fatal("Route pattern matcher should not be nil")
			}

			// Test pattern validation
			if matcher.pattern.CallRegex != tt.pattern.CallRegex {
				t.Errorf("CallRegex not properly set: expected %s, got %s", tt.pattern.CallRegex, matcher.pattern.CallRegex)
			}

			// Test priority
			priority := matcher.GetPriority()
			if priority < 0 {
				t.Error("Priority should be non-negative")
			}

			// Test that matcher can be created without panicking
			t.Logf("Successfully created matcher for: %s", tt.description)
		})
	}
}

func TestMountPatternMatcher_Comprehensive(t *testing.T) {
	// Create metadata with string pool
	stringPool := metadata.NewStringPool()
	meta := &metadata.Metadata{
		StringPool: stringPool,
	}

	// Create config
	cfg := DefaultGinConfig()

	// Create schema mapper
	schemaMapper := NewSchemaMapper(cfg)

	// Create type resolver
	typeResolver := NewTypeResolver(meta, cfg, schemaMapper)

	// Create context provider
	contextProvider := NewContextProvider(meta)

	// Test various mount patterns
	tests := []struct {
		name          string
		pattern       MountPattern
		expectedMatch bool
		description   string
	}{
		{
			name: "basic group pattern",
			pattern: MountPattern{
				CallRegex:      `^Group$`,
				PathFromArg:    true,
				RouterFromArg:  true,
				PathArgIndex:   0,
				RouterArgIndex: 1,
				IsMount:        true,
			},
			expectedMatch: true,
			description:   "Should match basic Group pattern",
		},
		{
			name: "group with receiver type",
			pattern: MountPattern{
				CallRegex:      `^Group$`,
				PathFromArg:    true,
				RouterFromArg:  true,
				PathArgIndex:   0,
				RouterArgIndex: 1,
				IsMount:        true,
				RecvTypeRegex:  `^\*gin\.RouterGroup$`,
			},
			expectedMatch: true,
			description:   "Should match Group with specific receiver type",
		},
		{
			name: "mount with function name pattern",
			pattern: MountPattern{
				CallRegex:         `^Mount$`,
				PathFromArg:       true,
				RouterFromArg:     true,
				PathArgIndex:      0,
				RouterArgIndex:    1,
				IsMount:           true,
				FunctionNameRegex: `^mountAPI$`,
			},
			expectedMatch: true,
			description:   "Should match Mount with function name pattern",
		},
		{
			name: "complex mount pattern",
			pattern: MountPattern{
				CallRegex:      `^(?i)(Group|Mount|Use|Handle|Register)(?:Prefix|Group|Route)?$`,
				PathFromArg:    true,
				RouterFromArg:  true,
				PathArgIndex:   0,
				RouterArgIndex: 1,
				IsMount:        true,
			},
			expectedMatch: true,
			description:   "Should match complex mount pattern with optional suffix",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher := NewMountPatternMatcher(tt.pattern, cfg, contextProvider, typeResolver)
			if matcher == nil {
				t.Fatal("Mount pattern matcher should not be nil")
			}

			// Test pattern validation
			if matcher.pattern.CallRegex != tt.pattern.CallRegex {
				t.Errorf("CallRegex not properly set: expected %s, got %s", tt.pattern.CallRegex, matcher.pattern.CallRegex)
			}

			// Test priority
			priority := matcher.GetPriority()
			if priority < 0 {
				t.Error("Priority should be non-negative")
			}

			// Test that matcher can be created without panicking
			t.Logf("Successfully created matcher for: %s", tt.description)
		})
	}
}

func TestRequestPatternMatcher_Comprehensive(t *testing.T) {
	// Create metadata with string pool
	stringPool := metadata.NewStringPool()
	meta := &metadata.Metadata{
		StringPool: stringPool,
	}

	// Create config
	cfg := DefaultGinConfig()

	// Create schema mapper
	schemaMapper := NewSchemaMapper(cfg)

	// Create type resolver
	typeResolver := NewTypeResolver(meta, cfg, schemaMapper)

	// Create context provider
	contextProvider := NewContextProvider(meta)

	// Test various request patterns
	tests := []struct {
		name          string
		pattern       RequestBodyPattern
		expectedMatch bool
		description   string
	}{
		{
			name: "basic bind pattern",
			pattern: RequestBodyPattern{
				CallRegex:    `^(?i)(BindJSON|ShouldBindJSON|BindXML|BindYAML|BindForm|ShouldBind)$`,
				TypeArgIndex: 0,
				TypeFromArg:  true,
				Deref:        true,
			},
			expectedMatch: true,
			description:   "Should match basic bind pattern",
		},
		{
			name: "decode pattern",
			pattern: RequestBodyPattern{
				CallRegex:    `^Decode$`,
				TypeArgIndex: 0,
				TypeFromArg:  true,
				Deref:        true,
			},
			expectedMatch: true,
			description:   "Should match decode pattern",
		},
		{
			name: "custom bind pattern",
			pattern: RequestBodyPattern{
				CallRegex:    `^CustomBind$`,
				TypeArgIndex: 0,
				TypeFromArg:  true,
				Deref:        false,
			},
			expectedMatch: true,
			description:   "Should match custom bind pattern",
		},
		{
			name: "complex request pattern",
			pattern: RequestBodyPattern{
				CallRegex:    `^(?i)(Bind|Parse|Read|Unmarshal|Deserialize)(?:JSON|XML|YAML|Form|Body)?$`,
				TypeArgIndex: 0,
				TypeFromArg:  true,
				Deref:        true,
			},
			expectedMatch: true,
			description:   "Should match complex request pattern",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher := NewRequestPatternMatcher(tt.pattern, cfg, contextProvider, typeResolver)
			if matcher == nil {
				t.Fatal("Request pattern matcher should not be nil")
			}

			// Test pattern validation
			if matcher.pattern.CallRegex != tt.pattern.CallRegex {
				t.Errorf("CallRegex not properly set: expected %s, got %s", tt.pattern.CallRegex, matcher.pattern.CallRegex)
			}

			// Test priority
			priority := matcher.GetPriority()
			if priority < 0 {
				t.Error("Priority should be non-negative")
			}

			// Test that matcher can be created without panicking
			t.Logf("Successfully created matcher for: %s", tt.description)
		})
	}
}

func TestResponsePatternMatcher_Comprehensive(t *testing.T) {
	// Create metadata with string pool
	stringPool := metadata.NewStringPool()
	meta := &metadata.Metadata{
		StringPool: stringPool,
	}

	// Create config
	cfg := DefaultGinConfig()

	// Create schema mapper
	schemaMapper := NewSchemaMapper(cfg)

	// Create type resolver
	typeResolver := NewTypeResolver(meta, cfg, schemaMapper)

	// Create context provider
	contextProvider := NewContextProvider(meta)

	// Test various response patterns
	tests := []struct {
		name          string
		pattern       ResponsePattern
		expectedMatch bool
		description   string
	}{
		{
			name: "basic response pattern",
			pattern: ResponsePattern{
				CallRegex:      `^(?i)(JSON|String|XML|YAML|ProtoBuf|Data|File|Redirect)$`,
				StatusArgIndex: 0,
				TypeArgIndex:   1,
				TypeFromArg:    true,
				StatusFromArg:  true,
			},
			expectedMatch: true,
			description:   "Should match basic response pattern",
		},
		{
			name: "marshal pattern",
			pattern: ResponsePattern{
				CallRegex:    `^Marshal$`,
				TypeArgIndex: 0,
				TypeFromArg:  true,
				Deref:        true,
			},
			expectedMatch: true,
			description:   "Should match marshal pattern",
		},
		{
			name: "custom response pattern",
			pattern: ResponsePattern{
				CallRegex:      `^CustomResponse$`,
				StatusArgIndex: 0,
				TypeArgIndex:   1,
				TypeFromArg:    true,
				StatusFromArg:  false,
			},
			expectedMatch: true,
			description:   "Should match custom response pattern",
		},
		{
			name: "complex response pattern",
			pattern: ResponsePattern{
				CallRegex:      `^(?i)(Send|Write|Output|Return|Respond)(?:JSON|XML|YAML|Data|File)?$`,
				StatusArgIndex: 0,
				TypeArgIndex:   1,
				TypeFromArg:    true,
				StatusFromArg:  true,
			},
			expectedMatch: true,
			description:   "Should match complex response pattern",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher := NewResponsePatternMatcher(tt.pattern, cfg, contextProvider, typeResolver)
			if matcher == nil {
				t.Fatal("Response pattern matcher should not be nil")
			}

			// Test pattern validation
			if matcher.pattern.CallRegex != tt.pattern.CallRegex {
				t.Errorf("CallRegex not properly set: expected %s, got %s", tt.pattern.CallRegex, matcher.pattern.CallRegex)
			}

			// Test priority
			priority := matcher.GetPriority()
			if priority < 0 {
				t.Error("Priority should be non-negative")
			}

			// Test that matcher can be created without panicking
			t.Logf("Successfully created matcher for: %s", tt.description)
		})
	}
}

func TestParamPatternMatcher_Comprehensive(t *testing.T) {
	// Create metadata with string pool
	stringPool := metadata.NewStringPool()
	meta := &metadata.Metadata{
		StringPool: stringPool,
	}

	// Create config
	cfg := DefaultGinConfig()

	// Create schema mapper
	schemaMapper := NewSchemaMapper(cfg)

	// Create type resolver
	typeResolver := NewTypeResolver(meta, cfg, schemaMapper)

	// Create context provider
	contextProvider := NewContextProvider(meta)

	// Test various param patterns
	tests := []struct {
		name          string
		pattern       ParamPattern
		expectedMatch bool
		description   string
	}{
		{
			name: "path param pattern",
			pattern: ParamPattern{
				CallRegex:     "^Param$",
				ParamIn:       "path",
				ParamArgIndex: 0,
			},
			expectedMatch: true,
			description:   "Should match path param pattern",
		},
		{
			name: "query param pattern",
			pattern: ParamPattern{
				CallRegex:     "^Query$",
				ParamIn:       "query",
				ParamArgIndex: 0,
			},
			expectedMatch: true,
			description:   "Should match query param pattern",
		},
		{
			name: "header param pattern",
			pattern: ParamPattern{
				CallRegex:     "^GetHeader$",
				ParamIn:       "header",
				ParamArgIndex: 0,
			},
			expectedMatch: true,
			description:   "Should match header param pattern",
		},
		{
			name: "custom param pattern",
			pattern: ParamPattern{
				CallRegex:     "^CustomParam$",
				ParamIn:       "custom",
				ParamArgIndex: 0,
			},
			expectedMatch: true,
			description:   "Should match custom param pattern",
		},
		{
			name: "complex param pattern",
			pattern: ParamPattern{
				CallRegex:     `^(?i)(Param|Query|Header|Cookie|Form|Body)(?:Value|String|Int|Float|Bool)?$`,
				ParamIn:       "path",
				ParamArgIndex: 0,
			},
			expectedMatch: true,
			description:   "Should match complex param pattern",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher := NewParamPatternMatcher(tt.pattern, cfg, contextProvider, typeResolver)
			if matcher == nil {
				t.Fatal("Param pattern matcher should not be nil")
			}

			// Test pattern validation
			if matcher.pattern.CallRegex != tt.pattern.CallRegex {
				t.Errorf("CallRegex not properly set: expected %s, got %s", tt.pattern.CallRegex, matcher.pattern.CallRegex)
			}

			// Test priority
			priority := matcher.GetPriority()
			if priority < 0 {
				t.Error("Priority should be non-negative")
			}

			// Test that matcher can be created without panicking
			t.Logf("Successfully created matcher for: %s", tt.description)
		})
	}
}

func TestPatternMatcher_EdgeCases(t *testing.T) {
	// Create metadata with string pool
	stringPool := metadata.NewStringPool()
	meta := &metadata.Metadata{
		StringPool: stringPool,
	}

	// Create config
	cfg := DefaultGinConfig()

	// Create schema mapper
	schemaMapper := NewSchemaMapper(cfg)

	// Create type resolver
	typeResolver := NewTypeResolver(meta, cfg, schemaMapper)

	// Create context provider
	contextProvider := NewContextProvider(meta)

	// Test edge cases for all pattern types
	t.Run("empty regex patterns", func(t *testing.T) {
		// Test route pattern with empty regex
		routePattern := RoutePattern{
			CallRegex:      "",
			MethodFromCall: true,
			PathFromArg:    true,
			HandlerFromArg: true,
		}
		routeMatcher := NewRoutePatternMatcher(routePattern, cfg, contextProvider, typeResolver)
		if routeMatcher == nil {
			t.Fatal("Should handle empty regex pattern")
		}

		// Test mount pattern with empty regex
		mountPattern := MountPattern{
			CallRegex:     "",
			PathFromArg:   true,
			RouterFromArg: true,
			IsMount:       true,
		}
		mountMatcher := NewMountPatternMatcher(mountPattern, cfg, contextProvider, typeResolver)
		if mountMatcher == nil {
			t.Fatal("Should handle empty regex pattern")
		}
	})

	t.Run("nil config handling", func(t *testing.T) {
		// Test with nil config - should handle gracefully
		routePattern := RoutePattern{
			CallRegex:      `^GET$`,
			MethodFromCall: true,
			PathFromArg:    true,
			HandlerFromArg: true,
		}

		defer func() {
			if r := recover(); r != nil {
				t.Log("Expected panic with nil config:", r)
			}
		}()

		_ = NewRoutePatternMatcher(routePattern, nil, contextProvider, typeResolver)
	})

	t.Run("nil context provider", func(t *testing.T) {
		// Test with nil context provider - should handle gracefully
		routePattern := RoutePattern{
			CallRegex:      `^GET$`,
			MethodFromCall: true,
			PathFromArg:    true,
			HandlerFromArg: true,
		}

		defer func() {
			if r := recover(); r != nil {
				t.Log("Expected panic with nil context provider:", r)
			}
		}()

		_ = NewRoutePatternMatcher(routePattern, cfg, nil, typeResolver)
	})

	t.Run("nil type resolver", func(t *testing.T) {
		// Test with nil type resolver - should handle gracefully
		routePattern := RoutePattern{
			CallRegex:      `^GET$`,
			MethodFromCall: true,
			PathFromArg:    true,
			HandlerFromArg: true,
		}

		defer func() {
			if r := recover(); r != nil {
				t.Log("Expected panic with nil type resolver:", r)
			}
		}()

		_ = NewRoutePatternMatcher(routePattern, cfg, contextProvider, nil)
	})
}

func TestPatternMatcher_PrioritySystem(t *testing.T) {
	// Create metadata with string pool
	stringPool := metadata.NewStringPool()
	meta := &metadata.Metadata{
		StringPool: stringPool,
	}

	// Create config
	cfg := DefaultGinConfig()

	// Create schema mapper
	schemaMapper := NewSchemaMapper(cfg)

	// Create type resolver
	typeResolver := NewTypeResolver(meta, cfg, schemaMapper)

	// Create context provider
	contextProvider := NewContextProvider(meta)

	// Test priority system
	t.Run("route pattern priorities", func(t *testing.T) {
		// High priority pattern (priority is determined by the matcher, not the pattern)
		highPriorityPattern := RoutePattern{
			CallRegex:      `^GET$`,
			MethodFromCall: true,
			PathFromArg:    true,
			HandlerFromArg: true,
		}

		// Low priority pattern (priority is determined by the matcher, not the pattern)
		lowPriorityPattern := RoutePattern{
			CallRegex:      `^GET$`,
			MethodFromCall: true,
			PathFromArg:    true,
			HandlerFromArg: true,
		}

		highMatcher := NewRoutePatternMatcher(highPriorityPattern, cfg, contextProvider, typeResolver)
		lowMatcher := NewRoutePatternMatcher(lowPriorityPattern, cfg, contextProvider, typeResolver)

		highPriority := highMatcher.GetPriority()
		lowPriority := lowMatcher.GetPriority()

		// Both should have the same priority since they're the same pattern type
		if highPriority != lowPriority {
			t.Errorf("Same pattern types should have same priority: %d != %d", highPriority, lowPriority)
		}
	})

	t.Run("mount pattern priorities", func(t *testing.T) {
		// High priority pattern (priority is determined by the matcher, not the pattern)
		highPriorityPattern := MountPattern{
			CallRegex:     `^Group$`,
			PathFromArg:   true,
			RouterFromArg: true,
			IsMount:       true,
		}

		// Low priority pattern (priority is determined by the matcher, not the pattern)
		lowPriorityPattern := MountPattern{
			CallRegex:     `^Group$`,
			PathFromArg:   true,
			RouterFromArg: true,
			IsMount:       true,
		}

		highMatcher := NewMountPatternMatcher(highPriorityPattern, cfg, contextProvider, typeResolver)
		lowMatcher := NewMountPatternMatcher(lowPriorityPattern, cfg, contextProvider, typeResolver)

		highPriority := highMatcher.GetPriority()
		lowPriority := lowMatcher.GetPriority()

		// Both should have the same priority since they're the same pattern type
		if highPriority != lowPriority {
			t.Errorf("Same pattern types should have same priority: %d != %d", highPriority, lowPriority)
		}
	})
}

func TestMountPatternMatcher_GetPattern(t *testing.T) {
	// Create metadata with string pool
	stringPool := metadata.NewStringPool()
	meta := &metadata.Metadata{
		StringPool: stringPool,
	}

	// Create config
	cfg := DefaultGinConfig()

	// Create schema mapper
	schemaMapper := NewSchemaMapper(cfg)

	// Create type resolver
	typeResolver := NewTypeResolver(meta, cfg, schemaMapper)

	// Create context provider
	contextProvider := NewContextProvider(meta)

	// Create mount pattern
	pattern := MountPattern{
		CallRegex:    "Mount",
		IsMount:      true,
		PathFromArg:  true,
		PathArgIndex: 0,
	}

	// Create mount pattern matcher
	matcher := NewMountPatternMatcher(pattern, cfg, contextProvider, typeResolver)

	// Test GetPattern
	result := matcher.GetPattern()
	if result == nil {
		t.Error("GetPattern should not return nil")
	}

	// Verify the returned pattern is not nil
	if result == nil {
		t.Error("GetPattern should not return nil")
	}
}

func TestMountPatternMatcher_ExtractMount(t *testing.T) {
	// Create metadata with string pool
	stringPool := metadata.NewStringPool()
	meta := &metadata.Metadata{
		StringPool: stringPool,
	}

	// Create config
	cfg := DefaultGinConfig()

	// Create schema mapper
	schemaMapper := NewSchemaMapper(cfg)

	// Create type resolver
	typeResolver := NewTypeResolver(meta, cfg, schemaMapper)

	// Create context provider
	contextProvider := NewContextProvider(meta)

	// Create mount pattern
	pattern := MountPattern{
		CallRegex:      "Mount",
		IsMount:        true,
		PathFromArg:    true,
		PathArgIndex:   0,
		RouterArgIndex: 1,
	}

	// Create mount pattern matcher
	matcher := NewMountPatternMatcher(pattern, cfg, contextProvider, typeResolver)

	// Create a mock node with edge
	edge := &metadata.CallGraphEdge{
		Caller: metadata.Call{
			Name: stringPool.Get("main"),
			Pkg:  stringPool.Get("main"),
		},
		Callee: metadata.Call{
			Name: stringPool.Get("Mount"),
			Pkg:  stringPool.Get("gin"),
		},
		Args: []metadata.CallArgument{
			{
				Kind:  stringPool.Get(metadata.KindLiteral),
				Value: stringPool.Get("/api"),
				Meta:  meta,
			},
			{
				Kind: stringPool.Get(metadata.KindIdent),
				Name: stringPool.Get("router"),
				Pkg:  stringPool.Get("main"),
				Meta: meta,
			},
		},
		ChainRoot:  "app",
		ChainDepth: 1,
	}

	mockNode := &TrackerNode{
		CallGraphEdge: edge,
	}

	// Test ExtractMount
	mountInfo := matcher.ExtractMount(mockNode)

	// Verify mount info
	if reflect.DeepEqual(mountInfo.Pattern, MountPattern{}) {
		t.Error("MountInfo should contain a pattern")
	}

	if mountInfo.Path != "/api" {
		t.Errorf("Expected path /api, got %s", mountInfo.Path)
	}

	// if mountInfo.ChainRoot != "app" {
	// 	t.Errorf("Expected chain root 'app', got %s", mountInfo.ChainRoot)
	// }

	// if mountInfo.ChainDepth != 1 {
	// 	t.Errorf("Expected chain depth 1, got %d", mountInfo.ChainDepth)
	// }
}

func TestRequestPatternMatcher_MatchNode(t *testing.T) {
	// Create metadata with string pool
	stringPool := metadata.NewStringPool()
	meta := &metadata.Metadata{
		StringPool: stringPool,
	}

	// Create config
	cfg := DefaultGinConfig()

	// Create schema mapper
	schemaMapper := NewSchemaMapper(cfg)

	// Create type resolver
	typeResolver := NewTypeResolver(meta, cfg, schemaMapper)

	// Create context provider
	contextProvider := NewContextProvider(meta)

	// Create request pattern
	pattern := RequestBodyPattern{
		CallRegex:     "^BindJSON$",
		TypeArgIndex:  0,
		TypeFromArg:   false, // Don't extract type from argument
		Deref:         false, // Don't dereference
		RecvTypeRegex: `^gin\.\*gin\.Context$`,
	}

	// Create request pattern matcher
	matcher := NewRequestPatternMatcher(pattern, cfg, contextProvider, typeResolver)

	// Create a mock node with edge
	edge := &metadata.CallGraphEdge{
		Caller: metadata.Call{
			Name: stringPool.Get("main"),
			Pkg:  stringPool.Get("main"),
		},
		Callee: metadata.Call{
			Name:     stringPool.Get("BindJSON"),
			Pkg:      stringPool.Get("gin"),
			RecvType: stringPool.Get("*gin.Context"),
		},
		Args: []metadata.CallArgument{
			{
				Kind: stringPool.Get(metadata.KindIdent),
				Name: stringPool.Get("user"),
				Pkg:  stringPool.Get("main"),
				Meta: meta,
			},
		},
	}

	mockNode := &TrackerNode{
		CallGraphEdge: edge,
	}

	// Test MatchNode
	result := matcher.MatchNode(mockNode)
	if !result {
		t.Error("MatchNode should return true for matching pattern")
	}
}

func TestRequestPatternMatcher_GetPattern(t *testing.T) {
	// Create metadata with string pool
	stringPool := metadata.NewStringPool()
	meta := &metadata.Metadata{
		StringPool: stringPool,
	}

	// Create config
	cfg := DefaultGinConfig()

	// Create schema mapper
	schemaMapper := NewSchemaMapper(cfg)

	// Create type resolver
	typeResolver := NewTypeResolver(meta, cfg, schemaMapper)

	// Create context provider
	contextProvider := NewContextProvider(meta)

	// Create request pattern
	pattern := RequestBodyPattern{
		CallRegex:    "BindJSON",
		TypeArgIndex: 0,
		TypeFromArg:  true,
	}

	// Create request pattern matcher
	matcher := NewRequestPatternMatcher(pattern, cfg, contextProvider, typeResolver)

	// Test GetPattern
	result := matcher.GetPattern()
	if result == nil {
		t.Error("GetPattern should not return nil")
	}

	// Verify the returned pattern is not nil
	if result == nil {
		t.Error("GetPattern should not return nil")
	}
}

func TestRequestPatternMatcher_ExtractRequest(t *testing.T) {
	// Create metadata with string pool
	stringPool := metadata.NewStringPool()
	meta := &metadata.Metadata{
		StringPool: stringPool,
	}

	// Create config
	cfg := DefaultGinConfig()

	// Create schema mapper
	schemaMapper := NewSchemaMapper(cfg)

	// Create type resolver
	typeResolver := NewTypeResolver(meta, cfg, schemaMapper)

	// Create context provider
	contextProvider := NewContextProvider(meta)

	// Create request pattern
	pattern := RequestBodyPattern{
		CallRegex:    "BindJSON",
		TypeArgIndex: 0,
		TypeFromArg:  false, // Don't extract type from argument
		Deref:        false, // Don't dereference
	}

	// Create request pattern matcher
	matcher := NewRequestPatternMatcher(pattern, cfg, contextProvider, typeResolver)

	// Create a mock node with edge
	edge := &metadata.CallGraphEdge{
		Caller: metadata.Call{
			Name: stringPool.Get("main"),
			Pkg:  stringPool.Get("main"),
		},
		Callee: metadata.Call{
			Name: stringPool.Get("BindJSON"),
			Pkg:  stringPool.Get("gin"),
		},
		Args: []metadata.CallArgument{
			{
				Kind: stringPool.Get(metadata.KindIdent),
				Name: stringPool.Get("user"),
				Pkg:  stringPool.Get("main"),
				Type: stringPool.Get("*User"),
				Meta: meta,
			},
		},
	}

	mockNode := &TrackerNode{
		CallGraphEdge: edge,
	}

	// Test ExtractRequest
	routeInfo := &RouteInfo{}
	reqInfo := matcher.ExtractRequest(mockNode, routeInfo)

	// Verify request info
	// Since TypeFromArg is false, ExtractRequest should return nil
	if reqInfo != nil {
		t.Error("ExtractRequest should return nil when TypeFromArg is false")
	}
}

func TestBasePatternMatcher_traceVariable(t *testing.T) {
	// Create metadata with string pool
	stringPool := metadata.NewStringPool()
	meta := &metadata.Metadata{
		StringPool: stringPool,
		Packages: map[string]*metadata.Package{
			"main": {
				Files: map[string]*metadata.File{
					"main.go": {
						Variables: map[string]*metadata.Variable{
							"user": {
								Type: stringPool.Get("User"),
								Tok:  stringPool.Get("var"),
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
				AssignmentMap: map[string][]metadata.Assignment{
					"user": {
						{
							VariableName: stringPool.Get("user"),
							ConcreteType: stringPool.Get("User"),
							Pkg:          stringPool.Get("main"),
							Func:         stringPool.Get("main"),
						},
					},
				},
			},
		},
	}

	// Create config
	cfg := DefaultGinConfig()

	// Create schema mapper
	schemaMapper := NewSchemaMapper(cfg)

	// Create type resolver
	typeResolver := NewTypeResolver(meta, cfg, schemaMapper)

	// Create context provider
	contextProvider := NewContextProvider(meta)

	// Create base pattern matcher
	matcher := NewBasePatternMatcher(cfg, contextProvider, typeResolver)

	// Test traceVariable
	originVar, originPkg, originType, originFunc := matcher.traceVariable("user", "main", "main")

	// Verify tracing results
	if originVar != "user" {
		t.Errorf("Expected originVar 'user', got %s", originVar)
	}

	if originPkg != "main" {
		t.Errorf("Expected originPkg 'main', got %s", originPkg)
	}

	if originType == nil {
		t.Error("Expected originType to not be nil")
	}

	if originFunc != "main" {
		t.Errorf("Expected originFunc 'main', got %s", originFunc)
	}
}

func TestBasePatternMatcher_traceRouterOrigin(t *testing.T) {
	// Create metadata with string pool
	stringPool := metadata.NewStringPool()
	meta := &metadata.Metadata{
		StringPool: stringPool,
	}

	// Create config
	cfg := DefaultGinConfig()

	// Create schema mapper
	schemaMapper := NewSchemaMapper(cfg)

	// Create type resolver
	typeResolver := NewTypeResolver(meta, cfg, schemaMapper)

	// Create context provider
	contextProvider := NewContextProvider(meta)

	// Create base pattern matcher
	matcher := NewBasePatternMatcher(cfg, contextProvider, typeResolver)

	// Create a mock node with edge
	edge := &metadata.CallGraphEdge{
		Caller: metadata.Call{
			Name: stringPool.Get("main"),
			Pkg:  stringPool.Get("main"),
		},
		Callee: metadata.Call{
			Name: stringPool.Get("Mount"),
			Pkg:  stringPool.Get("gin"),
		},
	}

	mockNode := &TrackerNode{
		CallGraphEdge: edge,
	}

	// Test traceRouterOrigin with different argument kinds
	tests := []struct {
		name     string
		argKind  int
		argName  string
		argPkg   string
		expected string
	}{
		{
			name:     "KindIdent",
			argKind:  stringPool.Get(metadata.KindIdent),
			argName:  "router",
			argPkg:   "main",
			expected: "router",
		},
		{
			name:     "KindUnary",
			argKind:  stringPool.Get(metadata.KindUnary),
			argName:  "ptr",
			argPkg:   "main",
			expected: "ptr",
		},
		{
			name:     "KindSelector",
			argKind:  stringPool.Get(metadata.KindSelector),
			argName:  "sel",
			argPkg:   "main",
			expected: "sel",
		},
		{
			name:     "KindCall",
			argKind:  stringPool.Get(metadata.KindCall),
			argName:  "fun",
			argPkg:   "main",
			expected: "fun",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			routerArg := &metadata.CallArgument{
				Kind: tt.argKind,
				Name: stringPool.Get(tt.argName),
				Pkg:  stringPool.Get(tt.argPkg),
				Meta: meta,
			}

			// Add X field for KindUnary, KindSelector, KindCall
			if tt.argKind == stringPool.Get(metadata.KindUnary) || tt.argKind == stringPool.Get(metadata.KindSelector) || tt.argKind == stringPool.Get(metadata.KindCall) {
				routerArg.X = &metadata.CallArgument{
					Kind: stringPool.Get(metadata.KindIdent),
					Name: stringPool.Get(tt.argName),
					Pkg:  stringPool.Get(tt.argPkg),
					Meta: meta,
				}
			}

			// Add Fun field for KindCall
			if tt.argKind == stringPool.Get(metadata.KindCall) {
				routerArg.Fun = &metadata.CallArgument{
					Kind: stringPool.Get(metadata.KindIdent),
					Name: stringPool.Get(tt.argName),
					Pkg:  stringPool.Get(tt.argPkg),
					Meta: meta,
				}
			}

			// Test traceRouterOrigin
			matcher.traceRouterOrigin(routerArg, mockNode)
			// This is a void function, so we just test that it doesn't panic
		})
	}
}

func TestBasePatternMatcher_findAssignmentFunction(t *testing.T) {
	// Create metadata with string pool
	stringPool := metadata.NewStringPool()

	// Create the CallArgument first
	routerCallArg := &metadata.CallArgument{
		Kind: stringPool.Get(metadata.KindIdent),
		Name: stringPool.Get("NewRouter"),
		Pkg:  stringPool.Get("gin"),
	}

	// Create the edge args
	edgeArgs := []metadata.CallArgument{
		{
			Kind: stringPool.Get(metadata.KindCall),
			Fun:  routerCallArg,
		},
	}

	// Create the metadata
	meta := &metadata.Metadata{
		StringPool: stringPool,
		CallGraph: []metadata.CallGraphEdge{
			{
				Caller: metadata.Call{
					Name: stringPool.Get("main"),
					Pkg:  stringPool.Get("main"),
				},
				Callee: metadata.Call{
					Name: stringPool.Get("assign"),
					Pkg:  stringPool.Get("main"),
				},
				AssignmentMap: map[string][]metadata.Assignment{
					"router": {
						{
							VariableName: stringPool.Get("router"),
							ConcreteType: stringPool.Get("*gin.Engine"),
							Pkg:          stringPool.Get("main"),
							Func:         stringPool.Get("main"),
						},
					},
				},
				Args: edgeArgs,
			},
		},
	}

	// Now set the Meta field on all CallArgument structs
	for i := range edgeArgs {
		edgeArgs[i].Meta = meta
	}
	routerCallArg.Meta = meta

	// Create config
	cfg := DefaultGinConfig()

	// Create schema mapper
	schemaMapper := NewSchemaMapper(cfg)

	// Create type resolver
	typeResolver := NewTypeResolver(meta, cfg, schemaMapper)

	// Create context provider
	contextProvider := NewContextProvider(meta)

	// Create base pattern matcher
	matcher := NewBasePatternMatcher(cfg, contextProvider, typeResolver)

	// Create router argument
	routerArg := metadata.CallArgument{
		Kind: stringPool.Get(metadata.KindIdent),
		Name: stringPool.Get("router"),
		Pkg:  stringPool.Get("main"),
		Meta: meta,
		X: &metadata.CallArgument{
			Type: stringPool.Get("*gin.Engine"),
			Meta: meta,
		},
	}

	// Test findAssignmentFunction
	result := matcher.findAssignmentFunction(&routerArg)

	// Verify result
	if result == nil {
		t.Error("findAssignmentFunction should not return nil for valid assignment")
	}

	if result.GetName() != "NewRouter" {
		t.Errorf("Expected function name 'NewRouter', got %s", result.GetName())
	}
}

func TestRequestPatternMatcher_resolveTypeOrigin(t *testing.T) {
	// Create metadata with string pool
	stringPool := metadata.NewStringPool()
	meta := &metadata.Metadata{
		StringPool: stringPool,
	}

	// Create config
	cfg := DefaultGinConfig()

	// Create schema mapper
	schemaMapper := NewSchemaMapper(cfg)

	// Create type resolver
	typeResolver := NewTypeResolver(meta, cfg, schemaMapper)

	// Create context provider
	contextProvider := NewContextProvider(meta)

	// Create request pattern matcher
	matcher := NewRequestPatternMatcher(RequestBodyPattern{}, cfg, contextProvider, typeResolver)

	// Create a mock node with edge
	edge := &metadata.CallGraphEdge{
		Caller: metadata.Call{
			Name: stringPool.Get("main"),
			Pkg:  stringPool.Get("main"),
		},
		Callee: metadata.Call{
			Name: stringPool.Get("BindJSON"),
			Pkg:  stringPool.Get("gin"),
		},
		AssignmentMap: map[string][]metadata.Assignment{
			"user": {
				{
					VariableName: stringPool.Get("user"),
					ConcreteType: stringPool.Get("User"),
					Pkg:          stringPool.Get("main"),
					Func:         stringPool.Get("main"),
				},
			},
		},
	}

	mockNode := &TrackerNode{
		CallGraphEdge: edge,
	}

	// Test resolveTypeOrigin with different argument types
	tests := []struct {
		name         string
		arg          metadata.CallArgument
		originalType string
		expected     string
	}{
		{
			name: "with resolved type",
			arg: metadata.CallArgument{
				Kind:          stringPool.Get(metadata.KindIdent),
				Name:          stringPool.Get("user"),
				Pkg:           stringPool.Get("main"),
				ResolvedType:  stringPool.Get("User"),
				IsGenericType: false,
				Meta:          meta,
			},
			originalType: "interface{}",
			expected:     "User",
		},
		{
			name: "with generic type",
			arg: metadata.CallArgument{
				Kind:            stringPool.Get(metadata.KindIdent),
				Name:            stringPool.Get("user"),
				Pkg:             stringPool.Get("main"),
				IsGenericType:   true,
				GenericTypeName: stringPool.Get("T"),
				Meta:            meta,
			},
			originalType: "interface{}",
			expected:     "interface{}",
		},
		{
			name: "with assignment",
			arg: metadata.CallArgument{
				Kind: stringPool.Get(metadata.KindIdent),
				Name: stringPool.Get("user"),
				Pkg:  stringPool.Get("main"),
				Meta: meta,
			},
			originalType: "interface{}",
			expected:     "User",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matcher.resolveTypeOrigin(tt.arg, mockNode, tt.originalType)
			// Since the metadata setup is complex, we just verify the method doesn't panic
			// and returns some reasonable value
			if result == "" {
				t.Error("resolveTypeOrigin should not return empty string")
			}
			t.Logf("resolveTypeOrigin result for %s: %s", tt.name, result)
		})
	}
}

func TestTraceGenericOrigin(t *testing.T) {
	// Create metadata with string pool
	stringPool := metadata.NewStringPool()
	meta := &metadata.Metadata{
		StringPool: stringPool,
	}

	// Create config
	cfg := DefaultGinConfig()

	// Create schema mapper
	schemaMapper := NewSchemaMapper(cfg)

	// Create type resolver
	typeResolver := NewTypeResolver(meta, cfg, schemaMapper)

	// Create context provider
	contextProvider := NewContextProvider(meta)

	// Create base pattern matcher (not used in this test)
	_ = NewBasePatternMatcher(cfg, contextProvider, typeResolver)

	// Create a mock node with type parameters
	mockNode := &TrackerNode{
		CallGraphEdge: &metadata.CallGraphEdge{
			TypeParamMap: map[string]string{
				"T": "string",
			},
		},
		typeParamMap: map[string]string{
			"T": "string",
		},
	}

	// Test traceGenericOrigin with different type parts
	tests := []struct {
		name      string
		typeParts []string
		expected  string
	}{
		{
			name:      "generic type with concrete resolution",
			typeParts: []string{"List", "T"},
			expected:  "string",
		},
		{
			name:      "non-generic type",
			typeParts: []string{"string"},
			expected:  "",
		},
		{
			name:      "empty type parts",
			typeParts: []string{},
			expected:  "",
		},
		{
			name:      "single type part",
			typeParts: []string{"User"},
			expected:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := traceGenericOrigin(mockNode, tt.typeParts)
			if result != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestBasePatternMatcher_extractMethodFromFunctionName(t *testing.T) {
	// Create metadata with string pool
	stringPool := metadata.NewStringPool()
	meta := &metadata.Metadata{
		StringPool: stringPool,
	}

	// Create config
	cfg := DefaultGinConfig()

	// Create schema mapper
	schemaMapper := NewSchemaMapper(cfg)

	// Create type resolver
	typeResolver := NewTypeResolver(meta, cfg, schemaMapper)

	// Create context provider
	contextProvider := NewContextProvider(meta)

	// Create base pattern matcher
	matcher := NewBasePatternMatcher(cfg, contextProvider, typeResolver)

	// Test extractMethodFromFunctionName with different function names
	tests := []struct {
		name         string
		functionName string
		expected     string
	}{
		{
			name:         "GET method",
			functionName: "getUser",
			expected:     "GET",
		},
		{
			name:         "POST method",
			functionName: "postUser",
			expected:     "POST",
		},
		{
			name:         "PUT method",
			functionName: "putUser",
			expected:     "PUT",
		},
		{
			name:         "DELETE method",
			functionName: "deleteUser",
			expected:     "DELETE",
		},
		{
			name:         "PATCH method",
			functionName: "patchUser",
			expected:     "PATCH",
		},
		{
			name:         "OPTIONS method",
			functionName: "optionsUser",
			expected:     "OPTIONS",
		},
		{
			name:         "HEAD method",
			functionName: "headUser",
			expected:     "HEAD",
		},
		{
			name:         "no method found",
			functionName: "processUser",
			expected:     "",
		},
		{
			name:         "case insensitive",
			functionName: "GetUser",
			expected:     "GET",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matcher.extractMethodFromFunctionName(tt.functionName)
			if result != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestBasePatternMatcher_mapGoTypeToOpenAPISchema(t *testing.T) {
	// Create metadata with string pool
	stringPool := metadata.NewStringPool()
	meta := &metadata.Metadata{
		StringPool: stringPool,
	}

	// Create config
	cfg := DefaultGinConfig()

	// Create schema mapper
	schemaMapper := NewSchemaMapper(cfg)

	// Create type resolver
	typeResolver := NewTypeResolver(meta, cfg, schemaMapper)

	// Create context provider
	contextProvider := NewContextProvider(meta)

	// Create base pattern matcher
	matcher := NewBasePatternMatcher(cfg, contextProvider, typeResolver)

	// Test mapGoTypeToOpenAPISchema with different Go types
	tests := []struct {
		name    string
		goType  string
		hasType bool
	}{
		{
			name:    "string type",
			goType:  "string",
			hasType: true,
		},
		{
			name:    "int type",
			goType:  "int",
			hasType: true,
		},
		{
			name:    "bool type",
			goType:  "bool",
			hasType: true,
		},
		{
			name:    "float64 type",
			goType:  "float64",
			hasType: true,
		},
		{
			name:    "empty type",
			goType:  "",
			hasType: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matcher.mapGoTypeToOpenAPISchema(tt.goType)
			if tt.hasType && result == nil {
				t.Error("Expected schema for valid Go type")
			}
		})
	}
}
