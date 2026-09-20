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
	"os"
	"path/filepath"
	"testing"
)

// writeConfig writes a config file inside the test's own directory — a test
// must never dirty the working tree.
func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "apispec.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

// TestAdoptFrameworkPatterns pins which configs keep the detected framework's
// route patterns and which replace them (issue #524).
//
// The replacement is deliberate for a config that names its own patterns —
// gin's `Handle(method, path, h)` and mux's `Handle(path, h)` misparse each
// other's calls, so "what you write is what matches" is what keeps a
// mixed-framework project honest (#211/#212). It was never meant to extend to a
// config that says nothing about `framework` at all, where it left the run
// documenting zero paths and exiting 0.
func TestAdoptFrameworkPatterns(t *testing.T) {
	composed := DefaultChiConfig()
	if len(composed.Framework.RoutePatterns) == 0 {
		t.Fatal("the composed config has no route patterns; the test proves nothing")
	}

	cases := []struct {
		name  string
		body  string
		adopt bool
		why   string
	}{
		{
			name:  "a config that never mentions framework",
			body:  "info:\n  title: Users API\n  version: 2.0.0\n",
			adopt: true,
			why:   "setting a title expresses no opinion about routing",
		},
		{
			name:  "a naming-only config",
			body:  "naming:\n  schemaNames: short\n",
			adopt: true,
			why:   "the first thing the README suggests configuring",
		},
		{
			name:  "an empty document",
			body:  "{}\n",
			adopt: true,
			why:   "nothing said, nothing lost",
		},
		{
			name:  "a config that declares its own patterns",
			body:  "framework:\n  routePatterns:\n    - callRegex: ^Handle$\n",
			adopt: false,
			why:   "what you write is what matches (#211)",
		},
		{
			name: "a framework block that declares something else",
			body: "framework:\n  requestContext:\n    typeRegexes: ['^myfw\\.Ctx$']\n",
			// The block is all-or-nothing on purpose: naming it opts out.
			adopt: false,
			why:   "the framework block is declared, so it replaces wholesale",
		},
		{
			name:  "an explicitly empty framework block",
			body:  "framework: {}\n",
			adopt: false,
			why:   "writing the key is an opinion; this is how a user says 'no patterns'",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := LoadAPISpecConfig(writeConfig(t, tc.body))
			if err != nil {
				t.Fatalf("load: %v", err)
			}
			before := len(cfg.Framework.RoutePatterns)
			cfg.AdoptFrameworkPatterns(DefaultChiConfig())
			after := len(cfg.Framework.RoutePatterns)

			// Adoption is visible as the config ending up with the composed
			// pattern set; leaving it alone is the count not moving. Counting
			// "has any patterns" would call a config that brought its OWN
			// single pattern adopted.
			got := after == len(composed.Framework.RoutePatterns) && after != before
			if got != tc.adopt {
				t.Errorf("adopted the detected route patterns = %v, want %v (had %d, now %d) — %s",
					got, tc.adopt, before, after, tc.why)
			}
			if !tc.adopt && after != before {
				t.Errorf("route pattern count moved from %d to %d without adopting", before, after)
			}
			if tc.adopt && cfg.Defaults.ResponseContentType == "" {
				t.Error("the framework's default response content type was not adopted with its patterns")
			}
		})
	}
}

// TestAdoptFrameworkPatternsLeavesCodeBuiltConfigsAlone is the regression the
// first version of this fix caused, and it is the more dangerous direction.
//
// A config built in code has no file to inspect, so the "did the file declare
// framework?" question answers false for every one of them — including
// `NewGenerator(DefaultChiConfig())` with its patterns deliberately narrowed.
// Adopting there would replace the caller's scoped patterns with the detected
// defaults: the same silent loss, pointed the other way.
func TestAdoptFrameworkPatternsLeavesCodeBuiltConfigsAlone(t *testing.T) {
	scoped := DefaultChiConfig()
	for i := range scoped.Framework.RoutePatterns {
		scoped.Framework.RoutePatterns[i].CallerPkgPatterns = []string{`/internal/api$`}
	}

	scoped.AdoptFrameworkPatterns(DefaultChiConfig())

	for i, p := range scoped.Framework.RoutePatterns {
		if len(p.CallerPkgPatterns) == 0 {
			t.Fatalf("route pattern %d lost its caller scope; the code-built config was overwritten", i)
		}
	}
}

// TestAdoptFrameworkPatternsIsInert covers the arguments that must do nothing.
func TestAdoptFrameworkPatternsIsInert(t *testing.T) {
	var nilCfg *APISpecConfig
	nilCfg.AdoptFrameworkPatterns(DefaultChiConfig()) // must not panic

	cfg := &APISpecConfig{}
	cfg.AdoptFrameworkPatterns(nil)
	if len(cfg.Framework.RoutePatterns) != 0 {
		t.Error("patterns appeared from a nil composed config")
	}

	// An in-code config with no opinion at all is the one code-built case that
	// SHOULD adopt: it is the library equivalent of an empty file.
	empty := &APISpecConfig{}
	empty.AdoptFrameworkPatterns(DefaultChiConfig())
	if len(empty.Framework.RoutePatterns) == 0 {
		t.Error("an empty in-code config did not adopt the detected patterns")
	}
}

// TestDeclaresFramework pins the accessor the engine reads.
func TestDeclaresFramework(t *testing.T) {
	var nilCfg *APISpecConfig
	if nilCfg.DeclaresFramework() {
		t.Error("a nil config declares a framework")
	}
	if (&APISpecConfig{}).DeclaresFramework() {
		t.Error("a config built in code declares a framework; there is no file to have declared it")
	}
	cfg, err := LoadAPISpecConfig(writeConfig(t, "framework:\n  routePatterns: []\n"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !cfg.DeclaresFramework() {
		t.Error("a file with a framework key does not report it")
	}
}
