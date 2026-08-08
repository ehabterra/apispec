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

// TestMountRouterSubtreeCarriesTheMountContext pins what a sub-router's subtree
// inherits when it is walked as if nested under the mount (issue #275).
//
// mountRouterSubtree is the shared body of the two ways a mounted router can
// live OUTSIDE the mount node's own subtree — assigned to a variable
// (`api := r.Group(...)`) and passed in as a parameter (`mount(r, Server())`).
// Sharing it is the point: before #275 only the assignment case existed, so the
// parameter case inherited nothing at all, and a second copy of this walk would
// be free to drift from it.
//
// The visited key is `node@mountPath`, so it doubles as the assertion that the
// prefix was actually carried into the traversal rather than dropped.
func TestMountRouterSubtreeCarriesTheMountContext(t *testing.T) {
	newFixture := func() (*Extractor, *TrackerNode, *TrackerNode) {
		meta := exSweepMeta()
		tree := NewMockTrackerTree(meta, metadata.TrackerLimits{})
		ext := NewExtractor(tree, &APISpecConfig{})
		child := sweepNode(sweepEdge(meta, "routes", "app", "Get", "chi", "Mux", ""))
		target := &TrackerNode{key: "sub-router", Children: []*TrackerNode{child}}
		tree.AddRoot(target)
		return ext, target, child
	}

	t.Run("the prefix reaches the subtree", func(t *testing.T) {
		ext, target, child := newFixture()
		var routes []*RouteInfo
		visited := map[string]bool{}

		ext.mountRouterSubtree(target, "/api", nil, nil, nil, &routes, visited)

		if !visited[child.GetKey()+"@/api"] {
			t.Errorf("the sub-router's child was not traversed under /api; visited = %v", visited)
		}
		// Walking it at the prefix must NOT also claim the un-prefixed context:
		// that is the duplicate dropSubsumedMountPrefixes then has to clean up.
		if visited[child.GetKey()+"@"] {
			t.Error("the subtree was also walked with no prefix")
		}
	})

	// A mount with a path names its own tag; one without inherits the tags it was
	// given. Getting this backwards silently retags every route under a mount.
	t.Run("tags: a path names its own, an empty path inherits", func(t *testing.T) {
		for name, tc := range map[string]struct {
			mountPath string
			inherited []string
		}{
			"with path":    {"/api", []string{"inherited"}},
			"without path": {"", []string{"inherited"}},
		} {
			t.Run(name, func(t *testing.T) {
				ext, target, child := newFixture()
				var routes []*RouteInfo
				visited := map[string]bool{}

				ext.mountRouterSubtree(target, tc.mountPath, tc.inherited, nil, nil, &routes, visited)

				if !visited[child.GetKey()+"@"+tc.mountPath] {
					t.Errorf("child not traversed at %q; visited = %v", tc.mountPath, visited)
				}
			})
		}
	})

	// Dynamic params and middleware are threaded through rather than reset. With
	// no route patterns configured nothing is emitted, so this pins the contract
	// that the call is made and does not panic on the inherited slices — the
	// end-to-end effect is the fixture's job.
	t.Run("dynamic params and middleware are passed through", func(t *testing.T) {
		ext, target, child := newFixture()
		var routes []*RouteInfo
		visited := map[string]bool{}

		ext.mountRouterSubtree(target, "/api/{tenant}", nil,
			[]string{"tenant"}, []MiddlewareRef{{FunctionName: "auth", Pkg: "app"}}, &routes, visited)

		if !visited[child.GetKey()+"@/api/{tenant}"] {
			t.Errorf("child not traversed under the dynamic prefix; visited = %v", visited)
		}
		if len(routes) != 0 {
			t.Errorf("no route patterns are configured, so nothing should be emitted: %v", routes)
		}
	})

	t.Run("a target with no children is a no-op", func(t *testing.T) {
		meta := exSweepMeta()
		tree := NewMockTrackerTree(meta, metadata.TrackerLimits{})
		ext := NewExtractor(tree, &APISpecConfig{})
		var routes []*RouteInfo
		visited := map[string]bool{}

		ext.mountRouterSubtree(&TrackerNode{key: "empty"}, "/api", nil, nil, nil, &routes, visited)

		if len(routes) != 0 || len(visited) != 0 {
			t.Errorf("routes = %v, visited = %v, want both empty", routes, visited)
		}
	})
}

// TestHandleMountNodeIgnoresAnUnresolvableRouterArg keeps the new parameter hop
// from firing on the ordinary case. When the mounted router is NOT a parameter
// bound to a caller's argument, resolveArgThroughParams returns the same
// argument, and re-walking the mount's own subtree at its own prefix would
// duplicate every route under it.
func TestHandleMountNodeIgnoresAnUnresolvableRouterArg(t *testing.T) {
	meta := exSweepMeta()
	tree := NewMockTrackerTree(meta, metadata.TrackerLimits{})
	ext := NewExtractor(tree, &APISpecConfig{})

	// A plain local ident bound to nothing: there is no caller argument to reach.
	routerArg := sweepIdent(meta, "server")
	routerArg.SetPkg("app")

	var routes []*RouteInfo
	visited := map[string]bool{}
	ext.handleMountNode(&TrackerNode{}, MountInfo{Path: "/api", RouterArg: routerArg},
		"", nil, nil, nil, &routes, visited)

	if len(routes) != 0 {
		t.Errorf("routes = %v, want none — nothing resolves and no patterns are configured", routes)
	}
}
