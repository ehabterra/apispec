package spec

import (
	"fmt"
	"strings"

	"github.com/ehabterra/swagen/internal/metadata"
)

// SimplifiedTrackerTree represents a simplified version of the tracker tree
// that uses enhanced metadata functionality and builds deep, framework-agnostic trees
// It implements the TrackerTreeInterface
type SimplifiedTrackerTree struct {
	meta   *metadata.Metadata
	limits metadata.TrackerLimits
	roots  []*SimplifiedTrackerNode

	// Cached relationships from metadata
	assignmentRelationships map[metadata.AssignmentKey]*metadata.AssignmentLink
	variableRelationships   map[metadata.ParamKey]*metadata.VariableLink

	// Internal tracking for deep traversal
	visited   map[string]int
	nodeCount int
}

// SimplifiedTrackerNode represents a simplified node in the tracker tree
// It implements the TrackerNodeInterface
type SimplifiedTrackerNode struct {
	Key        string
	Parent     *SimplifiedTrackerNode
	Children   []*SimplifiedTrackerNode
	Edge       *metadata.CallGraphEdge
	Argument   *metadata.CallArgument
	ArgType    metadata.ArgumentType
	ArgIndex   int
	ArgContext string

	// Type parameter information
	TypeParamMap map[string]string

	// Assignment and variable information for deep linking
	AssignmentMap map[string][]metadata.Assignment
	VariableMap   map[string]*metadata.CallArgument

	// Flag to indicate if children have been processed for deep structure
	ChildrenProcessed bool
}

// NewSimplifiedTrackerTree creates a new simplified tracker tree
func NewSimplifiedTrackerTree(meta *metadata.Metadata, limits metadata.TrackerLimits) *SimplifiedTrackerTree {
	t := &SimplifiedTrackerTree{
		meta:      meta,
		limits:    limits,
		roots:     make([]*SimplifiedTrackerNode, 0),
		visited:   make(map[string]int),
		nodeCount: 0,
	}

	// Only proceed if metadata is not nil
	if meta != nil {
		// Get pre-built relationships from metadata
		t.assignmentRelationships = meta.GetAssignmentRelationships()
		t.variableRelationships = meta.GetVariableRelationships()

		// Build tree using enhanced deep traversal approach
		t.buildTree()
	}

	return t
}

// buildTree builds the enhanced tree structure with deep traversal
func (t *SimplifiedTrackerTree) buildTree() {
	// Find root functions (main functions only - framework-agnostic)
	roots := t.meta.CallGraphRoots()

	// Create root nodes for main functions
	rootNodesCreated := make(map[string]bool)

	for _, edge := range roots {
		callerName := t.meta.StringPool.GetString(edge.Caller.Name)

		// Process main function as root (framework-agnostic)
		if callerName == "main" {
			rootKey := edge.Caller.BaseID()
			if !rootNodesCreated[rootKey] {
				rootNode := t.createNodeFromEdge(edge, nil)
				if rootNode != nil {
					t.roots = append(t.roots, rootNode)
					rootNodesCreated[rootKey] = true
				}
			}
		}
	}

	// Second pass: build the complete tree structure with deep traversal
	for _, rootNode := range t.roots {
		t.buildCompleteTree(rootNode)
	}
}

// buildCompleteTree builds the complete tree structure starting from the main root
func (t *SimplifiedTrackerTree) buildCompleteTree(rootNode *SimplifiedTrackerNode) {
	// Track created nodes to avoid duplicates
	createdNodes := make(map[string]*SimplifiedTrackerNode)
	createdNodes[rootNode.Key] = rootNode

	// Process all call graph edges to build the complete tree
	for i := range t.meta.CallGraph {
		edge := &t.meta.CallGraph[i]
		callerBaseID := edge.Caller.BaseID()

		// Find the node for this caller
		var callerNode *SimplifiedTrackerNode
		if callerBaseID == rootNode.Key {
			callerNode = rootNode
		} else {
			callerNode = t.findNodeByKey(callerBaseID, rootNode)
		}

		if callerNode != nil {
			// Check if we already have a node for this callee
			// Use GenericID to differentiate between generic type instantiations
			calleeKey := edge.Callee.GenericID()
			var calleeNode *SimplifiedTrackerNode

			if existingNode, exists := createdNodes[calleeKey]; exists {
				// Use existing node
				calleeNode = existingNode
			} else {
				// Create a new node for the callee
				calleeNode = t.createNodeFromCallee(edge, callerNode)
				if calleeNode != nil {
					createdNodes[calleeKey] = calleeNode
				}
			}

			if calleeNode != nil && calleeNode != callerNode {
				// Only add as child if not already a child and not the same node
				isAlreadyChild := false
				for _, child := range callerNode.Children {
					if child.Key == calleeNode.Key {
						isAlreadyChild = true
						break
					}
				}

				if !isAlreadyChild {
					callerNode.Children = append(callerNode.Children, calleeNode)

					// Now build the deep structure for this callee node
					// but only if we haven't already processed it
					if !calleeNode.ChildrenProcessed {
						calleeNode.ChildrenProcessed = true
						t.buildNodeDeepStructure(calleeNode, edge)
					}
				}
			}
		}
	}
}

