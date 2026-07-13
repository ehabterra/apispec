// Copyright 2025 Ehab Terra
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package spec

import (
	"fmt"
	"maps"
	"os"
	"sort"
	"strings"
	"sync"

	"github.com/ehabterra/apispec/internal/metadata"
)

// Object pools for frequently created objects
var (
	trackerNodePool = sync.Pool{
		New: func() interface{} {
			return &TrackerNode{}
		},
	}
)

// getTrackerNode returns a TrackerNode from the pool
func getTrackerNode() *TrackerNode {
	node := trackerNodePool.Get().(*TrackerNode)
	// Reset the node to clean state
	*node = TrackerNode{}
	return node
}

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

// String returns the string representation of ArgumentType
func (at ArgumentType) String() string {
	switch at {
	case ArgTypeDirectCallee:
		return "DirectCallee"
	case ArgTypeFunctionCall:
		return "FunctionCall"
	case ArgTypeVariable:
		return "Variable"
	case ArgTypeLiteral:
		return "Literal"
	case ArgTypeSelector:
		return "Selector"
	case ArgTypeComplex:
		return "Complex"
	case ArgTypeUnary:
		return "Unary"
	case ArgTypeBinary:
		return "Binary"
	case ArgTypeIndex:
		return "Index"
	case ArgTypeComposite:
		return "Composite"
	case ArgTypeTypeAssert:
		return "TypeAssert"
	default:
		return "Unknown"
	}
}

// TrackerNode represents a node in the call graph tree.
type TrackerNode struct {
	key      string
	keyReady bool // key has been computed + normalized (deref-trimmed)
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
	// Compute + normalize once. Key() is called per-node inside the hot
	// traverseTree passes, so doing the TrimPrefix on every call (even when the
	// key is already cached) showed up in profiles — cache the final value.
	if nd.keyReady {
		return nd.key
	}
	if nd.key == "" {
		switch {
		case nd.CallArgument != nil:
			nd.key = nd.ID()
		case nd.CallGraphEdge != nil:
			nd.key = nd.Callee.ID()
		}
	}
	nd.key = strings.TrimPrefix(nd.key, "*")
	nd.keyReady = true
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

// GetTypeParamMap returns the type parameter map
func (nd *TrackerNode) GetTypeParamMap() map[string]string {
	return nd.TypeParams()
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
			// Should retain the order of the children
			newChildren := make([]*TrackerNode, 0, len(child.Parent.Children)-1)
			for _, item := range child.Parent.Children {
				if item.Key() != child.Key() {
					newChildren = append(newChildren, item)
				}
			}
			child.Parent.Children = newChildren
		}
	}
}

// TrackerTree represents the call graph as a tree structure.
type TrackerTree struct {
	meta      *metadata.Metadata
	positions map[string]bool
	roots     []*TrackerNode
	limits    metadata.TrackerLimits

	// logger receives traversal-time warnings (limit truncations, etc.).
	// May be nil; callers should reach it via t.warn / t.info.
	logger metadata.VerboseLogger

	// Enhanced tracking indices
	variableNodes map[paramKey][]*TrackerNode // Track variable nodes by name

	// Chain relationships for efficient lookup
	chainParentMap map[string]*metadata.CallGraphEdge

	// Interface resolution cache
	interfaceResolutionMap map[interfaceKey]string

	// Performance optimizations
	nodeMap map[string]*TrackerNode // O(1) node lookup by edge ID
	idCache map[string]string       // Cache for ID generation

	// warnedKeys dedupes per-node limit warnings (recursion-depth,
	// max-nodes, max-children) so a single hot node doesn't spam stderr
	// once per visiting call path.
	warnedKeys map[string]struct{}

	// parentFnIndex maps a function key ("pkg\x00name\x00recvBare") to the
	// indices of call-graph edges whose ParentFunction is that function — i.e.
	// calls made inside a func literal defined in it. Built once, lazily, the
	// first time the handler-factory resolver needs it, so projects without
	// that pattern pay nothing. nil until built.
	parentFnIndex map[string][]int

	// closureAttached dedupes handler-factory closure-body attachment by
	// concrete-method key, so a method's returned closure is expanded at most
	// once and re-entrant traversal can't fan it out repeatedly.
	closureAttached map[string]bool

	// traceCache memoizes metadata.TraceVariableOrigin per (variable, caller
	// function, caller package). The build calls it for every edge parameter
	// and argument, and the same triple recurs constantly — it dominated the
	// CPU profile (≈36%% of a full run on a large project) before memoization.
	// Sound because metadata is immutable during the build.
	traceCache map[string]traceResult

	// nodesBuilt is the cumulative count of real tracker nodes created during
	// this tree's construction. Unlike the per-path `visited` stack counter, it
	// only ever increases, so it is the true measure of total traversal work.
	// It backs the MaxNodesPerTree safety brake: a densely-connected (or cyclic)
	// call graph re-expands shared callees along every distinct path, which is
	// exponential in the worst case, and stack depth alone never reflects that.
	// Capping the cumulative total bounds wall-clock time on such graphs.
	nodesBuilt int
}

// traceResult is a memoized TraceVariableOrigin outcome.
type traceResult struct {
	originVar  string
	originPkg  string
	originType *metadata.CallArgument
	originFunc string
}

// traceOrigin is a memoized metadata.TraceVariableOrigin (see traceCache).
func (t *TrackerTree) traceOrigin(varName, callerName, callerPkg string) (string, string, *metadata.CallArgument, string) {
	key := varName + "\x00" + callerName + "\x00" + callerPkg
	if r, ok := t.traceCache[key]; ok {
		return r.originVar, r.originPkg, r.originType, r.originFunc
	}
	v, p, a, fn := metadata.TraceVariableOrigin(varName, callerName, callerPkg, t.meta)
	if t.traceCache == nil {
		t.traceCache = map[string]traceResult{}
	}
	t.traceCache[key] = traceResult{originVar: v, originPkg: p, originType: a, originFunc: fn}
	return v, p, a, fn
}

