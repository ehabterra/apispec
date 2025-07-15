package metadata

import (
	"go/ast"
	"go/token"
	"go/types"
	"strings"
)

const (
	interfaceTypeName = "interface"
	blankIdentifier   = "_"
	indexScope        = "index"
	selectorScope     = "selector"
	rawScope          = "raw"
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
func getFilePkgName(pkgs map[string]map[string]*ast.File, importPath string) string {
	if pkg, ok := pkgs[importPath]; ok {
		for _, file := range pkg {
			return file.Name.Name
		}
	}
	return ""
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

// analyzeAssignmentValue analyzes the value being assigned to determine concrete types
func analyzeAssignmentValue(expr ast.Expr, pkgs map[string]map[string]*ast.File, fileToInfo map[*ast.File]*types.Info, funcMap map[string]*ast.FuncDecl, fset *token.FileSet, pkgName string) (string, string) {
	switch e := expr.(type) {
	case *ast.Ident:
		// Simple identifier - return the type name
		return pkgName, getTypeName(e)

	case *ast.SelectorExpr:
		// Package-qualified type - simplified
		return pkgName, getTypeName(e)

	case *ast.CallExpr:
		// Function call - try to determine return type
		if sel, ok := e.Fun.(*ast.SelectorExpr); ok {
			// Look for constructor patterns like NewType()
			if strings.HasPrefix(sel.Sel.Name, "New") {
				typeName := strings.TrimPrefix(sel.Sel.Name, "New")
				return pkgName, typeName
			}
		}
		return pkgName, getTypeName(e)

	default:
		return pkgName, getTypeName(e)
	}
}
