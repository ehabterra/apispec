package spec

import (
	"fmt"
	"maps"
	"strings"
	"sync"

	"github.com/ehabterra/swagen/internal/metadata"
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
	ID       string
	Parent   *TrackerNode
	Children []*TrackerNode
	*metadata.CallGraphEdge
	*metadata.CallArgument

	// Enhanced argument classification
	ArgType    ArgumentType
	IsArgument bool
	ArgIndex   int    // Position in argument list
	ArgContext string // Context where argument is used

	RootAssignmentMap map[string][]metadata.Assignment `yaml:"root_assignments,omitempty"`
}

func (nd *TrackerNode) AddChild(child *TrackerNode) {
	nd.Children = append(nd.Children, child)
	if child.Parent != nil && child.Parent.ID != nd.ID {
		detachChild(child)
		child.Parent = nd
	}
}

func (nd *TrackerNode) AddChildren(children []*TrackerNode) {
	nd.Children = append(nd.Children, children...)
	for _, child := range children {
		if child.Parent != nil && child.Parent.ID != nd.ID {
			detachChild(child)
			child.Parent = nd
		}
	}
}

func detachChild(child *TrackerNode) {
	if child.Parent != nil {
		if len(child.Parent.Children) == 1 {
			child.Parent.Children = child.Parent.Children[:0]
		} else {
			for i, item := range child.Parent.Children {
				if item.ID == child.ID {
					child.Parent.Children[i] = child.Parent.Children[len(child.Parent.Children)-1]
					child.Parent.Children = child.Parent.Children[:len(child.Parent.Children)-1]
					break
				}
			}
		}
	}
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
	variableNodes map[string]*TrackerNode // Track variable nodes by name
}

type assignmentKey struct {
	Name      string
	Pkg       string
	Type      string
	Container string // new field for function name
}

func (k assignmentKey) String() string {
	return k.Pkg + k.Type + k.Name + k.Container
}

