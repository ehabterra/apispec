package spec

import (
	"testing"

	"github.com/ehabterra/apispec/internal/metadata"
)

// metaWithImports builds a minimal Metadata whose single file imports the given
// package paths, for exercising the import-based security detector.
func metaWithImports(paths ...string) *metadata.Metadata {
	meta := &metadata.Metadata{StringPool: metadata.NewStringPool()}
	imports := make(map[int]int, len(paths))
	for i, p := range paths {
		imports[i] = meta.StringPool.Get(p)
	}
	meta.Packages = map[string]*metadata.Package{
		"app": {Files: map[string]*metadata.File{"main.go": {Imports: imports}}},
	}
	return meta
}

func TestApplySecurityPresets(t *testing.T) {
	t.Run("echo middleware import merges JWT/basic/apikey mappings", func(t *testing.T) {
		cfg := &APISpecConfig{}
		ApplySecurityPresets(cfg, metaWithImports("github.com/labstack/echo/v4/middleware"))

		if len(cfg.SecurityMappings) == 0 {
			t.Fatal("expected echo bundle mappings to be merged")
		}
		// The bundle should resolve echo's middleware.JWT to bearerAuth.
		reqs, _, _ := resolveSecurity(
			[]MiddlewareRef{{FunctionName: "JWT", Pkg: "github.com/labstack/echo/v4/middleware"}},
			cfg.SecurityMappings,
		)
		if !reqHasScheme(reqs, "bearerAuth") {
			t.Errorf("echo middleware.JWT did not resolve to bearerAuth: %+v", reqs)
		}
		if _, ok := cfg.presetSchemes["bearerAuth"]; !ok {
			t.Errorf("bearerAuth preset scheme not registered; got %v", cfg.presetSchemes)
		}
	})

	t.Run("no auth imports merges nothing", func(t *testing.T) {
		cfg := &APISpecConfig{}
		ApplySecurityPresets(cfg, metaWithImports("net/http", "github.com/go-chi/chi/v5"))
		if len(cfg.SecurityMappings) != 0 {
			t.Errorf("expected no mappings for non-auth imports, got %+v", cfg.SecurityMappings)
		}
		if len(cfg.presetSchemes) != 0 {
			t.Errorf("expected no preset schemes, got %v", cfg.presetSchemes)
		}
	})

	t.Run("user mapping is placed before presets and contributes", func(t *testing.T) {
		cfg := &APISpecConfig{
			SecurityMappings: []SecurityMapping{
				{FunctionNameRegex: "^JWT$", PkgRegex: "labstack/echo", Schemes: []SecurityRequirement{{"myScheme": {}}}},
			},
		}
		ApplySecurityPresets(cfg, metaWithImports("github.com/labstack/echo/v4/middleware"))
		// User mappings must come before the appended presets.
		if len(cfg.SecurityMappings) == 0 || len(cfg.SecurityMappings[0].Schemes) == 0 {
			t.Fatalf("expected user mapping first; got %+v", cfg.SecurityMappings)
		}
		if _, ok := cfg.SecurityMappings[0].Schemes[0]["myScheme"]; !ok {
			t.Errorf("user mapping is not first: %+v", cfg.SecurityMappings[0])
		}
		// resolveSecurity merges every match, so when the same middleware matches
		// both the user mapping and a preset, both schemes are present.
		reqs, _, _ := resolveSecurity(
			[]MiddlewareRef{{FunctionName: "JWT", Pkg: "github.com/labstack/echo/v4/middleware"}},
			cfg.SecurityMappings,
		)
		if !reqHasScheme(reqs, "myScheme") {
			t.Errorf("user mapping did not contribute myScheme: %+v", reqs)
		}
		if !reqHasScheme(reqs, "bearerAuth") {
			t.Errorf("preset mapping did not contribute bearerAuth: %+v", reqs)
		}
	})

	t.Run("echo middleware skip preset silences non-auth middleware", func(t *testing.T) {
		cfg := &APISpecConfig{}
		ApplySecurityPresets(cfg, metaWithImports("github.com/labstack/echo/v4/middleware"))
		pkg := "github.com/labstack/echo/v4/middleware"
		// Logger/Recover/CORS are resolved (matched skip) — not unresolved, no scheme.
		reqs, pub, unresolved := resolveSecurity([]MiddlewareRef{
			{FunctionName: "Logger", Pkg: pkg},
			{FunctionName: "Recover", Pkg: pkg},
			{FunctionName: "CORSWithConfig", Pkg: pkg},
		}, cfg.SecurityMappings)
		if pub || len(reqs) != 0 || len(unresolved) != 0 {
			t.Fatalf("skip preset should resolve to nothing: reqs=%+v pub=%v unresolved=%+v", reqs, pub, unresolved)
		}
		// JWT is excluded from the skip list and still resolves to a scheme.
		jwt, _, jwtUnresolved := resolveSecurity(
			[]MiddlewareRef{{FunctionName: "JWT", Pkg: pkg}}, cfg.SecurityMappings)
		if !reqHasScheme(jwt, "bearerAuth") || len(jwtUnresolved) != 0 {
			t.Errorf("JWT must still resolve to bearerAuth: %+v unresolved=%+v", jwt, jwtUnresolved)
		}
	})

	t.Run("fiber jwt contrib import", func(t *testing.T) {
		cfg := &APISpecConfig{}
		ApplySecurityPresets(cfg, metaWithImports("github.com/gofiber/contrib/jwt"))
		reqs, _, _ := resolveSecurity(
			[]MiddlewareRef{{FunctionName: "New", Pkg: "github.com/gofiber/contrib/jwt"}},
			cfg.SecurityMappings,
		)
		if !reqHasScheme(reqs, "bearerAuth") {
			t.Errorf("fiber jwt New did not resolve to bearerAuth: %+v", reqs)
		}
	})
}

