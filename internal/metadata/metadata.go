package metadata

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"maps"
	"slices"
	"strings"
)

const MainFunc = "main"

// CallIdentifierType represents different types of identifiers used in the call graph
type CallIdentifierType int

const (
	// BaseID - Function/method name with package, no position or generics
	BaseID CallIdentifierType = iota
	// GenericID - Includes generic type parameters but no position
	GenericID
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

	// Performance optimization: cache for different ID types
	idCache map[CallIdentifierType]string
}

func NewCallIdentifier(pkg, name, recvType, position string, generics map[string]string) *CallIdentifier {
	return &CallIdentifier{
		pkg:      pkg,
		name:     name,
		recvType: recvType,
		position: position,
		generics: generics,
		idCache:  make(map[CallIdentifierType]string),
	}
}

// ID returns the identifier based on the specified type
func (ci *CallIdentifier) ID(idType CallIdentifierType) string {
	// Check cache first for performance optimization
	if cached, exists := ci.idCache[idType]; exists {
		return cached
	}

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

	var result string
	switch idType {
	case BaseID:
		result = base
	case GenericID:
		// Include generics but no position
		if len(ci.generics) > 0 {
			var genericParts []string
			for param, concrete := range ci.generics {
				genericParts = append(genericParts, fmt.Sprintf("%s=%s", param, concrete))
			}
			slices.Sort(genericParts)
			result = fmt.Sprintf("%s[%s]", base, strings.Join(genericParts, ","))
		} else {
			result = base
		}
	case InstanceID:
		// Include generics and position for instance identification
		var parts []string
		parts = append(parts, base)

		if len(ci.generics) > 0 {
			var genericParts []string
			for param, concrete := range ci.generics {
				genericParts = append(genericParts, fmt.Sprintf("%s=%s", param, concrete))
			}
			slices.Sort(genericParts)
			parts = append(parts, fmt.Sprintf("[%s]", strings.Join(genericParts, ",")))
		}

		if ci.position != "" {
			parts = append(parts, fmt.Sprintf("@%s", ci.position))
		}

		result = strings.Join(parts, "")
		result = strings.TrimPrefix(result, "*")
	default:
		result = base
	}

	// Cache the result for future lookups
	ci.idCache[idType] = result
	return result
}

// Helper function to strip ID to base format
func StripToBase(id string) string {
	callerID := id
	idIndex := strings.IndexAny(id, "@[")

	if idIndex >= 0 {
		callerID = id[:idIndex]
	}
	return callerID
}

var assignmentCount int
var processAssignmentCount int

// GenerateMetadata extracts all metadata and call graph info
func GenerateMetadata(pkgs map[string]map[string]*ast.File, fileToInfo map[*ast.File]*types.Info, importPaths map[string]string, fset *token.FileSet) *Metadata {
	return GenerateMetadataWithLogger(pkgs, fileToInfo, importPaths, fset, nil)
}

// VerboseLogger interface for conditional logging
type VerboseLogger interface {
	Printf(format string, args ...any)
	Println(args ...any)
	Print(args ...any)
}

