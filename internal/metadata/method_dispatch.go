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
	"go/constant"
	"go/token"
	"go/types"
	"strings"
)

// httpMethodVerbs is the set of HTTP verbs a method-dispatch branch may name.
var httpMethodVerbs = map[string]bool{
	"GET": true, "POST": true, "PUT": true, "DELETE": true,
	"PATCH": true, "HEAD": true, "OPTIONS": true, "CONNECT": true, "TRACE": true,
}

// detectMethodDispatch finds control-flow dispatch on `r.Method` inside a
// handler body — a `switch r.Method { case http.MethodGet: … }` or an
// `if r.Method == http.MethodGet { … } else if … ` chain — and returns one
// MethodBranch per arm (the HTTP method(s) it handles and its source line
// range). Returns nil when the body doesn't dispatch on the request method, so
// ordinary handlers carry no MethodDispatch.
//
// Method values are resolved through the type checker's constant value
// (`info.Types[expr].Value`), so `http.MethodGet`, a bare `"GET"` literal, and
// a project-local `const MyGet = "GET"` all resolve uniformly. The request
// operand is identified by type (`*net/http.Request`), not by parameter name,
// so it is robust to any naming.
func detectMethodDispatch(body *ast.BlockStmt, info *types.Info, fset *token.FileSet) []MethodBranch {
	if body == nil || info == nil || fset == nil {
		return nil
	}
	var branches []MethodBranch
	ast.Inspect(body, func(n ast.Node) bool {
		switch stmt := n.(type) {
		case *ast.SwitchStmt:
			if !isRequestMethodExpr(stmt.Tag, info) {
				return true
			}
			for _, clause := range stmt.Body.List {
				cc, ok := clause.(*ast.CaseClause)
				if !ok || len(cc.List) == 0 {
					continue // the `default:` arm names no method
				}
				methods := caseMethodVerbs(cc.List, info)
				if len(methods) == 0 {
					continue
				}
				branches = append(branches, MethodBranch{
					Methods:   methods,
					StartLine: fset.Position(cc.Pos()).Line,
					EndLine:   fset.Position(cc.End()).Line,
				})
			}
			return false // switch handled; nested method-switches are not expected
		case *ast.IfStmt:
			if arms, ok := methodIfBranches(stmt, info, fset); ok {
				branches = append(branches, arms...)
				return false // the whole if/else-if chain is consumed here
			}
		}
		return true
	})
	return branches
}

// detectLitDispatch records the r.Method dispatch of every function literal in
// the file that has one, keyed by the literal's identity — the same
// `pkg.FuncLit:<stable position>` a closure route carries as its handler.
//
// A literal is not in File.Functions, so the dispatch of a closure written at a
// registration site (`http.HandleFunc("/x", func(w, r) { switch r.Method … })`)
// had no record reachable from the route's handler: it was folded into the
// ENCLOSING declaration's MethodDispatch, alongside the arms of every other
// literal in that function, and a consumer holding the closure's identity could
// neither find it nor tell the arms apart (issue #382).
//
// The whole file is walked rather than each declaration's body, because a
// registration is as often written inside a method (`func (s *Server) routes()`)
// as inside a plain function, and methods are not in File.Functions at all.
//
// Nested literals are recorded independently, and an outer literal's entry also
// covers the arms of a literal nested inside it. Both are true statements about
// where the dispatch is written, and the range each entry carries is what tells
// a consumer which arms are its own.
func detectLitDispatch(file *ast.File, info *types.Info, pkgName string, fset *token.FileSet, meta *Metadata) {
	if file == nil || info == nil || fset == nil || meta == nil {
		return
	}
	ast.Inspect(file, func(n ast.Node) bool {
		lit, ok := n.(*ast.FuncLit)
		if !ok || lit.Body == nil {
			return true
		}
		branches := detectMethodDispatch(lit.Body, info, fset)
		if len(branches) == 0 {
			return true
		}
		key := pkgName + "." + funcLitName(meta.stablePosition(lit.Pos(), fset))
		if meta.LitDispatch == nil {
			meta.LitDispatch = map[string]DispatchScope{}
		}
		meta.LitDispatch[key] = dispatchScope(lit.Body, branches, BlockFuncLit, fset, meta)
		return true
	})
}

