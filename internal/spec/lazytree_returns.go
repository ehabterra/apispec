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
	"strings"

	"github.com/ehabterra/apispec/internal/metadata"
)

// Returned producers (issue #633).
//
// A variable assigned from a helper call — `v1 := V1(r)` — is claimed by that
// call: the caller's `v1.GET(...)` hangs under the V1 call node. That is right
// only when the helper's call node is itself the router. When V1 builds a group
// inside its body and returns it,
//
//	func V1(r *gin.Engine) *gin.RouterGroup { g := r.Group("/v1"); return g }
//
// the value the caller holds is the Group call's result, and the prefix lives
// on the Group node, which is V1's CHILD. Prefixes flow by containment (golden
// rule #11), so routes hung beside it rather than beneath it were documented
// at the root.
//
// The binding added here follows the returned value to the call that produced
// it — a returned local to its own assignment, a returned call to itself, a
// returned call of another helper through that helper's return in turn, and a
// returned parameter to the value the call site passed — and hangs the
// caller's edges under THAT node.
//
// The producing node is not unique, though. V1's Group call has one key, but
// V1 can be invoked from several places with different parents (`a :=
// Version(pub); b := Version(adm)`), and every invocation materialises its own
// copy of that key. A plain receiverChildren entry would put a's routes under
// b's group too. So a binding records the ANCESTRY that identifies the
// invocation it belongs to — the keys of the nodes above the producing node, up
// to the first one that belongs to the caller's scope — and GetChildren adds
// the edges only beneath a copy with that ancestry. The ancestry is short (one
// key per hop the value made), so checking it costs no per-child O(depth) work.

// maxReturnHops bounds how far a returned value is followed through helpers.
// It is a cycle guard as much as a cost bound: a recursive helper returns its
// own call.
const maxReturnHops = 16

// returnBinding is one set of edges claimed by a variable whose value a helper
// returned, to be added under a copy of the producing node whose ancestors are
// exactly path (nearest first). A path element matches a node with that key,
// or an argument node whose producer is that key — the copy a Mount of the
// variable expands, which is where a mount prefix applies.
type returnBinding struct {
	path  []int32
	specs []childSpec
}

// returnPlace is where a value lives in the tree: the node key that produces
// it and, when that node is shared by several invocations, the ancestry that
// selects this one. An empty path means the producer belongs to a scope with
// one instance per caller (the value was passed in by the caller), so a plain
// receiverChildren entry is already exact.
type returnPlace struct {
	key  string
	path []string
}

// invocation identifies the chain of helper calls a value is being resolved
// in, nearest first: the call that invoked the function being read, the call
// that invoked ITS function, and so on up to the caller's own scope, which is
// noInvocation. Every node a helper builds has one copy per such chain, so
// whatever names one of them — a parameter's argument, a local's producer, a
// call site's position — is read in the whole chain, never in a truncated one:
// cut it anywhere and the answer is the same for every invocation above the
// cut, which binds each caller's routes beneath all of their copies.
//
// Chains are interned so a memo key carries one, not a composed string.
type invocation int32

const noInvocation invocation = 0

// invocationFrame is one link of an invocation chain: the call site, its edge,
// and the chain of the function that call sits in.
type invocationFrame struct {
	edge *metadata.CallGraphEdge
	site string
	up   invocation
}

