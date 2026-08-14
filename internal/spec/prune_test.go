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
	"slices"
	"testing"

	"github.com/ehabterra/apispec/internal/metadata"
)

// reachCase drives computeReach over a named graph, so the tests read as the
// graph shapes they are about rather than as planKey literals.
func reachCase(t *testing.T, graph map[string][]string, matching []string, roots ...string) map[string]bool {
	t.Helper()
	id := func(name string) planKey { return planKey{key: name} }
	rootIDs := make([]planKey, 0, len(roots))
	for _, r := range roots {
		rootIDs = append(rootIDs, id(r))
	}
	childrenOf := func(k planKey) []planKey {
		out := make([]planKey, 0, len(graph[k.key]))
		for _, c := range graph[k.key] {
			out = append(out, id(c))
		}
		return out
	}
	matches := func(k planKey) bool { return slices.Contains(matching, k.key) }

	got := computeReach(rootIDs, childrenOf, matches)
	byName := map[string]bool{}
	for k, v := range got {
		byName[k.key] = v
	}
	return byName
}

func assertReach(t *testing.T, got map[string]bool, want map[string]bool) {
	t.Helper()
	for name, expect := range want {
		if got[name] != expect {
			t.Errorf("reach[%s] = %v, want %v", name, got[name], expect)
		}
	}
}

// TestComputeReachMarksThePathToAMatch covers the base case the prune rests on:
// a node is kept when something below it matches, and dropped when nothing does.
func TestComputeReachMarksThePathToAMatch(t *testing.T) {
	// root -> a -> b -> match, and root -> dead -> alsoDead
	got := reachCase(t,
		map[string][]string{
			"root": {"a", "dead"},
			"a":    {"b"},
			"b":    {"match"},
			"dead": {"alsoDead"},
		},
		[]string{"match"},
		"root")

	assertReach(t, got, map[string]bool{
		"root": true, "a": true, "b": true, "match": true,
		"dead": false, "alsoDead": false,
	})
}

// TestComputeReachHandlesCycleToAMatch is the case a forward DFS with
// memoization gets WRONG, and the reason the fixpoint runs backwards from the
// matches instead.
//
// x -> y -> x, and x -> match. A forward DFS entering x marks it in progress,
// descends into y, sees x is in progress, treats that edge as contributing
// nothing, and caches y = false — before it ever reaches match through x's other
// child. y really can reach the match, via x.
func TestComputeReachHandlesCycleToAMatch(t *testing.T) {
	got := reachCase(t,
		map[string][]string{
			"x": {"y", "match"},
			"y": {"x"},
		},
		[]string{"match"},
		"x")

	assertReach(t, got, map[string]bool{"x": true, "match": true})
	if !got["y"] {
		t.Error("y reaches the match through x and must be kept; a forward DFS would cache a premature false here")
	}
}

// TestComputeReachDropsAMatchlessCycle is the same shape without a match: a
// cycle must not keep itself alive.
func TestComputeReachDropsAMatchlessCycle(t *testing.T) {
	got := reachCase(t,
		map[string][]string{
			"root": {"x"},
			"x":    {"y"},
			"y":    {"x"},
		},
		nil,
		"root")

	assertReach(t, got, map[string]bool{"root": false, "x": false, "y": false})
}

// TestComputeReachKeepsAMatchingRoot covers a root that matches on its own,
// with nothing below it.
func TestComputeReachKeepsAMatchingRoot(t *testing.T) {
	got := reachCase(t, map[string][]string{"root": nil}, []string{"root"}, "root")
	assertReach(t, got, map[string]bool{"root": true})
}

// TestComputeReachJoinsDiamond covers a node reached along two paths, only one
// of which leads to a match — the shared-helper shape the tree unfolds per path.
func TestComputeReachJoinsDiamond(t *testing.T) {
	got := reachCase(t,
		map[string][]string{
			"root":   {"left", "right"},
			"left":   {"shared"},
			"right":  {"shared"},
			"shared": {"match", "dead"},
		},
		[]string{"match"},
		"root")

	assertReach(t, got, map[string]bool{
		"root": true, "left": true, "right": true, "shared": true, "match": true,
		"dead": false,
	})
}

