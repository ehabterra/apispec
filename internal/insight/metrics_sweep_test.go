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

package insight

import (
	"fmt"
	"strings"
	"testing"

	"github.com/ehabterra/apispec/internal/metadata"
	"github.com/ehabterra/apispec/internal/spec"
)

// sweepMeta builds an empty metadata with a fresh string pool, plus a call
// factory that always sets Meta so BaseID/GetString work on synthetic calls.
func sweepMeta() (*metadata.Metadata, func(pkg, recv, name, pos string) metadata.Call) {
	meta := &metadata.Metadata{
		StringPool: metadata.NewStringPool(),
		Callers:    map[string][]*metadata.CallGraphEdge{},
		Packages:   map[string]*metadata.Package{},
	}
	mk := func(pkg, recv, name, pos string) metadata.Call {
		return metadata.Call{
			Meta:     meta,
			Pkg:      meta.StringPool.Get(pkg),
			RecvType: meta.StringPool.Get(recv),
			Name:     meta.StringPool.Get(name),
			Position: meta.StringPool.Get(pos),
		}
	}
	return meta, mk
}

func TestCalleeIsBuiltinAndClassifyPkg(t *testing.T) {
	meta, mk := sweepMeta()
	meta.CurrentModulePath = "example.com/app"

	builtin := mk("", "", "append", "")
	if !calleeIsBuiltin(meta, &builtin) {
		t.Error("append should be reported as builtin")
	}
	// Stdlib calls are NOT builtins: they stay in the trace as leaves so the
	// handler subtree (and its metrics) is complete.
	stdlib := mk("net/url", "Values", "Get", "")
	if calleeIsBuiltin(meta, &stdlib) {
		t.Error("net/url Values.Get must not be filtered as builtin")
	}

	cases := []struct {
		pkg, recv, want string
	}{
		{"whatever", "error", "standard"},      // err.Error() on the builtin error interface
		{"", "", "standard"},                   // universe scope
		{"example.com/app", "", "project"},     // module root
		{"example.com/app/sub", "", "project"}, // module subpackage
		{"fmt", "", "standard"},                // stdlib, no dot in first segment
		{"net/http", "", "standard"},           // stdlib with slash
		{"github.com/gin-gonic/gin", "", "library"} /* third-party */}
	for _, c := range cases {
		if got := classifyPkg(meta, c.pkg, c.recv); got != c.want {
			t.Errorf("classifyPkg(%q, %q) = %q, want %q", c.pkg, c.recv, got, c.want)
		}
	}
}

// TestAnalyzeAdjacency_EdgeCases drives the shared BFS core through its
// self-edge skip, chain-depth tracking, nil/pointer/value argument counting
// and the depth-truncation guard.
func TestAnalyzeAdjacency_EdgeCases(t *testing.T) {
	meta, mk := sweepMeta()
	pool := meta.StringPool
	const pkg = "example.com/app"
	const root = pkg + ".H.Do"

	rootCall := mk(pkg, "H", "Do", "/app/h.go:5:1")
	selfEdge := &metadata.CallGraphEdge{Caller: rootCall, Callee: mk(pkg, "H", "Do", "/app/h.go:6:2")}
	ptrArg := &metadata.CallArgument{Meta: meta, Type: pool.Get("*Foo")}
	valArg := &metadata.CallArgument{Meta: meta, Type: pool.Get("Bar")}
	toA := &metadata.CallGraphEdge{
		Caller:     rootCall,
		Callee:     mk(pkg, "S", "A", "/app/s.go:10:3"),
		ChainDepth: 3,
		Args:       []*metadata.CallArgument{nil, ptrArg, valArg},
	}
	meta.Callers[root] = []*metadata.CallGraphEdge{selfEdge, toA}

	// A long chain under A trips the maxTraceDepth guard.
	prev := "S"
	prevName := "A"
	for i := 0; i < 20; i++ {
		recv := fmt.Sprintf("C%d", i)
		key := pkg + "." + prev + "." + prevName
		meta.Callers[key] = []*metadata.CallGraphEdge{{
			Caller: mk(pkg, prev, prevName, ""),
			Callee: mk(pkg, recv, "Run", ""),
		}}
		prev, prevName = recv, "Run"
	}

	m, tg := analyzeFromHandler(meta, root)
	if !m.DepthTruncated {
		t.Error("expected DepthTruncated for a >maxTraceDepth chain")
	}
	if m.ChainDepth != 3 {
		t.Errorf("ChainDepth = %d, want 3", m.ChainDepth)
	}
	if m.PointerArgs != 1 || m.ValueArgs != 1 {
		t.Errorf("args = ptr %d / val %d, want 1 / 1", m.PointerArgs, m.ValueArgs)
	}
	for _, n := range tg.Nodes {
		if n.ID == root && n.Kind != "handler" {
			t.Errorf("root kind = %q", n.Kind)
		}
	}
}

