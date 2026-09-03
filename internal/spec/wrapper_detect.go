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
	"sort"
	"strconv"
	"strings"

	"github.com/ehabterra/apispec/internal/metadata"
)

// DetectedWrapper is a route pattern derived from a project's own router type —
// a method that registers by forwarding its own parameters to the framework
// (issue #235).
type DetectedWrapper struct {
	// RecvType is the wrapper type as metadata renders it: `pkg.*Router`.
	RecvType string
	// Methods are the method names this pattern covers, sorted. Several methods
	// with identical roles share one pattern.
	Methods []string
	// Via names what they delegate to, for the report: `chi.Mux.Method`.
	Via string
	// Pattern is the derived RoutePattern, ready to use. Empty for a mount.
	Pattern RoutePattern
	// Mount is set instead of Pattern when the method groups routes under a
	// prefix rather than registering one.
	Mount *MountPattern
	// Response / Request / Param are set when the method is the project's own
	// responder, decoder or parameter reader rather than a registrar. They are NOT
	// mutually exclusive: roles are accumulated per method, so a context method
	// that writes a status and reads a parameter carries both, and a consumer has
	// to apply each one it finds rather than the first.
	Response *ResponsePattern
	Request  *RequestBodyPattern
	Param    *ParamPattern
	// Complete is true when every role the inner pattern names was resolved to a
	// parameter. An incomplete derivation is reported but never applied: half a
	// registration produces a route with the wrong path rather than no route.
	Complete bool
}

// wrapperDetectRounds bounds the transitive search. Delegation chains are short
// (`Get` -> `Methods` -> chi), and the bound keeps a cyclic graph from spinning.
const wrapperDetectRounds = 4

// DetectRouterWrappers derives route patterns for a project's own router type.
//
// A house router — gitea's `modules/web.Router` in front of `*chi.Mux` — registers
// nothing that the framework's own patterns can see: the framework call happens
// inside the wrapper, where the path and handler are the wrapper's parameters. So
// the project documents no routes at all, and nothing says why (issue #235).
//
// The wrapper is derived rather than guessed, from one fact: a method of an
// in-module type whose call to a framework registration FORWARDS ITS OWN
// PARAMETERS. That is what separates a wrapper from an ordinary registration
// helper — `func (s *Server) routes() { r.Get("/users", s.list) }` names its path
// literally and is not a wrapper; `func (r *Router) Get(pattern string, h ...any)`
// passes `pattern` through, and it is.
//
// Derivation is transitive, because a wrapper usually funnels its verb methods
// through one registrar: `Get` forwards to `Methods`, which forwards to chi.
func DetectRouterWrappers(meta *metadata.Metadata, cfg *APISpecConfig) []DetectedWrapper {
	if meta == nil || cfg == nil {
		return nil
	}
	// Route patterns seed the router half; the response/request/parameter halves
	// stand on their own, so a config carrying only those still derives.
	fw := cfg.Framework
	if len(fw.RoutePatterns) == 0 && len(fw.ResponsePatterns) == 0 &&
		len(fw.RequestBodyPatterns) == 0 && len(fw.ParamPatterns) == 0 {
		return nil
	}
	cp := NewContextProvider(meta)
	params := newParamIndex(meta)

	// Seeds are the framework's own patterns; each round can derive wrappers of
	// the wrappers found so far.
	seeds := make([]RoutePattern, len(cfg.Framework.RoutePatterns))
	copy(seeds, cfg.Framework.RoutePatterns)

	found := map[string]DetectedWrapper{} // key: recvType + "." + method
	for round := 0; round < wrapperDetectRounds; round++ {
		derived := derivationRound(meta, cp, params, seeds, found)
		if len(derived) == 0 {
			break
		}
		seeds = derived
	}

	wrappers := groupWrappers(found)
	wrappers = append(wrappers, detectGroupMounts(meta, cp, params, wrappers)...)
	return append(wrappers, detectValueWrappers(meta, cp, params, cfg)...)
}