func GenerateMetadataWithLogger(pkgs map[string]map[string]*ast.File, fileToInfo map[*ast.File]*types.Info, importPaths map[string]string, fset *token.FileSet, logger VerboseLogger) *Metadata {
	funcMap := BuildFuncMap(pkgs)

	if logger != nil {
		logger.Println("funcMap Count:", len(funcMap))
	}
	if logger != nil {
		logger.Printf("Processing %d packages...\n", len(pkgs))
	}

	// Determine the current module path from import paths
	var currentModulePath string
	var packagePaths []string

	// Collect all unique package paths
	pathSet := make(map[string]bool)
	for _, importPath := range importPaths {
		if !pathSet[importPath] {
			pathSet[importPath] = true
			packagePaths = append(packagePaths, importPath)
		}
	}

	// Find the longest common prefix among all package paths
	if len(packagePaths) > 0 {
		currentModulePath = packagePaths[0]
		for _, path := range packagePaths[1:] {
			// Find common prefix
			commonPrefix := ""
			minLen := min(len(path), len(currentModulePath))

			for i := range minLen {
				if currentModulePath[i] != path[i] {
					break
				}
				commonPrefix += string(currentModulePath[i])
			}
			// If we found a common prefix, use it
			if commonPrefix != "" && strings.Contains(commonPrefix, "/") {
				currentModulePath = commonPrefix
			} else {
				// If no common prefix, try to find a reasonable module path
				// Look for the shortest path that contains a domain
				if strings.Contains(path, ".") && (currentModulePath == "" || len(path) < len(currentModulePath)) {
					// Extract the module part (everything before the first internal/ or pkg/ or cmd/)
					parts := strings.Split(path, "/")
					for i, part := range parts {
						if part == "internal" || part == "pkg" || part == "cmd" {
							currentModulePath = strings.Join(parts[:i], "/")
							break
						}
					}
					if currentModulePath == "" {
						currentModulePath = path
					}
				}
			}
		}
	}

	metadata := &Metadata{
		StringPool: NewStringPool(),
		Packages:   make(map[string]*Package),
		CallGraph:  make([]CallGraphEdge, 0),

		ParentFunctions: make(map[string][]*CallGraphEdge),

		// Initialize performance optimization caches
		traceVariableCache: make(map[string]TraceVariableResult),
		methodLookupCache:  make(map[string]*Method),

		// Set the current module path
		CurrentModulePath: currentModulePath,
	}

	for pkgName, files := range pkgs {
		pkg := &Package{
			Files: make(map[string]*File),
		}

		// Collect methods for types
		allTypeMethods := make(map[string][]Method)
		allTypes := make(map[string]*Type)

		// First pass: collect all methods
		for fileName, file := range files {
			info := fileToInfo[file]
			for _, decl := range file.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || fn.Recv == nil || len(fn.Recv.List) == 0 {
					continue
				}
				recvType := getTypeName(fn.Recv.List[0].Type, info)

				// Skip mock/fake/stub methods
				if isMockName(recvType) || isMockName(fn.Name.Name) {
					continue
				}

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
								returnVars = append(returnVars, *ExprToCallArgument(expr, info, pkgName, fset, metadata))
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
						assignments := processAssignment(expr, file, info, pkgName, fset, fileToInfo, funcMap, metadata)
						processAssignmentCount++
						for _, assign := range assignments {
							varName := metadata.StringPool.GetString(assign.VariableName)
							assignmentsInFunc[varName] = append(assignmentsInFunc[varName], assign)
						}
					}
					return true
				})

				m := Method{
					Name:          metadata.StringPool.Get(fn.Name.Name),
					Receiver:      metadata.StringPool.Get(recvType),
					Signature:     *ExprToCallArgument(fn.Type, info, pkgName, fset, metadata),
					Position:      metadata.StringPool.Get(getFuncPosition(fn, fset)),
					Scope:         metadata.StringPool.Get(getScope(fn.Name.Name)),
					AssignmentMap: assignmentsInFunc,
					TypeParams:    typeParams,
					ReturnVars:    returnVars,
					Filename:      metadata.StringPool.Get(fileName),
				}
				m.SignatureStr = metadata.StringPool.Get(CallArgToString(&m.Signature))
				allTypeMethods[recvType] = append(allTypeMethods[recvType], m)
			}
		}

		// Second pass: process each file
		for fileName, file := range files {
			info := fileToInfo[file]
			fullPath := buildFullPath(importPaths[pkgName], fileName)

			// Heuristic pre-sizing to reduce reallocations on large files
			declsCount := len(file.Decls)
			importsCount := len(file.Imports)
			f := &File{
				Types:           make(map[string]*Type),
				Functions:       make(map[string]*Function, declsCount),
				Variables:       make(map[string]*Variable, declsCount),
				StructInstances: make([]StructInstance, 0, declsCount/4),
				Imports:         make(map[int]int, importsCount),
			}

			// Collect constants for this file
			constMap := collectConstants(file, info, pkgName, fset, metadata)

			// Process types
			processTypes(file, info, pkgName, fset, f, allTypeMethods, allTypes, metadata)

			// Process functions
			processFunctions(file, info, pkgName, fset, f, fileToInfo, funcMap, metadata)

			// Process variables and constants
			processVariables(file, info, pkgName, fset, f, metadata)

			// Process struct instances and assignments
			processStructInstances(file, info, pkgName, fset, f, constMap, metadata)

			// Process imports
			processImports(file, metadata, f)

			pkg.Types = allTypes
			pkg.Files[fullPath] = f
		}

		metadata.Packages[pkgName] = pkg
	}

	// Analyze interface implementations
	analyzeInterfaceImplementations(metadata.Packages, metadata.StringPool)

	if logger != nil {
		logger.Println("Building call graph...")
	}
	for pkgName, files := range pkgs {
		// Build call graph
		buildCallGraph(files, pkgs, pkgName, fileToInfo, fset, funcMap, metadata)
	}
	if logger != nil {
		logger.Printf("Call graph built with %d edges\n", len(metadata.CallGraph))
	}

	metadata.BuildCallGraphMaps()

	roots := metadata.CallGraphRoots()
	for _, edge := range roots {
		metadata.TraverseCallerChildren(edge, func(parent, child *CallGraphEdge) {
			if len(parent.TypeParamMap) > 0 && len(child.TypeParamMap) > 0 {
				missing := false
				for k := range parent.TypeParamMap {
					if _, ok := child.TypeParamMap[k]; !ok {
						missing = true
						break
					}
				}

				if missing {
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
			}
		})
	}

	// Process function return types to fill ResolvedType
	metadata.ProcessFunctionReturnTypes()

	// Finalize string pool
	metadata.StringPool.Finalize()

	if logger != nil {
		logger.Println("process assignment Count:", processAssignmentCount)
	}
	if logger != nil {
		logger.Println("assignment Count:", assignmentCount)
	}

	return metadata
}

// ClassifyArgument determines the type of an argument for enhanced processing
func (m *Metadata) ClassifyArgument(arg *CallArgument) ArgumentType {
	switch arg.GetKind() {
	case KindCall, KindFuncLit:
		return ArgTypeFunctionCall
	case KindTypeConversion:
		// Type conversions are not function calls and should be handled differently
		return ArgTypeComplex
	case KindIdent:
		if strings.HasPrefix(arg.GetType(), "func(") {
			return ArgTypeFunctionCall
		}
		return ArgTypeVariable
	case KindLiteral:
		return ArgTypeLiteral
	case KindSelector:
		return ArgTypeSelector
	case KindUnary:
		return ArgTypeUnary
	case KindBinary:
		return ArgTypeBinary
	case KindIndex:
		return ArgTypeIndex
	case KindCompositeLit:
		return ArgTypeComposite
	case KindTypeAssert:
		return ArgTypeTypeAssert
	default:
		return ArgTypeComplex
	}
}

// BuildAssignmentRelationships builds assignment relationships for all call graph edges
func (m *Metadata) BuildAssignmentRelationships() map[AssignmentKey]*AssignmentLink {
	relationships := make(map[AssignmentKey]*AssignmentLink)

	for i := range m.CallGraph {
		edge := &m.CallGraph[i]

		callerName := m.StringPool.GetString(edge.Caller.Name)
		callerPkg := m.StringPool.GetString(edge.Caller.Pkg)

		// Get root assignments
		if pkg, ok := m.Packages[callerPkg]; ok {
			for _, file := range pkg.Files {
				if fn, ok := file.Functions[callerName]; ok && callerName == MainFunc {
					for recvVarName, assigns := range fn.AssignmentMap {
						assignment := assigns[len(assigns)-1]

						if edge.CalleeRecvVarName != recvVarName {
							continue
						}

						akey := AssignmentKey{
							Name:      recvVarName,
							Pkg:       callerPkg,
							Type:      m.StringPool.GetString(assignment.ConcreteType),
							Container: callerName,
						}

						relationships[akey] = &AssignmentLink{
							AssignmentKey: akey,
							Assignment:    &assignment,
							Edge:          edge,
						}
					}
				}
			}
		}

		// Process assignments for this edge
		for recvVarName, assigns := range edge.AssignmentMap {
			assignment := assigns[len(assigns)-1] // Latest assignment

			akey := AssignmentKey{
				Name:      recvVarName,
				Pkg:       m.StringPool.GetString(assignment.Pkg),
				Type:      m.StringPool.GetString(assignment.ConcreteType),
				Container: m.StringPool.GetString(assignment.Func),
			}

			var assignmentEdge = edge

			// Get nested edges to link to the assignment
			if callers, exists := m.Callers[edge.Callee.BaseID()]; exists {
				for _, nestedEdge := range callers {
					if nestedEdge.CalleeRecvVarName == recvVarName {
						assignmentEdge = nestedEdge
						break
					}
				}
			}

			relationships[akey] = &AssignmentLink{
				AssignmentKey: akey,
				Assignment:    &assignment,
				Edge:          assignmentEdge,
			}
		}
	}

	return relationships
}

