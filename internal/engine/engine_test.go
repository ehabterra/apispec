package engine

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultEngineConfig(t *testing.T) {
	config := DefaultEngineConfig()

	if config.InputDir != DefaultInputDir {
		t.Errorf("Expected InputDir to be %s, got %s", DefaultInputDir, config.InputDir)
	}

	if config.OutputFile != DefaultOutputFile {
		t.Errorf("Expected OutputFile to be %s, got %s", DefaultOutputFile, config.OutputFile)
	}

	if config.Title != DefaultTitle {
		t.Errorf("Expected Title to be %s, got %s", DefaultTitle, config.Title)
	}

	if config.APIVersion != DefaultAPIVersion {
		t.Errorf("Expected APIVersion to be %s, got %s", DefaultAPIVersion, config.APIVersion)
	}

	if config.OpenAPIVersion != DefaultOpenAPIVersion {
		t.Errorf("Expected OpenAPIVersion to be %s, got %s", DefaultOpenAPIVersion, config.OpenAPIVersion)
	}
}

func TestNewEngine(t *testing.T) {
	// Test with nil config
	engine := NewEngine(nil)
	if engine == nil {
		t.Fatal("Expected engine to be created")
	}

	// Test with custom config
	customConfig := &EngineConfig{
		InputDir:   "/custom/path",
		Title:      "Custom API",
		APIVersion: "2.0.0",
	}

	engine = NewEngine(customConfig)
	if engine.config.InputDir != "/custom/path" {
		t.Errorf("Expected InputDir to be /custom/path, got %s", engine.config.InputDir)
	}

	if engine.config.Title != "Custom API" {
		t.Errorf("Expected Title to be Custom API, got %s", engine.config.Title)
	}

	if engine.config.APIVersion != "2.0.0" {
		t.Errorf("Expected APIVersion to be 2.0.0, got %s", engine.config.APIVersion)
	}
}

func TestEngine_GenerateOpenAPI_InvalidDirectory(t *testing.T) {
	engine := NewEngine(&EngineConfig{
		InputDir: "/non/existent/directory",
	})

	_, err := engine.GenerateOpenAPI()
	if err == nil {
		t.Fatal("Expected error for non-existent directory")
	}

	if !contains(err.Error(), "input directory does not exist") {
		t.Errorf("Expected error to contain 'input directory does not exist', got: %s", err.Error())
	}
}

func TestEngine_GenerateOpenAPI_NoGoModule(t *testing.T) {
	// Create a temporary directory without go.mod
	tempDir, err := os.MkdirTemp("", "apispec_test_no_module")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer func() {
		if err := os.RemoveAll(tempDir); err != nil {
			t.Errorf("Failed to remove temp directory: %v", err)
		}
	}()

	engine := NewEngine(&EngineConfig{
		InputDir: tempDir,
	})

	_, err = engine.GenerateOpenAPI()
	if err == nil {
		t.Fatal("Expected error for directory without go.mod")
	}

	if !contains(err.Error(), "no go.mod found") {
		t.Errorf("Expected error to contain 'no go.mod found', got: %s", err.Error())
	}
}

func TestEngine_GenerateOpenAPI_ValidDirectory(t *testing.T) {
	// Create a temporary directory with a simple Go module
	tempDir, err := os.MkdirTemp("", "apispec_test_valid")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer func() {
		if err := os.RemoveAll(tempDir); err != nil {
			t.Errorf("Failed to remove temp directory: %v", err)
		}
	}()

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
	http.HandleFunc("/hello", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("hello"))
	})
	http.ListenAndServe(":8080", nil)
}`
	err = os.WriteFile(goFile, []byte(goContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write main.go: %v", err)
	}

	engine := NewEngine(&EngineConfig{
		InputDir: tempDir,
	})

	spec, err := engine.GenerateOpenAPI()
	if err != nil {
		t.Fatalf("Expected successful generation, got error: %v", err)
	}

	if spec == nil {
		t.Fatal("Expected non-nil OpenAPI spec")
	}

	if spec.OpenAPI != DefaultOpenAPIVersion {
		t.Errorf("Expected OpenAPI version %s, got %s", DefaultOpenAPIVersion, spec.OpenAPI)
	}

	if spec.Info.Title != DefaultTitle {
		t.Errorf("Expected title %s, got %s", DefaultTitle, spec.Info.Title)
	}
}

func TestEngine_GenerateOpenAPI_WithConfig(t *testing.T) {
	// Create a temporary directory with a Go module
	tempDir, err := os.MkdirTemp("", "apispec_test_with_config")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer func() {
		if err := os.RemoveAll(tempDir); err != nil {
			t.Errorf("Failed to remove temp directory: %v", err)
		}
	}()

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

	// Create a custom config file
	configFile := filepath.Join(tempDir, "apispec.yaml")
	configContent := `framework:
  routePatterns:
    - callRegex: "^HandleFunc$"
      pathFromArg: true
      handlerFromArg: true
      pathArgIndex: 0
      methodArgIndex: -1
      handlerArgIndex: 1
      recvTypeRegex: "^net/http(\\.\\*ServeMux)?$"
