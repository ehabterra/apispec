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

import "github.com/ehabterra/apispec/internal/metadata"

// Barren-subtree pruning (issue #318).
//
// The lazy tree materializes one node per PATH, so a callee reached along many
// paths is rebuilt once per path: measured on a 163-route service, 12,882
// distinct callee keys unfold into 7,147,505 nodes. Of those, 6,930,424 (97.6%)
// sit in subtrees that match no pattern at all — the walk builds 7.1M nodes to
// consult 104K.
//
// Those subtrees can be skipped outright, and soundly rather than heuristically,
// because of a property the extractor already documents and relies on for its
// per-edge matcher memos: every matcher family's MatchNode is a pure function of
// the CALL EDGE, and only extraction depends on a node's ancestry. So "does this
// subtree contain anything a matcher would accept" is a property of content
// identity, not of path — which is exactly what a per-path unfolding cannot
// exploit on its own.
//
// The answer is therefore computed once over the PLAN graph (content identities,
// of which there are thousands) instead of over the unfolded tree (millions).
// A node that can reach a match is still built, so provenance resolution walking
// UP from a matching node is unaffected: nothing it can reach is ever pruned.

// specIdentity returns the plan-memo key the node built from this spec would
// have. It must agree with planFor's key exactly, or the reachability answer
// would be recorded against an identity the expansion never asks about.
func specIdentity(spec childSpec) planKey {
	if spec.arg != nil {
		return planKey{key: spec.key, edge: spec.argEdge, arg: spec.arg, isArg: true}
	}
	return planKey{key: spec.key, edge: spec.edge, isArg: false}
}

// specEdge is the edge the node built from this spec would carry — the argument's
// own edge for an argument child, the callee edge otherwise, mirroring the
// assignments GetChildren makes.
func specEdge(spec childSpec) *metadata.CallGraphEdge {
	if spec.arg != nil {
		return spec.argEdge
	}
	return spec.edge
}

// specNode builds the throwaway node an identity's plan is computed from. It has
// no parent, which is safe by buildPlan's own contract: "Nothing here may depend
// on the node's parent — per-path concerns live in GetChildren."
func (t *LazyTree) specNode(spec childSpec) *LazyNode {
	n := &LazyNode{tree: t, key: spec.key}
	if spec.arg != nil {
		n.edge, n.arg, n.argType, n.isArgument = spec.argEdge, spec.arg, spec.argType, true
		return n
	}
	n.edge = spec.edge
	return n
}

// canReachMatch reports whether the subtree of the node this spec would create
// can contain any node a matcher accepts. False means the subtree is provably
// empty of anything the spec generation reads, so it need not be built.
//
// Answers true when pruning is not in force, so the guard is inert unless a
// predicate has been supplied.
func (t *LazyTree) canReachMatch(spec childSpec) bool {
	if t.edgeMatches == nil {
		return true
	}
	if !t.reachBuilt {
		t.buildReachIndex()
	}
	return t.reach[specIdentity(spec)]
}

// buildReachIndex computes, for every content identity reachable from the roots,
// whether its subtree can reach a matcher-accepted edge.
//
// Two passes, both linear:
//
//  1. Enumerate the identities and record REVERSE edges (child -> parents),
//     seeding the identities whose own edge matches.
//  2. Propagate backwards from the seeds. Reachability is a least fixpoint over
//     an OR, and running it backwards from the matches reaches exactly the
//     identities with a path to one — with no special handling for cycles, which
//     a forward DFS with memoization would get wrong (a node whose only route to
//     a match runs through a back edge would cache a premature false).
func (t *LazyTree) buildReachIndex() {
	if t.reachBuilt || t.edgeMatches == nil {
		return
	}
	t.reachBuilt = true
	t.buildRelations()

	// Roots are identities too: one that cannot reach a match is not worth
	// expanding either.
	specOf := map[planKey]childSpec{}
	roots := make([]planKey, 0, len(t.roots))
	for _, r := range t.roots {
		root, ok := r.(*LazyNode)
		if !ok {
			continue
		}
		spec := childSpec{key: root.key, edge: root.edge, arg: root.arg, argType: root.argType}
		if root.isArgument {
			spec.argEdge = root.edge
		}
		id := specIdentity(spec)
		specOf[id] = spec
		roots = append(roots, id)
	}

	childrenOf := func(id planKey) []planKey {
		spec, ok := specOf[id]
		if !ok {
			return nil
		}
		// planFor memoizes, so the plans built here are the same objects the
		// expansion will use — this warms the memo rather than duplicating it.
		plan := t.planFor(t.specNode(spec))
		out := make([]planKey, 0, len(plan))
		for _, child := range plan {
			childID := specIdentity(child)
			if _, seen := specOf[childID]; !seen {
				specOf[childID] = child
			}
			out = append(out, childID)
		}
		return out
	}
	matches := func(id planKey) bool {
		spec, ok := specOf[id]
		return ok && t.edgeMatches(specEdge(spec))
	}

	t.reach = computeReach(roots, childrenOf, matches)
}

