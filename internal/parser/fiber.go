package parser

import (
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	"github.com/ehabterra/swagen/internal/core"
)

// convertFiberPathToOpenAPI converts Fiber path parameters to OpenAPI format
func convertFiberPathToOpenAPI(path string) string {
	// Fiber uses :param for path parameters, convert to {param}
	result := path
	for i := 0; i < len(result)-1; i++ {
		if result[i] == ':' {
			// Find the end of the parameter name
			end := i + 1
			for end < len(result) && result[end] != '/' {
				end++
			}
			paramName := result[i+1 : end]
			result = result[:i] + "{" + paramName + "}" + result[end:]
			i = i + len(paramName) + 1 // Skip the replaced part
		}
	}
	return result
}

type FiberCallExprStrategy struct{}

func (s *FiberCallExprStrategy) TryParse(node ast.Node, fset *token.FileSet, funcMap map[string]*ast.FuncDecl, goFiles []*ast.File, next core.RouteStrategy, info *types.Info) (*core.ParsedRoute, bool) {
	call, ok := node.(*ast.CallExpr)
	if !ok {
		if next != nil {
			return next.TryParse(node, fset, funcMap, goFiles, next, info)
		}
		return nil, false
	}

	// Check if it's a Fiber router method call
	if se, ok := call.Fun.(*ast.SelectorExpr); ok {
		if isFiberHTTPMethod(se.Sel.Name) {
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
				path = convertFiberPathToOpenAPI(path)

				handlerName := getHandlerName(call.Args[1])

				if handlerName != "" {
					if handlerFunc, exists := funcMap[handlerName]; exists {
						requestType := extractFiberRequestBodyType(handlerFunc, goFiles)
						responses := extractFiberResponseTypes(handlerFunc, goFiles, info)

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

type FiberParser struct {
	chain     *core.StrategyChain
	listeners []core.EventListener
	info      *types.Info // for type resolution
}

func NewFiberParserWithTypes(strategies []core.RouteStrategy, info *types.Info, listeners ...core.EventListener) *FiberParser {
	return &FiberParser{
		chain:     core.NewStrategyChain(strategies...),
		listeners: listeners,
		info:      info,
	}
}

func DefaultFiberParserWithTypes(info *types.Info) *FiberParser {
	return NewFiberParserWithTypes(
		[]core.RouteStrategy{
			&FiberCallExprStrategy{},
		},
		info,
	)
}

func (p *FiberParser) AddListener(listener core.EventListener) {
	p.listeners = append(p.listeners, listener)
}

func (p *FiberParser) Parse(fset *token.FileSet, files []*ast.File) ([]core.ParsedRoute, error) {
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

func isFiberHTTPMethod(method string) bool {
	switch method {
	case "Get", "Post", "Put", "Delete", "Patch", "Options", "Head":
		return true
	}
	return false
}

func extractFiberRequestBodyType(fn *ast.FuncDecl, goFiles []*ast.File) string {
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
		// Check for Fiber-specific binding methods
		if se, ok := call.Fun.(*ast.SelectorExpr); ok {
			if isFiberBindingMethod(se.Sel.Name) {
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

func isFiberBindingMethod(name string) bool {
	switch name {
	case "Bind", "BindJSON", "BindXML", "BindForm", "BindQuery", "BodyParser":
		return true
	}
	return false
}

func extractFiberResponseTypes(fn *ast.FuncDecl, goFiles []*ast.File, info *types.Info) []core.ResponseInfo {
	var responses []core.ResponseInfo
	if fn.Body == nil {
		return responses
	}

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

	statusCodes := findAllStatusCodes(fn)

	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		// Handle chained calls: c.Status(...).JSON(...)
		if se, ok := call.Fun.(*ast.SelectorExpr); ok && isFiberJSONMethod(se.Sel.Name) {
			var statusCode int
			var responseArg ast.Expr

			// Check if the receiver is a CallExpr (i.e., c.Status(...))
			if recvCall, ok := se.X.(*ast.CallExpr); ok {
				if recvSel, ok := recvCall.Fun.(*ast.SelectorExpr); ok && recvSel.Sel.Name == "Status" && len(recvCall.Args) == 1 {
					statusCode = resolveStatusCode(recvCall.Args[0])
				}
			}
			// If not found, fallback to first argument (for c.JSON(status, ...))
			if statusCode == 0 && len(call.Args) >= 2 {
				statusCode = resolveStatusCode(call.Args[0])
				responseArg = call.Args[1]
			} else if len(call.Args) == 1 {
				responseArg = call.Args[0]
			}
			if statusCode == 0 {
				statusCode = findBestStatusCode(statusCodes, call.Pos())
				if statusCode == 0 {
					statusCode = 200 // default
				}
			}

			var responseType string
			var mapKeys map[string]string
			if responseArg != nil {
				if ident, ok := responseArg.(*ast.Ident); ok {
					responseType = resolveVarTypeInFunc(fn, ident.Name, goFiles)
					if responseType == "" {
						responseType = ident.Name
					}
				} else if comp, ok := responseArg.(*ast.CompositeLit); ok {
					if info != nil && info.Types != nil {
						if typ := info.Types[comp.Type].Type; typ != nil {
							if named, ok := typ.(*types.Named); ok {
								responseType = named.Obj().Name()
							} else if slice, ok := typ.(*types.Slice); ok {
								if elem, ok := slice.Elem().(*types.Named); ok {
									responseType = "[]" + elem.Obj().Name()
								}
							} else if mapType, ok := typ.(*types.Map); ok {
								responseType = typ.String()
								mapKeys = extractMapKeysFromCompositeLit(comp, mapType, info)
							} else {
								responseType = typ.String()
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

		// Standard Go json.NewEncoder().Encode() patterns
		if isJSONEncodeCall(call, aliases) {
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
					if info != nil && info.Types != nil {
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

	if fn != nil && fn.Name != nil {
		println("[DEBUG] Fiber handler:", fn.Name.Name)
		for _, r := range responses {
			println("  [DEBUG] Response:", r.StatusCode, r.Type, r.MediaType)
		}
	}

	return responses
}

// isFiberJSONMethod checks if a function name is a Fiber JSON response method
func isFiberJSONMethod(name string) bool {
	switch name {
	case "JSON", "Send", "SendString", "SendFile", "SendStream", "SendStatus":
		return true
	}
	return false
}
