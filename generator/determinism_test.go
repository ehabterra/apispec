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
