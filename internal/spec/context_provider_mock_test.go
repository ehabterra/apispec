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

// TestContextProvider_WithMockTrackerTree demonstrates proper use of MockTrackerTree for testing
func TestContextProvider_WithMockTrackerTree(t *testing.T) {
	// Create metadata with string pool
	stringPool := metadata.NewStringPool()
	meta := &metadata.Metadata{
		StringPool: stringPool,
	}

	// Create a call graph edge
	caller := metadata.Call{
		Meta: meta,
		Name: stringPool.Get("main"),
		Pkg:  stringPool.Get("main"),
	}
	callee := metadata.Call{
		Meta:     meta,
		Name:     stringPool.Get("handler"),
		Pkg:      stringPool.Get("main"),
		RecvType: stringPool.Get("Handler"),
	}
	edge := metadata.CallGraphEdge{
		Caller: caller,
		Callee: callee,
	}

	// Create a mock node that implements TrackerNodeInterface
	mockNode := &TrackerNode{
		key:           "test-handler",
		CallGraphEdge: &edge,
	}

	// Create mock tracker tree
	limits := metadata.TrackerLimits{
		MaxNodesPerTree:    100,
		MaxChildrenPerNode: 10,
		MaxArgsPerFunction: 5,
		MaxNestedArgsDepth: 3,
	}
	mockTree := NewMockTrackerTree(meta, limits)
	mockTree.AddRoot(mockNode)

	// Test context provider with mock tree
	provider := NewContextProvider(meta)

	// Test GetCalleeInfo with the mock node
	name, pkg, recvType := provider.GetCalleeInfo(mockNode)

	if name != "handler" {
		t.Errorf("Expected name 'handler', got '%s'", name)
	}
	if pkg != "main" {
		t.Errorf("Expected pkg 'main', got '%s'", pkg)
	}
	if recvType != "Handler" {
		t.Errorf("Expected recvType 'Handler', got '%s'", recvType)
	}

	// Verify mock tree functionality
	roots := mockTree.GetRoots()
	if len(roots) != 1 {
		t.Errorf("Expected 1 root, got %d", len(roots))
	}

	if roots[0].GetKey() != "test-handler" {
		t.Errorf("Expected root key 'test-handler', got '%s'", roots[0].GetKey())
	}
}

// TestContextProvider_GetCalleeInfo_WithNilEdge tests edge case handling
func TestContextProvider_GetCalleeInfo_WithNilEdge(t *testing.T) {
	meta := &metadata.Metadata{}
	provider := NewContextProvider(meta)

	// Create a mock node with nil edge
	mockNode := &TrackerNode{
		key:           "test-node",
		CallGraphEdge: nil, // Nil edge
	}

	name, pkg, recvType := provider.GetCalleeInfo(mockNode)

	// Should return empty strings for nil edge
	if name != "" || pkg != "" || recvType != "" {
		t.Errorf("Expected empty strings for nil edge, got name='%s', pkg='%s', recvType='%s'", name, pkg, recvType)
	}
}

// TestContextProvider_GetCalleeInfo_WithMalformedNode tests error handling
func TestContextProvider_GetCalleeInfo_WithMalformedNode(t *testing.T) {
	// Create metadata with string pool
	stringPool := metadata.NewStringPool()
	meta := &metadata.Metadata{
		StringPool: stringPool,
	}

	provider := NewContextProvider(meta)

	// Create edge with invalid indices
	edge := metadata.CallGraphEdge{
		Caller: metadata.Call{
			Meta: meta,
			Name: -1, // Invalid index
			Pkg:  -1, // Invalid index
		},
		Callee: metadata.Call{
			Meta:     meta,
			Name:     -1, // Invalid index
			Pkg:      -1, // Invalid index
			RecvType: -1, // Invalid index
		},
	}

	mockNode := &TrackerNode{
		key:           "malformed-node",
		CallGraphEdge: &edge,
	}

	name, pkg, recvType := provider.GetCalleeInfo(mockNode)

	// Should handle invalid indices gracefully
	if name == "" && pkg == "" && recvType == "" {
		t.Log("Correctly handled malformed node with invalid indices")
	}
}
