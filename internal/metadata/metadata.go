package metadata

import (
	"go/ast"
	"go/token"
	"go/types"
	"strings"
)

// GenerateMetadata extracts all metadata and call graph info
func GenerateMetadata(pkgs map[string]map[string]*ast.File, fileToInfo map[*ast.File]*types.Info, importPaths map[string]string, fset *token.FileSet) *Metadata {
	funcMap := BuildFuncMap(pkgs)

	pool := NewStringPool()
	metadata := &Metadata{
		StringPool: pool,
		Packages:   make(map[string]*Package),
		CallGraph:  make([]CallGraphEdge, 0),
	}

	for pkgName, files := range pkgs {
		pkg := &Package{
			ImportPath: pool.Get(importPaths[pkgName]),
			Files:      make(map[string]*File),
		}

		// Collect methods for types
		allTypeMethods := make(map[string][]Method)
		allTypes := make(map[string]*Type)

		// First pass: collect all methods
		for _, file := range files {
			info := fileToInfo[file]
			for _, decl := range file.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || fn.Recv == nil || len(fn.Recv.List) == 0 {
					continue
				}
				recvType := getTypeName(fn.Recv.List[0].Type)
				m := Method{
					Name:      pool.Get(fn.Name.Name),
					Receiver:  pool.Get(recvType),
					Signature: ExprToCallArgument(fn.Type, info, pkgName, fset),
					Position:  pool.Get(getFuncPosition(fn, fset)),
					Scope:     pool.Get(getScope(fn.Name.Name)),
				}
				m.SignatureStr = pool.Get(callArgToString(m.Signature))
				allTypeMethods[recvType] = append(allTypeMethods[recvType], m)
			}
		}

		// Second pass: process each file
		for fileName, file := range files {
			info := fileToInfo[file]
			fullPath := buildFullPath(importPaths[pkgName], fileName)

			f := &File{
				Types:           make(map[string]*Type),
				Functions:       make(map[string]*Function),
				Variables:       make(map[string]*Variable),
				StructInstances: make([]StructInstance, 0),
				Selectors:       make([]Selector, 0),
				Assignments:     make([]Assignment, 0),
				Imports:         make(map[int]int),
			}

			// Collect constants for this file
			constMap := collectConstants(file, info, pkgName, fset)

			// Process types
			processTypes(file, info, pkgName, fset, pool, f, allTypeMethods, allTypes)

			// Process functions
			processFunctions(file, info, pkgName, fset, pool, f)

			// Process variables and constants
			processVariables(file, info, pkgName, fset, pool, f, constMap)

			// Process struct instances and assignments
			processStructInstancesAndAssignments(file, info, pkgName, fset, pool, f, constMap, pkgs, fileToInfo, funcMap)

			// Process selectors
			processSelectors(file, info, pkgName, fset, pool, f)

			// Process imports
			processImports(file, pool, f)

			pkg.Types = allTypes
			pkg.Files[fullPath] = f
		}

		// Build call graph
		buildCallGraph(files, pkgs, pkgName, fileToInfo, fset, funcMap, pool, metadata)

		metadata.Packages[pkgName] = pkg
	}

	// Analyze interface implementations
	analyzeInterfaceImplementations(metadata.Packages, pool)

	// Finalize string pool
	pool.Finalize()

	return metadata
}

// BuildFuncMap creates a map of function names to their declarations.
func BuildFuncMap(pkgs map[string]map[string]*ast.File) map[string]*ast.FuncDecl {
	funcMap := make(map[string]*ast.FuncDecl)
	for _, files := range pkgs {
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
	}
	return funcMap
}

// handlerName get handler for funcMap map
//
//	pkgName is file.Name.Name
//	recvTypeName is just ident.Name for the receiver
func handlerName(pkgName, recvTypeName, fnName string) string {
	var key string

	if recvTypeName == "" {
		// Only for top-level functions, use package prefix if not main
		if pkgName != "" && pkgName != "main" {
			key = pkgName + "." + fnName
		} else {
			key = fnName
		}

	} else {
		// For methods, always use TypeName.MethodName (no package prefix)
		key = recvTypeName + "." + fnName
	}

	return key
}

