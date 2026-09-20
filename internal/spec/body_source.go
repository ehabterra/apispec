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
	"strings"

	"github.com/ehabterra/apispec/internal/metadata"
)

// bodySourceResolver determines whether a CallArgument can be traced back to
// a request-context body accessor as configured by
// FrameworkConfig.RequestContext. This is the "request-origin taint" check
// used to disambiguate generic decoders (json.Decode, json.Unmarshal,
// render.DecodeJSON, ...) from non-request decoding.
type bodySourceResolver struct {
	contextProvider ContextProvider
	typeREs         []*regexp.Regexp
	accessorREs     []*regexp.Regexp
	readers         []BodyReader
}

// matchBodyReader reports whether a call is a configured body reader, and where
// its source argument sits. An empty PkgRegex matches any package.
//
// A negative SourceArgIndex declines the reader rather than being returned: the
// value comes from a user's config file, and every caller indexes with it.
func (r *bodySourceResolver) matchBodyReader(calleeFunc, calleePkg string) (int, bool) {
	if calleeFunc == "" {
		return 0, false
	}
	for _, reader := range r.readers {
		if reader.SourceArgIndex < 0 {
			continue
		}
		if reader.CallRegex != "" {
			re, err := cachedRegex(reader.CallRegex)
			if err != nil || !re.MatchString(calleeFunc) {
				continue
			}
		}
		if reader.PkgRegex != "" {
			re, err := cachedRegex(reader.PkgRegex)
			if err != nil || !re.MatchString(calleePkg) {
				continue
			}
		}
		return reader.SourceArgIndex, true
	}
	return 0, false
}

// newBodySourceResolver compiles the configured regexes once. Returns a
// resolver whose Enabled() reports false if no RequestContext is configured;
// in that case callers should fall back to the prior receiver-only matching
// behaviour.
func newBodySourceResolver(cfg *APISpecConfig, contextProvider ContextProvider) *bodySourceResolver {
	r := &bodySourceResolver{contextProvider: contextProvider}
	if cfg == nil {
		return r
	}
	for _, p := range cfg.Framework.RequestContext.TypeRegexes {
		if re, err := cachedRegex(p); err == nil {
			r.typeREs = append(r.typeREs, re)
		}
	}
	for _, p := range cfg.Framework.RequestContext.BodyAccessors {
		if re, err := cachedRegex(p); err == nil {
			r.accessorREs = append(r.accessorREs, re)
		}
	}
	r.readers = cfg.Framework.RequestContext.BodyReaders
	return r
}

// Enabled reports whether RequestContext is configured. When false, the
// resolver should be skipped and matchers retain their prior behaviour.
func (r *bodySourceResolver) Enabled() bool {
	return r != nil && len(r.typeREs) > 0
}

// IsRequestSource returns true if the given source argument at the given call
// site can be traced through selectors, idents, assignments and parameter
// boundaries to a body accessor on a request-context typed root.
//
// When Enabled() is false, IsRequestSource returns true (permissive) so
// callers can use it unconditionally.
func (r *bodySourceResolver) IsRequestSource(arg *metadata.CallArgument, edge *metadata.CallGraphEdge) bool {
	if !r.Enabled() {
		return true
	}
	visited := make(map[string]bool, 4)
	return r.check(arg, edge, visited)
}

// chainSegment is one accessor in a selector/method chain. isCall=true marks
// method calls, which render as "Name()" so that BodyAccessors regexes can
// distinguish a field from a method.
type chainSegment struct {
	name   string
	isCall bool
	// node is the expression this segment ends at — `c.Req` for the `Req` of
	// `c.Req.Body` — so the chain can be asked what type it has PART WAY along.
	// See chainMatches.
	node *metadata.CallArgument
}

// chainType returns the type of the expression a segment ends at.
func (s chainSegment) chainType() string {
	if s.node == nil {
		return ""
	}
	if t := s.node.GetResolvedType(); t != "" {
		return t
	}
	return s.node.GetType()
}

