package parser

import (
	"encoding/json"
	"fmt"
	"go/ast"
	goparser "go/parser"
	"go/token"
	"os"
	"path/filepath"
	"testing"

	"github.com/ehabterra/swagen/internal/parser"
	"github.com/ehabterra/swagen/internal/spec"
)

// findProjectRoot finds the directory containing go.mod file
func findProjectRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("could not find go.mod file")
		}
		dir = parent
	}
}

func TestEndToEnd_Gin(t *testing.T) {
	// Find project root and build testdata path
	projectRoot, err := findProjectRoot()
	if err != nil {
		t.Fatalf("failed to find project root: %v", err)
	}

	// 1. Parse the Gin example app from the testdata directory
	fset := token.NewFileSet()
	path := filepath.Join(projectRoot, "testdata", "gin")
	pkgs, err := goparser.ParseDir(fset, path, nil, goparser.ParseComments)
	if err != nil {
		t.Fatalf("failed to parse dir: %v", err)
	}

	var files []*ast.File
	for _, pkg := range pkgs {
		for _, f := range pkg.Files {
			files = append(files, f)
		}
	}

	// 2. Use the Gin parser with proper type information
	p := parser.DefaultGinParser()
	routes, err := p.Parse(fset, files, nil)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if len(routes) != 5 {
		t.Fatalf("expected 5 routes, got %d. Routes: %#v", len(routes), routes)
	}

	// 3. Generate OpenAPI spec
	// Replace spec.NewOpenAPIGenerator with direct call to MapMetadataToOpenAPI. Update GeneratorConfig usage to only use OpenAPIVersion, Title, APIVersion.
	specObj, err := spec.MapParsedRoutesToOpenAPI(routes, files, spec.GeneratorConfig{
		OpenAPIVersion: "3.0.0",
		Title:          "Gin Example API",
		APIVersion:     "1.0.0",
	})
	if err != nil {
		t.Fatalf("GenerateFromRoutes failed: %v", err)
	}

	// 4. Marshal to JSON for inspection
	jsonData, err := json.MarshalIndent(specObj, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal spec: %v", err)
	}

	// 5. Verify the POST endpoint has a request body
	var result map[string]interface{}
	if err := json.Unmarshal(jsonData, &result); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}

	paths := result["paths"].(map[string]interface{})
	usersPath, ok := paths["/users"].(map[string]interface{})
	if !ok {
		t.Fatal("path /users not found")
	}
	postOp, ok := usersPath["post"].(map[string]interface{})
	if !ok {
		t.Fatal("POST method for /users not found")
	}

	requestBody, ok := postOp["requestBody"].(map[string]interface{})
	if !ok {
		t.Fatalf("requestBody not found for POST /users. Spec:\n%s", jsonData)
	}
	content, ok := requestBody["content"].(map[string]interface{})
	if !ok {
		t.Fatal("content not found in requestBody")
	}
	appJSON, ok := content["application/json"].(map[string]interface{})
	if !ok {
		t.Fatal("application/json not found in content")
	}
	schema, ok := appJSON["schema"].(map[string]interface{})
	if !ok {
		t.Fatal("schema not found in application/json")
	}
	ref, ok := schema["$ref"].(string)
	if !ok || ref != "#/components/schemas/User" {
		t.Fatalf("expected schema $ref to be #/components/schemas/User, got %s", ref)
	}

	// Check POST /users responses for 201 and 400
	postResponses, ok := postOp["responses"].(map[string]interface{})
	if !ok {
		t.Fatal("responses not found for POST /users")
	}
	if _, ok := postResponses["201"]; !ok {
		t.Error("POST /users missing 201 response")
	}
	if _, ok := postResponses["400"]; !ok {
		t.Error("POST /users missing 400 response")
	}

	// Check GET/PUT/DELETE /users/{id} for 200, 204, 400, 404
	userIdPath, ok := paths["/users/{id}"].(map[string]interface{})
	if !ok {
		t.Fatal("path /users/{id} not found")
	}
	for _, method := range []string{"get", "put", "delete"} {
		op, ok := userIdPath[method].(map[string]interface{})
		if !ok {
			t.Errorf("method %s not found for /users/{id}", method)
			continue
		}
		responses, ok := op["responses"].(map[string]interface{})
		if !ok {
			t.Errorf("responses not found for %s /users/{id}", method)
			continue
		}
		// GET and PUT should have 200, 400, 404
		if method == "get" || method == "put" {
			for _, code := range []string{"200", "400", "404"} {
				if _, ok := responses[code]; !ok {
					t.Errorf("%s /users/{id} missing %s response", method, code)
				}
			}
		}
		// DELETE should have 204, 400, 404
		if method == "delete" {
			for _, code := range []string{"204", "400", "404"} {
				if _, ok := responses[code]; !ok {
					t.Errorf("DELETE /users/{id} missing %s response", code)
				}
			}
		}
	}

	t.Log("Successfully generated OpenAPI spec with request body from Gin code.")
}
