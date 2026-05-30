package insight

import (
	"strings"

	"github.com/ehabterra/apispec/internal/metadata"
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

// calleeIsStdOrBuiltin reports whether a callee is a Go builtin or lives
// in the standard library, so the trace can skip it. Project packages
// (under the current module path) and third-party dependencies (import
// paths with a dotted domain, e.g. github.com/...) are kept — the latter
// includes web frameworks, which are meaningful in a trace.
func calleeIsStdOrBuiltin(meta *metadata.Metadata, c *metadata.Call) bool {
	name := meta.StringPool.GetString(c.Name)
	if builtinFuncs[name] { // robust against older metadata that mis-qualified builtins
		return true
	}
	return classifyPkg(meta, meta.StringPool.GetString(c.Pkg), meta.StringPool.GetString(c.RecvType)) == "standard"
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
// base-ID and computes Tier-1 metrics + the trace graph. meta.Callers
// must be populated.
func analyzeFromHandler(meta *metadata.Metadata, root string) (Metrics, TraceGraph) {
	m := Metrics{}
	tg := TraceGraph{Nodes: []TraceNode{}, Edges: []TraceEdge{}}

	// adjacency (deduped callees per caller) + depth from BFS
	adj := map[string][]string{}
	depth := map[string]int{root: 0}
	visited := map[string]bool{root: true}
	queue := []string{root}
	fanoutSum, fanoutNodes := 0, 0
	info := map[string]traceMeta{} // per-node display metadata for the tooltip

	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		d := depth[cur]
		if d >= maxTraceDepth {
			m.DepthTruncated = true
			continue
		}
		seen := map[string]bool{}
		for _, e := range meta.Callers[cur] {
			// Skip Go builtins (len, append, make, …) and standard-library
			// calls (fmt, net/http, encoding/json, …) so the trace shows
			// the project's own call flow (and third-party frameworks),
			// not stdlib noise.
			if calleeIsStdOrBuiltin(meta, &e.Callee) {
				continue
			}
			callee := e.Callee.BaseID()
			if callee == "" || callee == cur || seen[callee] {
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

	buildTraceGraph(&tg, adj, depth, root, info)
	tg.Paths = enumeratePaths(adj, root, maxPathSample)
	return m, tg
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
	pkg    string
	symbol string
	pos    string
	origin string
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
func buildTraceGraph(tg *TraceGraph, adj map[string][]string, depth map[string]int, root string, info map[string]traceMeta) {
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