// fanOutMeta builds a root with a huge fan-out (601 leaves plus a "Zzz"
// callee that has its own callee), enough to trip the maxReachable cap and
// the trace-graph node budget.
func fanOutMeta(t *testing.T) (*metadata.Metadata, string) {
	t.Helper()
	meta, mk := sweepMeta()
	const pkg = "example.com/app"
	root := pkg + ".H.Do"
	rootCall := mk(pkg, "H", "Do", "")

	edges := []*metadata.CallGraphEdge{{Caller: rootCall, Callee: mk(pkg, "", "Zzz", "")}}
	for i := 0; i <= 600; i++ {
		edges = append(edges, &metadata.CallGraphEdge{
			Caller: rootCall,
			Callee: mk(pkg, "", fmt.Sprintf("F%03d", i), ""),
		})
	}
	meta.Callers[root] = edges
	// Zzz has a callee, so adj gains a source that the 60-node trace graph
	// excludes (it sorts after every Fxxx id).
	meta.Callers[pkg+".Zzz"] = []*metadata.CallGraphEdge{{
		Caller: mk(pkg, "", "Zzz", ""),
		Callee: mk(pkg, "", "Sub", ""),
	}}
	return meta, root
}

func TestAnalyze_ReachableAndGraphCaps(t *testing.T) {
	meta, root := fanOutMeta(t)

	m, tg := analyzeFromHandler(meta, root)
	if !m.DepthTruncated {
		t.Error("adjacency: expected DepthTruncated once maxReachable is exceeded")
	}
	if !tg.Truncated || len(tg.Nodes) != maxGraphNodes {
		t.Errorf("adjacency: trace graph = %d nodes truncated=%v, want %d/true", len(tg.Nodes), tg.Truncated, maxGraphNodes)
	}

	m2, tg2, ok := analyzeResolvedCallGraph(meta, root)
	if !ok || !m2.DepthTruncated {
		t.Errorf("resolved: ok=%v truncated=%v, want true/true", ok, m2.DepthTruncated)
	}
	if !tg2.Truncated {
		t.Error("resolved: trace graph should be truncated")
	}
}

