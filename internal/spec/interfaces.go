package spec

import (
	"github.com/ehabterra/swagen/internal/metadata"
)

// PatternMatcher defines the interface for pattern matching operations
type PatternMatcher interface {
	// MatchNode checks if a node matches a specific pattern
	MatchNode(node TrackerNodeInterface) bool

	// GetPattern returns the pattern that was matched
	GetPattern() interface{}

	// GetPriority returns the priority of this pattern (higher = more specific)
	GetPriority() int
}

// RoutePatternMatcher matches route patterns
type RoutePatternMatcher interface {
	PatternMatcher

	// ExtractRoute extracts route information from a matched node
	ExtractRoute(node TrackerNodeInterface) RouteInfo
}

// MountPatternMatcher matches mount patterns
type MountPatternMatcher interface {
	PatternMatcher

	// ExtractMount extracts mount information from a matched node
	ExtractMount(node TrackerNodeInterface) MountInfo
}

// RequestPatternMatcher matches request body patterns
type RequestPatternMatcher interface {
	PatternMatcher

	// ExtractRequest extracts request information from a matched node
	ExtractRequest(node TrackerNodeInterface, route *RouteInfo) *RequestInfo
}

// ResponsePatternMatcher matches response patterns
type ResponsePatternMatcher interface {
	PatternMatcher

	// ExtractResponse extracts response information from a matched node
	ExtractResponse(node TrackerNodeInterface) *ResponseInfo
}

// ParamPatternMatcher matches parameter patterns
type ParamPatternMatcher interface {
	PatternMatcher

	// ExtractParam extracts parameter information from a matched node
	ExtractParam(node TrackerNodeInterface) *Parameter
}

// TypeResolver defines the interface for type resolution operations
type TypeResolver interface {
	// ResolveType resolves a Go type to its concrete type
	ResolveType(arg metadata.CallArgument, context TrackerNodeInterface) string

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
	ExtractRouteFromNode(node TrackerNodeInterface, pattern RoutePattern) RouteInfo

	// TraverseForRoutes traverses the tree to find routes
	TraverseForRoutes(node TrackerNodeInterface, mountPath string, mountTags []string, routes *[]RouteInfo)
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
	ExecuteRoutePattern(node TrackerNodeInterface) (RouteInfo, bool)

	// ExecuteMountPattern executes a mount pattern match
	ExecuteMountPattern(node TrackerNodeInterface) (MountInfo, bool)

	// ExecuteRequestPattern executes a request pattern match
	ExecuteRequestPattern(node TrackerNodeInterface, route *RouteInfo) (*RequestInfo, bool)

	// ExecuteResponsePattern executes a response pattern match
	ExecuteResponsePattern(node TrackerNodeInterface) (*ResponseInfo, bool)

	// ExecuteParamPattern executes a parameter pattern match
	ExecuteParamPattern(node TrackerNodeInterface) (*Parameter, bool)
}

// ContextProvider defines the interface for providing context information
type ContextProvider interface {
	// GetString gets a string from the string pool
	GetString(idx int) string

	// GetCalleeInfo gets callee information from a node
	GetCalleeInfo(node TrackerNodeInterface) (name, pkg, recvType string)

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

// TrackerNodeInterface defines the interface for tracker tree nodes
type TrackerNodeInterface interface {
	// GetKey returns the unique key of the node
	GetKey() string

	// GetParent returns the parent node
	GetParent() TrackerNodeInterface

	// GetChildren returns the children nodes
	GetChildren() []TrackerNodeInterface

	// GetEdge returns the call graph edge
	GetEdge() *metadata.CallGraphEdge

	// GetArgument returns the call argument
	GetArgument() *metadata.CallArgument

	// GetArgType returns the argument type
	GetArgType() metadata.ArgumentType

	// GetArgIndex returns the argument index
	GetArgIndex() int

	// GetArgContext returns the argument context
	GetArgContext() string

	// GetTypeParamMap returns the type parameter map
	GetTypeParamMap() map[string]string

	// GetRootAssignmentMap returns the root assignment map
	GetRootAssignmentMap() map[string][]metadata.Assignment
}

// TrackerTreeInterface defines the interface for tracker tree operations
type TrackerTreeInterface interface {
	// GetRoots returns the root nodes of the tracker tree
	GetRoots() []TrackerNodeInterface

	// GetNodeCount returns the total number of nodes in the tree
	GetNodeCount() int

	// FindNodeByKey finds a node by its key
	FindNodeByKey(key string) TrackerNodeInterface

	// GetFunctionContext returns context information for a function
	GetFunctionContext(functionName string) (*metadata.Function, string, string)

	// TraverseTree traverses the tree with a visitor function
	TraverseTree(visitor func(node TrackerNodeInterface) bool)

	// GetMetadata returns the underlying metadata
	GetMetadata() *metadata.Metadata

	// GetLimits returns the tracker limits
	GetLimits() metadata.TrackerLimits
}
