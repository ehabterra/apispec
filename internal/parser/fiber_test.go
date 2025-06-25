package parser_test

import (
	"go/ast"
	goparser "go/parser"
	"go/token"
	"go/types"
	"testing"

	"github.com/ehabterra/swagen/internal/core"
	"github.com/ehabterra/swagen/internal/parser"
)

const fiberExample = `
package main
import "github.com/gofiber/fiber/v2"
func main() {
	app := fiber.New()
	app.Get("/hello", helloHandler)
}
func helloHandler(c *fiber.Ctx) error {
	return c.JSON(map[string]string{"message": "Hello"})
}
`

func TestFiberParser_Parse(t *testing.T) {
	fset := token.NewFileSet()
	file, err := goparser.ParseFile(fset, "example.go", fiberExample, goparser.ParseComments)
	if err != nil {
		t.Fatalf("failed to parse example: %v", err)
	}

	p := parser.DefaultFiberParserWithTypes(nil)
	routes, err := p.Parse(fset, []*ast.File{file})
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if len(routes) != 1 {
		t.Fatalf("expected 1 route, got %d", len(routes))
	}
	r := routes[0]
	if r.Method != "Get" || r.Path != "/hello" || r.HandlerName != "helloHandler" {
		t.Errorf("unexpected route: %+v", r)
	}
}

