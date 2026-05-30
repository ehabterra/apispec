package spec

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/ehabterra/apispec/internal/metadata"
)

func loadEchoTree(t *testing.T) (*TrackerTree, *metadata.Metadata) {
	t.Helper()
	meta, err := metadata.LoadMetadata("../../testdata/echo/metadata.yaml")
	if err != nil {
		t.Skipf("fixture unavailable: %v", err)
	}
	meta.BuildCallGraphMaps()
	tree := NewTrackerTree(meta, metadata.TrackerLimits{
		MaxNodesPerTree: 50000, MaxChildrenPerNode: 500, MaxArgsPerFunction: 100,
		MaxNestedArgsDepth: 100, MaxRecursionDepth: 1000,
	}, nil)
	return tree, meta
}

func TestFieldResolverHelpers(t *testing.T) {
	if got := stripPointer("*Foo"); got != "Foo" {
		t.Errorf("stripPointer(*Foo) = %q", got)
	}
	if got := stripPointer("Foo"); got != "Foo" {
		t.Errorf("stripPointer(Foo) = %q", got)
	}
	if pkg, ty := splitPkgType("github.com/x/y.Type"); pkg != "github.com/x/y" || ty != "Type" {
		t.Errorf("splitPkgType = %q,%q", pkg, ty)
	}
	if pkg, ty := splitPkgType("Bare"); pkg != "" || ty != "Bare" {
		t.Errorf("splitPkgType(Bare) = %q,%q", pkg, ty)
	}
}

func TestSplitNodeLabel(t *testing.T) {
	if l, pos := splitNodeLabel("pkg.Fn@file.go:10"); l != "pkg.Fn" || pos != "file.go:10" {
		t.Errorf("splitNodeLabel = %q,%q", l, pos)
	}
	if l, pos := splitNodeLabel("pkg.Fn"); l != "pkg.Fn" || pos != "" {
		t.Errorf("splitNodeLabel(no @) = %q,%q", l, pos)
	}
}

func TestTrackerTreeAccessors(t *testing.T) {
	tree, meta := loadEchoTree(t)
	if tree.GetMetadata() != meta {
		t.Error("GetMetadata mismatch")
	}
	if tree.GetLimits().MaxNodesPerTree == 0 {
		t.Error("GetLimits not set")
	}
	roots := tree.GetRoots()
	if len(roots) == 0 {
		t.Skip("fixture produced no roots")
	}
	if tree.GetNodeCount() == 0 {
		t.Error("GetNodeCount = 0")
	}
	n := 0
	tree.TraverseTree(func(node TrackerNodeInterface) bool {
		_ = node.GetKey()
		_ = node.GetParent()
		_ = node.GetChildren()
		_ = node.GetEdge()
		_ = node.GetArgument()
		_ = node.GetArgType()
		_ = node.GetArgIndex()
		_ = node.GetArgContext()
		_ = node.GetTypeParamMap()
		_ = node.GetRootAssignmentMap()
		n++
		return n < 50
	})
	if k := roots[0].GetKey(); k != "" {
		if tree.FindNodeByKey(k) == nil {
			t.Errorf("FindNodeByKey(%q) = nil", k)
		}
	}
	if tree.FindNodeByKey("definitely::not::a::key") != nil {
		t.Error("FindNodeByKey(bogus) should be nil")
	}
}

func TestVisualizationFromTree(t *testing.T) {
	tree, meta := loadEchoTree(t)
	roots := tree.GetRoots()
	if len(roots) == 0 {
		t.Skip("no roots")
	}
	if DrawTrackerTree(roots) == "" {
		t.Error("DrawTrackerTree returned empty string")
	}
	data := DrawTrackerTreeCytoscape(roots)
	if data == nil {
		t.Fatal("DrawTrackerTreeCytoscape nil")
	}
	_ = DrawTrackerTreeCytoscapeWithMetadata(roots, meta)
	_ = OrderTrackerTreeNodesDepthFirst(data)
	_ = TraverseTrackerTreeBranchOrder(data)
}

func TestExportToFiles(t *testing.T) {
	tree, meta := loadEchoTree(t)
	roots := tree.GetRoots()
	dir := t.TempDir()
	check := func(err error, what string) {
		if err != nil {
			t.Errorf("%s: %v", what, err)
		}
	}
	check(GenerateCytoscapeHTML(roots, filepath.Join(dir, "t.html")), "GenerateCytoscapeHTML")
	check(ExportCytoscapeJSON(roots, filepath.Join(dir, "t.json")), "ExportCytoscapeJSON")
	check(GenerateCallGraphCytoscapeHTML(meta, filepath.Join(dir, "cg.html")), "GenerateCallGraphCytoscapeHTML")
	check(ExportCallGraphCytoscapeJSON(meta, filepath.Join(dir, "cg.json")), "ExportCallGraphCytoscapeJSON")
	check(GenerateOptimizedCallGraphHTML(meta, filepath.Join(dir, "opt.html"), "paginated"), "opt/paginated")
	check(GenerateOptimizedCallGraphHTML(meta, filepath.Join(dir, "opt2.html"), ""), "opt/default")
	check(GeneratePaginatedCytoscapeHTML(meta, filepath.Join(dir, "pag.html"), 100), "GeneratePaginatedCytoscapeHTML")
}

func TestPaginatedServer(t *testing.T) {
	_, meta := loadEchoTree(t)
	srv := NewPaginatedCallGraphServer(meta, 50)
	for _, q := range []string{"", "?page=1", "?page=2&depth=3", "?package=echo", "?page=0"} {
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/"+q, nil))
		if rec.Code != http.StatusOK {
			t.Errorf("q=%q code=%d", q, rec.Code)
		}
		if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
			t.Errorf("q=%q content-type=%q", q, ct)
		}
	}
	if err := GenerateServerBasedCytoscapeHTML("http://localhost:9999", filepath.Join(t.TempDir(), "sb.html")); err != nil {
		t.Errorf("GenerateServerBasedCytoscapeHTML: %v", err)
	}
}
