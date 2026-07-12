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
	"reflect"
	"testing"
)

// metaWithEdges builds a minimal Metadata whose call graph has one edge per
// (caller, callee) pair, all in package "p", so BaseIDs are "p.<name>".
func metaWithEdges(edges [][2]string) *Metadata {
	m := &Metadata{StringPool: NewStringPool()}
	// RecvType/Position/Scope zero-values are valid pool indexes; -1 is the
	// "unset" sentinel used by real metadata generation.
	call := func(name string) Call {
		return Call{
			Meta: m, Name: m.StringPool.Get(name), Pkg: m.StringPool.Get("p"),
			RecvType: -1, Position: -1, Scope: -1, SignatureStr: -1,
		}
	}
	for _, e := range edges {
		m.CallGraph = append(m.CallGraph, CallGraphEdge{Caller: call(e[0]), Callee: call(e[1])})
	}
	return m
}

// assertCalleesFirst checks the documented order invariant: for every call
// edge that crosses components, the callee's component index is smaller than
// the caller's (callees before callers).
func assertCalleesFirst(t *testing.T, s *CallGraphSCC, edges [][2]string) {
	t.Helper()
	for _, e := range edges {
		cu, cv := s.ComponentOf["p."+e[0]], s.ComponentOf["p."+e[1]]
		if cu != cv && cv > cu {
			t.Errorf("edge %s->%s: callee component %d not before caller component %d", e[0], e[1], cv, cu)
		}
	}
}

func TestSCC_DiamondDAG(t *testing.T) {
	edges := [][2]string{{"a", "b"}, {"a", "c"}, {"b", "d"}, {"c", "d"}}
	s := BuildCallGraphSCC(metaWithEdges(edges))

	if len(s.Components) != 4 {
		t.Fatalf("components = %d, want 4 singletons: %v", len(s.Components), s.Components)
	}
	for c := range s.Components {
		if s.Recursive[c] {
			t.Errorf("component %v marked recursive in a DAG", s.Components[c])
		}
	}
	assertCalleesFirst(t, s, edges)
	// d is the only sink: it must be finalized first.
	if got := s.Components[0]; !reflect.DeepEqual(got, []string{"p.d"}) {
		t.Errorf("first (most-callee) component = %v, want [p.d]", got)
	}
	// a is the only source: it must come last.
	if got := s.Components[len(s.Components)-1]; !reflect.DeepEqual(got, []string{"p.a"}) {
		t.Errorf("last (most-caller) component = %v, want [p.a]", got)
	}
}

func TestSCC_CycleCondenses(t *testing.T) {
	edges := [][2]string{{"a", "b"}, {"b", "c"}, {"c", "a"}, {"c", "d"}}
	s := BuildCallGraphSCC(metaWithEdges(edges))

	if len(s.Components) != 2 {
		t.Fatalf("components = %v, want [[p.d] [p.a p.b p.c]]", s.Components)
	}
	if !reflect.DeepEqual(s.Components[0], []string{"p.d"}) {
		t.Errorf("component 0 = %v, want [p.d] (callee first)", s.Components[0])
	}
	if !reflect.DeepEqual(s.Components[1], []string{"p.a", "p.b", "p.c"}) {
		t.Errorf("component 1 = %v, want [p.a p.b p.c]", s.Components[1])
	}
	if !s.Recursive[1] || s.Recursive[0] {
		t.Errorf("recursive flags = %v, want cycle component only", s.Recursive)
	}
	if !s.SameComponent("p.a", "p.c") || s.SameComponent("p.a", "p.d") {
		t.Error("SameComponent misclassifies cycle membership")
	}
	if !s.InCycle("p.b") || s.InCycle("p.d") {
		t.Error("InCycle misclassifies")
	}
	// Condensed DAG: cycle component -> d's component, and acyclic.
	if !reflect.DeepEqual(s.DAG[1], []int{0}) || len(s.DAG[0]) != 0 {
		t.Errorf("DAG = %v, want [[] [0]]", s.DAG)
	}
}

func TestSCC_SelfLoop(t *testing.T) {
	s := BuildCallGraphSCC(metaWithEdges([][2]string{{"f", "f"}, {"g", "f"}}))
	if !s.InCycle("p.f") {
		t.Error("self-recursive f not marked in cycle")
	}
	if s.InCycle("p.g") {
		t.Error("g wrongly marked in cycle")
	}
}

func TestSCC_UnknownID(t *testing.T) {
	s := BuildCallGraphSCC(metaWithEdges([][2]string{{"a", "b"}}))
	if s.InCycle("p.zzz") || s.SameComponent("p.a", "p.zzz") {
		t.Error("unknown IDs must be in no component")
	}
}

func TestSCC_Deterministic(t *testing.T) {
	// The same logical graph, inserted in different edge orders, must yield
	// byte-identical condensations (component order, numbering, DAG).
	base := [][2]string{
		{"main", "h1"}, {"main", "h2"}, {"h1", "util"}, {"h2", "util"},
		{"util", "log"}, {"h1", "rec1"}, {"rec1", "rec2"}, {"rec2", "rec1"},
		{"rec2", "log"}, {"h2", "h2"},
	}
	perms := [][]int{
		{0, 1, 2, 3, 4, 5, 6, 7, 8, 9},
		{9, 8, 7, 6, 5, 4, 3, 2, 1, 0},
		{4, 0, 7, 2, 9, 5, 1, 8, 3, 6},
	}
	var first *CallGraphSCC
	for _, perm := range perms {
		edges := make([][2]string, len(base))
		for i, p := range perm {
			edges[i] = base[p]
		}
		s := BuildCallGraphSCC(metaWithEdges(edges))
		if first == nil {
			first = s
			assertCalleesFirst(t, s, base)
			continue
		}
		if !reflect.DeepEqual(first, s) {
			t.Errorf("condensation differs across edge insertion orders:\n%v\nvs\n%v", first, s)
		}
	}
}
