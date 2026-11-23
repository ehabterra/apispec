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
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "main.go", returnedGroupSrc, parser.ParseComments)
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

	// For every GET node, the lines of the Group calls above it.
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
