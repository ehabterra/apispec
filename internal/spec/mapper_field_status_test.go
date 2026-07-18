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

// compositeLit builds `TypeName{args...}` as a KindCompositeLit CallArgument.
func compositeLit(meta *metadata.Metadata, typeName string, args ...*metadata.CallArgument) *metadata.CallArgument {
	lit := metadata.NewCallArgument(meta)
	lit.SetKind(metadata.KindCompositeLit)
	lit.X = mkIdent(meta, typeName, "")
	lit.Args = args
	return lit
}

// kvArg builds `field: val` as a KindKeyValue CallArgument.
func kvArg(meta *metadata.Metadata, field string, val *metadata.CallArgument) *metadata.CallArgument {
	kv := metadata.NewCallArgument(meta)
	kv.SetKind(metadata.KindKeyValue)
	kv.X = mkIdent(meta, field, "")
	kv.Fun = val
	return kv
}

// regType registers a struct type with the given field order (for positional
// composite resolution).
func regType(meta *metadata.Metadata, pkg, name string, fields ...string) {
	t := &metadata.Type{Name: meta.StringPool.Get(name)}
	for _, f := range fields {
		t.Fields = append(t.Fields, metadata.Field{Name: meta.StringPool.Get(f)})
	}
	if meta.Packages == nil {
		meta.Packages = map[string]*metadata.Package{}
	}
	if meta.Packages[pkg] == nil {
		meta.Packages[pkg] = &metadata.Package{Files: map[string]*metadata.File{}}
	}
	if meta.Packages[pkg].Files["f"] == nil {
		meta.Packages[pkg].Files["f"] = &metadata.File{Types: map[string]*metadata.Type{}, Functions: map[string]*metadata.Function{}}
	}
	meta.Packages[pkg].Files["f"].Types[name] = t
}

func TestStructFieldValue(t *testing.T) {
	meta := &metadata.Metadata{StringPool: metadata.NewStringPool()}
	impl := NewContextProvider(meta)
	regType(meta, "app", "APIError", "Status", "Message")
	scope := &metadata.Function{Pkg: meta.StringPool.Get("app")}
	st := httpStatusSelector(meta, "StatusNotFound")

	t.Run("keyed hit", func(t *testing.T) {
		lit := compositeLit(meta, "APIError", kvArg(meta, "Message", mkIdent(meta, "m", "")), kvArg(meta, "Status", &st))
		if got := structFieldValue(impl, lit, "Status", scope); got == nil || got.GetKind() != metadata.KindSelector {
			t.Errorf("keyed Status should resolve to the selector, got %v", got)
		}
	})
	t.Run("keyed miss", func(t *testing.T) {
		lit := compositeLit(meta, "APIError", kvArg(meta, "Message", mkIdent(meta, "m", "")))
		if got := structFieldValue(impl, lit, "Status", scope); got != nil {
			t.Errorf("absent keyed field must be nil, got %v", got)
		}
	})
	t.Run("positional", func(t *testing.T) {
		lit := compositeLit(meta, "APIError", &st, mkIdent(meta, "m", ""))
		got := structFieldValue(impl, lit, "Status", scope)
		if got == nil || got.GetKind() != metadata.KindSelector {
			t.Errorf("positional Status (field 0) should resolve, got %v", got)
		}
	})
	t.Run("positional empty -> nil", func(t *testing.T) {
		lit := compositeLit(meta, "APIError")
		if got := structFieldValue(impl, lit, "Status", scope); got != nil {
			t.Errorf("empty composite must yield nil, got %v", got)
		}
	})
	t.Run("field absent from type -> nil", func(t *testing.T) {
		lit := compositeLit(meta, "APIError", &st, mkIdent(meta, "m", ""))
		if got := structFieldValue(impl, lit, "Nonexistent", scope); got != nil {
			t.Errorf("field not in the type must yield nil, got %v", got)
		}
	})
	t.Run("nil type expr -> nil", func(t *testing.T) {
		lit := metadata.NewCallArgument(meta)
		lit.SetKind(metadata.KindCompositeLit)
		lit.Args = []*metadata.CallArgument{&st}
		if got := structFieldValue(impl, lit, "Status", scope); got != nil {
			t.Errorf("composite without a type expr must yield nil, got %v", got)
		}
	})
	t.Run("cross-package via findTypeAnywhere", func(t *testing.T) {
		// scope in pkg "other" (no APIError there) — the type is only in "app".
		otherScope := &metadata.Function{Pkg: meta.StringPool.Get("other")}
		lit := compositeLit(meta, "APIError", &st, mkIdent(meta, "m", ""))
		if got := structFieldValue(impl, lit, "Status", otherScope); got == nil {
			t.Error("positional field should resolve via the all-packages type fallback")
		}
	})
	t.Run("package-qualified positional type", func(t *testing.T) {
		regType(meta, "errs", "HTTPError", "Status", "Message")
		// lit type is `errs.HTTPError{...}` — a selector, not a bare ident.
		lit := metadata.NewCallArgument(meta)
		lit.SetKind(metadata.KindCompositeLit)
		qual := metadata.NewCallArgument(meta)
		qual.SetKind(metadata.KindSelector)
		qual.X, qual.Sel = mkIdent(meta, "errs", ""), mkIdent(meta, "HTTPError", "")
		lit.X = qual
		lit.Args = []*metadata.CallArgument{&st, mkIdent(meta, "m", "")}
		if got := structFieldValue(impl, lit, "Status", scope); got == nil {
			t.Error("pkg-qualified positional type should resolve the field")
		}
	})
}

