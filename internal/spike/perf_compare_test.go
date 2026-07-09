// Performance comparison for docs/TRACKER_REDESIGN.md §3.1: the current
// analysis-structure pipeline (packages.Load + metadata + tracker tree)
// vs. the spike pipeline (packages.Load + SSA + VTA), stage by stage, on the
// same fixtures.
//
// Caveats printed with the results:
//   - The VTA load is heavier by design: it loads dependency *bodies*
//     (NeedDeps|NeedSyntax) so calls into frameworks resolve; the engine's
//     load parses only the target module. dense_graph/cyclic_graph have no
//     dependencies, so those rows compare like for like.
//   - The tracker tree runs under the engine's default safety limits
//     (MaxNodesPerTree etc.) — its time is *truncation-bounded*, not the cost
//     of full traversal. VTA has no such limits and still covers everything.
package spike_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/ehabterra/apispec/internal/engine"
	"github.com/ehabterra/apispec/internal/metadata"
	intspec "github.com/ehabterra/apispec/internal/spec"
)

func TestComparePerf(t *testing.T) {
	fixtures := []string{
		"../../testdata/mux_path_params", // small app + gorilla/mux
		"../../testdata/fiber",           // medium app + fiber v2 (big dep)
		"../../testdata/dense_graph",     // tree stress: dense call graph, no deps
		"../../testdata/cyclic_graph",    // tree stress: cycles, no deps
	}

	type row struct {
		name                            string
		metaDur, treeDur                time.Duration // current pipeline
		vtaTm, lightTm                  vtaTimings    // spike pipeline (full / no dep bodies)
		callEdges, vtaNodes, lightNodes int
		curTotal, vtaTotal, lightTotal  time.Duration
	}
	var rows []row

	for _, dir := range fixtures {
		name := filepath.Base(dir)
		abs, err := filepath.Abs(dir)
		if err != nil {
			t.Fatalf("abs %s: %v", dir, err)
		}

		// --- Current pipeline: load+metadata (one engine call), then tree.
		cfg := engine.DefaultEngineConfig()
		cfg.InputDir = abs
		eng := engine.NewEngine(cfg)

		start := time.Now()
		meta, err := eng.GenerateMetadataOnly()
		metaDur := time.Since(start)
		if err != nil {
			t.Fatalf("%s: GenerateMetadataOnly: %v", name, err)
		}

		limits := metadata.TrackerLimits{
			MaxNodesPerTree:    cfg.MaxNodesPerTree,
			MaxChildrenPerNode: cfg.MaxChildrenPerNode,
			MaxArgsPerFunction: cfg.MaxArgsPerFunction,
			MaxNestedArgsDepth: cfg.MaxNestedArgsDepth,
			MaxRecursionDepth:  cfg.MaxRecursionDepth,
		}
		start = time.Now()
		tree := intspec.NewTrackerTree(meta, limits, engine.NewVerboseLogger(false))
		treeDur := time.Since(start)
		if tree == nil {
			t.Fatalf("%s: nil tracker tree", name)
		}

		// --- Spike pipeline, full: load with dep bodies + SSA + VTA.
		cg, _, vtaTm := buildVTA(t, dir)

		// --- Spike pipeline, light: deps from export data only (no bodies) —
		// the realistic integration mode for reachability queries.
		cgLight, _, lightTm := buildVTAMode(t, dir, false)

		// Sanity: the light graph must still answer the mux reachability
		// question (edge to the bodiless mux.Vars declaration suffices).
		if name == "mux_path_params" {
			handler := funcByName(t, cgLight, "testdata/mux_path_params", "getOrder")
			if ok, _ := reaches(cgLight, handler, isMuxVars); !ok {
				t.Error("light mode: getOrder no longer reaches mux.Vars")
			}
		}

		rows = append(rows, row{
			name:       name,
			metaDur:    metaDur,
			treeDur:    treeDur,
			vtaTm:      vtaTm,
			lightTm:    lightTm,
			callEdges:  len(meta.CallGraph),
			vtaNodes:   len(cg.Nodes),
			lightNodes: len(cgLight.Nodes),
			curTotal:   metaDur + treeDur,
			vtaTotal:   vtaTm.total(),
			lightTotal: lightTm.total(),
		})
	}

	t.Log("fixture            | current: load+meta   tree  | total   || vta-full: load  ssa   vta  | total  | nodes || vta-light: load  ssa   vta  | total  | nodes || meta edges")
	for _, r := range rows {
		t.Logf("%-18s | %12v %7v | %7v || %10v %5v %5v | %6v | %5d || %11v %5v %5v | %6v | %5d || %d",
			r.name,
			r.metaDur.Round(time.Millisecond), r.treeDur.Round(time.Millisecond), r.curTotal.Round(time.Millisecond),
			r.vtaTm.load.Round(time.Millisecond), r.vtaTm.ssa.Round(time.Millisecond), r.vtaTm.vta.Round(time.Millisecond),
			r.vtaTotal.Round(time.Millisecond), r.vtaNodes,
			r.lightTm.load.Round(time.Millisecond), r.lightTm.ssa.Round(time.Millisecond), r.lightTm.vta.Round(time.Millisecond),
			r.lightTotal.Round(time.Millisecond), r.lightNodes,
			r.callEdges)
	}
}
