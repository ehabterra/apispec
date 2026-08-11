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
	"testing"
)

// funcLitSrc exercises the shapes the pruned walk has to get right: a plain
// function (no literal), a literal, a literal nested in a literal, sibling
// literals that must not be confused for each other, and a literal in a
// composite-literal field.
const funcLitSrc = `package p

import "net/http"

func plain(w http.ResponseWriter, r *http.Request) {
	println("plain")
}

func outer() {
	f := func() {
		println("inner-one")
		g := func() {
			println("innermost")
		}
		g()
	}
	f()
	h := func() { println("sibling") }
	h()
}

type handlers struct{ fn func() }

func composite() {
	_ = handlers{fn: func() { println("in-composite") }}
}
`

// exhaustiveEnclosingFuncLit is the pre-#225 implementation: it visits every
// node in the file and never prunes. Kept here purely as the oracle the pruned
// version is diffed against.
func exhaustiveEnclosingFuncLit(file *ast.File, pos token.Pos) *ast.FuncLit {
	var found *ast.FuncLit
	ast.Inspect(file, func(n ast.Node) bool {
		if n == nil {
			return true
		}
		if n.Pos()+1 <= pos && pos <= n.End()-1 {
			if funcLit, ok := n.(*ast.FuncLit); ok {
				found = funcLit
			}
		}
		return true
	})
	return found
}

// TestFindEnclosingFunctionLiteralMatchesExhaustiveWalk is the guard for the
// pruning in #225: for every position in the file — inside literals, between
// them, in a plain function, and outside the declarations entirely — the pruned
// walk must return exactly what the exhaustive walk returned.
func TestFindEnclosingFunctionLiteralMatchesExhaustiveWalk(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "p.go", funcLitSrc, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	tf := fset.File(file.Pos())
	if tf == nil {
		t.Fatal("no token.File")
	}

	var checked, inside int
	for off := 0; off <= tf.Size(); off++ {
		pos := tf.Pos(off)
		want := exhaustiveEnclosingFuncLit(file, pos)
		got := findEnclosingFunctionLiteral(file, pos)
		checked++
		if want != nil {
			inside++
		}
		if got != want {
			t.Fatalf("offset %d (%s): pruned walk = %v, exhaustive walk = %v",
				off, fset.Position(pos), describeFuncLit(fset, got), describeFuncLit(fset, want))
		}
	}

	// Guard the fixture itself: if it stopped containing literals the
	// comparison above would pass trivially.
	if inside == 0 {
		t.Fatal("no position in the fixture landed inside a function literal")
	}
	t.Logf("compared %d positions, %d inside a function literal", checked, inside)
}

func describeFuncLit(fset *token.FileSet, fl *ast.FuncLit) string {
	if fl == nil {
		return "nil"
	}
	return "funcLit@" + fset.Position(fl.Pos()).String()
}

// TestFindEnclosingFunctionLiteralInnermost states the contract directly rather
// than by comparison: nested literals resolve to the innermost one.
func TestFindEnclosingFunctionLiteralInnermost(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "p.go", funcLitSrc, 0)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	// Collect literals by the line their body starts on.
	lits := map[string]*ast.FuncLit{}
	ast.Inspect(file, func(n ast.Node) bool {
		fl, ok := n.(*ast.FuncLit)
		if !ok {
			return true
		}
		ast.Inspect(fl.Body, func(m ast.Node) bool {
			bl, ok := m.(*ast.BasicLit)
			if ok && bl.Kind == token.STRING {
				if _, seen := lits[bl.Value]; !seen {
					lits[bl.Value] = fl
				}
			}
			return true
		})
		return true
	})

	for _, name := range []string{`"innermost"`, `"sibling"`, `"in-composite"`} {
		lit, ok := lits[name]
		if !ok {
			t.Fatalf("fixture no longer contains %s", name)
		}
		// A position inside that literal's body must resolve to it, not to an
		// enclosing literal.
		pos := lit.Body.Lbrace + 1
		if got := findEnclosingFunctionLiteral(file, pos); got != lit {
			t.Errorf("%s: got %s, want %s", name, describeFuncLit(fset, got), describeFuncLit(fset, lit))
		}
	}

	// A call inside a plain function is in no literal at all.
	var plainBody *ast.BlockStmt
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == "plain" {
			plainBody = fn.Body
		}
	}
	if plainBody == nil {
		t.Fatal("fixture no longer declares plain()")
	}
	if got := findEnclosingFunctionLiteral(file, plainBody.Lbrace+1); got != nil {
		t.Errorf("inside plain(): got %s, want nil", describeFuncLit(fset, got))
	}
}

// BenchmarkFindEnclosingFunctionLiteral measures the shape #225 is about: the
// cost per query against a file, which the unpruned walk paid in full for every
// call expression.
func BenchmarkFindEnclosingFunctionLiteral(b *testing.B) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "p.go", funcLitSrc, 0)
	if err != nil {
		b.Fatalf("parse: %v", err)
	}
	tf := fset.File(file.Pos())
	pos := tf.Pos(tf.Size() / 2)

	b.Run("pruned", func(b *testing.B) {
		for b.Loop() {
			_ = findEnclosingFunctionLiteral(file, pos)
		}
	})
	b.Run("exhaustive", func(b *testing.B) {
		for b.Loop() {
			_ = exhaustiveEnclosingFuncLit(file, pos)
		}
	})
}
