package spec

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/ehabterra/apispec/pkg/patterns"
)

const (
	defaultRequestContentType  = "application/json"
	defaultResponseContentType = "application/json"
	defaultResponseStatus      = 200
)

// FrameworkConfig defines framework-specific extraction patterns
type FrameworkConfig struct {
	// Route extraction patterns
	RoutePatterns []RoutePattern `yaml:"routePatterns" json:"routePatterns,omitempty"`

	// Request body extraction patterns
	RequestBodyPatterns []RequestBodyPattern `yaml:"requestBodyPatterns" json:"requestBodyPatterns,omitempty"`

	// Response extraction patterns
	ResponsePatterns []ResponsePattern `yaml:"responsePatterns" json:"responsePatterns,omitempty"`

	// Parameter extraction patterns
	ParamPatterns []ParamPattern `yaml:"paramPatterns" json:"paramPatterns,omitempty"`

	// Mount/subrouter patterns
	MountPatterns []MountPattern `yaml:"mountPatterns" json:"mountPatterns,omitempty"`

	// Security/auth middleware patterns. These recognise middleware-application
	// calls (e.g. r.Use, r.With, Group(mw...), per-route middleware args, or
	// handler-wrapping) and describe the SCOPE over which the middleware
	// applies. The engine stays framework-agnostic: which scheme a given
	// middleware maps to is resolved separately via APISpecConfig.SecurityMappings.
	SecurityPatterns []SecurityPattern `yaml:"securityPatterns" json:"securityPatterns,omitempty"`

	// RequestContext describes how to recognise the request-bearing parameter
	// of a handler and the accessor chain that yields its body. Used to gate
	// generic decoders (json.Decode, json.Unmarshal, render.DecodeJSON, ...)
	// so they are only treated as request-body extraction when the bytes
	// actually originate from an HTTP request.
	RequestContext RequestContextConfig `yaml:"requestContext,omitempty" json:"requestContext,omitempty"`
}

// RequestContextConfig describes the types and accessors that identify an
// HTTP request body source for a framework.
type RequestContextConfig struct {
	// TypeRegexes match the (fully-qualified) types of handler parameters that
	// carry an HTTP request. The leftmost root of a body-source expression
	// must have one of these types for the chain to be considered a body
	// source. Example for net/http: "^\\*?net/http\\.Request$".
	TypeRegexes []string `yaml:"typeRegexes,omitempty" json:"typeRegexes,omitempty"`

	// BodyAccessors are regexes matched against the dot-joined accessor chain
	// applied to a request-context root. The chain is the sequence of
	// selectors/calls between the root ident and the leaf expression, with
	// method calls rendered as "Name()". Examples:
	//   net/http -> "^Body$"
	//   gin      -> "^Request\\.Body$"
	//   echo     -> "^Request\\(\\)\\.Body$"
	//   fiber    -> "^Body\\(\\)$"
	BodyAccessors []string `yaml:"bodyAccessors,omitempty" json:"bodyAccessors,omitempty"`
}

// MethodMapping defines how to extract HTTP methods from function names
type MethodMapping struct {
	Patterns []string `yaml:"patterns,omitempty" json:"patterns,omitempty"` // Function name patterns (e.g., ["get", "list", "show"])
	Method   string   `yaml:"method,omitempty" json:"method,omitempty"`     // HTTP method (e.g., "GET")
	Priority int      `yaml:"priority,omitempty" json:"priority,omitempty"` // Higher priority = checked first
}

// MethodExtractionConfig defines how to extract HTTP methods
type MethodExtractionConfig struct {
	// Method mappings from function names
	MethodMappings []MethodMapping `yaml:"methodMappings,omitempty" json:"methodMappings,omitempty"`

	// Extraction strategy
	UsePrefix     bool `yaml:"usePrefix,omitempty" json:"usePrefix,omitempty"`         // Check for prefix matches (getUser -> GET)
	UseContains   bool `yaml:"useContains,omitempty" json:"useContains,omitempty"`     // Check for contains matches (userGet -> GET)
	CaseSensitive bool `yaml:"caseSensitive,omitempty" json:"caseSensitive,omitempty"` // Case sensitive matching

	// Fallback behavior
	DefaultMethod    string `yaml:"defaultMethod,omitempty" json:"defaultMethod,omitempty"`       // Default method when none found
	InferFromContext bool   `yaml:"inferFromContext,omitempty" json:"inferFromContext,omitempty"` // Try to infer from call context
}

