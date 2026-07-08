package generator

import (
	"os"
	"path/filepath"
	"testing"

	intspec "github.com/ehabterra/apispec/internal/spec"
	"github.com/ehabterra/apispec/spec"
)

// loadTestdataWithFixtureConfig generates the spec for a fixture using its
// committed used-config.yaml when one exists (the exact config
// scripts/compare-spec.sh feeds the CLI), falling back to the supplied default
// framework config otherwise. This keeps these smoke tests faithful to the
// snapshots they mirror.
func loadTestdataWithFixtureConfig(t *testing.T, name string, fallback *spec.APISpecConfig) *spec.OpenAPISpec {
	t.Helper()
	dir := filepath.Join("..", "testdata", name)

	cfg := fallback
	cfgPath := filepath.Join(dir, "used-config.yaml")
	if _, err := os.Stat(cfgPath); err == nil {
		loaded, lerr := spec.LoadAPISpecConfig(cfgPath)
		if lerr != nil {
			t.Fatalf("LoadAPISpecConfig(%s): %v", cfgPath, lerr)
		}
		cfg = loaded
	}

	out, err := NewGenerator(cfg).GenerateFromDirectory(dir)
	if err != nil {
		t.Fatalf("GenerateFromDirectory(%s): %v", dir, err)
	}
	if out == nil || out.Paths == nil {
		t.Fatalf("nil spec or paths for %s", name)
	}
	return out
}

// TestTestdata_Frameworks is a structural smoke test over the top-level
// per-framework fixtures that previously had no automated coverage — they were
// only reachable through the manual scripts/compare-spec.sh flow. Each fixture's
// expected routes mirror its committed openapi-7.53.yaml snapshot. The
// assertions are deliberately structural (paths present, methods present, no
// dangling $refs, no unresolved-type placeholders) so ordinary schema evolution
// doesn't churn them, but a whole framework silently dropping routes fails loud.
func TestTestdata_Frameworks(t *testing.T) {
	type route struct {
		path    string
		methods []string
	}
	cases := []struct {
		name     string
		fallback *spec.APISpecConfig
		routes   []route
	}{
		{
			name:     "chi",
			fallback: spec.DefaultChiConfig(),
			routes: []route{
				{"/payment/payment/process", []string{"POST"}},
				{"/payment/stripe/pk", []string{"GET"}},
				{"/products/", []string{"GET", "POST"}},
				{"/products/{id}", []string{"GET"}},
				{"/users/", []string{"GET", "POST"}},
				{"/users/{id}", []string{"GET"}},
			},
		},
		{
			name:     "fiber",
			fallback: spec.DefaultFiberConfig(),
			routes: []route{
				{"/api/info", []string{"GET"}},
				{"/health", []string{"GET"}},
				{"/products/", []string{"GET", "POST"}},
				{"/products/{id}", []string{"GET"}},
				{"/users/", []string{"GET", "POST"}},
				{"/users/{id}", []string{"GET", "PUT", "DELETE"}},
			},
		},
		{
			name:     "gin",
			fallback: spec.DefaultGinConfig(),
			routes: []route{
				{"/users/", []string{"GET", "POST"}},
				{"/users/{id}", []string{"GET", "PUT", "DELETE"}},
			},
		},
		{
			name:     "mux",
			fallback: spec.DefaultMuxConfig(),
			routes: []route{
				{"/api/v1/health", []string{"GET"}},
				{"/users", []string{"GET", "POST"}},
				{"/users/{id}", []string{"GET", "PUT", "DELETE"}},
			},
		},
		{
			name:     "generic",
			fallback: spec.DefaultHTTPConfig(),
			routes: []route{
				{"/api/email/send", []string{"POST"}},
				{"/api/users", []string{"POST"}},
				{"/api/users/list", []string{"POST"}},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := loadTestdataWithFixtureConfig(t, tc.name, tc.fallback)
			noDanglingRefs(t, out)
			noUnresolvedPlaceholders(t, out)

			for _, r := range tc.routes {
				item, ok := out.Paths[r.path]
				if !ok {
					t.Errorf("path %q missing; have %v", r.path, mapPathKeys(out.Paths))
					continue
				}
				for _, m := range r.methods {
					if opFor(item, m) == nil {
						t.Errorf("%s %s: expected %s operation, missing", m, r.path, m)
					}
				}
			}
		})
	}
}

// TestTestdata_MuxPathParams locks in gorilla/mux path-parameter wiring.
// Mux exposes path vars as a map (`mux.Vars(r)["id"]`), so the parameter name
// is a map key rather than a call argument. This previously produced a bogus
// `net/http.Request` parameter (the Vars call's request arg misread as a name)
// and left the real `{id}` flagged "present in path but not found in the code".
// After the fix every /users/{id} operation must carry exactly one clean path
// parameter named `id` (string, required) with no warning and no bogus entry.
func TestTestdata_MuxPathParams(t *testing.T) {
	out := loadTestdataWithFixtureConfig(t, "mux", spec.DefaultMuxConfig())

	item, ok := out.Paths["/users/{id}"]
	if !ok {
		t.Fatalf("/users/{id} missing; have %v", mapPathKeys(out.Paths))
	}

	for _, tc := range []struct {
		method string
		op     *intspec.Operation
	}{
		{"GET", item.Get},
		{"PUT", item.Put},
		{"DELETE", item.Delete},
	} {
		if tc.op == nil {
			t.Errorf("%s /users/{id} missing", tc.method)
			continue
		}
		pathParams := make([]intspec.Parameter, 0, len(tc.op.Parameters))
		for _, p := range tc.op.Parameters {
			if p.In == "path" {
				pathParams = append(pathParams, p)
			}
		}
		if len(pathParams) != 1 {
			t.Errorf("%s /users/{id}: want exactly 1 path param, got %d: %+v", tc.method, len(pathParams), pathParams)
			continue
		}
		p := pathParams[0]
		if p.Name != "id" {
			t.Errorf("%s /users/{id}: path param name = %q, want \"id\" (no bogus request param)", tc.method, p.Name)
		}
		if !p.Required {
			t.Errorf("%s /users/{id}: path param should be required", tc.method)
		}
		if p.Schema == nil || p.Schema.Type != "string" {
			t.Errorf("%s /users/{id}: path param schema = %+v, want {type: string}", tc.method, p.Schema)
		}
		if _, warned := p.Extensions["x-warning"]; warned {
			t.Errorf("%s /users/{id}: path param carries an x-warning; the handler reads it via mux.Vars so it should be clean", tc.method)
		}
	}
}
