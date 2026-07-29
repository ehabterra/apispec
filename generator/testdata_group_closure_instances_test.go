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
