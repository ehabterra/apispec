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
	"regexp"

	"github.com/ehabterra/apispec/internal/metadata"
)

// requestOrigin is what a parameter read's receiver was shown to come from.
type requestOrigin int

const (
	// requestOriginUnknown could not be placed, so the parameter is kept:
	// only what provably is not the request is dropped (golden rule #7).
	requestOriginUnknown requestOrigin = iota
	// requestOriginRequest traces to a value of a RequestContext type.
	requestOriginRequest
	// requestOriginElsewhere was built where the request never reached it —
	// `url.Parse(cfg)`, `url.Values{}`, the query of a URL the handler made.
	requestOriginElsewhere
)

// maxRequestOriginHops bounds the walk back through chains, assignments,
// selectors and parameter bindings; it is also what ends a cycle.
const maxRequestOriginHops = 16

// readsOutsideRequest reports whether a parameter read is PROVABLY made on
// something other than the request, so it names no parameter of this
// operation (issue #552).
//
// `url.Values` sits on both sides the way `http.Header` does (#543): it is the
// request's query, and also the query of any URL a handler parses, builds or
// forwards. `Get` on it matched on the type alone, so gitea's
// `u.Query().Get("clientname")` — read off a Redis connection URI — was
// documented as a query parameter of every operation that reached the code
// that opens the connection.
//
// The value is followed back through what each call is made on: a method's
// result is its receiver's (`r.URL.Query()` → `r.URL` → `r`), a field its
// owner's, a variable its assignment's, a parameter its caller's argument.
// Anything whose static type is a configured RequestContext type is the
// request. A composite literal, or a plain function none of whose arguments
// trace to the request (`url.Parse(connString)`), is elsewhere — and only
// that drops the read.
func (p *ParamPatternMatcherImpl) readsOutsideRequest(node TrackerNodeInterface) bool {
	if p == nil || node == nil || node.GetEdge() == nil || p.cfg == nil {
		return false
	}
	requestTypes := compileAll(p.cfg.Framework.RequestContext.TypeRegexes)
	if len(requestTypes) == 0 {
		return false // no request types configured: nothing to prove against
	}
	w := requestOriginWalker{cp: p.contextProvider, requestTypes: requestTypes}
	return w.callOrigin(node.GetEdge(), node, 0) == requestOriginElsewhere
}

// compileAll compiles the patterns that are valid, skipping the rest.
func compileAll(patterns []string) []*regexp.Regexp {
	out := make([]*regexp.Regexp, 0, len(patterns))
	for _, pat := range patterns {
		if re, err := cachedRegex(pat); err == nil {
			out = append(out, re)
		}
	}
	return out
}

type requestOriginWalker struct {
	cp           ContextProvider
	requestTypes []*regexp.Regexp
}

// callOrigin places what a call is made on: the root of its chain, then that
// root's recorded receiver.
func (w requestOriginWalker) callOrigin(call *metadata.CallGraphEdge, node TrackerNodeInterface, hops int) requestOrigin {
	for call != nil && call.ChainParent != nil && hops < maxRequestOriginHops {
		call = call.ChainParent
		hops++
	}
	if call == nil || call.Receiver == nil || hops >= maxRequestOriginHops {
		return requestOriginUnknown
	}
	return w.valueOrigin(call.Receiver, node, hops+1)
}

