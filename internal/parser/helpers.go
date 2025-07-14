package parser

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"net/http"
	"regexp"
	"strconv"
	"strings"
)

// httpStatusMap contains all HTTP status constants from net/http package
// We'll populate this map with known constants
// This is more reliable than reflection for constants
var httpStatusMap = map[string]int{
	// 1xx Informational
	"StatusContinue":           http.StatusContinue,           // 100
	"StatusSwitchingProtocols": http.StatusSwitchingProtocols, // 101
	"StatusProcessing":         http.StatusProcessing,         // 102
	"StatusEarlyHints":         http.StatusEarlyHints,         // 103

	// 2xx Success
	"StatusOK":                   http.StatusOK,                   // 200
	"StatusCreated":              http.StatusCreated,              // 201
	"StatusAccepted":             http.StatusAccepted,             // 202
	"StatusNonAuthoritativeInfo": http.StatusNonAuthoritativeInfo, // 203
	"StatusNoContent":            http.StatusNoContent,            // 204
	"StatusResetContent":         http.StatusResetContent,         // 205
	"StatusPartialContent":       http.StatusPartialContent,       // 206
	"StatusMultiStatus":          http.StatusMultiStatus,          // 207
	"StatusAlreadyReported":      http.StatusAlreadyReported,      // 208
	"StatusIMUsed":               http.StatusIMUsed,               // 226

	// 3xx Redirection
	"StatusMultipleChoices":   http.StatusMultipleChoices,   // 300
	"StatusMovedPermanently":  http.StatusMovedPermanently,  // 301
	"StatusFound":             http.StatusFound,             // 302
	"StatusSeeOther":          http.StatusSeeOther,          // 303
	"StatusNotModified":       http.StatusNotModified,       // 304
	"StatusUseProxy":          http.StatusUseProxy,          // 305
	"StatusTemporaryRedirect": http.StatusTemporaryRedirect, // 307
	"StatusPermanentRedirect": http.StatusPermanentRedirect, // 308

	// 4xx Client Error
	"StatusBadRequest":                   http.StatusBadRequest,                   // 400
	"StatusUnauthorized":                 http.StatusUnauthorized,                 // 401
	"StatusPaymentRequired":              http.StatusPaymentRequired,              // 402
	"StatusForbidden":                    http.StatusForbidden,                    // 403
	"StatusNotFound":                     http.StatusNotFound,                     // 404
	"StatusMethodNotAllowed":             http.StatusMethodNotAllowed,             // 405
	"StatusNotAcceptable":                http.StatusNotAcceptable,                // 406
	"StatusProxyAuthRequired":            http.StatusProxyAuthRequired,            // 407
	"StatusRequestTimeout":               http.StatusRequestTimeout,               // 408
	"StatusConflict":                     http.StatusConflict,                     // 409
	"StatusGone":                         http.StatusGone,                         // 410
	"StatusLengthRequired":               http.StatusLengthRequired,               // 411
	"StatusPreconditionFailed":           http.StatusPreconditionFailed,           // 412
	"StatusRequestEntityTooLarge":        http.StatusRequestEntityTooLarge,        // 413
	"StatusRequestURITooLong":            http.StatusRequestURITooLong,            // 414
	"StatusUnsupportedMediaType":         http.StatusUnsupportedMediaType,         // 415
	"StatusRequestedRangeNotSatisfiable": http.StatusRequestedRangeNotSatisfiable, // 416
	"StatusExpectationFailed":            http.StatusExpectationFailed,            // 417
	"StatusTeapot":                       http.StatusTeapot,                       // 418
	"StatusMisdirectedRequest":           http.StatusMisdirectedRequest,           // 421
	"StatusUnprocessableEntity":          http.StatusUnprocessableEntity,          // 422
	"StatusLocked":                       http.StatusLocked,                       // 423
	"StatusFailedDependency":             http.StatusFailedDependency,             // 424
	"StatusTooEarly":                     http.StatusTooEarly,                     // 425
	"StatusUpgradeRequired":              http.StatusUpgradeRequired,              // 426
	"StatusPreconditionRequired":         http.StatusPreconditionRequired,         // 428
	"StatusTooManyRequests":              http.StatusTooManyRequests,              // 429
	"StatusRequestHeaderFieldsTooLarge":  http.StatusRequestHeaderFieldsTooLarge,  // 431
	"StatusUnavailableForLegalReasons":   http.StatusUnavailableForLegalReasons,   // 451

	// 5xx Server Error
	"StatusInternalServerError":           http.StatusInternalServerError,           // 500
	"StatusNotImplemented":                http.StatusNotImplemented,                // 501
	"StatusBadGateway":                    http.StatusBadGateway,                    // 502
	"StatusServiceUnavailable":            http.StatusServiceUnavailable,            // 503
	"StatusGatewayTimeout":                http.StatusGatewayTimeout,                // 504
	"StatusHTTPVersionNotSupported":       http.StatusHTTPVersionNotSupported,       // 505
	"StatusVariantAlsoNegotiates":         http.StatusVariantAlsoNegotiates,         // 506
	"StatusInsufficientStorage":           http.StatusInsufficientStorage,           // 507
	"StatusLoopDetected":                  http.StatusLoopDetected,                  // 508
	"StatusNotExtended":                   http.StatusNotExtended,                   // 510
	"StatusNetworkAuthenticationRequired": http.StatusNetworkAuthenticationRequired, // 511
}