// detectGroupMounts derives the prefix-grouping methods of a type already known
// to be a router.
//
// A house router usually carries its group prefix in a FIELD —
// `Group(pattern string, fn func())` appends to it, calls the closure, and
// restores it — so there is no sub-router to mount and nothing in the framework's
// own patterns can see the prefix. Every route registered inside the closure then
// comes out at the wrong path (`/items` instead of `/api/items`).
//
// The shape is read from facts rather than names: a method of a router type that
// takes a path string and a function, and CALLS that function. A method that only
// takes them without calling is not grouping anything.
func detectGroupMounts(meta *metadata.Metadata, cp ContextProvider, params *paramIndex, routers []DetectedWrapper) []DetectedWrapper {
	types := map[string]bool{}
	for _, w := range routers {
		if w.Complete && w.Mount == nil {
			types[w.RecvType] = true
		}
	}
	if len(types) == 0 {
		return nil
	}

	// Which (method, callee) pairs exist, so "does it call its own parameter?" is
	// a lookup rather than a scan per candidate.
	calls := map[string]map[string]bool{}
	for i := range meta.CallGraph {
		edge := &meta.CallGraph[i]
		recv := cp.GetString(edge.Caller.RecvType)
		if recv == "" {
			continue
		}
		key := cp.GetString(edge.Caller.Pkg) + "." + recv + "." + cp.GetString(edge.Caller.Name)
		if calls[key] == nil {
			calls[key] = map[string]bool{}
		}
		calls[key][cp.GetString(edge.Callee.Name)] = true
	}

	var out []DetectedWrapper
	for fqRecv := range types {
		pkg, recvType, ok := splitFQRecv(fqRecv)
		if !ok {
			continue
		}
		w := &wrapperMethod{pkg: pkg, recvType: recvType}
		for _, method := range methodsOf(meta, w) {
			w.name = cp.GetString(method.Name)
			pathIdx, funcParam, ok := groupShape(method)
			if !ok {
				continue
			}
			if !calls[fqRecv+"."+w.name][funcParam] {
				continue // takes a function but never runs it: not a group
			}
			out = append(out, DetectedWrapper{
				RecvType: fqRecv,
				Methods:  []string{w.name},
				Via:      "prefix held by " + fqRecv,
				Mount: &MountPattern{
					CallRegex:     "^" + regexp.QuoteMeta(w.name) + "$",
					RecvTypeRegex: recvTypeRegexFor(w),
					PathFromArg:   true,
					PathArgIndex:  pathIdx,
					IsMount:       true,
				},
				Complete: true,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].RecvType != out[j].RecvType {
			return out[i].RecvType < out[j].RecvType
		}
		return out[i].Methods[0] < out[j].Methods[0]
	})
	return out
}

// groupShape reports whether a method takes a path string plus a function, and
// where each sits.
func groupShape(m *metadata.Method) (pathIdx int, funcParam string, ok bool) {
	pathIdx = -1
	for i, param := range m.Signature.Args {
		switch param.GetKind() {
		case metadata.KindFuncType:
			if funcParam == "" {
				funcParam = param.GetName()
			}
		case metadata.KindIdent:
			if pathIdx < 0 && param.GetType() == "string" {
				pathIdx = i
			}
		}
	}
	return pathIdx, funcParam, pathIdx >= 0 && funcParam != ""
}

// methodsOf returns a type's declared methods, in file order.
func methodsOf(meta *metadata.Metadata, w *wrapperMethod) []*metadata.Method {
	pkg, ok := meta.Packages[w.pkg]
	if !ok {
		return nil
	}
	bare := strings.TrimPrefix(w.recvType, "*")
	var out []*metadata.Method
	for _, fileName := range meta.SortedFileNames(w.pkg) {
		file := pkg.Files[fileName]
		if file == nil {
			continue
		}
		if typ, ok := file.Types[bare]; ok {
			for i := range typ.Methods {
				out = append(out, &typ.Methods[i])
			}
		}
	}
	return out
}

// splitFQRecv splits `pkg/path.*Type` into its package and receiver.
func splitFQRecv(fq string) (pkg, recv string, ok bool) {
	i := strings.LastIndex(fq, ".")
	if i <= 0 || i == len(fq)-1 {
		return "", "", false
	}
	return fq[:i], fq[i+1:], true
}

// derivationRound matches every edge against the seed patterns and derives a
// wrapper for each in-module method that forwards its parameters into one.
// Returns the patterns derived this round, which seed the next.
func derivationRound(meta *metadata.Metadata, cp ContextProvider, params *paramIndex, seeds []RoutePattern, found map[string]DetectedWrapper) []RoutePattern {
	matchers := make([]*RoutePatternMatcherImpl, 0, len(seeds))
	for _, p := range seeds {
		matchers = append(matchers, NewRoutePatternMatcher(p, &APISpecConfig{}, cp))
	}

	var derived []RoutePattern
	for i := range meta.CallGraph {
		edge := &meta.CallGraph[i]
		caller := wrapperMethodOf(meta, cp, edge)
		if caller == nil {
			continue
		}
		node := newEdgeNode(edge)
		for j, matcher := range matchers {
			if !matcher.MatchNode(node) {
				continue
			}
			wrapper, ok := deriveWrapper(cp, params, caller, seeds[j], edge)
			if !ok {
				continue
			}
			key := wrapper.RecvType + "." + wrapper.Methods[0]
			if _, seen := found[key]; seen {
				continue
			}
			found[key] = wrapper
			if wrapper.Complete {
				derived = append(derived, wrapper.Pattern)
			}
			break // one pattern per edge: the first match is the framework's own
		}
	}
	return derived
}

// wrapperMethod identifies the enclosing method of a registration call.
type wrapperMethod struct {
	pkg      string
	recvType string // as metadata renders it: `*Router`
	name     string
}

func (w wrapperMethod) fqRecv() string { return w.pkg + "." + w.recvType }

// wrapperMethodOf returns the method a registration call is written in, or nil
// when it is not written in an in-module method.
//
// Both halves matter. A registration inside a plain function is an ordinary
// registrar, not a wrapper — there is no type to scope a pattern to. A
// registration inside a dependency's method is that dependency's business, and
// the project cannot be documented by describing it.
func wrapperMethodOf(meta *metadata.Metadata, cp ContextProvider, edge *metadata.CallGraphEdge) *wrapperMethod {
	recv := cp.GetString(edge.Caller.RecvType)
	if recv == "" {
		return nil
	}
	pkg := cp.GetString(edge.Caller.Pkg)
	if pkg == "" || meta.CurrentModulePath == "" || !strings.HasPrefix(pkg, meta.CurrentModulePath) {
		return nil
	}
	return &wrapperMethod{pkg: pkg, recvType: recv, name: cp.GetString(edge.Caller.Name)}
}

// deriveWrapper reads the roles of one registration call and expresses them in
// terms of the enclosing method's own parameters.
func deriveWrapper(cp ContextProvider, params *paramIndex, w *wrapperMethod, inner RoutePattern, edge *metadata.CallGraphEdge) (DetectedWrapper, bool) {
	out := DetectedWrapper{
		RecvType: w.fqRecv(),
		Methods:  []string{w.name},
		Via:      cp.GetString(edge.Callee.Pkg) + "." + cp.GetString(edge.Callee.Name),
		Pattern: RoutePattern{
			CallRegex:     "^" + regexp.QuoteMeta(w.name) + "$",
			RecvTypeRegex: recvTypeRegexFor(w),
		},
	}

	arg := func(idx int) *metadata.CallArgument {
		if idx < 0 || idx >= len(edge.Args) {
			return nil
		}
		return edge.Args[idx]
	}

	pathOK, handlerOK := false, false
	if inner.PathFromArg {
		if i, ok := params.indexOf(w, arg(inner.PathArgIndex)); ok {
			out.Pattern.PathFromArg, out.Pattern.PathArgIndex = true, i
			pathOK = true
		}
	}
	if inner.HandlerFromArg {
		// Resolved against the call's real arity, so a wrapper that adds its own
		// middleware — `r.engine.GET(path, r.auth, h)` — still forwards from the
		// handler and not from the guard it inserted (#386). The DERIVED pattern
		// keeps a fixed index: the wrapper's own signature is not variadic, so
		// the handler sits at one parameter position.
		if i, ok := inner.HandlerArgIndexFor(len(edge.Args)); ok {
			if j, ok := params.indexOf(w, arg(i)); ok {
				out.Pattern.HandlerFromArg, out.Pattern.HandlerArgIndex = true, j
				handlerOK = true
			}
		}
	}

	// The verb: forwarded as a parameter, or fixed because the wrapper method is
	// named for it (Get/Post/…). A verb that is neither leaves the route on the
	// default, which is what happens today.
	if idx := innerVerbArgIndex(inner); idx >= 0 {
		if i, ok := params.indexOf(w, arg(idx)); ok {
			out.Pattern.MethodFromArg = true
			out.Pattern.MethodArgIndex = i
		}
	}
	if !out.Pattern.MethodFromArg && isHTTPMethod(strings.ToUpper(w.name)) {
		out.Pattern.MethodFromCall = true
	}

	// Path and handler are what a registration IS. Without both, applying the
	// pattern would produce a route with a wrong path rather than no route, so it
	// is reported and left unapplied.
	out.Complete = pathOK && handlerOK
	return out, true
}

// innerVerbArgIndex returns the argument index a pattern reads its verb from, or
// -1 when the verb does not travel as an argument.
//
// MethodArgIndex cannot be read on its own: it defaults to 0, so for a pattern
// whose verb comes from the CALL name (chi's `r.Get(path, h)`) index 0 is the
// path. Treating that as the verb would derive a wrapper whose "verb" is its
// path parameter.
func innerVerbArgIndex(inner RoutePattern) int {
	if inner.MethodFromCall || inner.MethodFromHandler || inner.MethodFromPath {
		return -1
	}
	if inner.MethodFromArg {
		return inner.MethodArgIndex
	}
	idx := inner.MethodArgIndex
	if inner.PathFromArg && idx == inner.PathArgIndex {
		return -1
	}
	if inner.HandlerFromArg && idx == inner.HandlerArgIndex {
		return -1
	}
	return idx
}

// recvTypeRegexFor renders the receiver constraint the way pattern matching reads
// it: `pkg.*Router`, with the pointer optional so a value receiver matches too.
func recvTypeRegexFor(w *wrapperMethod) string {
	return recvTypeRegexForNames(w.pkg, []string{strings.TrimPrefix(w.recvType, "*")})
}

// recvTypeRegexForNames builds the same constraint over several type names.
func recvTypeRegexForNames(pkg string, names []string) string {
	quoted := make([]string, 0, len(names))
	for _, n := range names {
		quoted = append(quoted, regexp.QuoteMeta(n))
	}
	alt := quoted[0]
	if len(quoted) > 1 {
		alt = "(" + strings.Join(quoted, "|") + ")"
	}
	return "^" + regexp.QuoteMeta(pkg) + `\.\*?` + alt + "$"
}

// embeddingRecvRegex is recvTypeRegexFor widened to the types that EMBED the
// declaring one.
//
// Go promotes an embedded type's methods, and a call is recorded against the type
// the caller used: gitea declares its responder on `services/context.Base` but
// every handler calls it through `*Context` or `*APIContext`, which embed Base. A
// pattern scoped to the declaring type alone therefore matched almost nothing —
// gitea documented 894 routes and 8 components, because the responder that
// produces every schema was never recognised at its call sites (issue #235).
func embeddingRecvRegex(meta *metadata.Metadata, w *wrapperMethod) string {
	bare := strings.TrimPrefix(w.recvType, "*")
	names := []string{bare}

	// Transitively: a type embedding a type that embeds the declaring one gets
	// the method too.
	if _, ok := meta.Packages[w.pkg]; !ok {
		return recvTypeRegexForNames(w.pkg, names)
	}
	for round := 0; round < wrapperDetectRounds; round++ {
		grew := false
		for fileName := range meta.SortedFiles(w.pkg) {
			// Sorted: names accumulates in scan order and is consumed as a
			// list, so map order would reach the caller.
			for typeName, typ := range meta.SortedTypes(w.pkg, fileName) {
				if containsName(names, typeName) {
					continue
				}
				for _, embedded := range typ.Embeds {
					embeddedName := strings.TrimPrefix(strings.TrimPrefix(meta.StringPool.GetString(embedded), "*"), w.pkg+".")
					if containsName(names, embeddedName) {
						names = append(names, typeName)
						grew = true
						break
					}
				}
			}
		}
		if !grew {
			break
		}
	}
	sort.Strings(names)
	return recvTypeRegexForNames(w.pkg, names)
}

// containsName reports whether names holds one.
func containsName(names []string, want string) bool {
	for _, n := range names {
		if n == want {
			return true
		}
	}
	return false
}

// groupWrappers merges methods whose derived roles are identical into one pattern
// — a router's eight verb methods become one line rather than eight.
func groupWrappers(found map[string]DetectedWrapper) []DetectedWrapper {
	byRoles := map[string]DetectedWrapper{}
	for _, w := range found {
		key := w.RecvType + "|" + rolesKey(w.Pattern) + "|" + boolKey(w.Complete)
		existing, ok := byRoles[key]
		if !ok {
			byRoles[key] = w
			continue
		}
		existing.Methods = append(existing.Methods, w.Methods...)
		byRoles[key] = existing
	}

	out := make([]DetectedWrapper, 0, len(byRoles))
	for _, w := range byRoles {
		sort.Strings(w.Methods)
		names := make([]string, 0, len(w.Methods))
		for _, m := range w.Methods {
			names = append(names, regexp.QuoteMeta(m))
		}
		w.Pattern.CallRegex = "^(" + strings.Join(names, "|") + ")$"
		out = append(out, w)
	}
	// Sorted: derived patterns reach the config, and config order reaches the
	// output (golden rule #1).
	sort.Slice(out, func(i, j int) bool {
		if out[i].RecvType != out[j].RecvType {
			return out[i].RecvType < out[j].RecvType
		}
		return out[i].Pattern.CallRegex < out[j].Pattern.CallRegex
	})
	return out
}

func rolesKey(p RoutePattern) string {
	return strings.Join([]string{
		boolKey(p.PathFromArg), itoa(p.PathArgIndex),
		boolKey(p.HandlerFromArg), itoa(p.HandlerArgIndex),
		boolKey(p.MethodFromArg), itoa(p.MethodArgIndex),
		boolKey(p.MethodFromCall),
	}, ",")
}

func boolKey(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

func itoa(i int) string { return strconv.Itoa(i) }

// detectValueWrappers derives response, request-body and parameter patterns for a
// project's own context type — the write-side and read-side twins of a router
// wrapper (issue #235).
//
// A house router is rarely alone: the same project usually answers through its
// own context (`ctx.JSON(status, body)`, `ctx.Bind(&in)`, `ctx.Param("id")`).
// Those calls are invisible to the framework's patterns for the same reason the
// router was — the framework call is inside the method, where the status, body
// and name are the method's parameters. gitea documented 856 routes and ZERO
// components until these were derived.
//
// The derivation is the router one: a method of an in-module type that forwards
// its own parameters into a known pattern. A method can satisfy several roles
// through several inner calls — a JSON responder sets the status through
// `WriteHeader(status)` and the body through `Encode(content)` — so roles are
// merged per method rather than taken from one call.
func detectValueWrappers(meta *metadata.Metadata, cp ContextProvider, params *paramIndex, cfg *APISpecConfig) []DetectedWrapper {
	byMethod := map[string]*DetectedWrapper{}

	// Wrappers stack: a project that wraps encoding/json in its own package (gitea
	// does) reaches the framework pattern two levels down, so each round feeds
	// what it derived back in as patterns to match against.
	seeds := *cfg
	for round := 0; round < wrapperDetectRounds; round++ {
		before := len(byMethod)
		detectValueRound(meta, cp, params, &seeds, byMethod)
		if len(byMethod) == before {
			break
		}
		seeds = seedsFrom(byMethod)
	}

	out := make([]DetectedWrapper, 0, len(byMethod))
	for _, w := range byMethod {
		out = append(out, *w)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].RecvType != out[j].RecvType {
			return out[i].RecvType < out[j].RecvType
		}
		return out[i].Methods[0] < out[j].Methods[0]
	})
	return out
}

