package spec

import (
	"fmt"
	"strings"

	"github.com/ehabterra/apispec/internal/metadata"
)

const (
	mermaidGraphHeader = "graph LR\n"
	nodePrefix         = "node_"
	edgePrefix         = "edge_"
)

// CallPathInfo represents information for a specific call path
type CallPathInfo struct {
	CallerPkg     string            `json:"caller_pkg,omitempty"`
	CallerName    string            `json:"caller_name,omitempty"`
	Position      string            `json:"position,omitempty"`
	ParamValues   []string          `json:"param_values,omitempty"`
	GenericValues map[string]string `json:"generic_values,omitempty"`
	// Enhanced FuncLit information
	FuncLitInfo *FuncLitInfo `json:"func_lit_info,omitempty"`
}

type FuncLitInfo struct {
	Position  string `json:"position,omitempty"`
	Package   string `json:"package,omitempty"`
	Signature string `json:"signature,omitempty"`
}

// DrawTrackerTree generates a Mermaid graph for the tracker tree.
func DrawTrackerTree(nodes []TrackerNodeInterface) string {
	var str = strings.Builder{}
	var counter = 0
	str.WriteString(mermaidGraphHeader)
	for _, node := range nodes {
		drawNode(node, &str, &counter)
	}
	return str.String()
}

func drawNode(node TrackerNodeInterface, str *strings.Builder, counter *int) {
	nodeID := fmt.Sprintf("%s%d", nodePrefix, *counter)
	for _, child := range node.GetChildren() {
		*counter++
		fmt.Fprintf(str, "  %s[%q] --> %s[%q]\n", nodeID, node.GetKey(), fmt.Sprintf("%s%d", nodePrefix, *counter), child.GetKey())
		drawNode(child, str, counter)
	}
}

// CytoscapeData represents the data structure for Cytoscape.js
// and related node/edge types.
type CytoscapeData struct {
	Nodes []CytoscapeNode `json:"nodes"`
	Edges []CytoscapeEdge `json:"edges"`
}

type CytoscapeNode struct {
	Data CytoscapeNodeData `json:"data"`
}

type CytoscapeNodeData struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Parent   string `json:"parent,omitempty"`
	Group    string `json:"group,omitempty"`
	Position string `json:"position,omitempty"`
	Type     string `json:"type,omitempty"`
	Depth    int    `json:"depth,omitempty"` // Depth level in the tree (0 = root)

	// Enhanced data for popup display
	Package          string            `json:"package,omitempty"`
	CallPaths        []CallPathInfo    `json:"call_paths,omitempty"`
	Generics         map[string]string `json:"generics,omitempty"`
	FunctionName     string            `json:"function_name,omitempty"`
	ReceiverType     string            `json:"receiver_type,omitempty"`
	IsParentFunction string            `json:"is_parent_function,omitempty"`
	Scope            string            `json:"scope,omitempty"`

	// Additional function metadata
	SignatureStr string `json:"signature_str,omitempty"`

	// Tracker tree specific data
	ArgType         string         `json:"arg_type,omitempty"`
	ArgIndex        int            `json:"arg_index,omitempty"`
	ArgContext      string         `json:"arg_context,omitempty"`
	ArgName         string         `json:"arg_name,omitempty"`
	ArgValue        string         `json:"arg_value,omitempty"`
	ArgResolvedType string         `json:"arg_resolved_type,omitempty"`
	RootAssignments map[string]int `json:"root_assignments,omitempty"`
}

type CytoscapeEdge struct {
	Data CytoscapeEdgeData `json:"data"`
}

type CytoscapeEdgeData struct {
	ID     string `json:"id"`
	Source string `json:"source"`
	Target string `json:"target"`
	Label  string `json:"label,omitempty"`
	Type   string `json:"type,omitempty"`
}

// DrawTrackerTreeCytoscape generates Cytoscape.js JSON data for the tracker tree.
func DrawTrackerTreeCytoscape(nodes []TrackerNodeInterface) *CytoscapeData {
	return DrawTrackerTreeCytoscapeWithMetadata(nodes, nil)
}

// DrawTrackerTreeCytoscapeWithMetadata generates Cytoscape.js JSON data for the tracker tree with metadata.
func DrawTrackerTreeCytoscapeWithMetadata(nodes []TrackerNodeInterface, meta *metadata.Metadata) *CytoscapeData {
	data := &CytoscapeData{
		Nodes: make([]CytoscapeNode, 0),
		Edges: make([]CytoscapeEdge, 0),
	}
	// Use base keys for node mapping to merge occurrences
	nodeMap := make(map[string]string)         // baseKey -> nodeID
	baseKeyToNodeIndex := make(map[string]int) // baseKey -> index in data.Nodes
	edgeCounter := 0
	nodeCounter := 0
	depth := 0 // Start at depth 0 for root nodes

	// Track edges by base keys to avoid duplicates
	edgeSet := make(map[string]bool) // "sourceBaseKey->targetBaseKey" -> bool

	// Track which base keys have had their children processed to prevent infinite loops
	childrenProcessed := make(map[string]bool) // baseKey -> bool

	for _, node := range nodes {
		_ = drawNodeCytoscapeWithDepth(node, data, nodeMap, baseKeyToNodeIndex, edgeSet, childrenProcessed, &edgeCounter, &nodeCounter, meta, depth)
	}
	return data
}

