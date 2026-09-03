// Copyright 2025 Ehab Terra
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

func TestDrawTrackerTreeCytoscape(t *testing.T) {
	// Test with nil nodes
	result := DrawTrackerTreeCytoscapeWithMetadata(nil, nil)
	if result == nil {
		t.Error("Expected non-nil result even for nil nodes")
		return
	}

	// Test with empty nodes slice
	emptyNodes := []TrackerNodeInterface{}
	result = DrawTrackerTreeCytoscapeWithMetadata(emptyNodes, nil)
	if result == nil {
		t.Error("Expected non-nil result for empty nodes slice")
		return
	}

	// Check that the result has the expected structure
	if result.Nodes == nil {
		t.Error("Expected non-nil Nodes slice")
	}
	if result.Edges == nil {
		t.Error("Expected non-nil Edges slice")
	}

	// For empty nodes slice, we expect empty slices (not nil)
	if len(result.Nodes) != 0 {
		t.Errorf("Expected empty Nodes slice for empty nodes input, got %d nodes", len(result.Nodes))
	}
	if len(result.Edges) != 0 {
		t.Errorf("Expected empty Edges slice for empty nodes input, got %d edges", len(result.Edges))
	}
}

func TestDrawCallGraphCytoscape(t *testing.T) {
	// Test with nil metadata
	result := DrawCallGraphCytoscape(nil)
	if result == nil {
		t.Error("Expected non-nil result even for nil metadata")
		return
	}

	// Test with empty metadata
	emptyMeta := &metadata.Metadata{}
	result = DrawCallGraphCytoscape(emptyMeta)
	if result == nil {
		t.Error("Expected non-nil result for empty metadata")
		return
	}

	// Check that the result has the expected structure
	if result.Nodes == nil {
		t.Error("Expected non-nil Nodes slice")
	}
	if result.Edges == nil {
		t.Error("Expected non-nil Edges slice")
	}

	// For empty metadata, we expect empty slices (not nil)
	if len(result.Nodes) != 0 {
		t.Errorf("Expected empty Nodes slice for empty metadata, got %d nodes", len(result.Nodes))
	}
	if len(result.Edges) != 0 {
		t.Errorf("Expected empty Edges slice for empty metadata, got %d edges", len(result.Edges))
	}
}

func TestBuildCallPathInfos(t *testing.T) {
	// Intern first and use the indices Get hands back. Writing 0 and 1 here
	// assumed the first string interned lands at slot 0, which is no longer
	// true — slot 0 is the reserved empty string (#449) — and was never
	// something a test needed to know.
	pool := metadata.NewStringPool()
	mainIdx := pool.Get("main")
	fooIdx := pool.Get("foo")

	meta := &metadata.Metadata{
		StringPool: pool,
		CallGraph: []metadata.CallGraphEdge{
			{
				Caller: metadata.Call{
					Name:     mainIdx,
					Pkg:      mainIdx,
					Position: -1,  // No position
					Meta:     nil, // Will be set after metadata creation
				},
				Callee: metadata.Call{
					Name: fooIdx,
					Pkg:  mainIdx,
					Meta: nil, // Will be set after metadata creation
				},
			},
		},
	}

	// Set Meta field on Call objects
	meta.CallGraph[0].Caller.Meta = meta
	meta.CallGraph[0].Callee.Meta = meta

	// Build call graph maps
	meta.BuildCallGraphMaps()

	// Test buildCallPathInfos - use the callee's BaseID
	calleeID := meta.CallGraph[0].Callee.BaseID()
	infos := buildCallPathInfos(meta, calleeID)

	// Should have one caller path
	if len(infos) != 1 {
		t.Errorf("Expected 1 call path, got %d", len(infos))
	}

	if len(infos) > 0 && (infos[0].CallerPkg != "main" || infos[0].CallerName != "main") {
		t.Errorf("Expected caller main.main, got %s.%s", infos[0].CallerPkg, infos[0].CallerName)
	}
}

func TestExtractParameterInfo(t *testing.T) {
	// Create a simple metadata
	meta := &metadata.Metadata{
		StringPool: metadata.NewStringPool(),
	}

	// Intern first, then use the returned indices — see TestBuildCallPathInfos.
	//
	// Kind is set too, and that is the point of this comment: it used to be
	// left at 0, so callArgToString fell to `default: return arg.GetRaw()` with
	// Raw also 0 — which resolved to pool slot 0, the first string interned,
	// which in this test happened to be "value1". The assertion below passed by
	// reading a field the test never set. With slot 0 reserved (#449) an unset
	// index reads as "", so the argument now has to say what it is: an ident
	// named "value1" of type "string".
	identIdx := meta.StringPool.Get(metadata.KindIdent)
	valueIdx := meta.StringPool.Get("value1")
	stringIdx := meta.StringPool.Get("string")
	argIdx := meta.StringPool.Get("arg1")

	// Create a call graph edge with parameter information
	edge := &metadata.CallGraphEdge{
		ParamArgMap: map[string]metadata.CallArgument{
			"param1": {
				Kind: identIdx,
				Name: valueIdx,
				Type: stringIdx,
				Meta: meta,
			},
		},
		Args: []*metadata.CallArgument{
			{
				Kind: identIdx,
				Name: argIdx,
				Meta: meta,
			},
		},
	}

	paramTypes, passedParams := extractParameterInfo(edge)

	// Check parameter types
	if len(paramTypes) != 1 {
		t.Errorf("Expected 1 parameter type, got %d", len(paramTypes))
	}

	if paramTypes[0] != "param1:string" {
		t.Errorf("Expected parameter type 'param1:string', got '%s'", paramTypes[0])
	}

	// Check passed parameters
	if len(passedParams) != 1 {
		t.Errorf("Expected 1 passed parameter, got %d", len(passedParams))
	}

	if passedParams[0] != "param1: value1" {
		t.Errorf("Expected passed parameter 'param1: value1', got '%s'", passedParams[0])
	}
}
