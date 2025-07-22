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
func ExprToCallArgument(expr ast.Expr, info *types.Info, pkgName string, fset *token.FileSet) CallArgument {
	if expr == nil {
		return CallArgument{Kind: kindRaw, Raw: ""}
	}

	switch e := expr.(type) {
	case *ast.Ident:
		return handleIdent(e, info, pkgName)
	case *ast.BasicLit:
		return CallArgument{Kind: kindLiteral, Value: e.Value}
	case *ast.SelectorExpr:
		return handleSelector(e, info, pkgName, fset)
	case *ast.CallExpr:
		return handleCallExpr(e, info, pkgName, fset)
	case *ast.UnaryExpr:
		return handleUnaryExpr(e, info, pkgName, fset)
	case *ast.BinaryExpr:
		return handleBinaryExpr(e, info, pkgName, fset)
	case *ast.IndexExpr:
		return handleIndexExpr(e, info, pkgName, fset)
	case *ast.IndexListExpr:
		return handleIndexListExpr(e, info, pkgName, fset)
	case *ast.ParenExpr:
		return handleParenExpr(e, info, pkgName, fset)
	case *ast.StarExpr:
		return handleStarExpr(e, info, pkgName, fset)
	case *ast.ArrayType:
		return handleArrayType(e, info, pkgName, fset)
	case *ast.SliceExpr:
		return handleSliceExpr(e, info, pkgName, fset)
	case *ast.CompositeLit:
		return handleCompositeLit(e, info, pkgName, fset)
	case *ast.KeyValueExpr:
		return handleKeyValueExpr(e, info, pkgName, fset)
	case *ast.TypeAssertExpr:
		return handleTypeAssertExpr(e, info, pkgName, fset)
	case *ast.FuncLit:
		return CallArgument{Kind: kindFuncLit, Value: valueFuncLit}
	case *ast.ChanType:
		return handleChanType(e, info, pkgName, fset)
	case *ast.MapType:
		return handleMapType(e, info, pkgName, fset)
	case *ast.StructType:
		return handleStructType(e, info, pkgName, fset)
	case *ast.InterfaceType:
		return handleInterfaceType(e, info, pkgName, fset)
	case *ast.Ellipsis:
		return handleEllipsis(e, info, pkgName, fset)
	case *ast.FuncType:
		return handleFuncType(e, info, pkgName, fset)
	}

	// Fallback for other complex expressions
	return CallArgument{Kind: kindRaw, Raw: ExprToString(expr)}
}

// handleIdent processes identifier expressions
func handleIdent(e *ast.Ident, info *types.Info, pkgName string) CallArgument {
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

	return CallArgument{Kind: kindIdent, Name: name, Pkg: pkg, Type: typeStr}
}

// handleSelector processes selector expressions
func handleSelector(e *ast.SelectorExpr, info *types.Info, pkgName string, fset *token.FileSet) CallArgument {
	x := ExprToCallArgument(e.X, info, pkgName, fset)
	sel := ExprToCallArgument(e.Sel, info, pkgName, fset)
	sel.X = &x

	return sel
}

// handleCallExpr processes function call expressions
func handleCallExpr(e *ast.CallExpr, info *types.Info, pkgName string, fset *token.FileSet) CallArgument {
	args := make([]CallArgument, len(e.Args))
	for i, a := range e.Args {
		args[i] = ExprToCallArgument(a, info, pkgName, fset)
	}
	fun := ExprToCallArgument(e.Fun, info, pkgName, fset)
	// Build parameter-to-argument mapping
	paramArgMap := make(map[string]CallArgument)
	typeParamMap := make(map[string]string)

	// Get the *types.Object for the function being called
	// This is crucial for getting the *declared* generic type parameters
	extractParamsAndTypeParams(e, info, args, paramArgMap, typeParamMap)

	return CallArgument{
		Kind:         kindCall,
		Fun:          &fun,
		Args:         args,
		Position:     getPosition(e.Pos(), fset),
		TypeParamMap: typeParamMap,
		ParamArgMap:  paramArgMap,
	}
}

// handleUnaryExpr processes unary expressions
func handleUnaryExpr(e *ast.UnaryExpr, info *types.Info, pkgName string, fset *token.FileSet) CallArgument {
	x := ExprToCallArgument(e.X, info, pkgName, fset)
	op := ""
	switch e.Op {
	case token.AND:
		op = "&"
	case token.MUL:
		op = "*"
	case token.NOT:
		op = "!"
	case token.XOR:
		op = "^"
	case token.SUB:
		op = "-"
	case token.ADD:
		op = "+"
	}
	return CallArgument{
		Kind:  kindUnary,
		Value: op,
		X:     &x,
	}
}

