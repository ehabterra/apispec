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

// TestTestdata_EmbeddedRouter locks in issue #643: a framework router embedded
// in a user struct promotes the framework's methods.
//
//	type Server struct{ *echo.Echo }
//	srv.Group("/grp").GET("/inline", h)
//
// The call is written against Server but runs echo's Group. The call graph
// recorded the WRITTEN receiver, which no framework receiver pattern names, so
// the group was never seen as a mount and its routes were documented at the
// root — or, for a route registered straight on the embedding type, not at all.
//
// The shadowing case is the negative: a type that declares its own Group is
// calling its own method, and the prefix it was handed must not be documented
// as if echo's Group had received it.
func TestTestdata_EmbeddedRouter(t *testing.T) {
	out := loadTestdataWithFixtureConfig(t, "embedded_router", spec.DefaultEchoConfig())
	noDanglingRefs(t, out)
	noNullSchemas(t, out)

	cases := []struct {
		path, why, body string
	}{
		{"/direct", "a route registered straight on the embedding type", "Reply"},
		{"/grp/inline", "a group created and used inline on the embedding type", "Reply"},
		{"/var/item", "a group from the embedding type, assigned to a variable", "Item"},
		{"/arg/r", "a group from the embedding type, passed as an argument", "Item"},
		{"/deep/x", "a method promoted through two embedded fields", "Reply"},
	}
	for _, tc := range cases {
		item, ok := out.Paths[tc.path]
		if !ok {
			t.Errorf("%s: path %q missing; have %v", tc.why, tc.path, mapPathKeys(out.Paths))
			continue
		}
		op := opFor(item, "GET")
		if op == nil {
			t.Errorf("%s: GET %s missing", tc.why, tc.path)
			continue
		}
		// /var/item and /arg/r answer through a helper promoted from an embedded
		// user type (Ctx → Base.JSON): the body must still reach the operation.
		if ref := schemaRefAt(op, "200"); !strings.HasSuffix(ref, "_"+tc.body) {
			t.Errorf("%s: GET %s 200 body = %q, want a $ref to %s", tc.why, tc.path, ref, tc.body)
		}
	}

	// Without the fix every prefix above was dropped.
	for _, bare := range []string{"/inline", "/item", "/r", "/x"} {
		if _, ok := out.Paths[bare]; ok {
			t.Errorf("path %q present: a group on the embedding type lost its prefix", bare)
		}
	}

	// Shadow.Group is Shadow's own method. Its argument is not a mount prefix.
	if _, ok := out.Paths["/ignored/s"]; ok {
		t.Errorf("/ignored/s present: the embedding type's own Group was matched as echo's")
	}
	// Change-detector: the honest path is "/fixed/s", the group Shadow.Group
	// returns. A group returned from a helper loses its prefix in general, with
	// or without embedding (issue #633); when that is fixed this flips.
	if _, ok := out.Paths["/s"]; !ok {
		t.Errorf("/s missing; the returned-group gap (#633) changed, update this test: %v", mapPathKeys(out.Paths))
	}
}
