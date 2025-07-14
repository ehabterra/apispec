package spec

import (
	"fmt"
	"strings"

	"github.com/ehabterra/swagen/internal/metadata"
)

// TrackerLimits holds configuration for tree/graph traversal limits.
type TrackerLimits struct {
	MaxNodesPerTree    int
	MaxChildrenPerNode int
	MaxArgsPerFunction int
	MaxNestedArgsDepth int
}

// TrackerNode represents a node in the call graph tree.
type TrackerNode struct {
	id       string
	children []*TrackerNode
	*metadata.CallGraphEdge
	*metadata.CallArgument
}

// TrackerTree represents the call graph as a tree structure.
type TrackerTree struct {
	meta      *metadata.Metadata
	positions map[string]bool
	roots     []*TrackerNode
	callers   map[string][]*metadata.CallGraphEdge
	callees   map[string][]*metadata.CallGraphEdge
	args      map[string][]*metadata.CallGraphEdge
}

// NewTrackerTree constructs a TrackerTree from metadata and limits.
func NewTrackerTree(meta *metadata.Metadata, limits TrackerLimits) *TrackerTree {
	t := &TrackerTree{
		meta:      meta,
		positions: make(map[string]bool),
		callers:   make(map[string][]*metadata.CallGraphEdge),
		callees:   make(map[string][]*metadata.CallGraphEdge),
		args:      make(map[string][]*metadata.CallGraphEdge),
	}

	for i := range meta.CallGraph {
		edge := &meta.CallGraph[i]
		callerID := edge.Caller.ID()
		calleeID := edge.Callee.ID()
		t.callers[callerID] = append(t.callers[callerID], edge)
		t.callees[calleeID] = append(t.callees[calleeID], edge)
		for _, arg := range edge.Args {
			argID := arg.ID()
			t.args[argID] = append(t.args[argID], edge)
		}
	}

	for i := range meta.CallGraph {
		var root = true
		edge := &meta.CallGraph[i]
		callerID := edge.Caller.ID()
		for _, rt := range t.roots {
			if rt.id == callerID {
				root = false
			}
		}
		if _, exists := t.callees[callerID]; exists {
			root = false
		}
		if _, exists := t.args[callerID]; exists {
			root = false
		}
		if root && getString(meta, edge.Caller.Name) == "main" {
			if node := NewTrackerNode(t, meta, "", callerID, nil, make(map[string]*TrackerNode), limits); node != nil {
				t.roots = append(t.roots, node)
			}
		}
	}
	return t
}