// buildNodeDeepStructure builds the deep structure for a node including arguments, assignments, and variables
func (t *SimplifiedTrackerTree) buildNodeDeepStructure(node *SimplifiedTrackerNode, edge *metadata.CallGraphEdge) {
	// Check depth limits
	if t.nodeCount >= t.limits.MaxNodesPerTree {
		return
	}

	// Process arguments deeply
	t.processArgumentsDeep(node, edge)

	// Link related assignments
	t.linkRelatedAssignments(node, edge)

	// Link related variables
	t.linkRelatedVariables(node, edge)

	// Process callees recursively
	t.processCalleesDeep(node, edge)
}

// processArgumentsDeep processes function arguments with deep traversal
func (t *SimplifiedTrackerTree) processArgumentsDeep(node *SimplifiedTrackerNode, edge *metadata.CallGraphEdge) {
	if edge == nil || len(edge.Args) == 0 {
		return
	}

	argCount := 0
	for i, arg := range edge.Args {
		if argCount >= t.limits.MaxArgsPerFunction {
			break
		}

		// Skip nil or empty arguments
		if arg.GetName() == "nil" || arg.ID() == "" {
			continue
		}

		// Create argument node
		argNode := t.createNodeFromArgument(&arg, node, edge, i)
		if argNode != nil {
			node.Children = append(node.Children, argNode)
			argCount++
			t.nodeCount++

			// Process nested arguments if they are function calls
			if arg.GetKind() == metadata.KindCall {
				t.processNestedArgumentsDeep(argNode, &arg, edge)
			}
		}
	}
}

// processNestedArgumentsDeep processes nested arguments recursively
func (t *SimplifiedTrackerTree) processNestedArgumentsDeep(parentNode *SimplifiedTrackerNode, arg *metadata.CallArgument, edge *metadata.CallGraphEdge) {
	if arg == nil || arg.Fun == nil {
		return
	}

	// Check depth limits
	if t.nodeCount >= t.limits.MaxNodesPerTree {
		return
	}

	// Process selector expressions (e.g., r.Get, r.Post)
	if arg.Fun.GetKind() == metadata.KindSelector {
		// Create node for the selector
		selectorNode := t.createNodeFromSelector(arg.Fun, parentNode, edge)
		if selectorNode != nil {
			parentNode.Children = append(parentNode.Children, selectorNode)
			t.nodeCount++

			// If this is a function call, try to find the actual function
			if arg.Fun.Sel != nil && strings.HasPrefix(arg.Fun.Sel.GetType(), "func(") {
				t.findAndLinkFunctionCall(selectorNode, arg.Fun, edge)
			}
		}
	}

	// Process function calls recursively
	if arg.Edge != nil {
		funcNode := t.createNodeFromEdge(arg.Edge, parentNode)
		if funcNode != nil {
			parentNode.Children = append(parentNode.Children, funcNode)
			t.nodeCount++

			// Recursively build the function's structure
			t.buildNodeDeepStructure(funcNode, arg.Edge)
		}
	}
}