func (r *bodySourceResolver) check(arg *metadata.CallArgument, edge *metadata.CallGraphEdge, visited map[string]bool) bool {
	if arg == nil || edge == nil {
		return false
	}

	// Strip address-of and deref so &x and *x trace the same as x.
	for arg != nil && (arg.GetKind() == metadata.KindUnary || arg.GetKind() == metadata.KindStar) {
		arg = arg.X
	}
	if arg == nil {
		return false
	}

	key := arg.ID()
	if visited[key] {
		return false
	}
	visited[key] = true

	switch arg.GetKind() {
	case metadata.KindSelector, metadata.KindCall:
		root, segs := peelAccessorChain(arg)
		if root != nil && r.chainMatches(root, segs, edge) {
			return true
		}
		// Root may itself be a variable whose origin yields a body source —
		// e.g. body := r.Body; json.NewDecoder(body).Decode(...).
		if root != nil && root.GetKind() == metadata.KindIdent && r.checkIdent(root, edge, visited) {
			return true
		}
		return false

	case metadata.KindIdent:
		return r.checkIdent(arg, edge, visited)
	}
	return false
}

// chainMatches reports whether the (root, segs) chain points at a request
// body: some prefix of the chain must have a type matching one of the
// configured TypeRegexes, and the accessors that FOLLOW that prefix must match
// one of the configured BodyAccessors.
//
// The request is not always the root. A house context holds it in a field —
// `c.Req.Body`, which is gitea's shape — so every prefix is offered its turn as
// the request rather than the root alone. Reading only the root's type answered
// "this decoder reads nothing from the request" for every project that owns a
// context type, which stayed invisible because the two callers both had a
// reason not to notice: at extraction time the derived wrapper pattern
// documents the body instead, and the derivation itself did not consult this
// check at all — so it could not tell a context's own `c.Req.Body` from an
// outbound `http.Response` decode, and read both as request bodies (#513).
//
// Walking past the root is allowed only when the root is something the handler
// was GIVEN — a parameter or a receiver — and not a local it built. Both of
// these reach an `*http.Request` one accessor in, and only the first is the
// request being served:
//
//	c.Req.Body           // c is the receiver: the context holds our request
//	resp.Request.Body    // resp came from http.Get: the request we SENT
//
// A local's own type is still read at i == 0, which is how
// `req := r; req.Body` keeps working — and is why an outbound request assigned
// to a local is accepted there, as it always has been. Closing that needs
// provenance run to the handler's own parameter, which is what #513 describes
// in full and this does not attempt.
func (r *bodySourceResolver) chainMatches(root *metadata.CallArgument, segs []chainSegment, edge *metadata.CallGraphEdge) bool {
	if root == nil || root.GetKind() != metadata.KindIdent || len(segs) == 0 {
		return false
	}
	// i is how many leading accessors belong to the request's own path: 0
	// offers the root, 1 the root plus one accessor, and so on. The last
	// segment is never offered, since something has to remain to match as the
	// body accessor.
	rootIsGiven := len(assignmentsAt(r.contextProvider, edge, root.GetName())) == 0
	for i := range segs {
		typ := ""
		if i == 0 {
			// The root alone, where a traced origin can supply the type a bare
			// ident does not carry.
			typ = r.identType(root, edge)
		} else if rootIsGiven {
			typ = segs[i-1].chainType()
		}
		if typ == "" || !matchAny(r.typeREs, typ) {
			continue
		}
		if matchAny(r.accessorREs, accessorString(segs[i:])) {
			return true
		}
	}
	return false
}

