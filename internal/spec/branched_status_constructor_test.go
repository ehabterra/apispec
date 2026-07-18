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
// fallback, and the no-match case.
func TestFindCallEdge(t *testing.T) {
	meta := &metadata.Metadata{StringPool: metadata.NewStringPool()}
	impl := NewContextProvider(meta)
	caller := metadata.Call{Name: meta.StringPool.Get("h"), Pkg: meta.StringPool.Get("app")}
	e := func(pos string) metadata.CallGraphEdge {
		return metadata.CallGraphEdge{
			Caller:   caller,
			Callee:   metadata.Call{Name: meta.StringPool.Get("New")},
			Position: meta.StringPool.Get(pos),
		}
	}
	meta.CallGraph = []metadata.CallGraphEdge{e("pos1"), e("pos2")}
	callerID := caller.ID()

	if got := findCallEdge(impl, callerID, "New", "pos2"); got == nil || impl.GetString(got.Position) != "pos2" {
		t.Errorf("valPos match should return the pos2 edge, got %v", got)
	}
	if got := findCallEdge(impl, callerID, "New", ""); got == nil || impl.GetString(got.Position) != "pos1" {
		t.Errorf("no valPos should return the first match (pos1), got %v", got)
	}
	if got := findCallEdge(impl, callerID, "New", "nope"); got == nil || impl.GetString(got.Position) != "pos1" {
		t.Errorf("unmatched valPos should fall back to the first match, got %v", got)
	}
	if got := findCallEdge(impl, callerID, "Missing", ""); got != nil {
		t.Errorf("unknown callee should return nil, got %v", got)
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
	// latestAssignment returns the latest RHS of the edge assignment.
	if la := latestAssignment(impl, edge, "x"); la == nil || la.GetKind() != metadata.KindSelector {
		t.Errorf("latestAssignment should return the selector RHS, got %v", la)
	}
	// latestCallerAssignment is function-scope only: it ignores the edge's own
	// map, so with no enclosing function it reports ok=false even though the edge
	// records `x`.
	if _, ok := latestCallerAssignment(impl, edge, "x"); ok {
		t.Error("function-scope lookup must ignore the edge map (ok=false)")
	}
}
