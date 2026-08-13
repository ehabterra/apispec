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

	// handlerMethods are the framework's handler-interface methods (net/http's
	// "ServeHTTP"), used to expand a handler passed as a value — see
	// handlerValueKeys and issue #204. Empty for func-handler frameworks.
	handlerMethods []string

	// entrypointStats records what the entrypoint gate decided, so the UI can
	// report it (the numbers otherwise live only in a --verbose line).
	entrypointStats EntrypointStats

	// truncated records that expansion stopped at the node budget rather than
	// because there was nothing left to expand. It is the difference between a
	// spec that is complete and one that simply ends, and it used to be visible
	// only as a line on stderr (issue #233).
	truncated bool

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

	// instanceTruncations counts the copies the per-scope instance cap refused,
	// and instanceFirst* records the first refusal. Unlike MaxNodesPerTree, this
	// cap used to say nothing at all when it fired, so a starved response body
	// looked like a type the mapper could not resolve (issue #224). Naming the
	// SCOPE is the point: whether the scope that ran out is a handler (the cap
	// working as intended on a deep diamond) or something that spans many routes
	// (a group closure, where one route's expansion is eating another's budget)
	// is exactly the distinction the number alone cannot make.
	instanceTruncations int
	instanceFirstScope  string
	instanceFirstKey    string
	instanceWarned      bool

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

	// nodesBuilt counts distinct callee keys — what MaxNodesPerTree bounds. It is
	// NOT the number of nodes: see nodesMaterialized, and issue #247 for why the
	// two are reported separately rather than one being made to mean the other. The per-path cycle guard
	// bounds each path, but a dense cyclic graph still has exponentially many
	// distinct acyclic paths — the same blow-up MaxNodesPerTree exists to
	// stop in the eager tree. Once the budget is spent, expansion returns
	// leaves.
	nodesBuilt int

	// nodesMaterialized counts every LazyNode created, which is the work the
	// unfolding actually does. Reported, not budgeted (#247).
	nodesMaterialized int

	// instanceCount counts node copies per (instance scope, callee ID) —
	// see DefaultMaxInstancesPerKey. A node is (edge, parent), so a callee reached
	// along many paths gets many copies; business-layer diamonds make that
	// exponential and would drain the node budget before traversal reaches
	// later router wiring. Nested by scope to avoid a key concatenation per
	// child instantiation (visible in profiles).
	instanceCount map[string]map[string]int
	// funcFieldImpls resolves a call through a func-typed struct field
	// (c.Action()) to the functions that field holds — the urfave/cli wiring
	// style that left gitea with zero routes (issue #143).
	funcFieldImpls funcFieldDispatch

	// entrypointPatterns declare func-typed fields whose stored value a library
	// calls back with no edge from this module, so the function must be rooted
	// for its subtree to exist at all (issue #220). Empty unless the project
	// imports a command library (or declares its own pattern).
	entrypointPatterns []EntrypointPattern
	// routeMatch gates which entrypoints earn a root: only those whose subtree
	// can reach a route registration.
	routeMatch func(*metadata.CallGraphEdge) bool
	// terminalRouteMatch matches only the calls that register ONE route. It is
	// what opens a budget scope; routeMatch (which also matches mounts and
	// groups) must not, or every route inside a group shares one allowance —
	// see TerminalRouteMatcher.
	terminalRouteMatch func(*metadata.CallGraphEdge) bool
	// routeReach is routeMatch's transitive closure — the functions whose
	// expansion leads to a route registration. Used to ORDER a node's callee
	// children so the budget is spent on routing code first (issue #264); never
	// to drop any. Built once, on first use.
	routeReach     map[string]bool
	routeReachOnce bool

	// routeScopeNodes counts nodes materialised under each route registration,
	// indexed by scope id (0 is the shared wiring walk above every
	// registration). Slice rather than map: ids are dense and handed out in
	// order, and this is read on every materialised node.
	routeScopeNodes []int
	// routeScopeKeys names the registration each id stands for, for reporting.
	routeScopeKeys []string
	// routeScopeCut marks scopes already counted as truncated, so the report
	// counts ROUTES rather than blocked expansion attempts — a truncated scope is
	// re-entered many times as the walk unwinds, which read as "58 of 6".
	routeScopeCut []bool
	// routeTruncations counts route subtrees cut short by their own budget, and
	// routeFirstTruncated names the first. Unlike the whole-walk truncation this
	// is LOCAL — the rest of the routes are unaffected — so it is reported
	// separately rather than folded into Truncated.
	routeTruncations    int
	routeFirstTruncated string
	routeWarned         bool
	// logger reports how many entrypoints were rooted vs skipped.
	logger metadata.VerboseLogger

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

	// nodeSlab hands out LazyNodes in chunks instead of one heap object each.
	// Unfolding a DAG into paths is inherently allocation-heavy — a call reached
	// along many paths gets a node per path — and on a large project those
	// individual allocations dominated both the allocator and the collector: 4.1GB
	// of the 11.5GB a gitea run allocated came from this one site, with GC frames
	// (madvise, scanObjectsSmall) taking ~60% of CPU. A slab keeps the same node
	// lifetime and identity, and simply stops asking the allocator per node.
	nodeSlab []LazyNode

	// seenKeys backs the node budget: distinct callee IDs ever materialized,
	// the same graph-sized unit as the eager tree's shared-node cap —
	// deliberately NOT scoped, or many scopes would exhaust the budget with
	// copies of the same graph.
	//
	// The value is a bit set, not a bool, because two counts are needed over the
	// same keys and a second map would double the largest allocation in the run:
	// seenAny is the reported total (nodesBuilt), seenWiring the subset first
	// reached ABOVE any route registration — the only keys the wiring budget may
	// be charged for. Keeping those apart is what makes the two budgets of #264
	// independent; see wiringNodesBuilt.
	seenKeys map[string]uint8

	// wiringNodesBuilt counts distinct callee keys reached by the WIRING walk —
	// the keys above every route registration. MaxNodesPerTree bounds this, not
	// nodesBuilt.
	//
	// Charging the wiring budget for keys discovered INSIDE route subtrees makes
	// the two budgets one budget again, and inverts the whole fix: expanding a
	// route in more detail then costs the walk its ability to find the NEXT
	// route. Measured on a ~900-route project before the split was completed,
	// raising the per-route allowance made the spec smaller — 181 paths at
	// 20,000, 163 at 200,000, 103 at 1,000,000 — which is the same "improving
	// expansion makes the spec worse" defect #264 exists to remove, reintroduced
	// one level down.
	wiringNodesBuilt int
}

