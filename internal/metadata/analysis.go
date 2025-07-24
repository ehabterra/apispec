package metadata

import (
	"go/ast"
	"go/token"
	"go/types"
	"strings"
)

// implementsInterface checks if a struct implements an interface
func implementsInterface(structMethods map[int]int, ifaceType *Type) bool {
	for _, ifaceMethod := range ifaceType.Methods {
		// Check if method exists with matching signature
		if structSignatureStr, exists := structMethods[ifaceMethod.Name]; !exists {
			return false
		} else if structSignatureStr != ifaceMethod.SignatureStr {
			// Method exists but signature doesn't match
			return false
		}
	}
	return true
}

// getEnclosingFunctionName finds the function that contains a given position
func getEnclosingFunctionName(file *ast.File, pos token.Pos) (string, string) {
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		if fn.Pos() <= pos && pos <= fn.End() {
			var parts []string

			// Check if this is a method (has a receiver)
			if fn.Recv != nil && len(fn.Recv.List) > 0 {
				recv := fn.Recv.List[0]
				recvType := getTypeName(recv.Type)
				parts = append(parts, recvType)
			}

			return fn.Name.Name, strings.Join(parts, ".")
		}
	}
	return "", ""
}

// getDefaultImportName returns the default import name for an import path (last non-version segment)
func getDefaultImportName(importPath string) string {
	parts := strings.Split(importPath, "/")
	if len(parts) == 0 {
		return ""
	}
	last := parts[len(parts)-1]
	// If last is a version (e.g., v5), use the one before it
	if len(parts) > 1 && strings.HasPrefix(last, "v") && len(last) > 1 && last[1] >= '0' && last[1] <= '9' {
		return parts[len(parts)-2]
	}
	return last
}

// getCalleeFunctionNameAndPackage extracts function name, package, and receiver type from a call expression
func getCalleeFunctionNameAndPackage(expr ast.Expr, file *ast.File, pkgName string, fileToInfo map[*ast.File]*types.Info, funcMap map[string]*ast.FuncDecl, fset *token.FileSet) (string, string, string) {
	switch x := expr.(type) {
	case *ast.Ident:
		// Simple identifier - assume it's a function in the current package
		return x.Name, pkgName, ""

	case *ast.SelectorExpr:
		if ident, ok := x.X.(*ast.Ident); ok {
			// Try to match ident.Name to an import alias or default import name
			for _, imp := range file.Imports {
				importPath := strings.Trim(imp.Path.Value, "\"")
				defaultName := getDefaultImportName(importPath)
				if (imp.Name != nil && imp.Name.Name == ident.Name) ||
					(imp.Name == nil && defaultName == ident.Name) {
					return x.Sel.Name, importPath, ""
				}
			}
			// If not an import, try to resolve as variable/method
			if info, exists := fileToInfo[file]; exists {
				if obj := info.ObjectOf(ident); obj != nil {
					if varObj, ok := obj.(*types.Var); ok {
						t := varObj.Type()
						receiverType := getReceiverTypeString(t)
						switch t := t.(type) {
						case *types.Named:
							if t.Obj().Pkg() != nil {
								return x.Sel.Name, t.Obj().Pkg().Path(), receiverType
							}
							return x.Sel.Name, pkgName, receiverType
						case *types.Pointer:
							if named, ok := t.Elem().(*types.Named); ok {
								return x.Sel.Name, named.Obj().Pkg().Path(), receiverType
							}
						case *types.Interface:
							// For interfaces, fallback to current package
							return x.Sel.Name, pkgName, receiverType
						}
					}
				}
			}
			// Default: assume it's a method call in current package
			return x.Sel.Name, pkgName, ""
		}

		// Handle more complex receiver expressions
		if info, exists := fileToInfo[file]; exists {
			if tv := info.Types[x.X]; tv.Type != nil {
				receiverType := getReceiverTypeString(tv.Type)
				switch t := tv.Type.(type) {
				case *types.Named:
					if t.Obj().Pkg() != nil {
						return x.Sel.Name, t.Obj().Pkg().Path(), receiverType
					}
					return x.Sel.Name, pkgName, receiverType
				case *types.Pointer:
					if named, ok := t.Elem().(*types.Named); ok {
						return x.Sel.Name, named.Obj().Pkg().Path(), receiverType
					}
				case *types.Interface:
					// For interfaces, fallback to current package
					return x.Sel.Name, pkgName, receiverType
				}
			}
		}
		return x.Sel.Name, pkgName, ""

	case *ast.CallExpr:
		return getCalleeFunctionNameAndPackage(x.Fun, file, pkgName, fileToInfo, funcMap, fset)

	case *ast.IndexExpr:
		// Handle indexed expressions like array[index] or map[key]
		// Recursively analyze the indexed expression (X) to find function calls
		return getCalleeFunctionNameAndPackage(x.X, file, pkgName, fileToInfo, funcMap, fset)

	case *ast.IndexListExpr:
		// Handle generic function or type instantiations like Func[T1, T2]
		// Recursively analyze the X field to find function calls
		return getCalleeFunctionNameAndPackage(x.X, file, pkgName, fileToInfo, funcMap, fset)
	}
	return "", "", ""
}

