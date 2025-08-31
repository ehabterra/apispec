package metadata

import (
	"fmt"
	"strings"

	"go/ast"
	"go/token"
	"go/types"
)

const (
	valueFuncLit = "func()"

	defaultSep = "."
)

// ExprToCallArgument returns a structured CallArgument for an expression.
func ExprToCallArgument(expr ast.Expr, info *types.Info, pkgName string, fset *token.FileSet, meta *Metadata) *CallArgument {
	if expr == nil {
		arg := NewCallArgument(meta)
		arg.SetKind(KindRaw)
		arg.SetRaw("")
		return arg
	}

	switch e := expr.(type) {
	case *ast.Ident:
		return handleIdent(e, info, pkgName, fset, meta)
	case *ast.BasicLit:
		arg := NewCallArgument(meta)
		arg.SetKind(KindLiteral)
		arg.SetValue(e.Value)
		return arg
	case *ast.SelectorExpr:
		return handleSelector(e, info, pkgName, fset, meta)
	case *ast.CallExpr:
		return handleCallExpr(e, info, pkgName, fset, meta)
	case *ast.UnaryExpr:
		return handleUnaryExpr(e, info, pkgName, fset, meta)
	case *ast.BinaryExpr:
		return handleBinaryExpr(e, info, pkgName, fset, meta)
	case *ast.IndexExpr:
		return handleIndexExpr(e, info, pkgName, fset, meta)
	case *ast.IndexListExpr:
		return handleIndexListExpr(e, info, pkgName, fset, meta)
	case *ast.ParenExpr:
		return handleParenExpr(e, info, pkgName, fset, meta)
	case *ast.StarExpr:
		return handleStarExpr(e, info, pkgName, fset, meta)
	case *ast.ArrayType:
		return handleArrayType(e, info, pkgName, fset, meta)
	case *ast.SliceExpr:
		return handleSliceExpr(e, info, pkgName, fset, meta)
	case *ast.CompositeLit:
		return handleCompositeLit(e, info, pkgName, fset, meta)
	case *ast.KeyValueExpr:
		return handleKeyValueExpr(e, info, pkgName, fset, meta)
	case *ast.TypeAssertExpr:
		return handleTypeAssertExpr(e, info, pkgName, fset, meta)
	case *ast.FuncLit:
		arg := NewCallArgument(meta)
		arg.SetKind(KindFuncLit)
		arg.SetValue(valueFuncLit)
		return arg
	case *ast.ChanType:
		return handleChanType(e, info, pkgName, fset, meta)
	case *ast.MapType:
		return handleMapType(e, info, pkgName, fset, meta)
	case *ast.StructType:
		return handleStructType(e, info, pkgName, fset, meta)
	case *ast.InterfaceType:
		return handleInterfaceType(e, meta)
	case *ast.Ellipsis:
		return handleEllipsis(e, info, pkgName, fset, meta)
	case *ast.FuncType:
		return handleFuncType(e, info, pkgName, fset, meta)
	}

	// Fallback for other complex expressions
	arg := NewCallArgument(meta)
	arg.SetKind(KindRaw)
	arg.SetRaw(ExprToString(expr))
	return arg
}

// handleIdent processes identifier expressions with StringPool integration
func handleIdent(e *ast.Ident, info *types.Info, pkgName string, fset *token.FileSet, meta *Metadata) *CallArgument {
	name := e.Name
	pkg := pkgName
	typeStr := ""

	if info != nil {
		if obj := info.ObjectOf(e); obj != nil {
			if typePkgName, ok := obj.(*types.PkgName); ok {
				// This is an imported package identifier
				// Use the real import path and the real package name
				pkg = typePkgName.Imported().Path()  // The real import path
				name = typePkgName.Imported().Name() // The real package name
				// Do NOT trim the package name from the import path
			} else {
				if obj.Pkg() != nil {
					pkg = obj.Pkg().Path()
				}
				typeStr = strings.TrimPrefix(obj.Type().String(), pkg+defaultSep)
			}
		}
	}

	arg := NewCallArgument(meta)
	arg.SetKind(KindIdent)
	arg.SetName(name)
	arg.SetPkg(pkg)
	arg.SetType(typeStr)
	arg.SetPosition(getPosition(e.Pos(), fset))
	return arg
}