// returnResolver follows returned values to their producers. It reads indexes
// buildRelations has already finished, so every answer is a pure function of
// the metadata and can be memoised.
type returnResolver struct {
	t             *LazyTree
	producerByVar map[recvKey]string
	// relsByVar: every assignment producerByVar was built from, per variable.
	relsByVar map[recvKey][]*metadata.AssignmentLink
	// localRels: same-scope assignments by (variable, function BaseID), the
	// key edgesByRecvVar uses — what a returned local resolves through.
	localRels map[string][]*metadata.AssignmentLink
	// claimedBy: the same-scope assignment whose variable an edge is called on;
	// nil when several assignments claim it (the variable is reassigned), which
	// leaves the edge's parent ambiguous.
	claimedBy map[*metadata.CallGraphEdge]*metadata.AssignmentLink
	// boundTo: per helper call site, the argument each of the helper's
	// parameter-receiver edges was bound to there (see LazyTree.boundAt).
	boundTo map[string]map[*metadata.CallGraphEdge]paramBind
	// moves counts, per helper edge, the call sites whose parameter binding
	// moved that edge under a producer — any producer: two bindings name
	// different keys for one node (`pub` and `Identity(pub)`), and a moved
	// call reads its arguments from no single one of them (see parentPath).
	moves map[*metadata.CallGraphEdge]int
	// pending counts, per helper edge, the parameter bindings that may still
	// move it once resolved, and pendingAt marks them per call site. A
	// binding is pending from before the first one is resolved until it is
	// bound or known never to resolve, so no answer counts moves that later
	// bindings would raise (see resolvePending).
	pending   map[*metadata.CallGraphEdge]int
	pendingAt map[string]map[*metadata.CallGraphEdge]int
	// frames holds the interned invocation chains (frames[0] is the caller's
	// own scope); frameIDs finds a chain by its frame.
	frames   []invocationFrame
	frameIDs map[invocationFrame]invocation
	// params memoises paramPlace by (function BaseID, parameter name).
	params map[string]paramMemo

	memo     map[returnMemoKey]returnMemo
	visiting map[returnMemoKey]bool
	// dead counts the helpers returned followed into without finding a place
	// for their value — it recurses into itself, it is past the hop bound, or
	// what it returns is placed in no one invocation — so holder can tell
	// "this call is not a helper to follow" (its own node is the producer)
	// from "this helper's value has no place" (neither does its node: the
	// value is not produced there, and a copy of a node deep in a helper is,
	// for the route walk, indistinguishable from the same copy in another
	// invocation, so edges bound beneath it are dropped).
	dead int
	// defers counts the answers that depended on a pending binding: a failure
	// that may turn into a place once that binding is decided.
	defers int
	// capped counts the lookups refused for the hop bound. An answer reached
	// while it tripped says how deep the question was asked, not what the
	// call returns, so it is not memoised: the same call asked with budget
	// left is resolved again. That holds for a place as much as a failure —
	// a refused follow falls back to the call node itself, a place that a
	// lookup with budget would have carried on to the helper's group.
	capped int
}

type paramBind struct {
	arg   metadata.CallArgument
	edge  *metadata.CallGraphEdge
	param string // the callee's parameter arg was passed for
	key   string
}

type paramMemo struct {
	place    returnPlace
	ok, done bool
	deferred bool
}

type returnMemoKey struct {
	edge *metadata.CallGraphEdge
	idx  int
	in   invocation
}

type returnMemo struct {
	place    returnPlace
	ok       bool
	dead     bool // failed after following the helper (see returnResolver.dead)
	deferred bool // failed on a pending binding (see returnResolver.defers)
}

// enter returns the chain of a call at site, by edge, inside a function
// invoked through up.
func (r *returnResolver) enter(edge *metadata.CallGraphEdge, site string, up invocation) invocation {
	if len(r.frames) == 0 {
		r.frames = []invocationFrame{{}}
		r.frameIDs = map[invocationFrame]invocation{}
	}
	f := invocationFrame{edge: edge, site: site, up: up}
	if id, ok := r.frameIDs[f]; ok {
		return id
	}
	id := invocation(len(r.frames))
	r.frames = append(r.frames, f)
	r.frameIDs[f] = id
	return id
}

// frame returns the innermost link of in; ok is false for noInvocation.
func (r *returnResolver) frame(in invocation) (invocationFrame, bool) {
	if in == noInvocation {
		return invocationFrame{}, false
	}
	return r.frames[in], true
}

// returned resolves where the value at index idx returned by the call pe lives,
// for pe inside a function invoked through in (noInvocation at the caller's own
// scope). ok is false when pe's callee has no recorded body, returns something
// that is not a variable or a call, or returns different values on different
// paths — then the call node itself is the honest producer.
func (r *returnResolver) returned(pe *metadata.CallGraphEdge, idx int, in invocation, hops int) (returnPlace, bool) {
	if pe == nil {
		return returnPlace{}, false
	}
	if hops > maxReturnHops {
		r.dead++
		r.capped++
		return returnPlace{}, false
	}
	mk := returnMemoKey{edge: pe, idx: idx, in: in}
	if m, ok := r.memo[mk]; ok {
		if m.dead {
			r.dead++
		}
		if m.deferred {
			r.defers++
		}
		return m.place, m.ok
	}
	// A recursive helper re-enters its own call in a longer chain each time,
	// so the cycle is the call itself being resolved again, in any chain.
	vk := returnMemoKey{edge: pe, idx: idx}
	if r.visiting[vk] {
		r.dead++ // recursive helper: its own call is not a producer to follow
		return returnPlace{}, false
	}
	before, deferred, capped := r.dead, r.defers, r.capped
	r.visiting[vk] = true
	place, ok := r.resolveReturned(pe, idx, in, hops)
	delete(r.visiting, vk)
	if r.capped != capped {
		return place, ok // the bound's answer, not the call's (see capped)
	}
	r.memo[mk] = returnMemo{place: place, ok: ok, dead: !ok && r.dead != before, deferred: !ok && r.defers != deferred}
	return place, ok
}

