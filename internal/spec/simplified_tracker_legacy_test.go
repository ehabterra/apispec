package spec

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ehabterra/swagen/internal/metadata"
)

// TestSimplifiedTrackerTree_LegacyTests adapts old tracker tests to work with new simplified tracker
func TestSimplifiedTrackerTree_LegacyTests(t *testing.T) {
	t.Run("EmptyTree", testEmptyTree)
	t.Run("MainSimpleTree", testMainSimpleTree)
	t.Run("StructTypesWithMethods", testStructTypesWithMethods)
	t.Run("ComplexCallGraph", testComplexCallGraph)
	t.Run("GenericFunctions", testGenericFunctions)
	t.Run("MultiPackage", testMultiPackage)
}

// testEmptyTree tests empty tree scenario
func testEmptyTree(t *testing.T) {
	meta := &metadata.Metadata{}
	limits := metadata.TrackerLimits{
		MaxNodesPerTree:    100,
		MaxChildrenPerNode: 10,
		MaxArgsPerFunction: 5,
		MaxNestedArgsDepth: 3,
	}

	tree := NewSimplifiedTrackerTree(meta, limits)
	require.NotNil(t, tree)

	roots := tree.GetRoots()
	assert.NotNil(t, roots)
	assert.Len(t, roots, 0)

	nodeCount := tree.GetNodeCount()
	assert.Equal(t, 0, nodeCount)
}

// testMainSimpleTree tests main simple tree scenario
func testMainSimpleTree(t *testing.T) {
	meta, err := metadata.LoadMetadata("tests/main.yaml")
	require.NoError(t, err, "Failed to load metadata from tests/main.yaml")

	limits := metadata.TrackerLimits{
		MaxNodesPerTree:    100,
		MaxChildrenPerNode: 10,
		MaxArgsPerFunction: 5,
		MaxNestedArgsDepth: 3,
	}

	tree := NewSimplifiedTrackerTree(meta, limits)
	require.NotNil(t, tree)

	treeRoots := tree.GetRoots()
	require.NotNil(t, treeRoots)
	require.GreaterOrEqual(t, len(treeRoots), 1, "Should have at least one root node")

	// Find the main.main root node
	var mainRoot TrackerNodeInterface
	for _, root := range treeRoots {
		if strings.Contains(root.GetKey(), "main.main") {
			mainRoot = root
			break
		}
	}
	require.NotNil(t, mainRoot, "Should find main.main root node")

	// Check children
	children := mainRoot.GetChildren()
	assert.GreaterOrEqual(t, len(children), 3) // Should have at least 3 children

	// Verify expected function calls exist
	foundFunctions := make(map[string]bool)
	for _, child := range children {
		key := child.GetKey()
		if strings.Contains(key, "fmt.Sprintf") {
			foundFunctions["Sprintf"] = true
		} else if strings.Contains(key, "fmt.Println") {
			foundFunctions["Println"] = true
		} else if strings.Contains(key, "strings.ToUpper") {
			foundFunctions["ToUpper"] = true
		}
	}

	assert.True(t, foundFunctions["Sprintf"], "Should find fmt.Sprintf call")
	assert.True(t, foundFunctions["Println"], "Should find fmt.Println call")
	assert.True(t, foundFunctions["ToUpper"], "Should find strings.ToUpper call")
}

// testStructTypesWithMethods tests struct types with methods scenario
func testStructTypesWithMethods(t *testing.T) {
	meta, err := metadata.LoadMetadata("tests/example.yaml")
	require.NoError(t, err, "Failed to load metadata from tests/example.yaml")

	limits := metadata.TrackerLimits{
		MaxNodesPerTree:    100,
		MaxChildrenPerNode: 10,
		MaxArgsPerFunction: 5,
		MaxNestedArgsDepth: 3,
	}

	tree := NewSimplifiedTrackerTree(meta, limits)
	require.NotNil(t, tree)

	roots := tree.GetRoots()
	require.NotNil(t, roots)
	require.GreaterOrEqual(t, len(roots), 1, "Should have at least one root node")

	// Find the example.main root node
	var mainRoot TrackerNodeInterface
	for _, root := range roots {
		if strings.Contains(root.GetKey(), "example.main") {
			mainRoot = root
			break
		}
	}
	require.NotNil(t, mainRoot, "Should find example.main root node")

	// Check root node
	assert.Contains(t, mainRoot.GetKey(), "example.main")

	// Check children
	children := mainRoot.GetChildren()
	assert.GreaterOrEqual(t, len(children), 2) // Should have at least 2 children

	// Verify expected function calls exist
	foundFunctions := make(map[string]bool)
	for _, child := range children {
		key := child.GetKey()
		if strings.Contains(key, "NewUser") {
			foundFunctions["NewUser"] = true
		} else if strings.Contains(key, "Println") {
			foundFunctions["Println"] = true
		}
	}

	assert.True(t, foundFunctions["NewUser"], "Should find NewUser call")
	assert.True(t, foundFunctions["Println"], "Should find Println call")
}

