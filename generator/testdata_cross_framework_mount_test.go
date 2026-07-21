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
)

// TestTestdata_CrossFrameworkMount covers issue #138: a chi router mounted
// under a net/http ServeMux must contribute its mount prefix to the routes
// registered on it. Before the fix the routes were found but documented at the
// sub-router's bare paths (`/users`), which is a silently wrong spec — the path
// looks plausible and nothing flags it.
//
// The same call registers both shapes, so the fixture pins both directions:
// `mux.Handle` with a ROUTER is a mount, `mux.Handle` with a plain handler
// value is a route. Gating only on the call name swallowed every ordinary
// handler-value route, which is what the /status case guards against.
func TestTestdata_CrossFrameworkMount(t *testing.T) {
	// nil config on purpose: the fix depends on multi-framework detection
	// merging net/http's patterns alongside chi's, exactly as the CLI does.
	// Passing an explicit single-framework config would bypass the merge and
	// test something this fixture is not about (the mixed_* fixtures do the same).
	out := loadTestdataWithFixtureConfig(t, "cross_framework_mount", nil)
	noDanglingRefs(t, out)
	noUnresolvedPlaceholders(t, out)

	for _, tc := range []struct {
		path, method, shape string
	}{
		{"/api/users", "GET", "chi route under a net/http mount"},
		{"/api/users/{id}", "GET", "chi route with a path param under a mount"},
		{"/status", "GET", "plain handler value on the mounting ServeMux"},
	} {
		item, ok := out.Paths[tc.path]
		if !ok {
			t.Errorf("%s missing (%s); have %v", tc.path, tc.shape, mapPathKeys(out.Paths))
			continue
		}
		if opFor(item, tc.method) == nil {
			t.Errorf("%s %s missing (%s)", tc.method, tc.path, tc.shape)
		}
	}

	// The un-prefixed forms must NOT appear: their presence would mean the
	// mount prefix was dropped (the #138 bug) or applied to a copy.
	for _, bare := range []string{"/users", "/users/{id}"} {
		if _, ok := out.Paths[bare]; ok {
			t.Errorf("%s emitted without its /api mount prefix (#138); have %v", bare, mapPathKeys(out.Paths))
		}
	}
}
