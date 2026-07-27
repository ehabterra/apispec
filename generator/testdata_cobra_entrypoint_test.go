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
)

// TestTestdata_CobraEntrypointRoutes is the cobra half of issue #220, against the
// REAL github.com/spf13/cobra.
//
// cobra is the most common command library in Go services, and its dispatcher is
// as external as urfave/cli's: `root.Execute()` reaches `RunE` through cobra's
// internals, so nothing in the module calls the function that registers the
// routes. Until the entrypoint presets, such a project documented nothing.
//
// The fixture covers both command bodies (`RunE` and `Run`) because a preset that
// named only one would look fine here and fail on half of real programs, plus a
// route-less command as the control for the "must reach a route registration"
// gate.
func TestTestdata_CobraEntrypointRoutes(t *testing.T) {
	out := loadTestdataWithFixtureConfig(t, "cobra_entrypoint_routes", nil)
	noDanglingRefs(t, out)
	noUnresolvedPlaceholders(t, out)

	// RunE: the serve command's routes, with their handler bodies.
	widgets, ok := out.Paths["/widgets"]
	if !ok {
		t.Fatalf("/widgets missing — cobra's RunE entrypoint was not rooted (#220); have %v", mapPathKeys(out.Paths))
	}
	get := opFor(widgets, "GET")
	post := opFor(widgets, "POST")
	if get == nil || post == nil {
		t.Fatalf("/widgets: want GET and POST, got get=%v post=%v", get != nil, post != nil)
	}
	if _, hasOK := get.Responses["200"]; !hasOK {
		t.Errorf("GET /widgets: 200 missing — the handler body was not reached; have %v", keysOf(get.Responses))
	}
	if post.RequestBody == nil {
		t.Error("POST /widgets: requestBody missing — the handler body was not reached")
	} else if ref := contentSchemaRef(post.RequestBody.Content); !strings.HasSuffix(ref, "_Widget") {
		t.Errorf("POST /widgets requestBody = %q, want Widget", ref)
	}

	// Run (no error return) is a separate field and needs its own coverage.
	if version := opFor(out.Paths["/version"], "GET"); version == nil {
		t.Errorf("GET /version missing — cobra's Run entrypoint was not rooted (#220); have %v",
			mapPathKeys(out.Paths))
	}

	// The migrate command registers nothing, so it must not contribute paths.
	if len(out.Paths) != 2 {
		t.Errorf("expected only /widgets and /version, have %v — a route-less command may have been expanded",
			mapPathKeys(out.Paths))
	}
}
