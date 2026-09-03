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

package main

import (
	"reflect"
	"testing"

	"github.com/ehabterra/apispec/internal/insight"
	"github.com/ehabterra/apispec/internal/spec"
)

// TestAnalysisInfo covers what the Insight view is told about the run. The one
// case that matters beyond plumbing: a project that declares no entry points
// reports none at all, rather than a block of zeros. "The gate found nothing" and
// "there was nothing for the gate to find" are different statements, and only the
// second is true of an ordinary web service.
func TestAnalysisInfo(t *testing.T) {
	t.Run("no entrypoints declared", func(t *testing.T) {
		got := analysisInfo("chi", []string{"chi"}, spec.EntrypointStats{}, spec.ExpansionStats{})
		want := insight.AnalysisInfo{Frameworks: []string{"chi"}, Primary: "chi", Engine: "lazy"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("analysisInfo = %+v, want %+v", got, want)
		}
	})

	t.Run("entrypoints declared", func(t *testing.T) {
		got := analysisInfo("mux", []string{"mux", "gin"},
			spec.EntrypointStats{Declared: 53, Rooted: 1, AlreadyReachable: 0, NoRoutes: 52}, spec.ExpansionStats{})
		if got.Entrypoints == nil {
			t.Fatal("entrypoints not reported")
		}
		want := insight.EntrypointInfo{Declared: 53, Rooted: 1, AlreadyReachable: 0, NoRoutes: 52}
		if *got.Entrypoints != want {
			t.Errorf("entrypoints = %+v, want %+v", *got.Entrypoints, want)
		}
	})

	t.Run("the tracker engine is named", func(t *testing.T) {
		// One engine since issue #425, and the report still names it: a reader
		// of an exported report should not have to know which release made it.
		if got := analysisInfo("chi", nil, spec.EntrypointStats{}, spec.ExpansionStats{}); got.Engine != "lazy" {
			t.Errorf("engine = %q, want lazy", got.Engine)
		}
	})
}

// TestBuildAPISpecConfigKeepsEverySection guards the other half of the config
// round-trip: the editor may offer a section, but the server builds the config it
// hands to the engine, and a section it forgets to copy is silently dropped —
// edits appear to save and change nothing. This failed for entrypoints, security
// patterns, handler methods and response context before it was fixed.
func TestBuildAPISpecConfigKeepsEverySection(t *testing.T) {
	req := &GenerateRequest{
		Framework: "chi",
		FrameworkConfig: &spec.FrameworkConfig{
			RoutePatterns:           []spec.RoutePattern{{CallRegex: "^Get$"}},
			RequestBodyPatterns:     []spec.RequestBodyPattern{{CallRegex: "^Decode$"}},
			ResponsePatterns:        []spec.ResponsePattern{{CallRegex: "^Encode$"}},
			ParamPatterns:           []spec.ParamPattern{{CallRegex: "^Param$", ParamIn: "path"}},
			MountPatterns:           []spec.MountPattern{{CallRegex: "^Mount$"}},
			SecurityPatterns:        []spec.SecurityPattern{{CallRegex: "^Use$", Scope: spec.SecurityScopeRouter}},
			EntrypointPatterns:      []spec.EntrypointPattern{{FieldRegex: "^Action$", RecvType: "example.com/cli.Command"}},
			HandlerInterfaceMethods: []string{"ServeHTTP"},
			RequestContext:          spec.RequestContextConfig{TypeRegexes: []string{`^net/http\.\*Request$`}},
			ResponseContext:         spec.ResponseContextConfig{WriterTypeRegexes: []string{`^net/http\.ResponseWriter$`}},
		},
	}

	cfg, _, err := buildAPISpecConfig(req, t.TempDir())
	if err != nil {
		t.Fatalf("buildAPISpecConfig: %v", err)
	}

	fc := cfg.Framework
	checks := []struct {
		section string
		got     int
	}{
		{"routePatterns", len(fc.RoutePatterns)},
		{"requestBodyPatterns", len(fc.RequestBodyPatterns)},
		{"responsePatterns", len(fc.ResponsePatterns)},
		{"paramPatterns", len(fc.ParamPatterns)},
		{"mountPatterns", len(fc.MountPatterns)},
		{"securityPatterns", len(fc.SecurityPatterns)},
		{"entrypointPatterns", len(fc.EntrypointPatterns)},
		{"handlerInterfaceMethods", len(fc.HandlerInterfaceMethods)},
		{"requestContext.typeRegexes", len(fc.RequestContext.TypeRegexes)},
		{"responseContext.writerTypeRegexes", len(fc.ResponseContext.WriterTypeRegexes)},
	}
	for _, c := range checks {
		if c.got != 1 {
			t.Errorf("%s: %d entries survived, want the 1 that was submitted", c.section, c.got)
		}
	}

	// The submitted values reach the engine as written, not merely in the right
	// count.
	if len(fc.EntrypointPatterns) == 1 && fc.EntrypointPatterns[0].FieldRegex != "^Action$" {
		t.Errorf("entrypoint fieldRegex = %q, want ^Action$", fc.EntrypointPatterns[0].FieldRegex)
	}
}

