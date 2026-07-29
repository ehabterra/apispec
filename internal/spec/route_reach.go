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

// routeReachSet returns the function base keys from which a route registration
// is transitively reachable — the set the tree uses to decide which subtree to
// spend its budget on first (issue #264).
//
// It is a backward walk from every registration site rather than the extractor's
// bottom-up pass over the SCC condensation, for one reason: it must follow
// FUNC LITERALS, and the call graph does not.
//
// A closure passed as an argument — `r.Route("/api", func(api chi.Router){ … })`,
// which is how most real projects group routes — is not a callee of the function
// that writes it, so no call edge leads there. Reachability that only follows
// call edges therefore reports that the enclosing function reaches no route,
// which is false for exactly the code that matters most. That blind spot is why
// an earlier attempt to PRUNE by pattern reachability deleted 124 of 312
// operations: `chi.Group` matches no route pattern and reaches none, yet every
// route lives inside its closure.
//
// So two relations propagate "leads to a registration" backwards:
//
//   - callee -> caller, the ordinary one;
//   - closure -> the function that lexically defines it, so a group closure's
//     routes count for the function that writes the group.
//
// The result is a HINT, never a gate. Ordering by it defers work; it never drops
// any, so a function this set misses is expanded late rather than not at all.
func routeReachSet(meta *metadata.Metadata, match func(*metadata.CallGraphEdge) bool) map[string]bool {
	if meta == nil || match == nil {
		return nil
	}

	// Predecessors: for a key, the functions whose own expansion leads to it.
	preds := make(map[string][]string, len(meta.CallGraph))
	addPred := func(of, pred string) {
		if of == "" || pred == "" || of == pred {
			return
		}
		preds[of] = append(preds[of], pred)
	}

	reaches := make(map[string]bool)
	var queue []string
	mark := func(key string) {
		if key == "" || reaches[key] {
			return
		}
		reaches[key] = true
		queue = append(queue, key)
	}

	for i := range meta.CallGraph {
		edge := &meta.CallGraph[i]
		caller := edge.Caller.BaseID()
		addPred(edge.Callee.BaseID(), caller)
		if edge.ParentFunction != nil {
			// The closure's body is written inside its parent, so reaching a
			// registration from the closure means the parent's subtree leads to
			// one — even though nothing CALLS the closure.
			addPred(caller, edge.ParentFunction.BaseID())
		}
		if match(edge) {
			mark(caller)
		}
	}

	for len(queue) > 0 {
		key := queue[0]
		queue = queue[1:]
		for _, pred := range preds[key] {
			mark(pred)
		}
	}
	return reaches
}
