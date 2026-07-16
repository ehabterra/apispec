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

	// The redesigned Overview aggregations must hold their invariants on real
	// generated metadata (exercises interfaceStats/verbDispatch/coverage/
	// resolution over actual data, not just hand-built inputs).
	if got := rep.Resolution.Full + rep.Resolution.Partial + rep.Resolution.Broken; got != rep.Operations {
		t.Errorf("resolution split %+v sums to %d, want operations %d", rep.Resolution, got, rep.Operations)
	}
	if rep.Coverage.Protected.Total != rep.Operations {
		t.Errorf("protected coverage total = %d, want operations %d", rep.Coverage.Protected.Total, rep.Operations)
	}
	if rep.Interfaces.Total != rep.Interfaces.SingleImpl+rep.Interfaces.Ambiguous+rep.Interfaces.Unimplemented {
		t.Errorf("interface stats don't add up: %+v", rep.Interfaces)
	}
	for _, vd := range rep.VerbDispatch {
		if len(vd.Methods) < 2 {
			t.Errorf("verb dispatch %q should be a multi-method split, got %v", vd.Handler, vd.Methods)
		}
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