var paramRe = regexp.MustCompile(`:([a-zA-Z0-9_]+)`)

func convertPathToOpenAPI(path string) string {
	return paramRe.ReplaceAllString(path, `{$1}`)
}

// BuildFuncMap creates a map of function names to their declarations.
func BuildFuncMap(files []*ast.File) map[string]*ast.FuncDecl {
	funcMap := make(map[string]*ast.FuncDecl)
	for _, file := range files {
		pkgName := ""
		if file.Name != nil {
			pkgName = file.Name.Name
		}
		ast.Inspect(file, func(n ast.Node) bool {
			// Handle regular functions
			if fn, isFn := n.(*ast.FuncDecl); isFn {
				var key string
				if fn.Recv == nil || len(fn.Recv.List) == 0 {
					// Only for top-level functions, use package prefix if not main
					if pkgName != "" && pkgName != "main" {
						key = pkgName + "." + fn.Name.Name
					} else {
						key = fn.Name.Name
					}
					funcMap[key] = fn
				} else {
					// For methods, always use TypeName.MethodName (no package prefix)
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
						methodKey := typeName + "." + fn.Name.Name
						funcMap[methodKey] = fn
					}
				}
			}
			return true
		})
	}
	return funcMap
}

func pkgPrefix(pkgName string) string {
	if pkgName != "" && pkgName != "main" {
		return pkgName + "."
	}
	return ""
}

// buildAliasMap creates a map of import aliases to their actual package names for a given file.
// e.g., `import myhttp "net/http"` -> {"myhttp": "http"}
func buildAliasMap(file *ast.File) map[string]string {
	aliases := make(map[string]string)
	for _, imp := range file.Imports {
		path := strings.Trim(imp.Path.Value, `"`)
		pkgName := path[strings.LastIndex(path, "/")+1:]

		if imp.Name != nil {
			// Alias is used, e.g., `myhttp "net/http"`
			aliases[imp.Name.Name] = pkgName
		} else {
			// No alias, the package name itself is the identifier
			aliases[pkgName] = pkgName
		}
	}
	return aliases
}

