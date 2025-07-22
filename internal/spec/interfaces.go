package spec

import (
	"github.com/ehabterra/swagen/internal/metadata"
)

// PatternMatcher defines the interface for pattern matching operations
type PatternMatcher interface {
	// MatchNode checks if a node matches a specific pattern
	MatchNode(node *TrackerNode) bool

	// GetPattern returns the pattern that was matched
	GetPattern() interface{}

	// GetPriority returns the priority of this pattern (higher = more specific)
	GetPriority() int
}

// RoutePatternMatcher matches route patterns
type RoutePatternMatcher interface {
	PatternMatcher

	// ExtractRoute extracts route information from a matched node
	ExtractRoute(node *TrackerNode) RouteInfo
}

// MountPatternMatcher matches mount patterns
type MountPatternMatcher interface {
	PatternMatcher

	// ExtractMount extracts mount information from a matched node
	ExtractMount(node *TrackerNode) MountInfo
}

// RequestPatternMatcher matches request body patterns
type RequestPatternMatcher interface {
	PatternMatcher

	// ExtractRequest extracts request information from a matched node
	ExtractRequest(node *TrackerNode, route *RouteInfo) *RequestInfo
}

// ResponsePatternMatcher matches response patterns
type ResponsePatternMatcher interface {
	PatternMatcher

	// ExtractResponse extracts response information from a matched node
	ExtractResponse(node *TrackerNode) *ResponseInfo
}

// ParamPatternMatcher matches parameter patterns
type ParamPatternMatcher interface {
	PatternMatcher

	// ExtractParam extracts parameter information from a matched node
	ExtractParam(node *TrackerNode) *Parameter
}

// TypeResolver defines the interface for type resolution operations
type TypeResolver interface {
	// ResolveType resolves a Go type to its concrete type
	ResolveType(arg metadata.CallArgument, context *TrackerNode) string

	// MapToOpenAPISchema maps a Go type to OpenAPI schema
	MapToOpenAPISchema(goType string) *Schema
}

// VariableTracer defines the interface for variable tracing operations
type VariableTracer interface {
	// TraceVariable traces a variable back to its origin
	TraceVariable(varName, funcName, pkgName string) (originVar, originPkg string, originType *metadata.CallArgument)

	// FindAssignmentFunction finds the assignment function for a variable
	FindAssignmentFunction(arg metadata.CallArgument) *metadata.CallArgument
}

// RouteExtractor defines the interface for route extraction operations
type RouteExtractor interface {
	// ExtractRoutes extracts all routes from the tracker tree
	ExtractRoutes() []RouteInfo

	// ExtractRouteFromNode extracts a single route from a node
	ExtractRouteFromNode(node *TrackerNode, pattern RoutePattern) RouteInfo

	// TraverseForRoutes traverses the tree to find routes
	TraverseForRoutes(node *TrackerNode, mountPath string, mountTags []string, routes *[]RouteInfo)
}

// MountInfo represents extracted mount information
type MountInfo struct {
	Path       string
	RouterArg  *metadata.CallArgument
	Assignment *metadata.CallArgument
	Pattern    MountPattern
}

// PatternExecutor defines the interface for pattern execution
type PatternExecutor interface {
	// ExecuteRoutePattern executes a route pattern match
	ExecuteRoutePattern(node *TrackerNode) (RouteInfo, bool)

	// ExecuteMountPattern executes a mount pattern match
	ExecuteMountPattern(node *TrackerNode) (MountInfo, bool)

	// ExecuteRequestPattern executes a request pattern match
	ExecuteRequestPattern(node *TrackerNode, route *RouteInfo) (*RequestInfo, bool)

	// ExecuteResponsePattern executes a response pattern match
	ExecuteResponsePattern(node *TrackerNode) (*ResponseInfo, bool)

	// ExecuteParamPattern executes a parameter pattern match
	ExecuteParamPattern(node *TrackerNode) (*Parameter, bool)
}

// ContextProvider defines the interface for providing context information
type ContextProvider interface {
	// GetString gets a string from the string pool
	GetString(idx int) string

	// GetCallerInfo gets caller information from a node
	GetCallerInfo(node *TrackerNode) (name, pkg string)

	// GetCalleeInfo gets callee information from a node
	GetCalleeInfo(node *TrackerNode) (name, pkg, recvType string)

	// GetArgumentInfo gets argument information
	GetArgumentInfo(arg metadata.CallArgument) string
}

// SchemaMapper defines the interface for schema mapping operations
type SchemaMapper interface {
	// MapGoTypeToOpenAPISchema maps a Go type to OpenAPI schema
	MapGoTypeToOpenAPISchema(goType string) *Schema

	// MapStatusCode maps a status code string to HTTP status code
	MapStatusCode(statusStr string) (int, bool)

	// MapMethodFromFunctionName extracts HTTP method from function name
	MapMethodFromFunctionName(funcName string) string
}

// OverrideApplier defines the interface for applying overrides
type OverrideApplier interface {
	// ApplyOverrides applies manual overrides to route info
	ApplyOverrides(routeInfo *RouteInfo)

	// HasOverride checks if there's an override for a function
	HasOverride(functionName string) bool
}
