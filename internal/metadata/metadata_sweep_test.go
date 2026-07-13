// Copyright 2026 Ehab Terra
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package metadata

import (
	"errors"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
)

// sweepMeta returns a minimal Metadata suitable for constructing
// CallArguments and exercising resolver helpers directly.
func sweepMeta() *Metadata {
	return &Metadata{
		StringPool: NewStringPool(),
		Packages:   map[string]*Package{},
	}
}

// sweepParseExpr parses a single expression and fails the test on error.
func sweepParseExpr(t *testing.T, src string) ast.Expr {
	t.Helper()
	e, err := parser.ParseExpr(src)
	if err != nil {
		t.Fatalf("ParseExpr(%q): %v", src, err)
	}
	return e
}

// sweepParseFile parses a whole file without type-checking.
func sweepParseFile(t *testing.T, src string) (*ast.File, *token.FileSet) {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "sweep.go", src, 0)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	return file, fset
}

// sweepTypeCheck parses and fully type-checks a file.
func sweepTypeCheck(t *testing.T, src string) (*ast.File, *types.Info, *token.FileSet) {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "sweep.go", src, 0)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	info := &types.Info{
		Types:     map[ast.Expr]types.TypeAndValue{},
		Defs:      map[*ast.Ident]types.Object{},
		Uses:      map[*ast.Ident]types.Object{},
		Instances: map[*ast.Ident]types.Instance{},
	}
	conf := types.Config{Importer: importer.Default()}
	if _, err := conf.Check("p", fset, []*ast.File{file}, info); err != nil {
		t.Fatalf("typecheck: %v", err)
	}
	return file, info, fset
}

// sweepIdent builds an ident CallArgument with just a name.
func sweepIdent(m *Metadata, name string) *CallArgument {
	a := arg(m, KindIdent)
	a.SetName(name)
	return a
}

// sweepLit builds a literal CallArgument with a value.
func sweepLit(m *Metadata, value string) *CallArgument {
	a := arg(m, KindLiteral)
	a.SetValue(value)
	return a
}

// sweepCall builds a Call with all pool indices explicitly unset so a zero
// index never aliases whatever string happens to sit at pool slot 0.
func sweepCall(m *Metadata, name, pkg string) Call {
	return Call{
		Meta:         m,
		Name:         m.StringPool.Get(name),
		Pkg:          m.StringPool.Get(pkg),
		Position:     -1,
		RecvType:     -1,
		Scope:        -1,
		SignatureStr: -1,
	}
}

// --- expression.go ---

func TestSweepCallArgToStringFallbacks(t *testing.T) {
	m := sweepMeta()

	if got := CallArgToString(nil); got != "" {
		t.Errorf("nil arg: got %q", got)
	}

	cases := []struct {
		name  string
		build func() *CallArgument
		want  string
	}{
		{"selector no X", func() *CallArgument {
			a := arg(m, KindSelector)
			a.Sel = sweepIdent(m, "Sel")
			return a
		}, "Sel"},
		{"call no fun", func() *CallArgument { return arg(m, KindCall) }, "call()"},
		{"unary no X", func() *CallArgument {
			a := arg(m, KindUnary)
			a.SetValue("&")
			return a
		}, "&"},
		{"binary no operands", func() *CallArgument {
			a := arg(m, KindBinary)
			a.SetValue("+")
			return a
		}, "+"},
		{"index no operands", func() *CallArgument { return arg(m, KindIndex) }, "index"},
		{"index list with X", func() *CallArgument {
			a := arg(m, KindIndexList)
			a.X = sweepIdent(m, "G")
			a.Args = []*CallArgument{sweepIdent(m, "int"), sweepIdent(m, "string")}
			return a
		}, "G[int, string]"},
		{"index list no X", func() *CallArgument { return arg(m, KindIndexList) }, "index_list"},
		{"paren no X", func() *CallArgument { return arg(m, KindParen) }, "()"},
		{"star no X", func() *CallArgument { return arg(m, KindStar) }, "*"},
		{"array type no X", func() *CallArgument {
			a := arg(m, KindArrayType)
			a.SetValue("5")
			return a
		}, "[5]"},
		{"slice with bounds", func() *CallArgument {
			a := arg(m, KindSlice)
			a.X = sweepIdent(m, "xs")
			a.Args = []*CallArgument{sweepLit(m, "1"), sweepLit(m, "2")}
			return a
		}, "xs[1:2]"},
		{"slice no X", func() *CallArgument { return arg(m, KindSlice) }, "slice"},
		{"composite no X", func() *CallArgument { return arg(m, KindCompositeLit) }, "{}"},
		{"key value no X", func() *CallArgument { return arg(m, KindKeyValue) }, "key: value"},
		{"type assert full", func() *CallArgument {
			a := arg(m, KindTypeAssert)
			a.X = sweepIdent(m, "v")
			a.Fun = sweepIdent(m, "int")
			return a
		}, "v.(int)"},
		{"type assert no X", func() *CallArgument { return arg(m, KindTypeAssert) }, "type_assert"},
		{"chan with X", func() *CallArgument {
			a := arg(m, KindChanType)
			a.X = sweepIdent(m, "int")
			return a
		}, "chan int"},
		{"chan no X", func() *CallArgument { return arg(m, KindChanType) }, "chan"},
		{"map no X", func() *CallArgument { return arg(m, KindMapType) }, "map"},
		{"ellipsis no X", func() *CallArgument { return arg(m, KindEllipsis) }, "..."},
		{"func type no fun", func() *CallArgument { return arg(m, KindFuncType) }, "func()"},
	}
	for _, tc := range cases {
		if got := CallArgToString(tc.build()); got != tc.want {
			t.Errorf("%s: got %q want %q", tc.name, got, tc.want)
		}
	}
}

func TestSweepExprToCallArgumentArms(t *testing.T) {
	m := sweepMeta()
	fset := token.NewFileSet()

	// Type assertion dispatch arm.
	ta := ExprToCallArgument(sweepParseExpr(t, "v.(int)"), nil, "p", fset, m)
	if ta.GetKind() != KindTypeAssert {
		t.Errorf("type assert kind: got %q", ta.GetKind())
	}

	// Fallback arm for expressions with no dedicated handler.
	bad := ExprToCallArgument(&ast.BadExpr{}, nil, "p", fset, m)
	if bad.GetKind() != KindRaw {
		t.Errorf("bad expr kind: got %q", bad.GetKind())
	}

	// Array type with a declared length.
	at := ExprToCallArgument(sweepParseExpr(t, "[5]int"), nil, "p", fset, m)
	if at.GetKind() != KindArrayType || at.GetValue() != "5" {
		t.Errorf("array type: kind %q value %q", at.GetKind(), at.GetValue())
	}

	// Three-index slice expression (Max set).
	sl := ExprToCallArgument(sweepParseExpr(t, "xs[1:2:3]"), nil, "p", fset, m)
	if sl.GetKind() != KindSlice || len(sl.Args) != 3 {
		t.Errorf("slice expr: kind %q args %d", sl.GetKind(), len(sl.Args))
	}

	// Directional channels.
	send := ExprToCallArgument(sweepParseExpr(t, "chan<- int"), nil, "p", fset, m)
	if send.GetValue() != "send" {
		t.Errorf("send chan: got %q", send.GetValue())
	}
	recv := ExprToCallArgument(sweepParseExpr(t, "<-chan int"), nil, "p", fset, m)
	if recv.GetValue() != "recv" {
		t.Errorf("recv chan: got %q", recv.GetValue())
	}

	// Struct type with an embedded field.
	st := ExprToCallArgument(sweepParseExpr(t, "struct{ int; A string }"), nil, "p", fset, m)
	if st.GetKind() != KindStructType || len(st.Args) != 2 {
		t.Fatalf("struct type: kind %q fields %d", st.GetKind(), len(st.Args))
	}
	if st.Args[0].GetKind() != KindEmbed {
		t.Errorf("embedded field kind: got %q", st.Args[0].GetKind())
	}
}

