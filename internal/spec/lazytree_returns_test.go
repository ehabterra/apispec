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
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/ehabterra/apispec/internal/metadata"
)

// returnedGroupSrc is the shape of issue #633 over a router declared in the
// package itself, so the relations are built from real metadata facts. The
// router is an interface, as a framework's methods are bodiless to the
// analysis: the Group call is the producer, with nothing inside it to follow.
const returnedGroupSrc = `package main

type Router interface {
	Group(p string) Router
	GET(p string)
}

func New() Router { return nil }

func V1(r Router) Router { return r.Group("/v1") }

func Version(parent Router) Router { return parent.Group("/v") }

func Pick(r Router, alt bool) Router {
	if alt {
		return r.Group("/alt")
	}
	return r.Group("/main")
}

func main() {
	r := New()
	v1 := V1(r)
	v1.GET("/same")
	pub := r.Group("/pub")
	adm := r.Group("/adm")
	a := Version(pub)
	a.GET("/a")
	b := Version(adm)
	b.GET("/b")
	p := Pick(r, true)
	p.GET("/pick")
}
`

// TestLazyTreeReturnedProducerBindsPerInvocation covers issue #633 at the
// relations layer: a variable assigned from a helper that returns a group is
// placed under the group call inside the helper — and only under the copy of it
// that belongs to the claiming invocation.
func TestLazyTreeReturnedProducerBindsPerInvocation(t *testing.T) {
	groupsAbove := getGroupLines(t, returnedGroupSrc)

	const (
		lineV1Group      = 10 // return r.Group("/v1")
		lineVersionGroup = 12 // return parent.Group("/v")
		linePub          = 25 // pub := r.Group("/pub")
		lineAdm          = 26 // adm := r.Group("/adm")
	)
	want := map[string][]int{
		`"/same"`: {lineV1Group},
		`"/a"`:    {lineVersionGroup, linePub},
		`"/b"`:    {lineVersionGroup, lineAdm},
		// Two different groups returned: neither is the producer (rule #7).
		`"/pick"`: nil,
	}
	for arg, chain := range want {
		got := groupsAbove[arg]
		if len(got) == 0 {
			t.Errorf("GET(%s) is not in the tree", arg)
			continue
		}
		for _, lines := range got {
			if !slices.Equal(lines, chain) {
				t.Errorf("GET(%s) sits under groups at lines %v, want %v — every copy has to be beneath its own invocation's group", arg, lines, chain)
			}
		}
	}
}

// buildReturnsTree type-checks the single-file package src and builds its
// lazy tree.
func buildReturnsTree(t *testing.T, src string) (*LazyTree, *metadata.Metadata) {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "main.go", src, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	info := &types.Info{
		Types: map[ast.Expr]types.TypeAndValue{},
		Defs:  map[*ast.Ident]types.Object{},
		Uses:  map[*ast.Ident]types.Object{},
	}
	if _, err := (&types.Config{}).Check("main", fset, []*ast.File{file}, info); err != nil {
		t.Fatalf("typecheck: %v", err)
	}
	meta := metadata.GenerateMetadata(
		map[string]map[string]*ast.File{"main": {"main.go": file}},
		map[*ast.File]*types.Info{file: info}, map[string]string{"main": "main"}, fset,
	)
	tree := NewLazyTree(meta, metadata.TrackerLimits{
		MaxNodesPerTree: 10000, MaxChildrenPerNode: 1000, MaxArgsPerFunction: 100,
		MaxNestedArgsDepth: 10, MaxRecursionDepth: 10,
	})
	return tree, meta
}

// getGroupLines builds a lazy tree over src and returns, for every GET node
// keyed by its path argument, the lines of the Group calls above each copy.
func getGroupLines(t *testing.T, src string) map[string][][]int {
	t.Helper()
	tree, meta := buildReturnsTree(t, src)
	groupsAbove := map[string][][]int{}
	var walk func(n TrackerNodeInterface, depth int)
	walk = func(n TrackerNodeInterface, depth int) {
		if depth > 20 {
			return
		}
		if e := n.GetEdge(); e != nil && n.GetArgument() == nil && getString(meta, e.Callee.Name) == "GET" {
			var lines []int
			for p := n.GetParent(); p != nil; p = p.GetParent() {
				if pe := p.GetEdge(); pe != nil && p.GetArgument() == nil && getString(meta, pe.Callee.Name) == "Group" {
					lines = append(lines, lineOf(t, getString(meta, pe.Position)))
				}
			}
			arg := ""
			if len(e.Args) > 0 {
				arg = e.Args[0].GetValue()
			}
			groupsAbove[arg] = append(groupsAbove[arg], lines)
		}
		for _, c := range n.GetChildren() {
			walk(c, depth+1)
		}
	}
	for _, root := range tree.GetRoots() {
		walk(root, 0)
	}
	return groupsAbove
}

