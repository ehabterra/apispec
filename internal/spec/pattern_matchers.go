package spec

import (
	"net/http"
	"regexp"
	"strings"

	"github.com/ehabterra/apispec/internal/metadata"
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
		matched, err := regexp.MatchString(r.pattern.RecvTypeRegex, fqRecvType)
		if err != nil || !matched {
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
func (r *RoutePatternMatcherImpl) ExtractRoute(node TrackerNodeInterface) RouteInfo {
	edge := node.GetEdge()
	routeInfo := RouteInfo{
		Method:    http.MethodPost, // Default method
		Package:   r.contextProvider.GetString(edge.Callee.Pkg),
		File:      r.contextProvider.GetString(edge.Position),
		Response:  make(map[string]*ResponseInfo),
		UsedTypes: make(map[string]*Schema),
	}

	if node.GetEdge() != nil {
		routeInfo.Metadata = node.GetEdge().Callee.Meta
	} else if node.GetArgument() != nil {
		routeInfo.Metadata = node.GetArgument().Meta
	}

	if routeInfo.File == "" && node.GetArgument() != nil {
		routeInfo.File = node.GetArgument().GetPosition()
	}

	r.extractRouteDetails(node, &routeInfo)

	// Extract handler information
	if r.pattern.HandlerFromArg && len(edge.Args) > r.pattern.HandlerArgIndex {
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
				originTypeStr = r.contextProvider.GetArgumentInfo(*originType)
			}
			if originTypeStr != "" {
				routeInfo.Summary = originTypeStr
			}
		}
	}

	return routeInfo
}

// extractRouteDetails extracts route details from a node
func (r *RoutePatternMatcherImpl) extractRouteDetails(node TrackerNodeInterface, routeInfo *RouteInfo) {
	edge := node.GetEdge()
	if r.pattern.MethodFromCall {
		funcName := r.contextProvider.GetString(edge.Callee.Name)
		routeInfo.Method = r.extractMethodFromFunctionNameWithConfig(funcName, r.pattern.MethodExtraction)
	} else if r.pattern.MethodFromHandler && r.pattern.HandlerFromArg && len(edge.Args) > r.pattern.HandlerArgIndex {
		// Extract method from handler function name
		handlerArg := edge.Args[r.pattern.HandlerArgIndex]
		handlerName := r.contextProvider.GetArgumentInfo(handlerArg)
		if handlerName != "" {
			routeInfo.Method = r.extractMethodFromFunctionNameWithConfig(handlerName, r.pattern.MethodExtraction)
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
			} else {
				// If not a valid method, try to extract from argument info
				argInfo := r.contextProvider.GetArgumentInfo(methodArg)
				if argInfo != "" {
					cleanArgInfo := strings.Trim(argInfo, "\"'")
					if r.isValidHTTPMethod(cleanArgInfo) {
						routeInfo.Method = strings.ToUpper(cleanArgInfo)
					}
				}
			}
		}

		// If we still don't have a method, try to infer from context (if enabled)
		if (routeInfo.Method == "" || routeInfo.Method == http.MethodPost) && r.pattern.MethodExtraction != nil && r.pattern.MethodExtraction.InferFromContext {
			routeInfo.Method = r.inferMethodFromContext(node, edge)
		}
	}

	if r.pattern.PathFromArg && len(edge.Args) > r.pattern.PathArgIndex {
		routeInfo.Path = r.contextProvider.GetArgumentInfo(edge.Args[r.pattern.PathArgIndex])
		if routeInfo.Path == "" {
			routeInfo.Path = "/"
		}
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
	}
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
		matched, err := regexp.MatchString(m.pattern.RecvTypeRegex, fqRecvType)
		if err != nil || !matched {
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
		mountInfo.Path = m.contextProvider.GetArgumentInfo(edge.Args[m.pattern.PathArgIndex])
	}

	// Extract router argument if available
	if m.pattern.RouterArgIndex >= 0 && len(edge.Args) > m.pattern.RouterArgIndex {
		mountInfo.RouterArg = &edge.Args[m.pattern.RouterArgIndex]

		// Trace router origin
		m.traceRouterOrigin(mountInfo.RouterArg, node)

		// Find assignment function
		mountInfo.Assignment = m.findAssignmentFunction(mountInfo.RouterArg)
	}

	return mountInfo
}