// seedsFrom turns the wrappers derived so far into patterns for the next round.
func seedsFrom(byMethod map[string]*DetectedWrapper) APISpecConfig {
	var cfg APISpecConfig
	for _, w := range byMethod {
		if !w.Complete {
			continue
		}
		switch {
		case w.Response != nil:
			cfg.Framework.ResponsePatterns = append(cfg.Framework.ResponsePatterns, *w.Response)
		case w.Request != nil:
			cfg.Framework.RequestBodyPatterns = append(cfg.Framework.RequestBodyPatterns, *w.Request)
		case w.Param != nil:
			cfg.Framework.ParamPatterns = append(cfg.Framework.ParamPatterns, *w.Param)
		}
	}
	return cfg
}

// detectValueRound matches every edge against one set of patterns and folds what
// it finds into byMethod.
func detectValueRound(meta *metadata.Metadata, cp ContextProvider, params *paramIndex, cfg *APISpecConfig, byMethod map[string]*DetectedWrapper) {

	respMatchers := make([]*ResponsePatternMatcherImpl, 0, len(cfg.Framework.ResponsePatterns))
	for _, p := range cfg.Framework.ResponsePatterns {
		respMatchers = append(respMatchers, NewResponsePatternMatcher(p, cfg, cp))
	}
	reqMatchers := make([]*RequestPatternMatcherImpl, 0, len(cfg.Framework.RequestBodyPatterns))
	for _, p := range cfg.Framework.RequestBodyPatterns {
		reqMatchers = append(reqMatchers, NewRequestPatternMatcher(p, cfg, cp))
	}
	paramMatchers := make([]*ParamPatternMatcherImpl, 0, len(cfg.Framework.ParamPatterns))
	for _, p := range cfg.Framework.ParamPatterns {
		paramMatchers = append(paramMatchers, NewParamPatternMatcher(p, cfg, cp))
	}

	for i := range meta.CallGraph {
		edge := &meta.CallGraph[i]
		w := wrapperMethodOf(meta, cp, edge)
		if w == nil {
			continue
		}
		node := newEdgeNode(edge)
		key := w.fqRecv() + "." + w.name

		// Widened once per method: the pattern has to match where handlers CALL
		// this method, which is through whatever type embeds it.
		recvRegex := embeddingRecvRegex(meta, w)
		for j, m := range respMatchers {
			if m.MatchNode(node) {
				mergeResponseRoles(byMethod, key, w, cp, params, cfg.Framework.ResponsePatterns[j], edge, recvRegex)
			}
		}
		for j, m := range reqMatchers {
			if m.MatchNode(node) {
				deriveRequestWrapper(byMethod, key, w, cp, params, cfg.Framework.RequestBodyPatterns[j], edge, recvRegex)
			}
		}
		for j, m := range paramMatchers {
			if m.MatchNode(node) {
				deriveParamWrapper(byMethod, key, w, cp, params, cfg.Framework.ParamPatterns[j], edge, recvRegex)
			}
		}
	}

}

