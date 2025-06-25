package parser

import (
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	"github.com/ehabterra/swagen/internal/core"
)

// convertChiPathToOpenAPI converts Chi path parameters to OpenAPI format
func convertChiPathToOpenAPI(path string) string {
	// Chi uses {param} format, OpenAPI uses {param} format
	// This is already compatible, but we can add any Chi-specific conversions here
	return path
}

type ChiCallExprStrategy struct{}

func (s *ChiCallExprStrategy) TryParse(node ast.Node, fset *token.FileSet, funcMap map[string]*ast.FuncDecl, goFiles []*ast.File, next core.RouteStrategy, info *types.Info) (*core.ParsedRoute, bool) {
	call, ok := node.(*ast.CallExpr)
	if !ok {
		if next != nil {
			return next.TryParse(node, fset, funcMap, goFiles, next, info)
		}
		return nil, false
	}

	// Check if it's a Chi router method call
	if se, ok := call.Fun.(*ast.SelectorExpr); ok {
		if isChiHTTPMethod(se.Sel.Name) {
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
						requestType := extractChiRequestBodyType(handlerFunc, goFiles)
						responses := extractChiResponseTypes(handlerFunc, goFiles, info)

						builder := core.NewParsedRouteBuilder(info)
						route := builder.Method(se.Sel.Name).
							Path(convertChiPathToOpenAPI(path)).
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

type ChiParser struct {
	chain     *core.StrategyChain
	listeners []core.EventListener
	info      *types.Info // for type resolution
}

func NewChiParserWithTypes(strategies []core.RouteStrategy, info *types.Info, listeners ...core.EventListener) *ChiParser {
	return &ChiParser{
		chain:     core.NewStrategyChain(strategies...),
		listeners: listeners,
		info:      info,
	}
}

func DefaultChiParserWithTypes(info *types.Info) *ChiParser {
	return NewChiParserWithTypes(
		[]core.RouteStrategy{
			&ChiCallExprStrategy{},
		},
		info,
	)
}

func (p *ChiParser) AddListener(listener core.EventListener) {
	p.listeners = append(p.listeners, listener)
}

func (p *ChiParser) Parse(fset *token.FileSet, files []*ast.File) ([]core.ParsedRoute, error) {
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

func isChiHTTPMethod(method string) bool {
	switch method {
	case "Get", "Post", "Put", "Delete", "Patch", "Options", "Head":
		return true
	}
	return false
}

func extractChiRequestBodyType(fn *ast.FuncDecl, goFiles []*ast.File) string {
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
		// Check for Chi-specific binding methods
		if se, ok := call.Fun.(*ast.SelectorExpr); ok {
			if isChiBindingMethod(se.Sel.Name) {
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

func isChiBindingMethod(name string) bool {
	switch name {
	case "Bind", "BindJSON", "BindXML", "BindForm", "BindQuery":
		return true
	}
	return false
}

func extractChiResponseTypes(fn *ast.FuncDecl, goFiles []*ast.File, info *types.Info) []core.ResponseInfo {
	// Build funcMap and callGraph once per file set
	funcMap := BuildFuncMap(goFiles)
	callGraph := BuildCallGraph(goFiles)

	extractFn := func(f *ast.FuncDecl) []interface{} {
		var responses []core.ResponseInfo
		if f.Body == nil {
			return nil
		}
		var file *ast.File
		for _, gf := range goFiles {
			for _, decl := range gf.Decls {
				if funcDecl, ok := decl.(*ast.FuncDecl); ok && funcDecl == f {
					file = gf
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
		statusCodes := findAllStatusCodes(f)
		ast.Inspect(f.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			if se, ok := call.Fun.(*ast.SelectorExpr); ok && isChiJSONMethod(se.Sel.Name) {
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
						responseType = resolveVarTypeInFunc(f, ident.Name, goFiles)
						if responseType == "" {
							responseType = ident.Name
						}
					} else if comp, ok := call.Args[1].(*ast.CompositeLit); ok {
						if info != nil && info.Types != nil {
							if typeAndValue, exists := info.Types[comp.Type]; exists && typeAndValue.Type != nil {
								if mapType, ok := typeAndValue.Type.(*types.Map); ok {
									mapKeys = extractMapKeysFromCompositeLit(comp, mapType, info)
								}
							}
						}
						responseType = getTypeName(comp.Type)
					}
					if responseType != "" {
						responses = append(responses, core.ResponseInfo{
							StatusCode: statusCode,
							Type:       responseType,
							MediaType:  "application/json",
							MapKeys:    mapKeys,
						})
					}
				}
			}
			if isJSONEncodeCall(call, aliases) {
				statusCode := findBestStatusCode(statusCodes, call.Pos())
				if statusCode == 0 {
					statusCode = 200 // default
				}
				var responseType string
				var mapKeys map[string]string
				if len(call.Args) > 0 {
					if ident, ok := call.Args[0].(*ast.Ident); ok {
						responseType = resolveVarTypeInFunc(f, ident.Name, goFiles)
						if responseType == "" {
							responseType = ident.Name
						}
					} else if comp, ok := call.Args[0].(*ast.CompositeLit); ok {
						if info != nil && info.Types != nil {
							if typeAndValue, exists := info.Types[comp.Type]; exists && typeAndValue.Type != nil {
								if mapType, ok := typeAndValue.Type.(*types.Map); ok {
									mapKeys = extractMapKeysFromCompositeLit(comp, mapType, info)
								}
							}
						}
						responseType = getTypeName(comp.Type)
					}
				}
				if responseType != "" {
					responses = append(responses, core.ResponseInfo{
						StatusCode: statusCode,
						Type:       responseType,
						MediaType:  "application/json",
						MapKeys:    mapKeys,
					})
				}
			}
			return true
		})
		return toIfaceSlice(responses)
	}

	var out []core.ResponseInfo

	if fn != nil && fn.Name != nil {
		results := ExtractNestedInfo(fn.Name.Name, funcMap, callGraph, nil, extractFn)
		for _, r := range results {
			if resp, ok := r.(core.ResponseInfo); ok {
				out = append(out, resp)
			}
		}
		println("[DEBUG] Chi handler:", fn.Name.Name)
		for _, r := range out {
			println("  [DEBUG] Response:", r.StatusCode, r.Type, r.MediaType)
		}
	}
	return out
}

// isChiJSONMethod checks if a function name is a Chi JSON response method
func isChiJSONMethod(name string) bool {
	switch name {
	case "JSON", "WriteJSON", "Render":
		return true
	}
	return false
}

// NewChiParserForTest creates a ChiParser with type information for testing
func NewChiParserForTest(files []*ast.File) (*ChiParser, error) {
	// Create types.Info to collect type information
	info := &types.Info{
		Types:      make(map[ast.Expr]types.TypeAndValue),
		Defs:       make(map[*ast.Ident]types.Object),
		Uses:       make(map[*ast.Ident]types.Object),
		Implicits:  make(map[ast.Node]types.Object),
		Selections: make(map[*ast.SelectorExpr]*types.Selection),
		Scopes:     make(map[ast.Node]*types.Scope),
	}

	// For tests, we'll skip type checking since we might not have all dependencies
	// and the parser can work with minimal type information
	return DefaultChiParserWithTypes(info), nil
}