// testComplexCallGraph tests complex call graph scenario
func testComplexCallGraph(t *testing.T) {
	meta, err := metadata.LoadMetadata("tests/complex.yaml")
	require.NoError(t, err, "Failed to load metadata from tests/complex.yaml")

	limits := metadata.TrackerLimits{
		MaxNodesPerTree:    100,
		MaxChildrenPerNode: 10,
		MaxArgsPerFunction: 5,
		MaxNestedArgsDepth: 3,
	}

	tree := NewSimplifiedTrackerTree(meta, limits)
	require.NotNil(t, tree)

	roots := tree.GetRoots()
	require.NotNil(t, roots)
	require.GreaterOrEqual(t, len(roots), 1, "Should have at least one root node")

	// Find the complex.main root node
	var mainRoot TrackerNodeInterface
	for _, root := range roots {
		if strings.Contains(root.GetKey(), "complex.main") {
			mainRoot = root
			break
		}
	}
	require.NotNil(t, mainRoot, "Should find complex.main root node")

	// Check root node
	assert.Contains(t, mainRoot.GetKey(), "complex.main")

	// Check children
	children := mainRoot.GetChildren()
	assert.GreaterOrEqual(t, len(children), 3) // Should have at least 3 children

	// Verify expected function calls exist
	foundFunctions := make(map[string]bool)
	for _, child := range children {
		key := child.GetKey()
		if strings.Contains(key, "NewService") {
			foundFunctions["NewService"] = true
		} else if strings.Contains(key, "NewHandler") {
			foundFunctions["NewHandler"] = true
		} else if strings.Contains(key, "Handle") {
			foundFunctions["Handle"] = true
		}
	}

	assert.True(t, foundFunctions["NewService"], "Should find NewService call")
	assert.True(t, foundFunctions["NewHandler"], "Should find NewHandler call")
	assert.True(t, foundFunctions["Handle"], "Should find Handle call")
}

// testGenericFunctions tests generic functions scenario
func testGenericFunctions(t *testing.T) {
	meta, err := metadata.LoadMetadata("tests/generic.yaml")
	require.NoError(t, err, "Failed to load metadata from tests/generic.yaml")

	limits := metadata.TrackerLimits{
		MaxNodesPerTree:    100,
		MaxChildrenPerNode: 10,
		MaxArgsPerFunction: 5,
		MaxNestedArgsDepth: 3,
	}

	tree := NewSimplifiedTrackerTree(meta, limits)
	require.NotNil(t, tree)

	roots := tree.GetRoots()
	require.NotNil(t, roots)
	require.GreaterOrEqual(t, len(roots), 1, "Should have at least one root node")

	// Find the generic.main root node
	var mainRoot TrackerNodeInterface
	for _, root := range roots {
		if strings.Contains(root.GetKey(), "generic.main") {
			mainRoot = root
			break
		}
	}
	require.NotNil(t, mainRoot, "Should find generic.main root node")

	// Check root node
	assert.Contains(t, mainRoot.GetKey(), "generic.main")

	// Check children
	children := mainRoot.GetChildren()
	assert.GreaterOrEqual(t, len(children), 4) // Should have at least 4 children

	// Verify expected function calls exist
	foundFunctions := make(map[string]bool)
	for _, child := range children {
		key := child.GetKey()
		if strings.Contains(key, "NewContainer") {
			foundFunctions["NewContainer"] = true
		} else if strings.Contains(key, "Get") {
			foundFunctions["Get"] = true
		} else if strings.Contains(key, "Set") {
			foundFunctions["Set"] = true
		} else if strings.Contains(key, "Process") {
			foundFunctions["Process"] = true
		}
	}

	assert.True(t, foundFunctions["NewContainer"], "Should find NewContainer call")
	assert.True(t, foundFunctions["Get"], "Should find Get call")
	assert.True(t, foundFunctions["Set"], "Should find Set call")
	assert.True(t, foundFunctions["Process"], "Should find Process call")

	// Check type parameters for generic functions
	for _, child := range children {
		key := child.GetKey()
		if strings.Contains(key, "NewContainer") {
			typeParams := child.GetTypeParamMap()
			assert.Contains(t, typeParams, "T")
			assert.Equal(t, "int", typeParams["T"])
		} else if strings.Contains(key, "Process") {
			typeParams := child.GetTypeParamMap()
			assert.Contains(t, typeParams, "T")
			assert.Equal(t, "string", typeParams["T"])
		}
	}
}

