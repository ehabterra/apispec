package spec

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/ehabterra/swagen/internal/metadata"
)

const (
	TypeSep             = "-->"
	kindIdent           = "ident"
	kindLiteral         = "literal"
	kindSelector        = "selector"
	kindCall            = "call"
	kindRaw             = "raw"
	kindIndex           = "index"
	kindUnary           = "unary"
	kindField           = "field"
	kindParen           = "paren"
	kindStar            = "star"
	kindArrayType       = "array_type"
	kindSlice           = "slice"
	kindCompositeLit    = "composite_lit"
	kindKeyValue        = "key_value"
	kindTypeAssert      = "type_assert"
	kindChanType        = "chan_type"
	kindMapType         = "map_type"
	kindStructType      = "struct_type"
	kindInterfaceType   = "interface_type"
	kindInterfaceMethod = "interface_method"
	kindEmbed           = "embed"
	kindEllipsis        = "ellipsis"
	kindFuncType        = "func_type"
	kindFuncResults     = "func_results"
	callEllipsis        = "call(...)"
	defaultSep          = "."
	slashSep            = "/"
)

// Extractor provides methods to extract OpenAPI information from a TrackerTree
// and SwagenConfig.
type Extractor struct {
	tree *TrackerTree
	cfg  *SwagenConfig
}

// NewExtractor creates a new extractor instance using a TrackerTree.
func NewExtractor(tree *TrackerTree, cfg *SwagenConfig) *Extractor {
	return &Extractor{
		tree: tree,
		cfg:  cfg,
	}
}

// ExtractRoutes extracts route information from the TrackerTree using mount/route/request/response/param patterns.
func (e *Extractor) ExtractRoutes() []RouteInfo {
	var routes []RouteInfo
	for _, root := range e.tree.GetRoots() {
		e.traverseForRoutes(root, "", nil, &routes)
	}
	return routes
}

// traverseForRoutes recursively traverses the TrackerTree to extract routes, handling mounts and routes.
func (e *Extractor) traverseForRoutes(node *TrackerNode, mountPath string, mountTags []string, routes *[]RouteInfo) {
	e.traverseForRoutesWithVisited(node, mountPath, mountTags, routes, make(map[string]bool))
}

