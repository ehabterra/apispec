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

// covspecFuncNode builds a function TrackerNode (edge-backed) with the given key.
func covspecFuncNode(meta *metadata.Metadata, key, name, pkg, recv string) *TrackerNode {
	sp := meta.StringPool
	callee := metadata.Call{
		Meta:     meta,
		Name:     sp.Get(name),
		Pkg:      sp.Get(pkg),
		RecvType: sp.Get(recv),
		Scope:    -1,
		Position: -1,
	}
	edge := &metadata.CallGraphEdge{Callee: callee}
	return &TrackerNode{key: key, CallGraphEdge: edge}
}

// TestCovspecDrawNodeCytoscapeWithDepth exercises the tracker-tree drawing
// function across node types, merging, edges, and argument/generic branches.
func TestCovspecDrawNodeCytoscapeWithDepth(t *testing.T) {
	meta := newTestMeta()
	sp := meta.StringPool

	// Root function node (main).
	root := covspecFuncNode(meta, "main.main@f1", "main", "main", "")

	// Child function node with a receiver type.
	child := covspecFuncNode(meta, "svc.Handler.Serve@f2", "Serve", "svc", "Handler")
	child.RootAssignmentMap = map[string][]metadata.Assignment{
		"x": {{VariableName: sp.Get("x")}},
	}

	// A second occurrence of the same base key (different position) to exercise
	// the merge path.
	childDup := covspecFuncNode(meta, "svc.Handler.Serve@f3", "Serve", "svc", "Handler")

	// Argument node.
	argCA := metadata.NewCallArgument(meta)
	argCA.SetKind(metadata.KindIdent)
	argCA.SetName("payload")
	argCA.SetType("svc.Payload")
	argCA.SetPkg("svc")
	argNode := &TrackerNode{
		key:          "svc.payload@a1",
		IsArgument:   true,
		ArgType:      ArgTypeVariable,
		ArgIndex:     0,
		ArgContext:   "body",
		CallArgument: argCA,
	}

	// Call-argument node (CallArgument set, no edge, not IsArgument).
	callArgCA := metadata.NewCallArgument(meta)
	callArgCA.SetKind(metadata.KindIdent)
	callArgCA.SetName("cfg")
	callArgNode := &TrackerNode{
		key:          "svc.cfg@c1",
		CallArgument: callArgCA,
	}

	// Generic node (no edge, no argument).
	genNode := &TrackerNode{key: "svc.generic@g1"}

	child.Children = []*TrackerNode{argNode, callArgNode, genNode}
	root.Children = []*TrackerNode{child, childDup}

	data := DrawTrackerTreeCytoscapeWithMetadata(
		[]TrackerNodeInterface{root, nil}, meta,
	)

	if len(data.Nodes) == 0 {
		t.Fatal("expected nodes")
	}
	if len(data.Edges) == 0 {
		t.Fatal("expected edges")
	}

	// Verify node types were assigned.
	types := map[string]int{}
	for _, n := range data.Nodes {
		types[n.Data.Type]++
	}
	if types["function"] == 0 {
		t.Errorf("expected a function node, got types=%v", types)
	}
	if types["argument"] == 0 {
		t.Errorf("expected an argument node, got types=%v", types)
	}
	if types["generic"] == 0 {
		t.Errorf("expected a generic node, got types=%v", types)
	}

	// The two Serve occurrences must have merged into one node.
	serveCount := 0
	for _, n := range data.Nodes {
		if n.Data.FunctionName == "Serve" {
			serveCount++
		}
	}
	if serveCount != 1 {
		t.Errorf("expected merged Serve node, got %d", serveCount)
	}
}