// wrapperShell returns the accumulating entry for a method, creating it once.
func wrapperShell(byMethod map[string]*DetectedWrapper, key string, w *wrapperMethod, via string) *DetectedWrapper {
	if existing, ok := byMethod[key]; ok {
		return existing
	}
	entry := &DetectedWrapper{
		RecvType: w.fqRecv(),
		Methods:  []string{w.name},
		Via:      via,
	}
	byMethod[key] = entry
	return entry
}

// mergeResponseRoles folds one inner response call into the method's derived
// pattern: the status from a `WriteHeader(status)`, the body from an
// `Encode(content)`, both of which the same responder performs.
func mergeResponseRoles(byMethod map[string]*DetectedWrapper, key string, w *wrapperMethod, cp ContextProvider, params *paramIndex, inner ResponsePattern, edge *metadata.CallGraphEdge, recvRegex string) {
	entry := wrapperShell(byMethod, key, w, cp.GetString(edge.Callee.Pkg)+"."+cp.GetString(edge.Callee.Name))
	if entry.Response == nil {
		entry.Response = &ResponsePattern{
			CallRegex:     "^" + regexp.QuoteMeta(w.name) + "$",
			RecvTypeRegex: recvRegex,
			TypeArgIndex:  -1,
		}
	}

	if inner.StatusFromArg && !entry.Response.StatusFromArg {
		if i, ok := params.indexOf(w, argAt(edge, inner.StatusArgIndex)); ok {
			entry.Response.StatusFromArg, entry.Response.StatusArgIndex = true, i
		}
	}
	if inner.TypeFromArg && !entry.Response.TypeFromArg {
		if i, ok := params.indexOf(w, argAt(edge, inner.TypeArgIndex)); ok {
			entry.Response.TypeFromArg, entry.Response.TypeArgIndex = true, i
			entry.Response.Deref = inner.Deref
		}
	}
	if inner.DefaultStatus != 0 && entry.Response.DefaultStatus == 0 {
		entry.Response.DefaultStatus = inner.DefaultStatus
	}

	// A body is what makes a responder worth describing: a status alone documents
	// nothing a client can use.
	entry.Complete = entry.Response.TypeFromArg
}

