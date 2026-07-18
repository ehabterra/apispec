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
	"sort"
	"testing"

	"github.com/ehabterra/apispec/internal/metadata"
)

func branchStatusMatcher(meta *metadata.Metadata) *ResponsePatternMatcherImpl {
	cfg := &APISpecConfig{Defaults: Defaults{ResponseContentType: "application/json"}}
	return &ResponsePatternMatcherImpl{
		BasePatternMatcher: &BasePatternMatcher{
			cfg:             cfg,
			contextProvider: NewContextProvider(meta),
			schemaMapper:    NewSchemaMapper(cfg),
		},
	}
}

// httpStatusSelector builds `http.StatusXxx` as a selector CallArgument.
func httpStatusSelector(meta *metadata.Metadata, name string) metadata.CallArgument {
	x := metadata.NewCallArgument(meta)
	x.SetKind(metadata.KindIdent)
	x.SetName("http")
	sel := metadata.NewCallArgument(meta)
	sel.SetKind(metadata.KindIdent)
	sel.SetName(name)
	a := metadata.NewCallArgument(meta)
	a.SetKind(metadata.KindSelector)
	a.X = x
	a.Sel = sel
	return *a
}

// TestExpandVarStatuses_Constants covers issue #155's core: a variable set to
// constant statuses across branches (statusCode = http.StatusNotFound), not
// through constructor calls, fans out to the concrete codes.
func TestExpandVarStatuses_Constants(t *testing.T) {
	meta := &metadata.Metadata{StringPool: metadata.NewStringPool()}
	m := branchStatusMatcher(meta)
	cp := m.contextProvider.(*ContextProviderImpl)

	fn := &metadata.Function{AssignmentMap: map[string][]metadata.Assignment{
		"statusCode": {
			{Value: httpStatusSelector(meta, "StatusNotFound")},
			{Value: httpStatusSelector(meta, "StatusBadRequest")},
			{Value: httpStatusSelector(meta, "StatusInternalServerError")},
		},
	}}
	codes, residue := m.expandVarStatuses("statusCode", fn, cp)
	sort.Ints(codes)
	if want := []int{400, 404, 500}; len(codes) != 3 || codes[0] != want[0] || codes[1] != want[1] || codes[2] != want[2] {
		t.Errorf("codes = %v, want %v", codes, want)
	}
	if residue {
		t.Error("all-constant branches must not report a residue")
	}
}

// TestExpandVarStatuses_Residue: a non-constant branch (a computed status)
// yields the constant codes plus residue=true, so the caller keeps an honest
// default.
func TestExpandVarStatuses_Residue(t *testing.T) {
	meta := &metadata.Metadata{StringPool: metadata.NewStringPool()}
	m := branchStatusMatcher(meta)
	cp := m.contextProvider.(*ContextProviderImpl)

	// A call with no status literal — a computed/dynamic status.
	dyn := metadata.NewCallArgument(meta)
	dyn.SetKind(metadata.KindCall)
	fun := metadata.NewCallArgument(meta)
	fun.SetKind(metadata.KindIdent)
	fun.SetName("statusFor")
	dyn.Fun = fun

	fn := &metadata.Function{AssignmentMap: map[string][]metadata.Assignment{
		"statusCode": {
			{Value: httpStatusSelector(meta, "StatusNotFound")},
			{Value: *dyn},
		},
	}}
	codes, residue := m.expandVarStatuses("statusCode", fn, cp)
	if len(codes) != 1 || codes[0] != 404 {
		t.Errorf("codes = %v, want [404]", codes)
	}
	if !residue {
		t.Error("a non-constant branch must report a residue")
	}
}

// TestExpandVarStatuses_SingleAssignmentNoFanOut: a single assignment is left
// to the normal latest-wins path (no fan-out).
func TestExpandVarStatuses_SingleAssignmentNoFanOut(t *testing.T) {
	meta := &metadata.Metadata{StringPool: metadata.NewStringPool()}
	m := branchStatusMatcher(meta)
	cp := m.contextProvider.(*ContextProviderImpl)

	fn := &metadata.Function{AssignmentMap: map[string][]metadata.Assignment{
		"statusCode": {{Value: httpStatusSelector(meta, "StatusOK")}},
	}}
	if codes, residue := m.expandVarStatuses("statusCode", fn, cp); codes != nil || residue {
		t.Errorf("single assignment must not fan out; got codes=%v residue=%v", codes, residue)
	}
}

