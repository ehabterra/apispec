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

package generator

import (
	"path/filepath"
	"testing"

	"github.com/ehabterra/apispec/internal/engine"
)

// TestTestdata_UnmatchedRouterIsReported covers issue #379: a project whose
// router apispec has no patterns for produced an empty spec, exit 0, and no
// diagnostic — indistinguishable from a project that genuinely serves no HTTP.
//
// The fixture is a hand-rolled router (handlers in a map, dispatched by hand),
// so it needs no third-party dependency and no pattern can be expected to match
// it. What must hold is that apispec KNOWS it found nothing: it analysed a
// non-empty call graph and matched no registration in it.
func TestTestdata_UnmatchedRouterIsReported(t *testing.T) {
	cfg := engine.DefaultEngineConfig()
	cfg.InputDir = filepath.Join("..", "testdata", "unmatched_router")

	eng := engine.NewEngine(cfg)
	out, err := eng.GenerateOpenAPI()
	if err != nil {
		t.Fatalf("GenerateOpenAPI: %v", err)
	}
	if len(out.Paths) != 0 {
		t.Fatalf("fixture documents %v; it exists precisely because nothing should match", mapPathKeys(out.Paths))
	}

	d := eng.GetRouteDiscovery()
	if !d.NothingMatched() {
		t.Errorf("an unmatched router must be reported, got %+v", d)
	}
	if d.CallEdges == 0 {
		t.Errorf("the fixture should give the walk something to analyse; got %d call edges", d.CallEdges)
	}
	if d.Paths != 0 || d.Packages == 0 || len(d.Frameworks) == 0 {
		t.Errorf("RouteDiscovery should carry what was analysed and under which patterns, got %+v", d)
	}
}

// TestRouteDiscoveryIsQuietWhenRoutesMatch is the other half: the diagnostic
// must not cry wolf on a project whose routes DO resolve, or it becomes noise
// everybody learns to ignore.
func TestRouteDiscoveryIsQuietWhenRoutesMatch(t *testing.T) {
	for _, fixture := range []string{"chi", "gin", "mux", "servemux"} {
		t.Run(fixture, func(t *testing.T) {
			cfg := engine.DefaultEngineConfig()
			cfg.InputDir = filepath.Join("..", "testdata", fixture)

			eng := engine.NewEngine(cfg)
			if _, err := eng.GenerateOpenAPI(); err != nil {
				t.Fatalf("GenerateOpenAPI: %v", err)
			}
			if d := eng.GetRouteDiscovery(); d.NothingMatched() {
				t.Errorf("%s documents routes; the no-routes diagnostic must stay quiet, got %+v", fixture, d)
			}
		})
	}
}
