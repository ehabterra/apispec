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

// LazyTree — docs/TRACKER_REDESIGN.md step 4 (groundwork).
//
// A second TrackerTreeInterface implementation that computes the call tree
// on demand instead of materializing the DAG's unfolding up front:
//
//   - a node is (edge/argument, parent); children are computed on first
//     access from meta.Callers plus the edge's arguments, and the *edge
//     list* per function key is memoized (the expensive part), while node
//     objects stay per-path so every node has exactly one true parent —
//     per-route isolation needs no shared-node discipline;
//   - cycles are cut by an ancestor-key check on the node's own path (the
//     per-path state the eager tree's global maps approximate);
//   - traversals visit each function key once globally, so they are linear
//     in the graph, not in its exponential unfolding.
//
// The eager tree's mutation overlays are represented here as query-time
// relations built once in buildRelations: chain order, receiver-variable
// and struct-field producer links (assignIndex, the eager assignmentKey
// composition), param bindings, interface-implementer fan-out, and
// handler-factory closure expansion. Parity is tracked by the fixture
// harness (TestLazyTreeParity) and the per-codebase meter
// (TestTreeParityDirs); the eager tree remains the production default
// until operation content — not just path sets — matches.

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/ehabterra/apispec/internal/metadata"
)

// LazyTree implements TrackerTreeInterface over metadata, expanding on demand.
type LazyTree struct {
	meta   *metadata.Metadata
	limits metadata.TrackerLimits
	roots  []TrackerNodeInterface

	// calleeEdges memoizes, per function base key, the filtered+ordered call
	// edges used to expand any node of that function. Computed once.
	calleeEdges map[string][]*metadata.CallGraphEdge

	// Relations the eager tree encodes by mutating node linkage, kept here as
	// plain indexes consulted during expansion (the step-5 direction):
	//
	// chainChildren: chained calls (r.HandleFunc(...).Methods("GET")) grouped
	// under the chain parent's callee ID — what processChainRelationships
	// wires by appending to Children.
	chainChildren map[string][]*metadata.CallGraphEdge
	// receiverChildren: calls made on a variable, grouped under the callee ID
	// of the call that produced the variable (g := app.Group("/x"); g.GET(...)
	// lists the g.GET edge under the Group call) — what the eager build wires
	// through assignmentIndex/variableNodes.
	receiverChildren map[string][]*metadata.CallGraphEdge
	// claimed marks edges owned by a receiverChildren producer. The eager
	// build's AddChildren pass detaches such nodes from their call-site
	// parent, so they appear only under the producer (a group's routes under
	// the Group call, not under main) — mirrored here by excluding them from
	// the plain caller expansion.
	claimed        map[*metadata.CallGraphEdge]bool
	relationsBuilt bool
	budgetWarned   bool

	// assignIndex mirrors the eager tree's assignmentIndex byte-for-byte: the
	// SAME assignmentKey composition (name, pkg, concrete type, container —
	// with the selector-Lhs container override) mapping to the producing
	// call's callee ID. Consumed at argument expansion with the same
	// TraceVariableOrigin-composed lookups the eager processArguments uses,
	// so variable and struct-field arguments resolve to their producers the
	// same way (functional options, builder wiring, plain var mounts).
	assignIndex map[assignmentKey]string
	// producerArgs: producer callee ID (an option/builder call like
	// WithCartRouter(x)) -> the producer IDs of its own arguments, so a
	// field lookup that lands on the option call can step through to the
	// value that was stored (CartAPIs(...) above).
	producerArgs map[string][]string

	// nodesBuilt counts every LazyNode created. The per-path cycle guard
	// bounds each path, but a dense cyclic graph still has exponentially many
	// distinct acyclic paths — the same blow-up MaxNodesPerTree exists to
	// stop in the eager tree. Once the budget is spent, expansion returns
	// leaves.
	nodesBuilt int

	// instanceCount counts node copies per (instance scope, callee ID) —
	// see maxInstancesPerKey. A node is (edge, parent), so a callee reached
	// along many paths gets many copies; business-layer diamonds make that
	// exponential and would drain the node budget before traversal reaches
	// later router wiring. Nested by scope to avoid a key concatenation per
	// child instantiation (visible in profiles).
	instanceCount map[string]map[string]int
	// argInstanceIDs holds the exact (position-qualified) IDs of every
	// top-level call argument in the graph. Used by edgesFor to skip a
	// callee edge only when THAT call site is already represented as an
	// argument node — meta.Args is keyed by position-stripped base ID, so
	// using it directly would let one `foo(q.Get("x"))` anywhere suppress
	// every `Values.Get` call site in the project.
	argInstanceIDs map[string]bool

	// plans memoizes each node content-identity's expansion plan — the
	// "(edgeID, relevant bindings)" memoization from the redesign doc §7:
	// bindings are embedded in instance keys, so binding-distinct instances
	// key distinct plans. Per-path work reduces to guards + allocation.
	plans map[planKey][]childSpec

	// genericTypes memoizes metadata.ExtractGenericTypes (regexp-backed),
	// which otherwise re-parses the same key for every node copy.
	genericTypes map[string][]string

	// traceCache memoizes metadata.TraceVariableOrigin, which dominates the
	// CPU profile when re-run for every per-path node copy of the same
	// argument (var, caller fn, caller pkg) -> (originVar, originPkg, originFunc).
	traceCache map[string][3]string

	// seenKeys backs the node budget: distinct callee IDs ever materialized,
	// the same graph-sized unit as the eager tree's shared-node cap —
	// deliberately NOT scoped, or many scopes would exhaust the budget with
	// copies of the same graph.
	seenKeys map[string]bool
}

