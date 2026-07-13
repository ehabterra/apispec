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
	"github.com/ehabterra/apispec/internal/metadata"
)

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
