package spec

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"testing"

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
			Args: []metadata.CallArgument{
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
			MaxNodesPerTree: 1,
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
		}

		// Create a mock tree to test the warning
		tree := &TrackerTree{
			limits: limits,
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
		Args: []metadata.CallArgument{
			{Meta: meta, Edge: &metadata.CallGraphEdge{}},
		},
	}

	// This should not trigger any warnings
	processArguments(nil, meta, nil, edge, make(map[string]int), nil, limits)

	// Create a visited map that doesn't exceed the limit
	visited := make(map[string]int)
	visited["node1"] = 1

	// This should not trigger any warnings
	NewTrackerNode(nil, meta, "parent", "test", nil, nil, visited, nil, limits)

	// This should not trigger any warnings
	NewTrackerNode(nil, meta, "parent", "test", nil, nil, visited, nil, limits)

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
		Args: []metadata.CallArgument{
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
		limits: limits,
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