// maxInstancesPerKey bounds node copies of the same callee WITHIN one
// instance scope (the subtree of the nearest argument-node ancestor —
// approximately "per handler"). Scoping matters: a response helper shared by
// every handler legitimately needs one copy per route for per-route value
// tracing, while call diamonds inside a single handler's business logic
// multiply copies combinatorially and must be cut — the role the eager
// tree's per-ID recursion cap plays.
const maxInstancesPerKey = 10

// budgetExhausted reports whether the cumulative node budget is spent.
func (t *LazyTree) budgetExhausted() bool {
	return t.limits.MaxNodesPerTree > 0 && t.nodesBuilt >= t.limits.MaxNodesPerTree
}

// genericTypesOf is a memoized metadata.ExtractGenericTypes.
func (t *LazyTree) genericTypesOf(key string) []string {
	if types, ok := t.genericTypes[key]; ok {
		return types
	}
	types := metadata.ExtractGenericTypes(key)
	if t.genericTypes == nil {
		t.genericTypes = map[string][]string{}
	}
	t.genericTypes[key] = types
	return types
}

// traceOrigin is a memoized metadata.TraceVariableOrigin: the same
// (variable, enclosing function) is traced once per tree instead of once per
// node copy.
func (t *LazyTree) traceOrigin(varName, callerName, callerPkg string) (string, string, string) {
	key := varName + "\x00" + callerName + "\x00" + callerPkg
	if r, ok := t.traceCache[key]; ok {
		return r[0], r[1], r[2]
	}
	originVar, originPkg, _, originFunc := metadata.TraceVariableOrigin(varName, callerName, callerPkg, t.meta)
	if t.traceCache == nil {
		t.traceCache = map[string][3]string{}
	}
	t.traceCache[key] = [3]string{originVar, originPkg, originFunc}
	return originVar, originPkg, originFunc
}

