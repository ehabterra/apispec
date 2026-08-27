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
	"strings"
	"testing"

	"github.com/ehabterra/apispec/spec"
)

// TestTestdata_GroupArgInline locks in issue #407: a router group created
// inline in a call's argument list keeps its prefix.
//
//	RegisterRouter(v1.Group("/mod"))   // the group has no variable to key on
//
// The routes were always found; they were documented at the root, because the
// prefix reaches nested routes by tree containment and this group sits beside
// the callee's subtree rather than above it. Two modules registering the same
// relative path then collapse into one path item, so an endpoint disappears
// rather than merely moving — which is why the whole path set is asserted here
// and not just the repaired rows.
//
// The three shapes that already worked are asserted alongside, since the fix
// re-parents registrations and would be a poor trade if it moved those.
func TestTestdata_GroupArgInline(t *testing.T) {
	out := loadTestdataWithFixtureConfig(t, "group_arg_inline", spec.DefaultGinConfig())
	noDanglingRefs(t, out)
	noNullSchemas(t, out)

	want := map[string]string{
		"/v1/inline/a":    "GET",  // created and used at the same site
		"/v1/viavar/d":    "GET",  // assigned to a variable, then passed
		"/v1/nested/c":    "GET",  // parent passed, subgroup made by the callee
		"/v1/samepkg/b":   "GET",  // created in the argument, same package
		"/v1/mod/login":   "POST", // created in the argument, another package
		"/v1/mod/profile": "GET",
	}

	for path, method := range want {
		item, ok := out.Paths[path]
		if !ok {
			t.Errorf("path %q missing; have %v", path, mapPathKeys(out.Paths))
			continue
		}
		op := opFor(item, method)
		if op == nil {
			t.Errorf("%s %s: operation missing", method, path)
			continue
		}
		if len(op.Responses) == 0 {
			t.Errorf("%s %s: no responses — the handler body did not travel with the route", method, path)
		}
	}

	// Every route belongs under /v1; a bare "/b" or "/login" is the defect.
	for path := range out.Paths {
		if !strings.HasPrefix(path, "/v1/") {
			t.Errorf("path %q is documented outside the group it was registered on", path)
		}
	}
	if len(out.Paths) != len(want) {
		t.Errorf("got %d paths, want %d: %v", len(out.Paths), len(want), mapPathKeys(out.Paths))
	}
}
