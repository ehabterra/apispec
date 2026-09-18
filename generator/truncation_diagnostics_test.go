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
	intspec "github.com/ehabterra/apispec/internal/spec"
)

// The diagnostics must survive the whole pipeline, not just the unit that
// builds them.
//
// Everywhere else these limits are forced, the assertions are on
// ExpansionStats — so a regression that dropped the diagnostic slice while
// leaving the statistics intact would pass every test in the repo. The
// statistics say how much; these say WHICH, and that is the half a reader acts
// on (CodeRabbit on #504).
func TestTruncatedOperationsReachTheGeneratorAccessor(t *testing.T) {
	cfg := engine.DefaultEngineConfig()
	cfg.InputDir = filepath.Join("..", "testdata", "group_closure_instances")
	// Far below one route's needs, so subtrees certainly truncate.
	cfg.MaxNodesPerRoute = 5

	eng := engine.NewEngine(cfg)
	if _, err := eng.GenerateOpenAPI(); err != nil {
		t.Fatalf("GenerateOpenAPI: %v", err)
	}

	stats := eng.GetExpansionStats()
	if stats.RouteTruncations == 0 {
		t.Fatal("nothing truncated at --max-nodes-per-route 5; the fixture is not exercising the limit")
	}

	got := eng.GetTruncatedOperations()
	if len(got) == 0 {
		t.Fatalf("%d route subtrees were truncated and the diagnostics named none of them — "+
			"the count is the part that was already reported; the endpoints are the new part",
			stats.RouteTruncations)
	}
	for _, op := range got {
		if op.Limit != intspec.TruncatedByRouteBudget {
			t.Errorf("entry %+v carries limit %q, want %q", op, op.Limit, intspec.TruncatedByRouteBudget)
		}
		// Every entry must be actionable: an operation, or failing that the
		// registration it could not be attributed to.
		if op.Path == "" && op.Registration == "" {
			t.Errorf("entry %+v names neither an operation nor a registration", op)
		}
	}
	// Each truncated scope is reported at least once.
	if len(got) < stats.RouteTruncations {
		t.Errorf("%d entries for %d truncated subtrees; every scope should be represented",
			len(got), stats.RouteTruncations)
	}

	// The same run must also answer the instance-cap question, and on this
	// fixture the honest answer is that nothing was lost — which is the answer
	// a refused-copy count cannot give on its own.
	for _, thin := range eng.GetThinOperations() {
		t.Errorf("%s %s (%s) reported as having lost its response schema, but this "+
			"fixture's responses all resolve", thin.Method, thin.Path, thin.Status)
	}
}