func TestSweepHandleSelectorPkgName(t *testing.T) {
	m := sweepMeta()
	selIdent := ast.NewIdent("util")
	sel := &ast.SelectorExpr{X: ast.NewIdent("a"), Sel: selIdent}
	imported := types.NewPackage("example.com/util", "util")
	importing := types.NewPackage("example.com/app", "app")
	info := &types.Info{Uses: map[*ast.Ident]types.Object{
		selIdent: types.NewPkgName(token.NoPos, importing, "util", imported),
	}}

	res := ExprToCallArgument(sel, info, "app", token.NewFileSet(), m)
	if res.GetPkg() != "example.com/util" {
		t.Errorf("selector pkg-name object: got pkg %q", res.GetPkg())
	}
}

// --- analysis.go ---

func TestSweepIsTypeConversionArms(t *testing.T) {
	if isTypeConversion(&ast.CallExpr{Fun: ast.NewIdent("f")}, nil) {
		t.Error("nil info should never report a conversion")
	}

	emptyInfo := &types.Info{Types: map[ast.Expr]types.TypeAndValue{}}
	star := &ast.CallExpr{Fun: &ast.StarExpr{X: ast.NewIdent("T")}}
	if !isTypeConversion(star, emptyInfo) {
		t.Error("star-expr fun should be a conversion")
	}
	iface := &ast.CallExpr{Fun: &ast.InterfaceType{Methods: &ast.FieldList{}}}
	if !isTypeConversion(iface, emptyInfo) {
		t.Error("interface-type fun should be a conversion")
	}

	// Parenthesized conversion resolves through info.Types[fun].IsType().
	file, info, _ := sweepTypeCheck(t, "package p\n\ntype T int\n\nvar x = (T)(1)\n")
	var conv *ast.CallExpr
	ast.Inspect(file, func(n ast.Node) bool {
		if c, ok := n.(*ast.CallExpr); ok {
			conv = c
		}
		return true
	})
	if conv == nil {
		t.Fatal("conversion call not found")
	}
	if !isTypeConversion(conv, info) {
		t.Error("(T)(1) should be a conversion")
	}
}

func TestSweepGetCalleeFunctionNameAndPackageArms(t *testing.T) {
	fset := token.NewFileSet()
	file := &ast.File{}
	mkSel := func(x ast.Expr, name string) *ast.SelectorExpr {
		return &ast.SelectorExpr{X: x, Sel: ast.NewIdent(name)}
	}

	// Ident receiver typed as a bare interface.
	identX := ast.NewIdent("r")
	ifaceType := types.NewInterfaceType(nil, nil)
	ifaceType.Complete()
	infoVar := &types.Info{Uses: map[*ast.Ident]types.Object{
		identX: types.NewVar(token.NoPos, nil, "r", ifaceType),
	}}
	name, pkg, recv := getCalleeFunctionNameAndPackage(mkSel(identX, "Read"), file, "p",
		map[*ast.File]*types.Info{file: infoVar}, nil, fset)
	if name != "Read" || pkg != "p" || recv != "interface" {
		t.Errorf("iface var receiver: got (%q,%q,%q)", name, pkg, recv)
	}

	// Complex receiver whose type is a Named with a nil package (error).
	callX := &ast.CallExpr{Fun: ast.NewIdent("f")}
	infoErr := &types.Info{Types: map[ast.Expr]types.TypeAndValue{
		callX: {Type: types.Universe.Lookup("error").Type()},
	}}
	name, pkg, recv = getCalleeFunctionNameAndPackage(mkSel(callX, "Error"), file, "p",
		map[*ast.File]*types.Info{file: infoErr}, nil, fset)
	if name != "Error" || pkg != "p" || recv != "error" {
		t.Errorf("error receiver: got (%q,%q,%q)", name, pkg, recv)
	}

	// Complex receiver typed as a bare interface.
	callX2 := &ast.CallExpr{Fun: ast.NewIdent("g")}
	infoIface := &types.Info{Types: map[ast.Expr]types.TypeAndValue{
		callX2: {Type: ifaceType},
	}}
	name, pkg, recv = getCalleeFunctionNameAndPackage(mkSel(callX2, "Do"), file, "p",
		map[*ast.File]*types.Info{file: infoIface}, nil, fset)
	if name != "Do" || pkg != "p" || recv != "interface" {
		t.Errorf("iface complex receiver: got (%q,%q,%q)", name, pkg, recv)
	}

	// Complex receiver typed as a basic type: no switch arm matches.
	callX3 := &ast.CallExpr{Fun: ast.NewIdent("h")}
	infoBasic := &types.Info{Types: map[ast.Expr]types.TypeAndValue{
		callX3: {Type: types.Typ[types.Int]},
	}}
	name, pkg, recv = getCalleeFunctionNameAndPackage(mkSel(callX3, "Do"), file, "p",
		map[*ast.File]*types.Info{file: infoBasic}, nil, fset)
	if name != "Do" || pkg != "p" || recv != "" {
		t.Errorf("basic receiver fallback: got (%q,%q,%q)", name, pkg, recv)
	}

	// CallExpr recurses into its Fun.
	name, _, _ = getCalleeFunctionNameAndPackage(&ast.CallExpr{Fun: ast.NewIdent("mk")}, file, "p", nil, nil, fset)
	if name != "mk" {
		t.Errorf("call expr recursion: got %q", name)
	}

	// IndexListExpr recurses into X.
	name, _, _ = getCalleeFunctionNameAndPackage(&ast.IndexListExpr{X: ast.NewIdent("gen")}, file, "p", nil, nil, fset)
	if name != "gen" {
		t.Errorf("index list recursion: got %q", name)
	}

	// Unhandled expression kinds resolve to nothing.
	name, pkg, recv = getCalleeFunctionNameAndPackage(&ast.BasicLit{Kind: token.INT, Value: "1"}, file, "p", nil, nil, fset)
	if name != "" || pkg != "" || recv != "" {
		t.Errorf("unhandled expr: got (%q,%q,%q)", name, pkg, recv)
	}
}

