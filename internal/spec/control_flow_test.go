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

package spec

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"sort"
	"testing"

	meta "github.com/ehabterra/apispec/internal/metadata"
)

// armAt builds a block standing for a region spanning whole lines, which is
// what every case here needs except the column test.
func armAt(startLine, endLine int) meta.Block {
	return meta.Block{
		Kind:      meta.BlockIf,
		StartLine: startLine, StartCol: 1,
		EndLine: endLine, EndCol: 80,
		Group: 1,
	}
}

func at(line, col int) codePos { return codePos{file: "h.go", line: line, col: col} }

func indexOf(blocks ...meta.Block) *blockIndex {
	return &blockIndex{byFile: map[string][]meta.Block{"h.go": blocks}}
}

func TestDominates(t *testing.T) {
	cases := []struct {
		name    string
		idx     *blockIndex
		earlier codePos
		later   codePos
		want    bool
	}{
		{
			// The guard clause: the status is inside an arm the body escaped,
			// so reaching the body means the status did not run.
			name:    "status inside an arm the body escapes",
			idx:     indexOf(armAt(10, 13)),
			earlier: at(11, 3),
			later:   at(15, 2),
			want:    false,
		},
		{
			name:    "both inside the same arm",
			idx:     indexOf(armAt(10, 13)),
			earlier: at(11, 3),
			later:   at(12, 3),
			want:    true,
		},
		{
			// Nesting is not escaping: the body is deeper in, but every path to
			// it still passed the status write.
			name:    "body nested deeper inside the same arm",
			idx:     indexOf(armAt(10, 16), armAt(12, 14)),
			earlier: at(11, 3),
			later:   at(13, 4),
			want:    true,
		},
		{
			// The rule is about the STATUS escaping, not the body: a status at
			// top level dominates a body inside a branch below it.
			name:    "status above the branch, body inside it",
			idx:     indexOf(armAt(12, 15)),
			earlier: at(10, 2),
			later:   at(13, 3),
			want:    true,
		},
		{
			name:    "neither inside any region",
			idx:     indexOf(armAt(20, 25)),
			earlier: at(10, 2),
			later:   at(11, 2),
			want:    true,
		},
		{
			name:    "written after the body",
			idx:     indexOf(),
			earlier: at(14, 2),
			later:   at(11, 2),
			want:    false,
		},
		{
			// Same line: only the column separates them.
			name:    "same line, earlier column first",
			idx:     indexOf(),
			earlier: at(11, 2),
			later:   at(11, 30),
			want:    true,
		},
		{
			name:    "same line, earlier column after",
			idx:     indexOf(),
			earlier: at(11, 30),
			later:   at(11, 2),
			want:    false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.idx.dominates(tc.earlier, tc.later); got != tc.want {
				t.Errorf("dominates(%v, %v) = %v, want %v", tc.earlier, tc.later, got, tc.want)
			}
		})
	}
}

// TestDominatesIsPermissiveWithoutFacts pins the direction of the conservatism:
// where the facts are missing the answer is "dominates", so a pairing that
// happened before still happens. A missing fact must never silently change a
// spec (golden rule #7).
func TestDominatesIsPermissiveWithoutFacts(t *testing.T) {
	idx := indexOf(armAt(10, 13))
	cases := []struct {
		name           string
		earlier, later codePos
	}{
		{"no position for the status", codePos{}, at(15, 2)},
		{"no position for the body", at(11, 3), codePos{}},
		{"unknown file", codePos{file: "other.go", line: 11, col: 3}, codePos{file: "other.go", line: 15, col: 2}},
		{"positions in different files", codePos{file: "a.go", line: 11, col: 3}, codePos{file: "b.go", line: 2, col: 2}},
		{"line zero", codePos{file: "h.go", line: 0, col: 3}, at(15, 2)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if !idx.dominates(tc.earlier, tc.later) {
				t.Errorf("dominates(%v, %v) = false; missing facts must keep the previous pairing",
					tc.earlier, tc.later)
			}
		})
	}
	var nilIdx *blockIndex
	if !nilIdx.dominates(at(11, 3), at(15, 2)) {
		t.Error("a nil index must answer permissively")
	}
}

// TestDominatesColumnsSeparateAOneLineArm is the case line-only ranges get
// wrong: the init call and the arm share every line, and only the column says
// the call is outside.
func TestDominatesColumnsSeparateAOneLineArm(t *testing.T) {
	// if err := before(); err != nil { inside(); return }
	// col:   9 (before)              33 (arm start)  35 (inside)   50 (arm end)
	arm := meta.Block{
		Kind:      meta.BlockIf,
		StartLine: 7, StartCol: 33,
		EndLine: 7, EndCol: 50,
		Group: 1,
	}
	idx := indexOf(arm)

	// A status written in the init dominates a body below the statement: it is
	// not inside the arm at all.
	if !idx.dominates(at(7, 9), at(9, 2)) {
		t.Error("a call in the if's init must not be treated as inside the arm")
	}
	// A status written inside the one-line arm does not dominate a body below.
	if idx.dominates(at(7, 35), at(9, 2)) {
		t.Error("a call inside the one-line arm must not dominate what follows the statement")
	}
}

