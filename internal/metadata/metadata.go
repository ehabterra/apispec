package metadata

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"hash/fnv"
	"maps"
	"path/filepath"
	"slices"
	"sort"
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
	return GenerateMetadataWithLogger(pkgs, fileToInfo, importPaths, fset, nil, "")
}

// VerboseLogger is the cross-cutting logging contract for the analyzer
// pipeline. Printf/Println/Print are progress-style entries gated on a
// verbose flag in the implementation. Warnf is for conditions the consumer
// likely wants to see regardless of verbosity — limit truncations, suspect
// inputs, recoverable extraction failures.
type VerboseLogger interface {
	Printf(format string, args ...any)
	Println(args ...any)
	Print(args ...any)
	Warnf(format string, args ...any)
}

// modulePath, when non-empty, is the authoritative module path (read from
// go.mod by the caller). It's preferred over inferring the path from import
// paths, which is only a heuristic and mis-detects when third-party packages
// are analyzed alongside the project (see the inference block below).
func GenerateMetadataWithLogger(pkgs map[string]map[string]*ast.File, fileToInfo map[*ast.File]*types.Info, importPaths map[string]string, fset *token.FileSet, logger VerboseLogger, modulePath string) *Metadata {
	funcMap := BuildFuncMap(pkgs)

	if logger != nil {
		logger.Println("funcMap Count:", len(funcMap))
	}
	if logger != nil {
		logger.Printf("Processing %d packages...\n", len(pkgs))
	}

	// Determine the current module path. Prefer the authoritative value the
	// caller read from go.mod; only infer it from import paths as a fallback.
	currentModulePath := modulePath
	var packagePaths []string

	// Collect all unique package paths in stable order: importPaths is a map,
	// so ranging it directly would make the fallback module-path inference
	// below depend on iteration order.
	pathSet := make(map[string]bool)
	for _, fileName := range slices.Sorted(maps.Keys(importPaths)) {
		importPath := importPaths[fileName]
		if !pathSet[importPath] {
			pathSet[importPath] = true
			packagePaths = append(packagePaths, importPath)
		}
	}

	// Fallback: infer the module path as the longest common prefix among all
	// package paths. This is heuristic — when third-party deps are analyzed
	// too, the common prefix can collapse (e.g. to "github.com/"), which
	// misclassifies project vs library packages. Hence go.mod is preferred.
	if currentModulePath == "" && len(packagePaths) > 0 {
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

		// External-type facts discovered during the type walk.
		ExternalTypes: make(map[string]ExternalTypeFact),
	}

	// Process packages and files in sorted order: both maps' iteration order
	// would otherwise decide string-pool interning order (and therefore the
	// entire serialized metadata) per run.
	sortedPkgNames := slices.Sorted(maps.Keys(pkgs))
	for _, pkgName := range sortedPkgNames {
		files := pkgs[pkgName]
		sortedFileNames := slices.Sorted(maps.Keys(files))
		pkg := &Package{
			Files: make(map[string]*File),
		}

		// Collect methods for types
		allTypeMethods := make(map[string][]Method)
		allTypes := make(map[string]*Type)

		// First pass: collect all methods
		for _, fileName := range sortedFileNames {
			file := files[fileName]
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
		for _, fileName := range sortedFileNames {
			file := files[fileName]
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

			// Register synthetic types for inline anonymous-struct local
			// vars before any expression walk runs handleIdent — otherwise
			// the synthetic key handleIdent emits would point at nothing.
			processLocalAnonymousStructs(file, info, pkgName, fset, f, metadata)

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
	// Build the call graph in a stable package order: pkgs is a map, so ranging
	// it directly makes the CallGraph edge order (and therefore roots, traversal
	// order, and the whole generated spec) differ between runs.
	for _, pkgName := range sortedPkgNames {
		// Build call graph
		buildCallGraph(pkgs[pkgName], pkgs, pkgName, fileToInfo, fset, funcMap, metadata)
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
// Method keys carry no package prefix, so cross-package collisions are
// last-writer-wins — iterate in sorted order so the winner is stable per run.
func BuildFuncMap(pkgs map[string]map[string]*ast.File) map[string]*ast.FuncDecl {
	funcMap := make(map[string]*ast.FuncDecl)
	for _, pkgPath := range slices.Sorted(maps.Keys(pkgs)) {
		files := pkgs[pkgPath]
		for _, fileName := range slices.Sorted(maps.Keys(files)) {
			file := files[fileName]
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
			if tspec, ok := spec.(*ast.TypeSpec); ok {
				processTypeSpec(tspec, info, pkgName, fset, f, allTypeMethods, allTypes, metadata, false)
			}
		}
	}

	// Function-local named types — e.g. `type Login struct{…}` declared inside
	// a handler — are not in file.Decls (they live in function bodies), so the
	// loop above misses them. A request/response bound to such a type would
	// then resolve to a dangling $ref. Walk function bodies to capture them.
	processLocalTypes(file, info, pkgName, fset, f, allTypeMethods, allTypes, metadata)
}

// processTypeSpec records a single type declaration into the file's type table.
// When local is true the spec came from inside a function body; such a type is
// only added if its name isn't already taken by a package-level type in this
// file, so a real package type is never shadowed by a function-local one.
func processTypeSpec(tspec *ast.TypeSpec, info *types.Info, pkgName string, fset *token.FileSet, f *File, allTypeMethods map[string][]Method, allTypes map[string]*Type, metadata *Metadata, local bool) {
	// Skip mock/fake/stub types
	if isMockName(tspec.Name.Name) {
		return
	}
	if local {
		if _, exists := f.Types[tspec.Name.Name]; exists {
			return
		}
	}

	t := &Type{
		Name:  metadata.StringPool.Get(tspec.Name.Name),
		Pkg:   metadata.StringPool.Get(pkgName),
		Scope: metadata.StringPool.Get(getScope(tspec.Name.Name)),
	}

	// Extract declared type-parameter names for generic types (e.g. the "T"
	// in `type Page[T any] struct{...}`). The spec layer zips these with the
	// concrete arguments of an instantiation (Page[User]) to substitute
	// parametric fields into real schemas.
	if tspec.TypeParams != nil {
		for _, tparam := range tspec.TypeParams.List {
			for _, name := range tparam.Names {
				t.TypeParams = append(t.TypeParams, name.Name)
			}
		}
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

// processLocalTypes captures named type declarations inside function bodies.
func processLocalTypes(file *ast.File, info *types.Info, pkgName string, fset *token.FileSet, f *File, allTypeMethods map[string][]Method, allTypes map[string]*Type, metadata *Metadata) {
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			gd, ok := n.(*ast.GenDecl)
			if !ok || gd.Tok != token.TYPE {
				return true
			}
			for _, spec := range gd.Specs {
				if tspec, ok := spec.(*ast.TypeSpec); ok {
					processTypeSpec(tspec, info, pkgName, fset, f, allTypeMethods, allTypes, metadata, true)
				}
			}
			return true
		})
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

// resolveExternalNamedTypes walks a types.Type and replaces every
// *types.Named that lives in an external package with a representation we
// can render in OpenAPI. Internal project types are kept as-is so they
// still produce named component schemas.
//
// The substitution rule (a general replacement for hard-coded per-library
// mappings):
//
//  1. If the external named type implements MarshalJSON or MarshalText,
//     it has chosen its own wire format. Almost every such type in the
//     Go ecosystem emits a string (uuid.UUID, decimal.Decimal, ulid.ULID,
//     sql.Null*, civil.Date, …) so we default to `string`. Users who
//     want a more precise format can add a typeMapping entry.
//  2. Otherwise, recursively resolve the underlying type. `type Foo []Bar`
//     unwraps cleanly, `time.Duration` resolves to its int64 underlying,
//     and so on.
//
// Examples (with currentModulePath = "github.com/me/proj"):
//
//	uuid.UUID                       → string  (MarshalJSON)
//	[]uuid.UUID                     → []string
//	*uuid.UUID                      → *string
//	decimal.Decimal                 → string  (MarshalJSON)
//	sql.NullString                  → string  (MarshalJSON, since Go 1.17)
//	time.Duration                   → int64   (underlying; no MarshalJSON)
//	github.com/me/proj/types.Local  → kept    (internal)
func resolveExternalNamedTypes(t types.Type, currentModulePath string) types.Type {
	if t == nil {
		return nil
	}
	switch tt := t.(type) {
	case *types.Named:
		if tt.Obj() != nil && tt.Obj().Pkg() != nil &&
			isExternalPackage(tt.Obj().Pkg().Path(), currentModulePath) {
			if hasCustomJSONMarshaler(tt) {
				return types.Typ[types.String]
			}
			// Recurse on the underlying so e.g. `type Foo []Bar` (external)
			// composes correctly when Bar is also external.
			return resolveExternalNamedTypes(tt.Underlying(), currentModulePath)
		}
		return tt
	case *types.Pointer:
		if elem := resolveExternalNamedTypes(tt.Elem(), currentModulePath); elem != tt.Elem() {
			return types.NewPointer(elem)
		}
		return tt
	case *types.Slice:
		if elem := resolveExternalNamedTypes(tt.Elem(), currentModulePath); elem != tt.Elem() {
			return types.NewSlice(elem)
		}
		return tt
	case *types.Array:
		if elem := resolveExternalNamedTypes(tt.Elem(), currentModulePath); elem != tt.Elem() {
			return types.NewArray(elem, tt.Len())
		}
		return tt
	case *types.Map:
		key := resolveExternalNamedTypes(tt.Key(), currentModulePath)
		val := resolveExternalNamedTypes(tt.Elem(), currentModulePath)
		if key != tt.Key() || val != tt.Elem() {
			return types.NewMap(key, val)
		}
		return tt
	case *types.Chan:
		if elem := resolveExternalNamedTypes(tt.Elem(), currentModulePath); elem != tt.Elem() {
			return types.NewChan(tt.Dir(), elem)
		}
		return tt
	default:
		return tt
	}
}

// hasCustomJSONMarshaler reports whether the named type (or its pointer
// form) implements json.Marshaler or encoding.TextMarshaler. encoding/json
// calls both, so either is enough to mean "JSON output differs from the
// type's Go layout".
func hasCustomJSONMarshaler(n *types.Named) bool {
	if n == nil {
		return false
	}
	// json.Marshaler / encoding.TextMarshaler both have a single zero-arg
	// method whose name is enough to identify them when source for the
	// interface itself isn't loaded. Checking by name is robust to whether
	// or not encoding/json was pulled into the analyzed package set.
	if hasZeroArgMethod(n, "MarshalJSON") || hasZeroArgMethod(n, "MarshalText") {
		return true
	}
	ptr := types.NewPointer(n)
	return hasZeroArgMethodOnPtr(ptr, "MarshalJSON") || hasZeroArgMethodOnPtr(ptr, "MarshalText")
}

func hasZeroArgMethod(n *types.Named, name string) bool {
	for i := 0; i < n.NumMethods(); i++ {
		m := n.Method(i)
		if m.Name() != name {
			continue
		}
		sig, ok := m.Type().(*types.Signature)
		if ok && sig.Params().Len() == 0 {
			return true
		}
	}
	return false
}

func hasZeroArgMethodOnPtr(ptr *types.Pointer, name string) bool {
	ms := types.NewMethodSet(ptr)
	for i := 0; i < ms.Len(); i++ {
		m := ms.At(i)
		if m.Obj().Name() != name {
			continue
		}
		sig, ok := m.Obj().Type().(*types.Signature)
		if ok && sig.Params().Len() == 0 {
			return true
		}
	}
	return false
}

// marshalerKind classifies how a named type encodes itself to JSON.
// encoding/json calls MarshalJSON in preference to MarshalText, so a type that
// implements MarshalJSON controls its own (possibly non-string) output and is
// the weaker, low-confidence signal — it must be checked first. Only a type
// with MarshalText but no MarshalJSON is guaranteed to encode as a JSON string.
func marshalerKind(n *types.Named) MarshalerKind {
	if hasMarshalerMethod(n, "MarshalJSON") {
		return MarshalerJSON
	}
	if hasMarshalerMethod(n, "MarshalText") {
		return MarshalerText
	}
	return MarshalerNone
}

// hasMarshalerMethod reports whether the named type (via its pointer method set,
// which includes value-receiver methods) has a method `name` with the exact
// marshaler signature `func() ([]byte, error)`. Validating the full signature —
// not just a zero-arg method of the right name — avoids misclassifying an
// unrelated method that merely shares the name.
func hasMarshalerMethod(n *types.Named, name string) bool {
	ms := types.NewMethodSet(types.NewPointer(n))
	for i := 0; i < ms.Len(); i++ {
		m := ms.At(i)
		if m.Obj().Name() != name {
			continue
		}
		if sig, ok := m.Obj().Type().(*types.Signature); ok && isMarshalerSignature(sig) {
			return true
		}
	}
	return false
}

// isMarshalerSignature reports whether sig is `func() ([]byte, error)`.
func isMarshalerSignature(sig *types.Signature) bool {
	if sig.Params().Len() != 0 || sig.Results().Len() != 2 {
		return false
	}
	byteSlice := types.NewSlice(types.Typ[types.Byte])
	errType := types.Universe.Lookup("error").Type()
	return types.Identical(sig.Results().At(0).Type(), byteSlice) &&
		types.Identical(sig.Results().At(1).Type(), errType)
}

// recordExternalTypeFacts walks t and records an ExternalTypeFact for every
// external (third-party) named type it contains — directly or nested inside
// pointers, slices, arrays, maps and channels. It never mutates the type:
// resolution policy lives entirely in the spec layer. Facts are keyed by both
// the full import path (github.com/google/uuid.UUID) and the short
// pkg-qualified name (uuid.UUID) so either lookup form resolves later.
func recordExternalTypeFacts(t types.Type, meta *Metadata) {
	recordExternalTypeFactsVisited(t, meta, make(map[*types.TypeName]struct{}))
}

// recordExternalTypeFactsVisited is the cycle-guarded worker. Recursive type
// definitions like `type T []T` would otherwise recurse forever (T → []T → T …);
// each external named type is tracked by its *types.TypeName before recursing.
func recordExternalTypeFactsVisited(t types.Type, meta *Metadata, visited map[*types.TypeName]struct{}) {
	switch tt := t.(type) {
	case *types.Named:
		obj := tt.Obj()
		if obj == nil || obj.Pkg() == nil {
			return
		}
		if !isExternalPackage(obj.Pkg().Path(), meta.CurrentModulePath) {
			return // internal type: it renders as its own component
		}
		if _, seen := visited[obj]; seen {
			return // already walked this named type — break the cycle
		}
		visited[obj] = struct{}{}
		if meta.ExternalTypes == nil {
			meta.ExternalTypes = make(map[string]ExternalTypeFact)
		}
		fact := ExternalTypeFact{
			Marshaler:  marshalerKind(tt),
			Underlying: tt.Underlying().String(),
		}
		meta.ExternalTypes[obj.Pkg().Path()+"."+obj.Name()] = fact // full import path
		meta.ExternalTypes[obj.Pkg().Name()+"."+obj.Name()] = fact // short pkg.Type
		// Recurse the underlying so e.g. external `type Foo []Bar` also records
		// Bar when it too is external.
		recordExternalTypeFactsVisited(tt.Underlying(), meta, visited)
	case *types.Pointer:
		recordExternalTypeFactsVisited(tt.Elem(), meta, visited)
	case *types.Slice:
		recordExternalTypeFactsVisited(tt.Elem(), meta, visited)
	case *types.Array:
		recordExternalTypeFactsVisited(tt.Elem(), meta, visited)
	case *types.Map:
		recordExternalTypeFactsVisited(tt.Key(), meta, visited)
		recordExternalTypeFactsVisited(tt.Elem(), meta, visited)
	case *types.Chan:
		recordExternalTypeFactsVisited(tt.Elem(), meta, visited)
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

// AnonStructTypePrefix is the tag carried by every synthetic type that
// represents an inline (anonymous) struct local var. Downstream
// (mapper.canAddRefSchemaForType) uses this to keep those entries
// inlined at their use site instead of promoting them to a $ref.
const AnonStructTypePrefix = "_apispec_anonstruct_"

// AnonStructKey returns the stable synthetic key used to register and
// later look up an inline anonymous-struct type. The key is built from the
// declaring package path plus the declaration's base filename, line, and
// column, so two distinct `var x struct{...}` declarations never collide
// (base filenames are unique within a package's directory). A raw token.Pos
// offset must NOT be used here: fset base offsets depend on the order
// packages.Load registered files, which varies between runs, so offset-based
// keys made the serialized metadata nondeterministic. The key is sanitized to
// [A-Za-z0-9_] so no downstream pkg.Type-splitting path (TypeParts and
// friends) can mangle it, and so it carries no machine-specific directory.
// Sanitizing can collapse distinct inputs ("a-b" and "a_b" both become
// "a_b"), so an FNV hash of the raw pkg path and base filename is appended
// to keep colliding locations distinct.
func AnonStructKey(pos token.Pos, fset *token.FileSet, pkgPath string) string {
	if pos == token.NoPos || fset == nil {
		return ""
	}
	p := fset.Position(pos)
	if !p.IsValid() {
		return ""
	}
	base := filepath.Base(p.Filename)
	sanitize := func(s string) string {
		return strings.Map(func(r rune) rune {
			if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
				return r
			}
			return '_'
		}, s)
	}
	h := fnv.New32a()
	h.Write([]byte(pkgPath))
	h.Write([]byte{0})
	h.Write([]byte(base))
	return fmt.Sprintf("%s%s_%s_%d_%d_%08x", AnonStructTypePrefix, sanitize(pkgPath), sanitize(base), p.Line, p.Column, h.Sum32())
}

// IsAnonStructTypeName reports whether name is one of the synthetic
// keys produced by AnonStructKey.
func IsAnonStructTypeName(name string) bool {
	return strings.Contains(name, AnonStructTypePrefix)
}

// processLocalAnonymousStructs walks info.Defs for variables whose
// type is an inline *types.Struct (anonymous struct) and registers a
// proper *Type for each one in f.Types under AnonStructKey(pos). This
// lets every downstream consumer (findTypesInMetadata,
// generateStructSchema, ...) treat them like any other struct type,
// without trying to round-trip the Go-syntax form through go/parser.
func processLocalAnonymousStructs(file *ast.File, info *types.Info, pkgName string, fset *token.FileSet, f *File, metadata *Metadata) {
	if info == nil {
		return
	}
	// info.Defs is a map, so collect matches first and register them sorted
	// by their position-string key — otherwise string-pool interning order
	// flips per run. (Raw token.Pos is not a valid sort key across runs; see
	// AnonStructKey.)
	type anonVar struct {
		st  *types.Struct
		key string
	}
	var vars []anonVar
	for ident, obj := range info.Defs {
		if obj == nil || ident == nil {
			continue
		}
		v, ok := obj.(*types.Var)
		if !ok {
			continue
		}
		st, ok := v.Type().(*types.Struct)
		if !ok {
			continue
		}
		key := AnonStructKey(v.Pos(), fset, pkgName)
		if key == "" {
			continue
		}
		vars = append(vars, anonVar{st: st, key: key})
	}
	slices.SortFunc(vars, func(a, b anonVar) int { return strings.Compare(a.key, b.key) })
	for _, v := range vars {
		if _, exists := f.Types[v.key]; exists {
			continue
		}
		f.Types[v.key] = buildAnonStructType(v.st, v.key, pkgName, metadata)
	}
}

// buildAnonStructType produces a *Type for a *types.Struct by walking
// its fields directly via go/types. Nested anonymous structs land on
// Field.NestedType so generateStructSchema's existing recursion picks
// them up.
func buildAnonStructType(s *types.Struct, name, pkgName string, metadata *Metadata) *Type {
	t := &Type{
		Name: metadata.StringPool.Get(name),
		Pkg:  metadata.StringPool.Get(pkgName),
		Kind: metadata.StringPool.Get("struct"),
	}
	for i := 0; i < s.NumFields(); i++ {
		fv := s.Field(i)
		tag := s.Tag(i)
		fieldType := fv.Type()

		f := Field{
			Name: metadata.StringPool.Get(fv.Name()),
			Type: metadata.StringPool.Get(fieldType.String()),
			Tag:  metadata.StringPool.Get(tag),
		}

		// Inline-struct field: record it as a NestedType so the
		// schema generator inlines the object instead of trying to
		// resolve "struct{...}" through name lookup. Field types like
		// "[]struct{...}" or "*struct{...}" fall through to the
		// string path for now.
		if nested, ok := fieldType.(*types.Struct); ok {
			f.NestedType = buildAnonStructType(nested, fv.Name()+"_nested", pkgName, metadata)
		}

		t.Fields = append(t.Fields, f)
	}
	return t
}

func processStructFields(structType *ast.StructType, pkgName string, metadata *Metadata, t *Type, info *types.Info) {
	for _, field := range structType.Fields.List {
		fieldType := getTypeName(field.Type, info)
		tag := getFieldTag(field)
		comments := getComments(field)

		if !IsPrimitiveType(fieldType) && info != nil {
			var fieldTypeInfo types.Type
			switch ft := field.Type.(type) {
			case *ast.StarExpr:
				fieldTypeInfo = info.TypeOf(ft.X)
			default:
				fieldTypeInfo = info.TypeOf(ft)
			}
			if fieldTypeInfo != nil {
				// Record facts about every external named type referenced by
				// this field (uuid.UUID, decimal.Decimal, []uuid.UUID, ...) so
				// the spec layer can resolve them without re-running go/types.
				// We deliberately keep the external *name* in fieldType — no
				// flattening here — so that config/registry overrides in the
				// spec layer can still match the type by name. The spec layer
				// (resolveExternalType) decides the actual schema; metadata
				// only reports what go/types can see.
				recordExternalTypeFacts(fieldTypeInfo, metadata)
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
	// Stable file order within the package (files is a map) so call-graph edge
	// order is deterministic across runs.
	fileNames := make([]string, 0, len(files))
	for fileName := range files {
		fileNames = append(fileNames, fileName)
	}
	sort.Strings(fileNames)
	for _, fileName := range fileNames {
		file := files[fileName]
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

		calleeAstFile := astFileFromFn(calleePkg, calleeFunc, calleeParts, pkgs, metadata)

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

// astFileFromFn locates the *ast.File declaring fnName in pkgName. recvType
// (may be empty for plain functions) disambiguates methods: several types can
// share a method name (e.g. Name()), and the previous name-only scan over the
// unsorted Files map returned whichever match the map order surfaced last,
// making the recorded callee assignments flip between runs. Files and types
// are walked in sorted order so even the fallback (name match on a different
// receiver) is deterministic.
func astFileFromFn(pkgName, fnName, recvType string, pkgs map[string]map[string]*ast.File, metadata *Metadata) *ast.File {
	pkg, pkgExists := metadata.Packages[pkgName]
	if !pkgExists {
		return nil
	}
	recvType = strings.TrimPrefix(recvType, "*")

	// Track method and function candidates separately: for a receiver lookup
	// (recvType != "") a same-named method on another type is a far better
	// guess than a same-named top-level function, and the file returned here
	// feeds fileToInfo/processAssignment — the wrong AST/info pair silently
	// corrupts the extracted assignments.
	var funcFallback, methodFallback *ast.File
	for _, fileName := range slices.Sorted(maps.Keys(pkg.Files)) {
		f := pkg.Files[fileName]
		if _, ok := f.Functions[fnName]; ok {
			if recvType == "" {
				return pkgs[pkgName][fileName]
			}
			if funcFallback == nil {
				funcFallback = pkgs[pkgName][fileName]
			}
		}
		for _, typeName := range slices.Sorted(maps.Keys(f.Types)) {
			for _, method := range f.Types[typeName].Methods {
				if metadata.StringPool.GetString(method.Name) != fnName {
					continue
				}
				methodFile := pkgs[pkgName][metadata.StringPool.GetString(method.Filename)]
				if recvType != "" && strings.TrimPrefix(metadata.StringPool.GetString(method.Receiver), "*") == recvType {
					return methodFile
				}
				if methodFallback == nil {
					methodFallback = methodFile
				}
			}
		}
	}

	if methodFallback != nil {
		return methodFallback
	}
	return funcFallback
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

// analyzeInterfaceImplementations analyzes which structs implement which interfaces.
// All four loops range over maps, so iterate keys in sorted order — the append
// order of Implements/ImplementedBy (and pool interning) must not vary per run.
func analyzeInterfaceImplementations(pkgs map[string]*Package, pool *StringPool) {
	sigMemo := make(map[int]string) // normalized signature by string-pool index
	pkgNames := slices.Sorted(maps.Keys(pkgs))
	// Pre-sort each package's type names once — the interface scan below runs
	// per struct, so sorting inside that loop would redo the same work
	// O(structs × packages) times.
	sortedTypeNames := make(map[string][]string, len(pkgs))
	for _, pkgName := range pkgNames {
		sortedTypeNames[pkgName] = slices.Sorted(maps.Keys(pkgs[pkgName].Types))
	}
	for _, pkgName := range pkgNames {
		pkg := pkgs[pkgName]
		for _, structName := range sortedTypeNames[pkgName] {
			stct := pkg.Types[structName]
			if stct.Kind != pool.Get("struct") {
				continue
			}

			structMethods := make(map[int]int) // name -> signature string
			for _, method := range stct.Methods {
				structMethods[method.Name] = method.SignatureStr
			}

			for _, interfacePkgName := range pkgNames {
				interfacePkg := pkgs[interfacePkgName]
				for _, interfaceName := range sortedTypeNames[interfacePkgName] {
					intrf := interfacePkg.Types[interfaceName]
					if intrf.Kind != pool.Get("interface") {
						continue
					}

					if implementsInterface(structMethods, intrf, pool, sigMemo) {
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
