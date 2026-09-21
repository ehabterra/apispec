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

package spec

import (
	"testing"

	"github.com/ehabterra/apispec/internal/metadata"
)

// comboMeta describes a builder type whose second field carries the path, which
// is what a positional constructor literal has to be matched against.
func comboMeta() *metadata.Metadata {
	sp := metadata.NewStringPool()
	return &metadata.Metadata{
		StringPool: sp,
		Packages: map[string]*metadata.Package{
			"example.com/app": {
				Name: sp.Get("app"),
				// In FILE types: TypeInPackage indexes pkg.Files[*].Types and
				// never the package-level map, so this mirrors what generation
				// actually produces.
				Files: map[string]*metadata.File{
					"app.go": {
						Types: map[string]*metadata.Type{
							"Combo": {
								Name: sp.Get("Combo"),
								Fields: []metadata.Field{
									{Name: sp.Get("r"), Type: sp.Get("*example.com/app.Router")},
									{Name: sp.Get("pattern"), Type: sp.Get("string")},
								},
							},
						},
					},
				},
			},
		},
	}
}

// comboLit builds `&Combo{r, pattern}` — the positional constructor literal,
// with the type expression the literal states it builds.
func comboLit(meta *metadata.Metadata, typePkg, typeName string) metadata.CallArgument {
	unary := metadata.NewCallArgument(meta)
	unary.Kind = meta.StringPool.Get(metadata.KindUnary)

	lit := metadata.NewCallArgument(meta)
	lit.Kind = meta.StringPool.Get(metadata.KindCompositeLit)
	typeExpr := metadata.NewCallArgument(meta)
	typeExpr.Kind = meta.StringPool.Get(metadata.KindIdent)
	typeExpr.Name = meta.StringPool.Get(typeName)
	typeExpr.Pkg = meta.StringPool.Get(typePkg)
	lit.X = typeExpr

	first := metadata.NewCallArgument(meta)
	first.Kind = meta.StringPool.Get(metadata.KindIdent)
	first.Name = meta.StringPool.Get("r")
	second := metadata.NewCallArgument(meta)
	second.Kind = meta.StringPool.Get(metadata.KindIdent)
	second.Name = meta.StringPool.Get("pattern")
	lit.Args = []*metadata.CallArgument{first, second}

	unary.X = lit
	return *unary
}

// constructorCall builds the call expression an assignment's right-hand side
// is: `r.Combo("/items")`, receiver type and all, with the ParamArgMap that
// binds the constructor's parameter — the fact that makes a call-site→edge
// index unnecessary for the variable-assigned builder (issue #506).
func constructorCall(meta *metadata.Metadata, recvType, method, pattern string) *metadata.CallArgument {
	call := metadata.NewCallArgument(meta)
	call.Kind = meta.StringPool.Get(metadata.KindCall)

	fun := metadata.NewCallArgument(meta)
	fun.Kind = meta.StringPool.Get(metadata.KindSelector)
	base := metadata.NewCallArgument(meta)
	base.Kind = meta.StringPool.Get(metadata.KindIdent)
	base.Name = meta.StringPool.Get("r")
	base.Type = meta.StringPool.Get(recvType)
	sel := metadata.NewCallArgument(meta)
	sel.Kind = meta.StringPool.Get(metadata.KindIdent)
	sel.Name = meta.StringPool.Get(method)
	fun.X, fun.Sel = base, sel
	call.Fun = fun

	arg := metadata.NewCallArgument(meta)
	arg.Kind = meta.StringPool.Get(metadata.KindLiteral)
	arg.Value = meta.StringPool.Get(`"` + pattern + `"`)
	call.Args = []*metadata.CallArgument{arg}
	call.ParamArgMap = map[string]metadata.CallArgument{"pattern": *arg}
	return call
}