// OrderTrackerTreeNodesDepthFirst orders Cytoscape nodes from a tracker tree in depth-first order
// starting from root nodes (main function) down to leaves, across all branches
func OrderTrackerTreeNodesDepthFirst(data *CytoscapeData) []CytoscapeNode {
	if len(data.Nodes) == 0 {
		return data.Nodes
	}

	// Build maps for quick lookup
	nodeMap := make(map[string]*CytoscapeNode)
	for i := range data.Nodes {
		nodeMap[data.Nodes[i].Data.ID] = &data.Nodes[i]
	}

	// Build edge adjacency map (parent -> children)
	edgesBySource := make(map[string][]string)
	for _, edge := range data.Edges {
		edgesBySource[edge.Data.Source] = append(edgesBySource[edge.Data.Source], edge.Data.Target)
	}

	// Find root nodes (nodes with depth 0 or no incoming edges)
	hasIncoming := make(map[string]bool)
	for _, edge := range data.Edges {
		hasIncoming[edge.Data.Target] = true
	}

	var roots []*CytoscapeNode
	var mainRoot *CytoscapeNode
	var otherRoots []*CytoscapeNode
	minDepth := 999999

	for i := range data.Nodes {
		node := &data.Nodes[i]
		// Track minimum depth
		if node.Data.Depth < minDepth {
			minDepth = node.Data.Depth
		}

		// Check if this is the main root node
		isMain := node.Data.ID == "node_0" || node.Data.Label == "main"
		isRoot := !hasIncoming[node.Data.ID] || node.Data.Depth == 0

		// Root if no incoming edges or depth is minimum (likely 0)
		if isRoot {
			if isMain {
				mainRoot = node
			} else {
				otherRoots = append(otherRoots, node)
			}
		}
	}

	// Prioritize main root first
	if mainRoot != nil {
		roots = append(roots, mainRoot)
		roots = append(roots, otherRoots...)
	} else if len(otherRoots) > 0 {
		roots = otherRoots
	} else {
		// If no roots found by depth 0, use minimum depth nodes
		for i := range data.Nodes {
			node := &data.Nodes[i]
			if node.Data.Depth == minDepth {
				if node.Data.ID == "node_0" || node.Data.Label == "main" {
					roots = append([]*CytoscapeNode{node}, roots...)
				} else {
					roots = append(roots, node)
				}
			}
		}
	}

	var orderedNodes []CytoscapeNode
	visited := make(map[string]bool)

	// Depth-first traversal function
	var dfs func(nodeID string)
	dfs = func(nodeID string) {
		if visited[nodeID] {
			return
		}
		visited[nodeID] = true

		if node, exists := nodeMap[nodeID]; exists {
			orderedNodes = append(orderedNodes, *node)
		}

		// Process children in order (depth-first)
		if children, exists := edgesBySource[nodeID]; exists {
			for _, childID := range children {
				dfs(childID)
			}
		}
	}

	// Start DFS from all root nodes
	for _, root := range roots {
		dfs(root.Data.ID)
	}

	// Add any remaining unvisited nodes (orphaned nodes, should be rare)
	for i := range data.Nodes {
		node := &data.Nodes[i]
		if !visited[node.Data.ID] {
			orderedNodes = append(orderedNodes, *node)
		}
	}

	return orderedNodes
}