// GetAssignmentRelationships returns the cached assignment relationships
func (m *Metadata) GetAssignmentRelationships() map[AssignmentKey]*AssignmentLink {
	if m.assignmentRelationships == nil {
		m.assignmentRelationships = m.BuildAssignmentRelationships()
	}
	return m.assignmentRelationships
}

// TraverseCallGraph traverses the call graph with a visitor function
func (m *Metadata) TraverseCallGraph(startFrom string, visitor func(*CallGraphEdge, int) bool) {
	visited := make(map[string]bool)
	m.traverseCallGraphHelper(startFrom, 0, visitor, visited)
}

// traverseCallGraphHelper is the internal implementation with cycle detection
func (m *Metadata) traverseCallGraphHelper(current string, depth int, visitor func(*CallGraphEdge, int) bool, visited map[string]bool) {
	if visited[current] {
		return
	}
	visited[current] = true

	if edges, exists := m.Callers[current]; exists {
		for _, edge := range edges {
			if !visitor(edge, depth) {
				return
			}
			m.traverseCallGraphHelper(edge.Callee.BaseID(), depth+1, visitor, visited)
		}
	}
}

// GetCallDepth returns the call depth for a function
func (m *Metadata) GetCallDepth(funcID string) int {
	if depth, exists := m.callDepth[funcID]; exists {
		return depth
	}

	// Calculate depth by traversing up the call graph
	depth := 0
	current := funcID

	for {
		if callers, exists := m.Callees[current]; exists && len(callers) > 0 {
			depth++
			current = callers[0].Caller.BaseID()
		} else {
			break
		}
	}

	m.callDepth[funcID] = depth
	return depth
}

// GetFunctionsAtDepth returns all functions at a specific call depth
func (m *Metadata) GetFunctionsAtDepth(targetDepth int) []*CallGraphEdge {
	var result []*CallGraphEdge

	for funcID := range m.Callers {
		if m.GetCallDepth(funcID) == targetDepth {
			if edges, exists := m.Callers[funcID]; exists {
				result = append(result, edges...)
			}
		}
	}

	return result
}

// IsReachableFrom checks if a function is reachable from another function
func (m *Metadata) IsReachableFrom(fromFunc, toFunc string) bool {
	visited := make(map[string]bool)

	var dfs func(current string) bool
	dfs = func(current string) bool {
		if current == toFunc {
			return true
		}
		if visited[current] {
			return false
		}
		visited[current] = true

		if edges, exists := m.Callers[current]; exists {
			for _, edge := range edges {
				if dfs(edge.Callee.BaseID()) {
					return true
				}
			}
		}
		return false
	}

	return dfs(fromFunc)
}

// GetCallPath returns the call path from one function to another
func (m *Metadata) GetCallPath(fromFunc, toFunc string) []*CallGraphEdge {
	visited := make(map[string]bool)
	var path []*CallGraphEdge

	var dfs func(current string) bool
	dfs = func(current string) bool {
		if current == toFunc {
			return true
		}
		if visited[current] {
			return false
		}
		visited[current] = true

		if edges, exists := m.Callers[current]; exists {
			for _, edge := range edges {
				path = append(path, edge)
				if dfs(edge.Callee.BaseID()) {
					return true
				}
				path = path[:len(path)-1] // backtrack
			}
		}
		return false
	}

	if dfs(fromFunc) {
		return path
	}
	return nil
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
func collectConstants(file *ast.File, info *types.Info, pkgName string, fset *token.FileSet, meta *Metadata) map[string]string {
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
					value := CallArgToString(ExprToCallArgument(vspec.Values[i], info, pkgName, fset, meta))
					constMap[name.Name] = value
				}
			}
		}
	}

	return constMap
}

// isMockName checks if a name contains mock-related patterns
func isMockName(name string) bool {
	lower := strings.ToLower(name)
	return strings.Contains(lower, "mock") || strings.Contains(lower, "fake") ||
		strings.Contains(lower, "stub") || strings.HasPrefix(lower, "mock") ||
		strings.HasSuffix(lower, "mock") || strings.Contains(lower, "mocked")
}

// processTypes processes all type declarations in a file
func processTypes(file *ast.File, info *types.Info, pkgName string, fset *token.FileSet, f *File, allTypeMethods map[string][]Method, allTypes map[string]*Type, metadata *Metadata) {
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

			// Skip mock/fake/stub types
			if isMockName(tspec.Name.Name) {
				continue
			}

			t := &Type{
				Name:  metadata.StringPool.Get(tspec.Name.Name),
				Pkg:   metadata.StringPool.Get(pkgName),
				Scope: metadata.StringPool.Get(getScope(tspec.Name.Name)),
			}

			// Extract comments
			t.Comments = metadata.StringPool.Get(getComments(tspec))

			// Process type kind
			processTypeKind(tspec, info, pkgName, fset, t, allTypes, metadata)

			// Add methods for non-interface types
			if t.Kind != metadata.StringPool.Get("interface") {
				specName := getTypeName(tspec, info)
				t.Methods = allTypeMethods[specName]
				t.Methods = append(t.Methods, allTypeMethods["*"+specName]...)
			}

			f.Types[tspec.Name.Name] = t
		}
	}
}