// warn forwards to the configured logger, defaulting to stderr when none
// was supplied so existing CLI behaviour is preserved verbatim.
func (t *TrackerTree) warn(format string, args ...any) {
	if t == nil || t.logger == nil {
		fmt.Fprintf(os.Stderr, format, args...)
		return
	}
	t.logger.Warnf(format, args...)
}

// warnOnce emits a warning only the first time a given key is seen. Use it
// for traversal limits that can be hit repeatedly for the same node from
// many call paths — repeating the message gives no extra information and
// drowns out everything else on stderr.
//
// When the receiver is nil (which a handful of code paths and tests rely on),
// we can't dedupe — we just forward to warn so the message still surfaces.
func (t *TrackerTree) warnOnce(key, format string, args ...any) {
	if t == nil {
		t.warn(format, args...)
		return
	}
	if t.warnedKeys == nil {
		t.warnedKeys = make(map[string]struct{})
	}
	if _, ok := t.warnedKeys[key]; ok {
		return
	}
	t.warnedKeys[key] = struct{}{}
	t.warn(format, args...)
}

// infoOnce is warnOnce's verbose-gated sibling. Same dedupe semantics, but
// silenced unless --verbose is on.
func (t *TrackerTree) infoOnce(key, format string, args ...any) {
	if t == nil || t.logger == nil {
		return
	}
	if t.warnedKeys == nil {
		t.warnedKeys = make(map[string]struct{})
	}
	if _, ok := t.warnedKeys[key]; ok {
		return
	}
	t.warnedKeys[key] = struct{}{}
	t.logger.Printf(format, args...)
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
// logger may be nil — warnings then route directly to stderr so the CLI
// behaviour callers had before this parameter existed is preserved.
func NewTrackerTree(meta *metadata.Metadata, limits metadata.TrackerLimits, logger metadata.VerboseLogger) *TrackerTree {
	t := &TrackerTree{
		meta:          meta,
		positions:     make(map[string]bool, 100), // Pre-allocate with estimated capacity
		variableNodes: make(map[paramKey][]*TrackerNode, 50),
		logger:        logger,

		limits:                 limits,
		chainParentMap:         make(map[string]*metadata.CallGraphEdge, 100),
		interfaceResolutionMap: make(map[interfaceKey]string, 50),

		// Initialize performance optimization caches with pre-allocated capacity
		nodeMap: make(map[string]*TrackerNode, 200),
		idCache: make(map[string]string, 100),
	}

	// Pre-allocate roots slice with estimated capacity
	estimatedRoots := max(
		// Rough estimate
		len(meta.CallGraph)*2, 10)
	t.roots = make([]*TrackerNode, 0, estimatedRoots)

	assignmentIndex := assigmentIndexMap{}

	visited := make(map[string]int, 200) // Pre-allocate with estimated capacity

	// Get pre-built relationships from metadata
	assignmentRelationships := meta.GetAssignmentRelationships()

	// Iterate in a stable order: this is a map, and the assignment below is
	// last-write-wins (assignmentIndex[akey] = …). When two relationships map to
	// the same akey, random map order would pick a different winner each run,
	// flipping variable resolution (and the final spec). Order by the edge's
	// instance ID (unique, position-based).
	rels := make([]*metadata.AssignmentLink, 0, len(assignmentRelationships))
	for _, a := range assignmentRelationships {
		rels = append(rels, a)
	}
	sort.Slice(rels, func(i, j int) bool {
		return rels[i].Edge.Callee.ID() < rels[j].Edge.Callee.ID()
	})

	for _, assignment := range rels {
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
		callerName := getString(meta, edge.Caller.Name)
		callerPkg := getString(meta, edge.Caller.Pkg)

		// Iterate params in a stable order: ParamArgMap is a map, and the nodes
		// appended to t.variableNodes below are consumed order-sensitively, so
		// random map order would make resolution (and the final spec) flip
		// between runs.
		paramNames := make([]string, 0, len(edge.ParamArgMap))
		for param := range edge.ParamArgMap {
			paramNames = append(paramNames, param)
		}
		sort.Strings(paramNames)
		for _, param := range paramNames {
			arg := edge.ParamArgMap[param]
			// Trace the actual argument from the caller's context, not the parameter name
			// This ensures each different argument (e.g., productMod, inventoryMod) traces to its own origin
			var argVarName string
			if arg.GetKind() == metadata.KindIdent {
				argVarName = arg.GetName()
			} else {
				// For non-ident arguments, fall back to parameter name
				argVarName = param
			}

			// Enhanced variable tracing and assignment linking
			// Use caller context to trace where the argument came from
			_, _, originArg, _ := t.traceOrigin(
				argVarName,
				callerName, // Trace from caller's context
				callerPkg,  // Trace from caller's context
			)

			if originArg == nil {
				continue
			}

			pkey := paramKey{
				Name:      param,
				Pkg:       calleePkg,  // Use cached value
				Container: calleeName, // Use cached value
			}

			t.variableNodes[pkey] = append(t.variableNodes[pkey], &TrackerNode{
				key:           originArg.ID(),
				CallGraphEdge: edge,
				CallArgument:  &arg,
			})
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
			if rt.Key() == metadata.StripToBase(callerID) {
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

// processChainRelationships establishes parent-child relationships for
// chained calls (e.g. `json.NewEncoder(w).Encode(v)` should reflect that
// Encode follows NewEncoder).
//
// Subtle: the tracker tree shares nodes by edge ID across all roots, so
// the same Encode tracker node is referenced from every handler that
// reaches `NewEncoder(w).Encode(v)`. Wiring the chain via
// AddChild(child) would detach the child from its existing call-site
// parent (the surrounding writeJSON-style helper) and re-parent it
// under the factory call (NewEncoder). That breaks per-route parameter
// tracing in `traceArgViaParent` — every consumer past the first chain
// resolve falls back to a bare `type: object`.
//
// Instead we append the child to the chain-parent's Children list
// without touching `childNode.Parent`, so the call-site parent stays
// the canonical ancestor for tracing while chain ordering is still
// represented in the tree.
func (t *TrackerTree) processChainRelationships() {
	for _, edge := range t.meta.CallGraph {
		if edge.ChainParent == nil {
			continue
		}
		parentKey := edge.ChainParent.Callee.ID()
		parentNode := t.findNodeByEdgeID(parentKey)
		if parentNode == nil {
			continue
		}
		childKey := edge.Callee.ID()
		childNode := t.findNodeByEdgeID(childKey)
		if childNode == nil || parentNode == childNode {
			continue
		}

		// Skip if the chain parent is already a child (idempotent under
		// repeated calls).
		alreadyChild := false
		for _, c := range parentNode.Children {
			if c == childNode {
				alreadyChild = true
				break
			}
		}
		if !alreadyChild {
			parentNode.Children = append(parentNode.Children, childNode)
		}
	}
}

// findNodeByEdgeID finds a node by its edge ID in the existing tree structure
func (t *TrackerTree) findNodeByEdgeID(edgeID string) *TrackerNode {
	// First try the hash map for O(1) lookup
	if node, exists := t.nodeMap[edgeID]; exists {
		return node
	}

	// Fallback to recursive search (should rarely happen if tree is built correctly)
	for _, root := range t.roots {
		if root.CallGraphEdge != nil && root.Callee.ID() == edgeID {
			// Cache the result for future lookups
			t.nodeMap[edgeID] = root
			return root
		}
		// Search in children recursively
		if found := t.findNodeInSubtree(root, edgeID); found != nil {
			// Cache the result for future lookups
			t.nodeMap[edgeID] = found
			return found
		}
	}
	return nil
}

// findNodeInSubtree recursively searches for a node with the given edge ID
func (t *TrackerTree) findNodeInSubtree(node *TrackerNode, edgeID string) *TrackerNode {
	// Perform search with cycle detection
	return t.findNodeInSubtreeWithVisited(node, edgeID, make(map[*TrackerNode]bool))
}

// findNodeInSubtreeWithVisited recursively searches for a node with cycle detection
func (t *TrackerTree) findNodeInSubtreeWithVisited(node *TrackerNode, edgeID string, visited map[*TrackerNode]bool) *TrackerNode {
	if node == nil {
		return nil
	}

	// Prevent infinite recursion
	if visited[node] {
		return nil
	}
	visited[node] = true

	// Early termination: limit search depth to prevent excessive recursion
	if len(visited) > 50 { // Limit search depth
		return nil
	}

	if node.CallGraphEdge != nil && node.Callee.ID() == edgeID {
		// Cache the result for future lookups
		t.nodeMap[edgeID] = node
		return node
	}

	// Limit children search to prevent excessive traversal
	maxChildrenToSearch := 20
	for i, child := range node.Children {
		if i >= maxChildrenToSearch {
			break
		}
		if found := t.findNodeInSubtreeWithVisited(child, edgeID, visited); found != nil {
			// Cache the result for future lookups
			t.nodeMap[edgeID] = found
			return found
		}
	}
	return nil
}

type assignmentNodes struct {
	assignmentIndex assigmentIndexMap
	sorted          []*TrackerNode // cached stable order, built once on first Assign
}

func (a *assignmentNodes) Assign(f func(*TrackerNode)) {
	// Stable order: this map is walked against the tree to attach assignment
	// nodes, so a random order would attach ambiguous matches differently each
	// run (flipping which route claims a shared node, and the final spec).
	//
	// traverseTree calls Assign once per visited node, but the map is immutable
	// for the whole traversal — so sort once and reuse the slice instead of
	// re-sorting (and re-allocating) on every node, which is O(nodes·N·logN).
	if a.sorted == nil {
		a.sorted = make([]*TrackerNode, 0, len(a.assignmentIndex))
		for _, nd := range a.assignmentIndex {
			a.sorted = append(a.sorted, nd)
		}
		sort.Slice(a.sorted, func(i, j int) bool { return a.sorted[i].Key() < a.sorted[j].Key() })
	}
	for _, nd := range a.sorted {
		f(nd)
	}
}

type variableNodes struct {
	variables map[paramKey][]*TrackerNode
	sorted    []*TrackerNode // cached first-of-each-key in stable order
}

func (v *variableNodes) Assign(f func(*TrackerNode)) {
	// Stable order (see assignmentNodes.Assign): build the sorted first-nodes
	// once and reuse across every traverseTree visit (the map doesn't change).
	if v.sorted == nil {
		keys := make([]paramKey, 0, len(v.variables))
		for k := range v.variables {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool {
			a, b := keys[i], keys[j]
			if a.Name != b.Name {
				return a.Name < b.Name
			}
			if a.Pkg != b.Pkg {
				return a.Pkg < b.Pkg
			}
			return a.Container < b.Container
		})
		v.sorted = make([]*TrackerNode, 0, len(keys))
		for _, k := range keys {
			if nds := v.variables[k]; len(nds) > 0 {
				v.sorted = append(v.sorted, nds[0])
			}
		}
	}
	for _, nd := range v.sorted {
		f(nd)
	}
}

func traverseTree(nodes []*TrackerNode, mapObject interface{ Assign(func(*TrackerNode)) }, limit int, nodeCount map[string]int) bool {
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

		mapObject.Assign(func(tn *TrackerNode) {
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
				} else if tn.Parent != nil {
					if len(nodeTypeParams) > 0 {
						// Filter out parent that have type parameters that are not in the node type parameters
						children := filterChildren([]*TrackerNode{node}, nodeTypeParams)

						tn.Parent.AddChildren(children)
					} else {
						tn.Parent.AddChild(node)
					}
				}
			}
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
func classifyArgument(arg *metadata.CallArgument) ArgumentType {
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

	// Pre-allocate slice with known capacity to reduce allocations
	expectedArgs := min(len(edge.Args), limits.MaxArgsPerFunction)
	children := make([]*TrackerNode, 0, expectedArgs)
	argCount := 0

	for i, arg := range edge.Args {
		argEdge := arg.Edge

		argID := arg.ID()
		argCount++

		if argCount >= limits.MaxArgsPerFunction {
			tree.warn("Warning: MaxArgsPerFunction limit (%d) reached for function %s.%s, truncating arguments\n",
				limits.MaxArgsPerFunction, getString(meta, edge.Caller.Name), getString(meta, edge.Callee.Name))
			break
		}

		if edge.Caller.ID() == metadata.StripToBase(argID) || edge.Callee.ID() == argID || arg.GetName() == "nil" || argID == "" {
			continue
		}

		argType := classifyArgument(arg)

		// Choose the most relevant edge for the argument node.
		// For function-call arguments, prefer the argument's own edge so matchers (e.g. requestBody detection)
		// see the actual callee rather than the parent route edge.
		argNodeEdge := edge
		if argType == ArgTypeFunctionCall && argEdge != nil {
			argNodeEdge = argEdge
		}

		argNode := newArgumentNode(tree, parentNode, argID, argNodeEdge, arg)

		if argNode == nil {
			continue
		}

		// Set argument-specific fields after node creation
		argNode.ArgType = argType
		argNode.IsArgument = true
		argNode.ArgIndex = i
		argNode.ArgContext = fmt.Sprintf("%s.%s", getString(meta, edge.Caller.Name), getString(meta, edge.Callee.Name))

		switch argType {
		case ArgTypeFunctionCall:
			if arg.Fun != nil && arg.Fun.GetKind() == metadata.KindSelector && arg.Fun.X.Type != -1 {
				selectorArg := arg.Fun
				varName := metadata.CallArgToString(selectorArg.X)

				pkey := paramKey{
					Name:      varName,
					Pkg:       getString(meta, edge.Caller.Pkg),
					Container: getString(meta, edge.Caller.Name),
				}

				if parents, ok := tree.variableNodes[pkey]; ok && len(parents) > 0 {
					// Link to most recent assignment (last in slice) for cleaner tree structure
					mostRecentParent := parents[len(parents)-1]
					mostRecentParent.Children = append(mostRecentParent.Children, argNode)
				}

				if selectorArg.Sel.GetKind() == metadata.KindIdent && strings.HasPrefix(selectorArg.Sel.GetType(), "func(") || strings.HasPrefix(selectorArg.Sel.GetType(), "func[") {
					// Enhanced variable tracing and assignment linking
					originVar, originPkg, _, _ := tree.traceOrigin(
						varName,
						getString(meta, edge.Caller.Name),
						getString(meta, edge.Caller.Pkg),
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

					// First check if ReceiverType is available (for function return values)
					if selectorArg.ReceiverType != nil && originVar == varName {
						recvType = selectorArg.ReceiverType.GetName()
					}

					var FuncType string

					if selectorArg.X.GetKind() == metadata.KindSelector && selectorArg.X.X != nil && selectorArg.X.X.Type != -1 {
						FuncType = selectorArg.X.X.GetType()
						FuncType = strings.ReplaceAll(FuncType, selectorArg.X.X.GetPkg()+".", "")
						FuncType = strings.TrimPrefix(FuncType, "*")
					} else if selectorArg.X.GetKind() == metadata.KindCall && selectorArg.X.Fun != nil && selectorArg.X.Fun.Type != -1 {
						FuncType = selectorArg.X.Fun.GetType()
						// For Call args, the package qualifier lives on Fun
						// (the function being called); X is unset by
						// handleCallExpr. Dereferencing X.X here was a
						// long-standing nil-deref that complex_chi_router
						// happens to trigger.
						FuncType = strings.ReplaceAll(FuncType, selectorArg.X.Fun.GetPkg()+".", "")
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

					// Look for a call graph edge where this function is the caller
					for _, ArgEdge := range meta.CallGraph {
						if ArgEdge.Caller.Name == funcNameIndex && ArgEdge.Caller.Pkg == pkgNameIndex && (ArgEdge.Caller.RecvType == recvTypeIndex || ArgEdge.Caller.RecvType == starRecvTypeIndex) {
							funcEdge = &ArgEdge
							id := funcEdge.Callee.ID()
							if childNode := NewTrackerNode(tree, meta, argNode.Key(), id, funcEdge, nil, visited, assignmentIndex, limits); childNode != nil {
								argNode.AddChild(childNode)
							}
						}
					}

					// Handler-factory pattern: the registered handler is a *call*
					// returning a func literal (e.g. `g.POST(p, h.Create())` where
					// `Create() echo.HandlerFunc { return func(c) error {…} }`). The
					// real body lives in the returned closure, so the loop above —
					// which keys on the method as Caller — finds nothing. Attach the
					// closure body explicitly, resolving the receiver's declared type
					// (here the interface) to its concrete implementer(s).
					tree.attachReturnedClosureBody(meta, argNode, selectorArg.X.GetType(), selectorArg.Sel.GetName(), selectorArg.Sel.GetPkg(), visited, assignmentIndex, limits)
				}
			}

			// Process function call arguments recursively
			if argNode := NewTrackerNode(tree, meta, parentNode.Key(), argID, argEdge, arg, visited, assignmentIndex, limits); argNode != nil {
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
			}

		case ArgTypeVariable:
			varName := metadata.CallArgToString(arg)
			// Enhanced variable tracing and assignment linking
			originVar, originPkg, _, _ := tree.traceOrigin(
				varName,
				getString(meta, edge.Caller.Name),
				getString(meta, edge.Caller.Pkg),
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
				argNode.Parent = parent
			} else {
				akey = assignmentKey{
					Name:      varName,
					Pkg:       getString(meta, edge.Callee.Pkg),
					Type:      arg.GetType(),
					Container: getString(meta, edge.Caller.Name),
				}

				if assignmentNode, ok := (*assignmentIndex)[akey]; ok {
					assignmentNode.Parent = argNode
				}
			}

			pkey := paramKey{
				Name:      originVar,
				Pkg:       originPkg,
				Container: getString(meta, edge.Caller.Name),
			}

			if parents, ok := tree.variableNodes[pkey]; ok && len(parents) > 0 {
				// Link to the most recent assignment (last in slice) as it represents
				// the actual value at the point of use. This creates a cleaner tree
				// structure while preserving the logical relationship.
				mostRecentParent := parents[len(parents)-1]
				mostRecentParent.Children = append(mostRecentParent.Children, argNode)

				// Optionally: Store reference to all possible origins for completeness
				// This allows tracking all possible values without cluttering the tree
				if argNode.RootAssignmentMap == nil {
					argNode.RootAssignmentMap = make(map[string][]metadata.Assignment, 1)
				}
				// The RootAssignmentMap will be populated by the assignment linking above
			}
			children = append(children, argNode)

		case ArgTypeLiteral:
			// Store literal for type inference
			children = append(children, argNode)

		case ArgTypeSelector:
			// Handling a function inside the selector
			// Process field/method access
			if arg.X != nil {
				if arg.Sel.GetKind() == metadata.KindIdent && (strings.HasPrefix(arg.Sel.GetType(), "func(") || strings.HasPrefix(arg.Sel.GetType(), "func[")) {
					varName := metadata.CallArgToString(arg.X)
					// Enhanced variable tracing and assignment linking
					originVar, originPkg, _, _ := tree.traceOrigin(
						varName,
						getString(meta, edge.Caller.Name),
						getString(meta, edge.Caller.Pkg),
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

					// Link param
					pkey := paramKey{
						Name:      originVar,
						Pkg:       originPkg,
						Container: getString(meta, edge.Caller.Name),
					}

					if parents, ok := tree.variableNodes[pkey]; ok && len(parents) > 0 {
						// Link to the most recent assignment (last in slice) as it represents
						// the actual value at the point of use
						mostRecentParent := parents[len(parents)-1]
						mostRecentParent.Children = append(mostRecentParent.Children, argNode)
					}

					// Get the correct edge for method calls
					funcNameIndex := arg.Sel.Name
					recvType := strings.ReplaceAll(originVar, arg.Sel.GetPkg()+".", "")
					// If the selector is a method, we need to get the type of the receiver
					if arg.Sel.Type != -1 && originVar == varName {
						recvType = arg.X.GetType()
						recvType = strings.ReplaceAll(recvType, arg.Sel.GetPkg()+".", "")
					}

					// First check if ReceiverType is available (for function return values)
					if arg.ReceiverType != nil && originVar == varName {
						recvType = arg.ReceiverType.GetName()
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
					starRecvTypeIndex := meta.StringPool.Get("*" + recvType)
					pkgNameIndex := arg.Sel.Pkg

					var funcEdge *metadata.CallGraphEdge

					// Look for a call graph edge where this function is the caller
					for _, ArgEdge := range meta.CallGraph {
						if ArgEdge.Caller.Name == funcNameIndex && ArgEdge.Caller.Pkg == pkgNameIndex && (ArgEdge.Caller.RecvType == recvTypeIndex || ArgEdge.Caller.RecvType == starRecvTypeIndex) {
							funcEdge = &ArgEdge
							id := funcEdge.Callee.ID()
							if childNode := NewTrackerNode(tree, meta, argNode.Key(), id, funcEdge, nil, visited, assignmentIndex, limits); childNode != nil {
								argNode.AddChild(childNode)
							}
						}
					}

					// NOTE: the handler-factory closure resolution intentionally
					// runs only in the KindCall branch above (handler registered as
					// h.Create()). This KindSelector branch is a bare method value
					// (h.Create) and is left to the existing direct-handler lookup.
				}
				varName := metadata.CallArgToString(arg)
				// Trace the base object
				baseVar, originPkg, _, _ := tree.traceOrigin(
					varName,
					getString(meta, edge.Caller.Name),
					getString(meta, edge.Caller.Pkg),
				)

				var parentType = arg.X.GetType()
				// Handle nested selectors: if arg.X is a selector, extract base from arg.X.X

				// For nested selectors, use the base variable's type
				if arg.X != nil && arg.X.GetKind() == metadata.KindSelector && arg.X.X != nil && arg.X.Sel != nil && arg.X.Sel.GetKind() == metadata.KindIdent {
					// Nested selector case: extract base from arg.X.X
					// arg.X is a selector (e.g., obj.field), arg.X.X is the base (e.g., obj)
					parentType = arg.X.X.GetType()
				}

				// Link to assignment if exists
				akey := assignmentKey{
					Name:      baseVar,
					Pkg:       originPkg,
					Type:      arg.GetType(),
					Container: getString(meta, edge.Caller.Name),
				}

				if parentType != "" {
					akey.Container = parentType
				}

				if assignmentNode, ok := (*assignmentIndex)[akey]; ok {
					// TODO: This is a workaround for parameter tracing issue.
					// Proper solution needed to trace parameters passed through functional options
					// (e.g., router parameters: WithOrderRouter(orderRouter) -> r.orderRouter = orderRouter -> router.Mount("/orders", r.orderRouter))
					// See testdata/router_mount_options/ for example case.
					// Current workaround: Set assignmentNode's parent to argNode (reverse link: assignment <- usage)
					// This allows tracking where assignments are used but is confusing and may not handle all cases correctly.
					assignmentNode.Parent = argNode
				}

				// Link to variable node if exists
				pkey := paramKey{
					Name:      baseVar,
					Pkg:       originPkg,
					Container: getString(meta, edge.Caller.Name),
				}

				if parents, ok := tree.variableNodes[pkey]; ok && len(parents) > 0 {
					// Link to most recent assignment (last in slice) for cleaner tree structure
					mostRecentParent := parents[len(parents)-1]
					mostRecentParent.Children = append(mostRecentParent.Children, argNode)
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
					originVar, originPkg, _, _ := tree.traceOrigin(
						arg.X.GetName(),
						getString(meta, edge.Caller.Name),
						getString(meta, edge.Caller.Pkg),
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

// newArgumentNode creates a lightweight TrackerNode for arguments without expensive traversal.
// This is optimized for performance - it skips full child traversal and recursion tracking
// that's done in NewTrackerNode, since arguments are typically leaf nodes.
func newArgumentNode(tree *TrackerTree, parentNode *TrackerNode, argID string, edge *metadata.CallGraphEdge, arg *metadata.CallArgument) *TrackerNode {
	if argID == "" {
		return nil
	}

	// Create new node using object pool
	node := getTrackerNode()
	node.CallGraphEdge = edge
	node.CallArgument = arg
	node.Parent = parentNode

	// Initialize minimal structures - only allocate if needed
	node.Children = make([]*TrackerNode, 0, 2)                         // Small capacity for argument nodes
	node.RootAssignmentMap = make(map[string][]metadata.Assignment, 2) // Small capacity

	// Add to node map for O(1) lookup (only if tree is available)
	if tree != nil && argID != "" {
		tree.nodeMap[argID] = node
	}

	return node
}

// NewTrackerNode creates a new TrackerNode for the tree.
func NewTrackerNode(tree *TrackerTree, meta *metadata.Metadata, parentID, id string, parentEdge *metadata.CallGraphEdge, callArg *metadata.CallArgument, visited map[string]int, assignmentIndex *assigmentIndexMap, limits metadata.TrackerLimits) *TrackerNode {
	if id == "" {
		return nil
	}

	// Direct recursion prevention
	if id == parentID {
		return nil
	}

	// OPTIMIZED: Simple recursion depth tracking with minimal overhead
	// Use a simple counter approach instead of complex map operations
	recursionDepthKey := id
	currentDepth := visited[recursionDepthKey]

	// Use configurable recursion depth limit to prevent infinite recursion.
	// Hitting this is a safety brake, not a truncation — analysis simply
	// stops following the path further. Demoted to verbose-only so a normal
	// run on a self-referential codebase doesn't look like an error.
	if currentDepth >= limits.MaxRecursionDepth {
		tree.infoOnce("recursion:"+id,
			"Info: MaxRecursionDepth limit (%d) reached for node %s (analysis continues)\n", limits.MaxRecursionDepth, id)
		return nil
	}

	// Limit total nodes to prevent memory explosion AND unbounded wall-clock
	// time. This gates on the cumulative count of nodes built for the whole
	// tree (tree.nodesBuilt), not on len(visited): `visited` is a per-path
	// recursion-stack counter (incremented on enter, decremented on exit), so
	// its size is the current stack depth, never the total work. A dense or
	// cyclic graph re-expands shared callees along exponentially many distinct
	// paths while keeping stack depth small, so the old len(visited) check
	// never fired and such graphs ran effectively forever. The cumulative
	// counter bounds them. Once the cap is hit, every further call returns a
	// leaf stub immediately, so the in-flight recursion unwinds cheaply.
	// A MaxNodesPerTree of 0 means "no cap" (the engine always sets a real
	// default; only some unit tests leave it zero). tree may be nil in a few
	// synthetic tests that drive NewTrackerNode directly — the cumulative
	// counter needs a tree, so it is simply disabled there.
	if tree != nil && limits.MaxNodesPerTree > 0 && tree.nodesBuilt >= limits.MaxNodesPerTree {
		// A single global key: once the cumulative cap is hit it is a tree-wide
		// condition, so warn exactly once rather than once per truncated node.
		tree.warnOnce("maxnodes",
			"Warning: MaxNodesPerTree limit (%d) reached, truncating tree (call graph too dense to traverse fully)\n", limits.MaxNodesPerTree)
		node := getTrackerNode()
		node.CallGraphEdge = parentEdge
		node.CallArgument = callArg
		if parentEdge == nil && callArg == nil {
			node.key = id
		}
		return node
	}

	// Increment recursion depth (single map operation)
	visited[recursionDepthKey]++
	if tree != nil {
		tree.nodesBuilt++
	}

	// Create new node using object pool
	node := getTrackerNode()
	node.CallGraphEdge = parentEdge
	node.CallArgument = callArg
	node.RootAssignmentMap = make(map[string][]metadata.Assignment, 5) // Pre-allocate with smaller capacity
	// Pre-allocate children slice with reasonable capacity
	node.Children = make([]*TrackerNode, 0, 6) // Further reduced to 6 for better memory usage
	if parentEdge == nil && callArg == nil {
		node.key = id
	}

	// Process children (callees)
	callerID := metadata.StripToBase(id)
	functionID := callerID

	if parentEdge != nil && parentEdge.CalleeVarName != "" {
		// Enhanced variable tracing and assignment linking
		originVar, originPkg, _, _ := tree.traceOrigin(
			parentEdge.CalleeVarName,
			getString(meta, parentEdge.Caller.Name),
			getString(meta, parentEdge.Caller.Pkg),
		)

		// Link to assignment node if found
		if originVar != "" && originPkg != "" {
			assignmentKey := assignmentKey{
				Name:      originVar,
				Pkg:       originPkg,
				Type:      getString(meta, parentEdge.Callee.RecvType),
				Container: getString(meta, parentEdge.Caller.Name),
			}
			if parent, ok := (*assignmentIndex)[assignmentKey]; ok {
				parent.Children = append(parent.Children, node)
			}
		}

		// Link to variable node if found
		pkey := paramKey{
			Name:      parentEdge.CalleeVarName,
			Pkg:       getString(meta, parentEdge.Caller.Pkg),  // Use cached value
			Container: getString(meta, parentEdge.Callee.Name), // Use cached value
		}

		if parentEdge.ParentFunction != nil {
			pkey.Container = getString(meta, parentEdge.ParentFunction.Name)
		}

		if parents, ok := tree.variableNodes[pkey]; ok && len(parents) > 0 {
			// Link to most recent assignment (last in slice) for cleaner tree structure
			mostRecentParent := parents[len(parents)-1]
			mostRecentParent.Children = append(mostRecentParent.Children, node)
		}

		// Get the correct edge for selector arguments
		recvType := strings.ReplaceAll(originVar, originPkg+".", "")

		functionID = originPkg + "." + recvType + "." + getString(meta, parentEdge.Callee.Name)
	}

	// Look for parent function edges in the ParentFunctions map that is exists in the Callers map
	if parentEdges, exists := meta.ParentFunctions[functionID]; meta.Callers[callerID] == nil && exists && len(parentEdges) > 0 {
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
				tree.warnOnce("maxchildren:"+id,
					"Warning: MaxChildrenPerNode limit (%d) reached for node %s, truncating children\n",
					limits.MaxChildrenPerNode, id)
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

			_, existsInArgs := meta.Args[metadata.StripToBase(calleeID)]

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
					originVar, originPkg, _, originFunc := tree.traceOrigin(
						edge.CalleeVarName,
						funcName,
						callerPkg,
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

					if parents, ok := tree.variableNodes[pkey]; ok && len(parents) > 0 {
						// Link to most recent assignment (last in slice) for cleaner tree structure
						mostRecentParent := parents[len(parents)-1]
						mostRecentParent.Children = append(mostRecentParent.Children, childNode)
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

	// Handle interface method calls by resolving to concrete implementations
	if parentEdge != nil && parentEdge.Callee.RecvType != -1 {
		recvTypeName := getString(meta, parentEdge.Callee.RecvType)
		calleePkg := getString(meta, parentEdge.Callee.Pkg)
		methodName := getString(meta, parentEdge.Callee.Name)

		if pkg, exists := meta.Packages[calleePkg]; exists {
			for _, file := range pkg.Files {
				if typ, exists := file.Types[recvTypeName]; exists {
					kindStr := getString(meta, typ.Kind)
					if kindStr == "interface" && len(typ.ImplementedBy) > 0 {
						for _, implTypeIdx := range typ.ImplementedBy {
							implTypeName := getString(meta, implTypeIdx)
							// ImplementedBy is "import/path.Type"; the import path
							// itself contains dots (github.com/…), so split on the
							// LAST dot — not every dot — to separate pkg from type.
							dot := strings.LastIndex(implTypeName, ".")
							if dot <= 0 || dot == len(implTypeName)-1 {
								continue
							}
							implPkg, implType := implTypeName[:dot], implTypeName[dot+1:]

							if implPkgObj, exists := meta.Packages[implPkg]; exists {
								for _, implFile := range implPkgObj.Files {
									if implTypeObj, exists := implFile.Types[implType]; exists {
										for _, method := range implTypeObj.Methods {
											if getString(meta, method.Name) != methodName {
												continue
											}

											concreteMethodID := implPkg + "." + implType + "." + methodName
											if concreteEdges, exists := meta.Callers[concreteMethodID]; exists {
												for _, concreteEdge := range concreteEdges {
													concreteCalleeID := concreteEdge.Callee.ID()
													if tree.nodeMap[concreteCalleeID] != nil {
														continue
													}

													if childNode := NewTrackerNode(tree, meta, id, concreteCalleeID, concreteEdge, nil, visited, assignmentIndex, limits); childNode != nil {
														node.AddChild(childNode)
													}
												}
											}
										}
									}
								}
							}
						}
					}
				}
			}
		}
	}

	// Add node to hash map for O(1) lookup optimization
	nodeID := node.Key()
	if nodeID != "" {
		tree.nodeMap[nodeID] = node
	}

	// Clean up recursion depth (single map operation)
	visited[recursionDepthKey]--
	if visited[recursionDepthKey] == 0 {
		delete(visited, recursionDepthKey)
	}

	return node
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

// GetMetadata returns the underlying metadata
func (t *TrackerTree) GetMetadata() *metadata.Metadata {
	return t.meta
}

// getString retrieves a string value from the metadata string pool.
func getString(meta *metadata.Metadata, index int) string {
	if meta == nil || meta.StringPool == nil {
		return ""
	}
	return meta.StringPool.GetString(index)
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

// implRef names a concrete type that implements an interface.
type implRef struct{ pkg, typ string }

// interfaceImplementers returns the concrete types recorded as implementing the
// interface named ifaceType in package ifacePkg (via the metadata's
// ImplementedBy index). Returns nil if the type is unknown or not an interface.
func interfaceImplementers(meta *metadata.Metadata, ifacePkg, ifaceType string) []implRef {
	pkg, ok := meta.Packages[ifacePkg]
	if !ok {
		return nil
	}
	var out []implRef
	for _, file := range pkg.Files {
		typ, ok := file.Types[ifaceType]
		if !ok || getString(meta, typ.Kind) != "interface" {
			continue
		}
		for _, idx := range typ.ImplementedBy {
			name := getString(meta, idx) // "import/path.Type"
			// The import path contains dots, so split on the LAST dot.
			dot := strings.LastIndex(name, ".")
			if dot <= 0 || dot == len(name)-1 {
				continue
			}
			out = append(out, implRef{name[:dot], name[dot+1:]})
		}
	}
	// Stable order: implementers feed the per-(pkg,method,typ) closureAttached
	// guard, so a random order would change which route claims a shared
	// handler's closure (and its params) between runs.
	sort.Slice(out, func(i, j int) bool {
		if out[i].pkg != out[j].pkg {
			return out[i].pkg < out[j].pkg
		}
		return out[i].typ < out[j].typ
	})
	return out
}

// attachReturnedClosureBody handles the handler-factory pattern: a route handler
// registered as a *call* to a method that returns a func literal
//
//	g.POST(p, h.Create())   // Create() echo.HandlerFunc { return func(c) {…} }
//
// The closure's calls are recorded with the func literal as Caller and the
// method as ParentFunction, so the normal Caller-keyed lookup misses them and
// the route ends up with no request/response body. This attaches those
// closure-body edges under argNode.
//
// recvDecl is the declared type of the handler's receiver expression (e.g. the
// interface "Handlers" in h.Create()); method/pkg name the handler method. When
// recvDecl is an interface, its concrete implementers are resolved so the
// closure defined on the implementing type is found. It only ever adds
// ParentFunction matches, so it is purely additive to the direct-handler loop.
func (t *TrackerTree) attachReturnedClosureBody(meta *metadata.Metadata, argNode *TrackerNode, recvDecl, method, pkg string, visited map[string]int, assignmentIndex *assigmentIndexMap, limits metadata.TrackerLimits) {
	if argNode == nil || method == "" || pkg == "" {
		return
	}
	recvDecl = strings.TrimPrefix(recvDecl, "*")
	// strip a package qualifier if one slipped in (e.g. "auth.Handlers").
	if dot := strings.LastIndex(recvDecl, "."); dot >= 0 {
		recvDecl = recvDecl[dot+1:]
	}
	if recvDecl == "" {
		return
	}

	// Candidate (pkg, bare-type) receivers the handler method may live on: the
	// declared type itself, plus every concrete implementer when it's an
	// interface (the impl may live in a different package than the interface).
	cands := []implRef{{pkg, recvDecl}}
	cands = append(cands, interfaceImplementers(meta, pkg, recvDecl)...)

	for _, c := range cands {
		// Expand each concrete method's returned closure at most once per tree.
		// Without this guard, re-entrant traversal (the closure body itself
		// containing factory-shaped calls) could fan the same body out
		// repeatedly and blow up on large interface-heavy graphs.
		methodKey := c.pkg + "\x00" + method + "\x00" + c.typ
		if t.closureAttached == nil {
			t.closureAttached = map[string]bool{}
		}
		if t.closureAttached[methodKey] {
			continue
		}
		t.closureAttached[methodKey] = true

		// The request/response/param calls (c.Bind, c.JSON, c.Param, …) sit at
		// the top of the returned closure, so a shallow descent captures them.
		// Cap recursion tightly here: the closure body routinely reaches the
		// business layer (usecase/repository interfaces), and the tracker's
		// per-path expansion would otherwise fan out exponentially through it.
		clim := limits
		if clim.MaxRecursionDepth > 2 {
			clim.MaxRecursionDepth = 2
		}
		for _, k := range t.parentFunctionEdges(meta, c.pkg, method, c.typ) {
			e := &meta.CallGraph[k]
			id := e.Callee.ID()
			if childNode := NewTrackerNode(t, meta, argNode.Key(), id, e, nil, visited, assignmentIndex, clim); childNode != nil {
				argNode.AddChild(childNode)
			}
		}
	}
}

// parentFunctionEdges returns the indices of call-graph edges whose
// ParentFunction is (pkg, name, recvBare) — the calls inside a func literal
// defined within that function. The index is built once and reused.
func (t *TrackerTree) parentFunctionEdges(meta *metadata.Metadata, pkg, name, recvBare string) []int {
	if t.parentFnIndex == nil {
		t.parentFnIndex = make(map[string][]int)
		for k := range meta.CallGraph {
			pf := meta.CallGraph[k].ParentFunction
			if pf == nil {
				continue
			}
			key := getString(meta, pf.Pkg) + "\x00" + getString(meta, pf.Name) + "\x00" +
				strings.TrimPrefix(getString(meta, pf.RecvType), "*")
			t.parentFnIndex[key] = append(t.parentFnIndex[key], k)
		}
	}
	return t.parentFnIndex[pkg+"\x00"+name+"\x00"+strings.TrimPrefix(recvBare, "*")]
}
