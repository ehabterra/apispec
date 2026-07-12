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

package metadata

// Roadmap step 1 of docs/TRACKER_REDESIGN.md: SCC condensation and reverse
// topological order over the call graph.
//
// Collapsing every strongly-connected component (a recursion cluster: a set
// of functions all mutually reachable through calls) into one unit yields a
// condensation that is acyclic by construction. Downstream analyses can then
// process functions bottom-up — callees before callers, each exactly once —
// instead of guarding cyclic re-visits with depth limits.
//
// Nodes are function BaseIDs ("pkg.name" / "pkg.recv.name"), the same keys
// meta.Callers uses, so results join directly with the existing reachability
// walks. Construction is deterministic: node and edge orders are sorted
// before traversal, so identical metadata always yields identical components,
// numbering, and DAG.

import "sort"

// CallGraphSCC is the condensation of the call graph into strongly-connected
// components.
type CallGraphSCC struct {
	// Components lists each SCC's member BaseIDs (sorted). Components appear
	// in reverse topological order of the condensation: every component is
	// listed before any component that calls into it. Iterating Components in
	// index order therefore processes callees before callers ("bottom-up").
	Components [][]string

	// ComponentOf maps a function BaseID to its index in Components.
	ComponentOf map[string]int

	// DAG is the condensed call graph: DAG[c] lists the component indices
	// that component c calls into (deduped, sorted, no self edges). Acyclic
	// by construction.
	DAG [][]int

	// Recursive[c] reports whether component c contains recursion: more than
	// one member, or a single member that calls itself.
	Recursive []bool
}

// SameComponent reports whether two functions are mutually recursive (belong
// to the same SCC). Unknown IDs are never in any component.
func (s *CallGraphSCC) SameComponent(a, b string) bool {
	ca, ok := s.ComponentOf[a]
	if !ok {
		return false
	}
	cb, ok := s.ComponentOf[b]
	return ok && ca == cb
}

// InCycle reports whether the function participates in recursion (direct or
// mutual).
func (s *CallGraphSCC) InCycle(fn string) bool {
	c, ok := s.ComponentOf[fn]
	return ok && s.Recursive[c]
}

// BuildCallGraphSCC condenses m's call graph. It reads m.CallGraph directly
// and does not require BuildCallGraphMaps to have run.
func BuildCallGraphSCC(m *Metadata) *CallGraphSCC {
	nodeSet := make(map[string]struct{}, len(m.CallGraph))
	adjSet := make(map[string]map[string]struct{}, len(m.CallGraph))
	for i := range m.CallGraph {
		edge := &m.CallGraph[i]
		u := edge.Caller.BaseID()
		v := edge.Callee.BaseID()
		if u == "" || v == "" {
			continue
		}
		nodeSet[u] = struct{}{}
		nodeSet[v] = struct{}{}
		if adjSet[u] == nil {
			adjSet[u] = make(map[string]struct{})
		}
		adjSet[u][v] = struct{}{}
	}

	nodes := make([]string, 0, len(nodeSet))
	for n := range nodeSet {
		nodes = append(nodes, n)
	}
	sort.Strings(nodes)

	adj := make(map[string][]string, len(adjSet))
	for u, vs := range adjSet {
		out := make([]string, 0, len(vs))
		for v := range vs {
			out = append(out, v)
		}
		sort.Strings(out)
		adj[u] = out
	}

	return condenseSCC(nodes, adj)
}

// condenseSCC runs Tarjan's algorithm (iterative, so pathological call-chain
// depth cannot overflow the goroutine stack) over a pre-sorted node list and
// pre-sorted adjacency, then builds the condensed DAG. Tarjan finalizes a
// component only after every component reachable from it, so the emission
// order is already the reverse topological (callees-first) order Components
// documents.
func condenseSCC(nodes []string, adj map[string][]string) *CallGraphSCC {
	res := &CallGraphSCC{ComponentOf: make(map[string]int, len(nodes))}

	index := make(map[string]int, len(nodes))   // discovery index, 1-based; 0 = unvisited
	lowlink := make(map[string]int, len(nodes)) // smallest index reachable from the node
	onStack := make(map[string]bool, len(nodes))
	var tarjanStack []string

	type frame struct {
		node string
		next int // next adjacency entry to visit
	}
	var callStack []frame
	counter := 0

	for _, root := range nodes {
		if index[root] != 0 {
			continue
		}
		counter++
		index[root], lowlink[root] = counter, counter
		tarjanStack = append(tarjanStack, root)
		onStack[root] = true
		callStack = append(callStack, frame{node: root})

		for len(callStack) > 0 {
			fi := len(callStack) - 1
			node := callStack[fi].node
			if callStack[fi].next < len(adj[node]) {
				next := adj[node][callStack[fi].next]
				callStack[fi].next++
				if index[next] == 0 {
					counter++
					index[next], lowlink[next] = counter, counter
					tarjanStack = append(tarjanStack, next)
					onStack[next] = true
					callStack = append(callStack, frame{node: next})
				} else if onStack[next] && index[next] < lowlink[node] {
					lowlink[node] = index[next]
				}
				continue
			}

			// All successors visited: retire the frame.
			callStack = callStack[:fi]
			if fi > 0 {
				parent := callStack[fi-1].node
				if lowlink[node] < lowlink[parent] {
					lowlink[parent] = lowlink[node]
				}
			}
			if lowlink[node] != index[node] {
				continue
			}
			// node is the root of a component: pop its members.
			var comp []string
			for {
				top := tarjanStack[len(tarjanStack)-1]
				tarjanStack = tarjanStack[:len(tarjanStack)-1]
				onStack[top] = false
				comp = append(comp, top)
				if top == node {
					break
				}
			}
			sort.Strings(comp)
			for _, member := range comp {
				res.ComponentOf[member] = len(res.Components)
			}
			res.Components = append(res.Components, comp)
		}
	}

	// Condensed DAG + recursion flags.
	res.DAG = make([][]int, len(res.Components))
	res.Recursive = make([]bool, len(res.Components))
	dagSets := make([]map[int]struct{}, len(res.Components))
	for c, comp := range res.Components {
		res.Recursive[c] = len(comp) > 1
	}
	for _, u := range nodes {
		cu := res.ComponentOf[u]
		for _, v := range adj[u] {
			cv := res.ComponentOf[v]
			if cv == cu {
				if u == v {
					res.Recursive[cu] = true // self-loop
				}
				continue
			}
			if dagSets[cu] == nil {
				dagSets[cu] = make(map[int]struct{})
			}
			dagSets[cu][cv] = struct{}{}
		}
	}
	for c, set := range dagSets {
		if len(set) == 0 {
			continue
		}
		out := make([]int, 0, len(set))
		for v := range set {
			out = append(out, v)
		}
		sort.Ints(out)
		res.DAG[c] = out
	}

	return res
}