// valueOrigin places one value, evaluated in node's frame.
func (w requestOriginWalker) valueOrigin(arg *metadata.CallArgument, node TrackerNodeInterface, hops int) requestOrigin {
	for arg != nil && (arg.GetKind() == metadata.KindUnary || arg.GetKind() == metadata.KindStar || arg.GetKind() == metadata.KindParen) {
		arg = arg.X
	}
	if arg == nil || node == nil || node.GetEdge() == nil || hops >= maxRequestOriginHops {
		return requestOriginUnknown
	}
	if w.isRequestType(arg) {
		return requestOriginRequest
	}
	edge := node.GetEdge()
	switch arg.GetKind() {
	case metadata.KindIdent:
		if rhs := latestAssignment(w.cp, edge, arg.GetName()); rhs != nil {
			return w.valueOrigin(rhs, node, hops+1)
		}
		if resolved, at := resolveArgThroughParams(arg, node); resolved != nil && resolved != arg && at != nil {
			return w.valueOrigin(resolved, at, hops+1)
		}
		// A parameter the tree path does not bind. Since #546 a helper's calls
		// on a bound parameter hang under the argument's PRODUCER rather than
		// under the helper's call site, so the parent frame names the
		// producer's parameters, not this one. The call graph still says what
		// every caller passes for it — the frame-blind rung paramValueFromCallSites
		// reads for a path.
		if origin, ok := w.paramOriginFromCallers(arg.GetName(), edge, hops); ok {
			return origin
		}
		// Neither assigned here nor a bound parameter: a PACKAGE-level
		// variable is set at start-up, never per request — a connection
		// string, a base URL from configuration.
		if w.isPackageVar(arg.GetPkg(), arg.GetName()) {
			return requestOriginElsewhere
		}
		return requestOriginUnknown
	case metadata.KindSelector:
		// `setting.Cache.Conn`: a package-level variable reached through its
		// package qualifier, which is as much configuration as a bare one.
		if x := arg.X; x != nil && x.GetKind() == metadata.KindIdent && x.GetType() == "" && arg.Sel != nil {
			if w.isPackageVar(arg.GetPkg(), arg.Sel.GetName()) {
				return requestOriginElsewhere
			}
		}
		return w.valueOrigin(arg.X, node, hops+1)
	case metadata.KindCompositeLit:
		return requestOriginElsewhere
	case metadata.KindCall:
		fun := arg.Fun
		// A METHOD's result belongs to its receiver; handleSelector records a
		// receiver type only for a method, which is what tells `u.Query()` from
		// the package-qualified `url.Parse(…)`.
		if fun != nil && fun.GetKind() == metadata.KindSelector && fun.ReceiverType != nil {
			return w.valueOrigin(fun.X, node, hops+1)
		}
		// A plain function builds a NEW value. It is the request's only when
		// an argument shows it was handed the request's data —
		// `url.ParseQuery(r.URL.RawQuery)` — and otherwise it is not the
		// request's query, whether or not every argument can be placed:
		// gitea's `ToRedisURI(connection)` is handed a connection string
		// through several configuration parameters, and still builds a URL
		// that is not the request's.
		//
		// This is the rule the write side already applies to a destination
		// (reachesWriter): a constructor none of whose arguments reach the
		// writer builds something that is not the response, even when an
		// argument cannot be followed — `bytes.NewBufferString(s)` is a
		// buffer whatever s is.
		for _, a := range arg.Args {
			if w.valueOrigin(a, node, hops+1) == requestOriginRequest {
				return requestOriginRequest
			}
		}
		return requestOriginElsewhere
	case metadata.KindLiteral:
		return requestOriginElsewhere
	}
	return requestOriginUnknown
}

// paramOriginFromCallers places a parameter of the function edge is written in
// by what its callers pass for it: the request if any caller hands it the
// request's data, elsewhere if every caller hands it something built
// elsewhere, and no answer otherwise — including when some caller does not
// bind the name at all, which means it is not a parameter.
func (w requestOriginWalker) paramOriginFromCallers(name string, edge *metadata.CallGraphEdge, hops int) (requestOrigin, bool) {
	impl, ok := w.cp.(*ContextProviderImpl)
	if !ok || impl.meta == nil || edge == nil || name == "" || hops >= maxRequestOriginHops {
		return requestOriginUnknown, false
	}
	callers := impl.meta.Callees[edge.Caller.BaseID()]
	if len(callers) == 0 {
		return requestOriginUnknown, false
	}
	elsewhere := true
	for _, call := range callers {
		bound, ok := call.ParamArgMap[name]
		if !ok {
			return requestOriginUnknown, false
		}
		switch w.valueOrigin(&bound, callSiteFrame{edge: call}, hops+1) {
		case requestOriginRequest:
			return requestOriginRequest, true
		case requestOriginUnknown:
			elsewhere = false
		}
	}
	if elsewhere {
		return requestOriginElsewhere, true
	}
	return requestOriginUnknown, false
}

// callSiteFrame evaluates a value at a call site with no tree path above it:
// assignments resolve in the caller's scope, and a parameter of the caller is
// followed through the call graph again rather than through a parent node.
type callSiteFrame struct{ edge *metadata.CallGraphEdge }

func (f callSiteFrame) GetKey() string                      { return "" }
func (f callSiteFrame) GetParent() TrackerNodeInterface     { return nil }
func (f callSiteFrame) GetChildren() []TrackerNodeInterface { return nil }
func (f callSiteFrame) GetEdge() *metadata.CallGraphEdge    { return f.edge }
func (f callSiteFrame) GetArgument() *metadata.CallArgument { return nil }
func (f callSiteFrame) GetTypeParamMap() map[string]string  { return nil }

// isPackageVar reports whether pkg declares a package-level variable or
// constant named name.
func (w requestOriginWalker) isPackageVar(pkg, name string) bool {
	impl, ok := w.cp.(*ContextProviderImpl)
	if !ok || impl.meta == nil || pkg == "" || name == "" {
		return false
	}
	p, ok := impl.meta.Packages[pkg]
	if !ok || p == nil {
		return false
	}
	for _, fileName := range impl.meta.SortedFileNames(pkg) {
		if f := p.Files[fileName]; f != nil {
			if _, ok := f.Variables[name]; ok {
				return true
			}
		}
	}
	return false
}

// isRequestType reports whether the value's static type is a configured
// RequestContext type.
func (w requestOriginWalker) isRequestType(arg *metadata.CallArgument) bool {
	for _, t := range []string{arg.GetResolvedType(), arg.GetType()} {
		if t != "" && matchAny(w.requestTypes, t) {
			return true
		}
	}
	return false
}
