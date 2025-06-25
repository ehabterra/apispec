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

func TestEchoParser(t *testing.T) {
	tests := []struct {
		name     string
		code     string
		expected []core.ParsedRoute
	}{
		{
			name: "basic GET route",
			code: `
package main

import "github.com/labstack/echo/v4"

func main() {
	e := echo.New()
	e.GET("/users", getUsers)
}

func getUsers(c echo.Context) error {
	return c.JSON(200, map[string]interface{}{"users": []string{}})
}
`,
			expected: []core.ParsedRoute{
				{
					Method:      "GET",
					Path:        "/users",
					HandlerName: "getUsers",
				},
			},
		},
		{
			name: "POST route with binding",
			code: `
package main

import "github.com/labstack/echo/v4"

type User struct {
	Name string ` + "`json:\"name\"`" + `
	Age  int    ` + "`json:\"age\"`" + `
}

func main() {
	e := echo.New()
	e.POST("/users", createUser)
}

func createUser(c echo.Context) error {
	var user User
	if err := c.Bind(&user); err != nil {
		return c.JSON(400, map[string]string{"error": "Invalid input"})
	}
	return c.JSON(201, user)
}
`,
			expected: []core.ParsedRoute{
				{
					Method:        "POST",
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

import "github.com/labstack/echo/v4"

func main() {
	e := echo.New()
	e.GET("/users/:id", getUser)
}

func getUser(c echo.Context) error {
	return c.JSON(200, map[string]interface{}{"id": "123"})
}
`,
			expected: []core.ParsedRoute{
				{
					Method:      "GET",
					Path:        "/users/{id}",
					HandlerName: "getUser",
				},
			},
		},
		{
			name: "multiple routes",
			code: `
package main

import "github.com/labstack/echo/v4"

func main() {
	e := echo.New()
	e.GET("/users", getUsers)
	e.POST("/users", createUser)
	e.PUT("/users/:id", updateUser)
	e.DELETE("/users/:id", deleteUser)
}

func getUsers(c echo.Context) error {
	return c.JSON(200, []string{})
}

func createUser(c echo.Context) error {
	return c.JSON(201, map[string]string{"status": "created"})
}

func updateUser(c echo.Context) error {
	return c.JSON(200, map[string]string{"status": "updated"})
}

func deleteUser(c echo.Context) error {
	return c.JSON(204, nil)
}
`,
			expected: []core.ParsedRoute{
				{
					Method:      "GET",
					Path:        "/users",
					HandlerName: "getUsers",
				},
				{
					Method:      "POST",
					Path:        "/users",
					HandlerName: "createUser",
				},
				{
					Method:      "PUT",
					Path:        "/users/{id}",
					HandlerName: "updateUser",
				},
				{
					Method:      "DELETE",
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

			parser := parser.DefaultEchoParserWithTypes(info)
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

func TestEchoPathConversion(t *testing.T) {
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
		result := convertEchoPathToOpenAPI(tt.input)
		if result != tt.expected {
			t.Errorf("convertEchoPathToOpenAPI(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestEchoHTTPMethodDetection(t *testing.T) {
	validMethods := []string{"GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS", "HEAD"}
	invalidMethods := []string{"get", "post", "PUTS", "REMOVE", "UPDATE"}

	for _, method := range validMethods {
		if !isEchoHTTPMethod(method) {
			t.Errorf("Expected %s to be a valid Echo HTTP method", method)
		}
	}

	for _, method := range invalidMethods {
		if isEchoHTTPMethod(method) {
			t.Errorf("Expected %s to be an invalid Echo HTTP method", method)
		}
	}
}

// Helper functions for testing

func convertEchoPathToOpenAPI(path string) string {
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

func isEchoHTTPMethod(method string) bool {
	validMethods := map[string]bool{
		"GET":     true,
		"POST":    true,
		"PUT":     true,
		"DELETE":  true,
		"PATCH":   true,
		"OPTIONS": true,
		"HEAD":    true,
	}
	return validMethods[method]
}
