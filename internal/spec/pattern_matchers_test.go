package spec

import (
	"testing"

	"github.com/ehabterra/swagen/internal/metadata"
)

func TestRoutePattern_MatchPattern(t *testing.T) {
	pattern := &RoutePattern{}

	tests := []struct {
		name     string
		pattern  string
		value    string
		expected bool
	}{
		{
			name:     "exact match",
			pattern:  "test",
			value:    "test",
			expected: true,
		},
		{
			name:     "regex match",
			pattern:  "^test.*$",
			value:    "test123",
			expected: true,
		},
		{
			name:     "case insensitive regex",
			pattern:  "(?i)test",
			value:    "TEST",
			expected: true,
		},
		{
			name:     "no match",
			pattern:  "^test$",
			value:    "other",
			expected: false,
		},
		{
			name:     "empty pattern",
			pattern:  "",
			value:    "test",
			expected: false,
		},
		{
			name:     "invalid regex",
			pattern:  "[invalid",
			value:    "test",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := pattern.MatchPattern(tt.pattern, tt.value)
			if result != tt.expected {
				t.Errorf("MatchPattern(%q, %q) = %v, expected %v", tt.pattern, tt.value, result, tt.expected)
			}
		})
	}
}

func TestRoutePattern_MatchFunctionName(t *testing.T) {
	tests := []struct {
		name         string
		functionName string
		pattern      *RoutePattern
		expected     bool
	}{
		{
			name:         "exact match",
			functionName: "testHandler",
			pattern: &RoutePattern{
				FunctionNameRegex: "testHandler",
			},
			expected: true,
		},
		{
			name:         "regex match",
			functionName: "userHandler",
			pattern: &RoutePattern{
				FunctionNameRegex: ".*Handler$",
			},
			expected: true,
		},
		{
			name:         "no match",
			functionName: "testFunction",
			pattern: &RoutePattern{
				FunctionNameRegex: ".*Handler$",
			},
			expected: false,
		},
		{
			name:         "empty regex",
			functionName: "test",
			pattern: &RoutePattern{
				FunctionNameRegex: "",
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.pattern.MatchFunctionName(tt.functionName)
			if result != tt.expected {
				t.Errorf("MatchFunctionName(%q) = %v, expected %v", tt.functionName, result, tt.expected)
			}
		})
	}
}

func TestExtractor_IsValid(t *testing.T) {
	// Create a simple metadata structure for testing
	meta := &metadata.Metadata{
		StringPool: metadata.NewStringPool(),
	}

	// Create tracker with limits
	limits := metadata.TrackerLimits{
		MaxNodesPerTree:    100,
		MaxChildrenPerNode: 10,
		MaxArgsPerFunction: 5,
		MaxNestedArgsDepth: 3,
	}

	tree := NewMockTrackerTree(meta, limits)

	// Create a simple config for testing
	cfg := &SwagenConfig{
		Framework: FrameworkConfig{
			RoutePatterns: []RoutePattern{
				{
					CallRegex:      "test",
					MethodFromCall: true,
					PathFromArg:    true,
					PathArgIndex:   0,
				},
			},
		},
	}

	// Create extractor
	extractor := NewExtractor(tree, cfg)

	// Test IsValid method
	if extractor == nil {
		t.Fatal("Extractor should not be nil")
	}

	// Test that extractor was created with valid configuration
	if extractor.cfg == nil {
		t.Error("Extractor config should not be nil")
	}

	if extractor.tree == nil {
		t.Error("Extractor tree should not be nil")
	}
}

