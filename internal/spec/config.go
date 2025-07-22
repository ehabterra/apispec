package spec

import (
	"net/http"
	"regexp"
	"strings"
)

const (
	defaultRequestContentType  = "application/json"
	defaultResponseContentType = "application/json"
	defaultResponseStatus      = 200
	primitiveObjectIDType      = "primitive.ObjectID"
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
	MethodFromCall bool `yaml:"methodFromCall,omitempty"` // Extract method from function name
	PathFromArg    bool `yaml:"pathFromArg,omitempty"`    // Extract path from argument
	HandlerFromArg bool `yaml:"handlerFromArg,omitempty"` // Extract handler from argument

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

// SwagenConfig is the main configuration struct
type SwagenConfig struct {
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

// MatchCallRegex checks if the call regex matches
func (p *RoutePattern) MatchCallRegex(callName string) bool {
	return p.MatchPattern(p.CallRegex, callName)
}

// MatchFunctionName checks if the function name regex matches
func (p *RoutePattern) MatchFunctionName(functionName string) bool {
	return p.MatchPattern(p.FunctionNameRegex, functionName)
}

// MatchCallChain checks if a call chain matches
func (p *RoutePattern) MatchCallChain(chain []string, selectors []string) bool {
	if len(chain) == 0 || len(selectors) < len(chain) {
		return false
	}
	for i := 0; i <= len(selectors)-len(chain); i++ {
		matched := true
		for j := 0; j < len(chain); j++ {
			if !strings.Contains(selectors[i+j], chain[j]) {
				matched = false
				break
			}
		}
		if matched {
			return true
		}
	}
	return false
}

// DefaultChiConfig returns a default configuration for Chi router
func DefaultChiConfig() *SwagenConfig {
	return &SwagenConfig{
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
					CallRegex:     `^Decode$`,
					TypeArgIndex:  0,
					TypeFromArg:   true,
					Deref:         true,
					RecvTypeRegex: ".*json(iter)?\\.\\*Decoder",
				},
				{
					CallRegex:     `^Unmarshal$`,
					TypeArgIndex:  0,
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
				},
				{
					CallRegex:    `^Marshal$`,
					TypeArgIndex: 1,
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
func DefaultEchoConfig() *SwagenConfig {
	return &SwagenConfig{
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

// DefaultFiberConfig returns a default configuration for Fiber framework
func DefaultFiberConfig() *SwagenConfig {
	return &SwagenConfig{
		Framework: FrameworkConfig{
			RoutePatterns: []RoutePattern{
				{
					CallRegex:       `^(?i)(GET|POST|PUT|DELETE|PATCH|OPTIONS|HEAD)$`,
					MethodFromCall:  true,
					PathFromArg:     true,
					HandlerFromArg:  true,
					PathArgIndex:    0,
					HandlerArgIndex: 1,
					RecvTypeRegex:   `^github\.com/gofiber/fiber(/v\d)?\.\*(App|Router)$`,
				},
			},
			RequestBodyPatterns: []RequestBodyPattern{
				{
					CallRegex:     `^BodyParser$`,
					TypeArgIndex:  0,
					TypeFromArg:   true,
					Deref:         true,
					RecvTypeRegex: `^github\.com/gofiber/fiber(/v\d)?\.\*Ctx$`,
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
					CallRegex:     `^Group$`,
					PathFromArg:   true,
					RouterFromArg: false,
					PathArgIndex:  0,
					IsMount:       true,
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
				Name: "github.com/gofiber/fiber/v2.Map",
				OpenAPIType: &Schema{
					Type: "object",
				},
			},
		},
	}
}

// DefaultGinConfig returns a default configuration for Gin framework
func DefaultGinConfig() *SwagenConfig {
	return &SwagenConfig{
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
					CallRegex:    `^(?i)(BindJSON|BindXML|BindYAML|BindForm)$`,
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

// DefaultHTTPConfig returns a default configuration for net/http
func DefaultHTTPConfig() *SwagenConfig {
	return &SwagenConfig{
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