func TestFindTypeAnywhere(t *testing.T) {
	meta := &metadata.Metadata{StringPool: metadata.NewStringPool()}
	regType(meta, "app", "APIError", "Status", "Message")
	if findTypeAnywhere(meta, "APIError") == nil {
		t.Error("APIError should be found across packages")
	}
	if findTypeAnywhere(meta, "Missing") != nil {
		t.Error("unknown type must be nil")
	}
	if findTypeAnywhere(nil, "APIError") != nil {
		t.Error("nil meta must be nil")
	}
}

func TestReturnFieldStatuses(t *testing.T) {
	meta := &metadata.Metadata{StringPool: metadata.NewStringPool()}
	m := branchStatusMatcher(meta)
	impl := m.contextProvider.(*ContextProviderImpl)
	regType(meta, "app", "APIError", "Status", "Message")

	st404 := httpStatusSelector(meta, "StatusNotFound")
	st500 := httpStatusSelector(meta, "StatusInternalServerError")
	// A mapper whose returns are two positional composites: 404 and 500.
	mapper := &metadata.Function{
		Pkg: meta.StringPool.Get("app"), Name: meta.StringPool.Get("MapError"),
		Returns: [][]metadata.CallArgument{
			{*compositeLit(meta, "APIError", &st404, mkIdent(meta, "m", ""))},
			{*compositeLit(meta, "APIError", &st500, mkIdent(meta, "m", ""))},
		},
	}
	set := map[int]bool{}
	residue := m.returnFieldStatuses(impl, mapper, "Status", set, map[string]bool{}, 0)
	if residue {
		t.Error("all-constant returns must not report residue")
	}
	got := keysOfIntSet(set)
	if len(got) != 2 || got[0] != 404 || got[1] != 500 {
		t.Errorf("got %v, want [404 500]", got)
	}

	t.Run("depth cap -> residue", func(t *testing.T) {
		s := map[int]bool{}
		if !m.returnFieldStatuses(impl, mapper, "Status", s, map[string]bool{}, maxMapperDepth+1) {
			t.Error("exceeding depth must report residue")
		}
	})
	t.Run("nil fn", func(t *testing.T) {
		if m.returnFieldStatuses(impl, nil, "Status", map[int]bool{}, map[string]bool{}, 0) {
			t.Error("nil fn must not be residue")
		}
	})
	t.Run("cycle guard", func(t *testing.T) {
		seen := map[string]bool{"app.MapError": true}
		if m.returnFieldStatuses(impl, mapper, "Status", map[int]bool{}, seen, 0) {
			t.Error("already-seen fn must short-circuit without residue")
		}
	})
}

