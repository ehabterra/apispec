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
	"strings"
	"testing"
)

// TestAnalysisLine covers the export's one-line account of how the spec was
// produced, including the case that must stay silent: nothing supplied. The
// report cannot derive any of this, so inventing a line would be a claim about a
// run nobody described.
func TestAnalysisLine(t *testing.T) {
	tests := []struct {
		name string
		info AnalysisInfo
		want string
	}{
		{
			name: "nothing supplied",
			info: AnalysisInfo{},
			want: "",
		},
		{
			name: "one framework needs no primary",
			info: AnalysisInfo{Frameworks: []string{"chi"}, Primary: "chi", Engine: "lazy"},
			want: "Analysed with frameworks chi, lazy tracker.",
		},
		{
			name: "several frameworks name the primary",
			info: AnalysisInfo{Frameworks: []string{"mux", "gin"}, Primary: "mux", Engine: "lazy"},
			want: "Analysed with frameworks mux + gin (primary: mux), lazy tracker.",
		},
		{
			name: "a configured framework with no detection",
			info: AnalysisInfo{Primary: "echo", Engine: "eager"},
			want: "Analysed with framework echo, eager tracker.",
		},
		{
			name: "entry points are spelled out, including why most were skipped",
			info: AnalysisInfo{
				Frameworks:  []string{"mux"},
				Primary:     "mux",
				Engine:      "lazy",
				Entrypoints: &EntrypointInfo{Declared: 53, Rooted: 1, AlreadyReachable: 0, NoRoutes: 52},
			},
			want: "Analysed with frameworks mux, lazy tracker, 53 CLI entry point(s) declared, 1 rooted (0 already reachable, 52 register no route).",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rep := &OverviewReport{Analysis: tc.info}
			if got := rep.AnalysisLine(); got != tc.want {
				t.Errorf("AnalysisLine =\n  %q\nwant\n  %q", got, tc.want)
			}
		})
	}
}

// TestBodyLine covers the export's body-resolution line, whose whole job is to
// say which KIND of gap is in play — the fixes differ.
func TestBodyLine(t *testing.T) {
	if got := (&OverviewReport{}).BodyLine(); got != "" {
		t.Errorf("no responses = %q, want empty", got)
	}

	rep := &OverviewReport{StatusBodies: []StatusBody{
		{Status: "200", Total: 10, WithSchema: 6, FreeForm: 2, Empty: 1, Unresolved: 1},
		{Status: "204", Total: 3, Empty: 3}, // no body by definition
		{Status: "500", Total: 2, WithSchema: 2},
	}}
	got := rep.BodyLine()
	want := "Response bodies: 8 describe their fields, 2 free-form (a Go map), 1 found but unresolved, 1 missing where one is expected, 3 empty by design."
	if got != want {
		t.Errorf("BodyLine =\n  %q\nwant\n  %q", got, want)
	}
}

// TestExportCarriesAnalysisContext pins that the export an AI (or an issue
// report) receives states what was analysed, not only what went wrong: the same
// unresolved type has a different fix depending on the framework and on whether
// the body was found at all.
func TestExportCarriesAnalysisContext(t *testing.T) {
	rep := BuildOverview(sampleSpec(), nil)
	rep.Analysis = AnalysisInfo{
		Frameworks:  []string{"mux", "gin"},
		Primary:     "mux",
		Engine:      "lazy",
		Entrypoints: &EntrypointInfo{Declared: 53, Rooted: 1, NoRoutes: 52},
	}

	md := BuildExportMarkdown(rep, ExportOptions{})
	for _, want := range []string{
		"frameworks mux + gin (primary: mux)",
		"53 CLI entry point(s) declared, 1 rooted",
		"Response bodies:",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("export is missing %q:\n%s", want, md)
		}
	}

	// Redaction still applies to the context line — an export is meant to be
	// shareable.
	redacted := BuildExportMarkdown(rep, ExportOptions{Redact: true, ModulePath: "mux"})
	if strings.Contains(redacted, "frameworks mux") {
		t.Errorf("redaction skipped the analysis line:\n%s", redacted)
	}
}