// RoutePattern defines how to extract route information
type RoutePattern struct {
	// Function call patterns to match
	CallRegex         string `yaml:"callRegex,omitempty" json:"callRegex,omitempty"`                 // e.g., '^BindJSON$'
	FunctionNameRegex string `yaml:"functionNameRegex,omitempty" json:"functionNameRegex,omitempty"` // e.g., '.*Handler$'
	RecvType          string `yaml:"recvType,omitempty" json:"recvType,omitempty"`                   // e.g., 'context.Context'
	RecvTypeRegex     string `yaml:"recvTypeRegex,omitempty" json:"recvTypeRegex,omitempty"`         // e.g., '^context\.Context$'

	// Argument extraction hints
	MethodArgIndex  int `yaml:"methodArgIndex,omitempty" json:"methodArgIndex,omitempty"`   // Which arg contains HTTP method
	PathArgIndex    int `yaml:"pathArgIndex,omitempty" json:"pathArgIndex,omitempty"`       // Which arg contains path
	HandlerArgIndex int `yaml:"handlerArgIndex,omitempty" json:"handlerArgIndex,omitempty"` // Which arg contains handler

	// Extraction hints
	MethodFromCall    bool `yaml:"methodFromCall,omitempty" json:"methodFromCall,omitempty"`       // Extract method from function name
	MethodFromHandler bool `yaml:"methodFromHandler,omitempty" json:"methodFromHandler,omitempty"` // Extract method from handler function name
	MethodFromPath    bool `yaml:"methodFromPath,omitempty" json:"methodFromPath,omitempty"`       // Extract method from a leading verb in the path arg (Go 1.22 ServeMux: "GET /users/{id}")
	PathFromArg       bool `yaml:"pathFromArg,omitempty" json:"pathFromArg,omitempty"`             // Extract path from argument
	HandlerFromArg    bool `yaml:"handlerFromArg,omitempty" json:"handlerFromArg,omitempty"`       // Extract handler from argument

	// Method extraction configuration
	MethodExtraction *MethodExtractionConfig `yaml:"methodExtraction,omitempty" json:"methodExtraction,omitempty"`

	// Package/type filtering
	CallerPkgPatterns      []string `yaml:"callerPkgPatterns,omitempty" json:"callerPkgPatterns,omitempty"`
	CallerRecvTypePatterns []string `yaml:"callerRecvTypePatterns,omitempty" json:"callerRecvTypePatterns,omitempty"`
	CalleePkgPatterns      []string `yaml:"calleePkgPatterns,omitempty" json:"calleePkgPatterns,omitempty"`
	CalleeRecvTypePatterns []string `yaml:"calleeRecvTypePatterns,omitempty" json:"calleeRecvTypePatterns,omitempty"`
}

// RequestBodyPattern defines how to extract request body information
type RequestBodyPattern struct {
	// Function call patterns to match
	CallRegex         string `yaml:"callRegex,omitempty" json:"callRegex,omitempty"`
	FunctionNameRegex string `yaml:"functionNameRegex,omitempty" json:"functionNameRegex,omitempty"`
	RecvType          string `yaml:"recvType,omitempty" json:"recvType,omitempty"`
	RecvTypeRegex     string `yaml:"recvTypeRegex,omitempty" json:"recvTypeRegex,omitempty"`

	// Argument extraction hints
	TypeArgIndex int `yaml:"typeArgIndex,omitempty" json:"typeArgIndex,omitempty"` // Which arg contains type info

	// Extraction hints
	TypeFromArg    bool `yaml:"typeFromArg,omitempty" json:"typeFromArg,omitempty"`       // Extract type from argument
	TypeFromReturn bool `yaml:"typeFromReturn,omitempty" json:"typeFromReturn,omitempty"` // Extract type from return value
	Deref          bool `yaml:"deref,omitempty" json:"deref,omitempty"`                   // Dereference pointer types

	// Body-source verification. When RequireRequestSource is true, the
	// matcher only accepts the call if its data source can be traced back to
	// a request-context body accessor (see FrameworkConfig.RequestContext).
	// Use BodyFromReceiver for chained decoders like *json.Decoder.Decode,
	// whose source is the argument given to json.NewDecoder. Otherwise the
	// source is taken from Args[BodySourceArgIndex].
	RequireRequestSource bool `yaml:"requireRequestSource,omitempty" json:"requireRequestSource,omitempty"`
	BodyFromReceiver     bool `yaml:"bodyFromReceiver,omitempty" json:"bodyFromReceiver,omitempty"`
	BodySourceArgIndex   int  `yaml:"bodySourceArgIndex,omitempty" json:"bodySourceArgIndex,omitempty"`

	// Context-aware validation
	AllowForGetMethods bool `yaml:"allowForGetMethods,omitempty" json:"allowForGetMethods,omitempty"` // Allow this pattern for GET/HEAD methods

	// Package/type filtering
	CallerPkgPatterns      []string `yaml:"callerPkgPatterns,omitempty" json:"callerPkgPatterns,omitempty"`
	CallerRecvTypePatterns []string `yaml:"callerRecvTypePatterns,omitempty" json:"callerRecvTypePatterns,omitempty"`
	CalleePkgPatterns      []string `yaml:"calleePkgPatterns,omitempty" json:"calleePkgPatterns,omitempty"`
	CalleeRecvTypePatterns []string `yaml:"calleeRecvTypePatterns,omitempty" json:"calleeRecvTypePatterns,omitempty"`
}