func TestFieldStatusOfValue(t *testing.T) {
	meta := &metadata.Metadata{StringPool: metadata.NewStringPool()}
	m := branchStatusMatcher(meta)
	impl := m.contextProvider.(*ContextProviderImpl)
	regType(meta, "app", "APIError", "Status", "Message")
	scope := &metadata.Function{Pkg: meta.StringPool.Get("app")}

	t.Run("empty composite -> nothing, no residue", func(t *testing.T) {
		set := map[int]bool{}
		if m.fieldStatusOfValue(impl, compositeLit(meta, "APIError"), "Status", scope, set, map[string]bool{}, 0) {
			t.Error("empty composite must not be residue")
		}
		if len(set) != 0 {
			t.Errorf("empty composite must add nothing, got %v", set)
		}
	})
	t.Run("non-constant field -> residue", func(t *testing.T) {
		dyn := metadata.NewCallArgument(meta)
		dyn.SetKind(metadata.KindCall)
		dyn.Fun = mkIdent(meta, "computeStatus", "")
		lit := compositeLit(meta, "APIError", dyn, mkIdent(meta, "m", ""))
		if !m.fieldStatusOfValue(impl, lit, "Status", scope, map[int]bool{}, map[string]bool{}, 0) {
			t.Error("non-constant field value must be residue")
		}
	})
	t.Run("nil value", func(t *testing.T) {
		if m.fieldStatusOfValue(impl, nil, "Status", scope, map[int]bool{}, map[string]bool{}, 0) {
			t.Error("nil value must not be residue")
		}
	})
	t.Run("ident resolving through a helper call", func(t *testing.T) {
		st := httpStatusSelector(meta, "StatusBadRequest")
		regType(meta, "app", "APIError", "Status", "Message")
		meta.Packages["app"].Files["f"].Functions["mapAs400"] = &metadata.Function{
			Pkg: meta.StringPool.Get("app"), Name: meta.StringPool.Get("mapAs400"),
			Returns: [][]metadata.CallArgument{{*compositeLit(meta, "APIError", &st, mkIdent(meta, "m", ""))}},
		}
		call := metadata.NewCallArgument(meta)
		call.SetKind(metadata.KindCall)
		call.Fun = mkIdent(meta, "mapAs400", "")
		fnScope := &metadata.Function{
			Pkg: meta.StringPool.Get("app"), Name: meta.StringPool.Get("MapError"),
			AssignmentMap: map[string][]metadata.Assignment{
				"api": {{Value: *call, CalleeFunc: "mapAs400", CalleePkg: "app"}},
			},
		}
		set := map[int]bool{}
		m.fieldStatusOfValue(impl, mkIdent(meta, "api", ""), "Status", fnScope, set, map[string]bool{}, 0)
		if !set[400] {
			t.Errorf("returned ident should resolve through the helper to 400, got %v", set)
		}
	})
}

func TestStatusesFromMapperField_Guards(t *testing.T) {
	meta := &metadata.Metadata{StringPool: metadata.NewStringPool()}
	m := branchStatusMatcher(meta)
	node := &fakeNode{edge: &metadata.CallGraphEdge{Caller: metadata.Call{Name: meta.StringPool.Get("h"), Pkg: meta.StringPool.Get("app")}}}

	sel := func(base, field string) *metadata.CallArgument {
		a := metadata.NewCallArgument(meta)
		a.SetKind(metadata.KindSelector)
		a.X, a.Sel = mkIdent(meta, base, ""), mkIdent(meta, field, "")
		return a
	}
	cases := []struct {
		name string
		arg  *metadata.CallArgument
	}{
		{"nil arg", nil},
		{"non-selector", mkIdent(meta, "x", "")},
		{"selector, no producing assignment", sel("api", "Status")},
	}
	// A base assigned from a call to an unknown function: the mapper can't be
	// located, so nothing resolves.
	node.edge.AssignmentMap = map[string][]metadata.Assignment{}
	{
		call := metadata.NewCallArgument(meta)
		call.SetKind(metadata.KindCall)
		call.Fun = mkIdent(meta, "NoSuchMapper", "")
		node.edge.AssignmentMap["api"] = []metadata.Assignment{{Value: *call, CalleeFunc: "NoSuchMapper", CalleePkg: "app"}}
	}
	if codes, residue := m.statusesFromMapperField(sel("api", "Status"), node); codes != nil || residue {
		t.Errorf("unknown mapper must yield (nil,false); got (%v,%v)", codes, nil)
	}
	node.edge.AssignmentMap = nil
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if codes, residue := m.statusesFromMapperField(tc.arg, node); codes != nil || residue {
				t.Errorf("guard %q must yield (nil,false); got (%v,%v)", tc.name, codes, residue)
			}
		})
	}
}