// handleSelector processes selector expressions with StringPool integration
func handleSelector(e *ast.SelectorExpr, info *types.Info, pkgName string, fset *token.FileSet, meta *Metadata) *CallArgument {
	x := ExprToCallArgument(e.X, info, pkgName, fset, meta)
	sel := ExprToCallArgument(e.Sel, info, pkgName, fset, meta)
	pkg := pkgName
	typeStr := ""

	if info != nil {
		if obj := info.ObjectOf(e.Sel); obj != nil {
			if typePkgName, ok := obj.(*types.PkgName); ok {
				// This is an imported package identifier
				// Use the real import path and the real package name
				pkg = typePkgName.Imported().Path() // The real import path
				// Do NOT trim the package name from the import path
			} else {
				if obj.Pkg() != nil {
					pkg = obj.Pkg().Path()
				}
				typeStr = strings.TrimPrefix(obj.Type().String(), pkg+defaultSep)
			}
		}
	}

	arg := NewCallArgument(meta)
	arg.SetKind(KindSelector)
	arg.X = x
	arg.Sel = sel
	arg.SetPkg(pkg)
	arg.SetType(typeStr)
	arg.SetPosition(getPosition(e.Pos(), fset))
	return arg
}

// handleCallExpr processes function call expressions with StringPool integration
func handleCallExpr(e *ast.CallExpr, info *types.Info, pkgName string, fset *token.FileSet, meta *Metadata) *CallArgument {
	args := make([]CallArgument, len(e.Args))
	for i, a := range e.Args {
		arg := ExprToCallArgument(a, info, pkgName, fset, meta)
		args[i] = *arg
	}
	fun := ExprToCallArgument(e.Fun, info, pkgName, fset, meta)

	// Build parameter-to-argument mapping
	paramArgMap := make(map[string]CallArgument)
	typeParamMap := make(map[string]string)

	// Get the *types.Object for the function being called
	// This is crucial for getting the *declared* generic type parameters
	extractParamsAndTypeParams(e, info, args, paramArgMap, typeParamMap)

	arg := NewCallArgument(meta)
	arg.SetKind(KindCall)
	arg.Fun = fun
	arg.Args = args
	arg.ParamArgMap = paramArgMap
	arg.SetPosition(getPosition(e.Pos(), fset))
	arg.TypeParamMap = typeParamMap
	return arg
}

// handleUnaryExprWithMetadata processes unary expressions with StringPool integration
func handleUnaryExpr(e *ast.UnaryExpr, info *types.Info, pkgName string, fset *token.FileSet, meta *Metadata) *CallArgument {
	x := ExprToCallArgument(e.X, info, pkgName, fset, meta)
	op := e.Op.String()

	arg := NewCallArgument(meta)
	arg.SetKind(KindUnary)
	arg.X = x
	arg.SetValue(op)
	arg.SetPosition(getPosition(e.Pos(), fset))
	return arg
}

// handleBinaryExpr processes binary expressions with StringPool integration
func handleBinaryExpr(e *ast.BinaryExpr, info *types.Info, pkgName string, fset *token.FileSet, meta *Metadata) *CallArgument {
	x := ExprToCallArgument(e.X, info, pkgName, fset, meta)
	y := ExprToCallArgument(e.Y, info, pkgName, fset, meta)
	op := e.Op.String()

	arg := NewCallArgument(meta)
	arg.SetKind(KindBinary)
	arg.X = x
	arg.Fun = y
	arg.SetValue(op)
	arg.SetPosition(getPosition(e.Pos(), fset))
	return arg
}

