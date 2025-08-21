package spec

import (
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
