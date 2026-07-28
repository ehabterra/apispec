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

package spike

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ehabterra/apispec/internal/callgraph"
	"github.com/ehabterra/apispec/internal/engine"
	"github.com/ehabterra/apispec/internal/metadata"
	"github.com/ehabterra/apispec/internal/spec"
)

// joinReport is what the two graphs agree and disagree about, per project.
type joinReport struct {
	metaEdges int // metadata edges carrying a position
	joined    int // …whose position VTA also has a call at
	agreed    int // …where VTA names the callee metadata named
	resolved  int // …where VTA names a DIFFERENT callee (the interesting ones)

	// classes counts why the disagreements disagree — the spec for what phase 2
	// is allowed to act on.
	classes map[callgraph.Disagreement]int
	// examples keeps one sample per class, for the report.
	examples map[callgraph.Disagreement]string
}

// TestVTACallSiteJoin measures the join the whole SSA/VTA migration rests on:
// can a metadata call edge find the same call in the resolved graph?
//
// It joins on POSITION rather than on function identity, because identity does
// not join for the code that matters — metadata names a closure `FuncLit:<pos>`
// and SSA names it `pkg.parent$1`, an overlap of zero on every project measured,
// while routes and handlers are overwhelmingly closures.
//
// The floor is deliberately modest. Two things legitimately fail to join and
// neither is a defect: metadata records calls into dependencies whose bodies were
// never loaded (so SSA has no call site for them), and VTA drops calls it proves
// unreachable. What the floor guards is the opposite failure — a systematic
// mismatch, where the two sides render positions differently or point at
// different tokens of the same call, which would make every later phase silently
// join nothing.
func TestVTACallSiteJoin(t *testing.T) {
	dirs := []string{
		filepath.Join("..", "..", "testdata", "complex_chi_router"),
		filepath.Join("..", "..", "testdata", "wrapper_router"),
		filepath.Join("..", "..", "testdata", "cli_entrypoint_routes"),
	}
	if env := os.Getenv("APISPEC_JOIN_DIRS"); env != "" {
		dirs = strings.Split(env, string(os.PathListSeparator))
	}

	for _, dir := range dirs {
		t.Run(filepath.Base(dir), func(t *testing.T) {
			report := joinAt(t, dir)
			if report.metaEdges == 0 {
				t.Fatalf("no positioned metadata edges in %s — the join cannot be measured", dir)
			}
			rate := 100 * float64(report.joined) / float64(report.metaEdges)
			t.Logf("%s: %d positioned metadata edges, %d joined (%.1f%%) — %d agree, %d resolved differently",
				filepath.Base(dir), report.metaEdges, report.joined, rate, report.agreed, report.resolved)

			for _, class := range []callgraph.Disagreement{
				callgraph.DisagreeInterface, callgraph.DisagreePromoted,
				callgraph.DisagreeAmbiguous, callgraph.DisagreeUnknown,
			} {
				if n := report.classes[class]; n > 0 {
					t.Logf("  %-26s %5d   e.g. %s", class, n, report.examples[class])
				}
			}

			if rate < 25 {
				t.Errorf("only %.1f%% of positioned edges joined; a systematic position mismatch would look exactly like this", rate)
			}
			// Whatever joins must overwhelmingly agree: a join that lands on the
			// wrong call would show up as disagreement everywhere, not just at the
			// interface calls VTA is supposed to resolve better.
			if report.joined > 0 {
				agree := 100 * float64(report.agreed) / float64(report.joined)
				if agree < 50 {
					t.Errorf("only %.1f%% of joined sites name the same callee — the join is landing on the wrong calls", agree)
				}
			}
		})
	}
}

// joinAt loads one project both ways and joins their call graphs on position.
func joinAt(t *testing.T, dir string) joinReport {
	t.Helper()

	eng := engine.NewEngine(&engine.EngineConfig{InputDir: dir, ResolveCallGraph: true})
	meta, err := eng.GenerateMetadataOnly()
	if err != nil {
		t.Fatalf("metadata for %s: %v", dir, err)
	}
	resolved := eng.GetResolvedCallGraph()
	if resolved == nil {
		t.Fatalf("no resolved graph for %s despite ResolveCallGraph", dir)
	}

	byPosition := resolved.CalleesAt()
	facts := spec.TypeFactsFor(meta)

	report := joinReport{
		classes:  map[callgraph.Disagreement]int{},
		examples: map[callgraph.Disagreement]string{},
	}
	for i := range meta.CallGraph {
		edge := &meta.CallGraph[i]
		position := meta.StringPool.GetString(edge.Position)
		if position == "" {
			continue
		}
		report.metaEdges++

		targets, ok := byPosition[callgraph.SiteKey(position, edge.Callee.BaseID())]
		if !ok {
			continue
		}
		report.joined++

		if containsID(targets, edge.Callee.BaseID()) {
			report.agreed++
			continue
		}
		report.resolved++
		class := callgraph.Classify(edge.Callee.BaseID(), targets, facts)
		report.classes[class]++
		if _, seen := report.examples[class]; !seen {
			report.examples[class] = edge.Callee.BaseID() + "  ->  " + strings.Join(targets, ", ")
		}
	}
	return report
}

func containsID(targets []string, want string) bool {
	for _, target := range targets {
		if target == want {
			return true
		}
	}
	return false
}

// TestCondenseGraphIsGraphAgnostic pins that the condensation can be run over a
// graph metadata does not own — the resolved call graph is the one that matters,
// and the algorithm never needed anything from Metadata.
func TestCondenseGraphIsGraphAgnostic(t *testing.T) {
	// a -> b -> c -> b (a recursion cluster), and a -> d standing alone.
	nodes := []string{"a", "b", "c", "d"}
	adj := map[string][]string{
		"a": {"b", "d"},
		"b": {"c"},
		"c": {"b"},
	}

	scc := metadata.CondenseGraph(nodes, adj)

	if !scc.SameComponent("b", "c") {
		t.Error("b and c call each other and were not condensed into one component")
	}
	if scc.SameComponent("a", "b") {
		t.Error("a calls b but is not called back; they are not one component")
	}
	if !scc.InCycle("b") {
		t.Error("b is in a recursion cluster and was not flagged")
	}
	if scc.InCycle("d") {
		t.Error("d calls nothing and was flagged as recursive")
	}

	// Callees before callers: whatever component holds a must come after the one
	// holding b, since that ordering is what summaries will depend on.
	if scc.ComponentOf["a"] <= scc.ComponentOf["b"] {
		t.Errorf("components are not in callees-first order: a=%d b=%d",
			scc.ComponentOf["a"], scc.ComponentOf["b"])
	}
}