// TestStatusCodeOfValue covers the constant, constructor-call, and
// non-constant shapes.
func TestStatusCodeOfValue(t *testing.T) {
	meta := &metadata.Metadata{StringPool: metadata.NewStringPool()}
	m := branchStatusMatcher(meta)
	cp := m.contextProvider.(*ContextProviderImpl)

	t.Run("constant selector", func(t *testing.T) {
		v := httpStatusSelector(meta, "StatusNotFound")
		if s, ok := m.statusCodeOfValue(&v, cp); !ok || s != 404 {
			t.Errorf("got (%d,%v), want (404,true)", s, ok)
		}
	})

	t.Run("constructor call carrying a status literal", func(t *testing.T) {
		call := metadata.NewCallArgument(meta)
		call.SetKind(metadata.KindCall)
		fun := metadata.NewCallArgument(meta)
		fun.SetKind(metadata.KindIdent)
		fun.SetName("NewError")
		call.Fun = fun
		msg := metadata.NewCallArgument(meta)
		msg.SetKind(metadata.KindIdent)
		msg.SetName("msg")
		st := httpStatusSelector(meta, "StatusBadRequest")
		call.Args = []*metadata.CallArgument{msg, &st}
		if s, ok := m.statusCodeOfValue(call, cp); !ok || s != 400 {
			t.Errorf("got (%d,%v), want (400,true)", s, ok)
		}
	})

	t.Run("non-constant is not a status", func(t *testing.T) {
		call := metadata.NewCallArgument(meta)
		call.SetKind(metadata.KindCall)
		fun := metadata.NewCallArgument(meta)
		fun.SetKind(metadata.KindIdent)
		fun.SetName("statusFor")
		call.Fun = fun
		if s, ok := m.statusCodeOfValue(call, cp); ok {
			t.Errorf("got (%d,true), want (_,false)", s)
		}
		if _, ok := m.statusCodeOfValue(nil, cp); ok {
			t.Error("nil value must be false")
		}
	})
}

// TestFindCallEdge covers the call-site position match, the first-match
// fallback, the no-match case, and the optional package filter.
func TestFindCallEdge(t *testing.T) {
	meta := &metadata.Metadata{StringPool: metadata.NewStringPool()}
	impl := NewContextProvider(meta)
	caller := metadata.Call{Name: meta.StringPool.Get("h"), Pkg: meta.StringPool.Get("app")}
	e := func(pkg, pos string) metadata.CallGraphEdge {
		return metadata.CallGraphEdge{
			Caller:   caller,
			Callee:   metadata.Call{Name: meta.StringPool.Get("New"), Pkg: meta.StringPool.Get(pkg)},
			Position: meta.StringPool.Get(pos),
		}
	}
	meta.CallGraph = []metadata.CallGraphEdge{e("pkgA", "pos1"), e("pkgB", "pos2")}
	callerID := caller.ID()

	// Package-agnostic ("" pkg): position match, first-match fallback, no-match.
	if got := findCallEdge(impl, callerID, "New", "", "pos2"); got == nil || impl.GetString(got.Position) != "pos2" {
		t.Errorf("valPos match should return the pos2 edge, got %v", got)
	}
	if got := findCallEdge(impl, callerID, "New", "", ""); got == nil || impl.GetString(got.Position) != "pos1" {
		t.Errorf("no valPos should return the first match (pos1), got %v", got)
	}
	if got := findCallEdge(impl, callerID, "New", "", "nope"); got == nil || impl.GetString(got.Position) != "pos1" {
		t.Errorf("unmatched valPos should fall back to the first match, got %v", got)
	}
	if got := findCallEdge(impl, callerID, "Missing", "", ""); got != nil {
		t.Errorf("unknown callee should return nil, got %v", got)
	}
	// Package filter: only the matching-package edge is eligible.
	if got := findCallEdge(impl, callerID, "New", "pkgB", ""); got == nil || impl.GetString(got.Position) != "pos2" {
		t.Errorf("pkg filter should select the pkgB edge, got %v", got)
	}
	if got := findCallEdge(impl, callerID, "New", "pkgC", ""); got != nil {
		t.Errorf("non-matching pkg must yield nil, got %v", got)
	}
}

