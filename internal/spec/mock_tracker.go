package spec

import (
	"github.com/ehabterra/apispec/internal/metadata"
)

// MockTrackerTree is a mock implementation of TrackerTreeInterface for testing
type MockTrackerTree struct {
	meta   *metadata.Metadata
	limits metadata.TrackerLimits
	roots  []*TrackerNode
}

// NewMockTrackerTree creates a new mock tracker tree
func NewMockTrackerTree(meta *metadata.Metadata, limits metadata.TrackerLimits) *MockTrackerTree {
	return &MockTrackerTree{
		meta:   meta,
		limits: limits,
		roots:  make([]*TrackerNode, 0),
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

// GetNodeCount returns the total number of nodes in the mock tree
func (m *MockTrackerTree) GetNodeCount() int {
	var count int
	var countNodes func(*TrackerNode)

	countNodes = func(node *TrackerNode) {
		if node == nil {
			count++ // Count nil nodes as well
			return
		}
		count++
		for _, child := range node.Children {
			countNodes(child)
		}
	}

	for _, root := range m.roots {
		countNodes(root)
	}

	return count
}

// FindNodeByKey finds a node by its key in the mock tree
func (m *MockTrackerTree) FindNodeByKey(key string) TrackerNodeInterface {
	var findNode func(*TrackerNode) *TrackerNode

	findNode = func(node *TrackerNode) *TrackerNode {
		if node == nil {
			return nil
		}

		if node.Key() == key {
			return node
		}

		for _, child := range node.Children {
			if found := findNode(child); found != nil {
				return found
			}
		}

		return nil
	}

	for _, root := range m.roots {
		if found := findNode(root); found != nil {
			return found
		}
	}

	return nil
}

// GetFunctionContext returns function context information for a function name
func (m *MockTrackerTree) GetFunctionContext(functionName string) (*metadata.Function, string, string) {
	// Search through all packages to find the function
	for pkgName, pkg := range m.meta.Packages {
		for fileName, file := range pkg.Files {
			if fn, exists := file.Functions[functionName]; exists {
				return fn, pkgName, fileName
			}
		}
	}

	return nil, "", ""
}

// TraverseTree traverses the mock tree with a visitor function
func (m *MockTrackerTree) TraverseTree(visitor func(node TrackerNodeInterface) bool) {
	var traverse func(*TrackerNode) bool

	traverse = func(node *TrackerNode) bool {
		if node == nil {
			return true
		}

		if !visitor(node) {
			return false
		}

		for _, child := range node.Children {
			if !traverse(child) {
				return false
			}
		}

		return true
	}

	for _, root := range m.roots {
		if !traverse(root) {
			break
		}
	}
}

// GetMetadata returns the underlying metadata
func (m *MockTrackerTree) GetMetadata() *metadata.Metadata {
	return m.meta
}

// GetLimits returns the tracker limits
func (m *MockTrackerTree) GetLimits() metadata.TrackerLimits {
	return m.limits
}