// processTypeKind determines the kind of type and processes it accordingly
func processTypeKind(tspec *ast.TypeSpec, info *types.Info, pkgName string, fset *token.FileSet, t *Type, allTypes map[string]*Type, metadata *Metadata) {
	switch ut := tspec.Type.(type) {
	case *ast.StructType:
		t.Kind = metadata.StringPool.Get("struct")
		processStructFields(ut, pkgName, metadata, t, info)
		allTypes[tspec.Name.Name] = t

	case *ast.InterfaceType:
		t.Kind = metadata.StringPool.Get("interface")
		processInterfaceMethods(ut, info, pkgName, fset, t, metadata)
		allTypes[tspec.Name.Name] = t

	case *ast.Ident:
		t.Kind = metadata.StringPool.Get("alias")
		t.Target = metadata.StringPool.Get(ut.Name)
		allTypes[tspec.Name.Name] = t

	default:
		t.Kind = metadata.StringPool.Get("other")
		allTypes[tspec.Name.Name] = t
	}
}

// IsPrimitiveType checks if a type is a Go primitive type
func IsPrimitiveType(typeName string) bool {
	// Remove pointer prefix for checking
	baseType := strings.TrimPrefix(typeName, "*")

	primitiveTypes := []string{
		"string", "int", "int8", "int16", "int32", "int64",
		"uint", "uint8", "uint16", "uint32", "uint64",
		"float32", "float64", "bool", "byte", "rune",
		"error", "interface{}", "struct{}", "any",
		"complex64", "complex128", "time.Time", "nil",
	}

	if slices.Contains(primitiveTypes, baseType) {
		return true
	}

	// Check for slice of primitives
	if after, ok := strings.CutPrefix(baseType, "[]"); ok {
		elementType := after
		if slices.Contains(primitiveTypes, elementType) {
			return true
		}
	}

	// Check for array of primitives (e.g., [5]int, [N]string, [...]bool)
	if strings.HasPrefix(baseType, "[") {
		// Find the closing bracket
		endIdx := strings.Index(baseType, "]")
		if endIdx > 1 {
			elementType := baseType[endIdx+1:]
			if slices.Contains(primitiveTypes, elementType) {
				return true
			}
		}
	}

	// Check for map with primitive key/value
	if strings.HasPrefix(baseType, "map[") {
		endIdx := strings.Index(baseType, "]")
		if endIdx > 4 {
			keyType := baseType[4:endIdx]
			valueType := strings.TrimSpace(baseType[endIdx+1:])

			// If both key and value are primitives, consider it primitive
			keyIsPrimitive := false
			valueIsPrimitive := false

			for _, primitive := range primitiveTypes {
				if keyType == primitive {
					keyIsPrimitive = true
				}
				if valueType == primitive {
					valueIsPrimitive = true
				}
			}

			if keyIsPrimitive && valueIsPrimitive {
				return true
			}
		}
	}

	return false
}

// isExternalType checks if a type is from an external package (not part of the current project)
func isExternalType(typeInfo types.Type, currentModulePath string) bool {
	switch t := typeInfo.(type) {
	case *types.Named:
		// For named types, check if the package is external
		if t.Obj() != nil && t.Obj().Pkg() != nil {
			pkgPath := t.Obj().Pkg().Path()
			return isExternalPackage(pkgPath, currentModulePath)
		}
		return false
	case *types.Pointer:
		// For pointer types, check the underlying type
		return isExternalType(t.Elem(), currentModulePath)
	case *types.Slice:
		// For slice types, check the element type
		return isExternalType(t.Elem(), currentModulePath)
	case *types.Array:
		// For array types, check the element type
		return isExternalType(t.Elem(), currentModulePath)
	case *types.Map:
		// For map types, check both key and element types
		return isExternalType(t.Key(), currentModulePath) || isExternalType(t.Elem(), currentModulePath)
	case *types.Chan:
		// For channel types, check the element type
		return isExternalType(t.Elem(), currentModulePath)
	default:
		// For primitive types and other types, they're not external
		return false
	}
}

// isExternalPackage checks if a package path represents an external package
func isExternalPackage(pkgPath, currentModulePath string) bool {
	// Standard library packages are not external for our purposes
	// (they don't need to be resolved since they're already primitive)
	if !strings.Contains(pkgPath, "/") && !strings.Contains(pkgPath, ".") {
		return false
	}

	// If the package path starts with the current module path, it's internal
	if strings.HasPrefix(pkgPath, currentModulePath) {
		return false
	}

	// Otherwise, assume it's external to the project
	return true
}

// processStructFields processes fields of a struct type
func processStructFields(structType *ast.StructType, pkgName string, metadata *Metadata, t *Type, info *types.Info) {
	for _, field := range structType.Fields.List {
		fieldType := getTypeName(field.Type, info)
		tag := getFieldTag(field)
		comments := getComments(field)

		if !IsPrimitiveType(fieldType) && info != nil {
			fieldTypeInfo := info.TypeOf(field.Type)
			if fieldTypeInfo != nil {
				// Only resolve external types to their underlying primitives
				// Internal project types should remain as-is since they'll be resolved from the project
				if isExternalType(fieldTypeInfo, metadata.CurrentModulePath) {
					underlyingFieldType := fieldTypeInfo.Underlying().String()
					if IsPrimitiveType(underlyingFieldType) {
						fieldType = underlyingFieldType
					}
				}
			}
		}

		if len(field.Names) == 0 {
			// Embedded (anonymous) field
			t.Embeds = append(t.Embeds, metadata.StringPool.Get(fieldType))
			continue
		}

		for _, name := range field.Names {
			scope := getScope(name.Name)
			f := Field{
				Name:     metadata.StringPool.Get(name.Name),
				Type:     metadata.StringPool.Get(fieldType),
				Tag:      metadata.StringPool.Get(tag),
				Scope:    metadata.StringPool.Get(scope),
				Comments: metadata.StringPool.Get(comments),
			}

			// Check if this field has a nested struct type
			if structTypeExpr, ok := field.Type.(*ast.StructType); ok {
				// Create a nested type for this field
				nestedType := &Type{
					Name:     metadata.StringPool.Get(name.Name + "_nested"),
					Pkg:      metadata.StringPool.Get(pkgName),
					Kind:     metadata.StringPool.Get("struct"),
					Scope:    metadata.StringPool.Get(getScope(name.Name)),
					Comments: metadata.StringPool.Get(comments),
				}
				processStructFields(structTypeExpr, pkgName, metadata, nestedType, info)
				f.NestedType = nestedType
			}

			t.Fields = append(t.Fields, f)
		}
	}
}