// TestCovspecDrawNodeNilMetadataFallback exercises the fallback string-index
// labelling when metadata has no string pool available at draw time.
func TestCovspecDrawNodeNilMetadataFallback(t *testing.T) {
	meta := newTestMeta()
	root := covspecFuncNode(meta, "main.main@f1", "main", "main", "")

	// Pass nil metadata so the func node uses the index-based fallback labels.
	data := DrawTrackerTreeCytoscapeWithMetadata([]TrackerNodeInterface{root}, nil)
	if len(data.Nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(data.Nodes))
	}
}

// TestCovspecOrderTrackerTreeNodesDepthFirst covers root selection, DFS
// ordering, and orphan handling.
func TestCovspecOrderTrackerTreeNodesDepthFirst(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		data := &CytoscapeData{}
		if got := OrderTrackerTreeNodesDepthFirst(data); len(got) != 0 {
			t.Fatalf("expected empty, got %d", len(got))
		}
	})

	t.Run("main root with children and orphan", func(t *testing.T) {
		data := &CytoscapeData{
			Nodes: []CytoscapeNode{
				{Data: CytoscapeNodeData{ID: "node_0", Label: "main", Depth: 0}},
				{Data: CytoscapeNodeData{ID: "node_1", Label: "a", Depth: 1}},
				{Data: CytoscapeNodeData{ID: "node_2", Label: "b", Depth: 2}},
				{Data: CytoscapeNodeData{ID: "node_9", Label: "orphan", Depth: 5}},
			},
			Edges: []CytoscapeEdge{
				{Data: CytoscapeEdgeData{Source: "node_0", Target: "node_1", Type: "calls"}},
				{Data: CytoscapeEdgeData{Source: "node_1", Target: "node_2", Type: "calls"}},
			},
		}
		got := OrderTrackerTreeNodesDepthFirst(data)
		if len(got) != 4 {
			t.Fatalf("expected 4 nodes, got %d", len(got))
		}
		if got[0].Data.ID != "node_0" {
			t.Errorf("expected main first, got %s", got[0].Data.ID)
		}
		// Orphan appended last.
		if got[len(got)-1].Data.Label != "orphan" {
			t.Errorf("expected orphan last, got %s", got[len(got)-1].Data.Label)
		}
	})

	t.Run("no main uses other roots", func(t *testing.T) {
		data := &CytoscapeData{
			Nodes: []CytoscapeNode{
				{Data: CytoscapeNodeData{ID: "node_5", Label: "handler", Depth: 0}},
				{Data: CytoscapeNodeData{ID: "node_6", Label: "helper", Depth: 1}},
			},
			Edges: []CytoscapeEdge{
				{Data: CytoscapeEdgeData{Source: "node_5", Target: "node_6", Type: "calls"}},
			},
		}
		got := OrderTrackerTreeNodesDepthFirst(data)
		if len(got) != 2 || got[0].Data.ID != "node_5" {
			t.Fatalf("unexpected order %+v", got)
		}
	})

	t.Run("no depth-0 roots falls back to min depth", func(t *testing.T) {
		data := &CytoscapeData{
			Nodes: []CytoscapeNode{
				{Data: CytoscapeNodeData{ID: "node_0", Label: "main", Depth: 3}},
				{Data: CytoscapeNodeData{ID: "node_1", Label: "x", Depth: 3}},
			},
			Edges: []CytoscapeEdge{
				// Both nodes have incoming edges so neither is a "root" by the
				// no-incoming rule; min-depth fallback must engage.
				{Data: CytoscapeEdgeData{Source: "node_1", Target: "node_0", Type: "calls"}},
				{Data: CytoscapeEdgeData{Source: "node_0", Target: "node_1", Type: "calls"}},
			},
		}
		got := OrderTrackerTreeNodesDepthFirst(data)
		if len(got) != 2 {
			t.Fatalf("expected 2 nodes, got %d", len(got))
		}
	})
}

