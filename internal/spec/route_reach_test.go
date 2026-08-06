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
	"fmt"
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

// edgeTo returns the fixture edge whose callee is named name. Addressing
// closureMeta's edges by position instead would keep compiling — and often keep
// passing — if an edge were ever inserted ahead of the one a test means, while
// silently asserting nothing about a route registration.
func edgeTo(t *testing.T, m *metadata.Metadata, name string) *metadata.CallGraphEdge {
	t.Helper()
	for i := range m.CallGraph {
		if m.StringPool.GetString(m.CallGraph[i].Callee.Name) == name {
			return &m.CallGraph[i]
		}
	}
	t.Fatalf("fixture has no edge calling %q", name)
	return nil
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

// TestRouteBudgetIsPerRegistrationNotGlobal is the property that turns 12
// documented paths into 210: a route's detail is charged to that route, so a
// handler too deep to expand fully costs its own detail and nothing else's.
//
// With one global budget, truncation is TOTAL — expansion is depth-first, so the
// routes not yet reached when it runs out are lost outright rather than
// documented in less detail.
func TestRouteBudgetIsPerRegistrationNotGlobal(t *testing.T) {
	m := closureMeta(t)
	tree := &LazyTree{meta: m, routeMatch: registersRoute(m), terminalRouteMatch: registersRoute(m)}

	// A node that is not a registration stays in the wiring scope.
	wiring := &LazyNode{tree: tree, key: "example.com/app.main"}
	if got := tree.scopeOf(wiring); got != 0 {
		t.Errorf("a non-registration node opened scope %d, want the shared wiring scope 0", got)
	}

	// Each registration opens its OWN scope, so two routes never share a budget.
	regEdge := edgeTo(t, m, "Get")
	first := &LazyNode{tree: tree, key: "route-1", edge: regEdge}
	second := &LazyNode{tree: tree, key: "route-2", edge: regEdge}
	a, b := tree.scopeOf(first), tree.scopeOf(second)
	if a == 0 || b == 0 {
		t.Fatalf("a registration did not open its own scope: got %d and %d", a, b)
	}
	if a == b {
		t.Errorf("two registrations share scope %d; one route's expansion could then starve the other", a)
	}

	// Spending one route's allowance must not touch the other's, nor the wiring
	// walk's.
	tree.limits.MaxNodesPerRoute = 2
	tree.routeScopeNodes[a] = 2
	if !tree.routeBudgetExhausted(a) {
		t.Error("a route that spent its allowance was not reported as exhausted")
	}
	if tree.routeBudgetExhausted(b) {
		t.Error("one route's exhausted budget also exhausted another's — the budgets are not independent")
	}
	if tree.routeBudgetExhausted(0) {
		t.Error("the wiring scope must be bounded by MaxNodesPerTree, not by the per-route limit")
	}
}

// TestRouteTruncationNamesTheRoute keeps the report actionable: the count alone
// cannot tell you which endpoint is under-documented.
func TestRouteTruncationNamesTheRoute(t *testing.T) {
	m := closureMeta(t)
	tree := &LazyTree{meta: m, routeMatch: registersRoute(m), terminalRouteMatch: registersRoute(m)}
	regEdge := edgeTo(t, m, "Get")
	scope := tree.scopeOf(&LazyNode{tree: tree, key: "GET /things", edge: regEdge})

	tree.noteRouteTruncation(scope)

	if tree.routeTruncations != 1 {
		t.Errorf("counted %d route truncations, want 1", tree.routeTruncations)
	}
	if tree.routeFirstTruncated != "GET /things" {
		t.Errorf("first truncated route = %q, want the registration's key", tree.routeFirstTruncated)
	}
	if !tree.routeWarned {
		t.Error("a truncated route produced no warning")
	}
}

// TestGroupCallDoesNotOpenARouteScope is the regression guard for the worst bug
// this work produced, caught on a real 246-route chi service.
//
// The reach predicate deliberately matches mounts and groups too — "does this
// subtree lead to a route?" must say yes for `r.Route("/api", …)`, or the whole
// group is written off. Using that same predicate to open a BUDGET scope means
// every route inside the group shares one allowance, and the routes past the
// first few are never discovered at all: 246 paths became 32.
//
// So scoping uses a terminal-route predicate, and a group must not open a scope.
func TestGroupCallDoesNotOpenARouteScope(t *testing.T) {
	m := closureMeta(t)
	// routeMatch matches the group call as well (it is the reach predicate);
	// terminalRouteMatch matches only the verb call.
	groupOrRoute := func(edge *metadata.CallGraphEdge) bool {
		name := m.StringPool.GetString(edge.Callee.Name)
		return name == "Get" || name == "Route"
	}
	tree := &LazyTree{meta: m, routeMatch: groupOrRoute, terminalRouteMatch: registersRoute(m)}

	groupEdge := edgeTo(t, m, "Route") // a group
	if got := tree.scopeOf(&LazyNode{tree: tree, key: "group", edge: groupEdge}); got != 0 {
		t.Errorf("a group call opened budget scope %d; every route inside it would then share one allowance (#264)", got)
	}

	routeEdge := edgeTo(t, m, "Get") // one route
	if got := tree.scopeOf(&LazyNode{tree: tree, key: "route", edge: routeEdge}); got == 0 {
		t.Error("a terminal route registration did not open its own budget scope")
	}
}

// TestRouteTruncationCountsRoutesNotAttempts pins that the report counts routes.
// A truncated subtree is re-entered many times as the walk unwinds, which once
// read as "truncated 58 of 6 route subtrees".
func TestRouteTruncationCountsRoutesNotAttempts(t *testing.T) {
	m := closureMeta(t)
	tree := &LazyTree{meta: m, routeMatch: registersRoute(m), terminalRouteMatch: registersRoute(m)}
	scope := tree.scopeOf(&LazyNode{tree: tree, key: "GET /things", edge: edgeTo(t, m, "Get")})

	for i := 0; i < 20; i++ {
		tree.noteRouteTruncation(scope)
	}
	if tree.routeTruncations != 1 {
		t.Errorf("counted %d truncations for one route re-entered 20 times, want 1", tree.routeTruncations)
	}
}

// TestWiringBudgetIsNotChargedForRouteDetail pins that the two budgets of #264
// are actually two.
//
// Splitting the limit is only half the fix. The keys a route's own subtree
// discovers were still counted into the single global nodesBuilt that
// MaxNodesPerTree reads, so expanding one route in more detail spent the
// allowance the walk needed to FIND the next route. That inverts the whole
// point: measured on a ~900-route project, raising the per-route allowance made
// the spec smaller — 181 paths at 20,000, 163 at 200,000, 103 at 1,000,000 —
// which is "improving expansion makes the spec worse" all over again, one level
// down from where #264 found it.
//
// The invariant: only keys the WIRING walk reaches may exhaust MaxNodesPerTree.
func TestWiringBudgetIsNotChargedForRouteDetail(t *testing.T) {
	m := closureMeta(t)
	tree := &LazyTree{meta: m, routeMatch: registersRoute(m), terminalRouteMatch: registersRoute(m)}
	tree.limits.MaxNodesPerTree = 2

	// Two keys reached by the wiring walk, and two more reached only below a
	// route registration — the shape of any handler with a helper under it.
	tree.countKey("example.com/app.main", 0)
	tree.countKey("example.com/app.registerAPI", 0)
	tree.countKey("example.com/app.decodeBody", 7)
	tree.countKey("example.com/app.respond", 7)

	if tree.nodesBuilt != 4 {
		t.Errorf("nodesBuilt = %d, want all 4 distinct keys — it is the reported total", tree.nodesBuilt)
	}
	if tree.wiringNodesBuilt != 2 {
		t.Errorf("wiringNodesBuilt = %d, want only the 2 keys above a registration; "+
			"charging route detail to the wiring budget makes deeper expansion cost route DISCOVERY (#264)",
			tree.wiringNodesBuilt)
	}
	if !tree.budgetExhausted() {
		t.Error("the wiring budget of 2 was not reported as spent by 2 wiring keys")
	}

	// Route detail beyond the wiring budget must not push it further.
	for i := 0; i < 50; i++ {
		tree.countKey(fmt.Sprintf("example.com/app.deep%d", i), 7)
	}
	if tree.wiringNodesBuilt != 2 {
		t.Errorf("50 keys reached below a route moved the wiring count to %d; "+
			"a route's detail must never spend the walk's allowance", tree.wiringNodesBuilt)
	}
}

// TestWiringKeyCountedWhenReachedLater pins the ordering case: a key first seen
// inside a route subtree, later reached by the wiring walk, still counts for the
// wiring budget. Marking it "already seen" and skipping it would let a walk that
// happens to descend a route first understate its own cost without bound.
func TestWiringKeyCountedWhenReachedLater(t *testing.T) {
	m := closureMeta(t)
	tree := &LazyTree{meta: m}

	tree.countKey("example.com/app.shared", 3) // first seen under a route
	if tree.wiringNodesBuilt != 0 {
		t.Fatalf("wiringNodesBuilt = %d after a route-only key, want 0", tree.wiringNodesBuilt)
	}
	tree.countKey("example.com/app.shared", 0) // then reached by the wiring walk

	if tree.wiringNodesBuilt != 1 {
		t.Errorf("wiringNodesBuilt = %d, want 1 — a key the wiring walk reaches counts for it "+
			"whenever that happens, not only if it was seen there first", tree.wiringNodesBuilt)
	}
	if tree.nodesBuilt != 1 {
		t.Errorf("nodesBuilt = %d, want 1 — the same key twice is still one distinct key", tree.nodesBuilt)
	}
}
