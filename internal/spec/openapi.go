package spec

import (
	"fmt"
	"go/ast"
	"go/token"
	"net/http"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"github.com/ehabterra/swagen/internal/core"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// OpenAPI 3.1.1 Specification Structures
// Based on https://spec.openapis.org/oas/v3.1.1.html

type OpenAPISpec struct {
	OpenAPI           string                 `json:"openapi" yaml:"openapi"`
	Info              Info                   `json:"info" yaml:"info"`
	JsonSchemaDialect string                 `json:"$schema,omitempty" yaml:"-"`
	Servers           []Server               `json:"servers,omitempty" yaml:"servers,omitempty"`
	Paths             map[string]PathItem    `json:"paths" yaml:"paths"`
	Webhooks          map[string]PathItem    `json:"webhooks,omitempty" yaml:"webhooks,omitempty"`
	Components        *Components            `json:"components,omitempty" yaml:"components,omitempty"`
	Security          []SecurityRequirement  `json:"security,omitempty" yaml:"security,omitempty"`
	Tags              []Tag                  `json:"tags,omitempty" yaml:"tags,omitempty"`
	ExternalDocs      *ExternalDocumentation `json:"externalDocs,omitempty" yaml:"externalDocs,omitempty"`
}

type Info struct {
	Title          string   `json:"title" yaml:"title"`
	Summary        string   `json:"summary,omitempty" yaml:"summary,omitempty"`
	Description    string   `json:"description,omitempty" yaml:"description,omitempty"`
	TermsOfService string   `json:"termsOfService,omitempty" yaml:"termsOfService,omitempty"`
	Contact        *Contact `json:"contact,omitempty" yaml:"contact,omitempty"`
	License        *License `json:"license,omitempty" yaml:"license,omitempty"`
	Version        string   `json:"version" yaml:"version"`
}

type Contact struct {
	Name  string `json:"name,omitempty" yaml:"name,omitempty"`
	URL   string `json:"url,omitempty" yaml:"url,omitempty"`
	Email string `json:"email,omitempty" yaml:"email,omitempty"`
}

type License struct {
	Name       string `json:"name" yaml:"name"`
	Identifier string `json:"identifier,omitempty" yaml:"identifier,omitempty"`
	URL        string `json:"url,omitempty" yaml:"url,omitempty"`
}

type Server struct {
	URL         string                    `json:"url" yaml:"url"`
	Description string                    `json:"description,omitempty" yaml:"description,omitempty"`
	Variables   map[string]ServerVariable `json:"variables,omitempty" yaml:"variables,omitempty"`
}

type ServerVariable struct {
	Enum        []string `json:"enum,omitempty" yaml:"enum,omitempty"`
	Default     string   `json:"default" yaml:"default"`
	Description string   `json:"description,omitempty" yaml:"description,omitempty"`
}

type PathItem struct {
	Ref         string      `json:"$ref,omitempty" yaml:"$ref,omitempty"`
	Summary     string      `json:"summary,omitempty" yaml:"summary,omitempty"`
	Description string      `json:"description,omitempty" yaml:"description,omitempty"`
	Get         *Operation  `json:"get,omitempty" yaml:"get,omitempty"`
	Put         *Operation  `json:"put,omitempty" yaml:"put,omitempty"`
	Post        *Operation  `json:"post,omitempty" yaml:"post,omitempty"`
	Delete      *Operation  `json:"delete,omitempty" yaml:"delete,omitempty"`
	Options     *Operation  `json:"options,omitempty" yaml:"options,omitempty"`
	Head        *Operation  `json:"head,omitempty" yaml:"head,omitempty"`
	Patch       *Operation  `json:"patch,omitempty" yaml:"patch,omitempty"`
	Trace       *Operation  `json:"trace,omitempty" yaml:"trace,omitempty"`
	Servers     []Server    `json:"servers,omitempty" yaml:"servers,omitempty"`
	Parameters  []Parameter `json:"parameters,omitempty" yaml:"parameters,omitempty"`
}

type Operation struct {
	Tags         []string               `json:"tags,omitempty" yaml:"tags,omitempty"`
	Summary      string                 `json:"summary,omitempty" yaml:"summary,omitempty"`
	Description  string                 `json:"description,omitempty" yaml:"description,omitempty"`
	ExternalDocs *ExternalDocumentation `json:"externalDocs,omitempty" yaml:"externalDocs,omitempty"`
	OperationID  string                 `json:"operationId,omitempty" yaml:"operationId,omitempty"`
	Parameters   []Parameter            `json:"parameters,omitempty" yaml:"parameters,omitempty"`
	RequestBody  *RequestBody           `json:"requestBody,omitempty" yaml:"requestBody,omitempty"`
	Responses    map[string]Response    `json:"responses" yaml:"responses"`
	Callbacks    map[string]Callback    `json:"callbacks,omitempty" yaml:"callbacks,omitempty"`
	Deprecated   bool                   `json:"deprecated,omitempty" yaml:"deprecated,omitempty"`
	Security     []SecurityRequirement  `json:"security,omitempty" yaml:"security,omitempty"`
	Servers      []Server               `json:"servers,omitempty" yaml:"servers,omitempty"`
}

type Parameter struct {
	Name            string               `json:"name" yaml:"name"`
	In              string               `json:"in" yaml:"in"`
	Description     string               `json:"description,omitempty" yaml:"description,omitempty"`
	Required        bool                 `json:"required,omitempty" yaml:"required,omitempty"`
	Deprecated      bool                 `json:"deprecated,omitempty" yaml:"deprecated,omitempty"`
	AllowEmptyValue bool                 `json:"allowEmptyValue,omitempty" yaml:"allowEmptyValue,omitempty"`
	Style           string               `json:"style,omitempty" yaml:"style,omitempty"`
	Explode         *bool                `json:"explode,omitempty" yaml:"explode,omitempty"`
	AllowReserved   bool                 `json:"allowReserved,omitempty" yaml:"allowReserved,omitempty"`
	Schema          *Schema              `json:"schema,omitempty" yaml:"schema,omitempty"`
	Example         interface{}          `json:"example,omitempty" yaml:"example,omitempty"`
	Examples        map[string]Example   `json:"examples,omitempty" yaml:"examples,omitempty"`
	Content         map[string]MediaType `json:"content,omitempty" yaml:"content,omitempty"`
}

type RequestBody struct {
	Description string               `json:"description,omitempty" yaml:"description,omitempty"`
	Content     map[string]MediaType `json:"content" yaml:"content"`
	Required    bool                 `json:"required,omitempty" yaml:"required,omitempty"`
}

type MediaType struct {
	Schema   *Schema             `json:"schema,omitempty" yaml:"schema,omitempty"`
	Example  interface{}         `json:"example,omitempty" yaml:"example,omitempty"`
	Examples map[string]Example  `json:"examples,omitempty" yaml:"examples,omitempty"`
	Encoding map[string]Encoding `json:"encoding,omitempty" yaml:"encoding,omitempty"`
}

type Encoding struct {
	ContentType   string            `json:"contentType,omitempty" yaml:"contentType,omitempty"`
	Headers       map[string]Header `json:"headers,omitempty" yaml:"headers,omitempty"`
	Style         string            `json:"style,omitempty" yaml:"style,omitempty"`
	Explode       *bool             `json:"explode,omitempty" yaml:"explode,omitempty"`
	AllowReserved bool              `json:"allowReserved,omitempty" yaml:"allowReserved,omitempty"`
}

type Response struct {
	Description string               `json:"description" yaml:"description"`
	Headers     map[string]Header    `json:"headers,omitempty" yaml:"headers,omitempty"`
	Content     map[string]MediaType `json:"content,omitempty" yaml:"content,omitempty"`
	Links       map[string]Link      `json:"links,omitempty" yaml:"links,omitempty"`
}

type Header struct {
	Description     string               `json:"description,omitempty" yaml:"description,omitempty"`
	Required        bool                 `json:"required,omitempty" yaml:"required,omitempty"`
	Deprecated      bool                 `json:"deprecated,omitempty" yaml:"deprecated,omitempty"`
	AllowEmptyValue bool                 `json:"allowEmptyValue,omitempty" yaml:"allowEmptyValue,omitempty"`
	Style           string               `json:"style,omitempty" yaml:"style,omitempty"`
	Explode         *bool                `json:"explode,omitempty" yaml:"explode,omitempty"`
	AllowReserved   bool                 `json:"allowReserved,omitempty" yaml:"allowReserved,omitempty"`
	Schema          *Schema              `json:"schema,omitempty" yaml:"schema,omitempty"`
	Example         interface{}          `json:"example,omitempty" yaml:"example,omitempty"`
	Examples        map[string]Example   `json:"examples,omitempty" yaml:"examples,omitempty"`
	Content         map[string]MediaType `json:"content,omitempty" yaml:"content,omitempty"`
}

type Schema struct {
	// JSON Schema 2020-12 Core
	Schema        string             `json:"$schema,omitempty" yaml:"$schema,omitempty"`
	Vocabulary    string             `json:"$vocabulary,omitempty" yaml:"$vocabulary,omitempty"`
	ID            string             `json:"$id,omitempty" yaml:"$id,omitempty"`
	Ref           string             `json:"$ref,omitempty" yaml:"$ref,omitempty"`
	DynamicRef    string             `json:"$dynamicRef,omitempty" yaml:"$dynamicRef,omitempty"`
	DynamicAnchor string             `json:"$dynamicAnchor,omitempty" yaml:"$dynamicAnchor,omitempty"`
	Defs          map[string]*Schema `json:"$defs,omitempty" yaml:"$defs,omitempty"`
	Comment       string             `json:"$comment,omitempty" yaml:"$comment,omitempty"`

	// JSON Schema Validation
	AllOf []Schema `json:"allOf,omitempty" yaml:"allOf,omitempty"`
	AnyOf []Schema `json:"anyOf,omitempty" yaml:"anyOf,omitempty"`
	OneOf []Schema `json:"oneOf,omitempty" yaml:"oneOf,omitempty"`
	Not   *Schema  `json:"not,omitempty" yaml:"not,omitempty"`
	If    *Schema  `json:"if,omitempty" yaml:"if,omitempty"`
	Then  *Schema  `json:"then,omitempty" yaml:"then,omitempty"`
	Else  *Schema  `json:"else,omitempty" yaml:"else,omitempty"`

	// Type-specific validation
	Type              interface{}         `json:"type,omitempty" yaml:"type,omitempty"` // string or []string
	Enum              []interface{}       `json:"enum,omitempty" yaml:"enum,omitempty"`
	Const             interface{}         `json:"const,omitempty" yaml:"const,omitempty"`
	MultipleOf        *float64            `json:"multipleOf,omitempty" yaml:"multipleOf,omitempty"`
	Maximum           *float64            `json:"maximum,omitempty" yaml:"maximum,omitempty"`
	ExclusiveMaximum  *float64            `json:"exclusiveMaximum,omitempty" yaml:"exclusiveMaximum,omitempty"`
	Minimum           *float64            `json:"minimum,omitempty" yaml:"minimum,omitempty"`
	ExclusiveMinimum  *float64            `json:"exclusiveMinimum,omitempty" yaml:"exclusiveMinimum,omitempty"`
	MaxLength         *int                `json:"maxLength,omitempty" yaml:"maxLength,omitempty"`
	MinLength         *int                `json:"minLength,omitempty" yaml:"minLength,omitempty"`
	Pattern           string              `json:"pattern,omitempty" yaml:"pattern,omitempty"`
	MaxItems          *int                `json:"maxItems,omitempty" yaml:"maxItems,omitempty"`
	MinItems          *int                `json:"minItems,omitempty" yaml:"minItems,omitempty"`
	UniqueItems       bool                `json:"uniqueItems,omitempty" yaml:"uniqueItems,omitempty"`
	MaxContains       *int                `json:"maxContains,omitempty" yaml:"maxContains,omitempty"`
	MinContains       *int                `json:"minContains,omitempty" yaml:"minContains,omitempty"`
	MaxProperties     *int                `json:"maxProperties,omitempty" yaml:"maxProperties,omitempty"`
	MinProperties     *int                `json:"minProperties,omitempty" yaml:"minProperties,omitempty"`
	Required          []string            `json:"required,omitempty" yaml:"required,omitempty"`
	DependentRequired map[string][]string `json:"dependentRequired,omitempty" yaml:"dependentRequired,omitempty"`

	// Object/Array schemas
	Properties            map[string]*Schema `json:"properties,omitempty" yaml:"properties,omitempty"`
	PatternProperties     map[string]*Schema `json:"patternProperties,omitempty" yaml:"patternProperties,omitempty"`
	AdditionalProperties  interface{}        `json:"additionalProperties,omitempty" yaml:"additionalProperties,omitempty"` // bool or Schema
	DependentSchemas      map[string]*Schema `json:"dependentSchemas,omitempty" yaml:"dependentSchemas,omitempty"`
	PropertyNames         *Schema            `json:"propertyNames,omitempty" yaml:"propertyNames,omitempty"`
	UnevaluatedItems      interface{}        `json:"unevaluatedItems,omitempty" yaml:"unevaluatedItems,omitempty"`           // bool or Schema
	UnevaluatedProperties interface{}        `json:"unevaluatedProperties,omitempty" yaml:"unevaluatedProperties,omitempty"` // bool or Schema
	Items                 interface{}        `json:"items,omitempty" yaml:"items,omitempty"`                                 // Schema or []Schema
	PrefixItems           []Schema           `json:"prefixItems,omitempty" yaml:"prefixItems,omitempty"`
	Contains              *Schema            `json:"contains,omitempty" yaml:"contains,omitempty"`

	// String formats (OpenAPI extensions)
	Format string `json:"format,omitempty" yaml:"format,omitempty"`

	// Metadata
	Title       string        `json:"title,omitempty" yaml:"title,omitempty"`
	Description string        `json:"description,omitempty" yaml:"description,omitempty"`
	Default     interface{}   `json:"default,omitempty" yaml:"default,omitempty"`
	Examples    []interface{} `json:"examples,omitempty" yaml:"examples,omitempty"`
	ReadOnly    bool          `json:"readOnly,omitempty" yaml:"readOnly,omitempty"`
	WriteOnly   bool          `json:"writeOnly,omitempty" yaml:"writeOnly,omitempty"`
	Deprecated  bool          `json:"deprecated,omitempty" yaml:"deprecated,omitempty"`

	// OpenAPI-specific extensions
	Discriminator *Discriminator         `json:"discriminator,omitempty" yaml:"discriminator,omitempty"`
	XML           *XML                   `json:"xml,omitempty" yaml:"xml,omitempty"`
	ExternalDocs  *ExternalDocumentation `json:"externalDocs,omitempty" yaml:"externalDocs,omitempty"`
	Example       interface{}            `json:"example,omitempty" yaml:"example,omitempty"`
}

type Discriminator struct {
	PropertyName string            `json:"propertyName" yaml:"propertyName"`
	Mapping      map[string]string `json:"mapping,omitempty" yaml:"mapping,omitempty"`
}

type XML struct {
	Name      string `json:"name,omitempty" yaml:"name,omitempty"`
	Namespace string `json:"namespace,omitempty" yaml:"namespace,omitempty"`
	Prefix    string `json:"prefix,omitempty" yaml:"prefix,omitempty"`
	Attribute bool   `json:"attribute,omitempty" yaml:"attribute,omitempty"`
	Wrapped   bool   `json:"wrapped,omitempty" yaml:"wrapped,omitempty"`
}

type Example struct {
	Summary       string      `json:"summary,omitempty" yaml:"summary,omitempty"`
	Description   string      `json:"description,omitempty" yaml:"description,omitempty"`
	Value         interface{} `json:"value,omitempty" yaml:"value,omitempty"`
	ExternalValue string      `json:"externalValue,omitempty" yaml:"externalValue,omitempty"`
}

type Link struct {
	OperationRef string                 `json:"operationRef,omitempty" yaml:"operationRef,omitempty"`
	OperationID  string                 `json:"operationId,omitempty" yaml:"operationId,omitempty"`
	Parameters   map[string]interface{} `json:"parameters,omitempty" yaml:"parameters,omitempty"`
	RequestBody  interface{}            `json:"requestBody,omitempty" yaml:"requestBody,omitempty"`
	Description  string                 `json:"description,omitempty" yaml:"description,omitempty"`
	Server       *Server                `json:"server,omitempty" yaml:"server,omitempty"`
}

type Callback map[string]PathItem

type Components struct {
	Schemas         map[string]*Schema        `json:"schemas,omitempty" yaml:"schemas,omitempty"`
	Responses       map[string]Response       `json:"responses,omitempty" yaml:"responses,omitempty"`
	Parameters      map[string]Parameter      `json:"parameters,omitempty" yaml:"parameters,omitempty"`
	Examples        map[string]Example        `json:"examples,omitempty" yaml:"examples,omitempty"`
	RequestBodies   map[string]RequestBody    `json:"requestBodies,omitempty" yaml:"requestBodies,omitempty"`
	Headers         map[string]Header         `json:"headers,omitempty" yaml:"headers,omitempty"`
	SecuritySchemes map[string]SecurityScheme `json:"securitySchemes,omitempty" yaml:"securitySchemes,omitempty"`
	Links           map[string]Link           `json:"links,omitempty" yaml:"links,omitempty"`
	Callbacks       map[string]Callback       `json:"callbacks,omitempty" yaml:"callbacks,omitempty"`
	PathItems       map[string]PathItem       `json:"pathItems,omitempty" yaml:"pathItems,omitempty"`
}

type SecurityScheme struct {
	Type             string      `json:"type" yaml:"type"`
	Description      string      `json:"description,omitempty" yaml:"description,omitempty"`
	Name             string      `json:"name,omitempty" yaml:"name,omitempty"`
	In               string      `json:"in,omitempty" yaml:"in,omitempty"`
	Scheme           string      `json:"scheme,omitempty" yaml:"scheme,omitempty"`
	BearerFormat     string      `json:"bearerFormat,omitempty" yaml:"bearerFormat,omitempty"`
	Flows            *OAuthFlows `json:"flows,omitempty" yaml:"flows,omitempty"`
	OpenIDConnectURL string      `json:"openIdConnectUrl,omitempty" yaml:"openIdConnectUrl,omitempty"`
}

type OAuthFlows struct {
	Implicit          *OAuthFlow `json:"implicit,omitempty" yaml:"implicit,omitempty"`
	Password          *OAuthFlow `json:"password,omitempty" yaml:"password,omitempty"`
	ClientCredentials *OAuthFlow `json:"clientCredentials,omitempty" yaml:"clientCredentials,omitempty"`
	AuthorizationCode *OAuthFlow `json:"authorizationCode,omitempty" yaml:"authorizationCode,omitempty"`
}

type OAuthFlow struct {
	AuthorizationURL string            `json:"authorizationUrl,omitempty" yaml:"authorizationUrl,omitempty"`
	TokenURL         string            `json:"tokenUrl,omitempty" yaml:"tokenUrl,omitempty"`
	RefreshURL       string            `json:"refreshUrl,omitempty" yaml:"refreshUrl,omitempty"`
	Scopes           map[string]string `json:"scopes" yaml:"scopes"`
}

type SecurityRequirement map[string][]string

type Tag struct {
	Name         string                 `json:"name" yaml:"name"`
	Description  string                 `json:"description,omitempty" yaml:"description,omitempty"`
	ExternalDocs *ExternalDocumentation `json:"externalDocs,omitempty" yaml:"externalDocs,omitempty"`
}

type ExternalDocumentation struct {
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
	URL         string `json:"url" yaml:"url"`
}

// Route represents a parsed API route from Go code
type ParsedRoute struct {
	Method        string
	Path          string
	Handler       string
	HandlerFunc   *ast.FuncDecl
	RequestType   string
	RequestSource string
	ResponseTypes []ResponseInfo
	Summary       string
	Description   string
	Tags          []string
	Parameters    []ParsedParameter
	Middleware    []string
	File          string
	Line          int
}

// ResponseInfo represents response information for OpenAPI spec
type ResponseInfo struct {
	StatusCode int
	Type       string
	MediaType  string
	MapKeys    map[string]string
}

type ParsedParameter struct {
	Name     string
	Type     string
	Source   string
	Required bool
	Example  string
}

// GeneratorConfig holds configuration for the OpenAPI generator.
type GeneratorConfig struct {
	OpenAPIVersion string
	Title          string
	Description    string
	APIVersion     string
	TermsOfService string
	ContactName    string
	ContactURL     string
	ContactEmail   string
	LicenseName    string
	LicenseURL     string
}

// OpenAPI Generator
type OpenAPIGenerator struct {
	spec        *OpenAPISpec
	components  *Components
	typeSchemas map[string]*Schema // Cache for Go type -> Schema conversion
}

func NewOpenAPIGenerator(config GeneratorConfig) *OpenAPIGenerator {
	info := Info{
		Title:          config.Title,
		Description:    config.Description,
		TermsOfService: config.TermsOfService,
		Version:        config.APIVersion,
	}
	if config.ContactName != "" || config.ContactURL != "" || config.ContactEmail != "" {
		info.Contact = &Contact{
			Name:  config.ContactName,
			URL:   config.ContactURL,
			Email: config.ContactEmail,
		}
	}
	if config.LicenseName != "" {
		info.License = &License{
			Name: config.LicenseName,
			URL:  config.LicenseURL,
		}
	}

	return &OpenAPIGenerator{
		spec: &OpenAPISpec{
			OpenAPI:           config.OpenAPIVersion,
			JsonSchemaDialect: "https://json-schema.org/draft/2020-12/schema",
			Info:              info,
			Paths:             make(map[string]PathItem),
		},
		components: &Components{
			Schemas: make(map[string]*Schema),
		},
		typeSchemas: make(map[string]*Schema),
	}
}

func (g *OpenAPIGenerator) GenerateFromRoutes(routes []core.ParsedRoute, goFiles []*ast.File) (*OpenAPISpec, error) {
	// Set up components
	g.spec.Components = g.components

	// Process each route
	for _, route := range routes {
		pathItem := g.spec.Paths[route.Path]

		operation := g.convertRouteToOperation(route, goFiles)

		// Add operation to path item
		switch strings.ToUpper(route.Method) {
		case "GET":
			pathItem.Get = operation
		case "POST":
			pathItem.Post = operation
		case "PUT":
			pathItem.Put = operation
		case "DELETE":
			pathItem.Delete = operation
		case "PATCH":
			pathItem.Patch = operation
		case "HEAD":
			pathItem.Head = operation
		case "OPTIONS":
			pathItem.Options = operation
		}

		g.spec.Paths[route.Path] = pathItem
	}

	return g.spec, nil
}

func (g *OpenAPIGenerator) convertRouteToOperation(route core.ParsedRoute, goFiles []*ast.File) *Operation {
	operation := &Operation{
		Summary:     "",         // No summary field in core.ParsedRoute
		Description: "",         // No description field in core.ParsedRoute
		Tags:        []string{}, // No tags field in core.ParsedRoute
		OperationID: g.generateOperationID(route),
		Parameters:  g.extractParameters(route, goFiles),
		Responses:   make(map[string]Response),
	}

	// Add request body if present
	if route.RequestType != "" {
		operation.RequestBody = g.createRequestBody(route, goFiles)
	}

	// Add responses
	for _, respInfo := range route.ResponseTypes {
		// Convert core.ResponseInfo to spec.ResponseInfo
		specRespInfo := ResponseInfo{
			StatusCode: respInfo.StatusCode,
			Type:       respInfo.Type,
			MediaType:  respInfo.MediaType,
			MapKeys:    respInfo.MapKeys,
		}
		response := g.createResponse(specRespInfo, goFiles)
		operation.Responses[fmt.Sprintf("%d", respInfo.StatusCode)] = response
	}

	return operation
}

func (g *OpenAPIGenerator) generateOperationID(route core.ParsedRoute) string {
	// If we have a handler name, use it directly
	if route.HandlerName != "" {
		return route.HandlerName
	}

	// Fallback: Generate operation ID from method and path
	operationID := strings.ToLower(route.Method)

	// Clean path and convert to camelCase
	path := strings.ReplaceAll(route.Path, "/", "_")
	path = strings.ReplaceAll(path, "{", "")
	path = strings.ReplaceAll(path, "}", "")
	path = strings.ReplaceAll(path, ":", "")

	parts := strings.Split(path, "_")
	for _, part := range parts {
		if part != "" {
			operationID += cases.Title(language.Und).String(part)
		}
	}

	return operationID
}

func (g *OpenAPIGenerator) extractParameters(route core.ParsedRoute, _ []*ast.File) []Parameter {
	var parameters []Parameter

	// Extract path parameters from the route path
	pathParams := g.extractPathParameters(route.Path)
	for _, param := range pathParams {
		parameters = append(parameters, Parameter{
			Name:        param,
			In:          "path",
			Required:    true,
			Description: fmt.Sprintf("Path parameter: %s", param),
			Schema: &Schema{
				Type: "string",
			},
		})
	}

	// No Parameters field in core.ParsedRoute, so we only extract from path
	return parameters
}

func (g *OpenAPIGenerator) extractPathParameters(path string) []string {
	var params []string

	// Extract {param} style parameters
	re := regexp.MustCompile(`\{([^}]+)\}`)
	matches := re.FindAllStringSubmatch(path, -1)
	for _, match := range matches {
		params = append(params, match[1])
	}

	// Extract :param style parameters (Gin style)
	re = regexp.MustCompile(`:([^/]+)`)
	matches = re.FindAllStringSubmatch(path, -1)
	for _, match := range matches {
		params = append(params, match[1])
	}

	return params
}

func (g *OpenAPIGenerator) createRequestBody(route core.ParsedRoute, goFiles []*ast.File) *RequestBody {
	schema := g.findSchemaForType(route.RequestType, goFiles, nil, make(map[string]bool))

	return &RequestBody{
		Description: fmt.Sprintf("Request body for %s", route.HandlerName),
		Content: map[string]MediaType{
			"application/json": {
				Schema: schema,
			},
		},
		Required: true,
	}
}

func (g *OpenAPIGenerator) createResponse(respInfo ResponseInfo, goFiles []*ast.File) Response {
	response := Response{
		Description: g.getStatusDescription(respInfo.StatusCode),
	}

	// For 204 No Content, do not emit content
	if respInfo.StatusCode == http.StatusNoContent {
		return response
	}

	// Inline a recursive object schema for any map type (gin.H, map[string]interface{}, map[string]any, map[string]string, etc.)
	if strings.HasPrefix(respInfo.Type, "map[") {
		var example map[string]interface{}
		if len(respInfo.MapKeys) > 0 {
			example = make(map[string]interface{})
			for k, typ := range respInfo.MapKeys {
				example[k] = exampleValueForType(typ)
			}
		} else {
			example = map[string]interface{}{"exampleKey": "exampleValue"}
		}
		anySchema := &Schema{
			Type:                 "object",
			AdditionalProperties: true,
			Example:              example,
		}
		response.Content = map[string]MediaType{
			respInfo.MediaType: {
				Schema: anySchema,
			},
		}
		return response
	}

	if len(respInfo.MapKeys) > 0 {
		props := make(map[string]*Schema)
		example := make(map[string]interface{})
		visited := make(map[string]bool)
		for k, typ := range respInfo.MapKeys {
			props[k] = g.findSchemaForType(typ, goFiles, nil, visited)
			example[k] = exampleValueForType(typ)
		}
		schema := &Schema{
			Type:       "object",
			Properties: props,
			Example:    example,
		}
		response.Content = map[string]MediaType{
			respInfo.MediaType: {Schema: schema},
		}
		return response
	}

	if respInfo.Type != "" && respInfo.Type != "string" {
		schema := g.findSchemaForType(respInfo.Type, goFiles, nil, make(map[string]bool))
		response.Content = map[string]MediaType{
			respInfo.MediaType: {
				Schema: schema,
			},
		}
	}

	return response
}

// exampleValueForType returns a simple example value for a given type name
func exampleValueForType(typ string) interface{} {
	switch typ {
	case "string":
		return "example"
	case "int", "int8", "int16", "int32", "int64":
		return 123
	case "float32", "float64":
		return 1.23
	case "bool":
		return true
	default:
		return nil
	}
}

func (g *OpenAPIGenerator) getStatusDescription(code int) string {
	descriptions := map[int]string{
		200: "OK",
		201: "Created",
		202: "Accepted",
		204: "No Content",
		400: "Bad Request",
		401: "Unauthorized",
		403: "Forbidden",
		404: "Not Found",
		405: "Method Not Allowed",
		409: "Conflict",
		422: "Unprocessable Entity",
		500: "Internal Server Error",
	}

	if desc, exists := descriptions[code]; exists {
		return desc
	}
	return fmt.Sprintf("Status %d", code)
}

// Add a helper to resolve the canonical type name and ast.Expr for a type alias
func resolveCanonicalType(typeName string, goFiles []*ast.File, visited map[string]bool) (string, ast.Expr) {
	if visited == nil {
		visited = make(map[string]bool)
	}
	if visited[typeName] {
		return typeName, nil // Prevent infinite recursion
	}
	visited[typeName] = true

	for _, file := range goFiles {
		for _, decl := range file.Decls {
			if genDecl, ok := decl.(*ast.GenDecl); ok && genDecl.Tok == token.TYPE {
				for _, spec := range genDecl.Specs {
					if typeSpec, ok := spec.(*ast.TypeSpec); ok {
						if typeSpec.Name.Name == typeName {
							switch t := typeSpec.Type.(type) {
							case *ast.Ident:
								// Recurse to resolve further
								return resolveCanonicalType(t.Name, goFiles, visited)
							case *ast.SelectorExpr:
								if x, ok := t.X.(*ast.Ident); ok {
									return resolveCanonicalType(x.Name+"."+t.Sel.Name, goFiles, visited)
								}
								return typeName, t
							default:
								// This is the base type (struct, map, etc.)
								return typeSpec.Name.Name, typeSpec.Type
							}
						}
					}
				}
			}
		}
	}
	// If not found, return the original typeName (for external types)
	return typeName, nil
}

// Add this helper at the top-level
func sanitizeSchemaName(typeName string) string {
	// Only remove invalid OpenAPI component key characters, but preserve case
	// Allow letters, numbers, underscores
	var out strings.Builder
	for _, r := range typeName {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			out.WriteRune(r)
		}
	}
	return out.String()
}