func (r *returnResolver) resolveReturned(pe *metadata.CallGraphEdge, idx int, in invocation, hops int) (returnPlace, bool) {
	rv, ok := r.t.returnValue(pe, idx)
	if !ok {
		return returnPlace{}, false
	}
	self := r.enter(pe, strings.TrimPrefix(pe.Callee.ID(), "*"), in)
	var inner returnPlace
	switch rv.GetKind() {
	case metadata.KindIdent:
		name := rv.GetName()
		if rels := r.localRels[name+"\x00"+pe.Callee.BaseID()]; len(rels) > 0 {
			if !sameProducer(rels) {
				return returnPlace{}, false // reassigned on some path: no one producer
			}
			inner, ok = r.holder(rels[0].Edge, rels[0].Assignment.ReturnIndex, self, hops+1)
			break
		}
		arg, isParam := pe.ParamArgMap[name]
		if !isParam {
			return returnPlace{}, false
		}
		// A returned parameter is the caller's own value, so its producer is in
		// the caller's scope and as exact as any receiver claim there.
		inner, ok = r.paramArgPlace(arg, pe, in, hops+1)
		if !ok {
			r.dead++
		}
		return inner, ok
	case metadata.KindCall:
		e := r.t.calleeBodyEdge(pe, rv)
		if e == nil {
			return returnPlace{}, false
		}
		inner, ok = r.holder(e, 0, self, hops+1)
	default:
		return returnPlace{}, false
	}
	if ok {
		inner, ok = r.anchor(inner, pe, in, hops)
	}
	if !ok {
		r.dead++
	}
	return inner, ok
}

// anchor completes inner, a place found inside the callee of pe, for pe inside
// a function invoked through in: anchored at pe's own node, the path goes on
// with where pe sits in ITS invocation.
func (r *returnResolver) anchor(inner returnPlace, pe *metadata.CallGraphEdge, in invocation, hops int) (returnPlace, bool) {
	site := strings.TrimPrefix(pe.Callee.ID(), "*")
	if n := len(inner.path); n > 0 && inner.path[n-1] == site {
		up, ok := r.parentPath(pe, in, hops+1)
		if !ok {
			return returnPlace{}, false
		}
		inner.path = append(inner.path[:n:n], up...)
	}
	return inner, true
}

// paramArgPlace resolves arg, a value the call pe passes, for pe inside a
// function invoked through in. A parameter of that function is whatever the
// invocation passed, read in turn in the invocation's own chain: without it,
// `Wrap(pub)` and `Wrap(adm)` around `return Identity(parent)` would share
// whichever producer tracing `parent` lands on. A parameter the function
// assigns to (`parent = other` on some path) holds that invocation's argument
// only on the paths that skip the assignment, so it has no place.
//
// A local of that function (`g := p.Group("/v"); return Identity(g)`) has a
// producer of its own, but one node per invocation too: it is followed in the
// chain, as a returned local is, or the copies made for LocalWrap(pub) and
// LocalWrap(adm) would both take every caller's routes.
//
// At the caller's own scope (no invocation) the value is resolved as argPlace
// does, which reads a parameter only where every call site agrees.
func (r *returnResolver) paramArgPlace(arg metadata.CallArgument, pe *metadata.CallGraphEdge, in invocation, hops int) (returnPlace, bool) {
	f, inside := r.frame(in)
	if !inside {
		return r.argPlace(arg, pe, hops)
	}
	if hops > maxReturnHops {
		r.capped++
		return returnPlace{}, false
	}
	if arg.GetKind() != metadata.KindIdent {
		// A call argument inside an invocation names a node of that
		// invocation, which argPlace places for none in particular.
		return returnPlace{}, false
	}
	name := arg.GetName()
	// The parameter test comes first: `p = q.Group("/x")` on one path is a
	// call result, recorded among the locals' producers like any other, and
	// would place p at that Group on every path.
	if passed, isParam := f.edge.ParamArgMap[name]; isParam {
		if r.t.assignsParam(f.edge, name) {
			return returnPlace{}, false
		}
		return r.paramArgPlace(passed, f.edge, f.up, hops+1)
	}
	if rels := r.localRels[name+"\x00"+pe.Caller.BaseID()]; len(rels) > 0 {
		if !sameProducer(rels) {
			return returnPlace{}, false // reassigned on some path: no one producer
		}
		inner, ok := r.holder(rels[0].Edge, rels[0].Assignment.ReturnIndex, in, hops+1)
		if !ok {
			return returnPlace{}, false
		}
		return r.anchor(inner, f.edge, f.up, hops)
	}
	return r.argPlace(arg, pe, hops) // package-level: one value for every invocation
}

