// Copyright 2026 Ehab Terra
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

package insight

import (
	"sort"
	"strings"
	"sync"

	"github.com/ehabterra/apispec/internal/metadata"
	"github.com/ehabterra/apispec/internal/spec"
)

// Traversal bounds. These keep the scoped analysis cheap and bound the
// call-path count so a fan-heavy endpoint reports "{max}+" rather than
// hanging. They mirror the spirit of the engine's TrackerLimits.
const (
	maxReachable  = 600  // distinct functions explored
	maxTraceDepth = 14   // BFS depth cap
	maxPaths      = 1000 // call-path count cap → CallPathsTruncated
	maxGraphNodes = 60   // nodes rendered in the trace diagram
	maxPathSample = 40   // distinct paths enumerated for the breakdown
)

// Metrics are the Tier-1, AST-feasible per-endpoint signals. They
// describe call-graph shape and argument passing — NOT control flow or
// SSA. Truncation flags surface the {max}+ convention.
type Metrics struct {
	Reachable          int     `json:"reachable"`
	MaxDepth           int     `json:"maxDepth"`
	FanoutAvg          float64 `json:"fanoutAvg"`
	FanoutMax          int     `json:"fanoutMax"`
	CallPaths          int     `json:"callPaths"`
	CallPathsTruncated bool    `json:"callPathsTruncated"`
	DepthTruncated     bool    `json:"depthTruncated"`
	PointerArgs        int     `json:"pointerArgs"`
	ValueArgs          int     `json:"valueArgs"`
	ChainDepth         int     `json:"chainDepth"`
	Grade              string  `json:"grade"`           // A..D (heuristic)
	GradeLowerBound    bool    `json:"gradeLowerBound"` // true when truncated
}

// TraceNode / TraceEdge / TraceGraph are the route-scoped subgraph fed to
// the UI's Cytoscape renderer.
type TraceNode struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Depth    int    `json:"depth"`
	Kind     string `json:"kind"`             // handler | callee | leaf
	Pkg      string `json:"pkg,omitempty"`    // import path
	Symbol   string `json:"symbol,omitempty"` // Recv.Method or Func
	Pos      string `json:"pos,omitempty"`    // file:line
	Calls    int    `json:"calls"`            // outgoing edges within this trace
	CalledBy int    `json:"calledBy"`         // incoming edges within this trace
	Origin   string `json:"origin,omitempty"` // project | library | standard
	// Resolved marks a concrete implementation node reached by resolving an
	// interface method call to its implementer — the UI badges these so the
	// "interface → impl" hop is visible.
	Resolved bool `json:"resolved,omitempty"`
	// Sites lists every distinct location this function is called from within
	// the trace (a function reached from several callers, or called more than
	// once by the same caller, has multiple). The UI shows them all and lets
	// the user open the source for any one. Populated only when there is more
	// than one; the single-site case is already covered by Pos.
	Sites []CallSite `json:"sites,omitempty"`
}

// CallSite is one location a trace node is invoked from.
type CallSite struct {
	Pos    string `json:"pos"`              // file:line of the call expression
	Caller string `json:"caller,omitempty"` // short label of the calling function
}

type TraceEdge struct {
	Source string `json:"source"`
	Target string `json:"target"`
}
type TraceGraph struct {
	Nodes     []TraceNode `json:"nodes"`
	Edges     []TraceEdge `json:"edges"`
	Truncated bool        `json:"truncated"`
	// Paths is a sample of the distinct handler→leaf call paths (each a
	// sequence of short labels), so the UI can show *where* the
	// call-paths count comes from. Capped at maxPathSample.
	Paths [][]string `json:"paths"`
}

// builtinFuncs are Go's universe-scope functions — they have no package
// and never carry useful information in a route trace.
var builtinFuncs = map[string]bool{
	"append": true, "cap": true, "clear": true, "close": true, "complex": true,
	"copy": true, "delete": true, "imag": true, "len": true, "make": true,
	"max": true, "min": true, "new": true, "panic": true, "print": true,
	"println": true, "real": true, "recover": true,
}

