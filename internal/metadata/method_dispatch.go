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
