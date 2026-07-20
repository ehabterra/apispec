// Copyright 2025 Ehab Terra
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
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/ehabterra/apispec/internal/metadata"
	"github.com/ehabterra/apispec/internal/typemodel"
)

const (
	// TypeSep aliases the type model's package/type separator; the single
	// source of truth is internal/typemodel.
	TypeSep    = typemodel.Sep
	defaultSep = "."

	// unresolvedStatus marks a ResponseInfo whose HTTP status could not be
	// pinned to a concrete code; buildResponses maps any StatusCode < 0 to the
	// OpenAPI "default" response. Used for the residue of a branched status set
	// with a non-constant branch (issue #155).
	unresolvedStatus = -1
)

// RouteInfo represents extracted route information
type RouteInfo struct {
	Path      string
	MountPath string
	Method    string
	Handler   string
	Package   string
	File      string
	Function  string
	Summary   string
	// Description is the operation's long description, sourced from the handler's
	// Go doc comment (issue #168) when not otherwise set.
	Description string
	Tags        []string
	Request     *RequestInfo
	Response    map[string]*ResponseInfo
	Params      []Parameter

	// OperationIDSuffix disambiguates the operationId when one handler yields
	// several operations (e.g. an r.Method dispatch split into GET/POST). Empty
	// for ordinary routes. Appended as "_<suffix>" to the computed operationId.
	OperationIDSuffix string

	// MethodExplicit is true when Method was resolved from the registration
	// (a verb-carrying call/arg/path, e.g. router.GET or "GET /x"), and false
	// when it fell back to the default. Only verb-less routes are eligible for
	// r.Method-dispatch splitting — a router that registers a concrete verb
	// won't dispatch the other verbs to the handler.
	MethodExplicit bool

	UsedTypes map[string]*Schema
	Metadata  *metadata.Metadata

	// Resolved router group prefix (if any)
	GroupPrefix string

	// Security holds the per-operation OpenAPI security requirements resolved
	// from auth middleware detected in scope. Semantics:
	//   nil          -> no per-operation security; the operation inherits the
	//                   document-level `security`.
	//   non-nil empty-> explicitly public (overrides global); renders as
	//                   `security: []`.
	//   non-empty    -> the operation is protected by these requirements.
	Security []SecurityRequirement

	// DynamicParams names path placeholders synthesized from unresolvable
	// call expressions (issue #34). The mapper uses these to emit one
	// shared component parameter per name and $ref it from each operation
	// instead of inlining a fresh declaration on every route.
	DynamicParams []string

	// Node is the tracker-tree node where this route was matched (the route
	// registration call). Its subtree is the interface-resolved handler flow;
	// the insight view traverses it to build the resolution trace. Not part of
	// the spec output.
	Node TrackerNodeInterface `json:"-"`
}

// OpenAPIPath returns the route's effective OpenAPI path (mount + path,
// converted to {param} form) — the same key buildPathsFromRoutes emits, so
// callers can match a RouteInfo to an OpenAPI path.
func (r *RouteInfo) OpenAPIPath() string {
	return convertPathToOpenAPI(joinPaths(r.MountPath, r.Path))
}

func NewRouteInfo() *RouteInfo {
	return &RouteInfo{
		Response:  make(map[string]*ResponseInfo),
		UsedTypes: make(map[string]*Schema),
	}
}

// IsValid checks if the route info is valid
func (r *RouteInfo) IsValid() bool {
	return r.Path != "" && r.Handler != ""
}

// RequestInfo represents request information
type RequestInfo struct {
	ContentType string
	BodyType    string
	Schema      *Schema

	// File and Line locate the call site that produced this request body, used
	// to attribute it to an r.Method dispatch branch (see splitMethodDispatchRoutes).
	File string
	Line int
}

// ResponseInfo represents response information
type ResponseInfo struct {
	StatusCode  int
	ContentType string
	BodyType    string
	Schema      *Schema

	// File and Line locate the call site that produced this response, used to
	// attribute it to an r.Method dispatch branch (see splitMethodDispatchRoutes).
	File string
	Line int
}

// Extractor provides a cleaner, more modular approach to extraction
type Extractor struct {
	tree            TrackerTreeInterface
	cfg             *APISpecConfig
	contextProvider ContextProvider
	schemaMapper    SchemaMapper
	typeResolver    TypeResolver
	overrideApplier OverrideApplier

	// Pattern matchers
	routeMatchers    []RoutePatternMatcher
	mountMatchers    []MountPatternMatcher
	securityMatchers []SecurityPatternMatcher
	requestMatchers  []RequestPatternMatcher
	responseMatchers []ResponsePatternMatcher
	paramMatchers    []ParamPatternMatcher

	// securityUnresolved collects auth middleware that was detected but matched
	// no SecurityMapping, deduped by identity. Surfaced as a warning (CLI) and
	// to the UI for interactive mapping.
	securityUnresolved    []MiddlewareRef
	securityUnresolvedSet map[string]struct{}

	// pathParamMismatches collects handlers that read a map-key path variable
	// (e.g. mux.Vars(r)["userId"]) whose key is not declared as a `{placeholder}`
	// in the route path — a likely typo, since the read will always be empty.
	pathParamMismatches  []PathParamMismatch
	pathParamMismatchSet map[string]struct{}

	// parentFnIndex maps a function's BaseID to call edges made inside func
	// literals lexically nested in it (keyed by ParentFunction). Lets wrapper
	// look-through reach a library call that lives in the closure a middleware
	// returns — the dominant net/http/echo idiom. Built lazily.
	parentFnIndex map[string][]*metadata.CallGraphEdge

	// scc is the call graph's SCC condensation, built lazily and shared by
	// every reachability summary (docs/TRACKER_REDESIGN.md step 3).
	scc *metadata.CallGraphSCC
	// callDepthsByFn caches handlerCallDepths per handler function: the BFS
	// is over the whole call graph and pairAndFillResponses runs once per
	// route extraction context.
	callDepthsByFn map[string]map[string]int
	// extractedRouteIDs marks route identities whose subtree walk has
	// already run in this extraction. Fragment extraction is pure, so a
	// re-visit of the same (function, mount, path, method) through another
	// traversal context reproduces byte-identical results — skip the walk.
	extractedRouteIDs map[string]bool

	// Per-edge matcher-verdict memos. The MatchNode implementations of the
	// response/request/param matcher families are pure functions of the call
	// edge (callee name/receiver/package and caller name), and the lazy tree
	// visits the same edge through many node copies — memoizing the FIRST
	// matching matcher index per edge (-1 = none) removes the dominant
	// repeated regex work from route-subtree walks. Extraction itself stays
	// per-node: it depends on the node's ancestry.
	respMatcherByEdge  map[*metadata.CallGraphEdge]int16
	reqMatcherByEdge   map[*metadata.CallGraphEdge]int16
	paramMatcherByEdge map[*metadata.CallGraphEdge]int16
	// Route matching keeps ALL matching indexes (not just the first):
	// executeRoutePattern arbitrates between them by priority and extraction
	// success. Multi-framework config merging multiplied the route-matcher
	// count, which made the per-node linear scan visible on large-project
	// profiles; matching depends only on edge facts, so it memoizes cleanly.
	routeMatchersByEdge map[*metadata.CallGraphEdge][]int16
	// reachSets caches, per accessor pattern, which function BaseIDs
	// transitively reach a matching call. See reachability.go.
	reachSets map[string]map[string]bool
	// mwResolved memoizes, per function BaseID, the mapping-matching
	// middleware refs transitively reachable through it. See reachability.go.
	mwResolved map[string][]MiddlewareRef
	mwOnStack  map[string]bool
}

// NewExtractor creates a new refactored extractor
func NewExtractor(tree TrackerTreeInterface, cfg *APISpecConfig) *Extractor {
	contextProvider := NewContextProvider(tree.GetMetadata())
	schemaMapper := NewSchemaMapper(cfg)
	typeResolver := NewTypeResolver(tree.GetMetadata(), cfg, schemaMapper)
	overrideApplier := NewOverrideApplier(cfg)

	extractor := &Extractor{
		tree:            tree,
		cfg:             cfg,
		contextProvider: contextProvider,
		schemaMapper:    schemaMapper,
		typeResolver:    typeResolver,
		overrideApplier: overrideApplier,
	}

	// Initialize pattern matchers
	extractor.initializePatternMatchers()

	return extractor
}

// initializePatternMatchers initializes all pattern matchers
func (e *Extractor) initializePatternMatchers() {
	// Initialize route matchers
	for _, pattern := range e.cfg.Framework.RoutePatterns {
		matcher := NewRoutePatternMatcher(pattern, e.cfg, e.contextProvider, e.typeResolver)
		e.routeMatchers = append(e.routeMatchers, matcher)
	}

	// Initialize mount matchers
	for _, pattern := range e.cfg.Framework.MountPatterns {
		matcher := NewMountPatternMatcher(pattern, e.cfg, e.contextProvider, e.typeResolver)
		e.mountMatchers = append(e.mountMatchers, matcher)
	}

	// Initialize security matchers
	for _, pattern := range e.cfg.Framework.SecurityPatterns {
		matcher := NewSecurityPatternMatcher(pattern, e.cfg, e.contextProvider, e.typeResolver)
		e.securityMatchers = append(e.securityMatchers, matcher)
	}

	// Initialize request matchers
	for _, pattern := range e.cfg.Framework.RequestBodyPatterns {
		matcher := NewRequestPatternMatcher(pattern, e.cfg, e.contextProvider, e.typeResolver)
		e.requestMatchers = append(e.requestMatchers, matcher)
	}

	// Initialize response matchers
	for _, pattern := range e.cfg.Framework.ResponsePatterns {
		matcher := NewResponsePatternMatcher(pattern, e.cfg, e.contextProvider, e.typeResolver)
		e.responseMatchers = append(e.responseMatchers, matcher)
	}

	// Initialize param matchers
	for _, pattern := range e.cfg.Framework.ParamPatterns {
		matcher := NewParamPatternMatcher(pattern, e.cfg, e.contextProvider, e.typeResolver)
		e.paramMatchers = append(e.paramMatchers, matcher)
	}
}

// ExtractRoutes extracts all routes from the tracker tree
func (e *Extractor) ExtractRoutes() []*RouteInfo {
	routes := make([]*RouteInfo, 0)
	for _, root := range e.tree.GetRoots() {
		e.traverseForRoutes(root, "", nil, nil, nil, &routes)
	}
	routes = dropSubsumedMountPrefixes(routes)

	// Split handlers that dispatch on r.Method (switch/if) into one route per
	// HTTP method, before the per-route diagnostics below run on the settled set.
	routes = splitMethodDispatchRoutes(routes)

	// Diagnose map-key path-variable reads whose key matches no path placeholder.
	// Done over the finalised route set so method/path are settled (handleRouteNode
	// runs on transient, pre-dedup route objects).
	for _, r := range routes {
		e.recordPathVarKeyMismatches(r)
	}
	return routes
}

// dropSubsumedMountPrefixes removes spurious partially-mounted duplicates of a
// route. A nested mount (e.g. main mounts /api → handler, handler mounts /user
// → userRoutes) is reached by the traversal through several contexts, so the
// same handler can be emitted at every *subset* of its mount chain
// (/api/user/{id} but also /api/{id}, /user/{id}, /{id}). Only the most-mounted
// one is real.
//
// A route is dropped when, for the same (Function, Method, Path), another route
// exists whose mount-path segments are a strict ordered *superset*: i.e. this
// route's segments are a subsequence of the other's. Genuine multi-mounts of
// the same sub-router at distinct prefixes (e.g. /v2/api and /{mountPoint}) are
// not subsequences of each other, so both survive.
func dropSubsumedMountPrefixes(routes []*RouteInfo) []*RouteInfo {
	type key struct{ fn, method, path string }
	groups := make(map[key][]int)
	segs := make([][]string, len(routes))
	for i, r := range routes {
		segs[i] = mountSegments(r.MountPath)
		k := key{r.Function, r.Method, r.Path}
		groups[k] = append(groups[k], i)
	}

	drop := make([]bool, len(routes))
	for _, idxs := range groups {
		if len(idxs) < 2 {
			continue
		}
		for _, i := range idxs {
			for _, j := range idxs {
				if i == j {
					continue
				}
				// Drop i if its segments are a strictly shorter subsequence
				// of j's (j is the more-complete mount of the same handler).
				if len(segs[i]) < len(segs[j]) && isSubsequence(segs[i], segs[j]) {
					drop[i] = true
					break
				}
			}
		}
	}

	// A dropped duplicate is the same handler reached through a partial
	// mount context, and its extraction resolved real fragments (error
	// bodies, defaults) that the surviving context may not have walked.
	// Fold each dropped route into its survivors before discarding, so the
	// output does not depend on which traversal context wins subsumption.
	// (Safe now that fragments are extracted purely and paired by frame —
	// there is no order-dependent junk left to amplify.)
	for _, idxs := range groups {
		var keep, dropped []int
		for _, i := range idxs {
			if drop[i] {
				dropped = append(dropped, i)
			} else {
				keep = append(keep, i)
			}
		}
		for _, di := range dropped {
			for _, ki := range keep {
				mergeRouteExtraction(routes[ki], routes[di])
			}
		}
	}

	out := routes[:0]
	for i, r := range routes {
		if !drop[i] {
			out = append(out, r)
		}
	}
	return out
}

// mountSegments splits a mount path into its non-empty segments
// ("/api/user/" → ["api","user"], "" → []).
func mountSegments(mountPath string) []string {
	mountPath = strings.Trim(mountPath, "/")
	if mountPath == "" {
		return nil
	}
	return strings.Split(mountPath, "/")
}

// isSubsequence reports whether a appears as an ordered (not necessarily
// contiguous) subsequence of b.
func isSubsequence(a, b []string) bool {
	if len(a) == 0 {
		return true
	}
	i := 0
	for _, x := range b {
		if x == a[i] {
			if i++; i == len(a) {
				return true
			}
		}
	}
	return false
}

// traverseForRoutes traverses the tree to find routes
func (e *Extractor) traverseForRoutes(node TrackerNodeInterface, mountPath string, mountTags []string, mountDynParams []string, mountMW []MiddlewareRef, routes *[]*RouteInfo) {
	e.traverseForRoutesWithVisited(node, mountPath, mountTags, mountDynParams, mountMW, routes, make(map[string]bool))
}

// traverseForRoutesWithVisited traverses with visited tracking to prevent cycles
func (e *Extractor) traverseForRoutesWithVisited(node TrackerNodeInterface, mountPath string, mountTags []string, mountDynParams []string, mountMW []MiddlewareRef, routes *[]*RouteInfo, visited map[string]bool) {
	if node == nil {
		return
	}

	// Prevent infinite recursion. The key includes mountPath so the same
	// sub-router can be mounted at multiple prefixes (each visit walks
	// the sub-tree under a different mount). Cycles within a single mount
	// context still short-circuit because mountPath only changes when a
	// Mount call introduces a new prefix — see issue #34 follow-up.
	nodeKey := node.GetKey() + "@" + mountPath
	if visited[nodeKey] {
		return
	}
	visited[nodeKey] = true

	routeInfo := NewRouteInfo()

	// Check for mount patterns first
	if mountInfo, isMount := e.executeMountPattern(node); isMount {
		e.handleMountNode(node, mountInfo, mountPath, mountTags, mountDynParams, mountMW, routes, visited)
	} else if isRoute := e.executeRoutePattern(node, routeInfo); isRoute {
		// Check for route patterns
		e.handleRouteNode(node, routeInfo, mountPath, mountTags, mountDynParams, mountMW, routes)
	} else {
		// Continue traversing children. Router-scoped auth middleware (e.g.
		// chi/echo `Use`) applies to its sibling routes/mounts that share the
		// same caller scope, so gather it per-caller and fold it into each
		// child's carried security.
		children := node.GetChildren()
		routerByCaller := e.collectRouterSecurityByCaller(children)
		for _, child := range children {
			childMW := mergeMW(mountMW, routerByCaller[e.callerKey(child)])
			e.traverseForRoutesWithVisited(child, mountPath, mountTags, mountDynParams, childMW, routes, visited)
		}
	}
}

