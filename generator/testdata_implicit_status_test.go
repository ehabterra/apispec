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
	"testing"

	"github.com/ehabterra/apispec/spec"
)

// TestTestdata_ImplicitStatus locks in issue #369: a net/http handler that
// writes a body without calling WriteHeader sends 200, so that is what the spec
// must say. Documenting it under "default" gave the endpoint no success
// response at all — client generators map "default" to the error branch.
//
// The fixture covers the three shapes the fix has to keep apart: the status is
// implied, the status is stated, and one branch states it while the other does
// not.
func TestTestdata_ImplicitStatus(t *testing.T) {
	out := loadTestdataWithFixtureConfig(t, "implicit_status", spec.DefaultHTTPConfig())
	noDanglingRefs(t, out)

	want := map[string]map[string][]string{
		"/items/{id}":      {"GET": {"200"}, "DELETE": {"204"}},
		"/items":           {"GET": {"200", "400"}, "POST": {"201"}},
		"/items/{id}/raw":  {"GET": {"200"}},
		"/items/{id}/etag": {"GET": {"default"}},
	}
	for path, byMethod := range want {
		item, ok := out.Paths[path]
		if !ok {
			t.Errorf("path %q missing; have %v", path, mapPathKeys(out.Paths))
			continue
		}
		for method, wantStatuses := range byMethod {
			op := opFor(item, method)
			if op == nil {
				t.Errorf("%s %s: expected operation, missing", method, path)
				continue
			}
			got := make([]string, 0, len(op.Responses))
			for status := range op.Responses {
				got = append(got, status)
			}
			sort.Strings(got)
			if !equalStrings(got, wantStatuses) {
				t.Errorf("%s %s: responses %v, want %v", method, path, got, wantStatuses)
			}
		}
	}

	// The implied 200 is a real response, not an empty slot: it carries the
	// body the handler encodes.
	if op := opFor(out.Paths["/items/{id}"], "GET"); op != nil {
		if resp, ok := op.Responses["200"]; !ok || !namesAComponent(resp.Content) {
			t.Errorf("GET /items/{id}: the implied 200 should carry the Item schema, got %+v", op.Responses)
		}
	}
	// A stated status still wins over the implied one — the encode that follows
	// WriteHeader(201) must not also produce a 200.
	if op := opFor(out.Paths["/items"], "POST"); op != nil {
		if _, ok := op.Responses["200"]; ok {
			t.Errorf("POST /items states 201 before the body; it must not also document 200: %+v", op.Responses)
		}
	}
	// The other half of the rule: a handler that DOES state a status apispec
	// cannot read keeps "default". Saying 200 there would be a guess — the
	// server sends whatever that WriteHeader was given (golden rule #7).
	if op := opFor(out.Paths["/items/{id}/etag"], "GET"); op != nil {
		if _, ok := op.Responses["200"]; ok {
			t.Errorf("GET /items/{id}/etag writes an unreadable status; its body must not be claimed as an implicit 200: %+v",
				op.Responses)
		}
	}
	// Every other operation resolves, so nothing else may land under "default".
	for path, item := range out.Paths {
		if path == "/items/{id}/etag" {
			continue
		}
		for _, method := range []string{"GET", "POST", "PUT", "DELETE"} {
			op := opFor(item, method)
			if op == nil {
				continue
			}
			if _, ok := op.Responses["default"]; ok {
				t.Errorf("%s %s: the status here is determinable, but a default response was emitted: %+v",
					method, path, op.Responses)
			}
		}
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestTestdata_NetHTTPSuccessStatusesAreExplicit is the regression half of
// issue #369 on the fixtures that showed it worst: every success response in
// the net/http-family fixtures used to sit under "default" (8 of them in chi
// alone) because their handlers encode without calling WriteHeader. None of
// these fixtures has a genuinely undeterminable status, so a "default" here
// means the implied 200 stopped being resolved.
func TestTestdata_NetHTTPSuccessStatusesAreExplicit(t *testing.T) {
	for _, tc := range []struct {
		fixture string
		cfg     *spec.APISpecConfig
	}{
		{"chi", spec.DefaultChiConfig()},
		{"mux", spec.DefaultMuxConfig()},
		{"servemux", spec.DefaultHTTPConfig()},
	} {
		t.Run(tc.fixture, func(t *testing.T) {
			out := loadTestdataWithFixtureConfig(t, tc.fixture, tc.cfg)
			okCount := 0
			for path, item := range out.Paths {
				for _, method := range []string{"GET", "POST", "PUT", "DELETE", "PATCH", "HEAD"} {
					op := opFor(item, method)
					if op == nil {
						continue
					}
					if _, bad := op.Responses["default"]; bad {
						t.Errorf("%s %s: success body documented under \"default\" instead of the implied 200 (#369): %+v",
							method, path, op.Responses)
					}
					if _, ok := op.Responses["200"]; ok {
						okCount++
					}
				}
			}
			if okCount == 0 {
				t.Errorf("%s documents no 200 at all, so this fixture no longer covers the implied-status path", tc.fixture)
			}
		})
	}
}
