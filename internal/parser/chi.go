package parser

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"net/http"
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

func (s *ChiCallExprStrategy) TryParse(node ast.Node, fset *token.FileSet, funcMap map[string]*ast.FuncDecl, goFiles []*ast.File, next core.RouteStrategy, info *types.Info, pkgName string, aliasMap map[string]string, currentFile *ast.File) ([]*core.ParsedRoute, bool) {
	call, ok := node.(*ast.CallExpr)
	if !ok {
		if next != nil {
			return next.TryParse(node, fset, funcMap, goFiles, next, info, pkgName, aliasMap, currentFile)
		}
		return nil, false
	}

	// Handle r.Mount("/prefix", subrouter)
	if se, ok := call.Fun.(*ast.SelectorExpr); ok {
		if se.Sel.Name == "Mount" && len(call.Args) == 2 {
			// Extract prefix
			prefixLit, ok := call.Args[0].(*ast.BasicLit)
			if !ok || prefixLit.Kind != token.STRING {
				return nil, false
			}
			prefix := strings.Trim(prefixLit.Value, "\"")

			// Find the subrouter function (e.g., users.Routes)
			subrouterName := getSubrouterName(call.Args[1], pkgName, aliasMap)
			if subrouterName != "" {
				// Debug logging
				fmt.Printf("[DEBUG] Looking for subrouter: %s\n", subrouterName)
				fmt.Printf("[DEBUG] Available functions: %v\n", getFuncMapKeys(funcMap))

				// Try exact match first
				if subrouterFunc, exists := funcMap[subrouterName]; exists {
					fmt.Printf("[DEBUG] Found subrouter function: %s\n", subrouterName)
					var newPkgName = pkgName

					if parts := strings.Split(subrouterName, "."); len(parts) > 1 {
						newPkgName = parts[0]
					}

					subRoutes := parseChiSubrouter(subrouterFunc, fset, funcMap, goFiles, info, newPkgName, aliasMap)
					for i := range subRoutes {
						if subRoutes[i].Path == "/" {
							subRoutes[i].Path = prefix
						} else {
							subRoutes[i].Path = prefix + subRoutes[i].Path
						}
						if len(subRoutes[i].Tags) == 0 {
							subRoutes[i].Tags = []string{prefix}
						} else {
							subRoutes[i].Tags = append(subRoutes[i].Tags, prefix)
						}
					}
					if len(subRoutes) > 0 {
						// Convert to slice of pointers
						result := make([]*core.ParsedRoute, len(subRoutes))
						for i := range subRoutes {
							result[i] = &subRoutes[i]
						}
						return result, true
					}
				}
				// If not found and it's a local function, try without package prefix
				if !strings.Contains(subrouterName, ".") && pkgName != "main" {
					if subrouterFunc, exists := funcMap[subrouterName]; exists {
						fmt.Printf("[DEBUG] Found local subrouter function: %s\n", subrouterName)
						subRoutes := parseChiSubrouter(subrouterFunc, fset, funcMap, goFiles, info, pkgName, aliasMap)
						for i := range subRoutes {
							if subRoutes[i].Path == "/" {
								subRoutes[i].Path = prefix
							} else {
								subRoutes[i].Path = prefix + subRoutes[i].Path
							}
							if len(subRoutes[i].Tags) == 0 {
								subRoutes[i].Tags = []string{prefix}
							} else {
								subRoutes[i].Tags = append(subRoutes[i].Tags, prefix)
							}
						}
						if len(subRoutes) > 0 {
							// Convert to slice of pointers
							result := make([]*core.ParsedRoute, len(subRoutes))
							for i := range subRoutes {
								result[i] = &subRoutes[i]
							}
							return result, true
						}
					}
				}
			}
		}
		// Check if it's a Chi router method call
		if isChiHTTPMethod(se.Sel.Name) {
			if len(call.Args) >= 2 {
				// Extract path from string literal
				pathLit, ok := call.Args[0].(*ast.BasicLit)
				if !ok || pathLit.Kind != token.STRING {
					if next != nil {
						return next.TryParse(node, fset, funcMap, goFiles, next, info, pkgName, aliasMap, currentFile)
					}
					return nil, false
				}
				path := strings.Trim(pathLit.Value, "\"")

				var handlerName string
				var handlerFunc *ast.FuncDecl
				var handlerLit *ast.FuncLit

				switch h := call.Args[1].(type) {
				case *ast.Ident, *ast.SelectorExpr, *ast.CallExpr:
					handlerName, handlerFunc = resolveHandlerFunc(h, funcMap, pkgName, aliasMap, info)
				case *ast.FuncLit:
					handlerName = generateAnonymousHandlerName(goFiles[0], call.Pos(), se.Sel.Name, path)
					handlerLit = h
				}

				if handlerFunc != nil {
					requestType := extractChiRequestBodyType(handlerFunc, goFiles)
					responses := extractChiResponseTypes(handlerFunc, goFiles, funcMap, info, pkgName)

					builder := core.NewParsedRouteBuilder(info)
					route := builder.Method(se.Sel.Name).
						Path(convertChiPathToOpenAPI(path)).
						Handler(handlerName, handlerFunc).
						SetRequestBody(requestType, "body").
						SetResponses(responses).
						Position(fset, call.Pos()).
						Build()

					return []*core.ParsedRoute{route}, true
				} else if handlerLit != nil {
					requestType := extractChiRequestBodyTypeFromLit(handlerLit, goFiles)
					responses := extractChiResponseTypesFromLit(handlerLit, goFiles, funcMap, info, pkgName)

					builder := core.NewParsedRouteBuilder(info)
					route := builder.Method(se.Sel.Name).
						Path(convertChiPathToOpenAPI(path)).
						Handler(handlerName, nil).
						SetRequestBody(requestType, "body").
						SetResponses(responses).
						Position(fset, call.Pos()).
						Build()

					return []*core.ParsedRoute{route}, true
				}
			}
		}
	}

	if next != nil {
		return next.TryParse(node, fset, funcMap, goFiles, next, info, pkgName, aliasMap, currentFile)
	}
	return nil, false
}