// TestCovspecTraverseTrackerTreeBranchOrder covers branch-order traversal.
func TestCovspecTraverseTrackerTreeBranchOrder(t *testing.T) {
	t.Run("empty returns nil", func(t *testing.T) {
		if got := TraverseTrackerTreeBranchOrder(&CytoscapeData{}); got != nil {
			t.Fatalf("expected nil, got %+v", got)
		}
	})

	t.Run("main first, argument edges ignored, orphan appended", func(t *testing.T) {
		data := &CytoscapeData{
			Nodes: []CytoscapeNode{
				{Data: CytoscapeNodeData{ID: "node_0", Label: "main", Depth: 0}},
				{Data: CytoscapeNodeData{ID: "node_1", Label: "a", Depth: 1}},
				{Data: CytoscapeNodeData{ID: "node_2", Label: "b", Depth: 2}},
				{Data: CytoscapeNodeData{ID: "node_7", Label: "orphan", Depth: 4}},
			},
			Edges: []CytoscapeEdge{
				{Data: CytoscapeEdgeData{Source: "node_0", Target: "node_1", Type: "calls"}},
				{Data: CytoscapeEdgeData{Source: "node_1", Target: "node_2", Type: "calls"}},
				// Argument edge must be ignored for branch traversal.
				{Data: CytoscapeEdgeData{Source: "node_2", Target: "node_7", Type: "argument"}},
			},
		}
		got := TraverseTrackerTreeBranchOrder(data)
		if len(got) != 4 {
			t.Fatalf("expected 4 nodes, got %d", len(got))
		}
		if got[0].Data.Label != "main" {
			t.Errorf("expected main first, got %s", got[0].Data.Label)
		}
		if got[len(got)-1].Data.Label != "orphan" {
			t.Errorf("expected orphan last, got %s", got[len(got)-1].Data.Label)
		}
	})

	t.Run("no main, other roots", func(t *testing.T) {
		data := &CytoscapeData{
			Nodes: []CytoscapeNode{
				{Data: CytoscapeNodeData{ID: "node_3", Label: "h", Depth: 0}},
				{Data: CytoscapeNodeData{ID: "node_4", Label: "k", Depth: 1}},
			},
			Edges: []CytoscapeEdge{
				{Data: CytoscapeEdgeData{Source: "node_3", Target: "node_4", Type: "calls"}},
			},
		}
		got := TraverseTrackerTreeBranchOrder(data)
		if len(got) != 2 || got[0].Data.ID != "node_3" {
			t.Fatalf("unexpected %+v", got)
		}
	})

	t.Run("min-depth fallback when all have incoming", func(t *testing.T) {
		data := &CytoscapeData{
			Nodes: []CytoscapeNode{
				{Data: CytoscapeNodeData{ID: "node_0", Label: "main", Depth: 2}},
				{Data: CytoscapeNodeData{ID: "node_1", Label: "y", Depth: 2}},
			},
			Edges: []CytoscapeEdge{
				{Data: CytoscapeEdgeData{Source: "node_1", Target: "node_0", Type: "calls"}},
				{Data: CytoscapeEdgeData{Source: "node_0", Target: "node_1", Type: "calls"}},
			},
		}
		got := TraverseTrackerTreeBranchOrder(data)
		if len(got) != 2 {
			t.Fatalf("expected 2 nodes, got %d", len(got))
		}
	})
}

