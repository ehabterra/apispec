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
	"maps"
	"strings"

	"github.com/ehabterra/apispec/internal/metadata"
)

// TrackerNode is a hand-assembled TrackerNodeInterface for tests.
//
// It is the shape the eager tree's node had before that engine was removed
// (issue #425), kept HERE — test-only — because a good number of unit tests
// need a node they can build directly: an edge, an argument, some children.
// The lazy tree's node cannot serve that purpose, since it exists only inside a
// materialised tree and computes its key from the tree's interner.
//
// Production code that needs a node for one edge uses edgeNode instead.
type TrackerNode struct {
	key      string
	keyReady bool
	Parent   *TrackerNode
	Children []*TrackerNode
	*metadata.CallGraphEdge
	*metadata.CallArgument

	typeParamMap map[string]string

	ArgType    ArgumentType
	IsArgument bool
	ArgIndex   int
	ArgContext string

	RootAssignmentMap map[string][]metadata.Assignment
}

// Key returns the node's identity, derived from its argument or its callee the
// way the tree used to derive it.
func (nd *TrackerNode) Key() string {
	if nd.keyReady {
		return nd.key
	}
	if nd.key == "" {
		switch {
		case nd.CallArgument != nil:
			nd.key = nd.ID()
		case nd.CallGraphEdge != nil:
			nd.key = nd.Callee.ID()
		}
	}
	nd.key = strings.TrimPrefix(nd.key, "*")
	nd.keyReady = true
	return nd.key
}

func (nd *TrackerNode) GetKey() string { return nd.Key() }

// TypeParams collects the type arguments in scope, rebuilding them from the
// edge, the argument and the ancestors on every read — the eager node's own
// behaviour, which several tests depend on: they seed `edge.TypeParamMap` and
// expect the node to pick it up.
func (nd *TrackerNode) TypeParams() map[string]string {
	visited := make(map[*TrackerNode]struct{})
	var collect func(n *TrackerNode, out map[string]string)
	collect = func(n *TrackerNode, out map[string]string) {
		if n == nil {
			return
		}
		if _, seen := visited[n]; seen {
			return
		}
		visited[n] = struct{}{}
		if n.CallGraphEdge != nil && len(n.CallGraphEdge.TypeParamMap) > 0 {
			maps.Copy(out, n.CallGraphEdge.TypeParamMap)
		}
		if n.CallArgument != nil {
			maps.Copy(out, n.CallArgument.TypeParams())
		}
		collect(n.Parent, out)
	}
	result := map[string]string{}
	collect(nd, result)
	nd.typeParamMap = result
	return nd.typeParamMap
}

func (nd *TrackerNode) GetTypeParamMap() map[string]string { return nd.TypeParams() }

func (nd *TrackerNode) GetParent() TrackerNodeInterface {
	if nd.Parent == nil {
		return nil
	}
	return nd.Parent
}

func (nd *TrackerNode) GetChildren() []TrackerNodeInterface {
	out := make([]TrackerNodeInterface, 0, len(nd.Children))
	for _, child := range nd.Children {
		out = append(out, child)
	}
	return out
}

func (nd *TrackerNode) GetEdge() *metadata.CallGraphEdge { return nd.CallGraphEdge }

func (nd *TrackerNode) GetArgument() *metadata.CallArgument { return nd.CallArgument }

// AddChild links a child both ways, as the tree did.
func (nd *TrackerNode) AddChild(child *TrackerNode) {
	if child == nil {
		return
	}
	child.Parent = nd
	nd.Children = append(nd.Children, child)
}

// MockTrackerTree is a mock implementation of TrackerTreeInterface for testing
type MockTrackerTree struct {
	meta  *metadata.Metadata
	roots []*TrackerNode
}

// NewMockTrackerTree creates a new mock tracker tree. The limits parameter is
// accepted for call-site convenience but unused — the shrunk
// TrackerTreeInterface no longer exposes limits.
func NewMockTrackerTree(meta *metadata.Metadata, _ metadata.TrackerLimits) *MockTrackerTree {
	return &MockTrackerTree{
		meta:  meta,
		roots: make([]*TrackerNode, 0),
	}
}

// AddRoot adds a root node to the mock tracker
func (m *MockTrackerTree) AddRoot(root *TrackerNode) {
	m.roots = append(m.roots, root)
}

// GetRoots returns the root nodes of the mock tracker tree
func (m *MockTrackerTree) GetRoots() []TrackerNodeInterface {
	roots := make([]TrackerNodeInterface, len(m.roots))
	for i, root := range m.roots {
		roots[i] = root
	}
	return roots
}

// GetMetadata returns the underlying metadata
func (m *MockTrackerTree) GetMetadata() *metadata.Metadata {
	return m.meta
}
