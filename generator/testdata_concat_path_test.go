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

	"github.com/ehabterra/apispec/spec"
)

// TestTestdata_ConcatPath covers registration paths built by concatenation
// (issue #274) — the shape every oapi-codegen-generated server uses,
// `r.Post(options.BaseURL+"/things", h)`.
//
// The whole argument used to resolve to nothing, so such a route was documented
// at its mount prefix alone, or at "/": the literal part of the path, the only
// part actually written in the source, was the part that disappeared. A project
// whose HTTP layer is generated therefore documented no usable paths at all.
//
// One case per class of operand, since each resolves by a different route, and
// the last one pins the failure mode: an operand nothing can evaluate becomes a
// placeholder rather than vanishing. A shortened path is worse than an obviously
// incomplete one — it claims the handler answers somewhere it does not.
func TestTestdata_ConcatPath(t *testing.T) {
	out := loadTestdataWithFixtureConfig(t, "concat_path", spec.DefaultChiConfig())
	noDanglingRefs(t, out)
	noUnresolvedPlaceholders(t, out)

	for _, tc := range []struct {
		path, method, why string
	}{
		{"/api/health", "GET", "a package-level const operand"},
		{"/things", "POST", "a struct field the caller left at its zero value"},
		{"/v2/items", "GET", "a parameter the caller passed a literal for"},
	} {
		item, ok := out.Paths[tc.path]
		if !ok {
			t.Errorf("%s missing (%s); have %v", tc.path, tc.why, mapPathKeys(out.Paths))
			continue
		}
		if op := opFor(item, tc.method); op == nil {
			t.Errorf("%s %s missing (%s)", tc.method, tc.path, tc.why)
		}
	}

	// The generated route carries its handler's body through, so folding the path
	// must not cost the request schema that hangs off it.
	if item, ok := out.Paths["/things"]; ok {
		if op := opFor(item, "POST"); op != nil {
			if op.RequestBody == nil {
				t.Error("POST /things: the handler decodes a ThingRequest, so a request body is expected")
			} else if mt, ok := op.RequestBody.Content["application/json"]; !ok || mt.Schema == nil ||
				!strings.HasSuffix(mt.Schema.Ref, "_ThingRequest") {
				t.Errorf("POST /things: want a $ref to ThingRequest, got %+v", op.RequestBody.Content)
			}
		}
	}

	// An operand that cannot be evaluated keeps the literal part and names the
	// unknown, instead of silently shortening the path.
	var dynamic []string
	for path := range out.Paths {
		if strings.HasSuffix(path, "/dyn") {
			dynamic = append(dynamic, path)
		}
	}
	sort.Strings(dynamic) // out.Paths is a map: sort so a failure reports the same path every run
	if len(dynamic) == 0 {
		t.Errorf("the route with an unresolvable prefix is gone; have %v", mapPathKeys(out.Paths))
	} else if !strings.Contains(dynamic[0], "{") {
		t.Errorf("unresolvable prefix resolved to %q; it must degrade to a placeholder, not to a shorter path", dynamic[0])
	}
}
