package metadata

import (
	"fmt"
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
// It can return either a declared function (*ast.FuncDecl) or a function literal (*ast.FuncLit)
func getEnclosingFunctionName(file *ast.File, pos token.Pos, info *types.Info, fset *token.FileSet, meta *Metadata) (string, string, string) {
	// First, check for function literals (they can be nested inside declared functions)
	funcLit := findEnclosingFunctionLiteral(file, pos)
	if funcLit != nil {
		// Return the function literal identifier (e.g., "FuncLitmain.go:42:15")
		position := getPosition(funcLit.Pos(), fset)
		var signatureStr string
		if fnType := info.TypeOf(funcLit.Type); fnType != nil {
			signatureStr = fnType.String()
		}
		return fmt.Sprintf("FuncLit:%s", position), "", signatureStr
	}

	// Fallback to declared functions
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
				recvType := getTypeName(recv.Type, info)
				parts = append(parts, recvType)
			}

			var signatureStr string
			signature := ExprToCallArgument(fn.Type, info, "", fset, meta)
			signatureStr = CallArgToString(signature)

			return fn.Name.Name, strings.Join(parts, "."), signatureStr
		}
	}
	return "", "", ""
}

// findEnclosingFunctionLiteral recursively searches for the innermost function literal containing the position
func findEnclosingFunctionLiteral(file *ast.File, pos token.Pos) *ast.FuncLit {
	var found *ast.FuncLit

	ast.Inspect(file, func(n ast.Node) bool {
		if n == nil {
			return true
		}

		// Check if this node contains our position
		if n.Pos() <= pos && pos <= n.End() {
			if funcLit, ok := n.(*ast.FuncLit); ok {
				// This is a function literal that contains our position
				// Keep the innermost one (most recent)
				found = funcLit
			}
		}

		return true
	})

	return found
}

// findParentFunction finds the parent function that contains a function literal
func findParentFunction(file *ast.File, pos token.Pos, info *types.Info, fset *token.FileSet, meta *Metadata) (string, string, string) {
	// Find the function literal first
	funcLit := findEnclosingFunctionLiteral(file, pos)
	if funcLit == nil {
		return "", "", ""
	}

	// Now find the function that contains this function literal
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		// Check if this function contains the function literal
		if fn.Pos() <= funcLit.Pos() && funcLit.End() <= fn.End() {
			var parts []string

			// Check if this is a method (has a receiver)
			if fn.Recv != nil && len(fn.Recv.List) > 0 {
				recv := fn.Recv.List[0]
				recvType := getTypeName(recv.Type, info)
				parts = append(parts, recvType)
			}

			var signatureStr string
			signature := ExprToCallArgument(fn.Type, info, "", fset, meta)
			if signature != nil {
				signatureStr = CallArgToString(signature)
			}

			return fn.Name.Name, strings.Join(parts, "."), signatureStr
		}
	}
	return "", "", ""
}

// DefaultImportName returns the default import name for an import path (last non-version segment)
func DefaultImportName(importPath string) string {
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

// isTypeConversion checks if a CallExpr represents a type conversion rather than a function call
func isTypeConversion(call *ast.CallExpr, info *types.Info) bool {
	if info == nil {
		return false
	}

	// Check if the function part is a type rather than a function
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		// Check if this identifier refers to a type
		if obj := info.ObjectOf(fun); obj != nil {
			_, isTypeName := obj.(*types.TypeName)
			return isTypeName
		}
	case *ast.SelectorExpr:
		// Check if this selector refers to a type (e.g., pkg.TypeName)
		if obj := info.ObjectOf(fun.Sel); obj != nil {
			_, isTypeName := obj.(*types.TypeName)
			return isTypeName
		}
	case *ast.ArrayType, *ast.SliceExpr, *ast.MapType, *ast.ChanType:
		// These are definitely type expressions
		return true
	case *ast.StarExpr:
		// Pointer type conversion
		return true
	case *ast.InterfaceType, *ast.StructType, *ast.FuncType:
		// These are type expressions
		return true
	}

	// Additional check: if the call has exactly one argument and the Fun resolves to a type
	if len(call.Args) == 1 {
		if tv, exists := info.Types[call.Fun]; exists && tv.IsType() {
			return true
		}
	}

	return false
}