// TestAnalyzeResolvedCallGraph_EdgeCases exercises the resolved-call-graph
// fallback: missing root, self edges, diamond revisits, chain depth, args,
// deep-chain truncation and interface→implementation resolution.
func TestAnalyzeResolvedCallGraph_EdgeCases(t *testing.T) {
	meta, mk := sweepMeta()
	pool := meta.StringPool
	const pkg = "example.com/app"
	const root = pkg + ".H.Do"

	if _, _, ok := analyzeResolvedCallGraph(meta, "no.such.Root"); ok {
		t.Fatal("unknown root must report ok=false")
	}

	rootCall := mk(pkg, "H", "Do", "/app/h.go:5:1")
	ptrArg := &metadata.CallArgument{Meta: meta, Type: pool.Get("*Foo")}
	valArg := &metadata.CallArgument{Meta: meta, Type: pool.Get("Bar")}
	meta.Callers[root] = []*metadata.CallGraphEdge{
		{Caller: rootCall, Callee: mk(pkg, "H", "Do", "/app/h.go:6:2")}, // self edge → skipped
		{Caller: rootCall, Callee: mk(pkg, "S", "A", "/app/s.go:10:3"), ChainDepth: 2,
			Args: []*metadata.CallArgument{nil, ptrArg, valArg}},
		{Caller: rootCall, Callee: mk(pkg, "", "D1", "")},
		{Caller: rootCall, Callee: mk(pkg, "", "D2", "")},
		{Caller: rootCall, Callee: mk(pkg, "UseCase", "Check", "/app/h.go:9:3")},
	}
	// Diamond: D1 and D2 both call DX, so the second enqueue sees it visited.
	for _, d := range []string{"D1", "D2"} {
		meta.Callers[pkg+"."+d] = []*metadata.CallGraphEdge{{
			Caller: mk(pkg, "", d, ""),
			Callee: mk(pkg, "", "DX", ""),
		}}
	}
	// Deep chain under S.A trips the depth cap.
	prev, prevName := "S", "A"
	for i := 0; i < 20; i++ {
		recv := fmt.Sprintf("C%d", i)
		meta.Callers[pkg+"."+prev+"."+prevName] = []*metadata.CallGraphEdge{{
			Caller: mk(pkg, prev, prevName, ""),
			Callee: mk(pkg, recv, "Run", ""),
		}}
		prev, prevName = recv, "Run"
	}
	// UseCase.Check has no body but is an interface method with a concrete impl.
	meta.Packages[pkg] = &metadata.Package{Files: map[string]*metadata.File{
		"h.go": {Types: map[string]*metadata.Type{
			"UseCase": {
				Kind:          pool.Get("interface"),
				ImplementedBy: []int{pool.Get("example.com/impl.Pay")},
			},
		}},
	}}
	meta.Callers["example.com/impl.Pay.Check"] = []*metadata.CallGraphEdge{{
		Caller: mk("example.com/impl", "Pay", "Check", "/impl/p.go:30:1"),
		Callee: mk("example.com/impl", "Repo", "Load", "/impl/p.go:31:2"),
	}}

	m, tg, ok := analyzeResolvedCallGraph(meta, root)
	if !ok {
		t.Fatal("analyzeResolvedCallGraph should succeed")
	}
	if !m.DepthTruncated {
		t.Error("expected DepthTruncated for the deep chain")
	}
	if m.ChainDepth != 2 || m.PointerArgs != 1 || m.ValueArgs != 1 {
		t.Errorf("chain=%d ptr=%d val=%d, want 2/1/1", m.ChainDepth, m.PointerArgs, m.ValueArgs)
	}
	found := false
	for _, n := range tg.Nodes {
		if n.ID == "example.com/impl.Pay.Check" {
			found = true
			if !n.Resolved {
				t.Error("concrete impl should carry the Resolved badge")
			}
		}
	}
	if !found {
		t.Error("interface method did not resolve to its concrete implementation")
	}
}

// fakeTrackerNode is a minimal spec.TrackerNodeInterface for driving
// analyzeTrackerSubtree without building a real tracker tree.
type fakeTrackerNode struct {
	key      string
	edge     *metadata.CallGraphEdge
	children []spec.TrackerNodeInterface
}

func (f *fakeTrackerNode) GetKey() string                           { return f.key }
func (f *fakeTrackerNode) GetParent() spec.TrackerNodeInterface     { return nil }
func (f *fakeTrackerNode) GetChildren() []spec.TrackerNodeInterface { return f.children }
func (f *fakeTrackerNode) GetEdge() *metadata.CallGraphEdge         { return f.edge }
func (f *fakeTrackerNode) GetArgument() *metadata.CallArgument      { return nil }
func (f *fakeTrackerNode) GetTypeParamMap() map[string]string       { return nil }

