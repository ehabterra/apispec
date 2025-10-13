package spec

import (
	"net/http"
	"regexp"

	"github.com/ehabterra/apispec/pkg/patterns"
)

const (
	defaultRequestContentType  = "application/json"
	defaultResponseContentType = "application/json"
	defaultResponseStatus      = 200
	primitiveObjectIDType      = "go.mongodb.org/mongo-driver/bson/primitive.ObjectID"
	primitiveObjectIDFormat    = "objectid"
)

// FrameworkConfig defines framework-specific extraction patterns
type FrameworkConfig struct {
	// Route extraction patterns
	RoutePatterns []RoutePattern `yaml:"routePatterns"`

	// Request body extraction patterns
	RequestBodyPatterns []RequestBodyPattern `yaml:"requestBodyPatterns"`

	// Response extraction patterns
	ResponsePatterns []ResponsePattern `yaml:"responsePatterns"`

	// Parameter extraction patterns
	ParamPatterns []ParamPattern `yaml:"paramPatterns"`

	// Mount/subrouter patterns
	MountPatterns []MountPattern `yaml:"mountPatterns"`
}

// MethodMapping defines how to extract HTTP methods from function names
type MethodMapping struct {
	Patterns []string `yaml:"patterns,omitempty"` // Function name patterns (e.g., ["get", "list", "show"])
	Method   string   `yaml:"method,omitempty"`   // HTTP method (e.g., "GET")
	Priority int      `yaml:"priority,omitempty"` // Higher priority = checked first
}

// MethodExtractionConfig defines how to extract HTTP methods
type MethodExtractionConfig struct {
	// Method mappings from function names
	MethodMappings []MethodMapping `yaml:"methodMappings,omitempty"`

	// Extraction strategy
	UsePrefix     bool `yaml:"usePrefix,omitempty"`     // Check for prefix matches (getUser -> GET)
	UseContains   bool `yaml:"useContains,omitempty"`   // Check for contains matches (userGet -> GET)
	CaseSensitive bool `yaml:"caseSensitive,omitempty"` // Case sensitive matching

	// Fallback behavior
	DefaultMethod    string `yaml:"defaultMethod,omitempty"`    // Default method when none found
	InferFromContext bool   `yaml:"inferFromContext,omitempty"` // Try to infer from call context
}

// RoutePattern defines how to extract route information
type RoutePattern struct {
	// Function call patterns to match
	CallRegex         string `yaml:"callRegex,omitempty"`         // e.g., '^BindJSON$'
	FunctionNameRegex string `yaml:"functionNameRegex,omitempty"` // e.g., '.*Handler$'
	RecvType          string `yaml:"recvType,omitempty"`          // e.g., 'context.Context'
	RecvTypeRegex     string `yaml:"recvTypeRegex,omitempty"`     // e.g., '^context\.Context$'

	// Argument extraction hints
	MethodArgIndex  int `yaml:"methodArgIndex,omitempty"`  // Which arg contains HTTP method
	PathArgIndex    int `yaml:"pathArgIndex,omitempty"`    // Which arg contains path
	HandlerArgIndex int `yaml:"handlerArgIndex,omitempty"` // Which arg contains handler

	// Extraction hints
	MethodFromCall    bool `yaml:"methodFromCall,omitempty"`    // Extract method from function name
	MethodFromHandler bool `yaml:"methodFromHandler,omitempty"` // Extract method from handler function name
	PathFromArg       bool `yaml:"pathFromArg,omitempty"`       // Extract path from argument
	HandlerFromArg    bool `yaml:"handlerFromArg,omitempty"`    // Extract handler from argument

	// Method extraction configuration
	MethodExtraction *MethodExtractionConfig `yaml:"methodExtraction,omitempty"`

	// Package/type filtering
	CallerPkgPatterns      []string `yaml:"callerPkgPatterns,omitempty"`
	CallerRecvTypePatterns []string `yaml:"callerRecvTypePatterns,omitempty"`
	CalleePkgPatterns      []string `yaml:"calleePkgPatterns,omitempty"`
	CalleeRecvTypePatterns []string `yaml:"calleeRecvTypePatterns,omitempty"`
}

