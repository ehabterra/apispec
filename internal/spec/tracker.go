package spec

import (
	"fmt"
	"maps"
	"strings"

	"github.com/ehabterra/swagen/internal/metadata"
)

const (
	trackerNodeKeyFormat = "%s@%d@%s"
)

// ArgumentType represents the classification of an argument
type ArgumentType int

const (
	ArgTypeDirectCallee ArgumentType = iota // Direct function call (existing callee)
	ArgTypeFunctionCall                     // Function call as argument
	ArgTypeVariable                         // Variable reference
	ArgTypeLiteral                          // Literal value
	ArgTypeSelector                         // Field/method selector
	ArgTypeComplex                          // Complex expression
	ArgTypeUnary                            // Unary expression (*ptr, &val)
	ArgTypeBinary                           // Binary expression (a + b)
	ArgTypeIndex                            // Index expression (arr[i])
	ArgTypeComposite                        // Composite literal (struct{})
	ArgTypeTypeAssert                       // Type assertion (val.(type))
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

	// Enhanced argument classification
	ArgType    ArgumentType
	IsArgument bool
	ArgIndex   int    // Position in argument list
	ArgContext string // Context where argument is used

	RootAssignmentMap map[string][]metadata.Assignment `yaml:"root_assignments,omitempty"`
}

// TrackerTree represents the call graph as a tree structure.
type TrackerTree struct {
	meta      *metadata.Metadata
	positions map[string]bool
	roots     []*TrackerNode
	callers   map[string][]*metadata.CallGraphEdge
	callees   map[string][]*metadata.CallGraphEdge
	args      map[string][]*metadata.CallGraphEdge

	// Enhanced tracking indices
	argumentNodes map[string][]*TrackerNode // Track argument nodes by ID
	variableNodes map[string]*TrackerNode   // Track variable nodes by name
	functionNodes map[string][]*TrackerNode // Track function nodes by name
}

type assignmentKey struct {
	Name     string
	Pkg      string
	Type     string
	Function string // new field for function name
}

// NewTrackerTree constructs a TrackerTree from metadata and limits.
func NewTrackerTree(meta *metadata.Metadata, limits TrackerLimits) *TrackerTree {
	t := &TrackerTree{
		meta:          meta,
		positions:     make(map[string]bool),
		callers:       make(map[string][]*metadata.CallGraphEdge),
		callees:       make(map[string][]*metadata.CallGraphEdge),
		args:          make(map[string][]*metadata.CallGraphEdge),
		variableNodes: make(map[string]*TrackerNode),
	}

	// Collect all callers and callee in maps
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

	assignmentIndex := make(map[assignmentKey]*TrackerNode)
	visited := make(map[string]*TrackerNode)

	// Search for root functions
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

		callerName := getString(meta, edge.Caller.Name)

		// Only select main function from root function to be the root
		// and construct the tree based on it
		if root && callerName == "main" {
			if node := NewTrackerNode(t, meta, "", callerID, nil, nil, visited, assignmentIndex, limits); node != nil {
				t.roots = append(t.roots, node)
			}
		}
	}
	return t
}

// classifyArgument determines the type of an argument for enhanced processing
func classifyArgument(arg metadata.CallArgument, edge *metadata.CallGraphEdge) ArgumentType {
	switch arg.Kind {
	case "call":
		return ArgTypeFunctionCall
	case "ident":
		if strings.HasPrefix(arg.Type, "func(") {
			return ArgTypeFunctionCall
		}
		return ArgTypeVariable
	case "literal":
		return ArgTypeLiteral
	case "selector":
		return ArgTypeSelector
	case "unary":
		return ArgTypeUnary
	case "binary":
		return ArgTypeBinary
	case "index":
		return ArgTypeIndex
	case "composite_lit":
		return ArgTypeComposite
	case "type_assert":
		return ArgTypeTypeAssert
	default:
		return ArgTypeComplex
	}
}

