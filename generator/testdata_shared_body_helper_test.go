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
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ehabterra/apispec/internal/engine"
)

// TestTestdata_SharedBodyHelper pins the FAILURE MODE of a request body that
// resolves through a shared generic decode helper (issue #269).
//
// The fixture puts several routes in one chi group closure, each handler a
// closure returned by a factory, each decoding through one generic
// `httpx.DecodeJSON[T]` in another package. That single `decoder.Decode(&data)`
// is the ONLY body call site in the program: what tells the routes apart is the
// type argument at each call, so every route's schema depends on binding that
// argument back to its own call site. One route's response converter reads a
// domain field that another route's handler writes — the shape that lets the
// producer relation reach a sibling's body (see the fixture doc comment).
//
// On a real service four endpoints came out documented with a SIBLING route's
// DTO — a confidently wrong schema, which is strictly worse than a missing one
// because nothing downstream can tell it is wrong. The rule this test encodes:
//
//	a truncated or starved route must be documented as LESS detailed,
//	never as DIFFERENTLY detailed.
//
// So a route is allowed to have no request body at all, and fails only when it
// claims a body belonging to a different route. The caps and budgets below are
// the levers that starve resolution; the assertion is identical under all of
// them.
//
// NOTE: this fixture does NOT reproduce #269 on its own — it is green here both
// before and after the provenance gate that fixes the defect, at every setting
// swept below. The mechanism is known (the producer relation reaching a sibling
// handler through a shared domain field) but a synthetic version of it has not
// been made to fire; on the affected service it fires with shipped defaults.
// The gate's actual evidence is that service (three endpoints corrected, no
// request body lost, rest of the spec byte-identical) and the unit tests in
// internal/spec/request_provenance_test.go. This file is fail-safe coverage and
// a change-detector: passing it is NOT evidence that the defect is fixed.
func TestTestdata_SharedBodyHelper(t *testing.T) {
	const fixture = "shared_body_helper"

	// want maps each route to the ONE DTO it may legitimately document.
	want := map[string]string{}
	for _, name := range []string{
		"alpha", "bravo", "charlie", "delta", "echo", "foxtrot",
	} {
		want["/api/"+name] = strings.ToUpper(name[:1]) + name[1:] + "Request"
	}

	// Each triple starves resolution differently: the instance cap bounds copies
	// of the shared helper, the node budget cuts expansion short. Both are ways
	// a route can lose its own binding, which is when substitution was observed.
	//
	// perRoute sweeps the OTHER direction, and it is the reason this fixture is
	// swept at all. #269 was found by expanding MORE, not less: #264's per-route
	// budget stopped the global limit applying below a registration, route
	// subtrees went deeper than any shipped version had reached, and four
	// endpoints flipped to a sibling's DTO. Depth is a starvation setting in
	// reverse — more of it gives a wrong candidate more chances to win — so the
	// same assertion has to hold with the budget raised far past the default.
	for _, tc := range []struct {
		instanceCap int
		maxNodes    int
		perRoute    int
	}{
		{0, 0, 0},         // defaults
		{1, 0, 0},         // harshest cap: one copy of the shared helper
		{5, 0, 0},         // fewer copies than routes
		{0, 20000, 0},     // budget above what the fixture needs
		{0, 400, 0},       // budget that truncates most routes
		{0, 150, 0},       // budget that truncates nearly everything
		{0, 0, 10},        // per-route budget that truncates every route
		{0, 0, 2000000},   // per-route budget far past the default: expand everything
		{1, 0, 2000000},   // deepest expansion with the copies capped to one
		{0, 150, 2000000}, // wiring truncated, route subtrees unbounded
	} {
		t.Run(fmt.Sprintf("cap=%d/nodes=%d/perRoute=%d", tc.instanceCap, tc.maxNodes, tc.perRoute), func(t *testing.T) {
			cfg := engine.DefaultEngineConfig()
			cfg.InputDir = filepath.Join("..", "testdata", fixture)
			if tc.instanceCap > 0 {
				cfg.MaxInstancesPerKey = tc.instanceCap
			}
			if tc.maxNodes > 0 {
				cfg.MaxNodesPerTree = tc.maxNodes
			}
			if tc.perRoute > 0 {
				cfg.MaxNodesPerRoute = tc.perRoute
			}

			out, err := engine.NewEngine(cfg).GenerateOpenAPI()
			if err != nil {
				t.Fatalf("GenerateOpenAPI: %v", err)
			}

			for path, wantDTO := range want {
				item, ok := out.Paths[path]
				if !ok {
					continue // a starved route may not be documented at all
				}
				op := opFor(item, "POST")
				if op == nil || op.RequestBody == nil {
					continue // no body is the acceptable degradation
				}
				mt, ok := op.RequestBody.Content["application/json"]
				if !ok || mt.Schema == nil || mt.Schema.Ref == "" {
					continue
				}
				if !strings.HasSuffix(mt.Schema.Ref, "_"+wantDTO) {
					t.Errorf("%s: documents %q, want the route's own %s — a starved route may lose its body, never borrow a sibling's",
						path, mt.Schema.Ref, wantDTO)
				}
			}
		})
	}
}
