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
	"slices"
	"strings"

	"github.com/ehabterra/apispec/internal/metadata"
)

// paramIndex answers the question wrapper detection turns on: does this argument
// come from one of the enclosing method's own PARAMETERS, and which one?
//
// That is the whole distinction between a router wrapper and an ordinary
// registration helper. `r.chiRouter.Method(methods, r.getPattern(pattern), h)`
// forwards `methods`, `pattern` and `h` — so the method is a registrar whose
// caller supplies the route. `r.Get("/users", listUsers)` names them, and
// describes one route rather than a way of registering routes (issue #235).
type paramIndex struct {
	meta *metadata.Metadata
	// byMethod caches a method's parameter names in declaration order.
	byMethod map[string][]string
	// variadicAt caches, per method, whether each parameter position is
	// variadic — see isVariadic.
	variadic map[string][]bool
}

func newParamIndex(meta *metadata.Metadata) *paramIndex {
	return &paramIndex{
		meta:     meta,
		byMethod: map[string][]string{},
		variadic: map[string][]bool{},
	}
}

// isVariadic reports whether the wrapper method's parameter at idx is declared
// `...T` rather than `[]T`.
//
// The two are a genuinely different fact and the derivation turns on it: a
// wrapper forwarding a variadic chain — `func (r *Router) Get(path string,
// handlers ...gin.HandlerFunc)` calling `r.engine.GET(path, handlers...)` —
// derives from an inner call of exactly TWO arguments, so the handler's position
// is recorded as a fixed index 1. A later three-argument call through the wrapper
// then reads index 1 and finds the MIDDLEWARE, documenting the whole operation
// from it: the middleware's doc comment as the summary, its parameter reads as
// the operation's, and the real handler's body and responses missing
// (issues #416, #386).
//
// Issue #416 filed this as blocked on a metadata gap, on the grounds that
// `...T` collapses to `[]T` in the type model and survives only in the rendered
// signature string — which golden rule #2 forbids parsing. That is true of
// TypeRef, and it is not the whole picture: the parameter's own CallArgument in
// the recorded signature keeps KindEllipsis, distinct from the KindArrayType a
// `[]T` parameter gets. The fact is already recorded, on the parameter rather
// than on the type, which is where the issue itself guessed it would belong.
func (p *paramIndex) isVariadic(w *wrapperMethod, idx int) bool {
	if p == nil || w == nil || idx < 0 {
		return false
	}
	key := w.fqRecv() + "." + w.name
	flags, ok := p.variadic[key]
	if !ok {
		if m := p.methodOf(w); m != nil {
			flags = make([]bool, 0, len(m.Signature.Args))
			for _, param := range m.Signature.Args {
				flags = append(flags, param.GetKind() == metadata.KindEllipsis)
			}
		}
		p.variadic[key] = flags
	}
	return idx < len(flags) && flags[idx]
}

// wrapperExprDepth bounds the walk over one argument expression. Real forwarding
// is shallow (`r.getPattern(pattern)`, `h[len(h)-1]`); the bound only stops a
// pathological expression.
const wrapperExprDepth = 6

// wrapperAssignHops bounds how far a local is followed back towards a parameter.
// A wrapper that unwraps a variadic `...any` needs two — gitea's does
// `fn := h[len(h)-1].(type)` and then `handlerFunc = fn` — and a few more cost
// nothing, since each hop is a map lookup on assignments already recorded.
const wrapperAssignHops = 4

