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
	"sort"
	"strings"
	"testing"

	intspec "github.com/ehabterra/apispec/internal/spec"
	"github.com/ehabterra/apispec/spec"
)

// TestTestdata_HandlerWrapperAttribution covers issue #364: a middleware applied
// at the registration site (`mux.Handle(p, withLogging(http.HandlerFunc(h)))`)
// replaced the handler — the operation was built from the MIDDLEWARE, so it
// carried the wrapper's name and doc comment, the wrapper's header reads as
// parameters, and none of the handler's request body or responses.
//
// The fixture registers the same handlers wrapped and directly, so the two are
// compared against each other rather than against a snapshot: whatever the
// direct route documents, the wrapped one must document too.
func TestTestdata_HandlerWrapperAttribution(t *testing.T) {
	for _, tc := range []struct {
		fixture string
		cfg     *spec.APISpecConfig
		pairs   []struct{ wrapped, direct, method string }
	}{
		{
			fixture: "handler_wrapper_attribution",
			cfg:     spec.DefaultHTTPConfig(),
			pairs: []struct{ wrapped, direct, method string }{
				{"/wrapped/items/{id}", "/direct/items/{id}", "GET"},
				{"/wrapped/items", "/direct/items", "POST"},
				// Nested wrappers resolve to the innermost handler.
				{"/chained/items/{id}", "/direct/items/{id}", "GET"},
				// The HandlerFunc-shaped wrapper, whose argument is a plain ident.
				{"/timed/items/{id}", "/direct/items/{id}", "GET"},
			},
		},
		{
			// Same wiring in a second framework — the defect is not net/http's
			// (golden rule #5).
			fixture: "handler_wrapper_attribution_chi",
			cfg:     spec.DefaultChiConfig(),
			pairs: []struct{ wrapped, direct, method string }{
				{"/wrapped/items/{id}", "/direct/items/{id}", "GET"},
				{"/wrapped/items", "/direct/items", "POST"},
			},
		},
	} {
		t.Run(tc.fixture, func(t *testing.T) {
			out := loadTestdataWithFixtureConfig(t, tc.fixture, tc.cfg)
			noDanglingRefs(t, out)

			for _, pair := range tc.pairs {
				wrapped := opFor(out.Paths[pair.wrapped], pair.method)
				direct := opFor(out.Paths[pair.direct], pair.method)
				if wrapped == nil || direct == nil {
					t.Fatalf("%s %s / %s: missing operation(s); have %v",
						pair.method, pair.wrapped, pair.direct, mapPathKeys(out.Paths))
				}

				// Same handler ⇒ same operationId, modulo the path it was
				// registered at (operationIds carry the handler, not the route).
				if wrapped.OperationID != direct.OperationID {
					t.Errorf("%s %s: operationId %q, but the same handler registered directly is %q — the wrapper replaced it",
						pair.method, pair.wrapped, wrapped.OperationID, direct.OperationID)
				}
				if wrapped.Summary != direct.Summary {
					t.Errorf("%s %s: summary %q, want the handler's %q",
						pair.method, pair.wrapped, wrapped.Summary, direct.Summary)
				}
				if (wrapped.RequestBody == nil) != (direct.RequestBody == nil) {
					t.Errorf("%s %s: request body presence differs from the direct registration (wrapped=%v direct=%v)",
						pair.method, pair.wrapped, wrapped.RequestBody != nil, direct.RequestBody != nil)
				}
				if got, want := sortedStatusKeys(wrapped), sortedStatusKeys(direct); !sameStrings(got, want) {
					t.Errorf("%s %s: responses %v, want the handler's %v",
						pair.method, pair.wrapped, got, want)
				}
				// The path parameter the handler reads must be found through the
				// wrapper too — it used to be flagged "present in the path but
				// not found in the code".
				for _, p := range wrapped.Parameters {
					if p.In == "path" && p.Extensions != nil {
						if _, warned := p.Extensions["x-warning"]; warned {
							t.Errorf("%s %s: path parameter %q is flagged as not found in the code",
								pair.method, pair.wrapped, p.Name)
						}
					}
				}
			}

			// Two different handlers behind the same wrapper stay distinct.
			ids := map[string]string{}
			for path, item := range out.Paths {
				for _, method := range []string{"GET", "POST"} {
					if op := opFor(item, method); op != nil {
						ids[method+" "+path] = op.OperationID
					}
				}
			}
			if a, b := ids["GET /wrapped/items/{id}"], ids["POST /wrapped/items"]; a != "" && a == b {
				t.Errorf("two handlers behind the same wrapper collapsed onto one operationId %q", a)
			}
		})
	}
}

// sortedStatusKeys is statusKeys with a stable order, so two operations'
// response sets compare.
func sortedStatusKeys(op *intspec.Operation) []string {
	keys := statusKeys(op)
	sort.Strings(keys)
	return keys
}

func sameStrings(a, b []string) bool {
	return strings.Join(a, ",") == strings.Join(b, ",")
}