// createNodeFromSelector creates a node from a selector expression
func (t *SimplifiedTrackerTree) createNodeFromSelector(selector *metadata.CallArgument, parent *SimplifiedTrackerNode, edge *metadata.CallGraphEdge) *SimplifiedTrackerNode {
	if selector == nil || selector.X == nil {
		return nil
	}

	node := &SimplifiedTrackerNode{
		Key:        fmt.Sprintf("%s.%s", selector.X.GetType(), selector.Sel.GetName()),
		Parent:     parent,
		Edge:       edge,
		Argument:   selector,
		ArgType:    metadata.ArgTypeSelector,
		ArgContext: fmt.Sprintf("selector in %s", edge.Caller.BaseID()),
	}

	return node
}

// findAndLinkFunctionCall finds and links a function call to its definition
func (t *SimplifiedTrackerTree) findAndLinkFunctionCall(selectorNode *SimplifiedTrackerNode, selector *metadata.CallArgument, edge *metadata.CallGraphEdge) {
	if selector == nil || selector.Sel == nil {
		return
	}

	// Look for the function in the call graph
	funcName := selector.Sel.GetName()
	recvType := selector.X.GetType()

	// Find edges where this function is called
	for i := range t.meta.CallGraph {
		callEdge := &t.meta.CallGraph[i]
		calleeName := t.meta.StringPool.GetString(callEdge.Callee.Name)

		if calleeName == funcName {
			// Check if this is the right receiver type
			calleeRecvType := t.meta.StringPool.GetString(callEdge.Callee.RecvType)
			if calleeRecvType == recvType || calleeRecvType == "*"+recvType {
				// Create a node for this function call
				funcNode := t.createNodeFromEdge(callEdge, selectorNode)
				if funcNode != nil {
					selectorNode.Children = append(selectorNode.Children, funcNode)
					t.nodeCount++

					// Recursively build the function's structure
					t.buildNodeDeepStructure(funcNode, callEdge)
				}
			}
		}
	}
}

// linkRelatedAssignments links nodes to related assignments
func (t *SimplifiedTrackerTree) linkRelatedAssignments(node *SimplifiedTrackerNode, edge *metadata.CallGraphEdge) {
	if edge == nil || t.assignmentRelationships == nil {
		return
	}

	// Get assignments for this edge
	for varName, assignments := range edge.AssignmentMap {
		for _, assignment := range assignments {
			// Create assignment key
			key := metadata.AssignmentKey{
				Name:      varName,
				Pkg:       t.meta.StringPool.GetString(assignment.Pkg),
				Type:      t.meta.StringPool.GetString(assignment.ConcreteType),
				Container: t.meta.StringPool.GetString(assignment.Func),
			}

			// Find related assignments
			if link, exists := t.assignmentRelationships[key]; exists {
				// Create assignment node
				assignmentNode := t.createAssignmentNode(assignment, node, edge)
				if assignmentNode != nil {
					node.Children = append(node.Children, assignmentNode)
					t.nodeCount++

					// Link to the assignment relationship
					if link.Assignment != nil {
						assignmentNode.AssignmentMap[varName] = []metadata.Assignment{*link.Assignment}
					}
				}
			}
		}
	}
}

// linkRelatedVariables links nodes to related variables
func (t *SimplifiedTrackerTree) linkRelatedVariables(node *SimplifiedTrackerNode, edge *metadata.CallGraphEdge) {
	if edge == nil || t.variableRelationships == nil {
		return
	}

	// Get variables for this edge
	for param, arg := range edge.ParamArgMap {
		// Create parameter key
		key := metadata.ParamKey{
			Name:      param,
			Pkg:       t.meta.StringPool.GetString(edge.Callee.Pkg),
			Container: t.meta.StringPool.GetString(edge.Callee.Name),
		}

		// Find related variables
		if link, exists := t.variableRelationships[key]; exists {
			// Create variable node
			variableNode := t.createVariableNode(param, &arg, node, edge)
			if variableNode != nil {
				node.Children = append(node.Children, variableNode)
				t.nodeCount++

				// Link to the variable relationship
				if link.Argument != nil {
					variableNode.VariableMap[param] = link.Argument
				}
			}
		}
	}
}