// indexOf returns the parameter position an argument comes from.
//
// Three shapes count, all of them "the caller supplied this value":
//
//	Get(pattern, h...)          -> the argument IS the parameter
//	r.getPattern(pattern)       -> the parameter is passed through a call
//	h[len(h)-1] via a local var -> the parameter reaches it by assignment
//
// A value the method made up itself resolves to nothing, which is what keeps an
// ordinary registration helper from being read as a wrapper.
func (p *paramIndex) indexOf(w *wrapperMethod, arg *metadata.CallArgument) (int, bool) {
	if p == nil || w == nil || arg == nil {
		return 0, false
	}
	params := p.paramsOf(w)
	if len(params) == 0 {
		return 0, false
	}

	// Directly, through the expression the argument is built from, and then back
	// through the assignments that produced any local it mentions. A wrapper that
	// unwraps a variadic handler takes two hops to reach the parameter:
	// `handlerFunc = fn`, `fn = h[len(h)-1].(T)`.
	names := identsIn(arg, wrapperExprDepth)
	seen := map[string]bool{}
	for hop := 0; hop < wrapperAssignHops && len(names) > 0; hop++ {
		if i, ok := matchParam(params, names); ok {
			return i, true
		}
		var next []string
		for _, name := range names {
			if name == "" || seen[name] {
				continue
			}
			seen[name] = true
			for _, assign := range p.assignmentsOf(w, name) {
				value := assign.Value
				next = append(next, identsIn(&value, wrapperExprDepth)...)
			}
		}
		names = next
	}
	return 0, false
}

// valueIndexOf returns the parameter position an argument IS, where indexOf
// settles for one it mentions.
//
// indexOf's rule — any identifier in the expression, through any assignment —
// is right for the roles a caller's value merely flows into: the path reaches
// the inner call through `r.getPattern(pattern)`, the handler through
// `h[len(h)-1]` and two locals, and a status through whatever the method computes
// from it (an error mapper's `api.Status` is resolved from the caller's error at
// the call site). It is wrong for a VALUE role, where the call site substitutes
// its own argument for the inner one wholesale:
//
//	func (rt *R) normalize(w http.ResponseWriter, req *http.Request) {
//		p := req.URL.Path
//		http.Error(w, "bad "+p, http.StatusBadRequest)
//	}
//
// The body mentions `req`, so the derived pattern read every caller's request as
// the response body, and a large real project documented *http.Request under
// three of its operations (issue #659). A message built from a parameter, a
// field of it, or a call over it is a value the method made; only the parameter
// itself — directly, through a plain local, its address or dereference, a type
// assertion or a conversion — is one the caller supplies.
//
// A local qualifies only when EVERY assignment to it does, and to the same
// parameter. One branch that builds its own value (`v := http.StatusText(code)`
// beside `v = contents[0]`) leaves the body undecidable from the signature, and
// declining it is the honest answer (golden rule #7).
func (p *paramIndex) valueIndexOf(w *wrapperMethod, arg *metadata.CallArgument) (int, bool) {
	if p == nil || w == nil || arg == nil {
		return 0, false
	}
	params := p.paramsOf(w)
	if len(params) == 0 {
		return 0, false
	}
	return p.identityOf(w, params, arg, wrapperAssignHops, map[string]bool{})
}

// identityOf resolves one value expression for valueIndexOf. onPath holds the
// locals on the current assignment path, so a cycle (`a = b; b = a`) stops while
// two branches through the same local both still resolve.
//
// A parameter is a local whose first assignment is the caller's argument, and
// it is held to the same rule as any other: the method may overwrite it.
// `v = map[string]any{"data": v}; Encode(v)` sends the map, and returning the
// parameter on its name alone documented the caller's bare value as the body
// (CodeRabbit on #660). So a reassigned parameter qualifies only while every
// assignment hands on the parameter itself — a cycle back to its own name is
// that — and a conditional default (`if v == nil { v = []T{} }`) declines like
// any local with one branch that builds its own value.
func (p *paramIndex) identityOf(w *wrapperMethod, params []string, arg *metadata.CallArgument, hops int, onPath map[string]bool) (int, bool) {
	name := valueOperandIdent(arg)
	if name == "" {
		return 0, false
	}
	param := slices.Index(params, name)
	if onPath[name] {
		return param, param >= 0
	}
	assigns := p.assignmentsOf(w, name)
	if len(assigns) == 0 {
		return param, param >= 0
	}
	if hops <= 0 {
		return 0, false
	}
	onPath[name] = true
	defer delete(onPath, name)
	found := param
	for _, assign := range assigns {
		value := assign.Value
		i, ok := p.identityOf(w, params, &value, hops-1, onPath)
		if !ok || (found >= 0 && i != found) {
			return 0, false
		}
		found = i
	}
	return found, true
}