// handleIndexExpr processes index expressions with StringPool integration
func handleIndexExpr(e *ast.IndexExpr, info *types.Info, pkgName string, fset *token.FileSet, meta *Metadata) *CallArgument {
	x := ExprToCallArgument(e.X, info, pkgName, fset, meta)
	index := ExprToCallArgument(e.Index, info, pkgName, fset, meta)

	arg := NewCallArgument(meta)
	arg.SetKind(KindIndex)
	arg.X = x
	arg.Fun = index
	arg.SetPosition(getPosition(e.Pos(), fset))
	return arg
}

// handleIndexListExp processes index list expressions with StringPool integration
func handleIndexListExpr(e *ast.IndexListExpr, info *types.Info, pkgName string, fset *token.FileSet, meta *Metadata) *CallArgument {
	x := ExprToCallArgument(e.X, info, pkgName, fset, meta)
	indices := make([]CallArgument, len(e.Indices))
	for i, idx := range e.Indices {
		index := ExprToCallArgument(idx, info, pkgName, fset, meta)
		indices[i] = *index
	}

	arg := NewCallArgument(meta)
	arg.SetKind(KindIndexList)
	arg.X = x
	arg.Args = indices
	arg.SetPosition(getPosition(e.Pos(), fset))
	return arg
}

// handleParenExpr processes parenthesized expressions with StringPool integration
func handleParenExpr(e *ast.ParenExpr, info *types.Info, pkgName string, fset *token.FileSet, meta *Metadata) *CallArgument {
	x := ExprToCallArgument(e.X, info, pkgName, fset, meta)

	arg := NewCallArgument(meta)
	arg.SetKind(KindParen)
	arg.X = x
	arg.SetPosition(getPosition(e.Pos(), fset))
	return arg
}

// handleStarExpr processes star expressions with StringPool integration
func handleStarExpr(e *ast.StarExpr, info *types.Info, pkgName string, fset *token.FileSet, meta *Metadata) *CallArgument {
	x := ExprToCallArgument(e.X, info, pkgName, fset, meta)

	arg := NewCallArgument(meta)
	arg.SetKind(KindStar)
	arg.X = x
	arg.SetPosition(getPosition(e.Pos(), fset))
	return arg
}

// handleArrayType processes array type expressions with StringPool integration
func handleArrayType(e *ast.ArrayType, info *types.Info, pkgName string, fset *token.FileSet, meta *Metadata) *CallArgument {
	elt := ExprToCallArgument(e.Elt, info, pkgName, fset, meta)

	var lenArg *CallArgument
	if e.Len != nil {
		lenArg = ExprToCallArgument(e.Len, info, pkgName, fset, meta)
	}

	arg := NewCallArgument(meta)
	arg.SetKind(KindArrayType)
	arg.X = elt
	if lenArg != nil {
		arg.SetValue(lenArg.GetValue())
	}
	arg.SetPosition(getPosition(e.Pos(), fset))
	return arg
}

// handleSliceExpr processes slice expressions with StringPool integration
func handleSliceExpr(e *ast.SliceExpr, info *types.Info, pkgName string, fset *token.FileSet, meta *Metadata) *CallArgument {
	x := ExprToCallArgument(e.X, info, pkgName, fset, meta)
	args := []CallArgument{}
	if e.Low != nil {
		l := ExprToCallArgument(e.Low, info, pkgName, fset, meta)
		args = append(args, *l)
	}
	if e.High != nil {
		h := ExprToCallArgument(e.High, info, pkgName, fset, meta)
		args = append(args, *h)
	}
	if e.Max != nil {
		m := ExprToCallArgument(e.Max, info, pkgName, fset, meta)
		args = append(args, *m)
	}

	arg := NewCallArgument(meta)
	arg.SetKind(KindSlice)
	arg.X = x
	arg.Args = args
	arg.SetPosition(getPosition(e.Pos(), fset))
	return arg
}