// calleeIsBuiltin reports whether a callee is a Go builtin (len, append,
// make, …), which never belongs in a call trace. Standard-library calls are
// NOT filtered here: the endpoint trace is the handler's complete scoped
// subtree, and metrics (reachable, fanout, paths, grade) are computed from
// it — dropping stdlib calls like r.URL.Query().Get would silently
// understate them. Stdlib callees are natural leaves (their bodies aren't
// analyzed, so they have no outgoing edges) and carry Origin "standard" so
// the UI can style or collapse them.
func calleeIsBuiltin(meta *metadata.Metadata, c *metadata.Call) bool {
	name := meta.StringPool.GetString(c.Name)
	return builtinFuncs[name] // robust against older metadata that mis-qualified builtins
}

// classifyPkg buckets a callee's package into "project" (under the
// current module), "standard" (Go builtin/stdlib or the builtin error
// interface) or "library" (third-party — a dotted import domain such as
// github.com/…, which includes web frameworks).
func classifyPkg(meta *metadata.Metadata, pkg, recv string) string {
	if recv == "error" {
		return "standard" // err.Error() on the builtin error interface
	}
	if pkg == "" {
		return "standard" // universe scope (builtins)
	}
	if mp := meta.CurrentModulePath; mp != "" && (pkg == mp || strings.HasPrefix(pkg, mp+"/")) {
		return "project"
	}
	first := pkg
	if i := strings.IndexByte(pkg, '/'); i >= 0 {
		first = pkg[:i]
	}
	if !strings.Contains(first, ".") {
		return "standard" // stdlib import paths have no dot in their first segment
	}
	return "library"
}

// analyzeFromHandler builds the scoped call subtree rooted at the handler
// base-ID from the raw call graph (meta.Callers must be populated) and
// computes Tier-1 metrics + the trace graph.
func analyzeFromHandler(meta *metadata.Metadata, root string) (Metrics, TraceGraph) {
	return analyzeAdjacency(meta, root, func(k string) []*metadata.CallGraphEdge { return meta.Callers[k] })
}

// analyzeAdjacency is the shared BFS/metrics core. callersOf returns the
// outgoing call edges of a function key, letting the same logic run over
// either the raw call graph or an interface-resolved tracker tree.
func analyzeAdjacency(meta *metadata.Metadata, root string, callersOf func(string) []*metadata.CallGraphEdge) (Metrics, TraceGraph) {
	m := Metrics{}
	tg := TraceGraph{Nodes: []TraceNode{}, Edges: []TraceEdge{}}

	// adjacency (deduped callees per caller) + depth from BFS
	adj := map[string][]string{}
	depth := map[string]int{root: 0}
	visited := map[string]bool{root: true}
	queue := []string{root}
	fanoutSum, fanoutNodes := 0, 0
	info := map[string]traceMeta{}            // per-node display metadata for the tooltip
	sites := map[string]map[string]CallSite{} // callee -> pos -> call site
	sp := meta.StringPool
	recordSite := func(callee, callerID string, c *metadata.Call) {
		pos := extractFilePos(sp.GetString(c.Position))
		if pos == "" {
			return
		}
		if sites[callee] == nil {
			sites[callee] = map[string]CallSite{}
		}
		if _, ok := sites[callee][pos]; !ok {
			sites[callee][pos] = CallSite{Pos: pos, Caller: shortLabel(callerID)}
		}
	}

	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		d := depth[cur]
		if d >= maxTraceDepth {
			m.DepthTruncated = true
			continue
		}
		seen := map[string]bool{}
		for _, e := range callersOf(cur) {
			// Skip Go builtins (len, append, make, …) only. Stdlib calls stay
			// in as leaves: the trace is the handler's complete subtree and
			// the metrics depend on all of it.
			if calleeIsBuiltin(meta, &e.Callee) {
				continue
			}
			callee := e.Callee.BaseID()
			if callee == "" || callee == cur {
				continue
			}
			recordSite(callee, cur, &e.Callee)
			if seen[callee] {
				continue
			}
			seen[callee] = true
			adj[cur] = append(adj[cur], callee)
			if _, ok := info[cur]; !ok {
				info[cur] = traceMetaFromCall(meta, &e.Caller)
			}
			info[callee] = traceMetaFromCall(meta, &e.Callee)
			if e.ChainDepth > m.ChainDepth {
				m.ChainDepth = e.ChainDepth
			}
			for _, a := range e.Args {
				if a == nil {
					continue
				}
				t := strings.TrimSpace(a.GetType())
				if t == "" {
					continue
				}
				if strings.HasPrefix(t, "*") {
					m.PointerArgs++
				} else {
					m.ValueArgs++
				}
			}
			if !visited[callee] {
				visited[callee] = true
				if len(visited) <= maxReachable {
					depth[callee] = d + 1
					if d+1 > m.MaxDepth {
						m.MaxDepth = d + 1
					}
					queue = append(queue, callee)
				} else {
					m.DepthTruncated = true
				}
			}
		}
		if n := len(adj[cur]); n > 0 {
			fanoutSum += n
			fanoutNodes++
			if n > m.FanoutMax {
				m.FanoutMax = n
			}
		}
	}

	m.Reachable = len(visited)
	if fanoutNodes > 0 {
		m.FanoutAvg = float64(fanoutSum) / float64(fanoutNodes)
	}
	m.CallPaths, m.CallPathsTruncated = countPaths(adj, root)
	m.Grade, m.GradeLowerBound = grade(m)

	buildTraceGraph(&tg, adj, depth, root, info, sites)
	tg.Paths = enumeratePaths(adj, root, maxPathSample)
	return m, tg
}