// reqHasScheme reports whether any requirement object contains the named scheme
// key. Used instead of len(reqs[0][name]) == 0, which also passes when the key
// is absent (nil slice has length 0).
func reqHasScheme(reqs []SecurityRequirement, name string) bool {
	for _, r := range reqs {
		if _, ok := r[name]; ok {
			return true
		}
	}
	return false
}

func TestReconcileSecuritySchemes(t *testing.T) {
	t.Run("only referenced preset schemes are emitted", func(t *testing.T) {
		cfg := &APISpecConfig{
			presetSchemes: map[string]SecurityScheme{
				"bearerAuth": schemeBearerJWT,
				"basicAuth":  schemeBasic, // available but unused
			},
		}
		routes := []*RouteInfo{
			{Security: []SecurityRequirement{{"bearerAuth": {}}}},
		}
		out := reconcileSecuritySchemes(cfg, routes)
		if _, ok := out["bearerAuth"]; !ok {
			t.Errorf("referenced bearerAuth not emitted: %v", out)
		}
		if _, ok := out["basicAuth"]; ok {
			t.Errorf("unused basicAuth should not be emitted: %v", out)
		}
	})

	t.Run("user-defined schemes always emitted", func(t *testing.T) {
		cfg := &APISpecConfig{
			SecuritySchemes: map[string]SecurityScheme{"apiKeyAuth": schemeAPIKey},
		}
		out := reconcileSecuritySchemes(cfg, nil)
		if _, ok := out["apiKeyAuth"]; !ok {
			t.Errorf("user-defined scheme dropped: %v", out)
		}
	})

	t.Run("global security references are honored", func(t *testing.T) {
		cfg := &APISpecConfig{
			Security:      []SecurityRequirement{{"bearerAuth": {}}},
			presetSchemes: map[string]SecurityScheme{"bearerAuth": schemeBearerJWT},
		}
		out := reconcileSecuritySchemes(cfg, nil)
		if _, ok := out["bearerAuth"]; !ok {
			t.Errorf("globally-referenced preset scheme not emitted: %v", out)
		}
	})
}
