package spec

// OpenAPISpec represents the root OpenAPI specification
type OpenAPISpec struct {
	OpenAPI      string                 `yaml:"openapi" json:"openapi"`
	Info         Info                   `yaml:"info,omitempty" json:"info,omitempty"`
	Servers      []Server               `yaml:"servers,omitempty" json:"servers,omitempty"`
	Paths        map[string]PathItem    `yaml:"paths" json:"paths"`
	Components   *Components            `yaml:"components,omitempty" json:"components,omitempty"`
	Security     []SecurityRequirement  `yaml:"security,omitempty" json:"security,omitempty"`
	Tags         []Tag                  `yaml:"tags,omitempty" json:"tags,omitempty"`
	ExternalDocs *ExternalDocumentation `yaml:"externalDocs,omitempty" json:"externalDocs,omitempty"`
}

// Info represents the OpenAPI info object
type Info struct {
	Title          string   `yaml:"title,omitempty" json:"title,omitempty"`
	TermsOfService string   `yaml:"termsOfService,omitempty" json:"termsOfService,omitempty"`
	Description    string   `yaml:"description,omitempty" json:"description,omitempty"`
	Version        string   `yaml:"version" json:"version"`
	Contact        *Contact `yaml:"contact,omitempty" json:"contact,omitempty"`
	License        *License `yaml:"license,omitempty" json:"license,omitempty"`
}

// Contact represents contact information
type Contact struct {
	Name  string `yaml:"name,omitempty" json:"name,omitempty"`
	URL   string `yaml:"url,omitempty" json:"url,omitempty"`
	Email string `yaml:"email,omitempty" json:"email,omitempty"`
}

// License represents license information
type License struct {
	Name string `yaml:"name" json:"name"`
	URL  string `yaml:"url,omitempty" json:"url,omitempty"`
}

// Server represents a server
type Server struct {
	URL         string                    `yaml:"url" json:"url"`
	Description string                    `yaml:"description,omitempty" json:"description,omitempty"`
	Variables   map[string]ServerVariable `yaml:"variables,omitempty" json:"variables,omitempty"`
}

// ServerVariable represents a server variable
type ServerVariable struct {
	Enum        []string `yaml:"enum,omitempty" json:"enum,omitempty"`
	Default     string   `yaml:"default" json:"default"`
	Description string   `yaml:"description,omitempty" json:"description,omitempty"`
}

// PathItem represents a path item in OpenAPI
type PathItem struct {
	Ref         string      `yaml:"$ref,omitempty" json:"$ref,omitempty"`
	Summary     string      `yaml:"summary,omitempty" json:"summary,omitempty"`
	Description string      `yaml:"description,omitempty" json:"description,omitempty"`
	Get         *Operation  `yaml:"get,omitempty" json:"get,omitempty"`
	Post        *Operation  `yaml:"post,omitempty" json:"post,omitempty"`
	Put         *Operation  `yaml:"put,omitempty" json:"put,omitempty"`
	Delete      *Operation  `yaml:"delete,omitempty" json:"delete,omitempty"`
	Patch       *Operation  `yaml:"patch,omitempty" json:"patch,omitempty"`
	Options     *Operation  `yaml:"options,omitempty" json:"options,omitempty"`
	Head        *Operation  `yaml:"head,omitempty" json:"head,omitempty"`
	Parameters  []Parameter `yaml:"parameters,omitempty" json:"parameters,omitempty"`
}

// Operation represents an OpenAPI operation
type Operation struct {
	Tags         []string               `yaml:"tags,omitempty" json:"tags,omitempty"`
	Summary      string                 `yaml:"summary,omitempty" json:"summary,omitempty"`
	Description  string                 `yaml:"description,omitempty" json:"description,omitempty"`
	OperationID  string                 `yaml:"operationId,omitempty" json:"operationId,omitempty"`
	Parameters   []Parameter            `yaml:"parameters,omitempty" json:"parameters,omitempty"`
	RequestBody  *RequestBody           `yaml:"requestBody,omitempty" json:"requestBody,omitempty"`
	Responses    map[string]Response    `yaml:"responses" json:"responses"`
	Security     []SecurityRequirement  `yaml:"security,omitempty" json:"security,omitempty"`
	ExternalDocs *ExternalDocumentation `yaml:"externalDocs,omitempty" json:"externalDocs,omitempty"`
}