// TestTrackerLimitsResolved pins the rule that lets a client say nothing about
// limits and still get the engine's behaviour: a zero means "default", and a
// value is passed through. Without this the UI could only ever run at the
// defaults, which is not enough for a large project (issue #233).
func TestTrackerLimitsResolved(t *testing.T) {
	defaults := defaultTrackerLimits()

	if got := (TrackerLimits{}).resolved(); got != defaults {
		t.Errorf("an empty request resolved to %+v, want the defaults %+v", got, defaults)
	}

	// Every field is independent: raising one must not silently reset the rest.
	raised := TrackerLimits{MaxNodesPerTree: 2_000_000}.resolved()
	if raised.MaxNodesPerTree != 2_000_000 {
		t.Errorf("max nodes = %d, want the requested 2000000", raised.MaxNodesPerTree)
	}
	if raised.MaxNodesPerRoute != defaults.MaxNodesPerRoute ||
		raised.MaxChildrenPerNode != defaults.MaxChildrenPerNode ||
		raised.MaxRecursionDepth != defaults.MaxRecursionDepth ||
		raised.MaxArgsPerFunction != defaults.MaxArgsPerFunction ||
		raised.MaxNestedArgsDepth != defaults.MaxNestedArgsDepth ||
		raised.MaxInstancesPerKey != defaults.MaxInstancesPerKey {
		t.Errorf("raising one limit changed the others: %+v", raised)
	}

	// A negative is not a limit; it means the same as unset rather than "no
	// bound", which would turn a typo into an unbounded walk.
	if got := (TrackerLimits{MaxNodesPerTree: -5}).resolved(); got.MaxNodesPerTree != defaults.MaxNodesPerTree {
		t.Errorf("negative max nodes = %d, want the default", got.MaxNodesPerTree)
	}

	// The two node budgets are independent: raising a route's DETAIL allowance
	// must not touch the budget that FINDS routes, or the UI would reproduce the
	// coupling #264 removed.
	perRoute := TrackerLimits{MaxNodesPerRoute: 5_000_000}.resolved()
	if perRoute.MaxNodesPerRoute != 5_000_000 {
		t.Errorf("max nodes per route = %d, want the requested 5000000", perRoute.MaxNodesPerRoute)
	}
	if perRoute.MaxNodesPerTree != defaults.MaxNodesPerTree {
		t.Errorf("raising the per-route budget changed the discovery budget to %d", perRoute.MaxNodesPerTree)
	}

	all := TrackerLimits{
		MaxNodesPerTree:    1,
		MaxNodesPerRoute:   7,
		MaxChildrenPerNode: 2,
		MaxArgsPerFunction: 3,
		MaxNestedArgsDepth: 4,
		MaxRecursionDepth:  5,
		MaxInstancesPerKey: 6,
	}
	if got := all.resolved(); got != all {
		t.Errorf("fully specified limits were altered: %+v, want %+v", got, all)
	}
}

// TestAnalysisInfoReportsTruncation covers what the Insight view is told when
// expansion stopped early — and that it says nothing when the walk finished,
// since an absent block is what "complete" means.
func TestAnalysisInfoReportsTruncation(t *testing.T) {
	finished := analysisInfo("chi", []string{"chi"}, spec.EntrypointStats{}, spec.ExpansionStats{NodesBuilt: 120, Limit: 50000})
	if finished.Expansion != nil {
		t.Errorf("a completed walk reported expansion %+v, want nothing", finished.Expansion)
	}

	cut := analysisInfo("chi", []string{"chi"}, spec.EntrypointStats{},
		spec.ExpansionStats{NodesBuilt: 50000, Limit: 50000, Truncated: true})
	if cut.Expansion == nil {
		t.Fatal("a truncated walk reported nothing")
	}
	if !cut.Expansion.Truncated || cut.Expansion.Limit != 50000 || cut.Expansion.NodesBuilt != 50000 {
		t.Errorf("expansion = %+v, want the truncation and its budget", *cut.Expansion)
	}
}