// valueOperandIdent returns the identifier an expression passes on unchanged,
// and "" for one that computes a new value. `&v`, `*v`, `(v)`, `v.(T)` and
// `T(v)` all hand on v itself; a selector into v, an index into it, arithmetic
// or a call over it do not.
func valueOperandIdent(arg *metadata.CallArgument) string {
	for depth := 0; arg != nil && depth < wrapperExprDepth; depth++ {
		switch arg.GetKind() {
		case metadata.KindIdent:
			return arg.GetName()
		case metadata.KindUnary:
			if arg.GetValue() != "&" {
				return ""
			}
			arg = arg.X
		case metadata.KindStar, metadata.KindParen, metadata.KindTypeAssert:
			arg = arg.X
		case metadata.KindTypeConversion:
			inner, ok := conversionOperand(arg)
			if !ok {
				return ""
			}
			arg = inner
		default:
			return ""
		}
	}
	return ""
}

// paramsOf returns a method's parameter names in declaration order.
func (p *paramIndex) paramsOf(w *wrapperMethod) []string {
	key := w.fqRecv() + "." + w.name
	if cached, ok := p.byMethod[key]; ok {
		return cached
	}

	var names []string
	if m := p.methodOf(w); m != nil {
		for _, param := range m.Signature.Args {
			names = append(names, param.GetName())
		}
	}
	p.byMethod[key] = names
	return names
}

// assignmentsOf returns the assignments to a local inside the wrapper method.
//
// A method's assignments are recorded on the METHOD, not on a function of the
// same name, so the type's method list is where to look; the function table is a
// fallback for the shapes recorded there instead.
func (p *paramIndex) assignmentsOf(w *wrapperMethod, name string) []metadata.Assignment {
	if name == "" {
		return nil
	}
	if m := p.methodOf(w); m != nil {
		if assigns := m.AssignmentMap[name]; len(assigns) > 0 {
			return assigns
		}
	}
	if fn := findFunctionByName(p.meta, w.pkg, w.name); fn != nil {
		return fn.AssignmentMap[name]
	}
	return nil
}

// methodOf finds the declaration of the wrapper method.
func (p *paramIndex) methodOf(w *wrapperMethod) *metadata.Method {
	pkg, ok := p.meta.Packages[w.pkg]
	if !ok {
		return nil
	}
	bare := strings.TrimPrefix(w.recvType, "*")
	for _, fileName := range p.meta.SortedFileNames(w.pkg) {
		file := pkg.Files[fileName]
		if file == nil {
			continue
		}
		typ, ok := file.Types[bare]
		if !ok {
			continue
		}
		for i := range typ.Methods {
			if p.meta.StringPool.GetString(typ.Methods[i].Name) == w.name {
				return &typ.Methods[i]
			}
		}
	}
	return nil
}

// matchParam returns the position of the first name that is a parameter.
func matchParam(params, names []string) (int, bool) {
	for _, name := range names {
		if name == "" {
			continue
		}
		for i, param := range params {
			if param == name {
				return i, true
			}
		}
	}
	return 0, false
}

// identsIn collects the identifier names an expression mentions, outermost
// first — enough to see the parameter inside `r.getPattern(pattern)` or
// `h[len(h)-1]` without interpreting either.
func identsIn(arg *metadata.CallArgument, depth int) []string {
	if arg == nil || depth <= 0 {
		return nil
	}
	var out []string
	if name := arg.GetName(); name != "" {
		out = append(out, name)
	}
	for _, child := range []*metadata.CallArgument{arg.X, arg.Sel, arg.Fun} {
		out = append(out, identsIn(child, depth-1)...)
	}
	for _, a := range arg.Args {
		out = append(out, identsIn(a, depth-1)...)
	}
	return out
}