// builderMeta is comboMeta plus the Router whose `Combo` method returns the
// literal, which is what a call read as an EXPRESSION has to be resolved
// through: there is no edge to read a RecvType off, only the receiver's type.
func builderMeta() *metadata.Metadata {
	meta := comboMeta()
	sp := meta.StringPool
	pkg := meta.Packages["example.com/app"]
	pkg.Files["app.go"].Types["Router"] = &metadata.Type{
		Name: sp.Get("Router"),
		Methods: []metadata.Method{{
			Name:       sp.Get("Combo"),
			Receiver:   sp.Get("*Router"),
			ReturnVars: []metadata.CallArgument{comboLit(meta, "example.com/app", "Combo")},
		}},
	}
	return meta
}

// The variable-assigned builder resolves from the assignment's call alone: the
// callee's returned literal says which parameter holds the path, and the call
// expression's own ParamArgMap says what this site passed for it (issue #506).
func TestFieldFromCallReadsConstructorArgument(t *testing.T) {
	meta := builderMeta()
	b := NewBasePatternMatcher(&APISpecConfig{}, NewContextProvider(meta))

	call := constructorCall(meta, "*example.com/app.Router", "Combo", "/items")
	got, ok := b.fieldFromCall(call, "pattern", "example.com/app", "Combo")
	if !ok {
		t.Fatal("the constructor's argument was not read back off the returned literal")
	}
	if got != "/items" {
		t.Errorf("value = %q, want %q", got, "/items")
	}

	// A field the literal does not carry stays unresolved rather than empty.
	if v, ok := b.fieldFromCall(call, "absent", "example.com/app", "Combo"); ok {
		t.Errorf("an undeclared field resolved to %q", v)
	}
}

// A chain assigned MID-WAY (`v := r.Combo("/x").Get(h)`) holds the verb's call,
// and a verb returns its receiver rather than a literal — so the constructor is
// one hop further in, exactly as it is for the ChainParent walk.
func TestFieldFromCallChainWalksPastTheVerb(t *testing.T) {
	meta := builderMeta()
	sp := meta.StringPool
	// Get returns `c`, an ident: no literal to read, so the walk must continue.
	recv := metadata.NewCallArgument(meta)
	recv.Kind = sp.Get(metadata.KindIdent)
	recv.Name = sp.Get("c")
	combo := meta.Packages["example.com/app"].Files["app.go"].Types["Combo"]
	combo.Methods = []metadata.Method{{
		Name:       sp.Get("Get"),
		Receiver:   sp.Get("*Combo"),
		ReturnVars: []metadata.CallArgument{*recv},
	}}

	// The verb is written ON the constructor's call: `r.Combo("/items").Get(h)`,
	// so the verb's receiver expression IS that call, typed by what it returns.
	verb := constructorCall(meta, "", "Get", "unused")
	verb.Fun.X = constructorCall(meta, "*example.com/app.Router", "Combo", "/items")
	verb.Fun.X.Type = sp.Get("*example.com/app.Combo")

	b := NewBasePatternMatcher(&APISpecConfig{}, NewContextProvider(meta))
	got, ok := b.fieldFromCallChain(verb, "pattern", "example.com/app", "Combo")
	if !ok {
		t.Fatal("the walk stopped at the verb instead of reaching the constructor")
	}
	if got != "/items" {
		t.Errorf("value = %q, want %q", got, "/items")
	}

	// Anything that is not a call ends the walk rather than being guessed at.
	ident := metadata.NewCallArgument(meta)
	ident.Kind = sp.Get(metadata.KindIdent)
	ident.Name = sp.Get("v")
	if v, ok := b.fieldFromCallChain(ident, "pattern", "example.com/app", "Combo"); ok {
		t.Errorf("a bare identifier resolved to %q", v)
	}
}

