package spec

import (
	"strings"
	"testing"

	"github.com/ehabterra/apispec/internal/metadata"
)

// TestEchoHandlerFactory_ResolvesClosureBody exercises the "handler factory"
// pattern that is idiomatic in many Echo projects (e.g. the widely-copied
// AleksK1NG clean-architecture layout):
//
//	type Handlers interface { Create() echo.HandlerFunc }
//	func (h *userHandlers) Create() echo.HandlerFunc {
//	    return func(c echo.Context) error {
//	        u := &User{}; c.Bind(u); return c.JSON(201, u)
//	    }
//	}
//	g.POST("/users", h.Create())   // handler arg is a *call* returning HandlerFunc
//
// Here the value registered as the route handler is `h.Create()` — a call
// expression (metadata.KindCall) whose return value is the closure that holds
// the real request/response logic. Resolving the route therefore requires:
//  1. interface dispatch (Handlers.Create -> *userHandlers.Create), and
//  2. descending into the function literal the method returns.
//
// Until that resolution exists, the route maps but its body does not: no
// request schema, no response schema, and components stays empty. This test
// pins the desired end state — the User schema flowing through.
func TestEchoHandlerFactory_ResolvesClosureBody(t *testing.T) {
	meta, err := metadata.LoadMetadata("../../testdata/echo_handler_factory/metadata.yaml")
	if err != nil {
		t.Skipf("fixture unavailable: %v", err)
	}
	meta.BuildCallGraphMaps()

	tree := NewTrackerTree(meta, metadata.TrackerLimits{
		MaxNodesPerTree: 50000, MaxChildrenPerNode: 500, MaxArgsPerFunction: 100,
		MaxNestedArgsDepth: 100, MaxRecursionDepth: 1000,
	}, nil)

	spec, err := MapMetadataToOpenAPI(tree, DefaultEchoConfig(), GeneratorConfig{
		OpenAPIVersion: "3.0.3", Title: "factory", APIVersion: "1.0.0",
	})
	if err != nil {
		t.Fatalf("MapMetadataToOpenAPI: %v", err)
	}

	const path = "/api/v1/users"
	pi, ok := spec.Paths[path]
	if !ok {
		t.Fatalf("route %s not mapped; paths=%v", path, keysOf(spec.Paths))
	}
	if pi.Post == nil {
		t.Fatalf("POST %s missing", path)
	}

	// Response body: c.JSON(http.StatusCreated, u) -> 201 with the User schema.
	if !operationReferencesUser(t, pi.Post, "response") {
		t.Errorf("POST %s: response body did not resolve to the User schema (closure body not traced)", path)
	}

	// Request body: c.Bind(u) -> requestBody with the User schema.
	if pi.Post.RequestBody == nil {
		t.Errorf("POST %s: no request body (c.Bind inside the closure not traced)", path)
	} else if !mediaReferencesUser(pi.Post.RequestBody.Content) {
		t.Errorf("POST %s: request body did not resolve to the User schema", path)
	}

	// The User type must end up registered as a component schema.
	if spec.Components.Schemas == nil || !hasUserSchema(spec.Components.Schemas) {
		t.Errorf("components.schemas missing User; got %v", keysOfSchemas(spec.Components.Schemas))
	}
}

// TestHandlerFactory_FunctionLocalRequestType checks that a request bound to a
// type declared *inside* the handler method (e.g. `type Login struct{…}`) is
// captured and emitted as a real component schema with fields — not left as a
// dangling $ref to an undefined ("unresolved") type.
func TestHandlerFactory_FunctionLocalRequestType(t *testing.T) {
	meta, err := metadata.LoadMetadata("../../testdata/echo_handler_factory/metadata.yaml")
	if err != nil {
		t.Skipf("fixture unavailable: %v", err)
	}
	meta.BuildCallGraphMaps()
	tree := NewTrackerTree(meta, metadata.TrackerLimits{
		MaxNodesPerTree: 50000, MaxChildrenPerNode: 500, MaxArgsPerFunction: 100,
		MaxNestedArgsDepth: 100, MaxRecursionDepth: 1000,
	}, nil)
	spec, err := MapMetadataToOpenAPI(tree, DefaultEchoConfig(), GeneratorConfig{
		OpenAPIVersion: "3.0.3", Title: "factory", APIVersion: "1.0.0",
	})
	if err != nil {
		t.Fatalf("MapMetadataToOpenAPI: %v", err)
	}

	pi, ok := spec.Paths["/api/v1/login"]
	if !ok || pi.Post == nil || pi.Post.RequestBody == nil {
		t.Fatalf("POST /api/v1/login or its request body missing; paths=%v", keysOf(spec.Paths))
	}
	ref := ""
	for _, mt := range pi.Post.RequestBody.Content {
		if mt.Schema != nil && mt.Schema.Ref != "" {
			ref = mt.Schema.Ref
		}
	}
	if ref == "" {
		t.Fatalf("login request body is not a $ref to the local type; content=%+v", pi.Post.RequestBody.Content)
	}
	// The referenced component must actually be defined, with the local fields.
	name := strings.TrimPrefix(ref, "#/components/schemas/")
	sc := spec.Components.Schemas[name]
	if sc == nil {
		t.Fatalf("dangling $ref: %q is referenced but not defined in components.schemas", name)
	}
	if _, ok := sc.Properties["email"]; !ok {
		t.Errorf("local Login schema missing field 'email'; got props %v", propNames(sc))
	}
	if _, ok := sc.Properties["password"]; !ok {
		t.Errorf("local Login schema missing field 'password'; got props %v", propNames(sc))
	}
}

func propNames(s *Schema) []string {
	out := make([]string, 0, len(s.Properties))
	for k := range s.Properties {
		out = append(out, k)
	}
	return out
}

// --- helpers ---

func keysOf(m map[string]PathItem) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func keysOfSchemas(m map[string]*Schema) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func hasUserSchema(m map[string]*Schema) bool {
	for k := range m {
		if strings.HasSuffix(k, "User") || strings.Contains(k, "User") {
			return true
		}
	}
	return false
}

func mediaReferencesUser(content map[string]MediaType) bool {
	for _, mt := range content {
		if schemaReferencesUser(mt.Schema) {
			return true
		}
	}
	return false
}

func operationReferencesUser(t *testing.T, op *Operation, _ string) bool {
	t.Helper()
	for _, resp := range op.Responses {
		if mediaReferencesUser(resp.Content) {
			return true
		}
	}
	return false
}

func schemaReferencesUser(s *Schema) bool {
	if s == nil {
		return false
	}
	if strings.Contains(s.Ref, "User") {
		return true
	}
	if s.Items != nil && schemaReferencesUser(s.Items) {
		return true
	}
	// A concrete (non-empty) object with the User fields also counts.
	if _, ok := s.Properties["email"]; ok {
		return true
	}
	return false
}