// In findSchemaForType, use sanitizeSchemaName for canonicalName
func (g *OpenAPIGenerator) findSchemaForType(typeName string, goFiles []*ast.File, mapKeys map[string]string, visited map[string]bool) *Schema {
	if visited == nil {
		visited = make(map[string]bool)
	}
	if visited[typeName] {
		return &Schema{Type: "object"}
	}
	visited[typeName] = true
	// Handle slice/array types as inline schemas
	if strings.HasPrefix(typeName, "[]") {
		itemType := strings.TrimPrefix(typeName, "[]")
		return &Schema{
			Type:  "array",
			Items: g.findSchemaForType(itemType, goFiles, nil, visited),
		}
	}

	// Handle primitive types first
	if schema := g.goTypeToPrimitiveSchema(typeName); schema != nil {
		return schema
	}

	// Canonicalize type name
	canonicalName, typeExpr := resolveCanonicalType(typeName, goFiles, make(map[string]bool))
	canonicalName = sanitizeSchemaName(canonicalName)

	// Check if schema already exists to avoid circular references
	if _, exists := g.components.Schemas[canonicalName]; exists {
		return &Schema{Ref: fmt.Sprintf("#/components/schemas/%s", canonicalName)}
	}

	// Create a placeholder to prevent circular references
	g.components.Schemas[canonicalName] = &Schema{}

	if typeExpr != nil {
		schema := g.astTypeToSchema(typeExpr, goFiles, visited)
		*g.components.Schemas[canonicalName] = *schema
		return &Schema{Ref: fmt.Sprintf("#/components/schemas/%s", canonicalName)}
	}

	if len(mapKeys) > 0 {
		props := make(map[string]*Schema)
		example := make(map[string]interface{})
		for k, typ := range mapKeys {
			if primitiveSchema := g.goTypeToPrimitiveSchema(typ); primitiveSchema != nil {
				props[k] = primitiveSchema
			} else if typ == "string" {
				props[k] = &Schema{Type: "string"}
			} else {
				props[k] = g.findSchemaForType(typ, goFiles, nil, visited)
			}
			example[k] = exampleValueForType(typ)
		}
		*g.components.Schemas[canonicalName] = Schema{
			Type:       "object",
			Properties: props,
			Example:    example,
		}
		return &Schema{Ref: fmt.Sprintf("#/components/schemas/%s", canonicalName)}
	}

	*g.components.Schemas[canonicalName] = Schema{
		Type:                 "object",
		AdditionalProperties: true,
		Example:              map[string]interface{}{"exampleKey": "exampleValue"},
	}
	return &Schema{Ref: fmt.Sprintf("#/components/schemas/%s", canonicalName)}
}

