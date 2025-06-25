package parser

import (
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	"github.com/ehabterra/swagen/internal/core"
)

// --- Event System (Observer Pattern) ---
type ParseEvent string

const (
	EventRouteFound ParseEvent = "route_found"
)

type EventListener func(event ParseEvent, route *core.ParsedRoute)

// --- Chain of Responsibility for Strategies ---
type RouteStrategy interface {
	TryParse(node ast.Node, fset *token.FileSet, funcMap map[string]*ast.FuncDecl, goFiles []*ast.File, next RouteStrategy, info *types.Info) (*core.ParsedRoute, bool)
}

type StrategyChain struct {
	strategies []RouteStrategy
}

func (c *StrategyChain) TryParse(node ast.Node, fset *token.FileSet, funcMap map[string]*ast.FuncDecl, goFiles []*ast.File, _ RouteStrategy, info *types.Info) (*core.ParsedRoute, bool) {
	return c.tryParseAt(0, node, fset, funcMap, goFiles, info)
}
func (c *StrategyChain) tryParseAt(i int, node ast.Node, fset *token.FileSet, funcMap map[string]*ast.FuncDecl, goFiles []*ast.File, info *types.Info) (*core.ParsedRoute, bool) {
	if i >= len(c.strategies) {
		return nil, false
	}
	return c.strategies[i].TryParse(node, fset, funcMap, goFiles, &strategyChainNext{c, i + 1}, info)
}

type strategyChainNext struct {
	chain *StrategyChain
	index int
}

func (n *strategyChainNext) TryParse(node ast.Node, fset *token.FileSet, funcMap map[string]*ast.FuncDecl, goFiles []*ast.File, _ RouteStrategy, info *types.Info) (*core.ParsedRoute, bool) {
	return n.chain.tryParseAt(n.index, node, fset, funcMap, goFiles, info)
}

// --- GinCallExprStrategy (example strategy) ---

type GinCallExprStrategy struct{}

func (s *GinCallExprStrategy) TryParse(node ast.Node, fset *token.FileSet, funcMap map[string]*ast.FuncDecl, goFiles []*ast.File, next core.RouteStrategy, info *types.Info) (*core.ParsedRoute, bool) {
	call, ok := node.(*ast.CallExpr)
	if !ok {
		if next != nil {
			return next.TryParse(node, fset, funcMap, goFiles, next, info)
		}
		return nil, false
	}

	// Check if it's a Gin router method call
	if se, ok := call.Fun.(*ast.SelectorExpr); ok {
		if isGinHTTPMethod(se.Sel.Name) {
			if len(call.Args) >= 2 {
				// Extract path from string literal
				pathLit, ok := call.Args[0].(*ast.BasicLit)
				if !ok || pathLit.Kind != token.STRING {
					if next != nil {
						return next.TryParse(node, fset, funcMap, goFiles, next, info)
					}
					return nil, false
				}
				path := strings.Trim(pathLit.Value, "\"")

				handlerName := getHandlerName(call.Args[1])

				if handlerName != "" {
					if handlerFunc, exists := funcMap[handlerName]; exists {
						requestType := extractGinRequestBodyType(handlerFunc, goFiles)
						responses := extractGinResponseTypes(handlerFunc, goFiles, info)

						builder := core.NewParsedRouteBuilder(info)
						route := builder.Method(se.Sel.Name).
							Path(path).
							Handler(handlerName, handlerFunc).
							SetRequestBody(requestType, "body").
							SetResponses(responses).
							Position(fset, call.Pos()).
							Build()

						return route, true
					}
				}
			}
		}
	}

	if next != nil {
		return next.TryParse(node, fset, funcMap, goFiles, next, info)
	}
	return nil, false
}

type GinParser struct {
	chain     *core.StrategyChain
	listeners []core.EventListener
	info      *types.Info // for type resolution
}

func NewGinParserWithTypes(strategies []core.RouteStrategy, info *types.Info, listeners ...core.EventListener) *GinParser {
	return &GinParser{
		chain:     core.NewStrategyChain(strategies...),
		listeners: listeners,
		info:      info,
	}
}

func DefaultGinParserWithTypes(info *types.Info) *GinParser {
	return NewGinParserWithTypes(
		[]core.RouteStrategy{
			&GinCallExprStrategy{},
		},
		info,
	)
}

func (p *GinParser) AddListener(listener core.EventListener) {
	p.listeners = append(p.listeners, listener)
}

func (p *GinParser) Parse(fset *token.FileSet, files []*ast.File) ([]core.ParsedRoute, error) {
	funcMap := buildFuncMap(files)
	var routes []core.ParsedRoute

	for _, file := range files {
		ast.Inspect(file, func(n ast.Node) bool {
			if route, ok := p.chain.TryParse(n, fset, funcMap, files, nil, p.info); ok {
				routes = append(routes, *route)
				for _, l := range p.listeners {
					l.OnRouteParsed(route)
				}
				return false
			}
			return true
		})
	}

	return routes, nil
}

func isGinHTTPMethod(method string) bool {
	switch method {
	case "GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS", "HEAD":
		return true
	}
	return false
}

