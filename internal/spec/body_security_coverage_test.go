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
	"testing"

	"github.com/ehabterra/apispec/internal/metadata"
)

// TestCovspecCheckIdentThroughAssignment traces `body := r.Body` and verifies
// that a subsequent ident use is classified as a request source (check ->
// checkIdent -> local assignment -> check(selector)).
func TestCovspecCheckIdentThroughAssignment(t *testing.T) {
	meta := newTestMeta()
	cp := NewContextProvider(meta)
	cfg := &APISpecConfig{Framework: FrameworkConfig{RequestContext: netHTTPRequestContext}}
	r := newBodySourceResolver(cfg, cp)
	if !r.Enabled() {
		t.Fatal("resolver should be enabled")
	}

	// body := r.Body
	reqRoot := mkIdent(meta, "r", "*net/http.Request")
	rBody := mkSelector(meta, reqRoot, mkIdent(meta, "Body", ""))

	edge := &metadata.CallGraphEdge{
		Caller: metadata.Call{Meta: meta, Name: meta.StringPool.Get("handler"), Pkg: meta.StringPool.Get("app"), RecvType: -1},
		AssignmentMap: map[string][]metadata.Assignment{
			"body": {{Value: *rBody}},
		},
	}

	bodyIdent := mkIdent(meta, "body", "")
	if !r.IsRequestSource(bodyIdent, edge) {
		t.Fatal("expected body ident to trace to request source")
	}
}

// TestCovspecCheckUnaryAndSelectorRoot covers the deref/address-of stripping
// loop and the selector-root-is-ident fallback in check().
func TestCovspecCheckUnaryAndSelectorRoot(t *testing.T) {
	meta := newTestMeta()
	cp := NewContextProvider(meta)
	cfg := &APISpecConfig{Framework: FrameworkConfig{RequestContext: netHTTPRequestContext}}
	r := newBodySourceResolver(cfg, cp)

	// &r.Body -> unary wrapping a selector on a request-typed root.
	reqRoot := mkIdent(meta, "r", "*net/http.Request")
	rBody := mkSelector(meta, reqRoot, mkIdent(meta, "Body", ""))
	unary := metadata.NewCallArgument(meta)
	unary.SetKind(metadata.KindUnary)
	unary.X = rBody

	edge := &metadata.CallGraphEdge{
		Caller: metadata.Call{Meta: meta, Name: meta.StringPool.Get("h"), Pkg: meta.StringPool.Get("app"), RecvType: -1},
	}
	if !r.IsRequestSource(unary, edge) {
		t.Fatal("expected &r.Body to be a request source")
	}

	// x.Foo where x is an unrelated ident with no assignment: chainMatches is
	// false, root is ident, checkIdent returns false.
	other := mkIdent(meta, "x", "*os.File")
	sel := mkSelector(meta, other, mkIdent(meta, "Foo", ""))
	if r.IsRequestSource(sel, edge) {
		t.Fatal("unrelated selector must not be a request source")
	}
}

// TestCovspecCheckIdentEdgeCases covers checkIdent guards: empty name, the
// name==callerName self-recursion guard, and the trace-origin fall-through.
func TestCovspecCheckIdentEdgeCases(t *testing.T) {
	meta := newTestMeta()
	cp := NewContextProvider(meta)
	cfg := &APISpecConfig{Framework: FrameworkConfig{RequestContext: netHTTPRequestContext}}
	r := newBodySourceResolver(cfg, cp)

	visited := map[string]bool{}
	if r.checkIdent(mkIdent(meta, "", ""), &metadata.CallGraphEdge{Caller: metadata.Call{Name: -1, Pkg: -1, RecvType: -1}}, visited) {
		t.Fatal("empty-name ident must not be a source")
	}

	// name == caller name -> self-recursion guard returns false.
	edge := &metadata.CallGraphEdge{
		Caller: metadata.Call{Meta: meta, Name: meta.StringPool.Get("selfName"), Pkg: meta.StringPool.Get("app"), RecvType: -1},
	}
	if r.checkIdent(mkIdent(meta, "selfName", ""), edge, map[string]bool{}) {
		t.Fatal("self-referential ident must not be a source")
	}

	// An unknown ident with no assignment and no traceable origin -> false.
	if r.checkIdent(mkIdent(meta, "unknownVar", ""), edge, map[string]bool{}) {
		t.Fatal("untraceable ident must not be a source")
	}
}