// buildFullPath creates the full path for a file
func buildFullPath(importPath, fileName string) string {
	if importPath != "" {
		return importPath + "/" + fileName
	}
	return fileName
}

// collectConstants collects all constants from a file
func collectConstants(file *ast.File, info *types.Info, pkgName string, fset *token.FileSet) map[string]string {
	constMap := make(map[string]string)

	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.CONST {
			continue
		}

		for _, spec := range genDecl.Specs {
			vspec, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}

			for i, name := range vspec.Names {
				if len(vspec.Values) > i {
					value := callArgToString(ExprToCallArgument(vspec.Values[i], info, pkgName, fset))
					constMap[name.Name] = value
				}
			}
		}
	}

	return constMap
}

// processTypes processes all type declarations in a file
func processTypes(file *ast.File, info *types.Info, pkgName string, fset *token.FileSet, pool *StringPool, f *File, allTypeMethods map[string][]Method, allTypes map[string]*Type) {
	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.TYPE {
			continue
		}

		for _, spec := range genDecl.Specs {
			tspec, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}

			t := &Type{
				Name:  pool.Get(tspec.Name.Name),
				Scope: pool.Get(getScope(tspec.Name.Name)),
			}

			// Extract comments
			t.Comments = pool.Get(getComments(tspec))

			// Process type kind
			processTypeKind(tspec, info, pkgName, fset, pool, t, allTypes)

			// Add methods for non-interface types
			if t.Kind != pool.Get("interface") {
				t.Methods = allTypeMethods[tspec.Name.Name]
			}

			f.Types[tspec.Name.Name] = t
		}
	}
}

// processTypeKind determines the kind of type and processes it accordingly
func processTypeKind(tspec *ast.TypeSpec, info *types.Info, pkgName string, fset *token.FileSet, pool *StringPool, t *Type, allTypes map[string]*Type) {
	switch ut := tspec.Type.(type) {
	case *ast.StructType:
		t.Kind = pool.Get("struct")
		processStructFields(ut, info, pkgName, fset, pool, t)
		allTypes[tspec.Name.Name] = t

	case *ast.InterfaceType:
		t.Kind = pool.Get("interface")
		processInterfaceMethods(ut, info, pkgName, fset, pool, t)
		allTypes[tspec.Name.Name] = t

	case *ast.Ident:
		t.Kind = pool.Get("alias")
		t.Target = pool.Get(ut.Name)
		allTypes[tspec.Name.Name] = t

	default:
		t.Kind = pool.Get("other")
		allTypes[tspec.Name.Name] = t
	}
}

// processStructFields processes fields of a struct type
func processStructFields(structType *ast.StructType, info *types.Info, pkgName string, fset *token.FileSet, pool *StringPool, t *Type) {
	for _, field := range structType.Fields.List {
		fieldType := getTypeName(field.Type)
		tag := getFieldTag(field)
		comments := getComments(field)

		if len(field.Names) == 0 {
			// Embedded (anonymous) field
			t.Embeds = append(t.Embeds, pool.Get(fieldType))
			continue
		}

		for _, name := range field.Names {
			scope := getScope(name.Name)
			f := Field{
				Name:     pool.Get(name.Name),
				Type:     pool.Get(fieldType),
				Tag:      pool.Get(tag),
				Scope:    pool.Get(scope),
				Comments: pool.Get(comments),
			}
			t.Fields = append(t.Fields, f)
		}
	}
}

// processInterfaceMethods processes methods of an interface type
func processInterfaceMethods(interfaceType *ast.InterfaceType, info *types.Info, pkgName string, fset *token.FileSet, pool *StringPool, t *Type) {
	for _, method := range interfaceType.Methods.List {
		if len(method.Names) > 0 {
			m := Method{
				Name:      pool.Get(method.Names[0].Name),
				Signature: ExprToCallArgument(method.Type.(*ast.FuncType), info, pkgName, fset),
				Scope:     pool.Get(getScope(method.Names[0].Name)),
			}
			m.SignatureStr = pool.Get(callArgToString(m.Signature))
			m.Comments = pool.Get(getComments(method))
			t.Methods = append(t.Methods, m)
		}
	}
}