// buildRelations constructs the chain and receiver-variable indexes once.
func (t *LazyTree) buildRelations() {
	if t.relationsBuilt {
		return
	}
	t.relationsBuilt = true
	t.chainChildren = map[string][]*metadata.CallGraphEdge{}
	t.receiverChildren = map[string][]*metadata.CallGraphEdge{}
	t.claimed = map[*metadata.CallGraphEdge]bool{}
	t.argInstanceIDs = map[string]bool{}
	meta := t.meta

	for i := range meta.CallGraph {
		for _, arg := range meta.CallGraph[i].Args {
			if arg == nil {
				continue
			}
			if id := arg.ID(); id != "" {
				t.argInstanceIDs[strings.TrimPrefix(id, "*")] = true
			}
		}
	}

	// Edges grouped by the receiver variable they're invoked on. Keyed by
	// (varName, exact caller BaseID): the caller's full identity — package,
	// receiver type, name — so `q := r.URL.Query()` in ten same-named
	// methods (assetsHandler.list, catalogHandler.list, …) stays ten
	// separate groups. A bare-name key collides them, piling every group's
	// edges under one arbitrary producer and claiming them away from the
	// other nine.
	type recvKey struct{ name, pkg, fn string }
	edgesByRecvVar := map[string][]*metadata.CallGraphEdge{}
	recvEdgeKey := func(varName string, caller *metadata.Call) string {
		return varName + "\x00" + caller.BaseID()
	}
	for i := range meta.CallGraph {
		edge := &meta.CallGraph[i]
		if edge.ChainParent != nil {
			parentKey := strings.TrimPrefix(edge.ChainParent.Callee.ID(), "*")
			t.chainChildren[parentKey] = append(t.chainChildren[parentKey], edge)
		}
		if edge.CalleeVarName != "" {
			k := recvEdgeKey(edge.CalleeVarName, &edge.Caller)
			edgesByRecvVar[k] = append(edgesByRecvVar[k], edge)
		}
	}

	// Assignment links: variable <- producing call. Sort by producing callee
	// ID so the receiverChildren lists are order-independent of the source map.
	rels := make([]*metadata.AssignmentLink, 0)
	for _, rel := range meta.GetAssignmentRelationships() {
		rels = append(rels, rel)
	}
	sort.Slice(rels, func(i, j int) bool {
		return rels[i].Edge.Callee.ID() < rels[j].Edge.Callee.ID()
	})
	producerByVar := map[recvKey]string{}
	for _, rel := range rels {
		producerKey := strings.TrimPrefix(rel.Edge.Callee.ID(), "*")
		// Bare-name key: consumed by TraceVariableOrigin-driven lookups
		// (param bindings, option-arg step-through), which only have bare
		// function names.
		producerByVar[recvKey{
			name: getString(meta, rel.Assignment.VariableName),
			pkg:  getString(meta, rel.Assignment.Pkg),
			fn:   getString(meta, rel.Assignment.Func),
		}] = producerKey
		// Claiming uses the exact-caller key: the assignment's producing edge
		// carries the full identity of the function the assignment lives in.
		//
		// Guard against a cross-scope name collision: a callee's internal
		// reassignment of a same-named variable (the canonical middleware
		// `r = r.WithContext(ctx)`, where `r` is a *http.Request) is recorded
		// in the call edge's AssignmentMap, so the link's Edge.Caller is the
		// *caller's* scope (which may have its own `r`, e.g. a chi.Router
		// receiver) while the assignment actually lives in the callee. Claiming
		// then would steal the caller's receiver route registrations (r.Get,
		// r.Group) onto the callee's producer, where they are never re-emitted
		// — dropping those routes. The assignment must genuinely live in the
		// scope whose receiver calls we claim.
		if getString(meta, rel.Assignment.Func) != getString(meta, rel.Edge.Caller.Name) {
			continue
		}
		edges := edgesByRecvVar[recvEdgeKey(getString(meta, rel.Assignment.VariableName), &rel.Edge.Caller)]
		if len(edges) == 0 {
			continue
		}
		t.receiverChildren[producerKey] = append(t.receiverChildren[producerKey], edges...)
		for _, edge := range edges {
			t.claimed[edge] = true
		}
	}

	// assignIndex: the eager tree's assignmentIndex, byte-for-byte key
	// composition (NewTrackerTree lines building akey, including the
	// selector-Lhs container override). Last write wins over the same sorted
	// order the eager build uses, so ambiguous keys pick the same winner.
	t.assignIndex = map[assignmentKey]string{}
	t.producerArgs = map[string][]string{}
	for _, rel := range rels {
		akey := assignmentKey{
			Name:      getString(meta, rel.Assignment.VariableName),
			Pkg:       getString(meta, rel.Assignment.Pkg),
			Type:      getString(meta, rel.Assignment.ConcreteType),
			Container: getString(meta, rel.Assignment.Func),
		}
		if rel.Assignment.Lhs.GetKind() == metadata.KindSelector &&
			rel.Assignment.Lhs.X != nil && rel.Assignment.Lhs.X.Type != -1 {
			akey.Container = getString(meta, rel.Assignment.Lhs.X.Type)
		}
		producerID := strings.TrimPrefix(rel.Edge.Callee.ID(), "*")
		t.assignIndex[akey] = producerID

		// Step-through for option/builder producers: the values the producing
		// call was given (WithCartRouter(cartRest.CartAPIs(app)) stores
		// CartAPIs' result, not WithCartRouter's).
		edge := rel.Edge
		callerPkg := getString(meta, edge.Caller.Pkg)
		callerFn := getString(meta, edge.Caller.Name)
		for _, arg := range edge.Args {
			if arg == nil {
				continue
			}
			switch arg.GetKind() {
			case metadata.KindIdent:
				if p, ok := producerByVar[recvKey{name: arg.GetName(), pkg: callerPkg, fn: callerFn}]; ok {
					t.producerArgs[producerID] = append(t.producerArgs[producerID], p)
				}
			case metadata.KindCall:
				if arg.Edge != nil {
					t.producerArgs[producerID] = append(t.producerArgs[producerID], strings.TrimPrefix(arg.Edge.Callee.ID(), "*"))
				} else if arg.Fun != nil {
					fun := arg.Fun
					if fun.GetKind() == metadata.KindSelector && fun.Sel != nil {
						fun = fun.Sel
					}
					if name, fpkg := fun.GetName(), fun.GetPkg(); name != "" && fpkg != "" {
						t.producerArgs[producerID] = append(t.producerArgs[producerID], fpkg+"."+name)
					}
				}
			}
		}
	}

	// Param bindings: a value passed into a function parameter (UserRoutes(g)
	// with func UserRoutes(rg *gin.RouterGroup)) makes the callee's calls on
	// that parameter belong to the value's producer — so a group's routes
	// registered in a helper still hang (prefixed) under the Group call. This
	// is what the eager build wires through variableNodes/ParamArgMap.
	for i := range meta.CallGraph {
		edge := &meta.CallGraph[i]
		if len(edge.ParamArgMap) == 0 {
			continue
		}
		params := make([]string, 0, len(edge.ParamArgMap))
		for param := range edge.ParamArgMap {
			params = append(params, param)
		}
		sort.Strings(params)
		for _, param := range params {
			arg := edge.ParamArgMap[param]
			if arg.GetKind() != metadata.KindIdent {
				continue
			}
			// The callee's calls on this param have Caller == the callee, so
			// the exact-caller key is (param, callee BaseID).
			paramEdges := edgesByRecvVar[recvEdgeKey(param, &edge.Callee)]
			if len(paramEdges) == 0 {
				continue
			}
			originVar, originPkg, originFunc := t.traceOrigin(
				arg.GetName(),
				getString(meta, edge.Caller.Name),
				getString(meta, edge.Caller.Pkg),
			)
			producerKey, ok := producerByVar[recvKey{name: originVar, pkg: originPkg, fn: originFunc}]
			if !ok {
				continue
			}
			t.receiverChildren[producerKey] = append(t.receiverChildren[producerKey], paramEdges...)
			for _, pe := range paramEdges {
				t.claimed[pe] = true
			}
		}
	}
}

