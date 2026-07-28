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
	"go/parser"
	"go/token"
	"testing"
)

// TestFormatPositionMatchesToken is the load-bearing test of this change: the
// rendering is reproduced by hand instead of calling token.Position.String, and
// the output is observable in tracker keys, in a closure's identity, and in the
// byte-compared metadata goldens. Every shape token handles specially is checked
// against token itself, so the two can never drift silently.
func TestFormatPositionMatchesToken(t *testing.T) {
	positions := []token.Position{
		{Filename: "main.go", Line: 12, Column: 34},
		{Filename: "a/b/main.go", Line: 1, Column: 1},
		{Filename: "/abs/path/main.go", Line: 4321, Column: 7},
		// No column: token omits it rather than printing ":0".
		{Filename: "main.go", Line: 9},
		// No filename: token prints the line without a leading colon.
		{Line: 9, Column: 2},
		{Line: 9},
		// Invalid (line 0): token prints the filename alone...
		{Filename: "main.go"},
		// ...or "-" when there is not even a filename.
		{},
	}
	for _, p := range positions {
		if got, want := formatPosition(p), p.String(); got != want {
			t.Errorf("formatPosition(%+v) = %q, want %q (token.Position.String)", p, got, want)
		}
	}
}

// TestPositionIndexMemoizes pins the behaviour the allocation win rests on: a
// repeated location is formatted once and interned once, and distinct locations
// stay distinct. Sharing an index between two locations would merge two call
// sites in the tracker.
func TestPositionIndexMemoizes(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "main.go", "package p\n\nvar a = 1\nvar b = 2\n", 0)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	m := &Metadata{StringPool: NewStringPool()}

	posA, posB := file.Decls[0].Pos(), file.Decls[1].Pos()

	idxA := m.positionIndex(posA, fset)
	if idxA < 0 {
		t.Fatal("a valid position did not resolve to a pooled string")
	}
	if again := m.positionIndex(posA, fset); again != idxA {
		t.Errorf("second lookup = %d, want the memoized %d", again, idxA)
	}
	if idxB := m.positionIndex(posB, fset); idxB == idxA {
		t.Error("two different locations share one pool index — they would collapse in tracker keys")
	}

	// The pooled string is what token would have produced.
	if got, want := m.positionString(posA, fset), fset.Position(posA).String(); got != want {
		t.Errorf("positionString = %q, want %q", got, want)
	}

	// Two distinct positions, cached once each.
	if len(m.posCache) != 2 {
		t.Errorf("cache holds %d entries, want 2", len(m.posCache))
	}
}

// TestPositionIndexRejectsNothing covers the paths that must yield "no string"
// rather than a bogus pool entry — the callers store the result directly on a
// node, where -1 means "no position recorded".
func TestPositionIndexRejectsNothing(t *testing.T) {
	fset := token.NewFileSet()
	m := &Metadata{StringPool: NewStringPool()}

	if got := m.positionIndex(token.NoPos, fset); got != -1 {
		t.Errorf("invalid position = %d, want -1", got)
	}
	if got := m.positionIndex(token.Pos(1), nil); got != -1 {
		t.Errorf("nil fileset = %d, want -1", got)
	}
	if got := m.positionString(token.NoPos, fset); got != "" {
		t.Errorf("invalid position string = %q, want empty", got)
	}

	// A Pos only means something in the FileSet that issued it. An empty FileSet
	// knows no positions, and a Pos past the end of a known file belongs to no
	// file either; both render as "-" through token, which must not be recorded as
	// a position (CodeRabbit, PR #228).
	if got := m.positionIndex(token.Pos(1), token.NewFileSet()); got != -1 {
		t.Errorf("position in an empty fileset = %d, want -1", got)
	}
	populated := token.NewFileSet()
	f := populated.AddFile("p.go", -1, 10)
	if got := m.positionIndex(f.Pos(3), populated); got < 0 {
		t.Error("a position inside a known file did not resolve")
	}
	if got := m.positionIndex(token.Pos(f.Base()+1000), populated); got != -1 {
		t.Errorf("position past the end of every file = %d, want -1", got)
	}

	var nilMeta *Metadata
	if got := nilMeta.positionIndex(token.Pos(1), fset); got != -1 {
		t.Errorf("nil metadata = %d, want -1", got)
	}
	if got := (&Metadata{}).positionIndex(token.Pos(1), fset); got != -1 {
		t.Errorf("metadata without a pool = %d, want -1", got)
	}
}

// TestFuncLitName pins the closure identifier. It is not cosmetic: it becomes a
// tracker key and the operationId of a closure route, so the shape has to be
// stated in one place rather than formatted at each of the three sites that mint
// it.
func TestFuncLitName(t *testing.T) {
	if got, want := funcLitName("main.go:12:34"), "FuncLit:main.go:12:34"; got != want {
		t.Errorf("funcLitName = %q, want %q", got, want)
	}
	if got := funcLitName(""); got != FuncLitPrefix {
		t.Errorf("funcLitName(\"\") = %q, want %q", got, FuncLitPrefix)
	}
}