// getHandlerName extracts the qualified name of a handler function from an AST expression.
// It correctly handles identifiers, selectors (pkg.Func), and function calls (pkg.Func()).
func getHandlerName(arg ast.Expr, pkgName string, aliasMap map[string]string) string {
	switch v := arg.(type) {
	case *ast.Ident:
		// A local function, e.g., `myHandler`
		if pkgName != "" && pkgName != "main" {
			return pkgName + "." + v.Name
		}
		return v.Name

	case *ast.SelectorExpr:
		// A function from another package, e.g., `handlers.LoadCart`
		if pkgIdent, ok := v.X.(*ast.Ident); ok {
			// Resolve alias if it exists
			if realPkgName, exists := aliasMap[pkgIdent.Name]; exists {
				return realPkgName + "." + v.Sel.Name
			}
			// Fallback to the identifier name
			return pkgIdent.Name + "." + v.Sel.Name
		}

	case *ast.CallExpr:
		// A function call that returns a handler, e.g., `handlers.LoadCart(...)`
		return getHandlerName(v.Fun, pkgName, aliasMap)
	}

	return "" // Return empty if the handler expression is not recognized
}

// GetTypeName recursively extracts a type name from an AST expression.
func GetTypeName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.CallExpr:
		if sel, ok := t.Fun.(*ast.SelectorExpr); ok {
			if id, ok := sel.X.(*ast.Ident); ok {
				return id.Name + "." + sel.Sel.Name
			}
			return sel.Sel.Name
		}
	case *ast.SelectorExpr:
		if x, ok := t.X.(*ast.Ident); ok {
			return x.Name + "." + t.Sel.Name
		}
	case *ast.StarExpr:
		return GetTypeName(t.X)
	case *ast.CompositeLit:
		// For struct literals like User{}, it returns the type name.
		return GetTypeName(t.Type)
	case *ast.ArrayType:
		return "[]" + GetTypeName(t.Elt)
	case *ast.MapType:
		return "map[" + GetTypeName(t.Key) + "]" + GetTypeName(t.Value)
	}
	return ""
}

// Add a helper to resolve the type of an identifier in a function scope
func resolveVarTypeInFunc(fn *ast.FuncDecl, varName string, goFiles []*ast.File) string {
	if fn == nil || fn.Body == nil {
		return ""
	}
	var varType string
	// 1. Check function parameters
	for _, param := range fn.Type.Params.List {
		for _, name := range param.Names {
			if name.Name == varName {
				varType = GetTypeName(param.Type)
				return varType
			}
		}
	}
	// 2. Check variable declarations and range statements
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if varType != "" {
			return false
		}
		// var declarations
		if decl, ok := n.(*ast.DeclStmt); ok {
			if genDecl, ok := decl.Decl.(*ast.GenDecl); ok && genDecl.Tok == token.VAR {
				for _, spec := range genDecl.Specs {
					if vspec, ok := spec.(*ast.ValueSpec); ok {
						for i, name := range vspec.Names {
							if name.Name == varName {
								if vspec.Type != nil {
									varType = GetTypeName(vspec.Type)
								} else if len(vspec.Values) > i {
									varType = GetTypeName(vspec.Values[i])
								}
								return false
							}
						}
					}
				}
			}
		}
		// short variable declarations (:=)
		if assign, ok := n.(*ast.AssignStmt); ok && assign.Tok == token.DEFINE {
			for i, lhs := range assign.Lhs {
				if ident, ok := lhs.(*ast.Ident); ok && ident.Name == varName {
					if i < len(assign.Rhs) {
						// Try to get type from the right-hand side
						if comp, ok := assign.Rhs[i].(*ast.CompositeLit); ok {
							varType = GetTypeName(comp.Type)
						} else {
							// For other expressions, try to infer the type
							varType = GetTypeName(assign.Rhs[i])
						}
					}
					return false
				}
			}
		}
		// range statements
		if forStmt, ok := n.(*ast.RangeStmt); ok {
			// for k, v := range expr { ... }
			if forStmt.Value != nil {
				if ident, ok := forStmt.Value.(*ast.Ident); ok && ident.Name == varName {
					// Try to get the element type of the ranged expression
					if collIdent, ok := forStmt.X.(*ast.Ident); ok {
						collType := resolveVarTypeInFunc(fn, collIdent.Name, goFiles)
						if strings.HasPrefix(collType, "[]") {
							varType = strings.TrimPrefix(collType, "[]")
						} else if strings.HasPrefix(collType, "map[") {
							closeIdx := strings.Index(collType, "]")
							if closeIdx != -1 && closeIdx+1 < len(collType) {
								varType = collType[closeIdx+1:]
							}
						} else {
							// fallback: just use the type name
							varType = collType
						}
					} else {
						// fallback: try to get type name directly
						collType := GetTypeName(forStmt.X)
						if strings.HasPrefix(collType, "[]") {
							varType = strings.TrimPrefix(collType, "[]")
						} else if strings.HasPrefix(collType, "map[") {
							closeIdx := strings.Index(collType, "]")
							if closeIdx != -1 && closeIdx+1 < len(collType) {
								varType = collType[closeIdx+1:]
							}
						} else {
							varType = collType
						}
					}
					return false
				}
			}
		}
		return true
	})

	// 3. Check for global (package-level) variables
	for _, file := range goFiles {
		for _, decl := range file.Decls {
			if genDecl, ok := decl.(*ast.GenDecl); ok && genDecl.Tok == token.VAR {
				for _, spec := range genDecl.Specs {
					if vspec, ok := spec.(*ast.ValueSpec); ok {
						for i, name := range vspec.Names {
							if name.Name == varName {
								if vspec.Type != nil {
									varType = GetTypeName(vspec.Type)
								} else if len(vspec.Values) > i {
									varType = GetTypeName(vspec.Values[i])
								}
							}
						}
					}
				}
			}
		}
	}
	return varType
}