// NewLazyTree builds the root layer (main functions, like the eager tree)
// and nothing else.
func NewLazyTree(meta *metadata.Metadata, limits metadata.TrackerLimits) *LazyTree {
	t := &LazyTree{
		meta:        meta,
		limits:      limits,
		calleeEdges: make(map[string][]*metadata.CallGraphEdge),
	}
	seen := map[string]bool{}
	for _, edge := range meta.CallGraphRoots() {
		callerID := edge.Caller.ID()
		if getString(meta, edge.Caller.Name) != metadata.MainFunc || seen[callerID] {
			continue
		}
		seen[callerID] = true
		t.roots = append(t.roots, &LazyNode{tree: t, key: strings.TrimPrefix(callerID, "*")})
	}
	return t
}

// edgesFor returns (and memoizes) the expansion edge list for a function base
// key: callee edges from meta.Callers with the eager tree's skip rules
// (self-calls, "nil", callees already present as arguments).
func (t *LazyTree) edgesFor(baseKey string) []*metadata.CallGraphEdge {
	if edges, ok := t.calleeEdges[baseKey]; ok {
		return edges
	}
	t.buildRelations()
	var out []*metadata.CallGraphEdge
	for _, edge := range t.meta.Callers[baseKey] {
		if t.claimed[edge] {
			continue // owned by its producer (see receiverChildren)
		}
		calleeID := edge.Callee.ID()
		if calleeID == edge.Caller.ID() || getString(t.meta, edge.Callee.Name) == "nil" {
			continue
		}
		if t.argInstanceIDs[strings.TrimPrefix(calleeID, "*")] {
			continue // this exact call site is represented as an argument node
		}
		out = append(out, edge)
	}
	t.calleeEdges[baseKey] = out
	return out
}

