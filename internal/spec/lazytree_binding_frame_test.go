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
	"strings"
	"testing"

	"github.com/ehabterra/apispec/internal/metadata"
)

// typedMeta builds type-checked metadata for one file of package main, so the
// binding facts (ParamArgMap, assignments, origins) are the ones the real
// generator records rather than hand-assembled records.
func typedMeta(t *testing.T, src string) *metadata.Metadata {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "main.go", src, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	info := &types.Info{
		Types:      map[ast.Expr]types.TypeAndValue{},
		Defs:       map[*ast.Ident]types.Object{},
		Uses:       map[*ast.Ident]types.Object{},
		Selections: map[*ast.SelectorExpr]*types.Selection{},
	}
	if _, err := (&types.Config{}).Check("main", fset, []*ast.File{file}, info); err != nil {
		t.Fatalf("typecheck: %v", err)
	}
	md := metadata.GenerateMetadata(map[string]map[string]*ast.File{"main": {"main.go": file}},
		map[*ast.File]*types.Info{file: info}, map[string]string{"main": "main"}, fset)
	md.BuildCallGraphMaps()
	return md
}

// The router type is an interface so its methods have no bodies: the origin of
// `api` stays `api` in main, as it does for an external framework's Group.
const bindingFrameSrc = `package main

type R interface {
	Group(p string) R
	GET(p string, h func())
}

func New() R { return nil }
func a()     {}
func b()     {}

func reg(r R, path string, h func()) { r.GET(path, h) }

func main() {
	r := New()
	api := r.Group("/api")
	reg(api, "/a", a)
	reg(api, "/b", b)
	reg(r.Group("/x"), "/c", a)
}
`

// TestLazyTreeHelperRegistrationsHangInsideTheBindingCall covers issue #615. A
// helper's calls on a parameter bound to a group belong under the group — that
// is where the prefix is — but every OTHER argument of those calls (the handler,
// the path) is a parameter of the helper too, bound only by the helper call's
// ParamArgMap. Hung straight under the group the registration had the prefix
// and no binding; so the binding argument itself is the frame between them, one
// per call site.
//
// A call argument (`reg(r.Group("/x"), …)`) is its own producer and keeps the
// plain receiver-children binding: a frame under it would be the producer again.
func TestLazyTreeHelperRegistrationsHangInsideTheBindingCall(t *testing.T) {
	meta := typedMeta(t, bindingFrameSrc)
	tree := NewLazyTree(meta, metadata.TrackerLimits{
		MaxNodesPerTree: 10000, MaxChildrenPerNode: 100, MaxArgsPerFunction: 100,
		MaxNestedArgsDepth: 10, MaxRecursionDepth: 100,
	})
	tree.buildRelations()

	const group = "main.R.Group@main.go:16:9"
	frames := tree.bindingFrames[group]
	if len(frames) != 2 {
		t.Fatalf("want one frame per call site binding api, got %d", len(frames))
	}
	for _, f := range frames {
		if f.argEdge == nil || getString(meta, f.argEdge.Callee.Name) != "reg" {
			t.Fatalf("frame %s is not an argument of a reg call", f.key)
		}
		if pe := tree.paramEdgesAt[f.arg]; len(pe) != 1 || getString(meta, pe[0].Callee.Name) != "GET" {
			t.Errorf("frame %s does not carry reg's r.GET: %d edges", f.key, len(pe))
		}
	}
	if kids := tree.receiverChildren[group]; len(kids) != 0 {
		t.Errorf("r.GET also hangs straight under the group, outside any frame: %d edges", len(kids))
	}
	if kids := tree.receiverChildren["main.R.Group@main.go:19:6"]; len(kids) != 1 {
		t.Errorf("the inline group call lost its receiver-children binding: %d edges", len(kids))
	}

	// The registrations reached under the group: each sits in its own call's
	// frame, so the path and handler resolve to that call's arguments.
	got := map[string]string{}
	var walk func(n TrackerNodeInterface, underGroup bool, depth int)
	walk = func(n TrackerNodeInterface, underGroup bool, depth int) {
		if depth > 12 {
			return
		}
		e := n.GetEdge()
		if n.GetArgument() == nil && e != nil && strings.HasPrefix(n.GetKey(), group) {
			underGroup = true
		}
		if underGroup && n.GetArgument() == nil && e != nil && getString(meta, e.Callee.Name) == "GET" {
			path, _ := resolveArgThroughParams(e.Args[0], n)
			h, _ := resolveArgThroughParams(e.Args[1], n)
			got[path.GetValue()] = h.GetName()
		}
		for _, c := range n.GetChildren() {
			walk(c, underGroup, depth+1)
		}
	}
	for _, root := range tree.GetRoots() {
		walk(root, false, 0)
	}
	want := map[string]string{`"/a"`: "a", `"/b"`: "b"}
	for path, h := range want {
		if got[path] != h {
			t.Errorf("registration %s under the group: handler %q, want %q (all: %v)", path, got[path], h, got)
		}
	}
	if len(got) != len(want) {
		t.Errorf("registrations under the group = %v, want %v", got, want)
	}
}

