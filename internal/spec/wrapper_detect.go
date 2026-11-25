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
	"reflect"
	"regexp"
	"slices"
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

	// bodyDeclined records that the response's body role was DECLINED: it
	// resolved to the same parameter as its status (see normaliseResponseRoles),
	// or it is a value the method builds from its parameters rather than one of
	// them (see paramIndex.valueIndexOf). Such a responder is still complete when
	// its status is known — the body is known not to be the caller's — and the
	// flag lives on the entry rather than in the merge that discovered it,
	// because roles accumulate across edges in whatever order the call graph
	// stores them.
	bodyDeclined bool
}

// ResponseResolved reports whether the response role describes something a
// call site can apply: a body read from a parameter, or a status read from one
// with the body known not to be the caller's (see bodyDeclined). A status whose
// body was merely never located is not enough — the method writes SOME body,
// and the walk into it documents that body better than a pattern that drops it.
//
// It is kept apart from Complete because Complete covers every role the method
// carries: a decoder role resolved beside a response half that resolved
// nothing makes the entry applicable, but not that half.
func (w DetectedWrapper) ResponseResolved() bool {
	p := w.Response
	return p != nil && (p.TypeFromArg || (w.bodyDeclined && p.StatusFromArg))
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
	wrappers = append(wrappers, detectForwardingMounts(meta, cp, params, cfg)...)
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
		// handler and not from the guard it inserted (#386).
		if i, ok := inner.HandlerArgIndexFor(len(edge.Args)); ok {
			if j, ok := params.indexOf(w, arg(i)); ok {
				out.Pattern.HandlerFromArg, out.Pattern.HandlerArgIndex = true, j
				// ...and when the wrapper forwards a VARIADIC chain, the derived
				// pattern has to say so too. The inner call is written once,
				// `r.engine.GET(path, handlers...)`, so it has two arguments
				// whatever the caller passes — which fixes the handler at index
				// 1, and a later `r.Get("/users", auth, endpoint)` then reads
				// the middleware (issue #416).
				out.Pattern.HandlerArgFromEnd = params.isVariadic(w, j)
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

	// Built from the project's real config and kept across rounds: seedsFrom
	// carries patterns only, so a resolver rebuilt per round would be disabled
	// from the second round on — and a decoder two wrappers deep would derive
	// unverified, which is the whole of #513 one level further down.
	body := newBodySourceResolver(cfg, cp)
	// Kept across rounds for the same reason: a method's own header writes
	// match only the framework's patterns, in the first round, while its body
	// role may come from a responder derived in a later one.
	media := newResponderMedia(cp, meta)

	// Wrappers stack: a project that wraps encoding/json in its own package (gitea
	// does) reaches the framework pattern two levels down, so each round feeds
	// what it derived back in as patterns to match against.
	//
	// The search stops at a fixed point: when a round leaves the seeds as they
	// were, the next would match the same patterns against the same edges. The
	// method count is not that signal — a round can resolve a method an earlier
	// one had only started (a `WriteHeader(code)` beside a forward to a
	// declined-body responder), and that method seeds the next layer while the
	// count stands still (CodeRabbit on #660).
	seeds := *cfg
	var declined []bool // the framework's own patterns decline nothing
	for round := 0; round < wrapperDetectRounds; round++ {
		detectValueRound(meta, cp, params, body, media, &seeds, declined, byMethod)
		next, nextDeclined := seedsFrom(byMethod)
		if !hasSeeds(next) || (round > 0 && sameSeeds(seeds, next) && slices.Equal(declined, nextDeclined)) {
			break
		}
		seeds, declined = next, nextDeclined
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
//
// declined runs parallel to the response patterns: it marks a seed whose body
// was declined (see bodyDeclined), which a pattern cannot say for itself — the
// pattern has no body role either way. A method forwarding to such a responder
// inherits the declined body: the inner method makes the body, so the outer one
// has none of the caller's to substitute either. Without it the forwarder came
// out with a resolved status, no body role and no reason why, and was left
// unapplied (CodeRabbit on #660). A seed that reads its body from a parameter
// is not declined, whatever else the method writes.
//
// Every resolved role is seeded on its own: a method that decodes the caller's
// value AND answers with the caller's status is both a decoder and a responder
// to whatever forwards to it. Seeded as one role only, the forwarder inherited
// the other never, and no later round could restore it (CodeRabbit on #660).
//
// Seeds come out in method order, so the fixed-point check in
// detectValueWrappers can compare two rounds' seeds directly, and so the order
// in which a forwarder meets its seeds — merges are first-wins per role — does
// not follow map iteration (golden rule #1).
func seedsFrom(byMethod map[string]*DetectedWrapper) (cfg APISpecConfig, declined []bool) {
	keys := make([]string, 0, len(byMethod))
	for k := range byMethod {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		w := byMethod[k]
		if !w.Complete {
			continue
		}
		if w.ResponseResolved() {
			cfg.Framework.ResponsePatterns = append(cfg.Framework.ResponsePatterns, *w.Response)
			declined = append(declined, w.bodyDeclined && !w.Response.TypeFromArg)
		}
		if w.Request != nil {
			cfg.Framework.RequestBodyPatterns = append(cfg.Framework.RequestBodyPatterns, *w.Request)
		}
		if w.Param != nil {
			cfg.Framework.ParamPatterns = append(cfg.Framework.ParamPatterns, *w.Param)
		}
	}
	return cfg, declined
}

func hasSeeds(cfg APISpecConfig) bool {
	f := cfg.Framework
	return len(f.ResponsePatterns) > 0 || len(f.RequestBodyPatterns) > 0 || len(f.ParamPatterns) > 0
}

// sameSeeds compares the derived patterns two rounds seeded from. The patterns
// are plain data (strings, ints, flags and string slices), which DeepEqual
// compares by value.
func sameSeeds(a, b APISpecConfig) bool {
	return reflect.DeepEqual(a.Framework.ResponsePatterns, b.Framework.ResponsePatterns) &&
		reflect.DeepEqual(a.Framework.RequestBodyPatterns, b.Framework.RequestBodyPatterns) &&
		reflect.DeepEqual(a.Framework.ParamPatterns, b.Framework.ParamPatterns)
}

// detectValueRound matches every edge against one set of patterns and folds what
// it finds into byMethod. declined is parallel to the response patterns, or nil
// when none of them declined its body (see seedsFrom).
func detectValueRound(meta *metadata.Metadata, cp ContextProvider, params *paramIndex, body *bodySourceResolver, media *responderMedia, cfg *APISpecConfig, declined []bool, byMethod map[string]*DetectedWrapper) {

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
		isParam := func(name string) bool { return slices.Contains(params.paramsOf(w), name) }
		for j, m := range respMatchers {
			if m.MatchNode(node) && !m.writesElsewhere(edge, isParam) {
				inner := cfg.Framework.ResponsePatterns[j]
				if inner.ContentTypeFromHeaderWrite {
					media.declaration(key, m, node)
				}
				if inner.StatusFromArg && !inner.TypeFromArg && !inner.ContentTypeFromHeaderWrite && !inner.DefersHeaders {
					media.commit(key, edge)
				}
				body, rawBytes := m.wrapperBodyArg(edge)
				innerDeclined := j < len(declined) && declined[j]
				hadBody := byMethod[key] != nil && byMethod[key].Response != nil && byMethod[key].Response.TypeFromArg
				mergeResponseRoles(byMethod, key, w, cp, params, inner, innerDeclined, edge, recvRegex, body, rawBytes)
				if e := byMethod[key]; !hadBody && e.Response != nil && e.Response.TypeFromArg && takesHeaderMediaType(&inner) {
					media.body(key, edge)
				}
			}
		}
		for j, m := range reqMatchers {
			if m.MatchNode(node) {
				deriveRequestWrapper(byMethod, key, w, cp, params, body, meta, cfg.Framework.RequestBodyPatterns[j], edge, recvRegex)
			}
		}
		for j, m := range paramMatchers {
			if m.MatchNode(node) {
				deriveParamWrapper(byMethod, key, w, cp, params, cfg.Framework.ParamPatterns[j], edge, recvRegex)
			}
		}
	}
	media.apply(byMethod)
}

// responderMedia finds the media type a derived responder sends its body
// under when the method declares it itself: `w.Header().Set("Content-Type",
// "application/problem+json")`, then `json.NewEncoder(w).Encode(v)`. The
// encoder writes no header, so the declaration is the body's media type at
// every call site (issue #652). Derived without it, the call site documented
// the caller's value under the JSON default beside the walk's own pairing
// inside the method, which reads the declaration — one response in two media
// types.
type responderMedia struct {
	flow *blockIndex
	// method key -> its Content-Type header writes
	decls map[string][]responderDecl
	// method key -> its status writes, which commit the headers
	commits map[string][]codePos
	// method key -> the call that gave it its body role, when that call's
	// body takes the header's media type (see takesHeaderMediaType)
	bodyAt map[string]codePos
}

type responderDecl struct {
	at        codePos
	mediaType string
	tentative bool
}

func newResponderMedia(cp ContextProvider, meta *metadata.Metadata) *responderMedia {
	var flow *blockIndex
	if impl, ok := cp.(*ContextProviderImpl); ok {
		flow = impl.controlFlow()
	} else {
		flow = newBlockIndex(meta)
	}
	return &responderMedia{flow: flow, decls: map[string][]responderDecl{}, commits: map[string][]codePos{}, bodyAt: map[string]codePos{}}
}

func (r *responderMedia) declaration(key string, m *ResponsePatternMatcherImpl, node TrackerNodeInterface) {
	mediaType, tentative, ok := m.headerDeclaration(node)
	if !ok {
		return
	}
	at := edgePosition(node.GetEdge())
	for _, d := range r.decls[key] {
		if d.at == at {
			return // the same write, met again in a later round
		}
	}
	r.decls[key] = append(r.decls[key], responderDecl{at: at, mediaType: mediaType, tentative: tentative})
}

func (r *responderMedia) commit(key string, edge *metadata.CallGraphEdge) {
	at := edgePosition(edge)
	if !slices.Contains(r.commits[key], at) {
		r.commits[key] = append(r.commits[key], at)
	}
}

// committed reports whether a status write in the method runs on every path
// to at: WriteHeader puts the headers on the wire, so a Content-Type set after
// it has no effect — the rule pairing applies inline (committedAt in
// pairAndFillResponses).
func (r *responderMedia) committed(key string, at codePos) bool {
	for _, c := range r.commits[key] {
		if c.file == at.file && c.valid() && r.flow.dominates(c, at) {
			return true
		}
	}
	return false
}

func (r *responderMedia) body(key string, edge *metadata.CallGraphEdge) {
	r.bodyAt[key] = edgePosition(edge)
}

// apply gives each responder the media type declared where its body is
// written: the last declaration in the method that dominates the body call,
// as pairing reads it (see pairAndFillResponses). One in a branch is not in
// effect on the other path, nor one written after a status write has
// committed the headers; a tentative one — a media type a serializer produces
// — leaves the body describing itself (issue #570).
func (r *responderMedia) apply(byMethod map[string]*DetectedWrapper) {
	for key, bodyAt := range r.bodyAt {
		entry := byMethod[key]
		if entry == nil || entry.Response == nil || !bodyAt.valid() {
			continue
		}
		var effect *responderDecl
		// decls and bodyAt share the method key, so every declaration here is
		// in the body's own function; dominates answers true across functions,
		// and that is only sound because of this keying.
		for i := range r.decls[key] {
			d := &r.decls[key][i]
			if d.at.file != bodyAt.file || !d.at.valid() || !r.flow.dominates(d.at, bodyAt) || r.committed(key, d.at) {
				continue
			}
			if effect == nil || effect.at.beforeOrAt(d.at) {
				effect = d
			}
		}
		if effect != nil && !effect.tentative {
			entry.Response.DeclaredContentType = effect.mediaType
		}
	}
}

// edgePosition is the call's own position in its caller.
func edgePosition(edge *metadata.CallGraphEdge) codePos {
	file, line, col := positionOfInstanceID(edge.Callee.ID())
	return codePos{file: file, line: line, col: col}
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
//
// A body role carries the inner call's media type with it. Lifted without one,
// the derived pattern documented every body under the JSON default, beside the
// body the walk documents inside the method: `http.Error(b.Resp, msg, status)`
// wrapped in a method answered text/plain AND application/json (issue #645).
//
// body is the value the inner call's body role writes, seen through a
// serializer, and rawBytes marks an inner call that writes its argument's bytes
// verbatim (both from wrapperBodyArg). Whether those bytes are a document is a fact about each
// CALL SITE, not about the method: `b.Blob(201, bs)` after `bs :=
// json.Marshal(item)` sends Item, while `b.PlainText(500, "failed")` sends
// bytes whose media type the method declares before writing (or the server
// sniffs). So the body role is kept, marked SerializedBodyOnly: the call site
// documents a body only where its argument traces to a serializer. Lifted
// unconditionally, a derived `PlainText` said application/json beside the
// text/plain the walk pairs inside the method; dropped altogether, the
// serialized call lost its $ref. A wrapper of such a method inherits the mark
// through the same check one round later, since the mark keeps RawBody.
//
// innerDeclined marks an inner call to a derived responder whose own body was
// declined (see seedsFrom): that responder makes its body itself, so this
// method's body is declined with it.
func mergeResponseRoles(byMethod map[string]*DetectedWrapper, key string, w *wrapperMethod, cp ContextProvider, params *paramIndex, inner ResponsePattern, innerDeclined bool, edge *metadata.CallGraphEdge, recvRegex string, body *metadata.CallArgument, rawBytes bool) {
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
			// A wrapper of `c.Status(code)` leaves the headers open as the
			// call it wraps does.
			entry.Response.DefersHeaders = inner.DefersHeaders
		}
	}
	if inner.TypeFromArg && !entry.Response.TypeFromArg {
		if i, ok := params.valueIndexOf(w, body); ok {
			entry.Response.TypeFromArg, entry.Response.TypeArgIndex = true, i
			entry.Response.Deref = inner.Deref
			entry.Response.DefaultContentType = inner.DefaultContentType
			entry.Response.SetsContentType = inner.SetsContentType
			entry.Response.DeclaredContentType = inner.DeclaredContentType
			entry.Response.RawBody = rawBytes
			entry.Response.SerializedBodyOnly = rawBytes
		} else if _, built := params.indexOf(w, body); built {
			// Built FROM the caller's values but not one of them — `"bad "+p`,
			// `http.StatusText(code)`: the method makes the body itself, so the
			// call site has none to substitute (issue #659).
			entry.bodyDeclined = true
		}
	}
	if innerDeclined {
		entry.bodyDeclined = true
	}
	normaliseResponseRoles(entry)
	if inner.DefaultStatus != 0 && entry.Response.DefaultStatus == 0 {
		entry.Response.DefaultStatus = inner.DefaultStatus
	}

	// A body is what makes a responder worth describing: a status alone documents
	// nothing a client can use — UNLESS the body was declined, because it
	// resolved to the status parameter or is built by the method from the
	// caller's values. There the status is known and the body is known not to be
	// the caller's, which is a complete description of `ctx.HTTPError(404)`: that
	// endpoint really can answer 404, and saying so with no body beats both
	// inventing one and dropping the status with it. Keeping the pattern is worth
	// 186 statuses on a large real project, every one of which had been resting
	// on the invented body (issue #485). With no status role either, a declined
	// body leaves nothing to apply, and the walk into the method documents what
	// it writes.
	//
	// Completeness is the ENTRY's, not this half's: a decoder or parameter role
	// already resolved on the method stays applicable whatever a response edge
	// met later fails to resolve. Recomputed from the response alone, a
	// `Decode(dst)` followed by `http.Error(w, "bad "+r.URL.Path, 400)` lost its
	// request body, and the same calls in the other order kept it (CodeRabbit on
	// #660).
	entry.Complete = entry.ResponseResolved() || entry.Request != nil || entry.Param != nil
}

// sameAsStatusParam reports whether a derived pattern would read its BODY from
// the parameter it already reads its STATUS from.
//
// One parameter cannot be both. Where the derivation arrives at that answer it
// has followed a local back to the wrong source:
//
//	func (b *Base) HTTPError(status int, contents ...string) {
//		v := http.StatusText(status)      // <- v traces back to `status`
//		if len(contents) > 0 {
//			v = contents[0]               //    ...and to `contents` as well
//		}
//		http.Error(b.Resp, v, status)
//	}
//
// The inner `http.Error` names its body at argument 1 and its status at 2, which
// is right. Mapping those back onto HTTPError's parameters, the status lands on
// `status` — correct — and the body follows `v` through the first assignment it
// finds and lands on `status` too. The derived pattern then reads a status
// CONSTANT as the payload, and on a large real project that documented eighteen
// error responses as `application/json: {type: integer}` bodies they never send
// (issue #485).
//
// Declining is the honest outcome, not a lesser one: a responder whose body
// cannot be located contributes no body, and the status half of the pattern
// still works. Guessing the other branch of that assignment would be a coin
// flip between `contents` and nothing (golden rule #7).
func sameAsStatusParam(p *ResponsePattern, typeIdx int) bool {
	return p != nil && p.StatusFromArg && p.StatusArgIndex == typeIdx
}

// normaliseResponseRoles enforces the one-parameter-one-role rule over the
// roles accumulated SO FAR, whichever order they arrived in.
//
// Checking it only while adding the body was order-dependent: a body-only edge
// reaching the merge first sets the body role with no status recorded yet — so
// nothing to collide with — and a status-only edge afterwards then lands on the
// same parameter, reproducing the state the check exists to prevent by the
// other route. detectValueRound walks the call graph in stored order, so which
// edge arrives first is not something the derivation gets to assume
// (CodeRabbit on #486).
//
// The STATUS is what survives a collision. It is read straight from an argument
// the caller passes, while the body reached the same parameter by following a
// local through an assignment — the weaker inference, and the one that was
// wrong in every observed case (issue #485).
func normaliseResponseRoles(entry *DetectedWrapper) {
	p := entry.Response
	if p == nil || !p.TypeFromArg || !sameAsStatusParam(p, p.TypeArgIndex) {
		return
	}
	p.TypeFromArg = false
	p.TypeArgIndex = -1
	p.Deref = false
	p.DefaultContentType = ""
	p.SetsContentType = false
	p.DeclaredContentType = ""
	p.RawBody = false
	p.SerializedBodyOnly = false
	// Sticky: a later merge must not read the absence of a body role as "no
	// body was ever found" and drop the status with it.
	entry.bodyDeclined = true
}

// deriveRequestWrapper derives a request-body pattern from a decoder wrapper —
// `func (c *Ctx) Bind(dst any) error { json.NewDecoder(c.r.Body).Decode(dst) }`.
func deriveRequestWrapper(byMethod map[string]*DetectedWrapper, key string, w *wrapperMethod, cp ContextProvider, params *paramIndex, body *bodySourceResolver, meta *metadata.Metadata, inner RequestBodyPattern, edge *metadata.CallGraphEdge, recvRegex string) {
	if !inner.TypeFromArg {
		return
	}
	// The destination is a value role like a response body: `Decode(&dst)`
	// fills the caller's dst, `Decode(&env.Data)` fills the method's own
	// envelope, and the call site can only substitute the first (issue #659).
	i, ok := params.valueIndexOf(w, argAt(edge, inner.TypeArgIndex))
	if !ok {
		return
	}
	gate, ok := requestSourceGateFor(w, params, body, meta, inner, edge)
	if !ok {
		return
	}
	entry := wrapperShell(byMethod, key, w, cp.GetString(edge.Callee.Pkg)+"."+cp.GetString(edge.Callee.Name))
	entry.Request = &RequestBodyPattern{
		CallRegex:            "^" + regexp.QuoteMeta(w.name) + "$",
		RecvTypeRegex:        recvRegex,
		TypeFromArg:          true,
		TypeArgIndex:         i,
		Deref:                inner.Deref,
		RequireRequestSource: gate.require,
		BodySourceArgIndex:   gate.argIndex,
	}
	entry.Complete = true
}

// requestSourceGate is what a derived decoder pattern must still check at the
// call site: nothing, or that the argument at argIndex is the request's body.
type requestSourceGate struct {
	require  bool
	argIndex int
}

// requestSourceGateFor decides whether a method that decodes JSON is decoding
// the HTTP REQUEST, and reports what the derived pattern has to verify where
// handlers call it. The bool is false when no pattern should be derived at all.
//
// A decoder wrapper is recognised by shape — a method forwarding its own
// parameter into a decode — and that shape is equally the shape of a client
// reading a provider's reply:
//
//	func (c *Ctx) Bind(dst any) error    { return json.NewDecoder(c.Req.Body).Decode(dst) }
//	func (c *client) fetch(out any) error { return json.NewDecoder(resp.Body).Decode(out) }
//
// Only where the bytes come FROM tells them apart, which is exactly what the
// inner pattern's RequireRequestSource already asks at extraction time. It was
// not asked here, so the second derived a pattern as readily as the first and a
// handler that called it documented the provider's type as its request body —
// displacing the handler's own, since both are concrete and the outbound call
// comes later (#513).
//
// Three answers:
//
//   - the method reads the request itself — derive, with nothing left to check;
//   - the method reads what it is HANDED (`func (h *H) decode(src io.Reader, v any)`) —
//     derive, and carry the obligation to the call site, where the argument is
//     a real value rather than a parameter. This is why the check cannot simply
//     be moved here wholesale: a wrapper's source is often knowable only from
//     its caller;
//   - anything else — derive nothing. Honest over wrong (golden rule #7): a
//     decode whose source is neither the request nor the caller's is not a
//     request body, and guessing costs a false type rather than a vague one.
func requestSourceGateFor(w *wrapperMethod, params *paramIndex, body *bodySourceResolver, meta *metadata.Metadata, inner RequestBodyPattern, edge *metadata.CallGraphEdge) (requestSourceGate, bool) {
	// A pattern that asks for no verification derives as it always did; so does
	// every project whose config declares no RequestContext to verify against.
	if !inner.RequireRequestSource || !body.Enabled() {
		return requestSourceGate{}, true
	}
	src := innerBodySource(inner, edge, meta)
	if src == nil {
		return requestSourceGate{}, false
	}
	if body.IsRequestSource(src, edge) {
		return requestSourceGate{}, true
	}
	if i, ok := params.indexOf(w, src); ok {
		return requestSourceGate{require: true, argIndex: i}, true
	}
	return requestSourceGate{}, false
}

// innerBodySource returns the value the inner decode reads from, the way
// RequestPatternMatcherImpl.bodySource does at extraction time — from the
// decoder's receiver (`json.NewDecoder(src)`) or from a declared argument.
func innerBodySource(inner RequestBodyPattern, edge *metadata.CallGraphEdge, meta *metadata.Metadata) *metadata.CallArgument {
	if inner.BodyFromReceiver {
		return resolveReceiverSource(edge, meta)
	}
	return argAt(edge, inner.BodySourceArgIndex)
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
