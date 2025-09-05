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

	if extractorTree.GetLimits() != limits {
		t.Error("Expected extractor tree limits to match")
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

// TestMockTrackerTree_ComplexHierarchy tests MockTrackerTree with complex node relationships
func TestMockTrackerTree_ComplexHierarchy(t *testing.T) {
	stringPool := metadata.NewStringPool()
	meta := &metadata.Metadata{
		StringPool: stringPool,
		Packages: map[string]*metadata.Package{
			"main": {
				Files: map[string]*metadata.File{
					"main.go": {
						Functions: map[string]*metadata.Function{
							"main": {
								Name: stringPool.Get("main"),
							},
							"handler": {
								Name: stringPool.Get("handler"),
							},
						},
					},
				},
			},
		},
	}

	limits := metadata.TrackerLimits{
		MaxNodesPerTree:    100,
		MaxChildrenPerNode: 10,
		MaxArgsPerFunction: 5,
		MaxNestedArgsDepth: 3,
	}

	mockTree := NewMockTrackerTree(meta, limits)

	// Create a hierarchy: root -> child1, child2 -> grandchild
	grandchild := &TrackerNode{
		key:      "grandchild",
		Children: []*TrackerNode{},
	}

	child1 := &TrackerNode{
		key:      "child1",
		Children: []*TrackerNode{grandchild},
	}
	grandchild.Parent = child1

	child2 := &TrackerNode{
		key:      "child2",
		Children: []*TrackerNode{},
	}

	root := &TrackerNode{
		key:      "root",
		Children: []*TrackerNode{child1, child2},
	}
	child1.Parent = root
	child2.Parent = root

	mockTree.AddRoot(root)

	// Test tree structure
	roots := mockTree.GetRoots()
	if len(roots) != 1 {
		t.Errorf("Expected 1 root, got %d", len(roots))
	}

	if roots[0].GetKey() != "root" {
		t.Errorf("Expected root key 'root', got '%s'", roots[0].GetKey())
	}

	// Test tree traversal
	visitedKeys := make(map[string]bool)
	mockTree.TraverseTree(func(node TrackerNodeInterface) bool {
		visitedKeys[node.GetKey()] = true
		return true
	})

	expectedKeys := []string{"root", "child1", "child2", "grandchild"}
	for _, key := range expectedKeys {
		if !visitedKeys[key] {
			t.Errorf("Expected to visit node with key '%s'", key)
		}
	}

	// Test node count
	count := mockTree.GetNodeCount()
	if count != 4 {
		t.Errorf("Expected 4 nodes, got %d", count)
	}

	// Test FindNodeByKey
	foundNode := mockTree.FindNodeByKey("grandchild")
	if foundNode == nil {
		t.Error("Expected to find grandchild node")
	}
	if foundNode.GetKey() != "grandchild" {
		t.Errorf("Expected found node key 'grandchild', got '%s'", foundNode.GetKey())
	}

	// Test GetFunctionContext
	fn, pkg, file := mockTree.GetFunctionContext("main")
	if fn == nil {
		t.Error("Expected to find main function")
	}
	if pkg != "main" {
		t.Errorf("Expected package 'main', got '%s'", pkg)
	}
	if file != "main.go" {
		t.Errorf("Expected file 'main.go', got '%s'", file)
	}
}

// TestMockTrackerTree_EdgeCases tests edge cases with MockTrackerTree
func TestMockTrackerTree_EdgeCases(t *testing.T) {
	meta := &metadata.Metadata{}
	limits := metadata.TrackerLimits{
		MaxNodesPerTree:    100,
		MaxChildrenPerNode: 10,
		MaxArgsPerFunction: 5,
		MaxNestedArgsDepth: 3,
	}

	mockTree := NewMockTrackerTree(meta, limits)

	// Test empty tree
	roots := mockTree.GetRoots()
	if len(roots) != 0 {
		t.Errorf("Expected 0 roots for empty tree, got %d", len(roots))
	}

	count := mockTree.GetNodeCount()
	if count != 0 {
		t.Errorf("Expected 0 nodes for empty tree, got %d", count)
	}

	// Test search in empty tree
	found := mockTree.FindNodeByKey("non-existing")
	if found != nil {
		t.Error("Expected nil for search in empty tree")
	}

	// Test traversal of empty tree
	visitCount := 0
	mockTree.TraverseTree(func(node TrackerNodeInterface) bool {
		visitCount++
		return true
	})
	if visitCount != 0 {
		t.Errorf("Expected 0 visits for empty tree, got %d", visitCount)
	}

	// Test function context in empty metadata
	fn, pkg, file := mockTree.GetFunctionContext("any")
	if fn != nil || pkg != "" || file != "" {
		t.Error("Expected empty results for empty metadata")
	}

	// Test adding nil node (should handle gracefully)
	mockTree.AddRoot(nil)
	count = mockTree.GetNodeCount()
	if count != 1 {
		t.Errorf("Expected 1 node after adding nil, got %d", count)
	}
}

// TestMockTrackerTree_EarlyTermination tests traversal early termination
func TestMockTrackerTree_EarlyTermination(t *testing.T) {
	meta := &metadata.Metadata{}
	limits := metadata.TrackerLimits{
		MaxNodesPerTree:    100,
		MaxChildrenPerNode: 10,
		MaxArgsPerFunction: 5,
		MaxNestedArgsDepth: 3,
	}

	mockTree := NewMockTrackerTree(meta, limits)

	// Create multiple root nodes
	root1 := &TrackerNode{key: "root1"}
	root2 := &TrackerNode{key: "root2"}
	root3 := &TrackerNode{key: "root3"}

	mockTree.AddRoot(root1)
	mockTree.AddRoot(root2)
	mockTree.AddRoot(root3)

	// Test early termination after first node
	visitCount := 0
	mockTree.TraverseTree(func(node TrackerNodeInterface) bool {
		visitCount++
		return false // Stop after first node
	})

	if visitCount != 1 {
		t.Errorf("Expected to visit exactly 1 node with early termination, visited %d", visitCount)
	}

	// Test normal traversal
	visitCount = 0
	mockTree.TraverseTree(func(node TrackerNodeInterface) bool {
		visitCount++
		return true // Continue traversal
	})

	if visitCount != 3 {
		t.Errorf("Expected to visit exactly 3 nodes, visited %d", visitCount)
	}
}