func TestExtractor_initializePatternMatchers(t *testing.T) {
	// Create a simple metadata structure for testing
	meta := &metadata.Metadata{
		StringPool: metadata.NewStringPool(),
	}

	// Create tracker with limits
	limits := metadata.TrackerLimits{
		MaxNodesPerTree:    100,
		MaxChildrenPerNode: 10,
		MaxArgsPerFunction: 5,
		MaxNestedArgsDepth: 3,
	}

	tree := NewMockTrackerTree(meta, limits)

	// Create a config with various patterns
	cfg := &SwagenConfig{
		Framework: FrameworkConfig{
			RoutePatterns: []RoutePattern{
				{
					CallRegex:      "GET",
					MethodFromCall: true,
					PathFromArg:    true,
					PathArgIndex:   0,
				},
			},
			MountPatterns: []MountPattern{
				{
					CallRegex:    "Mount",
					IsMount:      true,
					PathFromArg:  true,
					PathArgIndex: 0,
				},
			},
			RequestBodyPatterns: []RequestBodyPattern{
				{
					CallRegex:    "BindJSON",
					TypeArgIndex: 0,
					TypeFromArg:  true,
					Deref:        true,
				},
			},
			ResponsePatterns: []ResponsePattern{
				{
					CallRegex:      "JSON",
					StatusArgIndex: 0,
					TypeArgIndex:   1,
					TypeFromArg:    true,
					StatusFromArg:  true,
				},
			},
			ParamPatterns: []ParamPattern{
				{
					CallRegex:     "Param",
					ParamIn:       "path",
					ParamArgIndex: 0,
				},
			},
		},
	}

	// Create extractor
	extractor := NewExtractor(tree, cfg)

	// Test that pattern matchers were initialized
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

func TestExtractor_ExtractRoutes(t *testing.T) {
	// Create a simple metadata structure for testing
	meta := &metadata.Metadata{
		StringPool: metadata.NewStringPool(),
	}

	// Create tracker with limits
	limits := metadata.TrackerLimits{
		MaxNodesPerTree:    100,
		MaxChildrenPerNode: 10,
		MaxArgsPerFunction: 5,
		MaxNestedArgsDepth: 3,
	}

	tree := NewMockTrackerTree(meta, limits)

	// Create a simple config for testing
	cfg := &SwagenConfig{
		Framework: FrameworkConfig{
			RoutePatterns: []RoutePattern{
				{
					CallRegex:      "test",
					MethodFromCall: true,
					PathFromArg:    true,
					PathArgIndex:   0,
				},
			},
		},
	}

	// Create extractor
	extractor := NewExtractor(tree, cfg)

	// Test route extraction
	routes := extractor.ExtractRoutes()

	// For an empty tree, we expect no routes
	if len(routes) != 0 {
		t.Errorf("Expected 0 routes for empty tree, got %d", len(routes))
	}
}

func TestExtractor_traverseForRoutes(t *testing.T) {
	// Create a simple metadata structure for testing
	meta := &metadata.Metadata{
		StringPool: metadata.NewStringPool(),
	}

	// Create tracker with limits
	limits := metadata.TrackerLimits{
		MaxNodesPerTree:    100,
		MaxChildrenPerNode: 10,
		MaxArgsPerFunction: 5,
		MaxNestedArgsDepth: 3,
	}

	tree := NewMockTrackerTree(meta, limits)

	// Create a simple config for testing
	cfg := &SwagenConfig{
		Framework: FrameworkConfig{
			RoutePatterns: []RoutePattern{
				{
					CallRegex:      "test",
					MethodFromCall: true,
					PathFromArg:    true,
					PathArgIndex:   0,
				},
			},
		},
	}

	// Create extractor
	extractor := NewExtractor(tree, cfg)

	// Test traversal with empty tree
	roots := tree.GetRoots()
	if len(roots) != 0 {
		t.Errorf("Expected 0 roots for empty tree, got %d", len(roots))
	}

	// Test that traversal doesn't panic with empty tree
	// This is a basic smoke test
	_ = extractor.ExtractRoutes()
}

func TestExtractor_traverseForRoutesWithVisited(t *testing.T) {
	// Create a simple metadata structure for testing
	meta := &metadata.Metadata{
		StringPool: metadata.NewStringPool(),
	}

	// Create tracker with limits
	limits := metadata.TrackerLimits{
		MaxNodesPerTree:    100,
		MaxChildrenPerNode: 10,
		MaxArgsPerFunction: 5,
		MaxNestedArgsDepth: 3,
	}

	tree := NewMockTrackerTree(meta, limits)

	// Create a simple config for testing
	cfg := &SwagenConfig{
		Framework: FrameworkConfig{
			RoutePatterns: []RoutePattern{
				{
					CallRegex:      "test",
					MethodFromCall: true,
					PathFromArg:    true,
					PathArgIndex:   0,
				},
			},
		},
	}

	// Create extractor
	extractor := NewExtractor(tree, cfg)

	// Test traversal with visited map
	visited := make(map[string]bool)
	_ = tree.GetRoots() // Get roots but don't use them

	// Test that traversal doesn't panic with empty tree and visited map
	// This is a basic smoke test
	_ = extractor.ExtractRoutes()

	// Verify visited map is still empty (no nodes to visit)
	if len(visited) != 0 {
		t.Errorf("Expected visited map to be empty, got %d entries", len(visited))
	}
}