func TestAnalyzeTrackerSubtree(t *testing.T) {
	meta, mk := sweepMeta()
	pool := meta.StringPool
	const pkg = "example.com/app"
	rootCall := mk(pkg, "H", "Do", "/app/h.go:5:1")

	t.Run("no children → no handler body", func(t *testing.T) {
		if _, _, ok := analyzeTrackerSubtree(meta, &fakeTrackerNode{}); ok {
			t.Error("empty route node should report ok=false")
		}
	})

	t.Run("only builtin callees → no adjacency", func(t *testing.T) {
		route := &fakeTrackerNode{children: []spec.TrackerNodeInterface{
			&fakeTrackerNode{edge: &metadata.CallGraphEdge{
				Caller: rootCall,
				Callee: mk("", "", "len", ""),
			}},
		}}
		if _, _, ok := analyzeTrackerSubtree(meta, route); ok {
			t.Error("builtin-only subtree should report ok=false")
		}
	})

	t.Run("full walk", func(t *testing.T) {
		ptrArg := &metadata.CallArgument{Meta: meta, Type: pool.Get("*Foo")}
		// Deep nested chain to trip the depth guard: level i's edge is
		// C(i-1) → C(i), with the handler calling C0 at the top.
		var deepChild *fakeTrackerNode
		for i := 17; i >= 0; i-- {
			caller := rootCall
			if i > 0 {
				caller = mk(pkg, "", fmt.Sprintf("C%d", i-1), "")
			}
			node := &fakeTrackerNode{edge: &metadata.CallGraphEdge{
				Caller: caller,
				Callee: mk(pkg, "", fmt.Sprintf("C%d", i), ""),
			}}
			if deepChild != nil {
				node.children = []spec.TrackerNodeInterface{deepChild}
			}
			deepChild = node
		}

		// Interface resolution boundary: the child edge's Caller is neither the
		// enclosing function nor the callee → surfaces the concrete impl node.
		resolved := &fakeTrackerNode{edge: &metadata.CallGraphEdge{
			Caller: mk("example.com/impl", "Pay", "Check", "/impl/p.go:30:1"),
			Callee: mk("example.com/impl", "Repo", "Load", "/impl/p.go:31:2"),
		}}
		svc := &fakeTrackerNode{
			edge: &metadata.CallGraphEdge{
				Caller:     rootCall,
				Callee:     mk(pkg, "S", "A", "/app/s.go:10:3"),
				ChainDepth: 2,
				Args:       []*metadata.CallArgument{nil, ptrArg},
			},
			children: []spec.TrackerNodeInterface{resolved},
		}

		// Edge-less wrapper node whose children are re-queued in place.
		wrapper := &fakeTrackerNode{children: []spec.TrackerNodeInterface{
			&fakeTrackerNode{edge: &metadata.CallGraphEdge{Caller: rootCall, Callee: mk(pkg, "S", "B", "")}},
		}}

		// Duplicate keys: the second node is skipped by the visited set.
		dup1 := &fakeTrackerNode{key: "dup", edge: &metadata.CallGraphEdge{Caller: rootCall, Callee: mk(pkg, "S", "C", "")}}
		dup2 := &fakeTrackerNode{key: "dup", edge: &metadata.CallGraphEdge{Caller: rootCall, Callee: mk(pkg, "S", "D", "")}}

		route := &fakeTrackerNode{children: []spec.TrackerNodeInterface{svc, wrapper, dup1, dup2, deepChild}}
		m, tg, ok := analyzeTrackerSubtree(meta, route)
		if !ok {
			t.Fatal("analyzeTrackerSubtree should succeed")
		}
		if !m.DepthTruncated {
			t.Error("expected DepthTruncated for the nested chain")
		}
		if m.ChainDepth != 2 || m.PointerArgs != 1 {
			t.Errorf("chain=%d ptr=%d, want 2/1", m.ChainDepth, m.PointerArgs)
		}
		var sawResolved, sawDupSkip bool
		for _, n := range tg.Nodes {
			if n.ID == "example.com/impl.Pay.Check" && n.Resolved {
				sawResolved = true
			}
			if n.ID == pkg+".S.D" {
				sawDupSkip = true // dup2 shares dup1's key → must NOT appear
			}
		}
		if !sawResolved {
			t.Error("resolution-boundary node missing from the trace")
		}
		if sawDupSkip {
			t.Error("node with an already-visited key should be skipped")
		}
	})

	t.Run("reachable cap", func(t *testing.T) {
		kids := make([]spec.TrackerNodeInterface, 0, 602)
		for i := 0; i <= 601; i++ {
			kids = append(kids, &fakeTrackerNode{edge: &metadata.CallGraphEdge{
				Caller: rootCall,
				Callee: mk(pkg, "", fmt.Sprintf("F%03d", i), ""),
			}})
		}
		m, _, ok := analyzeTrackerSubtree(meta, &fakeTrackerNode{children: kids})
		if !ok || !m.DepthTruncated {
			t.Errorf("ok=%v truncated=%v, want true/true", ok, m.DepthTruncated)
		}
	})
}

func TestCachedTrackerTree_NilMeta(t *testing.T) {
	if cachedTrackerTree(nil) != nil {
		t.Error("nil metadata must yield a nil tree")
	}
}