// Bits in seenKeys. seenAny is every key ever materialized; seenWiring marks the
// keys the wiring walk reached, which is the subset MaxNodesPerTree bounds.
const (
	seenAny uint8 = 1 << iota
	seenWiring
)

// DefaultMaxInstancesPerKey bounds node copies of the same callee WITHIN one
// instance scope (the subtree of the nearest argument-node ancestor —
// approximately "per handler"). Scoping matters: a response helper shared by
// every handler legitimately needs one copy per route for per-route value
// tracing, while call diamonds inside a single handler's business logic
// multiply copies combinatorially and must be cut — the role the eager
// tree's per-ID recursion cap plays.
//
// 25, not 10, because 10 measurably starved a real ~330-route chi service: 274
// of 350 success bodies came out with no schema, and raising the cap recovered
// them (issue #224).
//
// WHY that service starved is not settled, and the comment here used to assert
// an explanation it could not support — that a `r.Route("/x", func(r chi.Router)
// {…})` group closure, being itself an argument node, becomes the scope for
// every route in the group. That mechanism does not reproduce:
// testdata/group_closure_instances registers 15 routes inside one group closure,
// all writing through one shared helper, and every route keeps its body down to
// `--max-instances-per-key 1` — because the handler argument node, not the group
// closure, is the nearest argument ancestor and therefore the scope.
//
// So the cap is per-handler in that shape, and the starving service has a shape
// not yet reproduced. Rather than guess again, noteInstanceTruncation now names
// the scope that ran out whenever the cap fires, which is the one fact that
// distinguishes a bounded diamond from a starved route.
//
// The value is measured, not guessed — and the measurement is a TRADE, because
// raising the cap costs projects that gain nothing from it:
//
//	cap   that service        a 107-route gin service
//	 10    76 / 390 bodies     42s  (107 paths)
//	 25   387 / 390 bodies     66s  (107 paths)
//	 40   390 / 390 bodies     82s  (107 paths)
//	400   390 / 390 bodies    ~2x again
//
// 25 recovered 99.2% of the bodies for +0.6s on the starved service, while 40
// bought the last three and cost the second service another 16s for no extra
// route at all.
//
// RAISED TO 100. 25 held for as long as it took another service to grow into
// it, and the way it failed is the argument: on a ~374-route service, adding
// three handlers in an unrelated feature pushed a shared response helper past
// 25 copies and silently deleted the body of an endpoint nobody had touched.
// Nine 2xx bodies were empty at 25 and none at 100. The failure is not "large
// projects need more" — it is that the threshold moves when you edit somewhere
// else, so no project can know it is safe (evidence on #224).
//
// Re-measured for the new value:
//
//	                          cap 25            cap 100
//	374-route chi service     13.4s, 9 empty    15.3s, 0 empty
//	this repo (34 routes)     13.2s, 0 empty    14.5s, 0 empty
//	163-route chi service      7.0s, identical  12.8s, identical
//	19-path medium project     1s, identical     1s, identical
//
// The thing being bought changed, which is why this trade reads differently
// from the 25-vs-40 one above: 40 bought three bodies, 100 buys the property
// that adding an endpoint cannot silently remove documentation from another.
//
// The cost is NOT uniform, and the third row is the honest part of this
// decision: that service emits a byte-identical spec at both caps and takes
// 1.8x as long at 100 — it pays the entire cost for nothing. The cap fires
// there 3.6M times, first inside an error-formatting diamond (fmt.Errorf in a
// mongo mapper) that no response body depends on: the cap is doing its job, and
// a higher cap simply buys each of those dead copies 4x more work. Projects in
// that position should set --max-instances-per-key 25 explicitly; what they
// give up by doing so is only the guarantee that the number stays safe when
// the code is edited somewhere else.
//
// This is still a mitigation, not the fix. The cap is applied to the wrong
// UNIT (#224): scoped per route rather than per argument ancestor, a handful of
// copies would be ample everywhere and no project would pay for another's
// shape. A group with more than ~100 routes sharing one helper still starves,
// silently — the number moved, the shape did not.
const DefaultMaxInstancesPerKey = 100

