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

package engine

import (
	"strings"
	"testing"

	intspec "github.com/ehabterra/apispec/internal/spec"
)

func TestParseStrictCategories(t *testing.T) {
	all := StrictCategories()

	cases := []struct {
		name    string
		value   string
		want    []StrictCategory
		wantErr string
	}{
		// The bare flag has to be the complete gate: a category added in a later
		// release must be enforced on a run that already asked for strictness,
		// not silently skipped because the flag was written before it existed.
		{name: "bare flag is every category", value: "", want: all},
		{name: "all is every category", value: "all", want: all},
		{name: "one category", value: "security", want: []StrictCategory{StrictSecurity}},
		{
			name:  "reported in the fixed order, not the typed one",
			value: "packages,security",
			want:  []StrictCategory{StrictSecurity, StrictPackages},
		},
		{name: "spaces and case are tolerated", value: " Security , Paths ", want: []StrictCategory{StrictSecurity, StrictPaths}},
		{name: "unknown category names the known ones", value: "securty", wantErr: "unknown --strict category"},
		{name: "a list of nothing is a mistake, not everything", value: ",,", wantErr: "no category"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseStrictCategories(tc.value)
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("ParseStrictCategories(%q) = %v, want error %q", tc.value, got, tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error %q does not mention %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseStrictCategories(%q): %v", tc.value, err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("got %v, want %v", got, tc.want)
				}
			}
		})
	}
}

// TestStrictFindingsPerCategory pins which recorded state produces which
// finding. The point of --strict is that a project can fail on lost auth while
// tolerating a truncated route, so a finding leaking into a category the user
// did not ask for is the defect to guard against.
func TestStrictFindingsPerCategory(t *testing.T) {
	cases := []struct {
		name     string
		engine   *Engine
		category StrictCategory
		want     string
		// otherCategories asserts the same state produces NOTHING elsewhere.
		quietElsewhere bool
	}{
		{
			name: "unmapped auth middleware",
			engine: &Engine{unresolvedSecurity: []intspec.MiddlewareRef{
				{FunctionName: "RequireAuth", Pkg: "app/mw"},
			}},
			category:       StrictSecurity,
			want:           "documented as public",
			quietElsewhere: true,
		},
		{
			name: "registrations whose path is built at runtime",
			engine: &Engine{
				unresolvedPaths: []intspec.UnresolvedPathRoute{{}, {}},
				// Zero paths as well — the runtime-path finding is the specific
				// diagnosis and must win, exactly as reportNoRoutes orders them.
				routeDiscovery: RouteDiscovery{CallEdges: 12},
			},
			category:       StrictPaths,
			want:           "2 registration(s) build their path at runtime",
			quietElsewhere: true,
		},
		{
			name:           "nothing matched at all",
			engine:         &Engine{routeDiscovery: RouteDiscovery{CallEdges: 12, Packages: 3}},
			category:       StrictPaths,
			want:           "0 paths documented from 12 call edge(s) across 3 package(s)",
			quietElsewhere: true,
		},
		{
			name: "a response lost its schema",
			engine: &Engine{thinOperations: []intspec.ThinOperation{
				{Method: "GET", Path: "/items", Status: "200"},
			}},
			category:       StrictSchemas,
			want:           "lost a response schema",
			quietElsewhere: true,
		},
		{
			name: "a reference had no component",
			engine: &Engine{unresolvedRefs: []intspec.UnresolvedRef{
				{GoType: "uuid.UUID", Component: "uuid_UUID", Sites: 4},
			}},
			category:       StrictSchemas,
			want:           "1 type(s) had no schema",
			quietElsewhere: true,
		},
		{
			name:           "the global node budget stopped expansion",
			engine:         &Engine{expansionStats: intspec.ExpansionStats{Truncated: true, Limit: 50000}},
			category:       StrictTruncation,
			want:           "node budget (50000)",
			quietElsewhere: true,
		},
		{
			name: "a per-route budget cut operations short",
			engine: &Engine{truncatedOperations: []intspec.TruncatedOperation{
				{Method: "GET", Path: "/a", Limit: intspec.TruncatedByRouteBudget},
				{Method: "GET", Path: "/b", Limit: intspec.TruncatedByRouteBudget},
			}},
			category:       StrictTruncation,
			want:           "2 operation(s) were cut short by the " + intspec.TruncatedByRouteBudget,
			quietElsewhere: true,
		},
		{
			name:           "packages that did not load",
			engine:         &Engine{skipped: []SkippedPackage{{Package: "app/db", Kind: skipParse}}},
			category:       StrictPackages,
			want:           "1 in-module package(s) were not analysed",
			quietElsewhere: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.engine.StrictFindings([]StrictCategory{tc.category})
			if len(got) == 0 {
				t.Fatalf("no finding for %s", tc.category)
			}
			joined := renderFindings(got)
			if !strings.Contains(joined, tc.want) {
				t.Errorf("finding does not say %q\ngot: %s", tc.want, joined)
			}
			for _, f := range got {
				if f.Category != tc.category {
					t.Errorf("finding in category %s, want %s: %s", f.Category, tc.category, f)
				}
				if f.Count == 0 {
					t.Errorf("finding with a zero count: %s", f)
				}
			}

			if !tc.quietElsewhere {
				return
			}
			for _, other := range StrictCategories() {
				if other == tc.category {
					continue
				}
				if extra := tc.engine.StrictFindings([]StrictCategory{other}); len(extra) > 0 {
					t.Errorf("category %s also fired: %s", other, renderFindings(extra))
				}
			}
		})
	}
}

