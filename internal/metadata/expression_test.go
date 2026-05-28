package metadata

import (
	"go/ast"
	"go/token"
	"go/types"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestHandleCallExpr_CapturesReturnType pins the fix for the
// err.Error() / generic-call-as-body-arg bug: handleCallExpr must set
// the CallArgument's Type field to the call's *return* type (from
// types.Info.TypeOf(callExpr)) so that downstream body-type derivation
// can use the actual returned type instead of stringifying the call
// (which produced unresolvable names like "error.Error").
func TestHandleCallExpr_CapturesReturnType(t *testing.T) {
	fset := token.NewFileSet()
	meta := &Metadata{StringPool: NewStringPool()}

	// Build an info whose Types map answers `info.TypeOf(callExpr)`
	// with a concrete return type, the way the type checker would.
	stringType := types.Typ[types.String]
	callExpr := &ast.CallExpr{
		Fun: &ast.SelectorExpr{
			X:   &ast.Ident{Name: "err"},
			Sel: &ast.Ident{Name: "Error"},
		},
	}
	info := &types.Info{
		Types: map[ast.Expr]types.TypeAndValue{
			callExpr: {Type: stringType},
		},
	}

	arg := handleCallExpr(callExpr, info, "pkg", fset, meta)
	assert.NotNil(t, arg)
	assert.Equal(t, KindCall, arg.GetKind())
	assert.Equal(t, "string", arg.GetType(),
		"call argument must carry the call's return type")
}

// TestHandleCallExpr_SkipsTupleReturn confirms that handleCallExpr
// does NOT collapse a multi-return call (types.Tuple) to a single
// type string — using just one element would silently drop info.
// The arg.Type stays empty and downstream falls back to the
// stringified form.
func TestHandleCallExpr_SkipsTupleReturn(t *testing.T) {
	fset := token.NewFileSet()
	meta := &Metadata{StringPool: NewStringPool()}

	callExpr := &ast.CallExpr{
		Fun: &ast.Ident{Name: "split"},
	}
	tuple := types.NewTuple(
		types.NewVar(token.NoPos, nil, "", types.Typ[types.String]),
		types.NewVar(token.NoPos, nil, "", types.Universe.Lookup("error").Type()),
	)
	info := &types.Info{
		Types: map[ast.Expr]types.TypeAndValue{
			callExpr: {Type: tuple},
		},
	}

	arg := handleCallExpr(callExpr, info, "pkg", fset, meta)
	assert.NotNil(t, arg)
	assert.Equal(t, KindCall, arg.GetKind())
	assert.Empty(t, arg.GetType(),
		"tuple-returning calls must not collapse to a single Type")
}

func TestHandleIndexListExpr(t *testing.T) {
	fset := token.NewFileSet()
	info := &types.Info{}
	meta := &Metadata{}

	// Create an index list expression: x[1, 2, 3]
	indexListExpr := &ast.IndexListExpr{
		X: &ast.Ident{Name: "x"},
		Indices: []ast.Expr{
			&ast.BasicLit{Kind: token.INT, Value: "1"},
			&ast.BasicLit{Kind: token.INT, Value: "2"},
			&ast.BasicLit{Kind: token.INT, Value: "3"},
		},
	}

	result := handleIndexListExpr(indexListExpr, info, "", fset, meta)
	assert.NotNil(t, result)
}

func TestHandleParenExpr(t *testing.T) {
	fset := token.NewFileSet()
	info := &types.Info{}
	meta := &Metadata{}

	// Create a parenthesized expression: (x + y)
	parenExpr := &ast.ParenExpr{
		X: &ast.BinaryExpr{
			X:  &ast.Ident{Name: "x"},
			Op: token.ADD,
			Y:  &ast.Ident{Name: "y"},
		},
	}

	result := handleParenExpr(parenExpr, info, "", fset, meta)
	assert.NotNil(t, result)
}

func TestHandleSliceExpr(t *testing.T) {
	fset := token.NewFileSet()
	info := &types.Info{}
	meta := &Metadata{}

	// Create a slice expression: x[1:3]
	sliceExpr := &ast.SliceExpr{
		X:    &ast.Ident{Name: "x"},
		Low:  &ast.BasicLit{Kind: token.INT, Value: "1"},
		High: &ast.BasicLit{Kind: token.INT, Value: "3"},
	}

	result := handleSliceExpr(sliceExpr, info, "", fset, meta)
	assert.NotNil(t, result)
}

func TestHandleTypeAssertExpr(t *testing.T) {
	fset := token.NewFileSet()
	info := &types.Info{}
	meta := &Metadata{}

	// Create a type assertion: x.(string)
	typeAssertExpr := &ast.TypeAssertExpr{
		X:    &ast.Ident{Name: "x"},
		Type: &ast.Ident{Name: "string"},
	}

	result := handleTypeAssertExpr(typeAssertExpr, info, "", fset, meta)
	assert.NotNil(t, result)
}

func TestHandleChanType(t *testing.T) {
	fset := token.NewFileSet()
	info := &types.Info{}
	meta := &Metadata{}

	// Create a channel type: chan int
	chanType := &ast.ChanType{
		Dir:   ast.SEND | ast.RECV,
		Value: &ast.Ident{Name: "int"},
	}

	result := handleChanType(chanType, info, "", fset, meta)
	assert.NotNil(t, result)
}

func TestHandleStructType(t *testing.T) {
	fset := token.NewFileSet()
	info := &types.Info{}
	meta := &Metadata{}

	// Create a struct type
	structType := &ast.StructType{
		Fields: &ast.FieldList{
			List: []*ast.Field{
				{
					Names: []*ast.Ident{{Name: "Name"}},
					Type:  &ast.Ident{Name: "string"},
				},
				{
					Names: []*ast.Ident{{Name: "Age"}},
					Type:  &ast.Ident{Name: "int"},
				},
			},
		},
	}

	result := handleStructType(structType, info, "", fset, meta)
	assert.NotNil(t, result)
}

func TestHandleInterfaceType(t *testing.T) {
	meta := &Metadata{}

	// Create an interface type
	interfaceType := &ast.InterfaceType{
		Methods: &ast.FieldList{
			List: []*ast.Field{
				{
					Names: []*ast.Ident{{Name: "String"}},
					Type: &ast.FuncType{
						Params: &ast.FieldList{},
						Results: &ast.FieldList{
							List: []*ast.Field{
								{Type: &ast.Ident{Name: "string"}},
							},
						},
					},
				},
			},
		},
	}

	result := handleInterfaceType(interfaceType, meta)
	assert.NotNil(t, result)
}

func TestHandleEllipsis(t *testing.T) {
	fset := token.NewFileSet()
	info := &types.Info{}
	meta := &Metadata{}

	// Create an ellipsis: ...
	ellipsis := &ast.Ellipsis{
		Elt: &ast.Ident{Name: "int"},
	}

	result := handleEllipsis(ellipsis, info, "", fset, meta)
	assert.NotNil(t, result)
}

func TestExprToString(t *testing.T) {

	// Test with a simple identifier
	ident := &ast.Ident{Name: "testVar"}
	result := ExprToString(ident)
	assert.Contains(t, result, "testVar") // The function returns the full AST representation

	// Test with nil
	result = ExprToString(nil)
	assert.Equal(t, "", result)
}