// ResponsePattern defines how to extract response information
type ResponsePattern struct {
	// Function call patterns to match
	CallRegex         string `yaml:"callRegex,omitempty" json:"callRegex,omitempty"`
	FunctionNameRegex string `yaml:"functionNameRegex,omitempty" json:"functionNameRegex,omitempty"`
	RecvType          string `yaml:"recvType,omitempty" json:"recvType,omitempty"`
	RecvTypeRegex     string `yaml:"recvTypeRegex,omitempty" json:"recvTypeRegex,omitempty"`

	// Argument extraction hints
	StatusArgIndex int `yaml:"statusArgIndex,omitempty" json:"statusArgIndex,omitempty"` // Which arg contains status code
	TypeArgIndex   int `yaml:"typeArgIndex,omitempty" json:"typeArgIndex,omitempty"`     // Which arg contains type info

	// Extraction hints
	StatusFromArg bool `yaml:"statusFromArg,omitempty" json:"statusFromArg,omitempty"` // Extract status from argument
	TypeFromArg   bool `yaml:"typeFromArg,omitempty" json:"typeFromArg,omitempty"`     // Extract type from argument
	Deref         bool `yaml:"deref,omitempty" json:"deref,omitempty"`                 // Dereference pointer types
	// DefaultStatus specifies a fallback status code when it can't be extracted from args
	DefaultStatus int `yaml:"defaultStatus,omitempty" json:"defaultStatus,omitempty"`
	// DefaultContentType overrides the config default content type when set
	DefaultContentType string `yaml:"defaultContentType,omitempty" json:"defaultContentType,omitempty"`

	// Package/type filtering
	CallerPkgPatterns      []string `yaml:"callerPkgPatterns,omitempty" json:"callerPkgPatterns,omitempty"`
	CallerRecvTypePatterns []string `yaml:"callerRecvTypePatterns,omitempty" json:"callerRecvTypePatterns,omitempty"`
	CalleePkgPatterns      []string `yaml:"calleePkgPatterns,omitempty" json:"calleePkgPatterns,omitempty"`
	CalleeRecvTypePatterns []string `yaml:"calleeRecvTypePatterns,omitempty" json:"calleeRecvTypePatterns,omitempty"`
}

// ParamPattern defines how to extract parameter information
type ParamPattern struct {
	// Function call patterns to match
	CallRegex         string `yaml:"callRegex,omitempty" json:"callRegex,omitempty"`
	FunctionNameRegex string `yaml:"functionNameRegex,omitempty" json:"functionNameRegex,omitempty"`
	RecvType          string `yaml:"recvType,omitempty" json:"recvType,omitempty"`
	RecvTypeRegex     string `yaml:"recvTypeRegex,omitempty" json:"recvTypeRegex,omitempty"`

	// Parameter location and extraction
	ParamIn       string `yaml:"paramIn,omitempty" json:"paramIn,omitempty"`             // path, query, header, cookie
	ParamArgIndex int    `yaml:"paramArgIndex,omitempty" json:"paramArgIndex,omitempty"` // Which arg contains parameter
	TypeArgIndex  int    `yaml:"typeArgIndex,omitempty" json:"typeArgIndex,omitempty"`   // Which arg contains type info

	// Extraction hints
	TypeFromArg bool `yaml:"typeFromArg,omitempty" json:"typeFromArg,omitempty"` // Extract type from argument
	Deref       bool `yaml:"deref,omitempty" json:"deref,omitempty"`             // Dereference pointer types

	// NameFromMapKey extracts parameter names from the string-literal keys used
	// to index this call's map result inside the handler, rather than from a
	// call argument. This is the gorilla/mux idiom `mux.Vars(r)["id"]`, where
	// the parameter name is a map key, not an argument. Only keys that also
	// appear as `{placeholder}` segments in the route path are emitted.
	NameFromMapKey bool `yaml:"nameFromMapKey,omitempty" json:"nameFromMapKey,omitempty"`

	// Package/type filtering
	CallerPkgPatterns      []string `yaml:"callerPkgPatterns,omitempty" json:"callerPkgPatterns,omitempty"`
	CallerRecvTypePatterns []string `yaml:"callerRecvTypePatterns,omitempty" json:"callerRecvTypePatterns,omitempty"`
	CalleePkgPatterns      []string `yaml:"calleePkgPatterns,omitempty" json:"calleePkgPatterns,omitempty"`
	CalleeRecvTypePatterns []string `yaml:"calleeRecvTypePatterns,omitempty" json:"calleeRecvTypePatterns,omitempty"`
}

