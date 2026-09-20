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
	"fmt"
	"sort"
	"strings"

	intspec "github.com/ehabterra/apispec/internal/spec"
)

// StrictCategory names one class of quality shortfall a run can be failed on.
//
// Every category below is something the engine ALREADY detects and already
// prints. What was missing is a consequence: a warning on stderr scrolls past
// in a CI log, and its effect is invisible in the spec diff to anyone not
// already looking for it — an operation whose `security` block was dropped
// reads exactly like an operation that genuinely has none (issue #297).
//
// The split into categories is not cosmetic. These shortfalls differ in how
// tolerable they are: a project may reasonably accept a truncated route while
// refusing to let an endpoint lose its documented authentication, so failing on
// one must not force failing on the other.
type StrictCategory string

const (
	// StrictSecurity: auth middleware matched no SecurityMapping, so the
	// operations behind it are documented as PUBLIC. Only middleware that shows
	// an auth signal counts — the unclassified rest is deliberately excluded,
	// because a gate that fires on every logger is a gate people switch off
	// (issue #520).
	StrictSecurity StrictCategory = "security"

	// StrictPaths: registrations that produced no documented operation at all —
	// a path built at runtime, or a run where nothing matched.
	StrictPaths StrictCategory = "paths"

	// StrictTruncation: an expansion budget stopped the walk, so an operation is
	// present and reads as finished while carrying less than the code supports.
	StrictTruncation StrictCategory = "truncation"

	// StrictSchemas: the document has an operation but not its shape — a
	// response with an empty schema, or a reference repaired with a placeholder.
	StrictSchemas StrictCategory = "schemas"

	// StrictPackages: in-module packages that did not load or parse, so whatever
	// they registered is absent and there is no way to know how much.
	StrictPackages StrictCategory = "packages"
)

// strictCategoryOrder is the order findings are reported in, and the order
// --strict without a value enables. Fixed rather than derived from a map, so
// the output is deterministic (golden rule #1), and severity-first: an endpoint
// documented as public is worse than one documented in less detail.
var strictCategoryOrder = []StrictCategory{
	StrictSecurity,
	StrictPaths,
	StrictSchemas,
	StrictTruncation,
	StrictPackages,
}

// StrictCategories returns every category, in report order.
func StrictCategories() []StrictCategory {
	out := make([]StrictCategory, len(strictCategoryOrder))
	copy(out, strictCategoryOrder)
	return out
}

// StrictCategoryNames returns every category name, for help text and error
// messages.
func StrictCategoryNames() []string {
	out := make([]string, 0, len(strictCategoryOrder))
	for _, c := range strictCategoryOrder {
		out = append(out, string(c))
	}
	return out
}

// ParseStrictCategories reads the value of --strict.
//
// Empty and "all" both mean every category: --strict on its own is the common
// case and has to be the complete gate, or a category added later would
// silently not be enforced on a run that asked for strictness.
func ParseStrictCategories(value string) ([]StrictCategory, error) {
	value = strings.TrimSpace(value)
	if value == "" || value == "all" {
		return StrictCategories(), nil
	}

	valid := make(map[StrictCategory]bool, len(strictCategoryOrder))
	for _, c := range strictCategoryOrder {
		valid[c] = true
	}

	selected := make(map[StrictCategory]bool, len(strictCategoryOrder))
	for _, part := range strings.Split(value, ",") {
		name := StrictCategory(strings.ToLower(strings.TrimSpace(part)))
		if name == "" {
			continue
		}
		if !valid[name] {
			return nil, fmt.Errorf("unknown --strict category %q (known: %s)",
				string(name), strings.Join(StrictCategoryNames(), ", "))
		}
		selected[name] = true
	}
	if len(selected) == 0 {
		return nil, fmt.Errorf("--strict was given no category (known: %s)",
			strings.Join(StrictCategoryNames(), ", "))
	}

	// Emitted in the fixed order rather than the order they were typed, so two
	// spellings of the same gate report identically.
	out := make([]StrictCategory, 0, len(selected))
	for _, c := range strictCategoryOrder {
		if selected[c] {
			out = append(out, c)
		}
	}
	return out, nil
}

// StrictFinding is one shortfall, already reported in full on stderr. Count is
// what a ratcheting CI check can compare across runs; Detail is the one-line
// reason, which repeats the stderr report deliberately — the gate's own output
// has to stand on its own when the log above it is folded away.
type StrictFinding struct {
	Category StrictCategory
	Count    int
	Detail   string
}

func (f StrictFinding) String() string {
	return fmt.Sprintf("[%s] %s", f.Category, f.Detail)
}