// analyzeFromTrackerTree builds the trace. It prefers the tracker tree's
// SCOPED subtree for this route — the tree resolves interface calls through
// parameter/assignment tracing (e.g. a handler param `uc UseCase` wired to a
// concrete `*Payment` at module init), which structural matching can't do.
// Walking only the route's subtree avoids whole-tree bleed-through. When the
// route isn't found in the tree (or has no body there) it falls back to the
// call graph augmented with structural interface resolution.
func analyzeFromTrackerTree(meta *metadata.Metadata, cfg *spec.APISpecConfig, method, path, fallbackRoot string) (Metrics, TraceGraph, bool) {
	if tree := cachedTrackerTree(meta); tree != nil && cfg != nil {
		for _, r := range spec.NewExtractor(tree, cfg).ExtractRoutes() {
			if r == nil || r.Node == nil || !strings.EqualFold(r.Method, method) || r.OpenAPIPath() != path {
				continue
			}
			if m, tg, ok := analyzeTrackerSubtree(meta, r.Node); ok {
				return m, tg, true
			}
			break
		}
	}
	return analyzeResolvedCallGraph(meta, fallbackRoot)
}

// analyzeResolvedCallGraph builds the trace from the call graph (scoped to the
// handler's own subtree) and resolves interface-method calls to concrete
// implementations via the metadata's ImplementedBy index. Used as a fallback
// when the tracker tree doesn't carry the route's resolved subtree.
func analyzeResolvedCallGraph(meta *metadata.Metadata, root string) (Metrics, TraceGraph, bool) {
	if len(meta.Callers[root]) == 0 {
		return Metrics{}, TraceGraph{}, false
	}

	sp := meta.StringPool
	m := Metrics{}
	adj := map[string][]string{}
	depth := map[string]int{root: 0}
	info := map[string]traceMeta{}
	sites := map[string]map[string]CallSite{} // callee -> pos -> site
	seen := map[string]map[string]bool{}
	visited := map[string]bool{root: true}
	queue := []string{root}

	recordSite := func(callee, callerID string, c *metadata.Call) {
		pos := extractFilePos(sp.GetString(c.Position))
		if pos == "" {
			return
		}
		if sites[callee] == nil {
			sites[callee] = map[string]CallSite{}
		}
		if _, ok := sites[callee][pos]; !ok {
			sites[callee][pos] = CallSite{Pos: pos, Caller: shortLabel(callerID)}
		}
	}
	addEdge := func(from, to string) {
		if from == to {
			return
		}
		if seen[from] == nil {
			seen[from] = map[string]bool{}
		}
		if !seen[from][to] {
			seen[from][to] = true
			adj[from] = append(adj[from], to)
		}
	}
	enqueue := func(id string, d int) {
		if visited[id] {
			return
		}
		visited[id] = true
		if len(visited) > maxReachable {
			m.DepthTruncated = true
			return
		}
		depth[id] = d
		if d > m.MaxDepth {
			m.MaxDepth = d
		}
		queue = append(queue, id)
	}

	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		d := depth[cur]
		if d >= maxTraceDepth {
			m.DepthTruncated = true
			continue
		}
		seenCallee := map[string]bool{}
		for _, e := range meta.Callers[cur] {
			if calleeIsBuiltin(meta, &e.Callee) {
				continue
			}
			callee := e.Callee.BaseID()
			if callee == "" || callee == cur {
				continue
			}
			recordSite(callee, cur, &e.Callee)
			if seenCallee[callee] {
				continue
			}
			seenCallee[callee] = true
			addEdge(cur, callee)
			info[callee] = traceMetaFromCall(meta, &e.Callee)
			if e.ChainDepth > m.ChainDepth {
				m.ChainDepth = e.ChainDepth
			}
			for _, a := range e.Args {
				if a == nil {
					continue
				}
				if t := strings.TrimSpace(a.GetType()); t != "" {
					if strings.HasPrefix(t, "*") {
						m.PointerArgs++
					} else {
						m.ValueArgs++
					}
				}
			}
			enqueue(callee, d+1)

			// Interface resolution: a callee with no body of its own that is an
			// interface method resolves to its concrete implementation(s). Link
			// the interface method to each concrete method and descend into it.
			if len(meta.Callers[callee]) == 0 {
				for cid, cmeta := range resolveImplementers(meta, &e.Callee) {
					addEdge(callee, cid)
					recordSite(cid, cur, &e.Callee)
					if _, ok := info[cid]; !ok {
						info[cid] = cmeta
					}
					enqueue(cid, d+2)
				}
			}
		}
	}

	m.Reachable = len(visited)
	fsum, fn := 0, 0
	for _, ch := range adj {
		if len(ch) > 0 {
			fsum += len(ch)
			fn++
			if len(ch) > m.FanoutMax {
				m.FanoutMax = len(ch)
			}
		}
	}
	if fn > 0 {
		m.FanoutAvg = float64(fsum) / float64(fn)
	}
	m.CallPaths, m.CallPathsTruncated = countPaths(adj, root)
	m.Grade, m.GradeLowerBound = grade(m)

	tg := TraceGraph{Nodes: []TraceNode{}, Edges: []TraceEdge{}}
	buildTraceGraph(&tg, adj, depth, root, info, sites)
	tg.Paths = enumeratePaths(adj, root, maxPathSample)
	return m, tg, true
}