// handleCompositeLit processes composite literal expressions
func handleCompositeLit(e *ast.CompositeLit, info *types.Info, pkgName string, fset *token.FileSet, meta *Metadata) *CallArgument {
	typeExpr := ExprToCallArgument(e.Type, info, pkgName, fset, meta)
	elts := make([]CallArgument, len(e.Elts))
	for i, elt := range e.Elts {
		elts[i] = *ExprToCallArgument(elt, info, pkgName, fset, meta)
	}

	arg := NewCallArgument(meta)
	arg.SetKind(KindCompositeLit)
	arg.X = typeExpr
	arg.Args = elts
	arg.SetPosition(getPosition(e.Pos(), fset))
	return arg
}

// handleKeyValueExpr processes key-value expressions
func handleKeyValueExpr(e *ast.KeyValueExpr, info *types.Info, pkgName string, fset *token.FileSet, meta *Metadata) *CallArgument {
	key := ExprToCallArgument(e.Key, info, pkgName, fset, meta)
	value := ExprToCallArgument(e.Value, info, pkgName, fset, meta)

	arg := NewCallArgument(meta)
	arg.SetKind(KindKeyValue)
	arg.X = key
	arg.Fun = value
	arg.SetPosition(getPosition(e.Pos(), fset))
	return arg
}

// handleTypeAssertExprWithMetadata processes type assertion expressions with StringPool integration
func handleTypeAssertExpr(e *ast.TypeAssertExpr, info *types.Info, pkgName string, fset *token.FileSet, meta *Metadata) *CallArgument {
	x := ExprToCallArgument(e.X, info, pkgName, fset, meta)
	typeExpr := ExprToCallArgument(e.Type, info, pkgName, fset, meta)

	arg := NewCallArgument(meta)
	arg.SetKind(KindTypeAssert)
	arg.X = x
	arg.Fun = typeExpr
	arg.SetPosition(getPosition(e.Pos(), fset))
	return arg
}

// handleChanType processes channel type expressions
func handleChanType(e *ast.ChanType, info *types.Info, pkgName string, fset *token.FileSet, meta *Metadata) *CallArgument {
	elt := ExprToCallArgument(e.Value, info, pkgName, fset, meta)

	var dir string
	switch e.Dir {
	case ast.SEND:
		dir = "send"
	case ast.RECV:
		dir = "recv"
	case ast.SEND | ast.RECV:
		dir = "bidir"
	}

	arg := NewCallArgument(meta)
	arg.SetKind(KindChanType)
	arg.X = elt
	arg.SetValue(dir)
	arg.SetPosition(getPosition(e.Pos(), fset))
	return arg
}

// handleMapType processes map type expressions
func handleMapType(e *ast.MapType, info *types.Info, pkgName string, fset *token.FileSet, meta *Metadata) *CallArgument {
	key := ExprToCallArgument(e.Key, info, pkgName, fset, meta)
	value := ExprToCallArgument(e.Value, info, pkgName, fset, meta)

	arg := NewCallArgument(meta)
	arg.SetKind(KindMapType)
	arg.X = key
	arg.Fun = value
	arg.SetPosition(getPosition(e.Pos(), fset))
	return arg
}

// handleStructType processes struct types with StringPool integration
func handleStructType(e *ast.StructType, info *types.Info, pkgName string, fset *token.FileSet, meta *Metadata) *CallArgument {
	fields := make([]CallArgument, 0, len(e.Fields.List))

	for _, field := range e.Fields.List {
		fieldType := ExprToCallArgument(field.Type, info, pkgName, fset, meta)

		if len(field.Names) == 0 {
			// Embedded field
			arg := NewCallArgument(meta)
			arg.SetKind(KindEmbed)
			arg.X = fieldType
			fields = append(fields, *arg)
		} else {
			// Named fields
			for _, name := range field.Names {
				arg := NewCallArgument(meta)
				arg.SetKind(KindField)
				arg.SetName(name.Name)
				arg.Type = fieldType.Type
				fields = append(fields, *arg)
			}
		}
	}

	arg := NewCallArgument(meta)
	arg.SetKind(KindStructType)
	arg.Args = fields
	arg.SetPosition(getPosition(e.Pos(), fset))
	return arg
}