// handleBinaryExpr processes binary expressions
func handleBinaryExpr(e *ast.BinaryExpr, info *types.Info, pkgName string, fset *token.FileSet) CallArgument {
	x := ExprToCallArgument(e.X, info, pkgName, fset)
	y := ExprToCallArgument(e.Y, info, pkgName, fset)
	op := e.Op.String()
	return CallArgument{
		Kind:  kindBinary,
		Value: op,
		X:     &x,
		Fun:   &y, // Reuse Fun field for the second operand
	}
}

// handleIndexExpr processes index expressions
func handleIndexExpr(e *ast.IndexExpr, info *types.Info, pkgName string, fset *token.FileSet) CallArgument {
	x := ExprToCallArgument(e.X, info, pkgName, fset)
	index := ExprToCallArgument(e.Index, info, pkgName, fset)
	return CallArgument{
		Kind: kindIndex,
		X:    &x,
		Fun:  &index, // Reuse Fun field for the index
	}
}

// handleIndexListExpr processes index list expressions
func handleIndexListExpr(e *ast.IndexListExpr, info *types.Info, pkgName string, fset *token.FileSet) CallArgument {
	x := ExprToCallArgument(e.X, info, pkgName, fset)
	indices := make([]CallArgument, len(e.Indices))
	for i, idx := range e.Indices {
		indices[i] = ExprToCallArgument(idx, info, pkgName, fset)
	}
	return CallArgument{
		Kind: kindIndexList,
		X:    &x,
		Args: indices,
	}
}

// handleParenExpr processes parenthesized expressions
func handleParenExpr(e *ast.ParenExpr, info *types.Info, pkgName string, fset *token.FileSet) CallArgument {
	x := ExprToCallArgument(e.X, info, pkgName, fset)
	return CallArgument{
		Kind: kindParen,
		X:    &x,
	}
}

// handleStarExpr processes star expressions
func handleStarExpr(e *ast.StarExpr, info *types.Info, pkgName string, fset *token.FileSet) CallArgument {
	x := ExprToCallArgument(e.X, info, pkgName, fset)
	return CallArgument{
		Kind: kindStar,
		X:    &x,
	}
}

// handleArrayType processes array type expressions
func handleArrayType(e *ast.ArrayType, info *types.Info, pkgName string, fset *token.FileSet) CallArgument {
	elt := ExprToCallArgument(e.Elt, info, pkgName, fset)
	len := ""
	if e.Len != nil {
		lenArg := ExprToCallArgument(e.Len, info, pkgName, fset)
		if lenArg.Kind == kindLiteral {
			len = lenArg.Value
		}
	}
	return CallArgument{
		Kind:  kindArrayType,
		Value: len,
		X:     &elt,
	}
}

// handleSliceExpr processes slice expressions
func handleSliceExpr(e *ast.SliceExpr, info *types.Info, pkgName string, fset *token.FileSet) CallArgument {
	x := ExprToCallArgument(e.X, info, pkgName, fset)
	args := []CallArgument{}
	if e.Low != nil {
		l := ExprToCallArgument(e.Low, info, pkgName, fset)
		args = append(args, l)
	}
	if e.High != nil {
		h := ExprToCallArgument(e.High, info, pkgName, fset)
		args = append(args, h)
	}
	if e.Max != nil {
		m := ExprToCallArgument(e.Max, info, pkgName, fset)
		args = append(args, m)
	}
	return CallArgument{
		Kind: kindSlice,
		X:    &x,
		Args: args,
	}
}

// handleCompositeLit processes composite literal expressions
func handleCompositeLit(e *ast.CompositeLit, info *types.Info, pkgName string, fset *token.FileSet) CallArgument {
	typeExpr := ExprToCallArgument(e.Type, info, pkgName, fset)
	elts := make([]CallArgument, len(e.Elts))
	for i, elt := range e.Elts {
		elts[i] = ExprToCallArgument(elt, info, pkgName, fset)
	}
	return CallArgument{
		Kind: kindCompositeLit,
		X:    &typeExpr,
		Args: elts,
	}
}

// handleKeyValueExpr processes key-value expressions
func handleKeyValueExpr(e *ast.KeyValueExpr, info *types.Info, pkgName string, fset *token.FileSet) CallArgument {
	key := ExprToCallArgument(e.Key, info, pkgName, fset)
	value := ExprToCallArgument(e.Value, info, pkgName, fset)
	return CallArgument{
		Kind: kindKeyValue,
		X:    &key,
		Fun:  &value, // Reuse Fun field for the value
	}
}

