package parser

import (
	"encoding/json"
	"go/ast"
	goparser "go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ehabterra/swagen/internal/parser"
	"github.com/ehabterra/swagen/internal/spec"
	"gopkg.in/yaml.v3"
)

func TestEndToEnd_Chi(t *testing.T) {
	// Find project root and build testdata path
	projectRoot, err := findProjectRoot()
	if err != nil {
		t.Fatalf("failed to find project root: %v", err)
	}

	// 1. Recursively collect all Go files in the Chi example app
	fset := token.NewFileSet()
	path := filepath.Join(projectRoot, "testdata", "chi")
	var goFiles []string
	if err := filepath.WalkDir(path, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if filepath.Ext(p) == ".go" && !strings.HasSuffix(p, "_test.go") {
			goFiles = append(goFiles, p)
		}
		return nil
	}); err != nil {
		t.Fatalf("failed to walk dir: %v", err)
	}

	var files []*ast.File
	for _, filePath := range goFiles {
		f, err := goparser.ParseFile(fset, filePath, nil, goparser.ParseComments)
		if err != nil {
			t.Fatalf("failed to parse file %s: %v", filePath, err)
		}
		t.Logf("[DEBUG] Parsed file: %s", filePath)
		files = append(files, f)
	}

	t.Logf("[DEBUG] Total files parsed: %d", len(files))

	// 2. Use the Chi parser with proper type information
	p := parser.DefaultChiParser()
	routes, err := p.Parse(fset, files, nil)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if len(routes) != 6 {
		t.Fatalf("expected 6 routes, got %d. Routes: %#v", len(routes), routes)
	}

	// Debug: Print the actual routes found
	t.Logf("[DEBUG] Found %d routes:", len(routes))
	for i, route := range routes {
		t.Logf("[DEBUG] Route %d: %s %s -> Handler: %s", i, route.Method, route.Path, route.HandlerName)
	}

	// 3. Generate OpenAPI spec
	specObj, err := spec.MapParsedRoutesToOpenAPI(routes, files, spec.GeneratorConfig{
		OpenAPIVersion: "3.0.0",
		Title:          "Chi Example API",
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

	t.Log("Successfully generated OpenAPI spec with request body from Chi code.")

	// Additional: Validate response status codes in openapi.yaml
	openapiPath := filepath.Join(projectRoot, "testdata", "chi", "openapi.yaml")
	data, err := os.ReadFile(openapiPath)
	if err != nil {
		t.Fatalf("failed to read openapi.yaml: %v", err)
	}
	var specYaml map[string]interface{}
	if err := yaml.Unmarshal(data, &specYaml); err != nil {
		t.Fatalf("failed to parse openapi.yaml: %v", err)
	}

	paths, ok = specYaml["paths"].(map[string]interface{})
	if !ok {
		t.Fatalf("openapi.yaml missing paths")
	}

	// /users POST should have 201 and 400
	if usersPath, ok := paths["/users"].(map[string]interface{}); ok {
		if postOp, ok := usersPath["post"].(map[string]interface{}); ok {
			if responses, ok := postOp["responses"].(map[string]interface{}); ok {
				if _, ok := responses["201"]; !ok {
					t.Errorf("/users POST missing 201 response")
				}
				if _, ok := responses["400"]; !ok {
					t.Errorf("/users POST missing 400 response")
				}
			}
		}
	}

	// /users/{id} GET should have 200, 400, 404
	if userPath, ok := paths["/users/{id}"].(map[string]interface{}); ok {
		if getOp, ok := userPath["get"].(map[string]interface{}); ok {
			if responses, ok := getOp["responses"].(map[string]interface{}); ok {
				for _, code := range []string{"200", "400", "404"} {
					if _, ok := responses[code]; !ok {
						t.Errorf("/users/{id} GET missing %s response", code)
					}
				}
			}
		}
	}

	// /users/{id} PUT should have 200, 400, 404
	if userPath, ok := paths["/users/{id}"].(map[string]interface{}); ok {
		if putOp, ok := userPath["put"].(map[string]interface{}); ok {
			if responses, ok := putOp["responses"].(map[string]interface{}); ok {
				for _, code := range []string{"200", "400", "404"} {
					if _, ok := responses[code]; !ok {
						t.Errorf("/users/{id} PUT missing %s response", code)
					}
				}
			}
		}
	}

	// /users/{id} DELETE should have 400, 404
	if userPath, ok := paths["/users/{id}"].(map[string]interface{}); ok {
		if deleteOp, ok := userPath["delete"].(map[string]interface{}); ok {
			if responses, ok := deleteOp["responses"].(map[string]interface{}); ok {
				for _, code := range []string{"400", "404"} {
					if _, ok := responses[code]; !ok {
						t.Errorf("/users/{id} DELETE missing %s response", code)
					}
				}
			}
		}
	}

	// /jsoniter POST should have 201 and 400
	if jsoniterPath, ok := paths["/jsoniter"].(map[string]interface{}); ok {
		if postOp, ok := jsoniterPath["post"].(map[string]interface{}); ok {
			if responses, ok := postOp["responses"].(map[string]interface{}); ok {
				if _, ok := responses["201"]; !ok {
					t.Errorf("/jsoniter POST missing 201 response")
				}
				if _, ok := responses["400"]; !ok {
					t.Errorf("/jsoniter POST missing 400 response")
				}
			}
		}
	}

	// Check schemas for JsoniterReq and JsoniterResp in the generated spec
	if components, ok := result["components"].(map[string]interface{}); ok {
		if schemas, ok := components["schemas"].(map[string]interface{}); ok {
			if _, ok := schemas["JsoniterReq"]; !ok {
				t.Errorf("JsoniterReq schema missing in components")
			}
			if _, ok := schemas["JsoniterResp"]; !ok {
				t.Errorf("JsoniterResp schema missing in components")
			}
		}
	}
}