// TraverseTrackerTreeBranchOrder returns nodes in branch-first order:
// Complete one branch (with all sub-branches) depth-first before moving to next branch.
// Each node appears exactly once in the order.
func TraverseTrackerTreeBranchOrder(data *CytoscapeData) []CytoscapeNode {
	if len(data.Nodes) == 0 {
		return nil
	}

	// Build fast lookups
	nodeMap := make(map[string]*CytoscapeNode)
	for i := range data.Nodes {
		nodeMap[data.Nodes[i].Data.ID] = &data.Nodes[i]
	}

	// Consider only function call edges to avoid cycles from argument edges
	children := make(map[string][]string)
	hasIncoming := make(map[string]bool)
	for _, e := range data.Edges {
		if e.Data.Type != "calls" {
			continue
		}
		children[e.Data.Source] = append(children[e.Data.Source], e.Data.Target)
		hasIncoming[e.Data.Target] = true
	}

	// Find roots, prioritizing "main" node (node_0 or label "main")
	var roots []string
	var mainRoot string
	var otherRoots []string
	minDepth := 1 << 30

	for i := range data.Nodes {
		n := &data.Nodes[i]
		if n.Data.Depth < minDepth {
			minDepth = n.Data.Depth
		}

		// Check if this is the main root node
		isMain := n.Data.ID == "node_0" || n.Data.Label == "main"
		isRoot := !hasIncoming[n.Data.ID] || n.Data.Depth == 0

		if isRoot {
			if isMain {
				mainRoot = n.Data.ID
			} else {
				otherRoots = append(otherRoots, n.Data.ID)
			}
		}
	}

	// Prioritize main root, fallback to minimum depth if no roots found
	if mainRoot != "" {
		roots = append(roots, mainRoot)
		roots = append(roots, otherRoots...)
	} else if len(otherRoots) > 0 {
		roots = otherRoots
	} else {
		// Fallback: use minimum depth nodes
		for i := range data.Nodes {
			n := &data.Nodes[i]
			if n.Data.Depth == minDepth {
				// Still prioritize main if found
				if n.Data.ID == "node_0" || n.Data.Label == "main" {
					roots = append([]string{n.Data.ID}, roots...)
				} else {
					roots = append(roots, n.Data.ID)
				}
			}
		}
	}

	var result []CytoscapeNode
	visited := make(map[string]bool)

	// DFS that visits each node exactly once, completing one branch before moving to next
	var dfs func(id string)
	dfs = func(id string) {
		if visited[id] {
			return // Already visited
		}
		visited[id] = true

		if node, ok := nodeMap[id]; ok {
			result = append(result, *node)
		}

		// Process all children depth-first
		if childIDs, exists := children[id]; exists {
			for _, childID := range childIDs {
				dfs(childID)
			}
		}
	}

	// Process each root branch completely before moving to next
	for _, r := range roots {
		dfs(r)
	}

	// Add any orphaned nodes
	for i := range data.Nodes {
		n := &data.Nodes[i]
		if !visited[n.Data.ID] {
			result = append(result, *n)
		}
	}

	return result
}

// DrawCallGraphCytoscape generates Cytoscape.js JSON data directly from call graph metadata.
func DrawCallGraphCytoscape(meta *metadata.Metadata) *CytoscapeData {
	data := &CytoscapeData{
		Nodes: make([]CytoscapeNode, 0),
		Edges: make([]CytoscapeEdge, 0),
	}

	// Handle nil metadata
	if meta == nil {
		return data
	}

	// Build lookup maps if not already built
	if meta.Callers == nil {
		meta.BuildCallGraphMaps()
	}

	// Track visited nodes to avoid duplicates
	visitedNodes := make(map[string]bool)
	nodePairEdges := make(map[string]bool) // Track edges between node pairs to ensure only one arrow per pair
	edgeIDNodeMap := make(map[string]string)

	nodeCounter := 0
	edgeCounter := 0

	// Process all call graph edges to ensure all nodes are included
	// This includes functions that appear as arguments but aren't reachable from roots
	for i := range meta.CallGraph {
		edge := &meta.CallGraph[i]
		processCallGraphEdge(meta, edge, data, visitedNodes, nodePairEdges, edgeIDNodeMap, &nodeCounter, &edgeCounter)
	}

	return data
}

