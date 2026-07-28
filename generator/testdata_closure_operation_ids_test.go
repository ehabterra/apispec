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
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// TestTestdata_ClosureOperationIDsAreReproducible pins the property that makes a
// generated spec reviewable: the same source must produce the same spec wherever
// it is checked out.
//
// A closure has no declared name, so apispec identifies it by where it is written,
// and that identity becomes the route's operationId. While the position carried
// the absolute path, every machine produced a different spec — 116 of
// photoprism's 131 operations were named after someone's home directory (issue
// #216).
//
// The fixture is generated twice from two different directories, because that is
// the only way to state the property: comparing strings against expected values
// would pass on the machine that wrote them.
func TestTestdata_ClosureOperationIDsAreReproducible(t *testing.T) {
	const fixture = "closure_operation_ids"
	src := filepath.Join("..", "testdata", fixture)

	original := operationIDsFrom(t, src)
	moved := operationIDsFrom(t, copyFixture(t, src, nil))

	if !reflect.DeepEqual(original, moved) {
		t.Errorf("the same source produced different operationIds in two checkouts:\n  %v\n  %v", original, moved)
	}

	// Both closures are documented, and they stay distinct: the position is what
	// separates two anonymous handlers in one file, so dropping too much of it
	// would collapse them.
	if len(original) != 2 {
		t.Fatalf("expected 2 closure operations, got %d: %v", len(original), original)
	}
	if original[0] == original[1] {
		t.Errorf("two closures share one operationId (%q) — they would collide in the spec", original[0])
	}

	for _, id := range original {
		if !strings.Contains(id, "FuncLit:") {
			t.Errorf("operationId %q does not identify a closure; the fixture stopped covering the case", id)
		}
		// The failure mode, stated directly: no operationId may name a location on
		// the machine that generated it.
		if path := closureIDPath(id); filepath.IsAbs(path) {
			t.Errorf("operationId %q carries the absolute path %q, so the spec differs per machine", id, path)
		}
	}
}

// operationIDsFrom generates the spec for a directory and returns its
// operationIds, sorted.
func operationIDsFrom(t *testing.T, dir string) []string {
	t.Helper()
	out, err := NewGenerator(nil).GenerateFromDirectory(dir)
	if err != nil {
		t.Fatalf("GenerateFromDirectory(%s): %v", dir, err)
	}
	var ids []string
	for _, item := range out.Paths {
		for _, method := range []string{"GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS", "HEAD"} {
			if op := opFor(item, method); op != nil && op.OperationID != "" {
				ids = append(ids, op.OperationID)
			}
		}
	}
	sort.Strings(ids)
	return ids
}

// closureIDPath returns the file part of a closure operationId
// (`pkg.FuncLit:<file>:<line>:<col>`).
func closureIDPath(id string) string {
	idx := strings.Index(id, "FuncLit:")
	if idx < 0 {
		return ""
	}
	rest := id[idx+len("FuncLit:"):]
	// Drop the trailing `:line:col`, leaving the path.
	for i := 0; i < 2; i++ {
		if last := strings.LastIndex(rest, ":"); last >= 0 {
			rest = rest[:last]
		}
	}
	return rest
}