// instanceBudget is the cap in force for this tree: the configured value, or the
// default. It is configurable because the right number depends on a project's
// shape — a group closure holding 40 routes needs more than one holding 5, and
// until the cap is scoped per route (issue #224) the only way to document such a
// project is to raise it.
func (t *LazyTree) instanceBudget() int {
	if t.limits.MaxInstancesPerKey > 0 {
		return t.limits.MaxInstancesPerKey
	}
	return DefaultMaxInstancesPerKey
}

// noteInstanceTruncation records — and, once, reports — a copy the instance cap
// refused.
//
// It warns rather than staying quiet because this cap is the one limit in the
// tree that used to truncate in silence: MaxNodesPerTree prints when it stops,
// and a starved response body from THIS cap was indistinguishable from a type
// the mapper could not resolve. Measured on a large real project, the cap fires
// 902,332 times in a default run, so the warning is deliberately once-only and
// carries the count in the stats instead.
//
// Scope and key are both named. The scope answers the question the count cannot:
// a handler scope that runs out is the cap doing its job on a deep diamond, while
// a scope spanning several routes means one route's expansion is consuming
// another's budget.
func (t *LazyTree) noteInstanceTruncation(scope, key string) {
	t.instanceTruncations++
	if t.instanceFirstKey == "" {
		t.instanceFirstScope = scope
		t.instanceFirstKey = key
	}
	if !t.instanceWarned {
		t.instanceWarned = true
		fmt.Fprintf(os.Stderr,
			"Warning: MaxInstancesPerKey limit (%d) reached, dropping repeated call copies (first: key %s in scope %s)\n",
			t.instanceBudget(), key, scopeLabel(scope))
	}
}

// scopeLabel renders an instance scope for a human. The empty scope is the
// wiring level — above any argument node — and "" would read as missing data.
func scopeLabel(scope string) string {
	if scope == "" {
		return "<router wiring>"
	}
	return scope
}

// leadsToRoute reports whether expanding a callee key can reach a route
// registration. Unknown answers "yes": this orders work, and a wrong "no" would
// push real routing code behind everything else.
func (t *LazyTree) leadsToRoute(key string) bool {
	if t.routeMatch == nil {
		return true
	}
	if !t.routeReachOnce {
		t.routeReachOnce = true
		t.routeReach = routeReachSet(t.meta, t.routeMatch)
	}
	if t.routeReach == nil {
		return true
	}
	return t.routeReach[metadata.StripToBase(key)]
}

