package spec

// LazyTree — docs/TRACKER_REDESIGN.md step 4 (groundwork).
//
// A second TrackerTreeInterface implementation that computes the call tree
// on demand instead of materializing the DAG's unfolding up front:
//
//   - a node is (edge/argument, parent); children are computed on first
//     access from meta.Callers plus the edge's arguments, and the *edge
//     list* per function key is memoized (the expensive part), while node
//     objects stay per-path so every node has exactly one true parent —
//     per-route isolation needs no shared-node discipline;
//   - cycles are cut by an ancestor-key check on the node's own path (the
//     per-path state the eager tree's global maps approximate);
//   - traversals visit each function key once globally, so they are linear
//     in the graph, not in its exponential unfolding.
//
// What this implementation intentionally does NOT yet reproduce from the
// eager TrackerTree: the mutation overlays — assignment/param cross-links
// (variableNodes / assignmentIndex), chain re-parenting, interface-method
// attachment of concrete implementations, and handler-factory closure
// attachment. Those move to relations consulted at query time (roadmap
// step 5). Until then the eager tree remains the production default;
// LazyTree's parity is tracked by the side-by-side fixture diff harness.

import (
	"sort"
	"strings"

	"github.com/ehabterra/apispec/internal/metadata"
)

// LazyTree implements TrackerTreeInterface over metadata, expanding on demand.
type LazyTree struct {
	meta   *metadata.Metadata
	limits metadata.TrackerLimits
	roots  []TrackerNodeInterface

	// calleeEdges memoizes, per function base key, the filtered+ordered call
	// edges used to expand any node of that function. Computed once.
	calleeEdges map[string][]*metadata.CallGraphEdge

	// Relations the eager tree encodes by mutating node linkage, kept here as
	// plain indexes consulted during expansion (the step-5 direction):
	//
	// chainChildren: chained calls (r.HandleFunc(...).Methods("GET")) grouped
	// under the chain parent's callee ID — what processChainRelationships
	// wires by appending to Children.
	chainChildren map[string][]*metadata.CallGraphEdge
	// receiverChildren: calls made on a variable, grouped under the callee ID
	// of the call that produced the variable (g := app.Group("/x"); g.GET(...)
	// lists the g.GET edge under the Group call) — what the eager build wires
	// through assignmentIndex/variableNodes.
	receiverChildren map[string][]*metadata.CallGraphEdge
	// claimed marks edges owned by a receiverChildren producer. The eager
	// build's AddChildren pass detaches such nodes from their call-site
	// parent, so they appear only under the producer (a group's routes under
	// the Group call, not under main) — mirrored here by excluding them from
	// the plain caller expansion.
	claimed        map[*metadata.CallGraphEdge]bool
	relationsBuilt bool
}

