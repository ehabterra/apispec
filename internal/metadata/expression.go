package metadata

import (
	"fmt"
	"strings"

	"go/ast"
	"go/token"
	"go/types"
)

// ExprToCallArgument returns a structured CallArgument for an expression.
func ExprToCallArgument(expr ast.Expr, info *types.Info, pkgName string, fset *token.FileSet) CallArgument {
	if expr == nil {
		return CallArgument{Kind: "raw", Raw: ""}
	}

	switch e := expr.(type) {
	case *ast.Ident:
		return handleIdent(e, info, pkgName)
	case *ast.BasicLit:
		return CallArgument{Kind: "literal", Value: e.Value}
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
		return CallArgument{Kind: "func_lit", Value: "func()"}
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
	return CallArgument{Kind: "raw", Raw: ExprToString(expr)}
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
				typeStr = strings.TrimPrefix(obj.Type().String(), pkg+".")
			}
		}
	}

	return CallArgument{Kind: "ident", Name: name, Pkg: pkg, Type: typeStr}
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
	return CallArgument{
		Kind: "call",
		Fun:  &fun,
		Args: args,
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
		Kind:  "unary",
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
		Kind:  "binary",
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
		Kind: "index",
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
		Kind: "index_list",
		X:    &x,
		Args: indices,
	}
}

// handleParenExpr processes parenthesized expressions
func handleParenExpr(e *ast.ParenExpr, info *types.Info, pkgName string, fset *token.FileSet) CallArgument {
	x := ExprToCallArgument(e.X, info, pkgName, fset)
	return CallArgument{
		Kind: "paren",
		X:    &x,
	}
}

// handleStarExpr processes star expressions
func handleStarExpr(e *ast.StarExpr, info *types.Info, pkgName string, fset *token.FileSet) CallArgument {
	x := ExprToCallArgument(e.X, info, pkgName, fset)
	return CallArgument{
		Kind: "star",
		X:    &x,
	}
}

// handleArrayType processes array type expressions
func handleArrayType(e *ast.ArrayType, info *types.Info, pkgName string, fset *token.FileSet) CallArgument {
	elt := ExprToCallArgument(e.Elt, info, pkgName, fset)
	len := ""
	if e.Len != nil {
		lenArg := ExprToCallArgument(e.Len, info, pkgName, fset)
		if lenArg.Kind == "literal" {
			len = lenArg.Value
		}
	}
	return CallArgument{
		Kind:  "array_type",
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
		Kind: "slice",
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
		Kind: "composite_lit",
		X:    &typeExpr,
		Args: elts,
	}
}

// handleKeyValueExpr processes key-value expressions
func handleKeyValueExpr(e *ast.KeyValueExpr, info *types.Info, pkgName string, fset *token.FileSet) CallArgument {
	key := ExprToCallArgument(e.Key, info, pkgName, fset)
	value := ExprToCallArgument(e.Value, info, pkgName, fset)
	return CallArgument{
		Kind: "key_value",
		X:    &key,
		Fun:  &value, // Reuse Fun field for the value
	}
}

// handleTypeAssertExpr processes type assertion expressions
func handleTypeAssertExpr(e *ast.TypeAssertExpr, info *types.Info, pkgName string, fset *token.FileSet) CallArgument {
	x := ExprToCallArgument(e.X, info, pkgName, fset)
	typeExpr := ExprToCallArgument(e.Type, info, pkgName, fset)
	return CallArgument{
		Kind: "type_assert",
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
		Kind:  "chan_type",
		Value: dir,
		X:     &elt,
	}
}

// handleMapType processes map type expressions
func handleMapType(e *ast.MapType, info *types.Info, pkgName string, fset *token.FileSet) CallArgument {
	key := ExprToCallArgument(e.Key, info, pkgName, fset)
	value := ExprToCallArgument(e.Value, info, pkgName, fset)
	return CallArgument{
		Kind: "map_type",
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
				Kind: "embed",
				X:    &fieldType,
			})
			continue
		}

		for _, name := range field.Names {
			fields = append(fields, CallArgument{
				Kind: "field",
				Name: name.Name,
				Type: fieldType.Type,
			})
		}
	}
	return CallArgument{
		Kind: "struct_type",
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
				Kind: "interface_method",
				Name: method.Names[0].Name,
			}
		}
	}
	return CallArgument{
		Kind: "interface_type",
		Args: methods,
	}
}

