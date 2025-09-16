package spec

import (
	"fmt"
	"maps"
	"strings"

	"github.com/ehabterra/apispec/internal/metadata"
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

// TrackerNode represents a node in the call graph tree.
type TrackerNode struct {
	key      string
	Parent   *TrackerNode
	Children []*TrackerNode
	*metadata.CallGraphEdge
	*metadata.CallArgument

	typeParamMap map[string]string

	// Enhanced argument classification
	ArgType    ArgumentType
	IsArgument bool
	ArgIndex   int    // Position in argument list
	ArgContext string // Context where argument is used

	RootAssignmentMap map[string][]metadata.Assignment `yaml:"root_assignments,omitempty"`
}

func (nd *TrackerNode) Key() string {
	switch {
	case nd.key != "":
	case nd.CallArgument != nil:
		nd.key = nd.CallArgument.ID()
	case nd.CallGraphEdge != nil:
		nd.key = nd.CallGraphEdge.Callee.ID()
	}

	nd.key = strings.TrimPrefix(nd.key, "*")

	return nd.key
}

// GetKey returns the unique key of the node
func (nd *TrackerNode) GetKey() string {
	return nd.Key()
}

func (nd *TrackerNode) TypeParams() map[string]string {
	if nd.typeParamMap == nil {
		nd.typeParamMap = map[string]string{}
	}

	// Use a visited map to avoid cycles
	visited := make(map[*TrackerNode]struct{})
	var collect func(n *TrackerNode, out map[string]string)
	collect = func(n *TrackerNode, out map[string]string) {
		if n == nil {
			return
		}
		if _, ok := visited[n]; ok {
			return
		}
		visited[n] = struct{}{}

		// Copy from CallGraphEdge
		if n.CallGraphEdge != nil && len(n.CallGraphEdge.TypeParamMap) > 0 {
			maps.Copy(out, n.CallGraphEdge.TypeParamMap)
		}
		// Copy from CallArgument
		if n.CallArgument != nil {
			maps.Copy(out, n.CallArgument.TypeParams())
		}
		// Copy from parent
		if n.Parent != nil {
			collect(n.Parent, out)
		}
	}

	// Always start with a fresh map to avoid stale/cyclic state
	result := map[string]string{}
	collect(nd, result)
	nd.typeParamMap = result
	return nd.typeParamMap
}

// GetParent returns the parent node
func (nd *TrackerNode) GetParent() TrackerNodeInterface {
	if nd.Parent == nil {
		return nil
	}
	return nd.Parent
}

// GetChildren returns the children nodes
func (nd *TrackerNode) GetChildren() []TrackerNodeInterface {
	children := make([]TrackerNodeInterface, len(nd.Children))
	for i, child := range nd.Children {
		children[i] = child
	}
	return children
}

// GetEdge returns the call graph edge
func (nd *TrackerNode) GetEdge() *metadata.CallGraphEdge {
	return nd.CallGraphEdge
}

// GetArgument returns the call argument
func (nd *TrackerNode) GetArgument() *metadata.CallArgument {
	return nd.CallArgument
}

// GetArgType returns the argument type
func (nd *TrackerNode) GetArgType() metadata.ArgumentType {
	// Convert local ArgumentType to metadata.ArgumentType
	switch nd.ArgType {
	case ArgTypeDirectCallee:
		return metadata.ArgTypeDirectCallee
	case ArgTypeFunctionCall:
		return metadata.ArgTypeFunctionCall
	case ArgTypeVariable:
		return metadata.ArgTypeVariable
	case ArgTypeLiteral:
		return metadata.ArgTypeLiteral
	case ArgTypeSelector:
		return metadata.ArgTypeSelector
	case ArgTypeComplex:
		return metadata.ArgTypeComplex
	case ArgTypeUnary:
		return metadata.ArgTypeUnary
	case ArgTypeBinary:
		return metadata.ArgTypeBinary
	case ArgTypeIndex:
		return metadata.ArgTypeIndex
	case ArgTypeComposite:
		return metadata.ArgTypeComposite
	case ArgTypeTypeAssert:
		return metadata.ArgTypeTypeAssert
	default:
		return metadata.ArgTypeComplex
	}
}

// GetArgIndex returns the argument index
func (nd *TrackerNode) GetArgIndex() int {
	return nd.ArgIndex
}

// GetArgContext returns the argument context
func (nd *TrackerNode) GetArgContext() string {
	return nd.ArgContext
}

// GetTypeParamMap returns the type parameter map
func (nd *TrackerNode) GetTypeParamMap() map[string]string {
	return nd.TypeParams()
}

// GetRootAssignmentMap returns the root assignment map
func (nd *TrackerNode) GetRootAssignmentMap() map[string][]metadata.Assignment {
	return nd.RootAssignmentMap
}

func (nd *TrackerNode) AddChild(child *TrackerNode) {
	nd.Children = append(nd.Children, child)
	if child.Parent != nil && child.Parent.Key() != nd.Key() {
		detachChild(child)
	}
	child.Parent = nd
}

func (nd *TrackerNode) AddChildren(children []*TrackerNode) {
	nd.Children = append(nd.Children, children...)
	for _, child := range children {
		if child.Parent != nil {
			if child.Parent.Key() != nd.Key() {
				detachChild(child)
			}
		}
		child.Parent = nd
	}
}

func detachChild(child *TrackerNode) {
	if child.Parent != nil {
		if len(child.Parent.Children) == 1 {
			child.Parent.Children = child.Parent.Children[:0]
		} else {
			for i, item := range child.Parent.Children {
				if item.Key() == child.Key() {
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
	limits    metadata.TrackerLimits

	// Enhanced tracking indices
	variableNodes map[paramKey]*TrackerNode // Track variable nodes by name

	// Chain relationships for efficient lookup
	chainParentMap map[string]*metadata.CallGraphEdge

	// Interface resolution cache
	interfaceResolutionMap map[interfaceKey]string
}

type paramKey struct {
	Name      string
	Pkg       string
	Container string
}

type assignmentKey struct {
	Name      string
	Pkg       string
	Type      string
	Container string
}

func (k assignmentKey) String() string {
	return k.Pkg + k.Type + k.Name + k.Container
}

type assigmentIndexMap map[assignmentKey]*TrackerNode

// interfaceKey represents a key for interface resolution in struct fields
type interfaceKey struct {
	InterfaceType string // The interface type name
	StructType    string // The struct type containing the embedded interface
	Pkg           string // Package where the struct is defined
}

func (k interfaceKey) String() string {
	return k.Pkg + k.StructType + k.InterfaceType
}

// NewTrackerTree constructs a TrackerTree from metadata and limits.
func NewTrackerTree(meta *metadata.Metadata, limits metadata.TrackerLimits) *TrackerTree {
	t := &TrackerTree{
		meta:          meta,
		positions:     make(map[string]bool),
		variableNodes: make(map[paramKey]*TrackerNode),

		limits:                 limits,
		chainParentMap:         make(map[string]*metadata.CallGraphEdge),
		interfaceResolutionMap: make(map[interfaceKey]string),
	}

	assignmentIndex := assigmentIndexMap{}

	visited := make(map[string]int)

	// Get pre-built relationships from metadata
	assignmentRelationships := meta.GetAssignmentRelationships()

	for _, assignment := range assignmentRelationships {
		recvVarName := getString(meta, assignment.Assignment.VariableName)
		pkgStr := getString(meta, assignment.Assignment.Pkg)
		typeStr := getString(meta, assignment.Assignment.ConcreteType)
		funcStr := getString(meta, assignment.Assignment.Func)

		akey := assignmentKey{
			Name:      recvVarName,
			Pkg:       pkgStr,
			Type:      typeStr,
			Container: funcStr,
		}

		// Handle selector assignments more efficiently
		if assignment.Assignment.Lhs.GetKind() == metadata.KindSelector &&
			assignment.Assignment.Lhs.X != nil &&
			assignment.Assignment.Lhs.X.Type != -1 {
			akey.Container = getString(meta, assignment.Assignment.Lhs.X.Type)
		}

		assignmentIndex[akey] = &TrackerNode{
			key:           assignment.Edge.Callee.ID(),
			CallGraphEdge: assignment.Edge,
		}
	}

	// Search for assignments and variables - optimized batch processing
	for i := range meta.CallGraph {
		edge := &meta.CallGraph[i]

		// Cache string lookups to avoid repeated calls
		calleeName := getString(meta, edge.Callee.Name)
		calleePkg := getString(meta, edge.Callee.Pkg)

		for param, arg := range edge.ParamArgMap {
			// Enhanced variable tracing and assignment linking
			_, _, originArg, _ := metadata.TraceVariableOrigin(
				param,
				calleeName, // Use cached value
				calleePkg,  // Use cached value
				meta,
			)

			if originArg == nil {
				continue
			}

			pkey := paramKey{
				Name:      param,
				Pkg:       calleePkg,  // Use cached value
				Container: calleeName, // Use cached value
			}

			t.variableNodes[pkey] = &TrackerNode{
				key:           originArg.ID(),
				CallGraphEdge: edge,
				CallArgument:  &arg,
			}
		}
	}

	// Pre-process chain relationships for efficient lookup
	chainParentMap := make(map[string]*metadata.CallGraphEdge)
	for i := range meta.CallGraph {
		edge := &meta.CallGraph[i]
		if edge.ChainParent != nil {
			// Use a simple string key for fast lookup
			chainKey := edge.ChainParent.Callee.ID()
			chainParentMap[chainKey] = edge.ChainParent
		}
	}

	// Store chain parent map in tree for efficient access
	t.chainParentMap = chainParentMap

	// Sync interface resolutions from metadata
	t.SyncInterfaceResolutionsFromMetadata()

	// Search for root functions
	roots := meta.CallGraphRoots()
	for i := range roots {
		edge := roots[i]

		callerName := getString(meta, edge.Caller.Name)
		callerID := edge.Caller.ID()
		exists := false

		for _, rt := range t.roots {
			if rt.Key() == stripToBase(callerID) {
				exists = true
			}
		}

		// Only select main function from root function to be the root
		// and construct the tree based on it
		if !exists && callerName == metadata.MainFunc {
			if node := NewTrackerNode(t, meta, "", callerID, nil, nil, visited, &assignmentIndex, t.limits); node != nil {
				node.key = callerID
				t.roots = append(t.roots, node)
			}
		}
	}

	// Assign children to nodes
	traverseTree(t.roots, &assignmentNodes{assignmentIndex: assignmentIndex}, 1, nil)

	// Assign children to nodes by params
	traverseTree(t.roots, &variableNodes{variables: t.variableNodes}, metadata.MaxSelfCallingDepth, nil)

	// Process chain calls efficiently - establish parent-child relationships
	t.processChainRelationships()

	return t
}

// processChainRelationships efficiently establishes parent-child relationships for chain calls
func (t *TrackerTree) processChainRelationships() {
	// Process chain relationships in a single pass through the call graph
	for _, edge := range t.meta.CallGraph {
		if edge.ChainParent != nil {
			// Find the parent node in the tree
			parentKey := edge.ChainParent.Callee.ID()
			parentNode := t.findNodeByEdgeID(parentKey)

			if parentNode != nil {
				// Find the child node in the tree
				childKey := edge.Callee.ID()
				childNode := t.findNodeByEdgeID(childKey)

				if childNode != nil && parentNode != childNode {
					// Establish parent-child relationship
					parentNode.AddChild(childNode)
				}
			}
		}
	}
}

// findNodeByEdgeID finds a node by its edge ID in the existing tree structure
func (t *TrackerTree) findNodeByEdgeID(edgeID string) *TrackerNode {
	// Search in roots
	for _, root := range t.roots {
		if root.CallGraphEdge != nil && root.CallGraphEdge.Callee.ID() == edgeID {
			return root
		}
		// Search in children recursively
		if found := t.findNodeInSubtree(root, edgeID); found != nil {
			return found
		}
	}
	return nil
}

// findNodeInSubtree recursively searches for a node with the given edge ID
func (t *TrackerTree) findNodeInSubtree(node *TrackerNode, edgeID string) *TrackerNode {
	if node.CallGraphEdge != nil && node.CallGraphEdge.Callee.ID() == edgeID {
		return node
	}
	for _, child := range node.Children {
		if found := t.findNodeInSubtree(child, edgeID); found != nil {
			return found
		}
	}
	return nil
}

type assignmentNodes struct {
	assignmentIndex assigmentIndexMap
}

func (a *assignmentNodes) Assign(f func(*TrackerNode) bool) {
	for _, nd := range a.assignmentIndex {
		f(nd)
	}
}

type variableNodes struct {
	variables map[paramKey]*TrackerNode
}

func (v *variableNodes) Assign(f func(*TrackerNode) bool) {
	for _, nd := range v.variables {
		f(nd)
	}
}

func traverseTree(nodes []*TrackerNode, mapObject interface{ Assign(func(*TrackerNode) bool) }, limit int, nodeCount map[string]int) bool {
	if nodeCount == nil {
		nodeCount = map[string]int{}
	}

	if limit == 0 {
		limit = metadata.MaxSelfCallingDepth
	}

	for _, node := range nodes {
		nodeKey := node.Key()
		if nodeKey == "" {
			continue
		}

		if count, ok := nodeCount[nodeKey]; ok {
			if count > limit {
				return false
			}
		}

		mapObject.Assign(func(tn *TrackerNode) bool {
			if nodeKey != "" && nodeKey == tn.Key() {
				nodeTypeParams := node.TypeParams()
				nodeCount[nodeKey]++

				if len(tn.Children) > 0 {
					if len(nodeTypeParams) > 0 {
						// Filter out children that have type parameters that are not in the node type parameters
						children := filterChildren(tn.Children, nodeTypeParams)

						node.AddChildren(children)
					} else {
						node.AddChildren(tn.Children)
					}
					return true
				} else if tn.Parent != nil {
					if len(nodeTypeParams) > 0 {
						// Filter out parent that have type parameters that are not in the node type parameters
						children := filterChildren([]*TrackerNode{node}, nodeTypeParams)

						tn.Parent.AddChildren(children)
					} else {
						tn.Parent.AddChild(node)
					}
					return true
				}
			}
			return false
		})

		if traverseTree(node.Children, mapObject, limit, nodeCount) {
			return true
		}
	}

	return false
}

func filterChildren(children []*TrackerNode, nodeTypeParams map[string]string) []*TrackerNode {
	filteredChildren := []*TrackerNode{}
	hasMatch := true
	for _, child := range children {
		childTypeParams := child.TypeParams()
		for key, value := range nodeTypeParams {
			if childValue, ok := childTypeParams[key]; !ok || value != childValue {
				hasMatch = false
				break
			}
		}
		if hasMatch {
			filteredChildren = append(filteredChildren, child)
		}
	}
	return filteredChildren
}

// classifyArgument determines the type of an argument for enhanced processing
func classifyArgument(arg metadata.CallArgument) ArgumentType {
	switch arg.GetKind() {
	case metadata.KindCall, metadata.KindFuncLit:
		return ArgTypeFunctionCall
	case metadata.KindIdent:
		if strings.HasPrefix(arg.GetType(), "func(") {
			return ArgTypeFunctionCall
		}
		return ArgTypeVariable
	case metadata.KindLiteral:
		return ArgTypeLiteral
	case metadata.KindSelector:
		return ArgTypeSelector
	case metadata.KindUnary:
		return ArgTypeUnary
	case metadata.KindBinary:
		return ArgTypeBinary
	case metadata.KindIndex:
		return ArgTypeIndex
	case metadata.KindCompositeLit:
		return ArgTypeComposite
	case metadata.KindTypeAssert:
		return ArgTypeTypeAssert
	default:
		return ArgTypeComplex
	}
}

// processArguments processes arguments with enhanced classification and tracking
func processArguments(tree *TrackerTree, meta *metadata.Metadata, parentNode *TrackerNode, edge *metadata.CallGraphEdge, visited map[string]int, assignmentIndex *assigmentIndexMap, limits metadata.TrackerLimits) []*TrackerNode {
	if edge == nil {
		return nil
	}
	var children []*TrackerNode
	argCount := 0

	for i, arg := range edge.Args {
		argEdge := arg.Edge

		argID := arg.ID()

		if argCount >= limits.MaxArgsPerFunction {
			break
		}

		if edge.Caller.ID() == stripToBase(argID) || edge.Callee.ID() == argID || arg.GetName() == "nil" || argID == "" {
			continue
		}

		argType := classifyArgument(arg)

		// var argNode *TrackerNode
		argNode := &TrackerNode{
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
			if arg.Fun != nil && arg.Fun.GetKind() == metadata.KindSelector && arg.Fun.X.Type != -1 {
				selectorArg := arg.Fun
				varName := metadata.CallArgToString(*selectorArg.X)

				pkey := paramKey{
					Name:      varName,
					Pkg:       getString(meta, edge.Caller.Pkg),
					Container: getString(meta, edge.Caller.Name),
				}

				if parent, ok := tree.variableNodes[pkey]; ok {
					parent.Children = append(parent.Children, argNode)
				}

				if selectorArg.Sel.GetKind() == metadata.KindIdent && strings.HasPrefix(selectorArg.Sel.GetType(), "func(") || strings.HasPrefix(selectorArg.Sel.GetType(), "func[") {
					// Enhanced variable tracing and assignment linking
					originVar, originPkg, _, _ := metadata.TraceVariableOrigin(
						varName,
						getString(meta, edge.Caller.Name),
						getString(meta, edge.Caller.Pkg),
						meta,
					)

					// Link to assignment if exists
					akey := assignmentKey{
						Name:      originVar,
						Pkg:       originPkg,
						Type:      selectorArg.X.GetType(),
						Container: getString(meta, edge.Caller.Name),
					}

					if parent, ok := (*assignmentIndex)[akey]; ok {
						parent.Children = append(parent.Children, argNode)
					}

					children = append(children, argNode)

					// Get the correct edge for selector arguments
					funcNameIndex := selectorArg.Sel.Name
					recvType := strings.ReplaceAll(originVar, selectorArg.Sel.GetPkg()+".", "")

					var FuncType string

					if selectorArg.X.GetKind() == metadata.KindSelector && selectorArg.X.X.Type != -1 {
						FuncType = selectorArg.X.X.GetType()
						FuncType = strings.ReplaceAll(FuncType, selectorArg.X.X.GetPkg()+".", "")
						FuncType = strings.TrimPrefix(FuncType, "*")
					} else if selectorArg.X.GetKind() == metadata.KindCall && selectorArg.X.Fun.Type != -1 {
						FuncType = selectorArg.X.Fun.GetType()
						FuncType = strings.ReplaceAll(FuncType, selectorArg.X.X.GetPkg()+".", "")
						FuncType = strings.TrimPrefix(FuncType, "*")
					}

					// Resolve interface types to concrete types using interface resolution
					concreteRecvType := tree.ResolveInterfaceFromMetadata(recvType, FuncType, selectorArg.Sel.GetPkg())
					if concreteRecvType != recvType {
						recvType = concreteRecvType
					}

					recvTypeIndex := meta.StringPool.Get(recvType)
					starRecvTypeIndex := meta.StringPool.Get("*" + recvType)
					pkgNameIndex := meta.StringPool.Get(selectorArg.Sel.GetPkg())

					var funcEdge *metadata.CallGraphEdge

					// Look for a call graph edge where this function is the callee
					for _, ArgEdge := range meta.CallGraph {
						if ArgEdge.Caller.Name == funcNameIndex && ArgEdge.Caller.Pkg == pkgNameIndex && (ArgEdge.Caller.RecvType == recvTypeIndex || ArgEdge.Caller.RecvType == starRecvTypeIndex) {
							funcEdge = &ArgEdge
							id := funcEdge.Callee.ID()
							if childNode := NewTrackerNode(tree, meta, argNode.Key(), id, funcEdge, selectorArg, visited, assignmentIndex, limits); childNode != nil {
								argNode.AddChild(childNode)
							}
						}
					}

				}
			}

			// Process function call arguments recursively
			if argNode := NewTrackerNode(tree, meta, parentNode.Key(), argID, argEdge, &arg, visited, assignmentIndex, limits); argNode != nil {
				argNode.Parent = parentNode
				argNode.ArgType = ArgTypeFunctionCall
				argNode.IsArgument = true
				argNode.ArgIndex = i
				argNode.ArgContext = fmt.Sprintf("%s.%s", getString(meta, edge.Caller.Name), getString(meta, edge.Callee.Name))

				// Process nested arguments
				if len(arg.Args) > 0 {
					argNode.AddChildren(processArguments(tree, meta, argNode, argEdge, visited, assignmentIndex, limits))
				}

				children = append(children, argNode)
				if arg.Fun != nil && arg.Fun.Position != -1 {
					tree.positions[arg.Fun.GetPosition()] = true
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

			// Link to assignment if exists
			akey := assignmentKey{
				Name:      originVar,
				Pkg:       originPkg,
				Type:      arg.GetType(),
				Container: getString(meta, edge.Caller.Name),
			}

			if parent, ok := (*assignmentIndex)[akey]; ok {
				parent.Children = append(parent.Children, argNode)
			}

			pkey := paramKey{
				Name:      originVar,
				Pkg:       originPkg,
				Container: getString(meta, edge.Caller.Name),
			}

			if parent, ok := tree.variableNodes[pkey]; ok {
				parent.Children = append(parent.Children, argNode)
			}
			children = append(children, argNode)

		case ArgTypeLiteral:
			// Store literal for type inference
			children = append(children, argNode)

		case ArgTypeSelector:
			// Handling a function inside the selector
			// Process field/method access
			if arg.X != nil {
				if arg.X.GetKind() == metadata.KindSelector {
					if arg.X.Sel.GetKind() == metadata.KindIdent {
						varName := metadata.CallArgToString(*arg.X.X)
						// Enhanced variable tracing and assignment linking
						originVar, originPkg, _, _ := metadata.TraceVariableOrigin(
							varName,
							getString(meta, edge.Caller.Name),
							getString(meta, edge.Caller.Pkg),
							meta,
						)

						_, _ = originVar, originPkg
					}
				}
				if arg.Sel.GetKind() == metadata.KindIdent && strings.HasPrefix(arg.Sel.GetType(), "func(") || strings.HasPrefix(arg.Sel.GetType(), "func[") {
					varName := metadata.CallArgToString(*arg.X)
					// Enhanced variable tracing and assignment linking
					originVar, originPkg, _, _ := metadata.TraceVariableOrigin(
						varName,
						getString(meta, edge.Caller.Name),
						getString(meta, edge.Caller.Pkg),
						meta,
					)

					// Link to assignment if exists
					akey := assignmentKey{
						Name:      originVar,
						Pkg:       originPkg,
						Type:      arg.GetType(),
						Container: getString(meta, edge.Caller.Name),
					}

					if parent, ok := (*assignmentIndex)[akey]; ok {
						parent.Children = append(parent.Children, argNode)
					}

					pkey := paramKey{
						Name:      originVar,
						Pkg:       originPkg,
						Container: getString(meta, edge.Caller.Name),
					}

					if parent, ok := tree.variableNodes[pkey]; ok {
						parent.Children = append(parent.Children, argNode)
					}
					children = append(children, argNode)

					// Get the correct edge for method calls
					funcNameIndex := arg.Sel.Name
					recvType := strings.ReplaceAll(originVar, arg.Sel.GetPkg()+".", "")
					// If the selector is a method, we need to get the type of the receiver
					if arg.Sel.Type != -1 && originVar == varName {
						recvType = arg.X.GetType()
						recvType = strings.ReplaceAll(recvType, arg.Sel.GetPkg()+".", "")
					}

					var FuncType string

					if arg.X.GetKind() == metadata.KindSelector && arg.X.X.Type != -1 {
						FuncType = arg.X.X.GetType()
						FuncType = strings.ReplaceAll(FuncType, arg.X.X.GetPkg()+".", "")
						FuncType = strings.TrimPrefix(FuncType, "*")
					} else if arg.X.GetKind() == metadata.KindCall && arg.X.Fun.Type != -1 {
						FuncType = arg.X.Fun.GetType()
						FuncType = strings.ReplaceAll(FuncType, arg.X.Fun.GetPkg()+".", "")
						FuncType = strings.TrimPrefix(FuncType, "*")
					}

					// Resolve interface types to concrete types using interface resolution
					concreteRecvType := tree.ResolveInterfaceFromMetadata(recvType, FuncType, arg.Sel.GetPkg())
					if concreteRecvType != recvType {
						recvType = concreteRecvType
					}

					recvTypeIndex := meta.StringPool.Get(recvType)
					pkgNameIndex := arg.Sel.Pkg

					var funcEdge *metadata.CallGraphEdge

					// Look for a call graph edge where this function is the caller
					for _, ArgEdge := range meta.CallGraph {
						if ArgEdge.Caller.Name == funcNameIndex && ArgEdge.Caller.Pkg == pkgNameIndex && ArgEdge.Caller.RecvType == recvTypeIndex {
							funcEdge = &ArgEdge
							id := funcEdge.Callee.ID()
							if childNode := NewTrackerNode(tree, meta, argNode.Key(), id, funcEdge, &arg, visited, assignmentIndex, limits); childNode != nil {
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

				// Link to assignment if exists
				akey := assignmentKey{
					Name:      baseVar,
					Pkg:       originPkg,
					Type:      arg.GetType(),
					Container: getString(meta, edge.Caller.Name),
				}

				if parentType != -1 {
					akey.Container = getString(meta, parentType)
				}

				if assignmentNode, ok := (*assignmentIndex)[akey]; ok {
					assignmentNode.Parent = argNode
				}

				children = append(children, argNode)
			} else {
				children = append(children, argNode)
			}

		case ArgTypeUnary:
			// Process unary expressions (*ptr, &val)
			if arg.X != nil {
				// Trace the operand
				if arg.X.GetKind() == metadata.KindIdent {
					originVar, originPkg, _, _ := metadata.TraceVariableOrigin(
						arg.X.GetName(),
						getString(meta, edge.Caller.Name),
						getString(meta, edge.Caller.Pkg),
						meta,
					)

					if parent, ok := (*assignmentIndex)[assignmentKey{
						Name:      originVar,
						Pkg:       originPkg,
						Type:      arg.X.GetType(),
						Container: getString(meta, edge.Caller.Name),
					}]; ok {
						parent.Children = append(parent.Children, argNode)
					}
					children = append(children, argNode)
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
func NewTrackerNode(tree *TrackerTree, meta *metadata.Metadata, parentID, id string, parentEdge *metadata.CallGraphEdge, callArg *metadata.CallArgument, visited map[string]int, assignmentIndex *assigmentIndexMap, limits metadata.TrackerLimits) *TrackerNode {
	if id == "" {
		return nil
	}

	// Recursion
	if id == parentID {
		return nil
	}
	nodeKey := id

	// Limit recursion depth to prevent infinite loops
	if len(visited) > limits.MaxNodesPerTree {
		// Return a simple node without children to prevent explosion
		node := &TrackerNode{
			CallGraphEdge: parentEdge,
			CallArgument:  callArg}
		if parentEdge == nil && callArg == nil {
			node.key = id
		}
		return node
	}

	if visited[nodeKey] > limits.MaxNodesPerTree {
		return nil
	}

	// Create new node
	node := &TrackerNode{
		CallGraphEdge: parentEdge, CallArgument: callArg, RootAssignmentMap: make(map[string][]metadata.Assignment)}
	if parentEdge == nil && callArg == nil {
		node.key = id
	}
	visited[nodeKey]++

	// Process children (callees)
	callerID := stripToBase(id)
	functionID := callerID

	if parentEdge != nil && parentEdge.CalleeVarName != "" {
		// Enhanced variable tracing and assignment linking
		originVar, originPkg, _, _ := metadata.TraceVariableOrigin(
			parentEdge.CalleeVarName,
			getString(meta, parentEdge.Caller.Name),
			getString(meta, parentEdge.Caller.Pkg),
			meta,
		)

		// Get the correct edge for selector arguments
		recvType := strings.ReplaceAll(originVar, originPkg+".", "")

		functionID = originPkg + "." + recvType + "." + getString(meta, parentEdge.Callee.Name)
	}

	// Look for parent function edges in the ParentFunctions map
	if parentEdges, exists := meta.ParentFunctions[functionID]; exists && len(parentEdges) > 0 {
		var visitedParentFunctionID = make(map[string]bool)

		for _, parentFunctionEdge := range parentEdges {
			parentFunctionID := parentFunctionEdge.Caller.ID()
			if visitedParentFunctionID[parentFunctionID] {
				continue
			}
			visitedParentFunctionID[parentFunctionID] = true
			if childNode := NewTrackerNode(tree, meta, id, parentFunctionID, parentFunctionEdge, nil, visited, assignmentIndex, limits); childNode != nil {
				node.AddChild(childNode)
			}
		}
	}

	if edges, exists := meta.Callers[callerID]; exists {
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

		// Limit the number of children to prevent explosion
		childCount := 0

		for i := range edges {
			if childCount >= limits.MaxChildrenPerNode {
				break
			}

			edge := *edges[i]

			calleeID := edge.Callee.ID()

			idTypes := metadata.ExtractGenericTypes(id)
			calleeTypes := metadata.ExtractGenericTypes(calleeID)

			if len(calleeTypes) > 0 && !metadata.IsSubset(idTypes, calleeTypes) {
				// Skip this instance of callee when it's generic but is not including callers types
				continue
			}

			_, existsInArgs := meta.Args[stripToBase(calleeID)]

			if edge.Callee.ID() == edge.Caller.ID() || getString(meta, edge.Callee.Name) == "nil" || existsInArgs {
				// Skip this child as it's already present in arguments
				continue
			}

			if childNode := NewTrackerNode(tree, meta, id, calleeID, &edge, nil, visited, assignmentIndex, limits); childNode != nil {
				var addedToParent bool

				// Process arguments for this edge using enhanced processing
				argumentChildren := processArguments(tree, meta, childNode, &edge, visited, assignmentIndex, limits)

				// If this node uses a variable as a receiver, link to its assignment node
				if childNode.CallGraphEdge != nil && childNode.CalleeVarName != "" && edge.Callee.RecvType != -1 {
					funcName := getString(meta, edge.Caller.Name)
					callerPkg := getString(meta, edge.Caller.Pkg)
					calleePkg := getString(meta, edge.Callee.Pkg)

					// Optimize receiver type resolution
					var calleeRecvType string
					if edge.Callee.RecvType != -1 {
						calleeRecvType = getString(meta, edge.Callee.RecvType)
						if calleeRecvType != "" {
							// Resolve interface types to concrete types using interface resolution
							concreteRecvType := tree.ResolveInterfaceFromMetadata(calleeRecvType, "", calleePkg)
							if concreteRecvType != calleeRecvType {
								calleeRecvType = concreteRecvType
							}

							// Build fully qualified type name efficiently
							if strings.HasPrefix(calleeRecvType, "*") {
								calleeRecvType = "*" + calleePkg + "." + calleeRecvType[1:]
							} else {
								calleeRecvType = calleePkg + "." + calleeRecvType
							}
						}
					}

					// Trace variable origin once and cache results
					originVar, originPkg, _, originFunc := metadata.TraceVariableOrigin(
						edge.CalleeVarName,
						funcName,
						callerPkg,
						meta,
					)

					// Link to assignment node if found
					if originVar != "" && originPkg != "" && originFunc != "" {
						assignmentKey := assignmentKey{
							Name:      originVar,
							Pkg:       originPkg,
							Type:      calleeRecvType,
							Container: originFunc,
						}
						if parent, ok := (*assignmentIndex)[assignmentKey]; ok {
							parent.Children = append(parent.Children, childNode)
						}
					}

					// Link to variable node if found
					pkey := paramKey{
						Name:      edge.CalleeVarName,
						Pkg:       callerPkg, // Use cached value
						Container: funcName,  // Use cached value
					}

					if parent, ok := tree.variableNodes[pkey]; ok {
						parent.Children = append(parent.Children, childNode)
					}
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

func stripToBase(id string) string {
	callerID := id
	idIndex := strings.IndexAny(id, "@[")

	if idIndex >= 0 {
		callerID = id[:idIndex]
	}
	return callerID
}

// GetRoots returns the root nodes of the tracker tree.
func (t *TrackerTree) GetRoots() []TrackerNodeInterface {
	if t == nil {
		return nil
	}

	roots := make([]TrackerNodeInterface, len(t.roots))
	for i, root := range t.roots {
		roots[i] = root
	}
	return roots
}

// FindNodeByKey finds a node by its key in the tracker tree
func (t *TrackerTree) FindNodeByKey(key string) TrackerNodeInterface {
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

	for _, root := range t.roots {
		if found := findNode(root); found != nil {
			return found
		}
	}

	return nil
}

// TraverseTree traverses the tree with a visitor function
func (t *TrackerTree) TraverseTree(visitor func(node TrackerNodeInterface) bool) {
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

	for _, root := range t.roots {
		if !traverse(root) {
			break
		}
	}
}

// GetMetadata returns the underlying metadata
func (t *TrackerTree) GetMetadata() *metadata.Metadata {
	return t.meta
}

// GetLimits returns the tracker limits
func (t *TrackerTree) GetLimits() metadata.TrackerLimits {
	return metadata.TrackerLimits{
		MaxNodesPerTree:    t.limits.MaxNodesPerTree,
		MaxChildrenPerNode: t.limits.MaxChildrenPerNode,
		MaxArgsPerFunction: t.limits.MaxArgsPerFunction,
		MaxNestedArgsDepth: t.limits.MaxNestedArgsDepth,
	}
}

// GetFunctionContext returns the *metadata.Function, package name, and file name for a function name.
func (t *TrackerTree) GetFunctionContext(functionName string) (*metadata.Function, string, string) {
	if functionName == "" {
		return nil, "", ""
	}

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
		originVar, originPkg, _, funName := metadata.TraceVariableOrigin(
			argNode.CallArgument.GetName(),
			argNode.ArgContext,
			"", // Use empty string for package, will be determined by TraceVariableOrigin
			t.meta,
		)

		// Look for the origin variable in variable nodes
		if originNode, ok := t.variableNodes[paramKey{
			Name:      originVar,
			Pkg:       originPkg,
			Container: funName,
		}]; ok {
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

// RegisterInterfaceResolution registers a mapping from an interface type to its concrete implementation
// in a specific struct context. This is used to resolve embedded interfaces in structs.
func (t *TrackerTree) RegisterInterfaceResolution(interfaceType, structType, pkg, concreteType string) {
	key := interfaceKey{
		InterfaceType: interfaceType,
		StructType:    structType,
		Pkg:           pkg,
	}
	t.interfaceResolutionMap[key] = concreteType
}

// ResolveInterface resolves an interface type to its concrete implementation in a struct context.
// Returns the concrete type if found, otherwise returns the original interface type.
func (t *TrackerTree) ResolveInterface(interfaceType, structType, pkg string) string {
	key := interfaceKey{
		InterfaceType: interfaceType,
		StructType:    structType,
		Pkg:           pkg,
	}

	if concreteType, exists := t.interfaceResolutionMap[key]; exists {
		return concreteType
	}

	return interfaceType
}

// GetInterfaceResolutions returns all registered interface resolutions for debugging
func (t *TrackerTree) GetInterfaceResolutions() map[interfaceKey]string {
	result := make(map[interfaceKey]string)
	for k, v := range t.interfaceResolutionMap {
		result[k] = v
	}
	return result
}

// SyncInterfaceResolutionsFromMetadata copies interface resolutions from metadata
func (t *TrackerTree) SyncInterfaceResolutionsFromMetadata() {
	if t.meta == nil {
		return
	}

	metaResolutions := t.meta.GetAllInterfaceResolutions()
	for metaKey, resolution := range metaResolutions {
		trackerKey := interfaceKey{
			InterfaceType: metaKey.InterfaceType,
			StructType:    metaKey.StructType,
			Pkg:           metaKey.Pkg,
		}
		t.interfaceResolutionMap[trackerKey] = resolution.ConcreteType
	}
}

// ResolveInterfaceFromMetadata resolves an interface using metadata and local cache
func (t *TrackerTree) ResolveInterfaceFromMetadata(interfaceType, structType, pkg string) string {
	// First check local cache
	concreteType := t.ResolveInterface(interfaceType, structType, pkg)
	if concreteType != interfaceType {
		return concreteType
	}

	// If not found locally, check metadata
	if t.meta != nil {
		if resolved, found := t.meta.GetInterfaceResolution(interfaceType, structType, pkg); found {
			// Cache it locally for future use
			t.RegisterInterfaceResolution(interfaceType, structType, pkg, resolved)
			return resolved
		}
	}

	return interfaceType
}
