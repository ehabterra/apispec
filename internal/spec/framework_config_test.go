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

import (
	"testing"

	"github.com/ehabterra/apispec/internal/core"
)

// TestFrameworkConfigsCoverRegistry ties this map to the core registry in both
// directions. Without it, a framework added to the registry and marked
// HasDefaultConfig would fall through to DefaultHTTPConfig at runtime — the
// same class of silent miss as the framework-count literal in issue #285.
func TestFrameworkConfigsCoverRegistry(t *testing.T) {
	registered := map[string]bool{}
	for _, fw := range core.Frameworks() {
		registered[fw.Name] = true
		_, hasConfig := frameworkConfigs[fw.Name]
		if fw.HasDefaultConfig && !hasConfig {
			t.Errorf("framework %q is marked HasDefaultConfig but has no entry in frameworkConfigs, so it silently falls back to net/http", fw.Name)
		}
		if !fw.HasDefaultConfig && hasConfig {
			t.Errorf("framework %q has a config constructor but is not marked HasDefaultConfig, so the UI never offers it", fw.Name)
		}
	}
	for name := range frameworkConfigs {
		if !registered[name] {
			t.Errorf("frameworkConfigs has %q, which is not in the core framework registry", name)
		}
	}
}

func TestDefaultConfigForFramework(t *testing.T) {
	for _, fw := range core.Frameworks() {
		if !fw.HasDefaultConfig {
			continue
		}
		t.Run(fw.Name, func(t *testing.T) {
			cfg := DefaultConfigForFramework(fw.Name)
			if cfg == nil {
				t.Fatalf("DefaultConfigForFramework(%q) = nil", fw.Name)
			}
			// A config with no route patterns documents nothing.
			if len(cfg.Framework.RoutePatterns) == 0 {
				t.Errorf("config for %q has no route patterns", fw.Name)
			}
		})
	}

	// Unknown names and the dependency-analysis-only frameworks fall back to
	// net/http rather than returning nil.
	for _, name := range []string{"", "iris", "fasthttp", "not-a-framework"} {
		if cfg := DefaultConfigForFramework(name); cfg == nil {
			t.Errorf("DefaultConfigForFramework(%q) = nil, want the net/http fallback", name)
		}
	}
}

func TestDefaultConfigForFrameworkIsCaseInsensitive(t *testing.T) {
	lower := DefaultConfigForFramework("gin")
	upper := DefaultConfigForFramework("GIN")
	if len(upper.Framework.RoutePatterns) != len(lower.Framework.RoutePatterns) {
		t.Errorf("case changed the resolved config: %q gave %d route patterns, %q gave %d",
			"gin", len(lower.Framework.RoutePatterns), "GIN", len(upper.Framework.RoutePatterns))
	}
}