// executeMountPattern executes mount pattern matching
func (e *Extractor) executeMountPattern(node TrackerNodeInterface) (MountInfo, bool) {
	var bestMatch MountInfo
	var bestPriority int
	var found bool

	for _, matcher := range e.mountMatchers {
		if matcher.MatchNode(node) {
			priority := matcher.GetPriority()
			if !found || priority > bestPriority {
				mountInfo := matcher.ExtractMount(node)
				bestMatch = mountInfo
				bestPriority = priority
				found = true
			}
		}
	}

	return bestMatch, found
}

// executeRoutePattern executes route pattern matching
func (e *Extractor) executeRoutePattern(node TrackerNodeInterface, routeInfo *RouteInfo) bool {
	var bestPriority int
	var found bool

	for _, i := range e.routeMatchersFor(node) {
		matcher := e.routeMatchers[i]
		priority := matcher.GetPriority()
		if !found || priority > bestPriority {
			found = matcher.ExtractRoute(node, routeInfo)
			if found {
				bestPriority = priority
			}
		}
	}

	return found
}

// routeMatchersFor returns the indexes of route matchers accepting the
// node's edge, in matcher order, memoized per edge (nil = none). Route
// MatchNode reads only edge facts (callee name/receiver/package, caller
// name), never node ancestry, so the set is a property of the edge.
func (e *Extractor) routeMatchersFor(node TrackerNodeInterface) []int16 {
	if node == nil || node.GetEdge() == nil {
		return nil
	}
	edge := node.GetEdge()
	if idxs, ok := e.routeMatchersByEdge[edge]; ok {
		return idxs
	}
	var idxs []int16
	for i, matcher := range e.routeMatchers {
		if matcher.MatchNode(node) {
			idxs = append(idxs, int16(i))
		}
	}
	if e.routeMatchersByEdge == nil {
		e.routeMatchersByEdge = map[*metadata.CallGraphEdge][]int16{}
	}
	e.routeMatchersByEdge[edge] = idxs
	return idxs
}

// collectNodeSecurity runs the security matchers on a single node and returns
// the middleware refs of the best-priority match plus its scope. matched is
// false when no security pattern applies (the common case).
func (e *Extractor) collectNodeSecurity(node TrackerNodeInterface) (refs []MiddlewareRef, scope string, matched bool) {
	var bestPriority int
	for _, m := range e.securityMatchers {
		if !m.MatchNode(node) {
			continue
		}
		priority := m.GetPriority()
		if !matched || priority > bestPriority {
			refs = m.ExtractMiddleware(node)
			scope = m.Scope()
			bestPriority = priority
			matched = true
		}
	}
	return refs, scope, matched
}

// callerKey identifies the enclosing function/closure of a node's call. Closure
// bodies are flattened into the tree, so a node's siblings may belong to
// different callers; router-scope middleware must only reach siblings sharing
// its caller (e.g. chi `rg.Use` inside a Group(func(rg){...}) closure applies to
// the group's routes, not to a Mount registered on the outer router).
func (e *Extractor) callerKey(node TrackerNodeInterface) string {
	if node == nil || node.GetEdge() == nil {
		return ""
	}
	edge := node.GetEdge()
	return e.contextProvider.GetString(edge.Caller.Name) + "|" + e.contextProvider.GetString(edge.Caller.Pkg)
}

// collectChainSecurity walks the route's chain-parent edges (e.g. the With in
// r.With(mw).Get(...)) and collects route-scope middleware declared on them.
// Because the chain links a route to exactly the calls it is chained on, this
// guards only that route — no leakage to sibling routes.
func (e *Extractor) collectChainSecurity(node TrackerNodeInterface) []MiddlewareRef {
	if node == nil || len(e.securityMatchers) == 0 {
		return nil
	}
	edge := node.GetEdge()
	if edge == nil {
		return nil
	}
	var refs []MiddlewareRef
	for parent := edge.ChainParent; parent != nil; parent = parent.ChainParent {
		var bestPriority int
		var bestRefs []MiddlewareRef
		var found bool
		for _, m := range e.securityMatchers {
			if m.Scope() != SecurityScopeRoute || !m.MatchEdge(parent) {
				continue
			}
			if p := m.GetPriority(); !found || p > bestPriority {
				bestRefs = m.ExtractMiddlewareFromEdge(parent)
				bestPriority = p
				found = true
			}
		}
		refs = append(refs, bestRefs...)
	}
	return refs
}

// collectRouterSecurityByCaller gathers router-scope middleware (e.g. `Use`)
// from a set of sibling nodes, grouped by their caller, so each sibling only
// inherits middleware declared in its own enclosing scope. This is an
// over-approximation of Go's "applies to routes registered after this call":
// in real code Use precedes the routes it guards, so folding it into every
// same-caller sibling is correct in practice.
func (e *Extractor) collectRouterSecurityByCaller(children []TrackerNodeInterface) map[string][]MiddlewareRef {
	if len(e.securityMatchers) == 0 {
		return nil
	}
	var byCaller map[string][]MiddlewareRef
	for _, child := range children {
		if refs, scope, ok := e.collectNodeSecurity(child); ok && scope == SecurityScopeRouter && len(refs) > 0 {
			if byCaller == nil {
				byCaller = make(map[string][]MiddlewareRef)
			}
			k := e.callerKey(child)
			byCaller[k] = append(byCaller[k], refs...)
		}
	}
	return byCaller
}

// mergeMW returns base + extra, deduped, without mutating base. Returns base
// unchanged when there is nothing to add.
func mergeMW(base, extra []MiddlewareRef) []MiddlewareRef {
	if len(extra) == 0 {
		return base
	}
	return dedupMiddlewareRefs(append(append([]MiddlewareRef{}, base...), extra...))
}

// recordUnresolved adds unmatched middleware to the diagnostics list, deduped
// by identity.
func (e *Extractor) recordUnresolved(refs []MiddlewareRef) {
	if len(refs) == 0 {
		return
	}
	if e.securityUnresolvedSet == nil {
		e.securityUnresolvedSet = make(map[string]struct{})
	}
	for _, r := range refs {
		key := r.String()
		if _, ok := e.securityUnresolvedSet[key]; ok {
			continue
		}
		e.securityUnresolvedSet[key] = struct{}{}
		e.securityUnresolved = append(e.securityUnresolved, r)
	}
}

// UnresolvedSecurity returns auth middleware detected during extraction that
// matched no SecurityMapping (deduped). Empty when nothing was unresolved.
func (e *Extractor) UnresolvedSecurity() []MiddlewareRef {
	return e.securityUnresolved
}

// dedupMiddlewareRefs removes duplicate refs by identity, preserving order.
func dedupMiddlewareRefs(refs []MiddlewareRef) []MiddlewareRef {
	if len(refs) <= 1 {
		return refs
	}
	seen := make(map[string]struct{}, len(refs))
	out := refs[:0]
	for _, r := range refs {
		key := r.String()
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, r)
	}
	return out
}

// applyRouteSecurity resolves and sets routeInfo.Security from the inherited
// (router/subtree) middleware plus any route-scope or handler-wrapper middleware
// on the route registration call itself. Unmatched middleware is recorded for
// diagnostics. When no security is configured/detected, routeInfo.Security is
// left nil so the operation inherits the document-level security and output is
// unchanged.
func (e *Extractor) applyRouteSecurity(node TrackerNodeInterface, routeInfo *RouteInfo, mountMW []MiddlewareRef) {
	if len(e.securityMatchers) == 0 {
		return
	}

	// Definite middleware: inherited router/subtree middleware plus route-scope
	// middleware on the call itself. These come from dedicated middleware slots,
	// so an unresolved one genuinely means "you forgot to map it" -> warn.
	definite := append([]MiddlewareRef{}, mountMW...)

	// Speculative middleware: the handler argument of a net/http-style Handle is
	// a wrapping call (auth(h)) that is syntactically indistinguishable from a
	// handler factory (newUserHandler()). We only treat it as auth when looking
	// through its body finds a known auth library; otherwise it is silently
	// ignored (no warning), since it is probably not middleware at all.
	var speculative []MiddlewareRef

	if refs, scope, ok := e.collectNodeSecurity(node); ok {
		switch scope {
		case SecurityScopeRoute:
			definite = append(definite, refs...)
		case SecurityScopeWrapper:
			speculative = append(speculative, refs...)
		}
	}

	// Chained-call middleware (e.g. chi r.With(mw).Get(...)): the middleware is
	// on the route's chain-parent edge, which guards only this route.
	definite = append(definite, e.collectChainSecurity(node)...)

	var reqs []SecurityRequirement
	public := false

	if d := dedupMiddlewareRefs(definite); len(d) > 0 {
		// Look through custom wrappers (e.g. middleware.Protected() that calls
		// jwtware.New) to the underlying library scheme.
		d = e.expandMiddlewareRefs(d)
		r, pub, unresolved := resolveSecurity(d, e.cfg.SecurityMappings)
		reqs = append(reqs, r...)
		public = public || pub
		e.recordUnresolved(unresolved)
	}
	if sp := dedupMiddlewareRefs(speculative); len(sp) > 0 {
		sp = e.expandMiddlewareRefs(sp)
		r, pub, _ := resolveSecurity(sp, e.cfg.SecurityMappings) // ignore unresolved (ambiguous slot)
		reqs = append(reqs, r...)
		public = public || pub
	}

	reqs = dedupSecurityRequirements(reqs)
	switch {
	case public:
		// Explicitly public: override any inherited/global security.
		routeInfo.Security = []SecurityRequirement{}
	case len(reqs) > 0:
		routeInfo.Security = reqs
	}
}

// expandMiddlewareRefs replaces custom-wrapper middleware with the library
// middleware it transitively calls, so a project-defined wrapper resolves to the
// underlying scheme. Refs that already match a mapping, or that can't be looked
// through (external/no body, or nothing matching found), are kept unchanged so
// genuinely-unmapped middleware is still reported.
func (e *Extractor) expandMiddlewareRefs(refs []MiddlewareRef) []MiddlewareRef {
	if len(e.cfg.SecurityMappings) == 0 {
		return refs
	}
	meta := e.tree.GetMetadata()
	if meta == nil || meta.Callers == nil {
		return refs
	}
	e.ensureParentFnIndex(meta)
	var out []MiddlewareRef
	for _, ref := range refs {
		if anyMappingMatches(ref, e.cfg.SecurityMappings) {
			out = append(out, ref)
			continue
		}
		key := middlewareBaseID(ref)
		if key == "" {
			out = append(out, ref)
			continue
		}
		if resolved := e.middlewareMatchesThrough(key, meta); len(resolved) > 0 {
			out = append(out, resolved...)
			continue
		}
		out = append(out, ref) // nothing matched downstream; keep for diagnostics
	}
	return dedupMiddlewareRefs(out)
}

// ensureParentFnIndex builds parentFnIndex once from the call graph: edges made
// inside func literals, grouped by the BaseID of the lexically-enclosing
// function. Used so look-through can follow into the closure a wrapper returns.
func (e *Extractor) ensureParentFnIndex(meta *metadata.Metadata) {
	if e.parentFnIndex != nil {
		return
	}
	idx := make(map[string][]*metadata.CallGraphEdge)
	for i := range meta.CallGraph {
		edge := &meta.CallGraph[i]
		if edge.ParentFunction == nil {
			continue
		}
		key := edge.ParentFunction.BaseID()
		if key == "" {
			continue
		}
		idx[key] = append(idx[key], edge)
	}
	e.parentFnIndex = idx
}

// calleeMiddlewareRef builds a MiddlewareRef from a call edge's callee.
func (e *Extractor) calleeMiddlewareRef(edge *metadata.CallGraphEdge) MiddlewareRef {
	if edge == nil {
		return MiddlewareRef{}
	}
	return MiddlewareRef{
		FunctionName: e.contextProvider.GetString(edge.Callee.Name),
		Pkg:          e.contextProvider.GetString(edge.Callee.Pkg),
		RecvType:     strings.TrimPrefix(e.contextProvider.GetString(edge.Callee.RecvType), "*"),
	}
}

// handleMountNode handles a mount node
func (e *Extractor) handleMountNode(node TrackerNodeInterface, mountInfo MountInfo, mountPath string, mountTags []string, mountDynParams []string, mountMW []MiddlewareRef, routes *[]*RouteInfo, visited map[string]bool) {
	// Update mount path if needed
	if mountInfo.Path != "" {
		if mountPath == "" || !strings.HasSuffix(mountPath, mountInfo.Path) {
			mountPath = joinPaths(mountPath, mountInfo.Path)
		}
	}

	// Carry dynamic placeholder names from this mount into nested routes
	// so each child operation $refs the shared component parameter.
	childDynParams := mountDynParams
	if len(mountInfo.DynamicParams) > 0 {
		childDynParams = appendUniqueStrings(mountDynParams, mountInfo.DynamicParams...)
	}

	// Subtree-scope middleware declared on the mount call itself
	// (e.g. echo/gin/fiber Group("/x", mw...)) guards the entire mounted
	// subtree, so fold it into the security carried to all children/assignment.
	subtreeMW := mountMW
	if refs, scope, ok := e.collectNodeSecurity(node); ok && scope == SecurityScopeSubtree {
		subtreeMW = mergeMW(mountMW, refs)
	}
	// Router-scope middleware among the mount's children (e.g. a `Use` inside a
	// chi Group(func(r){ r.Use(...); ... }) closure) is correlated per caller.
	routerByCaller := e.collectRouterSecurityByCaller(node.GetChildren())

	// Handle router assignment if present
	if mountInfo.Assignment != nil {
		e.handleRouterAssignment(mountInfo, mountPath, mountTags, childDynParams, subtreeMW, routes, visited)
	}

	// Continue traversing children
	for _, child := range node.GetChildren() {
		var newTags []string
		if mountPath != "" {
			newTags = []string{mountPath}
		} else {
			newTags = mountTags
		}
		childMW := mergeMW(subtreeMW, routerByCaller[e.callerKey(child)])
		e.traverseForRoutesWithVisited(child, mountPath, newTags, childDynParams, childMW, routes, visited)
	}
}

