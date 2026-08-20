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

// wrapperMeta declares the functions the peel has to reason about: two
// middlewares (same type in and out), an adapter that takes a function but is
// NOT middleware-shaped, and the handlers themselves.
func wrapperMeta(t *testing.T) *metadata.Metadata {
	t.Helper()
	pool := metadata.NewStringPool()
	meta := &metadata.Metadata{StringPool: pool}

	const pkg = "app"
	fn := func(name, paramType, returnType string) *metadata.Function {
		f := &metadata.Function{Name: pool.Get(name), Pkg: pool.Get(pkg)}
		f.Signature = *metadata.NewCallArgument(meta)
		param := metadata.NewCallArgument(meta)
		param.SetKind(metadata.KindIdent)
		param.SetName("next")
		if paramType != "" {
			param.Type = pool.Get(paramType)
		}
		f.Signature.Args = []*metadata.CallArgument{param}
		if returnType != "" {
			f.Signature.ResolvedType = pool.Get(returnType)
		}
		return f
	}
	meta.Packages = map[string]*metadata.Package{
		pkg: {Files: map[string]*metadata.File{
			"main.go": {Functions: map[string]*metadata.Function{
				// func(http.Handler) http.Handler
				"withLogging": fn("withLogging", "Handler", "http.Handler"),
				// func(http.HandlerFunc) http.HandlerFunc
				"withTiming": fn("withTiming", "HandlerFunc", "http.HandlerFunc"),
				// func(func(Req) (Resp, error)) http.HandlerFunc — an adapter,
				// not a middleware: what goes in is not what comes out.
				"HandleRequest": fn("HandleRequest", "func(Req) (Resp, error)", "http.HandlerFunc"),
				"getItem":       {Name: pool.Get("getItem"), Pkg: pool.Get(pkg)},
				"createItem":    {Name: pool.Get("createItem"), Pkg: pool.Get(pkg)},
				"handleCreate":  {Name: pool.Get("handleCreate"), Pkg: pool.Get(pkg)},
			}},
		}},
	}
	return meta
}

func wrapperIdent(meta *metadata.Metadata, name string) *metadata.CallArgument {
	a := metadata.NewCallArgument(meta)
	a.SetKind(metadata.KindIdent)
	a.SetName(name)
	a.Pkg = meta.StringPool.Get("app")
	return a
}

func wrapperConversion(meta *metadata.Metadata, inner *metadata.CallArgument) *metadata.CallArgument {
	a := metadata.NewCallArgument(meta)
	a.SetKind(metadata.KindTypeConversion)
	a.Args = []*metadata.CallArgument{inner}
	return a
}

func wrapperCall(meta *metadata.Metadata, callee string, args ...*metadata.CallArgument) *metadata.CallArgument {
	a := metadata.NewCallArgument(meta)
	a.SetKind(metadata.KindCall)
	a.Fun = wrapperIdent(meta, callee)
	a.Args = args
	return a
}

// TestHandlerArgValuePeelsWrappers pins what the handler position resolves to
// for each wiring style (issue #364). The rule has to separate a middleware
// wrapping the handler from an adapter that merely takes a function, because
// peeling the latter would rename operations that are correct today.
func TestHandlerArgValuePeelsWrappers(t *testing.T) {
	meta := wrapperMeta(t)

	tests := []struct {
		name string
		arg  *metadata.CallArgument
		want string // resolved handler name, "" for "unchanged"
	}{
		{
			name: "conversion of a handler",
			arg:  wrapperConversion(meta, wrapperIdent(meta, "getItem")),
			want: "getItem",
		},
		{
			name: "middleware wrapping a converted handler",
			arg:  wrapperCall(meta, "withLogging", wrapperConversion(meta, wrapperIdent(meta, "getItem"))),
			want: "getItem",
		},
		{
			name: "middleware taking the handler as a plain ident",
			arg:  wrapperCall(meta, "withTiming", wrapperIdent(meta, "createItem")),
			want: "createItem",
		},
		{
			name: "nested middlewares peel to the innermost handler",
			arg: wrapperCall(meta, "withTiming",
				wrapperCall(meta, "withLogging", wrapperConversion(meta, wrapperIdent(meta, "getItem")))),
			want: "getItem",
		},
		{
			name: "an adapter that is not middleware-shaped is left alone",
			arg:  wrapperCall(meta, "HandleRequest", wrapperIdent(meta, "handleCreate")),
			want: "", // the adapter instantiation stays the operation (#367)
		},
		{
			name: "a handler factory has nothing to peel",
			arg:  wrapperCall(meta, "withLogging"),
			want: "",
		},
		{
			name: "two handler-shaped arguments are ambiguous",
			arg: wrapperCall(meta, "withLogging",
				wrapperConversion(meta, wrapperIdent(meta, "getItem")),
				wrapperConversion(meta, wrapperIdent(meta, "createItem"))),
			want: "",
		},
		{
			name: "a conversion of a value is not a handler",
			arg:  wrapperConversion(meta, wrapperIdent(meta, "payload")),
			want: "payload", // the conversion peels; the value simply isn't a function
		},
		{
			name: "a middleware given a value, not a function",
			arg:  wrapperCall(meta, "withLogging", wrapperIdent(meta, "payload")),
			want: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := handlerArgValue(tc.arg)
			if tc.want == "" {
				if got != tc.arg {
					t.Errorf("expected the argument to stay as it is, got %q (kind %v)", got.GetName(), got.GetKind())
				}
				return
			}
			if got.GetName() != tc.want {
				t.Errorf("resolved to %q, want %q", got.GetName(), tc.want)
			}
		})
	}
}