// orderTowardRoutes moves the callee children that lead to a route registration
// ahead of those that do not, leaving each group's relative order alone.
//
// This is the whole of issue #264 at the tree level. The node budget is one
// allowance for the entire walk, and the extractor's traversal is depth-first:
// whatever is expanded first spends it. On a ~900-route project that was
// `modules/setting` and `modules/log`, and the run documented 12 paths before
// truncating — not because routing code is expensive, but because the budget was
// gone before the walk reached it.
//
// Ordering rather than pruning is the safe half of that fix. Nothing is dropped,
// so a subtree the reach set misses — and it will miss some, since reachability
// through data flow is undecidable in general — is merely expanded later. Pruning
// was tried and reverted: it deleted 124 of 312 operations.
//
// ARGUMENT children are deliberately left in place at the front. A route group's
// closure is an argument, not a callee, and it is the single most route-dense
// child a node can have; reordering around it could only hurt.
func (t *LazyTree) orderTowardRoutes(plan []childSpec, from int) {
	if t.routeMatch == nil || from >= len(plan) {
		return
	}
	seg := plan[from:]
	leading := make([]childSpec, 0, len(seg))
	trailing := make([]childSpec, 0, len(seg))
	for _, spec := range seg {
		if t.leadsToRoute(spec.key) {
			leading = append(leading, spec)
		} else {
			trailing = append(trailing, spec)
		}
	}
	if len(leading) == 0 || len(trailing) == 0 {
		return // nothing to reorder; keep the slice untouched
	}
	copy(seg, leading)
	copy(seg[len(leading):], trailing)
}

// DefaultMaxNodesPerRoute bounds the nodes materialised below ONE route
// registration.
//
// It exists because a single global budget makes truncation total: expansion is
// depth-first, so the routes not yet reached when it runs out are lost outright
// rather than documented in less detail. Measured on a ~900-route project, that
// was 12 paths of ~900 — the budget spent inside modules/setting before the walk
// reached the routers at all.
//
// Per route, a handler too deep to document fully costs its own detail and
// nothing else's, which is the difference between a spec that is missing 98% of
// its endpoints and one where a few endpoints are missing a schema.
//
// THE VALUE IS SET BY WHAT IT MUST NOT COST, not by what it saves. A per-route
// cap is a NEW restriction: before it, a route's detail was bounded only by the
// global budget, so every project that never hit that budget was effectively
// unbounded below its registrations. Anything low enough to be a useful ceiling
// is therefore a regression for those projects, and a silent one — a truncated
// route loses a request body or a response schema, not a path, so the spec still
// looks complete. Measured against per-project snapshots: at 20,000 three real
// projects lost detail on endpoints they had always documented; 200,000 cleared
// all but one endpoint, whose four responses need somewhere past 400,000; a
// million restores exact parity everywhere.
//
// It buys the ceiling cheaply because deep routes are rare. On the project with
// that endpoint, a million costs nothing measurable (9.0s either way) — only the
// one deep route expands further. Where routes ARE deep it is the whole trade:
// a ~900-route project documents 491 paths at 20,000, 581 at 200,000 and 640 at
// a million, for 66s / 112s / 166s. Paying that by default is the right way
// round, because a project with no deep route does not pay it at all, and one
// that has them was losing endpoints silently.
const DefaultMaxNodesPerRoute = 1000000

// routeBudget is the per-registration cap in force for this tree.
func (t *LazyTree) routeBudget() int {
	if t.limits.MaxNodesPerRoute > 0 {
		return t.limits.MaxNodesPerRoute
	}
	return DefaultMaxNodesPerRoute
}