func TestSweepFindParentFunctionTopLevelLit(t *testing.T) {
	m := sweepMeta()
	file, fset := sweepParseFile(t, "package p\n\nvar f = func() int { return 1 }\n")
	var lit *ast.FuncLit
	ast.Inspect(file, func(n ast.Node) bool {
		if fl, ok := n.(*ast.FuncLit); ok {
			lit = fl
		}
		return true
	})
	if lit == nil {
		t.Fatal("func lit not found")
	}
	name, parts, sig := findParentFunction(file, lit.Body.Lbrace+1, &types.Info{}, fset, m)
	if name != "" || parts != "" || sig != "" {
		t.Errorf("top-level func lit has no parent, got (%q,%q,%q)", name, parts, sig)
	}
}

func TestSweepAnalyzeAssignmentValueFallbacks(t *testing.T) {
	m := sweepMeta()
	fset := token.NewFileSet()

	// Selector whose base is not a plain identifier.
	selExpr := &ast.SelectorExpr{X: &ast.CallExpr{Fun: ast.NewIdent("f")}, Sel: ast.NewIdent("Field")}
	pkg, argOut := analyzeAssignmentValue(selExpr, nil, "h", "p", m, fset)
	if pkg != "p" || argOut == nil {
		t.Errorf("selector fallback: pkg %q arg %v", pkg, argOut)
	}

	// Call whose Fun is neither ident nor ident-based selector. A non-nil
	// (empty) info is required: the fallback still runs ExprToCallArgument,
	// which reaches extractParamsAndTypeParams and dereferences info for the
	// IndexExpr fun. Real callers always pass a populated info here.
	callExpr := sweepParseExpr(t, "fns[0]()")
	pkg, argOut = analyzeAssignmentValue(callExpr, &types.Info{}, "h", "p", m, fset)
	if pkg != "p" || argOut == nil {
		t.Errorf("call fallback: pkg %q arg %v", pkg, argOut)
	}

	// Type assertion with no asserted type (switch guard form).
	taExpr := &ast.TypeAssertExpr{X: ast.NewIdent("v")}
	pkg, argOut = analyzeAssignmentValue(taExpr, nil, "h", "p", m, fset)
	if pkg != "p" || argOut == nil || argOut.GetType() != "interface{}" {
		t.Errorf("type assert fallback: pkg %q type %q", pkg, argOut.GetType())
	}
}

func TestSweepTraceVariableOriginTypeParam(t *testing.T) {
	m := sweepMeta()
	edge := CallGraphEdge{TypeParamMap: map[string]string{"T": "app.User"}}
	edge.Caller = sweepCall(m, "main", "app")
	edge.Callee = sweepCall(m, "Handle", "app")
	m.CallGraph = []CallGraphEdge{edge}

	ov, op, ot, caller := TraceVariableOrigin("T", "Handle", "app", m)
	if ov != "T" || op != "app" || caller != "main" {
		t.Errorf("got (%q,%q,%q)", ov, op, caller)
	}
	if ot == nil || ot.GetType() != "app.User" {
		t.Fatalf("expected concrete type app.User, got %v", ot)
	}
}

func TestSweepTraceVariableOriginReturnUnwrap(t *testing.T) {
	m := sweepMeta()
	sp := m.StringPool

	// Function-path callee whose return value is a selector wrapping an ident.
	selRet := arg(m, KindSelector)
	selRet.Sel = sweepIdent(m, "res")
	fnH := &Function{Name: sp.Get("h"), AssignmentMap: map[string][]Assignment{
		"v": {{VariableName: sp.Get("v"), CalleeFunc: "mk", CalleePkg: "app", Value: *sweepLit(m, "0")}},
	}}
	fnMk := &Function{Name: sp.Get("mk"), ReturnVars: []CallArgument{*selRet}}
	m.Packages["app"] = &Package{Files: map[string]*File{
		"a.go": {Functions: map[string]*Function{"h": fnH, "mk": fnMk}},
	}}

	ov, op, _, caller := TraceVariableOrigin("v", "h", "app", m)
	if ov != "res" || op != "app" || caller != "mk" {
		t.Errorf("function selector unwrap: got (%q,%q,%q)", ov, op, caller)
	}

	// Method-path callees: unary→literal (break + literal return) and
	// selector→ident (recursive trace).
	unaryRet := arg(m, KindUnary)
	unaryRet.X = sweepLit(m, "1")
	selRet2 := arg(m, KindSelector)
	selRet2.Sel = sweepIdent(m, "z")
	typS := &Type{Name: sp.Get("S"), Methods: []Method{
		{Name: sp.Get("mval"), ReturnVars: []CallArgument{*unaryRet}},
		{Name: sp.Get("msel"), ReturnVars: []CallArgument{*selRet2}},
	}}
	fnH2 := &Function{Name: sp.Get("h2"), AssignmentMap: map[string][]Assignment{
		"w":  {{VariableName: sp.Get("w"), CalleeFunc: "mval", CalleePkg: "app2", Value: *sweepLit(m, "0")}},
		"w2": {{VariableName: sp.Get("w2"), CalleeFunc: "msel", CalleePkg: "app2", Value: *sweepLit(m, "0")}},
	}}
	m.Packages["app2"] = &Package{Files: map[string]*File{
		"b.go": {Functions: map[string]*Function{"h2": fnH2}, Types: map[string]*Type{"S": typS}},
	}}

	_, op, ot, _ := TraceVariableOrigin("w", "h2", "app2", m)
	if op != "app2" || ot == nil || ot.GetKind() != KindLiteral {
		t.Errorf("method literal return: pkg %q origin %v", op, ot)
	}
	ov, op, _, caller = TraceVariableOrigin("w2", "h2", "app2", m)
	if ov != "z" || op != "app2" || caller != "msel" {
		t.Errorf("method selector unwrap: got (%q,%q,%q)", ov, op, caller)
	}
}

// --- types.go ---

func TestSweepStringPoolGuards(t *testing.T) {
	var zero StringPool
	if got := zero.Get("x"); got != -1 {
		t.Errorf("zero-value pool Get: got %d", got)
	}

	sp := NewStringPool()
	wantErr := errors.New("boom")
	if err := sp.UnmarshalYAML(func(interface{}) error { return wantErr }); !errors.Is(err, wantErr) {
		t.Errorf("UnmarshalYAML error passthrough: got %v", err)
	}
}

func TestSweepExtractGenericTypesPlainList(t *testing.T) {
	got := ExtractGenericTypes("pkg.F[int,string]")
	if len(got) != 2 || got[0] != "int" || got[1] != "string" {
		t.Errorf("got %v", got)
	}
}

