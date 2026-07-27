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

package metadata

import (
	"go/types"
	"reflect"
	"testing"
)

// cacheTestMeta builds metadata with three edges, two of which share a callee
// (name, pkg) — so "the first match" is a meaningful claim.
func cacheTestMeta() *Metadata {
	m := &Metadata{StringPool: NewStringPool()}
	edge := func(caller, callee, calleePkg string) CallGraphEdge {
		return CallGraphEdge{
			Caller: Call{Name: m.StringPool.Get(caller), Pkg: m.StringPool.Get("app"), Meta: m},
			Callee: Call{Name: m.StringPool.Get(callee), Pkg: m.StringPool.Get(calleePkg), Meta: m},
		}
	}
	m.CallGraph = []CallGraphEdge{
		edge("first", "target", "app"),  // the first edge whose callee is app.target
		edge("other", "elsewhere", "x"), // noise between the two matches
		edge("second", "target", "app"), // a later edge with the same callee
	}
	return m
}

// TestCalleeEdgesByNamePkg pins the contract that lets the index replace a linear
// scan of the whole call graph: same edges, same order, and no answer at all
// while the graph is still being built.
func TestCalleeEdgesByNamePkg(t *testing.T) {
	m := cacheTestMeta()
	name := m.StringPool.Get("target")
	pkg := m.StringPool.Get("app")

	t.Run("no answer before the index is built", func(t *testing.T) {
		// TraceVariableOrigin runs during call-graph construction, when no index
		// exists; those callers must fall back to scanning rather than see an
		// empty result.
		if _, ok := m.calleeEdgesByNamePkg(name, pkg); ok {
			t.Error("index answered before BuildCallGraphMaps")
		}
	})

	m.BuildCallGraphMaps()

	t.Run("matching edges in CallGraph order", func(t *testing.T) {
		edges, ok := m.calleeEdgesByNamePkg(name, pkg)
		if !ok {
			t.Fatal("index did not answer after BuildCallGraphMaps")
		}
		var callers []string
		for _, e := range edges {
			callers = append(callers, m.StringPool.GetString(e.Caller.Name))
		}
		// Order is the whole point: the caller takes the FIRST match, so the
		// bucket must list them exactly as a scan of CallGraph would.
		if want := []string{"first", "second"}; !reflect.DeepEqual(callers, want) {
			t.Errorf("callers = %v, want %v", callers, want)
		}
	})

	t.Run("equivalent to scanning the graph", func(t *testing.T) {
		// The property the optimisation rests on, checked directly rather than
		// argued: for every callee in the graph, the index yields what a scan
		// yields.
		for i := range m.CallGraph {
			c := &m.CallGraph[i].Callee
			var scanned []*CallGraphEdge
			for j := range m.CallGraph {
				e := &m.CallGraph[j]
				if e.Callee.Name == c.Name && e.Callee.Pkg == c.Pkg {
					scanned = append(scanned, e)
				}
			}
			indexed, ok := m.calleeEdgesByNamePkg(c.Name, c.Pkg)
			if !ok {
				t.Fatalf("index stopped answering for edge %d", i)
			}
			if !reflect.DeepEqual(indexed, scanned) {
				t.Errorf("edge %d: index and scan disagree", i)
			}
		}
	})

	t.Run("a grown graph invalidates the index", func(t *testing.T) {
		m.CallGraph = append(m.CallGraph, CallGraphEdge{
			Caller: Call{Name: m.StringPool.Get("late"), Pkg: m.StringPool.Get("app"), Meta: m},
			Callee: Call{Name: name, Pkg: pkg, Meta: m},
		})
		if _, ok := m.calleeEdgesByNamePkg(name, pkg); ok {
			t.Error("index answered for a graph it was not built for — it would miss the new edge")
		}
		m.BuildCallGraphMaps()
		edges, ok := m.calleeEdgesByNamePkg(name, pkg)
		if !ok || len(edges) != 3 {
			t.Errorf("after rebuild: ok=%v edges=%d, want ok and 3", ok, len(edges))
		}
	})

	t.Run("an unknown callee has no edges", func(t *testing.T) {
		edges, ok := m.calleeEdgesByNamePkg(m.StringPool.Get("nope"), pkg)
		if !ok || len(edges) != 0 {
			t.Errorf("unknown callee: ok=%v edges=%v", ok, edges)
		}
	})
}