// Parameter represents an OpenAPI parameter
type Parameter struct {
	Name        string                 `yaml:"name" json:"name"`
	In          string                 `yaml:"in" json:"in"`
	Description string                 `yaml:"description,omitempty" json:"description,omitempty"`
	Required    bool                   `yaml:"required,omitempty" json:"required,omitempty"`
	Schema      *Schema                `yaml:"schema,omitempty" json:"schema,omitempty"`
	Example     interface{}            `yaml:"example,omitempty" json:"example,omitempty"`
	Extensions  map[string]interface{} `yaml:",inline" json:",inline"`
}

// RequestBody represents an OpenAPI request body
type RequestBody struct {
	Description string               `yaml:"description,omitempty" json:"description,omitempty"`
	Content     map[string]MediaType `yaml:"content" json:"content"`
	Required    bool                 `yaml:"required,omitempty" json:"required,omitempty"`
}

// Response represents an OpenAPI response
type Response struct {
	Description string               `yaml:"description" json:"description"`
	Headers     map[string]Header    `yaml:"headers,omitempty" json:"headers,omitempty"`
	Content     map[string]MediaType `yaml:"content,omitempty" json:"content,omitempty"`
	Links       map[string]Link      `yaml:"links,omitempty" json:"links,omitempty"`
}

// Header represents an OpenAPI header
type Header struct {
	Description string      `yaml:"description,omitempty" json:"description,omitempty"`
	Schema      *Schema     `yaml:"schema,omitempty" json:"schema,omitempty"`
	Example     interface{} `yaml:"example,omitempty" json:"example,omitempty"`
}

// MediaType represents an OpenAPI media type
type MediaType struct {
	Schema   *Schema             `yaml:"schema,omitempty" json:"schema,omitempty"`
	Example  interface{}         `yaml:"example,omitempty" json:"example,omitempty"`
	Examples map[string]Example  `yaml:"examples,omitempty" json:"examples,omitempty"`
	Encoding map[string]Encoding `yaml:"encoding,omitempty" json:"encoding,omitempty"`
}