// TestStatusesFromConstructorField_Guards covers the early-out guards.
func TestStatusesFromConstructorField_Guards(t *testing.T) {
	meta := &metadata.Metadata{StringPool: metadata.NewStringPool()}
	m := branchStatusMatcher(meta)

	sel := func(base, field string) *metadata.CallArgument {
		a := metadata.NewCallArgument(meta)
		a.SetKind(metadata.KindSelector)
		x := metadata.NewCallArgument(meta)
		x.SetKind(metadata.KindIdent)
		x.SetName(base)
		f := metadata.NewCallArgument(meta)
		f.SetKind(metadata.KindIdent)
		f.SetName(field)
		a.X, a.Sel = x, f
		return a
	}
	node := &fakeNode{edge: &metadata.CallGraphEdge{
		Caller: metadata.Call{Name: meta.StringPool.Get("h"), Pkg: meta.StringPool.Get("app")},
	}}

	cases := []struct {
		name string
		arg  *metadata.CallArgument
		node TrackerNodeInterface
	}{
		{"nil arg", nil, node},
		{"non-selector", mkIdent(meta, "x", ""), node},
		{"selector, empty field", sel("e", ""), node},
		{"selector, unresolvable base scope", sel("e", "Code"), node},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			codes, residue := m.statusesFromConstructorField(tc.arg, tc.node)
			if codes != nil || residue {
				t.Errorf("guard %q should yield (nil,false); got (%v,%v)", tc.name, codes, residue)
			}
		})
	}
}

// TestConstructorFieldParam covers resolving a return composite-literal field to
// its source parameter (and the no-match / non-composite cases).
func TestConstructorFieldParam(t *testing.T) {
	meta := &metadata.Metadata{StringPool: metadata.NewStringPool()}

	kv := func(field, param string) *metadata.CallArgument {
		e := metadata.NewCallArgument(meta)
		e.SetKind(metadata.KindKeyValue)
		key := metadata.NewCallArgument(meta)
		key.SetKind(metadata.KindIdent)
		key.SetName(field)
		val := metadata.NewCallArgument(meta)
		val.SetKind(metadata.KindIdent)
		val.SetName(param)
		e.X, e.Fun = key, val
		return e
	}
	// return &APIError{Message: message, Code: code}
	lit := metadata.NewCallArgument(meta)
	lit.SetKind(metadata.KindCompositeLit)
	lit.Args = []*metadata.CallArgument{kv("Message", "message"), kv("Code", "code")}
	addr := metadata.NewCallArgument(meta)
	addr.SetKind(metadata.KindUnary)
	addr.X = lit
	ctor := &metadata.Function{ReturnVars: []metadata.CallArgument{*addr}}

	if got := constructorFieldParam(ctor, "Code"); got != "code" {
		t.Errorf(`constructorFieldParam(_, "Code") = %q, want "code"`, got)
	}
	if got := constructorFieldParam(ctor, "Missing"); got != "" {
		t.Errorf(`constructorFieldParam(_, "Missing") = %q, want ""`, got)
	}
	// A non-composite return contributes no field param.
	plain := &metadata.Function{ReturnVars: []metadata.CallArgument{*mkIdent(meta, "x", "")}}
	if got := constructorFieldParam(plain, "Code"); got != "" {
		t.Errorf(`non-composite return should yield "", got %q`, got)
	}
}

