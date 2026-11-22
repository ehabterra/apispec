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
	"go/parser"
	"go/token"
	"go/types"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// parseBody parses a function body snippet and returns the blocks collected
// from it, plus the fileset, so a test can also assert positions.
func parseBody(t *testing.T, body string) ([]Block, *token.FileSet, *ast.FuncDecl) {
	t.Helper()
	src := "package p\n\nfunc h() {\n" + body + "\n}\n"
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "h.go", src, 0)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == "h" {
			return collectBlocks(fn.Body, fset), fset, fn
		}
	}
	t.Fatal("function h not found")
	return nil, nil, nil
}

// shape renders the blocks as "kind:parent:group" in order, which is the whole
// structure this file is about and reads far better than field-by-field asserts.
func shape(blocks []Block) string {
	parts := make([]string, 0, len(blocks))
	for _, b := range blocks {
		parts = append(parts, string(b.Kind)+":"+itoa(b.Parent)+":"+itoa(b.Group))
	}
	return strings.Join(parts, " ")
}

func itoa(n int) string {
	if n == 0 {
		return "-"
	}
	return string(rune('0' + n))
}

func TestCollectBlocksShapes(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{
			name: "if without else",
			body: "if x() {\n  a()\n}",
			want: "if:-:1",
		},
		{
			// The whole chain is ONE group: every arm is an alternative, and
			// none is nested in another. This is the relation the collector
			// exists for.
			name: "if else-if else chain",
			body: "if x() {\n a()\n} else if y() {\n b()\n} else {\n c()\n}",
			want: "if:-:1 if:-:1 else:-:1",
		},
		{
			name: "two independent ifs get distinct groups",
			body: "if x() {\n a()\n}\nif y() {\n b()\n}",
			want: "if:-:1 if:-:2",
		},
		{
			// A nested if is a region INSIDE the outer arm (parent 1) and its
			// own conditional (group 2) — nesting and exclusivity are separate.
			name: "nested if",
			body: "if x() {\n if y() {\n  a()\n }\n}",
			want: "if:-:1 if:1:2",
		},
		{
			name: "switch cases share a group, default included",
			body: "switch v() {\ncase 1:\n a()\ncase 2:\n b()\ndefault:\n c()\n}",
			want: "case:-:1 case:-:1 default:-:1",
		},
		{
			name: "type switch",
			body: "switch v().(type) {\ncase int:\n a()\ndefault:\n b()\n}",
			want: "case:-:1 default:-:1",
		},
		{
			name: "select",
			body: "select {\ncase <-ch():\n a()\ndefault:\n b()\n}",
			want: "case:-:1 default:-:1",
		},
		{
			// A loop body is exclusive with nothing, so it carries no group.
			name: "for and range bodies carry no group",
			body: "for i := 0; i < 3; i++ {\n a()\n}\nfor range s() {\n b()\n}",
			want: "scope:-:- loop:-:- loop:-:-",
		},
		{
			// The initializer's names end with the statement, and the scope is
			// not a branch: it has no group and nothing is parented to it.
			name: "if init opens a scope, else-if init another",
			body: "if a := f(); a {\n x()\n} else if b := g(); b {\n y()\n} else {\n z()\n}",
			want: "scope:-:- if:-:1 scope:-:- if:-:1 else:-:1",
		},
		{
			name: "switch and type switch inits open a scope",
			body: "switch a := f(); a {\ncase 1:\n x()\n}\nswitch b := g(); v := b.(type) {\ncase int:\n _ = v\n}",
			want: "scope:-:- case:-:1 scope:-:- case:-:2",
		},
		{
			name: "no init, no scope",
			body: "if x() {\n a()\n}\nswitch {\ncase true:\n}\nfor {\n}",
			want: "if:-:1 case:-:2 loop:-:-",
		},
		{
			name: "bare block",
			body: "{\n a()\n}",
			want: "block:-:-",
		},
		{
			name: "function literal body is a region",
			body: "f(func() {\n a()\n})",
			want: "func:-:-",
		},
		{
			// The shape behind issue #382: control flow inside a closure is
			// recorded, parented to the closure's body.
			name: "conditional inside a function literal",
			body: "f(func() {\n if x() {\n  a()\n } else {\n  b()\n }\n})",
			want: "func:-:- if:1:1 else:1:1",
		},
		{
			name: "literal in an if arm is parented to the arm",
			body: "if x() {\n f(func() {\n  a()\n })\n}",
			want: "if:-:1 func:1:-",
		},
		{
			name: "labeled statement is transparent",
			body: "loop:\nfor {\n a()\n}",
			want: "loop:-:-",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			blocks, _, _ := parseBody(t, tc.body)
			if got := shape(blocks); got != tc.want {
				t.Errorf("blocks =\n  %q\nwant\n  %q", got, tc.want)
			}
		})
	}
}

