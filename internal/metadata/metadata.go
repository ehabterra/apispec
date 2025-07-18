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

				// Use funcMap to get callee function declaration
				var assignmentsInFunc = make(map[string][]Assignment)

				ast.Inspect(fn, func(nd ast.Node) bool {
					switch expr := nd.(type) {
					case *ast.AssignStmt:
						assignments := processAssignment(expr, file, info, pkgName, fset, pool, fileToInfo, funcMap, metadata)
						for _, assign := range assignments {
							varName := pool.GetString(assign.VariableName)
							assignmentsInFunc[varName] = append(assignmentsInFunc[varName], assign)
						}
					}
					return true
				})

				m := Method{
					Name:          pool.Get(fn.Name.Name),
					Receiver:      pool.Get(recvType),
					Signature:     ExprToCallArgument(fn.Type, info, pkgName, fset),
					Position:      pool.Get(getFuncPosition(fn, fset)),
					Scope:         pool.Get(getScope(fn.Name.Name)),
					AssignmentMap: assignmentsInFunc,
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
				Imports:         make(map[int]int),
			}

			// Collect constants for this file
			constMap := collectConstants(file, info, pkgName, fset)

			// Process types
			processTypes(file, info, pkgName, fset, pool, f, allTypeMethods, allTypes)

			// Process functions
			processFunctions(file, info, pkgName, fset, pool, f, fileToInfo, funcMap, metadata)

			// Process variables and constants
			processVariables(file, info, pkgName, fset, pool, f)

			// Process struct instances and assignments
			processStructInstancesAndAssignments(file, info, pkgName, fset, pool, f, constMap, pkgs, fileToInfo, funcMap, metadata)

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
	for pkgPath, files := range pkgs {
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
						if pkgName != "" {
							key = pkgPath + "." + fn.Name.Name
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
							methodKey := pkgPath + "." + typeName + "." + fn.Name.Name
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
		if pkgName != "" {
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
		processStructFields(ut, pool, t)
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
func processStructFields(structType *ast.StructType, pool *StringPool, t *Type) {
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
func processFunctions(file *ast.File, info *types.Info, pkgName string, fset *token.FileSet, pool *StringPool, f *File, fileToInfo map[*ast.File]*types.Info, funcMap map[string]*ast.FuncDecl, metadata *Metadata) {
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv != nil {
			continue
		}

		comments := getComments(fn)

		// Extract type parameter names for generics
		typeParams := []string{}
		if fn.Type != nil && fn.Type.TypeParams != nil {
			for _, tparam := range fn.Type.TypeParams.List {
				for _, name := range tparam.Names {
					typeParams = append(typeParams, name.Name)
				}
			}
		}

		// Extract return value origins
		var returnVars []CallArgument
		if fn.Body != nil {
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				ret, ok := n.(*ast.ReturnStmt)
				if !ok {
					return true
				}
				for _, expr := range ret.Results {
					returnVars = append(returnVars, ExprToCallArgument(expr, info, pkgName, fset))
				}
				// For now, only record the first return statement (can be extended for control flow)
				return false
			})
		}

		// Use funcMap to get callee function declaration
		var assignmentsInFunc = make(map[string][]Assignment)

		ast.Inspect(fn, func(nd ast.Node) bool {
			switch expr := nd.(type) {
			case *ast.AssignStmt:
				assignments := processAssignment(expr, file, info, pkgName, fset, pool, fileToInfo, funcMap, metadata)
				for _, assign := range assignments {
					varName := pool.GetString(assign.VariableName)
					assignmentsInFunc[varName] = append(assignmentsInFunc[varName], assign)
				}
			}
			return true
		})

		f.Functions[fn.Name.Name] = &Function{
			Name:          pool.Get(fn.Name.Name),
			Signature:     ExprToCallArgument(fn.Type, info, pkgName, fset),
			Position:      pool.Get(getFuncPosition(fn, fset)),
			Scope:         pool.Get(getScope(fn.Name.Name)),
			Comments:      pool.Get(comments),
			TypeParams:    typeParams,
			ReturnVars:    returnVars,
			AssignmentMap: assignmentsInFunc,
		}
	}
}

// processVariables processes all variable and constant declarations in a file
func processVariables(file *ast.File, info *types.Info, pkgName string, fset *token.FileSet, pool *StringPool, f *File) {
	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || (genDecl.Tok != token.VAR && genDecl.Tok != token.CONST) {
			continue
		}

		var tok string
		// genDecl is *ast.GenDecl
		switch genDecl.Tok {
		case token.CONST:
			tok = "const"
		case token.VAR:
			tok = "var"
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
					Tok:      pool.Get(tok),
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
func processStructInstancesAndAssignments(file *ast.File, info *types.Info, pkgName string, fset *token.FileSet, pool *StringPool, f *File, constMap map[string]string, pkgs map[string]map[string]*ast.File, fileToInfo map[*ast.File]*types.Info, funcMap map[string]*ast.FuncDecl, metadata *Metadata) {
	ast.Inspect(file, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.CompositeLit:
			processStructInstance(x, info, pkgName, fset, pool, f, constMap)
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
func processAssignment(assign *ast.AssignStmt, file *ast.File, info *types.Info, pkgName string, fset *token.FileSet, pool *StringPool, fileToInfo map[*ast.File]*types.Info, funcMap map[string]*ast.FuncDecl, metadata *Metadata) []Assignment {
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

		// Find the enclosing function name for this assignment
		funcName, _ := getEnclosingFunctionName(file, assign.Pos())

		// Handle identifier assignments (var = ...)
		switch expr := lhsExpr.(type) {
		case *ast.Ident:
			if expr.Name == "_" {
				// Skip blank identifier
				continue
			}
			if rhsExpr != nil {
				val := ExprToCallArgument(rhsExpr, info, pkgName, fset)
				_, concreteTypeArg := analyzeAssignmentValue(rhsExpr, info, funcName, pkgName, metadata, fset)
				concreteType := ""
				if concreteTypeArg != nil {
					concreteType = concreteTypeArg.Type
				}
				if concreteType != "" {
					assign := Assignment{
						VariableName: pool.Get(expr.Name),
						Pkg:          pool.Get(pkgName),
						ConcreteType: pool.Get(concreteType),
						Position:     pool.Get(getPosition(assign.Pos(), fset)),
						Scope:        pool.Get(getScope(expr.Name)),
						Value:        val,
					}
					// If RHS is a function call, record callee info
					if callExpr, ok := rhsExpr.(*ast.CallExpr); ok {
						calleeFunc, calleePkg, _ := getCalleeFunctionNameAndPackage(callExpr.Fun, file, pkgName, fileToInfo, funcMap, fset)
						assign.CalleeFunc = calleeFunc
						assign.CalleePkg = calleePkg
						assign.ReturnIndex = 0 // For now, always first return value
					}
					assignments = append(assignments, assign)
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

		var assignVarName string

		ast.Inspect(file, func(n ast.Node) bool {
			if call, ok := n.(*ast.CallExpr); ok {
				processCallExpression(call, file, pkgs, pkgName, assignVarName, fileToInfo, funcMap, fset, pool, metadata, info)
				assignVarName = ""
			} else if assign, ok := n.(*ast.AssignStmt); ok {
				// Find which variable this call is assigned to
				for i, rhs := range assign.Rhs {
					if _, ok := rhs.(*ast.CallExpr); ok && i < len(assign.Lhs) {
						if ident, ok := assign.Lhs[i].(*ast.Ident); ok {
							assignVarName = ident.Name
							break
						}
					}
				}
			}

			return true
		})
	}
}

// processCallExpression processes a function call expression
func processCallExpression(call *ast.CallExpr, file *ast.File, pkgs map[string]map[string]*ast.File, pkgName, assignVarName string, fileToInfo map[*ast.File]*types.Info, funcMap map[string]*ast.FuncDecl, fset *token.FileSet, pool *StringPool, metadata *Metadata, info *types.Info) {
	callerFunc, callerParts := getEnclosingFunctionName(file, call.Pos())
	calleeFunc, calleePkg, calleeParts := getCalleeFunctionNameAndPackage(call.Fun, file, pkgName, fileToInfo, funcMap, fset)

	if callerFunc != "" && calleeFunc != "" {
		// Collect arguments
		args := make([]CallArgument, len(call.Args))
		for i, arg := range call.Args {
			args[i] = ExprToCallArgument(arg, info, pkgName, fset)
		}

		// Build parameter-to-argument mapping
		paramArgMap := make(map[string]CallArgument)
		typeParamMap := make(map[string]string)

		// Get the *types.Object for the function being called
		// This is crucial for getting the *declared* generic type parameters
		var funcObj types.Object
		switch fun := call.Fun.(type) {
		case *ast.Ident:
			funcObj = info.ObjectOf(fun)
		case *ast.SelectorExpr:
			funcObj = info.ObjectOf(fun.Sel)
		case *ast.IndexExpr: // For calls like `Func[T]()`
			if ident, ok := fun.X.(*ast.Ident); ok {
				funcObj = info.ObjectOf(ident)
			} else if sel, ok := fun.X.(*ast.SelectorExpr); ok {
				funcObj = info.ObjectOf(sel.Sel)
			}
		case *ast.IndexListExpr: // For calls like `Func[T1, T2]()`
			if ident, ok := fun.X.(*ast.Ident); ok {
				funcObj = info.ObjectOf(ident)
			} else if sel, ok := fun.X.(*ast.SelectorExpr); ok {
				funcObj = info.ObjectOf(sel.Sel)
			}
		}

		if funcObj != nil {
			if fobj, isFunc := funcObj.(*types.Func); isFunc {
				if sig, isSig := fobj.Type().(*types.Signature); isSig {
					// Handle regular parameters
					tup := sig.Params()
					for i := 0; i < tup.Len(); i++ {
						field := tup.At(i)
						if i < len(args) {
							paramArgMap[field.Name()] = args[i]
						}
					}

					// Handle generic type parameters from the *declared* function signature
					if sig.TypeParams() != nil {
						// Attempt to extract explicit type arguments from the call expression syntax
						var explicitTypeArgExprs []ast.Expr
						switch fun := call.Fun.(type) {
						case *ast.IndexExpr:
							explicitTypeArgExprs = []ast.Expr{fun.Index}
						case *ast.IndexListExpr:
							explicitTypeArgExprs = fun.Indices
						case *ast.SelectorExpr:
							// For cases like pkg.Func[T] or receiver.Method[T]
							switch selX := fun.X.(type) {
							case *ast.IndexExpr:
								explicitTypeArgExprs = []ast.Expr{selX.Index}
							case *ast.IndexListExpr:
								explicitTypeArgExprs = selX.Indices
							}
						case *ast.Ident, *ast.ParenExpr: // Handle cases where type arguments are inferred
							// If it's an Ident (e.g., HandleRequest(handler)) or wrapped in Parens,
							// type arguments are inferred, not explicitly in call.Fun syntax.
							// We will use info.Instances below to get inferred types.
							explicitTypeArgExprs = nil // Ensure it's nil or empty
						default:
							explicitTypeArgExprs = nil // Default case, no explicit type arguments
						}

						// If explicit type arguments are provided, use them
						if len(explicitTypeArgExprs) > 0 {
							for i := 0; i < sig.TypeParams().Len(); i++ {
								tparam := sig.TypeParams().At(i)
								name := tparam.Obj().Name()

								if i < len(explicitTypeArgExprs) {
									typeArgExpr := explicitTypeArgExprs[i]
									if typeOfTypeArg := info.TypeOf(typeArgExpr); typeOfTypeArg != nil {
										typeParamMap[name] = typeOfTypeArg.String()
									} else {
										typeParamMap[name] = getTypeName(typeArgExpr)
									}
								}
							}
						} else {
							// No explicit type arguments in the call syntax.
							// This means type inference is happening.
							// We need to get the instantiated types from the *call expression itself*.
							if instance, ok := info.Instances[call.Fun.(*ast.Ident)]; ok {
								if instance.TypeArgs != nil {
									for i := 0; i < sig.TypeParams().Len(); i++ {
										tparam := sig.TypeParams().At(i)
										name := tparam.Obj().Name()
										if i < instance.TypeArgs.Len() {
											inferredType := instance.TypeArgs.At(i)
											typeParamMap[name] = inferredType.String()
										}
									}
								}
							}
						}
					}
				}
			}
		}

		funName := handlerName(calleePkg, calleeParts, calleeFunc)
		funcName := strings.TrimPrefix(funName, "*")

		// Use funcMap to get callee function declaration
		var assignmentsInFunc = make(map[string][]Assignment)

		if fn, ok := funcMap[funcName]; ok {
			ast.Inspect(fn, func(nd ast.Node) bool {
				switch expr := nd.(type) {
				case *ast.AssignStmt:
					// IMPORTANT: The `file` argument in processAssignment should be the file of the *callee*,
					// not the caller. Otherwise, info.ObjectOf might return nil for objects not in the caller's file.
					// We need to find the correct `*ast.File` object for the callee's declaration.
					// This lookup is more complex than just using `pos.Filename` because `pkgs` is keyed by package path,
					// and `fileToInfo` maps `*ast.File` pointers.
					var calleeAstFile *ast.File
					for _, f := range pkgs[calleePkg] {
						// Simple heuristic: if the function's position is within this file, it's the one.
						// This assumes a function declaration is entirely within one file.
						// A more robust check might involve comparing FileSet positions.
						if fset.Position(fn.Pos()).Filename == fset.Position(f.Pos()).Filename {
							calleeAstFile = f
							break
						}
					}

					if calleeAstFile != nil {
						fnInfo := fileToInfo[calleeAstFile]
						assignments := processAssignment(expr, calleeAstFile, fnInfo, calleePkg, fset, pool, fileToInfo, funcMap, metadata)
						for _, assign := range assignments {
							varName := pool.GetString(assign.VariableName)
							assignmentsInFunc[varName] = append(assignmentsInFunc[varName], assign)
						}
					}
				}
				return true
			})
		}

		// Create the call graph edge
		var calleeVarName string
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
			if ident, ok := sel.X.(*ast.Ident); ok && ident.Obj != nil {
				// This identifies the variable name of the receiver for method calls (e.g., "myStruct.Method()")
				calleeVarName = ident.Name
			}
		}
		cgEdge := CallGraphEdge{
			Caller: Call{
				Meta:     metadata,
				Name:     pool.Get(callerFunc),
				Pkg:      pool.Get(pkgName),
				RecvType: pool.Get(callerParts),
			},
			Callee: Call{
				Meta:     metadata,
				Name:     pool.Get(calleeFunc),
				Pkg:      pool.Get(calleePkg),
				RecvType: pool.Get(calleeParts),
			},
			Position:          int(call.Pos()),
			Args:              args,
			AssignmentMap:     assignmentsInFunc,
			ParamArgMap:       paramArgMap,
			TypeParamMap:      typeParamMap,
			CalleeVarName:     calleeVarName,
			CalleeRecvVarName: assignVarName,
			meta:              metadata,
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
