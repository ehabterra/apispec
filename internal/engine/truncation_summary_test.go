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

// The zero case is the one that matters most and is the one that used to be
// impossible to get: gitea drops 25,267,970 call copies and loses nothing, so a
// reader who sees only the count concludes the run is broken (issue #296).
func TestThinSummarySaysNothingWasLost(t *testing.T) {
	e := &Engine{}
	if got := e.thinSummary(); got != "no operation lost a response schema" {
		t.Errorf("got %q, want the explicit all-clear", got)
	}
}

func TestThinSummaryNamesTheOperationsThatLostASchema(t *testing.T) {
	e := &Engine{thinOperations: []intspec.ThinOperation{
		{Method: "GET", Path: "/widgets", Status: "200"},
		{Method: "POST", Path: "/widgets", Status: "201"},
	}}
	got := e.thinSummary()
	for _, want := range []string{"2 operation(s)", "GET /widgets (200)", "POST /widgets (201)"} {
		if !strings.Contains(got, want) {
			t.Errorf("summary %q does not mention %q", got, want)
		}
	}
}

// Capped, because the point is to stay readable: a run that starves hundreds of
// operations has a different problem, and burying the counts above it helps
// nobody. The full list is on the diagnostics for a CI gate to read.
func TestThinSummaryCapsALongList(t *testing.T) {
	var many []intspec.ThinOperation
	for i := 0; i < 25; i++ {
		many = append(many, intspec.ThinOperation{Method: "GET", Path: "/p", Status: "200"})
	}
	got := (&Engine{thinOperations: many}).thinSummary()
	if !strings.Contains(got, "and 15 more") {
		t.Errorf("summary %q does not cap the list", got)
	}
	if !strings.Contains(got, "25 operations") {
		t.Errorf("summary %q does not give the true total", got)
	}
}

// The per-route budget names endpoints when it can and says so when it cannot —
// for a router wrapper the matched node is shared, so a truncated scope may have
// no operation of its own (issue #503).
func TestTruncatedSummaryNamesOperationsAndFallsBack(t *testing.T) {
	e := &Engine{truncatedOperations: []intspec.TruncatedOperation{
		{Method: "GET", Path: "/a", Limit: intspec.TruncatedByRouteBudget},
		{Registration: "pkg.Router.Get@x.go:1:1", Limit: intspec.TruncatedByRouteBudget},
	}}
	got := e.truncatedSummary(intspec.TruncatedByRouteBudget, "unused")
	if !strings.Contains(got, "GET /a") {
		t.Errorf("summary %q does not name the attributed operation", got)
	}
	if !strings.Contains(got, "operation not attributed") {
		t.Errorf("summary %q does not admit the unattributed scope", got)
	}

	// With nothing of that limit recorded, the caller's own "first at" is used
	// rather than inventing an empty list.
	if got := e.truncatedSummary("some other limit", "x.go:9"); got != "first at x.go:9" {
		t.Errorf("got %q, want the fallback", got)
	}
	if got := (&Engine{}).truncatedSummary(intspec.TruncatedByRouteBudget, ""); got != "no operation could be attributed" {
		t.Errorf("got %q, want the no-fallback wording", got)
	}
}

func TestTruncationAccessorsReturnWhatWasRecorded(t *testing.T) {
	e := &Engine{
		thinOperations:      []intspec.ThinOperation{{Path: "/a"}},
		truncatedOperations: []intspec.TruncatedOperation{{Path: "/b"}},
	}
	if got := e.GetThinOperations(); len(got) != 1 || got[0].Path != "/a" {
		t.Errorf("GetThinOperations = %v", got)
	}
	if got := e.GetTruncatedOperations(); len(got) != 1 || got[0].Path != "/b" {
		t.Errorf("GetTruncatedOperations = %v", got)
	}
}