// processCallGraphEdge processes a call graph edge and adds nodes/edges to the Cytoscape data
func processCallGraphEdge(meta *metadata.Metadata, edge *metadata.CallGraphEdge, data *CytoscapeData, visitedNodes, nodePairEdges map[string]bool, edgeIDNodeMap map[string]string, nodeCounter, edgeCounter *int) {
	if edge == nil {
		return
	}

	// Create caller node
	callerID := edge.Caller.BaseID()
	if !visitedNodes[callerID] {
		*nodeCounter++
		callerNodeID := fmt.Sprintf("node_%d", *nodeCounter)
		visitedNodes[callerID] = true
		edgeIDNodeMap[callerID] = callerNodeID

		callerName := meta.StringPool.GetString(edge.Caller.Name)
		callerPkg := meta.StringPool.GetString(edge.Caller.Pkg)
		receiverType := meta.StringPool.GetString(edge.Caller.RecvType)
		callerPosition := meta.StringPool.GetString(edge.Caller.Position)

		if strings.HasPrefix(callerName, "FuncLit:") {
			callerPosition = callerName[len("FuncLit:"):]
		}

		// Extract function-level parameter types and generics (from any call to this function)
		var generics map[string]string

		if len(edge.TypeParamMap) > 0 {
			generics = edge.TypeParamMap
		}

		signatureStr := meta.StringPool.GetString(edge.Caller.SignatureStr)

		// Build function label (just the name)
		label := callerName
		// Fix FuncLit naming for labels
		if strings.HasPrefix(callerName, "FuncLit:") {
			label = "FuncLit"
		}
		if receiverType != "" {
			label = receiverType + "." + label
		}

		// Create position info from call paths
		positionInfo := callerPosition
		if positionInfo == "" {
			if callerName == "main" {
				positionInfo = "root function"
			} else {
				positionInfo = "entry point"
			}
		}

		// Determine parent for FuncLit caller using ParentFunction
		var parentID string
		if strings.HasPrefix(callerName, "FuncLit:") && edge.ParentFunction != nil {
			// For FuncLit caller, use the parent function as the parent
			parentFuncName := meta.StringPool.GetString(edge.ParentFunction.Name)
			parentFuncPkg := meta.StringPool.GetString(edge.ParentFunction.Pkg)

			// Find the parent function node ID
			for _, node := range data.Nodes {
				if node.Data.FunctionName == parentFuncName && node.Data.Package == parentFuncPkg {
					parentID = node.Data.ID
					break
				}
			}

			// If parent node doesn't exist yet, create it
			if parentID == "" {
				parentID = ensureParentFunctionNode(meta, edge.ParentFunction, data, visitedNodes, nodeCounter)
			}
		}

		callerScope := edge.Caller.GetScope()

		data.Nodes = append(data.Nodes, CytoscapeNode{
			Data: CytoscapeNodeData{
				ID:           callerNodeID,
				Label:        label,
				Parent:       parentID,
				Type:         "function",
				Package:      callerPkg,
				Generics:     generics,
				FunctionName: callerName,
				ReceiverType: receiverType,
				Position:     positionInfo,
				SignatureStr: signatureStr,
				Scope:        callerScope,
			},
		})

		// Note: For FuncLit nodes, the parent relationship is set via the Parent field in the node data
		// No separate "contains" edge is needed as Cytoscape will handle compound nodes automatically
	}

	// Create callee node
	calleeID := edge.Callee.BaseID()
	if !visitedNodes[calleeID] {
		*nodeCounter++
		calleeNodeID := fmt.Sprintf("node_%d", *nodeCounter)
		visitedNodes[calleeID] = true
		edgeIDNodeMap[calleeID] = calleeNodeID

		calleeName := meta.StringPool.GetString(edge.Callee.Name)
		calleePkg := meta.StringPool.GetString(edge.Callee.Pkg)
		receiverType := meta.StringPool.GetString(edge.Callee.RecvType)

		// Build detailed call path information for this function (who calls this function)
		callPathInfos := buildCallPathInfos(meta, edge.Callee.BaseID())

		// Extract function-level parameter types and generics (from any call to this function)
		var generics map[string]string
		if len(callPathInfos) > 0 {
			// Use the first call path to get function-level parameter types
			generics = make(map[string]string)
			if edge.TypeParamMap != nil {
				generics = edge.TypeParamMap
			}
		}

		signatureStr := meta.StringPool.GetString(edge.Callee.SignatureStr)

		// Build function label (just the name)
		label := calleeName
		// Fix FuncLit naming for labels
		if strings.HasPrefix(calleeName, "FuncLit:") {
			label = "FuncLit"
		}
		if receiverType != "" {
			label = receiverType + "." + label
		}

		// Determine parent for FuncLit caller using ParentFunction
		var parentID string
		if strings.HasPrefix(calleeName, "FuncLit:") && edge.ParentFunction != nil {
			// For FuncLit caller, use the parent function as the parent
			parentFuncName := meta.StringPool.GetString(edge.ParentFunction.Name)
			parentFuncPkg := meta.StringPool.GetString(edge.ParentFunction.Pkg)

			// Find the parent function node ID
			for _, node := range data.Nodes {
				if node.Data.FunctionName == parentFuncName && node.Data.Package == parentFuncPkg {
					parentID = node.Data.ID
					break
				}
			}

			// If parent node doesn't exist yet, create it
			if parentID == "" {
				parentID = ensureParentFunctionNode(meta, edge.ParentFunction, data, visitedNodes, nodeCounter)
			}
		}

		calleeScope := edge.Callee.GetScope()

		data.Nodes = append(data.Nodes, CytoscapeNode{
			Data: CytoscapeNodeData{
				ID:           calleeNodeID,
				Label:        label,
				Parent:       parentID,
				Type:         "function",
				Package:      calleePkg,
				CallPaths:    callPathInfos,
				Generics:     generics,
				FunctionName: calleeName,
				ReceiverType: receiverType,
				Position:     "", // Don't show position for callee nodes
				SignatureStr: signatureStr,
				Scope:        calleeScope,
			},
		})
	}

	// Find the node IDs for caller and callee
	var callerNodeID, calleeNodeID string
	callerNodeID = edgeIDNodeMap[callerID]
	calleeNodeID = edgeIDNodeMap[calleeID]

	// Create edge between caller and callee only if we haven't already created one between these nodes
	if callerNodeID != "" && calleeNodeID != "" {
		// Create a unique key for this node pair (source -> target)
		nodePairKey := callerNodeID + "->" + calleeNodeID

		// Only create edge if we haven't already created one between these nodes
		if !nodePairEdges[nodePairKey] {
			edgeID := fmt.Sprintf("edge_%d", *edgeCounter)
			*edgeCounter++

			data.Edges = append(data.Edges, CytoscapeEdge{
				Data: CytoscapeEdgeData{
					ID:     edgeID,
					Source: callerNodeID,
					Target: calleeNodeID,
					Type:   "calls",
				},
			})

			// Mark this node pair as having an edge
			nodePairEdges[nodePairKey] = true
		}
	}
}

