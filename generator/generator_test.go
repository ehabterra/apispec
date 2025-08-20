package generator

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ehabterra/swagen/spec"
)

func TestNewGenerator(t *testing.T) {
	config := spec.DefaultGinConfig()
	gen := NewGenerator(config)

	if gen == nil {
		t.Fatal("NewGenerator returned nil")
	}

	if gen.config == nil {
		t.Error("Generator config is nil")
	}
}

func TestGenerateFromDirectory_ValidDirectory(t *testing.T) {
	// Create a temporary test directory with Go files
	tempDir, err := os.MkdirTemp("", "swagen_test")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a simple Go file without external dependencies
	goFile := filepath.Join(tempDir, "main.go")
	goContent := `package main

import "net/http"

func main() {
	http.HandleFunc("/hello", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("hello"))
	})
	http.ListenAndServe(":8080", nil)
}`

	err = os.WriteFile(goFile, []byte(goContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	// Create go.mod file
	goModFile := filepath.Join(tempDir, "go.mod")
	goModContent := `module testapp

go 1.21`

	err = os.WriteFile(goModFile, []byte(goModContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write go.mod: %v", err)
	}

	config := spec.DefaultHTTPConfig()
	gen := NewGenerator(config)

	spec, err := gen.GenerateFromDirectory(tempDir)
	if err != nil {
		t.Fatalf("GenerateFromDirectory failed: %v", err)
	}

	if spec == nil {
		t.Fatal("Generated spec is nil")
	}

	// Basic validation of the generated spec
	if spec.OpenAPI == "" {
		t.Error("OpenAPI version is empty")
	}

	// Check if Paths section exists (this should always be present)
	if spec.Paths == nil {
		t.Error("Paths section is nil")
	}

	// Log some basic info about what was generated
	t.Logf("Generated OpenAPI spec with version: %s", spec.OpenAPI)
	t.Logf("Paths count: %d", len(spec.Paths))
}

func TestGenerateFromDirectory_InvalidDirectory(t *testing.T) {
	config := spec.DefaultGinConfig()
	gen := NewGenerator(config)

	// Test with non-existent directory
	_, err := gen.GenerateFromDirectory("/non/existent/directory")
	if err == nil {
		t.Error("Expected error for non-existent directory")
	}
}

func TestGenerateFromDirectory_NoGoFiles(t *testing.T) {
	// Create a temporary test directory without Go files
	tempDir, err := os.MkdirTemp("", "swagen_test_no_go")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a non-Go file
	textFile := filepath.Join(tempDir, "readme.txt")
	err = os.WriteFile(textFile, []byte("This is not a Go file"), 0644)
	if err != nil {
		t.Fatalf("Failed to write text file: %v", err)
	}

	config := spec.DefaultGinConfig()
	gen := NewGenerator(config)

	_, err = gen.GenerateFromDirectory(tempDir)
	if err == nil {
		t.Error("Expected error for directory without Go files")
	}
}

func TestGenerateFromDirectory_InvalidGoCode(t *testing.T) {
	// Create a temporary test directory with invalid Go code
	tempDir, err := os.MkdirTemp("", "swagen_test_invalid")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a Go file with syntax errors
	goFile := filepath.Join(tempDir, "main.go")
	goContent := `package main

import "github.com/gin-gonic/gin"

func main() {
	r := gin.Default()
	r.GET("/hello", func(c *gin.Context) {
		c.JSON(200, gin.H{"message": "hello"})
	// Missing closing brace
	r.Run(":8080")
}`

	err = os.WriteFile(goFile, []byte(goContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	config := spec.DefaultGinConfig()
	gen := NewGenerator(config)

	_, err = gen.GenerateFromDirectory(tempDir)
	if err == nil {
		t.Error("Expected error for invalid Go code")
	}
}

func TestFindModuleRoot(t *testing.T) {
	// Test with a directory that has go.mod
	tempDir, err := os.MkdirTemp("", "swagen_test_module")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create go.mod file
	goModFile := filepath.Join(tempDir, "go.mod")
	goModContent := `module testapp

go 1.21`

	err = os.WriteFile(goModFile, []byte(goModContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write go.mod: %v", err)
	}

	moduleRoot, err := findModuleRoot(tempDir)
	if err != nil {
		t.Fatalf("findModuleRoot failed: %v", err)
	}
	if moduleRoot != tempDir {
		t.Errorf("Expected module root %s, got %s", tempDir, moduleRoot)
	}

	// Test with a subdirectory
	subDir := filepath.Join(tempDir, "subdir")
	err = os.Mkdir(subDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create subdirectory: %v", err)
	}

	moduleRoot, err = findModuleRoot(subDir)
	if err != nil {
		t.Fatalf("findModuleRoot failed: %v", err)
	}
	if moduleRoot != tempDir {
		t.Errorf("Expected module root %s, got %s", tempDir, moduleRoot)
	}
}

func TestFindModuleRoot_NoGoMod(t *testing.T) {
	// Test with a directory that doesn't have go.mod
	tempDir, err := os.MkdirTemp("", "swagen_test_no_module")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	moduleRoot, err := findModuleRoot(tempDir)
	if err == nil {
		t.Error("Expected error for directory without go.mod")
	}
	if moduleRoot != "" {
		t.Errorf("Expected empty module root, got %s", moduleRoot)
	}
}
