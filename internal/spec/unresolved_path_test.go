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

func TestPathIsAllPlaceholders(t *testing.T) {
	cases := []struct {
		name    string
		path    string
		dynamic []string
		want    bool
	}{
		{
			// The table-driven registration: the verb and the path are both
			// read off a struct at runtime.
			name: "verb and path both synthesized", path: "{Method} {Path}",
			dynamic: []string{"Method", "Path"}, want: true,
		},
		{
			// The chained house router, after the literal verb was split off.
			name: "bare placeholder", path: "{pattern}",
			dynamic: []string{"pattern"}, want: true,
		},
		{
			name: "placeholder with separators only", path: "/{pattern}/",
			dynamic: []string{"pattern"}, want: true,
		},
		{
			// #274: an unresolved prefix with a literal tail is a real endpoint
			// documented approximately, not a non-endpoint.
			name: "unresolved prefix, literal tail", path: "{dynamicBase}/dyn",
			dynamic: []string{"dynamicBase"}, want: false,
		},
		{
			// A route's own path is fully literal; the placeholder in the
			// EMITTED path comes from the mount chain, which is not this test's
			// input (see the doc comment).
			name: "no placeholders at all", path: "/users", dynamic: nil, want: false,
		},
		{
			// A genuine path parameter is content: the endpoint is real and
			// parameterized. Only SYNTHESIZED names are subtracted.
			name: "real path parameter", path: "/{id}", dynamic: nil, want: false,
		},
		{
			name: "synthesized alongside a real parameter", path: "{base}/{id}",
			dynamic: []string{"base"}, want: false,
		},
		{name: "empty path", path: "", dynamic: []string{"x"}, want: false},
	}
	for _, c := range cases {
		if got := pathIsAllPlaceholders(c.path, c.dynamic); got != c.want {
			t.Errorf("%s: pathIsAllPlaceholders(%q, %v) = %v, want %v",
				c.name, c.path, c.dynamic, got, c.want)
		}
	}
}

func TestUndocumentablePath(t *testing.T) {
	cases := []struct {
		name  string
		route *RouteInfo
		want  bool
	}{
		{
			// Nothing about the location is known.
			name: "bare placeholder path",
			route: &RouteInfo{Path: "/{pattern}", DynamicParams: []string{"pattern"},
				PathUnresolved: true},
			want: true,
		},
		{
			// The verb went unread too, so the key carries a space: not a path
			// template at any prefix.
			name: "unreadable verb leaves whitespace",
			route: &RouteInfo{Path: "/{Method} {Path}", DynamicParams: []string{"Method", "Path"},
				PathUnresolved: true},
			want: true,
		},
		{
			name: "unreadable verb under a literal mount",
			route: &RouteInfo{MountPath: "/api", Path: "/{Method} {Path}",
				DynamicParams: []string{"Method", "Path"}, PathUnresolved: true},
			want: true,
		},
		{
			// gitea's Any() through its wrapper: the tail is approximate, the
			// endpoint is locatable. Measured — dropping this deleted three real
			// operations from gitea's spec.
			name: "unreadable tail under a literal mount",
			route: &RouteInfo{MountPath: "/repo/{username}/info/lfs", Path: "/{path}",
				DynamicParams: []string{"path"}, PathUnresolved: true},
			want: false,
		},
		{
			// The subrouter's root, mounted somewhere unreadable: the route's
			// own path resolved, so it is not this rule's business.
			name: "resolved path under an unreadable mount",
			route: &RouteInfo{MountPath: "/{mountPoint}", Path: "/",
				DynamicParams: []string{"mountPoint"}},
			want: false,
		},
		{name: "ordinary route", route: &RouteInfo{Path: "/users"}, want: false},
		{name: "nil route", route: nil, want: false},
	}
	for _, c := range cases {
		reason, got := undocumentablePath(c.route)
		if got != c.want {
			t.Errorf("%s: undocumentablePath() = %v (%q), want %v", c.name, got, reason, c.want)
		}
		if got && reason == "" {
			t.Errorf("%s: a dropped route must carry a reason", c.name)
		}
	}
}

func TestDropUnresolvedPathRoutes(t *testing.T) {
	keep := &RouteInfo{Path: "/health", Method: "GET", Function: "pkg.health"}
	partly := &RouteInfo{Path: "/{prefix}/partly", Method: "GET", Function: "pkg.partly",
		DynamicParams: []string{"prefix"}}
	drop := &RouteInfo{Path: "/{pattern}", Method: "POST", Function: "pkg.h",
		DynamicParams: []string{"pattern"}, PathUnresolved: true}

	kept, reports := dropUnresolvedPathRoutes([]*RouteInfo{keep, drop, partly})
	if len(kept) != 2 || kept[0] != keep || kept[1] != partly {
		t.Fatalf("want the documentable routes kept in order, got %+v", kept)
	}
	if len(reports) != 1 {
		t.Fatalf("want one report, got %d: %+v", len(reports), reports)
	}
	if reports[0].Path != "/{pattern}" || reports[0].Method != "POST" || reports[0].Handler != "pkg.h" {
		t.Errorf("report = %+v, want the dropped route's path, method and handler", reports[0])
	}
	// No node on a hand-built route: the position is unknown, not a panic.
	if reports[0].Position != "" {
		t.Errorf("position = %q, want empty for a route with no registration node", reports[0].Position)
	}

	// Nothing to drop: the input slice is returned as it is, so the ordinary
	// case allocates nothing.
	in := []*RouteInfo{keep, partly}
	out, none := dropUnresolvedPathRoutes(in)
	if none != nil {
		t.Errorf("want no reports, got %+v", none)
	}
	if len(out) != len(in) || &out[0] != &in[0] {
		t.Error("with nothing to drop the routes must pass through untouched")
	}
}
