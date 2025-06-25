package core

import (
	"go/ast"
	"go/token"
	"go/types"
)

// RouteStrategy defines the interface for different route parsing strategies
type RouteStrategy interface {
	TryParse(node ast.Node, fset *token.FileSet, funcMap map[string]*ast.FuncDecl, goFiles []*ast.File, next RouteStrategy, info *types.Info) (*ParsedRoute, bool)
}

// EventListener defines the interface for parser events
type EventListener interface {
	OnRouteParsed(route *ParsedRoute)
}

// ParsedRoute represents a parsed API route
type ParsedRoute struct {
	Method        string
	Path          string
	HandlerName   string
	RequestType   string
	RequestSource string
	ResponseTypes []ResponseInfo
	Position      token.Position
}

// ResponseInfo represents response information
type ResponseInfo struct {
	StatusCode int
	Type       string
	MediaType  string
	MapKeys    map[string]string
}

// ParsedRouteBuilder provides a fluent interface for building ParsedRoute
type ParsedRouteBuilder struct {
	Route *ParsedRoute
	Info  *types.Info
}

// NewParsedRouteBuilder creates a new ParsedRouteBuilder
func NewParsedRouteBuilder(info *types.Info) *ParsedRouteBuilder {
	return &ParsedRouteBuilder{
		Route: &ParsedRoute{},
		Info:  info,
	}
}

// Method sets the HTTP method
func (b *ParsedRouteBuilder) Method(method string) *ParsedRouteBuilder {
	b.Route.Method = method
	return b
}

// Path sets the route path
func (b *ParsedRouteBuilder) Path(path string) *ParsedRouteBuilder {
	b.Route.Path = path
	return b
}

// Handler sets the handler name and function
func (b *ParsedRouteBuilder) Handler(name string, fn *ast.FuncDecl) *ParsedRouteBuilder {
	b.Route.HandlerName = name
	return b
}

// Position sets the position information
func (b *ParsedRouteBuilder) Position(fset *token.FileSet, pos token.Pos) *ParsedRouteBuilder {
	b.Route.Position = fset.Position(pos)
	return b
}

// SetRequestBody sets the request body type and source
func (b *ParsedRouteBuilder) SetRequestBody(reqType, reqSource string) *ParsedRouteBuilder {
	b.Route.RequestType = reqType
	b.Route.RequestSource = reqSource
	return b
}

// SetResponses sets the response types
func (b *ParsedRouteBuilder) SetResponses(responses []ResponseInfo) *ParsedRouteBuilder {
	b.Route.ResponseTypes = responses
	return b
}

// Build returns the built ParsedRoute
func (b *ParsedRouteBuilder) Build() *ParsedRoute {
	return b.Route
}

// StrategyChain represents a chain of parsing strategies
type StrategyChain struct {
	strategies []RouteStrategy
}

// NewStrategyChain creates a new strategy chain
func NewStrategyChain(strategies ...RouteStrategy) *StrategyChain {
	return &StrategyChain{strategies: strategies}
}

// TryParse attempts to parse using each strategy in the chain
func (c *StrategyChain) TryParse(node ast.Node, fset *token.FileSet, funcMap map[string]*ast.FuncDecl, goFiles []*ast.File, next RouteStrategy, info *types.Info) (*ParsedRoute, bool) {
	for _, strategy := range c.strategies {
		if route, ok := strategy.TryParse(node, fset, funcMap, goFiles, next, info); ok {
			return route, true
		}
	}
	return nil, false
}