// computeReach returns the identities with a path to one that matches.
//
// Enumerate forwards recording REVERSE edges, then propagate backwards from the
// matches. Backwards is what makes cycles a non-issue: a forward DFS with
// memoization caches a premature false for a node whose only route to a match
// runs through a back edge that was still in progress (X -> Y -> X with X -> M
// wrongly answers false for Y), so it would need SCC condensation or repeated
// passes to be correct. Propagating from the matches needs neither.
func computeReach(roots []planKey, childrenOf func(planKey) []planKey, matches func(planKey) bool) map[planKey]bool {
	reach := map[planKey]bool{}
	rev := map[planKey][]planKey{}
	seen := map[planKey]bool{}
	var seeds, frontier []planKey

	visit := func(id planKey) {
		if seen[id] {
			return
		}
		seen[id] = true
		frontier = append(frontier, id)
		if matches(id) && !reach[id] {
			reach[id] = true
			seeds = append(seeds, id)
		}
	}
	for _, id := range roots {
		visit(id)
	}
	for len(frontier) > 0 {
		id := frontier[len(frontier)-1]
		frontier = frontier[:len(frontier)-1]
		for _, child := range childrenOf(id) {
			rev[child] = append(rev[child], id)
			visit(child)
		}
	}
	for len(seeds) > 0 {
		id := seeds[len(seeds)-1]
		seeds = seeds[:len(seeds)-1]
		for _, parent := range rev[id] {
			if !reach[parent] {
				reach[parent] = true
				seeds = append(seeds, parent)
			}
		}
	}
	return reach
}

// edgeOnlyNode presents a bare call edge as a TrackerNodeInterface, so a
// matcher family can be evaluated for an edge before any node exists for it.
//
// Sound because of the property this whole mechanism rests on, stated at the
// extractor's per-edge matcher memos: every MatchNode implementation reads only
// GetEdge(). The remaining methods answer as an unparented, childless node, so a
// matcher that started consulting ancestry would see "no ancestry" and — since
// every such check is a further condition to satisfy — match no MORE than the
// real node would. The prune stays an over-approximation either way.
type edgeOnlyNode struct{ edge *metadata.CallGraphEdge }

func (n edgeOnlyNode) GetKey() string                      { return "" }
func (n edgeOnlyNode) GetParent() TrackerNodeInterface     { return nil }
func (n edgeOnlyNode) GetChildren() []TrackerNodeInterface { return nil }
func (n edgeOnlyNode) GetEdge() *metadata.CallGraphEdge    { return n.edge }
func (n edgeOnlyNode) GetArgument() *metadata.CallArgument { return nil }
func (n edgeOnlyNode) GetTypeParamMap() map[string]string  { return nil }

// edgeMatchesAnyFamily reports whether any configured matcher accepts the edge.
// It is the base case of the barren-subtree prune: an edge no family accepts
// contributes nothing to the spec, and a subtree of only such edges contributes
// nothing at all.
//
// EVERY family that the extraction walk consults has to be represented here. A
// family left out would make the prune drop subtrees that family would have
// matched — routes going quietly missing, which is the failure mode this guards.
func (e *Extractor) edgeMatchesAnyFamily(edge *metadata.CallGraphEdge) bool {
	if edge == nil {
		return false
	}
	node := edgeOnlyNode{edge: edge}
	for _, m := range e.routeMatchers {
		if m.MatchNode(node) {
			return true
		}
	}
	for _, m := range e.mountMatchers {
		if m.MatchNode(node) {
			return true
		}
	}
	for _, m := range e.securityMatchers {
		if m.MatchNode(node) {
			return true
		}
	}
	for _, m := range e.requestMatchers {
		if m.MatchNode(node) {
			return true
		}
	}
	for _, m := range e.responseMatchers {
		if m.MatchNode(node) {
			return true
		}
	}
	for _, m := range e.paramMatchers {
		if m.MatchNode(node) {
			return true
		}
	}
	return false
}

// enableBarrenPruning hands the tree the matcher-derived predicate, memoized per
// edge because the reachability build asks once per identity and edges repeat
// across them. A tree that does not support pruning ignores this.
func (e *Extractor) enableBarrenPruning() {
	lazy, ok := e.tree.(*LazyTree)
	if !ok {
		return
	}
	memo := map[*metadata.CallGraphEdge]bool{}
	lazy.SetEdgeMatcher(func(edge *metadata.CallGraphEdge) bool {
		if v, ok := memo[edge]; ok {
			return v
		}
		v := e.edgeMatchesAnyFamily(edge)
		memo[edge] = v
		return v
	})
}