func TestSweepTraverseCallerChildrenSelfLoop(t *testing.T) {
	m := sweepMeta()
	edgeAB := CallGraphEdge{Caller: sweepCall(m, "a", "p"), Callee: sweepCall(m, "b", "p")}
	edgeBB := CallGraphEdge{Caller: sweepCall(m, "b", "p"), Callee: sweepCall(m, "b", "p")}
	m.CallGraph = []CallGraphEdge{edgeAB, edgeBB}
	m.BuildCallGraphMaps()

	var visits int
	m.TraverseCallerChildren(&m.CallGraph[0], func(parent, child *CallGraphEdge) { visits++ })
	if visits != 1 {
		t.Errorf("self loop should be visited once, got %d", visits)
	}

	// At the self-calling depth cap the child edge is skipped entirely.
	m.callDepth[m.CallGraph[0].Callee.BaseID()] = MaxSelfCallingDepth
	visits = 0
	m.TraverseCallerChildren(&m.CallGraph[0], func(parent, child *CallGraphEdge) { visits++ })
	if visits != 0 {
		t.Errorf("capped self loop should be skipped, got %d visits", visits)
	}
}

func TestSweepCallArgumentGuards(t *testing.T) {
	m := sweepMeta()

	c := &Call{Scope: -1, Meta: m}
	if got := c.GetScope(); got != "" {
		t.Errorf("negative scope: got %q", got)
	}

	a := arg(m, KindIdent)
	if got := a.GetGenericTypeName(); got != "" {
		t.Errorf("unset generic type name: got %q", got)
	}

	defer func() {
		if recover() == nil {
			t.Error("NewCallArgument(nil) should panic")
		}
	}()
	NewCallArgument(nil)
}

func TestSweepCallArgumentIDArms(t *testing.T) {
	m := sweepMeta()

	withTParams := func(a *CallArgument) *CallArgument {
		a.TypeParamMap = map[string]string{"T": "int"}
		return a
	}
	emptyIdent := func() *CallArgument { return arg(m, KindIdent) }

	// Selector: X's type-param suffix propagates to the selector ID.
	sel := arg(m, KindSelector)
	sel.X = withTParams(sweepIdent(m, "recv"))
	sel.Sel = sweepIdent(m, "M")
	if got := sel.ID(); !strings.Contains(got, "[T=int]") {
		t.Errorf("selector tparam propagation: got %q", got)
	}

	// Call with no Fun falls back to the kind label.
	if got := arg(m, KindCall).ID(); got != KindCall {
		t.Errorf("call no fun: got %q", got)
	}
	// Call with a generic Fun propagates its type params.
	call := arg(m, KindCall)
	call.Fun = withTParams(sweepIdent(m, "F"))
	if got := call.ID(); !strings.Contains(got, "[T=int]") {
		t.Errorf("call tparam propagation: got %q", got)
	}

	// Type conversion, with and without Fun.
	conv := arg(m, KindTypeConversion)
	conv.Fun = sweepIdent(m, "T")
	if got := conv.ID(); !strings.Contains(got, "T") {
		t.Errorf("conversion with fun: got %q", got)
	}
	if got := arg(m, KindTypeConversion).ID(); got != KindTypeConversion {
		t.Errorf("conversion without fun: got %q", got)
	}

	// Unary: empty X ID falls back to the arg's package; tparams propagate;
	// missing X yields an empty ID.
	un := arg(m, KindUnary)
	un.SetValue("&")
	un.SetPkg("pk")
	un.X = emptyIdent()
	if got := un.ID(); got != "pk" { // leading "&" is trimmed by ID()
		t.Errorf("unary pkg fallback: got %q", got)
	}
	un2 := arg(m, KindUnary)
	un2.SetValue("&")
	un2.X = withTParams(sweepIdent(m, "v"))
	if got := un2.ID(); !strings.Contains(got, "[T=int]") {
		t.Errorf("unary tparam propagation: got %q", got)
	}
	if got := arg(m, KindUnary).ID(); got != "" {
		t.Errorf("unary without X: got %q", got)
	}

	// Composite literal: same three shapes.
	cl := arg(m, KindCompositeLit)
	cl.X = withTParams(sweepIdent(m, "User"))
	if got := cl.ID(); !strings.Contains(got, "[T=int]") {
		t.Errorf("composite tparam propagation: got %q", got)
	}
	if got := arg(m, KindCompositeLit).ID(); got != "" {
		t.Errorf("composite without X: got %q", got)
	}

	// Index: pkg fallback, tparam propagation, and missing X.
	ix := arg(m, KindIndex)
	ix.SetPkg("pk")
	ix.X = emptyIdent()
	if got := ix.ID(); got != "pk" {
		t.Errorf("index pkg fallback: got %q", got)
	}
	ix2 := arg(m, KindIndex)
	ix2.X = withTParams(sweepIdent(m, "xs"))
	if got := ix2.ID(); !strings.Contains(got, "[T=int]") {
		t.Errorf("index tparam propagation: got %q", got)
	}
	if got := arg(m, KindIndex).ID(); got != "" {
		t.Errorf("index without X: got %q", got)
	}
}

func TestSweepCallBuildIdentifierNilMeta(t *testing.T) {
	c := &Call{}
	if got := c.BaseID(); got != "." {
		t.Errorf("nil-meta BaseID: got %q", got)
	}
}

func TestSweepExtractReturnTypeFromSignatureCallKind(t *testing.T) {
	m := sweepMeta()
	sig := arg(m, KindCall)
	fun := sweepIdent(m, "mk")
	fun.SetType("User")
	sig.Fun = fun
	if got := m.extractReturnTypeFromSignature(*sig); got != "User" {
		t.Errorf("got %q", got)
	}
}

func TestSweepExtractTypeFromCallArgumentArms(t *testing.T) {
	m := sweepMeta()

	mkTyped := func(kind, typ string) *CallArgument {
		a := arg(m, kind)
		a.SetType(typ)
		return a
	}

	cases := []struct {
		name  string
		build func() *CallArgument
		want  string
	}{
		{"call no fun", func() *CallArgument { return arg(m, KindCall) }, "func()"},
		{"composite no X", func() *CallArgument { return mkTyped(KindCompositeLit, "User") }, "User"},
		{"unary no X", func() *CallArgument { return mkTyped(KindUnary, "User") }, "User"},
		{"literal", func() *CallArgument { return mkTyped(KindLiteral, "string") }, "string"},
		{"sized array", func() *CallArgument {
			a := arg(m, KindArrayType)
			a.SetValue("5")
			a.X = mkTyped(KindLiteral, "int")
			return a
		}, "[5]int"},
		{"array no X", func() *CallArgument { return mkTyped(KindArrayType, "int") }, "[]int"},
		{"slice with X", func() *CallArgument {
			a := arg(m, KindSlice)
			a.X = mkTyped(KindLiteral, "int")
			return a
		}, "[]int"},
		{"slice no X", func() *CallArgument { return mkTyped(KindSlice, "int") }, "[]int"},
		{"map fallback type", func() *CallArgument { return mkTyped(KindMapType, "map[string]int") }, "map[string]int"},
		{"star no X", func() *CallArgument { return mkTyped(KindStar, "User") }, "*User"},
	}
	for _, tc := range cases {
		if got := m.extractTypeFromCallArgument(tc.build()); got != tc.want {
			t.Errorf("%s: got %q want %q", tc.name, got, tc.want)
		}
	}
}