func TestResolveImplementers_EdgeCases(t *testing.T) {
	meta, mk := sweepMeta()
	pool := meta.StringPool

	empty := mk("", "", "", "")
	if got := resolveImplementers(meta, &empty); got != nil {
		t.Errorf("empty callee should resolve to nil, got %v", got)
	}

	meta.Packages["example.com/api"] = &metadata.Package{Files: map[string]*metadata.File{
		"i.go": {Types: map[string]*metadata.Type{
			"Svc": {
				Kind:          pool.Get("interface"),
				ImplementedBy: []int{pool.Get("nodots"), pool.Get("trailing.")},
			},
		}},
	}}
	callee := mk("example.com/api", "Svc", "Handle", "")
	if got := resolveImplementers(meta, &callee); len(got) != 0 {
		t.Errorf("malformed implementer names should be skipped, got %v", got)
	}
}

func TestEnumeratePaths_Caps(t *testing.T) {
	adj := map[string][]string{"r": {"a", "b"}}
	if out := enumeratePaths(adj, "r", 0); len(out) != 0 {
		t.Errorf("cap 0 should yield no paths, got %v", out)
	}
	if out := enumeratePaths(adj, "r", 1); len(out) != 1 {
		t.Errorf("cap 1 should yield exactly one path, got %v", out)
	}
}

func TestCountPaths_MemoAndTruncation(t *testing.T) {
	// Diamond: D's count is memoised on the B branch and reused on the C branch.
	diamond := map[string][]string{"A": {"B", "C"}, "B": {"D"}, "C": {"D"}, "D": {"E"}}
	if n, tr := countPaths(diamond, "A"); n != 2 || tr {
		t.Errorf("diamond = %d trunc=%v, want 2 false", n, tr)
	}

	// Doubled edges: 2^10 = 1024 paths exceed maxPaths → truncated.
	wide := map[string][]string{}
	for i := 0; i < 10; i++ {
		next := fmt.Sprintf("n%d", i+1)
		wide[fmt.Sprintf("n%d", i)] = []string{next, next}
	}
	if n, tr := countPaths(wide, "n0"); n != maxPaths || !tr {
		t.Errorf("wide = %d trunc=%v, want %d true", n, tr, maxPaths)
	}
}

func TestGrade_MiddleBands(t *testing.T) {
	if g, lb := grade(Metrics{MaxDepth: 6, CallPaths: 30, FanoutMax: 10}); g != "B" || lb {
		t.Errorf("grade = %s lb=%v, want B false", g, lb)
	}
	if g, _ := grade(Metrics{MaxDepth: 9, CallPaths: 100}); g != "C" {
		t.Errorf("grade = %s, want C", g)
	}
}

func TestBuildTraceGraph_NoEdges(t *testing.T) {
	meta, _ := sweepMeta()
	_, tg := analyzeFromHandler(meta, "example.com/app.Lonely")
	if tg.Edges == nil {
		t.Error("Edges must be non-nil for JSON")
	}
	if len(tg.Edges) != 0 || len(tg.Nodes) != 1 {
		t.Errorf("nodes=%d edges=%d, want 1/0", len(tg.Nodes), len(tg.Edges))
	}
}

func TestSortedSites_CallerTieBreak(t *testing.T) {
	sites := map[string]CallSite{
		"k1": {Pos: "/p/a.go:1", Caller: "beta"},
		"k2": {Pos: "/p/a.go:1", Caller: "alpha"},
	}
	out := sortedSites(sites)
	if len(out) != 2 || out[0].Caller != "alpha" || out[1].Caller != "beta" {
		t.Errorf("tie-break by caller failed: %v", out)
	}
}

// --- endpoint.go sweeps ------------------------------------------------------

func TestBuildEndpointWithSource_NilInputs(t *testing.T) {
	rep := BuildEndpointWithSource(nil, nil, nil, "get", "/x", TraceSourceTracker)
	if rep.Found || rep.Method != "GET" || rep.Path != "/x" {
		t.Errorf("nil spec should degrade gracefully: %+v", rep)
	}

	// Nil Callers map triggers the lazy BuildCallGraphMaps branch.
	s := &spec.OpenAPISpec{Paths: map[string]spec.PathItem{
		"/x": {Get: &spec.Operation{OperationID: "example.com/x.Nope", Responses: map[string]spec.Response{"200": {}}}},
	}}
	meta := &metadata.Metadata{StringPool: metadata.NewStringPool()}
	rep = BuildEndpointWithSource(s, meta, nil, "GET", "/x", TraceSourceTracker)
	if !rep.Found || rep.HandlerFound {
		t.Errorf("found=%v handlerFound=%v, want true/false", rep.Found, rep.HandlerFound)
	}
	if meta.Callers == nil {
		t.Error("BuildCallGraphMaps should have populated Callers")
	}
}