// handleRouteNode handles a route node
func (e *Extractor) handleRouteNode(node TrackerNodeInterface, routeInfo *RouteInfo, mountPath string, mountTags []string, mountDynParams []string, mountMW []MiddlewareRef, routes *[]*RouteInfo) {
	// Remember the matched node so consumers (e.g. the insight trace) can
	// traverse the interface-resolved handler subtree.
	routeInfo.Node = node
	// Prepend mount path if present
	if mountPath != "" {
		routeInfo.MountPath = joinPaths(mountPath, routeInfo.MountPath)
	}

	// Set tags from mountTags if present
	if len(mountTags) > 0 {
		routeInfo.Tags = mountTags
	}

	// Merge inherited mount dynamic params with any produced by the route itself.
	if len(mountDynParams) > 0 {
		routeInfo.DynamicParams = appendUniqueStrings(mountDynParams, routeInfo.DynamicParams...)
	}

	// Resolve per-operation security: inherited (router/subtree) middleware plus
	// any route-scope or handler-wrapper middleware on the route call itself.
	e.applyRouteSecurity(node, routeInfo, mountMW)

	// The same route CALL SITE reached again through another traversal
	// context reproduces byte-identical extraction (fragments are pure and
	// children expansions are memoized), so the expensive subtree walk runs
	// once per site. The key must be the matched call site's identity, not
	// the extracted fields: chain-style routes (Methods("GET").Path(...).
	// HandlerFunc(...)) arrive here with Function/Path still empty — they
	// resolve from chain children during extraction — so field-based keys
	// would alias every such chain in the package onto one identity.
	// Distinct mount contexts have distinct keys and still run — their
	// fragments merge below.
	if edge := node.GetEdge(); edge != nil {
		routeID := routeInfo.MountPath + chainSep + edge.Callee.ID()
		if e.extractedRouteIDs[routeID] {
			return
		}
		if e.extractedRouteIDs == nil {
			e.extractedRouteIDs = map[string]bool{}
		}
		e.extractedRouteIDs[routeID] = true
	}

	// Extract route/request/response/params from children. Response
	// candidates are collected with their call-site CHAIN during the walk
	// and resolved afterwards by pairAndFillResponses — see there for the
	// order-insensitive pairing model.
	visitedEdges := make(map[chainStep]bool)
	var respCandidates []responseCandidate
	e.extractRouteChildren(node, routeInfo, mountTags, routes, visitedEdges, &chainInterner{}, 0, &respCandidates)
	e.pairAndFillResponses(routeInfo, respCandidates)

	// Add map-key path params (mux.Vars) for placeholders the handler reads via
	// the accessor — including through helper wrappers the subtree walk misses.
	e.completeMapKeyPathParams(routeInfo)

	// Apply overrides
	e.overrideApplier.ApplyOverrides(routeInfo)

	if routeInfo.IsValid() && routes != nil {
		// Update existing route or add new one. Dedup key is the
		// effective OpenAPI identity (mount + path + method + handler)
		// rather than just the handler name, so the same sub-router
		// mounted at multiple prefixes yields one route per prefix
		// instead of the last-mount-wins behaviour from before.
		var found bool
		for i := range *routes {
			if (*routes)[i].Function == routeInfo.Function &&
				(*routes)[i].MountPath == routeInfo.MountPath &&
				(*routes)[i].Path == routeInfo.Path &&
				(*routes)[i].Method == routeInfo.Method {
				mergeRouteExtraction((*routes)[i], routeInfo)
				found = true
				break
			}
		}
		if !found {
			*routes = append(*routes, routeInfo)
		}
	}
}

// mergeRouteExtraction folds a re-extraction of the same route (reached
// through another traversal context) into the existing RouteInfo instead of
// replacing it wholesale. Different contexts can each resolve fragments the
// others miss — e.g. one context binds the success body to the default slot
// while another loses it to an error status — so the union with
// informative-wins slot competition keeps extraction order-independent.
func mergeRouteExtraction(existing, next *RouteInfo) {
	for slot, resp := range next.Response {
		existing.Response[slot] = preferResponseInfo(existing.Response[slot], resp)
	}
	existing.Request = preferRequestInfo(existing.Request, next.Request)
	if len(existing.Params) == 0 {
		existing.Params = next.Params
	}
	if len(existing.Tags) == 0 {
		existing.Tags = next.Tags
	}
	if len(existing.Security) == 0 {
		existing.Security = next.Security
	}
}

// handleRouterAssignment handles router assignment for mounts
func (e *Extractor) handleRouterAssignment(mountInfo MountInfo, mountPath string, mountTags []string, mountDynParams []string, mountMW []MiddlewareRef, routes *[]*RouteInfo, visited map[string]bool) {
	// Find the target node for the assignment
	targetNode := e.findTargetNode(mountInfo.Assignment)
	if targetNode != nil {
		// Router-scope middleware among the assigned router's children (e.g.
		// a `Use` registered on the sub-router) guards its routes, correlated
		// per caller.
		children := targetNode.GetChildren()
		routerByCaller := e.collectRouterSecurityByCaller(children)
		for _, child := range children {
			var newTags []string
			if mountPath != "" {
				newTags = []string{mountPath}
			} else {
				newTags = mountTags
			}
			childMW := mergeMW(mountMW, routerByCaller[e.callerKey(child)])
			e.traverseForRoutesWithVisited(child, mountPath, newTags, mountDynParams, childMW, routes, visited)
		}
	}
}

// appendUniqueStrings returns base + extras with duplicates removed,
// preserving first-seen order. Used to merge mount-inherited dynamic
// placeholder names into a route without ballooning the slice.
func appendUniqueStrings(base []string, extras ...string) []string {
	if len(extras) == 0 {
		return base
	}
	seen := make(map[string]struct{}, len(base)+len(extras))
	out := make([]string, 0, len(base)+len(extras))
	for _, s := range base {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	for _, s := range extras {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

// findTargetNode finds the target node for an assignment
func (e *Extractor) findTargetNode(assignment *metadata.CallArgument) TrackerNodeInterface {
	if assignment == nil {
		return nil
	}

	// Use breadth-first search to find the target node
	queue := e.tree.GetRoots()
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:] // dequeue

		if node.GetKey() == assignment.ID() {
			return node
		}

		queue = append(queue, node.GetChildren()...)
	}

	return nil
}

// extractRouteChildren extracts request, response, and params from children nodes
// responseCandidate is a node that matched a response pattern during the
// route walk, together with the chain of enclosing call-site instance IDs
// (the "frames" entered from the route down to the statement). The chain is
// what pairs a bodyless status write with the body write that follows it in
// the same helper invocation — and keeps two invocations of the same helper
// (respondWithError(w, 400) vs (w, 500)) apart.
type responseCandidate struct {
	node  TrackerNodeInterface
	chain string
}

// chainSep separates call-site instance IDs in chain keys.
const chainSep = "\x1f"

// chainStep is one interned recursion step: the parent chain's handle plus
// the callee instance ID entered at that step. The walk previously built an
// O(depth) key string per visited child — quadratic in total bytes over deep
// walks and the dominant allocation source on large projects; interning makes
// each step one map operation while preserving value equality exactly, so
// dedupe behaviour is unchanged. chainStep doubles as the response-candidate
// dedupe key (parent = frame handle, callee = statement's call-site ID).
type chainStep struct {
	parent int
	callee string
}

// chainInterner assigns stable small handles to recursion chains within one
// route walk. Handle 0 is the empty chain — the route/handler frame.
type chainInterner struct {
	ids   map[chainStep]int
	steps []chainStep // handle n is steps[n-1]
}

func (ci *chainInterner) push(parent int, callee string) int {
	st := chainStep{parent: parent, callee: callee}
	if id, ok := ci.ids[st]; ok {
		return id
	}
	if ci.ids == nil {
		ci.ids = map[chainStep]int{}
	}
	ci.steps = append(ci.steps, st)
	ci.ids[st] = len(ci.steps)
	return len(ci.steps)
}

// strings reconstructs a handle's chain root→leaf. Only response candidates
// pay this cost — the hot walk never materializes chains.
func (ci *chainInterner) strings(id int) []string {
	var n int
	for h := id; h != 0; h = ci.steps[h-1].parent {
		n++
	}
	if n == 0 {
		return nil
	}
	out := make([]string, n)
	for h := id; h != 0; h = ci.steps[h-1].parent {
		n--
		out[n] = ci.steps[h-1].callee
	}
	return out
}

// frameChainKey identifies the FRAME a response statement executes in: the
// recursion chain truncated at the invocation of the statement's caller
// function (keeping the invocation prefix, so two invocations of the same
// helper stay distinct). Statements whose caller is not on the chain execute
// in the route/handler frame itself — leaf-call detours the walk descends
// through (an encoder chain, a fiber Status().JSON() chain) must not split
// that frame, so the key falls back to the route frame ("").
func frameChainKey(chain []string, node TrackerNodeInterface) string {
	edge := node.GetEdge()
	if edge == nil {
		return strings.Join(chain, chainSep)
	}
	callerBase := edge.Caller.BaseID()
	for i := len(chain) - 1; i >= 0; i-- {
		if metadata.StripToBase(chain[i]) == callerBase {
			return strings.Join(chain[:i+1], chainSep)
		}
	}
	return "" // route/handler frame
}

func (e *Extractor) extractRouteChildren(routeNode TrackerNodeInterface, route *RouteInfo, mountTags []string, routes *[]*RouteInfo, visitedEdges map[chainStep]bool, ci *chainInterner, chainID int, respCandidates *[]responseCandidate) {
	for _, child := range routeNode.GetChildren() {
		// Check for route patterns in children nodes
		if isRoute := e.executeRoutePattern(child, route); isRoute {
			e.handleRouteNode(child, route, "", mountTags, route.DynamicParams, nil, routes)
		}

		// Extract request. A route's body may be matched at several nodes
		// (e.g. the same handler reached through more than one tracker path);
		// keep the most specific result so a concrete type isn't clobbered by
		// a later generic `object` — which happens when one path resolves the
		// type through a binding wrapper and another doesn't.
		if req := e.extractRequestFromNode(child, route); req != nil {
			// Record the call site so a method-dispatch handler can attribute
			// this request body to the right verb branch by line range.
			if f, l, _ := calleePosition(child); req.File == "" {
				req.File, req.Line = f, l
			}
			route.Request = preferRequestInfo(route.Request, req)
		}

		// Collect response candidates with their call-site chain; resolution
		// and pairing happen in pairAndFillResponses once the walk is done.
		// Deduped per (chain, call site): the same statement reached again
		// through the same frames is one candidate; the same statement in a
		// different helper invocation is a separate one.
		// Match first (memoized per edge — cheap), and only then build the
		// per-(chain, site) dedupe key; the chain string itself is
		// reconstructed from the interner only for actual candidates.
		if child != nil && child.GetEdge() != nil && e.matchesResponsePattern(child) {
			candKey := chainStep{parent: chainID, callee: child.GetEdge().Callee.ID()}
			if !visitedEdges[candKey] {
				visitedEdges[candKey] = true
				*respCandidates = append(*respCandidates, responseCandidate{node: child, chain: frameChainKey(ci.strings(chainID), child)})
			}
		}

		// Extract parameters
		route.Params = append(route.Params, e.extractParamsFromNode(child, route)...)

		// Recursive extraction. The chain grows only through CALL nodes —
		// argument nodes reference values within the current frame.
		childChainID := chainID
		if child != nil && child.GetArgument() == nil && child.GetEdge() != nil {
			childChainID = ci.push(chainID, child.GetEdge().Callee.ID())
		}
		e.extractRouteChildren(child, route, mountTags, routes, visitedEdges, ci, childChainID, respCandidates)
	}

	// Extract parameters from the route node itself
	route.Params = append(route.Params, e.extractParamsFromNode(routeNode, route)...)
}

// matchesResponsePattern reports whether any response matcher accepts the node.
func (e *Extractor) matchesResponsePattern(node TrackerNodeInterface) bool {
	return e.responseMatcherIndex(node) >= 0
}

// responseMatcherIndex returns the first response matcher accepting the
// node's edge, memoized per edge (see the memo fields for why this is sound).
func (e *Extractor) responseMatcherIndex(node TrackerNodeInterface) int16 {
	if node == nil || node.GetEdge() == nil {
		return -1
	}
	edge := node.GetEdge()
	if idx, ok := e.respMatcherByEdge[edge]; ok {
		return idx
	}
	idx := int16(-1)
	for i, matcher := range e.responseMatchers {
		if matcher.MatchNode(node) {
			idx = int16(i)
			break
		}
	}
	if e.respMatcherByEdge == nil {
		e.respMatcherByEdge = map[*metadata.CallGraphEdge]int16{}
	}
	e.respMatcherByEdge[edge] = idx
	return idx
}

// pairAndFillResponses resolves the collected response candidates and fills
// route.Response, replacing the old "attach body to the lowest bodyless
// status seen so far" behavior — which depended on traversal order — with a
// deterministic model:
//
//  1. every candidate is extracted as a PURE function (against an empty
//     response map, so no slot peeking): statuses resolve only from the
//     pattern's own arguments (including parameter tracing) or its
//     configured default; unresolved statuses come out as -1;
//  2. fragments are deduped by (frame chain, call site, status, body) —
//     the same statement reached through shortcut relations within one
//     frame yields byte-identical fragments; the same statement executed
//     in DIFFERENT frames (a shared helper like respondWithError called
//     from two branches) must keep one fragment per frame so each frame's
//     pending status finds its body;
//  3. fragments are ordered by SOURCE POSITION and paired: a bodyless
//     status write leaves its status pending on its call-site chain, and
//     the next unknown-status body on the same chain adopts it — exactly
//     how `c.Status(400)` is followed by its `c.JSON(err)` in the code;
//  4. bodies that remain unpaired land in distinct negative slots (the
//     mapper's "default" collapse), numbered in source order.
//
// The model is independent of tree shape and traversal order: both tracker
// trees see the same call sites and chains.
func (e *Extractor) pairAndFillResponses(route *RouteInfo, candidates []responseCandidate) {
	type fragment struct {
		resp   *ResponseInfo
		chain  string
		caller string // fragment statement's enclosing function (BaseID)
		file   string
		line   int
		col    int
	}

	saved := route.Response
	var frags []fragment
	seen := map[string]bool{}
	for _, cand := range candidates {
		route.Response = map[string]*ResponseInfo{} // pure extraction: no slot peeking
		resps := e.extractResponsesMatched(cand.node, route)
		file, line, col := calleePosition(cand.node)
		siteID := cand.node.GetEdge().Callee.ID()
		caller := cand.node.GetEdge().Caller.BaseID()
		for _, resp := range resps {
			if resp == nil || (resp.BodyType == "" && resp.StatusCode < 100) {
				continue // nothing resolved
			}
			status := resp.StatusCode
			if status < 0 {
				status = -1 // normalize "unknown"
				resp.StatusCode = -1
			}
			dedupeKey := cand.chain + chainSep + siteID + chainSep + strconv.Itoa(status) + chainSep + resp.BodyType
			if seen[dedupeKey] {
				continue
			}
			seen[dedupeKey] = true
			// Carry the call-site position so a method-dispatch handler can
			// attribute this response to the right verb branch by line range.
			resp.File, resp.Line = file, line
			frags = append(frags, fragment{resp: resp, chain: cand.chain, caller: caller, file: file, line: line, col: col})
		}
	}
	route.Response = saved

	sort.SliceStable(frags, func(i, j int) bool {
		if frags[i].file != frags[j].file {
			return frags[i].file < frags[j].file
		}
		if frags[i].line != frags[j].line {
			return frags[i].line < frags[j].line
		}
		return frags[i].col < frags[j].col
	})

	store := func(resp *ResponseInfo) {
		slot := fmt.Sprintf("%d", resp.StatusCode)
		existing := route.Response[slot]
		switch {
		case existing == nil:
			route.Response[slot] = resp
		case existing.BodyType == "" && resp.BodyType != "":
			route.Response[slot] = resp
		case existing.BodyType != "" && resp.BodyType == "":
			// keep the informative one
		default:
			route.Response[slot] = preferResponseInfo(existing, resp)
		}
	}

	pending := map[string]bool{} // chain -> a bodyless status awaits its body
	pendingStatus := map[string]int{}
	var unpaired []*fragment
	for i := range frags {
		f := &frags[i]
		status, body := f.resp.StatusCode, f.resp.BodyType
		known := status >= 100 && status < 600
		switch {
		case known && body == "":
			store(f.resp)
			pending[f.chain] = true
			pendingStatus[f.chain] = status
		case known:
			store(f.resp)
		case body != "":
			if pending[f.chain] {
				f.resp.StatusCode = pendingStatus[f.chain]
				delete(pending, f.chain)
				delete(pendingStatus, f.chain)
				store(f.resp)
			} else {
				unpaired = append(unpaired, f)
			}
		}
	}

	// Unpaired bodies become undetermined-status ("default") candidates —
	// but only from the SHALLOWEST call depth present, measured as call-graph
	// distance from the route's handler to the statement's enclosing
	// function (tree-shape independent: both trackers share the metadata).
	// Response writes live in the handler or its immediate response helpers,
	// while deeper unknown bodies are almost always outbound payloads (an
	// Encode inside an HTTP client several calls down) that merely resemble
	// responses.
	if len(unpaired) > 0 {
		depths := e.handlerCallDepths(route)
		depthOf := func(f *fragment) int {
			if d, ok := depths[f.caller]; ok {
				return d
			}
			return 1 << 20 // unreachable from the handler: deepest possible
		}
		minDepth := -1
		for _, f := range unpaired {
			if d := depthOf(f); minDepth < 0 || d < minDepth {
				minDepth = d
			}
		}
		unknown := 0
		for _, f := range unpaired {
			if depthOf(f) != minDepth {
				continue
			}
			unknown++
			f.resp.StatusCode = -unknown
			store(f.resp)
		}
	}
}

// handlerCallDepths returns the call-graph distance (in hops) from the
// route's handler function to every function reachable from it, via a BFS
// over meta.Callers. Used to rank undetermined-status response fragments by
// how close to the handler they were written.
func (e *Extractor) handlerCallDepths(route *RouteInfo) map[string]int {
	if cached, ok := e.callDepthsByFn[route.Function]; ok {
		return cached
	}
	meta := route.Metadata
	if meta == nil {
		meta = e.tree.GetMetadata()
	}
	depths := map[string]int{}
	if meta == nil || route.Function == "" {
		return depths
	}
	start := route.Function
	depths[start] = 0
	queue := []string{start}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		next := depths[cur] + 1
		for _, edge := range meta.Callers[cur] {
			callee := edge.Callee.BaseID()
			if _, ok := depths[callee]; ok {
				continue
			}
			depths[callee] = next
			queue = append(queue, callee)
		}
	}
	if e.callDepthsByFn == nil {
		e.callDepthsByFn = map[string]map[string]int{}
	}
	e.callDepthsByFn[route.Function] = depths
	return depths
}