// analyzeTrackerSubtree builds the trace from the tracker tree's node structure
// under a route node — SCOPED to that route, so no shared-helper bleed-through.
// A node's id is the function it calls (callee BaseID); its children are that
// function's resolved sub-calls. When the tree resolved an interface call (via
// parameter/assignment tracing) it attaches the concrete implementation's body
// as children of the interface-call node, with those child edges' Caller being
// the concrete method. We surface that concrete method as its own node, so the
// hop "interface → concrete impl → body" is explicit. Returns ok=false when the
// route node carries no handler body (caller then falls back to the call graph).
func analyzeTrackerSubtree(meta *metadata.Metadata, routeNode spec.TrackerNodeInterface) (Metrics, TraceGraph, bool) {
	m := Metrics{}
	empty := TraceGraph{Nodes: []TraceNode{}, Edges: []TraceEdge{}}

	kids := routeNode.GetChildren()
	root := ""
	for _, c := range kids {
		if e := c.GetEdge(); e != nil && e.Caller.BaseID() != "" {
			root = e.Caller.BaseID()
			break
		}
	}
	if root == "" {
		return m, empty, false
	}

	sp := meta.StringPool
	adj := map[string][]string{}
	depth := map[string]int{root: 0}
	info := map[string]traceMeta{}
	sites := map[string]map[string]CallSite{} // callee -> pos -> site
	seen := map[string]map[string]bool{}
	visited := map[string]bool{}
	recordSite := func(callee, callerID string, c *metadata.Call) {
		pos := extractFilePos(sp.GetString(c.Position))
		if pos == "" {
			return
		}
		if sites[callee] == nil {
			sites[callee] = map[string]CallSite{}
		}
		if _, ok := sites[callee][pos]; !ok {
			sites[callee][pos] = CallSite{Pos: pos, Caller: shortLabel(callerID)}
		}
	}
	addEdge := func(from, to string) {
		if from == to {
			return
		}
		if seen[from] == nil {
			seen[from] = map[string]bool{}
		}
		if !seen[from][to] {
			seen[from][to] = true
			adj[from] = append(adj[from], to)
		}
	}
	setDepth := func(id string, d int) {
		if _, ok := depth[id]; !ok {
			depth[id] = d
			if d > m.MaxDepth {
				m.MaxDepth = d
			}
		}
	}

	type qi struct {
		node spec.TrackerNodeInterface
		id   string
		d    int
	}
	var queue []qi
	for _, c := range kids {
		if e := c.GetEdge(); e != nil {
			info[root] = traceMetaFromCall(meta, &e.Caller)
			break
		}
	}
	for _, c := range kids {
		queue = append(queue, qi{c, root, 0})
	}

	for len(queue) > 0 {
		it := queue[0]
		queue = queue[1:]
		if it.d >= maxTraceDepth {
			m.DepthTruncated = true
			continue
		}
		if k := it.node.GetKey(); k != "" {
			if visited[k] {
				continue
			}
			visited[k] = true
		}
		ce := it.node.GetEdge()
		if ce == nil {
			for _, c := range it.node.GetChildren() {
				queue = append(queue, qi{c, it.id, it.d})
			}
			continue
		}
		if calleeIsBuiltin(meta, &ce.Callee) {
			continue
		}
		callee := ce.Callee.BaseID()
		if callee == "" {
			continue
		}
		parentID, d := it.id, it.d
		// Resolution boundary: this call's Caller differs from the function we
		// are inside → the tree resolved an interface to a concrete impl. Show
		// the concrete method as an intermediate node.
		if caller := ce.Caller.BaseID(); caller != "" && caller != parentID && caller != callee {
			addEdge(parentID, caller)
			recordSite(caller, parentID, &ce.Caller)
			setDepth(caller, d+1)
			if _, ok := info[caller]; !ok {
				cm := traceMetaFromCall(meta, &ce.Caller)
				cm.resolved = true // concrete impl reached via interface resolution
				info[caller] = cm
			}
			parentID, d = caller, d+1
		}
		if callee != parentID {
			addEdge(parentID, callee)
			recordSite(callee, parentID, &ce.Callee)
			info[callee] = traceMetaFromCall(meta, &ce.Callee)
			setDepth(callee, d+1)
			if ce.ChainDepth > m.ChainDepth {
				m.ChainDepth = ce.ChainDepth
			}
			for _, a := range ce.Args {
				if a == nil {
					continue
				}
				if t := strings.TrimSpace(a.GetType()); t != "" {
					if strings.HasPrefix(t, "*") {
						m.PointerArgs++
					} else {
						m.ValueArgs++
					}
				}
			}
		}
		if len(depth) > maxReachable {
			m.DepthTruncated = true
			continue
		}
		for _, c := range it.node.GetChildren() {
			queue = append(queue, qi{c, callee, d + 1})
		}
	}

	if len(adj[root]) == 0 {
		return m, empty, false
	}

	m.Reachable = len(depth)
	fsum, fn := 0, 0
	for _, ch := range adj {
		if len(ch) > 0 {
			fsum += len(ch)
			fn++
			if len(ch) > m.FanoutMax {
				m.FanoutMax = len(ch)
			}
		}
	}
	if fn > 0 {
		m.FanoutAvg = float64(fsum) / float64(fn)
	}
	m.CallPaths, m.CallPathsTruncated = countPaths(adj, root)
	m.Grade, m.GradeLowerBound = grade(m)

	tg := TraceGraph{Nodes: []TraceNode{}, Edges: []TraceEdge{}}
	buildTraceGraph(&tg, adj, depth, root, info, sites)
	tg.Paths = enumeratePaths(adj, root, maxPathSample)
	return m, tg, true
}

