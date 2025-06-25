package parser

import (
	"go/ast"
)

// FuncCallGraph maps function names to the names of functions they call
// e.g., {"userHandler": ["handleUser", "logRequest"]}
type FuncCallGraph map[string][]string

// BuildFuncMap builds a map of all function names to their declarations across all files
func BuildFuncMap(goFiles []*ast.File) map[string]*ast.FuncDecl {
	funcMap := make(map[string]*ast.FuncDecl)
	for _, file := range goFiles {
		for _, decl := range file.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok {
				funcMap[fn.Name.Name] = fn
			}
		}
	}
	return funcMap
}

// BuildCallGraph builds a call graph for all functions in the codebase
func BuildCallGraph(goFiles []*ast.File) FuncCallGraph {
	funcMap := BuildFuncMap(goFiles)
	graph := make(FuncCallGraph)
	for name, fn := range funcMap {
		calls := findCalledFunctions(fn)
		graph[name] = calls
	}
	return graph
}

// findCalledFunctions returns a list of function names called by the given function
func findCalledFunctions(fn *ast.FuncDecl) []string {
	var calls []string
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if ident, ok := call.Fun.(*ast.Ident); ok {
			calls = append(calls, ident.Name)
		} else if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
			// Optionally handle method calls if needed
			if id, ok := sel.X.(*ast.Ident); ok && id.Name == "" {
				calls = append(calls, sel.Sel.Name)
			}
		}
		return true
	})
	return calls
}

// Recursive extraction with cycle prevention
// The extractFn should extract request/response types from a *ast.FuncDecl
func ExtractNestedInfo(
	fnName string,
	funcMap map[string]*ast.FuncDecl,
	callGraph FuncCallGraph,
	visited map[string]struct{},
	extractFn func(*ast.FuncDecl) []interface{}, // can be specialized for request/response
) []interface{} {
	if visited == nil {
		visited = make(map[string]struct{})
	}
	if _, seen := visited[fnName]; seen {
		return nil
	}
	visited[fnName] = struct{}{}

	fn, ok := funcMap[fnName]
	if !ok {
		return nil
	}

	results := extractFn(fn)
	for _, callee := range callGraph[fnName] {
		nested := ExtractNestedInfo(callee, funcMap, callGraph, visited, extractFn)
		results = append(results, nested...)
	}
	return results
}
