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

// TestHandlerFactory_TraceResolvesAcrossPackages drives the insight endpoint
// report for the echo_handler_factory fixture, whose handler is an interface
// method (api.Handlers.Create) implemented in a *different* package
// (handlers.userHandlers) and returned as a closure. The trace must still
// locate the handler in the call graph — otherwise the UI shows "the handler
// couldn't be located in the call graph for this route".
func TestHandlerFactory_TraceResolvesAcrossPackages(t *testing.T) {
	// Driven through the ENGINE rather than the checked-in metadata.yaml: the
	// analysis needs facts a YAML round-trip does not restore, so a tree built
	// from the file resolves less than the pipeline does. The eager tree
	// tolerated it; keeping the file as the input would test a shape nothing
	// ships (issue #425).
	cfg := spec.DefaultEchoConfig()
	eng := engine.NewEngine(&engine.EngineConfig{
		InputDir:                     "../../testdata/echo_handler_factory",
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
		t.Fatalf("GenerateOpenAPI: %v", err)
	}
	meta := eng.GetMetadata()
	if meta == nil {
		t.Fatal("no metadata from the engine")
	}

	for _, tc := range []struct{ method, path string }{
		{"POST", "/api/v1/users"},
		{"GET", "/api/v1/users/{id}"},
	} {
		rep := BuildEndpointWithSource(out, meta, cfg, tc.method, tc.path, TraceSourceTracker)
		if !rep.Found {
			t.Errorf("%s %s: route not found in spec", tc.method, tc.path)
			continue
		}
		if !rep.HandlerFound {
			t.Errorf("%s %s: handler not located in the call graph (interface→impl across packages not resolved)", tc.method, tc.path)
			continue
		}
		if len(rep.Trace.Nodes) == 0 {
			t.Errorf("%s %s: handler located but trace is empty", tc.method, tc.path)
		}
	}
}