// TestHandlerUnbound pins which handler names make an operationId (issue #615):
// a declared function, a package-level variable, or a value whose type serves
// through the handler interface is a name a reader can find; a parameter or
// local that no hop resolved renders as its TYPE and names nothing.
func TestHandlerUnbound(t *testing.T) {
	meta := typedMeta(t, `package main

type HandlerFunc func()

type statusHandler struct{}

func (s *statusHandler) ServeHTTP() {}

var global HandlerFunc

func named() {}

func reg(h HandlerFunc) { _ = h }

func mount(v *statusHandler) { _ = v }

func main() { reg(named); mount(&statusHandler{}) }
`)
	ident := func(name, typ string) *metadata.CallArgument {
		a := metadata.NewCallArgument(meta)
		a.SetKind(metadata.KindIdent)
		a.SetName(name)
		a.SetPkg("main")
		a.SetType(typ)
		return a
	}
	methods := []string{"ServeHTTP"}
	for _, c := range []struct {
		name, typ string
		want      bool
	}{
		{"named", "func()", false},
		{"global", "main.HandlerFunc", false},
		{"v", "*main.statusHandler", false},
		{"h", "main.HandlerFunc", true},
	} {
		if got := handlerUnbound(ident(c.name, c.typ), methods); got != c.want {
			t.Errorf("handlerUnbound(%s %s) = %v, want %v", c.name, c.typ, got, c.want)
		}
	}
	lit := metadata.NewCallArgument(meta)
	lit.SetKind(metadata.KindFuncLit)
	if handlerUnbound(lit, methods) || handlerUnbound(nil, methods) {
		t.Error("a closure, or no argument, is not an unbound name")
	}
}

// gateNode is a TrackerNodeInterface carrying only what readsAsStatement looks
// at: the node's edge and argument.
type gateNode struct {
	edge *metadata.CallGraphEdge
	arg  *metadata.CallArgument
}

func (g *gateNode) GetKey() string                      { return "" }
func (g *gateNode) GetParent() TrackerNodeInterface     { return nil }
func (g *gateNode) GetChildren() []TrackerNodeInterface { return nil }
func (g *gateNode) GetEdge() *metadata.CallGraphEdge    { return g.edge }
func (g *gateNode) GetArgument() *metadata.CallArgument { return g.arg }
func (g *gateNode) GetTypeParamMap() map[string]string  { return nil }

// TestReadsAsStatement pins the extraction gate (issue #615): a binding frame
// — an ident argument hung under a producer, carrying its helper call's edge —
// is not read, while a call node, an argument under its own call, and an
// argument that is itself a call (carrying its own edge, wherever it hangs)
// are. The last one is the only copy of `parse(req.Header.Get("X"))`'s read a
// large service's walk reaches; gating it lost a thousand parameters.
func TestReadsAsStatement(t *testing.T) {
	meta := &metadata.Metadata{StringPool: metadata.NewStringPool()}
	producer := &metadata.CallGraphEdge{}
	helperCall := &metadata.CallGraphEdge{}
	parseCall := &metadata.CallGraphEdge{}
	getCall := &metadata.CallGraphEdge{}

	ident := metadata.NewCallArgument(meta)
	ident.SetKind(metadata.KindIdent)
	ident.SetName("values")
	callArg := metadata.NewCallArgument(meta)
	callArg.SetKind(metadata.KindCall)
	callArg.Edge = getCall

	for _, tc := range []struct {
		name          string
		parent, child TrackerNodeInterface
		want          bool
	}{
		{"call node", &gateNode{edge: producer}, &gateNode{edge: helperCall}, true},
		{"argument under its own call", &gateNode{edge: helperCall}, &gateNode{edge: helperCall, arg: ident}, true},
		{"binding frame under a producer", &gateNode{edge: producer}, &gateNode{edge: helperCall, arg: ident}, false},
		{"call argument under the call it feeds", &gateNode{edge: parseCall}, &gateNode{edge: getCall, arg: callArg}, true},
		{"nil child", &gateNode{edge: producer}, nil, false},
	} {
		if got := readsAsStatement(tc.parent, tc.child); got != tc.want {
			t.Errorf("%s: readsAsStatement = %v, want %v", tc.name, got, tc.want)
		}
	}
}
