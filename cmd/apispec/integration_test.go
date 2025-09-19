package main

import (
	"bytes"
	"flag"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMainCLI_Help(t *testing.T) {
	// Test help command by calling parseFlags directly with --help
	// The flag package provides a default help flag that returns flag.ErrHelp
	_, err := parseFlags([]string{"--help"})
	if err == nil {
		t.Error("Expected error for --help flag")
	}

	// The error should be flag.ErrHelp
	if err != flag.ErrHelp {
		t.Errorf("Expected flag.ErrHelp, got: %v", err)
	}
}

func TestMainCLI_Version(t *testing.T) {
	// Capture stdout for version output
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Test version by calling parseFlags and printVersion
	config, err := parseFlags([]string{"--version"})
	if err != nil {
		t.Fatalf("Failed to parse version flag: %v", err)
	}

	if !config.ShowVersion {
		t.Error("ShowVersion should be true")
	}

	// Call printVersion function directly
	printVersion()

	// Restore stdout
	err = w.Close()
	if err != nil {
		t.Fatalf("Failed to close stdout: %v", err)
	}
	os.Stdout = oldStdout

	// Read captured output
	var buf bytes.Buffer
	_, err = io.Copy(&buf, r)
	if err != nil {
		t.Fatalf("Failed to copy output: %v", err)
	}
	outputStr := buf.String()

	if !strings.Contains(outputStr, "apispec version") {
		t.Error("Version output should contain 'apispec version'")
	}
	if !strings.Contains(outputStr, "Commit") {
		t.Error("Version output should contain 'Commit'")
	}
	if !strings.Contains(outputStr, "Build date") {
		t.Error("Version output should contain 'Build date'")
	}
}

func TestMainCLI_VersionShorthand(t *testing.T) {
	// Test version shorthand by calling parseFlags
	config, err := parseFlags([]string{"-V"})
	if err != nil {
		t.Fatalf("Failed to parse version shorthand flag: %v", err)
	}

	if !config.ShowVersion {
		t.Error("ShowVersion should be true for -V flag")
	}
}

func TestMainCLI_GenerateOpenAPI(t *testing.T) {
	// Create a temporary test directory with Go files
	tempDir, err := os.MkdirTemp("", "apispec_test")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer func() {
		if err := os.RemoveAll(tempDir); err != nil {
			t.Errorf("Failed to remove temp directory: %v", err)
		}
	}()

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

	// Test OpenAPI generation by calling functions directly
	config, err := parseFlags([]string{tempDir})
	if err != nil {
		t.Fatalf("Failed to parse flags: %v", err)
	}

	// Generate OpenAPI spec
	data, genEngine, err := runGeneration(config)
	if err != nil {
		t.Fatalf("OpenAPI generation failed: %v", err)
	}

	if genEngine == nil {
		t.Fatal("Engine should not be nil")
	}

	if len(data) == 0 {
		t.Fatal("Generated data should not be empty")
	}

	// Validate the output contains expected content
	outputStr := string(data)
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
	tempDir, err := os.MkdirTemp("", "apispec_test_config")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer func() {
		if err := os.RemoveAll(tempDir); err != nil {
			t.Errorf("Failed to remove temp directory: %v", err)
		}
	}()

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

	// Test OpenAPI generation with custom config by calling functions directly
	config, err := parseFlags([]string{"-c", configFile, tempDir})
	if err != nil {
		t.Fatalf("Failed to parse flags: %v", err)
	}

	if config.ConfigFile != configFile {
		t.Errorf("Expected config file %s, got %s", configFile, config.ConfigFile)
	}

	// Generate OpenAPI spec
	data, genEngine, err := runGeneration(config)
	if err != nil {
		t.Fatalf("OpenAPI generation with config failed: %v", err)
	}

	if genEngine == nil {
		t.Fatal("Engine should not be nil")
	}

	// Validate the output contains expected content
	outputStr := string(data)
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
	// Test with non-existent directory by calling functions directly
	config, err := parseFlags([]string{"/non/existent/directory"})
	if err != nil {
		t.Fatalf("Failed to parse flags: %v", err)
	}

	// Should pass parse but fail generation
	_, _, err = runGeneration(config)
	if err == nil {
		t.Error("Expected error for non-existent directory")
	}

	if !strings.Contains(err.Error(), "directory does not exist") && !strings.Contains(err.Error(), "no such file") {
		t.Errorf("Error should mention directory issue, got: %v", err)
	}
}

func TestMainCLI_InvalidConfigFile(t *testing.T) {
	// Create a temporary test directory
	tempDir, err := os.MkdirTemp("", "apispec_test_invalid_config")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer func() {
		if err := os.RemoveAll(tempDir); err != nil {
			t.Errorf("Failed to remove temp directory: %v", err)
		}
	}()

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

	// Test with non-existent config file by calling functions directly
	config, err := parseFlags([]string{"-c", "/non/existent/config.yaml", tempDir})
	if err != nil {
		t.Fatalf("Failed to parse flags: %v", err)
	}

	// Should fail during generation due to config loading
	_, _, err = runGeneration(config)
	if err == nil {
		t.Error("Expected error for non-existent config file")
	}

	if !strings.Contains(err.Error(), "failed to load config") && !strings.Contains(err.Error(), "no such file") {
		t.Errorf("Error should mention config loading issue, got: %v", err)
	}
}

func TestMainCLI_NoGoFiles(t *testing.T) {
	// Create a temporary test directory without Go files
	tempDir, err := os.MkdirTemp("", "apispec_test_no_go")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer func() {
		if err := os.RemoveAll(tempDir); err != nil {
			t.Errorf("Failed to remove temp directory: %v", err)
		}
	}()

	// Create a non-Go file
	textFile := filepath.Join(tempDir, "readme.txt")
	err = os.WriteFile(textFile, []byte("This is not a Go file"), 0644)
	if err != nil {
		t.Fatalf("Failed to write text file: %v", err)
	}

	// Test with directory containing no Go files by calling functions directly
	config, err := parseFlags([]string{tempDir})
	if err != nil {
		t.Fatalf("Failed to parse flags: %v", err)
	}

	// Should fail during generation due to missing Go module
	_, _, err = runGeneration(config)
	if err == nil {
		t.Error("Expected error for directory with no Go files")
	}

	if !strings.Contains(err.Error(), "could not find Go module") && !strings.Contains(err.Error(), "go.mod") {
		t.Errorf("Error should mention Go module issue, got: %v", err)
	}
}

func TestMainCLI_InvalidGoCode(t *testing.T) {
	// Create a temporary test directory with invalid Go code
	tempDir, err := os.MkdirTemp("", "apispec_test_invalid_go")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer func() {
		if err := os.RemoveAll(tempDir); err != nil {
			t.Errorf("Failed to remove temp directory: %v", err)
		}
	}()

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

	// Test with invalid Go code by calling functions directly
	config, err := parseFlags([]string{tempDir})
	if err != nil {
		t.Fatalf("Failed to parse flags: %v", err)
	}

	// Should fail during generation due to syntax errors
	_, _, err = runGeneration(config)
	if err == nil {
		t.Error("Expected error for invalid Go code")
	}

	// Should contain some indication of parsing/compilation error
	errStr := err.Error()
	if !strings.Contains(errStr, "error") && !strings.Contains(errStr, "Error") && !strings.Contains(errStr, "failed") {
		t.Errorf("Error should contain error indication, got: %v", err)
	}
}

// Additional tests for the extracted functions

func TestParseFlags(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected *CLIConfig
		wantErr  bool
	}{
		{
			name: "default values",
			args: []string{},
			expected: &CLIConfig{
				InputDir:           ".",
				OutputFile:         "openapi.json",
				Title:              "Generated API",
				APIVersion:         "1.0.0",
				ContactName:        "Ehab",
				ContactURL:         "https://ehabterra.github.io/",
				ContactEmail:       "ehabterra@hotmail.com",
				OpenAPIVersion:     "3.1.1",
				MaxNodesPerTree:    10000,
				MaxChildrenPerNode: 150,
				MaxArgsPerFunction: 30,
				MaxNestedArgsDepth: 50,
			},
		},
		{
			name: "custom values",
			args: []string{"-d", "/custom/dir", "-o", "custom.yaml", "-t", "My API"},
			expected: &CLIConfig{
				InputDir:      "/custom/dir",
				OutputFile:    "custom.yaml",
				Title:         "My API",
				OutputFlagSet: true, // Because -o was used
			},
		},
		{
			name: "positional argument overrides dir flag",
			args: []string{"-d", "/flag/dir", "/positional/dir"},
			expected: &CLIConfig{
				InputDir: "/positional/dir",
			},
		},
		{
			name: "output flag set detection",
			args: []string{"-o", "test.yaml"},
			expected: &CLIConfig{
				OutputFile:    "test.yaml",
				OutputFlagSet: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config, err := parseFlags(tt.args)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseFlags() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if config.InputDir != tt.expected.InputDir && tt.expected.InputDir != "" {
					t.Errorf("InputDir = %v, want %v", config.InputDir, tt.expected.InputDir)
				}
				if config.OutputFile != tt.expected.OutputFile && tt.expected.OutputFile != "" {
					t.Errorf("OutputFile = %v, want %v", config.OutputFile, tt.expected.OutputFile)
				}
				if config.Title != tt.expected.Title && tt.expected.Title != "" {
					t.Errorf("Title = %v, want %v", config.Title, tt.expected.Title)
				}
				if config.OutputFlagSet != tt.expected.OutputFlagSet {
					t.Errorf("OutputFlagSet = %v, want %v", config.OutputFlagSet, tt.expected.OutputFlagSet)
				}
			}
		})
	}
}

func TestPrintVersion(t *testing.T) {
	// Capture stdout for version output
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Call printVersion function directly
	printVersion()

	// Restore stdout
	err := w.Close()
	if err != nil {
		t.Fatalf("Failed to close stdout: %v", err)
	}
	os.Stdout = oldStdout

	// Read captured output
	var buf bytes.Buffer
	_, err = io.Copy(&buf, r)
	if err != nil {
		t.Fatalf("Failed to copy output: %v", err)
	}
	outputStr := buf.String()

	if !strings.Contains(outputStr, "apispec version") {
		t.Error("Version output should contain 'apispec version'")
	}
	if !strings.Contains(outputStr, "Commit") {
		t.Error("Version output should contain 'Commit'")
	}
	if !strings.Contains(outputStr, "Build date") {
		t.Error("Version output should contain 'Build date'")
	}
}
