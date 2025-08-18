package metadata

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"maps"
	"sort"
	"strings"
)

// CallIdentifierType represents different types of identifiers used in the call graph
type CallIdentifierType int

const (
	// BaseID - Function/method name with package, no position or generics
	BaseID CallIdentifierType = iota
	// InstanceID - Includes position and generic type parameters for specific call instances
	InstanceID
)

// CallIdentifier manages different identifier formats for calls
type CallIdentifier struct {
	pkg      string
	name     string
	recvType string
	position string
	generics map[string]string
}

func NewCallIdentifier(pkg, name, recvType, position string, generics map[string]string) *CallIdentifier {
	return &CallIdentifier{
		pkg:      pkg,
		name:     name,
		recvType: recvType,
		position: position,
		generics: generics,
	}
}

// ID returns the identifier based on the specified type
func (ci *CallIdentifier) ID(idType CallIdentifierType) string {
	var base string

	// Build base identifier
	if ci.recvType != "" {
		if strings.HasPrefix(ci.recvType, "*") {
			base = fmt.Sprintf("%s.%s.%s", ci.pkg, ci.recvType[1:], ci.name)
		} else {
			base = fmt.Sprintf("%s.%s.%s", ci.pkg, ci.recvType, ci.name)
		}
	} else {
		base = fmt.Sprintf("%s.%s", ci.pkg, ci.name)
	}
	base = strings.TrimPrefix(base, "*")

	switch idType {
	case BaseID:
		return base
	case InstanceID:
		// Include generics and position for instance identification
		var parts []string
		parts = append(parts, base)

		if len(ci.generics) > 0 {
			var genericParts []string
			for param, concrete := range ci.generics {
				genericParts = append(genericParts, fmt.Sprintf("%s=%s", param, concrete))
			}
			sort.Slice(genericParts, func(i, j int) bool { return genericParts[i] < genericParts[j] })
			parts = append(parts, fmt.Sprintf("[%s]", strings.Join(genericParts, ",")))
		}

		if ci.position != "" {
			parts = append(parts, fmt.Sprintf("@%s", ci.position))
		}

		id := strings.Join(parts, "")
		id = strings.TrimPrefix(id, "*")

		return id
	default:
		return base
	}
}

// Helper function to strip ID to base format
func stripToBase(id string) string {
	// Remove position (@...)
	if idx := strings.Index(id, "@"); idx >= 0 {
		id = id[:idx]
	}
	// Remove generics ([...])
	if idx := strings.Index(id, "["); idx >= 0 {
		id = id[:idx]
	}
	return id
}

var assignmentCount int
var processAssignmentCount int

