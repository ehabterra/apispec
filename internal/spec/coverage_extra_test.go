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
	roots := tree.GetRoots()
	if len(roots) == 0 {
		t.Skip("fixture produced no roots")
	}
	n := 0
	var walk func(node TrackerNodeInterface)
	walk = func(node TrackerNodeInterface) {
		if node == nil || n >= 50 {
			return
		}
		n++
		_ = node.GetKey()
		_ = node.GetParent()
		_ = node.GetEdge()
		_ = node.GetArgument()
		_ = node.GetTypeParamMap()
		for _, c := range node.GetChildren() {
			walk(c)
		}
	}
	for _, r := range roots {
		walk(r)
	}
	if n == 0 {
		t.Error("no nodes visited")
	}
}

func TestVisualizationFromTree(t *testing.T) {
	tree, meta := loadEchoTree(t)
	roots := tree.GetRoots()
	if len(roots) == 0 {
		t.Skip("no roots")
	}
	data := DrawTrackerTreeCytoscapeWithMetadata(roots, meta)
	if data == nil {
		t.Fatal("DrawTrackerTreeCytoscapeWithMetadata nil")
	}
	_ = OrderTrackerTreeNodesDepthFirst(data)
	_ = TraverseTrackerTreeBranchOrder(data)
}

func TestExportToFiles(t *testing.T) {
	_, meta := loadEchoTree(t)
	dir := t.TempDir()
	check := func(err error, what string) {
		if err != nil {
			t.Errorf("%s: %v", what, err)
		}
	}
	check(GenerateCallGraphCytoscapeHTML(meta, filepath.Join(dir, "cg.html")), "GenerateCallGraphCytoscapeHTML")
	check(GeneratePaginatedCytoscapeHTML(meta, filepath.Join(dir, "pag.html"), 100), "GeneratePaginatedCytoscapeHTML")
}