// buildRelations constructs the chain and receiver-variable indexes once.
func (t *LazyTree) buildRelations() {
	if t.relationsBuilt {
		return
	}
	t.relationsBuilt = true
	t.chainChildren = map[string][]*metadata.CallGraphEdge{}
	t.receiverChildren = map[string][]*metadata.CallGraphEdge{}
	t.claimed = map[*metadata.CallGraphEdge]bool{}
	meta := t.meta

	// Edges grouped by the receiver variable they're invoked on:
	// (varName, callerPkg, callerFunc) -> edges.
	type recvKey struct{ name, pkg, fn string }
	edgesByRecvVar := map[recvKey][]*metadata.CallGraphEdge{}
	for i := range meta.CallGraph {
		edge := &meta.CallGraph[i]
		if edge.ChainParent != nil {
			parentKey := strings.TrimPrefix(edge.ChainParent.Callee.ID(), "*")
			t.chainChildren[parentKey] = append(t.chainChildren[parentKey], edge)
		}
		if edge.CalleeVarName != "" {
			k := recvKey{
				name: edge.CalleeVarName,
				pkg:  getString(meta, edge.Caller.Pkg),
				fn:   getString(meta, edge.Caller.Name),
			}
			edgesByRecvVar[k] = append(edgesByRecvVar[k], edge)
		}
	}

	// Assignment links: variable <- producing call. Sort by producing callee
	// ID so the receiverChildren lists are order-independent of the source map.
	rels := make([]*metadata.AssignmentLink, 0)
	for _, rel := range meta.GetAssignmentRelationships() {
		rels = append(rels, rel)
	}
	sort.Slice(rels, func(i, j int) bool {
		return rels[i].Edge.Callee.ID() < rels[j].Edge.Callee.ID()
	})
	producerByVar := map[recvKey]string{}
	for _, rel := range rels {
		k := recvKey{
			name: getString(meta, rel.Assignment.VariableName),
			pkg:  getString(meta, rel.Assignment.Pkg),
			fn:   getString(meta, rel.Assignment.Func),
		}
		producerKey := strings.TrimPrefix(rel.Edge.Callee.ID(), "*")
		producerByVar[k] = producerKey
		edges := edgesByRecvVar[k]
		if len(edges) == 0 {
			continue
		}
		t.receiverChildren[producerKey] = append(t.receiverChildren[producerKey], edges...)
		for _, edge := range edges {
			t.claimed[edge] = true
		}
	}

	// Param bindings: a value passed into a function parameter (UserRoutes(g)
	// with func UserRoutes(rg *gin.RouterGroup)) makes the callee's calls on
	// that parameter belong to the value's producer — so a group's routes
	// registered in a helper still hang (prefixed) under the Group call. This
	// is what the eager build wires through variableNodes/ParamArgMap.
	for i := range meta.CallGraph {
		edge := &meta.CallGraph[i]
		if len(edge.ParamArgMap) == 0 {
			continue
		}
		params := make([]string, 0, len(edge.ParamArgMap))
		for param := range edge.ParamArgMap {
			params = append(params, param)
		}
		sort.Strings(params)
		for _, param := range params {
			arg := edge.ParamArgMap[param]
			if arg.GetKind() != metadata.KindIdent {
				continue
			}
			paramEdges := edgesByRecvVar[recvKey{
				name: param,
				pkg:  getString(meta, edge.Callee.Pkg),
				fn:   getString(meta, edge.Callee.Name),
			}]
			if len(paramEdges) == 0 {
				continue
			}
			originVar, originPkg, _, originFunc := metadata.TraceVariableOrigin(
				arg.GetName(),
				getString(meta, edge.Caller.Name),
				getString(meta, edge.Caller.Pkg),
				meta,
			)
			producerKey, ok := producerByVar[recvKey{name: originVar, pkg: originPkg, fn: originFunc}]
			if !ok {
				continue
			}
			t.receiverChildren[producerKey] = append(t.receiverChildren[producerKey], paramEdges...)
			for _, pe := range paramEdges {
				t.claimed[pe] = true
			}
		}
	}
}

// NewLazyTree builds the root layer (main functions, like the eager tree)
// and nothing else.
func NewLazyTree(meta *metadata.Metadata, limits metadata.TrackerLimits) *LazyTree {
	t := &LazyTree{
		meta:        meta,
		limits:      limits,
		calleeEdges: make(map[string][]*metadata.CallGraphEdge),
	}
	seen := map[string]bool{}
	for _, edge := range meta.CallGraphRoots() {
		callerID := edge.Caller.ID()
		if getString(meta, edge.Caller.Name) != metadata.MainFunc || seen[callerID] {
			continue
		}
		seen[callerID] = true
		root := &LazyNode{tree: t, key: strings.TrimPrefix(callerID, "*")}
		root.rootAssignments = rootAssignmentsFor(meta, edge)
		t.roots = append(t.roots, root)
	}
	return t
}

// rootAssignmentsFor mirrors the eager tree's root behavior: the root node
// carries the main function's assignment map.
func rootAssignmentsFor(meta *metadata.Metadata, edge *metadata.CallGraphEdge) map[string][]metadata.Assignment {
	callerName := getString(meta, edge.Caller.Name)
	callerPkg := getString(meta, edge.Caller.Pkg)
	out := map[string][]metadata.Assignment{}
	if pkg, ok := meta.Packages[callerPkg]; ok {
		fileNames := make([]string, 0, len(pkg.Files))
		for name := range pkg.Files {
			fileNames = append(fileNames, name)
		}
		sort.Strings(fileNames)
		for _, name := range fileNames {
			if fn, ok := pkg.Files[name].Functions[callerName]; ok {
				for k, v := range fn.AssignmentMap {
					out[k] = append(out[k], v...)
				}
			}
		}
	}
	return out
}

