package generator

import (
	"path/filepath"
	"testing"

	intspec "github.com/ehabterra/apispec/internal/spec"
	"github.com/ehabterra/apispec/spec"
)

// TestGenerateFromDirectory_ServeMuxMethodRouting verifies Go 1.22
// net/http.ServeMux method-aware routing: the HTTP method is carried on the
// registration pattern ("GET /users/{id}"), path wildcards become path
// parameters, and r.PathValue("id") is recognised as a path parameter.
func TestGenerateFromDirectory_ServeMuxMethodRouting(t *testing.T) {
	dir := filepath.Join("..", "testdata", "servemux")

	gen := NewGenerator(spec.DefaultHTTPConfig())
	out, err := gen.GenerateFromDirectory(dir)
	if err != nil {
		t.Fatalf("GenerateFromDirectory(%s) failed: %v", dir, err)
	}
	if out == nil || out.Paths == nil {
		t.Fatal("nil spec or paths")
	}

	op := func(path, method string) *intspec.Operation {
		t.Helper()
		item, ok := out.Paths[path]
		if !ok {
			t.Fatalf("path %q missing; paths=%v", path, mapPathKeys(out.Paths))
		}
		switch method {
		case "GET":
			return item.Get
		case "POST":
			return item.Post
		default:
			t.Fatalf("unhandled method %q", method)
			return nil
		}
	}

	// GET /users/{id} — method parsed from the pattern, {id} wildcard.
	getUser := op("/users/{id}", "GET")
	if getUser == nil {
		item := out.Paths["/users/{id}"]
		t.Fatalf("GET /users/{id} missing; the method was not parsed from the pattern. item=%+v", item)
	}
	var hasID bool
	for _, p := range getUser.Parameters {
		if p.In == "path" && p.Name == "id" {
			hasID = true
			// The param must come from r.PathValue("id"), not the
			// ensureAllPathParams fallback (which tags an x-warning).
			if _, warned := p.Extensions["x-warning"]; warned {
				t.Errorf("GET /users/{id}: \"id\" param was synthesized as a fallback, not detected from r.PathValue; ext=%+v", p.Extensions)
			}
		}
	}
	if !hasID {
		t.Errorf("GET /users/{id}: missing path parameter \"id\"; params=%+v", getUser.Parameters)
	}

	// POST /users — method parsed from the pattern, JSON request body bound.
	createUser := op("/users", "POST")
	if createUser == nil {
		t.Fatal("POST /users missing; the method was not parsed from the pattern")
	}
	if createUser.RequestBody == nil {
		t.Errorf("POST /users: expected a request body from json.Decode(&req)")
	}

	// GET /health — plain method-prefixed route, no params.
	if health := op("/health", "GET"); health == nil {
		t.Fatal("GET /health missing")
	}

	// Regression guard: the method prefix must not leak into the path key.
	for path := range out.Paths {
		if path == "GET /users/{id}" || path == "POST /users" || path == "GET /health" {
			t.Errorf("method prefix leaked into path key: %q", path)
		}
	}
}
