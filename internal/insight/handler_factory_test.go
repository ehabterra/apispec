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

	"github.com/ehabterra/apispec/internal/metadata"
	"github.com/ehabterra/apispec/internal/spec"
)

// TestHandlerFactory_TraceResolvesAcrossPackages drives the insight endpoint
// report for the echo_handler_factory fixture, whose handler is an interface
// method (api.Handlers.Create) implemented in a *different* package
// (handlers.userHandlers) and returned as a closure. The trace must still
// locate the handler in the call graph — otherwise the UI shows "the handler
// couldn't be located in the call graph for this route".
func TestHandlerFactory_TraceResolvesAcrossPackages(t *testing.T) {
	meta, err := metadata.LoadMetadata("../../testdata/echo_handler_factory/metadata.yaml")
	if err != nil {
		t.Skipf("fixture unavailable: %v", err)
	}
	meta.BuildCallGraphMaps()

	tree := spec.NewTrackerTree(meta, metadata.TrackerLimits{
		MaxNodesPerTree: 50000, MaxChildrenPerNode: 500, MaxArgsPerFunction: 100,
		MaxNestedArgsDepth: 100, MaxRecursionDepth: 1000,
	}, nil)
	cfg := spec.DefaultEchoConfig()
	out, err := spec.MapMetadataToOpenAPI(tree, cfg, spec.GeneratorConfig{
		OpenAPIVersion: "3.0.3", Title: "factory", APIVersion: "1.0.0",
	})
	if err != nil {
		t.Fatalf("MapMetadataToOpenAPI: %v", err)
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
