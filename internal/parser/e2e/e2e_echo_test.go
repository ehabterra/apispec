package parser

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/importer"
	goparser "go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ehabterra/swagen/internal/core"
	"github.com/ehabterra/swagen/internal/parser"
	"github.com/ehabterra/swagen/internal/spec"
	"gopkg.in/yaml.v3"
)

func TestEndToEnd_Echo(t *testing.T) {
	// Find project root and build testdata path
	projectRoot, err := findProjectRoot()
	if err != nil {
		t.Fatalf("failed to find project root: %v", err)
	}

	// Test directory containing Echo app
	testDir := filepath.Join(projectRoot, "testdata", "echo")

	// Check if test directory exists
	if _, err := os.Stat(testDir); os.IsNotExist(err) {
		t.Skipf("Test directory %s does not exist, skipping Echo end-to-end test", testDir)
	}

	// Parse the Echo application
	routes, goFiles, err := parseEchoApp(testDir)
	if err != nil {
		t.Fatalf("Failed to parse Echo app: %v", err)
	}

	// Verify we found routes
	if len(routes) == 0 {
		t.Fatal("No routes found in Echo app")
	}

	// 3. Generate OpenAPI spec
	specObj, err := spec.MapParsedRoutesToOpenAPI(routes, goFiles, spec.GeneratorConfig{
		OpenAPIVersion: "3.0.0",
		Title:          "Echo Example API",
		APIVersion:     "1.0.0",
	})
	if err != nil {
		t.Fatalf("GenerateFromRoutes failed: %v", err)
	}

	// Verify the spec has the expected structure
	if specObj.OpenAPI != "3.1.1" {
		t.Errorf("Expected OpenAPI version 3.1.1, got %s", specObj.OpenAPI)
	}

	if specObj.Info.Title != "Echo API" {
		t.Errorf("Expected title 'Echo API', got %s", specObj.Info.Title)
	}

	// Verify we have paths
	if len(specObj.Paths) == 0 {
		t.Fatal("No paths found in generated spec")
	}

	// Check for specific expected routes
	expectedRoutes := []string{
		"/users",
		"/users/{id}",
		"/health",
		"/api/info",
	}

	for _, expectedRoute := range expectedRoutes {
		if _, exists := specObj.Paths[expectedRoute]; !exists {
			t.Errorf("Expected route %s not found in generated spec", expectedRoute)
		}
	}

	// Verify specific route details
	if usersPath, exists := specObj.Paths["/users"]; exists {
		// Check GET /users
		if usersPath.Get == nil {
			t.Error("Expected GET /users operation not found")
		} else {
			if usersPath.Get.OperationID != "getUsers" {
				t.Errorf("Expected operation ID 'getUsers', got %s", usersPath.Get.OperationID)
			}
		}

		// Check POST /users
		if usersPath.Post == nil {
			t.Error("Expected POST /users operation not found")
		} else {
			if usersPath.Post.OperationID != "createUser" {
				t.Errorf("Expected operation ID 'createUser', got %s", usersPath.Post.OperationID)
			}
		}
	}

	// Check path parameter route
	if userPath, exists := specObj.Paths["/users/{id}"]; exists {
		if userPath.Get == nil {
			t.Error("Expected GET /users/{id} operation not found")
		} else {
			if userPath.Get.OperationID != "getUser" {
				t.Errorf("Expected operation ID 'getUser', got %s", userPath.Get.OperationID)
			}
		}

		if userPath.Put == nil {
			t.Error("Expected PUT /users/{id} operation not found")
		} else {
			if userPath.Put.OperationID != "updateUser" {
				t.Errorf("Expected operation ID 'updateUser', got %s", userPath.Put.OperationID)
			}
		}

		if userPath.Delete == nil {
			t.Error("Expected DELETE /users/{id} operation not found")
		} else {
			if userPath.Delete.OperationID != "deleteUser" {
				t.Errorf("Expected operation ID 'deleteUser', got %s", userPath.Delete.OperationID)
			}
		}
	}

	// Verify components exist
	if specObj.Components == nil {
		t.Fatal("Expected components section not found")
	}

	// Check for expected schemas
	expectedSchemas := []string{"User", "CreateUserRequest", "UpdateUserRequest", "ErrorResponse", "SuccessResponse"}
	for _, schemaName := range expectedSchemas {
		if _, exists := specObj.Components.Schemas[schemaName]; !exists {
			t.Errorf("Expected schema %s not found in components", schemaName)
		}
	}

	// Debug: Print actual schemas
	t.Logf("Actual schemas in components: %v", getSchemaNames(specObj.Components.Schemas))

	// Debug: Print routes and their types
	for _, route := range routes {
		t.Logf("Route: %s %s -> Handler: %s, RequestType: %s", route.Method, route.Path, route.HandlerName, route.RequestType)
		for i, resp := range route.ResponseTypes {
			t.Logf("  Response %d: Status=%d, Type=%s, MediaType=%s", i, resp.StatusCode, resp.Type, resp.MediaType)
		}
	}

	// Print the OpenAPI schema for GET /users
	if usersPath, exists := specObj.Paths["/users"]; exists && usersPath.Get != nil {
		if resp, ok := usersPath.Get.Responses["200"]; ok {
			if resp.Content != nil {
				if mt, ok := resp.Content["application/json"]; ok && mt.Schema != nil {
					jsonSchema, _ := json.MarshalIndent(mt.Schema, "", "  ")
					t.Logf("OpenAPI schema for GET /users 200 response: %s", string(jsonSchema))
				}
			}
		}
	}

	// Print the actual User schema from components
	if userSchema, exists := specObj.Components.Schemas["User"]; exists {
		jsonUserSchema, _ := json.MarshalIndent(userSchema, "", "  ")
		t.Logf("User schema in components: %s", string(jsonUserSchema))
	}

	// Test JSON output
	jsonData, err := json.MarshalIndent(specObj, "", "  ")
	if err != nil {
		t.Fatalf("Failed to marshal spec to JSON: %v", err)
	}

	// Debug: Print first 500 characters of JSON
	t.Logf("JSON output (first 500 chars): %s", string(jsonData[:min(500, len(jsonData))]))

	// Verify JSON contains expected content (robust substring check)
	jsonStr := string(jsonData)
	if !strings.Contains(jsonStr, "\"openapi\": \"3.1.1\"") {
		t.Error("JSON output does not contain expected OpenAPI version")
	}

	if !strings.Contains(jsonStr, "\"title\": \"Echo API\"") {
		t.Error("JSON output does not contain expected title")
	}

	// Test YAML output
	yamlData, err := yaml.Marshal(specObj)
	if err != nil {
		t.Fatalf("Failed to generate YAML: %v", err)
	}

	yamlStr := string(yamlData)
	if !strings.Contains(yamlStr, "openapi: 3.1.1") {
		t.Error("YAML output does not contain expected OpenAPI version")
	}

	if !strings.Contains(yamlStr, "title: Echo API") {
		t.Error("YAML output does not contain expected title")
	}

	t.Log("Successfully generated OpenAPI spec from Echo code.")
}

func parseEchoApp(dir string) ([]core.ParsedRoute, []*ast.File, error) {
	// Parse all Go files in the directory
	fset := token.NewFileSet()
	pkgs, err := goparser.ParseDir(fset, dir, nil, goparser.ParseComments)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse directory: %w", err)
	}

	var files []*ast.File
	for _, pkg := range pkgs {
		for _, f := range pkg.Files {
			files = append(files, f)
		}
	}

	// Set up type checking
	conf := types.Config{Importer: importer.For("source", nil)}
	_, err = conf.Check("main", fset, files, nil)
	if err != nil {
		// For testing, we'll continue even if type checking fails
		// as the parser can work with minimal type information
	}

	// Use the Echo parser
	p := parser.DefaultEchoParser()
	routes, err := p.Parse(fset, files, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse routes: %w", err)
	}

	return routes, files, nil
}

func getSchemaNames(schemas map[string]*spec.Schema) []string {
	var names []string
	for name := range schemas {
		names = append(names, name)
	}
	return names
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
