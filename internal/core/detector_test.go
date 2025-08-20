package core

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewFrameworkDetector(t *testing.T) {
	detector := NewFrameworkDetector()
	if detector == nil {
		t.Error("NewFrameworkDetector returned nil")
	}
}

func TestDetect_NoGoFiles(t *testing.T) {
	// Create a temporary directory without Go files
	tempDir, err := os.MkdirTemp("", "swagen_test_no_go")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	detector := NewFrameworkDetector()
	framework, err := detector.Detect(tempDir)
	if err != nil {
		t.Fatalf("Detect failed: %v", err)
	}

	// Should return "net/http" as default when no Go files are found
	if framework != "net/http" {
		t.Errorf("Expected net/http framework, got %s", framework)
	}
}

func TestDetect_WithGoFiles(t *testing.T) {
	// Create a temporary directory with Go files
	tempDir, err := os.MkdirTemp("", "swagen_test_with_go")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a Go file
	goFile := filepath.Join(tempDir, "main.go")
	goContent := `package main

import "net/http"

func main() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Hello"))
	})
}`

	err = os.WriteFile(goFile, []byte(goContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	detector := NewFrameworkDetector()
	framework, err := detector.Detect(tempDir)
	if err != nil {
		t.Fatalf("Detect failed: %v", err)
	}

	// Should detect net/http framework
	if framework != "net/http" {
		t.Errorf("Expected net/http framework, got %s", framework)
	}
}

func TestCollectGoFiles(t *testing.T) {
	// Create a temporary directory with mixed file types
	tempDir, err := os.MkdirTemp("", "swagen_test_collect")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create Go files
	goFiles := []string{"main.go", "handler.go", "utils.go"}
	for _, filename := range goFiles {
		goFile := filepath.Join(tempDir, filename)
		goContent := `package main

func main() {}`

		err = os.WriteFile(goFile, []byte(goContent), 0644)
		if err != nil {
			t.Fatalf("Failed to write test file %s: %v", filename, err)
		}
	}

	// Create non-Go files
	nonGoFiles := []string{"readme.txt", "config.yaml", "data.json"}
	for _, filename := range nonGoFiles {
		nonGoFile := filepath.Join(tempDir, filename)
		err = os.WriteFile(nonGoFile, []byte("content"), 0644)
		if err != nil {
			t.Fatalf("Failed to write test file %s: %v", filename, err)
		}
	}

	goFilesFound, err := CollectGoFiles(tempDir)
	if err != nil {
		t.Fatalf("CollectGoFiles failed: %v", err)
	}

	// Should find exactly 3 Go files
	if len(goFilesFound) != 3 {
		t.Errorf("Expected 3 Go files, found %d", len(goFilesFound))
	}

	// Check that all expected Go files are found
	for _, expectedFile := range goFiles {
		found := false
		for _, foundFile := range goFilesFound {
			if filepath.Base(foundFile) == expectedFile {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected Go file %s not found", expectedFile)
		}
	}
}

func TestDetect_InvalidDirectory(t *testing.T) {
	detector := NewFrameworkDetector()

	// Test with non-existent directory
	_, err := detector.Detect("/non/existent/directory")
	if err == nil {
		t.Error("Expected error for non-existent directory")
	}
}