func (g *OpenAPIGenerator) astTypeToSchema(expr ast.Expr, goFiles []*ast.File, visited map[string]bool) *Schema {
	if visited == nil {
		visited = make(map[string]bool)
	}

	switch t := expr.(type) {
	case *ast.Ident:
		// For primitive types, don't apply recursion protection
		if primitiveSchema := g.goTypeToPrimitiveSchema(t.Name); primitiveSchema != nil {
			return primitiveSchema
		}

		// Check if we've already visited this type to prevent infinite recursion
		if visited[t.Name] {
			// Return a reference to avoid infinite recursion
			return &Schema{Ref: fmt.Sprintf("#/components/schemas/%s", sanitizeSchemaName(t.Name))}
		}
		visited[t.Name] = true
		return g.findSchemaForType(t.Name, goFiles, nil, visited)
	case *ast.ArrayType:
		return &Schema{
			Type:  "array",
			Items: g.astTypeToSchema(t.Elt, goFiles, visited),
		}
	case *ast.MapType:
		return &Schema{
			Type:                 "object",
			AdditionalProperties: g.astTypeToSchema(t.Value, goFiles, visited),
		}
	case *ast.StarExpr:
		return g.astTypeToSchema(t.X, goFiles, visited)
	case *ast.SelectorExpr:
		if ident, ok := t.X.(*ast.Ident); ok {
			fullName := ident.Name + "." + t.Sel.Name
			if visited[fullName] {
				return &Schema{Ref: fmt.Sprintf("#/components/schemas/%s", sanitizeSchemaName(fullName))}
			}
			visited[fullName] = true
			return g.findSchemaForType(fullName, goFiles, nil, visited)
		}
	case *ast.StructType:
		schema := &Schema{
			Type:       "object",
			Properties: make(map[string]*Schema),
		}
		var required []string
		for _, field := range t.Fields.List {
			for _, name := range field.Names {
				fieldName := name.Name
				if !ast.IsExported(fieldName) {
					continue
				}
				jsonName := fieldName
				isRequired := false
				if field.Tag != nil {
					tag := strings.Trim(field.Tag.Value, "`")
					if jsonTag := reflect.StructTag(tag).Get("json"); jsonTag != "" {
						parts := strings.Split(jsonTag, ",")
						if parts[0] != "" && parts[0] != "-" {
							jsonName = parts[0]
						}
						for _, part := range parts[1:] {
							if part == "required" || !strings.Contains(part, "omitempty") {
								isRequired = true
							}
						}
					}
					if bindingTag := reflect.StructTag(tag).Get("binding"); bindingTag != "" {
						if strings.Contains(bindingTag, "required") {
							isRequired = true
						}
					}
				}
				fieldSchema := g.astTypeToSchema(field.Type, goFiles, visited)
				if field.Tag != nil {
					g.addValidationFromTags(fieldSchema, field.Tag.Value)
				}
				schema.Properties[jsonName] = fieldSchema
				if isRequired {
					required = append(required, jsonName)
				}
			}
		}
		if len(required) > 0 {
			schema.Required = required
		}
		return schema
	default:
		// For any other type, return a generic object to avoid infinite recursion
		return &Schema{Type: "object"}
	}
	return &Schema{Type: "object"}
}