// scopeOf returns the budget scope a child of n belongs to: a new one when n is
// itself a route registration, otherwise n's own.
//
// A registration opens a scope rather than joining one, so everything the route
// pulls in — its handler, that handler's helpers, their types — is charged to
// that route and to nothing else.
func (t *LazyTree) scopeOf(n *LazyNode) int {
	if n == nil {
		return 0
	}
	if n.routeScope != 0 || t.terminalRouteMatch == nil || n.edge == nil || !t.terminalRouteMatch(n.edge) {
		return n.routeScope
	}
	// First registration on this path: open a scope for it.
	if len(t.routeScopeNodes) == 0 {
		t.routeScopeNodes = []int{0} // id 0 is the wiring walk
		t.routeScopeKeys = []string{""}
	}
	t.routeScopeNodes = append(t.routeScopeNodes, 0)
	t.routeScopeKeys = append(t.routeScopeKeys, n.key)
	return len(t.routeScopeNodes) - 1
}

// budgetExhausted reports whether the WIRING budget is spent — the walk that
// finds route registrations in the first place.
//
// It no longer bounds the whole tree. Detail below a registration is bounded per
// route (routeBudget), so a deep handler cannot consume the allowance the
// undiscovered routes still need (issue #264) — which is why the count it reads
// is wiringNodesBuilt and not the global nodesBuilt.
func (t *LazyTree) budgetExhausted() bool {
	return t.limits.MaxNodesPerTree > 0 && t.wiringNodesBuilt >= t.limits.MaxNodesPerTree
}

// countKey records that a callee key was materialised in the given budget
// scope, keeping the two counts the two budgets read.
//
// The wiring bit is set on its own occasion rather than only the first time a
// key is seen at all: a key can be reached below a route first and by the wiring
// walk afterwards, and skipping it then would let a walk that happens to descend
// into a route early understate its own cost without bound.
func (t *LazyTree) countKey(key string, scope int) {
	if t.seenKeys == nil {
		t.seenKeys = map[string]uint8{}
	}
	seen := t.seenKeys[key]
	if seen&seenAny == 0 {
		t.nodesBuilt++
	}
	if scope == 0 && seen&seenWiring == 0 {
		seen |= seenWiring
		t.wiringNodesBuilt++
	}
	t.seenKeys[key] = seen | seenAny
}

// routeBudgetExhausted reports whether this node's route has spent its own
// allowance. Scope 0 — the wiring walk — is bounded by budgetExhausted instead.
func (t *LazyTree) routeBudgetExhausted(scope int) bool {
	return scope > 0 && scope < len(t.routeScopeNodes) && t.routeScopeNodes[scope] >= t.routeBudget()
}

