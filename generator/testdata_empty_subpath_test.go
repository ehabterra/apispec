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

// TestTestdata_EmptySubpath locks in issue #609: a route registered with an
// empty path under a group is served at the group's own path.
//
//	users := api.Group("/users")
//	users.GET("", list)    // /api/users — was documented as /api/users/
//	orgs.GET("/", list)    // /api/orgs/ — an explicit "/" IS a trailing slash
//
// The empty path used to be normalised to "/" at extraction, and joinPaths
// always inserted a separator, so the two spellings collapsed into the one
// the framework does not serve. A client generated from the spec then called
// the wrong URL. The whole path set is asserted per framework so a slash
// creeping back onto any route fails loud, and the gin fixture keeps the
// explicit-"/" route beside the empty ones — and the chi fixture pins the same
// for Route and Mount prefixes — so the fix cannot simply strip every trailing
// slash.
func TestTestdata_EmptySubpath(t *testing.T) {
	cases := []struct {
		fixture string
		cfg     *spec.APISpecConfig
		want    map[string][]string
	}{
		{
			fixture: "empty_subpath_gin",
			cfg:     spec.DefaultGinConfig(),
			want: map[string][]string{
				"/":               {"GET"}, // empty path on the engine itself
				"/api/users":      {"GET", "POST"},
				"/api/users/{id}": {"GET"},
				"/api/health":     {"GET"}, // a Group("") adds nothing to the prefix
				"/api/orgs/":      {"GET"}, // explicit "/" keeps its slash
			},
		},
		{
			// chi has no empty-path form; it is the explicit-"/" control on
			// the mount path, where the prefix arrives through a Route or
			// Mount rather than a group (issue #609's acceptance).
			fixture: "empty_subpath_chi",
			cfg:     spec.DefaultChiConfig(),
			want: map[string][]string{
				"/items/":     {"GET"},
				"/items/{id}": {"GET"},
				"/users/":     {"GET"},
			},
		},
		{
			fixture: "empty_subpath_echo",
			cfg:     spec.DefaultEchoConfig(),
			want: map[string][]string{
				"/api/articles":        {"GET", "POST"},
				"/api/articles/{slug}": {"GET"},
			},
		},
		{
			fixture: "empty_subpath_fiber",
			cfg:     spec.DefaultFiberConfig(),
			want: map[string][]string{
				"/api/items":      {"GET", "POST"},
				"/api/items/{id}": {"GET"},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.fixture, func(t *testing.T) {
			out := loadTestdataWithFixtureConfig(t, tc.fixture, tc.cfg)
			noDanglingRefs(t, out)
			noNullSchemas(t, out)

			for path, methods := range tc.want {
				item, ok := out.Paths[path]
				if !ok {
					t.Errorf("path %q missing; have %v", path, mapPathKeys(out.Paths))
					continue
				}
				for _, method := range methods {
					op := opFor(item, method)
					if op == nil {
						t.Errorf("%s %s: operation missing", method, path)
						continue
					}
					if len(op.Responses) == 0 {
						t.Errorf("%s %s: no responses — the handler did not travel with the route", method, path)
					}
				}
			}
			if len(out.Paths) != len(tc.want) {
				t.Errorf("got %d paths, want %d: %v", len(out.Paths), len(tc.want), mapPathKeys(out.Paths))
			}
		})
	}
}