// Collects all import aliases for encoding/json and github.com/json-iterator/go in a file
func CollectJSONAliases(file *ast.File) map[string]struct{} {
	aliases := make(map[string]struct{})
	for _, imp := range file.Imports {
		importPath := strings.Trim(imp.Path.Value, "\"")
		if importPath == "encoding/json" || importPath == "github.com/json-iterator/go" {
			if imp.Name != nil {
				aliases[imp.Name.Name] = struct{}{}
			} else {
				// Default import name
				switch importPath {
				case "encoding/json":
					aliases["json"] = struct{}{}
				case "github.com/json-iterator/go":
					aliases["jsoniter"] = struct{}{}
				}
			}
		}
	}
	return aliases
}

// isJSONEncodeCall checks if a call is json.NewEncoder().Encode() or jsoniter.NewEncoder().Encode() (with alias support)
func isJSONEncodeCall(call *ast.CallExpr, aliases map[string]struct{}) bool {
	if len(call.Args) == 0 {
		return false
	}

	if se, ok := call.Fun.(*ast.SelectorExpr); ok {
		if se.Sel.Name == "Encode" {
			if callExpr, ok := se.X.(*ast.CallExpr); ok {
				if se2, ok := callExpr.Fun.(*ast.SelectorExpr); ok {
					if se2.Sel.Name == "NewEncoder" {
						if ident, ok := se2.X.(*ast.Ident); ok {
							_, found := aliases[ident.Name]
							return found
						}
					}
				}
			}
		}
	}
	return false
}

// isJSONDecodeCall checks if a call is json.NewDecoder().Decode() or jsoniter.NewDecoder().Decode() (with alias support)
func isJSONDecodeCall(call *ast.CallExpr, aliases map[string]struct{}) bool {
	if len(call.Args) == 0 {
		return false
	}

	if se, ok := call.Fun.(*ast.SelectorExpr); ok {
		if se.Sel.Name == "Decode" {
			if callExpr, ok := se.X.(*ast.CallExpr); ok {
				if se2, ok := callExpr.Fun.(*ast.SelectorExpr); ok {
					if se2.Sel.Name == "NewDecoder" {
						if ident, ok := se2.X.(*ast.Ident); ok {
							_, found := aliases[ident.Name]
							return found
						}
					}
				}
			}
		}
	}
	return false
}

