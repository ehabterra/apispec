package spec

import (
	"testing"

	"github.com/ehabterra/swagen/internal/metadata"
)

func TestSimplifiedTrackerTree_InterfaceImplementation(t *testing.T) {
	// Test that SimplifiedTrackerTree implements TrackerTreeInterface
	var _ TrackerTreeInterface = (*SimplifiedTrackerTree)(nil)
}

func TestSimplifiedTrackerNode_InterfaceImplementation(t *testing.T) {
	// Test that SimplifiedTrackerNode implements TrackerNodeInterface
	var _ TrackerNodeInterface = (*SimplifiedTrackerNode)(nil)
}

func TestNewSimplifiedTrackerTree(t *testing.T) {
	// Create test metadata
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

	tree := NewSimplifiedTrackerTree(meta, limits)

	if tree == nil {
		t.Fatal("Expected non-nil tracker tree")
	}

	if tree.GetMetadata() != meta {
		t.Error("Expected metadata to match")
	}

	if tree.GetLimits() != limits {
		t.Error("Expected limits to match")
	}
}

func TestSimplifiedTrackerTree_GetRoots(t *testing.T) {
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

	tree := NewSimplifiedTrackerTree(meta, limits)
	roots := tree.GetRoots()

	// Should return interface slice
	if roots == nil {
		t.Fatal("Expected non-nil roots")
	}

	// Should be able to access interface methods
	for _, root := range roots {
		if root.GetKey() == "" {
			t.Error("Expected non-empty key for root node")
		}
	}
}

func TestSimplifiedTrackerTree_GetNodeCount(t *testing.T) {
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

	tree := NewSimplifiedTrackerTree(meta, limits)
	count := tree.GetNodeCount()

	if count < 0 {
		t.Error("Expected non-negative node count")
	}
}

func TestSimplifiedTrackerTree_FindNodeByKey(t *testing.T) {
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

	tree := NewSimplifiedTrackerTree(meta, limits)

	// Test finding existing node
	roots := tree.GetRoots()
	if len(roots) > 0 {
		existingKey := roots[0].GetKey()
		foundNode := tree.FindNodeByKey(existingKey)
		if foundNode == nil {
			t.Errorf("Expected to find node with key: %s", existingKey)
		}
		if foundNode.GetKey() != existingKey {
			t.Errorf("Expected found node to have key: %s, got: %s", existingKey, foundNode.GetKey())
		}
	}

	// Test finding non-existing node
	nonExistingNode := tree.FindNodeByKey("non-existing-key")
	if nonExistingNode != nil {
		t.Error("Expected nil for non-existing key")
	}
}

func TestSimplifiedTrackerTree_GetFunctionContext(t *testing.T) {
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

	tree := NewSimplifiedTrackerTree(meta, limits)

	// Test finding existing function
	fn, pkg, file := tree.GetFunctionContext("main")
	if fn == nil {
		t.Error("Expected to find function 'main'")
	}
	if pkg != "main" {
		t.Errorf("Expected package 'main', got: %s", pkg)
	}
	if file != "main.go" {
		t.Errorf("Expected file 'main.go', got: %s", file)
	}

	// Test finding non-existing function
	fn, pkg, file = tree.GetFunctionContext("non-existing-function")
	if fn != nil {
		t.Error("Expected nil for non-existing function")
	}
	if pkg != "" {
		t.Errorf("Expected empty package, got: %s", pkg)
	}
	if file != "" {
		t.Errorf("Expected empty file, got: %s", file)
	}
}

func TestSimplifiedTrackerTree_TraverseTree(t *testing.T) {
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

	tree := NewSimplifiedTrackerTree(meta, limits)

	// Test traversal - may not have nodes in this simple test
	visitedNodes := make(map[string]bool)
	tree.TraverseTree(func(node TrackerNodeInterface) bool {
		visitedNodes[node.GetKey()] = true
		return true // Continue traversal
	})

	// If there are nodes, we should visit them
	if len(visitedNodes) > 0 {
		// Test early termination
		visitCount := 0
		tree.TraverseTree(func(node TrackerNodeInterface) bool {
			visitCount++
			return false // Stop traversal after first node
		})

		if visitCount != 1 {
			t.Errorf("Expected to visit exactly 1 node, visited: %d", visitCount)
		}
	} else {
		// If no nodes, traversal should still work without errors
		tree.TraverseTree(func(node TrackerNodeInterface) bool {
			return true
		})
	}
}