// MountPattern defines how to extract mount/subrouter information
type MountPattern struct {
	// Function call patterns to match
	CallRegex         string `yaml:"callRegex,omitempty" json:"callRegex,omitempty"`
	FunctionNameRegex string `yaml:"functionNameRegex,omitempty" json:"functionNameRegex,omitempty"`
	RecvType          string `yaml:"recvType,omitempty" json:"recvType,omitempty"`
	RecvTypeRegex     string `yaml:"recvTypeRegex,omitempty" json:"recvTypeRegex,omitempty"`

	// Argument extraction hints
	PathArgIndex   int `yaml:"pathArgIndex,omitempty" json:"pathArgIndex,omitempty"`     // Which arg contains mount path
	RouterArgIndex int `yaml:"routerArgIndex,omitempty" json:"routerArgIndex,omitempty"` // Which arg contains router

	// Extraction hints
	PathFromArg   bool `yaml:"pathFromArg,omitempty" json:"pathFromArg,omitempty"`     // Extract path from argument
	RouterFromArg bool `yaml:"routerFromArg,omitempty" json:"routerFromArg,omitempty"` // Extract router from argument
	IsMount       bool `yaml:"isMount,omitempty" json:"isMount,omitempty"`             // This is a mount operation

	// Package/type filtering
	CallerPkgPatterns      []string `yaml:"callerPkgPatterns,omitempty" json:"callerPkgPatterns,omitempty"`
	CallerRecvTypePatterns []string `yaml:"callerRecvTypePatterns,omitempty" json:"callerRecvTypePatterns,omitempty"`
	CalleePkgPatterns      []string `yaml:"calleePkgPatterns,omitempty" json:"calleePkgPatterns,omitempty"`
	CalleeRecvTypePatterns []string `yaml:"calleeRecvTypePatterns,omitempty" json:"calleeRecvTypePatterns,omitempty"`
}

// Security scope values for SecurityPattern.Scope. They describe how far the
// middleware matched by a SecurityPattern reaches.
const (
	// SecurityScopeRouter: middleware applies to routes registered on the SAME
	// receiver/router, in the same scope, AFTER this call (e.g. chi/echo/gin/mux
	// `Use`). Correlation keys on (caller function/closure, callee recvType) and
	// source-position ordering — not on the receiver variable name, which is not
	// always populated.
	SecurityScopeRouter = "router"
	// SecurityScopeSubtree: middleware applies to everything in the mounted
	// subtree (Group/Route closures, echo/gin/fiber Group(mw...)).
	SecurityScopeSubtree = "subtree"
	// SecurityScopeRoute: middleware applies to this single route registration
	// call only (chi `With`, echo/gin/fiber per-route middleware args).
	SecurityScopeRoute = "route"
	// SecurityScopeWrapper: the handler argument is wrapped by an auth function;
	// the wrapping call's identity is the middleware (net/http, mux Handle).
	SecurityScopeWrapper = "wrapper"
)

// SecurityPattern defines how to recognise an auth/security middleware
// application and the scope over which it applies. It mirrors MountPattern's
// matcher fields; the resulting OpenAPI scheme is resolved separately through
// SecurityMapping so the engine never hardcodes a framework or library.
type SecurityPattern struct {
	// Function call patterns to match (same matcher fields as other patterns).
	CallRegex         string `yaml:"callRegex,omitempty" json:"callRegex,omitempty"` // e.g. '^Use$', '^With$', '^Group$'
	FunctionNameRegex string `yaml:"functionNameRegex,omitempty" json:"functionNameRegex,omitempty"`
	RecvType          string `yaml:"recvType,omitempty" json:"recvType,omitempty"`
	RecvTypeRegex     string `yaml:"recvTypeRegex,omitempty" json:"recvTypeRegex,omitempty"` // e.g. chi.*Mux / echo.*(Echo|Group)

	// Scope describes how far the matched middleware reaches. One of the
	// SecurityScope* constants (router|subtree|route|wrapper).
	Scope string `yaml:"scope,omitempty" json:"scope,omitempty"`

	// Argument extraction hints — where the middleware value(s) live on the call.
	MiddlewareArgIndex    int  `yaml:"middlewareArgIndex,omitempty" json:"middlewareArgIndex,omitempty"`       // index of the first middleware arg
	MiddlewareVariadic    bool `yaml:"middlewareVariadic,omitempty" json:"middlewareVariadic,omitempty"`       // collect args from MiddlewareArgIndex..end
	MiddlewareExcludeLast bool `yaml:"middlewareExcludeLast,omitempty" json:"middlewareExcludeLast,omitempty"` // with variadic: skip the final arg (the handler), for gin/fiber per-route mw
	MiddlewareFromRecv    bool `yaml:"middlewareFromRecv,omitempty" json:"middlewareFromRecv,omitempty"`       // the middleware value is the receiver (rare)
	HandlerArgIndex       int  `yaml:"handlerArgIndex,omitempty" json:"handlerArgIndex,omitempty"`             // wrapped/guarded handler arg (scope=wrapper/route)

	// Package/type filtering.
	CallerPkgPatterns      []string `yaml:"callerPkgPatterns,omitempty" json:"callerPkgPatterns,omitempty"`
	CallerRecvTypePatterns []string `yaml:"callerRecvTypePatterns,omitempty" json:"callerRecvTypePatterns,omitempty"`
	CalleePkgPatterns      []string `yaml:"calleePkgPatterns,omitempty" json:"calleePkgPatterns,omitempty"`
	CalleeRecvTypePatterns []string `yaml:"calleeRecvTypePatterns,omitempty" json:"calleeRecvTypePatterns,omitempty"`
}