func extractGinRequestBodyType(fn *ast.FuncDecl, goFiles []*ast.File) string {
	if fn == nil || fn.Body == nil {
		return ""
	}
	var boundVarName string
	var file *ast.File
	for _, f := range goFiles {
		for _, decl := range f.Decls {
			if funcDecl, ok := decl.(*ast.FuncDecl); ok && funcDecl == fn {
				file = f
				break
			}
		}
		if file != nil {
			break
		}
	}
	aliases := map[string]struct{}{"json": {}, "jsoniter": {}}
	if file != nil {
		aliases = CollectJSONAliases(file)
	}
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		// Check for Gin-specific binding methods
		if se, ok := call.Fun.(*ast.SelectorExpr); ok {
			if isGinBindingMethod(se.Sel.Name) {
				if len(call.Args) > 0 {
					if unary, ok := call.Args[0].(*ast.UnaryExpr); ok && unary.Op == token.AND {
						if ident, ok := unary.X.(*ast.Ident); ok {
							boundVarName = ident.Name
							return false
						}
					}
				}
			}
		}
		// Check for json.NewDecoder(r.Body).Decode(&var) pattern
		if isJSONDecodeCall(call, aliases) {
			if len(call.Args) > 0 {
				if unary, ok := call.Args[0].(*ast.UnaryExpr); ok && unary.Op == token.AND {
					if ident, ok := unary.X.(*ast.Ident); ok {
						boundVarName = ident.Name
						return false
					}
				}
			}
		}
		return true
	})
	if boundVarName == "" {
		return ""
	}
	var varType string
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if varType != "" {
			return false
		}
		decl, ok := n.(*ast.DeclStmt)
		if !ok {
			return true
		}
		genDecl, ok := decl.Decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.VAR {
			return true
		}
		for _, spec := range genDecl.Specs {
			if vspec, ok := spec.(*ast.ValueSpec); ok {
				for _, name := range vspec.Names {
					if name.Name == boundVarName {
						if typeName := getTypeName(vspec.Type); typeName != "" {
							varType = typeName
							return false
						}
					}
				}
			}
		}
		return true
	})
	return varType
}

func isGinBindingMethod(name string) bool {
	switch name {
	case "Bind", "BindJSON", "BindXML", "BindForm", "BindQuery", "ShouldBindJSON":
		return true
	}
	return false
}

func extractGinResponseTypes(fn *ast.FuncDecl, goFiles []*ast.File, info *types.Info) []core.ResponseInfo {
	var responses []core.ResponseInfo
	if fn.Body == nil {
		return responses
	}

	// Collect JSON aliases from the file containing this function
	var file *ast.File
	for _, f := range goFiles {
		for _, decl := range f.Decls {
			if funcDecl, ok := decl.(*ast.FuncDecl); ok && funcDecl == fn {
				file = f
				break
			}
		}
		if file != nil {
			break
		}
	}
	aliases := map[string]struct{}{"json": {}, "jsoniter": {}} // fallback
	if file != nil {
		aliases = CollectJSONAliases(file)
	}

	addResponse := func(statusCode int, responseType string, mapKeys map[string]string) {
		for _, r := range responses {
			if r.StatusCode == statusCode && r.Type == responseType {
				return
			}
		}
		responses = append(responses, core.ResponseInfo{
			StatusCode: statusCode,
			Type:       responseType,
			MediaType:  "application/json",
			MapKeys:    mapKeys,
		})
	}

	// First pass: collect all WriteHeader calls with their status codes
	statusCodes := findAllStatusCodes(fn)

	// Second pass: find all response calls and associate them with status codes
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			// Check for Gin-specific JSON methods
			if se, ok := call.Fun.(*ast.SelectorExpr); ok && isGinJSONMethod(se.Sel.Name) {
				if len(call.Args) >= 2 {
					statusCode := resolveStatusCode(call.Args[0])
					if statusCode == 0 {
						statusCode = findBestStatusCode(statusCodes, call.Pos())
						if statusCode == 0 {
							statusCode = 200 // default
						}
					}
					var responseType string
					var mapKeys map[string]string
					if ident, ok := call.Args[1].(*ast.Ident); ok {
						responseType = resolveVarTypeInFunc(fn, ident.Name, goFiles)
						if responseType == "" {
							responseType = ident.Name
						}
					} else if comp, ok := call.Args[1].(*ast.CompositeLit); ok {
						responseType = getTypeName(comp.Type)
						if info != nil {
							if typ := info.Types[comp.Type].Type; typ != nil {
								if mapType, ok := typ.(*types.Map); ok {
									mapKeys = extractMapKeysFromCompositeLit(comp, mapType, info)
								}
							}
						}
					}
					if responseType != "" {
						addResponse(statusCode, responseType, mapKeys)
					}
				}
			}
			// Check for standard Go json.NewEncoder().Encode() patterns
			if isJSONEncodeCall(call, aliases) {
				// Find the appropriate status code from collected status codes
				statusCode := findBestStatusCode(statusCodes, call.Pos())
				if statusCode == 0 {
					statusCode = 200 // default
				}
				var responseType string
				var mapKeys map[string]string
				if len(call.Args) > 0 {
					if ident, ok := call.Args[0].(*ast.Ident); ok {
						responseType = resolveVarTypeInFunc(fn, ident.Name, goFiles)
						if responseType == "" {
							responseType = ident.Name
						}
					} else if comp, ok := call.Args[0].(*ast.CompositeLit); ok {
						responseType = getTypeName(comp.Type)
						if info != nil {
							if typ := info.Types[comp.Type].Type; typ != nil {
								if mapType, ok := typ.(*types.Map); ok {
									mapKeys = extractMapKeysFromCompositeLit(comp, mapType, info)
								}
							}
						}
					}
				}
				if responseType != "" {
					addResponse(statusCode, responseType, mapKeys)
				}
			}
		}
		return true
	})

	return responses
}

func isGinJSONMethod(name string) bool {
	switch name {
	case "JSON", "JSONP", "XML", "YAML", "ProtoBuf", "String", "Data", "File", "Redirect":
		return true
	}
	return false
}