// GenerateMetadata extracts all metadata and call graph info
func GenerateMetadata(pkgs map[string]map[string]*ast.File, fileToInfo map[*ast.File]*types.Info, importPaths map[string]string, fset *token.FileSet) *Metadata {
	funcMap := BuildFuncMap(pkgs)

	fmt.Println("funcMap Count:", len(funcMap))

	pool := NewStringPool()
	metadata := &Metadata{
		StringPool: pool,
		Packages:   make(map[string]*Package),
		CallGraph:  make([]CallGraphEdge, 0),
	}

	for pkgName, files := range pkgs {
		pkg := &Package{
			Files: make(map[string]*File),
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
						processAssignmentCount++
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
				m.SignatureStr = pool.Get(CallArgToString(m.Signature))
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
			processStructInstances(file, info, pkgName, fset, pool, f, constMap)

			// Process imports
			processImports(file, pool, f)

			pkg.Types = allTypes
			pkg.Files[fullPath] = f
		}

		metadata.Packages[pkgName] = pkg
	}

	// Analyze interface implementations
	analyzeInterfaceImplementations(metadata.Packages, pool)

	for pkgName, files := range pkgs {
		// Build call graph
		buildCallGraph(files, pkgs, pkgName, fileToInfo, fset, funcMap, pool, metadata)
	}

	metadata.BuildCallGraphMaps()

	roots := metadata.CallGraphRoots()
	for _, edge := range roots {
		metadata.TraverseCallerChildren(edge, func(parent, child *CallGraphEdge) {
			if len(parent.TypeParamMap) > 0 && len(child.TypeParamMap) > 0 {
				newChild := *child
				newChild.TypeParamMap = map[string]string{}

				maps.Copy(newChild.TypeParamMap, child.TypeParamMap)
				// Add parent types
				maps.Copy(newChild.TypeParamMap, parent.TypeParamMap)

				// Reset id
				newChild.Caller.identifier = nil
				newChild.Caller.Edge = &newChild
				newChild.Caller.buildIdentifier()

				newChild.Callee.identifier = nil
				newChild.Callee.Edge = &newChild
				newChild.Callee.buildIdentifier()

				metadata.CallGraph = append(metadata.CallGraph, newChild)
				metadata.Callers[newChild.Caller.identifier.ID(BaseID)] = append(metadata.Callers[newChild.Caller.identifier.ID(BaseID)], &newChild)
			}
		})
	}

	// Finalize string pool
	pool.Finalize()

	fmt.Println("process assignment Count:", processAssignmentCount)
	fmt.Println("assignment Count:", assignmentCount)

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
					value := CallArgToString(ExprToCallArgument(vspec.Values[i], info, pkgName, fset))
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
				specName := getTypeName(tspec)
				t.Methods = allTypeMethods[specName]
				t.Methods = append(t.Methods, allTypeMethods["*"+specName]...)
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
			m.SignatureStr = pool.Get(CallArgToString(m.Signature))
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
		var maxReturnCount int

		if fn.Body != nil {
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				ret, ok := n.(*ast.ReturnStmt)
				if !ok {
					return true
				}

				// Track the maximum number of return values seen
				if len(ret.Results) > maxReturnCount {
					maxReturnCount = len(ret.Results)
					returnVars = nil // Clear and rebuild with the most complete return
					for _, expr := range ret.Results {
						returnVars = append(returnVars, ExprToCallArgument(expr, info, pkgName, fset))
					}
				}

				return true // Continue traversal to see all returns
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
					v.Value = pool.Get(CallArgToString(ExprToCallArgument(vspec.Values[i], info, pkgName, fset)))
				}

				f.Variables[name.Name] = v
			}
		}
	}
}

