package spec

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ehabterra/apispec/internal/metadata"
)

func TestGenerateCytoscapeHTML(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "cytoscape_test")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a simple tracker tree for testing
	meta := &metadata.Metadata{}
	limits := metadata.TrackerLimits{
		MaxNodesPerTree:    100,
		MaxChildrenPerNode: 10,
		MaxArgsPerFunction: 5,
		MaxNestedArgsDepth: 3,
	}
	tree := NewTrackerTree(meta, limits)

	// Test HTML generation
	outputPath := filepath.Join(tempDir, "test_diagram.html")
	err = GenerateCytoscapeHTML(tree.GetRoots(), outputPath)
	if err != nil {
		t.Fatalf("Failed to generate Cytoscape HTML: %v", err)
	}

	// Check if file was created
	if _, err := os.Stat(outputPath); os.IsNotExist(err) {
		t.Fatal("Expected HTML file to be created")
	}

	// Check file size (should be reasonable)
	fileInfo, err := os.Stat(outputPath)
	if err != nil {
		t.Fatalf("Failed to get file info: %v", err)
	}

	if fileInfo.Size() < 1000 { // Should be at least 1KB
		t.Errorf("Generated HTML file seems too small: %d bytes", fileInfo.Size())
	}
}

func TestExportCytoscapeJSON(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "cytoscape_json_test")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a simple tracker tree for testing
	meta := &metadata.Metadata{}
	limits := metadata.TrackerLimits{
		MaxNodesPerTree:    100,
		MaxChildrenPerNode: 10,
		MaxArgsPerFunction: 5,
		MaxNestedArgsDepth: 3,
	}
	tree := NewMockTrackerTree(meta, limits)

	// Test JSON export
	outputPath := filepath.Join(tempDir, "test_diagram.json")
	err = ExportCytoscapeJSON(tree.GetRoots(), outputPath)
	if err != nil {
		t.Fatalf("Failed to export Cytoscape JSON: %v", err)
	}

	// Check if file was created
	if _, err := os.Stat(outputPath); os.IsNotExist(err) {
		t.Fatal("Expected JSON file to be created")
	}

	// Check file size (should be reasonable)
	fileInfo, err := os.Stat(outputPath)
	if err != nil {
		t.Fatalf("Failed to get file info: %v", err)
	}

	if fileInfo.Size() < 30 { // Should be at least 30 bytes (empty tree JSON)
		t.Errorf("Generated JSON file seems too small: %d bytes", fileInfo.Size())
	}
}

func TestExportWithEmptyTree(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "cytoscape_empty_test")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create an empty tracker tree
	meta := &metadata.Metadata{}
	limits := metadata.TrackerLimits{
		MaxNodesPerTree:    100,
		MaxChildrenPerNode: 10,
		MaxArgsPerFunction: 5,
		MaxNestedArgsDepth: 3,
	}
	tree := NewMockTrackerTree(meta, limits)

	// Test HTML generation with empty tree
	htmlPath := filepath.Join(tempDir, "empty_diagram.html")
	err = GenerateCytoscapeHTML(tree.GetRoots(), htmlPath)
	if err != nil {
		t.Fatalf("Failed to generate HTML with empty tree: %v", err)
	}

	// Test JSON export with empty tree
	jsonPath := filepath.Join(tempDir, "empty_diagram.json")
	err = ExportCytoscapeJSON(tree.GetRoots(), jsonPath)
	if err != nil {
		t.Fatalf("Failed to export JSON with empty tree: %v", err)
	}

	// Check if files were created
	if _, err := os.Stat(htmlPath); os.IsNotExist(err) {
		t.Fatal("Expected HTML file to be created for empty tree")
	}
	if _, err := os.Stat(jsonPath); os.IsNotExist(err) {
		t.Fatal("Expected JSON file to be created for empty tree")
	}

	// Check file sizes
	htmlInfo, err := os.Stat(htmlPath)
	if err != nil {
		t.Fatalf("Failed to get HTML file info: %v", err)
	}
	jsonInfo, err := os.Stat(jsonPath)
	if err != nil {
		t.Fatalf("Failed to get JSON file info: %v", err)
	}

	if htmlInfo.Size() < 1000 {
		t.Errorf("Generated HTML file seems too small: %d bytes", htmlInfo.Size())
	}
	if jsonInfo.Size() < 30 {
		t.Errorf("Generated JSON file seems too small: %d bytes", jsonInfo.Size())
	}
}

func TestExportWithInvalidPath(t *testing.T) {
	// Create a simple tracker tree for testing
	meta := &metadata.Metadata{}
	limits := metadata.TrackerLimits{
		MaxNodesPerTree:    100,
		MaxChildrenPerNode: 10,
		MaxArgsPerFunction: 5,
		MaxNestedArgsDepth: 3,
	}
	tree := NewMockTrackerTree(meta, limits)

	// Test with invalid path (parent directory doesn't exist)
	invalidPath := "/non/existent/path/diagram.html"
	err := GenerateCytoscapeHTML(tree.GetRoots(), invalidPath)
	if err == nil {
		t.Error("Expected error when writing to invalid path")
	}

	// Test JSON export with invalid path
	err = ExportCytoscapeJSON(tree.GetRoots(), invalidPath)
	if err == nil {
		t.Error("Expected error when writing JSON to invalid path")
	}
}