// TestWrappedHandlerResolvesMethodValuesAndEdges covers the two resolution paths
// the fixture wiring does not reach: a handler passed as a METHOD value
// (`withLogging(h.GetItem)`), and a wrapper call the call graph recorded an edge
// for, where the callee is read from the edge rather than from the call's Fun.
func TestWrappedHandlerResolvesMethodValuesAndEdges(t *testing.T) {
	meta := wrapperMeta(t)
	pool := meta.StringPool

	// A method value: app.Handler.GetItem.
	pkg := meta.Packages["app"]
	pkg.Files["main.go"].Types = map[string]*metadata.Type{
		"Handler": {Methods: []metadata.Method{{Name: pool.Get("GetItem"), Receiver: pool.Get("*Handler")}}},
	}

	sel := metadata.NewCallArgument(meta)
	sel.SetKind(metadata.KindSelector)
	sel.X = wrapperIdent(meta, "h")
	sel.X.Type = pool.Get("*app.Handler")
	sel.Sel = wrapperIdent(meta, "GetItem")

	if got := handlerArgValue(wrapperCall(meta, "withLogging", sel)); got != sel {
		t.Errorf("a wrapped method value should resolve to the method, got %q", got.GetName())
	}

	// The same wrapper, but resolved through the recorded call edge.
	viaEdge := metadata.NewCallArgument(meta)
	viaEdge.SetKind(metadata.KindCall)
	viaEdge.Args = []*metadata.CallArgument{wrapperIdent(meta, "getItem")}
	viaEdge.Edge = &metadata.CallGraphEdge{
		Caller: metadata.Call{Meta: meta, Name: pool.Get("main"), Pkg: pool.Get("app"), RecvType: -1},
		Callee: metadata.Call{Meta: meta, Name: pool.Get("withLogging"), Pkg: pool.Get("app"), RecvType: -1},
	}
	if got := handlerArgValue(viaEdge); got.GetName() != "getItem" {
		t.Errorf("a wrapper resolved through its edge should peel to the handler, got %q", got.GetName())
	}

	// No metadata to consult: nothing is known to be a function, so nothing is
	// peeled — the honest answer rather than a guess.
	bare := wrapperCall(meta, "withLogging", wrapperIdent(meta, "getItem"))
	bare.Meta = nil
	for _, a := range bare.Args {
		a.Meta = nil
	}
	if got := handlerArgValue(bare); got != bare {
		t.Error("without metadata the wrapper must stay as it is")
	}
}

