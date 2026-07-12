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

// Integration check for roadmap step 1 (docs/TRACKER_REDESIGN.md): SCC
// condensation over real fixture metadata. Verifies the callees-first
// invariant edge-by-edge and that recursion clusters condense on the fixtures
// built to stress exactly that.
package spike_test

import (
	"path/filepath"
	"testing"

	"github.com/ehabterra/apispec/internal/engine"
	"github.com/ehabterra/apispec/internal/metadata"
)

func fixtureMetadata(t *testing.T, dir string) *metadata.Metadata {
	t.Helper()
	abs, err := filepath.Abs(dir)
	if err != nil {
		t.Fatalf("abs %s: %v", dir, err)
	}
	cfg := engine.DefaultEngineConfig()
	cfg.InputDir = abs
	meta, err := engine.NewEngine(cfg).GenerateMetadataOnly()
	if err != nil {
		t.Fatalf("%s: GenerateMetadataOnly: %v", dir, err)
	}
	return meta
}

func TestSCC_Fixtures(t *testing.T) {
	cases := []struct {
		dir           string
		wantRecursive bool // fixture is built around recursion/cycles
	}{
		{"../../testdata/cyclic_graph", true},
		{"../../testdata/dense_graph", false},
		{"../../testdata/mux_path_params", false},
	}

	for _, tc := range cases {
		name := filepath.Base(tc.dir)
		t.Run(name, func(t *testing.T) {
			meta := fixtureMetadata(t, tc.dir)
			scc := metadata.BuildCallGraphSCC(meta)

			// Invariant: for every call edge crossing components, the callee's
			// component precedes the caller's (bottom-up order is safe).
			crossEdges, cycleEdges := 0, 0
			for i := range meta.CallGraph {
				edge := &meta.CallGraph[i]
				u, v := edge.Caller.BaseID(), edge.Callee.BaseID()
				if u == "" || v == "" {
					continue
				}
				cu, cv := scc.ComponentOf[u], scc.ComponentOf[v]
				switch {
				case cu == cv:
					cycleEdges++
				case cv > cu:
					t.Errorf("callees-first violated: %s (comp %d) -> %s (comp %d)", u, cu, v, cv)
				default:
					crossEdges++
				}
			}

			recursive := 0
			largest := 0
			for c := range scc.Components {
				if scc.Recursive[c] {
					recursive++
				}
				if len(scc.Components[c]) > largest {
					largest = len(scc.Components[c])
				}
			}
			if tc.wantRecursive && recursive == 0 {
				t.Errorf("%s: expected recursive components, found none", name)
			}

			t.Logf("%s: %d edges -> %d components (%d recursive, largest=%d, cross=%d, in-cycle=%d)",
				name, len(meta.CallGraph), len(scc.Components), recursive, largest, crossEdges, cycleEdges)
		})
	}
}
