// Copyright 2025 Ehab Terra
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package spec

import (
	"testing"

	"github.com/ehabterra/apispec/internal/metadata"
)

// TestExtractorWithMockTrackerTree demonstrates proper use of MockTrackerTree for extractor testing
func TestExtractorWithMockTrackerTree(t *testing.T) {
	// Create metadata with string pool
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

	// Create mock tracker tree for isolated testing
	mockTree := NewMockTrackerTree(meta, limits)

	// Create a test node that simulates a router pattern
	testNode := &TrackerNode{
		key:           "test-router-node",
		CallGraphEdge: nil, // Simple test case without complex edge
	}
	mockTree.AddRoot(testNode)

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
		},
		Defaults: Defaults{
			RequestContentType:  "application/json",
			ResponseContentType: "application/json",
			ResponseStatus:      200,
		},
	}

	// Create extractor with mock tree
	extractor := NewExtractor(mockTree, cfg)

	// Test extraction - should work without errors
	routes := extractor.ExtractRoutes()

	// Verify basic functionality
	if routes == nil {
		t.Error("Expected non-nil routes slice")
	}

	// Test extractor properties
	if extractor.tree != mockTree {
		t.Error("Expected extractor to use the mock tree")
	}

	if extractor.cfg != cfg {
		t.Error("Expected extractor to use the provided config")
	}

	// Test that mock tree is accessible through interface
	extractorTree := extractor.tree
	if extractorTree.GetMetadata() != meta {
		t.Error("Expected extractor tree metadata to match")
	}
}

// TestPatternMatchersWithMockNodes tests pattern matchers with mock nodes
func TestPatternMatchersWithMockNodes(t *testing.T) {
	stringPool := metadata.NewStringPool()
	meta := &metadata.Metadata{
		StringPool: stringPool,
	}

	// Create context provider
	contextProvider := NewContextProvider(meta)

	// Create config and schema mapper
	cfg := &APISpecConfig{}
	schemaMapper := NewSchemaMapper(cfg)
	typeResolver := NewTypeResolver(meta, cfg, schemaMapper)

	// Test route pattern matcher
	routePattern := RoutePattern{
		CallRegex:      "Get",
		MethodFromCall: true,
		PathFromArg:    true,
		PathArgIndex:   0,
	}

	matcher := NewRoutePatternMatcher(routePattern, cfg, contextProvider, typeResolver)

	// Create a mock node for testing
	mockNode := &TrackerNode{
		key:           "mock-get-node",
		CallGraphEdge: nil, // Simple case for unit testing
	}

	// Test pattern matching functionality
	if matcher.GetPriority() <= 0 {
		t.Error("Expected positive priority for route pattern matcher")
	}

	pattern := matcher.GetPattern()
	if pattern == nil {
		t.Error("Expected non-nil pattern")
	}

	// Test MatchNode - should handle mock node gracefully
	matches := matcher.MatchNode(mockNode)
	// Expected result depends on implementation, but should not panic
	_ = matches
}

// TestTypeResolverWithMockNodes tests type resolver with mock nodes
func TestTypeResolverWithMockNodes(t *testing.T) {
	stringPool := metadata.NewStringPool()
	meta := &metadata.Metadata{
		StringPool: stringPool,
	}

	cfg := &APISpecConfig{}
	schemaMapper := NewSchemaMapper(cfg)
	typeResolver := NewTypeResolver(meta, cfg, schemaMapper)

	// Create a mock node for type resolution context
	mockNode := &TrackerNode{
		key: "mock-type-node",
		typeParamMap: map[string]string{
			"T": "string",
			"U": "int",
		},
	}

	// Create test arguments for type resolution
	testArg := metadata.NewCallArgument(meta)
	testArg.SetKind("ident")
	testArg.SetName("testVar")
	testArg.SetType("T")

	// Test type resolution with mock context
	resolvedType := typeResolver.ResolveType(*testArg, mockNode)
	// Should handle mock node gracefully
	_ = resolvedType

	// Test with nil node - should not panic
	testArg2 := metadata.NewCallArgument(meta)
	testArg2.SetKind("ident")
	testArg2.SetType("string")
	resolvedType = typeResolver.ResolveType(*testArg2, nil)
	if resolvedType == "" {
		t.Error("Expected type resolver to handle nil node gracefully")
	}
}

// TestContextProviderWithMockNodes tests context provider with mock nodes
func TestContextProviderWithMockNodes(t *testing.T) {
	stringPool := metadata.NewStringPool()
	meta := &metadata.Metadata{
		StringPool: stringPool,
	}

	provider := NewContextProvider(meta)

	// Test with mock node that has edge
	mockEdge := &metadata.CallGraphEdge{
		Caller: metadata.Call{
			Meta: meta,
			Name: stringPool.Get("mockCaller"),
			Pkg:  stringPool.Get("main"),
		},
		Callee: metadata.Call{
			Meta:     meta,
			Name:     stringPool.Get("mockCallee"),
			Pkg:      stringPool.Get("handlers"),
			RecvType: stringPool.Get("Handler"),
		},
	}

	mockNode := &TrackerNode{
		key:           "mock-node-with-edge",
		CallGraphEdge: mockEdge,
	}

	// Test GetCalleeInfo with mock node
	name, pkg, recvType := provider.GetCalleeInfo(mockNode)
	if name != "mockCallee" {
		t.Errorf("Expected name 'mockCallee', got '%s'", name)
	}
	if pkg != "handlers" {
		t.Errorf("Expected pkg 'handlers', got '%s'", pkg)
	}
	if recvType != "Handler" {
		t.Errorf("Expected recvType 'Handler', got '%s'", recvType)
	}

	// Test with mock node without edge
	mockNodeNoEdge := &TrackerNode{
		key:           "mock-node-no-edge",
		CallGraphEdge: nil,
	}

	name, pkg, recvType = provider.GetCalleeInfo(mockNodeNoEdge)
	if name != "" || pkg != "" || recvType != "" {
		t.Errorf("Expected empty strings for node without edge, got name='%s', pkg='%s', recvType='%s'",
			name, pkg, recvType)
	}
}