// lineOf reads the line out of a recorded "file:line:col" position.
func lineOf(t *testing.T, pos string) int {
	t.Helper()
	parts := strings.Split(pos, ":")
	if len(parts) < 3 {
		t.Fatalf("unexpected position %q", pos)
	}
	line, err := strconv.Atoi(parts[len(parts)-2])
	if err != nil {
		t.Fatalf("unexpected position %q: %v", pos, err)
	}
	return line
}

// returnedEdgeSrc holds helper shapes around issue #633's: ones whose returned
// value has no single producer to follow, which must leave the caller's routes
// under the helper call as before, and a method helper, which must be followed
// like a function.
const returnedEdgeSrc = `package main

type Mux struct{}

func (m *Mux) Group(p string) *Mux { return &Mux{} }

func (m *Mux) GET(p string) {}

func New() *Mux { return &Mux{} }

func Named(r *Mux) (g *Mux) {
	g = r.Group("/named")
	return
}

func Reassigned(r *Mux, alt bool) *Mux {
	g := r.Group("/x")
	if alt {
		g = r.Group("/y")
	}
	return g
}

func Mixed(r *Mux, alt bool) *Mux {
	if alt {
		return r
	}
	return r.Group("/m")
}

func TwoLocals(r *Mux, alt bool) *Mux {
	a := r.Group("/a")
	b := r.Group("/b")
	if alt {
		return a
	}
	return b
}

func Rec(r *Mux) *Mux { return Rec(r) }

func Nil(r *Mux) *Mux { return nil }

func V1(r *Mux) *Mux { return r.Group("/v1") }

func register(g *Mux) { g.GET("/reg") }

func Inner(r *Mux, alt bool) *Mux {
	g := r.Group("/p")
	if alt {
		g = r.Group("/q")
	}
	return g.Group("/inner")
}

func Via(f func(*Mux) *Mux, r *Mux) *Mux { return f(r) }

func id(g *Mux) *Mux { return g }

func Arg(r *Mux) *Mux { return id(r.Group("/arg")) }

func Fresh(r *Mux) *Mux { return new(Mux) }

func pass(g *Mux) { regPassed(g) }

func regPassed(g *Mux) { g.GET("/passed") }

type API struct{}

func (API) Mount(r *Mux) *Mux { return r.Group("/api") }

func main() {
	r := New()
	n := Named(r)
	n.GET("/named")
	x := Reassigned(r, true)
	x.GET("/reassigned")
	m := Mixed(r, true)
	m.GET("/mixed")
	tl := TwoLocals(r, true)
	tl.GET("/twolocals")
	rc := Rec(r)
	rc.GET("/rec")
	rc2 := Rec(r)
	rc2.GET("/rec2")
	vi := Via(V1, r)
	vi.GET("/via")
	ar := Arg(r)
	ar.GET("/arg")
	fr := Fresh(r)
	fr.GET("/fresh")
	ps := V1(r)
	pass(ps)
	nl := Nil(r)
	nl.GET("/nil")
	register(V1(r))
	in := Inner(r, true)
	in.GET("/inner")
	api := API{}.Mount(r)
	api.GET("/api")
}
`