// TestTypeStringOf covers the memoized type renderer, including the paths a
// caller can hit before metadata exists.
func TestTypeStringOf(t *testing.T) {
	m := &Metadata{StringPool: NewStringPool()}
	typ := types.Typ[types.String]

	if got := typeStringOf(m, nil); got != "" {
		t.Errorf("nil type = %q, want empty", got)
	}
	if got := typeStringOf(nil, typ); got != typ.String() {
		t.Errorf("nil metadata = %q, want %q", got, typ.String())
	}

	first := typeStringOf(m, typ)
	if first != typ.String() {
		t.Errorf("rendered %q, want %q", first, typ.String())
	}
	// Second call must come from the cache and be identical — the whole premise
	// is that rendering is a pure function of the type.
	if second := typeStringOf(m, typ); second != first {
		t.Errorf("cached call returned %q, want %q", second, first)
	}
	if len(m.typeStrCache) != 1 {
		t.Errorf("cache holds %d entries, want 1", len(m.typeStrCache))
	}

	// A different type gets its own entry rather than reusing one.
	if got := typeStringOf(m, types.Typ[types.Int]); got != "int" {
		t.Errorf("int rendered as %q", got)
	}
}

// TestImplementersOf covers the memoized implementers query: sorted order, and
// the guards.
func TestImplementersOf(t *testing.T) {
	m := &Metadata{StringPool: NewStringPool()}
	iface := "net/http.Handler"
	want := m.StringPool.Get(iface)
	withImpl := &Type{Implements: []int{want}}
	without := &Type{}

	// Two packages, each with an implementer and a non-implementer; the names are
	// chosen so map order and sorted order differ.
	m.Packages = map[string]*Package{
		"z/pkg": {Types: map[string]*Type{"Zebra": withImpl, "Nope": without}},
		"a/pkg": {Types: map[string]*Type{"Apple": withImpl, "Other": without}},
	}

	got := m.ImplementersOf(iface)
	expected := []string{"a/pkg.Apple", "z/pkg.Zebra"}
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("ImplementersOf = %v, want %v (sorted, implementers only)", got, expected)
	}
	// Cached call: same content, and the cache is what answers.
	if again := m.ImplementersOf(iface); !reflect.DeepEqual(again, expected) {
		t.Errorf("cached ImplementersOf = %v, want %v", again, expected)
	}
	if len(m.implementersCache) != 1 {
		t.Errorf("cache holds %d entries, want 1", len(m.implementersCache))
	}

	if got := m.ImplementersOf(""); got != nil {
		t.Errorf("empty key = %v, want nil", got)
	}
	if got := m.ImplementersOf("nobody.Implements.This"); got != nil {
		t.Errorf("unknown interface = %v, want nil", got)
	}
	var nilMeta *Metadata
	if got := nilMeta.ImplementersOf(iface); got != nil {
		t.Errorf("nil metadata = %v, want nil", got)
	}
}

// TestSortedFileNames covers the cached per-package file ordering — the order is
// load-bearing (lookups take the first file that declares a name), so this pins
// that caching cannot change it.
func TestSortedFileNames(t *testing.T) {
	m := &Metadata{StringPool: NewStringPool()}
	m.Packages = map[string]*Package{
		"app": {Files: map[string]*File{"z.go": {}, "a.go": {}, "m.go": {}}},
	}

	want := []string{"a.go", "m.go", "z.go"}
	if got := m.SortedFileNames("app"); !reflect.DeepEqual(got, want) {
		t.Errorf("SortedFileNames = %v, want %v", got, want)
	}
	if got := m.SortedFileNames("app"); !reflect.DeepEqual(got, want) {
		t.Errorf("cached SortedFileNames = %v, want %v", got, want)
	}

	// A package that gains a file must not keep serving the stale order — the
	// metadata is still being assembled while some lookups run.
	m.Packages["app"].Files["b.go"] = &File{}
	if got := m.SortedFileNames("app"); !reflect.DeepEqual(got, []string{"a.go", "b.go", "m.go", "z.go"}) {
		t.Errorf("after adding a file = %v, want the new name in sorted position", got)
	}

	if got := m.SortedFileNames("missing"); got != nil {
		t.Errorf("unknown package = %v, want nil", got)
	}
}