// processInterfaceMethods processes methods of an interface type
func processInterfaceMethods(interfaceType *ast.InterfaceType, info *types.Info, pkgName string, fset *token.FileSet, t *Type, metadata *Metadata) {
	for _, method := range interfaceType.Methods.List {
		if len(method.Names) > 0 {
			m := Method{
				Name:      metadata.StringPool.Get(method.Names[0].Name),
				Signature: *ExprToCallArgument(method.Type.(*ast.FuncType), info, pkgName, fset, metadata),
				Scope:     metadata.StringPool.Get(getScope(method.Names[0].Name)),
			}
			m.SignatureStr = metadata.StringPool.Get(CallArgToString(&m.Signature))
			m.Comments = metadata.StringPool.Get(getComments(method))
			t.Methods = append(t.Methods, m)
		}
	}
}

// processFunctions processes all function declarations in a file
func processFunctions(file *ast.File, info *types.Info, pkgName string, fset *token.FileSet, f *File, fileToInfo map[*ast.File]*types.Info, funcMap map[string]*ast.FuncDecl, metadata *Metadata) {
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv != nil {
			continue
		}

		// Skip mock/fake/stub functions
		if isMockName(fn.Name.Name) {
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
						returnVars = append(returnVars, *ExprToCallArgument(expr, info, pkgName, fset, metadata))
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
				assignments := processAssignment(expr, file, info, pkgName, fset, fileToInfo, funcMap, metadata)
				for _, assign := range assignments {
					varName := metadata.StringPool.GetString(assign.VariableName)
					assignmentsInFunc[varName] = append(assignmentsInFunc[varName], assign)
				}
			}
			return true
		})

		f.Functions[fn.Name.Name] = &Function{
			Name:          metadata.StringPool.Get(fn.Name.Name),
			Pkg:           metadata.StringPool.Get(pkgName),
			Signature:     *ExprToCallArgument(fn.Type, info, pkgName, fset, metadata),
			Position:      metadata.StringPool.Get(getFuncPosition(fn, fset)),
			Scope:         metadata.StringPool.Get(getScope(fn.Name.Name)),
			Comments:      metadata.StringPool.Get(comments),
			TypeParams:    typeParams,
			ReturnVars:    returnVars,
			AssignmentMap: assignmentsInFunc,
		}

		f.Functions[fn.Name.Name].SignatureStr = metadata.StringPool.Get(CallArgToString(&f.Functions[fn.Name.Name].Signature))
	}
}

// processVariables processes all variable and constant declarations in a file
func processVariables(file *ast.File, info *types.Info, pkgName string, fset *token.FileSet, f *File, metadata *Metadata) {
	groupIndex := 0

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
			groupIndex++ // Each const declaration group gets a unique index
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
					Name:       metadata.StringPool.Get(name.Name),
					Pkg:        metadata.StringPool.Get(pkgName),
					Tok:        metadata.StringPool.Get(tok),
					Type:       metadata.StringPool.Get(getTypeName(vspec.Type, info)),
					Position:   metadata.StringPool.Get(getVarPosition(name, fset)),
					Comments:   metadata.StringPool.Get(comments),
					GroupIndex: groupIndex,
				}

				// Enhanced constant processing with types.Info
				if tok == "const" && info != nil {
					if obj := info.ObjectOf(name); obj != nil {
						if c, ok := obj.(*types.Const); ok {
							// Get the actual computed value from types.Info
							if c.Val() != nil {
								v.ComputedValue = c.Val()
							}
							// Get the underlying type
							if c.Type() != nil && c.Type().Underlying() != nil {
								v.ResolvedType = metadata.StringPool.Get(c.Type().Underlying().String())
							}
						}
					}
				}

				if len(vspec.Values) > i {
					v.Value = metadata.StringPool.Get(CallArgToString(ExprToCallArgument(vspec.Values[i], info, pkgName, fset, metadata)))
				}

				f.Variables[name.Name] = v
			}
		}
	}
}

// processStructInstances processes struct literals and assignments
func processStructInstances(file *ast.File, info *types.Info, pkgName string, fset *token.FileSet, f *File, constMap map[string]string, metadata *Metadata) {
	ast.Inspect(file, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.CompositeLit:
			processStructInstance(x, info, pkgName, fset, f, constMap, metadata)
		}
		return true
	})
}

// processStructInstance processes a struct literal
func processStructInstance(cl *ast.CompositeLit, info *types.Info, pkgName string, fset *token.FileSet, f *File, constMap map[string]string, metadata *Metadata) {
	typeName := getTypeName(cl.Type, info)
	if typeName == "" {
		return
	}

	fields := map[int]int{}
	for _, elt := range cl.Elts {
		if kv, ok := elt.(*ast.KeyValueExpr); ok {
			key := CallArgToString(ExprToCallArgument(kv.Key, info, pkgName, fset, metadata))
			val := CallArgToString(ExprToCallArgument(kv.Value, info, pkgName, fset, metadata))

			// Use constant value if available
			if ident, ok := kv.Value.(*ast.Ident); ok {
				if cval, exists := constMap[ident.Name]; exists {
					val = cval
				}
			}

			// Check if this might be an interface assignment
			// Look for patterns like: InterfaceName: &concreteImpl{...}
			if isEmbeddedInterfaceAssignment(kv, info) {
				registerEmbeddedInterfaceResolution(kv, typeName, pkgName, metadata, info, fset)
			}

			fields[metadata.StringPool.Get(key)] = metadata.StringPool.Get(val)
		}
	}

	f.StructInstances = append(f.StructInstances, StructInstance{
		Type:     metadata.StringPool.Get(typeName),
		Pkg:      metadata.StringPool.Get(pkgName),
		Position: metadata.StringPool.Get(getPosition(cl.Pos(), fset)),
		Fields:   fields,
	})
}

