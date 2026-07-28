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
}

func newParamIndex(meta *metadata.Metadata) *paramIndex {
	return &paramIndex{meta: meta, byMethod: map[string][]string{}}
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