// StrictFindings returns the shortfalls from the most recent generation that
// fall into the given categories, in report order.
//
// It reads only state the engine has already recorded, so it costs nothing and
// changes nothing: --strict decides an exit code, never a document. A run with
// --strict and one without produce byte-identical specs, which is what lets a
// project turn the gate on without re-reviewing its output.
func (e *Engine) StrictFindings(categories []StrictCategory) []StrictFinding {
	if e == nil || len(categories) == 0 {
		return nil
	}
	want := make(map[StrictCategory]bool, len(categories))
	for _, c := range categories {
		want[c] = true
	}

	var findings []StrictFinding
	add := func(c StrictCategory, count int, format string, args ...interface{}) {
		if !want[c] || count == 0 {
			return
		}
		findings = append(findings, StrictFinding{
			Category: c,
			Count:    count,
			Detail:   fmt.Sprintf(format, args...),
		})
	}

	// Fires whether or not any SecurityMapping is configured, which is where it
	// parts company with the stderr warning: that one stays quiet when there are
	// no mappings at all, because auth detection is then effectively off and the
	// line would be noise on every run. A gate is not noise — it was asked for —
	// and a project with auth middleware and no mappings is the WORST case of
	// this finding, not an exempt one: every guarded endpoint is public in the
	// document.
	add(StrictSecurity, len(e.unresolvedSecurity),
		"%d auth middleware matched no security scheme, so the operations behind %s are documented as public: %s",
		len(e.unresolvedSecurity), plural(len(e.unresolvedSecurity), "it", "them"),
		namedMiddleware(e.unresolvedSecurity))

	// Two shapes of "the endpoint is missing entirely", reported apart because
	// the remedies are different and a run can only be in one of them: nothing
	// matched at all, or registrations matched and lost their path. Ordered the
	// way reportNoRoutes orders them, for the same reason (issue #428).
	if len(e.unresolvedPaths) > 0 {
		add(StrictPaths, len(e.unresolvedPaths),
			"%d registration(s) build their path at runtime and are not documented",
			len(e.unresolvedPaths))
	} else if e.routeDiscovery.NothingMatched() {
		add(StrictPaths, 1,
			"0 paths documented from %d call edge(s) across %d package(s) — no route registration matched",
			e.routeDiscovery.CallEdges, e.routeDiscovery.Packages)
	}

	// Thin responses are the measured cost of the instance cap, which is the
	// number issue #296 asked for: the cap's own truncation count runs to
	// millions on a large project and is almost always harmless, so it cannot be
	// gated on directly.
	add(StrictSchemas, len(e.thinOperations), "%s", e.thinSummary())
	add(StrictSchemas, len(e.unresolvedRefs),
		"%d type(s) had no schema and were inlined as unresolved", len(e.unresolvedRefs))

	// The global budget and the per-route budget are one category because the
	// consequence is one thing — an operation that reads as finished and is not
	// — but reported apart, because only the per-route one names endpoints and
	// only the global one means the shortfall is unbounded.
	if want[StrictTruncation] && e.expansionStats.Truncated {
		add(StrictTruncation, 1,
			"tree expansion stopped at the node budget (%d) — the spec is incomplete by an unknown amount, raise --max-nodes",
			e.expansionStats.Limit)
	}
	// One finding per budget rather than one total: the flag to reach for is the
	// budget's own, so a count that merged two of them would name neither.
	for _, lim := range truncationLimits(e.truncatedOperations) {
		n := 0
		for _, op := range e.truncatedOperations {
			if op.Limit == lim {
				n++
			}
		}
		add(StrictTruncation, n, "%d operation(s) were cut short by the %s: %s",
			n, lim, e.truncatedSummary(lim, ""))
	}

	add(StrictPackages, len(e.skipped),
		"%d in-module package(s) were not analysed, so the spec is incomplete", len(e.skipped))

	return findings
}

// truncationLimits names the budgets that fired, in a fixed order.
func truncationLimits(ops []intspec.TruncatedOperation) []string {
	seen := map[string]bool{}
	var out []string
	for _, op := range ops {
		if !seen[op.Limit] {
			seen[op.Limit] = true
			out = append(out, op.Limit)
		}
	}
	sort.Strings(out)
	return out
}

// namedMiddleware renders middleware refs for a one-line report, sorted so the
// message is stable across runs (golden rule #1) and capped so a project with
// many of them still produces a readable failure.
func namedMiddleware(refs []intspec.MiddlewareRef) string {
	const maxNamed = 10
	names := make([]string, 0, len(refs))
	for _, ref := range refs {
		names = append(names, ref.String())
	}
	sort.Strings(names)
	if len(names) > maxNamed {
		return strings.Join(names[:maxNamed], ", ") +
			fmt.Sprintf(", and %d more", len(names)-maxNamed)
	}
	return strings.Join(names, ", ")
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