// processAssignment processes a variable assignment
func processAssignment(assign *ast.AssignStmt, file *ast.File, info *types.Info, pkgName string, fset *token.FileSet, fileToInfo map[*ast.File]*types.Info, funcMap map[string]*ast.FuncDecl, metadata *Metadata) []Assignment {
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
		funcName, _, _ := getEnclosingFunctionName(file, assign.Pos(), info, fset, metadata)

		// Handle identifier assignments (var = ...)
		switch expr := lhsExpr.(type) {
		case *ast.Ident:
			if expr.Name == "_" {
				// Skip blank identifier
				continue
			}
			if rhsExpr != nil {
				val := *ExprToCallArgument(rhsExpr, info, pkgName, fset, metadata)
				_, concreteTypeArg := analyzeAssignmentValue(rhsExpr, info, funcName, pkgName, metadata, fset)
				concreteType := ""
				if concreteTypeArg != nil {
					concreteType = concreteTypeArg.GetType()
				}

				if funcName == "" {
					continue
				}

				// if concreteType != "" {
				assignment := Assignment{
					VariableName: metadata.StringPool.Get(expr.Name),
					Pkg:          metadata.StringPool.Get(pkgName),
					ConcreteType: metadata.StringPool.Get(concreteType),
					Position:     metadata.StringPool.Get(getPosition(assign.Pos(), fset)),
					Scope:        metadata.StringPool.Get(getScope(expr.Name)),
					Value:        val,
					Lhs:          *ExprToCallArgument(lhsExpr, info, pkgName, fset, metadata),
					Func:         metadata.StringPool.Get(funcName),
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
			// }
		// Handle selector assignments (obj.Field = ...)
		case *ast.SelectorExpr:
			if rhsExpr != nil {
				lhsArg := *ExprToCallArgument(lhsExpr, info, pkgName, fset, metadata)
				assignments = append(assignments, Assignment{
					VariableName: metadata.StringPool.Get(CallArgToString(&lhsArg)),
					Pkg:          metadata.StringPool.Get(pkgName),
					ConcreteType: lhsArg.Type,
					Position:     metadata.StringPool.Get(getPosition(assign.Pos(), fset)),
					Scope:        metadata.StringPool.Get("selector"),
					Value:        *ExprToCallArgument(rhsExpr, info, pkgName, fset, metadata),
					Lhs:          *ExprToCallArgument(lhsExpr, info, pkgName, fset, metadata),
				})
			}
		// Handle index assignments (arr[i] = ...)
		case *ast.IndexExpr, *ast.IndexListExpr:
			if rhsExpr != nil {
				assignments = append(assignments, Assignment{
					VariableName: metadata.StringPool.Get(CallArgToString(ExprToCallArgument(lhsExpr, info, pkgName, fset, metadata))),
					Pkg:          metadata.StringPool.Get(pkgName),
					ConcreteType: metadata.StringPool.Get("index"),
					Position:     metadata.StringPool.Get(getPosition(assign.Pos(), fset)),
					Scope:        metadata.StringPool.Get("index"),
					Value:        *ExprToCallArgument(rhsExpr, info, pkgName, fset, metadata),
					Lhs:          *ExprToCallArgument(lhsExpr, info, pkgName, fset, metadata),
				})
			}
		// Fallback: record any other LHS as a raw assignment
		default:
			if lhsExpr != nil && rhsExpr != nil {
				assignments = append(assignments, Assignment{
					VariableName: metadata.StringPool.Get(CallArgToString(ExprToCallArgument(lhsExpr, info, pkgName, fset, metadata))),
					Pkg:          metadata.StringPool.Get(pkgName),
					ConcreteType: metadata.StringPool.Get("raw"),
					Position:     metadata.StringPool.Get(getPosition(assign.Pos(), fset)),
					Scope:        metadata.StringPool.Get("raw"),
					Value:        *ExprToCallArgument(rhsExpr, info, pkgName, fset, metadata),
					Lhs:          *ExprToCallArgument(lhsExpr, info, pkgName, fset, metadata),
				})
			}
		}
	}

	return assignments
}

// processImports processes import statements
func processImports(file *ast.File, metadata *Metadata, f *File) {
	for _, imp := range file.Imports {
		importPath := getImportPath(imp)
		alias := getImportAlias(imp)
		if alias == "" {
			alias = importPath
		}
		f.Imports[metadata.StringPool.Get(alias)] = metadata.StringPool.Get(importPath)
	}
}

// buildCallGraph builds the call graph for all files in a package
func buildCallGraph(files map[string]*ast.File, pkgs map[string]map[string]*ast.File, pkgName string, fileToInfo map[*ast.File]*types.Info, fset *token.FileSet, funcMap map[string]*ast.FuncDecl, metadata *Metadata) {
	for _, file := range files {
		var argMap = map[string]*CallArgument{}
		var calleeMap = map[string]*CallGraphEdge{}

		info := fileToInfo[file]

		var assignStmt *ast.AssignStmt

		ast.Inspect(file, func(n ast.Node) bool {
			if n == nil {
				return true
			}

			if call, ok := n.(*ast.CallExpr); ok {
				processCallExpression(call, file, pkgs, pkgName, assignStmt, fileToInfo, funcMap, fset, metadata, info, calleeMap, argMap)
				assignStmt = nil
			} else if assign, ok := n.(*ast.AssignStmt); ok {
				// Find which variable this call is assigned to
				assignStmt = assign
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

func getTypeWithGenerics(expr ast.Expr, info *types.Info) types.Type {
	var (
		instance types.Object
		found    bool
	)

	if indexExpr, ok := expr.(*ast.IndexExpr); ok {
		return getTypeWithGenerics(indexExpr.X, info)
	}

	// First try to get instance information for generics
	switch fun := expr.(type) {
	case *ast.Ident:
		instance, found = info.Uses[fun]
	case *ast.SelectorExpr:
		instance, found = info.Uses[fun.Sel]
	case *ast.ParenExpr:
		if ident, ok := fun.X.(*ast.Ident); ok {
			instance, found = info.Uses[ident]
		}
	}
	if found {
		typ := instance.Type()
		if basicTyp, ok := typ.(*types.Basic); !ok || basicTyp.Kind() != types.Invalid {
			return typ
		}
	}

	// Fallback to TypeOf for non-generic types
	if typ := info.TypeOf(expr); typ != nil {
		return typ
	}

	return nil
}

// processCallExpression processes a function call expression
func processCallExpression(call *ast.CallExpr, file *ast.File, pkgs map[string]map[string]*ast.File, pkgName string, parentAssign *ast.AssignStmt, fileToInfo map[*ast.File]*types.Info, funcMap map[string]*ast.FuncDecl, fset *token.FileSet, metadata *Metadata, info *types.Info, calleeMap map[string]*CallGraphEdge, argMap map[string]*CallArgument) {
	// Skip type conversions as they are not function calls
	if isTypeConversion(call, info) {
		return
	}

	callerFunc, callerParts, callerSignatureStr := getEnclosingFunctionName(file, call.Pos(), info, fset, metadata)
	calleeFunc, calleePkg, calleeParts := getCalleeFunctionNameAndPackage(call.Fun, file, pkgName, fileToInfo, funcMap, fset)

	// Skip mock calls
	if isMockName(calleeFunc) || isMockName(calleePkg) || isMockName(callerFunc) {
		return
	}

	var calleeSignatureStr string
	calleeType := getTypeWithGenerics(call.Fun, info)
	if calleeType != nil {
		calleeSignatureStr = calleeType.String()
	}

	if callerFunc != "" && calleeFunc != "" {
		// Determine if the caller is a function literal
		var parentFunction *Call

		if strings.HasPrefix(callerFunc, "FuncLit:") {
			callerInstance := pkgName + "." + callerFunc + "@" + callerFunc[strings.Index(callerFunc, ":")+1:]

			if _, ok := calleeMap[callerInstance]; !ok {
				// For function literals, we need to find the parent function
				// that contains this function literal
				parentFunc, parentParts, signatureStr := findParentFunction(file, call.Pos(), info, fset, metadata)
				if parentFunc != "" {
					parentScope := getScope(parentFunc)
					parentFunction = &Call{
						Meta:         metadata,
						Name:         metadata.StringPool.Get(parentFunc),
						Pkg:          metadata.StringPool.Get(pkgName),
						Position:     -1, // No position for parent function
						RecvType:     metadata.StringPool.Get(parentParts),
						Scope:        metadata.StringPool.Get(parentScope),
						SignatureStr: metadata.StringPool.Get(signatureStr),
					}
				}
			}
		}

		// Collect arguments
		args := make([]*CallArgument, len(call.Args))
		for i, arg := range call.Args {
			args[i] = ExprToCallArgument(arg, info, pkgName, fset, metadata)
			argMap[args[i].ID()] = args[i]
		}

		// Build parameter-to-argument mapping
		paramArgMap := make(map[string]CallArgument)
		typeParamMap := make(map[string]string)

		// Get the *types.Object for the function being called
		// This is crucial for getting the *declared* generic type parameters
		extractParamsAndTypeParams(call, info, args, paramArgMap, typeParamMap)

		cgEdge := &CallGraphEdge{
			Args:           args,
			Position:       metadata.StringPool.Get(getPosition(call.Pos(), fset)),
			ParamArgMap:    paramArgMap,
			TypeParamMap:   typeParamMap,
			ParentFunction: parentFunction,
			meta:           metadata,
		}

		if parentFunction != nil {
			metadata.ParentFunctions[parentFunction.ID()] = append(metadata.ParentFunctions[parentFunction.ID()], cgEdge)
		}

		// Determine scope for caller
		callerScope := getScope(callerFunc)

		cgEdge.Caller = *cgEdge.NewCall(
			metadata.StringPool.Get(callerFunc),
			metadata.StringPool.Get(pkgName),
			-1, // No position for caller
			metadata.StringPool.Get(callerParts),
			metadata.StringPool.Get(callerScope),
		)
		cgEdge.Caller.SignatureStr = metadata.StringPool.Get(callerSignatureStr)

		// Determine scope for callee
		calleeScope := getScope(calleeFunc)

		cgEdge.Callee = *cgEdge.NewCall(
			metadata.StringPool.Get(calleeFunc),
			metadata.StringPool.Get(calleePkg),
			metadata.StringPool.Get(getPosition(call.Pos(), fset)),
			metadata.StringPool.Get(calleeParts),
			metadata.StringPool.Get(calleeScope),
		)
		cgEdge.Callee.SignatureStr = metadata.StringPool.Get(calleeSignatureStr)

		// Use instance ID for calleeMap indexing to avoid conflicts
		calleeInstance := cgEdge.Callee.InstanceID()
		if _, ok := calleeMap[calleeInstance]; ok {
			return
		}

		// Use funcMap to get callee function declaration
		var assignmentsInFunc = make(map[string][]Assignment)

		calleeAstFile := astFileFromFn(calleePkg, calleeFunc, pkgs, metadata)

		if calleeAstFile != nil {
			fnInfo := fileToInfo[calleeAstFile]
			var funcName string

			if calleeParts == "" {
				funcName = calleePkg + "." + calleeFunc
			} else {
				calleeParts = strings.TrimPrefix(calleeParts, "*")

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
						assignments := processAssignment(expr, calleeAstFile, fnInfo, calleePkg, fset, fileToInfo, funcMap, metadata)
						for _, assign := range assignments {
							varName := CallArgToString(&assign.Lhs)
							assignmentsInFunc[varName] = append(assignmentsInFunc[varName], assign)
						}
					}
					return true
				})
			}
		}

		var assignVarName string
		// If this call's result is assigned to a variable in the caller, record that mapping as an assignment entry
		if parentAssign != nil {
			assignments := processAssignment(parentAssign, file, info, pkgName, fset, fileToInfo, funcMap, metadata)
			for _, assign := range assignments {
				varName := CallArgToString(&assign.Lhs)
				assignVarName = varName
				if callerFunc == MainFunc {
					assignmentsInFunc[varName] = append(assignmentsInFunc[varName], assign)
				}
			}
		}

		// Create the call graph edge
		var calleeVarName string
		var chainParent *CallGraphEdge
		var chainRoot string
		var chainDepth int

		if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
			if ident, ok := sel.X.(*ast.Ident); ok && ident.Obj != nil {
				// Simple method call on a variable (e.g., "app.Method()")
				calleeVarName = ident.Name
				chainRoot = ident.Name
				chainDepth = 0
			} else if chainCall, ok := sel.X.(*ast.CallExpr); ok {
				// Chained method call (e.g., "app.Group().Use()")
				// Find the parent call in our current callees
				processCallExpression(chainCall, file, pkgs, pkgName, parentAssign, fileToInfo, funcMap, fset, metadata, info, calleeMap, argMap)
				chainParent = &metadata.CallGraph[len(metadata.CallGraph)-1]
				chainRoot = chainParent.CalleeVarName
				chainDepth = chainParent.ChainDepth + 1

				// Fallback: try to extract root variable from the chain
				if chainRoot == "" {
					if rootVar := extractRootVariable(sel.X); rootVar != "" {
						calleeVarName = rootVar
						chainRoot = rootVar
						chainDepth = 1
					}
				}
			}
		}

		cgEdge.AssignmentMap = assignmentsInFunc
		cgEdge.CalleeVarName = calleeVarName
		cgEdge.CalleeRecvVarName = assignVarName
		cgEdge.ChainParent = chainParent
		cgEdge.ChainRoot = chainRoot
		cgEdge.ChainDepth = chainDepth

		// Apply type parameter resolution
		applyTypeParameterResolution(cgEdge)

		calleeMap[calleeInstance] = cgEdge

		metadata.CallGraph = append(metadata.CallGraph, *cgEdge)
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
			for _, t := range f.Types {
				for _, method := range t.Methods {
					methodName := metadata.StringPool.GetString(method.Name)
					if methodName == fnName {
						astFile = pkgs[pkgName][metadata.StringPool.GetString(method.Filename)]
						break
					}
				}
			}

		}
	}

	return astFile
}

// extractRootVariable recursively extracts the root variable from a chained expression
func extractRootVariable(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.SelectorExpr:
		return extractRootVariable(e.X)
	case *ast.CallExpr:
		if sel, ok := e.Fun.(*ast.SelectorExpr); ok {
			return extractRootVariable(sel.X)
		}
	}
	return ""
}