// processCalleesDeep processes callees recursively with deep traversal
func (t *SimplifiedTrackerTree) processCalleesDeep(node *SimplifiedTrackerNode, edge *metadata.CallGraphEdge) {
	if edge == nil || t.nodeCount >= t.limits.MaxNodesPerTree {
		return
	}

	// Find all edges where this function is the caller
	callerBaseID := edge.Callee.BaseID()
	if callers, exists := t.meta.Callers[callerBaseID]; exists {
		for _, callerEdge := range callers {
			// Check depth limits
			if t.nodeCount >= t.limits.MaxNodesPerTree {
				return
			}

			// Check if we already have a node for this callee
			calleeKey := callerEdge.Callee.BaseID()

			// Don't create a node if it's the same as the current node
			if calleeKey == node.Key {
				continue
			}

			// Check if we already have this node as a child
			isAlreadyChild := false
			for _, child := range node.Children {
				if child.Key == calleeKey {
					isAlreadyChild = true
					break
				}
			}

			if !isAlreadyChild {
				// Create callee node
				calleeNode := t.createNodeFromEdge(callerEdge, node)
				if calleeNode != nil {
					node.Children = append(node.Children, calleeNode)
					t.nodeCount++

					// Recursively build the callee's structure
					t.buildNodeDeepStructure(calleeNode, callerEdge)
				}
			}
		}
	}
}

// createAssignmentNode creates a node for an assignment
func (t *SimplifiedTrackerTree) createAssignmentNode(assignment metadata.Assignment, parent *SimplifiedTrackerNode, edge *metadata.CallGraphEdge) *SimplifiedTrackerNode {
	node := &SimplifiedTrackerNode{
		Key:           fmt.Sprintf("assignment:%s", assignment.Lhs.GetName()),
		Parent:        parent,
		Edge:          edge,
		ArgType:       metadata.ArgTypeVariable,
		ArgContext:    fmt.Sprintf("assignment in %s", t.meta.StringPool.GetString(assignment.Func)),
		AssignmentMap: make(map[string][]metadata.Assignment),
	}

	return node
}

// createVariableNode creates a node for a variable
func (t *SimplifiedTrackerTree) createVariableNode(paramName string, arg *metadata.CallArgument, parent *SimplifiedTrackerNode, edge *metadata.CallGraphEdge) *SimplifiedTrackerNode {
	node := &SimplifiedTrackerNode{
		Key:         fmt.Sprintf("variable:%s", paramName),
		Parent:      parent,
		Edge:        edge,
		Argument:    arg,
		ArgType:     metadata.ArgTypeVariable,
		ArgContext:  fmt.Sprintf("parameter in %s", edge.Callee.BaseID()),
		VariableMap: make(map[string]*metadata.CallArgument),
	}

	return node
}

// findNodeByKey recursively searches for a node with the given key
func (t *SimplifiedTrackerTree) findNodeByKey(key string, startNode *SimplifiedTrackerNode) *SimplifiedTrackerNode {
	if startNode.Key == key {
		return startNode
	}

	for _, child := range startNode.Children {
		if found := t.findNodeByKey(key, child); found != nil {
			return found
		}
	}

	return nil
}

// createNodeFromEdge creates a node from a call graph edge
func (t *SimplifiedTrackerTree) createNodeFromEdge(edge *metadata.CallGraphEdge, parent *SimplifiedTrackerNode) *SimplifiedTrackerNode {
	node := &SimplifiedTrackerNode{
		Key:           edge.Caller.BaseID(), // Use BaseID for root nodes (no generics, no position)
		Parent:        parent,
		Edge:          edge,
		TypeParamMap:  make(map[string]string),
		AssignmentMap: make(map[string][]metadata.Assignment),
		VariableMap:   make(map[string]*metadata.CallArgument),
	}

	// Copy type parameters if present
	if edge.TypeParamMap != nil {
		for k, v := range edge.TypeParamMap {
			node.TypeParamMap[k] = v
		}
	}

	return node
}

// createNodeFromCallee creates a node from a callee in a call graph edge
func (t *SimplifiedTrackerTree) createNodeFromCallee(edge *metadata.CallGraphEdge, parent *SimplifiedTrackerNode) *SimplifiedTrackerNode {
	node := &SimplifiedTrackerNode{
		Key:           edge.Callee.GenericID(),
		Parent:        parent,
		Edge:          edge,
		TypeParamMap:  make(map[string]string),
		AssignmentMap: make(map[string][]metadata.Assignment),
		VariableMap:   make(map[string]*metadata.CallArgument),
	}

	// Copy type parameters if present
	if edge.TypeParamMap != nil {
		for k, v := range edge.TypeParamMap {
			node.TypeParamMap[k] = v
		}
	}

	return node
}

