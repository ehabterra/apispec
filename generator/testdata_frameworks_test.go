package generator

import (
	"os"
	"path/filepath"
	"strings"
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
		{
			name:     "generic_structs",
			fallback: spec.DefaultHTTPConfig(),
			routes: []route{
				{"/users", []string{"POST"}},
				{"/products", []string{"POST"}},
				{"/user", []string{"POST"}},
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

// TestTestdata_MuxAdvancedPathParams locks in the harder gorilla/mux path-param
// cases beyond the direct `mux.Vars(r)["id"]` idiom:
//   - /products/{sku:[a-z0-9-]+}: the regex constraint is stripped from the
//     OpenAPI path ({sku}) and surfaced as a schema pattern; param is clean.
//   - /orders/{id}: the handler reads the var through a helper (pathVar wraps
//     mux.Vars), resolved via call-graph reachability; param is clean.
//   - /items/{id}: the handler never reads the var, so it stays warned.
func TestTestdata_MuxAdvancedPathParams(t *testing.T) {
	out := loadTestdataWithFixtureConfig(t, "mux_path_params", spec.DefaultMuxConfig())

	pathParam := func(t *testing.T, path string) *intspec.Parameter {
		t.Helper()
		item, ok := out.Paths[path]
		if !ok {
			t.Fatalf("%s missing; have %v", path, mapPathKeys(out.Paths))
		}
		if item.Get == nil {
			t.Fatalf("GET %s missing", path)
		}
		var found []intspec.Parameter
		for _, p := range item.Get.Parameters {
			if p.In == "path" {
				found = append(found, p)
			}
		}
		if len(found) != 1 {
			t.Fatalf("GET %s: want 1 path param, got %d: %+v", path, len(found), found)
		}
		return &found[0]
	}

	isWarned := func(p *intspec.Parameter) bool {
		_, ok := p.Extensions["x-warning"]
		return ok
	}

	// Regex-constrained param: path normalized, pattern captured, clean.
	if _, ok := out.Paths["/products/{sku:[a-z0-9-]+}"]; ok {
		t.Errorf("regex constraint leaked into the OpenAPI path; want /products/{sku}")
	}
	sku := pathParam(t, "/products/{sku}")
	if sku.Name != "sku" || isWarned(sku) {
		t.Errorf("/products/{sku}: want clean 'sku', got name=%q warned=%v", sku.Name, isWarned(sku))
	}
	if sku.Schema == nil || sku.Schema.Pattern != "[a-z0-9-]+" {
		t.Errorf("/products/{sku}: want schema.pattern=[a-z0-9-]+, got %+v", sku.Schema)
	}

	// Helper indirection: clean via call-graph reachability.
	if id := pathParam(t, "/orders/{id}"); id.Name != "id" || isWarned(id) {
		t.Errorf("/orders/{id}: want clean 'id' (helper-wrapped mux.Vars), got name=%q warned=%v", id.Name, isWarned(id))
	}

	// Unread placeholder: stays warned.
	if id := pathParam(t, "/items/{id}"); !isWarned(id) {
		t.Errorf("/items/{id}: want warned (handler never reads the var), got clean")
	}
}

// TestTestdata_MuxPathParamKeyMismatch checks the map-key diagnostic: the fixture's
// getTag handler reads mux.Vars(r)["tag"] on a /tags/{id} route — a typo the read
// will silently return empty for. Reachability alone can't catch it (the handler
// does reach mux.Vars, so {id} wires clean), but recovering the actual key does.
// Every other route in the fixture reads a key that matches its placeholder, so
// exactly one mismatch must be reported.
func TestTestdata_MuxPathParamKeyMismatch(t *testing.T) {
	dir := filepath.Join("..", "testdata", "mux_path_params")
	g := NewGenerator(spec.DefaultMuxConfig())
	if _, err := g.GenerateFromDirectory(dir); err != nil {
		t.Fatalf("GenerateFromDirectory: %v", err)
	}

	got := g.PathParamMismatches()
	if len(got) != 1 {
		t.Fatalf("want exactly 1 path-param mismatch, got %d: %+v", len(got), got)
	}
	m := got[0]
	if m.Key != "tag" {
		t.Errorf("mismatch key = %q, want \"tag\"", m.Key)
	}
	if m.Path != "/tags/{id}" {
		t.Errorf("mismatch path = %q, want \"/tags/{id}\"", m.Path)
	}
	if m.Method != "GET" {
		t.Errorf("mismatch method = %q, want \"GET\"", m.Method)
	}
	if !strings.HasSuffix(m.Handler, "getTag") {
		t.Errorf("mismatch handler = %q, want it to name getTag", m.Handler)
	}
}
