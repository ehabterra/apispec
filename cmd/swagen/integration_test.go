package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestMainCLI_Help(t *testing.T) {
	// Test help command
	cmd := exec.Command("go", "run", ".", "--help")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Help command failed: %v\nOutput: %s", err, output)
	}

	outputStr := string(output)
	if !strings.Contains(outputStr, "Usage:") {
		t.Error("Help output should contain 'Usage:'")
	}
	if !strings.Contains(outputStr, "swagen") {
		t.Error("Help output should contain 'swagen'")
	}
}

func TestMainCLI_Version(t *testing.T) {
	// Test version command
	cmd := exec.Command("go", "run", ".", "--version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Version command failed: %v\nOutput: %s", err, output)
	}

	outputStr := string(output)
	if !strings.Contains(outputStr, "swagen version") {
		t.Error("Version output should contain 'swagen version'")
	}
	if !strings.Contains(outputStr, "Commit") {
		t.Error("Version output should contain 'Commit'")
	}
	if !strings.Contains(outputStr, "Build date") {
		t.Error("Version output should contain 'Build date'")
	}
}

func TestMainCLI_VersionShorthand(t *testing.T) {
	// Test version command shorthand
	cmd := exec.Command("go", "run", ".", "-V")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Version shorthand command failed: %v\nOutput: %s", err, output)
	}

	outputStr := string(output)
	if !strings.Contains(outputStr, "swagen version") {
		t.Error("Version output should contain 'swagen version'")
	}
}

func TestMainCLI_GenerateOpenAPI(t *testing.T) {
	// Create a temporary test directory with Go files
	tempDir, err := os.MkdirTemp("", "swagen_test")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

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

	// Test OpenAPI generation - run from swagen root directory
	cmd := exec.Command("go", "run", "github.com/ehabterra/swagen/cmd/swagen", tempDir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("OpenAPI generation failed: %v\nOutput: %s", err, output)
	}

	// Validate the output contains expected content
	outputStr := string(output)
	if !strings.Contains(outputStr, "\"openapi\":") {
		t.Error("Generated output should contain OpenAPI specification")
	}
	if !strings.Contains(outputStr, "\"paths\":") {
		t.Error("Generated output should contain paths section")
	}
	if !strings.Contains(outputStr, "\"components\":") {
		t.Error("Generated output should contain components section")
	}
}

func TestMainCLI_GenerateOpenAPIWithConfig(t *testing.T) {
	// Create a temporary test directory with Go files
	tempDir, err := os.MkdirTemp("", "swagen_test_config")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a Go file
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

	// Create a custom config file
	configFile := filepath.Join(tempDir, "swagen.yaml")
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

	// Test OpenAPI generation with custom config - run from swagen root directory
	cmd := exec.Command("go", "run", "github.com/ehabterra/swagen/cmd/swagen", "-c", configFile, tempDir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("OpenAPI generation with config failed: %v\nOutput: %s", err, output)
	}

	// Validate the output contains expected content
	outputStr := string(output)
	if !strings.Contains(outputStr, "\"openapi\":") {
		t.Error("Generated output should contain OpenAPI specification")
	}
	if !strings.Contains(outputStr, "\"paths\":") {
		t.Error("Generated output should contain paths section")
	}
	if !strings.Contains(outputStr, "\"components\":") {
		t.Error("Generated output should contain components section")
	}
}

func TestMainCLI_InvalidDirectory(t *testing.T) {
	// Test with non-existent directory
	cmd := exec.Command("go", "run", "github.com/ehabterra/swagen/cmd/swagen", "/non/existent/directory")
	output, err := cmd.CombinedOutput()

	// Should fail with an error
	if err == nil {
		t.Error("Expected error for non-existent directory")
	}

	outputStr := string(output)
	if !strings.Contains(outputStr, "input directory does not exist") {
		t.Error("Error output should contain 'input directory does not exist'")
	}
}

func TestMainCLI_InvalidConfigFile(t *testing.T) {
	// Create a temporary test directory
	tempDir, err := os.MkdirTemp("", "swagen_test_invalid_config")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a Go file
	goFile := filepath.Join(tempDir, "main.go")
	goContent := `package main

func main() {}`

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

	// Test with non-existent config file
	cmd := exec.Command("go", "run", "github.com/ehabterra/swagen/cmd/swagen", "-c", "/non/existent/config.yaml", tempDir)
	output, err := cmd.CombinedOutput()

	// Should fail with an error
	if err == nil {
		t.Error("Expected error for non-existent config file")
	}

	outputStr := string(output)
	if !strings.Contains(outputStr, "failed to load config") {
		t.Error("Error output should contain 'failed to load config'")
	}
}

func TestMainCLI_NoGoFiles(t *testing.T) {
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

	// Test with directory containing no Go files
	cmd := exec.Command("go", "run", "github.com/ehabterra/swagen/cmd/swagen", tempDir)
	output, err := cmd.CombinedOutput()

	// Should fail with an error
	if err == nil {
		t.Error("Expected error for directory with no Go files")
	}

	outputStr := string(output)
	if !strings.Contains(outputStr, "could not find Go module") {
		t.Error("Error output should contain 'could not find Go module'")
	}
}

func TestMainCLI_InvalidGoCode(t *testing.T) {
	// Create a temporary test directory with invalid Go code
	tempDir, err := os.MkdirTemp("", "swagen_test_invalid_go")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a Go file with syntax errors
	goFile := filepath.Join(tempDir, "main.go")
	goContent := `package main

import "net/http"

func main() {
	http.HandleFunc("/hello", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("hello"))
	// Missing closing brace
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

	// Test with invalid Go code
	cmd := exec.Command("go", "run", "github.com/ehabterra/swagen/cmd/swagen", tempDir)
	output, err := cmd.CombinedOutput()

	// Should fail with an error
	if err == nil {
		t.Error("Expected error for invalid Go code")
	}

	outputStr := string(output)
	if !strings.Contains(outputStr, "error") && !strings.Contains(outputStr, "Error") {
		t.Error("Error output should contain error message")
	}
}
