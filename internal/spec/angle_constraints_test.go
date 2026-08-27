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

package spec

import "testing"

// TestStripAngleConstraints covers the constraint tail fiber puts after a path
// parameter (issue #357), including the two shapes that make it more than a
// substring cut: an argument list holding arbitrary text, and an unterminated
// bracket that must not swallow the rest of the path.
func TestStripAngleConstraints(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"no constraint", "/items/:id", "/items/:id"},
		{"nothing at all", "/health", "/health"},
		{"simple", "/items/:id<int>", "/items/:id"},
		{"guid", "/users/:uid<guid>", "/users/:uid"},
		{"argument with punctuation", "/events/:day<datetime(2006-01-02)>", "/events/:day"},
		{"chained with ;", "/pages/:n<min(1);max(500)>", "/pages/:n"},
		{"two constrained params", "/a/:x<int>/b/:y<guid>", "/a/:x/b/:y"},
		{"constrained then plain", "/a/:x<int>/b/:y", "/a/:x/b/:y"},
		{"trailing segment survives", "/items/:id<int>/children", "/items/:id/children"},
		{
			// The reason parens are tracked instead of cutting at the first '>':
			// the constraint's own argument can contain one.
			name: "closing bracket inside the argument",
			in:   "/re/:s<regex([^>]+)>/tail",
			want: "/re/:s/tail",
		},
		{
			name: "nested parens in the argument",
			in:   "/re/:s<regex((a|b)+)>",
			want: "/re/:s",
		},
		{
			// Not a constraint at all. Dropping to end-of-string here would
			// delete a real path segment, so the remainder is kept verbatim.
			name: "unterminated bracket keeps the remainder",
			in:   "/items/:id<int/children",
			want: "/items/:id<int/children",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := stripAngleConstraints(tc.in); got != tc.want {
				t.Errorf("stripAngleConstraints(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestConvertPathToOpenAPIWithConstraints checks the strip through the real
// conversion, so the constraint is gone AND the parameter still becomes a
// template placeholder — the two halves that must not trade against each other.
func TestConvertPathToOpenAPIWithConstraints(t *testing.T) {
	cases := []struct{ in, want string }{
		{"/items/:id<int>", "/items/{id}"},
		{"/pages/:n<min(1);max(500)>", "/pages/{n}"},
		{"/plain/:name", "/plain/{name}"},
		// mux/chi's own constraint form must be unaffected by the new strip.
		{"/items/{id:[0-9]+}", "/items/{id}"},
	}
	for _, tc := range cases {
		got, _ := convertPathToOpenAPI(tc.in)
		if got != tc.want {
			t.Errorf("convertPathToOpenAPI(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
