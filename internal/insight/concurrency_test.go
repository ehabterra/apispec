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
	"sync"
	"testing"

	"github.com/ehabterra/apispec/internal/engine"
	"github.com/ehabterra/apispec/internal/spec"
)

// TestConcurrentInsightAnalysis pins that two insight requests over ONE
// metadata cannot corrupt it.
//
// The UI server is the only concurrent consumer of the analysis pipeline: it
// holds a single *Metadata and a single tracker tree, takes s.mu only long
// enough to COPY those pointers, and then runs the whole analysis outside the
// lock. Two browser tabs — or one tab firing the endpoint and export requests
// together — therefore walked the same structures at the same time.
//
// That is not a benign race. Nearly every identity in the analysis pipeline is
// computed lazily and memoized in place (CallIdentifier.idCache, Call's
// base/instance ID caches, the tracker tree's plan/trace/generic caches), so a
// read path writes maps. It crashed a real session with:
//
//	fatal error: concurrent map writes
//	  metadata.(*CallIdentifier).ID(...)
//	  metadata.(*Call).BaseID(...)
//	  spec.(*Extractor).followsControlFlow(...)
//	  insight.analyzeFromTrackerTree(...)
//	  main.(*UIServer).handleInsightEndpoint(...)
//
// which is unrecoverable — no handler recover() can save the process.
//
// Run under -race this test fails on the write/write pair; without -race it
// reproduces the fatal error often enough to matter. Both endpoints are
// exercised because they reach the shared state by different routes:
// BuildEndpointWithSource walks the tracker tree, BuildOverview walks the call
// graph, and both mutate the metadata's identifier caches.
func TestConcurrentInsightAnalysis(t *testing.T) {
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

	// Route list comes from a warm-up overview, so the concurrent phase below
	// starts from whatever caches a first request would already have filled —
	// the race must not depend on starting cold.
	warm := BuildOverview(out, meta)
	if len(warm.Endpoints) == 0 {
		t.Skip("no endpoints in overview")
	}

	const goroutines = 8
	var wg sync.WaitGroup
	for i := range goroutines {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			// Half the workers take the tracker path and half the overview
			// path, so the two ways into the shared state overlap.
			if i%2 == 0 {
				for _, ep := range warm.Endpoints {
					if rep := BuildEndpointWithSource(out, meta, cfg, ep.Method, ep.Path, TraceSourceTracker); rep == nil {
						t.Errorf("nil endpoint report for %s %s", ep.Method, ep.Path)
						return
					}
				}
				return
			}
			if rep := BuildOverview(out, meta); rep == nil || len(rep.Endpoints) != len(warm.Endpoints) {
				t.Errorf("overview disagreed with the warm-up run under concurrency")
			}
		}(i)
	}
	wg.Wait()
}