// Helper to get subrouter name with package context
func getSubrouterName(expr ast.Expr, pkgName string, aliasMap map[string]string) string {

	switch v := expr.(type) {
	case *ast.Ident:
		// A local function, e.g., `Routes`
		if pkgName != "" && pkgName != "main" {
			return pkgName + "." + v.Name
		}
		return v.Name
	case *ast.SelectorExpr:
		// A function from another package, e.g., `users.Routes`
		if pkgIdent, ok := v.X.(*ast.Ident); ok {
			if realPkgName, exists := aliasMap[pkgIdent.Name]; exists {
				return realPkgName + "." + v.Sel.Name
			}
			return pkgIdent.Name + "." + v.Sel.Name
		}
	}
	return ""
}

// Helper function to get function map keys for debugging
func getFuncMapKeys(funcMap map[string]*ast.FuncDecl) []string {
	keys := make([]string, 0, len(funcMap))
	for k := range funcMap {
		keys = append(keys, k)
	}
	return keys
}

// Helper to recursively parse subrouter functions for Mount
func parseChiSubrouter(fn *ast.FuncDecl, fset *token.FileSet, funcMap map[string]*ast.FuncDecl, goFiles []*ast.File, info *types.Info, pkgName string, aliasMap map[string]string) []core.ParsedRoute {
	var routes []core.ParsedRoute
	if fn == nil || fn.Body == nil {
		return routes
	}
	// Find the current file for this function
	var currentFile *ast.File
	for _, file := range goFiles {
		for _, decl := range file.Decls {
			if funcDecl, ok := decl.(*ast.FuncDecl); ok && funcDecl == fn {
				currentFile = file
				break
			}
		}
		if currentFile != nil {
			break
		}
	}
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if routeSlice, ok := (&ChiCallExprStrategy{}).TryParse(call, fset, funcMap, goFiles, nil, info, pkgName, aliasMap, currentFile); ok && len(routeSlice) > 0 {
			for _, route := range routeSlice {
				routes = append(routes, *route)
			}
		}
		return true
	})
	return routes
}

type ChiParser struct {
	chain     *core.StrategyChain
	listeners []core.EventListener
}

func DefaultChiParser() *ChiParser {
	return &ChiParser{
		chain: core.NewStrategyChain(
			&ChiCallExprStrategy{},
		),
	}
}

func (p *ChiParser) AddListener(listener core.EventListener) {
	p.listeners = append(p.listeners, listener)
}

