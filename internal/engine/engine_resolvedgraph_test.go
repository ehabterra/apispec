package engine

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestResolveCallGraph_Wiring verifies the ResolveCallGraph config flag
// builds the SSA+VTA graph from the engine's own package load, and — the
// property step 3 (function summaries) depends on — that metadata call-graph
// BaseIDs join the resolved graph's function IDs.
func TestResolveCallGraph_Wiring(t *testing.T) {
	dir, err := filepath.Abs("../../testdata/mux_path_params")
	if err != nil {
		t.Fatal(err)
	}

	cfg := DefaultEngineConfig()
	cfg.InputDir = dir
	cfg.ResolveCallGraph = true
	eng := NewEngine(cfg)

	meta, err := eng.GenerateMetadataOnly()
	if err != nil {
		t.Fatalf("GenerateMetadataOnly: %v", err)
	}

	resolved := eng.GetResolvedCallGraph()
	if resolved == nil {
		t.Fatal("GetResolvedCallGraph returned nil with ResolveCallGraph enabled")
	}

	// Joinability: every named module-local caller in metadata's call graph
	// must index into the resolved graph under the same BaseID.
	const modPkg = "testdata/mux_path_params"
	checked := 0
	for i := range meta.CallGraph {
		id := meta.CallGraph[i].Caller.BaseID()
		// HasPrefix rather than a fixed-width slice: a BaseID shorter than
		// len(modPkg)+1 (short stdlib/framework callers like "main.main") would
		// otherwise panic with slice-bounds-out-of-range instead of just
		// failing the module-prefix check.
		if !strings.HasPrefix(id, modPkg+".") {
			continue
		}
		checked++
		if len(resolved.FunctionsByID(id)) == 0 {
			t.Errorf("metadata caller BaseID %q has no resolved-graph function", id)
		}
	}
	if checked == 0 {
		t.Fatal("no module-local callers found in metadata call graph")
	}

	// The graph answers the queries step 3 will ask (no depth bound).
	if !resolved.ReachesID(modPkg+".getOrder", "github.com/gorilla/mux.Vars") {
		t.Error("resolved graph: getOrder does not reach mux.Vars through pathVar")
	}

	// Default stays off: no resolved graph unless asked for.
	off := NewEngine(&EngineConfig{InputDir: dir})
	if _, err := off.GenerateMetadataOnly(); err != nil {
		t.Fatalf("GenerateMetadataOnly (flag off): %v", err)
	}
	if off.GetResolvedCallGraph() != nil {
		t.Error("resolved graph built despite ResolveCallGraph=false")
	}
}
