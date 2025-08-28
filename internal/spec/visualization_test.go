package spec

import (
	"strings"
	"testing"
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