// GetRoots implements TrackerTreeInterface.
func (t *LazyTree) GetRoots() []TrackerNodeInterface {
	if t == nil {
		return nil
	}
	return t.roots
}

// GetMetadata implements TrackerTreeInterface.
func (t *LazyTree) GetMetadata() *metadata.Metadata { return t.meta }

// LazyNode implements TrackerNodeInterface. Identity is (content, parent):
// node objects are per-path, so Parent is always the actual expansion parent.
type LazyNode struct {
	tree   *LazyTree
	key    string
	parent *LazyNode

	edge *metadata.CallGraphEdge
	arg  *metadata.CallArgument

	argType    ArgumentType
	isArgument bool

	typeParams map[string]string // GetTypeParamMap cache

	children []TrackerNodeInterface // nil = not yet expanded
	expanded bool
}

// GetKey implements TrackerNodeInterface.
func (n *LazyNode) GetKey() string { return n.key }

// GetParent implements TrackerNodeInterface.
func (n *LazyNode) GetParent() TrackerNodeInterface {
	if n.parent == nil {
		return nil
	}
	return n.parent
}

// GetEdge implements TrackerNodeInterface.
func (n *LazyNode) GetEdge() *metadata.CallGraphEdge { return n.edge }

// GetArgument implements TrackerNodeInterface.
func (n *LazyNode) GetArgument() *metadata.CallArgument { return n.arg }

// GetTypeParamMap implements TrackerNodeInterface: bindings from this node's
// edge/argument merged with its ancestors', nearest binding winning.
func (n *LazyNode) GetTypeParamMap() map[string]string {
	if n.typeParams != nil {
		return n.typeParams
	}
	out := map[string]string{}
	for cur := n; cur != nil; cur = cur.parent {
		if cur.edge != nil {
			for k, v := range cur.edge.TypeParamMap {
				if _, ok := out[k]; !ok {
					out[k] = v
				}
			}
		}
		if cur.arg != nil {
			for k, v := range cur.arg.TypeParams() {
				if _, ok := out[k]; !ok {
					out[k] = v
				}
			}
		}
	}
	n.typeParams = out
	return out
}

// onPath reports whether key is already an ancestor of n (cycle guard: the
// per-path state a lazy unfolding needs, in contrast to a global seen-set).
// instanceScope identifies the counting scope for maxInstancesPerKey: the
// key of the nearest argument-node ancestor (the handler/value subtree this
// node belongs to), or "" at wiring level. Each scope gets its own copy
// allowance, so shared helpers trace per route while intra-handler diamonds
// stay bounded.
func (n *LazyNode) instanceScope() string {
	for cur := n; cur != nil; cur = cur.parent {
		if cur.isArgument {
			return cur.key
		}
	}
	return ""
}

func (n *LazyNode) onPath(key string) bool {
	for cur := n; cur != nil; cur = cur.parent {
		if cur.key == key {
			return true
		}
	}
	return false
}

// GetChildren implements TrackerNodeInterface, expanding on first access:
// argument nodes for the node's own edge, then callee nodes from the
// memoized edge list, generics-filtered like the eager tree.
// childSpec is one planned child of a node: either an argument child (arg
// set) or a callee child (arg nil). Specs carry everything needed to
// materialize a LazyNode except the parent, which is per-path.
type childSpec struct {
	key string

	// argument child
	arg     *metadata.CallArgument
	argEdge *metadata.CallGraphEdge
	argType ArgumentType

	// callee child
	edge *metadata.CallGraphEdge
	// chainParented children are listed under this node but parented at the
	// call-site scope (processChainRelationships' rule), so chained-call
	// arguments trace through the enclosing call's ParamArgMap.
	chainParented bool
}

// planKey is a node's content identity — everything except its parent. Two
// per-path copies of the same call (same key, edge, argument) share one
// expansion plan; relevant generic bindings are embedded in the instance key
// itself ("fn[T=User]@pos"), so binding-distinct instances get distinct plans.
type planKey struct {
	key   string
	edge  *metadata.CallGraphEdge
	arg   *metadata.CallArgument
	isArg bool
}