// TestStrictFindingsQuietOnACleanRun: the gate exists to be passed. A run that
// recorded nothing must produce nothing, whatever it is asked about — including
// a project that legitimately documents no paths because it has no call graph
// to walk (NothingMatched is deliberately false there).
func TestStrictFindingsQuietOnACleanRun(t *testing.T) {
	clean := &Engine{routeDiscovery: RouteDiscovery{Paths: 12, CallEdges: 400, Packages: 5}}
	if got := clean.StrictFindings(StrictCategories()); len(got) > 0 {
		t.Errorf("clean run produced findings: %s", renderFindings(got))
	}

	empty := &Engine{routeDiscovery: RouteDiscovery{}}
	if got := empty.StrictFindings(StrictCategories()); len(got) > 0 {
		t.Errorf("empty call graph produced findings: %s", renderFindings(got))
	}

	// No categories means no gate, even with something to report.
	dirty := &Engine{skipped: []SkippedPackage{{Package: "app/db", Kind: skipParse}}}
	if got := dirty.StrictFindings(nil); len(got) > 0 {
		t.Errorf("an empty category list gated on something: %s", renderFindings(got))
	}
}

// TestStrictFindingsOrderIsFixed guards golden rule #1 at the gate: the same
// recorded state has to produce the same report, in the same order, on every
// run — a CI failure message that reshuffles is a diff nobody can read.
func TestStrictFindingsOrderIsFixed(t *testing.T) {
	e := &Engine{
		skipped:             []SkippedPackage{{Package: "app/db", Kind: skipParse}},
		truncatedOperations: []intspec.TruncatedOperation{{Method: "GET", Path: "/a", Limit: intspec.TruncatedByRouteBudget}},
		unresolvedRefs:      []intspec.UnresolvedRef{{GoType: "uuid.UUID", Sites: 1}},
		unresolvedPaths:     []intspec.UnresolvedPathRoute{{}},
		unresolvedSecurity:  []intspec.MiddlewareRef{{FunctionName: "RequireAuth", Pkg: "app/mw"}},
	}

	want := []StrictCategory{
		StrictSecurity, StrictPaths, StrictSchemas, StrictTruncation, StrictPackages,
	}
	for run := 0; run < 5; run++ {
		got := e.StrictFindings(StrictCategories())
		if len(got) != len(want) {
			t.Fatalf("got %d findings, want %d: %s", len(got), len(want), renderFindings(got))
		}
		for i := range got {
			if got[i].Category != want[i] {
				t.Fatalf("finding %d is %s, want %s\n%s", i, got[i].Category, want[i], renderFindings(got))
			}
		}
	}
}

// TestNamedMiddlewareIsSortedAndCapped: the middleware list reaches a failure
// message, so it is sorted (the extractor's order is not a contract) and capped
// so a project with many of them still fails readably.
func TestNamedMiddlewareIsSortedAndCapped(t *testing.T) {
	var refs []intspec.MiddlewareRef
	for _, n := range []string{"zeta", "alpha", "mid"} {
		refs = append(refs, intspec.MiddlewareRef{FunctionName: n, Pkg: "app/mw"})
	}
	got := namedMiddleware(refs)
	if want := "app/mw.alpha, app/mw.mid, app/mw.zeta"; got != want {
		t.Errorf("namedMiddleware = %q, want %q", got, want)
	}

	refs = nil
	for i := 0; i < 14; i++ {
		refs = append(refs, intspec.MiddlewareRef{FunctionName: string(rune('a'+i)) + "Auth", Pkg: "app/mw"})
	}
	if got := namedMiddleware(refs); !strings.Contains(got, "and 4 more") {
		t.Errorf("namedMiddleware did not cap the list: %q", got)
	}
}

func renderFindings(findings []StrictFinding) string {
	var b strings.Builder
	for i, f := range findings {
		if i > 0 {
			b.WriteString("; ")
		}
		b.WriteString(f.String())
	}
	return b.String()
}
