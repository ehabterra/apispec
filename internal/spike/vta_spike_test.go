// Package spike_test is the validation spike for docs/TRACKER_REDESIGN.md §3.1:
// build SSA from the same go/packages load the engine already performs, run
// x/tools VTA call-graph construction, and verify it reproduces the facts the
// hand-rolled resolution machinery computes today — plus the deferred
// router-as-function-parameter case it currently cannot.
//
// Spike code: not wired into the product. Delete or promote after evaluation.
package spike_test

import (
	"fmt"
	"go/constant"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/tools/go/callgraph"
	"golang.org/x/tools/go/callgraph/cha"
	"golang.org/x/tools/go/callgraph/vta"
	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"
)

// vtaTimings records per-stage wall-clock for the SSA+VTA pipeline.
type vtaTimings struct {
	load, ssa, vta time.Duration
}

func (tm vtaTimings) total() time.Duration { return tm.load + tm.ssa + tm.vta }

// buildVTA loads the module at dir (whole program, deps included), builds SSA
// with generics instantiated, and returns the VTA call graph seeded by CHA.
func buildVTA(t *testing.T, dir string) (*callgraph.Graph, *ssa.Program, vtaTimings) {
	return buildVTAMode(t, dir, true)
}

// buildVTAMode is buildVTA with a switch for dependency bodies. depBodies=false
// loads dependencies from export data only (types, no syntax) — the realistic
// integration mode: module functions get SSA bodies, calls into deps resolve
// to declared (bodiless) functions, which is all reachability-to-accessor
// queries need.
func buildVTAMode(t *testing.T, dir string, depBodies bool) (*callgraph.Graph, *ssa.Program, vtaTimings) {
	t.Helper()

	abs, err := filepath.Abs(dir)
	if err != nil {
		t.Fatalf("abs %s: %v", dir, err)
	}

	mode := packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles |
		packages.NeedImports | packages.NeedTypes |
		packages.NeedTypesSizes | packages.NeedTypesInfo | packages.NeedSyntax
	if depBodies {
		mode |= packages.NeedDeps
	}

	var tm vtaTimings
	start := time.Now()
	cfg := &packages.Config{
		Mode: mode,
		Dir:  abs,
	}
	pkgs, err := packages.Load(cfg, "./...")
	if err != nil {
		t.Fatalf("packages.Load: %v", err)
	}
	if packages.PrintErrors(pkgs) > 0 {
		t.Fatal("packages contain errors")
	}
	tm.load = time.Since(start)

	start = time.Now()
	prog, _ := ssautil.AllPackages(pkgs, ssa.InstantiateGenerics)
	prog.Build()
	tm.ssa = time.Since(start)

	start = time.Now()
	cg := vta.CallGraph(ssautil.AllFunctions(prog), cha.CallGraph(prog))
	tm.vta = time.Since(start)

	t.Logf("%s: load=%v ssa=%v vta=%v (nodes=%d)", dir, tm.load, tm.ssa, tm.vta, len(cg.Nodes))
	return cg, prog, tm
}

// funcByName finds the unique function with the given name in the given
// package path.
func funcByName(t *testing.T, cg *callgraph.Graph, pkgPath, name string) *ssa.Function {
	t.Helper()
	var found *ssa.Function
	for fn := range cg.Nodes {
		if fn == nil || fn.Pkg == nil {
			continue
		}
		if fn.Pkg.Pkg.Path() == pkgPath && fn.Name() == name {
			if found != nil {
				t.Fatalf("multiple functions %s.%s", pkgPath, name)
			}
			found = fn
		}
	}
	if found == nil {
		t.Fatalf("function %s.%s not in call graph", pkgPath, name)
	}
	return found
}

// reaches reports whether target is transitively callable from from in the
// call graph, and returns the call path when it is.
func reaches(cg *callgraph.Graph, from *ssa.Function, target func(*ssa.Function) bool) (bool, []*callgraph.Edge) {
	start := cg.Nodes[from]
	if start == nil {
		return false, nil
	}
	path := callgraph.PathSearch(start, func(n *callgraph.Node) bool {
		return n.Func != nil && target(n.Func)
	})
	return path != nil, path
}

func isMuxVars(fn *ssa.Function) bool {
	return fn.Name() == "Vars" && fn.Pkg != nil && fn.Pkg.Pkg.Path() == "github.com/gorilla/mux"
}

