package spec

import "github.com/ehabterra/apispec/internal/metadata"

// This file holds the auth/security presets. Two pure-data layers keep the
// engine framework-agnostic:
//
//  1. Framework SCOPE presets (chiSecurityPatterns, echoSecurityPatterns, ...):
//     how each framework attaches middleware (Use / Group / per-route). They
//     detect middleware and its reach but assign no scheme.
//  2. Library IDENTITY presets (securityLibraryBundles) selected by an import
//     detector (ApplySecurityPresets): map a known auth library's middleware to
//     a security scheme + catalog entry.
//
// Merge order is framework preset -> library presets -> user config, so the
// user's own SecurityMappings always win (they are matched first).

// ---- Framework scope presets -------------------------------------------------

// chiSecurityPatterns: chi attaches middleware via r.Use(mw...) on a Router/Mux,
// including inside Group(func(r){...}) closures.
func chiSecurityPatterns() []SecurityPattern {
	return []SecurityPattern{
		{
			CallRegex:          `^Use$`,
			Scope:              SecurityScopeRouter,
			MiddlewareArgIndex: 0,
			MiddlewareVariadic: true,
			RecvTypeRegex:      `^github\.com/go-chi/chi(/v\d)?\.\*?(Router|Mux)$`,
		},
	}
}

// echoSecurityPatterns: e.Use(mw...) (router), e.Group("/x", mw...) (subtree),
// and e.GET("/x", h, mw...) (per-route, middleware after the handler).
func echoSecurityPatterns() []SecurityPattern {
	recv := `^github\.com/labstack/echo(/v\d)?\.\*(Echo|Group)$`
	return []SecurityPattern{
		{CallRegex: `^Use$`, Scope: SecurityScopeRouter, MiddlewareArgIndex: 0, MiddlewareVariadic: true, RecvTypeRegex: recv},
		{CallRegex: `^Group$`, Scope: SecurityScopeSubtree, MiddlewareArgIndex: 1, MiddlewareVariadic: true, RecvTypeRegex: recv},
		{CallRegex: `^(?i)(GET|POST|PUT|DELETE|PATCH|OPTIONS|HEAD)$`, Scope: SecurityScopeRoute, MiddlewareArgIndex: 2, MiddlewareVariadic: true, RecvTypeRegex: recv},
	}
}

// ginSecurityPatterns: r.Use(mw...) (router) and r.Group("/x", mw...) (subtree).
// Per-route middleware (where the handler is the last variadic arg) is not
// modeled to avoid mistaking the handler for middleware.
func ginSecurityPatterns() []SecurityPattern {
	recv := `^github\.com/gin-gonic/gin\.\*(Engine|RouterGroup)$`
	return []SecurityPattern{
		{CallRegex: `^Use$`, Scope: SecurityScopeRouter, MiddlewareArgIndex: 0, MiddlewareVariadic: true, RecvTypeRegex: recv},
		{CallRegex: `^Group$`, Scope: SecurityScopeSubtree, MiddlewareArgIndex: 1, MiddlewareVariadic: true, RecvTypeRegex: recv},
		// r.GET("/x", mw..., handler): middleware are the args between the path
		// and the final handler arg.
		{CallRegex: `^(?i)(GET|POST|PUT|DELETE|PATCH|OPTIONS|HEAD|Handle)$`, Scope: SecurityScopeRoute, MiddlewareArgIndex: 1, MiddlewareVariadic: true, MiddlewareExcludeLast: true, RecvTypeRegex: recv},
	}
}