// follow is returned for a caller that falls back on another answer when the
// value cannot be followed. wait reports a failure that depended on a pending
// binding: the value may yet have a place, so no fallback may stand in for it.
func (r *returnResolver) follow(pe *metadata.CallGraphEdge, idx int, in invocation, hops int) (place returnPlace, ok, wait bool) {
	before := r.defers
	place, ok = r.returned(pe, idx, in, hops)
	return place, ok, !ok && r.defers != before
}

// holder is the place of the value at index idx of the call e, for e inside a
// function invoked through in: what e's result resolves to when e is itself a
// helper returning a producer, otherwise e's own node.
//
// A helper followed into whose value has no place has no holder either (see
// returnResolver.dead). One that recurses into itself is the plainest case:
// expansion stops at the recursive call, so a node keyed by it, beneath a copy
// of itself, never materialises, and edges bound there would vanish.
func (r *returnResolver) holder(e *metadata.CallGraphEdge, idx int, in invocation, hops int) (returnPlace, bool) {
	before := r.dead
	if p, ok, wait := r.follow(e, idx, in, hops); ok || wait {
		return p, ok
	}
	if r.dead != before {
		return returnPlace{}, false
	}
	key := strings.TrimPrefix(e.Callee.ID(), "*")
	if r.t.argInstanceIDs[key] {
		return returnPlace{}, false // materialised as an argument node, not as a callee child
	}
	path, ok := r.parentPath(e, in, hops)
	if !ok {
		return returnPlace{}, false
	}
	return returnPlace{key: key, path: path}, true
}

// parentPath is the ancestry of e's node for e inside a function invoked
// through in, nearest first, up to the first node of the caller's scope.
// Mirrors how expansion places an edge: under the producer a parameter binding
// moved it to (boundAt), under the producer of the local it is called on
// (claimed), or in the function's own body under the call site.
//
// A moved or claimed edge hangs where buildRelations bound it, which it decided
// with no invocation to read from (its tree place). The ancestry read in the
// chain picks one invocation's copy among those, so it must extend the tree
// place; when it does not, the tree has no such copy and there is no place.
func (r *returnResolver) parentPath(e *metadata.CallGraphEdge, in invocation, hops int) ([]string, bool) {
	f, inside := r.frame(in)
	if inside {
		if b, ok := r.boundTo[f.site][e]; ok {
			// A parameter the helper assigns to (`if swap { p = q }`) holds the
			// bound argument only on the paths that skip the assignment, so
			// the binding does not say where e runs.
			if r.t.assignsParam(b.edge, b.param) {
				return nil, false
			}
			// Moved at another call site too, e is one node shared with it.
			// That is exact while e's arguments are the same in every
			// invocation (`Ident(r)` twice: one /ident group), but not when
			// they come from the invocation: `Version(pub, "/a")` and
			// `Version(pub, "/b")` returning `parent.Group(suffix)` are one
			// Group node under pub that carries neither suffix, and binding
			// both callers' routes beneath it merges them. No single
			// invocation owns that node. The count is per edge, not per
			// producer: `pub` and `Identity(pub)` are two keys for one node,
			// and even under distinct producers (`Version(adm, "/b")`) the
			// moved calls resolve the suffix from no one invocation.
			//
			// A pending binding may yet move e under the same producer, so it
			// counts as one: an answer given now must stay true once every
			// binding is in, whichever statement came first.
			if r.argsVary(e, b.edge) {
				moves := r.moves[e]
				if moves > 1 {
					return nil, false
				}
				if moves+r.pending[e] > 1 {
					r.defers++
					return nil, false
				}
			}
			p, ok := r.argPlace(b.arg, b.edge, hops+1)
			if !ok {
				p = returnPlace{key: b.key}
			}
			if f.up != noInvocation {
				// b.edge's caller is itself invoked through f.up: read the
				// argument there, among the copies p names.
				tree := p
				p, ok = r.paramArgPlace(b.arg, b.edge, f.up, hops+1)
				if !ok || !extends(p, tree) {
					return nil, false
				}
			}
			return append([]string{p.key}, p.path...), true
		}
		if r.pendingAt[f.site][e] > 0 {
			r.defers++ // not yet known whether this call site moves e
			return nil, false
		}
	}
	if rel, claimed := r.claimedBy[e]; claimed {
		if rel == nil {
			return nil, false
		}
		h, ok := r.holder(rel.Edge, rel.Assignment.ReturnIndex, in, hops+1)
		if !ok {
			return nil, false
		}
		if inside {
			tree, ok := r.returned(rel.Edge, rel.Assignment.ReturnIndex, noInvocation, hops+1)
			if !ok {
				tree = returnPlace{key: strings.TrimPrefix(rel.Edge.Callee.ID(), "*")}
			}
			if !extends(h, tree) {
				return nil, false
			}
		}
		return append([]string{h.key}, h.path...), true
	}
	if !inside {
		return nil, true
	}
	return []string{f.site}, true
}