func extractParamsAndTypeParams(call *ast.CallExpr, info *types.Info, args []*CallArgument, paramArgMap map[string]CallArgument, typeParamMap map[string]string) {
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
									typeParamMap[name] = getTypeName(typeArgExpr, info)
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
								if firstArg.GetKind() == KindIdent {
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

						paramArgMap[field.Name()] = *args[i]
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
		arg := edge.Args[i]
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
	if arg.Type != -1 {
		// Check if the type is a generic type parameter (e.g., "TRequest", "TData")
		if concreteType, exists := typeParamMap[arg.GetType()]; exists || len(arg.TypeParamMap) > 0 {
			arg.ResolvedType = arg.Meta.StringPool.Get(concreteType)
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
		applyTypeParameterResolutionToArgument(arg.Args[i], paramArgMap, arg.Args[i].TypeParamMap)
	}
}

// isEmbeddedInterfaceAssignment checks if a key-value pair represents an interface assignment
func isEmbeddedInterfaceAssignment(kv *ast.KeyValueExpr, info *types.Info) bool {
	// Check if the value is a concrete type assignment (like &concreteType{...})
	switch val := kv.Value.(type) {
	case *ast.UnaryExpr:
		// Handle &concreteType{...} patterns
		if val.Op == token.AND {
			if compositeLit, ok := val.X.(*ast.CompositeLit); ok {
				concreteType := getTypeName(compositeLit.Type, info)
				if concreteType != "" && !strings.Contains(concreteType, ".") {
					// This looks like an interface assignment
					return true
				}
			}
		}
	case *ast.CompositeLit:
		// Handle direct struct literals
		concreteType := getTypeName(val.Type, info)
		if concreteType != "" && !strings.Contains(concreteType, ".") {
			return true
		}
	}

	return false
}

// registerEmbeddedInterfaceResolution registers an interface-to-concrete type mapping
func registerEmbeddedInterfaceResolution(kv *ast.KeyValueExpr, structTypeName, pkgName string, metadata *Metadata, info *types.Info, fset *token.FileSet) {
	// Get the interface field name
	fieldName := ""
	if ident, ok := kv.Key.(*ast.Ident); ok {
		fieldName = ident.Name
	} else {
		return
	}

	// Get the concrete type
	concreteType := ""
	switch val := kv.Value.(type) {
	case *ast.UnaryExpr:
		// Handle &concreteType{...} patterns
		if val.Op == token.AND {
			if compositeLit, ok := val.X.(*ast.CompositeLit); ok {
				concreteType = "*" + getTypeName(compositeLit.Type, info)
			}
		}
	case *ast.CompositeLit:
		// Handle direct struct literals
		concreteType = getTypeName(val.Type, info)
	}

	if concreteType != "" && fieldName != "" {
		position := getPosition(kv.Pos(), fset)
		metadata.RegisterInterfaceResolution(fieldName, structTypeName, pkgName, concreteType, position)
	}
}