// buildCallPaths builds a list of direct call paths for a function with position information
func buildCallPaths(meta *metadata.Metadata, functionID string) []string {
	var paths []string

	// Get all direct callers of this function (only immediate callers, not full call chains)
	if callers, exists := meta.Callees[functionID]; exists {
		seen := make(map[string]bool)
		for _, caller := range callers {
			callerName := meta.StringPool.GetString(caller.Caller.Name)
			callerPkg := meta.StringPool.GetString(caller.Caller.Pkg)
			callerPosition := meta.StringPool.GetString(caller.Caller.Position)

			// Create path with position information
			path := callerPkg + "." + callerName
			if callerPosition != "" {
				path += " @ " + callerPosition
			}

			// Only add unique paths to avoid duplicates
			if !seen[path] {
				paths = append(paths, path)
				seen[path] = true
			}
		}
	}

	return paths
}

// buildCallPathInfos builds detailed call path information for a function
// It shows who calls this function (the callers of this function)
func buildCallPathInfos(meta *metadata.Metadata, functionID string) []CallPathInfo {
	var callPathInfos []CallPathInfo

	// Get all edges where this function is the callee (being called)
	if callees, exists := meta.Callees[functionID]; exists {
		seen := make(map[string]bool)
		for _, edge := range callees {
			// The caller is the function that calls the given function
			callerName := meta.StringPool.GetString(edge.Caller.Name)
			callerPkg := meta.StringPool.GetString(edge.Caller.Pkg)

			// Use edge position (where the call is made)
			position := meta.StringPool.GetString(edge.Position)

			// Fix FuncLit naming - extract position from name if it's a FuncLit
			var funcLitInfo *FuncLitInfo
			if strings.HasPrefix(callerName, "FuncLit:") {
				// Extract position from FuncLit name (format: FuncLit:position)
				parts := strings.SplitN(callerName, ":", 2)
				if len(parts) == 2 {
					callerName = "FuncLit"
					// If we don't have position from edge, use the one from the name
					if position == "" {
						position = parts[1]
					}

					// Create FuncLit info
					funcLitInfo = &FuncLitInfo{
						Position: parts[1],
						Package:  callerPkg,
					}

					// Try to get signature from the edge's caller information
					// Get signature from the caller's type information
					signature := extractFuncLitSignature(meta, &edge.Caller)
					if signature != "" {
						funcLitInfo.Signature = signature
					}
				}
			}

			// Extract parameter values for this specific call
			_, paramValues := extractParameterInfo(edge)

			// Extract generic values for this specific call
			genericValues := make(map[string]string)
			if edge.TypeParamMap != nil {
				genericValues = edge.TypeParamMap
			}

			// Create unique key for this call path
			pathKey := callerPkg + "." + callerName + ":" + position
			if !seen[pathKey] {
				callPathInfos = append(callPathInfos, CallPathInfo{
					CallerPkg:     callerPkg,
					CallerName:    callerName,
					Position:      position,
					ParamValues:   paramValues,
					GenericValues: genericValues,
					FuncLitInfo:   funcLitInfo,
				})
				seen[pathKey] = true
			}
		}
	}

	return callPathInfos
}

// extractParameterInfo extracts parameter types and passed parameters from a call graph edge
func extractParameterInfo(edge *metadata.CallGraphEdge) ([]string, []string) {
	var paramTypes []string
	var passedParams []string

	// Extract from ParamArgMap if available (this gives us parameter name -> argument mapping)
	if edge.ParamArgMap != nil {
		for paramName, arg := range edge.ParamArgMap {
			// Get the parameter type
			argType := arg.GetType()
			if argType == "" {
				argType = arg.GetResolvedType()
			}
			if argType == "" {
				argType = "unknown"
			}
			paramTypes = append(paramTypes, fmt.Sprintf("%s:%s", paramName, argType))

			// Get the actual value being passed
			argValue := metadata.CallArgToString(&arg)

			// Format as name: value, but handle empty parameter names
			if paramName != "" {
				passedParams = append(passedParams, paramName+": "+argValue)
			} else {
				passedParams = append(passedParams, argValue)
			}
		}
	} else {
		// Fallback: extract from Args if ParamArgMap is not available
		for i, arg := range edge.Args {
			paramType := arg.GetType()
			if paramType == "" {
				paramType = arg.GetResolvedType()
			}
			if paramType == "" {
				paramType = "unknown"
			}
			paramTypes = append(paramTypes, fmt.Sprintf("arg%d:%s", i, paramType))

			// Get the actual value being passed and format as name: value
			argValue := arg.GetValue()
			if argValue == "" {
				argValue = arg.GetName()
			}
			if argValue == "" {
				argValue = arg.GetRaw()
			}
			if argValue == "" {
				argValue = "nil"
			}
			passedParams = append(passedParams, fmt.Sprintf("arg%d: %s", i, argValue))
		}
	}

	return paramTypes, passedParams
}

