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
	typeResolver    TypeResolver
}

// NewBasePatternMatcher creates a new base pattern matcher
func NewBasePatternMatcher(cfg *APISpecConfig, contextProvider ContextProvider, typeResolver TypeResolver) *BasePatternMatcher {
	return &BasePatternMatcher{
		contextProvider: contextProvider,
		cfg:             cfg,
		schemaMapper:    NewSchemaMapper(cfg),
		typeResolver:    typeResolver,
	}
}

// resolvePathArg renders a CallArgument as an OpenAPI path string.
//
// Literals and const idents resolve to their value via the context
// provider. Function-call expressions (e.g. r.Mount(mountPoint(prefix,
// "/api"), sub)) cannot be statically evaluated without interpreting
// the Go body — see issue #34 — so they surface as a {placeholder}
// named after the called function. The second return value, dynamicName,
// is the placeholder name when one was synthesized (so the caller can
// register a shared component parameter) and the empty string otherwise.
//
// All other kinds fall through to GetArgumentInfo for backwards
// compatibility — handling KindIdent (non-const variable) and
// KindBinary (`prefix + "/x"`) similarly is a possible follow-up but
// is out of scope for the initial fix.
func (b *BasePatternMatcher) resolvePathArg(arg *metadata.CallArgument) (path, dynamicName string) {
	if arg == nil {
		return "", ""
	}
	if arg.GetKind() == metadata.KindCall {
		name := arg.GetName()
		if name == "" && arg.Fun != nil {
			name = arg.Fun.GetName()
		}
		if name == "" {
			name = "path"
		}
		return "{" + name + "}", name
	}
	return b.contextProvider.GetArgumentInfo(arg), ""
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
func NewRoutePatternMatcher(pattern RoutePattern, cfg *APISpecConfig, contextProvider ContextProvider, typeResolver TypeResolver) *RoutePatternMatcherImpl {
	return &RoutePatternMatcherImpl{
		BasePatternMatcher: NewBasePatternMatcher(cfg, contextProvider, typeResolver),
		pattern:            pattern,
	}
}

// MatchNode checks if a node matches the route pattern
func (r *RoutePatternMatcherImpl) MatchNode(node TrackerNodeInterface) bool {
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
		if !r.pattern.MatchFunctionName(funcName) {
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
	if r.pattern.HandlerFromArg && len(edge.Args) > r.pattern.HandlerArgIndex {
		found = true
		handlerArg := edge.Args[r.pattern.HandlerArgIndex]
		if handlerArg.GetKind() == metadata.KindIdent || handlerArg.GetKind() == metadata.KindFuncLit {

			handlerName := handlerArg.GetName()
			// Use variable tracing to resolve handler
			originVar, originPkg, originType, _ := r.traceVariable(
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

			var originTypeStr string
			if originType != nil {
				originTypeStr = r.contextProvider.GetArgumentInfo(originType)
			}
			if originTypeStr != "" {
				routeInfo.Summary = originTypeStr
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
	} else if r.pattern.MethodFromHandler && r.pattern.HandlerFromArg && len(edge.Args) > r.pattern.HandlerArgIndex {
		// Extract method from handler function name. Only a real mapping hit
		// makes the verb explicit — a DefaultMethod fallback keeps the route
		// open so a `switch r.Method` handler still splits per dispatch verb.
		handlerArg := edge.Args[r.pattern.HandlerArgIndex]
		handlerName := r.contextProvider.GetArgumentInfo(handlerArg)
		if handlerName != "" {
			var matched bool
			routeInfo.Method, matched = r.methodFromFunctionName(handlerName, r.pattern.MethodExtraction)
			routeInfo.MethodExplicit = matched
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

		// If we still don't have a method, try to infer from context (if enabled)
		if routeInfo.Method == "" && r.pattern.MethodExtraction != nil && r.pattern.MethodExtraction.InferFromContext {
			routeInfo.Method = r.inferMethodFromContext(node, edge)
			routeInfo.MethodExplicit = true
			found = true
		}
	}

	if r.pattern.PathFromArg && len(edge.Args) > r.pattern.PathArgIndex {
		path, dynName := r.resolvePathArg(edge.Args[r.pattern.PathArgIndex])
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
		if dynName != "" {
			routeInfo.DynamicParams = append(routeInfo.DynamicParams, dynName)
		}
		found = true
	}

	if r.pattern.HandlerFromArg && len(edge.Args) > r.pattern.HandlerArgIndex {
		routeInfo.Handler = r.contextProvider.GetArgumentInfo(edge.Args[r.pattern.HandlerArgIndex])
		routeInfo.Function = r.contextProvider.GetArgumentInfo(edge.Args[r.pattern.HandlerArgIndex])

		pkg := edge.Args[r.pattern.HandlerArgIndex].GetPkg()
		if pkg == "" {
			if node != nil && edge != nil && edge.Args[r.pattern.HandlerArgIndex].Fun != nil {
				pkg = edge.Args[r.pattern.HandlerArgIndex].Fun.GetPkg()
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

// inferMethodFromContext attempts to infer HTTP method from context
func (r *RoutePatternMatcherImpl) inferMethodFromContext(node TrackerNodeInterface, edge *metadata.CallGraphEdge) string {
	// Check if context inference is enabled
	if r.pattern.MethodExtraction == nil || !r.pattern.MethodExtraction.InferFromContext {
		return ""
	}

	// Try to find method from chained calls (like Mux .Methods("GET"))
	if node != nil {
		// Look for parent or sibling nodes that might contain method info
		parent := node.GetParent()
		if parent != nil {
			// Check if parent has method information
			for _, child := range parent.GetChildren() {
				if child != node && child.GetEdge() != nil {
					childEdge := child.GetEdge()
					callName := r.contextProvider.GetString(childEdge.Callee.Name)

					// Look for Methods call
					if callName == "Methods" && len(childEdge.Args) > 0 {
						methodArg := childEdge.Args[0]
						methodValue := strings.Trim(methodArg.GetValue(), "\"'")
						if r.isValidHTTPMethod(methodValue) {
							return strings.ToUpper(methodValue)
						}

						// Try argument info as well
						argInfo := r.contextProvider.GetArgumentInfo(methodArg)
						cleanArgInfo := strings.Trim(argInfo, "\"'")
						if r.isValidHTTPMethod(cleanArgInfo) {
							return strings.ToUpper(cleanArgInfo)
						}
					}
				}
			}
		}
	}

	// Try to infer from handler function name using pattern's method extraction config
	handlerName := r.contextProvider.GetString(edge.Caller.Name)
	if handlerName != "" {
		method := r.extractMethodFromFunctionNameWithConfig(handlerName, r.pattern.MethodExtraction)
		if method != "" && method != "POST" { // Don't use POST as default
			return method
		}
	}

	// Also try the handler from the arguments if available
	if len(edge.Args) > 1 {
		handlerArg := edge.Args[1] // Typically the handler is the second argument
		argInfo := r.contextProvider.GetArgumentInfo(handlerArg)
		if argInfo != "" {
			method := r.extractMethodFromFunctionNameWithConfig(argInfo, r.pattern.MethodExtraction)
			if method != "" && method != "POST" {
				return method
			}
		}
	}

	// Default fallback
	return "GET"
}

// MountPatternMatcherImpl implements MountPatternMatcher
type MountPatternMatcherImpl struct {
	*BasePatternMatcher
	pattern MountPattern
}

// NewMountPatternMatcher creates a new mount pattern matcher
func NewMountPatternMatcher(pattern MountPattern, cfg *APISpecConfig, contextProvider ContextProvider, typeResolver TypeResolver) *MountPatternMatcherImpl {
	return &MountPatternMatcherImpl{
		BasePatternMatcher: NewBasePatternMatcher(cfg, contextProvider, typeResolver),
		pattern:            pattern,
	}
}

// MatchNode checks if a node matches the mount pattern
func (m *MountPatternMatcherImpl) MatchNode(node TrackerNodeInterface) bool {
	if node == nil || node.GetEdge() == nil {
		return false
	}

	edge := node.GetEdge()
	callName := m.contextProvider.GetString(edge.Callee.Name)
	recvType := m.contextProvider.GetString(edge.Callee.RecvType)
	recvPkg := m.contextProvider.GetString(edge.Callee.Pkg)

	// Build fully qualified receiver type
	fqRecvType := recvPkg
	if fqRecvType != "" && recvType != "" {
		fqRecvType += "." + recvType
	} else if recvType != "" {
		fqRecvType = recvType
	}

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

	// Check receiver type
	if m.pattern.RecvTypeRegex != "" {
		re, err := cachedRegex(m.pattern.RecvTypeRegex)
		if err != nil || !re.MatchString(fqRecvType) {
			return false
		}
	} else if m.pattern.RecvType != "" && m.pattern.RecvType != fqRecvType {
		return false
	}

	return m.pattern.IsMount
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
		path, dynName := m.resolvePathArg(edge.Args[m.pattern.PathArgIndex])
		mountInfo.Path = path
		if dynName != "" {
			mountInfo.DynamicParams = append(mountInfo.DynamicParams, dynName)
		}
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
func NewSecurityPatternMatcher(pattern SecurityPattern, cfg *APISpecConfig, contextProvider ContextProvider, typeResolver TypeResolver) *SecurityPatternMatcherImpl {
	return &SecurityPatternMatcherImpl{
		BasePatternMatcher: NewBasePatternMatcher(cfg, contextProvider, typeResolver),
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

	callName := s.contextProvider.GetString(edge.Callee.Name)
	recvType := s.contextProvider.GetString(edge.Callee.RecvType)
	recvPkg := s.contextProvider.GetString(edge.Callee.Pkg)

	// Build fully qualified receiver type
	fqRecvType := recvPkg
	if fqRecvType != "" && recvType != "" {
		fqRecvType += "." + recvType
	} else if recvType != "" {
		fqRecvType = recvType
	}

	if s.pattern.CallRegex != "" && !s.matchPattern(s.pattern.CallRegex, callName) {
		return false
	}

	if s.pattern.FunctionNameRegex != "" {
		funcName := s.contextProvider.GetString(edge.Caller.Name)
		if !s.matchPattern(s.pattern.FunctionNameRegex, funcName) {
			return false
		}
	}

	if s.pattern.RecvTypeRegex != "" {
		re, err := cachedRegex(s.pattern.RecvTypeRegex)
		if err != nil || !re.MatchString(fqRecvType) {
			return false
		}
	} else if s.pattern.RecvType != "" && s.pattern.RecvType != fqRecvType {
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
func NewRequestPatternMatcher(pattern RequestBodyPattern, cfg *APISpecConfig, contextProvider ContextProvider, typeResolver TypeResolver) *RequestPatternMatcherImpl {
	return &RequestPatternMatcherImpl{
		BasePatternMatcher: NewBasePatternMatcher(cfg, contextProvider, typeResolver),
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

	// Body-source verification: only meaningful for ambiguous decoders
	// (json.Decode, json.Unmarshal, render.DecodeJSON, ...). Receiver-based
	// patterns like *gin.Context.BindJSON are already unambiguous because
	// the receiver type IS the request.
	//
	// Note: this gate is evaluated once per tracker node, and a decoder helper
	// shared by several routes has a single node — so the source cannot be
	// resolved per-route HERE without attributing one route's argument to all
	// (an io.Reader helper decoding r.Body in one route and a buffer in another
	// would otherwise leak the body into the second). The known io.Reader-helper
	// false negative is left to a per-route gate rather than risk that false
	// positive. See the discussion on issue #170's response counterpart.
	if r.pattern.RequireRequestSource && r.bodyResolver != nil && r.bodyResolver.Enabled() {
		src := r.bodySource(edge)
		if src == nil || !r.bodyResolver.IsRequestSource(src, edge) {
			return false
		}
	}

	return true
}

// bodySource returns the CallArgument that carries the decoder's input bytes
// for the given call edge, according to the pattern configuration.
func (r *RequestPatternMatcherImpl) bodySource(edge *metadata.CallGraphEdge) *metadata.CallArgument {
	if edge == nil {
		return nil
	}
	if r.pattern.BodyFromReceiver {
		return resolveReceiverSource(edge, r.bodyResolver.metadata())
	}
	idx := r.pattern.BodySourceArgIndex
	if idx < 0 || idx >= len(edge.Args) {
		return nil
	}
	return edge.Args[idx]
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
			bodyType = r.resolveTypeOrigin(arg, typeNode, bodyType)

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
		schema, _ := mapGoTypeToOpenAPISchema(route.UsedTypes, bodyType, route.Metadata, r.cfg, nil)
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

	// Original logic for type resolution
	if arg.GetKind() == metadata.KindIdent || arg.GetKind() == metadata.KindFuncLit {
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