// TestAssignmentLookups covers the two canonical assignment entry points and the
// scope difference between them (issue #182): assignmentsAt / latestAssignment
// consult the call edge first then the enclosing function; latestCallerAssignment
// resolves the enclosing-function scope only and ignores the edge's own map.
func TestAssignmentLookups(t *testing.T) {
	meta := &metadata.Metadata{StringPool: metadata.NewStringPool()}
	impl := NewContextProvider(meta)

	rhs := httpStatusSelector(meta, "StatusOK")
	// Unused pooled int fields must be -1, not the zero value: index 0 is a valid
	// interned string ("http" here), so a zero Position/Scope would resolve to it.
	edge := &metadata.CallGraphEdge{
		Caller: metadata.Call{
			Name: meta.StringPool.Get("h"), Pkg: meta.StringPool.Get("app"),
			Position: -1, RecvType: -1, Scope: -1, SignatureStr: -1,
		},
		AssignmentMap: map[string][]metadata.Assignment{"x": {{
			Value:        rhs,
			VariableName: -1, Pkg: -1, ConcreteType: -1, Position: -1, Scope: -1, Func: -1,
		}}},
	}

	// Edge-map hit: no enclosing-function fallback needed.
	if got := assignmentsAt(impl, edge, "x"); len(got) != 1 {
		t.Fatalf("edge-map hit: got %d assignments, want 1", len(got))
	}
	// Guards.
	if assignmentsAt(impl, nil, "x") != nil {
		t.Error("nil edge must yield nil")
	}
	if assignmentsAt(impl, edge, "") != nil {
		t.Error("empty name must yield nil")
	}
	// Unknown var with no enclosing function registered -> nil.
	if assignmentsAt(impl, edge, "missing") != nil {
		t.Error("unknown var with no function scope must yield nil")
	}
	// latestAssignment returns the latest RHS of the edge assignment. The
	// edge-first lookup recovers an assignment recorded only on the call edge —
	// the case #189 relies on to resolve constructor-field statuses for error
	// variables assigned inside returned handler closures.
	if la := latestAssignment(impl, edge, "x"); la == nil || la.GetKind() != metadata.KindSelector {
		t.Errorf("latestAssignment should return the selector RHS, got %v", la)
	}
}