type assigmentIndexMap struct {
	lock  sync.RWMutex
	index map[assignmentKey]*TrackerNode
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

	fmt.Printf("Call graphs: %d\n", len(meta.CallGraph))

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

	assignmentIndex := &assigmentIndexMap{
		index: map[assignmentKey]*TrackerNode{},
	}

	visited := make(map[string]*TrackerNode)

	// Search for root functions
	for i := range meta.CallGraph {
		var root = true
		edge := &meta.CallGraph[i]
		callerID := edge.Caller.ID()
		for _, rt := range t.roots {
			if rt.ID == callerID {
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

	fmt.Println("assignments index: ", len(assignmentIndex.index))
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
func processArguments(tree *TrackerTree, meta *metadata.Metadata, parentNode *TrackerNode, edge *metadata.CallGraphEdge, visited map[string]*TrackerNode, assignmentIndex *assigmentIndexMap, allArgIDs map[string]bool, limits TrackerLimits) []*TrackerNode {
	if edge == nil {
		return nil
	}
	var children []*TrackerNode
	argCount := 0

	for i, arg := range edge.Args {
		argEdge := arg.Edge
		if argEdge != nil {
			callerName := meta.StringPool.GetString(argEdge.Caller.Name)
			callerType := meta.StringPool.GetString(argEdge.Caller.RecvType)
			callerPkg := meta.StringPool.GetString(argEdge.Caller.Pkg)
			calleeName := meta.StringPool.GetString(argEdge.Callee.Name)
			calleePkg := meta.StringPool.GetString(argEdge.Callee.Pkg)

			_, _, _, _, _ = callerName, callerType, callerPkg, calleeName, calleePkg
		}
		argID := arg.ID()

		allArgIDs[argID] = true

		if arg.TypeParamMap == nil {
			arg.TypeParamMap = map[string]string{}
		}

		// Propagate type resolving
		if len(edge.TypeParamMap) > 0 {
			maps.Copy(arg.TypeParamMap, edge.TypeParamMap)
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

		// var argNode *TrackerNode
		argNode := &TrackerNode{
			ID:            argID,
			Parent:        parentNode,
			CallArgument:  &arg,
			CallGraphEdge: edge, // Include the edge to preserve type parameters
			ArgType:       argType,
			IsArgument:    true,
			ArgIndex:      i,
			ArgContext:    fmt.Sprintf("%s.%s", getString(meta, edge.Caller.Name), getString(meta, edge.Callee.Name)),
		}

		switch argType {
		case ArgTypeFunctionCall:
			if arg.Fun != nil && arg.Fun.Kind == kindSelector && arg.Fun.X.Type != "" {
				selectorArg := arg.Fun
				if selectorArg.Sel.Kind == kindIdent && strings.HasPrefix(selectorArg.Sel.Type, "func(") || strings.HasPrefix(selectorArg.Sel.Type, "func[") {
					varName := metadata.CallArgToString(*selectorArg.X)
					// Enhanced variable tracing and assignment linking
					originVar, originPkg, _, _ := metadata.TraceVariableOrigin(
						varName,
						getString(meta, edge.Caller.Name),
						getString(meta, edge.Caller.Pkg),
						meta,
					)

					// Register variable node
					tree.variableNodes[varName] = argNode

					// Link to assignment if exists
					akey := assignmentKey{
						Name:      originVar,
						Pkg:       originPkg,
						Type:      selectorArg.X.Type,
						Container: getString(meta, edge.Caller.Name),
					}

					assignmentIndex.lock.RLock()
					if parent, ok := assignmentIndex.index[akey]; ok {
						parent.AddChild(argNode)
					} else {
						children = append(children, argNode)
					}
					assignmentIndex.lock.RUnlock()

					// TODO: Get the correct edge
					funcNameIndex := meta.StringPool.Get(selectorArg.Sel.Name)
					recvType := strings.ReplaceAll(originVar, selectorArg.Sel.Pkg+".", "")
					recvTypeIndex := meta.StringPool.Get(recvType)
					starRecvTypeIndex := meta.StringPool.Get("*" + recvType)
					pkgNameIndex := meta.StringPool.Get(selectorArg.Sel.Pkg)

					var funcEdge *metadata.CallGraphEdge

					// Look for a call graph edge where this function is the callee
					for _, ArgEdge := range meta.CallGraph {
						if ArgEdge.Caller.Name == funcNameIndex && ArgEdge.Caller.Pkg == pkgNameIndex && (ArgEdge.Caller.RecvType == recvTypeIndex || ArgEdge.Caller.RecvType == starRecvTypeIndex) {
							funcEdge = &ArgEdge
							if childNode := NewTrackerNode(tree, meta, argNode.ID, funcEdge.Callee.ID(), funcEdge, selectorArg, visited, assignmentIndex, limits); childNode != nil {
								argNode.AddChild(childNode)
							}
						}
					}

				}
			}

			// Process function call arguments recursively
			if argNode := NewTrackerNode(tree, meta, parentNode.ID, argID, argEdge, nil, visited, assignmentIndex, limits); argNode != nil {
				argNode.Parent = parentNode

				// Register assignments for this node (from AssignmentMap)
				if argEdge != nil {
					// funcName := getString(meta, argEdge.Caller.Name)
					// callerPkg := getString(meta, argEdge.Caller.Pkg)

					calleeName := getString(meta, argEdge.Callee.Name)
					calleePkg := getString(meta, argEdge.Callee.Pkg)

					for recvVarName, assigns := range argEdge.AssignmentMap {
						// for assignIndex := len(assigns) - 1; assignIndex >= 0; assignIndex-- {
						assignment := assigns[len(assigns)-1]
						// if assignment.Value.Pkg == calleePkg {
						akey := assignmentKey{
							Name:      recvVarName,
							Pkg:       calleePkg,
							Type:      getString(meta, assignment.ConcreteType),
							Container: calleeName,
						}

						if assignment.Lhs.X != nil && assignment.Lhs.X.Type != "" {
							akey.Container = assignment.Lhs.X.Type
						}

						assignmentIndex.lock.Lock()
						assignmentIndex.index[akey] = argNode
						assignmentIndex.lock.Unlock()
						break
						// }
						// }
					}
				}

				argNode.ArgType = ArgTypeFunctionCall
				argNode.IsArgument = true
				argNode.ArgIndex = i
				argNode.ArgContext = fmt.Sprintf("%s.%s", getString(meta, edge.Caller.Name), getString(meta, edge.Callee.Name))

				// Process nested arguments
				if len(arg.Args) > 0 {
					childArgs := make([]metadata.CallArgument, len(arg.Args))
					copy(childArgs, arg.Args)
					argNode.AddChildren(processArguments(tree, meta, argNode, argEdge, visited, assignmentIndex, allArgIDs, limits))
				}

				children = append(children, argNode)
				if arg.Fun != nil && arg.Fun.Position != "" {
					tree.positions[arg.Fun.Position] = true
				}
				argCount++
			}

		case ArgTypeVariable:
			varName := metadata.CallArgToString(arg)
			// Enhanced variable tracing and assignment linking
			originVar, originPkg, _, _ := metadata.TraceVariableOrigin(
				varName,
				getString(meta, edge.Caller.Name),
				getString(meta, edge.Caller.Pkg),
				meta,
			)

			// Register variable node
			tree.variableNodes[varName] = argNode

			// Link to assignment if exists
			akey := assignmentKey{
				Name:      originVar,
				Pkg:       originPkg,
				Type:      arg.Type,
				Container: getString(meta, edge.Caller.Name),
			}

			assignmentIndex.lock.RLock()
			if parent, ok := assignmentIndex.index[akey]; ok {
				parent.AddChild(argNode)
			} else {
				children = append(children, argNode)
			}
			assignmentIndex.lock.RUnlock()

		case ArgTypeLiteral:
			// Store literal for type inference
			children = append(children, argNode)

		case ArgTypeSelector:
			// TODO: handling a function inside the selector
			// Process field/method access
			if arg.X != nil {
				if arg.Sel.Kind == kindIdent && strings.HasPrefix(arg.Sel.Type, "func(") || strings.HasPrefix(arg.Sel.Type, "func[") {
					varName := metadata.CallArgToString(*arg.X)
					// Enhanced variable tracing and assignment linking
					originVar, originPkg, _, _ := metadata.TraceVariableOrigin(
						varName,
						getString(meta, edge.Caller.Name),
						getString(meta, edge.Caller.Pkg),
						meta,
					)

					// Register variable node
					tree.variableNodes[varName] = argNode

					// Link to assignment if exists
					akey := assignmentKey{
						Name:      originVar,
						Pkg:       originPkg,
						Type:      arg.Type,
						Container: getString(meta, edge.Caller.Name),
					}

					assignmentIndex.lock.RLock()
					if parent, ok := assignmentIndex.index[akey]; ok {
						parent.AddChild(argNode)
					} else {
						children = append(children, argNode)
					}
					assignmentIndex.lock.RUnlock()

					// TODO: Get the correct edge
					funcNameIndex := meta.StringPool.Get(arg.Sel.Name)
					recvType := strings.ReplaceAll(originVar, arg.Sel.Pkg+".", "")
					recvTypeIndex := meta.StringPool.Get(recvType)
					pkgNameIndex := meta.StringPool.Get(arg.Sel.Pkg)

					var funcEdge *metadata.CallGraphEdge

					// Look for a call graph edge where this function is the callee
					for _, ArgEdge := range meta.CallGraph {
						if ArgEdge.Caller.Name == funcNameIndex && ArgEdge.Caller.Pkg == pkgNameIndex && ArgEdge.Caller.RecvType == recvTypeIndex {
							funcEdge = &ArgEdge
							if childNode := NewTrackerNode(tree, meta, argNode.ID, funcEdge.Callee.ID(), funcEdge, &arg, visited, assignmentIndex, limits); childNode != nil {
								argNode.AddChild(childNode)
							}
						}
					}

				}
				varName := metadata.CallArgToString(arg)
				// Trace the base object
				baseVar, originPkg, _, _ := metadata.TraceVariableOrigin(
					varName,
					getString(meta, edge.Caller.Name),
					getString(meta, edge.Caller.Pkg),
					meta,
				)
				var parentType = arg.X.Type

				// Link to base variable if exists
				// if baseNode, ok := tree.variableNodes[baseVar]; ok {
				// 	baseNode.children = append(baseNode.children, argNode)
				// } else {
				// 	children = append(children, argNode)
				// }

				// Link to assignment if exists
				akey := assignmentKey{
					Name:      baseVar,
					Pkg:       originPkg,
					Type:      arg.Type,
					Container: getString(meta, edge.Caller.Name),
				}

				if parentType != "" {
					akey.Container = parentType
				}

				assignmentIndex.lock.RLock()
				if assignmentNode, ok := assignmentIndex.index[akey]; ok {
					argNode.AddChild(assignmentNode)
					allArgIDs[assignmentNode.ID] = true
				}
				assignmentIndex.lock.RUnlock()

				children = append(children, argNode)
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

					assignmentIndex.lock.RLock()
					if parent, ok := assignmentIndex.index[assignmentKey{
						Name:      originVar,
						Pkg:       originPkg,
						Type:      arg.X.Type,
						Container: getString(meta, edge.Caller.Name),
					}]; ok {
						parent.AddChild(argNode)
					} else {
						children = append(children, argNode)
					}
					assignmentIndex.lock.RUnlock()
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
func NewTrackerNode(tree *TrackerTree, meta *metadata.Metadata, parentID, id string, parentEdge *metadata.CallGraphEdge, callArg *metadata.CallArgument, visited map[string]*TrackerNode, assignmentIndex *assigmentIndexMap, limits TrackerLimits) *TrackerNode {
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
		return &TrackerNode{ID: id, CallGraphEdge: parentEdge, CallArgument: callArg}
	}

	// Create new node
	node := &TrackerNode{ID: id, CallGraphEdge: parentEdge, CallArgument: callArg, RootAssignmentMap: make(map[string][]metadata.Assignment)}
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
	callerID := stripID(id)

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
			calleeID := stripID(edge.Callee.ID())

			if edge.Callee.ID() == edge.Caller.ID() || getString(meta, edge.Callee.Name) == "nil" || allArgIDs[calleeID] {
				// Skip this child as it's already present in arguments
				continue
			}

			if childNode := NewTrackerNode(tree, meta, id, edge.Callee.ID(), &edge, nil, visited, assignmentIndex, limits); childNode != nil {
				var addedToParent bool

				// Process arguments for this edge using enhanced processing
				argumentChildren := processArguments(tree, meta, childNode, &edge, visited, assignmentIndex, allArgIDs, limits)

				// Register assignments for this node (from AssignmentMap)
				funcName := getString(meta, edge.Caller.Name)
				callerPkg := getString(meta, edge.Caller.Pkg)

				calleeName := getString(meta, edge.Callee.Name)
				calleePkg := getString(meta, edge.Callee.Pkg)

				var assignmentMap = map[string][]metadata.Assignment{}

				if len(node.RootAssignmentMap) > 0 {
					assignmentMap = node.RootAssignmentMap
				} else {
					assignmentMap = edge.AssignmentMap
				}
				// maps.Copy(assignmentMap, edge.AssignmentMap)

				for recvVarName, assigns := range assignmentMap {
					for assignIndex := len(assigns) - 1; assignIndex >= 0; assignIndex-- {
						assignment := assigns[assignIndex]
						if assignment.CalleeFunc == calleeName && assignment.CalleePkg == calleePkg {
							akey := assignmentKey{
								Name:      recvVarName,
								Pkg:       callerPkg, // Use caller's package to match TraceVariableOrigin
								Type:      getString(meta, assignment.ConcreteType),
								Container: funcName,
							}
							assignmentIndex.lock.Lock()
							assignmentIndex.index[akey] = childNode
							assignmentIndex.lock.Unlock()
							break
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

					assignmentIndex.lock.RLock()
					if parent, ok := assignmentIndex.index[assignmentKey{
						Name:      originVar,
						Pkg:       originPkg,
						Type:      calleeRecvType,
						Container: originFunc,
					}]; ok {
						parent.AddChild(childNode)
						addedToParent = true
					}
					assignmentIndex.lock.RUnlock()

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

				childNode.AddChildren(argumentChildren)
				if !addedToParent {
					node.AddChild(childNode)
				}
				childCount++
			}
		}
	}

	return node
}

func stripID(id string) string {
	callerID := id
	idIndex := strings.Index(id, "@")

	if idIndex >= 0 {
		callerID = id[:idIndex]
	}
	return callerID
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
		for _, child := range node.Children {
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