// fiberSecurityPatterns: app.Use([path,] mw...) (router) and
// app.Group("/x", mw...) (subtree). Path-string args are ignored because a
// literal does not resolve to a middleware identity.
func fiberSecurityPatterns() []SecurityPattern {
	recv := `^github\.com/gofiber/fiber(/v\d)?\.\*?(App|Router|Group)$`
	return []SecurityPattern{
		{CallRegex: `^Use$`, Scope: SecurityScopeRouter, MiddlewareArgIndex: 0, MiddlewareVariadic: true, RecvTypeRegex: recv},
		{CallRegex: `^Group$`, Scope: SecurityScopeSubtree, MiddlewareArgIndex: 1, MiddlewareVariadic: true, RecvTypeRegex: recv},
		// app.Get("/x", mw..., handler): middleware between the path and the
		// final handler arg.
		{CallRegex: `^(?i)(GET|POST|PUT|DELETE|PATCH|OPTIONS|HEAD|All|Add)$`, Scope: SecurityScopeRoute, MiddlewareArgIndex: 1, MiddlewareVariadic: true, MiddlewareExcludeLast: true, RecvTypeRegex: recv},
	}
}

// muxSecurityPatterns: gorilla/mux uses r.Use(mw...) on a Router (and on
// subrouters), which the router scope covers.
func muxSecurityPatterns() []SecurityPattern {
	return []SecurityPattern{
		{CallRegex: `^Use$`, Scope: SecurityScopeRouter, MiddlewareArgIndex: 0, MiddlewareVariadic: true, RecvTypeRegex: `^github\.com/gorilla/mux\.\*?Router$`},
	}
}

// ---- Library identity presets + import detector ------------------------------

// securityLibraryBundle pairs an import-path detector with the security mappings
// and scheme catalog entries that library contributes.
type securityLibraryBundle struct {
	ImportRegexes []string                  // any match -> bundle applies
	Mappings      []SecurityMapping         // identity -> requirement
	Schemes       map[string]SecurityScheme // catalog entries (added only when referenced)
}

// Common scheme catalog entries reused across bundles.
var (
	schemeBearerJWT = SecurityScheme{Type: "http", Scheme: "bearer", BearerFormat: "JWT"}
	schemeBasic     = SecurityScheme{Type: "http", Scheme: "basic"}
	schemeAPIKey    = SecurityScheme{Type: "apiKey", In: "header", Name: "Authorization"}
)

// securityLibraryBundles returns the built-in library presets. The engine never
// references a specific library directly — it only consumes the merged
// SecurityMappings / SecuritySchemes these bundles produce.
func securityLibraryBundles() []securityLibraryBundle {
	bearer := func(fn, pkg string) SecurityMapping {
		return SecurityMapping{FunctionNameRegex: fn, PkgRegex: pkg, Schemes: []SecurityRequirement{{"bearerAuth": {}}}}
	}
	basic := func(fn, pkg string) SecurityMapping {
		return SecurityMapping{FunctionNameRegex: fn, PkgRegex: pkg, Schemes: []SecurityRequirement{{"basicAuth": {}}}}
	}
	apiKey := func(fn, pkg string) SecurityMapping {
		return SecurityMapping{FunctionNameRegex: fn, PkgRegex: pkg, Schemes: []SecurityRequirement{{"apiKeyAuth": {}}}}
	}

	return []securityLibraryBundle{
		// Echo built-in JWT / BasicAuth / KeyAuth middleware.
		{
			ImportRegexes: []string{`^github\.com/labstack/echo(/v\d+)?/middleware$`},
			Mappings: []SecurityMapping{
				bearer(`^JWT(WithConfig)?$`, `^github\.com/labstack/echo(/v\d+)?/middleware$`),
				basic(`^BasicAuth(WithConfig)?$`, `^github\.com/labstack/echo(/v\d+)?/middleware$`),
				apiKey(`^KeyAuth(WithConfig)?$`, `^github\.com/labstack/echo(/v\d+)?/middleware$`),
			},
			Schemes: map[string]SecurityScheme{"bearerAuth": schemeBearerJWT, "basicAuth": schemeBasic, "apiKeyAuth": schemeAPIKey},
		},
		// echo-jwt module (github.com/labstack/echo-jwt).
		{
			ImportRegexes: []string{`^github\.com/labstack/echo-jwt(/v\d+)?$`},
			Mappings:      []SecurityMapping{bearer(`^(JWT|WithConfig)$`, `^github\.com/labstack/echo-jwt(/v\d+)?$`)},
			Schemes:       map[string]SecurityScheme{"bearerAuth": schemeBearerJWT},
		},
		// gin-jwt (appleboy): authMiddleware.MiddlewareFunc().
		{
			ImportRegexes: []string{`^github\.com/appleboy/gin-jwt(/v\d+)?$`},
			Mappings:      []SecurityMapping{bearer(`^MiddlewareFunc$`, `^github\.com/appleboy/gin-jwt(/v\d+)?$`)},
			Schemes:       map[string]SecurityScheme{"bearerAuth": schemeBearerJWT},
		},
		// gin built-in BasicAuth.
		{
			ImportRegexes: []string{`^github\.com/gin-gonic/gin$`},
			Mappings:      []SecurityMapping{basic(`^BasicAuth(ForRealm)?$`, `^github\.com/gin-gonic/gin$`)},
			Schemes:       map[string]SecurityScheme{"basicAuth": schemeBasic},
		},
		// Fiber JWT (gofiber/contrib/jwt and gofiber/jwt).
		{
			ImportRegexes: []string{`^github\.com/gofiber/(contrib/jwt|jwt(/v\d+)?)$`},
			Mappings:      []SecurityMapping{bearer(`^New$`, `^github\.com/gofiber/(contrib/jwt|jwt(/v\d+)?)$`)},
			Schemes:       map[string]SecurityScheme{"bearerAuth": schemeBearerJWT},
		},
		// Fiber built-in BasicAuth / KeyAuth.
		{
			ImportRegexes: []string{`^github\.com/gofiber/fiber(/v\d+)?/middleware/(basicauth|keyauth)$`},
			Mappings: []SecurityMapping{
				basic(`^New$`, `^github\.com/gofiber/fiber(/v\d+)?/middleware/basicauth$`),
				apiKey(`^New$`, `^github\.com/gofiber/fiber(/v\d+)?/middleware/keyauth$`),
			},
			Schemes: map[string]SecurityScheme{"basicAuth": schemeBasic, "apiKeyAuth": schemeAPIKey},
		},
	}
}

