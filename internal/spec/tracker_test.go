package spec

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ehabterra/apispec/internal/metadata"
)

// TestTrackerWarnings tests the new debug warning functionality
func TestTrackerWarnings(t *testing.T) {
	// Capture stdout to test warning messages
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	defer func() {
		os.Stdout = oldStdout
		_ = w.Close()
	}()

	// Create a mock metadata
	stringPool := metadata.NewStringPool()
	meta := &metadata.Metadata{
		StringPool: stringPool,
		Packages: map[string]*metadata.Package{
			"main": {
				Files: map[string]*metadata.File{
					"test.go": {
						Types: map[string]*metadata.Type{
							"TestType": {
								Name: stringPool.Get("TestType"),
								Kind: stringPool.Get("struct"),
							},
						},
					},
				},
			},
		},
	}

	// Test MaxArgsPerFunction warning
	t.Run("MaxArgsPerFunction warning", func(t *testing.T) {
		// Reset stdout capture
		os.Stdout = oldStdout
		r, w, _ = os.Pipe()
		os.Stdout = w

		// Create a tracker with very low limits
		limits := metadata.TrackerLimits{
			MaxArgsPerFunction: 1,
		}

		// Create a mock edge with many arguments
		edge := &metadata.CallGraphEdge{
			Caller: metadata.Call{
				Meta: meta,
				Name: stringPool.Get("TestFunction"),
				Pkg:  stringPool.Get("main"),
			},
			Callee: metadata.Call{
				Meta: meta,
				Name: stringPool.Get("AnotherFunction"),
				Pkg:  stringPool.Get("main"),
			},
			Args: []*metadata.CallArgument{
				{Meta: meta, Edge: &metadata.CallGraphEdge{
					Caller: metadata.Call{Meta: meta, Name: stringPool.Get("arg1"), Pkg: stringPool.Get("main")},
					Callee: metadata.Call{Meta: meta, Name: stringPool.Get("target1"), Pkg: stringPool.Get("main")},
				}, Name: stringPool.Get("arg1"), Kind: 1, Type: stringPool.Get("string"), Position: stringPool.Get("pos1")}, // KindIdent
				{Meta: meta, Edge: &metadata.CallGraphEdge{
					Caller: metadata.Call{Meta: meta, Name: stringPool.Get("arg2"), Pkg: stringPool.Get("main")},
					Callee: metadata.Call{Meta: meta, Name: stringPool.Get("target2"), Pkg: stringPool.Get("main")},
				}, Name: stringPool.Get("arg2"), Kind: 1, Type: stringPool.Get("int"), Position: stringPool.Get("pos2")}, // KindIdent
				{Meta: meta, Edge: &metadata.CallGraphEdge{
					Caller: metadata.Call{Meta: meta, Name: stringPool.Get("arg3"), Pkg: stringPool.Get("main")},
					Callee: metadata.Call{Meta: meta, Name: stringPool.Get("target3"), Pkg: stringPool.Get("main")},
				}, Name: stringPool.Get("arg3"), Kind: 1, Type: stringPool.Get("bool"), Position: stringPool.Get("pos3")}, // KindIdent
			},
		}

		// This should trigger the warning
		processArguments(nil, meta, nil, edge, make(map[string]int), nil, limits)

		// Read the output
		_ = w.Close()
		var buf bytes.Buffer
		_, _ = buf.ReadFrom(r)
		output := buf.String()

		// Check if warning was printed
		expectedWarning := fmt.Sprintf("Warning: MaxArgsPerFunction limit (%d) reached", limits.MaxArgsPerFunction)
		if !strings.Contains(output, expectedWarning) {
			t.Errorf("Expected warning message containing '%s', got: %s", expectedWarning, output)
		}
	})

	// Test MaxNodesPerTree warning
	t.Run("MaxNodesPerTree warning", func(t *testing.T) {
		// Reset stdout capture
		os.Stdout = oldStdout
		r, w, _ = os.Pipe()
		os.Stdout = w

		// Create a tracker with very low limits
		limits := metadata.TrackerLimits{
			MaxNodesPerTree:   1,
			MaxRecursionDepth: 10, // Set higher to avoid hitting this first
		}

		// Create a visited map that exceeds the limit
		visited := make(map[string]int)
		for i := 0; i < 5; i++ {
			visited[fmt.Sprintf("node%d", i)] = 1
		}

		// This should trigger the warning
		NewTrackerNode(nil, meta, "parent", "test", nil, nil, visited, nil, limits)

		// Read the output
		_ = w.Close()
		var buf bytes.Buffer
		_, _ = buf.ReadFrom(r)
		output := buf.String()

		// Check if warning was printed
		expectedWarning := fmt.Sprintf("Warning: MaxNodesPerTree limit (%d) reached", limits.MaxNodesPerTree)
		if !strings.Contains(output, expectedWarning) {
			t.Errorf("Expected warning message containing '%s', got: %s", expectedWarning, output)
		}
	})

	// Test MaxChildrenPerNode warning
	t.Run("MaxChildrenPerNode warning", func(t *testing.T) {
		// Reset stdout capture
		os.Stdout = oldStdout
		r, w, _ = os.Pipe()
		os.Stdout = w

		// Create a tracker with very low limits
		limits := metadata.TrackerLimits{
			MaxChildrenPerNode: 1,
			MaxRecursionDepth:  10,
		}

		// Create a mock tree to test the warning
		tree := &TrackerTree{
			limits:  limits,
			nodeMap: make(map[string]*TrackerNode),
		}

		// Create multiple edges in metadata to exceed the limit
		parentID := "main.parent"
		meta.Callers = make(map[string][]*metadata.CallGraphEdge)
		meta.Callers[parentID] = []*metadata.CallGraphEdge{
			{
				Caller: metadata.Call{Meta: meta, Name: stringPool.Get("parent"), Pkg: stringPool.Get("main")},
				Callee: metadata.Call{Meta: meta, Name: stringPool.Get("child1"), Pkg: stringPool.Get("main")},
			},
			{
				Caller: metadata.Call{Meta: meta, Name: stringPool.Get("parent"), Pkg: stringPool.Get("main")},
				Callee: metadata.Call{Meta: meta, Name: stringPool.Get("child2"), Pkg: stringPool.Get("main")},
			},
			{
				Caller: metadata.Call{Meta: meta, Name: stringPool.Get("parent"), Pkg: stringPool.Get("main")},
				Callee: metadata.Call{Meta: meta, Name: stringPool.Get("child3"), Pkg: stringPool.Get("main")},
			},
		}

		// This should trigger the warning when processing many children
		NewTrackerNode(tree, meta, "parent", parentID, nil, nil, make(map[string]int), nil, limits)

		// Read the output
		_ = w.Close()
		var buf bytes.Buffer
		_, _ = buf.ReadFrom(r)
		output := buf.String()

		// Check if warning was printed
		expectedWarning := fmt.Sprintf("Warning: MaxChildrenPerNode limit (%d) reached", limits.MaxChildrenPerNode)
		if !strings.Contains(output, expectedWarning) {
			t.Errorf("Expected warning message containing '%s', got: %s", expectedWarning, output)
		}
	})
}