// A positional literal is read by FIELD INDEX, so a literal of some OTHER
// struct that happens to have an element at that index would answer with a
// value that has nothing to do with the receiver. The literal states which type
// it builds, and that is what keeps the index meaningful.
func TestLiteralConstructsChecksTypeIdentity(t *testing.T) {
	meta := comboMeta()
	for _, tc := range []struct {
		name     string
		litPkg   string
		litName  string
		typePkg  string
		typeName string
		want     bool
	}{
		{"same package and name", "example.com/app", "Combo", "example.com/app", "Combo", true},
		{"another type entirely", "example.com/app", "Route", "example.com/app", "Combo", false},
		{"same name, DIFFERENT package", "example.com/other", "Combo", "example.com/app", "Combo", false},
		// Unverifiable is not disqualifying: with no package recorded the name
		// is all the evidence there is, which is the behaviour this check was
		// added around.
		{"no package on the literal", "", "Combo", "example.com/app", "Combo", true},
	} {
		lit := comboLit(meta, tc.litPkg, tc.litName)
		got := literalConstructs(unwrapComposite(&lit), tc.typePkg, tc.typeName)
		if got != tc.want {
			t.Errorf("%s: literalConstructs = %v, want %v", tc.name, got, tc.want)
		}
	}
	// A literal with no type expression at all is left to the field-index match.
	bare := metadata.NewCallArgument(meta)
	bare.Kind = meta.StringPool.Get(metadata.KindCompositeLit)
	if !literalConstructs(bare, "example.com/app", "Combo") {
		t.Error("a literal with no type expression must not be declined outright")
	}
}

// The callee of a call read as an expression is named by calleeNameOf, never by
// Fun.GetName() alone: a cross-package constructor has a SELECTOR Fun whose name
// lives in .Sel, and reading the ident name would resolve nothing for every call
// that crosses a package (golden rule #10).
func TestCallReturnVarsCrossPackageFunction(t *testing.T) {
	meta := builderMeta()
	sp := meta.StringPool
	meta.Packages["example.com/app"].Files["app.go"].Functions = map[string]*metadata.Function{
		"NewCombo": {
			Name:       sp.Get("NewCombo"),
			Pkg:        sp.Get("example.com/app"),
			ReturnVars: []metadata.CallArgument{comboLit(meta, "example.com/app", "Combo")},
		},
	}
	b := NewBasePatternMatcher(&APISpecConfig{}, NewContextProvider(meta))

	// `app.NewCombo("/items")`: the qualifier is the package, so the base has no
	// type of its own and Fun.GetName() is empty.
	call := constructorCall(meta, "", "NewCombo", "/items")
	call.Fun.X.Name = sp.Get("app")
	call.Fun.X.Pkg = sp.Get("example.com/app")

	if len(b.callReturnVars(call)) == 0 {
		t.Fatal("a cross-package constructor resolved no return values — the name was read off the ident, not .Sel")
	}
	got, ok := b.fieldFromCall(call, "pattern", "example.com/app", "Combo")
	if !ok || got != "/items" {
		t.Errorf("fieldFromCall = (%q, %v), want (%q, true)", got, ok, "/items")
	}

	// A callee nothing declares resolves nothing.
	unknown := constructorCall(meta, "", "Missing", "/items")
	unknown.Fun.X.Name = sp.Get("app")
	unknown.Fun.X.Pkg = sp.Get("example.com/app")
	if len(b.callReturnVars(unknown)) != 0 {
		t.Error("an undeclared callee must resolve no return values")
	}
}