// ApplySecurityPresets merges built-in library presets into cfg based on the
// project's imports (from meta). It only adds mappings/schemes for libraries the
// project actually imports, keeping the engine agnostic and the spec lean.
// User-supplied SecurityMappings keep precedence: presets are appended after
// them and matched in order. Scheme definitions go into cfg.presetSchemes and
// are emitted only when referenced (see the mapper's reconciliation).
func ApplySecurityPresets(cfg *APISpecConfig, meta *metadata.Metadata) {
	if cfg == nil {
		return
	}
	imports := collectImports(meta)
	if len(imports) == 0 {
		return
	}

	for _, bundle := range securityLibraryBundles() {
		if !anyImportMatches(imports, bundle.ImportRegexes) {
			continue
		}
		cfg.SecurityMappings = append(cfg.SecurityMappings, bundle.Mappings...)
		if len(bundle.Schemes) > 0 && cfg.presetSchemes == nil {
			cfg.presetSchemes = make(map[string]SecurityScheme)
		}
		for name, scheme := range bundle.Schemes {
			if _, exists := cfg.presetSchemes[name]; !exists {
				cfg.presetSchemes[name] = scheme
			}
		}
	}
}

// collectImports returns the unique set of import paths referenced anywhere in
// the metadata.
func collectImports(meta *metadata.Metadata) []string {
	if meta == nil || meta.StringPool == nil {
		return nil
	}
	seen := make(map[string]struct{})
	var out []string
	for _, pkg := range meta.Packages {
		for _, file := range pkg.Files {
			for _, pathIdx := range file.Imports {
				p := meta.StringPool.GetString(pathIdx)
				if p == "" {
					continue
				}
				if _, ok := seen[p]; !ok {
					seen[p] = struct{}{}
					out = append(out, p)
				}
			}
		}
	}
	return out
}

// anyImportMatches reports whether any import path matches any of the regexes.
func anyImportMatches(imports, regexes []string) bool {
	for _, expr := range regexes {
		re, err := cachedRegex(expr)
		if err != nil {
			continue
		}
		for _, imp := range imports {
			if re.MatchString(imp) {
				return true
			}
		}
	}
	return false
}