// RequestPatternMatcherImpl implements RequestPatternMatcher
type RequestPatternMatcherImpl struct {
	*BasePatternMatcher
	pattern RequestBodyPattern
}

// NewRequestPatternMatcher creates a new request pattern matcher
func NewRequestPatternMatcher(pattern RequestBodyPattern, cfg *APISpecConfig, contextProvider ContextProvider, typeResolver TypeResolver) *RequestPatternMatcherImpl {
	return &RequestPatternMatcherImpl{
		BasePatternMatcher: NewBasePatternMatcher(cfg, contextProvider, typeResolver),
		pattern:            pattern,
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
		matched, err := regexp.MatchString(r.pattern.RecvTypeRegex, fqRecvType)
		if err != nil || !matched {
			return false
		}
	} else if r.pattern.RecvType != "" && r.pattern.RecvType != fqRecvType {
		return false
	}

	return true
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
		bodyType := r.contextProvider.GetArgumentInfo(arg)

		// Check if this is a literal value - if so, determine appropriate type
		if arg.GetKind() == metadata.KindLiteral {
			bodyType = determineLiteralType(bodyType)
		} else {
			// Check for resolved type information in the CallArgument
			if resolvedType := arg.GetResolvedType(); resolvedType != "" {
				bodyType = resolvedType
			} else if arg.IsGenericType && arg.GenericTypeName != -1 {
				// If it's a generic type, try to resolve it from the edge's type parameters
				if concreteType, exists := node.GetTypeParamMap()[arg.GetGenericTypeName()]; exists {
					bodyType = concreteType
				}
			}

			// Trace type origin
			bodyType = r.resolveTypeOrigin(arg, node, bodyType)

			// Apply dereferencing if needed
			if r.pattern.Deref && strings.HasPrefix(bodyType, "*") {
				bodyType = strings.TrimPrefix(bodyType, "*")
			}
		}

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
	re, err := regexp.Compile(pattern)
	if err != nil {
		return false
	}
	return re.MatchString(value)
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
func (r *RequestPatternMatcherImpl) resolveTypeOrigin(arg metadata.CallArgument, node TrackerNodeInterface, originalType string) string {
	// NEW: If the argument has resolved type information, use it
	if resolvedType := arg.GetResolvedType(); resolvedType != "" {
		return resolvedType
	}

	typeParts := TypeParts(originalType)

	// If it's a generic type with a concrete resolution, use it
	genericType := traceGenericOrigin(node, typeParts)
	if genericType != "" {
		return genericType
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

func traceGenericOrigin(node TrackerNodeInterface, typeParts Parts) string {
	typeParams := node.GetTypeParamMap()

	if len(typeParams) > 0 && typeParts.TypeName != "" {
		searchType := typeParts.TypeName
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
	if funcName == "" {
		return ""
	}

	// Use default config if none provided
	if config == nil {
		config = DefaultMethodExtractionConfig()
	}

	// Prepare function name based on case sensitivity
	searchName := funcName
	if !config.CaseSensitive {
		searchName = strings.ToLower(funcName)
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

	// Check prefix matches first if enabled
	if config.UsePrefix {
		for _, mapping := range mappings {
			for _, pattern := range mapping.Patterns {
				searchPattern := pattern
				if !config.CaseSensitive {
					searchPattern = strings.ToLower(pattern)
				}

				if strings.HasPrefix(searchName, searchPattern) {
					// Make sure it's a word boundary (not part of another word)
					if len(searchName) == len(searchPattern) || !b.isLetter(rune(searchName[len(searchPattern)])) {
						return mapping.Method
					}
				}
			}
		}
	}

	// Check contains matches if enabled
	if config.UseContains {
		for _, mapping := range mappings {
			for _, pattern := range mapping.Patterns {
				searchPattern := pattern
				if !config.CaseSensitive {
					searchPattern = strings.ToLower(pattern)
				}

				if strings.Contains(searchName, searchPattern) {
					return mapping.Method
				}
			}
		}
	}

	return config.DefaultMethod
}

// isLetter checks if a rune is a letter
func (b *BasePatternMatcher) isLetter(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}