// SecurityMapping resolves a middleware *identity* (the function, constructor,
// or method value applied as middleware) to one or more OpenAPI security
// requirements. It is framework-agnostic and shared across frameworks; default
// presets ship these for well-known libraries (selected by an import detector)
// and users can override or extend them.
type SecurityMapping struct {
	// Match the resolved middleware identity. Empty fields are ignored.
	FunctionNameRegex string `yaml:"functionNameRegex,omitempty" json:"functionNameRegex,omitempty"` // e.g. '^authMiddleware$', '^New$'
	PkgRegex          string `yaml:"pkgRegex,omitempty" json:"pkgRegex,omitempty"`                   // e.g. 'github.com/golang-jwt/.*'
	RecvTypeRegex     string `yaml:"recvTypeRegex,omitempty" json:"recvTypeRegex,omitempty"`         // for method-value middleware (h.authMiddleware)

	// Resulting requirement(s). Entries in Schemes are ANDed (all required).
	// SchemesAnyOf contributes alternative requirement objects (OR).
	Schemes      []SecurityRequirement   `yaml:"schemes,omitempty" json:"schemes,omitempty"`
	SchemesAnyOf [][]SecurityRequirement `yaml:"schemesAnyOf,omitempty" json:"schemesAnyOf,omitempty"`

	// Public marks the matched scope as explicitly unauthenticated: it clears
	// inherited security for the affected route(s)/subtree (e.g. a skipper or
	// AllowUnauthenticated middleware), yielding `security: []`.
	Public bool `yaml:"public,omitempty" json:"public,omitempty"`

	// Skip marks the matched middleware as known non-security (e.g. logging,
	// CORS, recovery, request-id, compression). It is treated as resolved so it
	// emits no scheme, does not affect inherited security, and — unlike a plain
	// unmapped middleware — is NOT reported in the unresolved diagnostics. Use it
	// to silence noise from utility middleware that share a router's Use/Group
	// slot with real auth middleware. Mutually exclusive with schemes/public.
	Skip bool `yaml:"skip,omitempty" json:"skip,omitempty"`
}

// validSecurityScopes is the set of accepted SecurityPattern.Scope values.
var validSecurityScopes = map[string]bool{
	SecurityScopeRouter:  true,
	SecurityScopeSubtree: true,
	SecurityScopeRoute:   true,
	SecurityScopeWrapper: true,
}

// ValidateSecurity checks the auth/security patterns and mappings for obvious
// mistakes: unknown scopes, patterns/mappings that can never match, and regexes
// that do not compile. It returns the first error encountered. It is a no-op
// when no security patterns/mappings are configured, so existing configs are
// unaffected. Call it on any config built outside LoadAPISpecConfig (e.g. the
// UI's structured/raw paths) to enforce the same checks.
func (c *APISpecConfig) ValidateSecurity() error {
	compile := func(field, expr string) error {
		if expr == "" {
			return nil
		}
		if _, err := regexp.Compile(expr); err != nil {
			return fmt.Errorf("securityConfig: invalid regex in %s %q: %w", field, expr, err)
		}
		return nil
	}

	for i, p := range c.Framework.SecurityPatterns {
		if !validSecurityScopes[p.Scope] {
			return fmt.Errorf("securityPatterns[%d]: invalid scope %q (want router|subtree|route|wrapper)", i, p.Scope)
		}
		if p.CallRegex == "" && p.FunctionNameRegex == "" && p.RecvType == "" && p.RecvTypeRegex == "" {
			return fmt.Errorf("securityPatterns[%d]: needs at least one matcher (callRegex/functionNameRegex/recvType/recvTypeRegex)", i)
		}
		for _, f := range []struct{ name, expr string }{
			{"callRegex", p.CallRegex}, {"functionNameRegex", p.FunctionNameRegex}, {"recvTypeRegex", p.RecvTypeRegex},
		} {
			if err := compile(fmt.Sprintf("securityPatterns[%d].%s", i, f.name), f.expr); err != nil {
				return err
			}
		}
	}

	for i, m := range c.SecurityMappings {
		if m.FunctionNameRegex == "" && m.PkgRegex == "" && m.RecvTypeRegex == "" {
			return fmt.Errorf("securityMappings[%d]: needs at least one identity matcher (functionNameRegex/pkgRegex/recvTypeRegex)", i)
		}
		if m.Skip && (m.Public || len(m.Schemes) > 0 || len(m.SchemesAnyOf) > 0) {
			return fmt.Errorf("securityMappings[%d]: skip:true is mutually exclusive with schemes/schemesAnyOf/public", i)
		}
		if !m.Skip && !m.Public && len(m.Schemes) == 0 && len(m.SchemesAnyOf) == 0 {
			return fmt.Errorf("securityMappings[%d]: needs schemes, schemesAnyOf, public:true, or skip:true", i)
		}
		// Reject blank scheme keys: `schemes: [{"": []}]` would emit an invalid
		// OpenAPI security requirement.
		for j, req := range m.Schemes {
			if err := checkSchemeKeys(fmt.Sprintf("securityMappings[%d].schemes[%d]", i, j), req); err != nil {
				return err
			}
		}
		for j, grp := range m.SchemesAnyOf {
			for k, req := range grp {
				if err := checkSchemeKeys(fmt.Sprintf("securityMappings[%d].schemesAnyOf[%d][%d]", i, j, k), req); err != nil {
					return err
				}
			}
		}
		for _, f := range []struct{ name, expr string }{
			{"functionNameRegex", m.FunctionNameRegex}, {"pkgRegex", m.PkgRegex}, {"recvTypeRegex", m.RecvTypeRegex},
		} {
			if err := compile(fmt.Sprintf("securityMappings[%d].%s", i, f.name), f.expr); err != nil {
				return err
			}
		}
	}
	return nil
}