// TestTrackerWithoutWarnings tests that no warnings are printed when limits are not exceeded
func TestTrackerWithoutWarnings(t *testing.T) {
	// Capture stdout to test that no warnings are printed
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	defer func() {
		os.Stdout = oldStdout
		_ = w.Close()
	}()

	// Create a mock metadata
	stringPool := metadata.NewStringPool()
	meta := &metadata.Metadata{
		StringPool: stringPool,
		Packages: map[string]*metadata.Package{
			"main": {
				Files: map[string]*metadata.File{
					"test.go": {
						Types: map[string]*metadata.Type{
							"TestType": {
								Name: stringPool.Get("TestType"),
								Kind: stringPool.Get("struct"),
							},
						},
					},
				},
			},
		},
	}

	// Test with high limits (should not trigger warnings)
	limits := metadata.TrackerLimits{
		MaxArgsPerFunction: 100,
		MaxNodesPerTree:    1000,
		MaxChildrenPerNode: 100,
		MaxRecursionDepth:  10,
	}

	// Create a mock edge with few arguments
	edge := &metadata.CallGraphEdge{
		Caller: metadata.Call{
			Meta: meta,
			Name: stringPool.Get("TestFunction"),
			Pkg:  stringPool.Get("main"),
		},
		Callee: metadata.Call{
			Meta: meta,
			Name: stringPool.Get("AnotherFunction"),
			Pkg:  stringPool.Get("main"),
		},
		Args: []*metadata.CallArgument{
			{Meta: meta, Edge: &metadata.CallGraphEdge{}},
		},
	}

	// This should not trigger any warnings
	processArguments(nil, meta, nil, edge, make(map[string]int), nil, limits)

	// Create a visited map that doesn't exceed the limit
	visited := make(map[string]int)
	visited["node1"] = 1

	// Create a mock tree
	tree := &TrackerTree{
		limits:  limits,
		nodeMap: make(map[string]*TrackerNode),
	}

	// This should not trigger any warnings
	NewTrackerNode(tree, meta, "parent", "test", nil, nil, visited, nil, limits)

	// This should not trigger any warnings
	NewTrackerNode(tree, meta, "parent", "test", nil, nil, visited, nil, limits)

	// Read the output
	_ = w.Close()
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	output := buf.String()

	// Check that no warnings were printed
	if strings.Contains(output, "Warning:") {
		t.Errorf("Expected no warning messages, got: %s", output)
	}
}