// noteRouteTruncation records a route subtree cut short by its own budget, and
// warns once. The route's own key is named: unlike the whole-walk truncation,
// this says exactly which endpoint is under-documented.
func (t *LazyTree) noteRouteTruncation(scope int) {
	for len(t.routeScopeCut) <= scope {
		t.routeScopeCut = append(t.routeScopeCut, false)
	}
	if t.routeScopeCut[scope] {
		return // already counted: one route, however often its subtree is re-entered
	}
	t.routeScopeCut[scope] = true
	t.routeTruncations++
	key := ""
	if scope < len(t.routeScopeKeys) {
		key = t.routeScopeKeys[scope]
	}
	if t.routeFirstTruncated == "" {
		t.routeFirstTruncated = key
	}
	if !t.routeWarned {
		t.routeWarned = true
		fmt.Fprintf(os.Stderr,
			"Warning: MaxNodesPerRoute limit (%d) reached, truncating one route's detail (first at %s)\n",
			t.routeBudget(), key)
	}
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
	var entrypointKeys []string
	t.funcFieldImpls, entrypointKeys = buildFuncFieldDispatch(meta, t.entrypointPatterns)
	t.addEntrypointRoots(entrypointKeys)

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
		// Claim receiver calls only when the assignment lives in the producing
		// edge's caller. Callee-body assignments can leak through AssignmentMap;
		// matching them by variable name would steal unrelated caller-scope edges.
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
// LazyTreeOption configures an optional tree capability. Options keep the
// constructor's existing two-argument form working for every caller that does
// not need them (notably the test suite).
type LazyTreeOption func(*LazyTree)

// WithHandlerInterfaceMethods supplies the framework's handler-interface methods
// so a handler passed as a value expands into its body (issue #204).
func WithHandlerInterfaceMethods(methods []string) LazyTreeOption {
	return func(t *LazyTree) { t.handlerMethods = methods }
}

// WithEntrypoints declares the func-typed fields whose stored functions a library
// dispatcher invokes, plus the predicate that decides whether an entrypoint's
// subtree reaches a route registration (issue #220). Without it the tree keeps
// its main-only roots.
func WithEntrypoints(patterns []EntrypointPattern, routeMatch func(*metadata.CallGraphEdge) bool, logger metadata.VerboseLogger) LazyTreeOption {
	return func(t *LazyTree) {
		t.entrypointPatterns = patterns
		t.routeMatch = routeMatch
		t.logger = logger
	}
}

// WithTerminalRouteMatcher supplies the predicate that decides which node opens a
// per-route budget scope: a single route registration, never a mount or group
// (issue #264). Without it no scope is opened and expansion is bounded only by
// MaxNodesPerTree, as it was before.
func WithTerminalRouteMatcher(match func(*metadata.CallGraphEdge) bool) LazyTreeOption {
	return func(t *LazyTree) { t.terminalRouteMatch = match }
}

// addEntrypointRoots appends a root per qualifying entrypoint. Called from
// buildRelations (which every expansion path already goes through) rather than
// from the constructor, because the candidate set comes out of the same walk that
// builds the func-field index.
func (t *LazyTree) addEntrypointRoots(candidates []string) {
	if len(candidates) == 0 || t.routeMatch == nil {
		return
	}
	existing := make(map[string]bool, len(t.roots))
	for _, r := range t.roots {
		existing[metadata.StripToBase(r.GetKey())] = true
	}
	rooted, stats := entrypointRoots(t.meta, candidates, t.routeMatch, t.logger)
	t.entrypointStats = stats
	for _, key := range rooted {
		if existing[key] {
			continue
		}
		existing[key] = true
		t.roots = append(t.roots, &LazyNode{tree: t, key: key})
	}
}

func NewLazyTree(meta *metadata.Metadata, limits metadata.TrackerLimits, opts ...LazyTreeOption) *LazyTree {
	t := &LazyTree{
		meta:        meta,
		limits:      limits,
		calleeEdges: make(map[string][]*metadata.CallGraphEdge),
	}
	for _, opt := range opts {
		opt(t)
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
//
// buildRelations runs here, not just on first expansion: entrypoint roots
// (issue #220) come out of the same walk that builds the func-field index, and
// the extractor asks for roots BEFORE it expands anything. Deferring left the
// extra roots invisible — the tree had them, nobody ever saw them. The call is
// memoized, so this only moves work that every expansion path pays anyway.
// ExpansionStats reports how far expansion got, and whether it stopped early.
func (t *LazyTree) ExpansionStats() ExpansionStats {
	t.buildRelations()
	return ExpansionStats{
		NodesBuilt:          t.nodesBuilt,
		NodesMaterialized:   t.nodesMaterialized,
		Limit:               t.limits.MaxNodesPerTree,
		Truncated:           t.truncated,
		InstanceTruncations: t.instanceTruncations,
		InstanceLimit:       t.instanceBudget(),
		InstanceFirstScope:  t.instanceFirstScope,
		InstanceFirstKey:    t.instanceFirstKey,
		RouteTruncations:    t.routeTruncations,
		RouteLimit:          t.routeBudget(),
		RouteFirstTruncated: t.routeFirstTruncated,
		RoutesScoped:        max(len(t.routeScopeNodes)-1, 0),
	}
}

// EntrypointStats reports what the entrypoint gate decided during this tree's
// build. Zero when the project declares no entrypoints.
func (t *LazyTree) EntrypointStats() EntrypointStats {
	t.buildRelations() // the gate runs there; asking first would report zeros
	return t.entrypointStats
}

func (t *LazyTree) GetRoots() []TrackerNodeInterface {
	if t == nil {
		return nil
	}
	t.buildRelations()
	return t.roots
}

// GetMetadata implements TrackerTreeInterface.
func (t *LazyTree) GetMetadata() *metadata.Metadata { return t.meta }

// LazyNode implements TrackerNodeInterface. Identity is (content, parent):
// node objects are per-path, so Parent is always the actual expansion parent.
// FIELD ORDER: the pointer-shaped fields come first and the two bools sit
// together at the end, so they share one padding word instead of each rounding
// the struct up on its own. That is worth stating because the natural grouping
// — argType with isArgument, expanded with children — costs 8 bytes per node,
// and nodes are the single most numerous allocation in a large run (they are
// slab-allocated in newNode for the same reason). See
// `go vet -vettool=$(which fieldalignment)`.
type LazyNode struct {
	tree   *LazyTree
	key    string
	parent *LazyNode

	edge *metadata.CallGraphEdge
	arg  *metadata.CallArgument

	typeParams map[string]string // GetTypeParamMap cache

	children []TrackerNodeInterface // nil = not yet expanded

	// routeScope identifies which budget this node's expansion is charged to:
	// the id of the nearest route-registration ancestor, or 0 for the wiring
	// walk above every registration (issue #264).
	//
	// Computed once, at creation, by inheriting the parent's — walking up to
	// find it per expansion would be O(depth) work on every child, which goes
	// quadratic over deep graphs and is the shape that shows up in profiles as
	// GC dominance rather than as the guilty frame.
	routeScope int

	argType    ArgumentType
	isArgument bool
	expanded   bool
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
// instanceScope identifies the counting scope for the instance budget: the
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
// FIELD ORDER: pointers first, then the int and the bool together at the end —
// a plan holds one of these per child of every expanded node.
type childSpec struct {
	// argument child
	arg     *metadata.CallArgument
	argEdge *metadata.CallGraphEdge

	// callee child
	edge *metadata.CallGraphEdge

	key string

	argType ArgumentType
	// chainParented children are listed under this node but parented at the
	// call-site scope (processChainRelationships' rule), so chained-call
	// arguments trace through the enclosing call's ParamArgMap.
	chainParented bool
}

// planKey is a node's content identity — everything except its parent. Two
// per-path copies of the same call (same key, edge, argument) share one
// expansion plan; relevant generic bindings are embedded in the instance key
// itself ("fn[T=User]@pos"), so binding-distinct instances get distinct plans.
//
// FIELD ORDER: the two pointers lead, so the pointer-scan prefix ends before
// the string's length word rather than spanning the whole struct.
type planKey struct {
	edge  *metadata.CallGraphEdge
	arg   *metadata.CallArgument
	key   string
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
	// Two budgets, and which one applies depends on where this node sits. Above
	// every route registration the WIRING budget bounds the walk that finds them;
	// below one, that route's own allowance bounds its detail. Splitting them is
	// what makes truncation local instead of total (issue #264).
	if n.routeScope == 0 && n.tree.budgetExhausted() {
		n.tree.truncated = true
		if !n.tree.budgetWarned {
			n.tree.budgetWarned = true
			fmt.Fprintf(os.Stderr,
				"Warning: MaxNodesPerTree limit (%d) reached, truncating the walk that finds routes (first at %s)\n",
				n.tree.limits.MaxNodesPerTree, n.key)
		}
		return nil // budget spent: further expansion yields leaves (cheap unwind)
	}
	if n.tree.routeBudgetExhausted(n.routeScope) {
		n.tree.noteRouteTruncation(n.routeScope)
		return nil // this route is documented in less detail; the others are not affected
	}
	n.expanded = true

	// A registration opens its own scope, so everything it pulls in is charged
	// to that route. Resolved once here rather than per child.
	childScope := n.tree.scopeOf(n)

	scope := n.instanceScope()
	if n.tree.instanceCount == nil {
		n.tree.instanceCount = map[string]map[string]int{}
	}
	scopeCounts := n.tree.instanceCount[scope]
	if scopeCounts == nil {
		scopeCounts = map[string]int{}
		n.tree.instanceCount[scope] = scopeCounts
	}
	plan := n.tree.planFor(n)
	if cap(n.children) < len(plan) {
		n.children = make([]TrackerNodeInterface, 0, len(plan))
	}
	childCount := 0
	for _, spec := range plan {
		if spec.arg == nil && childCount >= n.tree.limits.MaxChildrenPerNode {
			continue
		}
		if n.onPath(spec.key) {
			continue // cycle: this call is already on the current path
		}
		if scopeCounts[spec.key] >= n.tree.instanceBudget() {
			// Diamond inside this scope: stop materializing further copies.
			// Reusing an existing instance instead would make the tree cyclic
			// (consumers of a memoized subtree could reach themselves), so the
			// bound is a skip — the role the eager per-ID recursion cap plays.
			n.tree.noteInstanceTruncation(scope, spec.key)
			continue
		}
		child := n.tree.newNode()
		child.tree = n.tree
		child.key = spec.key
		child.parent = n
		child.edge = spec.edge
		child.routeScope = childScope
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
		// Two different quantities, deliberately kept apart (issue #247).
		//
		// nodesMaterialized is the WORK: one node per path a call is reached along,
		// which is what costs time and memory. nodesBuilt is what the budget
		// bounds — distinct callee keys — and the two differ by more than an order
		// of magnitude: a 246-route service builds 208,394 nodes across 20,198
		// keys.
		//
		// The budget stays on keys because switching it to nodes, though truthful,
		// starves route discovery: a global budget is spent depth-first on whatever
		// is expanded first, and on gitea that is modules/setting and modules/log.
		// Measured, gitea documents 900 paths at a raised KEY budget and 1 path at
		// a node budget of fifteen million. The unit is the real problem and it is
		// #224; what this counter can honestly do meanwhile is report the work
		// instead of hiding it.
		n.tree.nodesMaterialized++
		if childScope > 0 && childScope < len(n.tree.routeScopeNodes) {
			n.tree.routeScopeNodes[childScope]++
		}
		n.tree.countKey(spec.key, childScope)
		n.children = append(n.children, child)
	}
	return n.children
}

// nodeSlabChunk is how many nodes are carved at once. Large enough that the
// allocation is rare, small enough that a tree which stops early has not
// reserved much it will never use.
const nodeSlabChunk = 4096

// newNode returns a zeroed node from the current slab, carving a new one when
// the current is spent. The returned pointer is stable: a slab is never grown in
// place (append would copy and invalidate every pointer already handed out), it
// is replaced.
func (t *LazyTree) newNode() *LazyNode {
	if len(t.nodeSlab) == 0 {
		t.nodeSlab = make([]LazyNode, nodeSlabChunk)
	}
	node := &t.nodeSlab[0]
	t.nodeSlab = t.nodeSlab[1:]
	return node
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
	// calleeStart marks where they begin, so orderTowardRoutes can reorder them
	// without disturbing the argument children ahead of them (issue #264).
	calleeStart := len(plan)
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
		// Func-typed field callee (c.Action() on a cli.Command): the field is a
		// dispatch point just like an interface method, and the functions
		// recorded for it are the bodies to follow (issue #143). Without this,
		// the whole registration subtree behind a CLI dispatcher is unreachable.
		for _, fnKey := range t.funcFieldImpls.keysFor(meta, n.edge) {
			expandKey(fnKey)
		}
	}
	// Method-value handler (g.GET("/", h.GetUsers)): the argument is a
	// selector whose body lives under the method's own base ID
	// (pkg.recvType.name), not under the argument's key — resolve it so the
	// handler body (responses, params) is reachable from the route node.
	for _, methodKey := range n.methodBaseKeys() {
		expandKey(methodKey)
	}
	// Handler *value* (r.Method(GET, "/health", deps.Health) / mux.Handle("/x", h)):
	// the argument names no method at all, so the framework's handler-interface
	// method supplies it — otherwise the handler body is unreachable (issue #204).
	for _, methodKey := range n.handlerValueKeys() {
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

	// Spend the budget on routing code first — see orderTowardRoutes.
	t.orderTowardRoutes(plan, calleeStart)
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

// handlerValueKeys resolves an argument that is a handler *value* — a variable
// or struct field holding something the framework invokes through an interface
// method — to the base ID(s) of that method's body (issue #204).
//
// This is the shape methodBaseKeys cannot serve: `r.Method(GET, "/health",
// deps.Health)` and `mux.Handle("/x", h)` name no method anywhere in the
// registration, so there is no selector to resolve. The method name comes from
// the framework config (FrameworkConfig.HandlerInterfaceMethods), never from a
// hardcoded "ServeHTTP" — frameworks whose handlers are func types declare none
// and get no keys.
//
// Resolution is honest in both directions: a concrete type contributes a key
// only for a handler method it actually declares, and an interface-typed value
// fans out to its recorded implementers (the same ImplementedBy index the
// method-call paths use) rather than guessing one. A value whose type declares
// no configured handler method yields nothing.
func (n *LazyNode) handlerValueKeys() []string {
	if !n.isArgument || n.arg == nil || len(n.tree.handlerMethods) == 0 {
		return nil
	}
	pkg, name := handlerValueTypeOf(n.arg)
	return handlerMethodKeys(n.tree.meta, n.tree.handlerMethods, pkg, name)
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