// GetChildren implements TrackerNodeInterface, expanding on first access.
// The expansion PLAN (which children exist, structurally) is memoized per
// content identity; only the per-path guards — cycle check, per-scope
// instance caps, node budget — and node allocation run per copy.
func (n *LazyNode) GetChildren() []TrackerNodeInterface {
	if n.expanded {
		return n.children
	}
	if n.tree.budgetExhausted() {
		if !n.tree.budgetWarned {
			n.tree.budgetWarned = true
			fmt.Fprintf(os.Stderr,
				"Warning: MaxNodesPerTree limit (%d) reached, truncating lazy expansion (first at %s)\n",
				n.tree.limits.MaxNodesPerTree, n.key)
		}
		return nil // budget spent: further expansion yields leaves (cheap unwind)
	}
	n.expanded = true

	scope := n.instanceScope()
	if n.tree.instanceCount == nil {
		n.tree.instanceCount = map[string]map[string]int{}
	}
	scopeCounts := n.tree.instanceCount[scope]
	if scopeCounts == nil {
		scopeCounts = map[string]int{}
		n.tree.instanceCount[scope] = scopeCounts
	}
	childCount := 0
	for _, spec := range n.tree.planFor(n) {
		if spec.arg == nil && childCount >= n.tree.limits.MaxChildrenPerNode {
			continue
		}
		if n.onPath(spec.key) {
			continue // cycle: this call is already on the current path
		}
		if scopeCounts[spec.key] >= maxInstancesPerKey {
			// Diamond inside this scope: stop materializing further copies.
			// Reusing an existing instance instead would make the tree cyclic
			// (consumers of a memoized subtree could reach themselves), so the
			// bound is a skip — the role the eager per-ID recursion cap plays.
			continue
		}
		child := &LazyNode{
			tree:   n.tree,
			key:    spec.key,
			parent: n,
			edge:   spec.edge,
		}
		if spec.arg != nil {
			child.edge = spec.argEdge
			child.arg = spec.arg
			child.argType = spec.argType
			child.isArgument = true
		} else {
			if spec.chainParented && n.parent != nil {
				child.parent = n.parent
			}
			childCount++
		}
		scopeCounts[spec.key]++
		if n.tree.seenKeys == nil {
			n.tree.seenKeys = map[string]bool{}
		}
		if !n.tree.seenKeys[spec.key] {
			n.tree.seenKeys[spec.key] = true
			n.tree.nodesBuilt++ // budget counts globally distinct keys, like the eager shared-node cap
		}
		n.children = append(n.children, child)
	}
	return n.children
}

// planFor returns (building on first use) the memoized expansion plan for
// the node's content identity.
func (t *LazyTree) planFor(n *LazyNode) []childSpec {
	pk := planKey{key: n.key, edge: n.edge, arg: n.arg, isArg: n.isArgument}
	if plan, ok := t.plans[pk]; ok {
		return plan
	}
	plan := t.buildPlan(n)
	if t.plans == nil {
		t.plans = map[planKey][]childSpec{}
	}
	t.plans[pk] = plan
	return plan
}