// checkSchemeKeys rejects blank/whitespace-only scheme names in a security
// requirement object.
func checkSchemeKeys(where string, req SecurityRequirement) error {
	for name := range req {
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("%s: blank security scheme name", where)
		}
	}
	return nil
}

// TypeMapping maps Go types to OpenAPI schemas
type TypeMapping struct {
	GoType      string  `yaml:"goType" json:"goType,omitempty"`
	OpenAPIType *Schema `yaml:"openapiType" json:"openapiType,omitempty"`
}

// Override provides manual overrides for specific functions
type Override struct {
	FunctionName   string   `yaml:"functionName" json:"functionName,omitempty"`
	Summary        string   `yaml:"summary,omitempty" json:"summary,omitempty"`
	Description    string   `yaml:"description,omitempty" json:"description,omitempty"`
	ResponseStatus int      `yaml:"responseStatus,omitempty" json:"responseStatus,omitempty"`
	ResponseType   string   `yaml:"responseType,omitempty" json:"responseType,omitempty"`
	Tags           []string `yaml:"tags,omitempty" json:"tags,omitempty"`
}

// IncludeExclude defines what to include/exclude
type IncludeExclude struct {
	Files     []string `yaml:"files" json:"files,omitempty"`
	Packages  []string `yaml:"packages" json:"packages,omitempty"`
	Functions []string `yaml:"functions" json:"functions,omitempty"`
	Types     []string `yaml:"types" json:"types,omitempty"`
}

// matchesPattern checks if a path matches a gitignore-style pattern
func matchesPattern(pattern, path string) bool {
	return patterns.Match(pattern, path)
}

// ShouldIncludeFile checks if a file should be included based on include/exclude patterns
func (ie *IncludeExclude) ShouldIncludeFile(filePath string) bool {
	// If no patterns specified, include everything
	if len(ie.Files) == 0 {
		return true
	}

	// Check if file matches any include pattern
	for _, pattern := range ie.Files {
		if matchesPattern(pattern, filePath) {
			return true
		}
	}
	return false
}

// ShouldIncludePackage checks if a package should be included based on include/exclude patterns
func (ie *IncludeExclude) ShouldIncludePackage(pkgPath string) bool {
	// If no patterns specified, include everything
	if len(ie.Packages) == 0 {
		return true
	}

	// Check if package matches any include pattern
	for _, pattern := range ie.Packages {
		if matchesPattern(pattern, pkgPath) {
			return true
		}
	}
	return false
}

// ShouldIncludeFunction checks if a function should be included based on include/exclude patterns
func (ie *IncludeExclude) ShouldIncludeFunction(funcName string) bool {
	// If no patterns specified, include everything
	if len(ie.Functions) == 0 {
		return true
	}

	// Check if function matches any include pattern
	for _, pattern := range ie.Functions {
		if matchesPattern(pattern, funcName) {
			return true
		}
	}
	return false
}

// ShouldIncludeType checks if a type should be included based on include/exclude patterns
func (ie *IncludeExclude) ShouldIncludeType(typeName string) bool {
	// If no patterns specified, include everything
	if len(ie.Types) == 0 {
		return true
	}

	// Check if type matches any include pattern
	for _, pattern := range ie.Types {
		if matchesPattern(pattern, typeName) {
			return true
		}
	}
	return false
}

// ShouldExcludeFile checks if a file should be excluded based on exclude patterns
func (ie *IncludeExclude) ShouldExcludeFile(filePath string) bool {
	// If no patterns specified, exclude nothing
	if len(ie.Files) == 0 {
		return false
	}

	// Check if file matches any exclude pattern
	for _, pattern := range ie.Files {
		if matchesPattern(pattern, filePath) {
			return true
		}
	}
	return false
}

// ShouldExcludePackage checks if a package should be excluded based on exclude patterns
func (ie *IncludeExclude) ShouldExcludePackage(pkgPath string) bool {
	// If no patterns specified, exclude nothing
	if len(ie.Packages) == 0 {
		return false
	}

	// Check if package matches any exclude pattern
	for _, pattern := range ie.Packages {
		if matchesPattern(pattern, pkgPath) {
			return true
		}
	}
	return false
}

// ShouldExcludeFunction checks if a function should be excluded based on exclude patterns
func (ie *IncludeExclude) ShouldExcludeFunction(funcName string) bool {
	// If no patterns specified, exclude nothing
	if len(ie.Functions) == 0 {
		return false
	}

	// Check if function matches any exclude pattern
	for _, pattern := range ie.Functions {
		if matchesPattern(pattern, funcName) {
			return true
		}
	}
	return false
}