// extends reports whether p is one of the copies tree names: the same node,
// with tree's ancestry as the start of its own.
func extends(p, tree returnPlace) bool {
	if p.key != tree.key || len(p.path) < len(tree.path) {
		return false
	}
	for i := range tree.path {
		if p.path[i] != tree.path[i] {
			return false
		}
	}
	return true
}

// argsVary reports whether any argument of e, a call in helper's callee body,
// can differ between invocations of the helper: it mentions one of the helper's
// parameters or locals. Literals and package-level names cannot.
func (r *returnResolver) argsVary(e, helper *metadata.CallGraphEdge) bool {
	assigns := r.t.calleeBody(helper).assigns
	var varies func(a *metadata.CallArgument) bool
	varies = func(a *metadata.CallArgument) bool {
		if a == nil {
			return false
		}
		if a.GetKind() == metadata.KindIdent {
			name := a.GetName()
			_, isParam := helper.ParamArgMap[name]
			return isParam || len(assigns[name]) > 0
		}
		if varies(a.X) || varies(a.Fun) {
			return true
		}
		for _, sub := range a.Args {
			if varies(sub) {
				return true
			}
		}
		return false
	}
	for _, a := range e.Args {
		if varies(a) {
			return true
		}
	}
	return false
}

// bind records that, at the helper call site, the helper's calls on a parameter
// were bound to b.
func (r *returnResolver) bind(site string, b paramBind, edges []*metadata.CallGraphEdge) {
	bound := r.boundTo[site]
	if bound == nil {
		bound = map[*metadata.CallGraphEdge]paramBind{}
		r.boundTo[site] = bound
	}
	for _, e := range edges {
		bound[e] = b
		r.moves[e]++
	}
}

// paramClaim is one call site's binding of a helper's calls on a parameter.
type paramClaim struct {
	bind  paramBind
	site  string
	edges []*metadata.CallGraphEdge
}

// resolvePending resolves the parameter bindings whose argument had no
// producer of its own, binds those that resolve and returns them.
//
// Resolving one binding can read how others moved a helper's calls
// (parentPath), so they are all pending before the first is resolved, and an
// answer that depended on a pending one is retried once the others are
// decided. Without that the answer followed statement order: a group returned
// by Suffixed(mk, "/oa") and passed on before the second Suffixed(mk, "/ob")
// call took the moved Group as its own and documented /mk/{suffix}/o.
// Each pass decides at least one binding or stops; the bindings still waiting
// on each other then stay unbound, where expansion leaves them anyway.
func (r *returnResolver) resolvePending(claims []paramClaim) []paramClaim {
	for _, c := range claims {
		r.pend(c.site, c.edges, 1)
	}
	var resolved []paramClaim
	for len(claims) > 0 {
		r.forget()
		var retry []paramClaim
		for _, c := range claims {
			before := r.defers
			place, ok := r.argPlace(c.bind.arg, c.bind.edge, 0)
			if !ok && r.defers != before {
				retry = append(retry, c)
				continue
			}
			r.pend(c.site, c.edges, -1)
			if !ok {
				continue
			}
			c.bind.key = place.key
			resolved = append(resolved, c)
			r.bind(c.site, c.bind, c.edges)
		}
		if len(retry) == len(claims) {
			for _, c := range retry {
				r.pend(c.site, c.edges, -1)
			}
			break
		}
		claims = retry
	}
	r.forget()
	return resolved
}