// findAllStatusCodes collects all WriteHeader calls with their status codes and positions
func findAllStatusCodes(fn *ast.FuncDecl) map[token.Pos]int {
	statusCodes := make(map[token.Pos]int)
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		if se, ok := call.Fun.(*ast.SelectorExpr); ok {
			if se.Sel.Name == "WriteHeader" {
				if len(call.Args) > 0 {
					statusCode := resolveStatusCode(call.Args[0])
					if statusCode > 0 {
						statusCodes[call.Pos()] = statusCode
					}
				}
			}
		}
		return true
	})
	return statusCodes
}

// findBestStatusCode finds the most appropriate status code for a given position
func findBestStatusCode(statusCodes map[token.Pos]int, pos token.Pos) int {
	// Find the closest WriteHeader call before this position
	var bestCode int
	var bestPos token.Pos

	for codePos, code := range statusCodes {
		if codePos < pos && (bestPos == 0 || codePos > bestPos) {
			bestCode = code
			bestPos = codePos
		}
	}

	return bestCode
}

// resolveStatusCode extracts the status code from an AST expression
func resolveStatusCode(arg ast.Expr) int {
	switch v := arg.(type) {
	case *ast.BasicLit:
		if v.Kind == token.INT {
			if code, err := strconv.Atoi(v.Value); err == nil {
				return code
			}
		}
	case *ast.Ident:
		// Handle constants like StatusOK (without package prefix)
		if code, exists := httpStatusMap[v.Name]; exists {
			return code
		}
	case *ast.SelectorExpr:
		// Handle http.StatusXXX or other package.StatusXXX
		if pkg, ok := v.X.(*ast.Ident); ok {
			// For http.StatusXXX, we can use our map directly
			if pkg.Name == "http" {
				if code, exists := httpStatusMap[v.Sel.Name]; exists {
					return code
				}
			}
			// Also handle full qualified names like "http.StatusOK"
			fullName := pkg.Name + "." + v.Sel.Name
			if strings.HasPrefix(fullName, "http.Status") {
				statusName := strings.TrimPrefix(fullName, "http.")
				if code, exists := httpStatusMap[statusName]; exists {
					return code
				}
			}
		}
	}
	return 0
}

// extractMapKeysFromCompositeLit extracts actual keys from map literals for better schema generation
func extractMapKeysFromCompositeLit(comp *ast.CompositeLit, mapType *types.Map, info *types.Info) map[string]string {
	mapKeys := make(map[string]string)
	valueType := mapType.Elem().String()

	for _, elt := range comp.Elts {
		if kv, ok := elt.(*ast.KeyValueExpr); ok {
			if key, ok := kv.Key.(*ast.BasicLit); ok && key.Kind == token.STRING {
				keyName := strings.Trim(key.Value, "\"")
				valType := GetTypeName(kv.Value)
				if valType == "" {
					// For string literals, use "string" type
					if lit, ok := kv.Value.(*ast.BasicLit); ok && lit.Kind == token.STRING {
						valType = "string"
					} else if ident, ok := kv.Value.(*ast.Ident); ok {
						// For identifiers, try to resolve the type
						if info != nil {
							if obj := info.Uses[ident]; obj != nil {
								if obj.Type() != nil {
									valType = obj.Type().String()
								}
							}
						}
						// If we can't resolve the type, assume it's a string for common cases
						if valType == "" {
							valType = "string"
						}
					} else {
						valType = valueType
					}
				}

				// Normalize type names to avoid creating unnecessary schemas
				if valType == "string" || valType == "int" || valType == "bool" || valType == "float64" {
					// Keep primitive types as-is
				} else if strings.HasPrefix(valType, "string") {
					valType = "string"
				}

				mapKeys[keyName] = valType
			}
		}
	}

	return mapKeys
}

func toIfaceSlice[T any](in []T) []interface{} {
	out := make([]interface{}, len(in))
	for i, v := range in {
		out[i] = v
	}
	return out
}

