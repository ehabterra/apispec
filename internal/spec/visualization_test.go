package spec

import (
	"strings"
	"testing"

	"github.com/ehabterra/apispec/internal/metadata"
)

func TestDrawTrackerTree(t *testing.T) {
	// Test with nil nodes
	result := DrawTrackerTree(nil)
	// Should return at least the header
	if result == "" {
		t.Error("Expected non-empty result even for nil nodes")
	}

	// Test with empty nodes slice
	emptyNodes := []TrackerNodeInterface{}
	result = DrawTrackerTree(emptyNodes)
	if result == "" {
		t.Error("Expected non-empty result for empty nodes slice")
	}

	// Should contain the Mermaid header
	if !strings.Contains(result, "graph LR") {
		t.Error("Expected result to contain Mermaid header")
	}
}

func TestDrawTrackerTreeCytoscape(t *testing.T) {
	// Test with nil nodes
	result := DrawTrackerTreeCytoscape(nil)
	if result == nil {
		t.Error("Expected non-nil result even for nil nodes")
		return
	}

	// Test with empty nodes slice
	emptyNodes := []TrackerNodeInterface{}
	result = DrawTrackerTreeCytoscape(emptyNodes)
	if result == nil {
		t.Error("Expected non-nil result for empty nodes slice")
		return
	}

	// Check that the result has the expected structure
	if result.Nodes == nil {
		t.Error("Expected non-nil Nodes slice")
	}
	if result.Edges == nil {
		t.Error("Expected non-nil Edges slice")
	}

	// For empty nodes slice, we expect empty slices (not nil)
	if len(result.Nodes) != 0 {
		t.Errorf("Expected empty Nodes slice for empty nodes input, got %d nodes", len(result.Nodes))
	}
	if len(result.Edges) != 0 {
		t.Errorf("Expected empty Edges slice for empty nodes input, got %d edges", len(result.Edges))
	}
}

func TestDrawCallGraphCytoscape(t *testing.T) {
	// Test with nil metadata
	result := DrawCallGraphCytoscape(nil)
	if result == nil {
		t.Error("Expected non-nil result even for nil metadata")
		return
	}

	// Test with empty metadata
	emptyMeta := &metadata.Metadata{}
	result = DrawCallGraphCytoscape(emptyMeta)
	if result == nil {
		t.Error("Expected non-nil result for empty metadata")
		return
	}

	// Check that the result has the expected structure
	if result.Nodes == nil {
		t.Error("Expected non-nil Nodes slice")
	}
	if result.Edges == nil {
		t.Error("Expected non-nil Edges slice")
	}

	// For empty metadata, we expect empty slices (not nil)
	if len(result.Nodes) != 0 {
		t.Errorf("Expected empty Nodes slice for empty metadata, got %d nodes", len(result.Nodes))
	}
	if len(result.Edges) != 0 {
		t.Errorf("Expected empty Edges slice for empty metadata, got %d edges", len(result.Edges))
	}
}

func TestBuildCallPaths(t *testing.T) {
	// Create a simple metadata with call graph
	meta := &metadata.Metadata{
		StringPool: metadata.NewStringPool(),
		CallGraph: []metadata.CallGraphEdge{
			{
				Caller: metadata.Call{
					Name: 0, // "main"
					Pkg:  1, // "main"
				},
				Callee: metadata.Call{
					Name: 2, // "foo"
					Pkg:  1, // "main"
				},
			},
		},
	}

	// Add strings to string pool
	meta.StringPool.Get("main")
	meta.StringPool.Get("main")
	meta.StringPool.Get("foo")

	// Build call graph maps
	meta.BuildCallGraphMaps()

	// Test buildCallPaths
	paths := buildCallPaths(meta, "main.foo")

	// Should have one caller path
	if len(paths) != 1 {
		t.Errorf("Expected 1 call path, got %d", len(paths))
	}

	if paths[0] != "main.main" {
		t.Errorf("Expected call path 'main.main', got '%s'", paths[0])
	}
}

func TestExtractParameterInfo(t *testing.T) {
	// Create a simple metadata
	meta := &metadata.Metadata{
		StringPool: metadata.NewStringPool(),
	}

	// Create a call graph edge with parameter information
	edge := &metadata.CallGraphEdge{
		ParamArgMap: map[string]metadata.CallArgument{
			"param1": {
				Name: 0, // "value1"
				Type: 1, // "string"
				Meta: meta,
			},
		},
		Args: []metadata.CallArgument{
			{
				Name: 2, // "arg1"
				Meta: meta,
			},
		},
	}

	// Add strings to string pool
	meta.StringPool.Get("value1")
	meta.StringPool.Get("string")
	meta.StringPool.Get("arg1")

	paramTypes, passedParams := extractParameterInfo(edge)

	// Check parameter types
	if len(paramTypes) != 1 {
		t.Errorf("Expected 1 parameter type, got %d", len(paramTypes))
	}

	if paramTypes[0] != "param1:string" {
		t.Errorf("Expected parameter type 'param1:string', got '%s'", paramTypes[0])
	}

	// Check passed parameters
	if len(passedParams) != 2 {
		t.Errorf("Expected 2 passed parameters, got %d", len(passedParams))
	}

	if passedParams[0] != "value1" {
		t.Errorf("Expected passed parameter 'value1', got '%s'", passedParams[0])
	}

	if passedParams[1] != "arg1" {
		t.Errorf("Expected passed parameter 'arg1', got '%s'", passedParams[1])
	}
}