// Every way the variable-assigned rung can decline, in one place: each is a
// "cannot tell", and a "cannot tell" has to reach the caller as one so the
// registration is reported rather than documented at a guessed path.
func TestReceiverVarRungDeclines(t *testing.T) {
	meta := builderMeta()
	sp := meta.StringPool
	b := NewBasePatternMatcher(&APISpecConfig{}, NewContextProvider(meta))

	// No invocation, and an invocation with no receiver variable — the chained
	// shape, which the ChainParent walk above answers instead.
	if _, ok := b.fieldFromReceiverVar(nil, nil, "pattern", "example.com/app", "Combo"); ok {
		t.Error("a nil invocation resolved a field")
	}
	if _, ok := b.fieldFromReceiverVar(nil, &metadata.CallGraphEdge{}, "pattern", "example.com/app", "Combo"); ok {
		t.Error("an invocation with no receiver variable resolved a field")
	}
	// A receiver variable with no assignment recorded anywhere.
	unknown := &metadata.CallGraphEdge{CalleeVarName: "nowhere"}
	if _, ok := b.fieldFromReceiverVar(nil, unknown, "pattern", "example.com/app", "Combo"); ok {
		t.Error("a variable with no assignment resolved a field")
	}

	// A call whose callee is unknown, and one that is not a call at all.
	if _, ok := b.fieldFromCall(nil, "pattern", "example.com/app", "Combo"); ok {
		t.Error("a nil call resolved a field")
	}
	if _, ok := b.fieldFromReturnedLiteral(nil, "pattern", "example.com/app", "Combo"); ok {
		t.Error("a nil edge resolved a field")
	}
	if got := b.callReturnVars(nil); got != nil {
		t.Error("a nil call has no return values")
	}
	noFun := metadata.NewCallArgument(meta)
	noFun.Kind = sp.Get(metadata.KindCall)
	if got := b.callReturnVars(noFun); got != nil {
		t.Error("a call with no Fun has no return values")
	}
	unnamed := metadata.NewCallArgument(meta)
	unnamed.Kind = sp.Get(metadata.KindCall)
	unnamed.Fun = metadata.NewCallArgument(meta)
	unnamed.Fun.Kind = sp.Get(metadata.KindSelector)
	if got := b.callReturnVars(unnamed); got != nil {
		t.Error("a call whose Fun names nothing has no return values")
	}

	// A receiver of a type nothing declares, and a type that declares no such
	// method: both are the method lookup finding nothing, which must not fall
	// through to the plain-function table and resolve a same-named function.
	missingType := constructorCall(meta, "*example.com/app.Unknown", "Combo", "/items")
	if got := b.callReturnVars(missingType); got != nil {
		t.Error("a receiver of an undeclared type resolved return values")
	}
	missingMethod := constructorCall(meta, "*example.com/app.Router", "Absent", "/items")
	if got := b.callReturnVars(missingMethod); got != nil {
		t.Error("a method the type does not declare resolved return values")
	}

	// The walk ends at anything that is not a chained call rather than
	// following the receiver expression into a variable it cannot evaluate.
	fromVar := constructorCall(meta, "", "Get", "/items")
	fromVar.Fun.X.Type = sp.Get("*example.com/app.Combo")
	if _, ok := b.fieldFromCallChain(fromVar, "pattern", "example.com/app", "Combo"); ok {
		t.Error("the walk resolved a field from a receiver it never evaluated")
	}

	// A returned value that is not a composite literal at all.
	ident := metadata.NewCallArgument(meta)
	ident.Kind = sp.Get(metadata.KindIdent)
	ident.Name = sp.Get("c")
	if _, ok := b.fieldInReturns([]metadata.CallArgument{*ident}, nil, "pattern", "example.com/app", "Combo"); ok {
		t.Error("a returned identifier is not a literal to read a field out of")
	}
	// A literal of ANOTHER type with an element at the same index: the field
	// index would answer, and the type check is what stops it.
	other := comboLit(meta, "example.com/app", "Route")
	if _, ok := b.fieldInReturns([]metadata.CallArgument{other}, nil, "pattern", "example.com/app", "Combo"); ok {
		t.Error("a literal of another type answered the receiver's field")
	}
	// The literal stores a parameter this call site did not bind.
	lit := comboLit(meta, "example.com/app", "Combo")
	if _, ok := b.fieldInReturns([]metadata.CallArgument{lit}, nil, "pattern", "example.com/app", "Combo"); ok {
		t.Error("an unbound parameter resolved to a value")
	}
}

