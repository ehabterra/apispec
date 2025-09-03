// Package spec exposes a stable public API for configuration and OpenAPI types,
// re-exported from the internal spec package.
package spec

import intspec "github.com/ehabterra/swagen/internal/spec"

// Re-export core configuration types
type SwagenConfig = intspec.SwagenConfig
type Info = intspec.Info
type Server = intspec.Server
type SecurityRequirement = intspec.SecurityRequirement
type SecurityScheme = intspec.SecurityScheme
type Tag = intspec.Tag
type ExternalDocumentation = intspec.ExternalDocumentation
type Schema = intspec.Schema
type Components = intspec.Components
type OpenAPISpec = intspec.OpenAPISpec

// Default framework configurations
func DefaultGinConfig() *SwagenConfig   { return intspec.DefaultGinConfig() }
func DefaultChiConfig() *SwagenConfig   { return intspec.DefaultChiConfig() }
func DefaultEchoConfig() *SwagenConfig  { return intspec.DefaultEchoConfig() }
func DefaultFiberConfig() *SwagenConfig { return intspec.DefaultFiberConfig() }
func DefaultMuxConfig() *SwagenConfig   { return intspec.DefaultMuxConfig() }
func DefaultHTTPConfig() *SwagenConfig  { return intspec.DefaultHTTPConfig() }

// LoadSwagenConfig loads a YAML configuration file.
func LoadSwagenConfig(path string) (*SwagenConfig, error) { return intspec.LoadSwagenConfig(path) }