// Generic status and response extraction for Gin/Echo/Fiber
// Returns (statusCode, responseArg)
func ExtractStatusAndResponseFromCall(call *ast.CallExpr, info *types.Info) (int, ast.Expr) {
	if se, ok := call.Fun.(*ast.SelectorExpr); ok {
		// Chained: c.Status(204).JSON(...)
		if se.Sel.Name == "JSON" {
			// Check for c.Status(204).JSON(...)
			if recvCall, ok := se.X.(*ast.CallExpr); ok {
				if recvSel, ok := recvCall.Fun.(*ast.SelectorExpr); ok && recvSel.Sel.Name == "Status" && len(recvCall.Args) == 1 {
					return resolveStatusCode(recvCall.Args[0]), call.Args[0]
				}
			}
			// Direct: c.JSON(400, ...)
			if len(call.Args) >= 2 {
				return resolveStatusCode(call.Args[0]), call.Args[1]
			} else if len(call.Args) == 1 {
				return http.StatusOK, call.Args[0] // default
			}
		}
	}
	return 0, nil
}

// resolveHandlerFunc tries to resolve the actual handler function declaration.
// It handles simple functions, aliased package functions, handler factories, and methods on structs.
func resolveHandlerFunc(expr ast.Expr, funcMap map[string]*ast.FuncDecl, pkgName string, aliasMap map[string]string, info *types.Info) (string, *ast.FuncDecl) {
	// First, try name-based resolution for functions and function factories
	switch h := expr.(type) {
	case *ast.Ident:
		handlerName := getHandlerName(expr, pkgName, aliasMap)
		if fn, ok := funcMap[handlerName]; ok {
			return handlerName, fn
		}

	case *ast.SelectorExpr:
		// Try name-based resolution first (for pkg.Function)
		handlerName := getHandlerName(expr, pkgName, aliasMap)
		if fn, ok := funcMap[handlerName]; ok {
			return handlerName, fn
		}
	// If that fails, fall through to the type-based resolution below for controller.Method

	case *ast.CallExpr:
		// This part handles handler factories, e.g., handlers.LoadCart(deps)
		funcName := getHandlerName(h.Fun, pkgName, aliasMap)
		if fn, ok := funcMap[funcName]; ok {
			if fn.Type.Results != nil && len(fn.Type.Results.List) > 0 {
				if _, ok := fn.Type.Results.List[0].Type.(*ast.FuncType); ok {
					// It returns a function. Try to find the literal if possible.
					for _, stmt := range fn.Body.List {
						if ret, ok := stmt.(*ast.ReturnStmt); ok && len(ret.Results) > 0 {
							if funLit, ok := ret.Results[0].(*ast.FuncLit); ok {
								// Return the factory name, but the func literal's body/type
								return funcName, &ast.FuncDecl{
									Name: &ast.Ident{Name: funcName},
									Type: funLit.Type,
									Body: funLit.Body,
								}
							}
						}
					}
					return funcName, fn // Return the factory function itself as a fallback
				}
			}
		}
		// Now, also try type-based resolution on h.Fun if it's a SelectorExpr
		if info != nil {
			if selExpr, ok := h.Fun.(*ast.SelectorExpr); ok {
				fmt.Printf("[DEBUG] selExpr: %v\n", selExpr.Sel.Name)
				fmt.Printf("[DEBUG] Found Selections: %v\n", info.Selections)
				if tv, ok := info.Selections[selExpr]; ok && tv.Obj() != nil {
					if fn, ok := tv.Obj().(*types.Func); ok {
						if recv := fn.Signature().Recv(); recv != nil {
							var namedType *types.Named
							if ptr, ok := recv.Type().(*types.Pointer); ok {
								namedType, _ = ptr.Elem().(*types.Named)
							} else {
								namedType, _ = recv.Type().(*types.Named)
							}
							if namedType != nil {
								typeName := namedType.Obj().Name()
								methodKey := typeName + "." + fn.Name()
								if methodFunc, exists := funcMap[methodKey]; exists {
									return methodKey, methodFunc
								}
								// Heuristic: If typeName is an interface, try all concrete types
								if _, ok := namedType.Underlying().(*types.Interface); ok {
									for k, methodFunc := range funcMap {
										if strings.HasSuffix(k, "."+fn.Name()) && k != methodKey {
											// Optionally: check that the method's signature matches
											return k, methodFunc
										}
									}
								}
							}
						}
					}
				}
			}
		}
	}

	// If name-based resolution fails and we have type info, try type-based resolution for methods.
	if info != nil {
		if selExpr, ok := expr.(*ast.SelectorExpr); ok {
			fmt.Printf("[DEBUG] selExpr: %v\n", selExpr.Sel.Name)
			fmt.Printf("[DEBUG] Found Selections: %v\n", info.Selections)
			// This handles controller.Method where controller is a variable.
			if tv, ok := info.Selections[selExpr]; ok && tv.Obj() != nil {
				// tv.Obj() is the method's `types.Func`.
				if fn, ok := tv.Obj().(*types.Func); ok {
					// The receiver of the method gives us the struct type.
					if recv := fn.Signature().Recv(); recv != nil {
						// recv.Type() is the type of the receiver, e.g., `*main.APIController`
						// We need the underlying named type to get the TypeName.
						var namedType *types.Named
						if ptr, ok := recv.Type().(*types.Pointer); ok {
							namedType, _ = ptr.Elem().(*types.Named)
						} else {
							namedType, _ = recv.Type().(*types.Named)
						}

						if namedType != nil {
							// namedType.Obj().Name() is the TypeName, e.g., `APIController`
							typeName := namedType.Obj().Name()
							methodKey := typeName + "." + fn.Name()
							if methodFunc, exists := funcMap[methodKey]; exists {
								return methodKey, methodFunc
							}
						}
					}
				}
			}
		}
	}
	return "", nil
}

