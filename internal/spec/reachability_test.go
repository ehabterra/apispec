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
	"fmt"
	"testing"

	"github.com/ehabterra/apispec/internal/metadata"
)

// reachMeta builds minimal metadata whose call graph has one edge per
// (callerPkg, caller, calleePkg, callee) tuple.
func reachMeta(edges [][4]string) *metadata.Metadata {
	m := &metadata.Metadata{StringPool: metadata.NewStringPool()}
	call := func(pkg, name string) metadata.Call {
		return metadata.Call{
			Meta: m, Name: m.StringPool.Get(name), Pkg: m.StringPool.Get(pkg),
			RecvType: -1, Position: -1, Scope: -1, SignatureStr: -1,
		}
	}
	for _, e := range edges {
		m.CallGraph = append(m.CallGraph, metadata.CallGraphEdge{
			Caller: call(e[0], e[1]),
			Callee: call(e[2], e[3]),
		})
	}
	m.BuildCallGraphMaps()
	return m
}

var muxVarsPattern = ParamPattern{
	CallRegex:      "^Vars$",
	RecvTypeRegex:  `^github\.com/gorilla/mux$`,
	NameFromMapKey: true,
}

// TestHandlerReachesAccessor_DeepChain locks in the step-3 improvement: the
// old walk was bounded at maxWrapperLookThroughDepth (6), so an accessor
// behind a deeper helper chain was invisible. The summary has no bound.
func TestHandlerReachesAccessor_DeepChain(t *testing.T) {
	const pkg = "example.com/app"
	const depth = 12 // comfortably past the old cap
	var edges [][4]string
	edges = append(edges, [4]string{pkg, "handler", pkg, "w1"})
	for i := 1; i < depth; i++ {
		edges = append(edges, [4]string{pkg, fmt.Sprintf("w%d", i), pkg, fmt.Sprintf("w%d", i+1)})
	}
	edges = append(edges, [4]string{pkg, fmt.Sprintf("w%d", depth), "github.com/gorilla/mux", "Vars"})

	e := &Extractor{}
	route := &RouteInfo{Metadata: reachMeta(edges), Function: pkg + ".handler", Package: pkg}
	if !e.handlerReachesAccessor(route, muxVarsPattern) {
		t.Errorf("accessor behind a %d-deep helper chain not reached (old cap was 6)", depth)
	}
}

// TestHandlerReachesAccessor_Recursion: mutually recursive helpers form an
// SCC; a match reachable from the cluster marks the whole cluster, with no
// depth accounting.
func TestHandlerReachesAccessor_Recursion(t *testing.T) {
	const pkg = "example.com/app"
	meta := reachMeta([][4]string{
		{pkg, "handler", pkg, "walkA"},
		{pkg, "walkA", pkg, "walkB"},
		{pkg, "walkB", pkg, "walkA"}, // cycle
		{pkg, "walkB", "github.com/gorilla/mux", "Vars"},
		{pkg, "lonely", pkg, "leaf"}, // control: no path to the accessor
	})

	e := &Extractor{}
	if !e.handlerReachesAccessor(&RouteInfo{Metadata: meta, Function: pkg + ".handler", Package: pkg}, muxVarsPattern) {
		t.Error("accessor behind a recursion cluster not reached")
	}
	if e.handlerReachesAccessor(&RouteInfo{Metadata: meta, Function: pkg + ".lonely", Package: pkg}, muxVarsPattern) {
		t.Error("false positive: lonely has no path to the accessor")
	}
}

// TestReachSet_CachedPerPattern: the summary is computed once per pattern and
// reused across routes.
func TestReachSet_CachedPerPattern(t *testing.T) {
	const pkg = "example.com/app"
	meta := reachMeta([][4]string{
		{pkg, "h1", "github.com/gorilla/mux", "Vars"},
		{pkg, "h2", pkg, "noop"},
	})

	e := &Extractor{}
	calls := 0
	key := "test-key"
	set1 := e.reachSet(meta, key, func(edge *metadata.CallGraphEdge) bool {
		calls++
		return edgeMatchesAccessor(meta, edge, muxVarsPattern)
	})
	evals := calls
	set2 := e.reachSet(meta, key, func(*metadata.CallGraphEdge) bool {
		calls++
		return false // must not be consulted: cached
	})
	if calls != evals {
		t.Error("second reachSet call re-evaluated the predicate instead of using the cache")
	}
	if !set1[pkg+".h1"] || set1[pkg+".h2"] {
		t.Errorf("reach set wrong: %v", set1)
	}
	if len(set2) != len(set1) {
		t.Error("cached set differs")
	}
}
