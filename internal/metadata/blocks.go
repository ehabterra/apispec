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
	"go/token"
)

// Control-flow block ranges.
//
// Metadata records where a call is written (`file:line:col` on the edge) but
// nothing about the code AROUND it, so the only structural question a consumer
// could previously ask was "same file?". That is too weak for anything that has
// to reason about what runs together: a status write in one `if` arm and a body
// write in the other arm are adjacent in source order and can never both run,
// and line order alone cannot tell them apart.
//
// collectBlocks records each control-flow region of a body as a source range,
// with two relations on top: PARENT (this region is nested in that one) and
// GROUP (these regions are arms of one conditional, so at most one runs).
// Together with Function.EndLine that is enough to place any call edge within
// its function's control flow using only the position the edge already carries.
//
// Nothing here interprets the code — no matcher, no framework knowledge. It
// records facts, and the spec layer decides what they mean (golden rule #4).

// collectBlocks returns the control-flow regions of a function body in source
// order. Nil for a declaration without one.
func collectBlocks(body *ast.BlockStmt, fset *token.FileSet) []Block {
	if body == nil || fset == nil {
		return nil
	}
	c := &blockCollector{fset: fset}
	c.stmts(body.List, 0)
	return c.blocks
}

// blockCollector accumulates blocks in source order. Indices handed around are
// 1-based (see Block.Parent), so 0 is "the function body itself".
type blockCollector struct {
	fset   *token.FileSet
	blocks []Block
	groups int
}

// add records one region and returns its 1-based index, for use as a parent.
func (c *blockCollector) add(kind BlockKind, start, end token.Pos, parent, group int, body []ast.Stmt) int {
	sp, ep := c.fset.Position(start), c.fset.Position(end)
	c.blocks = append(c.blocks, Block{
		Kind:       kind,
		StartLine:  sp.Line,
		StartCol:   sp.Column,
		EndLine:    ep.Line,
		EndCol:     ep.Column,
		Parent:     parent,
		Group:      group,
		Terminates: terminates(body),
	})
	return len(c.blocks)
}

// terminates reports whether a statement list ends in a way that stops control
// reaching the code after its conditional.
//
// Syntactic certainty only: a `return`, a `panic(...)`, or a branch statement.
// A call that never returns by convention (os.Exit, log.Fatal) is NOT counted —
// deciding that needs the type checker and the answer would be a guess for any
// project-local wrapper, and a wrong "terminates" turns sequential statements
// into alternatives (golden rule #7).
func terminates(body []ast.Stmt) bool {
	if len(body) == 0 {
		return false
	}
	switch last := body[len(body)-1].(type) {
	case *ast.ReturnStmt, *ast.BranchStmt:
		return true
	case *ast.ExprStmt:
		call, ok := last.X.(*ast.CallExpr)
		if !ok {
			return false
		}
		ident, ok := call.Fun.(*ast.Ident)
		return ok && ident.Name == "panic"
	case *ast.LabeledStmt:
		return terminates([]ast.Stmt{last.Stmt})
	default:
		return false
	}
}

// nextGroup mints the id shared by the arms of one conditional.
func (c *blockCollector) nextGroup() int {
	c.groups++
	return c.groups
}

func (c *blockCollector) stmts(list []ast.Stmt, parent int) {
	for _, stmt := range list {
		c.stmt(stmt, parent)
	}
}

// stmt records the regions a statement opens, then descends. Statements that
// open nothing are still scanned for function literals, whose bodies are
// regions of their own.
func (c *blockCollector) stmt(stmt ast.Stmt, parent int) {
	switch s := stmt.(type) {
	case nil:
		return

	case *ast.BlockStmt:
		idx := c.add(BlockPlain, s.Lbrace, s.Rbrace, parent, 0, s.List)
		c.stmts(s.List, idx)

	case *ast.IfStmt:
		c.ifChain(s, parent)

	case *ast.SwitchStmt:
		c.stmt(s.Init, parent)
		c.exprs(parent, s.Tag)
		c.clauses(s.Body, parent)

	case *ast.TypeSwitchStmt:
		c.stmt(s.Init, parent)
		c.stmt(s.Assign, parent)
		c.clauses(s.Body, parent)

	case *ast.SelectStmt:
		c.clauses(s.Body, parent)

	case *ast.ForStmt:
		c.stmt(s.Init, parent)
		c.exprs(parent, s.Cond)
		c.stmt(s.Post, parent)
		idx := c.add(BlockLoop, s.Body.Lbrace, s.Body.Rbrace, parent, 0, s.Body.List)
		c.stmts(s.Body.List, idx)

	case *ast.RangeStmt:
		c.exprs(parent, s.Key, s.Value, s.X)
		idx := c.add(BlockLoop, s.Body.Lbrace, s.Body.Rbrace, parent, 0, s.Body.List)
		c.stmts(s.Body.List, idx)

	case *ast.LabeledStmt:
		c.stmt(s.Stmt, parent)

	default:
		// Anything else (an expression, an assignment, a return, a defer) opens
		// no region of its own, but may carry a function literal that does.
		c.funcLits(stmt, parent)
	}
}