// FuncSignature returns a string representation of a function's signature.
func FuncSignature(fn *ast.FuncDecl) string {
	if fn == nil || fn.Type == nil {
		return ""
	}
	params := []string{}
	if fn.Type.Params != nil {
		for _, field := range fn.Type.Params.List {
			typeStr := ExprToString(field.Type)
			for range field.Names {
				params = append(params, typeStr)
			}
			if len(field.Names) == 0 {
				params = append(params, typeStr)
			}
		}
	}
	results := []string{}
	if fn.Type.Results != nil {
		for _, field := range fn.Type.Results.List {
			typeStr := ExprToString(field.Type)
			for range field.Names {
				results = append(results, typeStr)
			}
			if len(field.Names) == 0 {
				results = append(results, typeStr)
			}
		}
	}
	return fmt.Sprintf("(%s) (%s)", strings.Join(params, ", "), strings.Join(results, ", "))
}

// FuncPosition returns the file:line:col position of a function.
func FuncPosition(fn *ast.FuncDecl, file *ast.File) string {
	if fn == nil || file == nil {
		return ""
	}
	pos := fn.Pos()
	return positionString(pos, file)
}

// VarPosition returns the file:line:col position of a variable identifier.
func VarPosition(ident *ast.Ident, file *ast.File) string {
	if ident == nil || file == nil {
		return ""
	}
	pos := ident.Pos()
	return positionString(pos, file)
}

// ExprToString returns a string representation of an expression.
func ExprToString(expr ast.Expr) string {
	if expr == nil {
		return ""
	}
	return fmt.Sprintf("%#v", expr)
}

// positionString returns a string representation of a position in a file.
func positionString(pos token.Pos, file *ast.File) string {
	if file == nil {
		return ""
	}
	fset := token.NewFileSet()
	posn := fset.Position(pos)
	return fmt.Sprintf("%s:%d:%d", posn.Filename, posn.Line, posn.Column)
}
