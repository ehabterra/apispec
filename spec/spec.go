// Package spec exposes a stable public API for configuration and OpenAPI types,
// re-exported from the internal spec package.
package spec

import intspec "github.com/ehabterra/apispec/internal/spec"

// Re-export core configuration types
type APISpecConfig = intspec.APISpecConfig
type Info = intspec.Info
type Server = intspec.Server
type SecurityRequirement = intspec.SecurityRequirement
type SecurityScheme = intspec.SecurityScheme
type SecurityPattern = intspec.SecurityPattern
type SecurityMapping = intspec.SecurityMapping
type MiddlewareRef = intspec.MiddlewareRef
type FrameworkConfig = intspec.FrameworkConfig
type Tag = intspec.Tag

// Security scope values for SecurityPattern.Scope.
const (
	SecurityScopeRouter  = intspec.SecurityScopeRouter
	SecurityScopeSubtree = intspec.SecurityScopeSubtree
	SecurityScopeRoute   = intspec.SecurityScopeRoute
	SecurityScopeWrapper = intspec.SecurityScopeWrapper
)
type ExternalDocumentation = intspec.ExternalDocumentation
type Schema = intspec.Schema
type Components = intspec.Components
type OpenAPISpec = intspec.OpenAPISpec

// Default framework configurations
func DefaultGinConfig() *APISpecConfig   { return intspec.DefaultGinConfig() }
func DefaultChiConfig() *APISpecConfig   { return intspec.DefaultChiConfig() }
func DefaultEchoConfig() *APISpecConfig  { return intspec.DefaultEchoConfig() }
func DefaultFiberConfig() *APISpecConfig { return intspec.DefaultFiberConfig() }
func DefaultMuxConfig() *APISpecConfig   { return intspec.DefaultMuxConfig() }
func DefaultHTTPConfig() *APISpecConfig  { return intspec.DefaultHTTPConfig() }

// LoadAPISpecConfig loads a YAML configuration file.
func LoadAPISpecConfig(path string) (*APISpecConfig, error) { return intspec.LoadAPISpecConfig(path) }