func TestSweepResolveUnaryReturnTypeDeref(t *testing.T) {
	m := sweepMeta()
	rv := arg(m, KindUnary)
	rv.SetType("*User")
	x := arg(m, KindLiteral)
	x.SetType("*User")
	rv.X = x
	if got := m.resolveUnaryReturnType(rv, "p"); got != "User" {
		t.Errorf("deref: got %q", got)
	}
}

func TestSweepProcessFunctionCallReturnTypeCallKind(t *testing.T) {
	m := sweepMeta()
	sp := m.StringPool

	sig := arg(m, KindFuncType)
	sig.ResolvedType = sp.Get("User")
	m.Packages["app"] = &Package{Files: map[string]*File{
		"a.go": {Functions: map[string]*Function{"mk": {Name: sp.Get("mk"), Signature: *sig}}},
	}}

	a := arg(m, KindCall)
	a.SetName("mk")
	a.SetPkg("app")
	a.Fun = arg(m, KindCall) // nested-call shape: name/pkg read from the outer arg
	m.processFunctionCallReturnType(a)
	if got := a.GetResolvedType(); got != "User" {
		t.Errorf("resolved type: got %q", got)
	}
}

func TestSweepGetInterfaceResolutionFound(t *testing.T) {
	m := sweepMeta()
	m.RegisterInterfaceResolution("Iface", "Struct", "p", "Concrete", "pos")
	got, ok := m.GetInterfaceResolution("Iface", "Struct", "p")
	if !ok || got != "Concrete" {
		t.Errorf("got (%q,%v)", got, ok)
	}
}

// --- metadata.go ---

func TestSweepCallIdentifierIDGenericAndDefault(t *testing.T) {
	ci := NewCallIdentifier("p", "f", "", "1:2", map[string]string{"T": "int"})
	if got := ci.ID(GenericID); got != "p.f[T=int]" {
		t.Errorf("generic ID: got %q", got)
	}
	plain := NewCallIdentifier("p", "f", "", "", nil)
	if got := plain.ID(GenericID); got != "p.f" {
		t.Errorf("generic ID without generics: got %q", got)
	}
	if got := plain.ID(CallIdentifierType(99)); got != "p.f" {
		t.Errorf("unknown ID type: got %q", got)
	}
}

func TestSweepGenerateMetadataModulePathInference(t *testing.T) {
	fset := token.NewFileSet()
	empty := map[string]map[string]*ast.File{}
	noInfo := map[*ast.File]*types.Info{}

	// No common prefix: the shorter dotted path wins via its module part.
	md := GenerateMetadata(empty, noInfo, map[string]string{
		"a.go": "zzz/bar/baz/quxx/more",
		"b.go": "a.com/internal/x",
	}, fset)
	if md.CurrentModulePath != "a.com" {
		t.Errorf("module part inference: got %q", md.CurrentModulePath)
	}

	// Empty first path: the dotted path is adopted wholesale.
	md = GenerateMetadata(empty, noInfo, map[string]string{
		"a.go": "",
		"b.go": "a.com/x",
	}, fset)
	if md.CurrentModulePath != "a.com/x" {
		t.Errorf("whole-path fallback: got %q", md.CurrentModulePath)
	}
}

func TestSweepGenerateMetadataSkipsMockReceivers(t *testing.T) {
	src := "package p\n\ntype MockSvc struct{}\n\nfunc (m MockSvc) Do() {}\n\ntype Svc struct{}\n\nfunc (s Svc) Run() {}\n"
	file, info, fset := sweepTypeCheck(t, src)
	md := GenerateMetadata(
		map[string]map[string]*ast.File{"p": {"p.go": file}},
		map[*ast.File]*types.Info{file: info},
		map[string]string{"p.go": "p"},
		fset,
	)
	pkg := md.Packages["p"]
	if pkg == nil {
		t.Fatal("package p missing")
	}
	var sawSvcRun, sawMock bool
	for _, f := range pkg.Files {
		for name, typ := range f.Types {
			if strings.Contains(strings.ToLower(name), "mock") {
				sawMock = true
			}
			for _, meth := range typ.Methods {
				if md.StringPool.GetString(meth.Name) == "Run" {
					sawSvcRun = true
				}
			}
		}
	}
	if !sawSvcRun {
		t.Error("Svc.Run not recorded")
	}
	if sawMock {
		t.Error("mock type should have been skipped")
	}
}

func TestSweepBuildFuncMapEmptyPkgName(t *testing.T) {
	fd := &ast.FuncDecl{Name: ast.NewIdent("F"), Type: &ast.FuncType{}}
	file := &ast.File{Name: ast.NewIdent(""), Decls: []ast.Decl{fd}}
	fm := BuildFuncMap(map[string]map[string]*ast.File{"p": {"a.go": file}})
	if fm["F"] == nil {
		t.Errorf("expected bare key for empty package name, got keys %v", fm)
	}
}

func TestSweepCollectConstantsNonValueSpec(t *testing.T) {
	m := sweepMeta()
	gd := &ast.GenDecl{Tok: token.CONST, Specs: []ast.Spec{
		&ast.TypeSpec{Name: ast.NewIdent("T"), Type: ast.NewIdent("int")},
	}}
	file := &ast.File{Name: ast.NewIdent("p"), Decls: []ast.Decl{gd}}
	if got := collectConstants(file, nil, "p", token.NewFileSet(), m); len(got) != 0 {
		t.Errorf("expected no constants, got %v", got)
	}
}

func TestSweepProcessTypeSpecSkips(t *testing.T) {
	m := sweepMeta()
	f := &File{Types: map[string]*Type{"User": {}}}

	processTypeSpec(&ast.TypeSpec{Name: ast.NewIdent("MockUser")}, nil, "p", nil, f, nil, nil, m, false)
	processTypeSpec(&ast.TypeSpec{Name: ast.NewIdent("User")}, nil, "p", nil, f, nil, nil, m, true)
	if len(f.Types) != 1 {
		t.Errorf("both specs should be skipped, got %d types", len(f.Types))
	}
}

func TestSweepIsMarshalerSignatureGuards(t *testing.T) {
	withParam := types.NewSignatureType(nil, nil, nil,
		types.NewTuple(types.NewParam(token.NoPos, nil, "x", types.Typ[types.Int])), nil, false)
	if isMarshalerSignature(withParam) {
		t.Error("signature with params is not a marshaler")
	}
	oneResult := types.NewSignatureType(nil, nil, nil, nil,
		types.NewTuple(types.NewParam(token.NoPos, nil, "", types.Typ[types.Bool])), false)
	if isMarshalerSignature(oneResult) {
		t.Error("signature with one result is not a marshaler")
	}
}

