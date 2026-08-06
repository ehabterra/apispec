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
	intspec "github.com/ehabterra/apispec/internal/spec"
)

// TestTestdata_GroupClosureInstances pins WHAT THE INSTANCE SCOPE IS, which is
// the question issue #224 turns on.
//
// The cap bounds copies of a callee within one "instance scope" — the subtree of
// the nearest argument-node ancestor. #224 asserted that a chi group closure,
// being itself an argument node, becomes that scope for every route registered
// inside it, so a helper shared by the group exhausts the budget after N routes
// and every later route silently loses its response body.
//
// This fixture is that shape: 15 routes in one `r.Route("/api", func(api
// chi.Router){…})` closure, every handler a method on a shared struct, every
// response written through one shared responder. If the group closure were the
// scope, the single Encode call inside that responder would be counted 15 times
// against it and the run would starve well before 15.
//
// It does not. Every route keeps its schema down to a cap of ONE, because the
// handler argument node — not the group closure — is the nearest argument
// ancestor. The scope is already per route in this shape.
//
// The test is therefore a change-detector in both directions: it fails if the
// scope ever regresses to something coarser than the handler, and it is the
// evidence that #224's stated mechanism is not what starves a real service.
func TestTestdata_GroupClosureInstances(t *testing.T) {
	const (
		fixture = "group_closure_instances"
		routes  = 15
	)

	// One is the harshest cap that still permits a single copy of each callee.
	// If the scope spanned the group, this could not document 15 routes.
	for _, cap := range []int{1, 25} {
		t.Run(fmt.Sprintf("cap=%d", cap), func(t *testing.T) {
			cfg := engine.DefaultEngineConfig()
			cfg.InputDir = filepath.Join("..", "testdata", fixture)
			cfg.MaxInstancesPerKey = cap

			out, err := engine.NewEngine(cfg).GenerateOpenAPI()
			if err != nil {
				t.Fatalf("GenerateOpenAPI: %v", err)
			}
			if got := len(out.Paths); got != routes {
				t.Fatalf("documented %d paths, want %d", got, routes)
			}

			var missing []string
			for path, item := range out.Paths {
				op := opFor(item, "GET")
				if op == nil {
					t.Errorf("GET %s missing", path)
					continue
				}
				resp, ok := op.Responses["200"]
				if !ok {
					missing = append(missing, path+" (no 200)")
					continue
				}
				if !namesAComponent(resp.Content) {
					missing = append(missing, path)
				}
			}
			if len(missing) > 0 {
				t.Errorf("%d of %d routes lost their response schema at --max-instances-per-key %d: %v\n"+
					"every handler writes through ONE shared responder, so this is the instance cap starving "+
					"routes that share a scope — the scope must be the handler, not the group closure (#224)",
					len(missing), routes, cap, missing)
			}
		})
	}
}

// TestGroupClosureInstanceCapIsReported pins that the cap does not truncate in
// silence. Before #224 it was the one limit in the tree that dropped work
// without saying so, which is why a starved response body was indistinguishable
// from a type the mapper could not resolve.
func TestGroupClosureInstanceCapIsReported(t *testing.T) {
	cfg := engine.DefaultEngineConfig()
	cfg.InputDir = filepath.Join("..", "testdata", "group_closure_instances")
	cfg.MaxInstancesPerKey = 1

	eng := engine.NewEngine(cfg)
	if _, err := eng.GenerateOpenAPI(); err != nil {
		t.Fatalf("GenerateOpenAPI: %v", err)
	}

	stats := eng.GetExpansionStats()
	if stats.InstanceTruncations == 0 {
		t.Fatal("a cap of 1 dropped no copies at all; the fixture is not exercising the cap and the report is untested")
	}
	if stats.InstanceLimit != 1 {
		t.Errorf("reported instance limit %d, want the configured 1", stats.InstanceLimit)
	}
	if stats.InstanceFirstKey == "" {
		t.Error("no key named for the first dropped copy — the count alone cannot be acted on")
	}
	// The scope is the diagnostic that matters: it is what tells a bounded
	// diamond from one route eating another's budget.
	if stats.InstanceFirstScope == "" {
		t.Error("no scope named for the first dropped copy")
	}
	if strings.Contains(stats.InstanceFirstScope, "FuncLit:") {
		t.Errorf("first starved scope is a closure (%s) — in this fixture the scope must be a handler",
			stats.InstanceFirstScope)
	}
}

