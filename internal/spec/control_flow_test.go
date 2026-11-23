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

// indexOf files the blocks under h.go as ONE declaration spanning the whole
// file, which is what the hand-built cases assume.
func indexOf(blocks ...meta.Block) *blockIndex {
	entries := make([]indexedBlock, len(blocks))
	for i, b := range blocks {
		entries[i] = indexedBlock{Block: b, declStart: 1, declEnd: 1 << 20}
	}
	return &blockIndex{byFile: map[string][]indexedBlock{"h.go": entries}}
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

// TestNewBlockIndexIncludesMethods pins issue #617: a method's regions are
// indexed, under the file the method is WRITTEN in — Type.Methods gathers every
// method of the package onto the type, wherever its declaration sits. Before,
// only functions were indexed, so inside a method dominance answered yes and
// exclusivity no for want of facts.
func TestNewBlockIndexIncludesMethods(t *testing.T) {
	typesSrc := `package main

type API struct{}

type Doer interface{ Do(bad bool) }
`
	handlersSrc := `package main

func (a *API) guard(bad bool) {
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
	typesFile, err := parser.ParseFile(fset, "types.go", typesSrc, 0)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	handlersFile, err := parser.ParseFile(fset, "handlers.go", handlersSrc, 0)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	files := []*ast.File{typesFile, handlersFile}
	info := &types.Info{
		Types: map[ast.Expr]types.TypeAndValue{},
		Defs:  map[*ast.Ident]types.Object{},
		Uses:  map[*ast.Ident]types.Object{},
	}
	if _, err := (&types.Config{}).Check("main", fset, files, info); err != nil {
		t.Fatalf("typecheck: %v", err)
	}
	pkgs := map[string]map[string]*ast.File{"main": {"types.go": typesFile, "handlers.go": handlersFile}}
	fileToInfo := map[*ast.File]*types.Info{typesFile: info, handlersFile: info}
	md := meta.GenerateMetadata(pkgs, fileToInfo, map[string]string{"main": "main"}, fset)

	idx := newBlockIndex(md)
	if got := idx.byFile["types.go"]; len(got) != 0 {
		t.Errorf("types.go declares no bodies, yet %d blocks were indexed under it", len(got))
	}
	blocks := idx.byFile["handlers.go"]
	if len(blocks) != 1 {
		t.Fatalf("want the method's one `if` arm indexed once under handlers.go, got %d blocks (files: %v)",
			len(blocks), indexedFiles(idx.byFile))
	}

	inArm := codePos{file: "handlers.go", line: 5, col: 3}
	belowArm := codePos{file: "handlers.go", line: 8, col: 2}
	if idx.dominates(inArm, belowArm) {
		t.Error("in a method, a call inside the returning arm must not dominate the call below it")
	}
	if !idx.exclusive(inArm, belowArm) {
		t.Error("in a method, the returning arm and the code below it must be exclusive")
	}
}

func indexedFiles(m map[string][]indexedBlock) []string {
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

// blockIndexOf runs metadata generation over one package's sources and indexes
// the result, so a case reads in Go rather than in hand-placed line numbers.
func blockIndexOf(t *testing.T, sources map[string]string) *blockIndex {
	t.Helper()
	fset := token.NewFileSet()
	names := make([]string, 0, len(sources))
	for name := range sources {
		names = append(names, name)
	}
	sort.Strings(names)
	files := make([]*ast.File, 0, len(names))
	byName := map[string]*ast.File{}
	for _, name := range names {
		f, err := parser.ParseFile(fset, name, sources[name], 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		files = append(files, f)
		byName[name] = f
	}
	info := &types.Info{
		Types: map[ast.Expr]types.TypeAndValue{},
		Defs:  map[*ast.Ident]types.Object{},
		Uses:  map[*ast.Ident]types.Object{},
	}
	if _, err := (&types.Config{}).Check("main", fset, files, info); err != nil {
		t.Fatalf("typecheck: %v", err)
	}
	fileToInfo := map[*ast.File]*types.Info{}
	for _, f := range files {
		fileToInfo[f] = info
	}
	md := meta.GenerateMetadata(map[string]map[string]*ast.File{"main": byName}, fileToInfo, map[string]string{"main": "main"}, fset)
	return newBlockIndex(md)
}

// TestExclusiveSiblingArmsStayInOneDeclaration pins the group collision the
// index used to have: Block.Group is numbered per declaration, so the first
// if/else of every function and method in a file is "group 1", and an arm of
// one read as a sibling arm of a conditional in another. Both bodies can run on
// one request, so they are not alternatives.
func TestExclusiveSiblingArmsStayInOneDeclaration(t *testing.T) {
	src := `package main

type API struct{}

func f(bad bool) {
	if bad {
		g(false)
	} else {
		other()
	}
}

func g(bad bool) {
	if bad {
		other()
	} else {
		other()
	}
}

func (a *API) m(bad bool) {
	if bad {
		other()
	} else {
		other()
	}
}

func other() {}
`
	idx := blockIndexOf(t, map[string]string{"h.go": src})
	pos := func(line int) codePos { return codePos{file: "h.go", line: line, col: 3} }
	fIf, fElse := pos(7), pos(9)
	gIf, gElse := pos(15), pos(17)
	mIf, mElse := pos(23), pos(25)

	cases := []struct {
		name string
		a, b codePos
		want bool
	}{
		{"if and else of f", fIf, fElse, true},
		{"if and else of g", gIf, gElse, true},
		{"if and else of method m", mIf, mElse, true},
		{"f's if arm and g's else arm", fIf, gElse, false},
		{"g's if arm and f's else arm", gIf, fElse, false},
		{"function f's if arm and method m's else arm", fIf, mElse, false},
		{"method m's if arm and function g's else arm", mIf, gElse, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := idx.exclusive(tc.a, tc.b); got != tc.want {
				t.Errorf("exclusive(%v, %v) = %v, want %v", tc.a, tc.b, got, tc.want)
			}
		})
	}
}

// TestExclusiveTerminatingArmStaysInOneDeclaration pins the other cross-
// declaration answer: a returning arm leaves its OWN function, so it makes the
// code after it in that function an alternative, and says nothing about a
// declaration written below it in the file.
func TestExclusiveTerminatingArmStaysInOneDeclaration(t *testing.T) {
	src := `package main

type API struct{}

func h(bad bool) {
	if bad {
		return
	}
	other()
}

func k() { other() }

func (a *API) mh(bad bool) {
	if bad {
		return
	}
	other()
}

func (a *API) mk() { other() }

func other() {}
`
	idx := blockIndexOf(t, map[string]string{"h.go": src})
	pos := func(line, col int) codePos { return codePos{file: "h.go", line: line, col: col} }
	hArm, hTail, kBody := pos(7, 3), pos(9, 2), pos(12, 12)
	mhArm, mhTail, mkBody := pos(16, 3), pos(18, 2), pos(21, 22)

	cases := []struct {
		name string
		a, b codePos
		want bool
	}{
		{"function: returning arm and its own tail", hArm, hTail, true},
		{"method: returning arm and its own tail", mhArm, mhTail, true},
		{"function arm and the next function's body", hArm, kBody, false},
		{"method arm and the next method's body", mhArm, mkBody, false},
		{"function arm and a later method's body", hArm, mhTail, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := idx.exclusive(tc.a, tc.b); got != tc.want {
				t.Errorf("exclusive(%v, %v) = %v, want %v", tc.a, tc.b, got, tc.want)
			}
		})
	}
}

// TestExclusiveUnknownDeclarationSpan: with no declaration span the
// terminates-after rule cannot tell the arm's own function from the next one,
// so it does not answer — the conservative direction for exclusivity.
func TestExclusiveUnknownDeclarationSpan(t *testing.T) {
	arm := meta.Block{Kind: meta.BlockIf, StartLine: 10, StartCol: 1, EndLine: 13, EndCol: 80, Group: 1, Terminates: true}
	idx := &blockIndex{byFile: map[string][]indexedBlock{"h.go": {{Block: arm}}}}
	if idx.exclusive(at(11, 3), at(20, 2)) {
		t.Error("a returning arm with no known declaration span must not make later code an alternative")
	}
}

// TestExclusiveTerminatingArmInsideLiteral: a returning arm inside a function
// literal leaves the literal only. Code after it in the literal is an
// alternative; code after the literal in the enclosing declaration is not —
// the literal returns and that code runs.
func TestExclusiveTerminatingArmInsideLiteral(t *testing.T) {
	src := `package main

type API struct{}

func (a *API) m(bad bool) {
	check := func() {
		if bad {
			return
		}
		other()
	}
	check()
	other()
	if bad {
		go func() { other() }()
		return
	}
	other()
}

func other() {}
`
	idx := blockIndexOf(t, map[string]string{"h.go": src})
	pos := func(line, col int) codePos { return codePos{file: "h.go", line: line, col: col} }
	litArm, litTail, declTail := pos(8, 4), pos(10, 3), pos(13, 2)
	nestedLit, armTail := pos(15, 15), pos(18, 2)

	cases := []struct {
		name string
		a, b codePos
		want bool
	}{
		{"literal's returning arm and the literal's own tail", litArm, litTail, true},
		{"literal's returning arm and the declaration after the literal", litArm, declTail, false},
		{"a literal nested inside a returning arm and the code after the arm", nestedLit, armTail, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := idx.exclusive(tc.a, tc.b); got != tc.want {
				t.Errorf("exclusive(%v, %v) = %v, want %v", tc.a, tc.b, got, tc.want)
			}
		})
	}
}

// TestInScopeIsSeparateFromDominance pins the split closureAssignedTo relies
// on: an initializer runs unconditionally, so it dominates the code after its
// statement, yet the names it declares are not visible there. The scope region
// must answer inScope without changing dominates, exclusive or sharesRegion.
func TestInScopeIsSeparateFromDominance(t *testing.T) {
	scope := meta.Block{Kind: meta.BlockScope, StartLine: 10, StartCol: 5, EndLine: 14, EndCol: 2}
	arm := armAt(10, 12)
	arm.StartCol = 30 // the init shares the arm's first line but not the arm
	idx := indexOf(scope, arm)

	init, inArm, after := at(10, 5), at(11, 3), at(16, 2)

	if idx.inScope(init, inArm) != true {
		t.Error("a use inside the statement's arm must see the init's binding")
	}
	if idx.inScope(init, after) {
		t.Error("a use after the statement must not see the init's binding")
	}
	if !idx.dominates(init, after) {
		t.Error("the init still dominates code after its statement (response pairing relies on it)")
	}
	if idx.sharesRegion(init, after) {
		t.Error("a scope must not count as a shared region")
	}
	if idx.sharesRegion(init, inArm) {
		t.Error("a scope must not count as a shared region when both positions lie inside it")
	}
	if idx.exclusive(init, after) {
		t.Error("a scope is not a branch")
	}
	// A binding declared before the statement is unaffected by it.
	if !idx.inScope(at(8, 2), after) {
		t.Error("an assignment outside any scope stays visible")
	}
	// Missing facts stay permissive.
	if !(*blockIndex)(nil).inScope(init, after) || !idx.inScope(codePos{}, after) {
		t.Error("missing facts must answer in scope")
	}
}