// TestCollectBlocksRangesAreColumnPrecise pins the reason ranges carry columns:
// on a one-line `if`, the arm and the code around it share every line, so a
// line-only range cannot answer containment. Written as a containment assertion
// rather than as literal numbers so it states the property, not the layout.
func TestCollectBlocksRangesAreColumnPrecise(t *testing.T) {
	// The call in the init runs before the arm; the call in the body is inside
	// it. Both sit on the same line.
	src := "if err := before(); err != nil { inside() }"
	blocks, fset, fn := parseBody(t, src)
	if len(blocks) != 2 || blocks[0].Kind != BlockScope {
		t.Fatalf("want a scope and an arm, got %d (%s)", len(blocks), shape(blocks))
	}
	arm := blocks[1]
	if arm.StartLine != arm.EndLine {
		t.Fatalf("this fixture is meant to be one line, got %d..%d", arm.StartLine, arm.EndLine)
	}

	var before, inside token.Position
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if ident, ok := call.Fun.(*ast.Ident); ok {
			switch ident.Name {
			case "before":
				before = fset.Position(ident.Pos())
			case "inside":
				inside = fset.Position(ident.Pos())
			}
		}
		return true
	})
	if before.Line == 0 || inside.Line == 0 {
		t.Fatal("did not find both calls")
	}
	if contains(arm, before) {
		t.Errorf("call in the if's init reported as inside the arm (line-only ranges would say this)")
	}
	if !contains(arm, inside) {
		t.Errorf("call in the if's body reported as outside the arm")
	}
}

// contains is the containment test a consumer of Block would write.
func contains(b Block, pos token.Position) bool {
	afterStart := pos.Line > b.StartLine || (pos.Line == b.StartLine && pos.Column >= b.StartCol)
	beforeEnd := pos.Line < b.EndLine || (pos.Line == b.EndLine && pos.Column <= b.EndCol)
	return afterStart && beforeEnd
}

// TestCollectBlocksExclusivity demonstrates the question the group answers: two
// statements that cannot both run, though they are adjacent in source order.
func TestCollectBlocksExclusivity(t *testing.T) {
	blocks, _, _ := parseBody(t, "if x() {\n a()\n} else {\n b()\n}\nif y() {\n c()\n}")
	if len(blocks) != 3 {
		t.Fatalf("want 3 blocks, got %d (%s)", len(blocks), shape(blocks))
	}
	ifArm, elseArm, other := blocks[0], blocks[1], blocks[2]
	if ifArm.Group != elseArm.Group {
		t.Errorf("if and else arms must share a group: %d vs %d", ifArm.Group, elseArm.Group)
	}
	if other.Group == ifArm.Group {
		t.Errorf("an unrelated if must not share the first chain's group")
	}
	if ifArm.Group == 0 || other.Group == 0 {
		t.Errorf("conditional arms must carry a group, got %d and %d", ifArm.Group, other.Group)
	}
}

