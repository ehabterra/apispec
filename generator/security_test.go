package generator

import (
	"path/filepath"
	"testing"

	"github.com/ehabterra/apispec/spec"
)

// TestGenerateFromDirectory_ChiGroupAuth is the phase-4 end-to-end check: a
// chi `Use(authMiddleware)` inside a Group(func(rg){...}) closure must protect
// only the routes mounted in that group, not sibling routes mounted on the
// outer router. complex_chi_router mounts /auth on the outer router and /user
// inside an authMiddleware-guarded group, both under /api.
func TestGenerateFromDirectory_ChiGroupAuth(t *testing.T) {
	dir := filepath.Join("..", "testdata", "complex_chi_router")

	cfg := spec.DefaultChiConfig()
	// Detect any chi `Use(mw...)` as router-scope middleware.
	cfg.Framework.SecurityPatterns = []spec.SecurityPattern{
		{
			CallRegex:          "^Use$",
			Scope:              spec.SecurityScopeRouter,
			MiddlewareArgIndex: 0,
			MiddlewareVariadic: true,
			RecvTypeRegex:      "chi",
		},
	}
	// Map the project's authMiddleware to a bearer scheme.
	cfg.SecurityMappings = []spec.SecurityMapping{
		{
			FunctionNameRegex: "^authMiddleware$",
			Schemes:           []spec.SecurityRequirement{{"bearerAuth": {}}},
		},
	}
	cfg.SecuritySchemes = map[string]spec.SecurityScheme{
		"bearerAuth": {Type: "http", Scheme: "bearer", BearerFormat: "JWT"},
	}

	out, err := NewGenerator(cfg).GenerateFromDirectory(dir)
	if err != nil {
		t.Fatalf("GenerateFromDirectory(%s): %v", dir, err)
	}
	if out == nil || out.Paths == nil {
		t.Fatal("nil spec or paths")
	}

	hasBearer := func(reqs *[]spec.SecurityRequirement) bool {
		if reqs == nil {
			return false
		}
		for _, r := range *reqs {
			if _, ok := r["bearerAuth"]; ok {
				return true
			}
		}
		return false
	}

	// Protected: routes inside the authMiddleware group.
	if item, ok := out.Paths["/api/user/{id}"]; !ok || item.Put == nil {
		t.Fatalf("PUT /api/user/{id} missing; paths=%v", mapPathKeys(out.Paths))
	} else if !hasBearer(item.Put.Security) {
		t.Errorf("PUT /api/user/{id}: expected bearerAuth, got security=%v", item.Put.Security)
	}
	if item, ok := out.Paths["/api/user/search"]; !ok || item.Get == nil {
		t.Fatalf("GET /api/user/search missing; paths=%v", mapPathKeys(out.Paths))
	} else if !hasBearer(item.Get.Security) {
		t.Errorf("GET /api/user/search: expected bearerAuth, got security=%v", item.Get.Security)
	}

	// Unprotected: a route mounted on the outer router, outside the group.
	if item, ok := out.Paths["/api/auth/login"]; !ok || item.Post == nil {
		t.Fatalf("POST /api/auth/login missing; paths=%v", mapPathKeys(out.Paths))
	} else if item.Post.Security != nil {
		t.Errorf("POST /api/auth/login: expected no security (outside the auth group), got %v", *item.Post.Security)
	}
}

// TestGenerateFromDirectory_EchoVarWrapperAuth covers the
// golang-echo-realworld-example-app pattern: a custom middleware constructor is
// stored in a local variable and then passed to an echo Group
// (mw := authMiddleware(secret); e.Group("/user", mw)). Resolving it needs both
// (1) tracing the `mw` variable back to authMiddleware via the caller's
// AssignmentMap and (2) looking through authMiddleware's body to the golang-jwt
// Parse call. The inline form (Group("/x", authMiddleware(secret))) must resolve
// the same way, and a route registered outside any group must stay open.
func TestGenerateFromDirectory_EchoVarWrapperAuth(t *testing.T) {
	dir := filepath.Join("..", "testdata", "auth_echo_var_wrapper")

	cfg := spec.DefaultEchoConfig()
	// Map the golang-jwt validation call reached via look-through to bearer.
	cfg.SecurityMappings = []spec.SecurityMapping{
		{
			FunctionNameRegex: "^Parse",
			PkgRegex:          `github\.com/golang-jwt/.*`,
			Schemes:           []spec.SecurityRequirement{{"bearerAuth": {}}},
		},
	}
	cfg.SecuritySchemes = map[string]spec.SecurityScheme{
		"bearerAuth": {Type: "http", Scheme: "bearer", BearerFormat: "JWT"},
	}

	out, err := NewGenerator(cfg).GenerateFromDirectory(dir)
	if err != nil {
		t.Fatalf("GenerateFromDirectory(%s): %v", dir, err)
	}
	if out == nil || out.Paths == nil {
		t.Fatal("nil spec or paths")
	}

	hasBearer := func(reqs *[]spec.SecurityRequirement) bool {
		if reqs == nil {
			return false
		}
		for _, r := range *reqs {
			if _, ok := r["bearerAuth"]; ok {
				return true
			}
		}
		return false
	}

	// Protected via a variable-assigned middleware.
	if item, ok := out.Paths["/user/"]; !ok || item.Get == nil {
		t.Fatalf("GET /user/ missing; paths=%v", mapPathKeys(out.Paths))
	} else if !hasBearer(item.Get.Security) {
		t.Errorf("GET /user/: expected bearerAuth (variable middleware), got security=%v", item.Get.Security)
	}

	// Protected via the inline constructor form.
	if item, ok := out.Paths["/profiles/{name}"]; !ok || item.Get == nil {
		t.Fatalf("GET /profiles/{name} missing; paths=%v", mapPathKeys(out.Paths))
	} else if !hasBearer(item.Get.Security) {
		t.Errorf("GET /profiles/{name}: expected bearerAuth (inline middleware), got security=%v", item.Get.Security)
	}

	// Open: registered directly on the engine, outside any guarded group.
	if item, ok := out.Paths["/health"]; !ok || item.Get == nil {
		t.Fatalf("GET /health missing; paths=%v", mapPathKeys(out.Paths))
	} else if item.Get.Security != nil {
		t.Errorf("GET /health: expected no security, got %v", *item.Get.Security)
	}
}