// edgesFor returns (and memoizes) the expansion edge list for a function base
// key: callee edges from meta.Callers with the eager tree's skip rules
// (self-calls, "nil", callees already present as arguments).
func (t *LazyTree) edgesFor(baseKey string) []*metadata.CallGraphEdge {
	if edges, ok := t.calleeEdges[baseKey]; ok {
		return edges
	}
	t.buildRelations()
	var out []*metadata.CallGraphEdge
	for _, edge := range t.meta.Callers[baseKey] {
		if t.claimed[edge] {
			continue // owned by its producer (see receiverChildren)
		}
		calleeID := edge.Callee.ID()
		if calleeID == edge.Caller.ID() || getString(t.meta, edge.Callee.Name) == "nil" {
			continue
		}
		if _, inArgs := t.meta.Args[metadata.StripToBase(calleeID)]; inArgs {
			continue
		}
		out = append(out, edge)
	}
	t.calleeEdges[baseKey] = out
	return out
}

// GetRoots implements TrackerTreeInterface.
func (t *LazyTree) GetRoots() []TrackerNodeInterface {
	if t == nil {
		return nil
	}
	return t.roots
}

// GetMetadata implements TrackerTreeInterface.
func (t *LazyTree) GetMetadata() *metadata.Metadata { return t.meta }

// GetLimits implements TrackerTreeInterface.
func (t *LazyTree) GetLimits() metadata.TrackerLimits { return t.limits }

// GetNodeCount implements TrackerTreeInterface: number of distinct function
// keys visited by a full traversal (linear; not the unfolding size).
func (t *LazyTree) GetNodeCount() int {
	count := 0
	t.TraverseTree(func(TrackerNodeInterface) bool { count++; return true })
	return count
}

// TraverseTree implements TrackerTreeInterface. Each node key is visited at
// most once globally, so the walk is linear in the call graph.
func (t *LazyTree) TraverseTree(visitor func(node TrackerNodeInterface) bool) {
	visited := map[string]bool{}
	var walk func(n TrackerNodeInterface) bool
	walk = func(n TrackerNodeInterface) bool {
		key := n.GetKey()
		if key != "" {
			if visited[key] {
				return true
			}
			visited[key] = true
		}
		if !visitor(n) {
			return false
		}
		for _, child := range n.GetChildren() {
			if !walk(child) {
				return false
			}
		}
		return true
	}
	for _, root := range t.roots {
		if !walk(root) {
			return
		}
	}
}

// FindNodeByKey implements TrackerTreeInterface.
func (t *LazyTree) FindNodeByKey(key string) TrackerNodeInterface {
	var found TrackerNodeInterface
	t.TraverseTree(func(n TrackerNodeInterface) bool {
		if n.GetKey() == key {
			found = n
			return false
		}
		return true
	})
	return found
}

// GetFunctionContext implements TrackerTreeInterface (same deterministic
// lookup as the eager tree).
func (t *LazyTree) GetFunctionContext(functionName string) (*metadata.Function, string, string) {
	if functionName == "" {
		return nil, "", ""
	}
	pkgNames := make([]string, 0, len(t.meta.Packages))
	for pkgName := range t.meta.Packages {
		pkgNames = append(pkgNames, pkgName)
	}
	sort.Strings(pkgNames)
	for _, pkgName := range pkgNames {
		pkg := t.meta.Packages[pkgName]
		fileNames := make([]string, 0, len(pkg.Files))
		for fileName := range pkg.Files {
			fileNames = append(fileNames, fileName)
		}
		sort.Strings(fileNames)
		for _, fileName := range fileNames {
			fns := pkg.Files[fileName].Functions
			fnKeys := make([]string, 0, len(fns))
			for key := range fns {
				fnKeys = append(fnKeys, key)
			}
			sort.Strings(fnKeys)
			for _, key := range fnKeys {
				fn := fns[key]
				if t.meta.StringPool.GetString(fn.Name) == functionName {
					return fn, pkgName, fileName
				}
			}
		}
	}
	return nil, "", ""
}

// LazyNode implements TrackerNodeInterface. Identity is (content, parent):
// node objects are per-path, so Parent is always the actual expansion parent.
type LazyNode struct {
	tree   *LazyTree
	key    string
	parent *LazyNode

	edge *metadata.CallGraphEdge
	arg  *metadata.CallArgument

	argType    ArgumentType
	isArgument bool
	argIndex   int
	argContext string

	rootAssignments map[string][]metadata.Assignment

	children []TrackerNodeInterface // nil = not yet expanded
	expanded bool
}

