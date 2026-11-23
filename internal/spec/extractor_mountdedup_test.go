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

import (
	"testing"

	"github.com/ehabterra/apispec/internal/metadata"
)

func TestIsSubsequence(t *testing.T) {
	cases := []struct {
		a, b []string
		want bool
	}{
		{nil, []string{"api", "user"}, true},                     // empty is a subsequence of anything
		{[]string{"api"}, []string{"api", "user"}, true},         // prefix
		{[]string{"user"}, []string{"api", "user"}, true},        // suffix
		{[]string{"api", "user"}, []string{"api", "user"}, true}, // equal
		{[]string{"v2", "api"}, []string{"mountPoint"}, false},   // distinct mounts
		{[]string{"mountPoint"}, []string{"v2", "api"}, false},
		{[]string{"user", "api"}, []string{"api", "user"}, false}, // order matters
	}
	for _, c := range cases {
		if got := isSubsequence(c.a, c.b); got != c.want {
			t.Errorf("isSubsequence(%v,%v)=%v want %v", c.a, c.b, got, c.want)
		}
	}
}

func TestMountSegments(t *testing.T) {
	cases := map[string][]string{
		"":           nil,
		"/":          nil,
		"/api/":      {"api"},
		"/api/user/": {"api", "user"},
		"api/user":   {"api", "user"},
		"/{mount}/":  {"{mount}"},
		"/v2/api":    {"v2", "api"},
	}
	for in, want := range cases {
		got := mountSegments(in)
		if len(got) != len(want) {
			t.Errorf("mountSegments(%q)=%v want %v", in, got, want)
			continue
		}
		for i := range got {
			if got[i] != want[i] {
				t.Errorf("mountSegments(%q)=%v want %v", in, got, want)
				break
			}
		}
	}
}

func TestDropSubsumedMountPrefixes(t *testing.T) {
	mk := func(fn, method, path, mount string) *RouteInfo {
		return &RouteInfo{Function: fn, Method: method, Path: path, MountPath: mount}
	}

	t.Run("nested mount keeps only the most-mounted", func(t *testing.T) {
		// user.Handler.delete reached at every subset of [api,user].
		routes := []*RouteInfo{
			mk("user.delete", "DELETE", "/{id}", ""),
			mk("user.delete", "DELETE", "/{id}", "/api/"),
			mk("user.delete", "DELETE", "/{id}", "/user/"),
			mk("user.delete", "DELETE", "/{id}", "/api/user/"),
		}
		got := dropSubsumedMountPrefixes(routes)
		if len(got) != 1 || got[0].MountPath != "/api/user/" {
			t.Fatalf("want only /api/user/, got %+v", mountsOf(got))
		}
	})

	t.Run("distinct multi-mounts both survive", func(t *testing.T) {
		// Same sub-router mounted at two genuinely different prefixes.
		routes := []*RouteInfo{
			mk("api.list", "GET", "/{id}", "/v2/api/"),
			mk("api.list", "GET", "/{id}", "/{mountPoint}/"),
		}
		got := dropSubsumedMountPrefixes(routes)
		if len(got) != 2 {
			t.Fatalf("both distinct mounts should survive, got %+v", mountsOf(got))
		}
	})

	t.Run("different handlers at same path are untouched", func(t *testing.T) {
		routes := []*RouteInfo{
			mk("main.FuncLit", "GET", "/", ""),
			mk("user.list", "GET", "/", "/api/user/"),
		}
		got := dropSubsumedMountPrefixes(routes)
		if len(got) != 2 {
			t.Fatalf("distinct functions must not be deduped, got %d", len(got))
		}
	})
}

// TestDropSubsumedMountPrefixesScopedToCallSite pins issue #614: subsumption
// only collapses copies of ONE registration call. Two registrations of a handler
// at prefixes that extend each other are both endpoints, even though the
// shorter mount is a subsequence of the longer.
func TestDropSubsumedMountPrefixesScopedToCallSite(t *testing.T) {
	meta := &metadata.Metadata{StringPool: metadata.NewStringPool()}
	site := func(pos string) *metadata.CallGraphEdge {
		return &metadata.CallGraphEdge{Callee: metadata.Call{
			Meta:     meta,
			Pkg:      meta.StringPool.Get("github.com/gin-gonic/gin"),
			Name:     meta.StringPool.Get("GET"),
			RecvType: meta.StringPool.Get("*RouterGroup"),
			Position: meta.StringPool.Get(pos),
		}}
	}
	at := func(edge *metadata.CallGraphEdge, mount, status string) *RouteInfo {
		r := NewRouteInfo()
		r.Function, r.Method, r.Path, r.MountPath = "main.getAttachment", "GET", "/attachments/:uuid", mount
		r.Node = &TrackerNode{CallGraphEdge: edge}
		r.Response[status] = &ResponseInfo{StatusCode: 0}
		return r
	}

	t.Run("same-site echo is dropped and folded", func(t *testing.T) {
		// One registration reached under a partial and a full mount context:
		// distinct tree nodes over the same call-graph edge.
		edge := site("main.go:10:7")
		echo := at(edge, "/:owner/:repo/", "404")
		full := at(edge, "/:owner/:repo/issues/:index/", "200")
		got := dropSubsumedMountPrefixes([]*RouteInfo{echo, full})
		if len(got) != 1 || got[0] != full {
			t.Fatalf("want only the most-mounted copy, got %v", mountsOf(got))
		}
		if _, ok := full.Response["404"]; !ok {
			t.Error("the echo's extraction must fold into the survivor")
		}
	})

	t.Run("different-site extending prefixes both survive", func(t *testing.T) {
		short := at(site("main.go:10:7"), "/:owner/:repo/", "404")
		long := at(site("main.go:11:30"), "/:owner/:repo/issues/:index/", "200")
		got := dropSubsumedMountPrefixes([]*RouteInfo{short, long})
		if len(got) != 2 {
			t.Fatalf("two registrations are two endpoints, got %v", mountsOf(got))
		}
		if _, ok := long.Response["404"]; ok {
			t.Error("a separate registration's extraction must not fold into another")
		}
	})

	t.Run("same line in another file is another site", func(t *testing.T) {
		short := at(site("a.go:10:7"), "/:owner/:repo/", "404")
		long := at(site("b.go:10:7"), "/:owner/:repo/issues/:index/", "200")
		if got := dropSubsumedMountPrefixes([]*RouteInfo{short, long}); len(got) != 2 {
			t.Fatalf("the site must include the file, got %v", mountsOf(got))
		}
	})
}

func mountsOf(rs []*RouteInfo) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = r.MountPath
	}
	return out
}