// NewTrackerNode creates a new TrackerNode for the tree.
func NewTrackerNode(tree *TrackerTree, meta *metadata.Metadata, parentID, id string, parentEdge *metadata.CallGraphEdge, visited map[string]*TrackerNode, limits TrackerLimits) *TrackerNode {
	if id == "" {
		return nil
	}

	// Create a unique node identifier that includes the call context and depth
	var nodeKey string
	if parentEdge != nil {
		// Include the position and parent to distinguish between different calls to the same function
		nodeKey = fmt.Sprintf("%s@%d@%s", id, parentEdge.Position, parentID)
	} else {
		// For root nodes or nodes without parent edge, just use the ID
		nodeKey = id
	}

	// Check if we've already created a node for this specific call context
	if nd, exists := visited[nodeKey]; exists {
		// Return the existing node to prevent duplicates within the same context
		return nd
	}

	// Limit recursion depth to prevent infinite loops
	if len(visited) > limits.MaxNodesPerTree {
		// Return a simple node without children to prevent explosion
		return &TrackerNode{id: id, CallGraphEdge: parentEdge}
	}

	// Create new node
	node := &TrackerNode{id: id, CallGraphEdge: parentEdge}
	visited[nodeKey] = node

	// Helper function to collect all argument IDs from a slice of arguments
	var collectArgIDs func(args []metadata.CallArgument) map[string]bool
	collectArgIDs = func(args []metadata.CallArgument) map[string]bool {
		argIDs := make(map[string]bool)
		for _, arg := range args {
			argIDs[arg.ID()] = true
			// Recursively collect IDs from nested arguments (limit depth)
			if len(arg.Args) > 0 && len(argIDs) < limits.MaxNestedArgsDepth {
				nestedIDs := collectArgIDs(arg.Args)
				for nestedID := range nestedIDs {
					argIDs[nestedID] = true
				}
			}
		}
		return argIDs
	}

	// Process children (callees)
	if edges, exists := tree.callers[id]; exists {
		// First pass: collect all argument IDs from all edges to avoid duplicates
		allArgIDs := make(map[string]bool)
		for _, edge := range edges {
			edgeArgIDs := collectArgIDs(edge.Args)
			for argID := range edgeArgIDs {
				allArgIDs[argID] = true
			}
		}

		// Limit the number of children to prevent explosion
		childCount := 0

		for i := range edges {
			if childCount >= limits.MaxChildrenPerNode {
				break
			}

			edge := edges[i]

			// Check if this callee is already present in any arguments
			calleeID := edge.Callee.ID()
			if edge.Callee.ID() == edge.Caller.ID() || getString(meta, edge.Callee.Name) == "nil" || allArgIDs[calleeID] {
				// Skip this child as it's already present in arguments
				continue
			}

			if childNode := NewTrackerNode(tree, meta, id, calleeID, edge, visited, limits); childNode != nil {
				// Process arguments for this edge (simplified)
				var traverseArgs func(parentID string, args []metadata.CallArgument) []*TrackerNode
				traverseArgs = func(parentID string, args []metadata.CallArgument) []*TrackerNode {
					var children []*TrackerNode
					argCount := 0

					for _, arg := range args {
						if argCount >= limits.MaxArgsPerFunction {
							break
						}

						if edge.Caller.ID() == arg.ID() || edge.Callee.ID() == arg.ID() || arg.Name == "nil" {
							continue
						}

						// Only process arguments that are function calls, not simple values
						if (arg.Kind == "call" && arg.Fun != nil) || (arg.Kind == "ident" && strings.HasPrefix(arg.Type, "func(")) {
							if argNode := NewTrackerNode(tree, meta, parentID, arg.ID(), edge, visited, limits); argNode != nil {
								if len(arg.Args) > 0 {
									childArgs := make([]metadata.CallArgument, len(arg.Args))
									copy(childArgs, arg.Args)
									argNode.children = append(argNode.children, traverseArgs(argNode.id, childArgs)...)
								}

								children = append(children, argNode)
								if arg.Fun != nil && arg.Fun.Position != "" {
									tree.positions[arg.Fun.Position] = true
								}
								argCount++
							}
						}
					}
					return children
				}
				childNode.children = append(childNode.children, traverseArgs(id, edge.Args)...)
				node.children = append(node.children, childNode)
				childCount++
			}
		}
	}

	return node
}

// GetRoots returns the root nodes of the tracker tree.
func (t *TrackerTree) GetRoots() []*TrackerNode {
	return t.roots
}

// GetFunctionContext returns the *metadata.Function, package name, and file name for a TrackerNode.
func (t *TrackerTree) GetFunctionContext(node *TrackerNode) (*metadata.Function, string, string) {
	if node == nil || node.CallGraphEdge == nil {
		return nil, "", ""
	}
	caller := node.CallGraphEdge.Caller
	for pkgName, pkg := range t.meta.Packages {
		for fileName, file := range pkg.Files {
			for _, fn := range file.Functions {
				if t.meta.StringPool.GetString(fn.Name) == t.meta.StringPool.GetString(caller.Name) {
					return fn, pkgName, fileName
				}
			}
		}
	}
	return nil, "", ""
}

// getString retrieves a string value from the metadata string pool.
func getString(meta *metadata.Metadata, index int) string {
	if meta == nil || meta.StringPool == nil {
		return ""
	}
	return meta.StringPool.GetString(index)
}

// GetNodeCount returns the total number of nodes in the tree
func (t *TrackerTree) GetNodeCount() int {
	var count int
	var countNodes func(*TrackerNode)
	countNodes = func(node *TrackerNode) {
		if node == nil {
			return
		}
		count++
		for _, child := range node.children {
			countNodes(child)
		}
	}

	for _, root := range t.roots {
		countNodes(root)
	}
	return count
}