// checkIdent traces an ident through assignments and parameter boundaries to
// see whether its value originates at a request body accessor.
func (r *bodySourceResolver) checkIdent(arg *metadata.CallArgument, edge *metadata.CallGraphEdge, visited map[string]bool) bool {
	if arg == nil || arg.GetKind() != metadata.KindIdent || edge == nil {
		return false
	}
	name := arg.GetName()
	if name == "" {
		return false
	}

	callerName := r.contextProvider.GetString(edge.Caller.Name)
	callerPkg := r.contextProvider.GetString(edge.Caller.Pkg)

	// 1) Local assignments: src := r.Body. latestAssignment checks the call
	// edge's map first, then the enclosing handler function's scope — a source
	// assigned in the handler body lives on the Function, not on the Decode call
	// edge. Latest-wins, consistent with TraceVariableOrigin.
	assigns := assignmentsAt(r.contextProvider, edge, name)
	if len(assigns) > 0 {
		last := assigns[len(assigns)-1]
		rhs := last.Value
		if rhs.Meta == nil {
			rhs.Meta = arg.Meta
		}
		if r.check(&rhs, edge, visited) {
			return true
		}
		// The bytes may have been READ from the request rather than named by
		// it: `data, _ := io.ReadAll(r.Body)` and then `json.Unmarshal(data,
		// &v)`. What a configured reader returns is whatever it was reading,
		// so the question passes to that call's source argument.
		//
		// Read off the assignment's recorded callee, the way the response side
		// unwraps a serializer (BodyTransforms, issue #195), rather than by
		// walking into every call the value came from: naming the readers keeps
		// this a regex check on a field already recorded, where the general walk
		// ran a full origin trace per argument and cost 10% of a run on gitea.
		if idx, ok := r.matchBodyReader(last.CalleeFunc, last.CalleePkg); ok &&
			rhs.GetKind() == metadata.KindCall && idx < len(rhs.Args) {
			if r.check(rhs.Args[idx], edge, visited) {
				return true
			}
		}
	}

	// 2) The ident may be a parameter — trace it up the call graph through
	// ParamArgMap so a helper like decode(r, &v) can be resolved from its
	// caller's argument.
	if name == callerName {
		// guard against pathological self-recursion
		return false
	}
	if meta := r.metadata(); meta != nil {
		_, _, originArg, _ := metadata.TraceVariableOrigin(name, callerName, callerPkg, meta)
		if originArg != nil && originArg != arg {
			// originArg is a synthesized ident with the resolved type; if its
			// type is already a request context, the ident itself is the
			// context (think handler receiving r *http.Request and passing r
			// directly), which is not a body source.
			if originType := originArg.GetType(); originType != "" && matchAny(r.typeREs, originType) {
				// The variable IS the request — not the request body.
				return false
			}
		}
	}

	return false
}

// identType returns the ident's declared type, preferring the resolved type
// when available. When the type is not on the CallArgument itself, it tries
// to recover it from local assignments or via TraceVariableOrigin.
func (r *bodySourceResolver) identType(arg *metadata.CallArgument, edge *metadata.CallGraphEdge) string {
	if arg == nil {
		return ""
	}
	if t := arg.GetResolvedType(); t != "" {
		return t
	}
	if t := arg.GetType(); t != "" {
		return t
	}
	// Try assignments visible at this call site.
	if edge != nil && len(edge.AssignmentMap) > 0 {
		if assigns, ok := edge.AssignmentMap[arg.GetName()]; ok {
			for i := len(assigns) - 1; i >= 0; i-- {
				if t := r.contextProvider.GetString(assigns[i].ConcreteType); t != "" {
					return t
				}
			}
		}
	}
	// Fall back to a call-graph trace.
	if meta := r.metadata(); meta != nil && edge != nil {
		callerName := r.contextProvider.GetString(edge.Caller.Name)
		callerPkg := r.contextProvider.GetString(edge.Caller.Pkg)
		_, _, originArg, _ := metadata.TraceVariableOrigin(arg.GetName(), callerName, callerPkg, meta)
		if originArg != nil {
			return originArg.GetType()
		}
	}
	return ""
}

func (r *bodySourceResolver) metadata() *metadata.Metadata {
	if impl, ok := r.contextProvider.(*ContextProviderImpl); ok {
		return impl.meta
	}
	return nil
}