// processFunctions processes all function declarations in a file
func processFunctions(file *ast.File, info *types.Info, pkgName string, fset *token.FileSet, pool *StringPool, f *File) {
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv != nil {
			continue
		}

		comments := getComments(fn)
		f.Functions[fn.Name.Name] = &Function{
			Name:      pool.Get(fn.Name.Name),
			Signature: ExprToCallArgument(fn.Type, info, pkgName, fset),
			Position:  pool.Get(getFuncPosition(fn, fset)),
			Scope:     pool.Get(getScope(fn.Name.Name)),
			Comments:  pool.Get(comments),
		}
	}
}

// processVariables processes all variable and constant declarations in a file
func processVariables(file *ast.File, info *types.Info, pkgName string, fset *token.FileSet, pool *StringPool, f *File, constMap map[string]string) {
	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || (genDecl.Tok != token.VAR && genDecl.Tok != token.CONST) {
			continue
		}

		for _, spec := range genDecl.Specs {
			vspec, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}

			comments := getComments(vspec)
			for i, name := range vspec.Names {
				v := &Variable{
					Name:     pool.Get(name.Name),
					Type:     pool.Get(getTypeName(vspec.Type)),
					Position: pool.Get(getVarPosition(name, fset)),
					Comments: pool.Get(comments),
				}

				if len(vspec.Values) > i {
					v.Value = pool.Get(callArgToString(ExprToCallArgument(vspec.Values[i], info, pkgName, fset)))
				}

				f.Variables[name.Name] = v
			}
		}
	}
}

// processStructInstancesAndAssignments processes struct literals and assignments
func processStructInstancesAndAssignments(file *ast.File, info *types.Info, pkgName string, fset *token.FileSet, pool *StringPool, f *File, constMap map[string]string, pkgs map[string]map[string]*ast.File, fileToInfo map[*ast.File]*types.Info, funcMap map[string]*ast.FuncDecl) {
	ast.Inspect(file, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.CompositeLit:
			processStructInstance(x, info, pkgName, fset, pool, f, constMap)
		case *ast.AssignStmt:
			f.Assignments = append(f.Assignments, processAssignment(x, info, pkgName, fset, pool, pkgs, fileToInfo, funcMap)...)
		}
		return true
	})
}

// processStructInstance processes a struct literal
func processStructInstance(cl *ast.CompositeLit, info *types.Info, pkgName string, fset *token.FileSet, pool *StringPool, f *File, constMap map[string]string) {
	typeName := getTypeName(cl.Type)
	if typeName == "" {
		return
	}

	fields := map[int]int{}
	for _, elt := range cl.Elts {
		if kv, ok := elt.(*ast.KeyValueExpr); ok {
			key := callArgToString(ExprToCallArgument(kv.Key, info, pkgName, fset))
			val := callArgToString(ExprToCallArgument(kv.Value, info, pkgName, fset))

			// Use constant value if available
			if ident, ok := kv.Value.(*ast.Ident); ok {
				if cval, exists := constMap[ident.Name]; exists {
					val = cval
				}
			}

			fields[pool.Get(key)] = pool.Get(val)
		}
	}

	f.StructInstances = append(f.StructInstances, StructInstance{
		Type:     pool.Get(typeName),
		Position: pool.Get(getPosition(cl.Pos(), fset)),
		Fields:   fields,
	})
}