// TestCovspecLookupAssignmentsFunction resolves a middleware ident through a
// plain function's AssignmentMap (fallback path in lookupAssignments).
func TestCovspecLookupAssignmentsFunction(t *testing.T) {
	meta := newTestMeta()
	sp := meta.StringPool

	fn := &metadata.Function{
		Name: sp.Get("setup"),
		Pkg:  sp.Get("app"),
		AssignmentMap: map[string][]metadata.Assignment{
			"mw": {{CalleeFunc: "JWT", CalleePkg: "mw"}},
		},
	}
	meta.Packages = map[string]*metadata.Package{
		"app": {
			Files: map[string]*metadata.File{
				"main.go": {Functions: map[string]*metadata.Function{"setup": fn}},
			},
		},
	}

	edge := &metadata.CallGraphEdge{
		Caller: metadata.Call{Meta: meta, Name: sp.Get("setup"), Pkg: sp.Get("app"), RecvType: -1},
	}
	arg := mkIdent(meta, "mw", "")

	ref, ok := resolveMiddlewareIdentRef(edge, arg, meta)
	if !ok {
		t.Fatal("expected middleware ref to resolve")
	}
	if ref.FunctionName != "JWT" || ref.Pkg != "mw" {
		t.Fatalf("got %+v", ref)
	}

	// Direct lookupAssignments hit.
	if got := lookupAssignments(edge, "mw", meta); len(got) != 1 {
		t.Fatalf("expected 1 assignment, got %d", len(got))
	}
	// Unknown var yields nothing.
	if got := lookupAssignments(edge, "nope", meta); got != nil {
		t.Fatalf("expected nil for unknown var, got %+v", got)
	}
}

// TestCovspecLookupAssignmentsMethod resolves through a method's AssignmentMap
// keyed under its receiver type.
func TestCovspecLookupAssignmentsMethod(t *testing.T) {
	meta := newTestMeta()
	sp := meta.StringPool

	// h.mw := middleware.JWT(secret) inside (h *Handler) Register.
	rhsCall := mkMethodCall(meta, mkIdent(meta, "middleware", ""), mkIdentPkg(meta, "JWT", "github.com/x/middleware"))
	method := metadata.Method{
		Name:     sp.Get("Register"),
		Receiver: sp.Get("*Handler"),
		AssignmentMap: map[string][]metadata.Assignment{
			"mw": {{Value: *rhsCall}},
		},
	}
	typ := &metadata.Type{Name: sp.Get("Handler"), Methods: []metadata.Method{method}}
	meta.Packages = map[string]*metadata.Package{
		"app": {
			Files: map[string]*metadata.File{
				"h.go": {Types: map[string]*metadata.Type{"Handler": typ}},
			},
		},
	}

	edge := &metadata.CallGraphEdge{
		Caller: metadata.Call{Meta: meta, Name: sp.Get("Register"), Pkg: sp.Get("app"), RecvType: sp.Get("Handler")},
	}
	arg := mkIdent(meta, "mw", "")

	ref, ok := resolveMiddlewareIdentRef(edge, arg, meta)
	if !ok {
		t.Fatal("expected method-scoped middleware ref to resolve")
	}
	if ref.FunctionName != "JWT" {
		t.Fatalf("got %+v", ref)
	}
}

// TestCovspecResolveMiddlewareIdentRefGuards covers the early-return guards.
func TestCovspecResolveMiddlewareIdentRefGuards(t *testing.T) {
	meta := newTestMeta()

	// Non-ident arg.
	sel := mkSelector(meta, mkIdent(meta, "a", ""), mkIdent(meta, "b", ""))
	if _, ok := resolveMiddlewareIdentRef(&metadata.CallGraphEdge{Caller: metadata.Call{Name: -1, Pkg: -1, RecvType: -1}}, sel, meta); ok {
		t.Fatal("non-ident arg must not resolve")
	}

	// No assignment recorded.
	edge := &metadata.CallGraphEdge{
		Caller: metadata.Call{Meta: meta, Name: meta.StringPool.Get("f"), Pkg: meta.StringPool.Get("app"), RecvType: -1},
	}
	if _, ok := resolveMiddlewareIdentRef(edge, mkIdent(meta, "x", ""), meta); ok {
		t.Fatal("missing assignment must not resolve")
	}

	// nil edge/arg.
	if _, ok := resolveMiddlewareIdentRef(nil, nil, meta); ok {
		t.Fatal("nil inputs must not resolve")
	}

	// lookupAssignments with nil meta and no edge assignment map.
	if got := lookupAssignments(edge, "x", nil); got != nil {
		t.Fatalf("expected nil with nil meta, got %+v", got)
	}
}