// createNodeFromArgument creates a node from a call argument
func (t *SimplifiedTrackerTree) createNodeFromArgument(arg *metadata.CallArgument, parent *SimplifiedTrackerNode, edge *metadata.CallGraphEdge, index int) *SimplifiedTrackerNode {
	if arg == nil {
		return nil
	}

	node := &SimplifiedTrackerNode{
		Key:           arg.ID(),
		Parent:        parent,
		Edge:          edge,
		Argument:      arg,
		ArgType:       t.classifyArgument(arg),
		ArgIndex:      index,
		ArgContext:    fmt.Sprintf("arg %d in %s", index, edge.Caller.BaseID()),
		TypeParamMap:  make(map[string]string),
		AssignmentMap: make(map[string][]metadata.Assignment),
		VariableMap:   make(map[string]*metadata.CallArgument),
	}

	return node
}

// classifyArgument determines the type of an argument
func (t *SimplifiedTrackerTree) classifyArgument(arg *metadata.CallArgument) metadata.ArgumentType {
	switch arg.GetKind() {
	case metadata.KindCall:
		return metadata.ArgTypeFunctionCall
	case metadata.KindIdent:
		if strings.HasPrefix(arg.GetType(), "func(") {
			return metadata.ArgTypeFunctionCall
		}
		return metadata.ArgTypeVariable
	case metadata.KindLiteral:
		return metadata.ArgTypeLiteral
	case metadata.KindSelector:
		return metadata.ArgTypeSelector
	case metadata.KindCompositeLit:
		return metadata.ArgTypeComposite
	case metadata.KindUnary:
		return metadata.ArgTypeUnary
	case metadata.KindBinary:
		return metadata.ArgTypeBinary
	case metadata.KindIndex:
		return metadata.ArgTypeIndex
	case metadata.KindTypeAssert:
		return metadata.ArgTypeTypeAssert
	default:
		return metadata.ArgTypeComplex
	}
}

// buildNodeChildren builds children for a node using metadata relationships
func (t *SimplifiedTrackerTree) buildNodeChildren(node *SimplifiedTrackerNode, edge *metadata.CallGraphEdge) {
	if edge == nil || t.nodeCount >= t.limits.MaxNodesPerTree {
		return
	}

	// Process arguments
	t.processArgumentsDeep(node, edge)

	// Link related assignments
	t.linkRelatedAssignments(node, edge)

	// Link related variables
	t.linkRelatedVariables(node, edge)

	// Process callees
	t.processCalleesDeep(node, edge)
}

// isGenericTypeCompatible checks if generic types are compatible
func (t *SimplifiedTrackerTree) isGenericTypeCompatible(caller, callee *metadata.CallGraphEdge) bool {
	if caller == nil || callee == nil {
		return false
	}

	// Extract generic types
	callerTypes := metadata.ExtractGenericTypes(caller.Caller.ID())
	calleeTypes := metadata.ExtractGenericTypes(callee.Callee.ID())

	// Check if callee types are a subset of caller types
	return metadata.IsSubset(callerTypes, calleeTypes)
}

// getNodeDepth gets the depth of a node in the tree
func (t *SimplifiedTrackerTree) getNodeDepth(node *SimplifiedTrackerNode) int {
	if node == nil {
		return 0
	}

	depth := 0
	current := node
	for current.Parent != nil {
		depth++
		current = current.Parent
	}

	return depth
}

// GetRoots returns the root nodes of the tracker tree
func (t *SimplifiedTrackerTree) GetRoots() []TrackerNodeInterface {
	if t == nil {
		return nil
	}

	roots := make([]TrackerNodeInterface, len(t.roots))
	for i, root := range t.roots {
		roots[i] = root
	}

	return roots
}

// GetNodeCount returns the total number of nodes in the tree
func (t *SimplifiedTrackerTree) GetNodeCount() int {
	if t == nil {
		return 0
	}

	// Count all nodes recursively, but only count reachable nodes
	count := 0
	visited := make(map[string]bool)

	for _, root := range t.roots {
		count += t.countNodesRecursiveReachable(root, visited)
	}

	return count
}