// GetKey implements TrackerNodeInterface.
func (n *LazyNode) GetKey() string { return n.key }

// GetParent implements TrackerNodeInterface.
func (n *LazyNode) GetParent() TrackerNodeInterface {
	if n.parent == nil {
		return nil
	}
	return n.parent
}

// GetEdge implements TrackerNodeInterface.
func (n *LazyNode) GetEdge() *metadata.CallGraphEdge { return n.edge }

// GetArgument implements TrackerNodeInterface.
func (n *LazyNode) GetArgument() *metadata.CallArgument { return n.arg }

// GetArgType implements TrackerNodeInterface.
func (n *LazyNode) GetArgType() metadata.ArgumentType { return toMetadataArgType(n.argType) }

// GetArgIndex implements TrackerNodeInterface.
func (n *LazyNode) GetArgIndex() int { return n.argIndex }

// GetArgContext implements TrackerNodeInterface.
func (n *LazyNode) GetArgContext() string { return n.argContext }

// GetRootAssignmentMap implements TrackerNodeInterface.
func (n *LazyNode) GetRootAssignmentMap() map[string][]metadata.Assignment {
	return n.rootAssignments
}

// GetTypeParamMap implements TrackerNodeInterface: bindings from this node's
// edge/argument merged with its ancestors', nearest binding winning.
func (n *LazyNode) GetTypeParamMap() map[string]string {
	out := map[string]string{}
	for cur := n; cur != nil; cur = cur.parent {
		if cur.edge != nil {
			for k, v := range cur.edge.TypeParamMap {
				if _, ok := out[k]; !ok {
					out[k] = v
				}
			}
		}
		if cur.arg != nil {
			for k, v := range cur.arg.TypeParams() {
				if _, ok := out[k]; !ok {
					out[k] = v
				}
			}
		}
	}
	return out
}

// onPath reports whether key is already an ancestor of n (cycle guard: the
// per-path state a lazy unfolding needs, in contrast to a global seen-set).
func (n *LazyNode) onPath(key string) bool {
	for cur := n; cur != nil; cur = cur.parent {
		if cur.key == key {
			return true
		}
	}
	return false
}

// GetChildren implements TrackerNodeInterface, expanding on first access:
// argument nodes for the node's own edge, then callee nodes from the
// memoized edge list, generics-filtered like the eager tree.
func (n *LazyNode) GetChildren() []TrackerNodeInterface {
	if n.expanded {
		return n.children
	}
	n.expanded = true

	meta := n.tree.meta
	limits := n.tree.limits

	// Argument nodes. For a call node, expand the arguments of the call that
	// produced it (n.edge.Args). For an argument node, expand only through the
	// argument's OWN edge (a function-call argument's nested call) — never the
	// parent edge it carries for context, or a literal argument would re-expand
	// its parent's args and reproduce itself forever.
	ownerEdge := n.edge
	if n.isArgument {
		ownerEdge = nil
		if n.argType == ArgTypeFunctionCall && n.arg != nil && n.arg.Edge != nil {
			ownerEdge = n.arg.Edge
		}
	}
	if ownerEdge != nil {
		for i, arg := range ownerEdge.Args {
			if i >= limits.MaxArgsPerFunction {
				break
			}
			argID := arg.ID()
			if argID == "" || arg.GetName() == "nil" ||
				ownerEdge.Caller.ID() == metadata.StripToBase(argID) || ownerEdge.Callee.ID() == argID {
				continue
			}
			key := strings.TrimPrefix(argID, "*")
			if n.onPath(key) {
				continue
			}
			argType := classifyArgument(arg)
			argEdge := ownerEdge
			if argType == ArgTypeFunctionCall && arg.Edge != nil {
				argEdge = arg.Edge
			}
			child := &LazyNode{
				tree:       n.tree,
				key:        key,
				parent:     n,
				edge:       argEdge,
				arg:        arg,
				argType:    argType,
				isArgument: true,
				argIndex:   i,
				argContext: getString(meta, ownerEdge.Caller.Name) + "." + getString(meta, ownerEdge.Callee.Name),
			}
			n.children = append(n.children, child)
		}
	}

	// Callee nodes: the function's own calls, then calls chained onto this
	// node's result, then calls made on variables this node's result was
	// assigned to — the latter two from the query-time relations.
	n.tree.buildRelations()
	baseKey := metadata.StripToBase(n.key)
	childCount := 0
	added := map[string]bool{}
	appendCallee := func(edge *metadata.CallGraphEdge) {
		if childCount >= limits.MaxChildrenPerNode {
			return
		}
		calleeID := strings.TrimPrefix(edge.Callee.ID(), "*")
		if added[calleeID] {
			return
		}
		// Same generics-instance filter as the eager tree: skip instantiations
		// whose type arguments aren't bound in this path's context.
		calleeTypes := metadata.ExtractGenericTypes(calleeID)
		if len(calleeTypes) > 0 && !metadata.IsSubset(metadata.ExtractGenericTypes(n.key), calleeTypes) {
			return
		}
		if n.onPath(calleeID) {
			return // cycle: this call is already on the current path
		}
		added[calleeID] = true
		n.children = append(n.children, &LazyNode{
			tree:   n.tree,
			key:    calleeID,
			parent: n,
			edge:   edge,
		})
		childCount++
	}
	expandKey := func(key string) {
		edges := n.tree.edgesFor(key)
		for _, edge := range edges {
			appendCallee(edge)
		}
		// No direct calls: follow into func literals defined in the function
		// (a factory's returned closure) via ParentFunctions, mirroring the
		// eager build's closure attachment.
		if len(edges) == 0 {
			for _, edge := range meta.ParentFunctions[key] {
				appendCallee(edge)
			}
		}
	}
	expandKey(baseKey)
	// Method-value handler (g.GET("/", h.GetUsers)): the argument is a
	// selector whose body lives under the method's own base ID
	// (pkg.recvType.name), not under the argument's key — resolve it so the
	// handler body (responses, params) is reachable from the route node.
	for _, methodKey := range n.methodBaseKeys() {
		expandKey(methodKey)
	}
	for _, edge := range n.tree.chainChildren[n.key] {
		appendCallee(edge)
	}
	for _, edge := range n.tree.receiverChildren[n.key] {
		appendCallee(edge)
	}
	return n.children
}

