package spec

import (
	"fmt"
	"maps"
	"strings"

	"github.com/ehabterra/swagen/internal/metadata"
)

const MainFunc = "main"

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

func (nd *TrackerNode) TypeParams() map[string]string {
	if nd.typeParamMap == nil {
		nd.typeParamMap = map[string]string{}
	}

	// bubbling type resolving
	if nd.CallGraphEdge != nil && len(nd.CallGraphEdge.TypeParamMap) > 0 {
		maps.Copy(nd.typeParamMap, nd.CallGraphEdge.TypeParamMap)
	}

	if nd.CallArgument != nil {
		maps.Copy(nd.typeParamMap, nd.CallArgument.TypeParams())
	}

	if nd.Parent != nil {
		maps.Copy(nd.typeParamMap, nd.Parent.TypeParams())
	}

	return nd.typeParamMap
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

	// Enhanced tracking indices
	variableNodes map[paramKey]*TrackerNode // Track variable nodes by name
}

type paramKey struct {
	Name      string
	Pkg       string
	Container string // new field for function name
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

type assigmentIndexMap map[assignmentKey]*TrackerNode

// NewTrackerTree constructs a TrackerTree from metadata and limits.
func NewTrackerTree(meta *metadata.Metadata, limits TrackerLimits) *TrackerTree {
	t := &TrackerTree{
		meta:          meta,
		positions:     make(map[string]bool),
		variableNodes: make(map[paramKey]*TrackerNode),
	}

	assignmentIndex := assigmentIndexMap{}

	visited := make(map[string]*TrackerNode)

	// Search for assignments
	for i := range meta.CallGraph {
		edge := &meta.CallGraph[i]

		callerName := getString(meta, edge.Caller.Name)
		callerPkg := getString(meta, edge.Caller.Pkg)

		calleeID := edge.Callee.ID()
		calleeName := getString(meta, edge.Callee.Name)

		var assignmentMap = map[string][]metadata.Assignment{}

		// Get root assignments
		if pkg, ok := meta.Packages[callerPkg]; ok {
			for _, file := range pkg.Files {
				if fn, ok := file.Functions[callerName]; ok && callerName == MainFunc {
					maps.Copy(assignmentMap, fn.AssignmentMap)
				}
			}
		}

		maps.Copy(assignmentMap, edge.AssignmentMap)

		for recvVarName, assigns := range assignmentMap {
			// This in some cases is comparing parent objects with children which is not correct
			// Need to be revised, I comment and check the possible issues
			assignment := assigns[len(assigns)-1]

			assignFunc := getString(meta, assignment.Func)

			akey := assignmentKey{
				Name:      recvVarName,
				Pkg:       getString(meta, assignment.Pkg),
				Type:      getString(meta, assignment.ConcreteType),
				Container: assignFunc,
			}

			if assignment.Lhs.X != nil && assignment.Lhs.X.Type != -1 {
				akey.Container = assignment.Lhs.X.GetType()
			}

			if recvVarName != edge.CalleeRecvVarName && assignment.Value.GetKind() == metadata.KindCall {
				akey.Name = edge.CalleeRecvVarName
			}

			assignmentIndex[akey] = &TrackerNode{
				key:           calleeID,
				CallGraphEdge: edge,
			}
		}

		for param, arg := range edge.ParamArgMap {
			// Enhanced variable tracing and assignment linking
			_, _, originArg, _ := metadata.TraceVariableOrigin(
				param,
				getString(meta, edge.Callee.Name),
				getString(meta, edge.Callee.Pkg),
				meta,
			)

			pkey := paramKey{
				Name:      param,
				Pkg:       getString(meta, edge.Callee.Pkg),
				Container: calleeName,
			}

			if originArg == nil {
				continue
			}

			t.variableNodes[pkey] = &TrackerNode{
				key:           originArg.ID(),
				CallGraphEdge: edge,
				CallArgument:  &arg,
			}
		}

	}

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
		if !exists && callerName == MainFunc {
			if node := NewTrackerNode(t, meta, "", callerID, nil, nil, visited, &assignmentIndex, limits); node != nil {
				node.key = callerID
				t.roots = append(t.roots, node)
			}
		}
	}

	// Assign children to nodes
	traverseTree(t.roots, &assignmentNodes{assignmentIndex: assignmentIndex}, 1, nil)

	// Assign children to nodes by params
	traverseTree(t.roots, &variableNodes{variables: t.variableNodes}, metadata.MaxSelfCallingDepth, nil)

	return t
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
		if count, ok := nodeCount[node.Key()]; ok {
			if count > limit {
				return false
			}
		}

		mapObject.Assign(func(tn *TrackerNode) bool {
			if node.Key() == tn.Key() {
				nodeCount[node.Key()]++

				if len(tn.Children) > 0 {
					node.AddChildren(tn.Children)
					return true
				} else if tn.Parent != nil {
					tn.Parent.AddChild(node)
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

// classifyArgument determines the type of an argument for enhanced processing
func classifyArgument(arg metadata.CallArgument) ArgumentType {
	switch arg.GetKind() {
	case metadata.KindCall:
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
func processArguments(tree *TrackerTree, meta *metadata.Metadata, parentNode *TrackerNode, edge *metadata.CallGraphEdge, visited map[string]*TrackerNode, assignmentIndex *assigmentIndexMap, limits TrackerLimits) []*TrackerNode {
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

					// TODO: Get the correct edge
					funcNameIndex := meta.StringPool.Get(selectorArg.Sel.GetName())
					recvType := strings.ReplaceAll(originVar, selectorArg.Sel.GetPkg()+".", "")
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
					childArgs := make([]metadata.CallArgument, len(arg.Args))
					copy(childArgs, arg.Args)
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

					// TODO: Get the correct edge
					funcNameIndex := meta.StringPool.Get(arg.Sel.GetName())
					recvType := strings.ReplaceAll(originVar, arg.Sel.GetPkg()+".", "")
					recvTypeIndex := meta.StringPool.Get(recvType)
					pkgNameIndex := meta.StringPool.Get(arg.Sel.GetPkg())

					var funcEdge *metadata.CallGraphEdge

					// Look for a call graph edge where this function is the callee
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
func NewTrackerNode(tree *TrackerTree, meta *metadata.Metadata, parentID, id string, parentEdge *metadata.CallGraphEdge, callArg *metadata.CallArgument, visited map[string]*TrackerNode, assignmentIndex *assigmentIndexMap, limits TrackerLimits) *TrackerNode {
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

	// Create new node
	node := &TrackerNode{
		CallGraphEdge: parentEdge, CallArgument: callArg, RootAssignmentMap: make(map[string][]metadata.Assignment)}
	if parentEdge == nil && callArg == nil {
		node.key = id
	}
	visited[nodeKey] = node

	// Process children (callees)
	callerID := stripToBase(id)

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

					if parent, ok := (*assignmentIndex)[assignmentKey{
						Name:      originVar,
						Pkg:       originPkg,
						Type:      calleeRecvType,
						Container: originFunc,
					}]; ok {
						parent.Children = append(parent.Children, childNode)
					}

					pkey := paramKey{
						Name:      edge.CalleeVarName,
						Pkg:       getString(meta, edge.Caller.Pkg),
						Container: getString(meta, edge.Caller.Name),
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