// deriveRequestWrapper derives a request-body pattern from a decoder wrapper —
// `func (c *Ctx) Bind(dst any) error { json.NewDecoder(c.r.Body).Decode(dst) }`.
func deriveRequestWrapper(byMethod map[string]*DetectedWrapper, key string, w *wrapperMethod, cp ContextProvider, params *paramIndex, inner RequestBodyPattern, edge *metadata.CallGraphEdge, recvRegex string) {
	if !inner.TypeFromArg {
		return
	}
	i, ok := params.indexOf(w, argAt(edge, inner.TypeArgIndex))
	if !ok {
		return
	}
	entry := wrapperShell(byMethod, key, w, cp.GetString(edge.Callee.Pkg)+"."+cp.GetString(edge.Callee.Name))
	entry.Request = &RequestBodyPattern{
		CallRegex:     "^" + regexp.QuoteMeta(w.name) + "$",
		RecvTypeRegex: recvRegex,
		TypeFromArg:   true,
		TypeArgIndex:  i,
		Deref:         inner.Deref,
	}
	entry.Complete = true
}

// deriveParamWrapper derives a parameter pattern from a reader wrapper —
// `func (c *Ctx) Param(name string) string { return chi.URLParam(c.r, name) }`.
// The location comes from the inner pattern: the wrapper reads whatever it reads.
func deriveParamWrapper(byMethod map[string]*DetectedWrapper, key string, w *wrapperMethod, cp ContextProvider, params *paramIndex, inner ParamPattern, edge *metadata.CallGraphEdge, recvRegex string) {
	if inner.ParamIn == "" || inner.NameFromMapKey {
		return
	}
	i, ok := params.indexOf(w, argAt(edge, inner.ParamArgIndex))
	if !ok {
		return
	}
	entry := wrapperShell(byMethod, key, w, cp.GetString(edge.Callee.Pkg)+"."+cp.GetString(edge.Callee.Name))
	entry.Param = &ParamPattern{
		CallRegex:     "^" + regexp.QuoteMeta(w.name) + "$",
		RecvTypeRegex: recvRegex,
		ParamIn:       inner.ParamIn,
		ParamArgIndex: i,
	}
	entry.Complete = true
}

// argAt returns an edge's argument at idx, or nil.
func argAt(edge *metadata.CallGraphEdge, idx int) *metadata.CallArgument {
	if idx < 0 || idx >= len(edge.Args) {
		return nil
	}
	return edge.Args[idx]
}