// getReceiverTypeString gets a string representation of the receiver type
func getReceiverTypeString(t types.Type) string {
	switch t := t.(type) {
	case *types.Named:
		name := t.Obj().Name()
		return name
	case *types.Pointer:
		// Handle pointer types like *MyStruct
		return "*" + getReceiverTypeString(t.Elem())
	case *types.Interface:
		return "interface"
	default:
		return ""
	}
}

// analyzeAssignmentValue analyzes the value being assigned to determine concrete types using full metadata tracing
func analyzeAssignmentValue(expr ast.Expr, info *types.Info, funcName string, pkgName string, metadata *Metadata, fset *token.FileSet) (string, *CallArgument) {
	if expr == nil {
		return pkgName, nil
	}

	// Use type info if available (works for stdlib and user code)
	if info != nil {
		if typ := info.TypeOf(expr); typ != nil {
			return pkgName, &CallArgument{Kind: kindIdent, Type: typ.String()}
		}
	}

	switch e := expr.(type) {
	case *ast.Ident:
		// Use TraceVariableOrigin for identifiers
		_, originPkg, originType, _ := TraceVariableOrigin(e.Name, funcName, pkgName, metadata)
		return originPkg, originType

	case *ast.SelectorExpr:
		// Try to resolve selector as var.field or package.type
		if ident, ok := e.X.(*ast.Ident); ok {
			// Try tracing the base identifier
			_, basePkg, baseType, _ := TraceVariableOrigin(ident.Name, funcName, pkgName, metadata)
			return basePkg, baseType
		}
		// Fallback: just get type name
		return pkgName, &CallArgument{Kind: kindIdent, Type: getTypeName(e)}

	case *ast.CallExpr:
		// For function calls, try to trace the return value
		if funIdent, ok := e.Fun.(*ast.Ident); ok {
			// Direct function call
			_, originPkg, originType, _ := TraceVariableOrigin(funIdent.Name, funcName, pkgName, metadata)
			return originPkg, originType
		}
		if sel, ok := e.Fun.(*ast.SelectorExpr); ok {
			// Method or package function call
			if _, ok := sel.X.(*ast.Ident); ok {
				_, originPkg, originType, _ := TraceVariableOrigin(sel.Sel.Name, funcName, pkgName, metadata)
				return originPkg, originType
			}
		}

		callType := ExprToCallArgument(e, info, pkgName, fset)
		// Fallback: just get type name
		return pkgName, &callType

	case *ast.TypeAssertExpr:
		// Type assertion: try to get asserted type
		if e.Type != nil {
			callType := ExprToCallArgument(e.Type, info, pkgName, fset)
			return pkgName, &callType
		}
		return pkgName, &CallArgument{Kind: kindIdent, Type: "interface{}"}

	case *ast.StarExpr:
		// Pointer dereference: trace the base
		return analyzeAssignmentValue(e.X, info, funcName, pkgName, metadata, fset)

	case *ast.CompositeLit:
		// Struct or array literal: get type name
		callType := ExprToCallArgument(e.Type, info, pkgName, fset)
		return pkgName, &callType

	default:
		callType := ExprToCallArgument(e, info, pkgName, fset)
		return pkgName, &callType
	}
}

// TraceVariableOrigin recursively traces the origin and type of a variable/parameter through the call graph.
// It supports cross-file and cross-package tracing.
// Returns: origin variable/parameter name, package, type (if resolvable), and the caller's function name.
func TraceVariableOrigin(
	varName string,
	funcName string,
	pkgName string,
	metadata *Metadata,
) (originVar string, originPkg string, originType *CallArgument, callerFuncName string) {
	visited := make(map[string]struct{})
	return traceVariableOriginHelper(varName, funcName, pkgName, metadata, visited)
}