// TestVTA_MuxAccessorReachability replays the mux path-param reachability
// facts that extractor.handlerReachesAccessor computes via bounded
// meta.Callers walks (asserted end-to-end in TestTestdata_MuxAdvancedPathParams
// and TestTestdata_MuxPathParamKeyMismatch):
//
//	getProduct → mux.Vars  (direct call)          → wired clean
//	getOrder   → mux.Vars  (via pathVar helper)   → wired clean
//	getTag     → mux.Vars  (direct, wrong key)    → wired clean (key diag separate)
//	getItem    ↛ mux.Vars                          → {id} stays warned
func TestVTA_MuxAccessorReachability(t *testing.T) {
	cg, _, _ := buildVTA(t, "../../testdata/mux_path_params")
	const pkg = "testdata/mux_path_params"

	cases := []struct {
		handler string
		want    bool
	}{
		{"getProduct", true},
		{"getOrder", true}, // the helper-indirection case: getOrder → pathVar → mux.Vars
		{"getTag", true},
		{"getItem", false},
	}

	for _, tc := range cases {
		handler := funcByName(t, cg, pkg, tc.handler)
		got, path := reaches(cg, handler, isMuxVars)
		if got != tc.want {
			t.Errorf("%s reaches mux.Vars = %v, want %v", tc.handler, got, tc.want)
			continue
		}
		if got {
			t.Logf("%s → mux.Vars via %s", tc.handler, fmtPath(path))
		}
	}
}

func fmtPath(path []*callgraph.Edge) string {
	s := ""
	for _, e := range path {
		if s != "" {
			s += " → "
		}
		s += e.Callee.Func.String()
	}
	return s
}

// TestVTA_FiberRouterAsParam targets the deferred gap (memory:
// router-as-function-parameter): fiber routes registered inside
// products.Routes(r fiber.Router), where the router arrives as an
// interface-typed parameter. The tracker tree does not traverse these; this
// test shows the facts needed to extract them are all present in SSA + VTA:
//
//  1. products.Routes is reachable from main (VTA edge through the call),
//  2. every registration inside Routes is an interface invoke whose method
//     (Get/Post), literal path, and handler function are recoverable.
func TestVTA_FiberRouterAsParam(t *testing.T) {
	cg, _, _ := buildVTA(t, "../../testdata/fiber")
	const productsPkg = "github.com/ehabterra/apispec/testdata/fiber/products"

	mainFn := funcByName(t, cg, "github.com/ehabterra/apispec/testdata/fiber", "main")
	routesFn := funcByName(t, cg, productsPkg, "Routes")

	// (1) Reachability: main → products.Routes.
	if ok, path := reaches(cg, mainFn, func(fn *ssa.Function) bool { return fn == routesFn }); !ok {
		t.Fatal("main does not reach products.Routes in the VTA graph")
	} else {
		t.Logf("main → products.Routes via %s", fmtPath(path))
	}

	// (2) Enumerate route registrations inside Routes: interface invokes on
	// fiber.Router with a literal path and a handler function argument.
	type reg struct{ method, path, handler string }
	var got []reg
	for _, b := range routesFn.Blocks {
		for _, instr := range b.Instrs {
			call, ok := instr.(ssa.CallInstruction)
			if !ok {
				continue
			}
			common := call.Common()
			if !common.IsInvoke() {
				continue
			}
			method := common.Method.Name()
			if method != "Get" && method != "Post" {
				continue
			}
			r := reg{method: method, path: "?", handler: "?"}
			if len(common.Args) > 0 {
				if c, ok := common.Args[0].(*ssa.Const); ok && c.Value != nil && c.Value.Kind() == constant.String {
					r.path = constant.StringVal(c.Value)
				}
			}
			// Handlers are passed variadically (...fiber.Handler): the arg is a
			// slice built from an alloc; recover the stored function(s).
			for _, arg := range common.Args[1:] {
				for _, fn := range funcsStoredIn(arg) {
					r.handler = fn.Name()
				}
			}
			got = append(got, r)
		}
	}

	want := []reg{
		{"Get", "/", "ListProducts"},
		{"Post", "/", "CreateProduct"},
		{"Get", "/:id", "GetProduct"},
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("registrations in products.Routes:\n got  %v\n want %v", got, want)
	} else {
		t.Logf("recovered registrations: %v", got)
	}
}

// funcsStoredIn digs through the SSA pattern for a variadic slice argument
// (alloc → index-addr → store → slice) and returns any function values stored
// into it, plus the direct case where the value is itself a function.
func funcsStoredIn(v ssa.Value) []*ssa.Function {
	var out []*ssa.Function
	switch v := v.(type) {
	case *ssa.Function:
		out = append(out, v)
	case *ssa.MakeClosure:
		if fn, ok := v.Fn.(*ssa.Function); ok {
			out = append(out, fn)
		}
	case *ssa.Slice:
		out = append(out, funcsStoredIn(v.X)...)
	case *ssa.Alloc:
		for _, ref := range *v.Referrers() {
			ia, ok := ref.(*ssa.IndexAddr)
			if !ok {
				continue
			}
			for _, iref := range *ia.Referrers() {
				if st, ok := iref.(*ssa.Store); ok {
					out = append(out, funcsStoredIn(st.Val)...)
				}
			}
		}
	case *ssa.ChangeType:
		out = append(out, funcsStoredIn(v.X)...)
	case *ssa.Convert:
		out = append(out, funcsStoredIn(v.X)...)
	}
	return out
}
