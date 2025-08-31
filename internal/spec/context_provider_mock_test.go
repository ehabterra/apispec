package spec

import (
	"testing"

	"github.com/ehabterra/swagen/internal/metadata"
)

// TestContextProvider_WithMockTrackerTree demonstrates proper use of MockTrackerTree for testing
func TestContextProvider_WithMockTrackerTree(t *testing.T) {
	// Create metadata with string pool
	stringPool := metadata.NewStringPool()
	meta := &metadata.Metadata{
		StringPool: stringPool,
	}

	// Create a call graph edge
	caller := metadata.Call{
		Meta: meta,
		Name: stringPool.Get("main"),
		Pkg:  stringPool.Get("main"),
	}
	callee := metadata.Call{
		Meta:     meta,
		Name:     stringPool.Get("handler"),
		Pkg:      stringPool.Get("main"),
		RecvType: stringPool.Get("Handler"),
	}
	edge := metadata.CallGraphEdge{
		Caller: caller,
		Callee: callee,
	}

	// Create a mock node that implements TrackerNodeInterface
	mockNode := &TrackerNode{
		key:           "test-handler",
		CallGraphEdge: &edge,
	}

	// Create mock tracker tree
	limits := metadata.TrackerLimits{
		MaxNodesPerTree:    100,
		MaxChildrenPerNode: 10,
		MaxArgsPerFunction: 5,
		MaxNestedArgsDepth: 3,
	}
	mockTree := NewMockTrackerTree(meta, limits)
	mockTree.AddRoot(mockNode)

	// Test context provider with mock tree
	provider := NewContextProvider(meta)

	// Test GetCalleeInfo with the mock node
	name, pkg, recvType := provider.GetCalleeInfo(mockNode)

	if name != "handler" {
		t.Errorf("Expected name 'handler', got '%s'", name)
	}
	if pkg != "main" {
		t.Errorf("Expected pkg 'main', got '%s'", pkg)
	}
	if recvType != "Handler" {
		t.Errorf("Expected recvType 'Handler', got '%s'", recvType)
	}

	// Verify mock tree functionality
	roots := mockTree.GetRoots()
	if len(roots) != 1 {
		t.Errorf("Expected 1 root, got %d", len(roots))
	}

	if roots[0].GetKey() != "test-handler" {
		t.Errorf("Expected root key 'test-handler', got '%s'", roots[0].GetKey())
	}
}

// TestContextProvider_GetCalleeInfo_WithNilEdge tests edge case handling
func TestContextProvider_GetCalleeInfo_WithNilEdge(t *testing.T) {
	meta := &metadata.Metadata{}
	provider := NewContextProvider(meta)

	// Create a mock node with nil edge
	mockNode := &TrackerNode{
		key:           "test-node",
		CallGraphEdge: nil, // Nil edge
	}

	name, pkg, recvType := provider.GetCalleeInfo(mockNode)

	// Should return empty strings for nil edge
	if name != "" || pkg != "" || recvType != "" {
		t.Errorf("Expected empty strings for nil edge, got name='%s', pkg='%s', recvType='%s'", name, pkg, recvType)
	}
}

// TestContextProvider_GetCalleeInfo_WithMalformedNode tests error handling
func TestContextProvider_GetCalleeInfo_WithMalformedNode(t *testing.T) {
	// Create metadata with string pool
	stringPool := metadata.NewStringPool()
	meta := &metadata.Metadata{
		StringPool: stringPool,
	}

	provider := NewContextProvider(meta)

	// Create edge with invalid indices
	edge := metadata.CallGraphEdge{
		Caller: metadata.Call{
			Meta: meta,
			Name: -1, // Invalid index
			Pkg:  -1, // Invalid index
		},
		Callee: metadata.Call{
			Meta:     meta,
			Name:     -1, // Invalid index
			Pkg:      -1, // Invalid index
			RecvType: -1, // Invalid index
		},
	}

	mockNode := &TrackerNode{
		key:           "malformed-node",
		CallGraphEdge: &edge,
	}

	name, pkg, recvType := provider.GetCalleeInfo(mockNode)

	// Should handle invalid indices gracefully
	if name == "" && pkg == "" && recvType == "" {
		t.Log("Correctly handled malformed node with invalid indices")
	}
}

// TestMockTrackerTree_InterfaceCompliance verifies mock implements interface
func TestMockTrackerTree_InterfaceCompliance(t *testing.T) {
	// Verify MockTrackerTree implements TrackerTreeInterface
	var _ TrackerTreeInterface = (*MockTrackerTree)(nil)

	meta := &metadata.Metadata{}
	limits := metadata.TrackerLimits{
		MaxNodesPerTree:    100,
		MaxChildrenPerNode: 10,
		MaxArgsPerFunction: 5,
		MaxNestedArgsDepth: 3,
	}

	mockTree := NewMockTrackerTree(meta, limits)

	// Test interface methods
	if mockTree.GetMetadata() != meta {
		t.Error("GetMetadata should return the provided metadata")
	}

	if mockTree.GetLimits() != limits {
		t.Error("GetLimits should return the provided limits")
	}

	// Test empty tree
	roots := mockTree.GetRoots()
	if len(roots) != 0 {
		t.Errorf("Expected 0 roots for empty tree, got %d", len(roots))
	}

	count := mockTree.GetNodeCount()
	if count != 0 {
		t.Errorf("Expected 0 nodes for empty tree, got %d", count)
	}
}

// TestMockTrackerTree_WithComplexHierarchy tests mock with complex node hierarchy
func TestMockTrackerTree_WithComplexHierarchy(t *testing.T) {
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
