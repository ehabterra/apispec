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
	"strings"
	"testing"

	"github.com/ehabterra/apispec/internal/metadata"
)

// TestInstanceBudget covers the configurable copy cap. It is configurable because
// the default is a compromise: the scope is a group closure rather than a route,
// so a group holding more routes than the budget starves every later route of its
// response body (issue #224). Until that scoping is fixed, raising the number is
// the only way to document such a project — which a constant does not allow.
func TestInstanceBudget(t *testing.T) {
	unset := &LazyTree{}
	if got := unset.instanceBudget(); got != DefaultMaxInstancesPerKey {
		t.Errorf("unset budget = %d, want the default %d", got, DefaultMaxInstancesPerKey)
	}

	configured := &LazyTree{limits: metadata.TrackerLimits{MaxInstancesPerKey: 200}}
	if got := configured.instanceBudget(); got != 200 {
		t.Errorf("configured budget = %d, want 200", got)
	}

	// A negative is not a budget; it means the same as unset rather than "no
	// cap", so a typo cannot turn the walk loose.
	negative := &LazyTree{limits: metadata.TrackerLimits{MaxInstancesPerKey: -1}}
	if got := negative.instanceBudget(); got != DefaultMaxInstancesPerKey {
		t.Errorf("negative budget = %d, want the default", got)
	}
}

// sharedHelperGraph builds the shape the cap exists for: `main` calls
// `step1..stepN`, each of those calls `helper`, and `helper` calls `shared` at ONE
// call site.
//
// That single site is what gets copied — once per path that reaches it — so all N
// copies share a key and count against one budget. Copies of a key, not distinct
// call sites, is what the cap bounds: N separate calls to `shared` would be N
// different keys and would never touch it.
func sharedHelperGraph(callers int) *metadata.Metadata {
	pool := metadata.NewStringPool()
	meta := &metadata.Metadata{StringPool: pool}
	pkg := pool.Get("example")

	// Pooled fields use -1 for "unset"; 0 is a valid index, so the zero value
	// would resolve to a real string.
	call := func(name, position string) metadata.Call {
		return metadata.Call{
			Meta: meta, Name: pool.Get(name), Pkg: pkg, Position: pool.Get(position),
			RecvType: -1, Scope: -1, SignatureStr: -1,
		}
	}

	// One position for main across every edge: a root is minted per distinct
	// caller instance, and several roots would each walk the whole graph again.
	mainCall := call(metadata.MainFunc, "main.go:1:1")
	for i := 1; i <= callers; i++ {
		step := fmt.Sprintf("step%d", i)
		meta.CallGraph = append(meta.CallGraph,
			metadata.CallGraphEdge{Caller: mainCall, Callee: call(step, fmt.Sprintf("main.go:%d:2", i))},
			metadata.CallGraphEdge{Caller: call(step, fmt.Sprintf("step.go:%d:1", i)), Callee: call("helper", fmt.Sprintf("step.go:%d:2", i))},
		)
	}
	// The one site every path converges on.
	meta.CallGraph = append(meta.CallGraph,
		metadata.CallGraphEdge{Caller: call("helper", "helper.go:1:1"), Callee: call("shared", "helper.go:2:2")},
	)
	meta.BuildCallGraphMaps()
	return meta
}

// countKey walks the whole tree and counts nodes whose key names fn.
func countKey(nodes []TrackerNodeInterface, fn string) int {
	n := 0
	for _, node := range nodes {
		if strings.Contains(node.GetKey(), "."+fn) {
			n++
		}
		n += countKey(node.GetChildren(), fn)
	}
	return n
}

// TestInstanceBudgetCapsTraversal is the behavioural half: the configured number
// is what the walk actually admits.
//
// Six functions call one shared helper. Copies of that helper are what the cap
// counts, so a budget of three has to yield three copies and a budget above the
// demand has to yield all six — the difference between a project whose later
// routes keep their response bodies and one where they silently vanish (#224).
func TestInstanceBudgetCapsTraversal(t *testing.T) {
	const callers = 6

	tests := []struct {
		name   string
		budget int
		want   int
	}{
		{"a budget below the demand admits exactly the budget", 3, 3},
		{"a budget above the demand admits every copy", 50, callers},
		{"unset uses the default, which covers this demand", 0, callers},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tree := NewLazyTree(sharedHelperGraph(callers), metadata.TrackerLimits{
				MaxNodesPerTree:    10000,
				MaxChildrenPerNode: 100,
				MaxInstancesPerKey: tc.budget,
			})
			if got := countKey(tree.GetRoots(), "shared"); got != tc.want {
				t.Errorf("budget %d admitted %d copies of the shared helper, want %d", tc.budget, got, tc.want)
			}
		})
	}
}

