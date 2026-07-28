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
