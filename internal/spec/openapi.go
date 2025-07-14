package spec

// OpenAPISpec represents the root OpenAPI specification
type OpenAPISpec struct {
	OpenAPI      string                 `yaml:"openapi"`
	Info         Info                   `yaml:"info,omitempty"`
	Servers      []Server               `yaml:"servers,omitempty"`
	Paths        map[string]PathItem    `yaml:"paths"`
	Components   *Components            `yaml:"components,omitempty"`
	Security     []SecurityRequirement  `yaml:"security,omitempty"`
	Tags         []Tag                  `yaml:"tags,omitempty"`
	ExternalDocs *ExternalDocumentation `yaml:"externalDocs,omitempty"`
}

// Info represents the OpenAPI info object
type Info struct {
	Title          string   `yaml:"title,omitempty"`
	TermsOfService string   `yaml:"termsOfService,omitempty"`
	Description    string   `yaml:"description,omitempty"`
	Version        string   `yaml:"version"`
	Contact        *Contact `yaml:"contact,omitempty"`
	License        *License `yaml:"license,omitempty"`
}

// Contact represents contact information
type Contact struct {
	Name  string `yaml:"name,omitempty"`
	URL   string `yaml:"url,omitempty"`
	Email string `yaml:"email,omitempty"`
}

// License represents license information
type License struct {
	Name string `yaml:"name"`
	URL  string `yaml:"url,omitempty"`
}

// Server represents a server
type Server struct {
	URL         string                    `yaml:"url"`
	Description string                    `yaml:"description,omitempty"`
	Variables   map[string]ServerVariable `yaml:"variables,omitempty"`
}

// ServerVariable represents a server variable
type ServerVariable struct {
	Enum        []string `yaml:"enum,omitempty"`
	Default     string   `yaml:"default"`
	Description string   `yaml:"description,omitempty"`
}

// PathItem represents a path item in OpenAPI
type PathItem struct {
	Ref         string      `yaml:"$ref,omitempty"`
	Summary     string      `yaml:"summary,omitempty"`
	Description string      `yaml:"description,omitempty"`
	Get         *Operation  `yaml:"get,omitempty"`
	Post        *Operation  `yaml:"post,omitempty"`
	Put         *Operation  `yaml:"put,omitempty"`
	Delete      *Operation  `yaml:"delete,omitempty"`
	Patch       *Operation  `yaml:"patch,omitempty"`
	Options     *Operation  `yaml:"options,omitempty"`
	Head        *Operation  `yaml:"head,omitempty"`
	Parameters  []Parameter `yaml:"parameters,omitempty"`
}

// Operation represents an OpenAPI operation
type Operation struct {
	Tags         []string               `yaml:"tags,omitempty"`
	Summary      string                 `yaml:"summary,omitempty"`
	Description  string                 `yaml:"description,omitempty"`
	OperationID  string                 `yaml:"operationId,omitempty"`
	Parameters   []Parameter            `yaml:"parameters,omitempty"`
	RequestBody  *RequestBody           `yaml:"requestBody,omitempty"`
	Responses    map[string]Response    `yaml:"responses"`
	Security     []SecurityRequirement  `yaml:"security,omitempty"`
	ExternalDocs *ExternalDocumentation `yaml:"externalDocs,omitempty"`
}

// Parameter represents an OpenAPI parameter
type Parameter struct {
	Name        string                 `yaml:"name"`
	In          string                 `yaml:"in"`
	Description string                 `yaml:"description,omitempty"`
	Required    bool                   `yaml:"required,omitempty"`
	Schema      *Schema                `yaml:"schema,omitempty"`
	Example     interface{}            `yaml:"example,omitempty"`
	Extensions  map[string]interface{} `yaml:",inline"`
}

// RequestBody represents an OpenAPI request body
type RequestBody struct {
	Description string               `yaml:"description,omitempty"`
	Content     map[string]MediaType `yaml:"content"`
	Required    bool                 `yaml:"required,omitempty"`
}