// RequestBodyPattern defines how to extract request body information
type RequestBodyPattern struct {
	// Function call patterns to match
	CallRegex         string `yaml:"callRegex,omitempty"`
	FunctionNameRegex string `yaml:"functionNameRegex,omitempty"`
	RecvType          string `yaml:"recvType,omitempty"`
	RecvTypeRegex     string `yaml:"recvTypeRegex,omitempty"`

	// Argument extraction hints
	TypeArgIndex int `yaml:"typeArgIndex,omitempty"` // Which arg contains type info

	// Extraction hints
	TypeFromArg    bool `yaml:"typeFromArg,omitempty"`    // Extract type from argument
	TypeFromReturn bool `yaml:"typeFromReturn,omitempty"` // Extract type from return value
	Deref          bool `yaml:"deref,omitempty"`          // Dereference pointer types

	// Context-aware validation
	AllowForGetMethods bool `yaml:"allowForGetMethods,omitempty"` // Allow this pattern for GET/HEAD methods

	// Package/type filtering
	CallerPkgPatterns      []string `yaml:"callerPkgPatterns,omitempty"`
	CallerRecvTypePatterns []string `yaml:"callerRecvTypePatterns,omitempty"`
	CalleePkgPatterns      []string `yaml:"calleePkgPatterns,omitempty"`
	CalleeRecvTypePatterns []string `yaml:"calleeRecvTypePatterns,omitempty"`
}

// ResponsePattern defines how to extract response information
type ResponsePattern struct {
	// Function call patterns to match
	CallRegex         string `yaml:"callRegex,omitempty"`
	FunctionNameRegex string `yaml:"functionNameRegex,omitempty"`
	RecvType          string `yaml:"recvType,omitempty"`
	RecvTypeRegex     string `yaml:"recvTypeRegex,omitempty"`

	// Argument extraction hints
	StatusArgIndex int `yaml:"statusArgIndex,omitempty"` // Which arg contains status code
	TypeArgIndex   int `yaml:"typeArgIndex,omitempty"`   // Which arg contains type info

	// Extraction hints
	StatusFromArg bool `yaml:"statusFromArg,omitempty"` // Extract status from argument
	TypeFromArg   bool `yaml:"typeFromArg,omitempty"`   // Extract type from argument
	Deref         bool `yaml:"deref,omitempty"`         // Dereference pointer types

	// Package/type filtering
	CallerPkgPatterns      []string `yaml:"callerPkgPatterns,omitempty"`
	CallerRecvTypePatterns []string `yaml:"callerRecvTypePatterns,omitempty"`
	CalleePkgPatterns      []string `yaml:"calleePkgPatterns,omitempty"`
	CalleeRecvTypePatterns []string `yaml:"calleeRecvTypePatterns,omitempty"`
}

// ParamPattern defines how to extract parameter information
type ParamPattern struct {
	// Function call patterns to match
	CallRegex         string `yaml:"callRegex,omitempty"`
	FunctionNameRegex string `yaml:"functionNameRegex,omitempty"`
	RecvType          string `yaml:"recvType,omitempty"`
	RecvTypeRegex     string `yaml:"recvTypeRegex,omitempty"`

	// Parameter location and extraction
	ParamIn       string `yaml:"paramIn,omitempty"`       // path, query, header, cookie
	ParamArgIndex int    `yaml:"paramArgIndex,omitempty"` // Which arg contains parameter
	TypeArgIndex  int    `yaml:"typeArgIndex,omitempty"`  // Which arg contains type info

	// Extraction hints
	TypeFromArg bool `yaml:"typeFromArg,omitempty"` // Extract type from argument
	Deref       bool `yaml:"deref,omitempty"`       // Dereference pointer types

	// Package/type filtering
	CallerPkgPatterns      []string `yaml:"callerPkgPatterns,omitempty"`
	CallerRecvTypePatterns []string `yaml:"callerRecvTypePatterns,omitempty"`
	CalleePkgPatterns      []string `yaml:"calleePkgPatterns,omitempty"`
	CalleeRecvTypePatterns []string `yaml:"calleeRecvTypePatterns,omitempty"`
}