// methodBaseKeys resolves a method-value argument (h.GetUsers) to the base
// ID of the method it references, so expansion can follow into its body.
func (n *LazyNode) methodBaseKeys() []string {
	arg := n.arg
	if !n.isArgument || arg == nil || arg.GetKind() != metadata.KindSelector || arg.Sel == nil {
		return nil
	}
	selName := arg.Sel.GetName()
	pkg := arg.Sel.GetPkg()
	if selName == "" || pkg == "" {
		return nil
	}
	recv := ""
	if arg.ReceiverType != nil {
		recv = arg.ReceiverType.GetName()
	} else if arg.X != nil && arg.X.Type != -1 {
		recv = arg.X.GetType()
	}
	recv = strings.TrimPrefix(recv, "*")
	recv = strings.TrimPrefix(recv, pkg+".")
	recv = strings.TrimPrefix(recv, "*")
	if recv == "" {
		return nil
	}
	keys := []string{pkg + "." + recv + "." + selName}
	// Interface receiver: fan out to every implementer's method, mirroring
	// the eager build's ImplementedBy attachment.
	keys = append(keys, n.tree.implementerKeys(pkg, recv, selName)...)
	return keys
}

// implementerKeys returns "implPkg.ImplType.method" for every recorded
// implementer when (pkg, recv) names an interface type; nil otherwise.
func (t *LazyTree) implementerKeys(pkg, recv, method string) []string {
	p, ok := t.meta.Packages[pkg]
	if !ok {
		return nil
	}
	fileNames := make([]string, 0, len(p.Files))
	for name := range p.Files {
		fileNames = append(fileNames, name)
	}
	sort.Strings(fileNames)
	var out []string
	for _, name := range fileNames {
		typ, ok := p.Files[name].Types[recv]
		if !ok || getString(t.meta, typ.Kind) != "interface" {
			continue
		}
		for _, implIdx := range typ.ImplementedBy {
			impl := getString(t.meta, implIdx) // "import/path.Type"
			if impl != "" {
				out = append(out, impl+"."+method)
			}
		}
	}
	return out
}

// toMetadataArgType converts the spec-local classification to the interface's
// metadata.ArgumentType (same mapping as TrackerNode.GetArgType).
func toMetadataArgType(at ArgumentType) metadata.ArgumentType {
	switch at {
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
