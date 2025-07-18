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
			name, _, _ := TraceVariableOrigin(tc.varName, tc.funcName, "main", meta)
			if name != tc.expectVar {
				t.Errorf("expected %q to resolve to %q, got %q", tc.varName, tc.expectVar, name)
			}
		})
	}
}