// MountPattern defines how to extract mount/subrouter information
type MountPattern struct {
	// Function call patterns to match
	CallRegex         string `yaml:"callRegex,omitempty"`
	FunctionNameRegex string `yaml:"functionNameRegex,omitempty"`
	RecvType          string `yaml:"recvType,omitempty"`
	RecvTypeRegex     string `yaml:"recvTypeRegex,omitempty"`

	// Argument extraction hints
	PathArgIndex   int `yaml:"pathArgIndex,omitempty"`   // Which arg contains mount path
	RouterArgIndex int `yaml:"routerArgIndex,omitempty"` // Which arg contains router

	// Extraction hints
	PathFromArg   bool `yaml:"pathFromArg,omitempty"`   // Extract path from argument
	RouterFromArg bool `yaml:"routerFromArg,omitempty"` // Extract router from argument
	IsMount       bool `yaml:"isMount,omitempty"`       // This is a mount operation

	// Package/type filtering
	CallerPkgPatterns      []string `yaml:"callerPkgPatterns,omitempty"`
	CallerRecvTypePatterns []string `yaml:"callerRecvTypePatterns,omitempty"`
	CalleePkgPatterns      []string `yaml:"calleePkgPatterns,omitempty"`
	CalleeRecvTypePatterns []string `yaml:"calleeRecvTypePatterns,omitempty"`
}

// TypeMapping maps Go types to OpenAPI schemas
type TypeMapping struct {
	GoType      string  `yaml:"goType"`
	OpenAPIType *Schema `yaml:"openapiType"`
}

// Override provides manual overrides for specific functions
type Override struct {
	FunctionName   string   `yaml:"functionName"`
	Summary        string   `yaml:"summary,omitempty"`
	Description    string   `yaml:"description,omitempty"`
	ResponseStatus int      `yaml:"responseStatus,omitempty"`
	ResponseType   string   `yaml:"responseType,omitempty"`
	Tags           []string `yaml:"tags,omitempty"`
}

// IncludeExclude defines what to include/exclude
type IncludeExclude struct {
	Files     []string `yaml:"files"`
	Packages  []string `yaml:"packages"`
	Functions []string `yaml:"functions"`
	Types     []string `yaml:"types"`
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
	RequestContentType  string `yaml:"requestContentType,omitempty"`
	ResponseContentType string `yaml:"responseContentType,omitempty"`
	ResponseStatus      int    `yaml:"responseStatus,omitempty"`
}

// ExternalType defines an external type that should be treated as known
type ExternalType struct {
	Name        string  `yaml:"name"`        // Full type name (e.g., "primitive.ObjectID")
	OpenAPIType *Schema `yaml:"openapiType"` // OpenAPI schema for this type
	Description string  `yaml:"description,omitempty"`
}