// TestLazyTreeReturnedProducerEdgeShapes pins the shapes a returned value is
// not followed through. Each route must stay in the tree: a binding to a node
// that never materialises (the recursive call inside Rec) drops it.
func TestLazyTreeReturnedProducerEdgeShapes(t *testing.T) {
	groupsAbove := getGroupLines(t, returnedEdgeSrc)
	const (
		lineInnerGroup = 53 // return g.Group("/inner")
		lineAPIGroup   = 70 // func (API) Mount(r *Mux) *Mux { return r.Group("/api") }
		lineV1Group    = 44 // func V1(r *Mux) *Mux { return r.Group("/v1") }
	)
	want := map[string][]int{
		// Different values on different paths: no one producer (rule #7).
		`"/reassigned"`: nil,
		`"/mixed"`:      nil,
		`"/twolocals"`:  nil,
		// The returned group is followed; its own receiver is reassigned, and
		// the tree keeps that call in Inner's body, so the binding does too.
		`"/inner"`: {lineInnerGroup},
		`"/rec"`:   nil,
		`"/rec2"`:  nil, // the second invocation reads the memoised recursion
		`"/nil"`:   nil,
		// A function value has no one callee whose return could be read, and
		// a builtin's result is no call to follow.
		`"/via"`:   nil,
		`"/fresh"`: nil,
		`"/api"`:   {lineAPIGroup},
		// Change detectors: a bare return of a named result, a helper call
		// passed inline, and a group passed inline through a helper returning
		// its parameter are not followed yet, so their routes keep the
		// helper's call as their parent, unprefixed.
		`"/named"`: nil,
		`"/reg"`:   nil,
		`"/arg"`:   nil,
		// A returned group passed on through two helpers: pass forwards its
		// parameter, and its one call site says which group that is.
		`"/passed"`: {lineV1Group},
	}
	for arg, chain := range want {
		got := groupsAbove[arg]
		if len(got) == 0 {
			t.Errorf("GET(%s) is not in the tree", arg)
			continue
		}
		for _, lines := range got {
			if !slices.Equal(lines, chain) {
				t.Errorf("GET(%s) sits under groups at lines %v, want %v", arg, lines, chain)
			}
		}
	}
}