func (e *Extractor) traverseForRoutesWithVisited(node *TrackerNode, mountPath string, mountTags []string, routes *[]RouteInfo, visited map[string]bool) {
	if node == nil {
		return
	}

	// Prevent infinite recursion by tracking visited nodes
	nodeID := node.id
	if visited[nodeID] {
		return
	}
	visited[nodeID] = true

	// Check if this node is a mount
	if mountPattern, ok := e.matchMountPattern(node); ok {
		// Update mount path if needed
		if mountPattern.PathFromArg && len(node.CallGraphEdge.Args) > mountPattern.PathArgIndex {
			newSegment := e.callArgToString(node.CallGraphEdge.Args[mountPattern.PathArgIndex], nil)
			// Prevent duplicate segments (e.g., /users/users)
			if mountPath == "" || !strings.HasSuffix(mountPath, newSegment) {
				mountPath = joinPaths(mountPath, newSegment)
			}
		}

		// If this mount call has a router argument that is a function call, record the mapping
		if mountPattern.RouterFromArg && len(node.CallGraphEdge.Args) > mountPattern.RouterArgIndex {
			routerArg := node.CallGraphEdge.Args[mountPattern.RouterArgIndex]
			// --- Integration: Trace router/group origin ---
			switch routerArg.Kind {
			case kindIdent:
				_, _, _ = metadata.TraceVariableOrigin(
					routerArg.Name,
					e.getString(node.Caller.Name),
					e.getString(node.Caller.Pkg),
					e.tree.meta,
				)
			case kindUnary, kindStar:
				if routerArg.X != nil {
					_, _, _ = metadata.TraceVariableOrigin(
						routerArg.X.Name,
						e.getString(node.Caller.Name),
						e.getString(node.Caller.Pkg),
						e.tree.meta,
					)
				}
			case kindSelector:
				if routerArg.X != nil {
					_, _, _ = metadata.TraceVariableOrigin(
						routerArg.X.Name,
						e.getString(node.Caller.Name),
						e.getString(node.Caller.Pkg),
						e.tree.meta,
					)
				}
			case kindCall:
				if routerArg.Fun != nil {
					_, _, _ = metadata.TraceVariableOrigin(
						routerArg.Fun.Name,
						e.getString(node.Caller.Name),
						e.getString(node.Caller.Pkg),
						e.tree.meta,
					)
				}
			case kindTypeAssert:
				if routerArg.Fun != nil && routerArg.Fun.Type != "" {
					// No further tracing needed, type is asserted
				}
			}
			// Assignment tracking: look up the assignment for this variable/field
			assignFunc := e.findAssignmentFunction(routerArg)
			if assignFunc != nil {
				// Apply BFT
				var (
					id         = assignFunc.ID()
					targetNode *TrackerNode
					queue      = e.tree.roots
				)

				for len(queue) > 0 {
					nd := queue[0]
					queue = queue[1:] // dequeue

					if nd.id == id {
						targetNode = nd
						queue = nil
						break
					}

					queue = append(queue, nd.children...)
				}

				if targetNode != nil {
					for _, child := range targetNode.children {
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
		}

		for _, child := range node.children {
			var newTags []string
			if mountPath != "" {
				newTags = []string{mountPath}
			} else {
				newTags = mountTags
			}
			e.traverseForRoutesWithVisited(child, mountPath, newTags, routes, visited)
		}

		return
	}

	// Check if this node is a route
	if routePattern, ok := e.matchRoutePatternWithPattern(node); ok {
		route := e.extractRouteFromNode(node, routePattern)
		// Prepend mount path if present
		if mountPath != "" && route.Path != "" {
			route.Path = joinPaths(mountPath, route.Path)
		}
		// Set tags from mountTags if present
		if len(mountTags) > 0 {
			route.Tags = mountTags
		}
		// Extract request/response/params from children
		e.extractRouteChildren(node, &route)
		if route.IsValid() {
			var found = false

			for i := range *routes {
				if (*routes)[i].Function == route.Function {
					(*routes)[i] = route
					found = true
					break
				}
			}
			if !found {
				*routes = append(*routes, route)
			}
		}

		return
	}

	// Otherwise, keep traversing
	for _, child := range node.children {
		e.traverseForRoutesWithVisited(child, mountPath, mountTags, routes, visited)
	}
}

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

	// Resolved router group prefix (if any)
	GroupPrefix string
}

// IsValid checks if the route info is valid
func (r *RouteInfo) IsValid() bool {
	return r.Path != "" && r.Method != "" && r.Handler != ""
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

// extractRouteFromNode extracts route information from a node and function
func (e *Extractor) extractRouteFromNode(node *TrackerNode, pattern RoutePattern) RouteInfo {
	routeInfo := RouteInfo{
		Package:  e.getString(node.Callee.Pkg),
		File:     e.getString(node.CallGraphEdge.Position),
		Response: make(map[string]*ResponseInfo),
	}

	if routeInfo.File == "" && node.CallArgument != nil {
		routeInfo.File = node.CallArgument.Position
	}

	e.extractRouteDetailsFromNode(node, pattern, &routeInfo)

	// --- Integration: Trace handler origin/type ---
	if pattern.HandlerFromArg && len(node.CallGraphEdge.Args) > pattern.HandlerArgIndex {
		handlerArg := node.CallGraphEdge.Args[pattern.HandlerArgIndex]
		if handlerArg.Kind == kindIdent {
			// Use TraceVariableOrigin to resolve handler
			originVar, originPkg, originType := metadata.TraceVariableOrigin(
				handlerArg.Name,
				e.getString(node.Caller.Name),
				e.getString(node.Caller.Pkg),
				e.tree.meta,
			)
			if originVar != "" {
				routeInfo.Handler = originVar
			}
			if originPkg != "" {
				routeInfo.Package = originPkg
			}

			var originTypeStr string
			if originType != nil {
				originTypeStr = e.callArgToString(*originType, nil)
			}
			if originTypeStr != "" {
				routeInfo.Summary = originTypeStr // Optionally store type info for debugging
			}
		}
	}

	// Extract request body information
	routeInfo.Request = e.extractRequestInfoFromNode(node, &routeInfo)

	// Extract response information
	response := e.extractResponseInfoFromNode(node)
	if response != nil && response.Schema != nil {
		routeInfo.Response[fmt.Sprintf("%d", response.StatusCode)] = response
	}

	// Extract parameters
	routeInfo.Params = e.extractParametersFromNode(node)

	// Apply overrides
	e.applyOverrides(&routeInfo)

	return routeInfo
}

// extractRouteDetailsFromNode extracts route details from a node
func (e *Extractor) extractRouteDetailsFromNode(node *TrackerNode, pattern RoutePattern, routeInfo *RouteInfo) {
	if pattern.MethodFromCall {
		funcName := e.getString(node.CallGraphEdge.Callee.Name)
		routeInfo.Method = e.extractMethodFromFunctionName(funcName)
	} else if pattern.MethodArgIndex >= 0 {
		routeInfo.Method = node.CallGraphEdge.Args[pattern.MethodArgIndex].Value
	}

	if pattern.PathFromArg && len(node.CallGraphEdge.Args) > pattern.PathArgIndex {
		routeInfo.Path = e.callArgToString(node.CallGraphEdge.Args[pattern.PathArgIndex], nil)
	}
	if pattern.HandlerFromArg && len(node.CallGraphEdge.Args) > pattern.HandlerArgIndex {
		routeInfo.Handler = e.callArgToString(node.CallGraphEdge.Args[pattern.HandlerArgIndex], nil)
		routeInfo.Function = e.callArgToString(node.CallGraphEdge.Args[pattern.HandlerArgIndex], nil)

		pkg := node.CallGraphEdge.Args[pattern.HandlerArgIndex].Pkg
		routeInfo.Package = pkg
	}
}

// extractRequestInfoFromNode extracts request info from a node
func (e *Extractor) extractRequestInfoFromNode(node *TrackerNode, route *RouteInfo) *RequestInfo {
	for _, pattern := range e.cfg.Framework.RequestBodyPatterns {
		if e.matchRequestBodyPattern(node, pattern, route) {
			return e.extractRequestBodyDetailsFromNode(node, pattern)
		}
	}
	return nil
}

// extractRequestBodyDetailsFromNode extracts request body details from a node
func (e *Extractor) extractRequestBodyDetailsFromNode(node *TrackerNode, pattern RequestBodyPattern) *RequestInfo {
	reqInfo := &RequestInfo{
		ContentType: e.cfg.Defaults.RequestContentType,
	}
	if pattern.TypeFromArg && len(node.CallGraphEdge.Args) > pattern.TypeArgIndex {
		arg := node.CallGraphEdge.Args[pattern.TypeArgIndex]
		bodyType := e.callArgToString(arg, nil)
		// --- Integration: Trace request body type origin ---
		switch arg.Kind {
		case kindIdent:
			_, _, originType := metadata.TraceVariableOrigin(
				arg.Name,
				e.getString(node.Caller.Name),
				e.getString(node.Caller.Pkg),
				e.tree.meta,
			)
			var originTypeStr string
			if originType != nil {
				originTypeStr = e.callArgToString(*originType, nil)
			}

			if originTypeStr != "" {
				bodyType = originTypeStr
			}
		case kindUnary, kindStar:
			if arg.X != nil {
				_, _, originType := metadata.TraceVariableOrigin(
					arg.X.Name,
					e.getString(node.Caller.Name),
					e.getString(node.Caller.Pkg),
					e.tree.meta,
				)
				var originTypeStr string
				if originType != nil {
					originTypeStr = e.callArgToString(*originType, nil)
				}

				if originTypeStr != "" {
					bodyType = originTypeStr
				}
			}
		case kindSelector:
			if arg.X != nil {
				_, _, originType := metadata.TraceVariableOrigin(
					arg.X.Name,
					e.getString(node.Caller.Name),
					e.getString(node.Caller.Pkg),
					e.tree.meta,
				)
				var originTypeStr string
				if originType != nil {
					originTypeStr = e.callArgToString(*originType, nil)
				}

				if originTypeStr != "" {
					bodyType = originTypeStr
				}
			}
		case kindCall:
			if arg.Fun != nil {
				// Recursively trace the function call's return value as a variable
				_, _, originType := metadata.TraceVariableOrigin(
					arg.Fun.Name,
					e.getString(node.Caller.Name),
					e.getString(node.Caller.Pkg),
					e.tree.meta,
				)
				var originTypeStr string
				if originType != nil {
					originTypeStr = e.callArgToString(*originType, nil)
				}

				if originTypeStr != "" {
					bodyType = originTypeStr
				}
			}
		case kindTypeAssert:
			if arg.Fun != nil && arg.Fun.Type != "" {
				bodyType = arg.Fun.Type
			}
		}
		// If the resolved type is a type parameter, try to resolve it using TypeParamMap
		if edge, ok := e.findCallGraphEdgeForNode(node); ok && bodyType != "" {
			if concrete, found := edge.TypeParamMap[bodyType]; found && concrete != "" {
				bodyType = concrete
			}
		}
		reqInfo.BodyType = bodyType
		if pattern.Deref && strings.HasPrefix(reqInfo.BodyType, "*") {
			reqInfo.BodyType = strings.TrimPrefix(reqInfo.BodyType, "*")
		}
	}

	reqInfo.Schema = e.mapGoTypeToOpenAPISchema(reqInfo.BodyType)

	if reqInfo.BodyType == "" {
		return nil
	}

	return reqInfo
}

// extractResponseInfoFromNode extracts response info from a node
func (e *Extractor) extractResponseInfoFromNode(node *TrackerNode) *ResponseInfo {
	for _, pattern := range e.cfg.Framework.ResponsePatterns {
		if e.matchResponsePattern(node, pattern) {
			return e.extractResponseDetailsFromNode(node, pattern)
		}
	}
	return &ResponseInfo{
		StatusCode:  e.cfg.Defaults.ResponseStatus,
		ContentType: e.cfg.Defaults.ResponseContentType,
	}
}

// extractResponseDetailsFromNode extracts response details from a node
func (e *Extractor) extractResponseDetailsFromNode(node *TrackerNode, pattern ResponsePattern) *ResponseInfo {
	respInfo := &ResponseInfo{
		StatusCode:  e.cfg.Defaults.ResponseStatus,
		ContentType: e.cfg.Defaults.ResponseContentType,
	}
	if pattern.StatusFromArg && len(node.CallGraphEdge.Args) > pattern.StatusArgIndex {
		statusStr := e.callArgToString(node.CallGraphEdge.Args[pattern.StatusArgIndex], nil)
		if status, ok := e.parseStatusCode(statusStr); ok {
			respInfo.StatusCode = status
		}
	}
	if pattern.TypeFromArg && len(node.CallGraphEdge.Args) > pattern.TypeArgIndex {
		arg := node.CallGraphEdge.Args[pattern.TypeArgIndex]
		bodyType := e.callArgToString(arg, nil)
		// --- Integration: Trace response body type origin ---
		switch arg.Kind {
		case kindIdent:
			_, _, originType := metadata.TraceVariableOrigin(
				arg.Name,
				e.getString(node.Caller.Name),
				e.getString(node.Caller.Pkg),
				e.tree.meta,
			)
			var originTypeStr string
			if originType != nil {
				originTypeStr = e.callArgToString(*originType, nil)
			}

			if originTypeStr != "" {
				bodyType = originTypeStr
			}
		case kindUnary, kindStar:
			if arg.X != nil {
				_, _, originType := metadata.TraceVariableOrigin(
					arg.X.Name,
					e.getString(node.Caller.Name),
					e.getString(node.Caller.Pkg),
					e.tree.meta,
				)

				var originTypeStr string
				if originType != nil {
					originTypeStr = e.callArgToString(*originType, nil)
				}
				if originTypeStr != "" {
					bodyType = originTypeStr
				}
			}
		case kindSelector:
			if arg.X != nil {
				_, _, originType := metadata.TraceVariableOrigin(
					arg.X.Name,
					e.getString(node.Caller.Name),
					e.getString(node.Caller.Pkg),
					e.tree.meta,
				)

				var originTypeStr string
				if originType != nil {
					originTypeStr = e.callArgToString(*originType, nil)
				}
				if originTypeStr != "" {
					bodyType = originTypeStr
				}
			}
		case kindCall:
			if arg.Fun != nil {
				// Recursively trace the function call's return value as a variable
				_, _, originType := metadata.TraceVariableOrigin(
					arg.Fun.Name,
					e.getString(node.Caller.Name),
					e.getString(node.Caller.Pkg),
					e.tree.meta,
				)
				var originTypeStr string
				if originType != nil {
					originTypeStr = e.callArgToString(*originType, nil)
				}

				if originTypeStr != "" {
					bodyType = originTypeStr
				}
			}
		case kindTypeAssert:
			if arg.Fun != nil && arg.Fun.Type != "" {
				bodyType = arg.Fun.Type
			}
		}
		// If the resolved type is a type parameter, try to resolve it using TypeParamMap
		if edge, ok := e.findCallGraphEdgeForNode(node); ok && bodyType != "" {
			if concrete, found := edge.TypeParamMap[bodyType]; found && concrete != "" {
				bodyType = concrete
			}
		}
		respInfo.BodyType = bodyType
		if pattern.Deref && strings.HasPrefix(respInfo.BodyType, "*") {
			respInfo.BodyType = strings.TrimPrefix(respInfo.BodyType, "*")
		}
	}

	respInfo.Schema = e.mapGoTypeToOpenAPISchema(respInfo.BodyType)

	return respInfo
}

// extractParametersFromNode extracts parameters from a node
func (e *Extractor) extractParametersFromNode(node *TrackerNode) []Parameter {
	var params []Parameter
	for _, pattern := range e.cfg.Framework.ParamPatterns {
		if e.matchParamPattern(node, pattern) {
			if param := e.extractParameterDetailsFromNode(node, pattern); param != nil {
				params = append(params, *param)
				break
			}
		}
	}

	return params
}

// extractParameterDetailsFromNode extracts parameter details from a node
func (e *Extractor) extractParameterDetailsFromNode(node *TrackerNode, pattern ParamPattern) *Parameter {
	param := &Parameter{
		In: pattern.ParamIn,
	}
	if len(node.CallGraphEdge.Args) > pattern.ParamArgIndex {
		param.Name = e.callArgToString(node.CallGraphEdge.Args[pattern.ParamArgIndex], nil)
	}
	if pattern.TypeFromArg && len(node.CallGraphEdge.Args) > pattern.TypeArgIndex {
		arg := node.CallGraphEdge.Args[pattern.TypeArgIndex]
		paramType := e.callArgToString(arg, nil)
		// --- Integration: Trace parameter type origin ---
		switch arg.Kind {
		case kindIdent:
			_, _, originType := metadata.TraceVariableOrigin(
				arg.Name,
				e.getString(node.Caller.Name),
				e.getString(node.Caller.Pkg),
				e.tree.meta,
			)
			var originTypeStr string
			if originType != nil {
				originTypeStr = e.callArgToString(*originType, nil)
			}

			if originTypeStr != "" {
				paramType = originTypeStr
			}
		case kindUnary, kindStar:
			if arg.X != nil {
				_, _, originType := metadata.TraceVariableOrigin(
					arg.X.Name,
					e.getString(node.Caller.Name),
					e.getString(node.Caller.Pkg),
					e.tree.meta,
				)
				var originTypeStr string
				if originType != nil {
					originTypeStr = e.callArgToString(*originType, nil)
				}

				if originTypeStr != "" {
					paramType = originTypeStr
				}
			}
		case kindSelector:
			if arg.X != nil {
				_, _, originType := metadata.TraceVariableOrigin(
					arg.X.Name,
					e.getString(node.Caller.Name),
					e.getString(node.Caller.Pkg),
					e.tree.meta,
				)
				var originTypeStr string
				if originType != nil {
					originTypeStr = e.callArgToString(*originType, nil)
				}

				if originTypeStr != "" {
					paramType = originTypeStr
				}
			}
		case kindCall:
			if arg.Fun != nil {
				// Recursively trace the function call's return value as a variable
				_, _, originType := metadata.TraceVariableOrigin(
					arg.Fun.Name,
					e.getString(node.Caller.Name),
					e.getString(node.Caller.Pkg),
					e.tree.meta,
				)
				var originTypeStr string
				if originType != nil {
					originTypeStr = e.callArgToString(*originType, nil)
				}

				if originTypeStr != "" {
					paramType = originTypeStr
				}
			}
		case kindTypeAssert:
			if arg.Fun != nil && arg.Fun.Type != "" {
				paramType = arg.Fun.Type
			}
		}
		// If the resolved type is a type parameter, try to resolve it using TypeParamMap
		if edge, ok := e.findCallGraphEdgeForNode(node); ok && paramType != "" {
			if concrete, found := edge.TypeParamMap[paramType]; found && concrete != "" {
				paramType = concrete
			}
		}
		if pattern.Deref && strings.HasPrefix(paramType, "*") {
			paramType = strings.TrimPrefix(paramType, "*")
		}
		param.Schema = e.mapGoTypeToOpenAPISchema(paramType)
	}
	// Ensure all parameters have a schema - default to string if none specified
	if param.Schema == nil {
		param.Schema = &Schema{Type: "string"}
	}
	// Ensure path parameters are always required
	if pattern.ParamIn == "path" {
		param.Required = true
	}
	return param
}

// applyOverrides applies manual overrides to route info
func (e *Extractor) applyOverrides(routeInfo *RouteInfo) {
	for _, override := range e.cfg.Overrides {
		if override.FunctionName == routeInfo.Function {
			if override.Summary != "" {
				routeInfo.Summary = override.Summary
			}
			if override.Description != "" {
				// Note: Description field not in RouteInfo, would need to add
			}
			if res, exists := routeInfo.Response[fmt.Sprintf("%d", override.ResponseStatus)]; exists && override.ResponseStatus != 0 && routeInfo.Response != nil {
				res.StatusCode = override.ResponseStatus
			}
			if override.ResponseType != "" && routeInfo.Response != nil {
				for _, res := range routeInfo.Response {
					res.BodyType = override.ResponseType
				}
			}
			if len(override.Tags) > 0 {
				routeInfo.Tags = override.Tags
			}
		}
	}
}

// Helper methods

// getString gets a string from the string pool
func (e *Extractor) getString(idx int) string {
	if e.tree.meta.StringPool == nil {
		return ""
	}
	return e.tree.meta.StringPool.GetString(idx)
}

// callArgToString converts a call argument to a string representation for OpenAPI extraction.
func (e *Extractor) callArgToString(arg metadata.CallArgument, sep *string) string {
	// Use provided separator or default
	separator := defaultSep
	if sep != nil && *sep != "" {
		separator = *sep
	}

	switch arg.Kind {
	case kindLiteral:
		// Remove quotes from string literals
		return strings.Trim(arg.Value, "\"")

	case kindKeyValue:
		return ""

	case kindMapType:
		if arg.X != nil && arg.Fun != nil {
			return fmt.Sprintf("map[%s]%s", e.callArgToString(*arg.X, nil), e.callArgToString(*arg.Fun, nil))
		}
		return "map"

	case kindUnary:
		// Handle unary expressions (e.g., *X)
		if arg.X != nil {
			return "*" + e.callArgToString(*arg.X, nil)
		}
		return "*"
	case kindIndex:
		// Handle unary expressions (e.g., *X)
		if arg.X != nil {
			return "*" + e.callArgToString(*arg.X, nil)
		}
		return "*"
	case kindCompositeLit:
		if arg.X != nil {
			return e.callArgToString(*arg.X, nil)
		}
		return ""

	case kindIdent:
		if arg.Pkg == "net/http" && strings.HasPrefix(arg.Name, "Status") {
			return arg.Pkg + defaultSep + arg.Name
		}

		// Try to resolve as a constant value from metadata
		if pkg, exists := e.tree.meta.Packages[arg.Pkg]; exists {
			for _, file := range pkg.Files {
				if variable, exists := file.Variables[arg.Name]; exists && e.getString(variable.Tok) == "const" {
					return strings.Trim(e.getString(variable.Value), "\"")
				}
			}
		}
		// If not a function type, build a qualified type string
		if !strings.HasPrefix(arg.Type, "func(") {
			if arg.Type != "" {
				// Check if this is a built-in Go type that doesn't need package prefix
				builtinTypes := []string{
					"string", "int", "int8", "int16", "int32", "int64",
					"uint", "uint8", "uint16", "uint32", "uint64",
					"float32", "float64", "bool", "byte", "rune",
					"error", "interface{}", "any",
				}

				// Check for map types (built-in)
				if strings.HasPrefix(arg.Type, "map[") {
					return arg.Type
				}

				// Check for slice types with built-in element types
				if strings.HasPrefix(arg.Type, "[]") {
					elementType := strings.TrimPrefix(arg.Type, "[]")
					elementType = strings.TrimPrefix(elementType, "*")
					for _, builtin := range builtinTypes {
						if elementType == builtin {
							return arg.Type
						}
					}
				}

				// Check for pointer types with built-in base types
				if strings.HasPrefix(arg.Type, "*") {
					baseType := strings.TrimPrefix(arg.Type, "*")
					for _, builtin := range builtinTypes {
						if baseType == builtin {
							return arg.Type
						}
					}
				}

				// Check if it's a built-in type
				for _, builtin := range builtinTypes {
					if arg.Type == builtin {
						return arg.Type
					}
				}

				// If we have a package and type, process as custom type
				if arg.Pkg != "" {
					// Remove slice, pointer, and redundant package prefixes
					argType := strings.TrimPrefix(arg.Type, "[]")
					argType = strings.TrimPrefix(argType, "*")
					argType = strings.TrimPrefix(argType, arg.Pkg+separator)

					// Add only if the pkg is deattached from the type
					if !strings.Contains(argType, "/") {
						// Re-add package prefix
						argType = arg.Pkg + TypeSep + argType
					}

					// If original type was a slice, add [] prefix
					if strings.HasPrefix(arg.Type, "[]") {
						argType = "[]" + argType
					}
					return argType
				}

				// If no package but has type, return as is
				return arg.Type
			}
		}

		if arg.Pkg != "" {
			return arg.Pkg + separator + arg.Name
		}

		// Fallback to variable name
		return arg.Name

	case kindSelector:
		// Handle selector expressions (e.g., pkg.X.Sel)
		if arg.X != nil {
			pkgKey := arg.X.Pkg + slashSep + arg.X.Name
			if pkg, exists := e.tree.meta.Packages[pkgKey]; exists {
				for _, file := range pkg.Files {
					if variable, exists := file.Variables[arg.Sel]; exists {
						return strings.Trim(e.getString(variable.Value), "\"")
					}
				}
			}
			xResult := e.callArgToString(*arg.X, strPtr(slashSep))
			if xResult != "" {
				return xResult + defaultSep + arg.Sel
			}
		}
		return arg.Sel

	case kindCall:
		// Handle function call expressions
		if arg.Fun != nil {
			return e.callArgToString(*arg.Fun, nil)
		}
		return callEllipsis

	case kindInterfaceType:
		// interface{}
		return "interface{}"
	case kindRaw:
		// Raw string value
		return arg.Raw
	}
	// Fallback for unknown kinds
	return ""
}

// strPtr returns a pointer to the given string (helper for separator passing)
func strPtr(s string) *string { return &s }

// matchPattern checks if a pattern matches a value
func (e *Extractor) matchPattern(pattern, value string) bool {
	if pattern == "" {
		return false
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return false
	}
	return re.MatchString(value)
}

// matchRoutePattern checks if a node matches a route pattern
func (e *Extractor) matchRoutePattern(node *TrackerNode, pattern RoutePattern) bool {
	if node == nil || node.CallGraphEdge == nil {
		return false
	}
	callName := e.getString(node.CallGraphEdge.Callee.Name)
	recvType := e.getString(node.CallGraphEdge.Callee.RecvType)
	recvPkg := e.getString(node.CallGraphEdge.Callee.Pkg)
	fqRecvType := recvPkg
	if fqRecvType != "" && recvType != "" {
		fqRecvType += "." + recvType
	} else if recvType != "" {
		fqRecvType = recvType
	}
	if pattern.CallRegex != "" && !pattern.MatchCallRegex(callName) {
		return false
	}
	if pattern.FunctionNameRegex != "" {
		funcName := e.getString(node.CallGraphEdge.Caller.Name)
		if !pattern.MatchFunctionName(funcName) {
			return false
		}
	}
	// If RecvTypeRegex is set, use regex matching for receiver type
	if pattern.RecvTypeRegex != "" {
		matched, err := regexp.MatchString(pattern.RecvTypeRegex, fqRecvType)
		if err != nil || !matched {
			return false
		}
	} else if pattern.RecvType != "" && pattern.RecvType != fqRecvType {
		return false
	}
	return true
}

// matchRequestBodyPattern checks if a node matches a request body pattern
func (e *Extractor) matchRequestBodyPattern(node *TrackerNode, pattern RequestBodyPattern, route *RouteInfo) bool {
	if node == nil || node.CallGraphEdge == nil {
		return false
	}
	callName := e.getString(node.CallGraphEdge.Callee.Name)
	recvType := e.getString(node.CallGraphEdge.Callee.RecvType)
	recvPkg := e.getString(node.CallGraphEdge.Callee.Pkg)
	fqRecvType := recvPkg
	if fqRecvType != "" && recvType != "" {
		fqRecvType += "." + recvType
	} else if recvType != "" {
		fqRecvType = recvType
	}
	if pattern.CallRegex != "" && !e.matchPattern(pattern.CallRegex, callName) {
		return false
	}
	if pattern.FunctionNameRegex != "" {
		funcName := e.getString(node.CallGraphEdge.Caller.Name)
		if !e.matchPattern(pattern.FunctionNameRegex, funcName) {
			return false
		}
	}
	// If RecvTypeRegex is set, use regex matching for receiver type
	if pattern.RecvTypeRegex != "" {
		matched, err := regexp.MatchString(pattern.RecvTypeRegex, fqRecvType)
		if err != nil || !matched {
			return false
		}
	} else if pattern.RecvType != "" && pattern.RecvType != fqRecvType {
		return false
	}
	// Context-aware validation: don't match request body patterns for GET/HEAD/DELETE methods
	// unless explicitly allowed by the pattern
	if !pattern.AllowForGetMethods {
		if route.Method == http.MethodGet || route.Method == http.MethodHead || route.Method == http.MethodDelete {
			return false
		}
	}
	return true
}

// matchResponsePattern checks if a node matches a response pattern
func (e *Extractor) matchResponsePattern(node *TrackerNode, pattern ResponsePattern) bool {
	if node == nil || node.CallGraphEdge == nil {
		return false
	}
	callName := e.getString(node.CallGraphEdge.Callee.Name)
	recvType := e.getString(node.CallGraphEdge.Callee.RecvType)
	recvPkg := e.getString(node.CallGraphEdge.Callee.Pkg)
	fqRecvType := recvPkg
	if fqRecvType != "" && recvType != "" {
		fqRecvType += "." + recvType
	} else if recvType != "" {
		fqRecvType = recvType
	}
	if pattern.CallRegex != "" && !e.matchPattern(pattern.CallRegex, callName) {
		return false
	}
	if pattern.FunctionNameRegex != "" {
		funcName := e.getString(node.CallGraphEdge.Caller.Name)
		if !e.matchPattern(pattern.FunctionNameRegex, funcName) {
			return false
		}
	}
	// If RecvTypeRegex is set, use regex matching for receiver type
	if pattern.RecvTypeRegex != "" {
		matched, err := regexp.MatchString(pattern.RecvTypeRegex, fqRecvType)
		if err != nil || !matched {
			return false
		}
	} else if pattern.RecvType != "" && pattern.RecvType != fqRecvType {
		return false
	}
	return true
}

// matchParamPattern checks if a node matches a parameter pattern
func (e *Extractor) matchParamPattern(node *TrackerNode, pattern ParamPattern) bool {
	if node == nil || node.CallGraphEdge == nil {
		return false
	}
	callName := e.getString(node.CallGraphEdge.Callee.Name)
	recvType := e.getString(node.CallGraphEdge.Callee.RecvType)
	recvPkg := e.getString(node.CallGraphEdge.Callee.Pkg)
	fqRecvType := recvPkg
	if fqRecvType != "" && recvType != "" {
		fqRecvType += "." + recvType
	} else if recvType != "" {
		fqRecvType = recvType
	}
	if pattern.CallRegex != "" && !e.matchPattern(pattern.CallRegex, callName) {
		return false
	}
	if pattern.FunctionNameRegex != "" {
		funcName := e.getString(node.CallGraphEdge.Caller.Name)
		if !e.matchPattern(pattern.FunctionNameRegex, funcName) {
			return false
		}
	}
	// If RecvTypeRegex is set, use regex matching for receiver type
	if pattern.RecvTypeRegex != "" {
		matched, err := regexp.MatchString(pattern.RecvTypeRegex, fqRecvType)
		if err != nil || !matched {
			return false
		}
	} else if pattern.RecvType != "" && pattern.RecvType != fqRecvType {
		return false
	}
	return true
}

// extractMethodFromFunctionName extracts HTTP method from function name
func (e *Extractor) extractMethodFromFunctionName(funcName string) string {
	methods := []string{"GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS", "HEAD"}
	for _, method := range methods {
		if strings.Contains(strings.ToUpper(funcName), method) {
			return method
		}
	}
	return ""
}

// parseStatusCode parses a status code string
func (e *Extractor) parseStatusCode(statusStr string) (int, bool) {
	// Remove quotes if present
	statusStr = strings.Trim(statusStr, "\"")

	statusStr = strings.TrimPrefix(statusStr, "net/http.")

	// Check for net/http status constants
	switch statusStr {
	case "StatusOK":
		return http.StatusOK, true
	case "StatusCreated":
		return http.StatusCreated, true
	case "StatusAccepted":
		return http.StatusAccepted, true
	case "StatusNoContent":
		return http.StatusNoContent, true
	case "StatusBadRequest":
		return http.StatusBadRequest, true
	case "StatusUnauthorized":
		return http.StatusUnauthorized, true
	case "StatusForbidden":
		return http.StatusForbidden, true
	case "StatusNotFound":
		return http.StatusNotFound, true
	case "StatusConflict":
		return http.StatusConflict, true
	case "StatusInternalServerError":
		return http.StatusInternalServerError, true
	case "StatusNotImplemented":
		return http.StatusNotImplemented, true
	case "StatusBadGateway":
		return http.StatusBadGateway, true
	case "StatusServiceUnavailable":
		return http.StatusServiceUnavailable, true
	}

	// Try to parse as integer
	var status int
	_, err := fmt.Sscanf(statusStr, "%d", &status)
	if err != nil {
		return 0, false
	}

	return status, true
}

// mapGoTypeToOpenAPISchema maps Go types to OpenAPI schemas
func (e *Extractor) mapGoTypeToOpenAPISchema(goType string) *Schema {
	// Check type mappings first
	for _, mapping := range e.cfg.TypeMapping {
		if mapping.GoType == goType {
			return mapping.OpenAPIType
		}
	}

	// Handle pointer types
	if strings.HasPrefix(goType, "*") {
		underlyingType := strings.TrimSpace(goType[1:])
		// For pointer types, we generate the same schema as the underlying type
		return e.mapGoTypeToOpenAPISchema(underlyingType)
	}

	// Handle map types
	if strings.HasPrefix(goType, "map[") {
		endIdx := strings.Index(goType, "]")
		if endIdx > 4 {
			keyType := goType[4:endIdx]
			valueType := strings.TrimSpace(goType[endIdx+1:])
			if keyType == "string" {
				// Handle specific value types for string-keyed maps
				switch valueType {
				case "string":
					return &Schema{
						Type:                 "object",
						AdditionalProperties: &Schema{Type: "string"},
					}
				case "interface{}", "any":
					// For interface{}, allow any type
					return &Schema{
						Type:                 "object",
						AdditionalProperties: &Schema{}, // Empty schema allows any type
					}
				case "int", "int8", "int16", "int32", "int64":
					return &Schema{
						Type:                 "object",
						AdditionalProperties: &Schema{Type: "integer"},
					}
				case "uint", "uint8", "uint16", "uint32", "uint64", "byte":
					return &Schema{
						Type:                 "object",
						AdditionalProperties: &Schema{Type: "integer", Minimum: 0},
					}
				case "float32", "float64":
					return &Schema{
						Type:                 "object",
						AdditionalProperties: &Schema{Type: "number"},
					}
				case "bool":
					return &Schema{
						Type:                 "object",
						AdditionalProperties: &Schema{Type: "boolean"},
					}
				default:
					// For custom types, recursively map the value type
					return &Schema{
						Type:                 "object",
						AdditionalProperties: e.mapGoTypeToOpenAPISchema(valueType),
					}
				}
			}
			// Non-string keys are not supported in OpenAPI, fallback to generic object
			return &Schema{Type: "object"}
		}
	}

	// Handle slice/array types
	if strings.HasPrefix(goType, "[]") {
		elemType := strings.TrimSpace(goType[2:])
		// For basic types, create inline array schema
		switch elemType {
		case "string":
			return &Schema{Type: "array", Items: &Schema{Type: "string"}}
		case "int", "int8", "int16", "int32", "int64":
			return &Schema{Type: "array", Items: &Schema{Type: "integer"}}
		case "uint", "uint8", "uint16", "uint32", "uint64", "byte":
			return &Schema{Type: "array", Items: &Schema{Type: "integer", Minimum: 0}}
		case "float32", "float64":
			return &Schema{Type: "array", Items: &Schema{Type: "number"}}
		case "bool":
			return &Schema{Type: "array", Items: &Schema{Type: "boolean"}}
		default:
			// For custom types, create a reference
			return &Schema{
				Type: "array",
				Items: &Schema{
					Ref: "#/components/schemas/" + schemaComponentNameReplacer.Replace(elemType),
				},
			}
		}
	}

	// Default mappings
	switch goType {
	case "string":
		return &Schema{Type: "string"}
	case "int", "int8", "int16", "int32", "int64":
		return &Schema{Type: "integer"}
	case "uint", "uint8", "uint16", "uint32", "uint64", "byte":
		return &Schema{Type: "integer", Minimum: 0}
	case "float32", "float64":
		return &Schema{Type: "number"}
	case "bool":
		return &Schema{Type: "boolean"}
	case "[]byte":
		return &Schema{Type: "string", Format: "byte"}
	case "[]string":
		return &Schema{Type: "array", Items: &Schema{Type: "string"}}
	case "[]int":
		return &Schema{Type: "array", Items: &Schema{Type: "integer"}}
	case "interface{}", "any":
		// For standalone interface{}, allow any type
		return &Schema{}
	default:
		if goType != "" {
			// For custom types, create a reference
			return &Schema{Ref: "#/components/schemas/" + schemaComponentNameReplacer.Replace(goType)}
		}

		return nil
	}
}

// matchMountPattern returns the first matching mount pattern and true if node is a mount
func (e *Extractor) matchMountPattern(node *TrackerNode) (MountPattern, bool) {
	for _, pattern := range e.cfg.Framework.MountPatterns {
		if e.matchMountNode(node, pattern) {
			return pattern, true
		}
	}
	return MountPattern{}, false
}

// matchMountNode checks if a node matches a mount pattern
func (e *Extractor) matchMountNode(node *TrackerNode, pattern MountPattern) bool {
	if node == nil || node.CallGraphEdge == nil {
		return false
	}
	callName := e.getString(node.CallGraphEdge.Callee.Name)
	recvType := e.getString(node.CallGraphEdge.Callee.RecvType)
	recvPkg := e.getString(node.CallGraphEdge.Callee.Pkg)
	fqRecvType := recvPkg
	if fqRecvType != "" && recvType != "" {
		fqRecvType += "." + recvType
	} else if recvType != "" {
		fqRecvType = recvType
	}
	if pattern.CallRegex != "" && !e.matchPattern(pattern.CallRegex, callName) {
		return false
	}
	if pattern.FunctionNameRegex != "" {
		funcName := e.getString(node.CallGraphEdge.Caller.Name)
		if !e.matchPattern(pattern.FunctionNameRegex, funcName) {
			return false
		}
	}
	// If RecvTypeRegex is set, use regex matching for receiver type
	if pattern.RecvTypeRegex != "" {
		matched, err := regexp.MatchString(pattern.RecvTypeRegex, fqRecvType)
		if err != nil || !matched {
			return false
		}
	} else if pattern.RecvType != "" && pattern.RecvType != fqRecvType {
		return false
	}
	return pattern.IsMount
}

// matchRoutePatternWithPattern returns the first matching route pattern and true if node is a route
func (e *Extractor) matchRoutePatternWithPattern(node *TrackerNode) (RoutePattern, bool) {
	for _, pattern := range e.cfg.Framework.RoutePatterns {
		if e.matchRoutePattern(node, pattern) {
			return pattern, true
		}
	}
	return RoutePattern{}, false
}

// extractRouteChildren extracts request, response, and params from children nodes
func (e *Extractor) extractRouteChildren(routeNode *TrackerNode, route *RouteInfo) {
	for _, child := range routeNode.children {
		// Request
		if req := e.extractRequestInfoFromNode(child, route); req != nil {
			route.Request = req
		}
		// Response
		if resp := e.extractResponseInfoFromNode(child); resp != nil && resp.BodyType != "" {
			route.Response[fmt.Sprintf("%d", resp.StatusCode)] = resp
		}
		// Params (from children)
		for _, pattern := range e.cfg.Framework.ParamPatterns {
			if e.matchParamPattern(child, pattern) {
				if param := e.extractParameterDetailsFromNode(child, pattern); param != nil {
					route.Params = append(route.Params, *param)
					break
				}
			}
		}

		// Recursive for getting inner
		e.extractRouteChildren(child, route)
	}
	// Params (from route node itself)
	for _, pattern := range e.cfg.Framework.ParamPatterns {
		if e.matchParamPattern(routeNode, pattern) {
			if param := e.extractParameterDetailsFromNode(routeNode, pattern); param != nil {
				route.Params = append(route.Params, *param)
				break
			}
		}
	}
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

func (e *Extractor) findAssignmentFunction(arg metadata.CallArgument) *metadata.CallArgument {
	// Look up for calls that assign to this argument
	// Return the function name for the argument that should be used for mount instead of the arg.
	for _, edge := range e.tree.meta.CallGraph {
		for _, varAssignments := range edge.AssignmentMap {
			for _, assign := range varAssignments {
				varName := e.getString(assign.VariableName)
				varType := e.getString(assign.ConcreteType)
				varPkg := e.getString(assign.Pkg)

				if varName == arg.Name && varPkg == arg.Pkg && varType == arg.X.Type {
					// Get the function name directly (it's already a string)
					// TODO: search for argument function that is containing the routes and pass it directly
					// by searching for callee as a caller
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

// Add helper to find the call graph edge for a node
func (e *Extractor) findCallGraphEdgeForNode(node *TrackerNode) (*metadata.CallGraphEdge, bool) {
	if node == nil || node.CallGraphEdge == nil {
		return nil, false
	}
	return node.CallGraphEdge, true
}

// Helper to extract a string representation from a *metadata.CallArgument for type usage
func extractTypeString(arg *metadata.CallArgument) string {
	if arg == nil {
		return ""
	}
	if arg.Type != "" {
		return arg.Type
	}
	if arg.Name != "" {
		return arg.Name
	}
	if arg.Value != "" {
		return arg.Value
	}
	return ""
}