// handleTypeAssertExpr processes type assertion expressions
func handleTypeAssertExpr(e *ast.TypeAssertExpr, info *types.Info, pkgName string, fset *token.FileSet) CallArgument {
	x := ExprToCallArgument(e.X, info, pkgName, fset)
	typeExpr := ExprToCallArgument(e.Type, info, pkgName, fset)
	return CallArgument{
		Kind: kindTypeAssert,
		X:    &x,
		Fun:  &typeExpr, // Reuse Fun field for the type
	}
}

// handleChanType processes channel type expressions
func handleChanType(e *ast.ChanType, info *types.Info, pkgName string, fset *token.FileSet) CallArgument {
	elt := ExprToCallArgument(e.Value, info, pkgName, fset)
	dir := ""
	switch e.Dir {
	case ast.SEND:
		dir = "send"
	case ast.RECV:
		dir = "recv"
	case ast.SEND | ast.RECV:
		dir = "bidir"
	}
	return CallArgument{
		Kind:  kindChanType,
		Value: dir,
		X:     &elt,
	}
}

// handleMapType processes map type expressions
func handleMapType(e *ast.MapType, info *types.Info, pkgName string, fset *token.FileSet) CallArgument {
	key := ExprToCallArgument(e.Key, info, pkgName, fset)
	value := ExprToCallArgument(e.Value, info, pkgName, fset)
	return CallArgument{
		Kind: kindMapType,
		X:    &key,
		Fun:  &value, // Reuse Fun field for the value type
	}
}

// handleStructType processes struct type expressions
func handleStructType(e *ast.StructType, info *types.Info, pkgName string, fset *token.FileSet) CallArgument {
	fields := make([]CallArgument, 0, len(e.Fields.List))
	for _, field := range e.Fields.List {
		fieldType := ExprToCallArgument(field.Type, info, pkgName, fset)
		comments := ""
		if field.Doc != nil {
			comments += field.Doc.Text()
		}
		if field.Comment != nil {
			if comments != "" {
				comments += "\n"
			}
			comments += field.Comment.Text()
		}
		if len(field.Names) == 0 {
			// Embedded (anonymous) field
			fields = append(fields, CallArgument{
				Kind: kindEmbed,
				X:    &fieldType,
			})
			continue
		}

		for _, name := range field.Names {
			fields = append(fields, CallArgument{
				Kind: kindField,
				Name: name.Name,
				Type: fieldType.Type,
			})
		}
	}
	return CallArgument{
		Kind: kindStructType,
		Args: fields,
	}
}

// handleInterfaceType processes interface type expressions
func handleInterfaceType(e *ast.InterfaceType, info *types.Info, pkgName string, fset *token.FileSet) CallArgument {
	methods := make([]CallArgument, len(e.Methods.List))
	for i, method := range e.Methods.List {
		// Simplified method representation
		if len(method.Names) > 0 {
			methods[i] = CallArgument{
				Kind: kindInterfaceMethod,
				Name: method.Names[0].Name,
			}
		}
	}
	return CallArgument{
		Kind: kindInterfaceType,
		Args: methods,
	}
}

// handleEllipsis processes ellipsis expressions
func handleEllipsis(e *ast.Ellipsis, info *types.Info, pkgName string, fset *token.FileSet) CallArgument {
	elt := ExprToCallArgument(e.Elt, info, pkgName, fset)
	return CallArgument{
		Kind: kindEllipsis,
		X:    &elt,
	}
}

// handleFuncType processes function type expressions
func handleFuncType(e *ast.FuncType, info *types.Info, pkgName string, fset *token.FileSet) CallArgument {
	var params []CallArgument
	if e.Params != nil {
		params = make([]CallArgument, len(e.Params.List))
		for i, field := range e.Params.List {
			fieldType := ExprToCallArgument(field.Type, info, pkgName, fset)
			params[i] = fieldType
		}
	}

	var results []CallArgument
	if e.Results != nil {
		results = make([]CallArgument, len(e.Results.List))
		for i, field := range e.Results.List {
			fieldType := ExprToCallArgument(field.Type, info, pkgName, fset)
			results[i] = fieldType
		}
	}

	return CallArgument{
		Kind: kindFuncType,
		Args: params, // Use Args for parameters
		Fun: &CallArgument{ // Use Fun field to store results
			Kind: kindFuncResults,
			Args: results,
		},
	}
}

