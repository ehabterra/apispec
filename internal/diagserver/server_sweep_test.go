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

package diagserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ehabterra/apispec/internal/metadata"
	"github.com/ehabterra/apispec/internal/spec"
)

// TestPackageHierarchyTree drives handlePackageHierarchy through its buildTree
// recursion: nested package names produce a parent→child→grandchild tree, the
// non-root detection (a package whose prefix is also a package), and the
// depth-0 rename. A pre-seeded dataCache short-circuits getAllData so no real
// diagram generation is needed.
func TestPackageHierarchyTree(t *testing.T) {
	s := New(&Config{Host: "localhost", Port: 8080, DiagramType: "call-graph", PageSize: 50, MaxDepth: 3})
	// Nested packages; "root/a/b/" (trailing slash) exercises the empty-name guard.
	s.metadata = &metadata.Metadata{Packages: map[string]*metadata.Package{
		"root":       {},
		"root/a":     {},
		"root/a/b":   {},
		"root/a/b/":  {},
		"standalone": {},
	}}
	s.cache = map[string]*spec.PaginatedCytoscapeData{}
	// getAllData(diagramType, true) => cacheKey "call-graph:full".
	s.dataCache = map[string]*spec.CytoscapeData{
		"call-graph:full": {
			Nodes: []spec.CytoscapeNode{
				{Data: spec.CytoscapeNodeData{ID: "n1", Package: "root/a"}},
				{Data: spec.CytoscapeNodeData{ID: "n2", Package: "root/a/b"}},
				{Data: spec.CytoscapeNodeData{ID: "n3", Package: ""}}, // empty package skipped
			},
		},
	}

	w := httptest.NewRecorder()
	s.handlePackageHierarchy(w, httptest.NewRequest(http.MethodGet, "/api/diagram/packages", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("packages -> %d", w.Code)
	}
	var resp PackageHierarchyResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.TotalCount != 2 {
		t.Errorf("TotalCount = %d, want 2", resp.TotalCount)
	}
	// "root" and "standalone" are roots; "root/a"* nest under root.
	var sawRoot bool
	for _, rp := range resp.RootPackages {
		if rp.FullPath == "root" {
			sawRoot = true
			if len(rp.Children) == 0 {
				t.Error("root should have children")
			}
			// Count aggregates descendants (n1 under root/a, n2 under root/a/b).
			if rp.Count != 2 {
				t.Errorf("root aggregate count = %d, want 2", rp.Count)
			}
		}
	}
	if !sawRoot {
		t.Error("expected a 'root' package node")
	}
}

// TestPackageHierarchyMetadataFailure covers the ensureMetadata error branch of
// handlePackageHierarchy (the other handlers' equivalents are already covered).
func TestPackageHierarchyMetadataFailure(t *testing.T) {
	s := New(&Config{Host: "localhost", Port: 8080, DiagramType: "call-graph", InputDir: t.TempDir()})
	w := httptest.NewRecorder()
	s.handlePackageHierarchy(w, httptest.NewRequest(http.MethodGet, "/api/diagram/packages", nil))
	if w.Code != http.StatusInternalServerError {
		t.Errorf("packages without loadable metadata -> %d, want 500", w.Code)
	}
}

// TestPaginatedDiagramClampsAndCache covers the page/size/depth clamping edges
// of handlePaginatedDiagram and the generatePaginatedDataInternal cache-hit
// path (a second identical request is served from the cache).
func TestPaginatedDiagramClampsAndCache(t *testing.T) {
	s := injectedServer(t)
	mux := muxFor(s)

	// size > 2000 clamps to 2000; depth < 0 clamps to 0.
	q := "/api/diagram/page?page=1&size=99999&depth=-5"
	if w := do(mux, http.MethodGet, q); w.Code != http.StatusOK {
		t.Fatalf("clamped page request -> %d", w.Code)
	}
	// Identical request again -> internal cache hit.
	if w := do(mux, http.MethodGet, q); w.Code != http.StatusOK {
		t.Fatalf("cached page request -> %d", w.Code)
	}
}

// TestPaginatedDiagramVerbose covers the Verbose logging branch inside
// generatePaginatedDataInternal.
func TestPaginatedDiagramVerbose(t *testing.T) {
	s := injectedServer(t)
	s.config.Verbose = true
	mux := muxFor(s)
	if w := do(mux, http.MethodGet, "/api/diagram/page?page=1&size=10&depth=2"); w.Code != http.StatusOK {
		t.Fatalf("verbose page request -> %d", w.Code)
	}
}

// TestTrackerTreePagination drives the tracker-tree branches of
// generatePaginatedDataInternal: the tracker-tree ordering, the non-call-graph
// depth-filter else branch, the tracker-tree pagination slice, and (with a
// tiny page size) the parent-node re-injection.
func TestTrackerTreePagination(t *testing.T) {
	s := injectedServer(t)
	s.config.DiagramType = "tracker-tree"
	mux := muxFor(s)

	for _, q := range []string{
		"/api/diagram/page?page=1&size=1&depth=2",
		"/api/diagram/page?page=2&size=1&depth=2",
		"/api/diagram/page?page=999&size=1", // start beyond len -> empty page
	} {
		if w := do(mux, http.MethodGet, q); w.Code != http.StatusOK {
			t.Errorf("tracker-tree %s -> %d", q, w.Code)
		}
	}
}

// TestExportBranches covers handleExport's CORS header branch, the depth<0
// clamp and the ensureMetadata failure path.
func TestExportBranches(t *testing.T) {
	s := injectedServer(t)
	s.config.EnableCORS = true
	mux := muxFor(s)

	w := do(mux, http.MethodGet, "/api/diagram/export?format=json&depth=-2")
	if w.Code != http.StatusOK {
		t.Fatalf("cors json export -> %d", w.Code)
	}
	if w.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Error("export missing CORS header")
	}

	bad := New(&Config{Host: "localhost", Port: 8080, DiagramType: "call-graph", InputDir: t.TempDir()})
	badMux := muxFor(bad)
	if w := do(badMux, http.MethodGet, "/api/diagram/export?format=json"); w.Code != http.StatusInternalServerError {
		t.Errorf("export without loadable metadata -> %d, want 500", w.Code)
	}
}