// TestCollectBlocksTerminates pins the fact that makes Go's dominant idiom
// legible: an arm that exits means nothing after its conditional runs when the
// arm did, so the two are alternatives even though only one is inside an arm.
//
// Only syntactic certainty counts. A call that never returns by convention
// (os.Exit, log.Fatal) is not one — deciding that needs the type checker, the
// answer would be a guess for any project-local wrapper, and a wrong
// "terminates" turns sequential statements into alternatives.
func TestCollectBlocksTerminates(t *testing.T) {
	cases := []struct {
		name string
		body string
		want []bool // per block, in source order
	}{
		{"arm ending in return", "if x() {\n a()\n return\n}", []bool{true}},
		{"arm falling through", "if x() {\n a()\n}", []bool{false}},
		{"panic ends an arm", "if x() {\n panic(\"no\")\n}", []bool{true}},
		{"a call that merely looks final does not", "if x() {\n exit()\n}", []bool{false}},
		{"else returns, if does not", "if x() {\n a()\n} else {\n return\n}", []bool{false, true}},
		// A branch statement leaves the region without ending the path: after a
		// `break` the statement following the switch runs, which is exactly
		// what "terminates" must not claim.
		{"break does not end a case", "switch v() {\ncase 1:\n a()\n break\ncase 2:\n b()\n}", []bool{false, false}},
		{"fallthrough does not", "switch v() {\ncase 1:\n a()\n fallthrough\ncase 2:\n b()\n}", []bool{false, false}},
		{"continue does not end a loop body", "for x() {\n continue\n}", []bool{false}},
		{"return inside a case does", "switch v() {\ncase 1:\n return\ncase 2:\n b()\n}", []bool{true, false}},
		{"empty arm", "if x() {\n}", []bool{false}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			blocks, _, _ := parseBody(t, tc.body)
			if len(blocks) != len(tc.want) {
				t.Fatalf("got %d blocks (%s), want %d", len(blocks), shape(blocks), len(tc.want))
			}
			for i, want := range tc.want {
				if blocks[i].Terminates != want {
					t.Errorf("block %d (%s): Terminates = %v, want %v",
						i, blocks[i].Kind, blocks[i].Terminates, want)
				}
			}
		})
	}
}

func TestCollectBlocksNilSafety(t *testing.T) {
	if got := collectBlocks(nil, token.NewFileSet()); got != nil {
		t.Errorf("nil body: want nil, got %v", got)
	}
	if got := collectBlocks(&ast.BlockStmt{}, nil); got != nil {
		t.Errorf("nil fileset: want nil, got %v", got)
	}
	if got := collectBlocks(&ast.BlockStmt{}, token.NewFileSet()); got != nil {
		t.Errorf("empty body: want nil, got %v", got)
	}
	// A synthesized switch with no clause list: the parser never produces one,
	// but metadata is also built over nodes this package synthesizes, and a
	// collector that panicked there would take a whole run down.
	synthetic := &ast.BlockStmt{List: []ast.Stmt{
		&ast.SwitchStmt{},
		&ast.SelectStmt{},
	}}
	if got := collectBlocks(synthetic, token.NewFileSet()); got != nil {
		t.Errorf("switch/select without a body: want nil, got %v", got)
	}
}

// TestBlockSurvivesYAMLRoundTrip guards the 1-BASED encoding of Parent and
// Group. Metadata is written and read back (LoadMetadata), and these fields are
// omitempty, so a plain 0-based index would serialize "the first block" and
// "no parent" identically — reading back a nesting relation that was never
// written. The zero value has to mean "none" for that reason, and this test
// fails if anyone re-bases the indices.
func TestBlockSurvivesYAMLRoundTrip(t *testing.T) {
	blocks, _, _ := parseBody(t, "if x() {\n if y() {\n  a()\n } else {\n  b()\n }\n}")
	if got := shape(blocks); got != "if:-:1 if:1:2 else:1:2" {
		t.Fatalf("fixture shape changed: %q", got)
	}

	data, err := yaml.Marshal(blocks)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back []Block
	if err := yaml.Unmarshal(data, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got := shape(back); got != shape(blocks) {
		t.Errorf("round trip changed the structure:\n got %q\nwant %q\nyaml:\n%s",
			shape(back), shape(blocks), data)
	}
	// The outermost arm has no parent, so its field must be absent rather than
	// written as an index some reader would resolve to a block.
	if strings.Contains(strings.SplitN(string(data), "- kind: if", 2)[1][:60], "parent:") {
		t.Errorf("a parentless block serialized a parent field:\n%s", data)
	}
}

func TestBodyEndLine(t *testing.T) {
	src := "package p\n\nfunc h() {\n a()\n}\n\nfunc noBody()\n"
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "h.go", src, 0)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var withBody, without *ast.FuncDecl
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		if fn.Name.Name == "h" {
			withBody = fn
		} else {
			without = fn
		}
	}
	if got := bodyEndLine(withBody, fset); got != 5 {
		t.Errorf("bodyEndLine = %d, want 5 (the closing brace)", got)
	}
	if got := bodyEndLine(without, fset); got != 0 {
		t.Errorf("declaration without a body: bodyEndLine = %d, want 0", got)
	}
	if got := bodyEndLine(nil, fset); got != 0 {
		t.Errorf("nil declaration: bodyEndLine = %d, want 0", got)
	}
}