// handleEllipsis processes ellipsis expressions
func handleEllipsis(e *ast.Ellipsis, info *types.Info, pkgName string, fset *token.FileSet) CallArgument {
	elt := ExprToCallArgument(e.Elt, info, pkgName, fset)
	return CallArgument{
		Kind: "ellipsis",
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
		Kind: "func_type",
		Args: params, // Use Args for parameters
		Fun: &CallArgument{ // Use Fun field to store results
			Kind: "func_results",
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

// shortType extracts package and type name from a full type string
func shortType(typeStr string) (string, string) {
	if typeStr == "" {
		return "", ""
	}
	// Remove pointer prefix
	typeStr = strings.TrimPrefix(typeStr, "*")
	// Only keep the last part after dot or slash
	if idx := strings.LastIndex(typeStr, "."); idx != -1 {
		return typeStr[:idx], typeStr[idx+1:]
	}
	if idx := strings.LastIndex(typeStr, "/"); idx != -1 {
		return typeStr[:idx], typeStr[idx+1:]
	}
	return "", typeStr
}

// getTypeString extracts type string from ast.Expr
func getTypeString(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		// Handle pointer receivers like *MyStruct
		return "*" + getTypeString(t.X)
	case *ast.SelectorExpr:
		// Handle qualified types like pkg.MyStruct
		return getTypeString(t.X) + "." + t.Sel.Name
	case *ast.IndexExpr:
		// Handle generic types like MyStruct[T]
		return getTypeString(t.X) + "[" + getTypeString(t.Index) + "]"
	case *ast.IndexListExpr:
		// Handle generic types with multiple type parameters like MyStruct[T, U]
		var indices []string
		for _, index := range t.Indices {
			indices = append(indices, getTypeString(index))
		}
		return getTypeString(t.X) + "[" + strings.Join(indices, ", ") + "]"
	case *ast.UnaryExpr:
		// Handle unary expressions like &user, *ptr, !flag, etc.
		op := ""
		switch t.Op {
		case token.AND:
			op = "*"
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
		return op + getTypeString(t.X)
	case *ast.CompositeLit:
		// Handle composite literals like MyStruct{Field: value}
		if t.Type != nil {
			return getTypeString(t.Type)
		}
		// If no explicit type, try to infer from the literal structure
		if len(t.Elts) > 0 {
			// For struct literals, try to infer the type from the first key-value pair
			if kv, ok := t.Elts[0].(*ast.KeyValueExpr); ok {
				if ident, ok := kv.Key.(*ast.Ident); ok {
					// This might be a struct literal, try to infer the type
					return "struct{" + ident.Name + "}"
				}
			}
		}
		return "composite_literal"
	case *ast.CallExpr:
		// Handle function calls that return types
		return getTypeString(t.Fun) + "()"
	case *ast.BinaryExpr:
		// Handle binary expressions like a + b, x == y, etc.
		// For type inference, we might want to focus on the left operand
		return getTypeString(t.X)
	case *ast.ParenExpr:
		// Handle parenthesized expressions like (x + y)
		return getTypeString(t.X)
	case *ast.ArrayType:
		// Handle array types like []int, [5]string
		elt := getTypeString(t.Elt)
		if t.Len != nil {
			len := getTypeString(t.Len)
			return "[" + len + "]" + elt
		}
		return "[]" + elt
	case *ast.SliceExpr:
		// Handle slice expressions like array[low:high]
		return getTypeString(t.X)
	case *ast.TypeAssertExpr:
		// Handle type assertions like x.(string)
		return getTypeString(t.Type)
	case *ast.FuncLit:
		// Handle function literals like func() { ... }
		return "func()"
	case *ast.ChanType:
		// Handle channel types like chan int, <-chan int, chan<- int
		elt := getTypeString(t.Value)
		switch t.Dir {
		case ast.SEND:
			return "chan<- " + elt
		case ast.RECV:
			return "<-chan " + elt
		case ast.SEND | ast.RECV:
			return "chan " + elt
		}
		return "chan " + elt
	case *ast.MapType:
		// Handle map types like map[string]int
		key := getTypeString(t.Key)
		value := getTypeString(t.Value)
		return "map[" + key + "]" + value
	case *ast.StructType:
		// Handle struct type definitions
		return "struct{}"
	case *ast.InterfaceType:
		// Handle interface type definitions
		return "interface{}"
	case *ast.Ellipsis:
		// Handle variadic arguments like ...args
		return "..." + getTypeString(t.Elt)
	case *ast.FuncType:
		// Handle function types like func(int, string) error
		var params []string
		if t.Params != nil {
			for _, field := range t.Params.List {
				paramType := getTypeString(field.Type)
				params = append(params, paramType)
			}
		}

		var results []string
		if t.Results != nil {
			for _, field := range t.Results.List {
				resultType := getTypeString(field.Type)
				results = append(results, resultType)
			}
		}

		if len(results) > 0 {
			return "func(" + strings.Join(params, ", ") + ") " + strings.Join(results, ", ")
		}
		return "func(" + strings.Join(params, ", ") + ")"
	case *ast.BasicLit:
		// Handle basic literals like "hello", 42, true
		switch t.Kind {
		case token.STRING:
			return "string"
		case token.INT:
			return "int"
		case token.FLOAT:
			return "float64"
		case token.CHAR:
			return "rune"
		case token.IMAG:
			return "complex128"
		}
		return "literal"
	default:
		// Fallback for other complex types
		return ""
	}
}

// callArgToString converts a CallArgument to its string representation
func callArgToString(arg CallArgument) string {
	switch arg.Kind {
	case "ident":
		return arg.Name
	case "literal":
		return arg.Value
	case "selector":
		if arg.X != nil {
			return fmt.Sprintf("%s.%s", callArgToString(*arg.X), arg.Sel)
		}
		return arg.Sel
	case "call":
		if arg.Fun != nil {
			return fmt.Sprintf("%s(%s)", callArgToString(*arg.Fun), strings.Join(callArgToStringArgs(arg.Args), ", "))
		}
		return "call()"
	case "unary":
		if arg.X != nil {
			return fmt.Sprintf("%s%s", arg.Value, callArgToString(*arg.X))
		}
		return arg.Value
	case "binary":
		if arg.X != nil && arg.Fun != nil {
			return fmt.Sprintf("%s %s %s", callArgToString(*arg.X), arg.Value, callArgToString(*arg.Fun))
		}
		return arg.Value
	case "index":
		if arg.X != nil && arg.Fun != nil {
			return fmt.Sprintf("%s[%s]", callArgToString(*arg.X), callArgToString(*arg.Fun))
		}
		return "index"
	case "index_list":
		if arg.X != nil {
			return fmt.Sprintf("%s[%s]", callArgToString(*arg.X), strings.Join(callArgToStringArgs(arg.Args), ", "))
		}
		return "index_list"
	case "paren":
		if arg.X != nil {
			return fmt.Sprintf("(%s)", callArgToString(*arg.X))
		}
		return "()"
	case "star":
		if arg.X != nil {
			return fmt.Sprintf("*%s", callArgToString(*arg.X))
		}
		return "*"
	case "array_type":
		if arg.X != nil {
			return fmt.Sprintf("[%s]%s", arg.Value, callArgToString(*arg.X))
		}
		return fmt.Sprintf("[%s]", arg.Value)
	case "slice":
		if arg.X != nil && len(arg.Args) >= 2 {
			return fmt.Sprintf("%s[%s:%s]", callArgToString(*arg.X), callArgToString(arg.Args[0]), callArgToString(arg.Args[1]))
		}
		return "slice"
	case "composite_lit":
		if arg.X != nil {
			return fmt.Sprintf("%s{%s}", callArgToString(*arg.X), strings.Join(callArgToStringArgs(arg.Args), ", "))
		}
		return "{}"
	case "key_value":
		if arg.X != nil && arg.Fun != nil {
			return fmt.Sprintf("%s: %s", callArgToString(*arg.X), callArgToString(*arg.Fun))
		}
		return "key: value"
	case "type_assert":
		if arg.X != nil && arg.Fun != nil {
			return fmt.Sprintf("%s.(%s)", callArgToString(*arg.X), callArgToString(*arg.Fun))
		}
		return "type_assert"
	case "func_lit":
		return arg.Value
	case "chan_type":
		if arg.X != nil {
			return fmt.Sprintf("chan %s", callArgToString(*arg.X))
		}
		return "chan"
	case "map_type":
		if arg.X != nil && arg.Fun != nil {
			return fmt.Sprintf("map[%s]%s", callArgToString(*arg.X), callArgToString(*arg.Fun))
		}
		return "map"
	case "struct_type":
		return fmt.Sprintf("struct{%s}", strings.Join(callArgToStringArgs(arg.Args), ", "))
	case "interface_type":
		return fmt.Sprintf("interface{%s}", strings.Join(callArgToStringArgs(arg.Args), ", "))
	case "ellipsis":
		if arg.X != nil {
			return fmt.Sprintf("...%s", callArgToString(*arg.X))
		}
		return "..."
	case "func_type":
		if arg.Fun != nil {
			return fmt.Sprintf("func(%s) %s", strings.Join(callArgToStringArgs(arg.Args), ", "), callArgToString(*arg.Fun))
		}
		return "func()"
	case "func_results":
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