// extractResponsesMatched runs the first matching response matcher on a
// previously-collected candidate node.
func (e *Extractor) extractResponsesMatched(node TrackerNodeInterface, route *RouteInfo) []*ResponseInfo {
	if idx := e.responseMatcherIndex(node); idx >= 0 {
		return e.responseMatchers[idx].ExtractResponse(node, route)
	}
	return nil
}

// calleePosition parses "file:line:col" out of the node's callee position,
// for source-order sorting of response fragments.
func calleePosition(n TrackerNodeInterface) (string, int, int) {
	edge := n.GetEdge()
	if edge == nil {
		return "", 0, 0
	}
	pos := edge.Callee.ID()
	at := strings.LastIndexByte(pos, '@')
	if at < 0 {
		return pos, 0, 0
	}
	pos = pos[at+1:]
	lastColon := strings.LastIndexByte(pos, ':')
	if lastColon < 0 {
		return pos, 0, 0
	}
	col, _ := strconv.Atoi(pos[lastColon+1:])
	rest := pos[:lastColon]
	midColon := strings.LastIndexByte(rest, ':')
	if midColon < 0 {
		return rest, 0, col
	}
	line, _ := strconv.Atoi(rest[midColon+1:])
	return rest[:midColon], line, col
}

// preferRequestInfo chooses the more specific of two request bodies for the
// same route. A concrete schema (a named-type $ref, an object with properties,
// a composed allOf, or an array) beats a generic placeholder (`{type: object}`
// from an unresolved `interface{}`). On a tie the newer one wins, preserving
// the previous last-write-wins behaviour.
func preferRequestInfo(cur, next *RequestInfo) *RequestInfo {
	if cur == nil {
		return next
	}
	if next == nil {
		return cur
	}
	curConcrete, nextConcrete := requestIsConcrete(cur), requestIsConcrete(next)
	if nextConcrete && !curConcrete {
		return next
	}
	if curConcrete && !nextConcrete {
		return cur
	}
	return next
}

// requestIsConcrete reports whether a request body carries a resolved type
// rather than a generic `object` fallback.
func requestIsConcrete(r *RequestInfo) bool {
	if r == nil || r.Schema == nil {
		return false
	}
	s := r.Schema
	return s.Ref != "" || len(s.Properties) > 0 || len(s.AllOf) > 0 || s.Items != nil
}

// preferResponseInfo deterministically picks between two responses competing
// for the same status slot — used for the "default" collapse, where several
// unresolved-status bodies (a success type and a framework error map) land
// together. A concrete schema (named-type $ref, object with properties, allOf,
// or array) beats a generic {type: object}; among equally concrete bodies a
// success type beats an error-named DTO; finally a stable BodyType tie-break
// keeps runs in agreement regardless of visitation order.
func preferResponseInfo(cur, next *ResponseInfo) *ResponseInfo {
	if cur == nil {
		return next
	}
	if next == nil {
		return cur
	}
	curConcrete, nextConcrete := responseIsConcrete(cur), responseIsConcrete(next)
	if nextConcrete != curConcrete {
		if nextConcrete {
			return next
		}
		return cur
	}
	curErr, nextErr := isErrorBodyType(cur.BodyType), isErrorBodyType(next.BodyType)
	if curErr != nextErr {
		if nextErr {
			return cur
		}
		return next
	}
	if next.BodyType < cur.BodyType {
		return next
	}
	return cur
}

// responseIsConcrete reports whether a response carries a resolved type rather
// than a generic `object` fallback.
func responseIsConcrete(r *ResponseInfo) bool {
	if r == nil || r.Schema == nil {
		return false
	}
	s := r.Schema
	return s.Ref != "" || len(s.Properties) > 0 || len(s.AllOf) > 0 || s.Items != nil
}

// isErrorBodyType reports whether a body type name looks like an error DTO
// (e.g. ErrorResponse, APIError). Used only as a tie-break for the default slot.
func isErrorBodyType(bodyType string) bool {
	return strings.Contains(strings.ToLower(bodyType), "error")
}

// extractRequestFromNode extracts request information from a node
func (e *Extractor) extractRequestFromNode(node TrackerNodeInterface, route *RouteInfo) *RequestInfo {
	if node == nil || node.GetEdge() == nil {
		return nil
	}
	edge := node.GetEdge()
	idx, ok := e.reqMatcherByEdge[edge]
	if !ok {
		idx = -1
		for i, matcher := range e.requestMatchers {
			if matcher.MatchNode(node) {
				idx = int16(i)
				break
			}
		}
		if e.reqMatcherByEdge == nil {
			e.reqMatcherByEdge = map[*metadata.CallGraphEdge]int16{}
		}
		e.reqMatcherByEdge[edge] = idx
	}
	if idx < 0 {
		return nil
	}
	return e.requestMatchers[idx].ExtractRequest(node, route)
}

// extractParamsFromNode extracts parameter information from a node. Most
// patterns yield at most one parameter (returned as a single-element slice),
// but map-key patterns (gorilla/mux's `Vars(r)["id"]`) can yield several,
// one per indexed key that matches a path placeholder.
func (e *Extractor) extractParamsFromNode(node TrackerNodeInterface, route *RouteInfo) []Parameter {
	if node == nil || node.GetEdge() == nil {
		return nil
	}
	edge := node.GetEdge()
	idx, ok := e.paramMatcherByEdge[edge]
	if !ok {
		idx = -1
		for i, matcher := range e.paramMatchers {
			if matcher.MatchNode(node) {
				idx = int16(i)
				break
			}
		}
		if e.paramMatcherByEdge == nil {
			e.paramMatcherByEdge = map[*metadata.CallGraphEdge]int16{}
		}
		e.paramMatcherByEdge[edge] = idx
	}
	if idx < 0 {
		return nil
	}
	{
		matcher := e.paramMatchers[idx]
		// Map-key accessors (mux.Vars) carry the parameter name as a map key,
		// not a call argument, so nothing is extracted from the node itself.
		// Their path params are added once per route in completeMapKeyPathParams,
		// which handles direct, inline, and helper-wrapped access uniformly via
		// call-graph reachability.
		if impl, ok := matcher.(*ParamPatternMatcherImpl); ok && impl.pattern.NameFromMapKey {
			return nil
		}
		if param := matcher.ExtractParam(node, route); param != nil {
			return []Parameter{*param}
		}
		return nil
	}
}

// completeMapKeyPathParams adds path parameters for frameworks whose path-var
// accessor returns a map indexed by name — the gorilla/mux idiom
// `mux.Vars(r)["id"]`. The name is a map key, not a call argument, so it can't
// be pulled from the accessor call; instead, if the route's handler reaches the
// accessor anywhere in its call graph (directly, inline, or through a helper
// like `id := readParam(r, "id")`), the handler reads request path variables,
// and every `{placeholder}` in the route path is a genuine path parameter.
//
// Names come from the path template (authoritative for path params), which is
// robust to every access form — assignment, blank `_ =`, inline call arg, or a
// dynamic key inside a helper — none of which are uniformly recoverable from the
// metadata. Routes whose handler never reaches the accessor are left untouched,
// so their placeholders still fall through to ensureAllPathParams and keep the
// "present in path but not found in the code" warning, matching the other
// frameworks.
func (e *Extractor) completeMapKeyPathParams(route *RouteInfo) {
	if route == nil || route.Metadata == nil {
		return
	}
	var accessor *ParamPattern
	for i := range e.cfg.Framework.ParamPatterns {
		if e.cfg.Framework.ParamPatterns[i].NameFromMapKey {
			accessor = &e.cfg.Framework.ParamPatterns[i]
			break
		}
	}
	if accessor == nil {
		return
	}

	placeholders := pathPlaceholders(route.Path)
	if len(placeholders) == 0 {
		return
	}
	covered := make(map[string]bool)
	for _, p := range route.Params {
		if p.In == "path" {
			covered[p.Name] = true
		}
	}
	missing := false
	for _, name := range placeholders {
		if !covered[name] {
			missing = true
			break
		}
	}
	if !missing {
		return
	}

	if !e.handlerReachesAccessor(route, *accessor) {
		return
	}

	patterns := pathParamPatterns(route.Path)
	for _, name := range placeholders {
		if covered[name] {
			continue
		}
		schema := &Schema{Type: "string"}
		if pat := patterns[name]; pat != "" {
			schema.Pattern = pat
		}
		route.Params = append(route.Params, Parameter{
			Name:     name,
			In:       accessor.ParamIn,
			Required: accessor.ParamIn == "path",
			Schema:   schema,
		})
	}
}

// PathParamMismatch records a handler reading a map-key path variable whose key
// has no matching `{placeholder}` in the route path — surfaced as a diagnostic.
type PathParamMismatch struct {
	Method  string // HTTP method
	Path    string // OpenAPI path (regex constraints stripped)
	Handler string // handler function (package-qualified)
	Key     string // key read in code, e.g. mux.Vars(r)["userId"]
}

// PathParamMismatches returns the map-key path-variable diagnostics gathered
// during extraction (keys read in code that no route placeholder declares).
func (e *Extractor) PathParamMismatches() []PathParamMismatch {
	return e.pathParamMismatches
}

