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
	iparser "github.com/ehabterra/swagen/internal/parser"
	"github.com/ehabterra/swagen/internal/spec"
	"gopkg.in/yaml.v3"
)

func TestEndToEnd_Fiber(t *testing.T) {
	projectRoot, err := findProjectRoot()
	if err != nil {
		t.Fatalf("failed to find project root: %v", err)
	}
	testDir := filepath.Join(projectRoot, "testdata", "fiber")
	if _, err := os.Stat(testDir); os.IsNotExist(err) {
		t.Skipf("Test directory %s does not exist, skipping Fiber end-to-end test", testDir)
	}

	routes, goFiles, err := parseFiberApp(testDir)
	if err != nil {
		t.Fatalf("Failed to parse Fiber app: %v", err)
	}
	if len(routes) == 0 {
		t.Fatal("No routes found in Fiber app")
	}

	// 3. Generate OpenAPI spec
	specObj, err := spec.MapParsedRoutesToOpenAPI(routes, goFiles, spec.GeneratorConfig{
		OpenAPIVersion: "3.0.0",
		Title:          "Fiber Example API",
		APIVersion:     "1.0.0",
	})
	if err != nil {
		t.Fatalf("GenerateFromRoutes failed: %v", err)
	}

	if specObj.OpenAPI != "3.1.1" {
		t.Errorf("Expected OpenAPI version 3.1.1, got %s", specObj.OpenAPI)
	}
	if specObj.Info.Title != "Fiber API" {
		t.Errorf("Expected title 'Fiber API', got %s", specObj.Info.Title)
	}
	if len(specObj.Paths) == 0 {
		t.Fatal("No paths found in generated spec")
	}

	expectedRoutes := []string{"/users", "/users/{id}", "/health", "/api/info"}
	for _, expectedRoute := range expectedRoutes {
		if _, exists := specObj.Paths[expectedRoute]; !exists {
			t.Errorf("Expected route %s not found in generated spec", expectedRoute)
		}
	}

	// --- Responses assertions ---
	// /users GET should have 200 response with array of User
	if usersPath, exists := specObj.Paths["/users"]; exists {
		if usersPath.Get == nil {
			t.Error("Expected GET /users operation not found")
		} else {
			resp, ok := usersPath.Get.Responses["200"]
			if !ok {
				t.Error("Expected 200 response for GET /users")
			} else {
				if resp.Content == nil {
					t.Error("Expected application/json content for GET /users 200 response")
				} else {
					schema := resp.Content["application/json"].Schema
					if schema == nil || schema.Type != "array" {
						t.Errorf("Expected array schema for GET /users 200 response, got %+v", schema)
					}
				}
			}
		}
	}
	// /users POST should have 201 response with SuccessResponse
	if usersPath, exists := specObj.Paths["/users"]; exists {
		if usersPath.Post == nil {
			t.Error("Expected POST /users operation not found")
		} else {
			resp, ok := usersPath.Post.Responses["201"]
			if !ok {
				t.Error("Expected 201 response for POST /users")
			} else {
				if resp.Content == nil {
					t.Error("Expected application/json content for POST /users 201 response")
				} else {
					schema := resp.Content["application/json"].Schema
					if schema == nil || schema.Ref == "" {
						t.Errorf("Expected schema ref for POST /users 201 response, got %+v", schema)
					}
				}
			}
		}
	}
	// /users/{id} GET should have 200 and 404 responses
	if userPath, exists := specObj.Paths["/users/{id}"]; exists {
		if userPath.Get == nil {
			t.Error("Expected GET /users/{id} operation not found")
		} else {
			if _, ok := userPath.Get.Responses["200"]; !ok {
				t.Error("Expected 200 response for GET /users/{id}")
			}
			if _, ok := userPath.Get.Responses["404"]; !ok {
				t.Error("Expected 404 response for GET /users/{id}")
			}
		}
	}
	// /users/{id} PUT should have 200 and 404 responses
	if userPath, exists := specObj.Paths["/users/{id}"]; exists {
		if userPath.Put == nil {
			t.Error("Expected PUT /users/{id} operation not found")
		} else {
			if _, ok := userPath.Put.Responses["200"]; !ok {
				t.Error("Expected 200 response for PUT /users/{id}")
			}
			if _, ok := userPath.Put.Responses["404"]; !ok {
				t.Error("Expected 404 response for PUT /users/{id}")
			}
		}
	}
	// /users/{id} DELETE should have 200 and 404 responses
	if userPath, exists := specObj.Paths["/users/{id}"]; exists {
		if userPath.Delete == nil {
			t.Error("Expected DELETE /users/{id} operation not found")
		} else {
			if _, ok := userPath.Delete.Responses["200"]; !ok {
				t.Error("Expected 200 response for DELETE /users/{id}")
			}
			if _, ok := userPath.Delete.Responses["404"]; !ok {
				t.Error("Expected 404 response for DELETE /users/{id}")
			}
		}
	}

	// Print all responses for manual inspection
	for path, item := range specObj.Paths {
		for _, op := range []*spec.Operation{item.Get, item.Post, item.Put, item.Delete} {
			if op == nil {
				continue
			}
			for code, resp := range op.Responses {
				t.Logf("%s %s -> %s: %+v", path, op.OperationID, code, resp)
			}
		}
	}

	// Print schemas for manual inspection
	if specObj.Components != nil {
		t.Logf("Actual schemas in components: %v", getSchemaNames(specObj.Components.Schemas))
	}

	jsonData, err := json.MarshalIndent(specObj, "", "  ")
	if err != nil {
		t.Fatalf("Failed to marshal spec to JSON: %v", err)
	}
	t.Logf("JSON output (first 500 chars): %s", string(jsonData[:min(500, len(jsonData))]))

	yamlData, err := yaml.Marshal(specObj)
	if err != nil {
		t.Fatalf("Failed to generate YAML: %v", err)
	}
	yamlStr := string(yamlData)
	if !strings.Contains(yamlStr, "openapi: 3.1.1") {
		t.Error("YAML output does not contain expected OpenAPI version")
	}
	if !strings.Contains(yamlStr, "title: Fiber API") {
		t.Error("YAML output does not contain expected title")
	}
	t.Log("Successfully generated OpenAPI spec from Fiber code.")
}

func parseFiberApp(dir string) ([]core.ParsedRoute, []*ast.File, error) {
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
	conf := types.Config{Importer: importer.For("source", nil)}
	_, _ = conf.Check("main", fset, files, nil)
	p := iparser.DefaultFiberParser()
	routes, err := p.Parse(fset, files, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse routes: %w", err)
	}
	return routes, files, nil
}
