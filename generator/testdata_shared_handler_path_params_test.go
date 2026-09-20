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

	intspec "github.com/ehabterra/apispec/internal/spec"
)

// TestTestdata_SharedHandlerPathParams pins that an operation declares exactly
// the path parameters its own template binds (issue #514).
//
// Parameters are attributed to a handler, and a handler can be mounted at more
// than one template — so it reads the union of their names and used to emit the
// union on both. The shorter route then declared a parameter its path has
// nowhere to bind, which OpenAPI forbids; it is the sort of defect a linter
// catches and the generator should not produce.
func TestTestdata_SharedHandlerPathParams(t *testing.T) {
	out := loadTestdataWithFixtureConfig(t, "shared_handler_path_params", intspec.DefaultChiConfig())
	noDanglingRefs(t, out)
	noUnresolvedPlaceholders(t, out)

	cases := []struct {
		name         string
		method, path string
		want         []string
		why          string
	}{
		{
			name: "the shorter of two templates",
			// `id` belongs to the sibling route only.
			method: "GET", path: "/codes/{codeId}/thing", want: []string{"codeId"},
			why: "the handler also reads id, which this template cannot bind",
		},
		{
			name:   "the longer of two templates",
			method: "GET", path: "/groups/{id}/codes/{codeId}/thing", want: []string{"codeId", "id"},
			why: "both names are declared here, so both belong",
		},
		{
			name: "a query parameter alongside a foreign path name",
			// Only the path name is bound to the template; the query parameter
			// is not, and must survive on both routes.
			method: "GET", path: "/reports", want: []string{"page"},
			why: "orgId belongs to the sibling; page is a query parameter and is not template-bound",
		},
		{
			name:   "the sibling that does declare it",
			method: "GET", path: "/orgs/{orgId}/reports", want: []string{"orgId", "page"},
			why: "the same handler, where orgId does bind",
		},
		{
			name:   "one handler, one route",
			method: "GET", path: "/users/{userId}", want: []string{"userId"},
			why: "the ordinary case must be untouched",
		},
		{
			name: "a misspelled read",
			// The handler reads teamID; the path declares teamId. The read is
			// always empty, so it is not a parameter — and the real one is
			// still declared, synthesized from the template.
			method: "GET", path: "/teams/{teamId}", want: []string{"teamId"},
			why: "a name no route declares cannot be a path parameter",
		},
		{
			name:   "a catch-all",
			method: "GET", path: "/static/{wildcard}", want: []string{"wildcard"},
			why: "the router matching the rest of the path is declared by the template",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			op := opFor(out.Paths[tc.path], tc.method)
			if op == nil {
				t.Fatalf("%s %s missing; have %v", tc.method, tc.path, mapPathKeys(out.Paths))
			}
			got := make([]string, 0, len(op.Parameters))
			for _, p := range op.Parameters {
				got = append(got, p.Name)
			}
			sort.Strings(got)
			want := append([]string(nil), tc.want...)
			sort.Strings(want)
			if strings.Join(got, ",") != strings.Join(want, ",") {
				t.Errorf("%s %s parameters = %v, want %v — %s", tc.method, tc.path, got, want, tc.why)
			}
		})
	}

	// The invariant behind all of it, asserted over the whole document so a
	// future route cannot reintroduce the defect somewhere else: every `in:
	// path` parameter must appear in its own template. This is what
	// `redocly lint` reports as path-parameters-defined.
	for path, item := range out.Paths {
		declared := make(map[string]bool)
		for _, name := range pathTemplateNames(path) {
			declared[name] = true
		}
		for _, method := range []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"} {
			op := opFor(item, method)
			if op == nil {
				continue
			}
			for _, p := range op.Parameters {
				if p.In == "path" && p.Name != "" && !declared[p.Name] {
					t.Errorf("%s %s declares path parameter %q, which the template does not bind", method, path, p.Name)
				}
			}
		}
	}
}

// pathTemplateNames returns the {placeholder} names of an OpenAPI path.
func pathTemplateNames(path string) []string {
	var names []string
	for {
		open := strings.IndexByte(path, '{')
		if open < 0 {
			return names
		}
		close := strings.IndexByte(path[open:], '}')
		if close < 0 {
			return names
		}
		names = append(names, path[open+1:open+close])
		path = path[open+close:]
	}
}