// handleInterfaceType processes interface type expressions
func handleInterfaceType(e *ast.InterfaceType, meta *Metadata) *CallArgument {
	methods := make([]CallArgument, len(e.Methods.List))
	for i, method := range e.Methods.List {
		if len(method.Names) > 0 {
			arg := NewCallArgument(meta)
			arg.SetKind(KindInterfaceMethod)
			arg.SetName(method.Names[0].Name)
			methods[i] = *arg
		}
	}

	arg := NewCallArgument(meta)
	arg.SetKind(KindInterfaceType)
	arg.Args = methods
	return arg
}

// handleEllipsis processes ellipsis expressions
func handleEllipsis(e *ast.Ellipsis, info *types.Info, pkgName string, fset *token.FileSet, meta *Metadata) *CallArgument {
	elt := ExprToCallArgument(e.Elt, info, pkgName, fset, meta)

	arg := NewCallArgument(meta)
	arg.SetKind(KindEllipsis)
	arg.X = elt
	arg.SetPosition(getPosition(e.Pos(), fset))
	return arg
}

// handleFuncType processes function type expressions
func handleFuncType(e *ast.FuncType, info *types.Info, pkgName string, fset *token.FileSet, meta *Metadata) *CallArgument {
	var params []CallArgument
	if e.Params != nil {
		params = make([]CallArgument, len(e.Params.List))
		for i, field := range e.Params.List {
			fieldType := ExprToCallArgument(field.Type, info, pkgName, fset, meta)
			params[i] = *fieldType
		}
	}

	var results []CallArgument
	if e.Results != nil {
		results = make([]CallArgument, len(e.Results.List))
		for i, field := range e.Results.List {
			fieldType := ExprToCallArgument(field.Type, info, pkgName, fset, meta)
			results[i] = *fieldType
		}
	}

	funcType := NewCallArgument(meta)
	funcType.SetKind(KindFuncType)
	funcType.Args = params

	resultsArg := NewCallArgument(meta)
	resultsArg.SetKind(KindFuncResults)
	resultsArg.Args = results
	funcType.Fun = resultsArg

	return funcType
}

// ExprToString returns a string representation of an expression.
func ExprToString(expr ast.Expr) string {
	if expr == nil {
		return ""
	}
	return fmt.Sprintf("%#v", expr)
}

// CallArgToString converts a CallArgument to its string representation
func CallArgToString(arg CallArgument) string {
	return callArgToString(arg, nil)
}