// TestReturnResolverGuards covers the inputs a returned value cannot be
// followed from: each must answer "no producer" rather than a guess, so the
// caller's edges stay under the helper's own call.
func TestReturnResolverGuards(t *testing.T) {
	tree, meta := buildReturnsTree(t, returnedEdgeSrc)
	newResolver := func() *returnResolver {
		return &returnResolver{
			t:             tree,
			producerByVar: map[recvKey]string{},
			relsByVar:     map[recvKey][]*metadata.AssignmentLink{},
			localRels:     map[string][]*metadata.AssignmentLink{},
			claimedBy:     map[*metadata.CallGraphEdge]*metadata.AssignmentLink{},
			boundTo:       map[string]map[*metadata.CallGraphEdge]paramBind{},
			moves:         map[*metadata.CallGraphEdge]int{},
			siteEdge:      map[string]*metadata.CallGraphEdge{},
			params:        map[string]paramMemo{},
			memo:          map[returnMemoKey]returnMemo{},
			visiting:      map[returnMemoKey]bool{},
		}
	}
	res := newResolver()
	edgeOf := func(caller, callee string) *metadata.CallGraphEdge {
		t.Helper()
		for i := range meta.CallGraph {
			e := &meta.CallGraph[i]
			if getString(meta, e.Caller.Name) == caller && getString(meta, e.Callee.Name) == callee {
				return e
			}
		}
		t.Fatalf("no call from %s to %s in the call graph", caller, callee)
		return nil
	}
	v1 := edgeOf("main", "V1")    // v1 := V1(r) is not in main; register(V1(r)) is
	inV1 := edgeOf("V1", "Group") // return r.Group("/v1")
	site := strings.TrimPrefix(v1.Callee.ID(), "*")
	inV1Key := strings.TrimPrefix(inV1.Callee.ID(), "*")

	if _, ok := res.returned(nil, 0, "", 0); ok {
		t.Error("returned(nil): want no producer")
	}
	if _, ok := res.returned(v1, 0, "", maxReturnHops+1); ok {
		t.Error("returned past the hop bound: want no producer")
	}
	if _, ok := res.returned(v1, 1, "", 0); ok {
		t.Error("returned at an index V1 does not return: want no producer")
	}
	if _, ok := res.argPlace(*v1.Args[0], v1, maxReturnHops+1); ok {
		t.Error("argPlace past the hop bound: want no producer")
	}
	bareCall := metadata.NewCallArgument(meta)
	bareCall.SetKind(metadata.KindCall)
	if _, ok := res.argPlace(*bareCall, v1, 0); ok {
		t.Error("argPlace of a call with no function: want no producer")
	}

	a := &metadata.AssignmentLink{Edge: v1, Assignment: &metadata.Assignment{}}
	b := &metadata.AssignmentLink{Edge: v1, Assignment: &metadata.Assignment{ReturnIndex: 1}}
	if sameProducer([]*metadata.AssignmentLink{a, b}) {
		t.Error("sameProducer: two result indexes of one call are two producers")
	}
	res.relsByVar[recvKey{name: "x"}] = []*metadata.AssignmentLink{a, b}
	if res.producerRel(recvKey{name: "x"}) != nil {
		t.Error("producerRel: want none for a variable with two producers")
	}
	if res.producerRel(recvKey{name: "missing"}) != nil {
		t.Error("producerRel: want none for an unassigned variable")
	}

	if body := tree.calleeBody(&metadata.CallGraphEdge{}); body.returns != nil {
		t.Error("calleeBody of an unnamed callee: want no body")
	}
	unknownRecv := *v1
	unknownRecv.Callee.RecvType = meta.StringPool.Get("Unknown")
	if body := tree.calleeBody(&unknownRecv); body.returns != nil {
		t.Error("calleeBody of a method on an unknown type: want no body")
	}
	if tree.calleeBodyEdge(v1, *bareCall) != nil {
		t.Error("calleeBodyEdge of an expression with no name or position: want none")
	}

	unknownMethod := *v1
	unknownMethod.Callee.RecvType = meta.StringPool.Get("Mux")
	if body := tree.calleeBody(&unknownMethod); body.returns != nil {
		t.Error("calleeBody of a method the type does not declare: want no body")
	}

	// Sanity: unobstructed, V1's returned group is found, beneath V1's call.
	if p, ok := newResolver().returned(v1, 0, "", 0); !ok || p.key != inV1Key {
		t.Fatalf("returned(V1) = %+v, %v; want the Group call inside V1", p, ok)
	}

	// A call whose variable is reassigned has no one parent: neither the
	// group it makes nor anything returned through it has a place.
	res = newResolver()
	res.claimedBy[inV1] = nil
	if _, ok := res.parentPath(inV1, site, "", 0); ok {
		t.Error("parentPath of a call on a reassigned variable: want none")
	}
	if _, ok := res.holder(inV1, 0, site, "", 0); ok {
		t.Error("holder of a call on a reassigned variable: want none")
	}
	res = newResolver()
	res.claimedBy[v1] = nil
	if _, ok := res.returned(v1, 0, "", 0); ok {
		t.Error("returned through a helper call with no one parent: want none")
	}

	// A call materialised as an argument node is never the producing callee
	// node, so neither it nor a call on the variable it produced has a place.
	saved := tree.argInstanceIDs
	tree.argInstanceIDs = map[string]bool{inV1Key: true}
	res = newResolver()
	if _, ok := res.holder(inV1, 0, site, "", 0); ok {
		t.Error("holder of an argument-node call: want none")
	}
	res.claimedBy[v1] = &metadata.AssignmentLink{Edge: inV1, Assignment: &metadata.Assignment{}}
	if _, ok := res.parentPath(v1, site, "", 0); ok {
		t.Error("parentPath under an argument-node producer: want none")
	}
	tree.argInstanceIDs = saved

	// A parameter binding whose value cannot be followed still places the
	// call under the producer the binding recorded.
	res = newResolver()
	res.bind(site, paramBind{arg: *bareCall, edge: v1, key: "bound"}, []*metadata.CallGraphEdge{inV1})
	if path, ok := res.parentPath(inV1, site, "", 0); !ok || !slices.Equal(path, []string{"bound"}) {
		t.Errorf("parentPath of a bound call = %v, %v; want [bound]", path, ok)
	}
	// Within a known invocation of V1's caller, the recorded producer is every
	// invocation's, not that one's.
	if path, ok := res.parentPath(inV1, site, "outer-site", 0); ok {
		t.Errorf("parentPath of a bound call in a known invocation = %v; want none", path)
	}

	// A helper call passed as an argument is followed into the helper.
	reg := edgeOf("main", "register")
	if p, ok := newResolver().argPlace(*reg.Args[0], reg, 0); !ok || p.key != inV1Key {
		t.Errorf("argPlace(V1(r)) = %+v, %v; want the Group call inside V1", p, ok)
	}

	root, ok := tree.GetRoots()[0].(*LazyNode)
	if !ok {
		t.Fatal("root is not a *LazyNode")
	}
	if root.hasAncestry([]int32{root.key}) {
		t.Error("hasAncestry: a root has no ancestors to match")
	}
}