// Response represents an OpenAPI response
type Response struct {
	Description string               `yaml:"description"`
	Headers     map[string]Header    `yaml:"headers,omitempty"`
	Content     map[string]MediaType `yaml:"content,omitempty"`
	Links       map[string]Link      `yaml:"links,omitempty"`
}

// Header represents an OpenAPI header
type Header struct {
	Description string      `yaml:"description,omitempty"`
	Schema      *Schema     `yaml:"schema,omitempty"`
	Example     interface{} `yaml:"example,omitempty"`
}

// MediaType represents an OpenAPI media type
type MediaType struct {
	Schema   *Schema             `yaml:"schema,omitempty"`
	Example  interface{}         `yaml:"example,omitempty"`
	Examples map[string]Example  `yaml:"examples,omitempty"`
	Encoding map[string]Encoding `yaml:"encoding,omitempty"`
}

// Schema represents an OpenAPI schema
type Schema struct {
	Type                 string                 `yaml:"type,omitempty"`
	Format               string                 `yaml:"format,omitempty"`
	Description          string                 `yaml:"description,omitempty"`
	Title                string                 `yaml:"title,omitempty"`
	Default              interface{}            `yaml:"default,omitempty"`
	Example              interface{}            `yaml:"example,omitempty"`
	ReadOnly             bool                   `yaml:"readOnly,omitempty"`
	WriteOnly            bool                   `yaml:"writeOnly,omitempty"`
	Deprecated           bool                   `yaml:"deprecated,omitempty"`
	Ref                  string                 `yaml:"$ref,omitempty"`
	AllOf                []*Schema              `yaml:"allOf,omitempty"`
	OneOf                []*Schema              `yaml:"oneOf,omitempty"`
	AnyOf                []*Schema              `yaml:"anyOf,omitempty"`
	Not                  *Schema                `yaml:"not,omitempty"`
	Items                *Schema                `yaml:"items,omitempty"`
	Properties           map[string]*Schema     `yaml:"properties,omitempty"`
	AdditionalProperties *Schema                `yaml:"additionalProperties,omitempty"`
	Required             []string               `yaml:"required,omitempty"`
	MinLength            int                    `yaml:"minLength,omitempty"`
	MaxLength            int                    `yaml:"maxLength,omitempty"`
	Pattern              string                 `yaml:"pattern,omitempty"`
	Minimum              float64                `yaml:"minimum,omitempty"`
	Maximum              float64                `yaml:"maximum,omitempty"`
	ExclusiveMinimum     bool                   `yaml:"exclusiveMinimum,omitempty"`
	ExclusiveMaximum     bool                   `yaml:"exclusiveMaximum,omitempty"`
	MultipleOf           float64                `yaml:"multipleOf,omitempty"`
	MinItems             int                    `yaml:"minItems,omitempty"`
	MaxItems             int                    `yaml:"maxItems,omitempty"`
	UniqueItems          bool                   `yaml:"uniqueItems,omitempty"`
	MinProperties        int                    `yaml:"minProperties,omitempty"`
	MaxProperties        int                    `yaml:"maxProperties,omitempty"`
	Enum                 []interface{}          `yaml:"enum,omitempty"`
	Discriminator        *Discriminator         `yaml:"discriminator,omitempty"`
	XML                  *XML                   `yaml:"xml,omitempty"`
	ExternalDocs         *ExternalDocumentation `yaml:"externalDocs,omitempty"`
}

// Discriminator represents an OpenAPI discriminator
type Discriminator struct {
	PropertyName string            `yaml:"propertyName"`
	Mapping      map[string]string `yaml:"mapping,omitempty"`
}

// XML represents XML serialization options
type XML struct {
	Name      string `yaml:"name,omitempty"`
	Namespace string `yaml:"namespace,omitempty"`
	Prefix    string `yaml:"prefix,omitempty"`
	Attribute bool   `yaml:"attribute,omitempty"`
	Wrapped   bool   `yaml:"wrapped,omitempty"`
}

