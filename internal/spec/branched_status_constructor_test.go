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
