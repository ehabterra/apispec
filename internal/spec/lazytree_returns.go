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

	memo     map[returnMemoKey]returnMemo
	visiting map[returnMemoKey]bool
}

type paramBind struct {
	arg  metadata.CallArgument
	edge *metadata.CallGraphEdge
	key  string
}

type returnMemoKey struct {
	edge *metadata.CallGraphEdge
	idx  int
	site string
}

type returnMemo struct {
	place returnPlace
	ok    bool
}

// returned resolves where the value at index idx returned by the call pe lives,
// for pe invoked inside a function that was itself invoked at outerSite ("" when
// that invocation is not known, i.e. at the caller's own scope). ok is false
// when pe's callee has no recorded body, returns something that is not a
// variable or a call, or returns different values on different paths — then the
// call node itself is the honest producer.
func (r *returnResolver) returned(pe *metadata.CallGraphEdge, idx int, outerSite string, hops int) (returnPlace, bool) {
	if pe == nil || hops > maxReturnHops {
		return returnPlace{}, false
	}
	mk := returnMemoKey{edge: pe, idx: idx, site: outerSite}
	if m, ok := r.memo[mk]; ok {
		return m.place, m.ok
	}
	if r.visiting[mk] {
		return returnPlace{}, false // recursive helper: its own call is not a producer to follow
	}
	r.visiting[mk] = true
	place, ok := r.resolveReturned(pe, idx, outerSite, hops)
	delete(r.visiting, mk)
	r.memo[mk] = returnMemo{place: place, ok: ok}
	return place, ok
}

func (r *returnResolver) resolveReturned(pe *metadata.CallGraphEdge, idx int, outerSite string, hops int) (returnPlace, bool) {
	rv, ok := r.t.returnValue(pe, idx)
	if !ok {
		return returnPlace{}, false
	}
	site := strings.TrimPrefix(pe.Callee.ID(), "*")
	var inner returnPlace
	switch rv.GetKind() {
	case metadata.KindIdent:
		name := rv.GetName()
		if rels := r.localRels[name+"\x00"+pe.Callee.BaseID()]; len(rels) > 0 {
			if !sameProducer(rels) {
				return returnPlace{}, false // reassigned on some path: no one producer
			}
			inner, ok = r.holder(rels[0].Edge, rels[0].Assignment.ReturnIndex, site, hops+1)
			break
		}
		arg, isParam := pe.ParamArgMap[name]
		if !isParam {
			return returnPlace{}, false
		}
		// A returned parameter is the caller's own value, so its producer is in
		// the caller's scope and as exact as any receiver claim there.
		return r.argPlace(arg, pe, hops+1)
	case metadata.KindCall:
		e := r.t.calleeBodyEdge(pe, rv)
		if e == nil {
			return returnPlace{}, false
		}
		inner, ok = r.holder(e, 0, site, hops+1)
	default:
		return returnPlace{}, false
	}
	if !ok {
		return returnPlace{}, false
	}
	// Anchored at pe's own node: extend with where pe sits in ITS invocation.
	if n := len(inner.path); n > 0 && inner.path[n-1] == site {
		up, ok := r.parentPath(pe, outerSite, hops+1)
		if !ok {
			return returnPlace{}, false
		}
		inner.path = append(inner.path[:n:n], up...)
	}
	return inner, true
}

// holder is the place of the value at index idx of the call e, for e inside a
// function invoked at site: what e's result resolves to when e is itself a
// helper returning a producer, otherwise e's own node.
func (r *returnResolver) holder(e *metadata.CallGraphEdge, idx int, site string, hops int) (returnPlace, bool) {
	if p, ok := r.returned(e, idx, site, hops); ok {
		return p, true
	}
	key := strings.TrimPrefix(e.Callee.ID(), "*")
	if r.t.argInstanceIDs[key] {
		return returnPlace{}, false // materialised as an argument node, not as a callee child
	}
	path, ok := r.parentPath(e, site, hops)
	if !ok {
		return returnPlace{}, false
	}
	return returnPlace{key: key, path: path}, true
}

