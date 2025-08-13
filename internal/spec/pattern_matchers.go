package spec

import (
	"net/http"
	"regexp"
	"strings"

	"github.com/ehabterra/swagen/internal/metadata"
)

// BasePatternMatcher provides common functionality for all pattern matchers
type BasePatternMatcher struct {
	contextProvider ContextProvider
	cfg             *SwagenConfig
	schemaMapper    SchemaMapper
	typeResolver    TypeResolver
}

// NewBasePatternMatcher creates a new base pattern matcher
func NewBasePatternMatcher(cfg *SwagenConfig, contextProvider ContextProvider, typeResolver TypeResolver) *BasePatternMatcher {
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
func NewRoutePatternMatcher(pattern RoutePattern, cfg *SwagenConfig, contextProvider ContextProvider, typeResolver TypeResolver) *RoutePatternMatcherImpl {
	return &RoutePatternMatcherImpl{
		BasePatternMatcher: NewBasePatternMatcher(cfg, contextProvider, typeResolver),
		pattern:            pattern,
	}
}

// MatchNode checks if a node matches the route pattern
func (r *RoutePatternMatcherImpl) MatchNode(node *TrackerNode) bool {
	if node == nil || node.CallGraphEdge == nil {
		return false
	}

	callName := r.contextProvider.GetString(node.CallGraphEdge.Callee.Name)
	recvType := r.contextProvider.GetString(node.CallGraphEdge.Callee.RecvType)
	recvPkg := r.contextProvider.GetString(node.CallGraphEdge.Callee.Pkg)

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
		funcName := r.contextProvider.GetString(node.CallGraphEdge.Caller.Name)
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
func (r *RoutePatternMatcherImpl) ExtractRoute(node *TrackerNode) RouteInfo {
	routeInfo := RouteInfo{
		Method:   http.MethodPost, // Default method
		Package:  r.contextProvider.GetString(node.Callee.Pkg),
		File:     r.contextProvider.GetString(node.CallGraphEdge.Position),
		Response: make(map[string]*ResponseInfo),
	}

	if routeInfo.File == "" && node.CallArgument != nil {
		routeInfo.File = node.CallArgument.Position
	}

	r.extractRouteDetails(node, &routeInfo)

	// Extract handler information
	if r.pattern.HandlerFromArg && len(node.CallGraphEdge.Args) > r.pattern.HandlerArgIndex {
		handlerArg := node.CallGraphEdge.Args[r.pattern.HandlerArgIndex]
		if handlerArg.Kind == kindIdent {
			// Use variable tracing to resolve handler
			originVar, originPkg, originType, _ := r.traceVariable(
				handlerArg.Name,
				r.contextProvider.GetString(node.Caller.Name),
				r.contextProvider.GetString(node.Caller.Pkg),
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
func (r *RoutePatternMatcherImpl) extractRouteDetails(node *TrackerNode, routeInfo *RouteInfo) {
	if r.pattern.MethodFromCall {
		funcName := r.contextProvider.GetString(node.CallGraphEdge.Callee.Name)
		routeInfo.Method = r.extractMethodFromFunctionName(funcName)
	} else if r.pattern.MethodArgIndex >= 0 {
		routeInfo.Method = node.CallGraphEdge.Args[r.pattern.MethodArgIndex].Value
	}

	if r.pattern.PathFromArg && len(node.CallGraphEdge.Args) > r.pattern.PathArgIndex {
		routeInfo.Path = r.contextProvider.GetArgumentInfo(node.CallGraphEdge.Args[r.pattern.PathArgIndex])
	}

	if r.pattern.HandlerFromArg && len(node.CallGraphEdge.Args) > r.pattern.HandlerArgIndex {
		routeInfo.Handler = r.contextProvider.GetArgumentInfo(node.CallGraphEdge.Args[r.pattern.HandlerArgIndex])
		routeInfo.Function = r.contextProvider.GetArgumentInfo(node.CallGraphEdge.Args[r.pattern.HandlerArgIndex])

		pkg := node.CallGraphEdge.Args[r.pattern.HandlerArgIndex].Pkg
		if pkg == "" {
			if node != nil && node.CallGraphEdge != nil && node.CallGraphEdge.Args[r.pattern.HandlerArgIndex].Fun != nil {
				pkg = node.CallGraphEdge.Args[r.pattern.HandlerArgIndex].Fun.Pkg
			}
		}
		routeInfo.Package = pkg
	}
}

// MountPatternMatcherImpl implements MountPatternMatcher
type MountPatternMatcherImpl struct {
	*BasePatternMatcher
	pattern MountPattern
}

// NewMountPatternMatcher creates a new mount pattern matcher
func NewMountPatternMatcher(pattern MountPattern, cfg *SwagenConfig, contextProvider ContextProvider, typeResolver TypeResolver) *MountPatternMatcherImpl {
	return &MountPatternMatcherImpl{
		BasePatternMatcher: NewBasePatternMatcher(cfg, contextProvider, typeResolver),
		pattern:            pattern,
	}
}

// MatchNode checks if a node matches the mount pattern
func (m *MountPatternMatcherImpl) MatchNode(node *TrackerNode) bool {
	if node == nil || node.CallGraphEdge == nil {
		return false
	}

	callName := m.contextProvider.GetString(node.CallGraphEdge.Callee.Name)
	recvType := m.contextProvider.GetString(node.CallGraphEdge.Callee.RecvType)
	recvPkg := m.contextProvider.GetString(node.CallGraphEdge.Callee.Pkg)

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
		funcName := m.contextProvider.GetString(node.CallGraphEdge.Caller.Name)
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
func (m *MountPatternMatcherImpl) ExtractMount(node *TrackerNode) MountInfo {
	mountInfo := MountInfo{
		Pattern: m.pattern,
	}

	// Extract path if available
	if m.pattern.PathFromArg && len(node.CallGraphEdge.Args) > m.pattern.PathArgIndex {
		mountInfo.Path = m.contextProvider.GetArgumentInfo(node.CallGraphEdge.Args[m.pattern.PathArgIndex])
	}

	// Extract router argument if available
	if m.pattern.RouterFromArg && len(node.CallGraphEdge.Args) > m.pattern.RouterArgIndex {
		mountInfo.RouterArg = &node.CallGraphEdge.Args[m.pattern.RouterArgIndex]

		// Trace router origin
		m.traceRouterOrigin(mountInfo.RouterArg, node)

		// Find assignment function
		mountInfo.Assignment = m.findAssignmentFunction(*mountInfo.RouterArg)
	}

	return mountInfo
}

// RequestPatternMatcherImpl implements RequestPatternMatcher
type RequestPatternMatcherImpl struct {
	*BasePatternMatcher
	pattern RequestBodyPattern
}

// NewRequestPatternMatcher creates a new request pattern matcher
func NewRequestPatternMatcher(pattern RequestBodyPattern, cfg *SwagenConfig, contextProvider ContextProvider, typeResolver TypeResolver) *RequestPatternMatcherImpl {
	return &RequestPatternMatcherImpl{
		BasePatternMatcher: NewBasePatternMatcher(cfg, contextProvider, typeResolver),
		pattern:            pattern,
	}
}

// MatchNode checks if a node matches the request pattern
func (r *RequestPatternMatcherImpl) MatchNode(node *TrackerNode) bool {
	if node == nil || node.CallGraphEdge == nil {
		return false
	}

	callName := r.contextProvider.GetString(node.CallGraphEdge.Callee.Name)
	recvType := r.contextProvider.GetString(node.CallGraphEdge.Callee.RecvType)
	recvPkg := r.contextProvider.GetString(node.CallGraphEdge.Callee.Pkg)

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
		funcName := r.contextProvider.GetString(node.CallGraphEdge.Caller.Name)
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
func (r *RequestPatternMatcherImpl) ExtractRequest(node *TrackerNode, route *RouteInfo) *RequestInfo {
	reqInfo := &RequestInfo{
		ContentType: r.cfg.Defaults.RequestContentType,
	}

	if r.pattern.TypeFromArg && len(node.CallGraphEdge.Args) > r.pattern.TypeArgIndex {
		arg := node.CallGraphEdge.Args[r.pattern.TypeArgIndex]
		bodyType := r.contextProvider.GetArgumentInfo(arg)

		// Check for resolved type information in the CallArgument
		if arg.ResolvedType != "" {
			bodyType = arg.ResolvedType
		} else if arg.IsGenericType && arg.GenericTypeName != "" {
			// If it's a generic type, try to resolve it from the edge's type parameters
			if concreteType, exists := node.TypeParams()[arg.GenericTypeName]; exists {
				bodyType = concreteType
			}
		}

		// Trace type origin
		bodyType = r.resolveTypeOrigin(arg, node, bodyType)

		// Apply dereferencing if needed
		if r.pattern.Deref && strings.HasPrefix(bodyType, "*") {
			bodyType = strings.TrimPrefix(bodyType, "*")
		}

		reqInfo.BodyType = bodyType
		reqInfo.Schema = r.mapGoTypeToOpenAPISchema(bodyType)
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

func (b *BasePatternMatcher) traceRouterOrigin(routerArg *metadata.CallArgument, node *TrackerNode) {
	// Trace router origin based on argument kind
	switch routerArg.Kind {
	case kindIdent:
		b.traceVariable(
			routerArg.Name,
			b.contextProvider.GetString(node.Caller.Name),
			b.contextProvider.GetString(node.Caller.Pkg),
		)
	case kindUnary, kindStar:
		if routerArg.X != nil {
			b.traceVariable(
				routerArg.X.Name,
				b.contextProvider.GetString(node.Caller.Name),
				b.contextProvider.GetString(node.Caller.Pkg),
			)
		}
	case kindSelector:
		if routerArg.X != nil {
			b.traceVariable(
				routerArg.X.Name,
				b.contextProvider.GetString(node.Caller.Name),
				b.contextProvider.GetString(node.Caller.Pkg),
			)
		}
	case kindCall:
		if routerArg.Fun != nil {
			b.traceVariable(
				routerArg.Fun.Name,
				b.contextProvider.GetString(node.Caller.Name),
				b.contextProvider.GetString(node.Caller.Pkg),
			)
		}
	}
}

func (b *BasePatternMatcher) findAssignmentFunction(arg metadata.CallArgument) *metadata.CallArgument {
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

				if varName == arg.Name && varPkg == arg.Pkg && varType == arg.X.Type {
					// Get the function name directly (it's already a string)
					for _, targetArg := range edge.Args {
						if targetArg.Kind == kindCall && targetArg.Fun != nil {
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
func (r *RequestPatternMatcherImpl) resolveTypeOrigin(arg metadata.CallArgument, node *TrackerNode, originalType string) string {
	// NEW: If the argument has resolved type information, use it
	if arg.ResolvedType != "" {
		return arg.ResolvedType
	}

	typeParts := TypeParts(originalType)

	// If it's a generic type with a concrete resolution, use it
	genericType := traceGenericOrigin(node, typeParts)
	if genericType != "" {
		return genericType
	}

	// Original logic for type resolution
	if arg.Kind == "ident" {
		// Check if this variable has assignments that might give us more type information
		if assignments, exists := node.CallGraphEdge.AssignmentMap[arg.Name]; exists {
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

func traceGenericOrigin(node *TrackerNode, typeParts []string) string {
	typeParams := node.TypeParams()

	if len(typeParams) > 0 && len(typeParts) > 1 {
		searchType := typeParts[1]
		exists := true

		var concreteType string

		for exists {
			concreteType, exists = typeParams[searchType]

			if concreteType != "" {
				searchType = concreteType
			}
		}
		return searchType
	}
	return ""
}

func (b *BasePatternMatcher) extractMethodFromFunctionName(funcName string) string {
	methods := []string{"GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS", "HEAD"}
	for _, method := range methods {
		if strings.Contains(strings.ToUpper(funcName), method) {
			return method
		}
	}
	return ""
}

func (b *BasePatternMatcher) mapGoTypeToOpenAPISchema(goType string) *Schema {
	// switch {
	// case strings.Contains(goType, TypeSep):
	// 	parts := strings.Split(goType, TypeSep)
	// 	goType = metadata.DefaultImportName(parts[0]) + TypeSep + parts[1]
	// case strings.Contains(goType, defaultSep):
	// 	parts := strings.Split(goType, defaultSep)
	// 	goType = metadata.DefaultImportName(parts[0]) + defaultSep + parts[1]
	// }

	// Use TypeResolver for schema mapping if available
	if b.typeResolver != nil {
		return b.typeResolver.MapToOpenAPISchema(goType)
	}

	// Fallback to schema mapper
	return b.schemaMapper.MapGoTypeToOpenAPISchema(goType)
}