// TestMethodRecordsBlocks pins issue #617: a method records its control-flow
// regions exactly as a function with the same body does. Methods used to record
// none, which left every control-flow query blind inside a method handler.
func TestMethodRecordsBlocks(t *testing.T) {
	const body = `{
	if bad {
		fail()
		return
	}
	for range 3 {
		fail()
	}
	succeed()
}`
	// The function and the method sit on lines that differ by a fixed offset,
	// so their ranges must agree once that offset is taken out.
	src := "package main\n\ntype API struct{}\n\ntype Doer interface{ Do(bad bool) }\n\n" +
		"func guard(bad bool) " + body + "\n\n" +
		"func (a *API) guard(bad bool) " + body + "\n\n" +
		"func fail()    {}\nfunc succeed() {}\n"
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
	md := GenerateMetadata(pkgs, map[*ast.File]*types.Info{file: info}, map[string]string{"main": "main"}, fset)

	var f *File
	for _, file := range md.SortedFiles("main") {
		f = file
	}
	if f == nil {
		t.Fatal("no file recorded for package main")
	}
	fn := f.Functions["guard"]
	if fn == nil || len(fn.Blocks) == 0 {
		t.Fatalf("function guard recorded no blocks: %+v", fn)
	}
	api := f.Types["API"]
	if api == nil || len(api.Methods) != 1 {
		t.Fatalf("want one method on API, got %+v", api)
	}
	m := api.Methods[0]
	if got, want := shape(m.Blocks), shape(fn.Blocks); got != want {
		t.Fatalf("method blocks %q, want the function's %q", got, want)
	}
	offset := m.Blocks[0].StartLine - fn.Blocks[0].StartLine
	if offset <= 0 {
		t.Fatalf("method blocks start at line %d, function's at %d: the method's own body was not walked",
			m.Blocks[0].StartLine, fn.Blocks[0].StartLine)
	}
	for i := range fn.Blocks {
		want := fn.Blocks[i]
		want.StartLine += offset
		want.EndLine += offset
		if m.Blocks[i] != want {
			t.Errorf("method block %d = %+v, want %+v", i, m.Blocks[i], want)
		}
	}

	// The method's EndLine bounds its body exactly as the function's does.
	if fn.EndLine == 0 || m.EndLine != fn.EndLine+offset {
		t.Errorf("method EndLine = %d, want the function's %d shifted by %d", m.EndLine, fn.EndLine, offset)
	}

	// An interface method has no body, so no regions and no end.
	for _, im := range f.Types["Doer"].Methods {
		if len(im.Blocks) != 0 || im.EndLine != 0 {
			t.Errorf("interface method recorded a body: blocks %+v, end line %d", im.Blocks, im.EndLine)
		}
	}
}

// TestCollectBlocksScopeExtent pins where an initializer's scope ends: with the
// whole statement, every else arm included, and not a line earlier or later.
func TestCollectBlocksScopeExtent(t *testing.T) {
	blocks, fset, fn := parseBody(t, "if h := f(); h != nil {\n a()\n} else {\n b()\n}\nafter()")
	scope := blocks[0]
	if scope.Kind != BlockScope {
		t.Fatalf("first block is %s, want scope", scope.Kind)
	}
	at := map[string]token.Position{}
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			if id, ok := call.Fun.(*ast.Ident); ok {
				at[id.Name] = fset.Position(id.Pos())
			}
		}
		return true
	})
	for name, want := range map[string]bool{"f": true, "a": true, "b": true, "after": false} {
		if got := contains(scope, at[name]); got != want {
			t.Errorf("scope contains %s() = %v, want %v", name, got, want)
		}
	}
}