func TestResolveHandlerKey_Paths(t *testing.T) {
	meta, mk := sweepMeta()
	pool := meta.StringPool
	const pkg = "example.com/hd"

	meta.Packages[pkg] = &metadata.Package{Files: map[string]*metadata.File{
		"h.go": {Types: map[string]*metadata.Type{
			"handler": {Name: pool.Get("handler"), Methods: []metadata.Method{
				{Name: pool.Get("List"), Position: pool.Get("/hd/h.go:10:2")},
			}},
			"maker": {Name: pool.Get("maker"), Methods: []metadata.Method{
				{Name: pool.Get("Make"), Position: pool.Get("/hd/m.go:20:1")},
			}},
		}},
		"fn.go": {Functions: map[string]*metadata.Function{
			"Serve": {Name: pool.Get("Serve"), Position: pool.Get("/hd/fn.go:5:1")},
		}},
	}}
	meta.Callers[pkg+".handler.List"] = []*metadata.CallGraphEdge{{
		Caller: mk(pkg, "handler", "List", "/hd/h.go:10:2"),
	}}
	meta.Callers[pkg+".FuncLit:/hd/m.go:22:9"] = []*metadata.CallGraphEdge{{
		Caller: mk(pkg, "", "FuncLit", "/hd/m.go:22:9"),
	}}

	// Receiver-insensitive candidate that is itself a caller.
	key, pos := resolveHandlerKey(meta, pkg+".Handler.List")
	if key != pkg+".handler.List" || pos != "/hd/h.go:10" {
		t.Errorf("candidate resolution = %q @ %q", key, pos)
	}

	// Handler-factory: the method is not a caller, its FuncLit is.
	key, pos = resolveHandlerKey(meta, pkg+".Maker.Make")
	if key != pkg+".FuncLit:/hd/m.go:22:9" || pos != "/hd/m.go:20" {
		t.Errorf("funclit resolution = %q @ %q", key, pos)
	}

	// Plain-function candidate (scanned from file Functions) that never
	// resolves to a caller: falls through with the declaration position.
	key, pos = resolveHandlerKey(meta, pkg+".Serve")
	if key != pkg+".Serve" || pos != "/hd/fn.go:5" {
		t.Errorf("function-scan fallback = %q @ %q", key, pos)
	}

	// Nothing matches at all.
	key, pos = resolveHandlerKey(meta, pkg+".Nothing")
	if key != pkg+".Nothing" || pos != "" {
		t.Errorf("miss fallback = %q @ %q", key, pos)
	}
}

func TestResolveInterfaceImplHandler(t *testing.T) {
	meta, mk := sweepMeta()
	pool := meta.StringPool

	meta.Packages["example.com/api"] = &metadata.Package{Files: map[string]*metadata.File{
		"i.go": {Types: map[string]*metadata.Type{
			"Svc": {Kind: pool.Get("interface"), ImplementedBy: []int{
				pool.Get("nodots"),
				pool.Get("example.com/impl.Real"),
			}},
			"Svc2":  {Kind: pool.Get("interface"), ImplementedBy: []int{pool.Get("example.com/impl.Ghost")}},
			"Other": {Kind: pool.Get("struct")},
		}},
	}}
	meta.Packages["example.com/impl"] = &metadata.Package{Files: map[string]*metadata.File{
		"r.go": {Types: map[string]*metadata.Type{
			"Real": {Name: pool.Get("Real"), Methods: []metadata.Method{
				{Name: pool.Get("Handle"), Position: pool.Get("/impl/r.go:30:1")},
			}},
			// Ghost.Handle exists as a method but has no caller edge and no
			// factory FuncLit — the "impl with no body" branch.
			"Ghost": {Name: pool.Get("Ghost"), Methods: []metadata.Method{
				{Name: pool.Get("Handle"), Position: pool.Get("/impl/g.go:40:1")},
			}},
		}},
	}}
	meta.Callers["example.com/impl.Real.Handle"] = []*metadata.CallGraphEdge{{
		Caller: mk("example.com/impl", "Real", "Handle", "/impl/r.go:30:1"),
	}}

	cases := []struct {
		name, opID, wantKey, wantPos string
	}{
		{"unparseable opID", "plain", "", ""},
		{"unknown package", "example.com/nope.Svc.Handle", "", ""},
		{"non-interface receiver", "example.com/api.Other.Handle", "", ""},
		{"impl with no body", "example.com/api.Svc2.Handle", "", ""},
		{"direct concrete method", "example.com/api.Svc.Handle", "example.com/impl.Real.Handle", "/impl/r.go:30"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			key, pos := resolveInterfaceImplHandler(meta, c.opID)
			if key != c.wantKey || pos != c.wantPos {
				t.Errorf("= %q @ %q, want %q @ %q", key, pos, c.wantKey, c.wantPos)
			}
		})
	}
}

