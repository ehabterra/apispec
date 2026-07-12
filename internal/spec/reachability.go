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

// Reachability summaries — docs/TRACKER_REDESIGN.md step 3.
//
// This file replaces the per-query, depth-bounded call-graph walks
// (previously capped by maxWrapperLookThroughDepth = 6) with function
// summaries:
//
//   - reachSet computes, in ONE bottom-up pass over the SCC condensation
//     (roadmap step 1), the set of ALL functions that transitively reach a
//     call matching a predicate. No depth limit; recursion is handled by the
//     condensation itself (a recursion cluster is a single unit).
//   - middlewareMatchesThrough resolves wrapper middleware to the
//     mapping-matching refs it transitively calls, memoized per function.
//     This one follows closure-internal edges (parentFnIndex) in addition to
//     direct calls, which the call-graph SCC does not order, so it memoizes
//     a demand-driven walk instead of using the bottom-up pass.

import (
	"github.com/ehabterra/apispec/internal/metadata"
)

// callGraphSCC lazily builds (and caches) the SCC condensation of the
// metadata call graph.
func (e *Extractor) callGraphSCC(meta *metadata.Metadata) *metadata.CallGraphSCC {
	if e.scc == nil {
		e.scc = metadata.BuildCallGraphSCC(meta)
	}
	return e.scc
}

// reachSet returns the set of function BaseIDs from which a call edge
// matching match is transitively reachable. Results are cached under
// cacheKey (derive it from whatever parameterizes match).
//
// Components are processed callees-first, so by the time a component is
// evaluated every cross-component callee is final; a match anywhere inside a
// recursion cluster marks the whole cluster.
func (e *Extractor) reachSet(meta *metadata.Metadata, cacheKey string, match func(*metadata.CallGraphEdge) bool) map[string]bool {
	if cached, ok := e.reachSets[cacheKey]; ok {
		return cached
	}

	scc := e.callGraphSCC(meta)
	compReaches := make([]bool, len(scc.Components))
	for c, comp := range scc.Components {
		reached := false
		for _, member := range comp {
			for _, edge := range meta.Callers[member] {
				if match(edge) {
					reached = true
					break
				}
				calleeComp, ok := scc.ComponentOf[edge.Callee.BaseID()]
				if ok && calleeComp != c && compReaches[calleeComp] {
					reached = true
					break
				}
			}
			if reached {
				break
			}
		}
		compReaches[c] = reached
	}

	set := make(map[string]bool)
	for id, c := range scc.ComponentOf {
		if compReaches[c] {
			set[id] = true
		}
	}
	if e.reachSets == nil {
		e.reachSets = make(map[string]map[string]bool)
	}
	e.reachSets[cacheKey] = set
	return set
}

// middlewareMatchesThrough returns the mapping-matching middleware refs
// transitively reachable from the function identified by key — through calls
// in its body and calls inside func literals it defines (parentFnIndex).
// Resolution stops at the first match on each path (a matching callee is
// reported, not descended into), mirroring the semantics the bounded walk
// had. Returns nil when nothing downstream matches.
//
// Memoized per function. Cycles: a function re-entered on the current
// resolution path contributes nothing (same as the old visited-set
// behavior); its own memo entry is computed from its remaining edges.
func (e *Extractor) middlewareMatchesThrough(key string, meta *metadata.Metadata) []MiddlewareRef {
	if cached, ok := e.mwResolved[key]; ok {
		return cached
	}
	if e.mwOnStack[key] {
		return nil
	}
	if e.mwOnStack == nil {
		e.mwOnStack = make(map[string]bool)
	}
	e.mwOnStack[key] = true
	defer delete(e.mwOnStack, key)

	var found []MiddlewareRef
	scan := func(edges []*metadata.CallGraphEdge) {
		for _, edge := range edges {
			callee := e.calleeMiddlewareRef(edge)
			if callee.empty() {
				continue
			}
			if anyMappingMatches(callee, e.cfg.SecurityMappings) {
				found = append(found, callee)
				continue
			}
			calleeKey := middlewareBaseID(callee)
			if calleeKey == "" {
				continue
			}
			found = append(found, e.middlewareMatchesThrough(calleeKey, meta)...)
		}
	}
	scan(meta.Callers[key])
	scan(e.parentFnIndex[key])

	found = dedupMiddlewareRefs(found)
	if e.mwResolved == nil {
		e.mwResolved = make(map[string][]MiddlewareRef)
	}
	e.mwResolved[key] = found
	return found
}
