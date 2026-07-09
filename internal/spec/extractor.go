package spec

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/ehabterra/apispec/internal/metadata"
)

const (
	TypeSep    = "-->"
	defaultSep = "."
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
	Tags      []string
	Request   *RequestInfo
	Response  map[string]*ResponseInfo
	Params    []Parameter

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
}

// ResponseInfo represents response information
type ResponseInfo struct {
	StatusCode  int
	ContentType string
	BodyType    string
	Schema      *Schema
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

	for _, matcher := range e.routeMatchers {
		if matcher.MatchNode(node) {
			priority := matcher.GetPriority()
			if !found || priority > bestPriority {
				found = matcher.ExtractRoute(node, routeInfo)
				if found {
					bestPriority = priority
				}
			}
		}
	}

	return found
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

// maxWrapperLookThroughDepth bounds how deep wrapper look-through follows the
// call graph (custom mw -> wrapper -> ... -> library constructor).
const maxWrapperLookThroughDepth = 6

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
		out = append(out, e.lookThroughMiddleware(ref, meta, make(map[string]bool), 0)...)
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

// lookThroughMiddleware resolves a single middleware ref, following the call
// graph through wrapper functions until a mapping matches.
func (e *Extractor) lookThroughMiddleware(ref MiddlewareRef, meta *metadata.Metadata, visited map[string]bool, depth int) []MiddlewareRef {
	if anyMappingMatches(ref, e.cfg.SecurityMappings) {
		return []MiddlewareRef{ref}
	}
	if depth >= maxWrapperLookThroughDepth {
		return []MiddlewareRef{ref}
	}
	key := middlewareBaseID(ref)
	if key == "" || visited[key] {
		return []MiddlewareRef{ref}
	}
	visited[key] = true

	// Calls made directly in the function's body, plus calls inside func
	// literals it defines/returns (e.g. a middleware that validates a token in
	// the http.Handler closure it returns).
	direct := meta.Callers[key]
	nested := e.parentFnIndex[key]
	if len(direct) == 0 && len(nested) == 0 {
		return []MiddlewareRef{ref} // external / no analyzable body
	}
	var found []MiddlewareRef
	scan := func(edges []*metadata.CallGraphEdge) {
		for _, edge := range edges {
			callee := e.calleeMiddlewareRef(edge)
			if callee.empty() {
				continue
			}
			for _, r := range e.lookThroughMiddleware(callee, meta, visited, depth+1) {
				if anyMappingMatches(r, e.cfg.SecurityMappings) {
					found = append(found, r)
				}
			}
		}
	}
	scan(direct)
	scan(nested)
	if len(found) > 0 {
		return found
	}
	return []MiddlewareRef{ref} // nothing matched downstream; keep for diagnostics
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

	// Extract route/request/response/params from children with visited edges tracking
	visitedEdges := make(map[string]bool)
	e.extractRouteChildren(node, routeInfo, mountTags, routes, visitedEdges)

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
				(*routes)[i] = routeInfo
				found = true
				break
			}
		}
		if !found {
			*routes = append(*routes, routeInfo)
		}
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
func (e *Extractor) extractRouteChildren(routeNode TrackerNodeInterface, route *RouteInfo, mountTags []string, routes *[]*RouteInfo, visitedEdges map[string]bool) {
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
			route.Request = preferRequestInfo(route.Request, req)
		}

		// Extract responses (multiple if status fan-out applies — see
		// ExtractResponse / issue #39). Each emitted ResponseInfo lands
		// under its own status-keyed slot in route.Response.
		for _, resp := range e.extractResponseFromNode(child, route, visitedEdges, routeNode.GetKey()) {
			if resp != nil && (resp.BodyType != "" || resp.StatusCode != 0) {
				route.Response[fmt.Sprintf("%d", resp.StatusCode)] = resp
			}
		}

		// Extract parameters
		route.Params = append(route.Params, e.extractParamsFromNode(child, route)...)

		// Recursive extraction
		e.extractRouteChildren(child, route, mountTags, routes, visitedEdges)
	}

	// Extract parameters from the route node itself
	route.Params = append(route.Params, e.extractParamsFromNode(routeNode, route)...)
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
	for _, matcher := range e.requestMatchers {
		if matcher.MatchNode(node) {
			return matcher.ExtractRequest(node, route)
		}
	}
	return nil
}

