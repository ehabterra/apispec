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
	"net/http"
	"strings"
	"unicode"

	"github.com/ehabterra/apispec/internal/metadata"
	"github.com/ehabterra/apispec/internal/typemodel"
)

// BasePatternMatcher provides common functionality for all pattern matchers
type BasePatternMatcher struct {
	contextProvider ContextProvider
	cfg             *APISpecConfig
	schemaMapper    SchemaMapper
}

// NewBasePatternMatcher creates a new base pattern matcher
func NewBasePatternMatcher(cfg *APISpecConfig, contextProvider ContextProvider) *BasePatternMatcher {
	return &BasePatternMatcher{
		contextProvider: contextProvider,
		cfg:             cfg,
		schemaMapper:    NewSchemaMapper(cfg),
	}
}

// resolvePathArg renders a CallArgument as an OpenAPI path string.
//
// Literals and const idents resolve to their value via the context
// provider. Function-call expressions (e.g. r.Mount(mountPoint(prefix,
// "/api"), sub)) cannot be statically evaluated without interpreting
// the Go body — see issue #34 — so they surface as a {placeholder}
// named after the called function. The second return value, dynamicName,
// are the placeholder names synthesized for this path (so the caller can
// register a shared component parameter for each) and nil when none were.
//
// A concatenation (`opts.BaseURL + "/things"`) folds to the joined value —
// see resolveConcatenatedPath, which is what generated servers need (#274).
//
// All other kinds fall through to GetArgumentInfo for backwards
// compatibility — handling KindIdent (non-const variable) similarly is a
// possible follow-up.
func (b *BasePatternMatcher) resolvePathArg(arg *metadata.CallArgument, node TrackerNodeInterface) (path string, dynamicNames []string) {
	if arg == nil {
		return "", nil
	}
	switch arg.GetKind() {
	case metadata.KindBinary:
		return b.resolveConcatenatedPath(arg, node)
	case metadata.KindCall:
		p, name := placeholderFor(arg)
		return p, dynamicNameList(name)
	}
	return b.contextProvider.GetArgumentInfo(arg), nil
}

// dynamicNameList wraps a single synthesized name, or nil when there is none.
func dynamicNameList(name string) []string {
	if name == "" {
		return nil
	}
	return []string{name}
}

// placeholderFor names an argument that cannot be evaluated statically, so the
// route stays addressable instead of losing the segment (issue #34).
func placeholderFor(arg *metadata.CallArgument) (path, dynamicName string) {
	name := arg.GetName()
	if name == "" && arg.Fun != nil {
		name = arg.Fun.GetName()
	}
	if name == "" && arg.Sel != nil {
		name = arg.Sel.GetName()
	}
	if name == "" {
		name = "path"
	}
	return "{" + name + "}", name
}

// resolveConcatenatedPath folds a `+` chain into one path (issue #274).
//
// This is the shape every oapi-codegen-generated server registers with —
// `r.Post(options.BaseURL+"/things", h)` — and before this the whole argument
// resolved to nothing, so the route was documented at its mount prefix alone
// (or at "/"), losing the only part of the path that was written literally.
//
// Each operand is resolved independently and the results are joined in source
// order. An operand that cannot be evaluated becomes a {placeholder} rather
// than disappearing: an unresolved prefix must leave the route addressable and
// visibly incomplete, not silently shorten its path.
func (b *BasePatternMatcher) resolveConcatenatedPath(arg *metadata.CallArgument, node TrackerNodeInterface) (path string, dynamicNames []string) {
	operands := flattenConcat(arg, nil)
	if operands == nil {
		// Not a `+` chain (no other operator builds a path); treat the whole
		// expression as one unresolvable value.
		p, name := placeholderFor(arg)
		return p, dynamicNameList(name)
	}

	var sb strings.Builder
	for _, operand := range operands {
		value, name := b.resolvePathOperand(operand, node)
		sb.WriteString(value)
		// EVERY placeholder needs its name reported: each one becomes a declared
		// path parameter, and a `{name}` left undeclared is an invalid path
		// template — which is what `a() + b() + "/x"` would produce if only the
		// first were returned.
		if name != "" {
			dynamicNames = appendUniqueStrings(dynamicNames, name)
		}
	}
	return sb.String(), dynamicNames
}

// flattenConcat returns the operands of a `+` chain left to right, or nil when
// the expression contains any other operator.
func flattenConcat(arg *metadata.CallArgument, acc []*metadata.CallArgument) []*metadata.CallArgument {
	if arg == nil {
		return acc
	}
	if arg.GetKind() != metadata.KindBinary {
		return append(acc, arg)
	}
	if arg.GetValue() != "+" || arg.X == nil || arg.Fun == nil {
		return nil
	}
	left := flattenConcat(arg.X, acc)
	if left == nil {
		return nil
	}
	return flattenConcat(arg.Fun, left)
}

// resolvePathOperand evaluates one operand of a concatenated path. It returns
// the value to append and, when the operand had to be approximated, the
// placeholder name the caller registers as a parameter.
func (b *BasePatternMatcher) resolvePathOperand(arg *metadata.CallArgument, node TrackerNodeInterface) (value, dynamicName string) {
	if arg == nil {
		return "", ""
	}
	// Literals and constants, including a const declared in another package.
	if v, ok := b.contextProvider.ConstantValue(arg); ok {
		return v, ""
	}
	// A prefix the caller passed in: `registerCRUD(r, "/users")` reaching
	// `m.Delete(base+"/{id}", h)`, or a generated server handed its base URL.
	if caller, ok := b.callerArgFor(arg, node); ok {
		if v, ok := b.contextProvider.ConstantValue(caller); ok {
			return v, ""
		}
	}
	// A field read off a struct the caller passed in, which is how a generated
	// server takes its base URL: `HandlerWithOptions(si, ChiServerOptions{})`
	// leaves BaseURL at its zero value, and the correct contribution is the
	// empty string rather than a placeholder standing in for nothing.
	if v, ok := b.structFieldValue(arg, node); ok {
		return v, ""
	}
	return placeholderFor(arg)
}

// structFieldValue resolves `x.Field` when x traces back to a composite literal
// at the call site. A field the literal does not set has its zero value, which
// for the string fields a path is built from is "" — that is a resolution, not
// a failure, so it reports ok.
func (b *BasePatternMatcher) structFieldValue(arg *metadata.CallArgument, node TrackerNodeInterface) (string, bool) {
	if node == nil || arg.GetKind() != metadata.KindSelector || arg.X == nil || arg.Sel == nil {
		return "", false
	}
	field := arg.Sel.GetName()
	if field == "" {
		return "", false
	}

	base := arg.X
	if resolved, ok := b.callerArgFor(base, node); ok {
		base = resolved
	}
	base = unwrapComposite(base)
	if base == nil || base.GetKind() != metadata.KindCompositeLit {
		return "", false
	}

	for _, elt := range base.Args {
		if elt == nil {
			continue
		}
		if elt.GetKind() != metadata.KindKeyValue || elt.X == nil || elt.Fun == nil {
			// A positional literal (`ChiServerOptions{"/api", nil}`) sets fields
			// without naming them, so "absent from the keys" would NOT mean the
			// zero value — concluding "" here is exactly the silent shortening
			// this function exists to avoid. Resolving it would mean matching the
			// element index against the struct's field order; until then the
			// honest answer is that the operand is unresolved.
			return "", false
		}
		if elt.X.GetName() != field {
			continue
		}
		if v, ok := b.contextProvider.ConstantValue(elt.Fun); ok {
			return v, true
		}
		return "", false // set, but to something we cannot evaluate
	}
	// Not among the literal's fields: the zero value.
	return "", true
}