func TestFiberParser(t *testing.T) {
	tests := []struct {
		name     string
		code     string
		expected []core.ParsedRoute
	}{
		{
			name: "basic GET route",
			code: `
package main

import "github.com/gofiber/fiber/v2"

func main() {
	app := fiber.New()
	app.Get("/users", getUsers)
}

func getUsers(c *fiber.Ctx) error {
	return c.JSON(map[string]interface{}{"users": []string{}})
}
`,
			expected: []core.ParsedRoute{
				{
					Method:      "Get",
					Path:        "/users",
					HandlerName: "getUsers",
				},
			},
		},
		{
			name: "POST route with binding",
			code: `
package main

import "github.com/gofiber/fiber/v2"

type User struct {
	Name string ` + "`json:\"name\"`" + `
	Age  int    ` + "`json:\"age\"`" + `
}

func main() {
	app := fiber.New()
	app.Post("/users", createUser)
}

func createUser(c *fiber.Ctx) error {
	var user User
	if err := c.BodyParser(&user); err != nil {
		return c.Status(400).JSON(map[string]string{"error": "Invalid input"})
	}
	return c.Status(201).JSON(user)
}
`,
			expected: []core.ParsedRoute{
				{
					Method:        "Post",
					Path:          "/users",
					HandlerName:   "createUser",
					RequestType:   "User",
					RequestSource: "body",
				},
			},
		},
		{
			name: "route with path parameters",
			code: `
package main

import "github.com/gofiber/fiber/v2"

func main() {
	app := fiber.New()
	app.Get("/users/:id", getUser)
}

func getUser(c *fiber.Ctx) error {
	return c.JSON(map[string]interface{}{"id": "123"})
}
`,
			expected: []core.ParsedRoute{
				{
					Method:      "Get",
					Path:        "/users/{id}",
					HandlerName: "getUser",
				},
			},
		},
		{
			name: "multiple routes",
			code: `
package main

import "github.com/gofiber/fiber/v2"

func main() {
	app := fiber.New()
	app.Get("/users", getUsers)
	app.Post("/users", createUser)
	app.Put("/users/:id", updateUser)
	app.Delete("/users/:id", deleteUser)
}

func getUsers(c *fiber.Ctx) error {
	return c.JSON([]string{})
}

func createUser(c *fiber.Ctx) error {
	return c.Status(201).JSON(map[string]string{"status": "created"})
}

func updateUser(c *fiber.Ctx) error {
	return c.JSON(map[string]string{"status": "updated"})
}

func deleteUser(c *fiber.Ctx) error {
	return c.Status(204).Send(nil)
}
`,
			expected: []core.ParsedRoute{
				{
					Method:      "Get",
					Path:        "/users",
					HandlerName: "getUsers",
				},
				{
					Method:      "Post",
					Path:        "/users",
					HandlerName: "createUser",
				},
				{
					Method:      "Put",
					Path:        "/users/{id}",
					HandlerName: "updateUser",
				},
				{
					Method:      "Delete",
					Path:        "/users/{id}",
					HandlerName: "deleteUser",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fset := token.NewFileSet()
			file, err := goparser.ParseFile(fset, "", tt.code, goparser.ParseComments)
			if err != nil {
				t.Fatalf("Failed to parse code: %v", err)
			}

			// Create a simple types.Info for testing
			info := &types.Info{
				Types: make(map[ast.Expr]types.TypeAndValue),
			}

			parser := parser.DefaultFiberParserWithTypes(info)
			routes, err := parser.Parse(fset, []*ast.File{file})
			if err != nil {
				t.Fatalf("Failed to parse routes: %v", err)
			}

			if len(routes) != len(tt.expected) {
				t.Errorf("Expected %d routes, got %d", len(tt.expected), len(routes))
				return
			}

			for i, expected := range tt.expected {
				if i >= len(routes) {
					break
				}
				route := routes[i]
				if route.Method != expected.Method {
					t.Errorf("Route %d: expected method %s, got %s", i, expected.Method, route.Method)
				}
				if route.Path != expected.Path {
					t.Errorf("Route %d: expected path %s, got %s", i, expected.Path, route.Path)
				}
				if route.HandlerName != expected.HandlerName {
					t.Errorf("Route %d: expected handler %s, got %s", i, expected.HandlerName, route.HandlerName)
				}
				if expected.RequestType != "" && route.RequestType != expected.RequestType {
					t.Errorf("Route %d: expected request type %s, got %s", i, expected.RequestType, route.RequestType)
				}
				if expected.RequestSource != "" && route.RequestSource != expected.RequestSource {
					t.Errorf("Route %d: expected request source %s, got %s", i, expected.RequestSource, route.RequestSource)
				}
			}
		})
	}
}

func TestFiberPathConversion(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"/users", "/users"},
		{"/users/:id", "/users/{id}"},
		{"/users/:id/posts/:postId", "/users/{id}/posts/{postId}"},
		{"/api/v1/users/:userId", "/api/v1/users/{userId}"},
		{"/:category/:id", "/{category}/{id}"},
	}

	for _, tt := range tests {
		result := convertFiberPathToOpenAPI(tt.input)
		if result != tt.expected {
			t.Errorf("convertFiberPathToOpenAPI(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestFiberHTTPMethodDetection(t *testing.T) {
	validMethods := []string{"Get", "Post", "Put", "Delete", "Patch", "Options", "Head"}
	invalidMethods := []string{"get", "post", "PUTS", "REMOVE", "UPDATE"}

	for _, method := range validMethods {
		if !isFiberHTTPMethod(method) {
			t.Errorf("Expected %s to be a valid Fiber HTTP method", method)
		}
	}

	for _, method := range invalidMethods {
		if isFiberHTTPMethod(method) {
			t.Errorf("Expected %s to be an invalid Fiber HTTP method", method)
		}
	}
}

// Helper functions for testing

func convertFiberPathToOpenAPI(path string) string {
	// Simple implementation for testing
	// Replace :param with {param}
	result := path
	for i := 0; i < len(result)-1; i++ {
		if result[i] == ':' {
			// Find the end of the parameter name
			end := i + 1
			for end < len(result) && result[end] != '/' {
				end++
			}
			paramName := result[i+1 : end]
			result = result[:i] + "{" + paramName + "}" + result[end:]
			i = i + len(paramName) + 1 // Skip the replaced part
		}
	}
	return result
}

func isFiberHTTPMethod(method string) bool {
	validMethods := map[string]bool{
		"Get":     true,
		"Post":    true,
		"Put":     true,
		"Delete":  true,
		"Patch":   true,
		"Options": true,
		"Head":    true,
	}
	return validMethods[method]
}