// extractResponseFromNode extracts response information from a node.
// Returns a slice because a single call site can yield multiple responses
// when conditional status codes apply (see ExtractResponse / issue #39).
//
// The visited-edge key is qualified by parentKey (the node through which this
// one was reached). A response helper invoked from several branches with
// different statuses — e.g. respondWithError(w,…,400) and
// respondWithError(w,…,500), both reaching the SAME WriteHeader edge — would
// otherwise be deduped by callee alone and collapse to the first status seen.
// Qualifying by the call site lets each distinct status be resolved. (The same
// node also gets reached via internal encoder-chain paths with no status
// context; those yield a "default" that buildResponses drops when it carries no
// new body information — see uninformativeDefault.)
func (e *Extractor) extractResponseFromNode(node TrackerNodeInterface, route *RouteInfo, visitedEdges map[string]bool, parentKey string) []*ResponseInfo {
	if node == nil || node.GetEdge() == nil {
		return nil
	}
	edge := node.GetEdge()
	edgeID := parentKey + "|" + edge.Callee.ID()
	if visitedEdges[edgeID] {
		return nil // already processed via this same call site
	}
	visitedEdges[edgeID] = true
	for _, matcher := range e.responseMatchers {
		if matcher.MatchNode(node) {
			return matcher.ExtractResponse(node, route)
		}
	}
	return nil
}