func (g *OpenAPIGenerator) goTypeToPrimitiveSchema(goType string) *Schema {
	switch goType {
	case "string":
		return &Schema{Type: "string"}
	case "int", "int8", "int16", "int32", "int64":
		return &Schema{Type: "integer"}
	case "uint", "uint8", "uint16", "uint32", "uint64":
		return &Schema{Type: "integer", Minimum: float64Ptr(0)}
	case "float32", "float64":
		return &Schema{Type: "number"}
	case "bool":
		return &Schema{Type: "boolean"}
	case "time.Time":
		return &Schema{Type: "string", Format: "date-time"}
	case "[]byte":
		return &Schema{Type: "string", Format: "byte"}
	case "interface{}", "any":
		return &Schema{Type: "object", AdditionalProperties: true}
	default:
		return nil // Not a primitive type
	}
}

func (g *OpenAPIGenerator) addValidationFromTags(schema *Schema, tagValue string) {
	tag := strings.Trim(tagValue, "`")
	structTag := reflect.StructTag(tag)

	// Handle validate tag
	if validateTag := structTag.Get("validate"); validateTag != "" {
		rules := strings.Split(validateTag, ",")
		for _, rule := range rules {
			g.applyValidationRule(schema, strings.TrimSpace(rule))
		}
	}

	// Handle binding tag
	if bindingTag := structTag.Get("binding"); bindingTag != "" {
		rules := strings.Split(bindingTag, ",")
		for _, rule := range rules {
			g.applyValidationRule(schema, strings.TrimSpace(rule))
		}
	}
}

