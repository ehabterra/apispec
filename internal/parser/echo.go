package parser

import (
	"go/ast"
	"go/token"
	"go/types"
	"net/http"
	"strings"

	"github.com/ehabterra/swagen/internal/core"
)

// --- EchoCallExprStrategy ---

type EchoCallExprStrategy struct{}

func (s *EchoCallExprStrategy) TryParse(node ast.Node, fset *token.FileSet, funcMap map[string]*ast.FuncDecl, goFiles []*ast.File, next core.RouteStrategy, info *types.Info) (*core.ParsedRoute, bool) {
	call, ok := node.(*ast.CallExpr)
	if !ok {
		if next != nil {
			return next.TryParse(node, fset, funcMap, goFiles, next, info)
		}
		return nil, false
	}

	// Check if it's an Echo router method call
	if se, ok := call.Fun.(*ast.SelectorExpr); ok {
		if isEchoHTTPMethod(se.Sel.Name) {
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
						requestType := extractEchoRequestBodyType(handlerFunc, goFiles)
						responses := extractEchoResponseTypes(handlerFunc, goFiles, info)

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

// --- EchoParser with Event System and Chain of Responsibility ---

type EchoParser struct {
	chain     *core.StrategyChain
	listeners []core.EventListener
	info      *types.Info // for type resolution
}

func NewEchoParserWithTypes(strategies []core.RouteStrategy, info *types.Info, listeners ...core.EventListener) *EchoParser {
	return &EchoParser{
		chain:     core.NewStrategyChain(strategies...),
		listeners: listeners,
		info:      info,
	}
}

func DefaultEchoParserWithTypes(info *types.Info) *EchoParser {
	return NewEchoParserWithTypes(
		[]core.RouteStrategy{
			&EchoCallExprStrategy{},
		},
		info,
	)
}

func (p *EchoParser) AddListener(listener core.EventListener) {
	p.listeners = append(p.listeners, listener)
}

func (p *EchoParser) Parse(fset *token.FileSet, files []*ast.File) ([]core.ParsedRoute, error) {
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

// --- Echo-specific helpers ---

func isEchoHTTPMethod(method string) bool {
	switch method {
	case "GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS", "HEAD":
		return true
	}
	return false
}

// extractEchoRequestBodyType extracts the request body type from Echo handler functions
func extractEchoRequestBodyType(fn *ast.FuncDecl, _ []*ast.File) string {
	if fn == nil || fn.Body == nil {
		return ""
	}

	var boundVarName string
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		// Check for Echo-specific binding methods
		se, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || !isEchoBindingMethod(se.Sel.Name) {
			return true
		}

		if len(call.Args) > 0 {
			if unary, ok := call.Args[0].(*ast.UnaryExpr); ok && unary.Op == token.AND {
				if ident, ok := unary.X.(*ast.Ident); ok {
					boundVarName = ident.Name
					return false
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

// isEchoBindingMethod checks if a function name is a common Echo binding method.
func isEchoBindingMethod(name string) bool {
	switch name {
	case "Bind", "BindJSON", "BindXML", "BindForm", "BindQuery":
		return true
	}
	return false
}

// extractEchoResponseTypes extracts response types from Echo handler functions
func extractEchoResponseTypes(fn *ast.FuncDecl, goFiles []*ast.File, info *types.Info) []core.ResponseInfo {
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
		ast.Inspect(f.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			if se, ok := call.Fun.(*ast.SelectorExpr); ok && isEchoJSONMethod(se.Sel.Name) {
				statusCode, responseArg := ExtractStatusAndResponseFromCall(call, info)
				if statusCode == 0 {
					statusCode = http.StatusOK // default
				}
				var responseType string
				var mapKeys map[string]string
				if responseArg != nil {
					if ident, ok := responseArg.(*ast.Ident); ok {
						responseType = resolveVarTypeInFunc(f, ident.Name, goFiles)
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
			}
			if isJSONEncodeCall(call, aliases) {
				statusCode := http.StatusOK // default
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
		println("[DEBUG] Echo handler:", fn.Name.Name)
		for _, r := range out {
			println("  [DEBUG] Response:", r.StatusCode, r.Type, r.MediaType)
		}
	}
	return out
}

// isEchoJSONMethod checks if a function name is an Echo JSON response method
func isEchoJSONMethod(name string) bool {
	switch name {
	case "JSON", "JSONPretty", "JSONBlob", "JSONP":
		return true
	}
	return false
}