// TestGroupClosurePerRouteBudgetIsScopedPerRoute pins WHAT OPENS A PER-ROUTE
// BUDGET SCOPE, which is the question issue #264 turns on — the same question
// as #224 above, one limit further down.
//
// The reach predicate that answers "does this subtree lead to a route?" has to
// say yes for `r.Route("/api", …)`, or the entire group is written off. Using
// that same predicate to open a BUDGET scope makes the group the scope, so
// every route inside shares one allowance and the routes past the first few are
// never discovered. That took a real 246-route chi service to 32 paths, which is
// why the two questions now have two predicates: RouteRegistrationMatcher for
// reachability, TerminalRouteMatcher for scoping.
//
// This fixture is the shape that separates them: 15 routes in ONE group
// closure. The budget is driven far below what a single route needs — at 5 all
// 15 subtrees truncate — and the assertions are the two facts a per-route
// budget must have:
//
//   - every route is still DISCOVERED. Truncation costs a route its detail; it
//     must never cost a sibling its existence. A group-scoped budget fails here
//     immediately, since one exhausted allowance would swallow the rest.
//   - the run reports 15 scopes, not 1. The path count alone cannot tell a
//     correctly-scoped budget from one that was simply never reached, so the
//     scope count is asserted directly.
func TestGroupClosurePerRouteBudgetIsScopedPerRoute(t *testing.T) {
	const (
		fixture = "group_closure_instances"
		routes  = 15
	)

	// 5 is far below one route's needs (every subtree truncates); 20000 is the
	// shipped default and truncates nothing. The assertions hold at both ends.
	for _, budget := range []int{5, 50, 500, 20000} {
		t.Run(fmt.Sprintf("budget=%d", budget), func(t *testing.T) {
			cfg := engine.DefaultEngineConfig()
			cfg.InputDir = filepath.Join("..", "testdata", fixture)
			cfg.MaxNodesPerRoute = budget

			eng := engine.NewEngine(cfg)
			out, err := eng.GenerateOpenAPI()
			if err != nil {
				t.Fatalf("GenerateOpenAPI: %v", err)
			}

			if got := len(out.Paths); got != routes {
				t.Errorf("documented %d paths at --max-nodes-per-route %d, want all %d — "+
					"a per-route budget bounds a route's DETAIL; if routes disappear it is being "+
					"charged to a scope that spans them (the group closure, #264)",
					got, budget, routes)
			}

			stats := eng.GetExpansionStats()
			if stats.RoutesScoped != routes {
				t.Errorf("%d budget scopes for %d routes — one scope per terminal route is the "+
					"whole point; %d would mean the group opened it",
					stats.RoutesScoped, routes, 1)
			}
			if stats.RouteLimit != budget {
				t.Errorf("reported per-route limit %d, want the configured %d", stats.RouteLimit, budget)
			}
			// The harshest setting must actually exercise the limit, or the
			// assertions above prove nothing about truncation.
			if budget == 5 && stats.RouteTruncations == 0 {
				t.Error("a budget of 5 truncated nothing; the fixture is not exercising the per-route limit")
			}
			if budget == 20000 && stats.RouteTruncations != 0 {
				t.Errorf("the shipped default truncated %d of this fixture's route subtrees — "+
					"a default that starves a 15-route fixture starves every real project",
					stats.RouteTruncations)
			}
		})
	}
}

// namesAComponent reports whether a response body resolves to a component,
// directly or as an array's element type.
func namesAComponent(content map[string]intspec.MediaType) bool {
	for _, mt := range content {
		if mt.Schema == nil {
			continue
		}
		if mt.Schema.Ref != "" {
			return true
		}
		if mt.Schema.Items != nil && mt.Schema.Items.Ref != "" {
			return true
		}
	}
	return false
}