// ShouldExcludeType checks if a type should be excluded based on exclude patterns
func (ie *IncludeExclude) ShouldExcludeType(typeName string) bool {
	// If no patterns specified, exclude nothing
	if len(ie.Types) == 0 {
		return false
	}

	// Check if type matches any exclude pattern
	for _, pattern := range ie.Types {
		if matchesPattern(pattern, typeName) {
			return true
		}
	}
	return false
}

// Defaults provides default values
type Defaults struct {
	RequestContentType  string `yaml:"requestContentType,omitempty" json:"requestContentType,omitempty"`
	ResponseContentType string `yaml:"responseContentType,omitempty" json:"responseContentType,omitempty"`
	ResponseStatus      int    `yaml:"responseStatus,omitempty" json:"responseStatus,omitempty"`
}

// ExternalType defines an external type that should be treated as known
type ExternalType struct {
	Name        string  `yaml:"name" json:"name,omitempty"`               // Full type name (e.g., "primitive.ObjectID")
	OpenAPIType *Schema `yaml:"openapiType" json:"openapiType,omitempty"` // OpenAPI schema for this type
	Description string  `yaml:"description,omitempty" json:"description,omitempty"`
}

// APISpecConfig is the main configuration struct
type APISpecConfig struct {
	// Framework-specific patterns
	Framework FrameworkConfig `yaml:"framework" json:"framework,omitempty"`

	// Type mappings
	TypeMapping []TypeMapping `yaml:"typeMapping" json:"typeMapping,omitempty"`

	// External types that should be treated as known
	ExternalTypes []ExternalType `yaml:"externalTypes" json:"externalTypes,omitempty"`

	// Manual overrides
	Overrides []Override `yaml:"overrides" json:"overrides,omitempty"`

	// Include/exclude filters
	Include IncludeExclude `yaml:"include" json:"include,omitempty"`
	Exclude IncludeExclude `yaml:"exclude" json:"exclude,omitempty"`

	// Defaults
	Defaults Defaults `yaml:"defaults" json:"defaults,omitempty"`

	// OpenAPI metadata
	Info            Info                      `yaml:"info" json:"info,omitempty"`
	Servers         []Server                  `yaml:"servers" json:"servers,omitempty"`
	Security        []SecurityRequirement     `yaml:"security" json:"security,omitempty"`
	SecuritySchemes map[string]SecurityScheme `yaml:"securitySchemes" json:"securitySchemes,omitempty"`

	// SecurityMappings resolve detected auth middleware to security schemes
	// (see SecurityMapping). Framework-agnostic; merged from library presets and
	// user config. Works together with Framework.SecurityPatterns (scope).
	SecurityMappings []SecurityMapping `yaml:"securityMappings" json:"securityMappings,omitempty"`

	// presetSchemes holds securityScheme definitions contributed by library
	// presets (see config_security.go). They are added to the output components
	// only when actually referenced by a resolved operation, so unused presets
	// don't bloat the spec. Not serialized.
	presetSchemes map[string]SecurityScheme `yaml:"-" json:"-"`

	// presetsApplied guards ApplySecurityPresets against appending the same
	// library mappings twice when a config is reused across runs. Not serialized.
	presetsApplied bool                   `yaml:"-" json:"-"`
	Tags           []Tag                  `yaml:"tags" json:"tags,omitempty"`
	ExternalDocs   *ExternalDocumentation `yaml:"externalDocs" json:"externalDocs,omitempty"`
}

// ShouldIncludeFile checks if a file should be included based on include/exclude filters
func (c *APISpecConfig) ShouldIncludeFile(filePath string) bool {
	// First check exclude patterns (exclude takes precedence)
	if c.Exclude.ShouldExcludeFile(filePath) {
		return false
	}

	// Then check include patterns
	return c.Include.ShouldIncludeFile(filePath)
}

// ShouldIncludePackage checks if a package should be included based on include/exclude filters
func (c *APISpecConfig) ShouldIncludePackage(pkgPath string) bool {
	// First check exclude patterns (exclude takes precedence)
	if c.Exclude.ShouldExcludePackage(pkgPath) {
		return false
	}

	// Then check include patterns
	return c.Include.ShouldIncludePackage(pkgPath)
}

// ShouldIncludeFunction checks if a function should be included based on include/exclude filters
func (c *APISpecConfig) ShouldIncludeFunction(funcName string) bool {
	// First check exclude patterns (exclude takes precedence)
	if c.Exclude.ShouldExcludeFunction(funcName) {
		return false
	}

	// Then check include patterns
	return c.Include.ShouldIncludeFunction(funcName)
}

// ShouldIncludeType checks if a type should be included based on include/exclude filters
func (c *APISpecConfig) ShouldIncludeType(typeName string) bool {
	// First check exclude patterns (exclude takes precedence)
	if c.Exclude.ShouldExcludeType(typeName) {
		return false
	}

	// Then check include patterns
	return c.Include.ShouldIncludeType(typeName)
}