func TestStructFieldIndex(t *testing.T) {
	meta := comboMeta()
	for _, tc := range []struct {
		field string
		want  int
		ok    bool
	}{
		{"r", 0, true},
		{"pattern", 1, true},
		{"absent", 0, false},
	} {
		got, ok := structFieldIndex(meta, "example.com/app", "Combo", tc.field)
		if ok != tc.ok || (ok && got != tc.want) {
			t.Errorf("structFieldIndex(%q) = (%d, %v), want (%d, %v)", tc.field, got, ok, tc.want, tc.ok)
		}
	}
	if _, ok := structFieldIndex(meta, "example.com/app", "Missing", "pattern"); ok {
		t.Error("an unknown type must not resolve a field index")
	}
	if _, ok := structFieldIndex(nil, "example.com/app", "Combo", "pattern"); ok {
		t.Error("nil metadata must not resolve a field index")
	}
}

// A constructor almost always writes its literal POSITIONALLY
// (`&Combo{r, pattern}`), so the element is found by matching the index against
// the struct's declared field order — the fact that makes the builder shape
// resolvable at all (issue #461).
func TestLiteralFieldElementPositional(t *testing.T) {
	meta := comboMeta()
	lit := metadata.NewCallArgument(meta)
	lit.Kind = meta.StringPool.Get(metadata.KindCompositeLit)
	first := metadata.NewCallArgument(meta)
	first.Kind = meta.StringPool.Get(metadata.KindIdent)
	first.Name = meta.StringPool.Get("r")
	second := metadata.NewCallArgument(meta)
	second.Kind = meta.StringPool.Get(metadata.KindIdent)
	second.Name = meta.StringPool.Get("pattern")
	lit.Args = []*metadata.CallArgument{first, second}

	got, ok := literalFieldElement(meta, lit, "pattern", "example.com/app", "Combo")
	if !ok || got == nil {
		t.Fatalf("positional element for %q not found", "pattern")
	}
	if got.GetName() != "pattern" {
		t.Errorf("resolved element is %q, want the second element", got.GetName())
	}

	// A field the struct does not declare cannot be positioned.
	if _, ok := literalFieldElement(meta, lit, "absent", "example.com/app", "Combo"); ok {
		t.Error("a field the struct does not declare must not resolve")
	}
}

// A keyed literal (`&Combo{r: r, pattern: pattern}`) is matched by key, and a
// field ABSENT from the keys is the zero value — which is not a path, so it is
// reported unresolved rather than as the empty string.
func TestLiteralFieldElementKeyed(t *testing.T) {
	meta := comboMeta()
	lit := metadata.NewCallArgument(meta)
	lit.Kind = meta.StringPool.Get(metadata.KindCompositeLit)

	kv := metadata.NewCallArgument(meta)
	kv.Kind = meta.StringPool.Get(metadata.KindKeyValue)
	key := metadata.NewCallArgument(meta)
	key.Kind = meta.StringPool.Get(metadata.KindIdent)
	key.Name = meta.StringPool.Get("pattern")
	val := metadata.NewCallArgument(meta)
	val.Kind = meta.StringPool.Get(metadata.KindLiteral)
	val.Value = meta.StringPool.Get(`"/items"`)
	kv.X, kv.Fun = key, val
	lit.Args = []*metadata.CallArgument{kv}

	got, ok := literalFieldElement(meta, lit, "pattern", "example.com/app", "Combo")
	if !ok || got == nil {
		t.Fatal("keyed element not found")
	}
	if got.GetValue() != `"/items"` {
		t.Errorf("resolved value %q, want %q", got.GetValue(), `"/items"`)
	}

	if _, ok := literalFieldElement(meta, lit, "r", "example.com/app", "Combo"); ok {
		t.Error("a field absent from a KEYED literal is the zero value, which is not a path")
	}
}

