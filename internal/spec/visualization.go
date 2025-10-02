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
	data := &CytoscapeData{
		Nodes: make([]CytoscapeNode, 0),
		Edges: make([]CytoscapeEdge, 0),
	}
	nodeMap := make(map[string]string)
	edgeCounter := 0
	nodeCounter := 0
	for _, node := range nodes {
		drawNodeCytoscape(node, data, nodeMap, &edgeCounter, &nodeCounter)
	}
	return data
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

	// Get root functions
	roots := meta.CallGraphRoots()

	// Track visited nodes to avoid duplicates
	visitedNodes := make(map[string]bool)
	visitedEdges := make(map[string]bool)
	nodePairEdges := make(map[string]bool) // Track edges between node pairs to ensure only one arrow per pair

	nodeCounter := 0
	edgeCounter := 0

	// Process each root and its call graph
	for _, root := range roots {
		processCallGraphEdge(meta, root, data, visitedNodes, visitedEdges, nodePairEdges, &nodeCounter, &edgeCounter)
	}

	return data
}

// processCallGraphEdge processes a call graph edge and adds nodes/edges to the Cytoscape data
func processCallGraphEdge(meta *metadata.Metadata, edge *metadata.CallGraphEdge, data *CytoscapeData, visitedNodes, visitedEdges, nodePairEdges map[string]bool, nodeCounter, edgeCounter *int) {
	if edge == nil {
		return
	}

	// Create caller node
	callerID := edge.Caller.BaseID()
	if !visitedNodes[callerID] {
		*nodeCounter++
		callerNodeID := fmt.Sprintf("node_%d", *nodeCounter)
		visitedNodes[callerID] = true

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
	for _, node := range data.Nodes {
		if node.Data.FunctionName == meta.StringPool.GetString(edge.Caller.Name) &&
			node.Data.Package == meta.StringPool.GetString(edge.Caller.Pkg) {
			callerNodeID = node.Data.ID
			break
		}
	}
	for _, node := range data.Nodes {
		if node.Data.FunctionName == meta.StringPool.GetString(edge.Callee.Name) &&
			node.Data.Package == meta.StringPool.GetString(edge.Callee.Pkg) {
			calleeNodeID = node.Data.ID
			break
		}
	}

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

	// Recursively process callees
	if callees, exists := meta.Callers[calleeID]; exists {
		for _, callee := range callees {
			calleeEdgeID := callee.Caller.BaseID() + "->" + callee.Callee.BaseID()
			if !visitedEdges[calleeEdgeID] {
				visitedEdges[calleeEdgeID] = true
				processCallGraphEdge(meta, callee, data, visitedNodes, visitedEdges, nodePairEdges, nodeCounter, edgeCounter)
			}
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
			argValue := metadata.CallArgToString(arg)

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

func drawNodeCytoscape(node TrackerNodeInterface, data *CytoscapeData, nodeMap map[string]string, edgeCounter, nodeCounter *int) {
	if node == nil {
		return
	}
	nodeID := fmt.Sprintf("%s%d", nodePrefix, *nodeCounter)
	data.Nodes = append(data.Nodes, CytoscapeNode{
		Data: CytoscapeNodeData{
			ID:    nodeID,
			Label: node.GetKey(),
			Type:  "function",
		},
	})
	for _, child := range node.GetChildren() {
		if child != nil {
			*nodeCounter++
			childID := fmt.Sprintf("%s%d", nodePrefix, *nodeCounter)
			nodeMap[child.GetKey()] = childID
			data.Nodes = append(data.Nodes, CytoscapeNode{
				Data: CytoscapeNodeData{
					ID:    nodeMap[child.GetKey()],
					Label: child.GetKey(),
					Type:  "function",
				},
			})
			edgeID := fmt.Sprintf("%s%d", edgePrefix, *edgeCounter)
			*edgeCounter++
			data.Edges = append(data.Edges, CytoscapeEdge{
				Data: CytoscapeEdgeData{
					ID:     edgeID,
					Source: nodeID,
					Target: childID,
					Type:   "calls",
				},
			})
			drawNodeCytoscape(child, data, nodeMap, edgeCounter, nodeCounter)
		}
	}
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