// detectBodyDispatch is detectMethodDispatch plus the body range its arms have
// to be scoped against, or nil when the body does not dispatch on the request
// method. It is what a METHOD handler needs (issue #427): Method records a
// Position but no EndLine, so its arms would otherwise be comparable only
// against the whole file — and a handler routinely shares its file with the
// helpers it calls.
func detectBodyDispatch(body *ast.BlockStmt, info *types.Info, fset *token.FileSet, meta *Metadata) *DispatchScope {
	if body == nil || meta == nil {
		return nil
	}
	branches := detectMethodDispatch(body, info, fset)
	if len(branches) == 0 {
		return nil
	}
	scope := dispatchScope(body, branches, BlockPlain, fset, meta)
	return &scope
}

// dispatchScope pairs the arms with the absolute position of the body they were
// found in.
func dispatchScope(body *ast.BlockStmt, branches []MethodBranch, kind BlockKind, fset *token.FileSet, meta *Metadata) DispatchScope {
	start, end := fset.Position(body.Lbrace), fset.Position(body.Rbrace)
	return DispatchScope{
		File: meta.StringPool.Get(start.Filename),
		Body: Block{
			Kind:      kind,
			StartLine: start.Line, StartCol: start.Column,
			EndLine: end.Line, EndCol: end.Column,
		},
		Branches: branches,
	}
}

// isRequestMethodExpr reports whether expr is `<request>.Method` where
// <request> is typed `*net/http.Request`.
func isRequestMethodExpr(expr ast.Expr, info *types.Info) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok || sel.Sel == nil || sel.Sel.Name != "Method" {
		return false
	}
	t := info.TypeOf(sel.X)
	return t != nil && t.String() == "*net/http.Request"
}

// caseMethodVerbs resolves the expressions of a `case` clause to HTTP verbs
// (a single clause may list several: `case http.MethodGet, http.MethodHead:`).
func caseMethodVerbs(exprs []ast.Expr, info *types.Info) []string {
	var out []string
	for _, e := range exprs {
		if v := constMethodVerb(e, info); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// constMethodVerb returns the upper-cased HTTP verb an expression constant-folds
// to, or "" if it is not a constant string naming a known method.
func constMethodVerb(e ast.Expr, info *types.Info) string {
	tv, ok := info.Types[e]
	if !ok || tv.Value == nil || tv.Value.Kind() != constant.String {
		return ""
	}
	v := strings.ToUpper(constant.StringVal(tv.Value))
	if httpMethodVerbs[v] {
		return v
	}
	return ""
}

// methodIfBranches walks an `if r.Method == X { … } else if r.Method == Y { … }`
// chain, returning one MethodBranch per matching arm. ok is false when the head
// `if` does not compare the request method (so the caller keeps descending).
func methodIfBranches(ifStmt *ast.IfStmt, info *types.Info, fset *token.FileSet) (arms []MethodBranch, ok bool) {
	for cur := ifStmt; cur != nil; {
		if verb := ifMethodEqVerb(cur.Cond, info); verb != "" && cur.Body != nil {
			ok = true
			arms = append(arms, MethodBranch{
				Methods:   []string{verb},
				StartLine: fset.Position(cur.Body.Pos()).Line,
				EndLine:   fset.Position(cur.Body.End()).Line,
			})
		}
		if elseIf, isIf := cur.Else.(*ast.IfStmt); isIf {
			cur = elseIf
		} else {
			cur = nil
		}
	}
	return arms, ok
}

// ifMethodEqVerb returns the verb when cond is `r.Method == <method>` (in either
// operand order), else "".
func ifMethodEqVerb(cond ast.Expr, info *types.Info) string {
	be, ok := cond.(*ast.BinaryExpr)
	if !ok || be.Op != token.EQL {
		return ""
	}
	switch {
	case isRequestMethodExpr(be.X, info):
		return constMethodVerb(be.Y, info)
	case isRequestMethodExpr(be.Y, info):
		return constMethodVerb(be.X, info)
	default:
		return ""
	}
}