// callerArgFor follows a parameter ident to the argument its caller passed.
//
// It tries the ordinary wrapper resolution first, and falls back to the
// enclosing-function lookup for a call written inside a closure — which is
// where a generated server registers its routes.
func (b *BasePatternMatcher) callerArgFor(arg *metadata.CallArgument, node TrackerNodeInterface) (*metadata.CallArgument, bool) {
	if arg == nil || arg.GetKind() != metadata.KindIdent || node == nil {
		return nil, false
	}
	if resolved, _ := resolveArgThroughParams(arg, node); resolved != nil && resolved != arg {
		if resolved.GetKind() != metadata.KindIdent {
			return resolved, true
		}
		arg = resolved
	}
	if captured, ok := paramArgOfEnclosingFunc(arg, node); ok {
		return captured, true
	}
	return b.uniqueCallerArg(arg, node)
}

// uniqueCallerArg resolves a parameter from the call sites of the function that
// declares it, used when the ancestor chain holds no frame for that function.
//
// It has to exist because a registration made on a router PARAMETER is re-homed
// under the producer of that router — `registerItems(r, "/v2")` reaching
// `r.Get(prefix+"/items", h)` hangs under `chi.NewRouter()` in main, so the call
// that binds `prefix` is nowhere on the path.
//
// Every call site must agree. One caller (the common case) resolves; two callers
// passing different prefixes leave the value genuinely ambiguous, and the honest
// answer there is the placeholder, not whichever literal happens to be first
// (golden rule #7).
func (b *BasePatternMatcher) uniqueCallerArg(arg *metadata.CallArgument, node TrackerNodeInterface) (*metadata.CallArgument, bool) {
	meta := b.metadata()
	if meta == nil || node == nil {
		return nil, false
	}
	name := arg.GetName()
	enclosing := enclosingFuncID(node)
	if name == "" || enclosing == "" {
		return nil, false
	}

	var chosen *metadata.CallArgument
	var chosenValue string
	for _, edge := range meta.Callees[enclosing] {
		callerArg, ok := edge.ParamArgMap[name]
		if !ok {
			return nil, false
		}
		value, ok := b.contextProvider.ConstantValue(&callerArg)
		if !ok {
			return nil, false
		}
		if chosen != nil && value != chosenValue {
			return nil, false // call sites disagree
		}
		bound := callerArg
		chosen, chosenValue = &bound, value
	}
	return chosen, chosen != nil
}

// enclosingFuncID names the function a call is written in, preferring the named
// function that defines a closure over the literal itself.
func enclosingFuncID(node TrackerNodeInterface) string {
	edge := node.GetEdge()
	if edge == nil {
		return ""
	}
	if edge.ParentFunction != nil {
		if id := edge.ParentFunction.BaseID(); id != "" {
			return id
		}
	}
	return edge.Caller.BaseID()
}

// metadata returns the metadata behind the context provider, or nil when the
// provider is not the standard implementation (mocks in tests).
func (b *BasePatternMatcher) metadata() *metadata.Metadata {
	if ctxImpl, ok := b.contextProvider.(*ContextProviderImpl); ok {
		return ctxImpl.meta
	}
	return nil
}

// paramArgOfEnclosingFunc resolves a parameter of the function that lexically
// encloses this call, following closures outward.
//
// argViaParent matches the frame whose callee is the call's immediate caller,
// which is exactly right for a wrapper helper and cannot work for a closure: a
// call inside `func(r chi.Router){...}` has the FUNC LITERAL as its caller, and
// no edge calls that literal by name. The parameter it reads (`options`) is
// captured from the function that defines the literal, so the frame to look for
// is the one whose callee is that PARENT function (issue #274).
func paramArgOfEnclosingFunc(arg *metadata.CallArgument, node TrackerNodeInterface) (*metadata.CallArgument, bool) {
	if arg == nil || arg.GetKind() != metadata.KindIdent || node == nil {
		return nil, false
	}
	edge := node.GetEdge()
	if edge == nil || edge.ParentFunction == nil {
		return nil, false
	}
	enclosing := edge.ParentFunction.BaseID()
	if enclosing == "" {
		return nil, false
	}
	p := enclosingFrame(node, enclosing, true)
	if p == nil {
		return nil, false
	}
	if callerArg, ok := p.GetEdge().ParamArgMap[arg.GetName()]; ok {
		return &callerArg, true
	}
	return nil, false
}

// unwrapComposite looks through &T{...} and *T{...} to the literal itself.
func unwrapComposite(arg *metadata.CallArgument) *metadata.CallArgument {
	for arg != nil {
		switch arg.GetKind() {
		case metadata.KindUnary, metadata.KindStar, metadata.KindParen:
			arg = arg.X
		default:
			return arg
		}
	}
	return nil
}

// serveMuxTrailingWildcard matches Go 1.22 ServeMux trailing wildcards
// ({path...}), which OpenAPI cannot express. The capture group keeps the
// parameter name so it can be rewritten to a plain {path} segment.
var serveMuxTrailingWildcard = mustCachedRegex(`\{([a-zA-Z_][a-zA-Z0-9_]*)\.\.\.\}`)

// splitMethodFromPath splits a Go 1.22 ServeMux registration pattern of the
// form "[METHOD ][HOST]/[PATH]" into its method and the remaining path. It
// returns an empty method (and the input unchanged) when no leading HTTP verb
// is present, so plain net/http patterns like "/health" pass through untouched.
func splitMethodFromPath(raw string) (method, path string) {
	raw = strings.Trim(raw, "\"'")
	i := strings.IndexByte(raw, ' ')
	if i <= 0 {
		return "", raw
	}
	candidate := strings.ToUpper(strings.TrimSpace(raw[:i]))
	if !isHTTPMethod(candidate) {
		return "", raw
	}
	return candidate, strings.TrimSpace(raw[i+1:])
}

// normalizeServeMuxPath rewrites ServeMux-specific path syntax into OpenAPI
// path templating: trailing wildcards ({path...}) collapse to {path}, and the
// {$} end-of-path anchor is dropped.
func normalizeServeMuxPath(path string) string {
	path = serveMuxTrailingWildcard.ReplaceAllString(path, "{$1}")
	path = strings.ReplaceAll(path, "{$}", "")
	return path
}

// isHTTPMethod reports whether s is a recognised HTTP method (upper-case).
func isHTTPMethod(s string) bool {
	switch s {
	case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete,
		http.MethodPatch, http.MethodOptions, http.MethodHead, http.MethodConnect, http.MethodTrace:
		return true
	default:
		return false
	}
}

// RoutePatternMatcherImpl implements RoutePatternMatcher
type RoutePatternMatcherImpl struct {
	*BasePatternMatcher
	pattern RoutePattern
}

// NewRoutePatternMatcher creates a new route pattern matcher
func NewRoutePatternMatcher(pattern RoutePattern, cfg *APISpecConfig, contextProvider ContextProvider) *RoutePatternMatcherImpl {
	return &RoutePatternMatcherImpl{
		BasePatternMatcher: NewBasePatternMatcher(cfg, contextProvider),
		pattern:            pattern,
	}
}

// MatchNode checks if a node matches the route pattern
func (r *RoutePatternMatcherImpl) MatchNode(node TrackerNodeInterface) bool {
	if node == nil || node.GetEdge() == nil {
		return false
	}

	edge := node.GetEdge()

	// Where the call is MADE can be as decisive as what is called: an identical
	// registration in another package may not belong in this spec (issue #238).
	if !r.matchCallScope(edge, r.pattern.scope()) {
		return false
	}

	callName := r.contextProvider.GetString(edge.Callee.Name)

	// Check call regex
	if r.pattern.CallRegex != "" && !r.matchPattern(r.pattern.CallRegex, callName) {
		return false
	}

	// Check function name regex
	if r.pattern.FunctionNameRegex != "" {
		funcName := r.contextProvider.GetString(edge.Caller.Name)
		if !r.pattern.MatchFunctionName(funcName) {
			return false
		}
	}

	// Check receiver type. Both the recorded receiver and the one the call
	// was written against are accepted — see recvForms (issue #260).
	if !matchesRecvType(r.contextProvider, &edge.Callee, r.pattern.RecvType, r.pattern.RecvTypeRegex) {
		return false
	}

	return true
}

