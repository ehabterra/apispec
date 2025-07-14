package parser

import (
	"go/ast"
	"go/types"
)

// FuncCallGraph maps function names to the names of functions they call
// e.g., {"userHandler": ["handleUser", "logRequest"]}
type FuncCallGraph map[string][]string

// BuildCallGraph builds a call graph for all functions in the codebase
func BuildCallGraph(goFiles []*ast.File, funcMap map[string]*ast.FuncDecl, fileToInfo map[*ast.File]*types.Info) FuncCallGraph {
	graph := make(FuncCallGraph)
	// Build a map from *ast.FuncDecl to *types.Info
	funcDeclToInfo := make(map[*ast.FuncDecl]*types.Info)
	for _, file := range goFiles {
		info := fileToInfo[file]
		for _, decl := range file.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok {
				funcDeclToInfo[fn] = info
			}
		}
	}
	for name, fn := range funcMap {
		calls := findCalledFunctions(fn, funcDeclToInfo[fn])
		graph[name] = calls
	}
	return graph
}

// findCalledFunctions returns a list of function names called by the given function
// Hybrid: records selector calls as Type.Method if possible, else receiver.Method, else just Method
func findCalledFunctions(fn *ast.FuncDecl, info *types.Info) []string {
	var calls []string
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		switch fun := call.Fun.(type) {
		case *ast.Ident:
			calls = append(calls, fun.Name)
		case *ast.SelectorExpr:
			method := fun.Sel.Name
			// Try to resolve receiver type
			recvType := ""
			if info != nil && info.Selections != nil {
				if selInfo, ok := info.Selections[fun]; ok {
					recvType = selInfo.Recv().String()
				}
			}
			if recvType == "" {
				// Try to get receiver as ident
				if id, ok := fun.X.(*ast.Ident); ok {
					recvType = id.Name
				}
			}
			if recvType != "" {
				calls = append(calls, recvType+"."+method)
			} else {
				calls = append(calls, method)
			}
		case *ast.IndexExpr:
			// Handle generic function calls: e.g., validator.DecodeJSON[Type]
			var callName string
			var typeArg string
			if fun.Index != nil {
				switch t := fun.Index.(type) {
				case *ast.Ident:
					typeArg = t.Name
				case *ast.SelectorExpr:
					// e.g., dtos.LoadCartRequest
					if pkg, ok := t.X.(*ast.Ident); ok {
						typeArg = pkg.Name + "." + t.Sel.Name
					} else {
						typeArg = t.Sel.Name
					}
				}
			}
			switch x := fun.X.(type) {
			case *ast.SelectorExpr:
				recv := ""
				if id, ok := x.X.(*ast.Ident); ok {
					recv = id.Name
				}
				callName = recv + "." + x.Sel.Name
			case *ast.Ident:
				callName = x.Name
			}
			if callName != "" {
				if typeArg != "" {
					calls = append(calls, callName+"["+typeArg+"]")
				} else {
					calls = append(calls, callName)
				}
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