// splitNodeLabel splits a node key by @ sign and returns label and position
func splitNodeLabel(nodeKey string) (string, string) {
	parts := strings.Split(nodeKey, "@")
	if len(parts) >= 2 {
		// Return first part as label, second part as position
		return parts[0], parts[1]
	}
	// If no @ sign, return the whole key as label with empty position
	return nodeKey, ""
}

func callArgument(callArgument *metadata.CallArgument) *metadata.CallArgument {
	if callArgument == nil {
		return nil
	}

	if callArgument.GetName() != "" || callArgument.GetValue() != "" {
		return callArgument
	}

	switch {
	case callArgument.Fun != nil:
		return callArgument.Fun
	case callArgument.Sel != nil:
		return callArgument.Sel
	case callArgument.X != nil:
		return callArgument.X
	}

	return callArgument
}

func drawNodeCytoscapeWithDepth(node TrackerNodeInterface, data *CytoscapeData, nodeMap map[string]string, baseKeyToNodeIndex map[string]int, edgeSet map[string]bool, childrenProcessed map[string]bool, edgeCounter, nodeCounter *int, meta *metadata.Metadata, depth int) string {
	if node == nil {
		return ""
	}

	// Get base key (without position) to merge all occurrences
	nodeKey := node.GetKey()
	baseKey := metadata.StripToBase(nodeKey)

	// Extract position from nodeKey (after @ character)
	_, positionFromLabel := splitNodeLabel(nodeKey)

	// Check if a node with this base key already exists
	var nodeID string
	var nodeIndex int
	var nodeData *CytoscapeNodeData
	isNewNode := false

	if existingID, exists := nodeMap[baseKey]; exists {
		// Node with same base key exists, merge into it
		nodeID = existingID
		nodeIndex = baseKeyToNodeIndex[baseKey]
		nodeData = &data.Nodes[nodeIndex].Data
	} else {
		// Create new node for this base key
		nodeID = fmt.Sprintf("%s%d", nodePrefix, *nodeCounter)
		*nodeCounter++
		nodeMap[baseKey] = nodeID

		label, _ := splitNodeLabel(baseKey)
		nodeData = &CytoscapeNodeData{
			ID:        nodeID,
			Label:     label,
			Type:      "function",
			Depth:     depth,                   // Set the depth level
			CallPaths: make([]CallPathInfo, 0), // Initialize CallPaths array
		}

		nodeIndex = len(data.Nodes)
		baseKeyToNodeIndex[baseKey] = nodeIndex
		isNewNode = true
	}

	// Add position from this occurrence if present
	if positionFromLabel != "" {
		if nodeData.Position == "" {
			nodeData.Position = positionFromLabel
		} else if !strings.Contains(nodeData.Position, positionFromLabel) {
			// Append position if not already present
			nodeData.Position += ", " + positionFromLabel
		}
	}

	// Add tracker tree specific data if available
	if trackerNode, ok := node.(*TrackerNode); ok {
		// Determine node type and set appropriate data
		if trackerNode.IsArgument {
			// This is an argument node
			if isNewNode {
				nodeData.Type = "argument"
				nodeData.ArgType = trackerNode.ArgType.String()
				nodeData.ArgIndex = trackerNode.ArgIndex
				nodeData.ArgContext = trackerNode.ArgContext

				// Add argument information if available
				if trackerNode.CallArgument != nil {
					callArgument := callArgument(trackerNode.CallArgument)
					nodeData.ArgName = callArgument.GetName()
					nodeData.Package = callArgument.GetPkg()
					nodeData.ArgValue = callArgument.GetValue()
					nodeData.ArgType = callArgument.GetType()
					nodeData.ArgResolvedType = callArgument.GetResolvedType()
				}
			}
		} else if trackerNode.CallGraphEdge != nil {
			// This is a function node with an edge
			if isNewNode {
				nodeData.Type = "function"
				// Get actual strings from metadata if available
				// Note: CallGraphEdge is checked for nil above, so we can access Callee directly
				callee := trackerNode.Callee
				if meta != nil && meta.StringPool != nil {
					nodeData.FunctionName = meta.StringPool.GetString(callee.Name)
					nodeData.Package = meta.StringPool.GetString(callee.Pkg)
					nodeData.ReceiverType = meta.StringPool.GetString(callee.RecvType)
					nodeData.SignatureStr = meta.StringPool.GetString(callee.SignatureStr)
				} else {
					// Fallback to indices if metadata not available
					nodeData.FunctionName = fmt.Sprintf("func_%d", callee.Name)
					nodeData.Package = fmt.Sprintf("pkg_%d", callee.Pkg)
					nodeData.ReceiverType = fmt.Sprintf("recv_%d", callee.RecvType)
					nodeData.SignatureStr = fmt.Sprintf("sig_%d", callee.SignatureStr)
				}
				nodeData.Scope = callee.GetScope()
			}

			// Merge CallPathInfo from this occurrence if metadata is available
			if meta != nil && meta.StringPool != nil {
				calleeBaseID := trackerNode.Callee.BaseID()
				callPathInfos := buildCallPathInfos(meta, calleeBaseID)

				// Merge call paths, avoiding duplicates
				if nodeData.CallPaths == nil {
					nodeData.CallPaths = make([]CallPathInfo, 0)
				}

				// Create a map to track unique call paths
				callPathMap := make(map[string]bool)
				for _, existingPath := range nodeData.CallPaths {
					pathKey := fmt.Sprintf("%s.%s:%s", existingPath.CallerPkg, existingPath.CallerName, existingPath.Position)
					callPathMap[pathKey] = true
				}

				// Add new call paths that aren't duplicates
				for _, newPath := range callPathInfos {
					pathKey := fmt.Sprintf("%s.%s:%s", newPath.CallerPkg, newPath.CallerName, newPath.Position)
					if !callPathMap[pathKey] {
						nodeData.CallPaths = append(nodeData.CallPaths, newPath)
						callPathMap[pathKey] = true
					}
				}

				// Set position from edge if not already set from label split
				edgePosition := meta.StringPool.GetString(trackerNode.Callee.Position)
				if edgePosition != "" {
					if nodeData.Position == "" {
						nodeData.Position = edgePosition
					} else if !strings.Contains(nodeData.Position, edgePosition) {
						nodeData.Position += ", " + edgePosition
					}
				}
			}
		} else if trackerNode.CallArgument != nil {
			// This is a call argument node (not a function)
			if isNewNode {
				nodeData.Type = "call_argument"
				callArgument := callArgument(trackerNode.CallArgument)
				nodeData.ArgName = callArgument.GetName()
				nodeData.Package = callArgument.GetPkg()
				nodeData.ArgValue = callArgument.GetValue()
				nodeData.ArgType = callArgument.GetType()
				nodeData.ArgResolvedType = callArgument.GetResolvedType()
			}
		} else {
			// This is a generic node (variable, literal, etc.)
			if isNewNode {
				nodeData.Type = "generic"
			}
		}

		// Merge root assignments if available (for any node type)
		if len(trackerNode.RootAssignmentMap) > 0 {
			if nodeData.RootAssignments == nil {
				nodeData.RootAssignments = make(map[string]int)
			}
			for key, assignments := range trackerNode.RootAssignmentMap {
				// Merge assignments count
				if existingCount, exists := nodeData.RootAssignments[key]; exists {
					nodeData.RootAssignments[key] = existingCount + len(assignments)
				} else {
					nodeData.RootAssignments[key] = len(assignments)
				}
			}
		}

		// Merge type parameters (for any node type)
		if trackerNode.TypeParams() != nil {
			if nodeData.Generics == nil {
				nodeData.Generics = make(map[string]string)
			}
			// Merge generics - use the first occurrence or merge if different
			for key, value := range trackerNode.TypeParams() {
				if existingValue, exists := nodeData.Generics[key]; !exists || existingValue == "" {
					nodeData.Generics[key] = value
				} else if existingValue != value {
					// If different values, keep both separated by comma
					nodeData.Generics[key] = existingValue + ", " + value
				}
			}
		}
	}

	// Remove package prefix from label (do this for both new and merged nodes)
	if nodeData.Package != "" {
		nodeData.Label = strings.TrimPrefix(nodeData.Label, nodeData.Package+".")
		nodeData.Label = strings.TrimPrefix(nodeData.Label, nodeData.Package+"/")
	}

	if strings.HasSuffix(nodeData.Label, ".main") {
		nodeData.Label = "main"
	}

	// Only append new node if this is the first occurrence
	if isNewNode {
		data.Nodes = append(data.Nodes, CytoscapeNode{
			Data: *nodeData,
		})
	}

	// Process children from all occurrences to merge all children
	// Use edgeSet to track which parent->child relationships we've already created
	// This prevents duplicates while allowing us to collect children from all occurrences

	// Track if we've visited this specific node occurrence to prevent infinite recursion
	// Use a composite key: baseKey + nodeKey to identify this specific occurrence
	occurrenceKey := baseKey + ":" + nodeKey
	if childrenProcessed[occurrenceKey] {
		// Already processed this specific occurrence, return to avoid infinite loop
		return nodeID
	}
	childrenProcessed[occurrenceKey] = true

	// Process children from this occurrence
	children := node.GetChildren()
	for _, child := range children {
		if child != nil {
			// Recursively process child node (it will add itself or return existing ID)
			childID := drawNodeCytoscapeWithDepth(child, data, nodeMap, baseKeyToNodeIndex, edgeSet, childrenProcessed, edgeCounter, nodeCounter, meta, depth+1)

			if childID != "" {
				// Get base keys for edge tracking
				childKey := child.GetKey()
				childBaseKey := metadata.StripToBase(childKey)

				// Create edge key using base keys to avoid duplicates across occurrences
				edgeKey := baseKey + "->" + childBaseKey

				// Only create edge if we haven't already created one between these base nodes
				if !edgeSet[edgeKey] && nodeKey != childKey {
					// Determine edge type based on node types
					edgeType := "calls" // Default for function calls
					if trackerNode, ok := node.(*TrackerNode); ok {
						if trackerNode.IsArgument || (trackerNode.CallArgument != nil && trackerNode.CallGraphEdge == nil) {
							edgeType = "argument" // Edge represents argument relationship
						}
					}

					edgeID := fmt.Sprintf("%s%d", edgePrefix, *edgeCounter)
					*edgeCounter++
					data.Edges = append(data.Edges, CytoscapeEdge{
						Data: CytoscapeEdgeData{
							ID:     edgeID,
							Source: nodeID,
							Target: childID,
							Type:   edgeType,
						},
					})

					// Mark this edge as created
					edgeSet[edgeKey] = true
				}
			}
		}
	}

	return nodeID
}