// TestSpecIdentityMatchesPlanKey pins the correspondence the whole mechanism
// depends on: the identity a reachability answer is recorded against must be the
// key planFor looks the plan up by, for both child shapes. If these drift, the
// prune silently answers about a different node than the one being expanded.
func TestSpecIdentityMatchesPlanKey(t *testing.T) {
	tree := &LazyTree{}

	// Distinct, non-nil edges throughout: an argument child carries argEdge and
	// a call child carries edge, so leaving either nil would let the test pass
	// even if specIdentity read the wrong one.
	calleeEdge := &metadata.CallGraphEdge{}
	argOwnEdge := &metadata.CallGraphEdge{}
	parentEdge := &metadata.CallGraphEdge{}

	callSpec := childSpec{key: "pkg.Callee", edge: calleeEdge}
	callNode := tree.specNode(callSpec)
	wantCall := planKey{key: callNode.key, edge: callNode.edge, arg: callNode.arg, isArg: callNode.isArgument}
	if got := specIdentity(callSpec); got != wantCall {
		t.Errorf("call child: specIdentity = %+v, planFor would use %+v", got, wantCall)
	}
	if callNode.edge != calleeEdge {
		t.Errorf("call child: node carries edge %p, want the spec's callee edge %p", callNode.edge, calleeEdge)
	}

	// An argument child carries its OWN edge, not the parent edge it holds for
	// context — the distinction GetChildren makes and the identity must follow.
	argSpec := childSpec{
		key:     "pkg.Arg",
		edge:    parentEdge,
		argEdge: argOwnEdge,
		arg:     &metadata.CallArgument{},
		argType: ArgTypeLiteral,
	}
	argNode := tree.specNode(argSpec)
	wantArg := planKey{key: argNode.key, edge: argNode.edge, arg: argNode.arg, isArg: argNode.isArgument}
	if got := specIdentity(argSpec); got != wantArg {
		t.Errorf("argument child: specIdentity = %+v, planFor would use %+v", got, wantArg)
	}
	if !argNode.isArgument {
		t.Error("a spec carrying an argument must build an argument node")
	}
	if argNode.edge != argOwnEdge {
		t.Errorf("argument child: node carries edge %p, want the argument's own edge %p (not the parent edge %p)",
			argNode.edge, argOwnEdge, parentEdge)
	}
	if got := specEdge(argSpec); got != argOwnEdge {
		t.Errorf("argument child: specEdge = %p, want the argument's own edge %p — the prune would test the wrong edge for a match",
			got, argOwnEdge)
	}
	if got := specEdge(callSpec); got != calleeEdge {
		t.Errorf("call child: specEdge = %p, want the callee edge %p", got, calleeEdge)
	}
}

// TestEdgeOnlyNodeAnswersAsUnparented pins the half of the prune's soundness
// argument that lives in this type rather than in the fixpoint.
//
// edgeOnlyNode lets a matcher family be evaluated for an edge before any node
// exists for it. That is sound because every MatchNode implementation reads only
// GetEdge() — but the argument does not stop there: should one start consulting
// ancestry, it must see NOTHING rather than something borrowed, since every such
// check is a further condition to satisfy. Answering empty keeps the prune an
// over-approximation, which is the direction that can only keep subtrees the
// real node would have kept, never drop one it would have matched.
func TestEdgeOnlyNodeAnswersAsUnparented(t *testing.T) {
	edge := &metadata.CallGraphEdge{}
	node := edgeOnlyNode{edge: edge}

	if node.GetEdge() != edge {
		t.Errorf("GetEdge = %p, want the edge under test %p", node.GetEdge(), edge)
	}
	if got := node.GetKey(); got != "" {
		t.Errorf("GetKey = %q, want empty: an edge has no node key yet", got)
	}
	if got := node.GetParent(); got != nil {
		t.Errorf("GetParent = %v, want nil: a matcher consulting ancestry must see none, not a borrowed one", got)
	}
	if got := node.GetChildren(); got != nil {
		t.Errorf("GetChildren = %v, want nil", got)
	}
	if got := node.GetArgument(); got != nil {
		t.Errorf("GetArgument = %v, want nil: the edge form carries no argument", got)
	}
	if got := node.GetTypeParamMap(); got != nil {
		t.Errorf("GetTypeParamMap = %v, want nil", got)
	}

	// It has to actually satisfy the interface the matchers take, which is the
	// whole point of the type.
	var _ TrackerNodeInterface = node
}

// TestCanReachMatchIsInertWithoutAPredicate covers the off switch: no predicate
// installed means no pruning, which is how every consumer other than the
// extractor sees the tree (the diagram server builds its own and never installs
// one). A tree that pruned by default would silently change what they read.
func TestCanReachMatchIsInertWithoutAPredicate(t *testing.T) {
	tree := &LazyTree{}
	spec := childSpec{key: "pkg.Anything"}

	if !tree.canReachMatch(spec) {
		t.Error("with no edge matcher installed every child must be kept; pruning must be opt-in")
	}
	if tree.reachBuilt {
		t.Error("no index should be built when pruning is off")
	}

	// buildReachIndex is likewise a no-op, so a consumer that never installs a
	// predicate pays nothing for the machinery.
	tree.buildReachIndex()
	if tree.reach != nil || tree.reachBuilt {
		t.Errorf("buildReachIndex built an index with no predicate: reach=%v built=%v", tree.reach, tree.reachBuilt)
	}
}