// trackerLimits are generous bounds for the insight tree (mirrors the diagram
// server's limits) — large enough to capture real flow while terminating.
func trackerLimits() metadata.TrackerLimits {
	return metadata.TrackerLimits{
		MaxNodesPerTree:    50000,
		MaxChildrenPerNode: 500,
		MaxArgsPerFunction: 100,
		MaxNestedArgsDepth: 100,
		MaxRecursionDepth:  1000,
	}
}

// cachedTrackerTree builds and memoizes the tracker tree for a metadata,
// rebuilding only when the metadata pointer changes (after a regenerate).
var (
	treeMu  sync.Mutex
	treeKey *metadata.Metadata
	treeVal spec.TrackerTreeInterface
)

func cachedTrackerTree(meta *metadata.Metadata) spec.TrackerTreeInterface {
	if meta == nil {
		return nil
	}
	treeMu.Lock()
	defer treeMu.Unlock()
	if treeKey == meta && treeVal != nil {
		return treeVal
	}
	treeVal = spec.NewTrackerTree(meta, trackerLimits(), nil)
	treeKey = meta
	return treeVal
}

// resolveImplementers maps an interface-method call to the concrete method(s)
// that implement it, using the metadata's ImplementedBy index. Returns concrete
// method base-IDs (that have a recorded body) → display metadata.
func resolveImplementers(meta *metadata.Metadata, callee *metadata.Call) map[string]traceMeta {
	sp := meta.StringPool
	recv := sp.GetString(callee.RecvType)
	pkgPath := sp.GetString(callee.Pkg)
	name := sp.GetString(callee.Name)
	if recv == "" || pkgPath == "" || name == "" {
		return nil
	}
	pkg, ok := meta.Packages[pkgPath]
	if !ok {
		return nil
	}
	out := map[string]traceMeta{}
	for _, file := range pkg.Files {
		typ, ok := file.Types[recv]
		if !ok || sp.GetString(typ.Kind) != "interface" {
			continue
		}
		for _, implIdx := range typ.ImplementedBy {
			implName := sp.GetString(implIdx) // "import/path.Type"
			dot := strings.LastIndex(implName, ".")
			if dot <= 0 || dot == len(implName)-1 {
				continue
			}
			concreteID := implName[:dot] + "." + implName[dot+1:] + "." + name
			if edges := meta.Callers[concreteID]; len(edges) > 0 {
				cm := traceMetaFromCall(meta, &edges[0].Caller)
				cm.resolved = true // concrete impl reached via interface resolution
				out[concreteID] = cm
			}
		}
	}
	return out
}

