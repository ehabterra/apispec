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
	"reflect"
	"testing"

	"github.com/ehabterra/apispec/internal/metadata"
)

// closureMeta builds a call graph in the shape that matters for #264:
//
//	main        -> registerAPI          (a plain call)
//	registerAPI -> chi.Router.Route     (the group call; its closure is an ARGUMENT)
//	FuncLit     -> chi.Router.Get       (the registration, written inside the closure)
//	main        -> loadSettings         (a plain call that leads nowhere near a route)
//	loadSettings-> ini.Section.Key
//
// The closure's edges carry ParentFunction = registerAPI, which is the only link
// back to the function that writes the group — nothing CALLS the closure.
func closureMeta(t *testing.T) *metadata.Metadata {
	t.Helper()
	m := &metadata.Metadata{StringPool: metadata.NewStringPool()}
	call := func(pkg, recv, name string) metadata.Call {
		return metadata.Call{
			Meta: m, Name: m.StringPool.Get(name), Pkg: m.StringPool.Get(pkg),
			RecvType: m.StringPool.Get(recv), Position: -1, Scope: -1, SignatureStr: -1,
		}
	}
	const app = "example.com/app"
	const chi = "github.com/go-chi/chi/v5"

	registerAPI := call(app, "", "registerAPI")
	edges := []metadata.CallGraphEdge{
		{Caller: call(app, "", "main"), Callee: registerAPI},
		{Caller: registerAPI, Callee: call(chi, "Router", "Route")},
		// Written inside registerAPI's closure; reachable only via ParentFunction.
		{
			Caller:         call(app, "", "FuncLit:main.go:10:20"),
			Callee:         call(chi, "Router", "Get"),
			ParentFunction: &registerAPI,
		},
		{Caller: call(app, "", "main"), Callee: call(app, "", "loadSettings")},
		{Caller: call(app, "", "loadSettings"), Callee: call("gopkg.in/ini.v1", "Section", "Key")},
	}
	m.CallGraph = append(m.CallGraph, edges...)
	m.BuildCallGraphMaps()
	return m
}

// registersRoute matches the chi verb call the fixture registers with.
func registersRoute(m *metadata.Metadata) func(*metadata.CallGraphEdge) bool {
	return func(edge *metadata.CallGraphEdge) bool {
		return m.StringPool.GetString(edge.Callee.Name) == "Get"
	}
}

// TestRouteReachSetFollowsFuncLiterals is the property the whole of #264 rests
// on, and the one a plain call-graph walk gets wrong.
//
// A group closure is an ARGUMENT, not a callee, so nothing calls it. Reachability
// that follows only call edges therefore reports that `registerAPI` reaches no
// route — false for the single most common way real projects group routes, and
// exactly why an earlier attempt to PRUNE by pattern reachability deleted 124 of
// 312 operations.
func TestRouteReachSetFollowsFuncLiterals(t *testing.T) {
	m := closureMeta(t)
	reach := routeReachSet(m, registersRoute(m))

	const app = "example.com/app"
	for _, key := range []string{
		app + ".FuncLit:main.go:10:20", // registers directly
		app + ".registerAPI",           // only via the closure it writes
		app + ".main",                  // only via registerAPI
	} {
		if !reach[key] {
			t.Errorf("%s does not lead to a route registration; a closure's routes must count for the function that writes it", key)
		}
	}

	for _, key := range []string{
		app + ".loadSettings",
		"gopkg.in/ini.v1.Section.Key",
	} {
		if reach[key] {
			t.Errorf("%s was marked as leading to a route; nothing below it registers one", key)
		}
	}
}

// TestRouteReachSetIsInertWithoutAPredicate covers the callers that supply none.
func TestRouteReachSetIsInertWithoutAPredicate(t *testing.T) {
	m := closureMeta(t)
	if got := routeReachSet(m, nil); got != nil {
		t.Errorf("routeReachSet with no predicate = %v, want nil", got)
	}
	if got := routeReachSet(nil, registersRoute(m)); got != nil {
		t.Errorf("routeReachSet with no metadata = %v, want nil", got)
	}
}

// TestOrderTowardRoutesDefersOnlyAndStably pins the two properties that make
// ordering safe where pruning was not: nothing is removed, and equal-class
// children keep their relative order so the output stays deterministic (golden
// rule #1).
func TestOrderTowardRoutesDefersOnlyAndStably(t *testing.T) {
	m := closureMeta(t)
	tree := &LazyTree{meta: m, routeMatch: registersRoute(m)}

	const app = "example.com/app"
	plan := []childSpec{
		{key: "arg-child"}, // an argument child, ahead of the callee segment
		{key: app + ".loadSettings"},
		{key: app + ".registerAPI"},
		{key: "gopkg.in/ini.v1.Section.Key"},
		{key: app + ".main"},
	}
	before := append([]childSpec(nil), plan...)

	tree.orderTowardRoutes(plan, 1)

	if plan[0].key != "arg-child" {
		t.Errorf("argument child moved to %q; a group closure is an argument and must stay ahead of the callee segment", plan[0].key)
	}
	wantOrder := []string{
		"arg-child",
		app + ".registerAPI", // leads to a route, kept in its original relative order
		app + ".main",
		app + ".loadSettings", // does not, deferred
		"gopkg.in/ini.v1.Section.Key",
	}
	var gotOrder []string
	for _, spec := range plan {
		gotOrder = append(gotOrder, spec.key)
	}
	if !reflect.DeepEqual(gotOrder, wantOrder) {
		t.Errorf("order = %v, want %v", gotOrder, wantOrder)
	}

	// Nothing dropped: ordering defers, it never prunes. This is the property
	// that makes an imperfect reach set safe — a subtree it misses is expanded
	// late rather than not at all.
	if len(plan) != len(before) {
		t.Fatalf("plan went from %d children to %d; ordering must not drop any", len(before), len(plan))
	}
	seen := map[string]int{}
	for _, spec := range plan {
		seen[spec.key]++
	}
	for _, spec := range before {
		if seen[spec.key] != 1 {
			t.Errorf("child %q appears %d times after ordering, want exactly once", spec.key, seen[spec.key])
		}
	}
}

// TestOrderTowardRoutesLeavesUniformPlansAlone keeps the common case free: when
// every child is in the same class there is nothing to reorder, and the slice
// must not be touched.
func TestOrderTowardRoutesLeavesUniformPlansAlone(t *testing.T) {
	m := closureMeta(t)
	const app = "example.com/app"

	t.Run("all lead to routes", func(t *testing.T) {
		tree := &LazyTree{meta: m, routeMatch: registersRoute(m)}
		plan := []childSpec{{key: app + ".registerAPI"}, {key: app + ".main"}}
		tree.orderTowardRoutes(plan, 0)
		if plan[0].key != app+".registerAPI" || plan[1].key != app+".main" {
			t.Errorf("uniform plan was reordered: %v", plan)
		}
	})

	t.Run("no predicate leaves the plan alone", func(t *testing.T) {
		tree := &LazyTree{meta: m}
		plan := []childSpec{{key: app + ".loadSettings"}, {key: app + ".registerAPI"}}
		tree.orderTowardRoutes(plan, 0)
		if plan[0].key != app+".loadSettings" {
			t.Errorf("a tree with no route predicate reordered its plan: %v", plan)
		}
	})
}
