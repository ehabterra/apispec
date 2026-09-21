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

package engine

import (
	"path/filepath"
	"strings"
	"testing"

	intspec "github.com/ehabterra/apispec/internal/spec"
)

// TestMiddlewareClassification pins which unmapped middleware is reported as
// missing a security scheme (issue #520).
//
// This is the most important warning apispec prints — an auth middleware nobody
// mapped means its routes are documented as PUBLIC — and every other middleware
// sharing the Use/group slot was announced the same way, so on a real service
// it was mostly false. A warning that is mostly false trains people past the
// true one.
//
// The split is made on what the body DOES, never on the name: a denylist of
// "logger"/"cors" is the guess golden rule #9 forbids. The fixture's non-auth
// middleware therefore refuse requests too, which is the honest test — refusing
// is not sufficient, reading a credential is.
func TestMiddlewareClassification(t *testing.T) {
	cfg := intspec.DefaultChiConfig()
	cfg.SecuritySchemes = map[string]intspec.SecurityScheme{
		"bearerAuth": {Type: "http", Scheme: "bearer"},
	}
	// One of the fixture's middleware is mapped; the rest are not, which is the
	// state a real project is in.
	cfg.SecurityMappings = []intspec.SecurityMapping{{
		FunctionNameRegex: `^authMiddleware$`,
		Schemes:           []intspec.SecurityRequirement{{"bearerAuth": []string{}}},
	}}

	e := NewEngine(&EngineConfig{
		InputDir:      filepath.Join("..", "..", "testdata", "middleware_classification"),
		APISpecConfig: cfg,
	})
	if _, err := e.GenerateOpenAPI(); err != nil {
		t.Fatalf("generation failed: %v", err)
	}

	reported := refNames(e.GetUnresolvedSecurity())
	quiet := refNames(e.GetUnclassifiedMiddleware())

	t.Run("reads a credential and maps to nothing", func(t *testing.T) {
		for _, want := range []string{
			"houseAuth",      // reads a header naming an api key
			"delegatingAuth", // reads it one call down, through a helper
			"basicGuard",     // r.BasicAuth() — the credential with no name
			"signatureAuth",  // a house credential name, caught by its 401
		} {
			if !hasName(reported, want) {
				t.Errorf("%s guards routes that are now documented as PUBLIC and was not reported; reported=%v",
					want, reported)
			}
		}
	})

	t.Run("refuses requests but shows no auth signal", func(t *testing.T) {
		// rateLimit REFUSES (429) and requestLogger reads a header — so a rule
		// keying on "can reject a request", or on "reads any header", calls
		// both of these auth. Neither is.
		for _, notWant := range []string{
			"requestLogger", "rateLimit",
			// A credential-looking value that is not a credential READ. All
			// three were reported before the signal required call semantics.
			"logsTheWord",      // logs the word "Authorization"
			"addsUpstreamAuth", // SETS an outbound Authorization header
			"countsForbidden",  // passes 403 to a metric, not to a writer
			// Authorisation, not authentication: reads a role from the context
			// and refuses with 403. The shape of RequireAuth-then-RequireRole,
			// where the routes already carry the scheme RequireAuth supplies —
			// reporting it claimed they were documented as public.
			"roleGate",
		} {
			if hasName(reported, notWant) {
				t.Errorf("%s shows no auth signal and must not be reported as auth; reported=%v", notWant, reported)
			}
			// Demoted, not dropped: a project whose auth middleware fetches its
			// credential somewhere this walk does not reach must still be able
			// to find it.
			if !hasName(quiet, notWant) {
				t.Errorf("%s was dropped entirely; it belongs in the verbose list, not nowhere", notWant)
			}
		}
	})

	t.Run("already mapped", func(t *testing.T) {
		if hasName(reported, "authMiddleware") || hasName(quiet, "authMiddleware") {
			t.Errorf("authMiddleware is mapped and must not be reported; reported=%v quiet=%v", reported, quiet)
		}
	})
}

func refNames(refs []intspec.MiddlewareRef) []string {
	out := make([]string, 0, len(refs))
	for _, r := range refs {
		out = append(out, r.String())
	}
	return out
}

func hasName(haystack []string, name string) bool {
	for _, h := range haystack {
		if h == name || strings.HasSuffix(h, "."+name) {
			return true
		}
	}
	return false
}
