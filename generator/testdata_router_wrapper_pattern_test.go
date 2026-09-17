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

	"github.com/ehabterra/apispec/internal/spec"
)

// A router WRAPPER must not lose the caller's path.
//
// handleRouteNode re-extracts into the same RouteInfo from a route node's
// children, because that is how a chain-style route resolves: the outer
// `Methods("GET")` carries no path and `.Path("/x")` supplies it. A wrapper
// descends into that same walk, and the registration it reaches is the SAME
// route one hop further from the literal:
//
//	func (r *Router) Methods(pattern string, h ...any) {
//		full := r.getPattern(pattern)
//		r.With(r.mw...).register(full, handler)
//	}
//
// `full` is a local assigned from a call, so it resolved to `{full}` — and that
// placeholder then replaced the caller's own literal, which dropped the route.
// On gitea it cost 10 endpoints, silently, and which endpoints depended on
// MaxInstancesPerKey: below the cap the walk never got deep enough to overwrite
// anything, so a LOWER cap documented MORE routes (issue #494).
//
// The three shapes are here together because only the third broke. A fixture
// with the first two alone reports the bug as fixed.
func TestTestdata_RouterWrapperPattern(t *testing.T) {
	out := loadTestdata(t, "router_wrapper_pattern", spec.DefaultHTTPConfig())
	noDanglingRefs(t, out)

	for _, tc := range []struct {
		path string
		why  string
	}{
		{"/plain", "control: registered directly on the mux, no wrapper"},
		{"/direct", "the wrapper's PARAMETER reaches the registration"},
		{"/direct-two", "the same, with a second caller so no single call site decides it"},
		{"/wrapped", "parameter -> local -> CHAINED call -> registration"},
		{"/wrapped-two", "the same, second caller"},
		{"/wrapped-three", "the same, third caller"},
	} {
		if _, ok := out.Paths[tc.path]; !ok {
			t.Errorf("path %q missing (%s); have %v", tc.path, tc.why, mapPathKeys(out.Paths))
		}
	}

	// No placeholder path survived. `{full}` is the wrapper's own local leaking
	// out as a path segment — a path no client can call, standing where a
	// literal was already known.
	for path := range out.Paths {
		if path == "{full}" || path == "/{full}" {
			t.Errorf("path %q is the wrapper's local variable, not a route", path)
		}
	}
}