// processArguments processes arguments with enhanced classification and tracking
func processArguments(tree *TrackerTree, meta *metadata.Metadata, parentNode *TrackerNode, edge *metadata.CallGraphEdge, callArg *metadata.CallArgument, visited map[string]*TrackerNode, assignmentIndex map[assignmentKey]*TrackerNode, limits TrackerLimits) []*TrackerNode {
	var children []*TrackerNode
	argCount := 0

	var args []metadata.CallArgument

	if callArg != nil {
		args = callArg.Args
	} else if edge != nil {
		args = edge.Args
	}

	for i, arg := range args {
		argID := arg.ID()

		if arg.TypeParamMap == nil {
			arg.TypeParamMap = map[string]string{}
		}

		// Propagate type resolving
		if edge != nil && len(edge.TypeParamMap) > 0 {
			maps.Copy(arg.TypeParamMap, edge.TypeParamMap)
		}
		if callArg != nil && len(callArg.TypeParamMap) > 0 {
			maps.Copy(arg.TypeParamMap, callArg.TypeParamMap)
		}

		// if _, ok := visited[argID]; ok {
		// 	return children
		// }
		// visited[argID] = nil

		if argCount >= limits.MaxArgsPerFunction {
			break
		}

		if edge.Caller.ID() == argID || edge.Callee.ID() == argID || arg.Name == "nil" || argID == "" {
			continue
		}

		argType := classifyArgument(arg, edge)

		argNode := &TrackerNode{
			id:            argID,
			CallArgument:  &arg,
			CallGraphEdge: edge, // NEW: Include the edge to preserve type parameters
			ArgType:       argType,
			IsArgument:    true,
			ArgIndex:      i,
			ArgContext:    fmt.Sprintf("%s.%s", getString(meta, edge.Caller.Name), getString(meta, edge.Callee.Name)),
		}

		switch argType {
		case ArgTypeFunctionCall:
			// Process function call arguments recursively
			if argNode := NewTrackerNode(tree, meta, parentNode.id, argID, edge, nil, visited, assignmentIndex, limits); argNode != nil {
				argNode.ArgType = ArgTypeFunctionCall
				argNode.IsArgument = true
				argNode.ArgIndex = i
				argNode.ArgContext = fmt.Sprintf("%s.%s", getString(meta, edge.Caller.Name), getString(meta, edge.Callee.Name))

				// Process nested arguments
				if len(arg.Args) > 0 {
					childArgs := make([]metadata.CallArgument, len(arg.Args))
					copy(childArgs, arg.Args)
					argNode.children = append(argNode.children, processArguments(tree, meta, argNode, edge, &arg, visited, assignmentIndex, limits)...)
				}

				children = append(children, argNode)
				if arg.Fun != nil && arg.Fun.Position != "" {
					tree.positions[arg.Fun.Position] = true
				}
				argCount++
			}

		case ArgTypeVariable:
			// Enhanced variable tracing and assignment linking
			originVar, originPkg, _, _ := metadata.TraceVariableOrigin(
				arg.Name,
				getString(meta, edge.Caller.Name),
				getString(meta, edge.Caller.Pkg),
				meta,
			)

			// Register variable node
			tree.variableNodes[arg.Name] = argNode

			// Link to assignment if exists
			akey := assignmentKey{
				Name:     originVar,
				Pkg:      originPkg,
				Type:     arg.Type,
				Function: getString(meta, edge.Caller.Name),
			}

			if parent, ok := assignmentIndex[akey]; ok {
				parent.children = append(parent.children, argNode)
			} else {
				children = append(children, argNode)
			}

		case ArgTypeLiteral:
			// Store literal for type inference
			children = append(children, argNode)

		case ArgTypeSelector:
			// Process field/method access
			if arg.X != nil {
				// Trace the base object
				baseVar, _, _, _ := metadata.TraceVariableOrigin(
					arg.X.Name,
					getString(meta, edge.Caller.Name),
					getString(meta, edge.Caller.Pkg),
					meta,
				)

				// Link to base variable if exists
				if baseNode, ok := tree.variableNodes[baseVar]; ok {
					baseNode.children = append(baseNode.children, argNode)
				} else {
					children = append(children, argNode)
				}
			} else {
				children = append(children, argNode)
			}

		case ArgTypeUnary:
			// Process unary expressions (*ptr, &val)
			if arg.X != nil {
				// Trace the operand
				if arg.X.Kind == "ident" {
					originVar, originPkg, _, _ := metadata.TraceVariableOrigin(
						arg.X.Name,
						getString(meta, edge.Caller.Name),
						getString(meta, edge.Caller.Pkg),
						meta,
					)

					if parent, ok := assignmentIndex[assignmentKey{
						Name:     originVar,
						Pkg:      originPkg,
						Type:     arg.X.Type,
						Function: getString(meta, edge.Caller.Name),
					}]; ok {
						parent.children = append(parent.children, argNode)
					} else {
						children = append(children, argNode)
					}
				} else {
					children = append(children, argNode)
				}
			} else {
				children = append(children, argNode)
			}

		case ArgTypeBinary:
			// Process binary expressions (a + b)
			children = append(children, argNode)

		case ArgTypeIndex:
			// Process index expressions (arr[i])
			children = append(children, argNode)

		case ArgTypeComposite:
			// Process composite literals (struct{})
			children = append(children, argNode)

		case ArgTypeTypeAssert:
			// Process type assertions (val.(type))
			children = append(children, argNode)

		default:
			// Complex expressions
			children = append(children, argNode)
		}
	}

	return children
}

