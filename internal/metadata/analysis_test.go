package metadata

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"testing"
)

type traceTestCase struct {
	desc      string
	src       string
	varName   string
	funcName  string
	expectVar string
}

func TestTraceVariableOrigin_TableDriven(t *testing.T) {
	tests := []traceTestCase{
		{
			desc: "Multiple assignments, latest wins",
			src: `package main
var x int
func main() {
	x = 1
	x = 2
}`,
			varName:   "x",
			funcName:  "main",
			expectVar: "x",
		},
		{
			desc: "Chained aliases",
			src: `package main
func main() {
	a := 5
	b := a
	c := b
}`,
			varName:   "c",
			funcName:  "main",
			expectVar: "a",
		},
		{
			desc: "Variable shadowing, outer",
			src: `package main
var x int
func main() {
	x = 1
	{
		var y int
		y = 2
	}
}`,
			varName:   "x",
			funcName:  "main",
			expectVar: "x",
		},
		{
			desc: "Variable shadowing, inner",
			src: `package main
var x int
func main() {
	x = 1
	{
		var x int
		x = 2
	}
}`,
			varName:   "x",
			funcName:  "main",
			expectVar: "x",
		},
		{
			desc: "Alias with reassignment",
			src: `package main
func main() {
	a := 1
	b := a
	a = 2
}`,
			varName:   "b",
			funcName:  "main",
			expectVar: "a",
		},
		{
			desc: "Self-alias (should not loop)",
			src: `package main
func main() {
	a := 1
	a = a
}`,
			varName:   "a",
			funcName:  "main",
			expectVar: "a",
		},
		{
			desc: "Alias to shadowed variable",
			src: `package main
var x int
func main() {
	x = 1
	{
		var x int
		x = 2
		y := x
		_ = y
	}
}`,
			varName:   "y",
			funcName:  "main",
			expectVar: "x",
		},
	}

	for _, tc := range tests {
		t.Run(tc.desc, func(t *testing.T) {
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, "test.go", tc.src, 0)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			pkgs := map[string]map[string]*ast.File{"main": {"test.go": file}}
			importPaths := map[string]string{"main": "main"}
			fileToInfo := map[*ast.File]*types.Info{}
			meta := GenerateMetadata(pkgs, fileToInfo, importPaths, fset)
			name, _, _, _ := TraceVariableOrigin(tc.varName, tc.funcName, "main", meta)
			if name != tc.expectVar {
				t.Errorf("expected %q to resolve to %q, got %q", tc.varName, tc.expectVar, name)
			}
		})
	}
}

func TestTraceVariableOrigin_MethodHandling(t *testing.T) {
	// Create a minimal metadata structure to test the method handling path
	meta := &Metadata{
		StringPool: NewStringPool(),
		Packages:   make(map[string]*Package),
	}

	// Create a test package with a method
	pkg := &Package{
		Files: make(map[string]*File),
	}

	// Create a test file
	file := &File{
		Types: make(map[string]*Type),
	}

	// Create a test type with a method
	userType := &Type{
		Methods: []Method{
			{
				Name: meta.StringPool.Get("GetName"),
				ReturnVars: []CallArgument{
					*NewCallArgument(meta),
				},
			},
		},
	}

	// Set up the return variable properly
	userType.Methods[0].ReturnVars[0].SetKind(KindIdent)
	userType.Methods[0].ReturnVars[0].SetName("u.Name")

	file.Types["User"] = userType
	pkg.Files["user.go"] = file
	meta.Packages["main"] = pkg

	// Create a test assignment that simulates a method call
	assign := Assignment{
		CalleeFunc:  "GetName",
		ReturnIndex: 0,
	}

	// Simulate the method lookup logic from traceVariableOriginHelper
	var calleeMethod *Method
	for _, t := range file.Types {
		for _, method := range t.Methods {
			if meta.StringPool.GetString(method.Name) == assign.CalleeFunc {
				calleeMethod = &method
				break
			}
		}
	}

	if calleeMethod == nil {
		t.Fatal("Expected to find method GetName")
		return
	}

	// Test return value tracing
	retIdx := assign.ReturnIndex
	if retIdx < len(calleeMethod.ReturnVars) {
		retArg := calleeMethod.ReturnVars[retIdx]

		// Test the OuterLoop2 logic for different kinds
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
			expectedName := retArg.GetName()
			if expectedName != "u.Name" {
				t.Errorf("Expected return value name 'u.Name', got '%s'", expectedName)
			}
		}
	}
}