// Example represents an OpenAPI example
type Example struct {
	Summary       string      `yaml:"summary,omitempty"`
	Description   string      `yaml:"description,omitempty"`
	Value         interface{} `yaml:"value,omitempty"`
	ExternalValue string      `yaml:"externalValue,omitempty"`
}

// Encoding represents an OpenAPI encoding
type Encoding struct {
	ContentType   string            `yaml:"contentType,omitempty"`
	Headers       map[string]Header `yaml:"headers,omitempty"`
	Style         string            `yaml:"style,omitempty"`
	Explode       bool              `yaml:"explode,omitempty"`
	AllowReserved bool              `yaml:"allowReserved,omitempty"`
}

// Link represents an OpenAPI link
type Link struct {
	OperationRef string                 `yaml:"operationRef,omitempty"`
	OperationID  string                 `yaml:"operationId,omitempty"`
	Parameters   map[string]interface{} `yaml:"parameters,omitempty"`
	RequestBody  interface{}            `yaml:"requestBody,omitempty"`
	Description  string                 `yaml:"description,omitempty"`
	Server       *Server                `yaml:"server,omitempty"`
}

// Components represents OpenAPI components
type Components struct {
	Schemas         map[string]*Schema        `yaml:"schemas,omitempty"`
	Responses       map[string]*Response      `yaml:"responses,omitempty"`
	Parameters      map[string]*Parameter     `yaml:"parameters,omitempty"`
	Examples        map[string]*Example       `yaml:"examples,omitempty"`
	RequestBodies   map[string]*RequestBody   `yaml:"requestBodies,omitempty"`
	Headers         map[string]*Header        `yaml:"headers,omitempty"`
	SecuritySchemes map[string]SecurityScheme `yaml:"securitySchemes,omitempty"`
	Links           map[string]*Link          `yaml:"links,omitempty"`
	Callbacks       map[string]interface{}    `yaml:"callbacks,omitempty"`
}

// SecurityScheme represents an OpenAPI security scheme
type SecurityScheme struct {
	Type             string      `yaml:"type"`
	Description      string      `yaml:"description,omitempty"`
	Name             string      `yaml:"name,omitempty"`
	In               string      `yaml:"in,omitempty"`
	Scheme           string      `yaml:"scheme,omitempty"`
	BearerFormat     string      `yaml:"bearerFormat,omitempty"`
	Flows            *OAuthFlows `yaml:"flows,omitempty"`
	OpenIDConnectURL string      `yaml:"openIdConnectUrl,omitempty"`
}

// OAuthFlows represents OAuth flows
type OAuthFlows struct {
	Implicit          *OAuthFlow `yaml:"implicit,omitempty"`
	Password          *OAuthFlow `yaml:"password,omitempty"`
	ClientCredentials *OAuthFlow `yaml:"clientCredentials,omitempty"`
	AuthorizationCode *OAuthFlow `yaml:"authorizationCode,omitempty"`
}

// OAuthFlow represents an OAuth flow
type OAuthFlow struct {
	AuthorizationURL string            `yaml:"authorizationUrl,omitempty"`
	TokenURL         string            `yaml:"tokenUrl,omitempty"`
	RefreshURL       string            `yaml:"refreshUrl,omitempty"`
	Scopes           map[string]string `yaml:"scopes"`
}

// SecurityRequirement represents a security requirement
type SecurityRequirement map[string][]string

// Tag represents an OpenAPI tag
type Tag struct {
	Name         string                 `yaml:"name"`
	Description  string                 `yaml:"description,omitempty"`
	ExternalDocs *ExternalDocumentation `yaml:"externalDocs,omitempty"`
}

// ExternalDocumentation represents external documentation
type ExternalDocumentation struct {
	Description string `yaml:"description,omitempty"`
	URL         string `yaml:"url"`
}
