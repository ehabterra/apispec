package generator

import (
	"os"
	"path/filepath"
	"testing"

	intspec "github.com/ehabterra/apispec/internal/spec"
	"github.com/ehabterra/apispec/spec"
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
	tempDir, err := os.MkdirTemp("", "apispec_test")
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
	tempDir, err := os.MkdirTemp("", "apispec_test_no_go")
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
	tempDir, err := os.MkdirTemp("", "apispec_test_invalid")
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

func TestGenerateFromDirectory_WithAPISpecConfig(t *testing.T) {
	// Create a temporary directory with a Go module
	tempDir, err := os.MkdirTemp("", "apispec_test_with_apispec_config")
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

	// Create a simple Go file
	goFile := filepath.Join(tempDir, "main.go")
	goContent := `package main

import "net/http"

func main() {
	http.HandleFunc("/api/users", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("users"))
	})
	http.ListenAndServe(":8080", nil)
}`
	err = os.WriteFile(goFile, []byte(goContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write main.go: %v", err)
	}

	// Create a custom APISpecConfig
	customConfig := &spec.APISpecConfig{
		Info: spec.Info{
			Title:       "Custom API from Config",
			Description: "This API was generated using a custom APISpecConfig",
			Version:     "2.0.0",
		},
		Framework: intspec.FrameworkConfig{
			RoutePatterns: []intspec.RoutePattern{
				{
					CallRegex:       "^HandleFunc$",
					PathFromArg:     true,
					HandlerFromArg:  true,
					PathArgIndex:    0,
					MethodArgIndex:  -1,
					HandlerArgIndex: 1,
					RecvTypeRegex:   "^net/http(\\.\\*ServeMux)?$",
				},
			},
		},
		Defaults: intspec.Defaults{
			RequestContentType:  "application/json",
			ResponseContentType: "application/json",
			ResponseStatus:      200,
		},
	}

	// Create generator with custom config
	gen := NewGenerator(customConfig)

	// Generate OpenAPI spec
	spec, err := gen.GenerateFromDirectory(tempDir)
	if err != nil {
		t.Fatalf("Expected successful generation, got error: %v", err)
	}

	if spec == nil {
		t.Fatal("Expected non-nil OpenAPI spec")
	}

	// Verify that the custom config was used
	if spec.Info.Title != "Custom API from Config" {
		t.Errorf("Expected title 'Custom API from Config', got %s", spec.Info.Title)
	}

	if spec.Info.Description != "This API was generated using a custom APISpecConfig" {
		t.Errorf("Expected description 'This API was generated using a custom APISpecConfig', got %s", spec.Info.Description)
	}

	if spec.Info.Version != "2.0.0" {
		t.Errorf("Expected version '2.0.0', got %s", spec.Info.Version)
	}
}
