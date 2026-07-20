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

package generator

import (
	"testing"

	intspec "github.com/ehabterra/apispec/internal/spec"
	"github.com/ehabterra/apispec/spec"
)

// TestTestdata_HandlerValueRoutes covers issue #204 across frameworks: a route
// registered with a handler *value* rather than a func names no method anywhere
// in the registration, so nothing resolved it and the operation had no params,
// request body, response, or summary. The method now comes from the framework
// config (FrameworkConfig.HandlerInterfaceMethods), so the handler's body is
// reachable — this must hold for every http.Handler-based framework, not just
// the one that prompted the fix (golden rule #5).
func TestTestdata_HandlerValueRoutes(t *testing.T) {
	for _, tc := range []struct {
		name, fixture, path, summary string
		cfg                          *spec.APISpecConfig
		schemaSuffix                 string
	}{
		{
			name:         "net/http mux.Handle",
			fixture:      "handler_doc_comments",
			path:         "/accounts",
			cfg:          spec.DefaultHTTPConfig(),
			summary:      "ServeHTTP serves the account resource directly.",
			schemaSuffix: "",
		},
		{
			name:         "chi r.Method with an http.Handler field",
			fixture:      "chi_method_handle",
			path:         "/health",
			cfg:          spec.DefaultChiConfig(),
			summary:      "ServeHTTP reports service health.",
			schemaSuffix: "HealthStatus",
		},
		{
			name:         "gorilla/mux r.Handle with a value",
			fixture:      "mux",
			path:         "/status",
			cfg:          spec.DefaultMuxConfig(),
			summary:      "ServeHTTP reports the service status.",
			schemaSuffix: "Status",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := loadTestdataWithFixtureConfig(t, tc.fixture, tc.cfg)
			noDanglingRefs(t, out)
			noUnresolvedPlaceholders(t, out)

			item, ok := out.Paths[tc.path]
			if !ok {
				t.Fatalf("%s missing; have %v", tc.path, mapPathKeys(out.Paths))
			}
			var op *intspec.Operation
			for _, m := range []string{"GET", "OPTIONS"} {
				if o := opFor(item, m); o != nil && o.Summary == tc.summary {
					op = o
					break
				}
			}
			if op == nil {
				t.Fatalf("%s: no operation carrying the handler-value summary %q (#204)", tc.path, tc.summary)
			}
			// The body must resolve too — the summary alone would mean the doc
			// comment was found by name while the handler stayed unexpanded.
			if tc.schemaSuffix != "" && len(op.Responses) == 0 {
				t.Errorf("%s: handler body did not resolve — no responses (#204)", tc.path)
			}
		})
	}
}
