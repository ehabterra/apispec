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
	"testing"

	"github.com/ehabterra/apispec/internal/metadata"
	"github.com/ehabterra/apispec/internal/spec"
)

// TestResolveImplementers verifies an interface method resolves to the
// concrete implementer recorded in ImplementedBy (with the implementer name
// "import/path.Type" split on the last dot, not every dot).
func TestResolveImplementers(t *testing.T) {
	pool := metadata.NewStringPool()
	mkCall := func(pkg, recv, name string) metadata.Call {
		return metadata.Call{Pkg: pool.Get(pkg), RecvType: pool.Get(recv), Name: pool.Get(name)}
	}
	iface := &metadata.Type{
		Kind:          pool.Get("interface"),
		ImplementedBy: []int{pool.Get("github.com/x/usecase.Payment")},
	}
	meta := &metadata.Metadata{
		StringPool: pool,
		Packages: map[string]*metadata.Package{
			"github.com/x/handlers": {Files: map[string]*metadata.File{
				"h.go": {Types: map[string]*metadata.Type{"UseCase": iface}},
			}},
		},
		Callers: map[string][]*metadata.CallGraphEdge{
			"github.com/x/usecase.Payment.Check": {{Caller: mkCall("github.com/x/usecase", "Payment", "Check")}},
		},
	}
	callee := mkCall("github.com/x/handlers", "UseCase", "Check")
	got := resolveImplementers(meta, &callee)
	if _, ok := got["github.com/x/usecase.Payment.Check"]; !ok {
		t.Fatalf("expected concrete impl resolved, got %v", got)
	}

	// An implementer with no recorded body is not returned.
	iface.ImplementedBy = append(iface.ImplementedBy, pool.Get("github.com/x/usecase.NoBody"))
	got = resolveImplementers(meta, &callee)
	if _, ok := got["github.com/x/usecase.NoBody.Check"]; ok {
		t.Fatal("implementer without a body should be skipped")
	}

	// Non-interface receiver → nothing.
	iface.Kind = pool.Get("struct")
	if len(resolveImplementers(meta, &callee)) != 0 {
		t.Fatal("non-interface should resolve to nothing")
	}
}

// TestTraceSites_MultipleCallLocations verifies that a function invoked from
// more than one location collects every distinct call site in node.Sites,
// while a singly-called function leaves Sites empty (its one location is
// already carried by Pos).
func TestTraceSites_MultipleCallLocations(t *testing.T) {
	pool := metadata.NewStringPool()
	meta := &metadata.Metadata{StringPool: pool}
	mk := func(pkg, recv, name, pos string) metadata.Call {
		return metadata.Call{Meta: meta, Pkg: pool.Get(pkg), RecvType: pool.Get(recv), Name: pool.Get(name), Position: pool.Get(pos)}
	}
	const root = "github.com/x/h.Handler.Do"
	meta.Callers = map[string][]*metadata.CallGraphEdge{
		root: {
			// Svc.Run called twice from two different lines.
			{Caller: mk("github.com/x/h", "Handler", "Do", "/proj/h.go:5"), Callee: mk("github.com/x/svc", "Svc", "Run", "/proj/h.go:10")},
			{Caller: mk("github.com/x/h", "Handler", "Do", "/proj/h.go:5"), Callee: mk("github.com/x/svc", "Svc", "Run", "/proj/h.go:22")},
			// Svc.Once called only once.
			{Caller: mk("github.com/x/h", "Handler", "Do", "/proj/h.go:5"), Callee: mk("github.com/x/svc", "Svc", "Once", "/proj/h.go:30")},
		},
	}

	_, tg := analyzeFromHandler(meta, root)
	byID := map[string]TraceNode{}
	for _, n := range tg.Nodes {
		byID[n.ID] = n
	}
	run, ok := byID["github.com/x/svc.Svc.Run"]
	if !ok {
		t.Fatalf("Svc.Run node missing; nodes=%v", byID)
	}
	if len(run.Sites) != 2 {
		t.Fatalf("Svc.Run should have 2 call sites, got %d (%v)", len(run.Sites), run.Sites)
	}
	if run.Sites[0].Pos != "/proj/h.go:10" || run.Sites[1].Pos != "/proj/h.go:22" {
		t.Errorf("call sites not sorted/complete: %v", run.Sites)
	}
	if once := byID["github.com/x/svc.Svc.Once"]; len(once.Sites) != 0 {
		t.Errorf("single-call node should leave Sites empty, got %v", once.Sites)
	}
}

// TestAnalyzers_Fixture drives the trace analyzers with real metadata loaded
// from a testdata fixture, covering the BFS, metrics, trace-graph build, the
// tracker-tree entry point (both the cfg/extractor path and the call-graph
// fallback), and interface resolution.
func TestAnalyzers_Fixture(t *testing.T) {
	m, err := metadata.LoadMetadata("../../testdata/echo/metadata.yaml")
	if err != nil {
		t.Skipf("fixture unavailable: %v", err)
	}
	m.BuildCallGraphMaps()
	const root = "github.com/ehabterra/apispec/testdata/echo.Handler.CreateUser"
	if len(m.Callers[root]) == 0 {
		t.Skip("fixture lacks expected handler")
	}

	// Raw call-graph trace.
	if _, tg := analyzeFromHandler(m, root); len(tg.Nodes) == 0 || tg.Nodes[0].ID != root {
		t.Fatalf("analyzeFromHandler: bad trace (%d nodes)", len(tg.Nodes))
	}

	// Call-graph + structural interface resolution.
	mtr, tg, ok := analyzeResolvedCallGraph(m, root)
	if !ok || len(tg.Nodes) == 0 {
		t.Fatalf("analyzeResolvedCallGraph: ok=%v nodes=%d", ok, len(tg.Nodes))
	}
	if mtr.Reachable < 1 || len(tg.Edges) == 0 {
		t.Fatalf("metrics look empty: reachable=%d edges=%d", mtr.Reachable, len(tg.Edges))
	}

	// Tracker entry, no cfg → falls back to the resolved call graph.
	if _, _, ok := analyzeFromTrackerTree(m, nil, "POST", "/users", root); !ok {
		t.Fatal("analyzeFromTrackerTree (nil cfg) should fall back and succeed")
	}

	// Tracker entry with a cfg → builds the tree + runs the extractor (route
	// may or may not match; either way the tree-building path is exercised).
	if _, _, ok := analyzeFromTrackerTree(m, spec.DefaultEchoConfig(), "POST", "/users", root); !ok {
		t.Fatal("analyzeFromTrackerTree (echo cfg) should succeed")
	}
}