// TestTrackerLimitsIntegration tests the integration of all limit checks
func TestTrackerLimitsIntegration(t *testing.T) {
	// Capture stdout to test warning messages
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	defer func() {
		os.Stdout = oldStdout
		_ = w.Close()
	}()

	// Create a mock metadata
	stringPool := metadata.NewStringPool()
	meta := &metadata.Metadata{
		StringPool: stringPool,
		Packages: map[string]*metadata.Package{
			"main": {
				Files: map[string]*metadata.File{
					"test.go": {
						Types: map[string]*metadata.Type{
							"TestType": {
								Name: stringPool.Get("TestType"),
								Kind: stringPool.Get("struct"),
							},
						},
					},
				},
			},
		},
	}

	// Test with very restrictive limits
	limits := metadata.TrackerLimits{
		MaxArgsPerFunction: 1,
		MaxNodesPerTree:    2,
		MaxChildrenPerNode: 1,
		MaxRecursionDepth:  10,
	}

	// Test all three warning conditions
	edge := &metadata.CallGraphEdge{
		Caller: metadata.Call{
			Meta: meta,
			Name: stringPool.Get("TestFunction"),
			Pkg:  stringPool.Get("main"),
		},
		Callee: metadata.Call{
			Meta: meta,
			Name: stringPool.Get("AnotherFunction"),
			Pkg:  stringPool.Get("main"),
		},
		Args: []*metadata.CallArgument{
			{Meta: meta, Edge: &metadata.CallGraphEdge{}},
			{Meta: meta, Edge: &metadata.CallGraphEdge{}},
		},
	}

	// Trigger MaxArgsPerFunction warning
	processArguments(nil, meta, nil, edge, make(map[string]int), nil, limits)

	// Trigger MaxNodesPerTree warning
	visited := make(map[string]int)
	for i := 0; i < 5; i++ {
		visited[fmt.Sprintf("node%d", i)] = 1
	}
	NewTrackerNode(nil, meta, "parent", "test", nil, nil, visited, nil, limits)

	// Trigger MaxChildrenPerNode warning
	tree := &TrackerTree{
		limits:  limits,
		nodeMap: make(map[string]*TrackerNode),
	}
	// Set up metadata to have multiple children for the same caller
	meta.Callers = make(map[string][]*metadata.CallGraphEdge)
	meta.Callers["test"] = []*metadata.CallGraphEdge{
		{
			Caller: metadata.Call{Meta: meta, Name: stringPool.Get("test"), Pkg: stringPool.Get("main")},
			Callee: metadata.Call{Meta: meta, Name: stringPool.Get("child1"), Pkg: stringPool.Get("main")},
		},
		{
			Caller: metadata.Call{Meta: meta, Name: stringPool.Get("test"), Pkg: stringPool.Get("main")},
			Callee: metadata.Call{Meta: meta, Name: stringPool.Get("child2"), Pkg: stringPool.Get("main")},
		},
	}
	// Use a fresh visited map to avoid MaxNodesPerTree limit
	freshVisited := make(map[string]int)
	NewTrackerNode(tree, meta, "parent", "test", nil, nil, freshVisited, nil, limits)

	// Read the output
	_ = w.Close()
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	output := buf.String()

	// Check that all three warnings were printed
	expectedWarnings := []string{
		"MaxArgsPerFunction limit",
		"MaxNodesPerTree limit",
		"MaxChildrenPerNode limit",
	}

	for _, expectedWarning := range expectedWarnings {
		if !strings.Contains(output, expectedWarning) {
			t.Errorf("Expected warning message containing '%s', got: %s", expectedWarning, output)
		}
	}
}

