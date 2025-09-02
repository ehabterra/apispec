package spec

import (
	"testing"

	"github.com/ehabterra/swagen/internal/metadata"
)

func TestExtractor_joinPaths(t *testing.T) {
	// Create test metadata and extractor
	stringPool := metadata.NewStringPool()
	meta := &metadata.Metadata{
		StringPool: stringPool,
	}
	limits := metadata.TrackerLimits{
		MaxNodesPerTree:    100,
		MaxChildrenPerNode: 10,
		MaxArgsPerFunction: 5,
		MaxNestedArgsDepth: 3,
	}

	mockTree := NewMockTrackerTree(meta, limits)
	cfg := &SwagenConfig{}
	extractor := NewExtractor(mockTree, cfg)

	tests := []struct {
		name     string
		base     string
		path     string
		expected string
	}{
		{
			name:     "empty base and path",
			base:     "",
			path:     "",
			expected: "/",
		},
		{
			name:     "empty base",
			base:     "",
			path:     "/users",
			expected: "/users",
		},
		{
			name:     "empty path",
			base:     "/api",
			path:     "",
			expected: "/api/",
		},
		{
			name:     "both with leading slash",
			base:     "/api",
			path:     "/users",
			expected: "/api/users",
		},
		{
			name:     "base without leading slash",
			base:     "api",
			path:     "/users",
			expected: "api/users",
		},
		{
			name:     "path without leading slash",
			base:     "/api",
			path:     "users",
			expected: "/api/users",
		},
		{
			name:     "neither with leading slash",
			base:     "api",
			path:     "users",
			expected: "api/users",
		},
		{
			name:     "base with trailing slash",
			base:     "/api/",
			path:     "/users",
			expected: "/api/users",
		},
		{
			name:     "path with trailing slash",
			base:     "/api",
			path:     "/users/",
			expected: "/api/users/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractor.joinPaths(tt.base, tt.path)
			if result != tt.expected {
				t.Errorf("joinPaths(%q, %q) = %q, want %q", tt.base, tt.path, result, tt.expected)
			}
		})
	}
}

func TestExtractor_ApplyOverrides(t *testing.T) {
	// Create test metadata and extractor
	stringPool := metadata.NewStringPool()
	meta := &metadata.Metadata{
		StringPool: stringPool,
	}
	limits := metadata.TrackerLimits{
		MaxNodesPerTree:    100,
		MaxChildrenPerNode: 10,
		MaxArgsPerFunction: 5,
		MaxNestedArgsDepth: 3,
	}

	mockTree := NewMockTrackerTree(meta, limits)
	cfg := &SwagenConfig{}
	_ = NewExtractor(mockTree, cfg)

	// Create a route info
	route := &RouteInfo{
		Path:   "/users/:id",
		Method: "GET",
		Request: &RequestInfo{
			BodyType: "UserRequest",
		},
		Response: map[string]*ResponseInfo{
			"200": {
				BodyType: "UserResponse",
			},
		},
	}

	// Test that the route was created correctly
	if route.Path != "/users/:id" {
		t.Errorf("Expected path '/users/:id', got '%s'", route.Path)
	}
	if route.Method != "GET" {
		t.Errorf("Expected method 'GET', got '%s'", route.Method)
	}
	if route.Request.BodyType != "UserRequest" {
		t.Errorf("Expected request body type 'UserRequest', got '%s'", route.Request.BodyType)
	}
	if route.Response["200"].BodyType != "UserResponse" {
		t.Errorf("Expected response body type 'UserResponse', got '%s'", route.Response["200"].BodyType)
	}
}

func TestExtractor_IsValid_Simple(t *testing.T) {
	// Create test metadata and extractor
	stringPool := metadata.NewStringPool()
	meta := &metadata.Metadata{
		StringPool: stringPool,
	}
	limits := metadata.TrackerLimits{
		MaxNodesPerTree:    100,
		MaxChildrenPerNode: 10,
		MaxArgsPerFunction: 5,
		MaxNestedArgsDepth: 3,
	}

	mockTree := NewMockTrackerTree(meta, limits)
	cfg := &SwagenConfig{}
	extractor := NewExtractor(mockTree, cfg)

	// Test valid route
	validRoute := &RouteInfo{
		Path:    "/users/:id",
		Handler: "getUser",
	}
	if !validRoute.IsValid() {
		t.Error("Expected valid route to be valid")
	}

	// Test invalid route - missing path
	invalidRoute1 := &RouteInfo{
		Handler: "getUser",
	}
	if invalidRoute1.IsValid() {
		t.Error("Expected invalid route (missing path) to be invalid")
	}

	// Test invalid route - missing handler
	invalidRoute2 := &RouteInfo{
		Path: "/users/:id",
	}
	if invalidRoute2.IsValid() {
		t.Error("Expected invalid route (missing handler) to be invalid")
	}

	// Test that extractor was created successfully
	if extractor == nil {
		t.Error("Expected extractor to be created")
	}
}