func TestSweepRecordExternalTypeFactsArms(t *testing.T) {
	m := &Metadata{StringPool: NewStringPool(), CurrentModulePath: "example.com/me"}

	// Named type with a nil package (error) is ignored and allocates nothing.
	recordExternalTypeFacts(types.Universe.Lookup("error").Type(), m)
	if m.ExternalTypes != nil {
		t.Fatal("error type must not allocate the facts map")
	}

	// A channel of an external named type records the element type and lazily
	// allocates the map.
	extPkg := types.NewPackage("github.com/ext/lib", "lib")
	named := types.NewNamed(types.NewTypeName(token.NoPos, extPkg, "T", nil), types.Typ[types.String], nil)
	recordExternalTypeFacts(types.NewChan(types.SendRecv, named), m)
	if _, ok := m.ExternalTypes["github.com/ext/lib.T"]; !ok {
		t.Errorf("chan element fact missing: %v", m.ExternalTypes)
	}

	// Recursive external type (type R []R) terminates via the cycle guard.
	rec := types.NewNamed(types.NewTypeName(token.NoPos, extPkg, "R", nil), nil, nil)
	rec.SetUnderlying(types.NewSlice(rec))
	recordExternalTypeFacts(rec, m)
	if _, ok := m.ExternalTypes["lib.R"]; !ok {
		t.Errorf("recursive type fact missing: %v", m.ExternalTypes)
	}
}

func TestSweepAnonStructKeyGuards(t *testing.T) {
	fset := token.NewFileSet()
	if got := AnonStructKey(token.NoPos, fset, "p"); got != "" {
		t.Errorf("NoPos: got %q", got)
	}
	if got := AnonStructKey(token.Pos(1), nil, "p"); got != "" {
		t.Errorf("nil fset: got %q", got)
	}
	if got := AnonStructKey(token.Pos(999999), fset, "p"); got != "" {
		t.Errorf("unregistered pos: got %q", got)
	}
}

func TestSweepProcessLocalAnonymousStructsGuards(t *testing.T) {
	m := sweepMeta()
	fset := token.NewFileSet()
	tf := fset.AddFile("x.go", -1, 100)
	pos := tf.Pos(5)
	st := types.NewStruct(nil, nil)

	info := &types.Info{Defs: map[*ast.Ident]types.Object{
		ast.NewIdent("a"): types.NewVar(token.NoPos, nil, "a", st), // invalid pos → empty key
		ast.NewIdent("b"): types.NewVar(pos, nil, "b", st),
		ast.NewIdent("c"): types.NewVar(pos, nil, "c", st), // duplicate key
	}}
	f := &File{Types: map[string]*Type{}}
	processLocalAnonymousStructs(&ast.File{}, info, "p", fset, f, m)
	if len(f.Types) != 1 {
		t.Errorf("expected exactly one anon struct type, got %d", len(f.Types))
	}
}

func TestSweepProcessStructFieldsEmbedded(t *testing.T) {
	m := sweepMeta()
	st, ok := sweepParseExpr(t, "struct{ int; A string }").(*ast.StructType)
	if !ok {
		t.Fatal("not a struct type")
	}
	typ := &Type{}
	processStructFields(st, "p", m, typ, nil)
	if len(typ.Embeds) != 1 || len(typ.Fields) != 1 {
		t.Errorf("embeds %d fields %d", len(typ.Embeds), len(typ.Fields))
	}
}

func TestSweepProcessFunctionsMockAndConstDecl(t *testing.T) {
	m := sweepMeta()
	file, fset := sweepParseFile(t, "package p\n\nfunc MockThing() {}\n\nfunc realFn() { const c = 1 }\n")
	f := &File{Functions: map[string]*Function{}}
	processFunctions(file, nil, "p", fset, f, nil, nil, m)
	if _, ok := f.Functions["MockThing"]; ok {
		t.Error("mock function should be skipped")
	}
	if _, ok := f.Functions["realFn"]; !ok {
		t.Error("realFn should be recorded")
	}
}

func TestSweepProcessVariablesNonValueSpec(t *testing.T) {
	m := sweepMeta()
	gd := &ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{
		&ast.TypeSpec{Name: ast.NewIdent("T"), Type: ast.NewIdent("int")},
	}}
	file := &ast.File{Name: ast.NewIdent("p"), Decls: []ast.Decl{gd}}
	f := &File{Variables: map[string]*Variable{}}
	processVariables(file, nil, "p", token.NewFileSet(), f, m)
	if len(f.Variables) != 0 {
		t.Errorf("expected no variables, got %d", len(f.Variables))
	}
}

func TestSweepProcessAssignmentArms(t *testing.T) {
	m := sweepMeta()
	fset := token.NewFileSet()

	// More RHS than LHS with no enclosing function: nothing is recorded.
	assign := &ast.AssignStmt{
		Lhs: []ast.Expr{ast.NewIdent("x")},
		Rhs: []ast.Expr{
			&ast.BasicLit{Kind: token.INT, Value: "1"},
			&ast.BasicLit{Kind: token.INT, Value: "2"},
		},
		Tok: token.ASSIGN,
	}
	if got := processAssignment(assign, &ast.File{Name: ast.NewIdent("p")}, nil, "p", fset, nil, nil, m); len(got) != 0 {
		t.Errorf("expected no assignments, got %d", len(got))
	}

	// Star-expression LHS lands in the raw fallback arm.
	file, fset2 := sweepParseFile(t, "package p\n\nfunc h() { *gp = 5 }\n")
	var starAssign *ast.AssignStmt
	ast.Inspect(file, func(n ast.Node) bool {
		if a, ok := n.(*ast.AssignStmt); ok {
			starAssign = a
		}
		return true
	})
	if starAssign == nil {
		t.Fatal("assignment not found")
	}
	got := processAssignment(starAssign, file, nil, "p", fset2, nil, nil, m)
	if len(got) != 1 || m.StringPool.GetString(got[0].ConcreteType) != "raw" {
		t.Fatalf("raw fallback: got %+v", got)
	}
}

func TestSweepGetTypeWithGenericsParen(t *testing.T) {
	identV := ast.NewIdent("v")
	paren := &ast.ParenExpr{X: identV}
	info := &types.Info{
		Uses:  map[*ast.Ident]types.Object{identV: types.NewVar(token.NoPos, nil, "v", types.Typ[types.Int])},
		Types: map[ast.Expr]types.TypeAndValue{},
	}
	typ := getTypeWithGenerics(paren, info)
	if typ == nil || typ.String() != "int" {
		t.Errorf("got %v", typ)
	}
}

func TestSweepProcessCallExpressionSkipsMock(t *testing.T) {
	m := sweepMeta()
	file, fset := sweepParseFile(t, "package p\n\nfunc h() { MockDo() }\n")
	var call *ast.CallExpr
	ast.Inspect(file, func(n ast.Node) bool {
		if c, ok := n.(*ast.CallExpr); ok {
			call = c
		}
		return true
	})
	if call == nil {
		t.Fatal("call not found")
	}
	processCallExpression(call, file, nil, "p", nil, map[*ast.File]*types.Info{}, nil, fset, m, nil, nil, nil)
	if len(m.CallGraph) != 0 {
		t.Errorf("mock callee must not create edges, got %d", len(m.CallGraph))
	}
}