// callArgToString converts a CallArgument to its string representation
func callArgToString(arg CallArgument, parent *CallArgument) string {
	switch arg.GetKind() {
	case KindIdent:
		if arg.Type != -1 && parent != nil {
			return arg.GetType()
		}
		return arg.GetName()
	case KindLiteral:
		return strings.Trim(arg.GetValue(), "\"")
	case KindSelector:
		if arg.X != nil {
			argX := callArgToString(*arg.X, &arg)
			argX = strings.TrimPrefix(argX, "*")

			return fmt.Sprintf("%s.%s", argX, arg.Sel.GetName())
		}
		return arg.Sel.GetName()
	case KindCall:
		if arg.Fun != nil {
			return fmt.Sprintf("%s(%s)", callArgToString(*arg.Fun, &arg), strings.Join(callArgToStringArgs(arg.Args, &arg), ", "))
		}
		return "call()"
	case KindUnary:
		if arg.X != nil {
			return fmt.Sprintf("%s%s", arg.GetValue(), callArgToString(*arg.X, &arg))
		}
		return arg.GetValue()
	case KindBinary:
		if arg.X != nil && arg.Fun != nil {
			return fmt.Sprintf("%s %s %s", callArgToString(*arg.X, &arg), arg.GetValue(), callArgToString(*arg.Fun, &arg))
		}
		return arg.GetValue()
	case KindIndex:
		if arg.X != nil && arg.Fun != nil {
			return fmt.Sprintf("%s[%s]", callArgToString(*arg.X, &arg), callArgToString(*arg.Fun, &arg))
		}
		return "index"
	case KindIndexList:
		if arg.X != nil {
			return fmt.Sprintf("%s[%s]", callArgToString(*arg.X, &arg), strings.Join(callArgToStringArgs(arg.Args, &arg), ", "))
		}
		return "index_list"
	case KindParen:
		if arg.X != nil {
			return fmt.Sprintf("(%s)", callArgToString(*arg.X, &arg))
		}
		return "()"
	case KindStar:
		if arg.X != nil {
			return fmt.Sprintf("*%s", callArgToString(*arg.X, &arg))
		}
		return "*"
	case KindArrayType:
		if arg.X != nil {
			return fmt.Sprintf("[%s]%s", arg.GetValue(), callArgToString(*arg.X, &arg))
		}
		return fmt.Sprintf("[%s]", arg.GetValue())
	case KindSlice:
		if arg.X != nil && len(arg.Args) >= 2 {
			return fmt.Sprintf("%s[%s:%s]", callArgToString(*arg.X, &arg), callArgToString(arg.Args[0], &arg), callArgToString(arg.Args[1], &arg))
		}
		return "slice"
	case KindCompositeLit:
		if arg.X != nil {
			return fmt.Sprintf("%s{%s}", callArgToString(*arg.X, &arg), strings.Join(callArgToStringArgs(arg.Args, &arg), ", "))
		}
		return "{}"
	case KindKeyValue:
		if arg.X != nil && arg.Fun != nil {
			return fmt.Sprintf("%s: %s", callArgToString(*arg.X, &arg), callArgToString(*arg.Fun, &arg))
		}
		return "key: value"
	case KindTypeAssert:
		if arg.X != nil && arg.Fun != nil {
			return fmt.Sprintf("%s.(%s)", callArgToString(*arg.X, &arg), callArgToString(*arg.Fun, &arg))
		}
		return "type_assert"
	case KindFuncLit:
		return arg.GetValue()
	case KindChanType:
		if arg.X != nil {
			return fmt.Sprintf("chan %s", callArgToString(*arg.X, &arg))
		}
		return "chan"
	case KindMapType:
		if arg.X != nil && arg.Fun != nil {
			return fmt.Sprintf("map[%s]%s", callArgToString(*arg.X, &arg), callArgToString(*arg.Fun, &arg))
		}
		return "map"
	case KindStructType:
		return fmt.Sprintf("struct{%s}", strings.Join(callArgToStringArgs(arg.Args, &arg), ", "))
	case KindInterfaceType:
		return fmt.Sprintf("interface{%s}", strings.Join(callArgToStringArgs(arg.Args, &arg), ", "))
	case KindEllipsis:
		if arg.X != nil {
			return fmt.Sprintf("...%s", callArgToString(*arg.X, &arg))
		}
		return "..."
	case KindFuncType:
		if arg.Fun != nil {
			return fmt.Sprintf("func(%s) %s", strings.Join(callArgToStringArgs(arg.Args, &arg), ", "), callArgToString(*arg.Fun, &arg))
		}
		return "func()"
	case KindFuncResults:
		return strings.Join(callArgToStringArgs(arg.Args, &arg), ", ")
	default:
		return arg.GetRaw()
	}
}

// callArgToStringArgs converts a slice of CallArguments to string representations
func callArgToStringArgs(args []CallArgument, parent *CallArgument) []string {
	result := make([]string, len(args))
	for i := range args {
		result[i] = callArgToString(args[i], parent)
	}
	return result
}
