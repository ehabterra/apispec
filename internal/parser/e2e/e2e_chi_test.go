package parser

import (
	"encoding/json"
	"go/ast"
	goparser "go/parser"
	"go/token"
	"os"
	"path/filepath"
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

	// 1. Parse the Chi example app from the testdata directory
	fset := token.NewFileSet()
	path := filepath.Join(projectRoot, "testdata", "chi")
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

	// 2. Use the Chi parser with proper type information
	p := parser.DefaultChiParserWithTypes(nil)
	routes, err := p.Parse(fset, files)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if len(routes) != 6 {
		t.Fatalf("expected 6 routes, got %d. Routes: %#v", len(routes), routes)
	}

	// 3. Generate OpenAPI spec
	gen := spec.NewOpenAPIGenerator(spec.GeneratorConfig{})
	specObj, err := gen.GenerateFromRoutes(routes, files)
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