func TestSweepAstFileFromFnReceiverFuncFallback(t *testing.T) {
	m := sweepMeta()
	astF := &ast.File{}
	m.Packages["p"] = &Package{Files: map[string]*File{
		"a.go": {Functions: map[string]*Function{"Do": {}}},
	}}
	pkgs := map[string]map[string]*ast.File{"p": {"a.go": astF}}
	if got := astFileFromFn("p", "Do", "*Recv", pkgs, m); got != astF {
		t.Errorf("receiver lookup should fall back to the same-named function's file")
	}
}

// sweepGenericFunc builds a *types.Func with the given type-parameter names,
// each also used as the type of one positional parameter.
func sweepGenericFunc(name string, tparamNames ...string) *types.Func {
	anyType := types.Universe.Lookup("any").Type()
	tparams := make([]*types.TypeParam, len(tparamNames))
	params := make([]*types.Var, len(tparamNames))
	for i, tn := range tparamNames {
		tparams[i] = types.NewTypeParam(types.NewTypeName(token.NoPos, nil, tn, nil), anyType)
		params[i] = types.NewParam(token.NoPos, nil, "p"+tn, tparams[i])
	}
	sig := types.NewSignatureType(nil, nil, tparams, types.NewTuple(params...), nil, false)
	return types.NewFunc(token.NoPos, nil, name, sig)
}

func TestSweepExtractParamsAndTypeParams(t *testing.T) {
	m := sweepMeta()

	run := func(call *ast.CallExpr, info *types.Info, args []*CallArgument) (map[string]CallArgument, map[string]string) {
		paramArgMap := map[string]CallArgument{}
		typeParamMap := map[string]string{}
		extractParamsAndTypeParams(call, info, args, paramArgMap, typeParamMap)
		return paramArgMap, typeParamMap
	}

	t.Run("index expr with selector base", func(t *testing.T) {
		selSel := ast.NewIdent("Foo")
		call := &ast.CallExpr{
			Fun: &ast.IndexExpr{
				X:     &ast.SelectorExpr{X: ast.NewIdent("ext"), Sel: selSel},
				Index: ast.NewIdent("int"),
			},
			Args: []ast.Expr{ast.NewIdent("v")},
		}
		info := &types.Info{Uses: map[*ast.Ident]types.Object{selSel: sweepGenericFunc("Foo", "T")}}
		pam, tpm := run(call, info, []*CallArgument{arg(m, KindIdent)})
		if tpm["T"] != "int" {
			t.Errorf("tpm: %v", tpm)
		}
		if _, ok := pam["pT"]; !ok {
			t.Errorf("pam: %v", pam)
		}
	})

	t.Run("index list expr ident base", func(t *testing.T) {
		funX := ast.NewIdent("Bar")
		call := &ast.CallExpr{
			Fun: &ast.IndexListExpr{
				X:       funX,
				Indices: []ast.Expr{ast.NewIdent("int"), ast.NewIdent("string")},
			},
			Args: []ast.Expr{ast.NewIdent("a"), ast.NewIdent("b")},
		}
		info := &types.Info{Uses: map[*ast.Ident]types.Object{funX: sweepGenericFunc("Bar", "T", "U")}}
		_, tpm := run(call, info, []*CallArgument{arg(m, KindIdent), arg(m, KindIdent)})
		if tpm["T"] != "int" || tpm["U"] != "string" {
			t.Errorf("tpm: %v", tpm)
		}
	})

	t.Run("index list expr selector base", func(t *testing.T) {
		selSel := ast.NewIdent("Baz")
		call := &ast.CallExpr{
			Fun: &ast.IndexListExpr{
				X:       &ast.SelectorExpr{X: ast.NewIdent("ext"), Sel: selSel},
				Indices: []ast.Expr{ast.NewIdent("int"), ast.NewIdent("string")},
			},
		}
		info := &types.Info{Uses: map[*ast.Ident]types.Object{selSel: sweepGenericFunc("Baz", "T", "U")}}
		_, tpm := run(call, info, nil)
		if tpm["T"] != "int" || tpm["U"] != "string" {
			t.Errorf("tpm: %v", tpm)
		}
	})

	t.Run("inference from generic function argument", func(t *testing.T) {
		funIdent := ast.NewIdent("Generic")
		argIdent := ast.NewIdent("h")
		call := &ast.CallExpr{Fun: funIdent, Args: []ast.Expr{argIdent}}
		anyType := types.Universe.Lookup("any").Type()
		argTP := types.NewTypeParam(types.NewTypeName(token.NoPos, nil, "A", nil), anyType)
		argSig := types.NewSignatureType(nil, nil, []*types.TypeParam{argTP},
			types.NewTuple(types.NewParam(token.NoPos, nil, "x", argTP)), nil, false)
		info := &types.Info{
			Uses:      map[*ast.Ident]types.Object{funIdent: sweepGenericFunc("Generic", "T")},
			Types:     map[ast.Expr]types.TypeAndValue{argIdent: {Type: argSig}},
			Instances: map[*ast.Ident]types.Instance{},
		}
		_, tpm := run(call, info, []*CallArgument{arg(m, KindIdent)})
		if len(tpm) == 0 {
			t.Errorf("expected an inferred entry, got %v", tpm)
		}
	})

	t.Run("inference from plain function argument", func(t *testing.T) {
		funIdent := ast.NewIdent("Generic")
		argIdent := ast.NewIdent("h")
		call := &ast.CallExpr{Fun: funIdent, Args: []ast.Expr{argIdent}}
		plainSig := types.NewSignatureType(nil, nil, nil,
			types.NewTuple(types.NewParam(token.NoPos, nil, "x", types.Typ[types.Int])), nil, false)
		info := &types.Info{
			Uses:      map[*ast.Ident]types.Object{funIdent: sweepGenericFunc("Generic", "T")},
			Types:     map[ast.Expr]types.TypeAndValue{argIdent: {Type: plainSig}},
			Instances: map[*ast.Ident]types.Instance{},
		}
		pam, _ := run(call, info, []*CallArgument{arg(m, KindIdent)})
		if _, ok := pam["pT"]; !ok {
			t.Errorf("pam: %v", pam)
		}
	})
}

func TestSweepApplyTypeParameterResolutionNil(t *testing.T) {
	applyTypeParameterResolution(nil)                     // must not panic
	applyTypeParameterResolutionToArgument(nil, nil, nil) // must not panic
}

// --- dependency_analyzer.go ---

// sweepPkg builds a *packages.Package whose Syntax is one parsed file with the
// given import paths.
func sweepPkg(t *testing.T, pkgPath, pkgName string, imports ...string) *packages.Package {
	t.Helper()
	var b strings.Builder
	b.WriteString("package " + pkgName + "\n")
	for _, imp := range imports {
		b.WriteString("import _ \"" + imp + "\"\n")
	}
	file, _ := sweepParseFile(t, b.String())
	return &packages.Package{PkgPath: pkgPath, Name: pkgName, Syntax: []*ast.File{file}}
}