// processStructInstances processes struct literals and assignments
func processStructInstances(file *ast.File, info *types.Info, pkgName string, fset *token.FileSet, pool *StringPool, f *File, constMap map[string]string) {
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
			key := CallArgToString(ExprToCallArgument(kv.Key, info, pkgName, fset))
			val := CallArgToString(ExprToCallArgument(kv.Value, info, pkgName, fset))

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
					assignment := Assignment{
						VariableName: pool.Get(expr.Name),
						Pkg:          pool.Get(pkgName),
						ConcreteType: pool.Get(concreteType),
						Position:     pool.Get(getPosition(assign.Pos(), fset)),
						Scope:        pool.Get(getScope(expr.Name)),
						Value:        val,
						Lhs:          ExprToCallArgument(lhsExpr, info, pkgName, fset),
						Func:         pool.Get(funcName),
					}
					// If RHS is a function call, record callee info
					if callExpr, ok := rhsExpr.(*ast.CallExpr); ok {
						calleeFunc, calleePkg, _ := getCalleeFunctionNameAndPackage(callExpr.Fun, file, pkgName, fileToInfo, funcMap, fset)
						assignment.CalleeFunc = calleeFunc
						assignment.CalleePkg = calleePkg
						assignment.ReturnIndex = 0 // For now, always first return value
					}
					assignments = append(assignments, assignment)
					assignmentCount++
				}
			}
		// Handle selector assignments (obj.Field = ...)
		case *ast.SelectorExpr:
			if rhsExpr != nil {
				lhsArg := ExprToCallArgument(lhsExpr, info, pkgName, fset)
				assignments = append(assignments, Assignment{
					VariableName: pool.Get(CallArgToString(lhsArg)),
					Pkg:          pool.Get(pkgName),
					ConcreteType: pool.Get(lhsArg.Type),
					Position:     pool.Get(getPosition(assign.Pos(), fset)),
					Scope:        pool.Get("selector"),
					Value:        ExprToCallArgument(rhsExpr, info, pkgName, fset),
					Lhs:          ExprToCallArgument(lhsExpr, info, pkgName, fset),
				})
			}
		// Handle index assignments (arr[i] = ...)
		case *ast.IndexExpr, *ast.IndexListExpr:
			if rhsExpr != nil {
				assignments = append(assignments, Assignment{
					VariableName: pool.Get(CallArgToString(ExprToCallArgument(lhsExpr, info, pkgName, fset))),
					Pkg:          pool.Get(pkgName),
					ConcreteType: pool.Get("index"),
					Position:     pool.Get(getPosition(assign.Pos(), fset)),
					Scope:        pool.Get("index"),
					Value:        ExprToCallArgument(rhsExpr, info, pkgName, fset),
					Lhs:          ExprToCallArgument(lhsExpr, info, pkgName, fset),
				})
			}
		// Fallback: record any other LHS as a raw assignment
		default:
			if lhsExpr != nil && rhsExpr != nil {
				assignments = append(assignments, Assignment{
					VariableName: pool.Get(CallArgToString(ExprToCallArgument(lhsExpr, info, pkgName, fset))),
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

// processImports processes import statements
func processImports(file *ast.File, pool *StringPool, f *File) {
	for _, imp := range file.Imports {
		importPath := getImportPath(imp)
		alias := getImportAlias(imp)
		if alias == "" {
			alias = importPath
		}
		f.Imports[pool.Get(alias)] = pool.Get(importPath)
	}
}

// buildCallGraph builds the call graph for all files in a package
func buildCallGraph(files map[string]*ast.File, pkgs map[string]map[string]*ast.File, pkgName string, fileToInfo map[*ast.File]*types.Info, fset *token.FileSet, funcMap map[string]*ast.FuncDecl, pool *StringPool, metadata *Metadata) {
	for _, file := range files {
		var argMap = map[string]*CallArgument{}
		var calleeMap = map[string]*CallGraphEdge{}

		info := fileToInfo[file]

		var assignVarName string

		ast.Inspect(file, func(n ast.Node) bool {
			if n == nil {
				return true
			}

			if call, ok := n.(*ast.CallExpr); ok {
				processCallExpression(call, file, pkgs, pkgName, assignVarName, fileToInfo, funcMap, fset, pool, metadata, info, calleeMap, argMap)
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

		for argID, arg := range argMap {
			if edge, ok := calleeMap[argID]; ok {
				arg.Edge = edge
			}
		}
	}
}

// processCallExpression processes a function call expression
func processCallExpression(call *ast.CallExpr, file *ast.File, pkgs map[string]map[string]*ast.File, pkgName, assignVarName string, fileToInfo map[*ast.File]*types.Info, funcMap map[string]*ast.FuncDecl, fset *token.FileSet, pool *StringPool, metadata *Metadata, info *types.Info, calleeMap map[string]*CallGraphEdge, argMap map[string]*CallArgument) {
	callerFunc, callerParts := getEnclosingFunctionName(file, call.Pos())
	calleeFunc, calleePkg, calleeParts := getCalleeFunctionNameAndPackage(call.Fun, file, pkgName, fileToInfo, funcMap, fset)

	if callerFunc != "" && calleeFunc != "" {
		// Collect arguments
		args := make([]CallArgument, len(call.Args))
		for i, arg := range call.Args {
			args[i] = ExprToCallArgument(arg, info, pkgName, fset)
			argMap[args[i].ID()] = &args[i]
		}

		// Build parameter-to-argument mapping
		paramArgMap := make(map[string]CallArgument)
		typeParamMap := make(map[string]string)

		// Get the *types.Object for the function being called
		// This is crucial for getting the *declared* generic type parameters
		extractParamsAndTypeParams(call, info, args, paramArgMap, typeParamMap)

		// Use funcMap to get callee function declaration
		var assignmentsInFunc = make(map[string][]Assignment)

		calleeAstFile := astFileFromFn(calleePkg, calleeFunc, pkgs, metadata)

		if calleeAstFile != nil {
			fnInfo := fileToInfo[calleeAstFile]
			var funcName string

			if calleeParts == "" {
				funcName = calleePkg + "." + calleeFunc
			} else {
				funcName = calleePkg + "." + calleeParts + "." + calleeFunc
			}

			if fn, ok := funcMap[funcName]; ok {
				ast.Inspect(fn, func(nd ast.Node) bool {
					if nd == nil {
						return true
					}

					switch expr := nd.(type) {
					case *ast.AssignStmt:
						// IMPORTANT: The `file` argument in processAssignment should be the file of the *callee*,
						// not the caller. Otherwise, info.ObjectOf might return nil for objects not in the caller's file.
						// We need to find the correct `*ast.File` object for the callee's declaration.
						// This lookup is more complex than just using `pos.Filename` because `pkgs` is keyed by package path,
						// and `fileToInfo` maps `*ast.File` pointers.
						assignments := processAssignment(expr, calleeAstFile, fnInfo, calleePkg, fset, pool, fileToInfo, funcMap, metadata)
						for _, assign := range assignments {
							varName := CallArgToString(assign.Lhs)
							assignmentsInFunc[varName] = append(assignmentsInFunc[varName], assign)
						}
					}
					return true
				})
			}
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
			Position:          int(call.Pos()),
			Args:              args,
			AssignmentMap:     assignmentsInFunc,
			ParamArgMap:       paramArgMap,
			TypeParamMap:      typeParamMap,
			CalleeVarName:     calleeVarName,
			CalleeRecvVarName: assignVarName,
			meta:              metadata,
		}

		cgEdge.Caller = *cgEdge.NewCall(
			pool.Get(callerFunc),
			pool.Get(pkgName),
			-1, // No position for caller
			pool.Get(callerParts),
		)

		cgEdge.Callee = *cgEdge.NewCall(
			pool.Get(calleeFunc),
			pool.Get(calleePkg),
			pool.Get(getPosition(call.Pos(), fset)),
			pool.Get(calleeParts),
		)

		// Apply type parameter resolution
		applyTypeParameterResolution(&cgEdge)

		// Use instance ID for calleeMap indexing to avoid conflicts
		calleeInstance := cgEdge.Callee.InstanceID()
		calleeMap[calleeInstance] = &cgEdge

		metadata.CallGraph = append(metadata.CallGraph, cgEdge)
	}
}

func astFileFromFn(pkgName, fnName string, pkgs map[string]map[string]*ast.File, metadata *Metadata) *ast.File {
	var astFile *ast.File

	if pkg, pkgExists := metadata.Packages[pkgName]; pkgExists {
		for fileName, f := range pkg.Files {
			if _, ok := f.Functions[fnName]; ok {
				astFile = pkgs[pkgName][fileName]
				break
			}
		}
	}

	return astFile
}

func extractParamsAndTypeParams(call *ast.CallExpr, info *types.Info, args []CallArgument, paramArgMap map[string]CallArgument, typeParamMap map[string]string) {
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

						// Handle type inference for different call expression types
						var instance types.Instance
						var found bool

						switch fun := call.Fun.(type) {
						case *ast.Ident:
							instance, found = info.Instances[fun]
						case *ast.SelectorExpr:
							// For selector expressions like pkg.Func, try to get the instance
							instance, found = info.Instances[fun.Sel]
						case *ast.ParenExpr:
							// For parenthesized expressions like (Func), unwrap and try again
							if ident, ok := fun.X.(*ast.Ident); ok {
								instance, found = info.Instances[ident]
							}
						}

						if found && instance.TypeArgs != nil {
							for i := 0; i < sig.TypeParams().Len(); i++ {
								tparam := sig.TypeParams().At(i)
								name := tparam.Obj().Name()
								if i < instance.TypeArgs.Len() {
									inferredType := instance.TypeArgs.At(i)
									typeParamMap[name] = inferredType.String()
								}
							}
						} else {
							// Try to infer types from function arguments
							// This is crucial for cases like HandleRequest(handleSendEmail)
							// where the type parameters are inferred from the argument types
							if len(args) > 0 {
								// Look at the first argument to infer type parameters
								firstArg := args[0]
								if firstArg.Kind == KindIdent {
									// Try to get the type of the argument
									if argType := info.TypeOf(call.Args[0]); argType != nil {
										// For function arguments, try to extract parameter types
										if sig, isSig := argType.(*types.Signature); isSig {
											// Check if this is a function type that can help infer generic parameters
											if sig.Params().Len() > 0 {
												// The first parameter type of the argument function
												// should correspond to the first type parameter of the generic function
												firstParamType := sig.Params().At(0).Type()
												if sig.TypeParams().Len() > 0 {
													// This is a generic function argument
													// Try to map its type parameters to the callee's type parameters
													for i := 0; i < sig.TypeParams().Len(); i++ {
														tparam := sig.TypeParams().At(i)
														calleeTParam := sig.TypeParams().At(i)
														if i < sig.TypeParams().Len() {
															// Map the argument's type parameter to the callee's type parameter
															typeParamMap[calleeTParam.Obj().Name()] = tparam.Obj().Name()
														}
													}
												} else {
													// Non-generic function argument
													// The first parameter type should map to the first type parameter
													if sig.TypeParams().Len() > 0 {
														firstTParam := sig.TypeParams().At(0)
														typeParamMap[firstTParam.Obj().Name()] = firstParamType.String()
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

				// Handle regular parameters
				tup := sig.Params()
				for i := 0; i < tup.Len(); i++ {
					field := tup.At(i)
					if i < len(args) {
						if args[i].TypeParamMap == nil {
							args[i].TypeParamMap = map[string]string{}
						}

						// Propagate type mapping to args
						maps.Copy(args[i].TypeParamMap, typeParamMap)

						paramArgMap[field.Name()] = args[i]
					}
				}
			}
		}
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

// applyTypeParameterResolution applies ParamArgMap and TypeParamMap to CallArgument structures
// to fill them with correct resolved type information
func applyTypeParameterResolution(edge *CallGraphEdge) {
	if edge == nil {
		return
	}

	// Apply type parameter resolution to all arguments
	for i := range edge.Args {
		arg := &edge.Args[i]
		applyTypeParameterResolutionToArgument(arg, edge.ParamArgMap, arg.TypeParamMap)
	}

	// Apply type parameter resolution to ParamArgMap values
	for paramName, arg := range edge.ParamArgMap {
		resolvedArg := arg
		applyTypeParameterResolutionToArgument(&resolvedArg, edge.ParamArgMap, edge.TypeParamMap)
		edge.ParamArgMap[paramName] = resolvedArg
	}
}

// applyTypeParameterResolutionToArgument applies type parameter resolution to a single CallArgument
func applyTypeParameterResolutionToArgument(arg *CallArgument, paramArgMap map[string]CallArgument, typeParamMap map[string]string) {
	if arg == nil {
		return
	}

	// Check if this argument represents a generic type parameter
	if arg.Type != "" {
		// Check if the type is a generic type parameter (e.g., "TRequest", "TData")
		if concreteType, exists := typeParamMap[arg.Type]; exists || len(arg.TypeParamMap) > 0 {
			arg.ResolvedType = concreteType
			arg.IsGenericType = true
			arg.GenericTypeName = arg.Type
		}
	}

	// Recursively apply to nested arguments
	if arg.X != nil {
		applyTypeParameterResolutionToArgument(arg.X, paramArgMap, arg.X.TypeParamMap)
	}
	if arg.Fun != nil {
		applyTypeParameterResolutionToArgument(arg.Fun, paramArgMap, arg.Fun.TypeParamMap)
	}
	for i := range arg.Args {
		applyTypeParameterResolutionToArgument(&arg.Args[i], paramArgMap, arg.Args[i].TypeParamMap)
	}
}