// extractParamsFromNode extracts parameter information from a node. Most
// patterns yield at most one parameter (returned as a single-element slice),
// but map-key patterns (gorilla/mux's `Vars(r)["id"]`) can yield several,
// one per indexed key that matches a path placeholder.
func (e *Extractor) extractParamsFromNode(node TrackerNodeInterface, route *RouteInfo) []Parameter {
	for _, matcher := range e.paramMatchers {
		if !matcher.MatchNode(node) {
			continue
		}
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
	return nil
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

// handlerReachesAccessor reports whether the route's handler transitively calls
// the map-key accessor described by pattern, following the call graph through
// helper functions up to maxWrapperLookThroughDepth. Seeds are the handler
// function's call-graph base IDs (matched by name + package), so it works
// regardless of the exact key format.
func (e *Extractor) handlerReachesAccessor(route *RouteInfo, pattern ParamPattern) bool {
	meta := route.Metadata
	if meta == nil || route.Function == "" {
		return false
	}
	// route.Function is package-qualified ("pkg/path.Handler"); call-graph
	// caller names are bare ("Handler"), so strip the package prefix to match.
	bareFunc := route.Function
	if route.Package != "" {
		bareFunc = strings.TrimPrefix(route.Function, route.Package+".")
	}
	visited := make(map[string]bool)
	for i := range meta.CallGraph {
		edge := &meta.CallGraph[i]
		if getString(meta, edge.Caller.Name) != bareFunc {
			continue
		}
		if route.Package != "" && getString(meta, edge.Caller.Pkg) != route.Package {
			continue
		}
		if e.reachesAccessor(meta, edge.Caller.BaseID(), pattern, visited, 0) {
			return true
		}
	}
	return false
}

// reachesAccessor walks meta.Callers from a function base ID, returning true if
// any transitively-reachable call edge matches the accessor pattern.
func (e *Extractor) reachesAccessor(meta *metadata.Metadata, key string, pattern ParamPattern, visited map[string]bool, depth int) bool {
	if key == "" || visited[key] || depth > maxWrapperLookThroughDepth {
		return false
	}
	visited[key] = true
	for _, edge := range meta.Callers[key] {
		if edgeMatchesAccessor(meta, edge, pattern) {
			return true
		}
		if e.reachesAccessor(meta, edge.Callee.BaseID(), pattern, visited, depth+1) {
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
	pattern ResponsePattern
}

// NewResponsePatternMatcher creates a new response pattern matcher
func NewResponsePatternMatcher(pattern ResponsePattern, cfg *APISpecConfig, contextProvider ContextProvider, typeResolver TypeResolver) *ResponsePatternMatcherImpl {
	return &ResponsePatternMatcherImpl{
		BasePatternMatcher: NewBasePatternMatcher(cfg, contextProvider, typeResolver),
		pattern:            pattern,
	}
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
		} else if callerArg := r.traceArgViaParent(statusArg, node); callerArg != nil {
			// The status arg is a parameter of the enclosing function
			// (e.g. WriteHeader(status) inside writeJSON(w, status, v)).
			// Walk up to the caller's call site and read the actual value.
			// Per-route tracker tree isolates each handler's path, so the
			// status pairs correctly with the body resolved below.
			callerStr := r.contextProvider.GetArgumentInfo(callerArg)
			if status, ok := r.schemaMapper.MapStatusCode(callerStr); ok {
				statusResolved = true
				respInfo.StatusCode = status
			}
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

		// Parameter tracing: if the body arg is a parameter of the
		// enclosing function (e.g. Encode(v) inside writeJSON(w, status,
		// v)), follow it to the caller's actual argument so we get the
		// concrete type — otherwise `v any` would resolve to a generic
		// object. Per-route isolation in the tracker tree means each
		// handler's response gets the type from its own call site.
		if callerArg := r.traceArgViaParent(arg, node); callerArg != nil {
			arg = callerArg
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
			if arg.GetKind() == metadata.KindCall {
				if t := arg.GetType(); t != "" {
					bodyType = t
				}
			}

			// Trace type origin for non-literal arguments
			bodyType = r.resolveTypeOrigin(arg, node, bodyType)

			// Apply dereferencing if needed
			if r.pattern.Deref && strings.HasPrefix(bodyType, "*") {
				bodyType = strings.TrimPrefix(bodyType, "*")
			}
		}

		respInfo.BodyType = preprocessingBodyType(bodyType)

		schema, _ := mapGoTypeToOpenAPISchema(route.UsedTypes, bodyType, route.Metadata, r.cfg, nil)

		// Wrapper specialisation: when the body resolves to a struct
		// whose fields are bound to constructor parameters at the
		// helper boundary (e.g. `response := NewEnvelope(msg, data,
		// code)` inside RespondWithSuccess), recover the caller-site
		// concrete type for each bound field and compose an `allOf`
		// override so per-route schemas reflect the actual payload
		// type instead of the wrapper's declared `interface{}`.
		if overrides := r.collectWrapperOverrides(arg, node); len(overrides) > 0 {
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
		if expanded := r.expandStatusesFromIdent(edge.Args[r.pattern.StatusArgIndex], edge); len(expanded) > 1 {
			out := make([]*ResponseInfo, 0, len(expanded))
			for _, st := range expanded {
				out = append(out, &ResponseInfo{
					StatusCode:  st,
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

// traceArgViaParent walks one step up the tracker tree to recover the
// caller-site value of a parameter ident. When a response pattern matches
// inside a helper (writeJSON-style) — e.g. WriteHeader(status) where
// status is a parameter of writeJSON — the matched call's args reference
// parameters, not literals. The parent tracker node represents the call
// to the helper, and that edge's ParamArgMap maps callee parameter
// names back to the caller's actual arguments.
//
// Returns nil when the arg isn't an ident, there is no parent node,
// or the parameter name isn't present in the parent's ParamArgMap.
//
// Per-route isolation in the tracker tree is what makes this sound:
// each handler's path through the helper is a distinct tracker subtree,
// so two routes that call writeJSON with different statuses each
// resolve to their own value independently.
func (r *ResponsePatternMatcherImpl) traceArgViaParent(arg *metadata.CallArgument, node TrackerNodeInterface) *metadata.CallArgument {
	return argViaParent(arg, node)
}

// argViaParent walks one step up the tracker tree to recover the caller-site
// value of a parameter ident. The parent tracker node represents the call into
// the function the matched call lives in, and that edge's ParamArgMap maps
// callee parameter names back to the caller's actual arguments. Returns nil
// when the arg isn't an ident, there's no parent, or the name isn't a mapped
// parameter. Shared by the response and request matchers.
func argViaParent(arg *metadata.CallArgument, node TrackerNodeInterface) *metadata.CallArgument {
	if arg == nil || arg.GetKind() != metadata.KindIdent || node == nil {
		return nil
	}
	parent := node.GetParent()
	if parent == nil {
		return nil
	}
	parentEdge := parent.GetEdge()
	if parentEdge == nil || parentEdge.ParamArgMap == nil {
		return nil
	}
	if callerArg, ok := parentEdge.ParamArgMap[arg.GetName()]; ok {
		return &callerArg
	}
	return nil
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
		next := argViaParent(arg, cur)
		if next == nil {
			break
		}
		arg = next
		cur = cur.GetParent()
	}
	return arg, cur
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
func (r *ResponsePatternMatcherImpl) expandStatusesFromIdent(arg *metadata.CallArgument, edge *metadata.CallGraphEdge) []int {
	if arg == nil || arg.GetKind() != metadata.KindIdent || edge == nil {
		return nil
	}
	impl, ok := r.contextProvider.(*ContextProviderImpl)
	if !ok || impl.meta == nil {
		return nil
	}
	callerName := impl.GetString(edge.Caller.Name)
	callerPkg := impl.GetString(edge.Caller.Pkg)
	fn := findFunction(impl.meta, callerPkg, callerName)
	if fn == nil {
		return nil
	}
	assigns, ok := fn.AssignmentMap[arg.GetName()]
	if !ok || len(assigns) < 2 {
		return nil
	}
	seen := make(map[int]struct{}, len(assigns))
	out := make([]int, 0, len(assigns))
	for _, a := range assigns {
		if a.Value.GetKind() != metadata.KindCall {
			continue
		}
		for _, callArg := range a.Value.Args {
			if callArg == nil {
				continue
			}
			argStr := impl.GetArgumentInfo(callArg)
			status, ok := r.schemaMapper.MapStatusCode(argStr)
			if !ok {
				continue
			}
			if _, dup := seen[status]; dup {
				break
			}
			seen[status] = struct{}{}
			out = append(out, status)
			break // first matching arg wins per assignment
		}
	}
	return out
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
	}

	return originalType
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
