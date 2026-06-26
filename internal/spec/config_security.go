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
	recv := `^github\.com/go-chi/chi(/v\d)?\.\*?(Router|Mux)$`
	return []SecurityPattern{
		{CallRegex: `^Use$`, Scope: SecurityScopeRouter, MiddlewareArgIndex: 0, MiddlewareVariadic: true, RecvTypeRegex: recv},
		// r.With(mw...).Get(...): With returns an inline router carrying the
		// middleware; it is resolved via the route's chain parent, so it guards
		// only the chained route (route scope).
		{CallRegex: `^With$`, Scope: SecurityScopeRoute, MiddlewareArgIndex: 0, MiddlewareVariadic: true, RecvTypeRegex: recv},
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
// subrouters) — router scope — and also handler-wrapping via
// r.Handle("/x", auth(h)) — wrapper scope.
func muxSecurityPatterns() []SecurityPattern {
	return []SecurityPattern{
		{CallRegex: `^Use$`, Scope: SecurityScopeRouter, MiddlewareArgIndex: 0, MiddlewareVariadic: true, RecvTypeRegex: `^github\.com/gorilla/mux\.\*?Router$`},
		{CallRegex: `^(Handle|HandleFunc)$`, Scope: SecurityScopeWrapper, HandlerArgIndex: 1, RecvTypeRegex: `^github\.com/gorilla/mux\.\*?Router$`},
	}
}

// httpSecurityPatterns: net/http has no dedicated middleware slot — auth is
// applied by wrapping the handler: mux.Handle("/x", auth(h)) or
// http.Handle("/x", auth(h)). This is the wrapper scope; whether the wrapping
// call is really auth is decided by look-through (does its body call a known
// auth library?), so handler factories like newUserHandler() are not misread as
// middleware.
func httpSecurityPatterns() []SecurityPattern {
	recv := `^net/http(\.\*ServeMux)?$`
	return []SecurityPattern{
		{CallRegex: `^Handle$`, Scope: SecurityScopeWrapper, HandlerArgIndex: 1, RecvTypeRegex: recv},
		{CallRegex: `^HandleFunc$`, Scope: SecurityScopeWrapper, HandlerArgIndex: 1, RecvTypeRegex: recv},
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
		// JWT token libraries (golang-jwt, dgrijalva). These are not middleware
		// themselves but are called inside custom auth middleware; wrapper
		// look-through finds them. Only token *validation* functions are mapped
		// (Parse*), never issuance (New/SignedString), so a login handler that
		// mints a token is not mistaken for a protected route.
		{
			ImportRegexes: []string{`^github\.com/golang-jwt/jwt(/v\d+)?$`, `^github\.com/dgrijalva/jwt-go$`},
			Mappings: []SecurityMapping{
				bearer(`^Parse(WithClaims|FromRequest|FromRequestWithClaims)?$`, `^github\.com/(golang-jwt/jwt(/v\d+)?|dgrijalva/jwt-go)$`),
			},
			Schemes: map[string]SecurityScheme{"bearerAuth": schemeBearerJWT},
		},
		// auth0 net/http JWT middleware.
		{
			ImportRegexes: []string{`^github\.com/auth0/go-jwt-middleware(/v\d+)?$`},
			Mappings:      []SecurityMapping{bearer(`^New$`, `^github\.com/auth0/go-jwt-middleware(/v\d+)?$`)},
			Schemes:       map[string]SecurityScheme{"bearerAuth": schemeBearerJWT},
		},
	}
}

// ---- Non-auth middleware skip presets ----------------------------------------

// securitySkipBundles returns import-gated mappings that mark well-known
// NON-auth middleware (logging, recovery, CORS, compression, request-id, …) as
// skip:true, so they are not reported as unresolved when they share a router's
// Use/Group slot with real auth middleware. Each bundle targets a framework's
// own middleware package and deliberately excludes that package's auth
// middleware (JWT/BasicAuth/KeyAuth), which the library bundles map to schemes.
func securitySkipBundles() []securityLibraryBundle {
	skip := func(fn, pkg string) SecurityMapping {
		return SecurityMapping{FunctionNameRegex: fn, PkgRegex: pkg, Skip: true}
	}
	return []securityLibraryBundle{
		// chi middleware (BasicAuth excluded).
		{
			ImportRegexes: []string{`^github\.com/go-chi/chi(/v\d+)?/middleware$`},
			Mappings: []SecurityMapping{skip(
				`^(Logger|Recoverer|RequestID|RealIP|Compress|Timeout|Heartbeat|StripSlashes|RedirectSlashes|NoCache|GetHead|CleanPath|AllowContentType|AllowContentEncoding|SetHeader|Throttle|ThrottleBacklog|Sunset|ContentCharset|URLFormat|RouteHeaders|Profiler|WithValue)$`,
				`^github\.com/go-chi/chi(/v\d+)?/middleware$`)},
		},
		// echo middleware (JWT/BasicAuth/KeyAuth excluded).
		{
			ImportRegexes: []string{`^github\.com/labstack/echo(/v\d+)?/middleware$`},
			Mappings: []SecurityMapping{skip(
				`^(Logger|Recover|CORS|Gzip|RequestID|Secure|BodyLimit|BodyDump|RateLimiter|Decompress|Timeout|CSRF|Static|Rewrite|AddTrailingSlash|RemoveTrailingSlash|MethodOverride|ContextTimeout|Proxy|RequestLogger)(WithConfig)?$`,
				`^github\.com/labstack/echo(/v\d+)?/middleware$`)},
		},
		// gin built-in + gin-contrib (BasicAuth excluded).
		{
			ImportRegexes: []string{`^github\.com/gin-gonic/gin$`, `^github\.com/gin-contrib/.*$`},
			Mappings: []SecurityMapping{
				skip(`^(Logger|LoggerWithConfig|LoggerWithFormatter|LoggerWithWriter|Recovery|RecoveryWithWriter|CustomRecovery|ErrorLogger)$`, `^github\.com/gin-gonic/gin$`),
				skip(`^(New|Default)$`, `^github\.com/gin-contrib/.*$`),
			},
		},
		// fiber middleware packages (basicauth/keyauth/jwt excluded by path).
		{
			ImportRegexes: []string{`^github\.com/gofiber/fiber(/v\d+)?/middleware/(logger|recover|cors|compress|requestid|limiter|etag|favicon|helmet|csrf|cache|pprof|timeout|encryptcookie|earlydata|idempotency|skip)$`},
			Mappings: []SecurityMapping{skip(`^New$`,
				`^github\.com/gofiber/fiber(/v\d+)?/middleware/(logger|recover|cors|compress|requestid|limiter|etag|favicon|helmet|csrf|cache|pprof|timeout|encryptcookie|earlydata|idempotency|skip)$`)},
		},
		// gorilla handlers (utility middleware; no auth in this package).
		{
			ImportRegexes: []string{`^github\.com/gorilla/handlers$`},
			Mappings: []SecurityMapping{skip(
				`^(CORS|LoggingHandler|CombinedLoggingHandler|CompressHandler|CompressHandlerLevel|RecoveryHandler|ProxyHeaders|ContentTypeHandler|HTTPMethodOverrideHandler)$`,
				`^github\.com/gorilla/handlers$`)},
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
	if cfg == nil || cfg.presetsApplied {
		return
	}
	imports := collectImports(meta)
	if len(imports) == 0 {
		// No imports yet (e.g. called before metadata is ready) — don't latch
		// presetsApplied, so a later run with real imports can still apply.
		return
	}

	// Library identity bundles first, then non-auth skip bundles. Both are
	// appended after any user mappings (which keep precedence). Skip bundles add
	// no schemes.
	bundles := append(securityLibraryBundles(), securitySkipBundles()...)
	for _, bundle := range bundles {
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
	// Latch only once imports have been seen, so reuse doesn't duplicate the
	// merged preset mappings.
	cfg.presetsApplied = true
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