// testMultiPackage tests multi-package scenario
func testMultiPackage(t *testing.T) {
	meta, err := metadata.LoadMetadata("tests/multipackage.yaml")
	require.NoError(t, err, "Failed to load metadata from tests/multipackage.yaml")

	limits := metadata.TrackerLimits{
		MaxNodesPerTree:    100,
		MaxChildrenPerNode: 10,
		MaxArgsPerFunction: 5,
		MaxNestedArgsDepth: 3,
	}

	tree := NewSimplifiedTrackerTree(meta, limits)
	require.NotNil(t, tree)

	roots := tree.GetRoots()
	require.NotNil(t, roots)
	require.GreaterOrEqual(t, len(roots), 1, "Should have at least one root node")

	// Find the multipackage.main root node
	var mainRoot TrackerNodeInterface
	for _, root := range roots {
		if strings.Contains(root.GetKey(), "multipackage.main") {
			mainRoot = root
			break
		}
	}
	require.NotNil(t, mainRoot, "Should find multipackage.main root node")

	// Check root node
	assert.Contains(t, mainRoot.GetKey(), "multipackage.main")

	// Check children
	children := mainRoot.GetChildren()
	assert.GreaterOrEqual(t, len(children), 4) // Should have at least 4 children

	// Verify expected function calls exist
	foundFunctions := make(map[string]bool)
	for _, child := range children {
		key := child.GetKey()
		if strings.Contains(key, "NewUser") && strings.Contains(key, "models") {
			foundFunctions["NewUser"] = true
		} else if strings.Contains(key, "NewUserService") && strings.Contains(key, "services") {
			foundFunctions["NewUserService"] = true
		} else if strings.Contains(key, "ProcessUser") && strings.Contains(key, "services") {
			foundFunctions["ProcessUser"] = true
		} else if strings.Contains(key, "Println") {
			foundFunctions["Println"] = true
		}
	}

	assert.True(t, foundFunctions["NewUser"], "Should find NewUser call")
	assert.True(t, foundFunctions["NewUserService"], "Should find NewUserService call")
	assert.True(t, foundFunctions["ProcessUser"], "Should find ProcessUser call")
	assert.True(t, foundFunctions["Println"], "Should find Println call")
}

// TestSimplifiedTrackerTree_GenericTypeDifferentiation tests that GenericID correctly differentiates between same function with different generic types
func TestSimplifiedTrackerTree_GenericTypeDifferentiation(t *testing.T) {
	// Create test metadata with generic functions
	meta := createGenericTestMetadata(t)

	limits := metadata.TrackerLimits{
		MaxNodesPerTree:    100,
		MaxChildrenPerNode: 10,
		MaxArgsPerFunction: 5,
		MaxNestedArgsDepth: 3,
	}

	tree := NewSimplifiedTrackerTree(meta, limits)
	require.NotNil(t, tree)

	roots := tree.GetRoots()
	require.Len(t, roots, 1)

	// Debug: Print tree structure
	t.Logf("Tree has %d roots", len(roots))
	for i, root := range roots {
		t.Logf("Root %d: key=%s", i, root.GetKey())
		children := root.GetChildren()
		t.Logf("  Root %d has %d children", i, len(children))
		for j, child := range children {
			t.Logf("    Child %d: key=%s", j, child.GetKey())
		}
	}

	// Check that we can find nodes with different generic types
	stringNode := tree.FindNodeByKey("main.main.Process[T=string]")
	require.NotNil(t, stringNode, "Should find Process[T=string] node")

	intNode := tree.FindNodeByKey("main.main.Process[T=int]")
	require.NotNil(t, intNode, "Should find Process[T=int] node")

	// Verify they are different nodes
	assert.NotEqual(t, stringNode.GetKey(), intNode.GetKey(), "Generic nodes should have different keys")
	assert.True(t, strings.Contains(stringNode.GetKey(), "[T=string]"), "String node should contain [T=string]")
	assert.True(t, strings.Contains(intNode.GetKey(), "[T=int]"), "Int node should contain [T=int]")
}

