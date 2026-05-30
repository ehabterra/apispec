package insight

import (
	"testing"

	"github.com/ehabterra/apispec/internal/engine"
	"github.com/ehabterra/apispec/internal/spec"
)

// TestEndToEnd_TrackerTraceFromTestdata generates a real spec + metadata from
// the echo testdata module via the engine, then drives BuildEndpointWithSource
// for every route under both trace sources. This exercises the parts the
// metadata-only fixtures can't: the Extractor route→node match and
// analyzeTrackerSubtree (the scoped tracker-tree walk), end to end.
func TestEndToEnd_TrackerTraceFromTestdata(t *testing.T) {
	cfg := spec.DefaultEchoConfig()
	eng := engine.NewEngine(&engine.EngineConfig{
		InputDir:                     "../../testdata/echo",
		APISpecConfig:                cfg,
		OpenAPIVersion:               "3.1.0",
		MaxNodesPerTree:              engine.DefaultMaxNodesPerTree,
		MaxChildrenPerNode:           engine.DefaultMaxChildrenPerNode,
		MaxArgsPerFunction:           engine.DefaultMaxArgsPerFunction,
		MaxNestedArgsDepth:           engine.DefaultMaxNestedArgsDepth,
		MaxRecursionDepth:            engine.DefaultMaxRecursionDepth,
		SkipCGOPackages:              true,
		AnalyzeFrameworkDependencies: true,
		AutoIncludeFrameworkPackages: true,
		AutoExcludeTests:             true,
		AutoExcludeMocks:             true,
	})
	out, err := eng.GenerateOpenAPI()
	if err != nil {
		t.Skipf("engine generate unavailable in this environment: %v", err)
	}
	meta := eng.GetMetadata()
	if out == nil || meta == nil || len(out.Paths) == 0 {
		t.Skip("no spec/metadata produced")
	}

	rep := BuildOverview(out, meta)
	if len(rep.Endpoints) == 0 {
		t.Skip("no endpoints in overview")
	}

	tracker, callgraph, handlerFound := 0, 0, 0
	for _, ep := range rep.Endpoints {
		tr := BuildEndpointWithSource(out, meta, cfg, ep.Method, ep.Path, TraceSourceTracker)
		if !tr.HandlerFound {
			continue
		}
		handlerFound++
		if tr.TraceSource == TraceSourceTracker && len(tr.Trace.Nodes) > 0 {
			tracker++
		}
		cg := BuildEndpointWithSource(out, meta, cfg, ep.Method, ep.Path, TraceSourceCallGraph)
		if cg.TraceSource == TraceSourceCallGraph && len(cg.Trace.Nodes) > 0 {
			callgraph++
		}
	}

	if handlerFound == 0 {
		t.Skip("no handlers located in the call graph for this fixture")
	}
	if tracker == 0 {
		t.Fatalf("expected at least one route to engage the tracker tree (analyzeTrackerSubtree); tracker=%d callgraph=%d of %d handlers", tracker, callgraph, handlerFound)
	}
	if callgraph == 0 {
		t.Fatalf("call-graph trace source produced nothing for %d handlers", handlerFound)
	}
}
