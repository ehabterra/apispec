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

package metadata

import (
	"go/ast"
	"testing"
)

// TestMultiValueAssignmentRecordsEveryName pins a gap that made ordinary Go
// invisible: `a, b := f()` has one right-hand side for two names, and pairing
// them by position recorded `a` and dropped `b`.
//
// Anything assigned from the second result of a call therefore had no recorded
// origin at all — `middlewares, handlerFunc := wrap(h)` is how gitea unwraps its
// handlers, and nothing could trace `handlerFunc` back to `h` (issue #235).
func TestMultiValueAssignmentRecordsEveryName(t *testing.T) {
	src := `package p

func wrap(h any) (int, any) { return 0, h }

func caller(h any) {
	middlewares, handlerFunc := wrap(h)
	_, _ = middlewares, handlerFunc

	single := wrap
	_ = single

	x, y := 1, 2 // one right-hand side per name: unchanged behaviour
	_, _ = x, y
}
`
	file, info, fset := sweepTypeCheck(t, src)
	meta := &Metadata{StringPool: NewStringPool()}

	var assignments []Assignment
	ast.Inspect(file, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok {
			return true
		}
		assignments = append(assignments, processAssignment(assign, file, info, "p", fset, nil, nil, meta)...)
		return true
	})

	byName := map[string][]Assignment{}
	for _, a := range assignments {
		name := meta.StringPool.GetString(a.VariableName)
		byName[name] = append(byName[name], a)
	}

	// Both names of the tuple assignment are recorded, and both point at the call
	// they came from — that is what lets a value be traced back to a parameter.
	for i, name := range []string{"middlewares", "handlerFunc"} {
		got := byName[name]
		if len(got) == 0 {
			t.Errorf("%q has no recorded assignment; a value from result %d of a call cannot be traced", name, i)
			continue
		}
		if got[0].CalleeFunc != "wrap" {
			t.Errorf("%q came from %q, want the call `wrap`", name, got[0].CalleeFunc)
		}
		// Which result it takes is what tells a consumer how to resolve its type.
		if got[0].ReturnIndex != i {
			t.Errorf("%q has ReturnIndex %d, want %d", name, got[0].ReturnIndex, i)
		}
	}

	// A name-per-value assignment is untouched: each name keeps its own value and
	// index 0.
	for _, name := range []string{"x", "y"} {
		got := byName[name]
		if len(got) == 0 {
			t.Fatalf("%q lost its assignment", name)
		}
		if got[0].ReturnIndex != 0 {
			t.Errorf("%q has ReturnIndex %d, want 0 — it is not a tuple", name, got[0].ReturnIndex)
		}
	}
}