// peelAccessorChain walks a selector/method-call chain and returns the root
// ident plus the ordered accessor segments (root→leaf). Returns (nil, nil)
// for shapes it cannot decompose.
func peelAccessorChain(arg *metadata.CallArgument) (*metadata.CallArgument, []chainSegment) {
	var segs []chainSegment
	cur := arg
	for cur != nil {
		switch cur.GetKind() {
		case metadata.KindSelector:
			name := ""
			if cur.Sel != nil {
				name = cur.Sel.GetName()
			}
			segs = append(segs, chainSegment{name: name, node: cur})
			cur = cur.X
		case metadata.KindCall:
			// A method call is a selector wrapped in a call.
			if cur.Fun == nil || cur.Fun.GetKind() != metadata.KindSelector {
				return nil, nil
			}
			name := ""
			if cur.Fun.Sel != nil {
				name = cur.Fun.Sel.GetName()
			}
			segs = append(segs, chainSegment{name: name, isCall: true, node: cur})
			cur = cur.Fun.X
		case metadata.KindIdent:
			// Reverse to root→leaf order.
			for i, j := 0, len(segs)-1; i < j; i, j = i+1, j-1 {
				segs[i], segs[j] = segs[j], segs[i]
			}
			return cur, segs
		default:
			return nil, nil
		}
	}
	return nil, nil
}

// accessorString renders a chain as a dotted path, with method calls marked
// by trailing parentheses. For example: "Request().Body" or "Request.Body".
func accessorString(segs []chainSegment) string {
	var b strings.Builder
	for i, s := range segs {
		if i > 0 {
			b.WriteByte('.')
		}
		b.WriteString(s.name)
		if s.isCall {
			b.WriteString("()")
		}
	}
	return b.String()
}

func matchAny(res []*regexp.Regexp, s string) bool {
	for _, re := range res {
		if re != nil && re.MatchString(s) {
			return true
		}
	}
	return false
}

// resolveReceiverSource returns the source argument carried by a decoder
// receiver. For a call like json.NewDecoder(r.Body).Decode(&v), the Decode
// edge's ChainParent is the NewDecoder edge, whose first argument is the
// reader. For decoder := json.NewDecoder(r.Body); decoder.Decode(&v), the
// receiver variable's latest assignment is looked up in the caller
// function's AssignmentMap.
func resolveReceiverSource(edge *metadata.CallGraphEdge, meta *metadata.Metadata) *metadata.CallArgument {
	if edge == nil {
		return nil
	}
	// Inline chain: ChainParent points at the factory call.
	if edge.ChainParent != nil && len(edge.ChainParent.Args) > 0 {
		return edge.ChainParent.Args[0]
	}
	// Variable receiver: look up its latest assignment in the caller
	// function's AssignmentMap. Note that edge.AssignmentMap holds the
	// callee body's assignments — the caller scope lives on the Function.
	recv := edge.CalleeVarName
	if recv == "" || meta == nil {
		return nil
	}
	callerName := meta.StringPool.GetString(edge.Caller.Name)
	callerPkg := meta.StringPool.GetString(edge.Caller.Pkg)
	fn := findFunction(meta, callerPkg, callerName)
	if fn == nil {
		return nil
	}
	assigns, ok := fn.AssignmentMap[recv]
	if !ok || len(assigns) == 0 {
		return nil
	}
	rhs := assigns[len(assigns)-1].Value
	// Expect the RHS to be a call to the factory; return its first argument.
	if rhs.GetKind() == metadata.KindCall && len(rhs.Args) > 0 {
		return rhs.Args[0]
	}
	return nil
}

// findFunction looks up a function declaration by (pkg, name) across all
// files in the package.
func findFunction(meta *metadata.Metadata, pkg, name string) *metadata.Function {
	if meta == nil {
		return nil
	}
	// Sorted: the first file declaring the name answers, so map order would
	// decide which declaration wins (golden rule #1).
	for _, file := range meta.SortedFiles(pkg) {
		if fn, ok := file.Functions[name]; ok {
			return fn
		}
	}
	return nil
}
