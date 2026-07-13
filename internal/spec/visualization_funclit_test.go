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

func TestExtractFuncLitSignature(t *testing.T) {
	sp := metadata.NewStringPool()
	meta := &metadata.Metadata{StringPool: sp}

	if got := extractFuncLitSignature(nil, &metadata.Call{}); got != "" {
		t.Errorf("nil meta = %q", got)
	}
	if got := extractFuncLitSignature(meta, nil); got != "" {
		t.Errorf("nil caller = %q", got)
	}

	withRecv := &metadata.Call{Meta: meta, Name: sp.Get("Create"), RecvType: sp.Get("Handler")}
	if got := extractFuncLitSignature(meta, withRecv); got != "Handler" {
		t.Errorf("receiver signature = %q, want Handler", got)
	}

	noRecv := &metadata.Call{Meta: meta, Name: sp.Get("main"), RecvType: -1}
	if got := extractFuncLitSignature(meta, noRecv); got != "func()" {
		t.Errorf("fallback = %q, want func()", got)
	}
}

func TestEnsureParentFunctionNode(t *testing.T) {
	sp := metadata.NewStringPool()
	meta := &metadata.Metadata{StringPool: sp}
	meta.BuildCallGraphMaps()

	if got := ensureParentFunctionNode(meta, nil, nil, nil, nil); got != "" {
		t.Errorf("nil parent = %q", got)
	}

	parent := &metadata.Call{
		Meta:     meta,
		Name:     sp.Get("registerRoutes"),
		Pkg:      sp.Get("main"),
		RecvType: sp.Get("Server"),
	}
	data := &CytoscapeData{}
	visited := map[string]bool{}
	counter := 0

	id := ensureParentFunctionNode(meta, parent, data, visited, &counter)
	if id == "" {
		t.Fatal("no node ID returned")
	}
	if len(data.Nodes) != 1 {
		t.Fatalf("nodes = %d, want 1", len(data.Nodes))
	}
	n := data.Nodes[0].Data
	if n.FunctionName != "registerRoutes" || n.Package != "main" {
		t.Errorf("node data = %+v", n)
	}
	if n.Label != "Server.registerRoutes" {
		t.Errorf("label = %q, want Server.registerRoutes", n.Label)
	}
	if n.IsParentFunction != "true" {
		t.Errorf("IsParentFunction = %q", n.IsParentFunction)
	}

	// Second call for the same parent reuses the existing node.
	id2 := ensureParentFunctionNode(meta, parent, data, visited, &counter)
	if id2 != id {
		t.Errorf("existing node id = %q, want %q", id2, id)
	}
	if len(data.Nodes) != 1 {
		t.Errorf("nodes after reuse = %d, want 1", len(data.Nodes))
	}
}