// TestCalculateCallGraphDepthCycle exercises calculateCallGraphDepth's
// no-root fallback: a pure cycle leaves every node with a non-zero in-degree,
// so the initial root scan finds nothing and the fallback loop runs.
func TestCalculateCallGraphDepthCycle(t *testing.T) {
	s := newTestServer()
	data := &spec.CytoscapeData{
		Nodes: []spec.CytoscapeNode{
			{Data: spec.CytoscapeNodeData{ID: "A"}},
			{Data: spec.CytoscapeNodeData{ID: "B"}},
			{Data: spec.CytoscapeNodeData{ID: "C"}},
		},
		Edges: []spec.CytoscapeEdge{
			{Data: spec.CytoscapeEdgeData{Source: "A", Target: "B"}},
			{Data: spec.CytoscapeEdgeData{Source: "B", Target: "C"}},
			{Data: spec.CytoscapeEdgeData{Source: "C", Target: "A"}},
		},
	}
	// No panic and no roots discovered -> empty depth map from the fallback.
	if depths := s.calculateCallGraphDepth(data); len(depths) != 0 {
		t.Errorf("pure cycle should yield no reachable depths, got %v", depths)
	}
}

// TestNodeMatchesFiltersExtra covers the CallPaths file-match branch and the
// generics-miss branch of nodeMatchesFilters that the existing table omits.
func TestNodeMatchesFiltersExtra(t *testing.T) {
	// File match via CallPaths when the node has no direct Position.
	nodeViaCallPath := spec.CytoscapeNode{Data: spec.CytoscapeNodeData{
		Label:     "pkg.Handler.Create",
		Position:  "",
		CallPaths: []spec.CallPathInfo{{Position: "internal/handler.go:42"}},
	}}
	if !nodeMatchesFilters(nodeViaCallPath, nil, []string{"handler.go"}, nil, nil, nil, "") {
		t.Error("expected a file match through CallPaths")
	}
	if nodeMatchesFilters(nodeViaCallPath, nil, []string{"missing.go"}, nil, nil, nil, "") {
		t.Error("unmatched file should reject even via CallPaths")
	}

	// Generics present but no filter matches -> reject.
	genericNode := spec.CytoscapeNode{Data: spec.CytoscapeNodeData{
		Label:    "pkg.Do",
		Generics: map[string]string{"T": "int"},
	}}
	if nodeMatchesFilters(genericNode, nil, nil, nil, nil, []string{"string"}, "") {
		t.Error("generics-miss should reject the node")
	}
}
