package parser

import (
	"go/ast"
	"go/token"
	"go/types"
	"net/http"
	"regexp"
	"strconv"
	"strings"
)

var paramRe = regexp.MustCompile(`:([a-zA-Z0-9_]+)`)

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

func convertPathToOpenAPI(path string) string {
	return paramRe.ReplaceAllString(path, `{$1}`)
}

// buildFuncMap creates a map of function names to their declarations.
func buildFuncMap(files []*ast.File) map[string]*ast.FuncDecl {
	funcMap := make(map[string]*ast.FuncDecl)
	for _, file := range files {
		for _, decl := range file.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok {
				funcMap[fn.Name.Name] = fn
			}
		}
	}
	return funcMap
}

// getHandlerName extracts the name of a handler function from an AST expression.
func getHandlerName(arg ast.Expr) string {
	if ident, ok := arg.(*ast.Ident); ok {
		return ident.Name
	}
	return ""
}

// getTypeName recursively extracts a type name from an AST expression.
func getTypeName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.SelectorExpr:
		if x, ok := t.X.(*ast.Ident); ok {
			return x.Name + "." + t.Sel.Name
		}
	case *ast.StarExpr:
		return getTypeName(t.X)
	case *ast.CompositeLit:
		// For struct literals like User{}, it returns the type name.
		return getTypeName(t.Type)
	case *ast.ArrayType:
		return "[]" + getTypeName(t.Elt)
	case *ast.MapType:
		return "map[" + getTypeName(t.Key) + "]" + getTypeName(t.Value)
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
				varType = getTypeName(param.Type)
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
									varType = getTypeName(vspec.Type)
								} else if len(vspec.Values) > i {
									varType = getTypeName(vspec.Values[i])
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
							varType = getTypeName(comp.Type)
						} else {
							// For other expressions, try to infer the type
							varType = getTypeName(assign.Rhs[i])
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
						collType := getTypeName(forStmt.X)
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
									varType = getTypeName(vspec.Type)
								} else if len(vspec.Values) > i {
									varType = getTypeName(vspec.Values[i])
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
				valType := getTypeName(kv.Value)
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