// TestStatusesFromMapperField_Resolves drives the full resolver: a selector
// api.Status whose base is assigned from a mapper call on the edge, through to
// the mapper's composite returns.
func TestStatusesFromMapperField_Resolves(t *testing.T) {
	meta := &metadata.Metadata{StringPool: metadata.NewStringPool()}
	m := branchStatusMatcher(meta)
	regType(meta, "app", "APIError", "Status", "Message")
	st404 := httpStatusSelector(meta, "StatusNotFound")
	st500 := httpStatusSelector(meta, "StatusInternalServerError")
	meta.Packages["app"].Files["f"].Functions["MapError"] = &metadata.Function{
		Pkg: meta.StringPool.Get("app"), Name: meta.StringPool.Get("MapError"),
		Returns: [][]metadata.CallArgument{
			{*compositeLit(meta, "APIError", &st404, mkIdent(meta, "m", ""))},
			{*compositeLit(meta, "APIError", &st500, mkIdent(meta, "m", ""))},
		},
	}
	call := metadata.NewCallArgument(meta)
	call.SetKind(metadata.KindCall)
	call.Fun = mkIdent(meta, "MapError", "")
	edge := &metadata.CallGraphEdge{
		Caller: metadata.Call{Name: meta.StringPool.Get("writeError"), Pkg: meta.StringPool.Get("app")},
		AssignmentMap: map[string][]metadata.Assignment{
			"api": {{Value: *call, CalleeFunc: "MapError", CalleePkg: "app"}},
		},
	}
	node := &fakeNode{edge: edge}
	arg := metadata.NewCallArgument(meta)
	arg.SetKind(metadata.KindSelector)
	arg.X, arg.Sel = mkIdent(meta, "api", ""), mkIdent(meta, "Status", "")

	codes, residue := m.statusesFromMapperField(arg, node)
	if residue {
		t.Error("all-constant mapper returns must not report residue")
	}
	if len(codes) != 2 || codes[0] != 404 || codes[1] != 500 {
		t.Errorf("codes = %v, want [404 500]", codes)
	}

	// Inline base: MapError(err).Status (no intermediate variable).
	t.Run("inline mapper call base", func(t *testing.T) {
		inlineCall := metadata.NewCallArgument(meta)
		inlineCall.SetKind(metadata.KindCall)
		inlineCall.Fun = mkIdent(meta, "MapError", "")
		sel := metadata.NewCallArgument(meta)
		sel.SetKind(metadata.KindSelector)
		sel.X, sel.Sel = inlineCall, mkIdent(meta, "Status", "")
		got, res := m.statusesFromMapperField(sel, node)
		if res || len(got) != 2 || got[0] != 404 || got[1] != 500 {
			t.Errorf("inline base: got (%v,%v), want ([404 500],false)", got, res)
		}
	})
}

// TestFieldStatusOfValue_MoreShapes covers the direct-helper-call return and a
// returned ident bound to a composite (not a call).
func TestFieldStatusOfValue_MoreShapes(t *testing.T) {
	meta := &metadata.Metadata{StringPool: metadata.NewStringPool()}
	m := branchStatusMatcher(meta)
	impl := m.contextProvider.(*ContextProviderImpl)
	regType(meta, "app", "APIError", "Status", "Message")
	scope := &metadata.Function{Pkg: meta.StringPool.Get("app")}

	t.Run("direct return of a helper call", func(t *testing.T) {
		st := httpStatusSelector(meta, "StatusBadRequest")
		meta.Packages["app"].Files["f"].Functions["mapAsX"] = &metadata.Function{
			Pkg: meta.StringPool.Get("app"), Name: meta.StringPool.Get("mapAsX"),
			Returns: [][]metadata.CallArgument{{*compositeLit(meta, "APIError", &st, mkIdent(meta, "m", ""))}},
		}
		call := metadata.NewCallArgument(meta)
		call.SetKind(metadata.KindCall)
		call.Fun = mkIdent(meta, "mapAsX", "")
		set := map[int]bool{}
		m.fieldStatusOfValue(impl, call, "Status", scope, set, map[string]bool{}, 0)
		if !set[400] {
			t.Errorf("direct helper-call return should resolve to 400, got %v", set)
		}
	})

	t.Run("returned ident bound to a composite", func(t *testing.T) {
		st := httpStatusSelector(meta, "StatusNotFound")
		fnScope := &metadata.Function{
			Pkg: meta.StringPool.Get("app"),
			AssignmentMap: map[string][]metadata.Assignment{
				"e": {{Value: *compositeLit(meta, "APIError", &st, mkIdent(meta, "m", ""))}},
			},
		}
		set := map[int]bool{}
		m.fieldStatusOfValue(impl, mkIdent(meta, "e", ""), "Status", fnScope, set, map[string]bool{}, 0)
		if !set[404] {
			t.Errorf("ident bound to a composite should resolve to 404, got %v", set)
		}
	})
}

func keysOfIntSet(s map[int]bool) []int {
	out := make([]int, 0, len(s))
	for k := range s {
		out = append(out, k)
	}
	sort.Ints(out)
	return out
}