// parentPath is the ancestry of e's node for e inside a function invoked at
// site, nearest first, up to the first node of the caller's scope. Mirrors how
// expansion places an edge: under the producer a parameter binding moved it to
// (boundAt), under the producer of the local it is called on (claimed), or in
// the function's own body under the call site.
func (r *returnResolver) parentPath(e *metadata.CallGraphEdge, site string, hops int) ([]string, bool) {
	if site != "" {
		if b, ok := r.boundTo[site][e]; ok {
			p, ok := r.argPlace(b.arg, b.edge, hops+1)
			if !ok {
				return []string{b.key}, true
			}
			return append([]string{p.key}, p.path...), true
		}
	}
	if rel, claimed := r.claimedBy[e]; claimed {
		if rel == nil {
			return nil, false
		}
		h, ok := r.holder(rel.Edge, rel.Assignment.ReturnIndex, site, hops+1)
		if !ok {
			return nil, false
		}
		return append([]string{h.key}, h.path...), true
	}
	if site == "" {
		return nil, true
	}
	return []string{site}, true
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
	}
}

// argPlace resolves the value passed as arg at the call edge, following it
// through a helper's return when it was produced by one, else to the producer
// argProducerKey names.
func (r *returnResolver) argPlace(arg metadata.CallArgument, edge *metadata.CallGraphEdge, hops int) (returnPlace, bool) {
	if hops > maxReturnHops {
		return returnPlace{}, false
	}
	switch arg.GetKind() {
	case metadata.KindIdent:
		// A local of the calling function first, read from its own assignment:
		// the origin trace below steps INTO a helper's return and, for a
		// returned call (`return r.Group("/x")`), comes out at the helper's
		// receiver — the router the group was made from, not the group.
		if rels := r.localRels[arg.GetName()+"\x00"+edge.Caller.BaseID()]; len(rels) > 0 && sameProducer(rels) {
			if p, ok := r.returned(rels[0].Edge, rels[0].Assignment.ReturnIndex, "", hops+1); ok {
				return p, true
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
			if p, ok := r.returned(rel.Edge, rel.Assignment.ReturnIndex, "", hops+1); ok {
				return p, true
			}
		}
		return returnPlace{key: key}, true
	case metadata.KindCall:
		if arg.Fun == nil {
			return returnPlace{}, false
		}
		if arg.Edge != nil {
			if p, ok := r.returned(arg.Edge, 0, "", hops+1); ok {
				return p, true
			}
		}
		return returnPlace{key: arg.ID()}, true
	}
	return returnPlace{}, false
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
// variable or the same call. Two different values (a branch returning another
// group) leave the result ambiguous, and golden rule #7 says not to pick one.
func (t *LazyTree) returnValue(pe *metadata.CallGraphEdge, idx int) (metadata.CallArgument, bool) {
	vars, returns := t.calleeReturns(pe)
	candidates := returns
	if len(candidates) == 0 && len(vars) > 0 {
		candidates = [][]metadata.CallArgument{vars}
	}
	var picked *metadata.CallArgument
	for i := range candidates {
		if idx < 0 || idx >= len(candidates[i]) {
			continue
		}
		v := &candidates[i][idx]
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
	return *picked, true
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

// calleeReturns returns the recorded return values of pe's callee, a method or
// a plain function. A callee outside the analysed packages has none, which
// keeps framework calls (`r.Group`) as producers rather than descending into
// their internals.
func (t *LazyTree) calleeReturns(pe *metadata.CallGraphEdge) ([]metadata.CallArgument, [][]metadata.CallArgument) {
	meta := t.meta
	name := getString(meta, pe.Callee.Name)
	pkg := getString(meta, pe.Callee.Pkg)
	if name == "" {
		return nil, nil
	}
	if recv := getString(meta, pe.Callee.RecvType); recv != "" {
		typ := typeByName(pkg, bareTypeName(recv), meta)
		if typ == nil {
			return nil, nil
		}
		for i := range typ.Methods {
			if getString(meta, typ.Methods[i].Name) == name {
				return typ.Methods[i].ReturnVars, typ.Methods[i].Returns
			}
		}
		return nil, nil
	}
	if fn := meta.FunctionInPackage(pkg, name); fn != nil {
		return fn.ReturnVars, fn.Returns
	}
	return nil, nil
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