func (p *ChiParser) Parse(fset *token.FileSet, files []*ast.File, fileToInfo map[*ast.File]*types.Info) ([]core.ParsedRoute, error) {
	funcMap := BuildFuncMap(files)
	var routes []core.ParsedRoute

	fmt.Printf("[DEBUG] Chi parser: parsing %d files\n", len(files))
	for _, file := range files {
		info := fileToInfo[file]
		if info == nil {
			// Create an empty types.Info if none is provided
			info = &types.Info{
				Types:      make(map[ast.Expr]types.TypeAndValue),
				Defs:       make(map[*ast.Ident]types.Object),
				Uses:       make(map[*ast.Ident]types.Object),
				Implicits:  make(map[ast.Node]types.Object),
				Selections: make(map[*ast.SelectorExpr]*types.Selection),
				Scopes:     make(map[ast.Node]*types.Scope),
			}
		}
		pkgName := ""
		if file.Name != nil {
			pkgName = file.Name.Name
		}
		fmt.Printf("[DEBUG] Chi parser: processing file in package %s\n", pkgName)
		aliasMap := buildAliasMap(file)
		// We no longer skip non-main packages, we inspect all of them.
		ast.Inspect(file, func(n ast.Node) bool {
			if routeSlice, ok := p.chain.TryParse(n, fset, funcMap, files, nil, info, pkgName, aliasMap, file); ok {
				fmt.Printf("[DEBUG] Chi parser: found %d routes in package %s\n", len(routeSlice), pkgName)
				for _, route := range routeSlice {
					routes = append(routes, *route)
					for _, l := range p.listeners {
						l.OnRouteParsed(route)
					}
				}
				return false
			}
			return true
		})
	}
	fmt.Printf("[DEBUG] Chi parser: total routes found: %d\n", len(routes))
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
						if typeName := GetTypeName(vspec.Type); typeName != "" {
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

// Add a robust lookup helper at the top or near extractChiResponseTypes
func lookupFuncDecl(funcMap map[string]*ast.FuncDecl, fn *ast.FuncDecl, pkgName string) (string, *ast.FuncDecl) {
	if fn == nil || fn.Name == nil {
		return "", nil
	}
	methodName := fn.Name.Name
	var keys []string
	// Try with package prefix
	if pkgName != "" && pkgName != "main" {
		keys = append(keys, pkgPrefix(pkgName)+methodName)
	}
	// Try without package prefix
	keys = append(keys, methodName)
	// Try with receiver type if method
	if fn.Recv != nil && len(fn.Recv.List) > 0 {
		var typeName string
		recvType := fn.Recv.List[0].Type
		if starExpr, ok := recvType.(*ast.StarExpr); ok {
			if ident, ok := starExpr.X.(*ast.Ident); ok {
				typeName = ident.Name
			}
		} else if ident, ok := recvType.(*ast.Ident); ok {
			typeName = ident.Name
		}
		if typeName != "" {
			keys = append(keys, typeName+"."+methodName)
		}
	}
	// Try all keys
	for _, key := range keys {
		if fn2, ok := funcMap[key]; ok {
			return key, fn2
		}
	}
	// Fallback: try by method name only
	for k, fn2 := range funcMap {
		if strings.HasSuffix(k, "."+methodName) {
			return k, fn2
		}
	}
	return "", nil
}

func extractChiResponseTypes(fn *ast.FuncDecl, goFiles []*ast.File, funcMap map[string]*ast.FuncDecl, info *types.Info, pkgName string) []core.ResponseInfo {
	// Build funcMap and callGraph once per file set
	// Create a fileToInfo map for BuildCallGraph
	fileToInfo := make(map[*ast.File]*types.Info)
	for _, file := range goFiles {
		fileToInfo[file] = info // Use the same info for all files in this context
	}
	callGraph := BuildCallGraph(goFiles, funcMap, fileToInfo)

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
							statusCode = http.StatusOK // default
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
						responseType = GetTypeName(comp.Type)
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
						responseType = GetTypeName(comp.Type)
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

	var results []interface{}
	funcName, lookupFn := lookupFuncDecl(funcMap, fn, pkgName)
	if lookupFn != nil {
		results = ExtractNestedInfo(funcName, funcMap, callGraph, nil, extractFn)
	} else {
		// fallback: try by method name only
		results = ExtractNestedInfo(fn.Name.Name, funcMap, callGraph, nil, extractFn)
	}

	var out []core.ResponseInfo
	for _, r := range results {
		if resp, ok := r.(core.ResponseInfo); ok {
			out = append(out, resp)
		}
	}
	println("[DEBUG] Chi handler:", fn.Name.Name)
	for _, r := range out {
		println("  [DEBUG] Response:", r.StatusCode, r.Type, r.MediaType)
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
	// For tests, we'll skip type checking since we might not have all dependencies
	// and the parser can work with minimal type information
	return DefaultChiParser(), nil
}

// Add extraction helpers for FuncLit
func extractChiRequestBodyTypeFromLit(fn *ast.FuncLit, goFiles []*ast.File) string {
	if fn == nil || fn.Body == nil {
		return ""
	}
	// Reuse logic from extractChiRequestBodyType, but for FuncLit
	return ""
}

func extractChiResponseTypesFromLit(fn *ast.FuncLit, goFiles []*ast.File, funcMap map[string]*ast.FuncDecl, info *types.Info, pkgName string) []core.ResponseInfo {
	if fn == nil || fn.Body == nil {
		return nil
	}
	// Reuse logic from extractChiResponseTypes, but for FuncLit
	return nil
}