// createGenericTestMetadata creates test metadata with generic functions
func createGenericTestMetadata(t *testing.T) *metadata.Metadata {
	stringPool := metadata.NewStringPool()
	// Add all required strings to string pool first
	stringPool.Get("")
	stringPool.Get("main")
	stringPool.Get("Process")
	stringPool.Get("string")
	stringPool.Get("int")

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
		CallGraph: []metadata.CallGraphEdge{
			{
				Caller: metadata.Call{
					Name: stringPool.Get("main"),
					Pkg:  stringPool.Get("main"),
				},
				Callee: metadata.Call{
					Name: stringPool.Get("Process"),
					Pkg:  stringPool.Get("main"),
				},
				TypeParamMap: map[string]string{
					"T": "string",
				},
			},
			{
				Caller: metadata.Call{
					Name: stringPool.Get("main"),
					Pkg:  stringPool.Get("main"),
				},
				Callee: metadata.Call{
					Name: stringPool.Get("Process"),
					Pkg:  stringPool.Get("main"),
				},
				TypeParamMap: map[string]string{
					"T": "int",
				},
			},
		},
	}

	// Set Meta field for all CallGraphEdge structs
	for i := range meta.CallGraph {
		edge := &meta.CallGraph[i]
		edge.Caller.Meta = meta
		edge.Callee.Meta = meta

		// Set Edge field so Call structs can access TypeParamMap
		edge.Caller.Edge = edge
		edge.Callee.Edge = edge
	}

	meta.BuildCallGraphMaps()
	return meta
}

// createTestMetadata creates properly initialized test metadata
func createTestMetadata(t *testing.T) *metadata.Metadata {
	stringPool := metadata.NewStringPool()
	// Add all required strings to string pool first
	stringPool.Get("")
	stringPool.Get("main")
	stringPool.Get("Level1")
	stringPool.Get("nested")
	stringPool.Get("main.go:1:1")
	stringPool.Get("nested.go:1:1")
	stringPool.Get("literal")
	stringPool.Get("test")
	stringPool.Get("test")

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
		CallGraph: []metadata.CallGraphEdge{
			{
				Caller: metadata.Call{
					Name: stringPool.Get("main"),
					Pkg:  stringPool.Get("main"),
				},
				Callee: metadata.Call{
					Name: stringPool.Get("Test"),
					Pkg:  stringPool.Get("test"),
				},
				Args: []metadata.CallArgument{
					{
						Kind:  stringPool.Get("literal"),
						Value: stringPool.Get("test"),
					},
				},
			},
		},
	}

	// Set Meta field for all CallGraphEdge structs and CallArguments
	for i := range meta.CallGraph {
		edge := &meta.CallGraph[i]
		edge.Caller.Meta = meta
		edge.Callee.Meta = meta

		// Set Meta field for all CallArguments
		for j := range edge.Args {
			edge.Args[j].Meta = meta
		}
	}

	meta.BuildCallGraphMaps()
	return meta
}

// TestSimplifiedTrackerTree_InterfaceCompatibility tests interface compatibility
func TestSimplifiedTrackerTree_InterfaceCompatibility(t *testing.T) {
	// Test that SimplifiedTrackerTree implements TrackerTreeInterface
	var _ TrackerTreeInterface = (*SimplifiedTrackerTree)(nil)

	// Test that SimplifiedTrackerNode implements TrackerNodeInterface
	var _ TrackerNodeInterface = (*SimplifiedTrackerNode)(nil)
}

// TestSimplifiedTrackerTree_NodeOperations tests node operations
func TestSimplifiedTrackerTree_NodeOperations(t *testing.T) {
	// Create test metadata using a helper function that properly initializes everything
	meta := createTestMetadata(t)

	limits := metadata.TrackerLimits{
		MaxNodesPerTree:    100,
		MaxChildrenPerNode: 10,
		MaxArgsPerFunction: 5,
		MaxNestedArgsDepth: 3,
	}

	tree := NewSimplifiedTrackerTree(meta, limits)
	require.NotNil(t, tree)

	// Test FindNodeByKey
	roots := tree.GetRoots()
	require.Len(t, roots, 1)

	rootKey := roots[0].GetKey()
	foundNode := tree.FindNodeByKey(rootKey)
	require.NotNil(t, foundNode)
	assert.Equal(t, rootKey, foundNode.GetKey())

	// Test finding non-existent node
	nonExistentNode := tree.FindNodeByKey("non-existent-key")
	assert.Nil(t, nonExistentNode)

	// Test GetFunctionContext
	fn, pkg, file := tree.GetFunctionContext("main")
	require.NotNil(t, fn)
	assert.Equal(t, "main", pkg)
	assert.Equal(t, "main.go", file)

	// Test GetFunctionContext for non-existent function
	fn, pkg, file = tree.GetFunctionContext("non-existent-function")
	assert.Nil(t, fn)
	assert.Equal(t, "", pkg)
	assert.Equal(t, "", file)
}