func TestSplitOpID_EdgeCases(t *testing.T) {
	if p, r, m := splitOpID3("plain"); p != "" || r != "" || m != "" {
		t.Errorf("splitOpID3(plain) = %q %q %q", p, r, m)
	}
	if p, n := splitOpID("nodot"); p != "" || n != "" {
		t.Errorf("splitOpID(nodot) = %q %q", p, n)
	}
	meta, _ := sweepMeta()
	if keys, file, line := candidateHandlerKeys(meta, "nodot"); keys != nil || file != "" || line != 0 {
		t.Errorf("candidateHandlerKeys(nodot) = %v %q %d", keys, file, line)
	}
}

func TestImplMethodPos_Misses(t *testing.T) {
	meta, _ := sweepMeta()
	pool := meta.StringPool
	meta.Packages["example.com/impl"] = &metadata.Package{Files: map[string]*metadata.File{
		"r.go": {Types: map[string]*metadata.Type{
			"Real": {Name: pool.Get("Real"), Methods: []metadata.Method{
				{Name: pool.Get("Handle"), Position: pool.Get("/impl/r.go:30:1")},
			}},
		}},
	}}

	if f, l := implMethodPos(meta, "example.com/absent", "X", "M"); f != "" || l != 0 {
		t.Errorf("missing package = %q:%d", f, l)
	}
	if f, l := implMethodPos(meta, "example.com/impl", "Ghost", "M"); f != "" || l != 0 {
		t.Errorf("missing type = %q:%d", f, l)
	}
	if f, l := implMethodPos(meta, "example.com/impl", "Real", "Nope"); f != "" || l != 0 {
		t.Errorf("missing method = %q:%d", f, l)
	}
	if f, l := implMethodPos(meta, "example.com/impl", "Real", "Handle"); f != "/impl/r.go" || l != 30 {
		t.Errorf("hit = %q:%d, want /impl/r.go:30", f, l)
	}
}

func TestSchemaSummary_Extras(t *testing.T) {
	sweepRef := func(name string) *spec.Schema { return &spec.Schema{Ref: refPrefix + name} }

	// allOf with a nil member is skipped.
	if got := schemaSummary(&spec.Schema{AllOf: []*spec.Schema{nil, sweepRef("Env")}}); got != "allOf[Env]" {
		t.Errorf("allOf with nil = %q", got)
	}
	// Typeless object with >5 properties gets the ellipsis.
	props := map[string]*spec.Schema{}
	for _, k := range []string{"a", "b", "c", "d", "e", "f"} {
		props[k] = &spec.Schema{Type: "string"}
	}
	got := schemaSummary(&spec.Schema{Properties: props})
	if !strings.HasPrefix(got, "object{a, b, c, d, e") || !strings.Contains(got, "…") {
		t.Errorf("object props = %q", got)
	}
	// No type, no properties → plain object.
	if got := schemaSummary(&spec.Schema{}); got != "object" {
		t.Errorf("bare schema = %q", got)
	}
	// shortName falls back from underscores to dots to the raw string.
	if shortName("a.b") != "b" || shortName("plain") != "plain" {
		t.Errorf("shortName fallbacks broken: %q %q", shortName("a.b"), shortName("plain"))
	}
}