// NewTrackerNode creates a new TrackerNode for the tree.
func NewTrackerNode(tree *TrackerTree, meta *metadata.Metadata, parentID, id string, parentEdge *metadata.CallGraphEdge, callArg *metadata.CallArgument, visited map[string]*TrackerNode, assignmentIndex map[assignmentKey]*TrackerNode, limits TrackerLimits) *TrackerNode {
	if id == "" {
		return nil
	}

	// Recursion
	if id == parentID {
		return nil
	}

	// Create a unique node identifier that includes the call context and depth
	// var nodeKey string
	// if parentEdge != nil {
	// 	// Include the position and parent to distinguish between different calls to the same function
	// 	nodeKey = fmt.Sprintf(trackerNodeKeyFormat, id, parentEdge.Position, parentID)
	// 	// nodeKey = fmt.Sprintf(trackerNodeKeyFormat, id, parentEdge.Position, parentID)
	// } else {
	// 	// For root nodes or nodes without parent edge, just use the ID
	nodeKey := id
	// }

	// // Check if we've already created a node for this specific call context
	// if nd, exists := visited[nodeKey]; exists {
	// 	// Return the existing node to prevent duplicates within the same context
	// 	return nd
	// }

	// Limit recursion depth to prevent infinite loops
	if len(visited) > limits.MaxNodesPerTree {
		// Return a simple node without children to prevent explosion
		return &TrackerNode{id: id, CallGraphEdge: parentEdge, CallArgument: callArg}
	}

	// Create new node
	node := &TrackerNode{id: id, CallGraphEdge: parentEdge, CallArgument: callArg, RootAssignmentMap: make(map[string][]metadata.Assignment)}
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
	callerID := id
	idIndex := strings.Index(id, "@")

	if idIndex >= 0 {
		callerID = id[:idIndex]
	}

	if edges, exists := tree.callers[callerID]; exists {

		if parentEdge == nil && len(edges) > 0 {
			// Set root assignments
			callerName := getStringFromPool(meta, edges[0].Caller.Name)
			callerPkg := getStringFromPool(meta, edges[0].Caller.Pkg)

			if pkg, ok := meta.Packages[callerPkg]; ok {
				for _, file := range pkg.Files {
					if fn, ok := file.Functions[callerName]; ok {
						maps.Copy(node.RootAssignmentMap, fn.AssignmentMap)
					}
				}
			}
		}

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

			edge := *edges[i]

			edge.TypeParamMap = map[string]string{}
			maps.Copy(edge.TypeParamMap, edges[i].TypeParamMap)

			// Propagate type resolving
			if parentEdge != nil && len(parentEdge.TypeParamMap) > 0 {
				maps.Copy(edge.TypeParamMap, parentEdge.TypeParamMap)
			}
			if callArg != nil && len(callArg.TypeParamMap) > 0 {
				maps.Copy(edge.TypeParamMap, callArg.TypeParamMap)
			}

			// Check if this callee is already present in any arguments
			calleeID := edge.Callee.ID()

			if edge.Callee.ID() == edge.Caller.ID() || getString(meta, edge.Callee.Name) == "nil" || allArgIDs[calleeID] {
				// Skip this child as it's already present in arguments
				continue
			}

			if childNode := NewTrackerNode(tree, meta, id, calleeID, &edge, nil, visited, assignmentIndex, limits); childNode != nil {
				var addedToParent bool

				// Process arguments for this edge using enhanced processing
				argumentChildren := processArguments(tree, meta, childNode, &edge, callArg, visited, assignmentIndex, limits)

				// Register assignments for this node (from AssignmentMap)
				funcName := getString(meta, edge.Caller.Name)
				callerPkg := getString(meta, edge.Caller.Pkg)

				calleeName := getString(meta, edge.Callee.Name)
				calleePkg := getString(meta, edge.Callee.Pkg)

				var assignmentMap map[string][]metadata.Assignment = edge.AssignmentMap
				if len(node.RootAssignmentMap) > 0 {
					assignmentMap = node.RootAssignmentMap
				}

				if edge.CalleeRecvVarName != "" {
					for _, assigns := range assignmentMap {
						for assignIndex := len(assigns) - 1; assignIndex >= 0; assignIndex-- {
							assignment := assigns[assignIndex]
							if assignment.CalleeFunc == calleeName && assignment.CalleePkg == calleePkg {
								akey := assignmentKey{
									Name:     childNode.CalleeRecvVarName,
									Pkg:      callerPkg, // Use caller's package to match TraceVariableOrigin
									Type:     getString(meta, assignment.ConcreteType),
									Function: funcName,
								}
								assignmentIndex[akey] = childNode
								break
							}
						}
					}
				}

				// If this node uses a variable as a receiver, link to its assignment node
				if childNode.CallGraphEdge != nil && childNode.CalleeVarName != "" && edge.Callee.RecvType != -1 {
					funcName := getString(meta, edge.Caller.Name)
					callerPkg := getString(meta, edge.Caller.Pkg)

					calleeRecvType := getString(meta, edge.Callee.RecvType)
					calleePkg := getString(meta, edge.Callee.Pkg)
					if calleeRecvType != "" {
						if strings.HasPrefix(calleeRecvType, "*") {
							calleeRecvType = "*" + calleePkg + "." + calleeRecvType[1:]
						} else {
							calleeRecvType = calleePkg + "." + calleeRecvType
						}
					}

					originVar, originPkg, _, originFunc := metadata.TraceVariableOrigin(
						edge.CalleeVarName,
						funcName,
						callerPkg,
						meta,
					)

					if parent, ok := assignmentIndex[assignmentKey{
						Name:     originVar,
						Pkg:      originPkg,
						Type:     calleeRecvType,
						Function: originFunc,
					}]; ok {
						parent.children = append(parent.children, childNode)
						addedToParent = true
					}

					// akey := assignmentKey{
					// 	Name:     edge.CalleeVarName,
					// 	Pkg:      callerPkg, // Use caller's package to match TraceVariableOrigin
					// 	Type:     calleeRecvType,
					// 	Function: funcName,
					// }
					// if parent, ok := assignmentIndex[akey]; ok {
					// 	parent.children = append(parent.children, childNode)
					// 	addedToParent = true
					// }
				}

				childNode.children = append(childNode.children, argumentChildren...)
				if !addedToParent {
					node.children = append(node.children, childNode)
				}
				childCount++
			}
		}
	}

	return node
}