// TestStatusesFromConstructorField_EdgeOnlyAssignment proves the #189 behavior at
// the layer it changed: when the error variable's `e := NewAPIError(...)`
// assignment is recorded only on the call edge — as it is for a variable
// assigned inside a returned handler closure, where function-scope lookup
// (findFunctionByName / ParentFunction) cannot reach it — the edge-first
// assignmentsAt lookup still resolves the constructor-field status through to the
// branch set. The pre-#189 function-scope-only path missed it (the operation lost
// its concrete error statuses); this is the minimal isolation of the real-project
// gain, which a synthetic full-pipeline fixture cannot reproduce because clean
// code keeps the closure's locals on the enclosing method's scope.
func TestStatusesFromConstructorField_EdgeOnlyAssignment(t *testing.T) {
	meta := &metadata.Metadata{StringPool: metadata.NewStringPool()}

	// Scope function app.handler records the branch variable `statusCode` (set
	// across three branches) but NOT the error variable `e` — `e` lives only on
	// the call edge below, so function-scope resolution alone would miss it.
	asg := func(v metadata.CallArgument) metadata.Assignment {
		return metadata.Assignment{Value: v, VariableName: -1, Pkg: -1, ConcreteType: -1, Position: -1, Scope: -1, Func: -1}
	}
	handlerFn := &metadata.Function{AssignmentMap: map[string][]metadata.Assignment{
		"statusCode": {
			asg(httpStatusSelector(meta, "StatusNotFound")),
			asg(httpStatusSelector(meta, "StatusBadRequest")),
			asg(httpStatusSelector(meta, "StatusInternalServerError")),
		},
	}}

	// Constructor app.NewAPIError returning &APIError{Message: message, Code: code}.
	kv := func(field, param string) *metadata.CallArgument {
		e := metadata.NewCallArgument(meta)
		e.SetKind(metadata.KindKeyValue)
		key := mkIdent(meta, field, "")
		val := mkIdent(meta, param, "")
		e.X, e.Fun = key, val
		return e
	}
	lit := metadata.NewCallArgument(meta)
	lit.SetKind(metadata.KindCompositeLit)
	lit.Args = []*metadata.CallArgument{kv("Message", "message"), kv("Code", "code")}
	addr := metadata.NewCallArgument(meta)
	addr.SetKind(metadata.KindUnary)
	addr.X = lit
	ctorFn := &metadata.Function{ReturnVars: []metadata.CallArgument{*addr}}

	meta.Packages = map[string]*metadata.Package{
		"app": {Files: map[string]*metadata.File{
			"f": {Functions: map[string]*metadata.Function{"handler": handlerFn, "NewAPIError": ctorFn}},
		}},
	}

	caller := metadata.Call{
		Name: meta.StringPool.Get("handler"), Pkg: meta.StringPool.Get("app"),
		Position: -1, RecvType: -1, Scope: -1, SignatureStr: -1,
	}

	// The NewAPIError call, recorded as `e`'s edge-only assignment RHS.
	ctorCall := metadata.NewCallArgument(meta)
	ctorCall.SetKind(metadata.KindCall)
	ctorCall.Fun = mkIdent(meta, "NewAPIError", "")
	ctorCall.Position = meta.StringPool.Get("callpos")

	baseEdge := &metadata.CallGraphEdge{
		Caller: caller,
		AssignmentMap: map[string][]metadata.Assignment{
			"e": {{Value: *ctorCall, CalleeFunc: "NewAPIError", CalleePkg: "app",
				VariableName: -1, Pkg: -1, ConcreteType: -1, Position: -1, Scope: -1, Func: -1}},
		},
	}
	node := &fakeNode{edge: baseEdge}

	// The constructor call edge binds parameter `code` to the branch variable.
	meta.CallGraph = []metadata.CallGraphEdge{{
		Caller: caller,
		Callee: metadata.Call{
			Name: meta.StringPool.Get("NewAPIError"), Pkg: meta.StringPool.Get("app"),
			Position: -1, RecvType: -1, Scope: -1, SignatureStr: -1,
		},
		Position:    meta.StringPool.Get("callpos"),
		ParamArgMap: map[string]metadata.CallArgument{"code": *mkIdent(meta, "statusCode", "")},
	}}

	// arg = e.Code
	arg := metadata.NewCallArgument(meta)
	arg.SetKind(metadata.KindSelector)
	arg.X = mkIdent(meta, "e", "")
	arg.Sel = mkIdent(meta, "Code", "")

	m := branchStatusMatcher(meta)
	codes, residue := m.statusesFromConstructorField(arg, node)
	sort.Ints(codes)
	if want := []int{400, 404, 500}; len(codes) != 3 || codes[0] != want[0] || codes[1] != want[1] || codes[2] != want[2] {
		t.Errorf("edge-only assignment must still resolve the branch set: codes = %v, want %v", codes, want)
	}
	if residue {
		t.Error("all-constant branches must not report a residue")
	}
}

// TestCalleeNameOf covers same-package idents, cross-package selectors, and the
// empty cases.
func TestCalleeNameOf(t *testing.T) {
	meta := &metadata.Metadata{StringPool: metadata.NewStringPool()}
	if got := calleeNameOf(mkIdent(meta, "NewErr", "")); got != "NewErr" {
		t.Errorf("ident: got %q, want NewErr", got)
	}
	// pkg.NewErr — a selector whose name lives in .Sel.
	sel := metadata.NewCallArgument(meta)
	sel.SetKind(metadata.KindSelector)
	sel.X, sel.Sel = mkIdent(meta, "pkg", ""), mkIdent(meta, "NewErr", "")
	if got := calleeNameOf(sel); got != "NewErr" {
		t.Errorf("selector: got %q, want NewErr", got)
	}
	if got := calleeNameOf(nil); got != "" {
		t.Errorf("nil: got %q, want empty", got)
	}
	// A selector with no Sel yields empty.
	bare := metadata.NewCallArgument(meta)
	bare.SetKind(metadata.KindSelector)
	if got := calleeNameOf(bare); got != "" {
		t.Errorf("selector without Sel: got %q, want empty", got)
	}
}