func TestFindNodeInSubtree_CycleDetection(t *testing.T) {
	// Create a tree with circular references to test cycle detection
	meta := &metadata.Metadata{}
	limits := metadata.TrackerLimits{
		MaxNodesPerTree:    1000,
		MaxChildrenPerNode: 100,
		MaxArgsPerFunction: 100,
		MaxNestedArgsDepth: 10,
	}
	tree := NewTrackerTree(meta, limits)

	// Create simple nodes for testing
	node1 := &TrackerNode{
		Children: make([]*TrackerNode, 0),
	}

	node2 := &TrackerNode{
		Children: make([]*TrackerNode, 0),
	}

	node3 := &TrackerNode{
		Children: make([]*TrackerNode, 0),
	}

	// Create a cycle: node1 -> node2 -> node3 -> node1
	node1.Children = append(node1.Children, node2)
	node2.Children = append(node2.Children, node3)
	node3.Children = append(node3.Children, node1)

	// Test that cycle detection prevents infinite recursion
	start := time.Now()
	result := tree.findNodeInSubtree(node1, "test-edge-id")
	duration := time.Since(start)

	// Should complete quickly (cycle detection should work)
	if duration > 100*time.Millisecond {
		t.Errorf("findNodeInSubtree took too long (%v), cycle detection may not be working", duration)
	}

	// Should return nil since we're looking for a non-existent edge ID
	if result != nil {
		t.Error("Expected to not find non-existent edge, but got result")
	}
}

func TestFindNodeInSubtreeWithVisited_CycleDetection(t *testing.T) {
	// Create a tree with circular references
	meta := &metadata.Metadata{}
	limits := metadata.TrackerLimits{
		MaxNodesPerTree:    1000,
		MaxChildrenPerNode: 100,
		MaxArgsPerFunction: 100,
		MaxNestedArgsDepth: 10,
	}
	tree := NewTrackerTree(meta, limits)

	node1 := &TrackerNode{
		Children: make([]*TrackerNode, 0),
	}

	node2 := &TrackerNode{
		Children: make([]*TrackerNode, 0),
	}

	node3 := &TrackerNode{
		Children: make([]*TrackerNode, 0),
	}

	// Create a cycle: node1 -> node2 -> node3 -> node1
	node1.Children = append(node1.Children, node2)
	node2.Children = append(node2.Children, node3)
	node3.Children = append(node3.Children, node1)

	// Test with visited map
	visited := make(map[*TrackerNode]bool)
	start := time.Now()
	result := tree.findNodeInSubtreeWithVisited(node1, "test-edge-id", visited)
	duration := time.Since(start)

	// Should complete quickly (cycle detection should work)
	if duration > 100*time.Millisecond {
		t.Errorf("findNodeInSubtreeWithVisited took too long (%v), cycle detection may not be working", duration)
	}

	// Should return nil since we're looking for a non-existent edge ID
	if result != nil {
		t.Error("Expected to not find non-existent edge, but got result")
	}

	// Check that visited map was populated
	if len(visited) == 0 {
		t.Error("Expected visited map to be populated")
	}
}

func TestFindNodeInSubtree_NoCycle(t *testing.T) {
	// Test normal tree without cycles
	meta := &metadata.Metadata{}
	limits := metadata.TrackerLimits{
		MaxNodesPerTree:    1000,
		MaxChildrenPerNode: 100,
		MaxArgsPerFunction: 100,
		MaxNestedArgsDepth: 10,
	}
	tree := NewTrackerTree(meta, limits)

	node1 := &TrackerNode{
		Children: make([]*TrackerNode, 0),
	}

	node2 := &TrackerNode{
		Children: make([]*TrackerNode, 0),
	}

	node3 := &TrackerNode{
		Children: make([]*TrackerNode, 0),
	}

	// Create a normal tree: node1 -> node2 -> node3
	node1.Children = append(node1.Children, node2)
	node2.Children = append(node2.Children, node3)

	// Test finding non-existing node (should return nil)
	result := tree.findNodeInSubtree(node1, "nonexistent")
	if result != nil {
		t.Error("Expected to not find nonexistent node, but got result")
	}
}

func TestFindNodeInSubtree_Performance(t *testing.T) {
	// Test performance with deep tree
	meta := &metadata.Metadata{}
	limits := metadata.TrackerLimits{
		MaxNodesPerTree:    1000,
		MaxChildrenPerNode: 100,
		MaxArgsPerFunction: 100,
		MaxNestedArgsDepth: 10,
	}
	tree := NewTrackerTree(meta, limits)

	// Create a deep tree (100 levels)
	root := &TrackerNode{
		Children: make([]*TrackerNode, 0),
	}

	current := root
	for i := 0; i < 100; i++ {
		child := &TrackerNode{
			Children: make([]*TrackerNode, 0),
		}
		current.Children = append(current.Children, child)
		current = child
	}

	// Test finding a non-existent node (should complete quickly)
	start := time.Now()
	result := tree.findNodeInSubtree(root, "nonexistent")
	duration := time.Since(start)

	// Should complete quickly
	if duration > 1*time.Second {
		t.Errorf("findNodeInSubtree took too long (%v) for deep tree", duration)
	}

	// Should return nil for non-existent node
	if result != nil {
		t.Error("Expected to not find nonexistent node, but got result")
	}
}