// buildPlan computes a node's structural children: argument specs for the
// call that produced it, then callee specs from the function's own calls and
// every query-time relation (implementer fan-out, method-value and producer
// resolution, chains, receiver claims). Nothing here may depend on the
// node's parent — per-path concerns live in GetChildren.
func (t *LazyTree) buildPlan(n *LazyNode) []childSpec {
	t.buildRelations()
	meta := t.meta
	var plan []childSpec

	// Argument children. For a call node, the arguments of the call that
	// produced it (n.edge.Args); for an argument node, only the argument's
	// OWN edge (a function-call argument's nested call) — never the parent
	// edge it carries for context, or a literal argument would re-expand its
	// parent's args and reproduce itself forever.
	ownerEdge := n.edge
	if n.isArgument {
		ownerEdge = nil
		if n.argType == ArgTypeFunctionCall && n.arg != nil && n.arg.Edge != nil {
			ownerEdge = n.arg.Edge
		}
	}
	if ownerEdge != nil {
		for i, arg := range ownerEdge.Args {
			if i >= t.limits.MaxArgsPerFunction {
				break
			}
			argID := arg.ID()
			if argID == "" || arg.GetName() == "nil" ||
				ownerEdge.Caller.ID() == metadata.StripToBase(argID) || ownerEdge.Callee.ID() == argID {
				continue
			}
			argType := classifyArgument(arg)
			argEdge := ownerEdge
			if argType == ArgTypeFunctionCall && arg.Edge != nil {
				argEdge = arg.Edge
			}
			plan = append(plan, childSpec{
				key:     strings.TrimPrefix(argID, "*"),
				arg:     arg,
				argEdge: argEdge,
				argType: argType,
			})
		}
	}

	// Callee children: the function's own calls, then relation-derived ones.
	added := map[string]bool{}
	appendCalleeOpts := func(edge *metadata.CallGraphEdge, chainParented, genericFilter bool) {
		calleeID := strings.TrimPrefix(edge.Callee.ID(), "*")
		if added[calleeID] {
			return
		}
		// Same generics-instance filter as the eager tree's direct-callee
		// loop: skip instantiations whose type arguments aren't bound in
		// this node's context. ParentFunctions (closure-body) edges skip the
		// filter — as in the eager build — because a generic factory's
		// closure calls carry SYMBOLIC bindings (DecodeJSON[TData=TRequest])
		// that resolve through the ancestor chain, not concrete ones.
		if genericFilter {
			calleeTypes := t.genericTypesOf(calleeID)
			if len(calleeTypes) > 0 && !metadata.IsSubset(t.genericTypesOf(n.key), calleeTypes) {
				return
			}
		}
		added[calleeID] = true
		plan = append(plan, childSpec{key: calleeID, edge: edge, chainParented: chainParented})
	}
	appendCallee := func(edge *metadata.CallGraphEdge, chainParented bool) {
		appendCalleeOpts(edge, chainParented, true)
	}
	expandKey := func(key string) {
		edges := t.edgesFor(key)
		for _, edge := range edges {
			appendCallee(edge, false)
		}
		// No direct calls: follow into func literals defined in the function
		// (a factory's returned closure) via ParentFunctions, mirroring the
		// eager build's closure attachment (which applies no generics filter).
		if len(edges) == 0 {
			for _, edge := range meta.ParentFunctions[key] {
				appendCalleeOpts(edge, false, false)
			}
		}
	}
	expandKey(metadata.StripToBase(n.key))
	// Interface-method callee (module.RegisterRoutes(...) where module is an
	// interface): fan out into the concrete implementers' method bodies —
	// the eager build's ImplementedBy attachment. Without this, dispatch on
	// an interface value (e.g. captured by a functional-options closure) is
	// a dead end, since the interface method itself has no body.
	if n.edge != nil {
		calleeRecv := strings.TrimPrefix(getString(meta, n.edge.Callee.RecvType), "*")
		if calleeRecv != "" {
			calleePkg := getString(meta, n.edge.Callee.Pkg)
			calleeName := getString(meta, n.edge.Callee.Name)
			for _, implKey := range t.implementerKeys(calleePkg, calleeRecv, calleeName) {
				expandKey(implKey)
			}
		}
	}
	// Method-value handler (g.GET("/", h.GetUsers)): the argument is a
	// selector whose body lives under the method's own base ID
	// (pkg.recvType.name), not under the argument's key — resolve it so the
	// handler body (responses, params) is reachable from the route node.
	for _, methodKey := range n.methodBaseKeys() {
		expandKey(methodKey)
	}
	// Variable/field argument (router.Mount("/cart", r.cartRouter) or
	// Mount("/x", subRouter)): the producer subtree — the registrations
	// claimed under the router that was stored into the variable/field —
	// becomes this argument's children, so the mount prefix applies to them.
	for _, producerID := range n.argProducerIDs() {
		for _, edge := range t.receiverChildren[producerID] {
			appendCallee(edge, false)
		}
		expandKey(metadata.StripToBase(producerID))
	}
	// Chain children are listed under this node (so matchers see
	// `.Methods("GET")` on the route call, or `.Use(mw)` on a group) but
	// parented at the call-site scope — processChainRelationships' rule.
	for _, edge := range t.chainChildren[n.key] {
		appendCallee(edge, true)
	}
	for _, edge := range t.receiverChildren[n.key] {
		appendCallee(edge, false)
	}
	return plan
}

