package spec

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/ehabterra/apispec/internal/metadata"
)

// Regex cache for performance optimization
var (
	regexCache = make(map[string]*regexp.Regexp)
	regexMutex sync.RWMutex
)

// getCachedRegex returns a cached compiled regex or compiles and caches a new one
func getCachedRegex(pattern string) (*regexp.Regexp, error) {
	regexMutex.RLock()
	if re, exists := regexCache[pattern]; exists {
		regexMutex.RUnlock()
		return re, nil
	}
	regexMutex.RUnlock()

	regexMutex.Lock()
	defer regexMutex.Unlock()

	// Double-check after acquiring write lock
	if re, exists := regexCache[pattern]; exists {
		return re, nil
	}

	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}

	regexCache[pattern] = re
	return re, nil
}

const (
	TypeSep    = "-->"
	defaultSep = "."
)

// RouteInfo represents extracted route information
type RouteInfo struct {
	Path     string
	Method   string
	Handler  string
	Package  string
	File     string
	Function string
	Summary  string
	Tags     []string
	Request  *RequestInfo
	Response map[string]*ResponseInfo
	Params   []Parameter

	UsedTypes map[string]*Schema
	Metadata  *metadata.Metadata

	// Resolved router group prefix (if any)
	GroupPrefix string
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
	requestMatchers  []RequestPatternMatcher
	responseMatchers []ResponsePatternMatcher
	paramMatchers    []ParamPatternMatcher
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
func (e *Extractor) ExtractRoutes() []RouteInfo {
	routes := make([]RouteInfo, 0)
	for _, root := range e.tree.GetRoots() {
		e.traverseForRoutes(root, "", nil, &routes)
	}
	return routes
}

// traverseForRoutes traverses the tree to find routes
func (e *Extractor) traverseForRoutes(node TrackerNodeInterface, mountPath string, mountTags []string, routes *[]RouteInfo) {
	e.traverseForRoutesWithVisited(node, mountPath, mountTags, routes, make(map[string]bool))
}

// traverseForRoutesWithVisited traverses with visited tracking to prevent cycles
func (e *Extractor) traverseForRoutesWithVisited(node TrackerNodeInterface, mountPath string, mountTags []string, routes *[]RouteInfo, visited map[string]bool) {
	if node == nil {
		return
	}

	// Prevent infinite recursion
	nodeKey := node.GetKey()
	if visited[nodeKey] {
		return
	}
	visited[nodeKey] = true

	// Check for mount patterns first
	if mountInfo, isMount := e.executeMountPattern(node); isMount {
		e.handleMountNode(node, mountInfo, mountPath, mountTags, routes, visited)
	} else if routeInfo, isRoute := e.executeRoutePattern(node); isRoute {
		// Check for route patterns
		e.handleRouteNode(node, routeInfo, mountPath, mountTags, routes)
	} else {
		// Continue traversing children
		for _, child := range node.GetChildren() {
			e.traverseForRoutesWithVisited(child, mountPath, mountTags, routes, visited)
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
func (e *Extractor) executeRoutePattern(node TrackerNodeInterface) (RouteInfo, bool) {
	var bestMatch RouteInfo
	var bestPriority int
	var found bool

	for _, matcher := range e.routeMatchers {
		if matcher.MatchNode(node) {
			priority := matcher.GetPriority()
			if !found || priority > bestPriority {
				routeInfo := matcher.ExtractRoute(node)
				bestMatch = routeInfo
				bestPriority = priority
				found = true
			}
		}
	}

	return bestMatch, found
}

// handleMountNode handles a mount node
func (e *Extractor) handleMountNode(node TrackerNodeInterface, mountInfo MountInfo, mountPath string, mountTags []string, routes *[]RouteInfo, visited map[string]bool) {
	// Update mount path if needed
	if mountInfo.Path != "" {
		if mountPath == "" || !strings.HasSuffix(mountPath, mountInfo.Path) {
			mountPath = e.joinPaths(mountPath, mountInfo.Path)
		}
	}

	// Handle router assignment if present
	if mountInfo.Assignment != nil {
		e.handleRouterAssignment(mountInfo, mountPath, mountTags, routes, visited)
	}

	// Continue traversing children
	for _, child := range node.GetChildren() {
		var newTags []string
		if mountPath != "" {
			newTags = []string{mountPath}
		} else {
			newTags = mountTags
		}
		e.traverseForRoutesWithVisited(child, mountPath, newTags, routes, visited)
	}
}

// handleRouteNode handles a route node
func (e *Extractor) handleRouteNode(node TrackerNodeInterface, routeInfo RouteInfo, mountPath string, mountTags []string, routes *[]RouteInfo) {
	// Prepend mount path if present
	if mountPath != "" && routeInfo.Path != "" {
		routeInfo.Path = e.joinPaths(mountPath, routeInfo.Path)
	}

	// Set tags from mountTags if present
	if len(mountTags) > 0 {
		routeInfo.Tags = mountTags
	}

	// Extract request/response/params from children
	e.extractRouteChildren(node, &routeInfo)

	// Apply overrides
	e.overrideApplier.ApplyOverrides(&routeInfo)

	if routeInfo.IsValid() {
		// Update existing route or add new one
		var found bool
		for i := range *routes {
			if (*routes)[i].Function == routeInfo.Function {
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
func (e *Extractor) handleRouterAssignment(mountInfo MountInfo, mountPath string, mountTags []string, routes *[]RouteInfo, visited map[string]bool) {
	// Find the target node for the assignment
	targetNode := e.findTargetNode(mountInfo.Assignment)
	if targetNode != nil {
		for _, child := range targetNode.GetChildren() {
			var newTags []string
			if mountPath != "" {
				newTags = []string{mountPath}
			} else {
				newTags = mountTags
			}
			e.traverseForRoutesWithVisited(child, mountPath, newTags, routes, visited)
		}
	}
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
func (e *Extractor) extractRouteChildren(routeNode TrackerNodeInterface, route *RouteInfo) {
	for _, child := range routeNode.GetChildren() {
		// Extract request
		if req := e.extractRequestFromNode(child, route); req != nil {
			route.Request = req
		}

		// Extract response
		if resp := e.extractResponseFromNode(child, route); resp != nil && resp.BodyType != "" {
			route.Response[fmt.Sprintf("%d", resp.StatusCode)] = resp
		}

		// Extract parameters
		if param := e.extractParamFromNode(child, route); param != nil {
			route.Params = append(route.Params, *param)
		}

		// Recursive extraction
		e.extractRouteChildren(child, route)
	}

	// Extract parameters from the route node itself
	if param := e.extractParamFromNode(routeNode, route); param != nil {
		route.Params = append(route.Params, *param)
	}
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

// extractResponseFromNode extracts response information from a node
func (e *Extractor) extractResponseFromNode(node TrackerNodeInterface, route *RouteInfo) *ResponseInfo {
	for _, matcher := range e.responseMatchers {
		if matcher.MatchNode(node) {
			return matcher.ExtractResponse(node, route)
		}
	}
	return &ResponseInfo{
		StatusCode:  e.cfg.Defaults.ResponseStatus,
		ContentType: e.cfg.Defaults.ResponseContentType,
	}
}

// extractParamFromNode extracts parameter information from a node
func (e *Extractor) extractParamFromNode(node TrackerNodeInterface, route *RouteInfo) *Parameter {
	for _, matcher := range e.paramMatchers {
		if matcher.MatchNode(node) {
			return matcher.ExtractParam(node, route)
		}
	}
	return nil
}

// joinPaths joins two URL paths cleanly
func (e *Extractor) joinPaths(a, b string) string {
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
		re, err := getCachedRegex(r.pattern.RecvTypeRegex)
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

// ExtractResponse extracts response information from a matched node
func (r *ResponsePatternMatcherImpl) ExtractResponse(node TrackerNodeInterface, route *RouteInfo) *ResponseInfo {
	respInfo := &ResponseInfo{
		StatusCode:  r.cfg.Defaults.ResponseStatus,
		ContentType: r.cfg.Defaults.ResponseContentType,
	}

	edge := node.GetEdge()
	if r.pattern.StatusFromArg && len(edge.Args) > r.pattern.StatusArgIndex {
		statusStr := r.contextProvider.GetArgumentInfo(edge.Args[r.pattern.StatusArgIndex])
		if status, ok := r.schemaMapper.MapStatusCode(statusStr); ok {
			respInfo.StatusCode = status
		}
	}

	if r.pattern.TypeFromArg && len(edge.Args) > r.pattern.TypeArgIndex {

		arg := edge.Args[r.pattern.TypeArgIndex]

		// If the argument is a type conversion, get the value of the original argument
		if arg.GetKind() == metadata.KindTypeConversion {
			arg = arg.Args[0]
		}

		bodyType := r.contextProvider.GetArgumentInfo(arg)

		// Check if this is a literal value - if so, determine appropriate type
		if arg.GetKind() == metadata.KindLiteral {
			// For literal values, determine the appropriate type based on the value
			bodyType = determineLiteralType(bodyType)
		} else {

			// Trace type origin for non-literal arguments
			bodyType = r.resolveTypeOrigin(arg, node, bodyType)

			// Apply dereferencing if needed
			if r.pattern.Deref && strings.HasPrefix(bodyType, "*") {
				bodyType = strings.TrimPrefix(bodyType, "*")
			}
		}

		respInfo.BodyType = preprocessingBodyType(bodyType)

		schema, _ := mapGoTypeToOpenAPISchema(route.UsedTypes, bodyType, route.Metadata, r.cfg, nil)
		respInfo.Schema = schema
	}

	return respInfo
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
		re, err := getCachedRegex(p.pattern.RecvTypeRegex)
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