func TestSweepFrameworkDetectorAnalyze(t *testing.T) {
	app := sweepPkg(t, "myapp-svc/app", "app",
		"github.com/gin-gonic/gin", "myapp-svc/models", "myapp-svc/utilmock")
	// An empty import path is skipped by the dependency-graph builder.
	app.Syntax[0].Imports = append(app.Syntax[0].Imports, &ast.ImportSpec{Path: &ast.BasicLit{Kind: token.STRING, Value: `""`}})

	models := sweepPkg(t, "myapp-svc/models", "models", "myapp-svc/util")
	util := sweepPkg(t, "myapp-svc/util", "util")
	mid := sweepPkg(t, "myapp-svc/mid", "mid", "myapp-svc/app")
	dep2 := sweepPkg(t, "myapp-svc/dep2", "dep2", "myapp-svc/mid")
	mocks := sweepPkg(t, "myapp-svc/mocks", "mocks", "myapp-svc/app")
	pkgs := []*packages.Package{app, models, util, mid, dep2, mocks}

	fd := NewFrameworkDetector()
	list, err := fd.AnalyzeFrameworkDependencies(pkgs, nil, nil, nil)
	if err != nil {
		t.Fatalf("analyze: %v", err)
	}

	byPath := map[string]*FrameworkDependency{}
	for _, dep := range list.AllPackages {
		byPath[dep.PackagePath] = dep
	}
	if d := byPath["myapp-svc/app"]; d == nil || d.FrameworkType != "gin" || !d.IsDirect {
		t.Errorf("app: %+v", byPath["myapp-svc/app"])
	}
	if d := byPath["myapp-svc/mid"]; d == nil || d.FrameworkType != "dependent" {
		t.Errorf("mid (direct dependent): %+v", byPath["myapp-svc/mid"])
	}
	if d := byPath["myapp-svc/dep2"]; d == nil || d.FrameworkType != "dependent" {
		t.Errorf("dep2 (transitive dependent): %+v", byPath["myapp-svc/dep2"])
	}
	if d := byPath["myapp-svc/models"]; d == nil || d.FrameworkType != "imported" {
		t.Errorf("models (imported): %+v", byPath["myapp-svc/models"])
	}
	if d := byPath["myapp-svc/util"]; d == nil || d.FrameworkType != "imported" {
		t.Errorf("util (recursively imported): %+v", byPath["myapp-svc/util"])
	}
	if _, ok := byPath["myapp-svc/mocks"]; ok {
		t.Error("mock package should be excluded")
	}
	if _, ok := byPath["myapp-svc/utilmock"]; ok {
		t.Error("mock import should be excluded")
	}

	// Direct helper coverage against the populated detector.
	if fd.isPackageImportedByProject("nope/never") {
		t.Error("unknown import should not be project-imported")
	}
}

func TestSweepFrameworkDetectorDepthAndExternal(t *testing.T) {
	app := sweepPkg(t, "myapp-svc/app", "app", "github.com/gin-gonic/gin", "myapp-svc/models")
	models := sweepPkg(t, "myapp-svc/models", "models", "myapp-svc/util")
	util := sweepPkg(t, "myapp-svc/util", "util")
	pkgs := []*packages.Package{app, models, util}

	// Depth 1: models is included, its own import is not followed.
	fd := NewFrameworkDetector()
	fd.Configure(false, 1)
	list, err := fd.AnalyzeFrameworkDependencies(pkgs, nil, nil, nil)
	if err != nil {
		t.Fatalf("analyze: %v", err)
	}
	var sawUtil bool
	for _, dep := range list.AllPackages {
		if dep.PackagePath == "myapp-svc/util" {
			sawUtil = true
		}
	}
	if sawUtil {
		t.Error("depth limit should stop recursion before util")
	}

	// IncludeExternalPackages pulls in the framework import itself.
	fd2 := NewFrameworkDetector()
	fd2.Configure(true, 3)
	list2, err := fd2.AnalyzeFrameworkDependencies(pkgs, nil, nil, nil)
	if err != nil {
		t.Fatalf("analyze external: %v", err)
	}
	var sawGin bool
	for _, dep := range list2.AllPackages {
		if dep.PackagePath == "github.com/gin-gonic/gin" {
			sawGin = true
		}
	}
	if !sawGin {
		t.Error("external mode should include the gin import")
	}
}

func TestSweepFrameworkDetectorHelpers(t *testing.T) {
	// DisableFramework must allocate the map when the config starts empty.
	bare := NewFrameworkDetectorWithConfig(FrameworkDetectorConfig{})
	bare.DisableFramework("http")
	if !bare.config.DisabledFrameworks["http"] {
		t.Error("DisableFramework on zero config did not record the framework")
	}

	// Custom framework keys are appended after the known ones, before http.
	cfg := DefaultFrameworkDetectorConfig()
	cfg.FrameworkPatterns["custom"] = []string{"x.y/custom"}
	fd := NewFrameworkDetectorWithConfig(cfg)
	order := fd.frameworkDetectionOrder()
	if len(order) < 2 || order[len(order)-1] != "http" {
		t.Fatalf("order: %v", order)
	}
	var sawCustom bool
	for _, k := range order {
		if k == "custom" {
			sawCustom = true
		}
	}
	if !sawCustom {
		t.Errorf("custom key missing from order: %v", order)
	}

	// A disabled framework's patterns are skipped during detection.
	appPkg := sweepPkg(t, "myapp-svc/app", "app", "github.com/gin-gonic/gin")
	fd2 := NewFrameworkDetector()
	fd2.DisableFramework("gin")
	if got := fd2.detectFrameworkType(appPkg); got != "" {
		t.Errorf("disabled gin still detected: %q", got)
	}

	// detectProjectRoot: empty detector, then divergent paths where the
	// non-domain path names the root.
	fd3 := NewFrameworkDetector()
	if got := fd3.detectProjectRoot(); got != "" {
		t.Errorf("empty detector root: %q", got)
	}
	fd3.packages["abc/x"] = nil
	fd3.packages["zzz.io/y"] = nil
	fd3.packages["qqq.io/z"] = nil
	if got := fd3.detectProjectRoot(); got != "abc" {
		t.Errorf("divergent root: %q", got)
	}

	// fallbackProjectPackageDetection heuristics.
	fd4 := NewFrameworkDetector()
	if !fd4.fallbackProjectPackageDetection("x.io/deep/models/things") {
		t.Error("project pattern /models/ should match")
	}
	if !fd4.fallbackProjectPackageDetection("my-proj/pkgname") {
		t.Error("hyphenated first segment should match")
	}

	// isProjectRelatedPackage rejects mock packages outright.
	if fd4.isProjectRelatedPackage("some/mockthing") {
		t.Error("mock import should not be project-related")
	}
}