// TestNewBlockIndexFromMetadata builds the index the way a run does — from
// generated metadata — and asks it the guard-clause question. It covers the
// step the hand-built indexes above skip: bucketing each function's blocks
// under the FILE its position names, which is what lets a position be resolved
// without knowing which declaration (or which function literal) it sits in.
// TestExclusive covers the question that decides whether one status write can
// be carried by two bodies: can both of them run?
func TestExclusive(t *testing.T) {
	// if (10..13, returns) / else (14..17), and code after at line 20.
	ifArm := meta.Block{Kind: meta.BlockIf, StartLine: 10, StartCol: 1, EndLine: 13, EndCol: 80, Group: 1, Terminates: true}
	elseArm := meta.Block{Kind: meta.BlockElse, StartLine: 14, StartCol: 1, EndLine: 17, EndCol: 80, Group: 1}
	loop := meta.Block{Kind: meta.BlockLoop, StartLine: 30, StartCol: 1, EndLine: 33, EndCol: 80}
	idx := indexOf(ifArm, elseArm, loop)

	cases := []struct {
		name string
		a, b codePos
		want bool
	}{
		{"sibling arms of one conditional", at(11, 3), at(15, 3), true},
		{"the same arm", at(11, 3), at(12, 3), false},
		{"an arm that returns, and code after the conditional", at(11, 3), at(20, 2), true},
		{"code after the conditional, and the arm", at(20, 2), at(11, 3), false},
		{"a loop body carries no group and exits nothing", at(31, 3), at(35, 2), false},
		{"neither is in a region", at(20, 2), at(21, 2), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := idx.exclusive(tc.a, tc.b); got != tc.want {
				t.Errorf("exclusive(%v, %v) = %v, want %v", tc.a, tc.b, got, tc.want)
			}
		})
	}

	// An arm that does NOT exit leaves the code after it reachable on the same
	// run, so they are not alternatives — the case a terminator fact is needed
	// to tell apart, and the one a guess would get wrong.
	falls := meta.Block{Kind: meta.BlockIf, StartLine: 10, StartCol: 1, EndLine: 13, EndCol: 80, Group: 1}
	if indexOf(falls).exclusive(at(11, 3), at(20, 2)) {
		t.Error("an arm that falls through must not make what follows an alternative")
	}

	// A switch case that ends in `break` leaves the switch and lands on the
	// statement after it, so the two are not alternatives. This is why only
	// `return` and `panic` set Terminates — see terminates() in metadata.
	caseArm := meta.Block{Kind: meta.BlockCase, StartLine: 40, StartCol: 1, EndLine: 43, EndCol: 80, Group: 2}
	if indexOf(caseArm).exclusive(at(41, 3), at(50, 2)) {
		t.Error("a case that breaks must not make the code after the switch an alternative")
	}

	// Missing facts answer false: nothing is an alternative on a guess.
	if idx.exclusive(codePos{}, at(15, 3)) || idx.exclusive(at(11, 3), codePos{}) {
		t.Error("unknown positions must not be reported as exclusive")
	}
	var nilIdx *blockIndex
	if nilIdx.exclusive(at(11, 3), at(15, 3)) {
		t.Error("a nil index must not report exclusivity")
	}
}

func TestNewBlockIndexFromMetadata(t *testing.T) {
	src := `package main

func guard(bad bool) {
	if bad {
		fail()
		return
	}
	succeed()
}

func fail()    {}
func succeed() {}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "guard.go", src, 0)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	info := &types.Info{
		Types: map[ast.Expr]types.TypeAndValue{},
		Defs:  map[*ast.Ident]types.Object{},
		Uses:  map[*ast.Ident]types.Object{},
	}
	if _, err := (&types.Config{}).Check("main", fset, []*ast.File{file}, info); err != nil {
		t.Fatalf("typecheck: %v", err)
	}
	pkgs := map[string]map[string]*ast.File{"main": {"guard.go": file}}
	md := meta.GenerateMetadata(pkgs, map[*ast.File]*types.Info{file: info}, map[string]string{"main": "main"}, fset)

	idx := newBlockIndex(md)
	blocks := idx.byFile["guard.go"]
	if len(blocks) != 1 {
		t.Fatalf("want the one `if` arm indexed under guard.go, got %d blocks (files: %v)",
			len(blocks), indexedFiles(idx.byFile))
	}
	if blocks[0].Kind != meta.BlockIf || blocks[0].Group == 0 {
		t.Errorf("indexed block = %+v, want an if arm carrying a group", blocks[0])
	}

	// fail() is inside the arm, succeed() below it: the shape the rule rejects.
	inArm := codePos{file: "guard.go", line: 5, col: 3}
	belowArm := codePos{file: "guard.go", line: 8, col: 2}
	if idx.dominates(inArm, belowArm) {
		t.Error("a call inside the returning arm must not dominate the call below it")
	}
	if !idx.dominates(belowArm, codePos{file: "guard.go", line: 8, col: 20}) {
		t.Error("two positions outside every region must dominate in source order")
	}
}

func indexedFiles(m map[string][]meta.Block) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func TestNewBlockIndexNilMetadata(t *testing.T) {
	idx := newBlockIndex(nil)
	if idx == nil {
		t.Fatal("newBlockIndex(nil) must return an empty index, not nil")
	}
	if len(idx.byFile) != 0 {
		t.Errorf("want an empty index, got %d files", len(idx.byFile))
	}
	if !idx.dominates(at(1, 1), at(2, 1)) {
		t.Error("an empty index must answer permissively")
	}
}