// pend adds delta to the pending bindings of edges at site.
func (r *returnResolver) pend(site string, edges []*metadata.CallGraphEdge, delta int) {
	if r.pending == nil {
		r.pending = map[*metadata.CallGraphEdge]int{}
		r.pendingAt = map[string]map[*metadata.CallGraphEdge]int{}
	}
	at := r.pendingAt[site]
	if at == nil {
		at = map[*metadata.CallGraphEdge]int{}
		r.pendingAt[site] = at
	}
	for _, e := range edges {
		r.pending[e] += delta
		at[e] += delta
	}
}

// forget drops every memoised answer, for when the bindings they were computed
// against have changed.
func (r *returnResolver) forget() {
	clear(r.memo)
	clear(r.params)
}

// argPlace resolves the value passed as arg at the call edge, following it
// through a helper's return when it was produced by one, else to the producer
// argProducerKey names.
func (r *returnResolver) argPlace(arg metadata.CallArgument, edge *metadata.CallGraphEdge, hops int) (returnPlace, bool) {
	if hops > maxReturnHops {
		r.capped++
		return returnPlace{}, false
	}
	switch arg.GetKind() {
	case metadata.KindIdent:
		// A local of the calling function first, read from its own assignment:
		// the origin trace below steps INTO a helper's return and, for a
		// returned call (`return r.Group("/x")`), comes out at the helper's
		// receiver — the router the group was made from, not the group.
		rels := r.localRels[arg.GetName()+"\x00"+edge.Caller.BaseID()]
		if len(rels) > 0 && sameProducer(rels) {
			if p, ok, wait := r.follow(rels[0].Edge, rels[0].Assignment.ReturnIndex, noInvocation, hops+1); ok || wait {
				return p, ok
			}
		}
		if len(rels) == 0 {
			if p, ok, isParam := r.paramPlace(arg.GetName(), &edge.Caller, hops+1); isParam {
				return p, ok
			}
		}
		originVar, originPkg, originFunc := r.t.traceOrigin(
			arg.GetName(),
			getString(r.t.meta, edge.Caller.Name),
			getString(r.t.meta, edge.Caller.Pkg),
		)
		k := recvKey{name: originVar, pkg: originPkg, fn: originFunc}
		key, ok := r.producerByVar[k]
		if !ok {
			return returnPlace{}, false
		}
		if rel := r.producerRel(k); rel != nil {
			if p, ok, wait := r.follow(rel.Edge, rel.Assignment.ReturnIndex, noInvocation, hops+1); ok || wait {
				return p, ok
			}
		}
		return returnPlace{key: key}, true
	case metadata.KindCall:
		if arg.Fun == nil {
			return returnPlace{}, false
		}
		if arg.Edge != nil {
			if p, ok, wait := r.follow(arg.Edge, 0, noInvocation, hops+1); ok || wait {
				return p, ok
			}
		}
		return returnPlace{key: arg.ID()}, true
	}
	return returnPlace{}, false
}

// paramPlace resolves the parameter name of the function fn with no invocation
// to read it from: through every call site of fn, and only when they all agree.
// isParam is false when name is not one of fn's parameters.
//
// Tracing the parameter's origin instead follows fn's first call site and
// answers with that caller's argument for every invocation — a helper called
// as Via(vx) and Via(vy) put both callers' routes under /vx.
func (r *returnResolver) paramPlace(name string, fn *metadata.Call, hops int) (place returnPlace, ok, isParam bool) {
	callers := r.t.meta.Callees[fn.BaseID()]
	if len(callers) == 0 {
		return returnPlace{}, false, false
	}
	if _, isParam := callers[0].ParamArgMap[name]; !isParam {
		return returnPlace{}, false, false
	}
	if r.t.assignsParam(callers[0], name) {
		return returnPlace{}, false, true // not every path holds a caller's argument
	}
	mk := name + "\x00" + fn.BaseID()
	if m, seen := r.params[mk]; seen {
		// A parameter reached again while it is being resolved forwards itself
		// through a recursive call: no producer along that path.
		if m.deferred {
			r.defers++
		}
		return m.place, m.ok && m.done, true
	}
	r.params[mk] = paramMemo{}
	deferred, capped := r.defers, r.capped
	var first returnPlace
	ok = true
	for i, c := range callers {
		arg, has := c.ParamArgMap[name]
		if !has {
			ok = false
			break
		}
		p, pok := r.argPlace(arg, c, hops)
		if !pok || (i > 0 && !samePlace(first, p)) {
			ok = false
			break
		}
		first = p
	}
	if !ok {
		first = returnPlace{}
	}
	if r.capped != capped {
		delete(r.params, mk) // the bound's answer, not the parameter's (see capped)
		return first, ok, true
	}
	r.params[mk] = paramMemo{place: first, ok: ok, done: true, deferred: !ok && r.defers != deferred}
	return first, ok, true
}