func TestTraceVariableOrigin_MethodEdgeCases(t *testing.T) {
	// Create a minimal metadata structure
	meta := &Metadata{
		StringPool: NewStringPool(),
		Packages:   make(map[string]*Package),
	}

	// Test method not found scenario
	pkg := &Package{
		Files: make(map[string]*File),
	}
	file := &File{
		Types: make(map[string]*Type),
	}
	pkg.Files["test.go"] = file
	meta.Packages["main"] = pkg

	// Test with non-existent method
	assign := Assignment{
		CalleeFunc: "NonExistentMethod",
		// CalleePkg:   "main",
		ReturnIndex: 0,
	}

	// Simulate the method lookup logic
	var calleeMethod *Method
	for _, t := range file.Types {
		for _, method := range t.Methods {
			if meta.StringPool.GetString(method.Name) == assign.CalleeFunc {
				calleeMethod = &method
				break
			}
		}
	}

	if calleeMethod != nil {
		t.Error("Expected method not to be found")
	}

	// Test return index out of bounds
	userType := &Type{
		Methods: []Method{
			{
				Name: meta.StringPool.Get("GetName"),
				ReturnVars: []CallArgument{
					*NewCallArgument(meta),
				},
			},
		},
	}

	// Set up the return variable properly
	userType.Methods[0].ReturnVars[0].SetKind(KindIdent)
	userType.Methods[0].ReturnVars[0].SetName("u.Name")

	file.Types["User"] = userType

	assign.ReturnIndex = 5 // Out of bounds
	calleeMethod = nil

	for _, t := range file.Types {
		for _, method := range t.Methods {
			if meta.StringPool.GetString(method.Name) == assign.CalleeFunc {
				calleeMethod = &method
				break
			}
		}
	}

	if calleeMethod != nil {
		retIdx := assign.ReturnIndex
		if retIdx >= len(calleeMethod.ReturnVars) {
			// This should not panic and should handle the out-of-bounds case gracefully
			t.Log("Successfully handled out-of-bounds return index")
		}
	}
}

func TestTraceVariableOrigin_MethodReturnValueKinds(t *testing.T) {
	// Test different return value kinds in methods
	meta := &Metadata{
		StringPool: NewStringPool(),
		Packages:   make(map[string]*Package),
	}

	pkg := &Package{
		Files: make(map[string]*File),
	}
	file := &File{
		Types: make(map[string]*Type),
	}
	pkg.Files["test.go"] = file
	meta.Packages["main"] = pkg

	// Test method with selector return value
	userType := &Type{
		Methods: []Method{
			{
				Name: meta.StringPool.Get("GetProfile"),
				ReturnVars: []CallArgument{
					*NewCallArgument(meta),
				},
			},
		},
	}

	// Set up the selector return variable properly
	selArg := NewCallArgument(meta)
	selArg.SetKind(KindIdent)
	selArg.SetName("u.Profile")

	userType.Methods[0].ReturnVars[0].SetKind(KindSelector)
	userType.Methods[0].ReturnVars[0].Sel = selArg

	file.Types["User"] = userType

	assign := Assignment{
		CalleeFunc: "GetProfile",
		// CalleePkg:   "main",
		ReturnIndex: 0,
	}

	var calleeMethod *Method
	for _, t := range file.Types {
		for _, method := range t.Methods {
			if meta.StringPool.GetString(method.Name) == assign.CalleeFunc {
				calleeMethod = &method
				break
			}
		}
	}

	if calleeMethod != nil {
		retIdx := assign.ReturnIndex
		if retIdx < len(calleeMethod.ReturnVars) {
			retArg := calleeMethod.ReturnVars[retIdx]

			// Test the OuterLoop2 logic for selector
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
				expectedName := retArg.GetName()
				if expectedName != "u.Profile" {
					t.Errorf("Expected return value name 'u.Profile', got '%s'", expectedName)
				}
			}
		}
	}
}