// MatchPattern checks if a pattern matches a value
func (p *RoutePattern) MatchPattern(pattern, value string) bool {
	if pattern == "" {
		return false
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return false
	}
	return re.MatchString(value)
}

// MatchFunctionName checks if the function name regex matches
func (p *RoutePattern) MatchFunctionName(functionName string) bool {
	return p.MatchPattern(p.FunctionNameRegex, functionName)
}

// The four per-framework RequestContext presets — netHTTPRequestContext,
// ginRequestContext, echoRequestContext, fiberRequestContext — used to
// live in this file. They now sit next to the framework default that owns
// them, in config_http.go, config_gin.go, config_echo.go, config_fiber.go.
// Chi and Mux reference netHTTPRequestContext from config_http.go directly
// (same package, no import needed).

// Shared pattern helpers used by the framework defaults. These collapse
// duplication that previously sat in every Default*Config function — for
// example the five net/http ResponseWriter patterns were copied verbatim
// across all six configs, drifting slightly over time (audit finding #3 /
// #11). Each helper returns a fresh value so callers can append to it
// without worrying about shared mutable state.

// netHTTPResponsePatterns returns the standard ResponseWriter and net/http
// helper response patterns. Identical across every framework default.
func netHTTPResponsePatterns() []ResponsePattern {
	return []ResponsePattern{
		{
			CallRegex:      `^WriteHeader$`,
			StatusArgIndex: 0,
			StatusFromArg:  true,
			TypeArgIndex:   -1,
			RecvTypeRegex:  `^net/http\.ResponseWriter$`,
		},
		{
			CallRegex:     `^Write$`,
			TypeArgIndex:  0,
			TypeFromArg:   true,
			Deref:         true,
			RecvTypeRegex: `^net/http\.ResponseWriter$`,
		},
		{
			CallRegex:          `^Error$`,
			StatusArgIndex:     2,
			StatusFromArg:      true,
			TypeFromArg:        true,
			TypeArgIndex:       1,
			RecvTypeRegex:      `^net/http$`,
			DefaultContentType: "text/plain; charset=utf-8",
		},
		{
			CallRegex:      `^NotFound$`,
			StatusArgIndex: -1, // Always 404
			StatusFromArg:  false,
			TypeArgIndex:   -1,
			RecvTypeRegex:  `^net/http$`,
			DefaultStatus:  http.StatusNotFound,
		},
		{
			CallRegex:      `^Redirect$`,
			StatusArgIndex: 3,
			StatusFromArg:  true,
			TypeArgIndex:   -1,
			RecvTypeRegex:  `^net/http$`,
		},
	}
}

// jsonMarshalPattern returns the encoding/json.Marshal-style response
// pattern. Identical across every framework default.
func jsonMarshalPattern() ResponsePattern {
	return ResponsePattern{
		CallRegex:    `^Marshal$`,
		TypeArgIndex: 0,
		TypeFromArg:  true,
		Deref:        true,
	}
}

// jsonEncodePattern returns the json.Encoder.Encode response pattern.
// recvTypeRegex varies between frameworks: pass "" to match any receiver,
// or `.*json(iter)?\.\*?Encoder` to restrict to JSON encoders specifically.
func jsonEncodePattern(recvTypeRegex string) ResponsePattern {
	return ResponsePattern{
		CallRegex:     `^Encode$`,
		TypeArgIndex:  0,
		TypeFromArg:   true,
		Deref:         true,
		RecvTypeRegex: recvTypeRegex,
	}
}

// jsonDecodeRequestPattern returns the json.Decoder.Decode request-body
// pattern. recvTypeRegex varies between frameworks (some restrict to
// *Decoder, some accept any receiver).
func jsonDecodeRequestPattern(recvTypeRegex string) RequestBodyPattern {
	return RequestBodyPattern{
		CallRegex:            `^Decode$`,
		TypeArgIndex:         0,
		TypeFromArg:          true,
		Deref:                true,
		RecvTypeRegex:        recvTypeRegex,
		RequireRequestSource: true,
		BodyFromReceiver:     true,
	}
}

// jsonUnmarshalRequestPattern returns the encoding/json.Unmarshal
// request-body pattern. recvTypeRegex varies similarly to Decode.
func jsonUnmarshalRequestPattern(recvTypeRegex string) RequestBodyPattern {
	return RequestBodyPattern{
		CallRegex:            `^Unmarshal$`,
		TypeArgIndex:         1,
		TypeFromArg:          true,
		Deref:                true,
		RecvTypeRegex:        recvTypeRegex,
		RequireRequestSource: true,
		BodySourceArgIndex:   0,
	}
}

// stdDefaults returns the Defaults block shared by every framework config,
// parameterised on responseStatus (HTTP-style defaults all use 200; Chi's
// older config kept its own constant — preserved here for parity).
func stdDefaults(responseStatus int) Defaults {
	return Defaults{
		RequestContentType:  defaultRequestContentType,
		ResponseContentType: defaultResponseContentType,
		ResponseStatus:      responseStatus,
	}
}