// samePlace reports whether two places are the same node.
func samePlace(a, b returnPlace) bool {
	if a.key != b.key || len(a.path) != len(b.path) {
		return false
	}
	for i := range a.path {
		if a.path[i] != b.path[i] {
			return false
		}
	}
	return true
}

// producerRel returns the assignment producerByVar recorded for k, when there
// is exactly one: it is what lets a variable's value be followed into the
// helper that produced it.
func (r *returnResolver) producerRel(k recvKey) *metadata.AssignmentLink {
	rels := r.relsByVar[k]
	if len(rels) == 0 || !sameProducer(rels) {
		return nil
	}
	return rels[0]
}

// sameProducer reports whether every assignment comes from one call result.
func sameProducer(rels []*metadata.AssignmentLink) bool {
	for _, rel := range rels[1:] {
		if rel.Edge != rels[0].Edge || rel.Assignment.ReturnIndex != rels[0].Assignment.ReturnIndex {
			return false
		}
	}
	return true
}

// returnValue is the value at index idx the callee of pe returns, provided every
// return statement that returns something other than nil there returns the same
// variable or the same call, and that variable is assigned once. Two different
// values (a branch returning another group, or reassigning the returned local)
// leave the result ambiguous, and golden rule #7 says not to pick one.
//
// The reassignment is read from the callee's own assignment list: the
// assignment links the resolver follows keep one assignment per variable, so
// they cannot tell a reassigned local from a single one.
func (t *LazyTree) returnValue(pe *metadata.CallGraphEdge, idx int) (metadata.CallArgument, bool) {
	body := t.calleeBody(pe)
	var picked *metadata.CallArgument
	for i := range body.returns {
		if idx < 0 || idx >= len(body.returns[i]) {
			continue
		}
		v := &body.returns[i][idx]
		if v.GetKind() == metadata.KindIdent && v.GetName() == "nil" {
			continue
		}
		if picked == nil {
			picked = v
			continue
		}
		if !sameReturnedValue(picked, v) {
			return metadata.CallArgument{}, false
		}
	}
	if picked == nil {
		return metadata.CallArgument{}, false
	}
	if picked.GetKind() == metadata.KindIdent {
		// A local is assigned once where it is declared; a parameter is
		// declared by the signature, so any assignment reassigns it.
		limit := 1
		if _, isParam := pe.ParamArgMap[picked.GetName()]; isParam {
			limit = 0
		}
		if len(body.assigns[picked.GetName()]) > limit {
			return metadata.CallArgument{}, false
		}
	}
	return *picked, true
}

// assignsParam reports whether the body of pe's callee assigns to its
// parameter name. Assignment links record only call results, so `parent =
// other` is read from the callee's own assignment list.
func (t *LazyTree) assignsParam(pe *metadata.CallGraphEdge, name string) bool {
	return len(t.calleeBody(pe).assigns[name]) > 0
}

// sameReturnedValue compares two return expressions: a variable by name (each
// return statement mentions it at its own position), a call by identity.
func sameReturnedValue(a, b *metadata.CallArgument) bool {
	if a.GetKind() != b.GetKind() {
		return false
	}
	if a.GetKind() == metadata.KindIdent {
		return a.GetName() == b.GetName()
	}
	return a.ID() == b.ID()
}

// calleeBody is what resolving a returned value reads from a callee's body.
// Every return statement, not ReturnVars: that is one of them, picked by arity.
type calleeBody struct {
	returns [][]metadata.CallArgument
	assigns map[string][]metadata.Assignment
}

// calleeBody returns the recorded body facts of pe's callee, a method or a
// plain function. A callee outside the analysed packages has none, which keeps
// framework calls (`r.Group`) as producers rather than descending into their
// internals.
func (t *LazyTree) calleeBody(pe *metadata.CallGraphEdge) calleeBody {
	meta := t.meta
	name := getString(meta, pe.Callee.Name)
	pkg := getString(meta, pe.Callee.Pkg)
	if name == "" {
		return calleeBody{}
	}
	if recv := getString(meta, pe.Callee.RecvType); recv != "" {
		typ := typeByName(pkg, bareTypeName(recv), meta)
		if typ == nil {
			return calleeBody{}
		}
		for i := range typ.Methods {
			if m := &typ.Methods[i]; getString(meta, m.Name) == name {
				return calleeBody{returns: m.Returns, assigns: m.AssignmentMap}
			}
		}
		return calleeBody{}
	}
	if fn := meta.FunctionInPackage(pkg, name); fn != nil {
		return calleeBody{returns: fn.Returns, assigns: fn.AssignmentMap}
	}
	return calleeBody{}
}

