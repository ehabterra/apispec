package parser

import (
	"go/ast"
	"go/token"
	"go/types"
	"net/http"
	"strings"

	"github.com/ehabterra/swagen/internal/core"
)

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
				path = convertPathToOpenAPI(path)

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
			if se, ok := call.Fun.(*ast.SelectorExpr); ok && isGinJSONMethod(se.Sel.Name) {
				statusCode, responseArg := ExtractStatusAndResponseFromCall(call, info)
				if statusCode == 0 {
					statusCode = http.StatusOK // default
				}
				// Always treat 204 as no content, regardless of responseArg
				if statusCode == http.StatusNoContent {
					responses = append(responses, core.ResponseInfo{
						StatusCode: statusCode,
						Type:       "",
						MediaType:  "",
						MapKeys:    nil,
					})
					return true
				}
				var responseType string
				var mapKeys map[string]string
				if responseArg != nil {
					if ident, ok := responseArg.(*ast.Ident); ok {
						if info != nil && info.Types != nil {
							if obj, ok := info.Uses[ident]; ok && obj.Type() != nil {
								typ := obj.Type()
								if named, ok := typ.(*types.Named); ok {
									if mapType, ok := named.Underlying().(*types.Map); ok {
										responseType = mapType.String()
										// No mapKeys for identifier, only for composite literal
									}
								}
							}
						}
						if responseType == "" {
							responseType = resolveVarTypeInFunc(f, ident.Name, goFiles)
							if responseType == "" {
								responseType = ident.Name
							}
						}
					} else if comp, ok := responseArg.(*ast.CompositeLit); ok {
						if info != nil && info.Types != nil {
							if typ := info.Types[comp.Type].Type; typ != nil {
								// Unwrap named types (e.g., gin.H)
								if named, ok := typ.(*types.Named); ok {
									typ = named.Underlying()
								}
								if mapType, ok := typ.(*types.Map); ok {
									responseType = mapType.String()
									mapKeys = extractMapKeysFromCompositeLit(comp, mapType, info)
								}
							}
						}
						if responseType == "" {
							responseType = getTypeName(comp.Type)
						}
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
				return true
			}
			if isJSONEncodeCall(call, aliases) {
				statusCode := findBestStatusCode(statusCodes, call.Pos())
				if statusCode == 0 {
					statusCode = http.StatusOK // default
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
						if info != nil {
							if typ := info.Types[comp.Type].Type; typ != nil {
								if mapType, ok := typ.(*types.Map); ok {
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
		println("[DEBUG] Gin handler:", fn.Name.Name)
		for _, r := range out {
			println("  [DEBUG] Response:", r.StatusCode, r.Type, r.MediaType)
		}
	}

	// After collecting responses, filter out 200 if 204 is present for DELETE
	if fn != nil && fn.Name != nil && strings.HasPrefix(strings.ToLower(fn.Name.Name), "delete") {
		var filtered []core.ResponseInfo
		has204 := false
		for _, r := range out {
			if r.StatusCode == http.StatusNoContent {
				has204 = true
				break
			}
		}
		if has204 {
			for _, r := range out {
				if r.StatusCode != http.StatusOK {
					filtered = append(filtered, r)
				}
			}
			out = filtered
		}
	}

	return out
}

func isGinJSONMethod(name string) bool {
	switch name {
	case "JSON", "JSONP", "XML", "YAML", "ProtoBuf", "String", "Data", "File", "Redirect":
		return true
	}
	return false
}