// APISpecConfig is the main configuration struct
type APISpecConfig struct {
	// Framework-specific patterns
	Framework FrameworkConfig `yaml:"framework"`

	// Type mappings
	TypeMapping []TypeMapping `yaml:"typeMapping"`

	// External types that should be treated as known
	ExternalTypes []ExternalType `yaml:"externalTypes"`

	// Manual overrides
	Overrides []Override `yaml:"overrides"`

	// Include/exclude filters
	Include IncludeExclude `yaml:"include"`
	Exclude IncludeExclude `yaml:"exclude"`

	// Defaults
	Defaults Defaults `yaml:"defaults"`

	// OpenAPI metadata
	Info            Info                      `yaml:"info"`
	Servers         []Server                  `yaml:"servers"`
	Security        []SecurityRequirement     `yaml:"security"`
	SecuritySchemes map[string]SecurityScheme `yaml:"securitySchemes"`
	Tags            []Tag                     `yaml:"tags"`
	ExternalDocs    *ExternalDocumentation    `yaml:"externalDocs"`
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

// DefaultChiConfig returns a default configuration for Chi router
func DefaultChiConfig() *APISpecConfig {
	return &APISpecConfig{
		Framework: FrameworkConfig{
			RoutePatterns: []RoutePattern{
				{
					CallRegex:       `(?i)(GET|POST|PUT|DELETE|PATCH|OPTIONS|HEAD)$`,
					MethodFromCall:  true,
					PathFromArg:     true,
					HandlerFromArg:  true,
					PathArgIndex:    0,
					HandlerArgIndex: 1,
					RecvTypeRegex:   "^github.com/go-chi/chi(/v\\d)?\\.\\*?(Router|Mux)$",
				},
			},
			RequestBodyPatterns: []RequestBodyPattern{
				{
					CallRegex:     `^DecodeJSON$`,
					TypeArgIndex:  1,
					TypeFromArg:   true,
					Deref:         true,
					RecvTypeRegex: "^github\\.com/go-chi/render$",
				},
				{
					CallRegex:     `^Decode$`,
					TypeArgIndex:  0,
					TypeFromArg:   true,
					Deref:         true,
					RecvTypeRegex: ".*json(iter)?\\.\\*Decoder",
				},
				{
					CallRegex:     `^Unmarshal$`,
					TypeArgIndex:  1,
					TypeFromArg:   true,
					Deref:         true,
					RecvTypeRegex: "json",
				},
			},
			ResponsePatterns: []ResponsePattern{
				{
					CallRegex:     `^JSON$`,
					TypeArgIndex:  2,
					TypeFromArg:   true,
					StatusFromArg: false,
					Deref:         true,
					RecvTypeRegex: "^github\\.com/go-chi/render$",
				},
				{
					CallRegex:      `^Status$`,
					StatusArgIndex: 1,
					StatusFromArg:  true,
					RecvTypeRegex:  "^github\\.com/go-chi/render$",
				},
				{
					CallRegex:      `^(?i)(JSON|String|XML|YAML|ProtoBuf|Data|File|Redirect)$`,
					StatusArgIndex: 0,
					TypeArgIndex:   1,
					TypeFromArg:    true,
					StatusFromArg:  true,
					Deref:          true,
				},
				{
					CallRegex:    `^Marshal$`,
					TypeArgIndex: 0,
					TypeFromArg:  true,
					Deref:        true,
				},
				{
					CallRegex:     `^Encode$`,
					TypeArgIndex:  0,
					TypeFromArg:   true,
					Deref:         true,
					RecvTypeRegex: ".*json(iter)?\\.\\*?Encoder",
				},
			},
			ParamPatterns: []ParamPattern{
				{
					CallRegex:     "^URLParam$",
					ParamIn:       "path",
					ParamArgIndex: 1,
					RecvTypeRegex: "^github\\.com/go-chi/chi(/v\\d)?$",
				},
				{
					CallRegex:     "^URLParam$",
					ParamIn:       "path",
					ParamArgIndex: 0,
					RecvTypeRegex: "^github\\.com/go-chi/chi(/v\\d)?\\.\\*?Context$",
				},
				{
					CallRegex:     "^URLParamFromCtx$",
					ParamIn:       "path",
					ParamArgIndex: 1,
					RecvTypeRegex: "^github\\.com/go-chi/chi(/v\\d)?$",
				},
				{
					CallRegex:     "^FormValue$",
					ParamIn:       "form",
					ParamArgIndex: 0,
				},
				{
					CallRegex:     "^Get$",
					ParamIn:       "query",
					ParamArgIndex: 0,
					RecvType:      "net/url.Values",
				},
				{
					CallRegex:     "^PathValue$",
					ParamIn:       "path",
					ParamArgIndex: 0,
					RecvType:      "net/http.*Request",
				},
			},
			MountPatterns: []MountPattern{
				{
					CallRegex:      `^Mount$`,
					PathFromArg:    true,
					RouterFromArg:  true,
					PathArgIndex:   0,
					RouterArgIndex: 1,
					IsMount:        true,
				},
				{
					CallRegex:      `^Route$`,
					PathFromArg:    true,
					RouterFromArg:  true,
					PathArgIndex:   0,
					RouterArgIndex: 1,
					IsMount:        true,
				},
			},
		},
		Defaults: Defaults{
			RequestContentType:  defaultRequestContentType,
			ResponseContentType: defaultResponseContentType,
			ResponseStatus:      defaultResponseStatus,
		},
		// example of external type mapping
		ExternalTypes: []ExternalType{
			{
				Name: primitiveObjectIDType,
				OpenAPIType: &Schema{
					Type:   "string",
					Format: primitiveObjectIDFormat,
				},
			},
		},
	}
}

// DefaultEchoConfig returns a default configuration for Echo framework
func DefaultEchoConfig() *APISpecConfig {
	return &APISpecConfig{
		Framework: FrameworkConfig{
			RoutePatterns: []RoutePattern{
				{
					CallRegex:       `^(?i)(GET|POST|PUT|DELETE|PATCH|OPTIONS|HEAD)$`,
					MethodFromCall:  true,
					PathFromArg:     true,
					HandlerFromArg:  true,
					PathArgIndex:    0,
					HandlerArgIndex: 1,
					RecvTypeRegex:   "^github\\.com/labstack/echo(/v\\d)?\\.\\*(Echo|Group)$",
				},
			},
			RequestBodyPatterns: []RequestBodyPattern{
				{
					CallRegex:     `^(?i)(Bind)$`,
					TypeArgIndex:  0,
					TypeFromArg:   true,
					Deref:         true,
					RecvTypeRegex: "github\\.com/labstack/echo/v\\d\\.Context",
				},
				{
					CallRegex:     `^Decode$`,
					TypeArgIndex:  0,
					TypeFromArg:   true,
					Deref:         true,
					RecvTypeRegex: ".*json(iter)?\\.\\*Decoder",
				},
				{
					CallRegex:     `^Unmarshal$`,
					TypeArgIndex:  1,
					TypeFromArg:   true,
					Deref:         true,
					RecvTypeRegex: "json",
				},
			},
			ResponsePatterns: []ResponsePattern{
				{
					CallRegex:      `^(?i)(JSON|String|XML|YAML|ProtoBuf|Data|File|Redirect)$`,
					StatusArgIndex: 0,
					TypeArgIndex:   1,
					TypeFromArg:    true,
					StatusFromArg:  true,
					Deref:          true,
					RecvTypeRegex:  "github\\.com/labstack/echo/v\\d\\.Context",
				},
				{
					CallRegex:    `^Marshal$`,
					TypeArgIndex: 0,
					TypeFromArg:  true,
					Deref:        true,
				},
				{
					CallRegex:     `^Encode$`,
					TypeArgIndex:  0,
					TypeFromArg:   true,
					Deref:         true,
					RecvTypeRegex: ".*json(iter)?\\.\\*?Encoder",
				},
			},
			ParamPatterns: []ParamPattern{
				{
					CallRegex:     "^Param$",
					ParamIn:       "path",
					ParamArgIndex: 0,
				},
				{
					CallRegex:     "^QueryParam$",
					ParamIn:       "query",
					ParamArgIndex: 0,
					RecvTypeRegex: "github\\.com/labstack/echo/v\\d\\.Context",
				},
				{
					CallRegex:     "^FormValue$",
					ParamIn:       "form",
					ParamArgIndex: 0,
				},
				{
					CallRegex:     "^Cookie$",
					ParamIn:       "cookie",
					ParamArgIndex: 0,
				},
			},
			MountPatterns: []MountPattern{
				{
					CallRegex:      `^Group$`,
					PathFromArg:    true,
					RouterFromArg:  true,
					PathArgIndex:   0,
					RouterArgIndex: 1,
					IsMount:        true,
					RecvTypeRegex:  "^github\\.com/labstack/echo(/v\\d)?\\.\\*(Echo|Group)$",
				},
			},
		},
		Exclude: IncludeExclude{
			Files: []string{
				"docs/*",
			},
		},
		Defaults: Defaults{
			RequestContentType:  defaultRequestContentType,
			ResponseContentType: defaultResponseContentType,
			ResponseStatus:      http.StatusOK,
		},
	}
}

// DefaultFiberConfig returns a default configuration for Fiber framework
func DefaultFiberConfig() *APISpecConfig {
	return &APISpecConfig{
		Framework: FrameworkConfig{
			RoutePatterns: []RoutePattern{
				{
					CallRegex:       `^(?i)(GET|POST|PUT|DELETE|PATCH|OPTIONS|HEAD)$`,
					MethodFromCall:  true,
					PathFromArg:     true,
					HandlerFromArg:  true,
					PathArgIndex:    0,
					HandlerArgIndex: 1,
					RecvTypeRegex:   `^github\.com/gofiber/fiber(/v\d)?\.\*?(App|Router|Group)$`,
				},
			},
			RequestBodyPatterns: []RequestBodyPattern{
				{
					CallRegex:     `^BodyParser$`,
					TypeArgIndex:  0,
					TypeFromArg:   true,
					Deref:         true,
					RecvTypeRegex: `^github\.com/gofiber/fiber(/v\d)?\.\*?Ctx$`,
				},
				{
					CallRegex:     `^Decode$`,
					TypeArgIndex:  0,
					TypeFromArg:   true,
					Deref:         true,
					RecvTypeRegex: ".*json(iter)?\\.\\*?Decoder",
				},
				{
					CallRegex:     `^Unmarshal$`,
					TypeArgIndex:  1,
					TypeFromArg:   true,
					Deref:         true,
					RecvTypeRegex: "json",
				},
			},
			ResponsePatterns: []ResponsePattern{
				{
					CallRegex:      `^JSON$`,
					StatusArgIndex: -1, // Fiber's c.JSON does not take status, only data
					TypeArgIndex:   0,
					TypeFromArg:    true,
					Deref:          true,
					RecvTypeRegex:  `^github\.com/gofiber/fiber(/v\d)?\.\*Ctx$`,
				},
				{
					CallRegex:      `^SendString$`,
					StatusArgIndex: -1,
					TypeArgIndex:   0,
					TypeFromArg:    true,
					RecvTypeRegex:  `^github\.com/gofiber/fiber(/v\d)?\.\*Ctx$`,
				},
				{
					CallRegex:      `^SendStatus$`,
					StatusArgIndex: 0,
					TypeArgIndex:   -1,
					RecvTypeRegex:  `^github\.com/gofiber/fiber(/v\d)?\.\*Ctx$`,
				},
				{
					CallRegex:    `^Marshal$`,
					TypeArgIndex: 0,
					TypeFromArg:  true,
					Deref:        true,
				},
				{
					CallRegex:     `^Encode$`,
					TypeArgIndex:  0,
					TypeFromArg:   true,
					Deref:         true,
					RecvTypeRegex: ".*json(iter)?\\.\\*?Encoder",
				},
			},
			ParamPatterns: []ParamPattern{
				{
					CallRegex:     "^Params$",
					ParamIn:       "path",
					ParamArgIndex: 0,
					RecvTypeRegex: `^github\.com/gofiber/fiber(/v\d)?\.\*Ctx$`,
				},
				{
					CallRegex:     "^Query$",
					ParamIn:       "query",
					ParamArgIndex: 0,
					RecvTypeRegex: `^github\.com/gofiber/fiber(/v\d)?\.\*Ctx$`,
				},
				{
					CallRegex:     "^FormValue$",
					ParamIn:       "form",
					ParamArgIndex: 0,
					RecvTypeRegex: `^github\.com/gofiber/fiber(/v\d)?\.\*Ctx$`,
				},
				{
					CallRegex:     "^Cookies$",
					ParamIn:       "cookie",
					ParamArgIndex: 0,
					RecvTypeRegex: `^github\.com/gofiber/fiber(/v\d)?\.\*Ctx$`,
				},
			},
			MountPatterns: []MountPattern{
				{
					CallRegex:      `^Mount$`,
					PathFromArg:    true,
					RouterFromArg:  true,
					PathArgIndex:   0,
					RouterArgIndex: 1,
					IsMount:        true,
				},
				{
					CallRegex:      `^Group$`,
					PathFromArg:    true,
					RouterFromArg:  true,
					PathArgIndex:   0,
					RouterArgIndex: 1,
					IsMount:        true,
					RecvTypeRegex:  `^github\.com/gofiber/fiber(/v\d)?\.\*?(App|Router|Group)$`,
				},
				{
					CallRegex:     `^Use$`,
					PathFromArg:   false,
					RouterFromArg: false,
					IsMount:       false,
					RecvTypeRegex: `^github\.com/gofiber/fiber(/v\d)?\.\*?(App|Router|Group)$`,
				},
			},
		},
		Defaults: Defaults{
			RequestContentType:  defaultRequestContentType,
			ResponseContentType: defaultResponseContentType,
			ResponseStatus:      http.StatusOK,
		},
		ExternalTypes: []ExternalType{
			{
				Name: "github.com/gofiber/fiber.Map",
				OpenAPIType: &Schema{
					Type: "object",
				},
			},
		},
	}
}

// DefaultGinConfig returns a default configuration for Gin framework
func DefaultGinConfig() *APISpecConfig {
	return &APISpecConfig{
		Framework: FrameworkConfig{
			RoutePatterns: []RoutePattern{
				{
					CallRegex:       `^(?i)(GET|POST|PUT|DELETE|PATCH|OPTIONS|HEAD)$`,
					MethodFromCall:  true,
					PathFromArg:     true,
					HandlerFromArg:  true,
					PathArgIndex:    0,
					HandlerArgIndex: 1,
					RecvTypeRegex:   "^github\\.com/gin-gonic/gin\\.\\*(Engine|RouterGroup)$",
				},
			},
			RequestBodyPatterns: []RequestBodyPattern{
				{
					CallRegex:    `^(?i)(BindJSON|ShouldBindJSON|BindXML|BindYAML|BindForm|ShouldBind)$`,
					TypeArgIndex: 0,
					TypeFromArg:  true,
					Deref:        true,
				},
				{
					CallRegex:    `^Decode$`,
					TypeArgIndex: 0,
					TypeFromArg:  true,
					Deref:        true,
				},
				{
					CallRegex:    `^Unmarshal$`,
					TypeArgIndex: 1,
					TypeFromArg:  true,
					Deref:        true,
				},
			},
			ResponsePatterns: []ResponsePattern{
				{
					CallRegex:      `^(?i)(JSON|String|XML|YAML|ProtoBuf|Data|File|Redirect)$`,
					StatusArgIndex: 0,
					TypeArgIndex:   1,
					TypeFromArg:    true,
					StatusFromArg:  true,
				},
				{
					CallRegex:    `^Marshal$`,
					TypeArgIndex: 0,
					TypeFromArg:  true,
					Deref:        true,
				},
				{
					CallRegex:    `^Encode$`,
					TypeArgIndex: 0,
					TypeFromArg:  true,
					Deref:        true,
				},
			},
			ParamPatterns: []ParamPattern{
				{
					CallRegex:     "^Param$",
					ParamIn:       "path",
					ParamArgIndex: 0,
				},
				{
					CallRegex:     "^Query$",
					ParamIn:       "query",
					ParamArgIndex: 0,
				},
				{
					CallRegex:     "^DefaultQuery$",
					ParamIn:       "query",
					ParamArgIndex: 0,
				},
				{
					CallRegex:     "^GetHeader$",
					ParamIn:       "header",
					ParamArgIndex: 0,
				},
			},
			MountPatterns: []MountPattern{
				{
					CallRegex:      `^Group$`,
					PathFromArg:    true,
					RouterFromArg:  true,
					PathArgIndex:   0,
					RouterArgIndex: 1,
					IsMount:        true,
					RecvTypeRegex:  "^github\\.com/gin-gonic/gin\\.\\*(Engine|RouterGroup)$",
				},
			},
		},
		Defaults: Defaults{
			RequestContentType:  defaultRequestContentType,
			ResponseContentType: defaultResponseContentType,
			ResponseStatus:      http.StatusOK,
		},
		ExternalTypes: []ExternalType{
			{
				Name: "github.com/gin-gonic/gin.H",
				OpenAPIType: &Schema{
					Type: "object",
				},
			},
		},
	}
}

// DefaultMethodExtractionConfig returns a default method extraction configuration
func DefaultMethodExtractionConfig() *MethodExtractionConfig {
	return &MethodExtractionConfig{
		MethodMappings: []MethodMapping{
			{Patterns: []string{"get", "list", "show", "find", "fetch", "retrieve"}, Method: "GET", Priority: 10},
			{Patterns: []string{"post", "create", "add", "new", "insert"}, Method: "POST", Priority: 10},
			{Patterns: []string{"put", "update", "edit", "modify", "replace"}, Method: "PUT", Priority: 10},
			{Patterns: []string{"delete", "remove", "destroy"}, Method: "DELETE", Priority: 10},
			{Patterns: []string{"patch", "partial"}, Method: "PATCH", Priority: 10},
			{Patterns: []string{"options"}, Method: "OPTIONS", Priority: 10},
			{Patterns: []string{"head"}, Method: "HEAD", Priority: 10},
		},
		UsePrefix:        true,
		UseContains:      true,
		CaseSensitive:    false,
		DefaultMethod:    "GET",
		InferFromContext: true,
	}
}

// DefaultMuxConfig returns a default configuration for Gorilla Mux framework
func DefaultMuxConfig() *APISpecConfig {
	return &APISpecConfig{
		Framework: FrameworkConfig{
			RoutePatterns: []RoutePattern{
				{
					CallRegex:         `^HandleFunc$`,
					MethodFromHandler: true,
					PathFromArg:       true,
					HandlerFromArg:    true,
					PathArgIndex:      0,
					HandlerArgIndex:   1,
					RecvTypeRegex:     `^github\.com/gorilla/mux\.\*?(Router|Route)$`,
					MethodExtraction:  DefaultMethodExtractionConfig(),
				},
				{
					CallRegex:         `^Handle$`,
					MethodFromHandler: true,
					PathFromArg:       true,
					HandlerFromArg:    true,
					PathArgIndex:      0,
					HandlerArgIndex:   1,
					RecvTypeRegex:     `^github\.com/gorilla/mux\.\*?(Router|Route)$`,
					MethodExtraction:  DefaultMethodExtractionConfig(),
				},
			},
			RequestBodyPatterns: []RequestBodyPattern{
				{
					CallRegex:     `^Decode$`,
					TypeArgIndex:  0,
					TypeFromArg:   true,
					Deref:         true,
					RecvTypeRegex: ".*json(iter)?\\.\\*?Decoder",
				},
				{
					CallRegex:     `^Unmarshal$`,
					TypeArgIndex:  1,
					TypeFromArg:   true,
					Deref:         true,
					RecvTypeRegex: "json",
				},
			},
			ResponsePatterns: []ResponsePattern{
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
					CallRegex:    `^Marshal$`,
					TypeArgIndex: 0,
					TypeFromArg:  true,
					Deref:        true,
				},
				{
					CallRegex:     `^Encode$`,
					TypeArgIndex:  0,
					TypeFromArg:   true,
					Deref:         true,
					RecvTypeRegex: ".*json(iter)?\\.\\*?Encoder",
				},
			},
			ParamPatterns: []ParamPattern{ // @note: mux does not have a ParamPattern and it's not supported in this version
				{
					CallRegex:     `^Vars$`,
					ParamIn:       "path",
					ParamArgIndex: 0,
					RecvTypeRegex: `^github\.com/gorilla/mux$`,
				},
			},
			MountPatterns: []MountPattern{
				{
					CallRegex:      `^PathPrefix$`,
					PathFromArg:    true,
					RouterFromArg:  true,
					PathArgIndex:   0,
					RouterArgIndex: 1,
					IsMount:        true,
					RecvTypeRegex:  `^github\.com/gorilla/mux\.\*?Router$`,
				},
				{
					CallRegex:      `^Subrouter$`,
					PathFromArg:    false,
					RouterFromArg:  true,
					RouterArgIndex: 0,
					IsMount:        true,
					RecvTypeRegex:  `^github\.com/gorilla/mux\.\*?Route$`,
				},
			},
		},
		Defaults: Defaults{
			RequestContentType:  defaultRequestContentType,
			ResponseContentType: defaultResponseContentType,
			ResponseStatus:      http.StatusOK,
		},
	}
}

