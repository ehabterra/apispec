package parser

import (
	"go/ast"
	"go/token"
	"go/types"
	"regexp"
	"strings"

	"github.com/ehabterra/swagen/internal/core"
)

// --- Echo-specific constants and patterns ---

var echoParamRe = regexp.MustCompile(`:([a-zA-Z0-9_]+)`)

func convertEchoPathToOpenAPI(path string) string {
	return echoParamRe.ReplaceAllString(path, `{$1}`)
}

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
				path = convertEchoPathToOpenAPI(path)

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

	// First pass: collect all WriteHeader calls with their status codes
	statusCodes := findAllStatusCodes(fn)

	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		// Check for Echo-specific JSON methods
		if se, ok := call.Fun.(*ast.SelectorExpr); ok {
			if isEchoJSONMethod(se.Sel.Name) {
				if len(call.Args) >= 2 {
					statusCode := resolveStatusCode(call.Args[0])
					if statusCode == 0 {
						return true
					}

					var responseType string
					var mapKeys map[string]string
					if ident, ok := call.Args[1].(*ast.Ident); ok {
						responseType = resolveVarTypeInFunc(fn, ident.Name, goFiles)
						if responseType == "" {
							responseType = ident.Name
						}
					} else if comp, ok := call.Args[1].(*ast.CompositeLit); ok {
						if info != nil {
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
			}
		}

		// Check for standard Go json.NewEncoder().Encode() patterns
		if isJSONEncodeCall(call, aliases) {
			// Find the appropriate status code from collected status codes
			statusCode := findBestStatusCode(statusCodes, call.Pos())
			if statusCode == 0 {
				statusCode = 200 // default to 200 if not found
			}

			var responseType string
			var mapKeys map[string]string
			if ident, ok := call.Args[0].(*ast.Ident); ok {
				responseType = resolveVarTypeInFunc(fn, ident.Name, goFiles)
				if responseType == "" {
					responseType = ident.Name
				}
			} else if comp, ok := call.Args[0].(*ast.CompositeLit); ok {
				responseType = getTypeName(comp.Type)
				// Extract map keys for better error response schemas
				if info != nil {
					if typ := info.Types[comp.Type].Type; typ != nil {
						if mapType, ok := typ.(*types.Map); ok {
							mapKeys = extractMapKeysFromCompositeLit(comp, mapType, info)
						}
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

		return true
	})

	return responses
}

// isEchoJSONMethod checks if a function name is an Echo JSON response method
func isEchoJSONMethod(name string) bool {
	switch name {
	case "JSON", "JSONPretty", "JSONBlob", "JSONP":
		return true
	}
	return false
}