// processAssignment processes a variable assignment
func processAssignment(assign *ast.AssignStmt, info *types.Info, pkgName string, fset *token.FileSet, pool *StringPool, pkgs map[string]map[string]*ast.File, fileToInfo map[*ast.File]*types.Info, funcMap map[string]*ast.FuncDecl) []Assignment {
	var assignments []Assignment

	lhsLen := len(assign.Lhs)
	rhsLen := len(assign.Rhs)
	maxLen := lhsLen
	if rhsLen > maxLen {
		maxLen = rhsLen
	}
	for i := 0; i < maxLen; i++ {
		var lhsExpr ast.Expr
		var rhsExpr ast.Expr
		if i < lhsLen {
			lhsExpr = assign.Lhs[i]
		}
		if i < rhsLen {
			rhsExpr = assign.Rhs[i]
		}

		// Handle identifier assignments (var = ...)
		switch expr := lhsExpr.(type) {
		case *ast.Ident:
			if expr.Name == "_" {
				// Skip blank identifier
				continue
			}
			if rhsExpr != nil {
				_, concreteType := analyzeAssignmentValue(rhsExpr, pkgs, fileToInfo, funcMap, fset, pkgName)
				if concreteType != "" {
					assignments = append(assignments, Assignment{
						VariableName: pool.Get(expr.Name),
						Pkg:          pool.Get(pkgName),
						ConcreteType: pool.Get(concreteType),
						Position:     pool.Get(getPosition(assign.Pos(), fset)),
						Scope:        pool.Get(getScope(expr.Name)),
						Value:        ExprToCallArgument(rhsExpr, info, pkgName, fset),
					})
				}
			}
		// Handle selector assignments (obj.Field = ...)
		case *ast.SelectorExpr:
			if rhsExpr != nil {
				lhsArg := ExprToCallArgument(lhsExpr, info, pkgName, fset)
				assignments = append(assignments, Assignment{
					VariableName: pool.Get(callArgToString(lhsArg)),
					Pkg:          pool.Get(pkgName),
					ConcreteType: pool.Get(lhsArg.X.Type),
					Position:     pool.Get(getPosition(assign.Pos(), fset)),
					Scope:        pool.Get("selector"),
					Value:        ExprToCallArgument(rhsExpr, info, pkgName, fset),
				})
			}
		// Handle index assignments (arr[i] = ...)
		case *ast.IndexExpr, *ast.IndexListExpr:
			if rhsExpr != nil {
				assignments = append(assignments, Assignment{
					VariableName: pool.Get(callArgToString(ExprToCallArgument(lhsExpr, info, pkgName, fset))),
					Pkg:          pool.Get(pkgName),
					ConcreteType: pool.Get("index"),
					Position:     pool.Get(getPosition(assign.Pos(), fset)),
					Scope:        pool.Get("index"),
					Value:        ExprToCallArgument(rhsExpr, info, pkgName, fset),
				})
			}
		// Fallback: record any other LHS as a raw assignment
		default:
			if lhsExpr != nil && rhsExpr != nil {
				assignments = append(assignments, Assignment{
					VariableName: pool.Get(callArgToString(ExprToCallArgument(lhsExpr, info, pkgName, fset))),
					Pkg:          pool.Get(pkgName),
					ConcreteType: pool.Get("raw"),
					Position:     pool.Get(getPosition(assign.Pos(), fset)),
					Scope:        pool.Get("raw"),
					Value:        ExprToCallArgument(rhsExpr, info, pkgName, fset),
				})
			}
		}
	}

	return assignments
}

// processSelectors processes selector expressions
func processSelectors(file *ast.File, info *types.Info, pkgName string, fset *token.FileSet, pool *StringPool, f *File) {
	ast.Inspect(file, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.CallExpr:
			if sel, ok := x.Fun.(*ast.SelectorExpr); ok {
				f.Selectors = append(f.Selectors, Selector{
					Expr:     ExprToCallArgument(sel, info, pkgName, fset),
					Kind:     pool.Get("call"),
					Position: pool.Get(getPosition(sel.Pos(), fset)),
				})
			}
		case *ast.SelectorExpr:
			f.Selectors = append(f.Selectors, Selector{
				Expr:     ExprToCallArgument(x, info, pkgName, fset),
				Kind:     pool.Get("field"),
				Position: pool.Get(getPosition(x.Pos(), fset)),
			})
		}
		return true
	})
}

// processImports processes import statements
func processImports(file *ast.File, pool *StringPool, f *File) {
	for _, imp := range file.Imports {
		importPath := getImportPath(imp)
		alias := getImportAlias(imp)
		f.Imports[pool.Get(alias)] = pool.Get(importPath)
	}
}