// recordPathVarKeyMismatches recovers the literal keys the handler reads through
// the map-key accessor (mux.Vars) and records a diagnostic for any key that is
// not a `{placeholder}` in the route path. The recovery uses the assignment
// tracker: a variable assigned from the accessor (`vars := mux.Vars(r)`, tagged
// with CalleeFunc/CalleePkg on the assignment) is "accessor-derived", and any
// `accessorVar["key"]` or inline `mux.Vars(r)["key"]` index yields that key.
func (e *Extractor) recordPathVarKeyMismatches(route *RouteInfo) {
	if route == nil || route.Metadata == nil || route.Function == "" {
		return
	}
	accessor := e.mapKeyAccessor()
	if accessor == nil {
		return
	}
	keys := e.recoverAccessorKeys(route, *accessor)
	if len(keys) == 0 {
		return
	}
	placeholders := make(map[string]bool)
	for _, n := range pathPlaceholders(route.Path) {
		placeholders[n] = true
	}
	// Sorted iteration keeps the diagnostics list deterministic.
	names := make([]string, 0, len(keys))
	for k := range keys {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, key := range names {
		if placeholders[key] {
			continue
		}
		openAPIPath := convertPathToOpenAPI(joinPaths(route.MountPath, route.Path))
		dedup := route.Method + " " + openAPIPath + " " + key
		if e.pathParamMismatchSet == nil {
			e.pathParamMismatchSet = make(map[string]struct{})
		}
		if _, ok := e.pathParamMismatchSet[dedup]; ok {
			continue
		}
		e.pathParamMismatchSet[dedup] = struct{}{}
		e.pathParamMismatches = append(e.pathParamMismatches, PathParamMismatch{
			Method:  route.Method,
			Path:    openAPIPath,
			Handler: route.Function,
			Key:     key,
		})
	}
}

// mapKeyAccessor returns the first configured NameFromMapKey param pattern, or
// nil when the framework has none (only gorilla/mux does by default).
func (e *Extractor) mapKeyAccessor() *ParamPattern {
	for i := range e.cfg.Framework.ParamPatterns {
		if e.cfg.Framework.ParamPatterns[i].NameFromMapKey {
			return &e.cfg.Framework.ParamPatterns[i]
		}
	}
	return nil
}

// recoverAccessorKeys returns the set of literal keys the route's handler reads
// through the map-key accessor (direct `vars["id"]` on an accessor-derived
// variable, or inline `mux.Vars(r)["id"]`). Dynamic keys and keys passed into
// helpers are not recovered — the diagnostic errs toward no false positives.
func (e *Extractor) recoverAccessorKeys(route *RouteInfo, accessor ParamPattern) map[string]struct{} {
	meta := route.Metadata
	bareFunc := route.Function
	if route.Package != "" {
		bareFunc = strings.TrimPrefix(route.Function, route.Package+".")
	}
	fn := findFunctionByName(meta, route.Package, bareFunc)
	if fn == nil {
		return nil
	}

	callRe, err1 := cachedRegex(accessor.CallRegex)
	recvRe, err2 := cachedRegex(accessor.RecvTypeRegex)
	if (accessor.CallRegex != "" && err1 != nil) || (accessor.RecvTypeRegex != "" && err2 != nil) {
		return nil
	}

	// Variables assigned directly from the accessor call (vars := mux.Vars(r)).
	accessorVars := make(map[string]bool)
	for name, asgns := range fn.AssignmentMap {
		for i := range asgns {
			a := &asgns[i]
			if (callRe == nil || callRe.MatchString(a.CalleeFunc)) &&
				(recvRe == nil || recvRe.MatchString(a.CalleePkg)) &&
				a.CalleeFunc != "" {
				accessorVars[name] = true
			}
		}
	}

	keys := make(map[string]struct{})
	for _, asgns := range fn.AssignmentMap {
		for i := range asgns {
			collectAccessorKeys(&asgns[i].Value, accessorVars, callRe, recvRe, keys)
		}
	}
	return keys
}

// collectAccessorKeys walks an expression tree, recording the literal key of any
// `X["key"]` index where X is an accessor-derived variable or an inline accessor
// call. Recurses so nested expressions (`"John " + vars["id"]`) are covered.
func collectAccessorKeys(arg *metadata.CallArgument, accessorVars map[string]bool, callRe, recvRe *regexp.Regexp, out map[string]struct{}) {
	if arg == nil {
		return
	}
	if arg.GetKind() == metadata.KindIndex && arg.Fun != nil && arg.Fun.GetKind() == metadata.KindLiteral && arg.X != nil {
		derived := false
		switch {
		case arg.X.GetKind() == metadata.KindIdent && accessorVars[arg.X.GetName()]:
			derived = true
		case isAccessorCall(arg.X, callRe, recvRe):
			derived = true
		}
		if derived {
			if key := strings.Trim(arg.Fun.GetValue(), "\"`"); key != "" {
				out[key] = struct{}{}
			}
		}
	}
	collectAccessorKeys(arg.X, accessorVars, callRe, recvRe, out)
	collectAccessorKeys(arg.Fun, accessorVars, callRe, recvRe, out)
	collectAccessorKeys(arg.Sel, accessorVars, callRe, recvRe, out)
	for i := range arg.Args {
		collectAccessorKeys(arg.Args[i], accessorVars, callRe, recvRe, out)
	}
}

// isAccessorCall reports whether a call-argument is a call to the accessor
// (e.g. `mux.Vars(r)`), matching the call name and receiver/package regexes.
func isAccessorCall(x *metadata.CallArgument, callRe, recvRe *regexp.Regexp) bool {
	if x == nil || x.GetKind() != metadata.KindCall || x.Fun == nil {
		return false
	}
	fun := x.Fun
	name := fun.GetName()
	pkg := fun.GetPkg()
	if fun.GetKind() == metadata.KindSelector && fun.Sel != nil {
		name = fun.Sel.GetName()
		if pkg == "" {
			pkg = fun.Sel.GetPkg()
		}
	}
	if callRe != nil && !callRe.MatchString(name) {
		return false
	}
	if recvRe != nil && !recvRe.MatchString(pkg) {
		return false
	}
	return true
}

// findFunctionByName locates a function by package and (bare) name, preferring
// the named package and falling back to any package declaring the name.
func findFunctionByName(meta *metadata.Metadata, pkg, name string) *metadata.Function {
	if meta == nil || name == "" {
		return nil
	}
	if p, ok := meta.Packages[pkg]; ok {
		for _, file := range p.Files {
			if fn, ok := file.Functions[name]; ok {
				return fn
			}
		}
	}
	// Fallback: any package declaring the name. Sort package keys so that when
	// several packages declare the same bare name the result is stable across
	// runs (map iteration order is random). Function names are unique within a
	// package, so the inner file order doesn't affect the result.
	pkgNames := make([]string, 0, len(meta.Packages))
	for p := range meta.Packages {
		pkgNames = append(pkgNames, p)
	}
	sort.Strings(pkgNames)
	for _, p := range pkgNames {
		for _, file := range meta.Packages[p].Files {
			if fn, ok := file.Functions[name]; ok {
				return fn
			}
		}
	}
	return nil
}

// callerAssignmentMap returns the variable→assignments map of the function or
// method that lexically contains a call edge's call site, preferring the map
// that actually records varName. A plain call resolves via its caller function.
// A call inside a returned closure (the handler-factory shape — a method
// returning func(w, r){…}) flattens into the tree with a FuncLit caller, but
// ast.Inspect records the closure's `err := NewX(…, status)` assignment on the
// *enclosing* declared function or method (via the edge's ParentFunction).
// Enclosing methods live in Type.Methods keyed by receiver, not in
// file.Functions, so findFunctionByName alone would miss them.
func callerAssignmentMap(impl *ContextProviderImpl, edge *metadata.CallGraphEdge, varName string) map[string][]metadata.Assignment {
	if fn := findFunctionByName(impl.meta, impl.GetString(edge.Caller.Pkg), impl.GetString(edge.Caller.Name)); fn != nil {
		if len(fn.AssignmentMap[varName]) > 0 {
			return fn.AssignmentMap
		}
	}
	pf := edge.ParentFunction
	if pf == nil {
		return nil
	}
	if fn := findFunctionByName(impl.meta, impl.GetString(pf.Pkg), impl.GetString(pf.Name)); fn != nil {
		if len(fn.AssignmentMap[varName]) > 0 {
			return fn.AssignmentMap
		}
	}
	return methodAssignmentMap(impl.meta, impl.GetString(pf.Pkg), impl.GetString(pf.RecvType), impl.GetString(pf.Name), varName)
}

// assignmentsAt returns the assignments to the variable `name` visible at the
// given call site: the call edge's own AssignmentMap first, then the enclosing
// function's scope (via callerAssignmentMap) for a variable assigned in the
// handler body rather than at the edge. The edge map and the function scope
// record the same assignment for a given variable, so consulting the edge first
// is a fast path, not a different answer.
//
// This is the one canonical call-site assignment lookup (issue #182): the
// request-body / response-destination resolvers reach it via latestAssignment
// (latest RHS), and the constructor-field status resolvers consume the full
// Assignment (CalleeFunc / position) directly. Returns nil when there is none.
func assignmentsAt(cp ContextProvider, edge *metadata.CallGraphEdge, name string) []metadata.Assignment {
	if name == "" || edge == nil {
		return nil
	}
	if assigns := edge.AssignmentMap[name]; len(assigns) > 0 {
		return assigns
	}
	if impl, ok := cp.(*ContextProviderImpl); ok {
		if am := callerAssignmentMap(impl, edge, name); am != nil {
			return am[name]
		}
	}
	return nil
}

// latestAssignment returns the right-hand side of the most recent assignment to
// the variable `name` visible at the given call site (see assignmentsAt for the
// scope). Returns nil when there is no such assignment. Used by the request-body
// source resolver (src := r.Body) and the response-destination resolver
// (dst := w, lw := &loggingWriter{w}).
func latestAssignment(cp ContextProvider, edge *metadata.CallGraphEdge, name string) *metadata.CallArgument {
	assigns := assignmentsAt(cp, edge, name)
	if len(assigns) == 0 {
		return nil
	}
	rhs := assigns[len(assigns)-1].Value
	return &rhs
}

// methodAssignmentMap finds the AssignmentMap of the method (pkg, receiver,
// name) whose map records varName. Methods are stored per-Type; receiver and
// name are matched exactly (the same receiver string findParentFunction records
// on ParentFunction), so an enclosing method's closure assignments resolve.
func methodAssignmentMap(meta *metadata.Metadata, pkg, recv, name, varName string) map[string][]metadata.Assignment {
	if meta == nil || name == "" {
		return nil
	}
	p, ok := meta.Packages[pkg]
	if !ok {
		return nil
	}
	for _, file := range p.Files {
		for _, t := range file.Types {
			for i := range t.Methods {
				m := &t.Methods[i]
				if meta.StringPool.GetString(m.Name) != name {
					continue
				}
				if recv != "" && meta.StringPool.GetString(m.Receiver) != recv {
					continue
				}
				if len(m.AssignmentMap[varName]) > 0 {
					return m.AssignmentMap
				}
			}
		}
	}
	return nil
}

// findMethodByName resolves the method (pkg, receiver, name) in the per-Type
// methods table. Methods never appear in file.Functions (processFunctions skips
// any decl with a receiver), so findFunctionByName cannot see them — anything
// resolving a handler that may be a method needs this fallback. An empty recv
// matches on the method name alone; receivers are compared with a leading `*`
// trimmed so a pointer-receiver handler matches its value-typed record.
func findMethodByName(meta *metadata.Metadata, pkg, recv, name string) *metadata.Method {
	if meta == nil || name == "" {
		return nil
	}
	p, ok := meta.Packages[pkg]
	if !ok {
		return nil
	}
	// Sort file keys: a receiver-less lookup can match in several files, and map
	// iteration order would make the winner (and any doc comment it carries)
	// vary between runs.
	fileNames := make([]string, 0, len(p.Files))
	for f := range p.Files {
		fileNames = append(fileNames, f)
	}
	sort.Strings(fileNames)
	for _, fname := range fileNames {
		for _, t := range p.Files[fname].Types {
			for i := range t.Methods {
				m := &t.Methods[i]
				if meta.StringPool.GetString(m.Name) != name {
					continue
				}
				if recv != "" && strings.TrimPrefix(meta.StringPool.GetString(m.Receiver), "*") != strings.TrimPrefix(recv, "*") {
					continue
				}
				return m
			}
		}
	}
	return nil
}

// handlerReachesAccessor reports whether the route's handler transitively calls
// the map-key accessor described by pattern. Reachability is a precomputed
// summary (reachSet, one bottom-up pass over the SCC condensation) rather
// than a per-route bounded walk, so helper indirection resolves at any depth.
// Seeds are the handler function's call-graph base IDs (matched by name +
// package), so it works regardless of the exact key format.
func (e *Extractor) handlerReachesAccessor(route *RouteInfo, pattern ParamPattern) bool {
	meta := route.Metadata
	if meta == nil || route.Function == "" {
		return false
	}
	reach := e.reachSet(meta, "accessor:"+pattern.CallRegex+"\x00"+pattern.RecvTypeRegex,
		func(edge *metadata.CallGraphEdge) bool {
			return edgeMatchesAccessor(meta, edge, pattern)
		})
	// route.Function is package-qualified ("pkg/path.Handler"); call-graph
	// caller names are bare ("Handler"), so strip the package prefix to match.
	bareFunc := route.Function
	if route.Package != "" {
		bareFunc = strings.TrimPrefix(route.Function, route.Package+".")
	}
	for i := range meta.CallGraph {
		edge := &meta.CallGraph[i]
		if getString(meta, edge.Caller.Name) != bareFunc {
			continue
		}
		if route.Package != "" && getString(meta, edge.Caller.Pkg) != route.Package {
			continue
		}
		if reach[edge.Caller.BaseID()] {
			return true
		}
	}
	return false
}

// edgeMatchesAccessor reports whether a call edge's callee matches a param
// pattern's accessor identity (call-name regex + fully-qualified receiver/pkg
// regex), mirroring ParamPatternMatcherImpl.MatchNode.
func edgeMatchesAccessor(meta *metadata.Metadata, edge *metadata.CallGraphEdge, pattern ParamPattern) bool {
	callName := getString(meta, edge.Callee.Name)
	recvType := getString(meta, edge.Callee.RecvType)
	recvPkg := getString(meta, edge.Callee.Pkg)
	fq := recvPkg
	if fq != "" && recvType != "" {
		fq += "." + recvType
	} else if recvType != "" {
		fq = recvType
	}
	if pattern.CallRegex != "" {
		if re, err := cachedRegex(pattern.CallRegex); err != nil || !re.MatchString(callName) {
			return false
		}
	}
	if pattern.RecvTypeRegex != "" {
		if re, err := cachedRegex(pattern.RecvTypeRegex); err != nil || !re.MatchString(fq) {
			return false
		}
	}
	return true
}

// pathPlaceholders returns the placeholder names in a route path, in order of
// appearance. It handles both plain `{name}` and constrained `{name:pattern}`
// (mux/chi) placeholders.
func pathPlaceholders(path string) []string {
	var names []string
	forEachPathParam(path, func(name, _ string) {
		if name != "" {
			names = append(names, name)
		}
	})
	return names
}

// joinPaths joins two URL paths cleanly
func joinPaths(a, b string) string {
	a = strings.TrimRight(a, "/")
	b = strings.TrimLeft(b, "/")
	if a == "" {
		return "/" + b
	}
	return a + "/" + b
}

// determineLiteralType determines the appropriate Go type for a literal value
func determineLiteralType(literalValue string) string {
	// Remove quotes if present
	cleanValue := strings.Trim(literalValue, "\"`")

	// Check for numeric literals
	if _, err := strconv.ParseInt(cleanValue, 10, 64); err == nil {
		return "int"
	}
	if _, err := strconv.ParseUint(cleanValue, 10, 64); err == nil {
		return "uint"
	}
	if _, err := strconv.ParseFloat(cleanValue, 64); err == nil {
		return "float64"
	}

	// Check for boolean literals
	if cleanValue == "true" || cleanValue == "false" {
		return "bool"
	}

	// Check for nil
	if cleanValue == "nil" {
		return "interface{}"
	}

	// Default to string for everything else
	return "string"
}

func preprocessingBodyType(bodyType string) string {
	if after, ok := strings.CutPrefix(bodyType, "[]"); ok && after != "" {
		bodyType = after
	}
	if after, ok := strings.CutPrefix(bodyType, "*"); ok && after != "" {
		bodyType = after
	}
	if after, ok := strings.CutPrefix(bodyType, "&"); ok && after != "" {
		bodyType = after
	}
	return bodyType
}

// ResponsePatternMatcherImpl implements ResponsePatternMatcher
type ResponsePatternMatcherImpl struct {
	*BasePatternMatcher
	pattern      ResponsePattern
	destResolver *responseDestResolver
}

// NewResponsePatternMatcher creates a new response pattern matcher
func NewResponsePatternMatcher(pattern ResponsePattern, cfg *APISpecConfig, contextProvider ContextProvider, typeResolver TypeResolver) *ResponsePatternMatcherImpl {
	return &ResponsePatternMatcherImpl{
		BasePatternMatcher: NewBasePatternMatcher(cfg, contextProvider, typeResolver),
		pattern:            pattern,
		destResolver:       newResponseDestResolver(cfg, contextProvider),
	}
}

// destination returns the encoder's write destination for the given node,
// resolved per-route through the call graph to its concrete value, plus the
// tracker edge in whose scope that value's provenance is read. For
// json.NewEncoder(x).Encode(v) the raw destination is the factory's first
// argument x; when x is a wrapper parameter (`func encodeTo(dst io.Writer, v)`)
// it is followed to the caller's actual argument at this route's call site, so
// the same helper resolves to the writer for `encodeTo(w, v)` and to a buffer
// for `encodeTo(&buf, v)`. Returns (nil, nil) when the pattern carries no
// receiver-based destination.
func (r *ResponsePatternMatcherImpl) destination(node TrackerNodeInterface) (*metadata.CallArgument, *metadata.CallGraphEdge) {
	if node == nil {
		return nil, nil
	}
	edge := node.GetEdge()
	if edge == nil || !r.pattern.DestFromReceiver {
		return nil, nil
	}
	dst := resolveReceiverSource(edge, r.destResolver.metadata())
	if dst == nil {
		return nil, edge
	}
	resolved, resolvedNode := resolveArgThroughParams(dst, node)
	dstEdge := edge
	if resolvedNode != nil && resolvedNode.GetEdge() != nil {
		dstEdge = resolvedNode.GetEdge()
	}
	return resolved, dstEdge
}

// MatchNode checks if a node matches the response pattern
func (r *ResponsePatternMatcherImpl) MatchNode(node TrackerNodeInterface) bool {
	if node == nil || node.GetEdge() == nil {
		return false
	}

	edge := node.GetEdge()
	callName := r.contextProvider.GetString(edge.Callee.Name)
	recvType := r.contextProvider.GetString(edge.Callee.RecvType)
	recvPkg := r.contextProvider.GetString(edge.Callee.Pkg)

	// Build fully qualified receiver type
	fqRecvType := recvPkg
	if fqRecvType != "" && recvType != "" {
		fqRecvType += "." + recvType
	} else if recvType != "" {
		fqRecvType = recvType
	}

	// Check call regex
	if r.pattern.CallRegex != "" && !r.matchPattern(r.pattern.CallRegex, callName) {
		return false
	}

	// Check function name regex
	if r.pattern.FunctionNameRegex != "" {
		funcName := r.contextProvider.GetString(edge.Caller.Name)
		if !r.matchPattern(r.pattern.FunctionNameRegex, funcName) {
			return false
		}
	}

	// Check receiver type
	if r.pattern.RecvTypeRegex != "" {
		re, err := cachedRegex(r.pattern.RecvTypeRegex)
		if err != nil || !re.MatchString(fqRecvType) {
			return false
		}
	} else if r.pattern.RecvType != "" && r.pattern.RecvType != fqRecvType {
		return false
	}

	return true
}

// GetPattern returns the response pattern
func (r *ResponsePatternMatcherImpl) GetPattern() interface{} {
	return r.pattern
}

// GetPriority returns the priority of this pattern
func (r *ResponsePatternMatcherImpl) GetPriority() int {
	priority := 0
	if r.pattern.CallRegex != "" {
		priority += 10
	}
	if r.pattern.FunctionNameRegex != "" {
		priority += 5
	}
	if r.pattern.RecvTypeRegex != "" || r.pattern.RecvType != "" {
		priority += 3
	}
	return priority
}

// ExtractResponse extracts response information from a matched node.
//
// Returns a slice to support conditional status codes (issue #39): when the
// status arg is a local variable reassigned across branches with different
// status codes, we emit one ResponseInfo per distinct status (all sharing
// the same body/schema). For the typical "one status per call" case, the
// slice has exactly one element — byte-identical to the previous
// single-response behaviour.
func (r *ResponsePatternMatcherImpl) ExtractResponse(node TrackerNodeInterface, route *RouteInfo) []*ResponseInfo {
	var (
		statusResolved bool
	)

	// Write-destination gating (issue #170), done here — NOT in MatchNode, which
	// is memoized per edge — because a generic encoder's destination is
	// per-route: the same helper node writes to the response in one route and a
	// buffer in another. Resolve the destination through the call graph to its
	// concrete value at THIS route's call site and drop the encode only when
	// that value provably does not trace to the response writer.
	if r.pattern.RequireResponseDestination && r.destResolver != nil && r.destResolver.Enabled() {
		if dst, dstEdge := r.destination(node); dst != nil && r.destResolver.ShouldDrop(dst, dstEdge) {
			return nil
		}
	}

	// Get least status code from response map
	leastStatusCode := 0
	for _, resp := range route.Response {
		if resp.StatusCode < leastStatusCode {
			leastStatusCode = resp.StatusCode
		}
	}

	contentType := r.cfg.Defaults.ResponseContentType
	if r.pattern.DefaultContentType != "" {
		contentType = r.pattern.DefaultContentType
	}

	respInfo := &ResponseInfo{
		StatusCode:  leastStatusCode - 1,
		ContentType: contentType,
	}

	edge := node.GetEdge()
	if r.pattern.StatusFromArg && len(edge.Args) > r.pattern.StatusArgIndex {
		statusArg := edge.Args[r.pattern.StatusArgIndex]
		statusStr := r.contextProvider.GetArgumentInfo(statusArg)
		if status, ok := r.schemaMapper.MapStatusCode(statusStr); ok {
			statusResolved = true
			respInfo.StatusCode = status
		} else if callerArg, _ := resolveArgThroughParams(statusArg, node); callerArg != statusArg {
			// The status arg is a parameter threaded through one or more
			// response helpers — e.g. WriteHeader(status) inside
			// respondJSON(w, status, v), itself called from
			// respondError(w, status, code, msg). Walk up EACH wrapper hop to
			// the handler's actual literal; a single hop (the common
			// writeJSON(w, status, v) shape) is just the one-iteration case.
			// resolveArgThroughParams stops at the first non-parameter, so this
			// is a strict superset of the single-hop lookup and never changes an
			// already-resolved status. Per-route tracker isolation pairs each
			// WriteHeader path with its handler's specific status.
			callerStr := r.contextProvider.GetArgumentInfo(callerArg)
			if status, ok := r.schemaMapper.MapStatusCode(callerStr); ok {
				statusResolved = true
				respInfo.StatusCode = status
			}
		} else if status, ok := r.statusFromConstructorField(statusArg, node); ok {
			// The status arg is a struct field (err.Code) whose value was
			// stored by a constructor call — the error-helper pattern
			// RespondWithError(w, NewAPIError(msg, 401)) → w.WriteHeader(err.Code).
			statusResolved = true
			respInfo.StatusCode = status
		}
	}

	if !statusResolved && r.pattern.DefaultStatus > 0 {
		respInfo.StatusCode = r.pattern.DefaultStatus
		statusResolved = true
	}

	if r.pattern.TypeFromArg && len(edge.Args) > r.pattern.TypeArgIndex {
		// If status code is not from argument, attach this body to an existing
		// response that has no body yet. route.Response is a map, so iterating it
		// and taking the "first" match is nondeterministic (Go randomizes map
		// order) — that flips the chosen status between runs. Pick the
		// lowest-numbered matching status instead, which is order-independent.
		if !r.pattern.StatusFromArg {
			best := -1
			for _, resp := range route.Response {
				if resp.BodyType == "" && resp.StatusCode >= 100 && resp.StatusCode < 600 {
					if best == -1 || resp.StatusCode < best {
						best = resp.StatusCode
					}
				}
			}
			if best != -1 {
				respInfo.StatusCode = best
			}
		}

		arg := edge.Args[r.pattern.TypeArgIndex]

		// Write-sink transform unwrap (issue #195): when the written value is the
		// result of a serialization transform (b := json.Marshal(v); w.Write(b)),
		// resolve the body from the transform's payload (v) rather than the []byte
		// the sink literally receives. No-op when the arg isn't a transform result
		// (a raw w.Write([]byte("ok"))), so raw writes are kept as-is.
		if payload := r.unwrapWriteSink(arg, edge); payload != nil {
			arg = payload
		}

		// Parameter tracing: if the body arg is a parameter of the
		// enclosing function (e.g. Encode(v) inside writeJSON(w, status,
		// v)), follow it to the caller's actual argument so we get the
		// concrete type — otherwise `v any` would resolve to a generic
		// object. Per-route isolation in the tracker tree means each
		// handler's response gets the type from its own call site.
		//
		// typeNode follows arg to the scope where it was resolved, so the
		// scope-dependent lookups below (concreteFromCalleeReturn,
		// resolveTypeOrigin, collectWrapperOverrides) read the right function
		// after a multi-hop trace, not the deepest helper's scope.
		typeNode := node
		if callerArg, callerNode := r.traceArgViaParent(arg, node); callerArg != nil {
			arg = callerArg
			if callerNode != nil {
				typeNode = callerNode
			}
		}

		// Type conversion like `[]byte(swaggerUIHTML)`: the *target* type of
		// the conversion (e.g. []byte) is what the function actually
		// receives, not the type of the inner value. Use the conversion's
		// Fun directly rather than peeling to the inner ident — otherwise a
		// const ident's literal value can leak into the schema as a $ref.
		var bodyType string
		if arg.GetKind() == metadata.KindTypeConversion && arg.Fun != nil {
			bodyType = r.contextProvider.GetArgumentInfo(arg.Fun)
		} else {
			bodyType = r.contextProvider.GetArgumentInfo(arg)
		}

		// Check if this is a literal value - if so, determine appropriate type
		if arg.GetKind() == metadata.KindLiteral {
			// For literal values, determine the appropriate type based on the value
			bodyType = determineLiteralType(bodyType)
		} else {
			// For ident arguments referring to a `const` declaration, the
			// context-provider rendering above returns the constant's
			// *value* (its literal contents — e.g. an embedded HTML
			// string), which then leaks into the schema as a $ref. Replace
			// it with the const's declared Go type when we can find it.
			if arg.GetKind() == metadata.KindIdent {
				if t := constIdentDeclaredType(arg, r.contextProvider); t != "" {
					bodyType = t
				}
			}

			// Call-expression body args (e.g. err.Error() in
			// http.Error(w, err.Error(), 400), or any helper(x) used
			// directly as a response payload) carry their *return* type on
			// the CallArgument — see metadata.handleCallExpr. Prefer it
			// over the stringified call, which would otherwise produce
			// an unresolvable name like "error.Error" or "pkg.Helper".
			resolvedConcrete := false
			if arg.GetKind() == metadata.KindCall {
				if t := arg.GetType(); t != "" {
					bodyType = t
					// When the call's return type is an interface, trace the
					// callee's return value to the concrete type it actually
					// returns (`Encode(makeAnimal())` where
					// makeAnimal() Animal { return Dog{} } → Dog). Mark it
					// resolved so resolveTypeOrigin's GetResolvedType fast-path
					// (which would restore the interface) is skipped.
					if concrete := r.concreteFromCalleeReturn(arg, typeNode.GetEdge(), t); concrete != "" {
						bodyType = concrete
						resolvedConcrete = true
					}
				}
			}

			// Trace type origin for non-literal arguments
			if !resolvedConcrete {
				bodyType = r.resolveTypeOrigin(arg, typeNode, bodyType)
			}

			// Apply dereferencing if needed
			if r.pattern.Deref && strings.HasPrefix(bodyType, "*") {
				bodyType = strings.TrimPrefix(bodyType, "*")
			}
		}

		// Inferred generic instantiations arrive as the go/types string
		// (pkg.Envelope[pkg.Product]); fold them into the internal form so they
		// key to the same clean component as a written Envelope[Product].
		bodyType = normalizeGenericInstanceName(bodyType)

		respInfo.BodyType = preprocessingBodyType(bodyType)

		schema, _ := mapGoTypeToOpenAPISchema(route.UsedTypes, bodyType, route.Metadata, r.cfg, nil)

		// Wrapper specialisation: when the body resolves to a struct
		// whose fields are bound to constructor parameters at the
		// helper boundary (e.g. `response := NewEnvelope(msg, data,
		// code)` inside RespondWithSuccess), recover the caller-site
		// concrete type for each bound field and compose an `allOf`
		// override so per-route schemas reflect the actual payload
		// type instead of the wrapper's declared `interface{}`.
		if overrides := r.collectWrapperOverrides(arg, typeNode); len(overrides) > 0 {
			schema = specialiseWrapperSchema(schema, overrides, bodyType, route.UsedTypes, route.Metadata, r.cfg)
		}

		respInfo.Schema = schema
	}

	// Conditional status codes (issue #39): if the status arg is a local
	// variable with multiple branched assignments mapping to *distinct*
	// status codes, emit one response per status, sharing the body/schema.
	// This runs before the "no status, no body — return nil" guard so that
	// patterns whose status arg is an opaque ident (e.g. RespondWithError(w,
	// err)) still produce responses when the branches encode the codes.
	if r.pattern.StatusFromArg && len(edge.Args) > r.pattern.StatusArgIndex {
		statusArg := edge.Args[r.pattern.StatusArgIndex]
		expanded, residue := r.expandStatusesFromIdent(statusArg, edge)
		if len(expanded) == 0 && !residue {
			// The status arg wasn't a directly branch-assigned local. It may be
			// a constructor field (err.Code) whose value was set across branches
			// then handed to the error constructor — issue #155.
			expanded, residue = r.statusesFromConstructorField(statusArg, node)
		}
		if len(expanded) == 0 && !residue {
			// Or a mapper field (api.Status) whose value is set across the return
			// branches of an error mapper (api := MapError(err)) — issue #187.
			expanded, residue = r.statusesFromMapperField(statusArg, node)
		}
		if len(expanded) > 1 || (len(expanded) >= 1 && residue) {
			out := make([]*ResponseInfo, 0, len(expanded)+1)
			for _, st := range expanded {
				out = append(out, &ResponseInfo{
					StatusCode:  st,
					ContentType: respInfo.ContentType,
					BodyType:    respInfo.BodyType,
					Schema:      respInfo.Schema,
				})
			}
			// A non-constant branch keeps an honest `default`: a fresh
			// unresolved status (below every real code), never a copy of an
			// already-resolved concrete status.
			if residue {
				out = append(out, &ResponseInfo{
					StatusCode:  unresolvedStatus,
					ContentType: respInfo.ContentType,
					BodyType:    respInfo.BodyType,
					Schema:      respInfo.Schema,
				})
			}
			return out
		}
	}

	if !statusResolved && respInfo.BodyType == "" {
		return nil
	}

	return []*ResponseInfo{respInfo}
}

// traceArgViaParent walks up the tracker tree to recover the caller-site value
// of a parameter ident. When a response pattern matches inside a helper
// (writeJSON-style) — e.g. Encode(v) where v is a parameter of writeJSON — the
// matched call's args reference parameters, not literals. Each parent tracker
// node represents a call, and that edge's ParamArgMap maps callee parameter
// names back to the caller's actual arguments.
//
// It follows the parameter across MULTIPLE hops (writeJSON -> render -> Encode),
// stopping at the first non-parameter — the concrete value — via
// resolveArgThroughParams. A single hop resolved only the outermost helper's
// parameter, leaving `v any` at a generic object for two-hop helper chains
// (issue #180). Returns nil when nothing resolves (the arg is unchanged).
//
// Per-route isolation in the tracker tree is what makes this sound: each
// handler's path through the helpers is a distinct tracker subtree, so two
// routes that call the same helper with different values each resolve to their
// own value independently.
func (r *ResponsePatternMatcherImpl) traceArgViaParent(arg *metadata.CallArgument, node TrackerNodeInterface) (*metadata.CallArgument, TrackerNodeInterface) {
	resolved, resolvedNode := resolveArgThroughParams(arg, node)
	if resolved == arg {
		return nil, nil
	}
	return resolved, resolvedNode
}

// argViaParent recovers the caller-site value of a parameter ident by finding
// the call into the function the matched call lives in and reading that edge's
// ParamArgMap (callee parameter name → caller argument). Returns nil when the
// arg isn't an ident or no such binding exists. Shared by the response and
// request matchers.
// argViaParent recovers the caller-site value of a parameter ident and the
// tracker node where it was resolved — the parent call node whose ParamArgMap
// carried the argument. The returned node is the scope in which the resolved
// argument lives, so downstream type resolution reads the right function
// (issue #180 / CodeRabbit review on PR #183). Returns (nil, nil) when the arg
// isn't a parameter reachable from the parent chain.
func argViaParent(arg *metadata.CallArgument, node TrackerNodeInterface) (*metadata.CallArgument, TrackerNodeInterface) {
	if arg == nil || arg.GetKind() != metadata.KindIdent || node == nil {
		return nil, nil
	}
	// Fast path: the immediate parent is normally the call into the enclosing
	// function.
	if parent := node.GetParent(); parent != nil {
		if pe := parent.GetEdge(); pe != nil && pe.ParamArgMap != nil {
			if callerArg, ok := pe.ParamArgMap[arg.GetName()]; ok {
				return &callerArg, parent
			}
		}
	}
	// Fallback: the node may be re-homed under a receiver-variable producer
	// rather than under its call site — e.g. `dec := json.NewDecoder(r.Body);
	// dec.Decode(dst)` parents Decode under NewDecoder, so the immediate parent
	// is not the call into the enclosing wrapper and the wrapper's param (dst)
	// never resolves. Walk ancestors for the edge whose callee IS the enclosing
	// function and read its ParamArgMap. Mirrors concreteFromParamBinding.
	edge := node.GetEdge()
	if edge == nil {
		return nil, nil
	}
	enclosing := edge.Caller.BaseID()
	if enclosing == "" {
		return nil, nil
	}
	for p := node.GetParent(); p != nil; p = p.GetParent() {
		pe := p.GetEdge()
		if pe == nil || pe.Callee.BaseID() != enclosing {
			continue
		}
		if pe.ParamArgMap == nil {
			return nil, nil
		}
		if callerArg, ok := pe.ParamArgMap[arg.GetName()]; ok {
			return &callerArg, p
		}
		return nil, nil
	}
	return nil, nil
}

// resolveArgThroughParams follows a parameter ident up through one or more
// wrapper calls to the caller's concrete argument. It is the request/response
// dual of inlining a binding helper: e.g. for `c.Bind(v)` inside a custom
// `ReadRequest(c, v)` wrapper, it returns the `&User{}` actually passed at the
// route's call site. Each hop maps a callee parameter to its caller argument
// via the parent edge's ParamArgMap; it stops at the first non-parameter (a
// local, literal, composite, …) or after a small hop cap. Returns the original
// arg unchanged when nothing resolves.
func resolveArgThroughParams(arg *metadata.CallArgument, node TrackerNodeInterface) (*metadata.CallArgument, TrackerNodeInterface) {
	cur := node
	const maxHops = 8
	for i := 0; i < maxHops; i++ {
		next, nextNode := argViaParent(arg, cur)
		if next == nil {
			break
		}
		arg = next
		// Advance to the node where the argument actually resolved, not blindly
		// cur.GetParent(): on the re-homed fallback path argViaParent resolves at
		// an ancestor several levels up, and cur.GetParent() would desync the
		// scope from the resolved argument (CodeRabbit review on PR #183). On the
		// fast path nextNode == cur.GetParent(), so this is unchanged there.
		if nextNode == nil {
			break
		}
		cur = nextNode
	}
	return arg, cur
}

// statusFromConstructorField resolves a status argument shaped like `x.Field`
// (a selector on a variable) whose value was stored into that field by a
// constructor call — the common error-helper pattern:
//
//	e := NewAPIError("...", http.StatusUnauthorized) // struct field Code: code
//	RespondWithError(w, e)                            // w.WriteHeader(err.Code)
//
// It follows the provenance precisely, hop by hop: the selector's base
// variable up through any wrapper parameters to the local it aliases; that
// local's assignment from the constructor call; the constructor's return
// composite-literal field whose key matches the selector (`Code` ← the
// parameter `code`); and finally that parameter's actual argument at the
// constructor call site. Returns (status, true) only when every hop resolves
// to a single known HTTP status — any missing or ambiguous hop returns false
// (honest over wrong).
func (r *ResponsePatternMatcherImpl) statusFromConstructorField(arg *metadata.CallArgument, node TrackerNodeInterface) (int, bool) {
	if arg == nil || arg.GetKind() != metadata.KindSelector || arg.X == nil || arg.Sel == nil {
		return 0, false
	}
	fieldName := arg.Sel.GetName()
	if fieldName == "" || arg.X.GetKind() != metadata.KindIdent {
		return 0, false
	}
	impl, ok := r.contextProvider.(*ContextProviderImpl)
	if !ok || impl.meta == nil {
		return 0, false
	}

	// 1. The selector's base variable, up through wrapper parameters to the
	//    local it aliases (err -> e), and the tracker node where it lives.
	baseVar, callerNode := resolveArgThroughParams(arg.X, node)
	if baseVar == nil || baseVar.GetKind() != metadata.KindIdent || callerNode == nil {
		return 0, false
	}
	callerEdge := callerNode.GetEdge()
	if callerEdge == nil {
		return 0, false
	}

	// 2. That local's latest assignment, which must come from a constructor call.
	//    Edge-first (assignmentsAt): the error variable may be assigned inside a
	//    returned handler closure, where its assignment is recorded on the call
	//    edge rather than a name-resolvable declared function's scope (#189).
	assigns := assignmentsAt(impl, callerEdge, baseVar.GetName())
	if len(assigns) == 0 {
		return 0, false
	}
	assign := assigns[len(assigns)-1]
	if assign.Value.GetKind() != metadata.KindCall || assign.CalleeFunc == "" {
		return 0, false
	}

	// 3. The constructor's return field matching the selector, and the actual
	//    argument bound to the parameter it assigns from, at the call site.
	statusArg, ok := constructorFieldArg(impl, callerEdge.Caller.ID(), assign.CalleeFunc,
		assign.CalleePkg, impl.GetString(assign.Value.Position), fieldName)
	if !ok {
		return 0, false
	}
	return r.schemaMapper.MapStatusCode(impl.GetArgumentInfo(statusArg))
}

// constructorFieldParam returns the parameter name a constructor's return
// composite-literal assigns into fieldName (e.g. for
// `return &APIError{Message: message, Code: code}` and fieldName "Code" it
// returns "code"). Empty when no return value is a composite literal keying
// that field to a bare parameter ident.
func constructorFieldParam(ctor *metadata.Function, fieldName string) string {
	for i := range ctor.ReturnVars {
		lit := compositeLitOf(&ctor.ReturnVars[i])
		if lit == nil {
			continue
		}
		for _, elt := range lit.Args {
			if elt == nil || elt.GetKind() != metadata.KindKeyValue {
				continue
			}
			key, val := elt.X, elt.Fun
			if key == nil || val == nil {
				continue
			}
			if key.GetKind() == metadata.KindIdent && key.GetName() == fieldName &&
				val.GetKind() == metadata.KindIdent {
				return val.GetName()
			}
		}
	}
	return ""
}

// statusesFromConstructorField is the one→many counterpart of
// statusFromConstructorField (issue #155): when the WriteHeader status argument
// (`err.Code`) resolves through the error constructor to a local variable whose
// value is set across switch/if branches, it fans that variable out to the set
// of concrete status codes the branches assign. residue is true when a branch
// is non-constant, so the caller keeps an honest `default` alongside the codes.
// Returns nil codes when the status is a single value (handled by the existing
// single-status path) or cannot be resolved to a branch variable.
func (r *ResponsePatternMatcherImpl) statusesFromConstructorField(arg *metadata.CallArgument, node TrackerNodeInterface) (codes []int, residue bool) {
	if arg == nil || arg.GetKind() != metadata.KindSelector || arg.X == nil || arg.Sel == nil {
		return nil, false
	}
	fieldName := arg.Sel.GetName()
	if fieldName == "" {
		return nil, false
	}
	impl, ok := r.contextProvider.(*ContextProviderImpl)
	if !ok || impl.meta == nil {
		return nil, false
	}

	// The selector base up to its concrete producer (through wrapper params):
	// either the constructor call itself (inline `Respond(w, NewErr(...))`) or a
	// local variable assigned from it (`e := NewErr(...); Respond(w, e)`). The
	// tracker node's caller is the scope where the branch variable lives.
	base, baseNode := resolveArgThroughParams(arg.X, node)
	if base == nil || baseNode == nil || baseNode.GetEdge() == nil {
		return nil, false
	}
	scope := findFunction(impl.meta, impl.GetString(baseNode.GetEdge().Caller.Pkg), impl.GetString(baseNode.GetEdge().Caller.Name))
	if scope == nil {
		return nil, false
	}

	var calleeFunc, valPos string
	switch base.GetKind() {
	case metadata.KindCall:
		calleeFunc = calleeNameOf(base.Fun)
		valPos = impl.GetString(base.Position)
	case metadata.KindIdent:
		// Edge-first (assignmentsAt): the error variable may be assigned inside a
		// returned handler closure, where the assignment is recorded on the call
		// edge rather than a name-resolvable declared function's scope (#189).
		as := assignmentsAt(impl, baseNode.GetEdge(), base.GetName())
		if len(as) == 0 || as[len(as)-1].Value.GetKind() != metadata.KindCall {
			return nil, false
		}
		calleeFunc = as[len(as)-1].CalleeFunc
		valPos = impl.GetString(as[len(as)-1].Value.Position)
	default:
		return nil, false
	}
	if calleeFunc == "" {
		return nil, false
	}

	// The constructor's return field's parameter and the argument bound to it —
	// the branch variable (`statusCode`). Package-agnostic (calleeFunc came from
	// the resolved call, not a known package).
	statusArg, ok := constructorFieldArg(impl, baseNode.GetEdge().Caller.ID(), calleeFunc, "", valPos, fieldName)
	if !ok || statusArg.GetKind() != metadata.KindIdent {
		return nil, false
	}
	return r.expandVarStatuses(statusArg.GetName(), scope, impl)
}

// calleeNameOf returns the bare name of the function a call invokes, from the
// call's Fun. A same-package call (`NewErr(...)`) has a plain ident Fun whose
// name is the function; a cross-package call (`pkg.NewErr(...)`) has a selector
// Fun whose name lives in .Sel. Returns "" when neither yields a name.
func calleeNameOf(fun *metadata.CallArgument) string {
	if fun == nil {
		return ""
	}
	if name := fun.GetName(); name != "" {
		return name
	}
	if fun.GetKind() == metadata.KindSelector && fun.Sel != nil {
		return fun.Sel.GetName()
	}
	return ""
}

// findCallEdge returns the call-graph edge from callerID to a callee named
// calleeFunc, preferring the one at valPos (disambiguating repeated calls to the
// same function in one caller) and falling back to the first match. When
// calleePkg is non-empty the callee package must match too (disambiguating
// same-named constructors across packages); "" skips the package check.
func findCallEdge(impl *ContextProviderImpl, callerID, calleeFunc, calleePkg, valPos string) *metadata.CallGraphEdge {
	var first *metadata.CallGraphEdge
	for i := range impl.meta.CallGraph {
		e := &impl.meta.CallGraph[i]
		if e.Caller.ID() != callerID || impl.GetString(e.Callee.Name) != calleeFunc {
			continue
		}
		if calleePkg != "" && impl.GetString(e.Callee.Pkg) != calleePkg {
			continue
		}
		if valPos != "" && impl.GetString(e.Position) == valPos {
			return e
		}
		if first == nil {
			first = e
		}
	}
	return first
}

// compositeLitOf peels an optional address-of (`&T{...}`) and returns the
// composite literal, or nil when arg is not (a pointer to) one.
func compositeLitOf(arg *metadata.CallArgument) *metadata.CallArgument {
	if arg == nil {
		return nil
	}
	if arg.GetKind() == metadata.KindUnary && arg.X != nil {
		arg = arg.X
	}
	if arg.GetKind() == metadata.KindCompositeLit {
		return arg
	}
	return nil
}

// constructorFieldArg resolves a selector field on a value produced by a
// constructor call: given the constructor reachable from callerID as
// (calleeFunc, calleePkg) at valPos, it locates that call edge, maps fieldName
// to the constructor parameter its return composite assigns it from
// (constructorFieldParam), and returns the argument bound to that parameter at
// the call site. calleePkg disambiguates same-named constructors across packages
// ("" skips the check); valPos disambiguates repeated calls in one caller. This
// is the shared "constructor return field -> bound argument" tail of both the
// single-status (statusFromConstructorField) and one->many
// (statusesFromConstructorField) resolvers (issue #182).
func constructorFieldArg(impl *ContextProviderImpl, callerID, calleeFunc, calleePkg, valPos, fieldName string) (*metadata.CallArgument, bool) {
	ctorEdge := findCallEdge(impl, callerID, calleeFunc, calleePkg, valPos)
	if ctorEdge == nil || ctorEdge.ParamArgMap == nil {
		return nil, false
	}
	ctor := findFunctionByName(impl.meta, impl.GetString(ctorEdge.Callee.Pkg), calleeFunc)
	if ctor == nil {
		return nil, false
	}
	paramName := constructorFieldParam(ctor, fieldName)
	if paramName == "" {
		return nil, false
	}
	a, ok := ctorEdge.ParamArgMap[paramName]
	if !ok {
		return nil, false
	}
	return &a, true
}

// expandStatusesFromIdent walks the caller's function-level AssignmentMap
// for the given ident and returns the distinct status codes implied by the
// RHS calls of each assignment. For each assignment whose value is a call,
// the first argument that parses as a known HTTP status (via
// schemaMapper.MapStatusCode) is taken as that branch's status.
//
// Returns nil when:
//   - the arg is not a KindIdent,
//   - the caller function or its AssignmentMap can't be located, or
//   - fewer than two assignments exist (single-branch flows are left
//     untouched so existing latest-wins behaviour is preserved).
func (r *ResponsePatternMatcherImpl) expandStatusesFromIdent(arg *metadata.CallArgument, edge *metadata.CallGraphEdge) ([]int, bool) {
	if arg == nil || arg.GetKind() != metadata.KindIdent || edge == nil {
		return nil, false
	}
	impl, ok := r.contextProvider.(*ContextProviderImpl)
	if !ok || impl.meta == nil {
		return nil, false
	}
	fn := findFunction(impl.meta, impl.GetString(edge.Caller.Pkg), impl.GetString(edge.Caller.Name))
	if fn == nil {
		return nil, false
	}
	return r.expandVarStatuses(arg.GetName(), fn, impl)
}

// expandVarStatuses fans a variable's branch assignments in function fn out to
// the set of HTTP status codes it can hold: `statusCode = http.StatusNotFound`
// (a constant, in any branch of a switch/if) and `err = NewError(msg, 404)` (a
// constructor call carrying a status literal) both count. residue is true when
// at least one assignment is non-constant (its concrete code can't be pinned),
// so the caller can keep an honest `default` for it. Returns nil codes when the
// variable has fewer than two assignments — single-branch flows resolve through
// the normal latest-wins path and must not be split. Issues #39 and #155.
func (r *ResponsePatternMatcherImpl) expandVarStatuses(name string, fn *metadata.Function, impl *ContextProviderImpl) (codes []int, residue bool) {
	assigns, ok := fn.AssignmentMap[name]
	if !ok || len(assigns) < 2 {
		return nil, false
	}
	seen := make(map[int]struct{}, len(assigns))
	for i := range assigns {
		if s, ok := r.statusCodeOfValue(&assigns[i].Value, impl); ok {
			if _, dup := seen[s]; !dup {
				seen[s] = struct{}{}
				codes = append(codes, s)
			}
			continue
		}
		residue = true
	}
	return codes, residue
}

// statusCodeOfValue resolves a single assignment right-hand side to an HTTP
// status code: a constant / const-selector (`http.StatusNotFound`, `404`)
// directly, or a constructor call by taking the first argument that parses as a
// status literal (`NewError(msg, 404)`). Returns false for a non-constant value
// (a computed status that can't be pinned).
func (r *ResponsePatternMatcherImpl) statusCodeOfValue(value *metadata.CallArgument, impl *ContextProviderImpl) (int, bool) {
	if value == nil {
		return 0, false
	}
	if value.GetKind() == metadata.KindCall {
		// Accept a constructor's status only when EXACTLY ONE argument parses as
		// a status — otherwise which numeric field is the HTTP status is a guess
		// (e.g. NewErr(retryAfter, code)), so keep it as an unresolved residue
		// rather than pick the first. Precise field→parameter resolution for the
		// multi-status case is handled by statusesFromConstructorField.
		found, count := 0, 0
		for _, a := range value.Args {
			if a == nil {
				continue
			}
			if s, ok := r.schemaMapper.MapStatusCode(impl.GetArgumentInfo(a)); ok {
				found = s
				count++
			}
		}
		if count == 1 {
			return found, true
		}
		return 0, false
	}
	return r.schemaMapper.MapStatusCode(impl.GetArgumentInfo(value))
}

// resolveTypeOrigin traces the origin of a type through assignments and type parameters
func (r *ResponsePatternMatcherImpl) resolveTypeOrigin(arg *metadata.CallArgument, node TrackerNodeInterface, originalType string) string {
	// NEW: If the argument has resolved type information, use it
	if resolvedType := arg.GetResolvedType(); resolvedType != "" {
		return resolvedType
	}

	// If it's a generic type with a concrete resolution, use it
	if arg.IsGenericType && arg.GenericTypeName != -1 {
		if concreteType, exists := node.GetTypeParamMap()[arg.GetGenericTypeName()]; exists {
			return concreteType
		}
	}

	// Selector expression like `api.Message` — resolve the field's declared
	// type via metadata so the schema mapper doesn't $ref a nonexistent
	// "APIError.Message" pseudo-type.
	if arg.GetKind() == metadata.KindSelector {
		if t := resolveSelectorFieldType(arg, r.contextProvider); t != "" {
			return t
		}
	}

	// Original logic for type resolution
	if arg.GetKind() == metadata.KindIdent {
		// Check if this variable has assignments that might give us more type information
		edge := node.GetEdge()
		if assignments, exists := edge.AssignmentMap[arg.GetName()]; exists {
			for _, assignment := range assignments {
				if assignment.ConcreteType != 0 {
					concreteType := r.contextProvider.GetString(assignment.ConcreteType)
					if concreteType != "" {
						return concreteType
					}
				}
			}
		}
		// Interface-typed body: the concrete value is usually assigned in the
		// enclosing handler (`var a Animal = Dog{}; Encode(a)`), not on this
		// call's edge. Resolve it to the concrete type so the schema documents
		// Dog rather than the empty Animal interface.
		if concrete := r.concreteFromEnclosingFunc(arg, edge, originalType); concrete != "" {
			return concrete
		}
		// Interface-typed function parameter: the concrete value is bound at the
		// call site that entered the enclosing function (`writeAnimal(w, Dog{})`
		// with the response `Encode(v)` inside writeAnimal). Resolve the param to
		// that argument.
		if concrete := r.concreteFromParamBinding(arg, node, originalType); concrete != "" {
			return concrete
		}
	}

	return originalType
}

// concreteFromEnclosingFunc resolves an interface-typed body argument to the
// concrete type assigned to it in the enclosing handler. It only fires when the
// original type is a known interface and the enclosing function assigns exactly
// one concrete (non-interface) type to the variable — an ambiguous set of
// concrete assignments keeps the interface (honest over wrong).
func (r *BasePatternMatcher) concreteFromEnclosingFunc(arg *metadata.CallArgument, edge *metadata.CallGraphEdge, originalType string) string {
	if edge == nil {
		return ""
	}
	meta := edge.Callee.Meta
	if meta == nil || !isInterfaceTypeName(originalType, meta) {
		return ""
	}
	callerName := r.contextProvider.GetString(edge.Caller.Name)
	if callerName == "" {
		return ""
	}
	fn := findFunctionByName(meta, r.contextProvider.GetString(edge.Caller.Pkg), callerName)
	if fn == nil {
		return ""
	}
	concrete := ""
	for _, a := range fn.AssignmentMap[arg.GetName()] {
		if a.ConcreteType == 0 {
			continue
		}
		ct := r.contextProvider.GetString(a.ConcreteType)
		if ct == "" || isInterfaceTypeName(ct, meta) {
			continue
		}
		if concrete != "" && concrete != ct {
			return "" // more than one concrete type assigned — ambiguous
		}
		concrete = ct
	}
	return concrete
}

// concreteFromParamBinding resolves an interface-typed parameter used as a
// response body to the concrete argument bound to it at the call site that
// entered the enclosing function. It walks up the tracker tree to the edge
// whose callee IS the enclosing function (the response edge's caller) — not the
// immediate parent, whose own parameters can shadow the name — and reads that
// edge's ParamArgMap. Non-interface / unresolvable arguments are ignored so the
// interface is kept.
func (r *BasePatternMatcher) concreteFromParamBinding(arg *metadata.CallArgument, node TrackerNodeInterface, originalType string) string {
	edge := node.GetEdge()
	if edge == nil {
		return ""
	}
	meta := edge.Callee.Meta
	if meta == nil || !isInterfaceTypeName(originalType, meta) {
		return ""
	}
	enclosing := edge.Caller.BaseID() // the function whose param `arg` is
	if enclosing == "" {
		return ""
	}
	for p := node.GetParent(); p != nil; p = p.GetParent() {
		pe := p.GetEdge()
		if pe == nil || pe.Callee.BaseID() != enclosing {
			continue
		}
		callerArg, ok := pe.ParamArgMap[arg.GetName()]
		if !ok {
			return ""
		}
		ct := r.contextProvider.GetArgumentInfo(&callerArg)
		if ct == "" || isInterfaceTypeName(ct, meta) {
			return ""
		}
		return ct
	}
	return ""
}

// concreteFromCalleeReturn resolves an interface-typed call result used as a
// response body to the concrete type the called function actually returns
// (`Encode(makeAnimal())` where makeAnimal() Animal { return Dog{} } → Dog). If
// the callee's captured return values name more than one concrete type it is
// ambiguous, so the interface is kept.
func (r *BasePatternMatcher) concreteFromCalleeReturn(arg *metadata.CallArgument, edge *metadata.CallGraphEdge, originalType string) string {
	if edge == nil || arg.Fun == nil {
		return ""
	}
	meta := edge.Callee.Meta
	if meta == nil || !isInterfaceTypeName(originalType, meta) {
		return ""
	}
	name := arg.Fun.GetName()
	if name == "" && arg.Fun.Sel != nil {
		name = arg.Fun.Sel.GetName()
	}
	fn := findFunctionByName(meta, arg.Fun.GetPkg(), name)
	if fn == nil {
		return ""
	}
	concrete := ""
	for i := range fn.ReturnVars {
		ct := r.contextProvider.GetArgumentInfo(&fn.ReturnVars[i])
		if ct == "" || isInterfaceTypeName(ct, meta) {
			continue
		}
		if concrete != "" && concrete != ct {
			return "" // more than one concrete type returned — ambiguous
		}
		concrete = ct
	}
	return concrete
}

// isInterfaceTypeName reports whether a type name resolves, in metadata, to an
// interface type.
func isInterfaceTypeName(typeName string, meta *metadata.Metadata) bool {
	if typeName == "" || meta == nil {
		return false
	}
	core := typemodel.Parse(typeName).Core()
	if core == nil {
		return false
	}
	t := typeByName(core.Pkg, core.Name, meta)
	return t != nil && getStringFromPool(meta, t.Kind) == "interface"
}

// ParamPatternMatcherImpl implements ParamPatternMatcher
type ParamPatternMatcherImpl struct {
	*BasePatternMatcher
	pattern ParamPattern
}

// NewParamPatternMatcher creates a new param pattern matcher
func NewParamPatternMatcher(pattern ParamPattern, cfg *APISpecConfig, contextProvider ContextProvider, typeResolver TypeResolver) *ParamPatternMatcherImpl {
	return &ParamPatternMatcherImpl{
		BasePatternMatcher: NewBasePatternMatcher(cfg, contextProvider, typeResolver),
		pattern:            pattern,
	}
}

// MatchNode checks if a node matches the param pattern
func (p *ParamPatternMatcherImpl) MatchNode(node TrackerNodeInterface) bool {
	if node == nil || node.GetEdge() == nil {
		return false
	}

	edge := node.GetEdge()
	callName := p.contextProvider.GetString(edge.Callee.Name)
	recvType := p.contextProvider.GetString(edge.Callee.RecvType)
	recvPkg := p.contextProvider.GetString(edge.Callee.Pkg)

	// Build fully qualified receiver type
	fqRecvType := recvPkg
	if fqRecvType != "" && recvType != "" {
		fqRecvType += "." + recvType
	} else if recvType != "" {
		fqRecvType = recvType
	}

	// Check call regex
	if p.pattern.CallRegex != "" && !p.matchPattern(p.pattern.CallRegex, callName) {
		return false
	}

	// Check function name regex
	if p.pattern.FunctionNameRegex != "" {
		funcName := p.contextProvider.GetString(edge.Caller.Name)
		if !p.matchPattern(p.pattern.FunctionNameRegex, funcName) {
			return false
		}
	}

	// Check receiver type
	if p.pattern.RecvTypeRegex != "" {
		re, err := cachedRegex(p.pattern.RecvTypeRegex)
		if err != nil || !re.MatchString(fqRecvType) {
			return false
		}
	} else if p.pattern.RecvType != "" && p.pattern.RecvType != fqRecvType {
		return false
	}

	return true
}

// GetPattern returns the param pattern
func (p *ParamPatternMatcherImpl) GetPattern() interface{} {
	return p.pattern
}

// GetPriority returns the priority of this pattern
func (p *ParamPatternMatcherImpl) GetPriority() int {
	priority := 0
	if p.pattern.CallRegex != "" {
		priority += 10
	}
	if p.pattern.FunctionNameRegex != "" {
		priority += 5
	}
	if p.pattern.RecvTypeRegex != "" || p.pattern.RecvType != "" {
		priority += 3
	}
	return priority
}

// ExtractParam extracts parameter information from a matched node
func (p *ParamPatternMatcherImpl) ExtractParam(node TrackerNodeInterface, route *RouteInfo) *Parameter {
	param := &Parameter{
		In: p.pattern.ParamIn,
	}

	edge := node.GetEdge()
	if len(edge.Args) > p.pattern.ParamArgIndex {
		param.Name = p.contextProvider.GetArgumentInfo(edge.Args[p.pattern.ParamArgIndex])
	}

	if p.pattern.TypeFromArg && len(edge.Args) > p.pattern.TypeArgIndex {
		arg := edge.Args[p.pattern.TypeArgIndex]
		paramType := p.contextProvider.GetArgumentInfo(arg)

		// Check if this is a literal value - if so, determine appropriate type
		if arg.GetKind() == metadata.KindLiteral {
			// For literal values, determine the appropriate type based on the value
			paramType = determineLiteralType(paramType)
		} else {
			// Trace type origin for non-literal arguments
			paramType = p.resolveTypeOrigin(arg, node, paramType)

			// Apply dereferencing if needed
			if p.pattern.Deref && strings.HasPrefix(paramType, "*") {
				paramType = strings.TrimPrefix(paramType, "*")
			}
		}

		schema, _ := mapGoTypeToOpenAPISchema(route.UsedTypes, paramType, route.Metadata, p.cfg, nil)
		param.Schema = schema
	}

	// Ensure all parameters have a schema - default to string if none specified
	if param.Schema == nil {
		param.Schema = &Schema{Type: "string"}
	}

	// Ensure path parameters are always required
	if p.pattern.ParamIn == "path" {
		param.Required = true
	}

	return param
}

// resolveTypeOrigin traces the origin of a type through assignments and type parameters
func (p *ParamPatternMatcherImpl) resolveTypeOrigin(arg *metadata.CallArgument, node TrackerNodeInterface, originalType string) string {
	// NEW: If the argument has resolved type information, use it
	if resolvedType := arg.GetResolvedType(); resolvedType != "" {
		return resolvedType
	}

	// If it's a generic type with a concrete resolution, use it
	if arg.IsGenericType && arg.GenericTypeName != -1 {
		if concreteType, exists := node.GetTypeParamMap()[arg.GetGenericTypeName()]; exists {
			return concreteType
		}
	}

	// Selector expression — resolve via metadata field lookup.
	if arg.GetKind() == metadata.KindSelector {
		if t := resolveSelectorFieldType(arg, p.contextProvider); t != "" {
			return t
		}
	}

	// Original logic for type resolution
	if arg.GetKind() == metadata.KindIdent {
		// Check if this variable has assignments that might give us more type information
		edge := node.GetEdge()
		if assignments, exists := edge.AssignmentMap[arg.GetName()]; exists {
			for _, assignment := range assignments {
				if assignment.ConcreteType != 0 {
					concreteType := p.contextProvider.GetString(assignment.ConcreteType)
					if concreteType != "" {
						return concreteType
					}
				}
			}
		}
	}

	return originalType
}

// OverrideApplierImpl implements OverrideApplier
type OverrideApplierImpl struct {
	cfg *APISpecConfig
}

// NewOverrideApplier creates a new override applier
func NewOverrideApplier(cfg *APISpecConfig) *OverrideApplierImpl {
	return &OverrideApplierImpl{
		cfg: cfg,
	}
}

// ApplyOverrides applies manual overrides to route info
func (o *OverrideApplierImpl) ApplyOverrides(routeInfo *RouteInfo) {
	for _, override := range o.cfg.Overrides {
		if override.FunctionName == routeInfo.Function {
			if override.Summary != "" {
				routeInfo.Summary = override.Summary
			}
			if res, exists := routeInfo.Response[fmt.Sprintf("%d", override.ResponseStatus)]; exists && override.ResponseStatus != 0 && routeInfo.Response != nil {
				res.StatusCode = override.ResponseStatus
			}
			if override.ResponseType != "" && routeInfo.Response != nil {
				for _, res := range routeInfo.Response {
					res.BodyType = preprocessingBodyType(override.ResponseType)
				}
			}
			if len(override.Tags) > 0 {
				routeInfo.Tags = override.Tags
			}
		}
	}
}

// HasOverride checks if there's an override for a function
func (o *OverrideApplierImpl) HasOverride(functionName string) bool {
	for _, override := range o.cfg.Overrides {
		if override.FunctionName == functionName {
			return true
		}
	}
	return false
}