// Schema represents an OpenAPI schema
type Schema struct {
	Type                 string                 `yaml:"type,omitempty" json:"type,omitempty"`
	Format               string                 `yaml:"format,omitempty" json:"format,omitempty"`
	Description          string                 `yaml:"description,omitempty" json:"description,omitempty"`
	Title                string                 `yaml:"title,omitempty" json:"title,omitempty"`
	Default              interface{}            `yaml:"default,omitempty" json:"default,omitempty"`
	Example              interface{}            `yaml:"example,omitempty" json:"example,omitempty"`
	ReadOnly             bool                   `yaml:"readOnly,omitempty" json:"readOnly,omitempty"`
	WriteOnly            bool                   `yaml:"writeOnly,omitempty" json:"writeOnly,omitempty"`
	Deprecated           bool                   `yaml:"deprecated,omitempty" json:"deprecated,omitempty"`
	Ref                  string                 `yaml:"$ref,omitempty" json:"$ref,omitempty"`
	AllOf                []*Schema              `yaml:"allOf,omitempty" json:"allOf,omitempty"`
	OneOf                []*Schema              `yaml:"oneOf,omitempty" json:"oneOf,omitempty"`
	AnyOf                []*Schema              `yaml:"anyOf,omitempty" json:"anyOf,omitempty"`
	Not                  *Schema                `yaml:"not,omitempty" json:"not,omitempty"`
	Items                *Schema                `yaml:"items,omitempty" json:"items,omitempty"`
	Properties           map[string]*Schema     `yaml:"properties,omitempty" json:"properties,omitempty"`
	AdditionalProperties *Schema                `yaml:"additionalProperties,omitempty" json:"additionalProperties,omitempty"`
	Required             []string               `yaml:"required,omitempty" json:"required,omitempty"`
	MinLength            int                    `yaml:"minLength,omitempty" json:"minLength,omitempty"`
	MaxLength            int                    `yaml:"maxLength,omitempty" json:"maxLength,omitempty"`
	Pattern              string                 `yaml:"pattern,omitempty" json:"pattern,omitempty"`
	Minimum              float64                `yaml:"minimum,omitempty" json:"minimum,omitempty"`
	Maximum              float64                `yaml:"maximum,omitempty" json:"maximum,omitempty"`
	ExclusiveMinimum     bool                   `yaml:"exclusiveMinimum,omitempty" json:"exclusiveMinimum,omitempty"`
	ExclusiveMaximum     bool                   `yaml:"exclusiveMaximum,omitempty" json:"exclusiveMaximum,omitempty"`
	MultipleOf           float64                `yaml:"multipleOf,omitempty" json:"multipleOf,omitempty"`
	MinItems             int                    `yaml:"minItems,omitempty" json:"minItems,omitempty"`
	MaxItems             int                    `yaml:"maxItems,omitempty" json:"maxItems,omitempty"`
	UniqueItems          bool                   `yaml:"uniqueItems,omitempty" json:"uniqueItems,omitempty"`
	MinProperties        int                    `yaml:"minProperties,omitempty" json:"minProperties,omitempty"`
	MaxProperties        int                    `yaml:"maxProperties,omitempty" json:"maxProperties,omitempty"`
	Enum                 []interface{}          `yaml:"enum,omitempty" json:"enum,omitempty"`
	Discriminator        *Discriminator         `yaml:"discriminator,omitempty" json:"discriminator,omitempty"`
	XML                  *XML                   `yaml:"xml,omitempty" json:"xml,omitempty"`
	ExternalDocs         *ExternalDocumentation `yaml:"externalDocs,omitempty" json:"externalDocs,omitempty"`
}

// Discriminator represents an OpenAPI discriminator
type Discriminator struct {
	PropertyName string            `yaml:"propertyName" json:"propertyName"`
	Mapping      map[string]string `yaml:"mapping,omitempty" json:"mapping,omitempty"`
}

// XML represents XML serialization options
type XML struct {
	Name      string `yaml:"name,omitempty" json:"name,omitempty"`
	Namespace string `yaml:"namespace,omitempty" json:"namespace,omitempty"`
	Prefix    string `yaml:"prefix,omitempty" json:"prefix,omitempty"`
	Attribute bool   `yaml:"attribute,omitempty" json:"attribute,omitempty"`
	Wrapped   bool   `yaml:"wrapped,omitempty" json:"wrapped,omitempty"`
}

// Example represents an OpenAPI example
type Example struct {
	Summary       string      `yaml:"summary,omitempty" json:"summary,omitempty"`
	Description   string      `yaml:"description,omitempty" json:"description,omitempty"`
	Value         interface{} `yaml:"value,omitempty" json:"value,omitempty"`
	ExternalValue string      `yaml:"externalValue,omitempty" json:"externalValue,omitempty"`
}

// Encoding represents an OpenAPI encoding
type Encoding struct {
	ContentType   string            `yaml:"contentType,omitempty" json:"contentType,omitempty"`
	Headers       map[string]Header `yaml:"headers,omitempty" json:"headers,omitempty"`
	Style         string            `yaml:"style,omitempty" json:"style,omitempty"`
	Explode       bool              `yaml:"explode,omitempty" json:"explode,omitempty"`
	AllowReserved bool              `yaml:"allowReserved,omitempty" json:"allowReserved,omitempty"`
}