// extractFuncLitSignature extracts the signature information for a FuncLit
func extractFuncLitSignature(meta *metadata.Metadata, caller *metadata.Call) string {
	if caller == nil || meta == nil {
		return ""
	}

	// Try to get the receiver type information
	if caller.RecvType != -1 {
		recvTypeStr := meta.StringPool.GetString(caller.RecvType)
		if recvTypeStr != "" {
			return recvTypeStr
		}
	}

	// If no receiver type information is available, try to construct a basic signature
	// This is a fallback for when we don't have complete type information
	return "func()"
}

// ensureParentFunctionNode ensures that a parent function node exists for FuncLit
func ensureParentFunctionNode(meta *metadata.Metadata, parentFunc *metadata.Call, data *CytoscapeData, visitedNodes map[string]bool, nodeCounter *int) string {
	if parentFunc == nil {
		return ""
	}

	parentFuncName := meta.StringPool.GetString(parentFunc.Name)
	parentFuncPkg := meta.StringPool.GetString(parentFunc.Pkg)
	parentID := parentFunc.BaseID()

	// Check if parent node already exists
	if visitedNodes[parentID] {
		// Find the existing node ID and mark it as parent function
		for i, node := range data.Nodes {
			if node.Data.FunctionName == parentFuncName && node.Data.Package == parentFuncPkg {
				// Mark existing node as parent function
				data.Nodes[i].Data.IsParentFunction = "true"
				return node.Data.ID
			}
		}
	}

	// Create the parent function node
	*nodeCounter++
	parentNodeID := fmt.Sprintf("node_%d", *nodeCounter)
	visitedNodes[parentID] = true

	receiverType := meta.StringPool.GetString(parentFunc.RecvType)

	// Build function label
	label := parentFuncName
	if receiverType != "" {
		label = receiverType + "." + label
	}

	// Build call path infos for parent function
	callPathInfos := buildCallPathInfos(meta, parentFunc.BaseID())

	// Extract parameter types and generics
	var generics map[string]string
	if len(callPathInfos) > 0 {
		generics = make(map[string]string)
	}

	signatureStr := meta.StringPool.GetString(parentFunc.SignatureStr)

	// Create position info
	var positionInfo string

	if positionInfo == "" && parentFuncName == "main" {
		positionInfo = "root function"
	}

	parentScope := parentFunc.GetScope()

	// Create the parent function node
	data.Nodes = append(data.Nodes, CytoscapeNode{
		Data: CytoscapeNodeData{
			ID:               parentNodeID,
			Label:            label,
			Type:             "function",
			Package:          parentFuncPkg,
			CallPaths:        callPathInfos,
			Generics:         generics,
			FunctionName:     parentFuncName,
			ReceiverType:     receiverType,
			IsParentFunction: "true",
			Position:         positionInfo,
			SignatureStr:     signatureStr,
			Scope:            parentScope,
		},
	})

	return parentNodeID
}