// ExprToString returns a string representation of an expression.
func ExprToString(expr ast.Expr) string {
	if expr == nil {
		return ""
	}
	return fmt.Sprintf("%#v", expr)
}

// callArgToString converts a CallArgument to its string representation
func callArgToString(arg CallArgument) string {
	switch arg.Kind {
	case kindIdent:
		return arg.Name
	case kindLiteral:
		return arg.Value
	case kindSelector:
		if arg.X != nil {
			return fmt.Sprintf("%s.%s", callArgToString(*arg.X), arg.Sel)
		}
		return arg.Sel
	case kindCall:
		if arg.Fun != nil {
			return fmt.Sprintf("%s(%s)", callArgToString(*arg.Fun), strings.Join(callArgToStringArgs(arg.Args), ", "))
		}
		return "call()"
	case kindUnary:
		if arg.X != nil {
			return fmt.Sprintf("%s%s", arg.Value, callArgToString(*arg.X))
		}
		return arg.Value
	case kindBinary:
		if arg.X != nil && arg.Fun != nil {
			return fmt.Sprintf("%s %s %s", callArgToString(*arg.X), arg.Value, callArgToString(*arg.Fun))
		}
		return arg.Value
	case kindIndex:
		if arg.X != nil && arg.Fun != nil {
			return fmt.Sprintf("%s[%s]", callArgToString(*arg.X), callArgToString(*arg.Fun))
		}
		return "index"
	case kindIndexList:
		if arg.X != nil {
			return fmt.Sprintf("%s[%s]", callArgToString(*arg.X), strings.Join(callArgToStringArgs(arg.Args), ", "))
		}
		return "index_list"
	case kindParen:
		if arg.X != nil {
			return fmt.Sprintf("(%s)", callArgToString(*arg.X))
		}
		return "()"
	case kindStar:
		if arg.X != nil {
			return fmt.Sprintf("*%s", callArgToString(*arg.X))
		}
		return "*"
	case kindArrayType:
		if arg.X != nil {
			return fmt.Sprintf("[%s]%s", arg.Value, callArgToString(*arg.X))
		}
		return fmt.Sprintf("[%s]", arg.Value)
	case kindSlice:
		if arg.X != nil && len(arg.Args) >= 2 {
			return fmt.Sprintf("%s[%s:%s]", callArgToString(*arg.X), callArgToString(arg.Args[0]), callArgToString(arg.Args[1]))
		}
		return "slice"
	case kindCompositeLit:
		if arg.X != nil {
			return fmt.Sprintf("%s{%s}", callArgToString(*arg.X), strings.Join(callArgToStringArgs(arg.Args), ", "))
		}
		return "{}"
	case kindKeyValue:
		if arg.X != nil && arg.Fun != nil {
			return fmt.Sprintf("%s: %s", callArgToString(*arg.X), callArgToString(*arg.Fun))
		}
		return "key: value"
	case kindTypeAssert:
		if arg.X != nil && arg.Fun != nil {
			return fmt.Sprintf("%s.(%s)", callArgToString(*arg.X), callArgToString(*arg.Fun))
		}
		return "type_assert"
	case kindFuncLit:
		return arg.Value
	case kindChanType:
		if arg.X != nil {
			return fmt.Sprintf("chan %s", callArgToString(*arg.X))
		}
		return "chan"
	case kindMapType:
		if arg.X != nil && arg.Fun != nil {
			return fmt.Sprintf("map[%s]%s", callArgToString(*arg.X), callArgToString(*arg.Fun))
		}
		return "map"
	case kindStructType:
		return fmt.Sprintf("struct{%s}", strings.Join(callArgToStringArgs(arg.Args), ", "))
	case kindInterfaceType:
		return fmt.Sprintf("interface{%s}", strings.Join(callArgToStringArgs(arg.Args), ", "))
	case kindEllipsis:
		if arg.X != nil {
			return fmt.Sprintf("...%s", callArgToString(*arg.X))
		}
		return "..."
	case kindFuncType:
		if arg.Fun != nil {
			return fmt.Sprintf("func(%s) %s", strings.Join(callArgToStringArgs(arg.Args), ", "), callArgToString(*arg.Fun))
		}
		return "func()"
	case kindFuncResults:
		return strings.Join(callArgToStringArgs(arg.Args), ", ")
	default:
		return arg.Raw
	}
}

// callArgToStringArgs converts a slice of CallArguments to string representations
func callArgToStringArgs(args []CallArgument) []string {
	result := make([]string, len(args))
	for i := range args {
		result[i] = callArgToString(args[i])
	}
	return result
}
