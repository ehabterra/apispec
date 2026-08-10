// Copyright 2026 Ehab Terra
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

package spec

import "strings"

// frameworkConfigs maps a framework name from the core registry to its default
// pattern config. It lives here rather than in the registry because
// internal/spec cannot be imported from internal/core without a cycle;
// TestFrameworkConfigsCoverRegistry asserts the two stay in step, so a
// framework added to the registry without a config fails the build's tests
// instead of silently falling back to net/http.
var frameworkConfigs = map[string]func() *APISpecConfig{
	"gin":      DefaultGinConfig,
	"chi":      DefaultChiConfig,
	"echo":     DefaultEchoConfig,
	"fiber":    DefaultFiberConfig,
	"mux":      DefaultMuxConfig,
	"net/http": DefaultHTTPConfig,
}

// DefaultConfigForFramework returns the default pattern config for the named
// framework, falling back to net/http for anything unrecognised (including the
// frameworks that are only classified during dependency analysis).
func DefaultConfigForFramework(name string) *APISpecConfig {
	if newConfig, ok := frameworkConfigs[strings.ToLower(name)]; ok {
		return newConfig()
	}
	return DefaultHTTPConfig()
}