// DefaultHTTPConfig returns a default configuration for net/http
func DefaultHTTPConfig() *APISpecConfig {
	return &APISpecConfig{
		Framework: FrameworkConfig{
			RoutePatterns: []RoutePattern{
				{
					CallRegex:       `^HandleFunc$`,
					PathFromArg:     true,
					HandlerFromArg:  true,
					PathArgIndex:    0,
					MethodArgIndex:  -1,
					HandlerArgIndex: 1,
					RecvTypeRegex:   "^net/http(\\.\\*ServeMux)?$",
				},
				{
					CallRegex:       `^Handle$`,
					PathFromArg:     true,
					HandlerFromArg:  true,
					PathArgIndex:    0,
					MethodArgIndex:  -1,
					HandlerArgIndex: 1,
				},
			},
			RequestBodyPatterns: []RequestBodyPattern{
				{
					CallRegex:    `^Decode$`,
					TypeArgIndex: 0,
					TypeFromArg:  true,
					Deref:        true,
				},
				{
					CallRegex:    `^Unmarshal$`,
					TypeArgIndex: 1,
					TypeFromArg:  true,
					Deref:        true,
				},
			},
			ResponsePatterns: []ResponsePattern{
				{
					CallRegex:      `^(?i)(JSON|String|XML|YAML|ProtoBuf|Data|File|Redirect)$`,
					StatusArgIndex: 0,
					TypeArgIndex:   1,
					TypeFromArg:    true,
					Deref:          true,
				},
				{
					CallRegex:    `^Marshal$`,
					TypeArgIndex: 0,
					TypeFromArg:  true,
					Deref:        true,
				},
				{
					CallRegex:    `^Encode$`,
					TypeArgIndex: 0,
					TypeFromArg:  true,
					Deref:        true,
				},
			},
			ParamPatterns: []ParamPattern{
				{
					CallRegex:     "^FormValue$",
					ParamIn:       "form",
					ParamArgIndex: 0,
				},
				{
					CallRegex:     "^Get$",
					ParamIn:       "header",
					ParamArgIndex: 0,
				},
				{
					CallRegex:     "^Cookie$",
					ParamIn:       "cookie",
					ParamArgIndex: 0,
				},
			},
		},
		Defaults: Defaults{
			RequestContentType:  defaultRequestContentType,
			ResponseContentType: defaultResponseContentType,
			ResponseStatus:      http.StatusOK,
		},
	}
}