// TestReturnResolverInvocationGuards covers what keeps one invocation's value
// out of another's: a parameter is answered only when every call site agrees,
// and a call moved under one producer by several call sites is a place only
// while its own arguments are the same in each.
func TestReturnResolverInvocationGuards(t *testing.T) {
	tree, meta := buildReturnsTree(t, returnedEdgeSrc)
	res := &returnResolver{
		t:        tree,
		boundTo:  map[string]map[*metadata.CallGraphEdge]paramBind{},
		moves:    map[*metadata.CallGraphEdge]int{},
		siteEdge: map[string]*metadata.CallGraphEdge{},
		params:   map[string]paramMemo{},
		memo:     map[returnMemoKey]returnMemo{},
		visiting: map[returnMemoKey]bool{},
	}
	var v1, inV1, pass *metadata.CallGraphEdge
	for i := range meta.CallGraph {
		e := &meta.CallGraph[i]
		switch caller, callee := getString(meta, e.Caller.Name), getString(meta, e.Callee.Name); {
		case caller == "main" && callee == "V1" && v1 == nil:
			v1 = e
		case caller == "V1" && callee == "Group":
			inV1 = e
		case caller == "main" && callee == "pass":
			pass = e
		}
	}
	if v1 == nil || inV1 == nil || pass == nil {
		t.Fatal("V1, its Group call or pass is missing from the call graph")
	}

	ident := func(name string) *metadata.CallArgument {
		a := metadata.NewCallArgument(meta)
		a.SetKind(metadata.KindIdent)
		a.SetName(name)
		return a
	}
	lit := metadata.NewCallArgument(meta)
	lit.SetKind(metadata.KindLiteral)
	lit.SetValue(`"/v1"`)
	call := metadata.NewCallArgument(meta)
	call.SetKind(metadata.KindCall)
	call.Fun = ident("join")
	call.Args = []*metadata.CallArgument{lit, ident("r")}
	sel := metadata.NewCallArgument(meta)
	sel.SetKind(metadata.KindSelector)
	sel.X = ident("cfg")
	sel.Sel = ident("Prefix")
	for _, c := range []struct {
		name string
		args []*metadata.CallArgument
		want bool
	}{
		{"a literal", []*metadata.CallArgument{lit}, false},
		{"a package-level name", []*metadata.CallArgument{ident("prefix")}, false},
		{"a field of a package-level name", []*metadata.CallArgument{sel}, false},
		{"the helper's parameter", []*metadata.CallArgument{ident("r")}, true},
		{"a call reading the helper's parameter", []*metadata.CallArgument{call}, true},
	} {
		e := *inV1
		e.Args = c.args
		if got := res.argsVary(&e, v1); got != c.want {
			t.Errorf("argsVary(%s) = %v, want %v", c.name, got, c.want)
		}
	}

	// V1's Group moved under one producer from two call sites: its "/v1" is
	// the same in both, so the shared node is still a place.
	b := paramBind{arg: *v1.Args[0], edge: v1, key: "bound"}
	res.bind("site-a", b, []*metadata.CallGraphEdge{inV1})
	res.bind("site-b", b, []*metadata.CallGraphEdge{inV1})
	if _, ok := res.parentPath(inV1, "site-a", "", 0); !ok {
		t.Error("parentPath of a shared move with constant arguments: want a place")
	}
	varying := *inV1
	varying.Args = []*metadata.CallArgument{ident("r")}
	res.bind("site-a", b, []*metadata.CallGraphEdge{&varying})
	res.bind("site-b", b, []*metadata.CallGraphEdge{&varying})
	if _, ok := res.parentPath(&varying, "site-a", "", 0); ok {
		t.Error("parentPath of a shared move whose argument is a parameter: want none")
	}
	// Moved at two call sites under different producer keys (`pub` and
	// `Identity(pub)` name one node): counted per call, so still shared.
	split := *inV1
	split.Args = []*metadata.CallArgument{ident("r")}
	other := b
	other.key = "other"
	res.bind("site-a", b, []*metadata.CallGraphEdge{&split})
	res.bind("site-b", other, []*metadata.CallGraphEdge{&split})
	if _, ok := res.parentPath(&split, "site-a", "", 0); ok {
		t.Error("parentPath of a varying move made under two producer keys: want none")
	}

	if _, _, isParam := res.paramPlace("g", &metadata.Call{}, 0); isParam {
		t.Error("paramPlace of a function nothing calls: want not a parameter")
	}
	if _, _, isParam := res.paramPlace("nope", &pass.Callee, 0); isParam {
		t.Error("paramPlace of a name that is not a parameter: want not a parameter")
	}
	// Reached again while its own resolution is in progress (a parameter
	// forwarded through a recursive call): no producer along that path.
	res.params["g\x00"+pass.Callee.BaseID()] = paramMemo{}
	if _, ok, isParam := res.paramPlace("g", &pass.Callee, 0); ok || !isParam {
		t.Errorf("paramPlace in progress = %v, %v; want no producer for a parameter", ok, isParam)
	}

	for _, c := range []struct {
		a, b returnPlace
		want bool
	}{
		{returnPlace{key: "k", path: []string{"p", "q"}}, returnPlace{key: "k", path: []string{"p", "q"}}, true},
		{returnPlace{key: "k", path: []string{"p", "q"}}, returnPlace{key: "k", path: []string{"p", "r"}}, false},
		{returnPlace{key: "k", path: []string{"p"}}, returnPlace{key: "k"}, false},
		{returnPlace{param: "g"}, returnPlace{param: "h"}, false},
	} {
		if got := samePlace(c.a, c.b); got != c.want {
			t.Errorf("samePlace(%+v, %+v) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

// TestReturnResolverPendingBindings covers the bindings still to be resolved:
// they count as moves and leave their call site undecided until they are bound
// or dropped, and bindings that only wait on each other are left unbound.
func TestReturnResolverPendingBindings(t *testing.T) {
	tree, meta := buildReturnsTree(t, returnedEdgeSrc)
	res := &returnResolver{
		t:        tree,
		boundTo:  map[string]map[*metadata.CallGraphEdge]paramBind{},
		moves:    map[*metadata.CallGraphEdge]int{},
		siteEdge: map[string]*metadata.CallGraphEdge{},
		params:   map[string]paramMemo{},
		memo:     map[returnMemoKey]returnMemo{},
		visiting: map[returnMemoKey]bool{},
	}
	var v1, inV1 *metadata.CallGraphEdge
	for i := range meta.CallGraph {
		e := &meta.CallGraph[i]
		switch caller, callee := getString(meta, e.Caller.Name), getString(meta, e.Callee.Name); {
		case caller == "main" && callee == "V1" && v1 == nil:
			v1 = e
		case caller == "V1" && callee == "Group":
			inV1 = e
		}
	}
	if v1 == nil || inV1 == nil {
		t.Fatal("V1 or its Group call is missing from the call graph")
	}
	param := metadata.NewCallArgument(meta)
	param.SetKind(metadata.KindIdent)
	param.SetName("r")

	// One call site has moved a Group whose argument varies; another may yet.
	varying := *inV1
	varying.Args = []*metadata.CallArgument{param}
	edges := []*metadata.CallGraphEdge{&varying}
	res.bind("site-a", paramBind{arg: *v1.Args[0], edge: v1, key: "bound"}, edges)
	res.pend("site-b", edges, 1)
	before := res.defers
	if _, ok := res.parentPath(&varying, "site-a", "", 0); ok || res.defers == before {
		t.Error("parentPath with a pending move of the same call: want none, deferred")
	}
	if _, ok := res.parentPath(&varying, "site-b", "", 0); ok {
		t.Error("parentPath at a call site whose binding is pending: want none")
	}
	res.pend("site-b", edges, -1)
	if _, ok := res.parentPath(&varying, "site-a", "", 0); !ok {
		t.Error("parentPath once the pending binding is dropped: want a place")
	}
	if p, ok := res.parentPath(&varying, "site-b", "", 0); !ok || len(p) != 1 || p[0] != "site-b" {
		t.Errorf("parentPath at a call site that binds nothing = %v, %v; want its own call", p, ok)
	}

	// A binding whose argument is V1's result, while that same call site's
	// move of V1's Group is pending: it waits on itself and is left unbound.
	site := strings.TrimPrefix(v1.Callee.ID(), "*")
	call := metadata.NewCallArgument(meta)
	call.SetKind(metadata.KindCall)
	call.Fun = v1.Args[0]
	call.Edge = v1
	waiting := paramClaim{bind: paramBind{arg: *call, edge: v1}, site: site, edges: []*metadata.CallGraphEdge{inV1}}
	if got := res.resolvePending([]paramClaim{waiting}); len(got) != 0 {
		t.Errorf("resolvePending of a binding waiting on itself = %d bound, want 0", len(got))
	}
	if n := res.pending[inV1]; n != 0 {
		t.Errorf("pending after resolvePending = %d, want 0", n)
	}
	if _, bound := res.boundTo[site][inV1]; bound {
		t.Error("resolvePending bound a binding it could not resolve")
	}
}