// TestNewNodeSlabHandsOutStablePointers pins the property the slab must keep: a
// node's address never moves, and no later carve writes over a node already
// handed out. Nodes reference their parent, so a slab that grew in place
// (append) would copy the backing array and leave every pointer already handed
// out pointing at a stale copy.
func TestNewNodeSlabHandsOutStablePointers(t *testing.T) {
	tree := &LazyTree{}

	// Cross a chunk boundary: the failure only appears when a second slab is
	// carved.
	nodes := make([]*LazyNode, 0, nodeSlabChunk+16)
	for i := 0; i < nodeSlabChunk+16; i++ {
		node := tree.newNode()
		// A DISTINCT value per node, never the same one twice: identical values
		// would read back correctly even from memory a later carve had reused,
		// so the check would pass on an allocator that hands out overlapping
		// slots. Offset by one so no node carries the zero a fresh slot has.
		node.key = int32(i) + 1 // +1 so none is the zero value a fresh slot carries
		nodes = append(nodes, node)
	}

	seen := make(map[*LazyNode]bool, len(nodes))
	for i, node := range nodes {
		if seen[node] {
			t.Fatalf("node %d reuses an address already handed out", i)
		}
		seen[node] = true
		if node.key != int32(i)+1 {
			t.Fatalf("node %d was written over after being handed out: key=%d, want %d",
				i, node.key, int32(i)+1)
		}
	}

	// The slab the tree is carving from must not contain a node it already
	// handed out: newNode advances past each slot, so the next free slot is
	// beyond every live node. (A "compare nodes[0] with tree.nodeSlab[0]" check
	// would assert the opposite and fail on correct code — after any hand-out
	// nodeSlab[0] is the NEXT slot, never the first one returned.)
	if len(tree.nodeSlab) > 0 {
		next := &tree.nodeSlab[0]
		if seen[next] {
			t.Error("the next free slot is a node already handed out")
		}
	}

	// A freshly carved node must be zero, not whatever the previous tenant left.
	fresh := tree.newNode()
	if fresh.key != 0 || fresh.parent != nil || fresh.edge != nil || fresh.expanded {
		t.Errorf("a new node is not zeroed: %+v", *fresh)
	}
}

// TestBudgetCountsKeysWhileStatsReportWork pins the distinction issue #247 is
// about: MaxNodesPerTree bounds distinct callee KEYS, while the tree materialises
// a node per path a call is reached along. Reporting only the budgeted number
// made a run look an order of magnitude cheaper than it was — a 246-route service
// builds 208,394 nodes across 20,198 keys.
//
// The budget deliberately stays on keys. Switching it to nodes is truthful and
// starves route discovery: a global budget is spent depth-first on whatever
// expands first, and gitea documents 900 paths at a raised key budget against 1
// path at a node budget of fifteen million. The unit is the real problem (#224).
func TestBudgetCountsKeysWhileStatsReportWork(t *testing.T) {
	// A diamond: several callers reach one shared call site, so that site is
	// materialised once per path while remaining a single key.
	meta := sharedHelperGraph(6)
	tree := NewLazyTree(meta, metadata.TrackerLimits{MaxNodesPerTree: 100000, MaxChildrenPerNode: 100})

	var walk func(n TrackerNodeInterface, depth int)
	walk = func(n TrackerNodeInterface, depth int) {
		if n == nil || depth > 12 {
			return
		}
		for _, child := range n.GetChildren() {
			walk(child, depth+1)
		}
	}
	for _, root := range tree.GetRoots() {
		walk(root, 0)
	}

	stats := tree.ExpansionStats()
	if stats.NodesBuilt == 0 {
		t.Fatal("no keys expanded; the fixture builds no tree")
	}
	if stats.NodesMaterialized < stats.NodesBuilt {
		t.Errorf("materialized %d nodes across %d keys — a node per key is the floor, not the ceiling",
			stats.NodesMaterialized, stats.NodesBuilt)
	}
	// A diamond reaches the same callee along several paths, which is precisely
	// the difference the two counters exist to show.
	if stats.NodesMaterialized == stats.NodesBuilt {
		t.Errorf("materialized == built (%d): the diamond in this fixture must produce more nodes than keys, "+
			"or the counters are measuring the same thing and the distinction is lost", stats.NodesBuilt)
	}
}

// TestInstanceTruncationIsRecordedAndNamed pins the reporting contract for the
// instance cap: it counts every copy it refuses, remembers the FIRST one, and
// warns once rather than per drop.
//
// The count alone is not actionable — a bounded diamond inside one handler and a
// route starved by a scope it shares with other routes produce the same number.
// The scope is what separates them, so it is recorded, and the empty scope (the
// router wiring, above any argument node) is rendered rather than left blank
// where it would read as missing data (issue #224).
func TestInstanceTruncationIsRecordedAndNamed(t *testing.T) {
	tree := &LazyTree{limits: metadata.TrackerLimits{MaxInstancesPerKey: 3}}

	tree.noteInstanceTruncation("pkg.handler", "pkg.helper@a.go:1:1")
	tree.noteInstanceTruncation("pkg.other", "pkg.helper@a.go:2:1")

	if tree.instanceTruncations != 2 {
		t.Errorf("counted %d truncations, want 2", tree.instanceTruncations)
	}
	if tree.instanceFirstScope != "pkg.handler" || tree.instanceFirstKey != "pkg.helper@a.go:1:1" {
		t.Errorf("first truncation recorded as (%q, %q), want the FIRST one",
			tree.instanceFirstScope, tree.instanceFirstKey)
	}
	if !tree.instanceWarned {
		t.Error("the cap fired without warning; a silent truncation is the defect this fixes")
	}

	// ExpansionStats' propagation of these is asserted end to end by
	// TestGroupClosureInstanceCapIsReported, which has a real tree; calling it
	// here would only exercise relation-building.
}

func TestScopeLabelRendersTheWiringLevel(t *testing.T) {
	if got := scopeLabel(""); got != "<router wiring>" {
		t.Errorf("scopeLabel(\"\") = %q — an empty scope is the wiring level, not missing data", got)
	}
	if got := scopeLabel("pkg.handler"); got != "pkg.handler" {
		t.Errorf("scopeLabel(%q) = %q, want it unchanged", "pkg.handler", got)
	}
}