// The rung declines anything that is not a field read off an identifier, before
// it walks any chain.
func TestReceiverFieldValueGuards(t *testing.T) {
	meta := comboMeta()
	b := NewBasePatternMatcher(&APISpecConfig{}, NewContextProvider(meta))

	notSelector := metadata.NewCallArgument(meta)
	notSelector.Kind = meta.StringPool.Get(metadata.KindIdent)
	notSelector.Name = meta.StringPool.Get("p")

	sel := metadata.NewCallArgument(meta)
	sel.Kind = meta.StringPool.Get(metadata.KindSelector)
	base := metadata.NewCallArgument(meta)
	base.Kind = meta.StringPool.Get(metadata.KindIdent)
	base.Name = meta.StringPool.Get("c")
	field := metadata.NewCallArgument(meta)
	field.Name = meta.StringPool.Get("pattern")
	sel.X, sel.Sel = base, field

	for name, arg := range map[string]*metadata.CallArgument{
		"nil argument":   nil,
		"not a selector": notSelector,
		"selector":       sel, // with a nil node
	} {
		if v, ok := b.receiverFieldValue(arg, nil); ok {
			t.Errorf("%s: resolved %q with no tracker node", name, v)
		}
	}
}

// An empty constructor argument is a RESOLVED value, not an unresolved one:
// ConstantValue("") returns ("", true), and the caller must honour that rather
// than testing the string, or a builder constructed with "" would emit a
// {placeholder} for a path it had actually resolved (review of #464).
func TestLiteralFieldElementKeepsEmptyValue(t *testing.T) {
	meta := comboMeta()
	lit := metadata.NewCallArgument(meta)
	lit.Kind = meta.StringPool.Get(metadata.KindCompositeLit)

	kv := metadata.NewCallArgument(meta)
	kv.Kind = meta.StringPool.Get(metadata.KindKeyValue)
	key := metadata.NewCallArgument(meta)
	key.Kind = meta.StringPool.Get(metadata.KindIdent)
	key.Name = meta.StringPool.Get("pattern")
	val := metadata.NewCallArgument(meta)
	val.Kind = meta.StringPool.Get(metadata.KindLiteral)
	val.Value = meta.StringPool.Get(`""`)
	kv.X, kv.Fun = key, val
	lit.Args = []*metadata.CallArgument{kv}

	elt, ok := literalFieldElement(meta, lit, "pattern", "example.com/app", "Combo")
	if !ok || elt == nil {
		t.Fatal("an empty value must still be found")
	}
	cp := NewContextProvider(meta)
	v, resolved := cp.ConstantValue(elt)
	if !resolved {
		t.Error("an empty string literal is resolved, so the ladder must not treat it as a failure")
	}
	if v != "" {
		t.Errorf("value = %q, want the empty string", v)
	}
}

// Two packages with a `Combo` builder is ordinary, and comparing bare names
// would make `two.Combo`'s field resolve from `one.Combo`'s constructor — a
// route stated confidently from the wrong value. That is the bare-name
// collision #457 was about; the check carries the package path (review of
// #464).
func TestReceiverTypeMatchesRequiresPackageIdentity(t *testing.T) {
	for _, tc := range []struct {
		name     string
		baseType string
		recvPkg  string
		recvName string
		want     bool
	}{
		{"same package and name", "*example.com/one.Combo", "example.com/one", "Combo", true},
		{"pointer-free base", "example.com/one.Combo", "example.com/one", "Combo", true},
		{"same name, DIFFERENT package", "*example.com/two.Combo", "example.com/one", "Combo", false},
		{"same package, different name", "*example.com/one.Router", "example.com/one", "Combo", false},
		// A base with no package cannot be shown to be the receiver's type.
		{"unqualified base", "*Combo", "example.com/one", "Combo", false},
		{"empty base", "", "example.com/one", "Combo", false},
		{"no receiver package", "*example.com/one.Combo", "", "Combo", false},
		// A path-suffix coincidence must not pass either.
		{"package suffix collision", "*evil.com/example.com/one.Combo", "example.com/one", "Combo", false},
	} {
		if got := receiverTypeMatches(tc.baseType, tc.recvPkg, tc.recvName); got != tc.want {
			t.Errorf("%s: receiverTypeMatches(%q, %q, %q) = %v, want %v",
				tc.name, tc.baseType, tc.recvPkg, tc.recvName, got, tc.want)
		}
	}
}