// TestSimplifiedTrackerTree_Traversal tests tree traversal
func TestSimplifiedTrackerTree_Traversal(t *testing.T) {
	stringPool := metadata.NewStringPool()
	// Add all required strings to string pool first
	stringPool.Get("")
	stringPool.Get("main")
	stringPool.Get("Level1")
	stringPool.Get("Level2")
	stringPool.Get("nested")
	stringPool.Get("main.go:1:1")
	stringPool.Get("nested.go:1:1")

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
		CallGraph: []metadata.CallGraphEdge{
			{
				Caller: metadata.Call{
					Name:     stringPool.Get("main"),
					Pkg:      stringPool.Get("main"),
					Position: stringPool.Get("main.go:1:1"),
					RecvType: stringPool.Get(""),
				},
				Callee: metadata.Call{
					Name:     stringPool.Get("Level1"),
					Pkg:      stringPool.Get("nested"),
					Position: stringPool.Get("nested.go:1:1"),
					RecvType: stringPool.Get(""),
				},
			},
			{
				Caller: metadata.Call{
					Name:     stringPool.Get("Level1"),
					Pkg:      stringPool.Get("nested"),
					Position: stringPool.Get("nested.go:1:1"),
					RecvType: stringPool.Get(""),
				},
				Callee: metadata.Call{
					Name:     stringPool.Get("Level2"),
					Pkg:      stringPool.Get("nested"),
					Position: stringPool.Get("nested.go:1:1"),
					RecvType: stringPool.Get(""),
				},
			},
		},
	}

	// Set Meta field for all CallGraphEdge structs and CallArguments
	for i := range meta.CallGraph {
		edge := &meta.CallGraph[i]
		edge.Caller.Meta = meta
		edge.Callee.Meta = meta
	}

	meta.BuildCallGraphMaps()

	limits := metadata.TrackerLimits{
		MaxNodesPerTree:    100,
		MaxChildrenPerNode: 10,
		MaxArgsPerFunction: 5,
		MaxNestedArgsDepth: 3,
	}

	tree := NewSimplifiedTrackerTree(meta, limits)
	require.NotNil(t, tree)

	// Test full traversal
	visitedNodes := make(map[string]bool)

	tree.TraverseTree(func(node TrackerNodeInterface) bool {
		visitedNodes[node.GetKey()] = true
		return true // Continue traversal
	})

	// The enhanced tracker builds a more efficient tree structure
	// so we just verify that all nodes are visited
	expectedNodeCount := tree.GetNodeCount()
	assert.Equal(t, expectedNodeCount, len(visitedNodes),
		"All tree nodes should be visited during traversal")

	// Test early termination
	visitCount := 0
	tree.TraverseTree(func(node TrackerNodeInterface) bool {
		visitCount++
		return false // Stop traversal after first node
	})

	assert.Equal(t, 1, visitCount, "Traversal should stop after first node when visitor returns false")
}

// TestSimplifiedTrackerTree_EdgeCases tests edge cases
func TestSimplifiedTrackerTree_EdgeCases(t *testing.T) {
	// Test with nil metadata
	limits := metadata.TrackerLimits{
		MaxNodesPerTree:    100,
		MaxChildrenPerNode: 10,
		MaxArgsPerFunction: 5,
		MaxNestedArgsDepth: 3,
	}

	// This should not panic
	tree := NewSimplifiedTrackerTree(nil, limits)
	// Note: In a real implementation, this might return nil or handle nil gracefully
	// For now, we'll just ensure it doesn't panic

	// Test with very restrictive limits
	meta := &metadata.Metadata{}
	veryRestrictiveLimits := metadata.TrackerLimits{
		MaxNodesPerTree:    1,
		MaxChildrenPerNode: 1,
		MaxArgsPerFunction: 1,
		MaxNestedArgsDepth: 1,
	}

	tree = NewSimplifiedTrackerTree(meta, veryRestrictiveLimits)
	require.NotNil(t, tree)

	// Verify limits are respected
	nodeCount := tree.GetNodeCount()
	assert.LessOrEqual(t, nodeCount, veryRestrictiveLimits.MaxNodesPerTree)
}
