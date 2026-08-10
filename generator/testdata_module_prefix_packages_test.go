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

// TestTestdata_ModulePrefixPackages guards issue #282. The two route-carrying
// packages are `.../module_prefix_packages/api` and `.../app`, whose import
// paths share the byte prefix `.../ap` but no whole path segment beyond the
// module. The dependency analyser used to infer the project root as the
// byte-wise longest common prefix and then use it as a HasPrefix membership
// test; it now takes the module path from go.mod.
//
// The fix narrows what counts as a project package, so the thing worth
// asserting is that nothing legitimate was narrowed away: both packages'
// routes, their params and their bodies must survive.
func TestTestdata_ModulePrefixPackages(t *testing.T) {
	out := loadTestdataWithFixtureConfig(t, "module_prefix_packages", spec.DefaultChiConfig())
	noDanglingRefs(t, out)

	want := map[string][]string{
		"/widgets/{id}":     {"GET"},
		"/widgets":          {"POST"},
		"/sessions/{token}": {"GET"},
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

	// Path params come from the handlers in both sibling packages, so a package
	// wrongly classified as external would show up here as a missing parameter.
	for _, tc := range []struct{ path, param string }{
		{"/widgets/{id}", "id"},
		{"/sessions/{token}", "token"},
	} {
		item, ok := out.Paths[tc.path]
		if !ok {
			continue
		}
		op := opFor(item, "GET")
		if op == nil {
			continue
		}
		var found bool
		for _, p := range op.Parameters {
			if p.Name == tc.param && p.In == "path" {
				found = true
			}
		}
		if !found {
			t.Errorf("GET %s: path parameter %q missing", tc.path, tc.param)
		}
	}

	// The request body on POST /widgets resolves through the api package's own
	// type — a component, not an empty schema.
	if item, ok := out.Paths["/widgets"]; ok {
		if op := opFor(item, "POST"); op != nil && op.RequestBody == nil {
			t.Error("POST /widgets: request body missing")
		}
	}
}