func TestSimplifiedTrackerNode_InterfaceMethods(t *testing.T) {
	// Create a test node
	node := &SimplifiedTrackerNode{
		Key:        "test-node",
		Parent:     nil,
		Children:   []*SimplifiedTrackerNode{},
		Edge:       nil,
		Argument:   nil,
		ArgType:    metadata.ArgTypeVariable,
		ArgIndex:   0,
		ArgContext: "test-context",
		TypeParamMap: map[string]string{
			"T": "string",
		},
	}

	// Test GetKey
	if node.GetKey() != "test-node" {
		t.Errorf("Expected key 'test-node', got: %s", node.GetKey())
	}

	// Test GetParent
	if node.GetParent() != nil {
		t.Error("Expected nil parent")
	}

	// Test GetChildren
	children := node.GetChildren()
	if len(children) != 0 {
		t.Errorf("Expected 0 children, got: %d", len(children))
	}

	// Test GetEdge
	if node.GetEdge() != nil {
		t.Error("Expected nil edge")
	}

	// Test GetArgument
	if node.GetArgument() != nil {
		t.Error("Expected nil argument")
	}

	// Test GetArgType
	if node.GetArgType() != metadata.ArgTypeVariable {
		t.Errorf("Expected ArgType ArgTypeVariable, got: %v", node.GetArgType())
	}

	// Test GetArgIndex
	if node.GetArgIndex() != 0 {
		t.Errorf("Expected ArgIndex 0, got: %d", node.GetArgIndex())
	}

	// Test GetArgContext
	if node.GetArgContext() != "test-context" {
		t.Errorf("Expected ArgContext 'test-context', got: %s", node.GetArgContext())
	}

	// Test GetTypeParamMap
	typeParams := node.GetTypeParamMap()
	if len(typeParams) != 1 {
		t.Errorf("Expected 1 type parameter, got: %d", len(typeParams))
	}
	if typeParams["T"] != "string" {
		t.Errorf("Expected type parameter T='string', got: %s", typeParams["T"])
	}
}

func TestSimplifiedTrackerNode_WithParentAndChildren(t *testing.T) {
	// Create parent node
	parent := &SimplifiedTrackerNode{
		Key: "parent-node",
	}

	// Create child nodes
	child1 := &SimplifiedTrackerNode{
		Key: "child-1",
	}
	child2 := &SimplifiedTrackerNode{
		Key: "child-2",
	}

	// Create node with parent and children
	node := &SimplifiedTrackerNode{
		Key:      "test-node",
		Parent:   parent,
		Children: []*SimplifiedTrackerNode{child1, child2},
	}

	// Test GetParent
	retrievedParent := node.GetParent()
	if retrievedParent == nil {
		t.Fatal("Expected non-nil parent")
	}
	if retrievedParent.GetKey() != "parent-node" {
		t.Errorf("Expected parent key 'parent-node', got: %s", retrievedParent.GetKey())
	}

	// Test GetChildren
	children := node.GetChildren()
	if len(children) != 2 {
		t.Errorf("Expected 2 children, got: %d", len(children))
	}

	childKeys := make(map[string]bool)
	for _, child := range children {
		childKeys[child.GetKey()] = true
	}

	if !childKeys["child-1"] {
		t.Error("Expected to find child-1")
	}
	if !childKeys["child-2"] {
		t.Error("Expected to find child-2")
	}
}

func TestSimplifiedTrackerTree_EmptyMetadata(t *testing.T) {
	// Test with empty metadata
	meta := &metadata.Metadata{}
	limits := metadata.TrackerLimits{
		MaxNodesPerTree:    100,
		MaxChildrenPerNode: 10,
		MaxArgsPerFunction: 5,
		MaxNestedArgsDepth: 3,
	}

	tree := NewSimplifiedTrackerTree(meta, limits)

	// Should handle empty metadata gracefully
	roots := tree.GetRoots()
	if roots == nil {
		t.Fatal("Expected non-nil roots even with empty metadata")
	}

	count := tree.GetNodeCount()
	if count < 0 {
		t.Error("Expected non-negative count even with empty metadata")
	}
}