// TestCovspecProcessCallGraphEdge covers the call-graph drawing path including
// FuncLit callers, receiver labels and generics, plus the nil-edge guard.
func TestCovspecProcessCallGraphEdge(t *testing.T) {
	t.Run("nil edge is a no-op", func(t *testing.T) {
		data := &CytoscapeData{}
		visited := map[string]bool{}
		pair := map[string]bool{}
		idMap := map[string]string{}
		nc, ec := 0, 0
		processCallGraphEdge(nil, nil, data, visited, pair, idMap, &nc, &ec)
		if len(data.Nodes) != 0 {
			t.Fatalf("expected no nodes for nil edge")
		}
	})

	t.Run("full call graph with funclit and generics", func(t *testing.T) {
		meta := newTestMeta()
		sp := meta.StringPool

		// main -> Handler.Serve (receiver-typed callee).
		mainCall := metadata.Call{Meta: meta, Name: sp.Get("main"), Pkg: sp.Get("main"), RecvType: -1, Position: sp.Get("main.go:1"), Scope: -1, SignatureStr: -1}
		serveCall := metadata.Call{Meta: meta, Name: sp.Get("Serve"), Pkg: sp.Get("svc"), RecvType: sp.Get("Handler"), Position: -1, Scope: -1, SignatureStr: -1}
		edge1 := metadata.CallGraphEdge{
			Caller:       mainCall,
			Callee:       serveCall,
			TypeParamMap: map[string]string{"T": "User"},
		}

		// A FuncLit caller with a parent function.
		parent := &metadata.Call{Meta: meta, Name: sp.Get("Register"), Pkg: sp.Get("svc"), RecvType: -1, Position: -1, Scope: -1, SignatureStr: -1}
		funcLitCall := metadata.Call{Meta: meta, Name: sp.Get("FuncLit:svc.go:10"), Pkg: sp.Get("svc"), RecvType: -1, Position: -1, Scope: -1, SignatureStr: -1}
		targetCall := metadata.Call{Meta: meta, Name: sp.Get("doWork"), Pkg: sp.Get("svc"), RecvType: -1, Position: -1, Scope: -1, SignatureStr: -1}
		edge2 := metadata.CallGraphEdge{
			Caller:         funcLitCall,
			Callee:         targetCall,
			ParentFunction: parent,
		}

		meta.CallGraph = []metadata.CallGraphEdge{edge1, edge2}
		meta.BuildCallGraphMaps()

		data := DrawCallGraphCytoscape(meta)
		if len(data.Nodes) == 0 {
			t.Fatal("expected nodes")
		}

		var sawReceiverLabel, sawFuncLit bool
		for _, n := range data.Nodes {
			if n.Data.Label == "Handler.Serve" {
				sawReceiverLabel = true
			}
			if n.Data.Label == "FuncLit" {
				sawFuncLit = true
			}
		}
		if !sawReceiverLabel {
			t.Errorf("expected receiver-qualified label; nodes=%+v", data.Nodes)
		}
		if !sawFuncLit {
			t.Errorf("expected FuncLit label; nodes=%+v", data.Nodes)
		}
	})
}

// TestCovspecExtractParameterInfoFallback covers the Args fallback branch (no
// ParamArgMap) of extractParameterInfo, including the empty-value default.
func TestCovspecExtractParameterInfoFallback(t *testing.T) {
	meta := newTestMeta()
	sp := meta.StringPool

	withType := metadata.NewCallArgument(meta)
	withType.SetKind(metadata.KindIdent)
	withType.SetName("u")
	withType.SetType("User")
	withType.SetValue("userVal")

	// An arg with no type/value/name/raw so the "unknown"/"nil" defaults fire.
	empty := metadata.NewCallArgument(meta)
	empty.SetKind(metadata.KindLiteral)

	edge := &metadata.CallGraphEdge{
		Caller: metadata.Call{Meta: meta, Name: sp.Get("main"), Pkg: sp.Get("main")},
		Args:   []*metadata.CallArgument{withType, empty},
	}

	paramTypes, passed := extractParameterInfo(edge)
	if len(paramTypes) != 2 || len(passed) != 2 {
		t.Fatalf("expected 2 params, got types=%v passed=%v", paramTypes, passed)
	}
	if paramTypes[0] != "arg0:User" {
		t.Errorf("expected arg0:User, got %s", paramTypes[0])
	}
	if paramTypes[1] != "arg1:unknown" {
		t.Errorf("expected arg1:unknown, got %s", paramTypes[1])
	}
	if passed[1] != "arg1: nil" {
		t.Errorf("expected arg1: nil, got %s", passed[1])
	}
}
