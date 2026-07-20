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
	"path/filepath"
	"testing"

	"github.com/ehabterra/apispec/internal/engine"
	"github.com/ehabterra/apispec/spec"
)

// TestHandlerValueEngineParity runs the handler-value fixtures through BOTH
// tracker engines and requires identical results.
//
// The eager tree materializes nodes up front while the lazy tree expands by key,
// so a capability added to one is silently absent from the other — which is
// exactly what happened when handler-value expansion (#204) first landed in
// LazyTree alone: summaries resolved (shared mapper code) while bodies did not.
// Both engines ship, so both must resolve the same routes.
func TestHandlerValueEngineParity(t *testing.T) {
	for _, tc := range []struct {
		name, fixture, path, method string
		cfg                         *spec.APISpecConfig
	}{
		{"chi handler value", "chi_method_handle", "/health", "GET", spec.DefaultChiConfig()},
		{"mux handler value", "mux", "/status", "GET", spec.DefaultMuxConfig()},
		{"net/http handler value", "handler_doc_comments", "/accounts", "OPTIONS", spec.DefaultHTTPConfig()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lazy := generateWithTracker(t, tc.fixture, tc.cfg, true)
			eager := generateWithTracker(t, tc.fixture, tc.cfg, false)

			lazyItem, ok := lazy.Paths[tc.path]
			if !ok {
				t.Fatalf("lazy: %s missing; have %v", tc.path, mapPathKeys(lazy.Paths))
			}
			eagerItem, ok := eager.Paths[tc.path]
			if !ok {
				t.Fatalf("eager: %s missing; have %v", tc.path, mapPathKeys(eager.Paths))
			}
			lazyOp, eagerOp := opFor(lazyItem, tc.method), opFor(eagerItem, tc.method)
			if lazyOp == nil || eagerOp == nil {
				t.Fatalf("%s %s: lazy=%v eager=%v", tc.method, tc.path, lazyOp != nil, eagerOp != nil)
			}

			if lazyOp.Summary != eagerOp.Summary {
				t.Errorf("summary differs between engines:\n lazy:  %q\n eager: %q", lazyOp.Summary, eagerOp.Summary)
			}
			if lazyOp.Summary == "" {
				t.Errorf("%s %s: expected a handler-value summary in both engines (#204)", tc.method, tc.path)
			}
			// Response *status sets* must match. Bodies are compared by presence
			// rather than by schema identity: the point is that the handler was
			// expanded at all, which is what diverged.
			if len(lazyOp.Responses) != len(eagerOp.Responses) {
				t.Errorf("response count differs: lazy=%d eager=%d", len(lazyOp.Responses), len(eagerOp.Responses))
			}
			for status := range lazyOp.Responses {
				if _, ok := eagerOp.Responses[status]; !ok {
					t.Errorf("status %q present in lazy but not eager", status)
				}
			}
		})
	}
}

// generateWithTracker builds the spec for a fixture with the chosen tracker
// engine. The public Generator always uses the lazy tree, so the engine is
// driven directly here.
func generateWithTracker(t *testing.T, fixture string, cfg *spec.APISpecConfig, lazy bool) *spec.OpenAPISpec {
	t.Helper()
	ec := engine.DefaultEngineConfig()
	ec.InputDir = filepath.Join("..", "testdata", fixture)
	ec.APISpecConfig = cfg
	ec.UseLazyTracker = lazy

	out, err := engine.NewEngine(ec).GenerateOpenAPI()
	if err != nil {
		t.Fatalf("GenerateOpenAPI(%s, lazy=%v): %v", fixture, lazy, err)
	}
	if out == nil || out.Paths == nil {
		t.Fatalf("nil spec for %s (lazy=%v)", fixture, lazy)
	}
	return out
}
