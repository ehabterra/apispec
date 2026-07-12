// Package callgraph builds the resolved call graph — step 2 of
// docs/TRACKER_REDESIGN.md.
//
// The syntactic call graph in internal/metadata does not contain the real
// edges for indirect calls: an interface method call has no concrete target,
// a handler stored in a variable or parameter has no edge to its body, a
// generic function has no concrete instantiation. This package produces a
// call graph where those targets are resolved, using x/tools SSA plus VTA
// (Variable Type Analysis — the algorithm behind govulncheck), built from
// the same go/packages load the engine already performs.
//
// Function identity: FunctionID formats SSA functions the way metadata
// formats Call.BaseID ("pkg.name", "pkg.recv.name" with the pointer star
// stripped), so results join directly with metadata's call graph and
// meta.Callers keys. Generic instantiations are collapsed to their origin.
// Closures follow SSA naming ("pkg.parent$1"); join those by position when
// needed.
//
// Determinism: every accessor that returns a set sorts it; building twice
// from the same packages yields identical results.
package callgraph

import (
	"sort"
	"strings"

	"go/types"

	xgraph "golang.org/x/tools/go/callgraph"
	"golang.org/x/tools/go/callgraph/cha"
	"golang.org/x/tools/go/callgraph/vta"
	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"
)

// Resolved is the VTA-resolved call graph plus an index joining SSA
// functions to metadata-style BaseIDs.
type Resolved struct {
	Prog  *ssa.Program
	Graph *xgraph.Graph

	byID map[string][]*ssa.Function
}

// Build constructs SSA (with generics instantiated) from already-loaded
// packages and resolves the call graph with VTA seeded by CHA.
//
// The packages must have been loaded with at least NeedName, NeedFiles,
// NeedCompiledGoFiles, NeedImports, NeedTypes, NeedTypesSizes, NeedTypesInfo
// and NeedSyntax. Dependency bodies (NeedDeps with syntax) are NOT required:
// calls into dependencies resolve to declared (bodiless) functions, which is
// sufficient for reachability and registration queries.
func Build(pkgs []*packages.Package) *Resolved {
	prog, _ := ssautil.AllPackages(pkgs, ssa.InstantiateGenerics)
	prog.Build()

	graph := vta.CallGraph(ssautil.AllFunctions(prog), cha.CallGraph(prog))

	r := &Resolved{
		Prog:  prog,
		Graph: graph,
		byID:  make(map[string][]*ssa.Function, len(graph.Nodes)),
	}
	for fn := range graph.Nodes {
		id := FunctionID(fn)
		if id == "" {
			continue
		}
		r.byID[id] = append(r.byID[id], fn)
	}
	for _, fns := range r.byID {
		sortFunctions(fns)
	}
	return r
}

// FunctionID formats fn like metadata's Call.BaseID: "pkg.name" for
// functions and closures ("pkg.parent$1"), "pkg.recv.name" for methods with
// the receiver's pointer star and type arguments stripped. Generic
// instantiations report their origin's ID. Returns "" for synthetic
// functions with no package (runtime intrinsics, wrappers without objects).
func FunctionID(fn *ssa.Function) string {
	if fn == nil {
		return ""
	}
	if origin := fn.Origin(); origin != nil && origin != fn {
		fn = origin
	}

	pkg := ""
	if fn.Pkg != nil && fn.Pkg.Pkg != nil {
		pkg = fn.Pkg.Pkg.Path()
	} else if obj := fn.Object(); obj != nil && obj.Pkg() != nil {
		pkg = obj.Pkg().Path()
	}
	if pkg == "" {
		return ""
	}

	if recv := fn.Signature.Recv(); recv != nil {
		return pkg + "." + bareRecvName(recv.Type()) + "." + fn.Name()
	}
	return pkg + "." + fn.Name()
}

// bareRecvName reduces a receiver type to the bare type name metadata uses
// in BaseIDs: no pointer star, no package qualifier, no type arguments.
func bareRecvName(t types.Type) string {
	if ptr, ok := t.(*types.Pointer); ok {
		t = ptr.Elem()
	}
	switch named := t.(type) {
	case *types.Named:
		return named.Obj().Name()
	case *types.Alias:
		return named.Obj().Name()
	}
	name := types.TypeString(t, func(*types.Package) string { return "" })
	name = strings.TrimPrefix(name, "*")
	name = strings.TrimPrefix(name, ".")
	if i := strings.IndexByte(name, '['); i >= 0 {
		name = name[:i]
	}
	return name
}

// FunctionsByID returns the SSA functions whose FunctionID equals id, sorted
// by position. Multiple results occur for closures sharing a parent name
// pattern or same-named functions in build-tag variants.
func (r *Resolved) FunctionsByID(id string) []*ssa.Function {
	return r.byID[id]
}

// IDs returns every indexed FunctionID, sorted.
func (r *Resolved) IDs() []string {
	ids := make([]string, 0, len(r.byID))
	for id := range r.byID {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// CalleesOf returns fn's resolved callees, deduped and sorted.
func (r *Resolved) CalleesOf(fn *ssa.Function) []*ssa.Function {
	node := r.Graph.Nodes[fn]
	if node == nil {
		return nil
	}
	seen := make(map[*ssa.Function]struct{}, len(node.Out))
	out := make([]*ssa.Function, 0, len(node.Out))
	for _, edge := range node.Out {
		callee := edge.Callee.Func
		if callee == nil {
			continue
		}
		if _, dup := seen[callee]; dup {
			continue
		}
		seen[callee] = struct{}{}
		out = append(out, callee)
	}
	sortFunctions(out)
	return out
}

// Reaches reports whether any function with FunctionID fromID transitively
// calls a function matching match. The walk is a plain BFS over resolved
// edges — no depth limit; cycles are handled by the visited set.
func (r *Resolved) Reaches(fromID string, match func(*ssa.Function) bool) bool {
	queue := append([]*ssa.Function(nil), r.byID[fromID]...)
	visited := make(map[*ssa.Function]struct{}, len(queue))
	for len(queue) > 0 {
		fn := queue[0]
		queue = queue[1:]
		if _, dup := visited[fn]; dup {
			continue
		}
		visited[fn] = struct{}{}
		for _, callee := range r.CalleesOf(fn) {
			if match(callee) {
				return true
			}
			if _, dup := visited[callee]; !dup {
				queue = append(queue, callee)
			}
		}
	}
	return false
}

// ReachesID is Reaches with the target given as a FunctionID.
func (r *Resolved) ReachesID(fromID, targetID string) bool {
	return r.Reaches(fromID, func(fn *ssa.Function) bool {
		return FunctionID(fn) == targetID
	})
}

// sortFunctions orders functions deterministically: by ID, then source
// position, then full SSA string as a final tiebreak.
func sortFunctions(fns []*ssa.Function) {
	sort.Slice(fns, func(i, j int) bool {
		idi, idj := FunctionID(fns[i]), FunctionID(fns[j])
		if idi != idj {
			return idi < idj
		}
		if fns[i].Pos() != fns[j].Pos() {
			return fns[i].Pos() < fns[j].Pos()
		}
		return fns[i].String() < fns[j].String()
	})
}