// TestWrapperHelperGuards exercises the refusals directly: every one of them is
// a place where peeling would be a guess, so "returns nothing" is the behaviour
// under test, not an incidental nil.
func TestWrapperHelperGuards(t *testing.T) {
	meta := wrapperMeta(t)

	t.Run("convertedHandler", func(t *testing.T) {
		if convertedHandler(nil) != nil {
			t.Error("nil argument")
		}
		twoArgs := wrapperConversion(meta, wrapperIdent(meta, "getItem"))
		twoArgs.Args = append(twoArgs.Args, wrapperIdent(meta, "createItem"))
		if convertedHandler(twoArgs) != nil {
			t.Error("a conversion has exactly one operand; anything else is not one")
		}
		lit := metadata.NewCallArgument(meta)
		lit.SetKind(metadata.KindLiteral)
		if convertedHandler(wrapperConversion(meta, lit)) != nil {
			t.Error("a converted literal is not a handler")
		}
	})

	t.Run("wrappedHandler", func(t *testing.T) {
		if wrappedHandler(nil) != nil {
			t.Error("nil call")
		}
		// A conversion of something that is not a function contributes no
		// candidate, so the wrapper has nothing to give.
		lit := metadata.NewCallArgument(meta)
		lit.SetKind(metadata.KindLiteral)
		if wrappedHandler(wrapperCall(meta, "withLogging", wrapperConversion(meta, lit))) != nil {
			t.Error("a converted literal must not be taken for the handler")
		}
		// A nested call that wraps nothing is not a candidate either.
		if wrappedHandler(wrapperCall(meta, "withLogging", wrapperCall(meta, "withLogging"))) != nil {
			t.Error("a nested call that wraps no handler must not be taken for one")
		}
	})

	t.Run("middlewareShaped", func(t *testing.T) {
		if middlewareShaped(wrapperCall(meta, "unknownFunc", wrapperIdent(meta, "getItem"))) {
			t.Error("a callee apispec cannot resolve has no known shape")
		}
		noMeta := wrapperCall(meta, "withLogging", wrapperIdent(meta, "getItem"))
		noMeta.Meta = nil
		if middlewareShaped(noMeta) {
			t.Error("without metadata there is no signature to compare")
		}
	})

	t.Run("a wrapper reached as a method", func(t *testing.T) {
		// `chain.Then(getItem)` — alice's shape: the wrapper is a METHOD, and
		// with no recorded call edge only the method table can say it is
		// middleware-shaped.
		pool := meta.StringPool
		sig := *metadata.NewCallArgument(meta)
		param := metadata.NewCallArgument(meta)
		param.SetKind(metadata.KindIdent)
		param.Type = pool.Get("Handler")
		sig.Args = []*metadata.CallArgument{param}
		sig.ResolvedType = pool.Get("http.Handler")
		meta.Packages["app"].Files["main.go"].Types = map[string]*metadata.Type{
			"Chain": {Methods: []metadata.Method{{Name: pool.Get("Then"), Receiver: pool.Get("Chain"), Signature: sig}}},
		}

		fun := metadata.NewCallArgument(meta)
		fun.SetKind(metadata.KindSelector)
		fun.X = wrapperIdent(meta, "chain")
		fun.X.Type = pool.Get("app.Chain")
		fun.Sel = wrapperIdent(meta, "Then")
		call := metadata.NewCallArgument(meta)
		call.SetKind(metadata.KindCall)
		call.Fun = fun
		call.Args = []*metadata.CallArgument{wrapperIdent(meta, "getItem")}

		if got := handlerArgValue(call); got.GetName() != "getItem" {
			t.Errorf("a method wrapper should peel to the handler it is given, got %q", got.GetName())
		}
	})

	t.Run("calleeFunctionOf", func(t *testing.T) {
		if calleeFunctionOf(nil) != nil {
			t.Error("nil call")
		}
		noFun := metadata.NewCallArgument(meta)
		noFun.SetKind(metadata.KindCall)
		if calleeFunctionOf(noFun) != nil {
			t.Error("a call with neither an edge nor a Fun names nothing")
		}
		// Through a selector Fun: pkg.withLogging(...).
		sel := metadata.NewCallArgument(meta)
		sel.SetKind(metadata.KindSelector)
		sel.X = wrapperIdent(meta, "app")
		sel.Sel = wrapperIdent(meta, "withLogging")
		viaSel := metadata.NewCallArgument(meta)
		viaSel.SetKind(metadata.KindCall)
		viaSel.Fun = sel
		if calleeFunctionOf(viaSel) == nil {
			t.Error("a qualified wrapper call should resolve through its selector")
		}
	})

	t.Run("namesAFunction", func(t *testing.T) {
		if namesAFunction(nil) {
			t.Error("nil argument")
		}
		lit := metadata.NewCallArgument(meta)
		lit.SetKind(metadata.KindFuncLit)
		if !namesAFunction(lit) {
			t.Error("a function literal is a function")
		}
		noMeta := wrapperIdent(meta, "getItem")
		noMeta.Meta = nil
		if namesAFunction(noMeta) {
			t.Error("without metadata nothing is known to be a function")
		}
		bareSel := metadata.NewCallArgument(meta)
		bareSel.SetKind(metadata.KindSelector)
		if namesAFunction(bareSel) {
			t.Error("a selector with no Sel names nothing")
		}
		if namesAFunction(wrapperIdent(meta, "payload")) {
			t.Error("an ident naming a value is not a function")
		}
	})
}

// TestConvertedValueKeys covers the tracker side of the same shape: expansion
// has to follow what a conversion converts (`http.HandlerFunc(getItem)`),
// because the conversion itself names a type and a type has no body.
func TestConvertedValueKeys(t *testing.T) {
	meta := wrapperMeta(t)
	node := func(arg *metadata.CallArgument, isArg bool) *LazyNode {
		return &LazyNode{tree: &LazyTree{meta: meta}, arg: arg, isArgument: isArg}
	}

	conv := wrapperConversion(meta, wrapperIdent(meta, "getItem"))
	if keys := node(conv, true).convertedValueKeys(); len(keys) != 1 {
		t.Errorf("a converted function should yield its own key, got %v", keys)
	}
	if keys := node(conv, false).convertedValueKeys(); keys != nil {
		t.Errorf("only an argument node converts anything, got %v", keys)
	}
	if keys := node(wrapperIdent(meta, "getItem"), true).convertedValueKeys(); keys != nil {
		t.Errorf("a plain ident is not a conversion, got %v", keys)
	}
	lit := metadata.NewCallArgument(meta)
	lit.SetKind(metadata.KindLiteral)
	if keys := node(wrapperConversion(meta, lit), true).convertedValueKeys(); keys != nil {
		t.Errorf("a converted literal has no body to expand, got %v", keys)
	}
	if keys := node(wrapperConversion(meta, nil), true).convertedValueKeys(); keys != nil {
		t.Errorf("a conversion of nothing yields nothing, got %v", keys)
	}
}