func appendTypeParamsToID(id string, typeParams map[string]string) string {
	if len(typeParams) > 0 {
		var typeParamStr string
		for key := range typeParams {
			typeParamStr += key + "_"
		}
		id += ":" + typeParamStr[:len(typeParamStr)-1]
	}

	return id
}

// GetRoots returns the root nodes of the tracker tree.
func (t *TrackerTree) GetRoots() []*TrackerNode {
	if t == nil {
		return nil
	}

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

// TraceArgumentOrigin traces an argument back to its original definition
func (t *TrackerTree) TraceArgumentOrigin(argNode *TrackerNode) *TrackerNode {
	if argNode == nil || !argNode.IsArgument {
		return nil
	}

	// For variable arguments, trace back to assignment
	if argNode.ArgType == ArgTypeVariable && argNode.CallArgument != nil {
		originVar, _, _, _ := metadata.TraceVariableOrigin(
			argNode.CallArgument.Name,
			argNode.ArgContext,
			"", // Use empty string for package, will be determined by TraceVariableOrigin
			t.meta,
		)

		// Look for the origin variable in variable nodes
		if originNode, ok := t.variableNodes[originVar]; ok {
			return originNode
		}
	}

	return nil
}

// FindVariableNodes returns all nodes that represent variables
func (t *TrackerTree) FindVariableNodes() []*TrackerNode {
	var result []*TrackerNode
	for _, node := range t.variableNodes {
		result = append(result, node)
	}
	return result
}

// FindVariableByName returns a variable node by name
func (t *TrackerTree) FindVariableByName(varName string) *TrackerNode {
	return t.variableNodes[varName]
}