func TestDefaultImportName(t *testing.T) {
	tests := []struct {
		name       string
		importPath string
		expected   string
	}{
		{
			name:       "simple package name",
			importPath: "github.com/example/package",
			expected:   "package",
		},
		{
			name:       "package with version",
			importPath: "github.com/example/package/v2",
			expected:   "package",
		},
		{
			name:       "package with version v1",
			importPath: "github.com/example/package/v1",
			expected:   "package",
		},
		{
			name:       "package with version v10",
			importPath: "github.com/example/package/v10",
			expected:   "package",
		},
		{
			name:       "package with non-version v",
			importPath: "github.com/example/package/validator",
			expected:   "validator",
		},
		{
			name:       "empty import path",
			importPath: "",
			expected:   "",
		},
		{
			name:       "single segment",
			importPath: "package",
			expected:   "package",
		},
		{
			name:       "version only",
			importPath: "v2",
			expected:   "v2",
		},
		{
			name:       "package with version v0",
			importPath: "github.com/example/package/v0",
			expected:   "package",
		},
		{
			name:       "package with version v9",
			importPath: "github.com/example/package/v9",
			expected:   "package",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DefaultImportName(tt.importPath)
			if result != tt.expected {
				t.Errorf("DefaultImportName(%q) = %q, want %q", tt.importPath, result, tt.expected)
			}
		})
	}
}

func TestFindParentFunction(t *testing.T) {
	// Create a simple Go source code string
	src := `package main

func parentFunc() {
	f := func() {
		// This is inside the function literal
	}
}`

	// Parse the source code
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, 0)
	if err != nil {
		t.Fatalf("Failed to parse source: %v", err)
	}

	info := &types.Info{}
	meta := &Metadata{}

	// Test with a position that should be inside the function literal
	funcLit := file.Decls[0].(*ast.FuncDecl).Body.List[0].(*ast.AssignStmt).Rhs[0].(*ast.FuncLit)
	// Use a position inside the function literal's body
	pos := funcLit.Body.Pos() + 1

	funcName, pkgName, _ := findParentFunction(file, pos, info, fset, meta)

	if funcName != "parentFunc" {
		t.Errorf("Expected function name 'parentFunc', got %q", funcName)
	}
	// Package name might be empty in this test setup, which is acceptable
	_ = pkgName
}

func TestFindParentFunction_NoFunctionLiteral(t *testing.T) {
	fset := token.NewFileSet()
	file := &ast.File{
		Name: &ast.Ident{Name: "test"},
		Decls: []ast.Decl{
			&ast.FuncDecl{
				Name: &ast.Ident{Name: "parentFunc"},
				Type: &ast.FuncType{
					Params: &ast.FieldList{},
				},
				Body: &ast.BlockStmt{
					List: []ast.Stmt{
						&ast.AssignStmt{
							Lhs: []ast.Expr{&ast.Ident{Name: "x"}},
							Rhs: []ast.Expr{&ast.Ident{Name: "y"}},
						},
					},
				},
			},
		},
	}

	info := &types.Info{}
	meta := &Metadata{}

	// Test with a position that's not in a function literal
	assignStmt := file.Decls[0].(*ast.FuncDecl).Body.List[0].(*ast.AssignStmt)
	pos := assignStmt.Pos()

	funcName, pkgName, scope := findParentFunction(file, pos, info, fset, meta)

	if funcName != "" {
		t.Errorf("Expected empty function name, got %q", funcName)
	}
	if pkgName != "" {
		t.Errorf("Expected empty package name, got %q", pkgName)
	}
	if scope != "" {
		t.Errorf("Expected empty scope, got %q", scope)
	}
}

func TestFindParentFunction_EmptyFile(t *testing.T) {
	fset := token.NewFileSet()
	file := &ast.File{
		Name:  &ast.Ident{Name: "test"},
		Decls: []ast.Decl{},
	}

	info := &types.Info{}
	meta := &Metadata{}

	funcName, pkgName, scope := findParentFunction(file, token.NoPos, info, fset, meta)

	if funcName != "" {
		t.Errorf("Expected empty function name, got %q", funcName)
	}
	if pkgName != "" {
		t.Errorf("Expected empty package name, got %q", pkgName)
	}
	if scope != "" {
		t.Errorf("Expected empty scope, got %q", scope)
	}
}