// calleeBodyEdge finds the call edge in pe's callee body that the returned call
// expression rv is: the call to the same name at the same position, or the root
// of the chain it ends.
//
// Matched on position and name, not on the expression's own ID: the edge
// records the callee as its receiver's method set resolves it, while the
// expression names the method where it is declared — a gin Engine's Group is
// promoted from the embedded RouterGroup, so `return r.Group("/x")` renders as
// RouterGroup.Group and its edge as Engine.Group.
func (t *LazyTree) calleeBodyEdge(pe *metadata.CallGraphEdge, rv metadata.CallArgument) *metadata.CallGraphEdge {
	name, pos := calleeNameOf(rv.Fun), rv.GetPosition()
	if name == "" || pos == "" {
		return nil
	}
	var found *metadata.CallGraphEdge
	bucket := t.meta.Callers[pe.Callee.BaseID()]
	for _, e := range bucket {
		if getString(t.meta, e.Position) != pos || getString(t.meta, e.Callee.Name) != name {
			continue
		}
		if found != nil {
			return nil // two candidates: not one producer to follow
		}
		found = e
	}
	// A chained expression (`r.PathPrefix("/api").Subrouter()`) is one value
	// produced by its chain, and the chain hangs under its root call — the node
	// an assignment of the same expression is linked to, and the one whose
	// prefix the chain carries.
	//
	// ChainParent is followed by identity, not by pointer: it is taken while the
	// call graph is still being appended to, so it can point into a superseded
	// backing array, and an index keyed by edge pointer would not recognise it.
	for i := 0; found != nil && found.ChainParent != nil; i++ {
		if i >= maxReturnHops {
			return nil
		}
		parentID := found.ChainParent.Callee.ID()
		found = nil
		for _, e := range bucket {
			if e.Callee.ID() == parentID {
				found = e
				break
			}
		}
	}
	return found
}

// bindReturned hangs edges under the node at place: a plain receiverChildren
// entry when the producer is unique to the caller's scope, otherwise a binding
// to the copies whose ancestry is place.path.
func (t *LazyTree) bindReturned(place returnPlace, edges []*metadata.CallGraphEdge) {
	if len(place.path) == 0 {
		t.receiverChildren[place.key] = append(t.receiverChildren[place.key], edges...)
		return
	}
	keyID := t.internKey(place.key)
	path := make([]int32, len(place.path))
	for i, p := range place.path {
		path[i] = t.internKey(p)
	}
	specs := make([]childSpec, 0, len(edges))
	for _, e := range edges {
		k := strings.TrimPrefix(e.Callee.ID(), "*")
		specs = append(specs, childSpec{key: k, keyID: t.internKey(k), edge: e})
	}
	for int(keyID) >= len(t.returnBound) {
		t.returnBound = append(t.returnBound, nil)
	}
	t.returnBound[keyID] = append(t.returnBound[keyID], returnBinding{path: path, specs: specs})
}

// returnBoundSpecs returns the edges bound beneath this copy of a returned
// producer (see returnBinding). Callee nodes only: an argument node is never
// the producing call itself.
func (n *LazyNode) returnBoundSpecs() []childSpec {
	t := n.tree
	if n.isArgument || int(n.key) >= len(t.returnBound) {
		return nil
	}
	var out []childSpec
	for _, b := range t.returnBound[n.key] {
		if n.hasAncestry(b.path) {
			out = append(out, b.specs...)
		}
	}
	return out
}

// hasAncestry reports whether the node's ancestors, nearest first, are path.
// An argument node stands for its producer: `r.Mount("/x", sub)` expands the
// factory that produced sub beneath the argument, and that copy is the one the
// mount prefix applies to.
func (n *LazyNode) hasAncestry(path []int32) bool {
	cur := n.parent
	for _, want := range path {
		if cur == nil {
			return false
		}
		if cur.key != want && (!cur.isArgument || !cur.producedBy(n.tree.keyString(want))) {
			return false
		}
		cur = cur.parent
	}
	return true
}

// producedBy reports whether this argument node's value was produced by key.
func (n *LazyNode) producedBy(key string) bool {
	for _, id := range n.argProducerIDs() {
		if id == key {
			return true
		}
	}
	return false
}