// GetPattern returns the route pattern
func (r *RoutePatternMatcherImpl) GetPattern() interface{} {
	return r.pattern
}

// GetPriority returns the priority of this pattern
func (r *RoutePatternMatcherImpl) GetPriority() int {
	// More specific patterns have higher priority
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

// ExtractRoute extracts route information from a matched node
func (r *RoutePatternMatcherImpl) ExtractRoute(node TrackerNodeInterface, routeInfo *RouteInfo) bool {
	found := false

	edge := node.GetEdge()
	if routeInfo == nil || routeInfo.File == "" || routeInfo.Package == "" {
		*routeInfo = RouteInfo{
			Method:    http.MethodPost, // Default method
			Package:   r.contextProvider.GetString(edge.Callee.Pkg),
			File:      r.contextProvider.GetString(edge.Position),
			Response:  make(map[string]*ResponseInfo),
			UsedTypes: make(map[string]*Schema),
		}
	}

	if edge != nil {
		routeInfo.Metadata = edge.Callee.Meta
	} else if node.GetArgument() != nil {
		routeInfo.Metadata = node.GetArgument().Meta
	}

	if routeInfo.File == "" && node.GetArgument() != nil {
		routeInfo.File = node.GetArgument().GetPosition()
	}

	found = r.extractRouteDetails(node, routeInfo)

	// Extract handler information
	if hi, ok := r.handlerArgIndex(edge); ok {
		found = true
		handlerArg := handlerArgValue(edge.Args[hi])
		if handlerArg.GetKind() == metadata.KindIdent || handlerArg.GetKind() == metadata.KindFuncLit {

			handlerName := handlerArg.GetName()
			// Use variable tracing to resolve handler
			// The traced origin *type* is deliberately unused: it renders as a
			// type string ("*pkg-->Handler"), which is meaningless as an
			// operation summary in a published spec and — since handlerDoc only
			// fills an empty summary — would suppress the handler's real doc
			// comment (#168).
			originVar, originPkg, _, _ := r.traceVariable(
				handlerName,
				r.contextProvider.GetString(edge.Caller.Name),
				r.contextProvider.GetString(edge.Caller.Pkg),
			)
			if originVar != "" {
				routeInfo.Handler = originVar
			}
			if originPkg != "" {
				routeInfo.Package = originPkg
			}
		}
	}

	return found
}

// extractRouteDetails extracts route details from a node
func (r *RoutePatternMatcherImpl) extractRouteDetails(node TrackerNodeInterface, routeInfo *RouteInfo) bool {
	found := false
	edge := node.GetEdge()

	if r.pattern.MethodFromCall {
		funcName := r.contextProvider.GetString(edge.Callee.Name)
		routeInfo.Method = r.extractMethodFromFunctionNameWithConfig(funcName, r.pattern.MethodExtraction)
		routeInfo.MethodExplicit = true
		found = true
	} else if hi, ok := r.handlerArgIndex(edge); ok && r.pattern.MethodFromHandler {
		// Extract method from handler function name. Only a real mapping hit
		// makes the verb explicit — a DefaultMethod fallback keeps the route
		// open so a `switch r.Method` handler still splits per dispatch verb.
		handlerArg := edge.Args[hi]
		handlerName := r.contextProvider.GetArgumentInfo(handlerArg)
		if handlerName != "" {
			var matched bool
			routeInfo.Method, matched = r.methodFromFunctionName(handlerName, r.pattern.MethodExtraction)
			routeInfo.MethodExplicit = matched
			found = true
		}
	} else if r.pattern.MethodFromArg && len(edge.Args) > r.pattern.MethodArgIndex {
		// The verb travels as an argument, and may name several:
		// `Methods("GET,POST", path, h)` registers both (issue #221). The rest are
		// carried on the route and expanded once the set is collected.
		if methods := r.verbsFromArg(edge.Args[r.pattern.MethodArgIndex]); len(methods) > 0 {
			routeInfo.Method = methods[0]
			routeInfo.ExtraMethods = methods[1:]
			routeInfo.MethodExplicit = true
			found = true
		}
	} else if r.pattern.MethodArgIndex >= 0 && len(edge.Args) > r.pattern.MethodArgIndex {
		methodArg := edge.Args[r.pattern.MethodArgIndex]
		methodValue := methodArg.GetValue()

		// Handle different method extraction patterns
		if methodValue != "" {
			// Clean up method value - remove quotes and extract HTTP method
			cleanMethod := strings.Trim(methodValue, "\"'")

			// Check if it's a valid HTTP method
			if r.isValidHTTPMethod(cleanMethod) {
				routeInfo.Method = strings.ToUpper(cleanMethod)
				routeInfo.MethodExplicit = true
				found = true
			} else {
				// If not a valid method, try to extract from argument info
				argInfo := r.contextProvider.GetArgumentInfo(methodArg)
				if argInfo != "" {
					cleanArgInfo := strings.Trim(argInfo, "\"'")
					if r.isValidHTTPMethod(cleanArgInfo) {
						routeInfo.Method = strings.ToUpper(cleanArgInfo)
						routeInfo.MethodExplicit = true
						found = true
					}
				}
			}
		}
	}

	if r.pattern.PathFromArg && len(edge.Args) > r.pattern.PathArgIndex {
		path, dynNames := r.resolvePathArg(edge.Args[r.pattern.PathArgIndex], node)
		// Go 1.22's net/http.ServeMux carries the HTTP method on the
		// registration pattern itself: mux.HandleFunc("GET /users/{id}", h).
		// When MethodFromPath is set, split the leading verb off the path and
		// normalise ServeMux-specific wildcard syntax ({id...}, {$}).
		if r.pattern.MethodFromPath {
			if method, rest := splitMethodFromPath(path); method != "" {
				routeInfo.Method = method
				routeInfo.MethodExplicit = true
				path = rest
			}
			path = normalizeServeMuxPath(path)
		}
		routeInfo.Path = path
		if routeInfo.Path == "" {
			routeInfo.Path = "/"
		}
		routeInfo.DynamicParams = appendUniqueStrings(routeInfo.DynamicParams, dynNames...)
		found = true
	}

	if hi, ok := r.handlerArgIndex(edge); ok {
		handlerArg := handlerArgValue(edge.Args[hi])
		routeInfo.Handler = r.contextProvider.GetArgumentInfo(handlerArg)
		routeInfo.Function = r.contextProvider.GetArgumentInfo(handlerArg)

		pkg := handlerArg.GetPkg()
		if pkg == "" {
			if node != nil && edge != nil && handlerArg.Fun != nil {
				pkg = handlerArg.Fun.GetPkg()
			}
		}
		routeInfo.Package = pkg
		found = true
	}

	return found
}

// isValidHTTPMethod checks if a string is a valid HTTP method
func (r *RoutePatternMatcherImpl) isValidHTTPMethod(method string) bool {
	validMethods := []string{
		"GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS", "HEAD", "TRACE", "CONNECT",
	}

	upperMethod := strings.ToUpper(method)
	for _, valid := range validMethods {
		if upperMethod == valid {
			return true
		}
	}
	return false
}

// MountPatternMatcherImpl implements MountPatternMatcher
type MountPatternMatcherImpl struct {
	*BasePatternMatcher
	pattern MountPattern
}

// NewMountPatternMatcher creates a new mount pattern matcher
func NewMountPatternMatcher(pattern MountPattern, cfg *APISpecConfig, contextProvider ContextProvider) *MountPatternMatcherImpl {
	return &MountPatternMatcherImpl{
		BasePatternMatcher: NewBasePatternMatcher(cfg, contextProvider),
		pattern:            pattern,
	}
}

// MatchNode checks if a node matches the mount pattern
func (m *MountPatternMatcherImpl) MatchNode(node TrackerNodeInterface) bool {
	if node == nil || node.GetEdge() == nil {
		return false
	}

	edge := node.GetEdge()

	// Where the call is MADE can be as decisive as what is called: an identical
	// registration in another package may not belong in this spec (issue #238).
	if !m.matchCallScope(edge, m.pattern.scope()) {
		return false
	}

	callName := m.contextProvider.GetString(edge.Callee.Name)

	// Check call regex
	if m.pattern.CallRegex != "" && !m.matchPattern(m.pattern.CallRegex, callName) {
		return false
	}

	// Check function name regex
	if m.pattern.FunctionNameRegex != "" {
		funcName := m.contextProvider.GetString(edge.Caller.Name)
		if !m.matchPattern(m.pattern.FunctionNameRegex, funcName) {
			return false
		}
	}

	// Check receiver type. Both the recorded receiver and the one the call
	// was written against are accepted — see recvForms (issue #260).
	if !matchesRecvType(m.contextProvider, &edge.Callee, m.pattern.RecvType, m.pattern.RecvTypeRegex) {
		return false
	}

	// Router-type gate: where the same call registers both routes and mounts,
	// only the argument's type tells them apart (issue #138).
	if m.pattern.RouterArgTypeRegex != "" && !m.routerArgIsRouter(edge) {
		return false
	}

	return m.pattern.IsMount
}

// routerArgIsRouter reports whether the pattern's router argument really holds a
// router, per RouterArgTypeRegex.
//
// A pass-through wrapper is looked through: `http.StripPrefix("/api", api)` has
// the type of its own result (net/http.Handler, which any handler satisfies), so
// judging the outer expression would tell us nothing — the router is the
// argument inside. Only one level is unwrapped, which covers the idiomatic
// shape without inviting a search through arbitrary call nesting.
func (m *MountPatternMatcherImpl) routerArgIsRouter(edge *metadata.CallGraphEdge) bool {
	if m.pattern.RouterArgIndex < 0 || len(edge.Args) <= m.pattern.RouterArgIndex {
		return false
	}
	re, err := cachedRegex(m.pattern.RouterArgTypeRegex)
	if err != nil {
		return false
	}
	arg := edge.Args[m.pattern.RouterArgIndex]
	if arg == nil {
		return false
	}
	if re.MatchString(arg.GetType()) {
		return true
	}
	if arg.GetKind() == metadata.KindCall {
		for _, inner := range arg.Args {
			if inner != nil && re.MatchString(inner.GetType()) {
				return true
			}
		}
	}
	return false
}

// GetPattern returns the mount pattern
func (m *MountPatternMatcherImpl) GetPattern() interface{} {
	return m.pattern
}

// GetPriority returns the priority of this pattern
func (m *MountPatternMatcherImpl) GetPriority() int {
	priority := 0
	if m.pattern.CallRegex != "" {
		priority += 10
	}
	if m.pattern.FunctionNameRegex != "" {
		priority += 5
	}
	if m.pattern.RecvTypeRegex != "" || m.pattern.RecvType != "" {
		priority += 3
	}
	return priority
}

// ExtractMount extracts mount information from a matched node
func (m *MountPatternMatcherImpl) ExtractMount(node TrackerNodeInterface) MountInfo {
	mountInfo := MountInfo{
		Pattern: m.pattern,
	}

	edge := node.GetEdge()
	// Extract path if available
	if m.pattern.PathFromArg && len(edge.Args) > m.pattern.PathArgIndex {
		path, dynNames := m.resolvePathArg(edge.Args[m.pattern.PathArgIndex], node)
		mountInfo.Path = path
		mountInfo.DynamicParams = appendUniqueStrings(mountInfo.DynamicParams, dynNames...)
	}

	// Extract router argument if available
	if m.pattern.RouterArgIndex >= 0 && len(edge.Args) > m.pattern.RouterArgIndex {
		mountInfo.RouterArg = edge.Args[m.pattern.RouterArgIndex]
		// Trace router origin
		m.traceRouterOrigin(mountInfo.RouterArg, node)

		// Find assignment function
		mountInfo.Assignment = m.findAssignmentFunction(mountInfo.RouterArg)
	}

	return mountInfo
}

// SecurityPatternMatcherImpl implements SecurityPatternMatcher
type SecurityPatternMatcherImpl struct {
	*BasePatternMatcher
	pattern SecurityPattern
}

// NewSecurityPatternMatcher creates a new security pattern matcher
func NewSecurityPatternMatcher(pattern SecurityPattern, cfg *APISpecConfig, contextProvider ContextProvider) *SecurityPatternMatcherImpl {
	return &SecurityPatternMatcherImpl{
		BasePatternMatcher: NewBasePatternMatcher(cfg, contextProvider),
		pattern:            pattern,
	}
}

// MatchNode checks if a node matches the security middleware pattern.
func (s *SecurityPatternMatcherImpl) MatchNode(node TrackerNodeInterface) bool {
	if node == nil {
		return false
	}
	return s.MatchEdge(node.GetEdge())
}

// MatchEdge checks if a call-graph edge matches the security middleware pattern.
func (s *SecurityPatternMatcherImpl) MatchEdge(edge *metadata.CallGraphEdge) bool {
	if edge == nil {
		return false
	}

	// Where the call is MADE can be as decisive as what is called: an identical
	// registration in another package may not belong in this spec (issue #238).
	if !s.matchCallScope(edge, s.pattern.scope()) {
		return false
	}

	callName := s.contextProvider.GetString(edge.Callee.Name)

	if s.pattern.CallRegex != "" && !s.matchPattern(s.pattern.CallRegex, callName) {
		return false
	}

	if s.pattern.FunctionNameRegex != "" {
		funcName := s.contextProvider.GetString(edge.Caller.Name)
		if !s.matchPattern(s.pattern.FunctionNameRegex, funcName) {
			return false
		}
	}

	// Check receiver type. Both the recorded receiver and the one the call
	// was written against are accepted — see recvForms (issue #260).
	if !matchesRecvType(s.contextProvider, &edge.Callee, s.pattern.RecvType, s.pattern.RecvTypeRegex) {
		return false
	}

	return true
}

// GetPattern returns the security pattern
func (s *SecurityPatternMatcherImpl) GetPattern() interface{} {
	return s.pattern
}

// GetPriority returns the priority of this pattern
func (s *SecurityPatternMatcherImpl) GetPriority() int {
	priority := 0
	if s.pattern.CallRegex != "" {
		priority += 10
	}
	if s.pattern.FunctionNameRegex != "" {
		priority += 5
	}
	if s.pattern.RecvTypeRegex != "" || s.pattern.RecvType != "" {
		priority += 3
	}
	return priority
}

// Scope returns the scope over which the matched middleware applies.
func (s *SecurityPatternMatcherImpl) Scope() string {
	return s.pattern.Scope
}

// ExtractMiddleware resolves the identity of each middleware value applied by
// the matched call.
//
// For wrapper scope the "middleware" is the function wrapping the handler
// argument (e.g. mux.Handle("/x", Auth(h))): the handler arg is itself a call
// whose Fun is the auth wrapper. For the other scopes the middleware values are
// taken from the call's args starting at MiddlewareArgIndex (a single arg, or
// all remaining args when MiddlewareVariadic is set).
func (s *SecurityPatternMatcherImpl) ExtractMiddleware(node TrackerNodeInterface) []MiddlewareRef {
	if node == nil {
		return nil
	}
	return s.ExtractMiddlewareFromEdge(node.GetEdge())
}

// ExtractMiddlewareFromEdge is the edge-level form of ExtractMiddleware.
func (s *SecurityPatternMatcherImpl) ExtractMiddlewareFromEdge(edge *metadata.CallGraphEdge) []MiddlewareRef {
	if edge == nil {
		return nil
	}
	var refs []MiddlewareRef

	if s.pattern.Scope == SecurityScopeWrapper {
		idx := s.pattern.HandlerArgIndex
		if idx >= 0 && idx < len(edge.Args) {
			// Only a wrapping call (e.g. Auth(h)) is middleware; a bare handler
			// ident/func-lit is the handler itself, not auth.
			if h := edge.Args[idx]; h.GetKind() == metadata.KindCall {
				if ref, ok := middlewareRefFromArg(h); ok {
					refs = append(refs, ref)
				}
			}
		}
		return refs
	}

	start := s.pattern.MiddlewareArgIndex
	if start < 0 {
		start = 0
	}
	end := start + 1
	if s.pattern.MiddlewareVariadic {
		end = len(edge.Args)
		// gin/fiber put the handler as the final variadic arg; exclude it so it
		// is not mistaken for middleware.
		if s.pattern.MiddlewareExcludeLast && end > start {
			end--
		}
	}
	var meta *metadata.Metadata
	if ctxImpl, ok := s.contextProvider.(*ContextProviderImpl); ok {
		meta = ctxImpl.meta
	}
	for i := start; i < end && i < len(edge.Args); i++ {
		arg := edge.Args[i]
		// A middleware passed as a local variable (mw := pkg.New(...)) resolves to
		// the underlying constructor so look-through / mappings can match it.
		if ref, ok := resolveMiddlewareIdentRef(edge, arg, meta); ok {
			refs = append(refs, ref)
			continue
		}
		if ref, ok := middlewareRefFromArg(arg); ok {
			refs = append(refs, ref)
		}
	}
	return refs
}

// RequestPatternMatcherImpl implements RequestPatternMatcher
type RequestPatternMatcherImpl struct {
	*BasePatternMatcher
	pattern      RequestBodyPattern
	bodyResolver *bodySourceResolver
}

// NewRequestPatternMatcher creates a new request pattern matcher
func NewRequestPatternMatcher(pattern RequestBodyPattern, cfg *APISpecConfig, contextProvider ContextProvider) *RequestPatternMatcherImpl {
	return &RequestPatternMatcherImpl{
		BasePatternMatcher: NewBasePatternMatcher(cfg, contextProvider),
		pattern:            pattern,
		bodyResolver:       newBodySourceResolver(cfg, contextProvider),
	}
}

// MatchNode checks if a node matches the request pattern
func (r *RequestPatternMatcherImpl) MatchNode(node TrackerNodeInterface) bool {
	if node == nil || node.GetEdge() == nil {
		return false
	}

	edge := node.GetEdge()

	// Where the call is MADE can be as decisive as what is called: an identical
	// registration in another package may not belong in this spec (issue #238).
	if !r.matchCallScope(edge, r.pattern.scope()) {
		return false
	}

	callName := r.contextProvider.GetString(edge.Callee.Name)

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

	// Check receiver type. Both the recorded receiver and the one the call
	// was written against are accepted — see recvForms (issue #260).
	if !matchesRecvType(r.contextProvider, &edge.Callee, r.pattern.RecvType, r.pattern.RecvTypeRegex) {
		return false
	}

	// NOTE: body-source verification (RequireRequestSource) is intentionally NOT
	// done here. MatchNode is memoized per edge, but an ambiguous decoder's
	// source is per-route — the same helper node decodes r.Body in one route and
	// a non-request reader in another. The gate lives in ExtractRequest
	// (per-route), where the source is resolved through the call graph to its
	// concrete value. Mirror of the response-destination gating (issue #170).

	return true
}

// bodySource returns the decoder's input source for the given node, resolved
// per-route through the call graph to its concrete value, plus the tracker edge
// in whose scope that value is checked. For json.NewDecoder(x).Decode(v) the
// raw source is the factory's first argument x; when x is a wrapper parameter
// (`func decodeFrom(src io.Reader, v)`) it is followed to the caller's actual
// argument at this route's call site, so the same helper resolves to r.Body for
// decodeFrom(r.Body, &v) and to a non-request reader otherwise.
func (r *RequestPatternMatcherImpl) bodySource(node TrackerNodeInterface) (*metadata.CallArgument, *metadata.CallGraphEdge) {
	if node == nil {
		return nil, nil
	}
	edge := node.GetEdge()
	if edge == nil {
		return nil, nil
	}
	var src *metadata.CallArgument
	if r.pattern.BodyFromReceiver {
		src = resolveReceiverSource(edge, r.bodyResolver.metadata())
	} else {
		idx := r.pattern.BodySourceArgIndex
		if idx < 0 || idx >= len(edge.Args) {
			return nil, edge
		}
		src = edge.Args[idx]
	}
	if src == nil {
		return nil, edge
	}
	resolved, resolvedNode := resolveArgThroughParams(src, node)
	srcEdge := edge
	if resolvedNode != nil && resolvedNode.GetEdge() != nil {
		srcEdge = resolvedNode.GetEdge()
	}
	return resolved, srcEdge
}

// GetPattern returns the request pattern
func (r *RequestPatternMatcherImpl) GetPattern() interface{} {
	return r.pattern
}

// GetPriority returns the priority of this pattern
func (r *RequestPatternMatcherImpl) GetPriority() int {
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

// ExtractRequest extracts request information from a matched node
func (r *RequestPatternMatcherImpl) ExtractRequest(node TrackerNodeInterface, route *RouteInfo) *RequestInfo {
	// Body-source verification, done here — NOT in MatchNode, which is memoized
	// per edge — because an ambiguous decoder's source is per-route. Resolve the
	// source through the call graph to its concrete value at THIS route's call
	// site and drop the body when it does not trace to an HTTP request. This
	// recognises a decoder wrapped in a `func decodeFrom(src io.Reader, v)`
	// helper called with r.Body (fixing the io.Reader-helper false negative)
	// without the shared-node false positive of a per-edge gate. Mirror of the
	// response-destination gating (issue #170).
	if r.pattern.RequireRequestSource && r.bodyResolver != nil && r.bodyResolver.Enabled() {
		src, srcEdge := r.bodySource(node)
		if src == nil || !r.bodyResolver.IsRequestSource(src, srcEdge) {
			return nil
		}
	}

	reqInfo := &RequestInfo{
		ContentType: r.cfg.Defaults.RequestContentType,
	}

	edge := node.GetEdge()
	if r.pattern.TypeFromArg && len(edge.Args) > r.pattern.TypeArgIndex {
		arg := edge.Args[r.pattern.TypeArgIndex]
		// Parameter tracing through a binding wrapper: when the bound value is a
		// parameter of the enclosing function (e.g. Bind(v) inside a custom
		// ReadRequest(c, v) wrapper), follow it up to the caller's actual
		// argument so the concrete request type is recovered instead of the
		// wrapper's `interface{}` parameter. Per-route tracker isolation makes
		// this sound: each route resolves its own call-site value.
		typeNode := node
		if resolved, rnode := resolveArgThroughParams(arg, node); resolved != arg && rnode != nil {
			arg = resolved
			typeNode = rnode
		}
		bodyType := r.contextProvider.GetArgumentInfo(arg)
		// Concrete types assigned to an interface-typed body, when there is more
		// than one — see oneOfSchemaFor (issue #201).
		var oneOfTypes []string

		// Check if this is a literal value - if so, determine appropriate type
		if arg.GetKind() == metadata.KindLiteral {
			bodyType = determineLiteralType(bodyType)
		} else {
			// Call-expression body args (e.g. helper(r) decoded into a
			// schema, or err.Error() rendered as a response) carry their
			// *return* type on the CallArgument — see handleCallExpr.
			// Prefer it over the stringified call, which would otherwise
			// produce an unresolvable name like "pkg.Method".
			if arg.GetKind() == metadata.KindCall {
				if t := arg.GetType(); t != "" {
					bodyType = t
				}
			}

			// Check for resolved type information in the CallArgument
			if resolvedType := arg.GetResolvedType(); resolvedType != "" {
				bodyType = resolvedType
			} else if arg.IsGenericType && arg.GenericTypeName != -1 {
				// If it's a generic type, try to resolve it from the edge's type parameters
				if concreteType, exists := node.GetTypeParamMap()[arg.GetGenericTypeName()]; exists {
					bodyType = concreteType
				}
			}

			// Trace type origin (in the scope the arg was resolved into, so a
			// wrapper-passed local's assignment is found at the call site).
			resolved := r.resolveTypeOrigin(arg, typeNode, bodyType)
			if resolved == bodyType {
				// Unchanged means the type did not narrow — when that is because
				// several concrete types are assigned, the body is polymorphic
				// and `oneOf` says so (issue #201).
				oneOfTypes = r.ambiguousConcreteSet(arg, typeNode, bodyType)
			}
			bodyType = resolved

			// Apply dereferencing if needed
			if r.pattern.Deref && strings.HasPrefix(bodyType, "*") {
				bodyType = strings.TrimPrefix(bodyType, "*")
			}
		}

		// Fold a generic instantiation into the internal form so a generic
		// request body keys to the same clean component as the equivalent
		// response body (no duplicate schema). Mirrors the response matcher.
		bodyType = normalizeGenericInstanceName(bodyType)

		reqInfo.BodyType = preprocessingBodyType(bodyType)
		// Build the polymorphic schema FIRST and skip the single-type mapping
		// when it applies: mapping the bare interface would register it as a
		// component that nothing then references, leaving an orphan
		// `{type: object}` in the output.
		schema := oneOfSchemaFor(route.UsedTypes, oneOfTypes, route.Metadata, r.cfg)
		if schema != nil {
			reqInfo.OneOfTypes = oneOfTypes
		} else {
			schema = mapGoTypeForRoute(route.UsedTypes, bodyType, route.Metadata, r.cfg)
		}
		reqInfo.Schema = schema
	}

	if reqInfo.BodyType == "" {
		return nil
	}

	return reqInfo
}

// Helper methods for BasePatternMatcher
func (b *BasePatternMatcher) matchPattern(pattern, value string) bool {
	if pattern == "" {
		return false
	}
	return cachedMatch(pattern, value)
}

func (b *BasePatternMatcher) traceVariable(varName, funcName, pkgName string) (originVar, originPkg string, originType *metadata.CallArgument, originFunc string) {
	ctxImpl, ok := b.contextProvider.(*ContextProviderImpl)
	if !ok || ctxImpl.meta == nil {
		return varName, pkgName, nil, originFunc
	}
	originVar, originPkg, originType, originFunc = metadata.TraceVariableOrigin(varName, funcName, pkgName, ctxImpl.meta)
	return originVar, originPkg, originType, originFunc
}

func (b *BasePatternMatcher) traceRouterOrigin(routerArg *metadata.CallArgument, node TrackerNodeInterface) {
	// Trace router origin based on argument kind
	edge := node.GetEdge()
	switch routerArg.GetKind() {
	case metadata.KindIdent:
		b.traceVariable(
			routerArg.GetName(),
			b.contextProvider.GetString(edge.Caller.Name),
			b.contextProvider.GetString(edge.Caller.Pkg),
		)
	case metadata.KindUnary, metadata.KindStar:
		if routerArg.X != nil {
			b.traceVariable(
				routerArg.X.GetName(),
				b.contextProvider.GetString(edge.Caller.Name),
				b.contextProvider.GetString(edge.Caller.Pkg),
			)
		}
	case metadata.KindSelector:
		if routerArg.X != nil {
			b.traceVariable(
				routerArg.X.GetName(),
				b.contextProvider.GetString(edge.Caller.Name),
				b.contextProvider.GetString(edge.Caller.Pkg),
			)
		}
	case metadata.KindCall:
		if routerArg.Fun != nil {
			b.traceVariable(
				routerArg.Fun.GetName(),
				b.contextProvider.GetString(edge.Caller.Name),
				b.contextProvider.GetString(edge.Caller.Pkg),
			)
		}
	}
}

func (b *BasePatternMatcher) findAssignmentFunction(arg *metadata.CallArgument) *metadata.CallArgument {
	// Use contextProvider to access metadata
	ctxImpl, ok := b.contextProvider.(*ContextProviderImpl)
	if !ok || ctxImpl.meta == nil {
		return nil
	}
	meta := ctxImpl.meta

	for _, edge := range meta.CallGraph {
		for _, varAssignments := range edge.AssignmentMap {
			for _, assign := range varAssignments {
				varName := b.contextProvider.GetString(assign.VariableName)
				varType := b.contextProvider.GetString(assign.ConcreteType)
				varPkg := b.contextProvider.GetString(assign.Pkg)

				if varName == arg.GetName() && varPkg == arg.GetPkg() && arg.X != nil && arg.X.Type != -1 && varType == arg.X.GetType() {
					// Get the function name directly (it's already a string)
					for _, targetArg := range edge.Args {
						if targetArg.GetKind() == metadata.KindCall && targetArg.Fun != nil {
							return targetArg.Fun
						}
					}
				}
			}
		}
	}
	return nil
}

// resolveTypeOrigin traces the origin of a type through assignments and type parameters
func (r *RequestPatternMatcherImpl) resolveTypeOrigin(arg *metadata.CallArgument, node TrackerNodeInterface, originalType string) string {
	// NEW: If the argument has resolved type information, use it
	if resolvedType := arg.GetResolvedType(); resolvedType != "" {
		return resolvedType
	}

	// If it's a generic type with a concrete resolution, use it
	if core := typemodel.Parse(originalType).Core(); core != nil {
		if genericType := traceGenericOrigin(node, core.Name); genericType != "" {
			return genericType
		}
	}

	// Selector expression — resolve via metadata field lookup.
	if arg.GetKind() == metadata.KindSelector {
		if t := resolveSelectorFieldType(arg, r.contextProvider); t != "" {
			return t
		}
	}

	// A request body is decoded into a POINTER (`Decode(&v)`), so the variable
	// is the unary's operand — unlike the response path, which encodes the value
	// itself. Peel it, or the ident-keyed resolution below never fires (this is
	// why interface-typed request bodies stayed unresolved, issue #164).
	target := arg
	if target.GetKind() == metadata.KindUnary && target.X != nil {
		target = target.X
	}

	// Original logic for type resolution
	if target.GetKind() == metadata.KindIdent || target.GetKind() == metadata.KindFuncLit {
		// Check if this variable has assignments that might give us more type information
		edge := node.GetEdge()
		if assignments, exists := edge.AssignmentMap[target.GetName()]; exists {
			for _, assignment := range assignments {
				if assignment.ConcreteType != 0 {
					concreteType := r.contextProvider.GetString(assignment.ConcreteType)
					if concreteType != "" {
						return concreteType
					}
				}
			}
		}
		// Interface-typed body: the concrete value is assigned in the enclosing
		// handler (`var a Animal = Dog{}; Decode(&a)`), not on this call's edge.
		// Resolve it so the schema documents Dog rather than the empty Animal
		// interface — the same resolution the response path already applies
		// (issue #164). An ambiguous set of assignments keeps the interface.
		if concrete := r.concreteFromEnclosingFunc(target, edge, originalType); concrete != "" {
			return concrete
		}
		// Interface-typed function parameter: the concrete value is bound at the
		// call site that entered the enclosing function (`decodeAnimal(r, Dog{})`).
		if concrete := r.concreteFromParamBinding(target, node, originalType); concrete != "" {
			return concrete
		}
	}

	return originalType
}

// traceGenericOrigin resolves a type-parameter name (the core name of the
// traced type, e.g. "T") through the node's type-parameter map to its
// concrete instantiation, following chained mappings.
func traceGenericOrigin(node TrackerNodeInterface, typeName string) string {
	typeParams := node.GetTypeParamMap()

	if len(typeParams) > 0 && typeName != "" {
		searchType := typeName
		foundMapping := false

		for {
			concreteType, exists := typeParams[searchType]
			if !exists || concreteType == "" {
				break
			}
			searchType = concreteType
			foundMapping = true
		}
		// Only return the concrete type if we found a mapping
		if foundMapping {
			return searchType
		}
	}
	return ""
}

func (b *BasePatternMatcher) extractMethodFromFunctionNameWithConfig(funcName string, config *MethodExtractionConfig) string {
	method, _ := b.methodFromFunctionName(funcName, config)
	return method
}

// methodFromFunctionName additionally reports whether a mapping word actually
// matched. A false report means the returned method is config.DefaultMethod
// (or empty) — a fallback, not evidence from the name — so callers can leave
// the route non-explicit and let method-dispatch splitting refine it.
func (b *BasePatternMatcher) methodFromFunctionName(funcName string, config *MethodExtractionConfig) (string, bool) {
	if funcName == "" {
		return "", false
	}

	// Use default config if none provided
	if config == nil {
		config = DefaultMethodExtractionConfig()
	}

	// Split the identifier into words at camelCase boundaries and
	// non-letter separators so a pattern only ever matches a whole word:
	// "deleteWidget" is [delete widget], and the "get" inside "widget" must
	// not match. The old substring checks lowercased the name first, which
	// erased the camel boundary and let GET (checked first) claim any name
	// containing those three letters.
	words := splitNameWords(funcName)
	if !config.CaseSensitive {
		for i, w := range words {
			words[i] = strings.ToLower(w)
		}
	}

	// Sort mappings by priority (highest first)
	mappings := make([]MethodMapping, len(config.MethodMappings))
	copy(mappings, config.MethodMappings)

	// Simple bubble sort by priority (descending)
	for i := 0; i < len(mappings)-1; i++ {
		for j := 0; j < len(mappings)-i-1; j++ {
			if mappings[j].Priority < mappings[j+1].Priority {
				mappings[j], mappings[j+1] = mappings[j+1], mappings[j]
			}
		}
	}

	patternWord := func(pattern string) string {
		if !config.CaseSensitive {
			return strings.ToLower(pattern)
		}
		return pattern
	}

	// Check the leading word first if enabled ("deleteUser" → DELETE).
	if config.UsePrefix && len(words) > 0 {
		for _, mapping := range mappings {
			for _, pattern := range mapping.Patterns {
				if words[0] == patternWord(pattern) {
					return mapping.Method, true
				}
			}
		}
	}

	// Then any whole word if enabled ("handleUserDelete" → DELETE).
	if config.UseContains {
		for _, mapping := range mappings {
			for _, pattern := range mapping.Patterns {
				p := patternWord(pattern)
				for _, w := range words {
					if w == p {
						return mapping.Method, true
					}
				}
			}
		}
	}

	return config.DefaultMethod, false
}

// splitNameWords splits an identifier into words at non-letter separators
// and camelCase boundaries: "handleHTTPDelete" → [handle HTTP Delete],
// "delete_widget" → [delete widget]. An uppercase run followed by a
// lowercase letter starts a new word (the "HTTPServer" → HTTP+Server rule).
func splitNameWords(name string) []string {
	var words []string
	var cur []rune
	runes := []rune(name)
	flush := func() {
		if len(cur) > 0 {
			words = append(words, string(cur))
			cur = nil
		}
	}
	for i, r := range runes {
		if !unicode.IsLetter(r) {
			flush()
			continue
		}
		if len(cur) > 0 && unicode.IsUpper(r) {
			prev := cur[len(cur)-1]
			if unicode.IsLower(prev) ||
				(unicode.IsUpper(prev) && i+1 < len(runes) && unicode.IsLower(runes[i+1])) {
				flush()
			}
		}
		cur = append(cur, r)
	}
	flush()
	return words
}

// verbsFromArg reads the HTTP verbs an argument names, in order, or nil.
//
// A registrar can name several in one call — gitea's `Methods("GET,POST", …)`
// registers both — so the result is a list. Every element has to be a real verb:
// a value that is only partly verb-like ("GET,everything") says the argument was
// not understood, and inventing a route from half of it would be worse than
// leaving the route with its default (golden rule #7).
//
// The value may be a literal or a constant this project declares
// (`http.MethodGet` is not resolvable — that package is not analysed — and a
// pattern relying on it keeps the older single-verb fallback path).
func (r *RoutePatternMatcherImpl) verbsFromArg(arg *metadata.CallArgument) []string {
	if arg == nil {
		return nil
	}
	raw, ok := r.contextProvider.ConstantValue(arg)
	if !ok || raw == "" {
		return nil
	}

	parts := strings.Split(raw, ",")
	methods := make([]string, 0, len(parts))
	seen := make(map[string]bool, len(parts))
	for _, part := range parts {
		verb := strings.ToUpper(strings.TrimSpace(strings.Trim(part, "\"'")))
		if verb == "" || !r.isValidHTTPMethod(verb) {
			return nil
		}
		if !seen[verb] {
			seen[verb] = true
			methods = append(methods, verb)
		}
	}
	return methods
}

// handlerArgValue returns the value a handler argument really names, looking
// through a type conversion.
//
// `r.Get("/x", http.HandlerFunc(listItems))` passes a conversion, and taking it at
// face value documented the operation as `net/http.HandlerFunc` with no body —
// the conversion names a type, and a type has no doc comment, no request and no
// responses. The handler is what was converted (issue #221).
//
// Only a conversion wrapping something that can BE a handler is peeled: an ident,
// a selector (`pkg.Handler`, `h.ServeHTTP`) or a literal function. Anything else
// stays as it is, since for a non-handler argument the conversion's own type is
// usually the answer (a `[]byte(body)` request body, for instance).
// handlerArgIndex locates the handler argument of a registration call, or
// reports false when this pattern does not read one or the call is too short.
// It is the single place the variadic-chain shape is resolved, so the route's
// identity, its verb-from-handler-name and its details all read the same
// argument (issue #386).
func (r *RoutePatternMatcherImpl) handlerArgIndex(edge *metadata.CallGraphEdge) (int, bool) {
	if !r.pattern.HandlerFromArg || edge == nil {
		return 0, false
	}
	return r.pattern.HandlerArgIndexFor(len(edge.Args))
}

func handlerArgValue(arg *metadata.CallArgument) *metadata.CallArgument {
	// Bounded because the peel is driven by the code's own nesting: a chain
	// deeper than this is not a wiring style, and a bound keeps a cyclic or
	// pathological argument tree from spinning here.
	for i := 0; i < maxHandlerUnwraps && arg != nil; i++ {
		switch arg.GetKind() {
		case metadata.KindTypeConversion:
			inner := convertedHandler(arg)
			if inner == nil {
				return arg
			}
			arg = inner
		case metadata.KindCall:
			inner := wrappedHandler(arg)
			if inner == nil {
				return arg
			}
			arg = inner
		default:
			return arg
		}
	}
	return arg
}

// maxHandlerUnwraps bounds how many wrapper layers handlerArgValue peels.
const maxHandlerUnwraps = 8

// convertedHandler returns what a type conversion converts, when that is
// something that can BE a handler. Anything else stays as it is, since for a
// non-handler argument the conversion's own type is usually the answer (a
// `[]byte(body)` request body, for instance).
func convertedHandler(arg *metadata.CallArgument) *metadata.CallArgument {
	if arg == nil || len(arg.Args) != 1 {
		return nil
	}
	switch inner := arg.Args[0]; inner.GetKind() {
	case metadata.KindIdent, metadata.KindSelector, metadata.KindFuncLit:
		return inner
	default:
		return nil
	}
}

// wrappedHandler returns the handler a middleware call wraps, or nil when the
// call is not wrapping one (issue #364).
//
// `mux.Handle(p, withLogging(http.HandlerFunc(getItem)))` passes a CALL at the
// handler position, and taking it at face value documents the operation as the
// MIDDLEWARE: its name becomes the operationId, its doc comment the summary, its
// header reads the parameters, and the handler's own request body and responses
// are missing entirely.
//
// What identifies a wrapper is the shape at this position, not the framework: a
// call one of whose arguments is itself a handler — an ident or selector naming a
// function apispec knows, a function literal, or a conversion of one. That covers
// `func(http.Handler) http.Handler`, `func(http.HandlerFunc) http.HandlerFunc` and
// every framework's equivalent, plus adapters (`gin.WrapH(mw(h))`), because the
// caller peels repeatedly.
//
// Two shapes deliberately do not peel:
//
//   - a handler FACTORY (`h.Create()`) — no argument is a handler, so the call
//     itself remains the answer, which is what issue #221's factory support needs;
//   - a call with SEVERAL handler-shaped arguments — which of them serves the
//     route is genuinely ambiguous, and today's behaviour with a warning-free
//     wrong answer is better than a guess (golden rule #7).
func wrappedHandler(call *metadata.CallArgument) *metadata.CallArgument {
	return wrappedHandlerDepth(call, 0)
}

func wrappedHandlerDepth(call *metadata.CallArgument, depth int) *metadata.CallArgument {
	if call == nil || len(call.Args) == 0 || depth >= maxHandlerUnwraps {
		return nil
	}
	// The callee's own shape settles the plain-ident case: `withTiming(getItem)`
	// carries no conversion to say which argument is the handler, so the wrapper
	// has to identify itself — a function that takes and returns the same type is
	// a middleware in any framework.
	shaped := middlewareShaped(call)
	var found *metadata.CallArgument
	for _, arg := range call.Args {
		candidate := arg
		switch arg.GetKind() {
		case metadata.KindTypeConversion:
			// A conversion of a function AT the handler position is the handler,
			// whatever the enclosing call is — `http.HandlerFunc(h)` says so
			// outright. This is what carries the adapters whose own signature is
			// not middleware-shaped (`gin.WrapH`, `echo.WrapHandler`).
			inner := convertedHandler(arg)
			if inner == nil || !namesAFunction(inner) {
				continue
			}
			candidate = inner
		case metadata.KindCall:
			// A nested wrapper (`withAuth(withLogging(http.HandlerFunc(h)))`):
			// the argument is itself wrapping a handler, so it is the handler for
			// this level. The caller peels it on the next pass.
			if wrappedHandlerDepth(arg, depth+1) == nil {
				continue
			}
		default:
			if !shaped || !namesAFunction(candidate) {
				continue
			}
		}
		if found != nil {
			return nil // ambiguous: more than one handler-shaped argument
		}
		found = candidate
	}
	return found
}

// middlewareShaped reports whether a call's callee takes and returns the same
// type — the middleware shape in every framework (`func(http.Handler)
// http.Handler`, `func(echo.HandlerFunc) echo.HandlerFunc`, and a
// `Chain.Then(http.Handler) http.Handler` helper alike).
//
// It exists to tell a middleware from an ADAPTER that also takes a function:
// `HandleRequest(handleCreateUser)` turns a `func(Req) (Resp, error)` into an
// http.HandlerFunc, so the function it is given is business logic reached
// through the adapter, not the HTTP handler the route registers — and the
// operation is the adapter instantiation, which is what apispec documents today
// (testdata/generic). Peeling it would rename those operations, which is a
// separate decision from this one (issue #367).
//
// Compared as structured type refs, not as rendered strings (golden rule #2):
// the parameter records its type as WRITTEN (`Handler`) while the result records
// it RESOLVED (`http.Handler`), so the names are what must match, and a package
// is compared only when both refs carry one.
func middlewareShaped(call *metadata.CallArgument) bool {
	fn := calleeFunctionOf(call)
	if fn == nil || len(fn.Signature.Args) != 1 || call.Meta == nil {
		return false
	}
	meta := call.Meta
	param := baseTypeRef(meta.TypeRefOf(fn.Signature.Args[0].Type))
	result := baseTypeRef(meta.TypeRefOf(fn.Signature.ResolvedType))
	if param == nil || result == nil || param.Name == "" {
		return false
	}
	if param.Pkg != "" && result.Pkg != "" && param.Pkg != result.Pkg {
		return false
	}
	return param.Name == result.Name
}

// baseTypeRef unwraps pointers, so a `*Handler` parameter and a `Handler` result
// are the same type for this comparison.
func baseTypeRef(ref *typemodel.TypeRef) *typemodel.TypeRef {
	for ref != nil && ref.Kind == typemodel.KindPointer && ref.Elem != nil {
		ref = ref.Elem
	}
	return ref
}

// calleeFunctionOf resolves the function a call invokes, by its own edge when the
// call graph recorded one, else by the name its Fun carries.
func calleeFunctionOf(call *metadata.CallArgument) *metadata.Function {
	if call == nil || call.Meta == nil {
		return nil
	}
	meta := call.Meta
	if call.Edge != nil {
		pkg := meta.StringPool.GetString(call.Edge.Callee.Pkg)
		name := meta.StringPool.GetString(call.Edge.Callee.Name)
		recv := strings.TrimPrefix(meta.StringPool.GetString(call.Edge.Callee.RecvType), "*")
		recv = strings.TrimPrefix(recv, pkg+".")
		if fn := findFunctionByName(meta, pkg, name); fn != nil {
			return fn
		}
		if m := findMethodByName(meta, pkg, recv, name); m != nil {
			return &metadata.Function{Name: m.Name, Signature: m.Signature}
		}
		return nil
	}
	fun := call.Fun
	if fun == nil {
		return nil
	}
	recv := ""
	if fun.GetKind() == metadata.KindSelector && fun.Sel != nil {
		if fun.X != nil {
			recv = strings.TrimPrefix(fun.X.GetType(), "*")
		}
		fun = fun.Sel
	}
	pkg, name := fun.GetPkg(), fun.GetName()
	if f := findFunctionByName(meta, pkg, name); f != nil {
		return f
	}
	// A wrapper reached as a METHOD with no recorded edge — `chain.Then(h)`,
	// the alice-style helper. Without this it resolves to nothing, so its
	// middleware shape is unknown and a plain-ident handler is not peeled.
	recv = strings.TrimPrefix(recv, pkg+".")
	if m := findMethodByName(meta, pkg, recv, name); m != nil {
		return &metadata.Function{Name: m.Name, Signature: m.Signature}
	}
	return nil
}

// namesAFunction reports whether an argument refers to a function body — a
// function literal, or an ident/selector that resolves to a declared function or
// method. It is what keeps wrappedHandler from peeling an ordinary argument: in
// `h.Create(db)` the `db` ident names a value, not a function, so the factory
// call stays the handler.
func namesAFunction(arg *metadata.CallArgument) bool {
	if arg == nil {
		return false
	}
	if arg.GetKind() == metadata.KindFuncLit {
		return true
	}
	meta := arg.Meta
	if meta == nil {
		return false
	}
	switch arg.GetKind() {
	case metadata.KindIdent:
		return findFunctionByName(meta, arg.GetPkg(), arg.GetName()) != nil
	case metadata.KindSelector:
		if arg.Sel == nil {
			return false
		}
		pkg, name := arg.Sel.GetPkg(), arg.Sel.GetName()
		if findFunctionByName(meta, pkg, name) != nil {
			return true
		}
		recv := ""
		if arg.X != nil {
			recv = strings.TrimPrefix(arg.X.GetType(), "*")
			recv = strings.TrimPrefix(recv, pkg+".")
		}
		return findMethodByName(meta, pkg, recv, name) != nil
	}
	return false
}