// countNodesRecursiveReachable counts only reachable nodes
func (t *SimplifiedTrackerTree) countNodesRecursiveReachable(node *SimplifiedTrackerNode, visited map[string]bool) int {
	if node == nil || visited[node.Key] {
		return 0
	}

	visited[node.Key] = true
	count := 1 // Count this node

	for _, child := range node.Children {
		count += t.countNodesRecursiveReachable(child, visited)
	}

	return count
}

// FindNodeByKey finds a node by its key
func (t *SimplifiedTrackerTree) FindNodeByKey(key string) TrackerNodeInterface {
	if t == nil {
		return nil
	}

	for _, root := range t.roots {
		if found := t.findNodeByKey(key, root); found != nil {
			return found
		}
	}

	return nil
}

// GetFunctionContext gets the function context for a node
func (t *SimplifiedTrackerTree) GetFunctionContext(functionName string) (*metadata.Function, string, string) {
	if t == nil || t.meta == nil {
		return nil, "", ""
	}

	// Search through packages to find the function
	for pkgName, pkg := range t.meta.Packages {
		for fileName, file := range pkg.Files {
			for _, fn := range file.Functions {
				if t.meta.StringPool.GetString(fn.Name) == functionName {
					return fn, pkgName, fileName
				}
			}
		}
	}

	return nil, "", ""
}

// TraverseTree traverses the tree with a visitor function
func (t *SimplifiedTrackerTree) TraverseTree(visitor func(TrackerNodeInterface) bool) {
	if t == nil || visitor == nil {
		return
	}

	for _, root := range t.roots {
		if !t.traverseNodeRecursive(root, visitor) {
			break
		}
	}
}

// traverseNodeRecursive traverses a node recursively
func (t *SimplifiedTrackerTree) traverseNodeRecursive(node *SimplifiedTrackerNode, visitor func(TrackerNodeInterface) bool) bool {
	if node == nil {
		return true
	}

	// Visit this node
	if !visitor(node) {
		return false
	}

	// Visit children
	for _, child := range node.Children {
		if !t.traverseNodeRecursive(child, visitor) {
			return false
		}
	}

	return true
}

// GetMetadata returns the metadata used by this tree
func (t *SimplifiedTrackerTree) GetMetadata() *metadata.Metadata {
	return t.meta
}

// GetLimits returns the limits used by this tree
func (t *SimplifiedTrackerTree) GetLimits() metadata.TrackerLimits {
	return t.limits
}

// GetKey returns the unique key of the node
func (n *SimplifiedTrackerNode) GetKey() string {
	return n.Key
}

// GetParent returns the parent node
func (n *SimplifiedTrackerNode) GetParent() TrackerNodeInterface {
	if n.Parent == nil {
		return nil
	}
	return n.Parent
}

// GetChildren returns the children nodes
func (n *SimplifiedTrackerNode) GetChildren() []TrackerNodeInterface {
	children := make([]TrackerNodeInterface, len(n.Children))
	for i, child := range n.Children {
		children[i] = child
	}
	return children
}

// GetEdge returns the call graph edge
func (n *SimplifiedTrackerNode) GetEdge() *metadata.CallGraphEdge {
	return n.Edge
}

// GetArgument returns the call argument
func (n *SimplifiedTrackerNode) GetArgument() *metadata.CallArgument {
	return n.Argument
}

// GetArgType returns the argument type
func (n *SimplifiedTrackerNode) GetArgType() metadata.ArgumentType {
	return n.ArgType
}

// GetArgIndex returns the argument index
func (n *SimplifiedTrackerNode) GetArgIndex() int {
	return n.ArgIndex
}

// GetArgContext returns the argument context
func (n *SimplifiedTrackerNode) GetArgContext() string {
	return n.ArgContext
}

// GetTypeParamMap returns the type parameter map
func (n *SimplifiedTrackerNode) GetTypeParamMap() map[string]string {
	return n.TypeParamMap
}

// GetRootAssignmentMap returns the root assignment map
func (n *SimplifiedTrackerNode) GetRootAssignmentMap() map[string][]metadata.Assignment {
	return n.AssignmentMap
}