defaults:
  requestContentType: "application/json"
  responseContentType: "application/json"
  responseStatus: 200`
	err = os.WriteFile(configFile, []byte(configContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	engine := NewEngine(&EngineConfig{
		InputDir:   tempDir,
		ConfigFile: configFile,
		Title:      "Custom API",
		APIVersion: "2.0.0",
	})

	spec, err := engine.GenerateOpenAPI()
	if err != nil {
		t.Fatalf("Expected successful generation, got error: %v", err)
	}

	if spec == nil {
		t.Fatal("Expected non-nil OpenAPI spec")
	}

	if spec.Info.Title != "Custom API" {
		t.Errorf("Expected title Custom API, got %s", spec.Info.Title)
	}

	if spec.Info.Version != "2.0.0" {
		t.Errorf("Expected version 2.0.0, got %s", spec.Info.Version)
	}
}

func TestEngine_GenerateOpenAPI_WithMetadata(t *testing.T) {
	// Create a temporary directory with a Go module
	tempDir, err := os.MkdirTemp("", "apispec_test_metadata")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer func() {
		if err := os.RemoveAll(tempDir); err != nil {
			t.Errorf("Failed to remove temp directory: %v", err)
		}
	}()

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
	http.HandleFunc("/hello", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("hello"))
	})
	http.ListenAndServe(":8080", nil)
}`
	err = os.WriteFile(goFile, []byte(goContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write main.go: %v", err)
	}

	engine := NewEngine(&EngineConfig{
		InputDir:      tempDir,
		WriteMetadata: true,
	})

	spec, err := engine.GenerateOpenAPI()
	if err != nil {
		t.Fatalf("Expected successful generation, got error: %v", err)
	}

	if spec == nil {
		t.Fatal("Expected non-nil OpenAPI spec")
	}

	// Check if metadata file was created
	metadataFile := filepath.Join(tempDir, DefaultMetadataFile)
	if _, err := os.Stat(metadataFile); os.IsNotExist(err) {
		t.Error("Expected metadata file to be created")
	}
}

func TestEngine_GenerateOpenAPI_WithDiagram(t *testing.T) {
	// Create a temporary directory with a Go module
	tempDir, err := os.MkdirTemp("", "apispec_test_diagram")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer func() {
		if err := os.RemoveAll(tempDir); err != nil {
			t.Errorf("Failed to remove temp directory: %v", err)
		}
	}()

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
	http.HandleFunc("/hello", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("hello"))
	})
	http.ListenAndServe(":8080", nil)
}`
	err = os.WriteFile(goFile, []byte(goContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write main.go: %v", err)
	}

	diagramPath := filepath.Join(tempDir, "diagram.html")
	engine := NewEngine(&EngineConfig{
		InputDir:    tempDir,
		DiagramPath: diagramPath,
	})

	spec, err := engine.GenerateOpenAPI()
	if err != nil {
		t.Fatalf("Expected successful generation, got error: %v", err)
	}

	if spec == nil {
		t.Fatal("Expected non-nil OpenAPI spec")
	}

	// Check if diagram file was created
	if _, err := os.Stat(diagramPath); os.IsNotExist(err) {
		t.Error("Expected diagram file to be created")
	}
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > len(substr) && (s[:len(substr)] == substr ||
			s[len(s)-len(substr):] == substr ||
			func() bool {
				for i := 0; i <= len(s)-len(substr); i++ {
					if s[i:i+len(substr)] == substr {
						return true
					}
				}
				return false
			}())))
}