// enumeratePaths lists distinct root→leaf paths (each a sequence of short
// labels), capped at `cap`. Mirrors countPaths' semantics: a child
// already on the current path (cycle) is skipped, and a node whose
// children are all cyclic is treated as a leaf.
func enumeratePaths(adj map[string][]string, root string, cap int) [][]string {
	out := [][]string{}
	onStack := map[string]bool{}
	var path []string
	var dfs func(n string)
	dfs = func(n string) {
		if len(out) >= cap {
			return
		}
		path = append(path, shortLabel(n))
		var real []string
		for _, k := range adj[n] {
			if !onStack[k] {
				real = append(real, k)
			}
		}
		if len(real) == 0 {
			cp := make([]string, len(path))
			copy(cp, path)
			out = append(out, cp)
		} else {
			onStack[n] = true
			for _, k := range real {
				if len(out) >= cap {
					break
				}
				dfs(k)
			}
			onStack[n] = false
		}
		path = path[:len(path)-1]
	}
	dfs(root)
	return out
}

// traceMeta is per-node display metadata surfaced in the trace tooltip.
type traceMeta struct {
	pkg      string
	symbol   string
	pos      string
	origin   string
	resolved bool // concrete impl reached via interface resolution
}

func traceMetaFromCall(meta *metadata.Metadata, c *metadata.Call) traceMeta {
	sp := meta.StringPool
	pkg := sp.GetString(c.Pkg)
	recv := sp.GetString(c.RecvType)
	name := sp.GetString(c.Name)
	sym := name
	if recv != "" {
		sym = recv + "." + name
	}
	return traceMeta{
		pkg:    pkg,
		symbol: sym,
		pos:    extractFilePos(sp.GetString(c.Position)),
		origin: classifyPkg(meta, pkg, recv),
	}
}

