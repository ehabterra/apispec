// Copyright 2025 Ehab Terra
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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

// HTTPSecondaryConfig is the merge-safe, receiver-scoped subset of the
// net/http config for layering under another framework's config.
func HTTPSecondaryConfig() *APISpecConfig { return intspec.HTTPSecondaryConfig() }

// MergeFrameworkConfigs layers secondary framework configs under the primary
// (first-occurrence-wins pattern dedupe; Info/Defaults stay the primary's).
func MergeFrameworkConfigs(primary *APISpecConfig, secondaries ...*APISpecConfig) *APISpecConfig {
	return intspec.MergeFrameworkConfigs(primary, secondaries...)
}

// SecondaryView returns a config's merge-safe subset: only receiver- or
// package-scoped patterns survive, so the view cannot claim another
// framework's calls when layered under it.
func SecondaryView(cfg *APISpecConfig) *APISpecConfig { return intspec.SecondaryView(cfg) }

// LoadAPISpecConfig loads a YAML configuration file.
func LoadAPISpecConfig(path string) (*APISpecConfig, error) { return intspec.LoadAPISpecConfig(path) }