// ifChain records every arm of one if/else-if/else chain under a single group.
// An `else if` is an ALTERNATIVE to the arm before it, not a region nested in
// it, so walking the chain iteratively here is what keeps the group flat —
// descending into the else as an ordinary statement would nest each arm inside
// the previous one and lose the exclusivity relation.
func (c *blockCollector) ifChain(stmt *ast.IfStmt, parent int) {
	group := c.nextGroup()
	for cur := stmt; cur != nil; {
		// The init and condition run BEFORE the arm is entered and are not part
		// of it — which is exactly why block ranges carry columns, since on a
		// one-line `if` they share the arm's lines.
		c.stmt(cur.Init, parent)
		c.exprs(parent, cur.Cond)

		idx := c.add(BlockIf, cur.Body.Lbrace, cur.Body.Rbrace, parent, group, cur.Body.List)
		c.stmts(cur.Body.List, idx)

		switch alt := cur.Else.(type) {
		case *ast.IfStmt:
			cur = alt // `else if`: another arm of this same chain
		case *ast.BlockStmt:
			elseIdx := c.add(BlockElse, alt.Lbrace, alt.Rbrace, parent, group, alt.List)
			c.stmts(alt.List, elseIdx)
			cur = nil
		default:
			cur = nil
		}
	}
}

// clauses records the arms of a switch, type switch or select. Every arm shares
// one group, the `default` included: it is as exclusive with the others as any
// case, and the reason MethodBranch drops it (no verb to name it by) does not
// apply to a plain control-flow fact.
func (c *blockCollector) clauses(body *ast.BlockStmt, parent int) {
	if body == nil {
		return
	}
	group := c.nextGroup()
	for _, clause := range body.List {
		switch cl := clause.(type) {
		case *ast.CaseClause:
			kind := BlockCase
			if len(cl.List) == 0 {
				kind = BlockDefault
			}
			c.exprs(parent, cl.List...)
			// A clause has no braces: its region runs from the colon to the end
			// of its last statement, so the case expressions stay outside it.
			idx := c.add(kind, cl.Colon, cl.End(), parent, group, cl.Body)
			c.stmts(cl.Body, idx)
		case *ast.CommClause:
			kind := BlockCase
			if cl.Comm == nil {
				kind = BlockDefault
			}
			c.stmt(cl.Comm, parent)
			idx := c.add(kind, cl.Colon, cl.End(), parent, group, cl.Body)
			c.stmts(cl.Body, idx)
		}
	}
}

// exprs descends into expressions for function literals only.
func (c *blockCollector) exprs(parent int, list ...ast.Expr) {
	for _, expr := range list {
		if expr != nil {
			c.funcLits(expr, parent)
		}
	}
}

// funcLits records the body of every function literal written in a node that
// opens no region itself. A literal gets no Function record, so this is the
// only place its extent is recorded — and a literal reached from here is walked
// with c.stmt, so control flow INSIDE it is recorded too (issue #382's shape: a
// `switch r.Method` inside a closure handler).
func (c *blockCollector) funcLits(node ast.Node, parent int) {
	ast.Inspect(node, func(n ast.Node) bool {
		lit, ok := n.(*ast.FuncLit)
		if !ok || lit.Body == nil {
			return true
		}
		idx := c.add(BlockFuncLit, lit.Body.Lbrace, lit.Body.Rbrace, parent, 0, lit.Body.List)
		c.stmts(lit.Body.List, idx)
		return false // this literal's interior is walked by c.stmts, not here
	})
}

// bodyEndLine is the line of a declaration's closing brace, or 0 when it has no
// body. Paired with Function.Position (its opening line) this bounds the
// declaration, which is what distinguishes "written in this function" from
// "written in this file".
func bodyEndLine(fn *ast.FuncDecl, fset *token.FileSet) int {
	if fn == nil || fn.Body == nil || fset == nil {
		return 0
	}
	return fset.Position(fn.Body.Rbrace).Line
}