// Link represents an OpenAPI link
type Link struct {
	OperationRef string                 `yaml:"operationRef,omitempty" json:"operationRef,omitempty"`
	OperationID  string                 `yaml:"operationId,omitempty" json:"operationId,omitempty"`
	Parameters   map[string]interface{} `yaml:"parameters,omitempty" json:"parameters,omitempty"`
	RequestBody  interface{}            `yaml:"requestBody,omitempty" json:"requestBody,omitempty"`
	Description  string                 `yaml:"description,omitempty" json:"description,omitempty"`
	Server       *Server                `yaml:"server,omitempty" json:"server,omitempty"`
}

// Components represents OpenAPI components
type Components struct {
	Schemas         map[string]*Schema        `yaml:"schemas,omitempty" json:"schemas,omitempty"`
	Responses       map[string]*Response      `yaml:"responses,omitempty" json:"responses,omitempty"`
	Parameters      map[string]*Parameter     `yaml:"parameters,omitempty" json:"parameters,omitempty"`
	Examples        map[string]*Example       `yaml:"examples,omitempty" json:"examples,omitempty"`
	RequestBodies   map[string]*RequestBody   `yaml:"requestBodies,omitempty" json:"requestBodies,omitempty"`
	Headers         map[string]*Header        `yaml:"headers,omitempty" json:"headers,omitempty"`
	SecuritySchemes map[string]SecurityScheme `yaml:"securitySchemes,omitempty" json:"securitySchemes,omitempty"`
	Links           map[string]*Link          `yaml:"links,omitempty" json:"links,omitempty"`
	Callbacks       map[string]interface{}    `yaml:"callbacks,omitempty" json:"callbacks,omitempty"`
}

// SecurityScheme represents an OpenAPI security scheme
type SecurityScheme struct {
	Type             string      `yaml:"type" json:"type"`
	Description      string      `yaml:"description,omitempty" json:"description,omitempty"`
	Name             string      `yaml:"name,omitempty" json:"name,omitempty"`
	In               string      `yaml:"in,omitempty" json:"in,omitempty"`
	Scheme           string      `yaml:"scheme,omitempty" json:"scheme,omitempty"`
	BearerFormat     string      `yaml:"bearerFormat,omitempty" json:"bearerFormat,omitempty"`
	Flows            *OAuthFlows `yaml:"flows,omitempty" json:"flows,omitempty"`
	OpenIDConnectURL string      `yaml:"openIdConnectUrl,omitempty" json:"openIdConnectUrl,omitempty"`
}

// OAuthFlows represents OAuth flows
type OAuthFlows struct {
	Implicit          *OAuthFlow `yaml:"implicit,omitempty" json:"implicit,omitempty"`
	Password          *OAuthFlow `yaml:"password,omitempty" json:"password,omitempty"`
	ClientCredentials *OAuthFlow `yaml:"clientCredentials,omitempty" json:"clientCredentials,omitempty"`
	AuthorizationCode *OAuthFlow `yaml:"authorizationCode,omitempty" json:"authorizationCode,omitempty"`
}

// OAuthFlow represents an OAuth flow
type OAuthFlow struct {
	AuthorizationURL string            `yaml:"authorizationUrl,omitempty" json:"authorizationUrl,omitempty"`
	TokenURL         string            `yaml:"tokenUrl,omitempty" json:"tokenUrl,omitempty"`
	RefreshURL       string            `yaml:"refreshUrl,omitempty" json:"refreshUrl,omitempty"`
	Scopes           map[string]string `yaml:"scopes" json:"scopes"`
}

// SecurityRequirement represents a security requirement
type SecurityRequirement map[string][]string

// Tag represents an OpenAPI tag
type Tag struct {
	Name         string                 `yaml:"name" json:"name"`
	Description  string                 `yaml:"description,omitempty" json:"description,omitempty"`
	ExternalDocs *ExternalDocumentation `yaml:"externalDocs,omitempty" json:"externalDocs,omitempty"`
}

// ExternalDocumentation represents external documentation
type ExternalDocumentation struct {
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
	URL         string `yaml:"url" json:"url"`
}