func (g *OpenAPIGenerator) applyValidationRule(schema *Schema, rule string) {
	if rule == "required" {
		// This is handled at the struct level
		return
	}

	if rule == "email" {
		schema.Format = "email"
	}

	if rule == "url" {
		schema.Format = "uri"
	}

	if strings.HasPrefix(rule, "min=") {
		if val, err := strconv.ParseFloat(rule[4:], 64); err == nil {
			if schema.Type == "string" {
				schema.MinLength = intPtr(int(val))
			} else {
				schema.Minimum = &val
			}
		}
	}

	if strings.HasPrefix(rule, "max=") {
		if val, err := strconv.ParseFloat(rule[4:], 64); err == nil {
			if schema.Type == "string" {
				schema.MaxLength = intPtr(int(val))
			} else {
				schema.Maximum = &val
			}
		}
	}

	if strings.HasPrefix(rule, "len=") {
		if val, err := strconv.Atoi(rule[4:]); err == nil {
			switch schema.Type {
			case "string":
				schema.MinLength = intPtr(val)
				schema.MaxLength = intPtr(val)
			case "array":
				schema.MinItems = intPtr(val)
				schema.MaxItems = intPtr(val)
			}
		}
	}

	if strings.HasPrefix(rule, "oneof=") {
		// e.g. oneof=foo bar baz
		values := strings.Fields(rule[6:])
		var enumVals []interface{}
		for _, v := range values {
			enumVals = append(enumVals, v)
		}
		schema.Enum = enumVals
	}
}

// Utility helpers
func intPtr(i int) *int {
	return &i
}

func float64Ptr(f float64) *float64 {
	return &f
}