// countPaths counts root→leaf paths over the adjacency DAG, capping at
// maxPaths. On-stack guard breaks cycles so it always terminates.
func countPaths(adj map[string][]string, root string) (int, bool) {
	memo := map[string]int{}
	onStack := map[string]bool{}
	truncated := false
	var dfs func(n string) int
	dfs = func(n string) int {
		if v, ok := memo[n]; ok {
			return v
		}
		kids := adj[n]
		if len(kids) == 0 {
			return 1
		}
		onStack[n] = true
		total := 0
		for _, k := range kids {
			if onStack[k] {
				continue // cycle
			}
			total += dfs(k)
			if total >= maxPaths {
				total = maxPaths
				truncated = true
				break
			}
		}
		onStack[n] = false
		if total == 0 {
			total = 1 // all children were cyclic → treat as terminal
		}
		if !truncated {
			memo[n] = total
		}
		return total
	}
	return dfs(root), truncated
}

func grade(m Metrics) (string, bool) {
	lb := m.CallPathsTruncated || m.DepthTruncated
	g := "D"
	switch {
	case m.MaxDepth <= 4 && m.CallPaths <= 10 && m.FanoutMax <= 6:
		g = "A"
	case m.MaxDepth <= 7 && m.CallPaths <= 50:
		g = "B"
	case m.MaxDepth <= 10 && m.CallPaths <= 200:
		g = "C"
	}
	if lb && g < "C" { // truncated → grade is at best a lower bound of C
		g = "C"
	}
	return g, lb
}

// buildTraceGraph emits up to maxGraphNodes nodes (closest to the
// handler) and the edges among them, enriched with per-node metadata.
func buildTraceGraph(tg *TraceGraph, adj map[string][]string, depth map[string]int, root string, info map[string]traceMeta, sites map[string]map[string]CallSite) {
	// in-trace fan-in (how many nodes call each node)
	calledBy := map[string]int{}
	for _, kids := range adj {
		for _, k := range kids {
			calledBy[k]++
		}
	}
	// nodes ordered by depth then name for stable rendering
	type nd struct {
		id string
		d  int
	}
	all := make([]nd, 0, len(depth))
	for id, d := range depth {
		all = append(all, nd{id, d})
	}
	// simple stable sort by depth then id
	for i := 1; i < len(all); i++ {
		for j := i; j > 0 && (all[j].d < all[j-1].d || (all[j].d == all[j-1].d && all[j].id < all[j-1].id)); j-- {
			all[j], all[j-1] = all[j-1], all[j]
		}
	}
	included := map[string]bool{}
	for _, n := range all {
		if len(tg.Nodes) >= maxGraphNodes {
			tg.Truncated = true
			break
		}
		kind := "callee"
		if n.id == root {
			kind = "handler"
		} else if len(adj[n.id]) == 0 {
			kind = "leaf"
		}
		md := info[n.id]
		tg.Nodes = append(tg.Nodes, TraceNode{
			ID:       n.id,
			Label:    shortLabel(n.id),
			Depth:    n.d,
			Kind:     kind,
			Pkg:      md.pkg,
			Symbol:   md.symbol,
			Pos:      md.pos,
			Origin:   md.origin,
			Resolved: md.resolved,
			Sites:    sortedSites(sites[n.id]),
			Calls:    len(adj[n.id]),
			CalledBy: calledBy[n.id],
		})
		included[n.id] = true
	}
	for src, kids := range adj {
		if !included[src] {
			continue
		}
		for _, k := range kids {
			if included[k] {
				tg.Edges = append(tg.Edges, TraceEdge{Source: src, Target: k})
			}
		}
	}
	if tg.Edges == nil {
		tg.Edges = []TraceEdge{}
	}
}

// sortedSites flattens the per-callee call-site set into a stable, sorted
// slice. Returns nil for 0 or 1 sites: a single location is already shown by
// the node's Pos, so Sites is reserved for the multi-location case the UI
// renders as a pickable list.
func sortedSites(m map[string]CallSite) []CallSite {
	if len(m) < 2 {
		return nil
	}
	out := make([]CallSite, 0, len(m))
	for _, s := range m {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Pos != out[j].Pos {
			return out[i].Pos < out[j].Pos
		}
		return out[i].Caller < out[j].Caller
	})
	return out
}

// shortLabel keeps the last package segment + symbol for readability.
func shortLabel(baseID string) string {
	// baseID looks like "github.com/x/y/pkg.Type.Method" or "pkg.Func"
	slash := strings.LastIndex(baseID, "/")
	tail := baseID
	if slash >= 0 {
		tail = baseID[slash+1:]
	}
	return tail
}