// buildCallGraph builds the call graph for all files in a package
func buildCallGraph(files map[string]*ast.File, pkgs map[string]map[string]*ast.File, pkgName string, fileToInfo map[*ast.File]*types.Info, fset *token.FileSet, funcMap map[string]*ast.FuncDecl, pool *StringPool, metadata *Metadata) {
	for _, file := range files {
		info := fileToInfo[file]

		ast.Inspect(file, func(n ast.Node) bool {
			if call, ok := n.(*ast.CallExpr); ok {
				processCallExpression(call, file, pkgs, pkgName, fileToInfo, funcMap, fset, pool, metadata, info)
			}
			return true
		})
	}
}

// processCallExpression processes a function call expression
func processCallExpression(call *ast.CallExpr, file *ast.File, pkgs map[string]map[string]*ast.File, pkgName string, fileToInfo map[*ast.File]*types.Info, funcMap map[string]*ast.FuncDecl, fset *token.FileSet, pool *StringPool, metadata *Metadata, info *types.Info) {
	callerFunc, callerParts := getEnclosingFunctionName(file, call.Pos())
	calleeFunc, calleePkg, calleeParts := getCalleeFunctionNameAndPackage(call.Fun, file, pkgName, fileToInfo, funcMap, fset)

	if callerFunc != "" && calleeFunc != "" {
		// Collect arguments
		args := make([]CallArgument, len(call.Args))
		for i, arg := range call.Args {
			args[i] = ExprToCallArgument(arg, info, pkgName, fset)
		}

		// Use funcMap to get callee function declaration
		var assignmentsInFunc []Assignment
		funName := handlerName(getFilePkgName(pkgs, calleePkg), calleeParts, calleeFunc)
		funcName := strings.TrimPrefix(funName, "*")
		fn, ok := funcMap[funcName]
		if ok {
			ast.Inspect(fn, func(nd ast.Node) bool {
				switch expr := nd.(type) {
				case *ast.AssignStmt:
					pos := fset.Position(fn.Pos())
					fnInfo := fileToInfo[pkgs[calleePkg][pos.Filename]]
					assignmentsInFunc = append(assignmentsInFunc, processAssignment(expr, fnInfo, calleePkg, fset, pool, pkgs, fileToInfo, funcMap)...)
				}
				return true
			})
		}

		// Create the call graph edge
		cgEdge := CallGraphEdge{
			Caller: Call{
				meta:     metadata,
				Name:     pool.Get(callerFunc),
				Pkg:      pool.Get(pkgName),
				RecvType: pool.Get(callerParts),
			},
			Callee: Call{
				meta:     metadata,
				Name:     pool.Get(calleeFunc),
				Pkg:      pool.Get(calleePkg),
				RecvType: pool.Get(calleeParts),
			},
			Position:    int(call.Pos()),
			Args:        args,
			Assignments: assignmentsInFunc,
			meta:        metadata,
		}
		metadata.CallGraph = append(metadata.CallGraph, cgEdge)
	}
}

// analyzeInterfaceImplementations analyzes which structs implement which interfaces
func analyzeInterfaceImplementations(pkgs map[string]*Package, pool *StringPool) {
	for pkgName, pkg := range pkgs {
		for structName, stct := range pkg.Types {
			if stct.Kind != pool.Get("struct") {
				continue
			}

			structMethods := make(map[int]int) // name -> signature string
			for _, method := range stct.Methods {
				structMethods[method.Name] = method.SignatureStr
			}

			for interfacePkgName, interfacePkg := range pkgs {
				for interfaceName, intrf := range interfacePkg.Types {
					if intrf.Kind != pool.Get("interface") {
						continue
					}

					if implementsInterface(structMethods, intrf) {
						stct.Implements = append(stct.Implements, pool.Get(interfacePkgName+"."+interfaceName))
						intrf.ImplementedBy = append(intrf.ImplementedBy, pool.Get(pkgName+"."+structName))
					}
				}
			}
		}
	}
}