// getCalleeFunctionNameAndPackage extracts function name, package, and receiver type from a call expression
func getCalleeFunctionNameAndPackage(expr ast.Expr, file *ast.File, pkgName string, fileToInfo map[*ast.File]*types.Info, funcMap map[string]*ast.FuncDecl, fset *token.FileSet) (string, string, string) {
	switch x := expr.(type) {
	case *ast.Ident:
		// Simple identifier - assume it's a function in the current package
		return x.Name, pkgName, ""

	case *ast.SelectorExpr:
		if ident, ok := x.X.(*ast.Ident); ok {

			// If not an import, try to resolve as variable/method
			if info, exists := fileToInfo[file]; exists {
				if obj := info.ObjectOf(ident); obj != nil {
					if pkg, ok := obj.(*types.PkgName); ok {
						return x.Sel.Name, pkg.Imported().Path(), ""
					} else if varObj, ok := obj.(*types.Var); ok {
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
			arg := NewCallArgument(metadata)
			arg.SetKind(KindIdent)
			arg.SetType(typ.String())
			return pkgName, arg
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
		arg := NewCallArgument(metadata)
		arg.SetKind(KindIdent)
		arg.SetType(getTypeName(e, info))
		return pkgName, arg

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

		callType := ExprToCallArgument(e, info, pkgName, fset, metadata)
		// Fallback: just get type name
		return pkgName, callType

	case *ast.TypeAssertExpr:
		// Type assertion: try to get asserted type
		if e.Type != nil {
			callType := ExprToCallArgument(e.Type, info, pkgName, fset, metadata)
			return pkgName, callType
		}
		arg := NewCallArgument(metadata)
		arg.SetKind(KindIdent)
		arg.SetType("interface{}")
		return pkgName, arg

	case *ast.StarExpr:
		// Pointer dereference: trace the base
		return analyzeAssignmentValue(e.X, info, funcName, pkgName, metadata, fset)

	case *ast.CompositeLit:
		// Struct or array literal: get type name
		callType := ExprToCallArgument(e.Type, info, pkgName, fset, metadata)
		return pkgName, callType

	default:
		callType := ExprToCallArgument(e, info, pkgName, fset, metadata)
		return pkgName, callType
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
	if varName == "" {
		return varName, pkgName, nil, funcName
	}

	// Optimize key generation using string builder
	var keyBuilder strings.Builder
	estimatedLen := len(pkgName) + len(funcName) + len(varName) + 3
	keyBuilder.Grow(estimatedLen)
	keyBuilder.WriteString(pkgName)
	keyBuilder.WriteByte('.')
	keyBuilder.WriteString(funcName)
	keyBuilder.WriteByte(':')
	keyBuilder.WriteString(varName)
	key := keyBuilder.String()

	if _, ok := visited[key]; ok {
		return varName, pkgName, nil, funcName // Prevent infinite recursion, return current funcName as caller
	}
	visited[key] = struct{}{}

	// Check cache first for performance optimization
	if metadata.traceVariableCache != nil {
		if cached, exists := metadata.traceVariableCache[key]; exists {
			return cached.OriginVar, cached.OriginPkg, cached.OriginType, cached.CallerFuncName
		}
	}

	// Helper function to cache and return results
	cacheAndReturn := func(originVar, originPkg, callerFuncName string, originType *CallArgument) (string, string, *CallArgument, string) {
		result := TraceVariableResult{
			OriginVar:      originVar,
			OriginPkg:      originPkg,
			OriginType:     originType,
			CallerFuncName: callerFuncName,
		}
		// Only cache results if packages are populated (to avoid caching incomplete results during metadata generation)
		if metadata.traceVariableCache != nil && len(metadata.Packages) > 0 {
			metadata.traceVariableCache[key] = result
		}
		return originVar, originPkg, originType, callerFuncName
	}

	// Cache string pool lookups
	funcNameIndex := metadata.StringPool.Get(funcName)
	pkgNameIndex := metadata.StringPool.Get(pkgName)

	// Look for a call graph edge where this function is the callee
	for _, edge := range metadata.CallGraph {
		if edge.Callee.Name == funcNameIndex && edge.Callee.Pkg == pkgNameIndex {
			callerName := metadata.StringPool.GetString(edge.Caller.Name)
			callerPkg := metadata.StringPool.GetString(edge.Caller.Pkg)

			// Type param (generic)
			if concrete, ok := edge.TypeParamMap[varName]; ok && concrete != "" {
				arg := NewCallArgument(metadata)
				arg.SetKind(KindIdent)
				arg.SetType(concrete)
				return cacheAndReturn(varName, pkgName, callerName, arg)
			}

			// See if this parameter is mapped
			if arg, ok := edge.ParamArgMap[varName]; ok {
				paramName := CallArgToString(&arg)

				spaceIndex := strings.LastIndex(paramName, " ")
				if spaceIndex > -1 {
					paramName = paramName[spaceIndex+1:]
				}
				bracketIndex := strings.Index(paramName, "(")
				if bracketIndex > -1 {
					paramName = paramName[:bracketIndex]
				}

				baseVar, basePkg, baseType, f := traceVariableOriginHelper(paramName, callerName, callerPkg, metadata, visited)
				return baseVar, basePkg, baseType, f
			}

			break // No need to search again for another edge
		}
	}

	// Try to find assignment in the same pkg
	if pkg, ok := metadata.Packages[pkgName]; ok {
		for _, file := range pkg.Files {
			if v, ok := file.Variables[varName]; ok {
				arg := NewCallArgument(metadata)
				arg.SetKind(KindIdent)
				arg.Type = v.Type
				return cacheAndReturn(varName, pkgName, funcName, arg)
			}

			if fn, ok := file.Functions[funcName]; ok {
				if assigns, ok := fn.AssignmentMap[varName]; ok && len(assigns) > 0 {
					// Use the most recent assignment (last in slice)
					assign := assigns[len(assigns)-1]
					// If the assignment is an alias (Value.Kind == kindIdent), recursively trace the RHS to the base variable
					if assign.Value.GetKind() == KindIdent && assign.Value.GetName() != varName {
						baseVar, basePkg, t, f := traceVariableOriginHelper(assign.Value.GetName(), funcName, pkgName, metadata, visited)
						return baseVar, basePkg, t, f
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
									OuterLoop:
										for retArg.GetKind() != KindIdent {
											switch retArg.GetKind() {
											case KindSelector:
												retArg = *retArg.Sel
											case KindUnary, KindCompositeLit:
												retArg = *retArg.X
											default:
												break OuterLoop
											}
										}
										if retArg.GetKind() == KindIdent && retArg.Name != -1 {
											_, _, t, f := traceVariableOriginHelper(retArg.GetName(), assign.CalleeFunc, assign.CalleePkg, metadata, visited)
											return retArg.GetName(), assign.CalleePkg, t, f
										}
										// For literals or other expressions, return as is
										return retArg.GetName(), assign.CalleePkg, &retArg, funcName
									}
								}

								// Looking for methods with caching
								methodKey := assign.CalleePkg + "." + assign.CalleeFunc
								var calleeMethod *Method
								var exists bool
								if metadata.methodLookupCache != nil {
									calleeMethod, exists = metadata.methodLookupCache[methodKey]
								}
								if !exists {
									for _, t := range calleeFile.Types {
										for _, method := range t.Methods {
											if metadata.StringPool.GetString(method.Name) == assign.CalleeFunc {
												calleeMethod = &method
												if metadata.methodLookupCache != nil {
													metadata.methodLookupCache[methodKey] = calleeMethod
												}
												break
											}
										}
										if calleeMethod != nil {
											break
										}
									}
									// Cache nil result to avoid repeated lookups
									if calleeMethod == nil && metadata.methodLookupCache != nil {
										metadata.methodLookupCache[methodKey] = nil
									}
								}
								if calleeMethod != nil {
									retIdx := assign.ReturnIndex
									if retIdx < len(calleeMethod.ReturnVars) {
										retArg := calleeMethod.ReturnVars[retIdx]
									OuterLoop2:
										for retArg.GetKind() != KindIdent {
											switch retArg.GetKind() {
											case KindSelector:
												retArg = *retArg.Sel
											case KindUnary, KindCompositeLit:
												retArg = *retArg.X
											default:
												break OuterLoop2
											}
										}
										if retArg.GetKind() == KindIdent && retArg.Name != -1 {
											_, _, t, f := traceVariableOriginHelper(retArg.GetName(), assign.CalleeFunc, assign.CalleePkg, metadata, visited)
											return retArg.GetName(), assign.CalleePkg, t, f
										}
										// For literals or other expressions, return as is
										return retArg.GetName(), assign.CalleePkg, &retArg, funcName
									}
								}
							}
						}
					}
					return cacheAndReturn(varName, pkgName, funcName, &assign.Value)
				}
			}
		}
	}

	// Fallback: return as is
	result := TraceVariableResult{
		OriginVar:      varName,
		OriginPkg:      pkgName,
		OriginType:     nil,
		CallerFuncName: funcName,
	}
	// Only cache results if packages are populated (to avoid caching incomplete results during metadata generation)
	if metadata.traceVariableCache != nil && len(metadata.Packages) > 0 {
		metadata.traceVariableCache[key] = result
	}
	return varName, pkgName, nil, funcName
}