func traceVariableOriginHelper(
	varName string,
	funcName string,
	pkgName string,
	metadata *Metadata,
	visited map[string]struct{},
) (originVar string, originPkg string, originType *CallArgument, callerFuncName string) {
	key := pkgName + "." + funcName + ":" + varName
	if _, ok := visited[key]; ok {
		return varName, pkgName, nil, funcName // Prevent infinite recursion, return current funcName as caller
	}
	visited[key] = struct{}{}

	funcNameIndex := metadata.StringPool.Get(funcName)
	pkgNameIndex := metadata.StringPool.Get(pkgName)

	// Look for a call graph edge where this function is the callee
	for _, edge := range metadata.CallGraph {
		if edge.Callee.Name == funcNameIndex && edge.Callee.Pkg == pkgNameIndex {
			callerName := metadata.StringPool.GetString(edge.Caller.Name)
			callerPkg := metadata.StringPool.GetString(edge.Caller.Pkg)

			// Type param (generic)
			if concrete, ok := edge.TypeParamMap[varName]; ok && concrete != "" {
				return varName, pkgName, &CallArgument{Kind: kindIdent, Type: concrete}, callerName
			}

			// See if this parameter is mapped
			if arg, ok := edge.ParamArgMap[varName]; ok {
				switch arg.Kind {
				case kindIdent:
					_, _, t, f := traceVariableOriginHelper(arg.Name, callerName, callerPkg, metadata, visited)
					return arg.Name, callerPkg, t, f
				case kindUnary, kindStar:
					if arg.X != nil {
						_, _, t, f := traceVariableOriginHelper(arg.X.Name, callerName, callerPkg, metadata, visited)
						return arg.X.Name, callerPkg, t, f
					}
				case kindSelector:
					if arg.X != nil {
						baseVar, basePkg, baseType, f := traceVariableOriginHelper(arg.X.Name, callerName, callerPkg, metadata, visited)
						return baseVar + "." + arg.Sel, basePkg, baseType, f
					}
				case kindCall:
					if arg.Fun != nil {
						_, _, t, f := traceVariableOriginHelper(arg.Fun.Name, callerName, callerPkg, metadata, visited)
						return arg.Fun.Name, callerPkg, t, f
					}
				case kindTypeAssert:
					// For type assertions, use the asserted type as the concrete type
					if arg.Fun != nil && arg.Fun.Type != "" {
						return varName, pkgName, arg.Fun, callerName
					}
				default:
					if arg.Kind != "" {
						return arg.Name, pkgName, &arg, callerName
					}
				}
			}

			break // No need to search again for another edge
		}
	}

	// Try to find assignment in the same pkg
	if pkg, ok := metadata.Packages[pkgName]; ok {
		for _, file := range pkg.Files {
			if v, ok := file.Variables[varName]; ok {
				return varName, pkgName, &CallArgument{Kind: kindIdent, Type: metadata.StringPool.GetString(v.Type)}, funcName
			}

			if fn, ok := file.Functions[funcName]; ok {
				if assigns, ok := fn.AssignmentMap[varName]; ok && len(assigns) > 0 {
					// Use the most recent assignment (last in slice)
					assign := assigns[len(assigns)-1]
					// If the assignment is an alias (Value.Kind == kindIdent), recursively trace the RHS
					if assign.Value.Kind == kindIdent && assign.Value.Name != varName {
						_, _, t, f := traceVariableOriginHelper(assign.Value.Name, funcName, pkgName, metadata, visited)
						return assign.Value.Name, pkgName, t, f
					}
					// If the assignment is from a function call, follow the return value
					if assign.CalleeFunc != "" && assign.CalleePkg != "" {
						calleePkg, ok := metadata.Packages[assign.CalleePkg]
						if ok {
							for _, calleeFile := range calleePkg.Files {
								if calleeFn, ok := calleeFile.Functions[assign.CalleeFunc]; ok {
									retIdx := assign.ReturnIndex
									if retIdx < len(calleeFn.ReturnVars) {
										retArg := calleeFn.ReturnVars[retIdx]
										if retArg.Kind == kindIdent && retArg.Name != "" {
											_, _, t, f := traceVariableOriginHelper(retArg.Name, assign.CalleeFunc, assign.CalleePkg, metadata, visited)
											return retArg.Name, assign.CalleePkg, t, f
										}
										// For literals or other expressions, return as is
										return retArg.Name, assign.CalleePkg, &retArg, funcName
									}
								}
							}
						}
					}
					return varName, pkgName, &assign.Value, funcName
				}
			}
		}
	}

	// Fallback: return as is
	return varName, pkgName, nil, funcName
}
