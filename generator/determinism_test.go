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
	"gopkg.in/yaml.v3"
)

// TestGenerateDeterministic asserts that generating a spec twice over the
// same fixture yields byte-identical YAML. The fixtures cover the flips seen
// historically: fiber responses, generic response resolution, operationIds,
// and multi-package traversal order.
func TestGenerateDeterministic(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping determinism fixtures in -short mode")
	}
	for _, name := range []string{"fiber", "generic", "mux", "complex_chi_router"} {
		t.Run(name, func(t *testing.T) {
			dir := filepath.Join("..", "testdata", name)

			base := marshalSpec(t, dir)
			got := marshalSpec(t, dir)
			if string(base) != string(got) {
				t.Fatalf("spec for %s differs between two runs:\n%s", name, firstDiffLine(string(base), string(got)))
			}
		})
	}
}

// TestGenerateDeterministicUnderTruncation runs the same fixtures with the
// budgets forced low enough to truncate, because that is where determinism
// broke in the field (issue #340): the same command on a large project dropped
// different responses and header parameters between runs, while every fixture
// small enough for TestGenerateDeterministic never spent a budget at all.
//
// Truncation itself is asserted, so the test cannot quietly stop exercising the
// path it exists for.
func TestGenerateDeterministicUnderTruncation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping determinism fixtures in -short mode")
	}
	for _, name := range []string{"complex_chi_router", "group_closure_instances", "mixed_gin_mux"} {
		t.Run(name, func(t *testing.T) {
			base, truncated := marshalTruncatedSpec(t, name)
			if !truncated {
				t.Fatalf("%s did not truncate at the forced budgets, so this fixture no longer covers the truncation path", name)
			}
			for run := 1; run < 3; run++ {
				got, _ := marshalTruncatedSpec(t, name)
				if string(got) != string(base) {
					t.Fatalf("spec for %s differs between runs at a truncating budget:\n%s",
						name, firstDiffLine(string(base), string(got)))
				}
			}
		})
	}
}

// marshalTruncatedSpec generates the fixture with both expansion budgets forced
// to their harshest useful values, and reports whether either one truncated.
func marshalTruncatedSpec(t *testing.T, fixture string) ([]byte, bool) {
	t.Helper()
	cfg := engine.DefaultEngineConfig()
	cfg.InputDir = filepath.Join("..", "testdata", fixture)
	cfg.MaxInstancesPerKey = 1
	cfg.MaxNodesPerRoute = 200

	eng := engine.NewEngine(cfg)
	out, err := eng.GenerateOpenAPI()
	if err != nil {
		t.Fatalf("GenerateOpenAPI(%s): %v", fixture, err)
	}
	data, err := yaml.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	stats := eng.GetExpansionStats()
	return data, stats.InstanceTruncations > 0 || stats.RouteTruncations > 0
}

func marshalSpec(t *testing.T, dir string) []byte {
	t.Helper()
	out, err := NewGenerator(nil).GenerateFromDirectory(dir)
	if err != nil {
		t.Fatalf("GenerateFromDirectory(%s) failed: %v", dir, err)
	}
	data, err := yaml.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func firstDiffLine(a, b string) string {
	al, bl := strings.Split(a, "\n"), strings.Split(b, "\n")
	n := len(al)
	if len(bl) < n {
		n = len(bl)
	}
	for i := 0; i < n; i++ {
		if al[i] != bl[i] {
			return fmt.Sprintf("line %d:\n  run0: %s\n  run1: %s", i+1, al[i], bl[i])
		}
	}
	return fmt.Sprintf("length differs: %d vs %d lines", len(al), len(bl))
}