// methodBaseKeys resolves a method-referencing argument to the base ID(s) of
// the method it points at, so expansion can follow into its body:
//
//   - method value:   g.GET("/", h.GetUsers)   — the arg IS a selector;
//   - handler factory: g.POST("/x", h.Create()) — the arg is a CALL whose Fun
//     is a selector; the body lives in the closure the method returns, which
//     the ParentFunctions fallback in expandKey then reaches.
//
// Interface receivers fan out to their recorded implementers in either form.
func (n *LazyNode) methodBaseKeys() []string {
	arg := n.arg
	if !n.isArgument || arg == nil {
		return nil
	}
	if arg.GetKind() == metadata.KindCall && arg.Fun != nil && arg.Fun.GetKind() == metadata.KindSelector {
		arg = arg.Fun
	}
	if arg.GetKind() != metadata.KindSelector || arg.Sel == nil {
		return nil
	}
	selName := arg.Sel.GetName()
	pkg := arg.Sel.GetPkg()
	if selName == "" || pkg == "" {
		return nil
	}
	recv := ""
	if arg.ReceiverType != nil {
		recv = arg.ReceiverType.GetName()
	} else if arg.X != nil && arg.X.Type != -1 {
		recv = arg.X.GetType()
	}
	recv = strings.TrimPrefix(recv, "*")
	recv = strings.TrimPrefix(recv, pkg+".")
	recv = strings.TrimPrefix(recv, "*")
	if recv == "" {
		return nil
	}
	keys := []string{pkg + "." + recv + "." + selName}
	// Interface receiver: fan out to every implementer's method, mirroring
	// the eager build's ImplementedBy attachment.
	keys = append(keys, n.tree.implementerKeys(pkg, recv, selName)...)
	return keys
}

// argProducerIDs resolves a variable or struct-field argument to the callee
// IDs that produced its value, using the eager processArguments' exact key
// composition (CallArgToString + TraceVariableOrigin + assignmentKey with
// the parent-type container for selectors). This is what lets a mounted
// router arrive through `r.Mount("/cart", r.cartRouter)` (field, functional
// options) or `r.Mount("/x", subRouter)` (plain variable).
func (n *LazyNode) argProducerIDs() []string {
	arg := n.arg
	if !n.isArgument || arg == nil || n.edge == nil {
		return nil
	}
	meta := n.tree.meta
	callerName := getString(meta, n.edge.Caller.Name)
	callerPkg := getString(meta, n.edge.Caller.Pkg)

	switch {
	case n.argType == ArgTypeSelector && arg.X != nil:
		varName := metadata.CallArgToString(arg)
		baseVar, originPkg, _ := n.tree.traceOrigin(varName, callerName, callerPkg)
		parentType := arg.X.GetType()
		// Nested selector (obj.field.sub): the base variable's type wins —
		// same rule as the eager selector branch.
		if arg.X.GetKind() == metadata.KindSelector && arg.X.X != nil && arg.X.Sel != nil &&
			arg.X.Sel.GetKind() == metadata.KindIdent {
			parentType = arg.X.X.GetType()
		}
		akey := assignmentKey{Name: baseVar, Pkg: originPkg, Type: arg.GetType(), Container: callerName}
		if parentType != "" {
			akey.Container = parentType
		}
		return n.tree.producersFor(akey)

	case n.argType == ArgTypeVariable:
		varName := metadata.CallArgToString(arg)
		originVar, originPkg, _ := n.tree.traceOrigin(varName, callerName, callerPkg)
		return n.tree.producersFor(assignmentKey{
			Name: originVar, Pkg: originPkg, Type: arg.GetType(), Container: callerName,
		})
	}
	return nil
}

// producersFor resolves an assignment key to its producer plus, when the
// producer is an option/builder call, the producers of that call's own
// arguments (the actually-stored values).
func (t *LazyTree) producersFor(akey assignmentKey) []string {
	producer, ok := t.assignIndex[akey]
	if !ok {
		return nil
	}
	return append([]string{producer}, t.producerArgs[producer]...)
}

// implementerKeys returns "implPkg.ImplType.method" for every recorded
// implementer when (pkg, recv) names an interface type; nil otherwise.
func (t *LazyTree) implementerKeys(pkg, recv, method string) []string {
	p, ok := t.meta.Packages[pkg]
	if !ok {
		return nil
	}
	fileNames := make([]string, 0, len(p.Files))
	for name := range p.Files {
		fileNames = append(fileNames, name)
	}
	sort.Strings(fileNames)
	var out []string
	for _, name := range fileNames {
		typ, ok := p.Files[name].Types[recv]
		if !ok || getString(t.meta, typ.Kind) != "interface" {
			continue
		}
		for _, implIdx := range typ.ImplementedBy {
			impl := getString(t.meta, implIdx) // "import/path.Type"
			if impl != "" {
				out = append(out, impl+"."+method)
			}
		}
	}
	return out
}
