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

func TestSplitMethodFromPath(t *testing.T) {
	cases := []struct {
		in         string
		wantMethod string
		wantPath   string
	}{
		// Go 1.22 ServeMux method-aware patterns.
		{`"GET /users/{id}"`, "GET", "/users/{id}"},
		{`"POST /users"`, "POST", "/users"},
		{`"get /health"`, "GET", "/health"}, // verb is upper-cased
		{`"DELETE /users/{id}"`, "DELETE", "/users/{id}"},
		// Plain net/http patterns pass through unchanged.
		{`"/health"`, "", "/health"},
		{`"/users/{id}"`, "", "/users/{id}"},
		// A leading non-verb token is not treated as a method.
		{`"FOO /bar"`, "", "FOO /bar"},
		{"", "", ""},
	}
	for _, c := range cases {
		gotMethod, gotPath := splitMethodFromPath(c.in)
		if gotMethod != c.wantMethod || gotPath != c.wantPath {
			t.Errorf("splitMethodFromPath(%q) = (%q, %q); want (%q, %q)",
				c.in, gotMethod, gotPath, c.wantMethod, c.wantPath)
		}
	}
}

func TestNormalizeServeMuxPath(t *testing.T) {
	cases := []struct{ in, want string }{
		{"/users/{id}", "/users/{id}"},
		{"/files/{path...}", "/files/{path}"}, // trailing wildcard
		{"/items/{$}", "/items/"},             // end-of-path anchor dropped
		{"/static/{dir...}/{$}", "/static/{dir}/"},
	}
	for _, c := range cases {
		if got := normalizeServeMuxPath(c.in); got != c.want {
			t.Errorf("normalizeServeMuxPath(%q) = %q; want %q", c.in, got, c.want)
		}
	}
}
