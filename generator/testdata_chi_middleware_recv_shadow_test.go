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

	"github.com/ehabterra/apispec/spec"
)

// TestTestdata_ChiMiddlewareRecvShadow guards the lazy-tracker regression where
// a middleware that reassigns the request variable `r` (the canonical
// `r = r.WithContext(ctx)` idiom) collided with the `r chi.Router` receiver at
// the registration site. The callee-body `r` leaked into the caller-scope call
// edge's AssignmentMap, and the tracker claimed the router's own receiver
// registrations (a direct r.Get, a nested r.Group subtree) onto the middleware
// producer — dropping them or re-homing them under the wrong path prefix
// (e.g. /api/v1/tenant surfaced as /api/v1/auth/tenant). Every route must
// appear at its correct path.
func TestTestdata_ChiMiddlewareRecvShadow(t *testing.T) {
	out := loadTestdataWithFixtureConfig(t, "chi_middleware_recv_shadow", spec.DefaultChiConfig())
	noDanglingRefs(t, out)
	noUnresolvedPlaceholders(t, out)

	want := map[string][]string{
		// Receiver-registered routes in the closure that installs the
		// r-reassigning middleware — these regressed.
		"/api/v1/tenant": {"GET"},
		"/api/v1/users/": {"GET", "POST"}, // nested r.Group subtree
		// Routes that always resolved (argument-passed helpers, sibling mounts).
		"/api/v1/caps":          {"GET"},
		"/api/v1/workflows":     {"GET"},
		"/api/v1/notifications": {"GET"},
		"/api/v1/auth/login":    {"POST"},
		"/api/v1/auth/me":       {"GET"},
	}
	for path, methods := range want {
		item, ok := out.Paths[path]
		if !ok {
			t.Errorf("path %q missing; have %v", path, mapPathKeys(out.Paths))
			continue
		}
		for _, m := range methods {
			if opFor(item, m) == nil {
				t.Errorf("%s %s: expected operation, missing", m, path)
			}
		}
	}

	// The regression re-homed the receiver routes under the sibling mount's
	// /auth prefix. Assert those wrong paths never appear.
	for _, wrong := range []string{"/api/v1/auth/tenant", "/api/v1/auth/users/"} {
		if _, ok := out.Paths[wrong]; ok {
			t.Errorf("mis-prefixed path %q present — receiver-registration claim leaked", wrong)
		}
	}
}
