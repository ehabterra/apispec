package generator

import (
	"os"
	"path/filepath"
	"testing"

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
