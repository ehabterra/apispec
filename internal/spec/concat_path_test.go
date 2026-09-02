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

// concatLit builds a string-literal CallArgument.
func concatLit(meta *metadata.Metadata, value string) *metadata.CallArgument {
	a := metadata.NewCallArgument(meta)
	a.SetKind(metadata.KindLiteral)
	a.SetValue(`"` + value + `"`)
	return a
}

// concatOf builds `left + right`.
func concatOf(meta *metadata.Metadata, left, right *metadata.CallArgument) *metadata.CallArgument {
	a := metadata.NewCallArgument(meta)
	a.SetKind(metadata.KindBinary)
	a.SetValue("+")
	a.X = left
	a.Fun = right
	return a
}

// TestResolveConcatenatedPath covers the path folding that generated servers
// need (issue #274): before it, a registration whose path is built by
// concatenation resolved to nothing at all, so the route was documented at its
// mount prefix alone — losing the only part of the path written in the source.
func TestResolveConcatenatedPath(t *testing.T) {
	meta := newTestMeta()
	b := &BasePatternMatcher{contextProvider: NewContextProvider(meta)}

	t.Run("two literals fold", func(t *testing.T) {
		arg := concatOf(meta, concatLit(meta, "/api"), concatLit(meta, "/things"))
		if path, dyn := b.resolvePathArg(arg, nil); path != "/api/things" || len(dyn) != 0 {
			t.Errorf("got (%q, %v), want (\"/api/things\", no placeholders)", path, dyn)
		}
	})

	t.Run("three operands fold left to right", func(t *testing.T) {
		inner := concatOf(meta, concatLit(meta, "/api"), concatLit(meta, "/v2"))
		arg := concatOf(meta, inner, concatLit(meta, "/things"))
		if path, _ := b.resolvePathArg(arg, nil); path != "/api/v2/things" {
			t.Errorf("got %q, want \"/api/v2/things\"", path)
		}
	})

	t.Run("an empty operand contributes nothing", func(t *testing.T) {
		// The generated shape: BaseURL is left at its zero value, so the path is
		// exactly the literal.
		arg := concatOf(meta, concatLit(meta, ""), concatLit(meta, "/things"))
		if path, _ := b.resolvePathArg(arg, nil); path != "/things" {
			t.Errorf("got %q, want \"/things\"", path)
		}
	})

	t.Run("an unresolvable operand becomes a placeholder, keeping the literal part", func(t *testing.T) {
		// Dropping the segment would claim the handler answers somewhere it does
		// not; the placeholder keeps the route addressable and visibly partial.
		unknown := metadata.NewCallArgument(meta)
		unknown.SetKind(metadata.KindCall)
		unknown.SetName("dynamicBase")

		arg := concatOf(meta, unknown, concatLit(meta, "/dyn"))
		path, dyn := b.resolvePathArg(arg, nil)
		if path != "{dynamicBase}/dyn" {
			t.Errorf("got %q, want \"{dynamicBase}/dyn\"", path)
		}
		if len(dyn) != 1 || dyn[0] != "dynamicBase" {
			t.Errorf("dynamic names = %v, want [dynamicBase] so the caller can register the parameter", dyn)
		}
	})

	t.Run("a non-concat operator is not folded", func(t *testing.T) {
		// Only `+` builds a path. Anything else is one opaque value.
		arg := concatOf(meta, concatLit(meta, "/a"), concatLit(meta, "/b"))
		arg.SetValue("-")
		arg.SetName("weird")
		if path, _ := b.resolvePathArg(arg, nil); path != "{weird}" {
			t.Errorf("got %q, want the whole expression as one placeholder", path)
		}
	})

	t.Run("a plain literal path is unchanged", func(t *testing.T) {
		if path, dyn := b.resolvePathArg(concatLit(meta, "/users"), nil); path != "/users" || len(dyn) != 0 {
			t.Errorf("got (%q, %v), want (\"/users\", no placeholders)", path, dyn)
		}
	})
}

// TestFlattenConcat pins the operand walk itself.
func TestFlattenConcat(t *testing.T) {
	meta := newTestMeta()

	t.Run("nil is no operands", func(t *testing.T) {
		if got := flattenConcat(nil, nil); got != nil {
			t.Errorf("got %v, want nil", got)
		}
	})

	t.Run("a leaf is one operand", func(t *testing.T) {
		if got := flattenConcat(concatLit(meta, "/x"), nil); len(got) != 1 {
			t.Errorf("got %d operands, want 1", len(got))
		}
	})

	t.Run("a nested concat flattens in source order", func(t *testing.T) {
		inner := concatOf(meta, concatLit(meta, "/a"), concatLit(meta, "/b"))
		got := flattenConcat(concatOf(meta, inner, concatLit(meta, "/c")), nil)
		if len(got) != 3 {
			t.Fatalf("got %d operands, want 3", len(got))
		}
		want := []string{`"/a"`, `"/b"`, `"/c"`}
		for i, op := range got {
			if op.GetValue() != want[i] {
				t.Errorf("operand %d = %q, want %q — source order decides the path", i, op.GetValue(), want[i])
			}
		}
	})

	t.Run("any other operator abandons the whole chain", func(t *testing.T) {
		arg := concatOf(meta, concatLit(meta, "/a"), concatLit(meta, "/b"))
		arg.SetValue("*")
		if got := flattenConcat(arg, nil); got != nil {
			t.Errorf("got %v, want nil so the caller falls back to a placeholder", got)
		}
	})
}

// pathNode is a tracker node carrying an edge and a parent, which is what the
// operand resolvers walk.
type pathNode struct {
	edge   *metadata.CallGraphEdge
	arg    *metadata.CallArgument
	parent TrackerNodeInterface
}

func (n *pathNode) GetKey() string                                         { return "" }
func (n *pathNode) GetChildren() []TrackerNodeInterface                    { return nil }
func (n *pathNode) GetCallGraphEdge() *metadata.CallGraphEdge              { return n.edge }
func (n *pathNode) GetCallArgument() *metadata.CallArgument                { return n.arg }
func (n *pathNode) GetArgContext() string                                  { return "" }
func (n *pathNode) GetArgIndex() int                                       { return 0 }
func (n *pathNode) GetArgType() metadata.ArgumentType                      { return metadata.ArgTypeDirectCallee }
func (n *pathNode) GetArgument() *metadata.CallArgument                    { return n.arg }
func (n *pathNode) GetEdge() *metadata.CallGraphEdge                       { return n.edge }
func (n *pathNode) GetParent() TrackerNodeInterface                        { return n.parent }
func (n *pathNode) GetTypeParamMap() map[string]string                     { return nil }
func (n *pathNode) GetRootAssignmentMap() map[string][]metadata.Assignment { return nil }

// concatIdent builds a bare ident.
func concatIdent(meta *metadata.Metadata, name string) *metadata.CallArgument {
	a := metadata.NewCallArgument(meta)
	a.SetKind(metadata.KindIdent)
	a.SetName(name)
	return a
}

// concatSelector builds `base.Field`.
func concatSelector(meta *metadata.Metadata, base, field string) *metadata.CallArgument {
	a := metadata.NewCallArgument(meta)
	a.SetKind(metadata.KindSelector)
	a.X = concatIdent(meta, base)
	a.Sel = concatIdent(meta, field)
	return a
}

// concatComposite builds a composite literal from the given elements.
func concatComposite(meta *metadata.Metadata, elts ...*metadata.CallArgument) *metadata.CallArgument {
	a := metadata.NewCallArgument(meta)
	a.SetKind(metadata.KindCompositeLit)
	a.Args = elts
	return a
}

// concatKeyValue builds `Key: value` for a composite literal.
func concatKeyValue(meta *metadata.Metadata, key string, value *metadata.CallArgument) *metadata.CallArgument {
	a := metadata.NewCallArgument(meta)
	a.SetKind(metadata.KindKeyValue)
	a.X = concatIdent(meta, key)
	a.Fun = value
	return a
}

// nodeWithCallerArg returns a node whose parent binds paramName to arg, the
// shape argViaParent reads when a helper's parameter is resolved to the value
// its caller passed.
func nodeWithCallerArg(meta *metadata.Metadata, paramName string, arg *metadata.CallArgument) TrackerNodeInterface {
	parentEdge := &metadata.CallGraphEdge{
		Caller:      metadata.Call{Meta: meta, Name: meta.StringPool.Get("main"), Pkg: meta.StringPool.Get("app"), RecvType: -1},
		Callee:      metadata.Call{Meta: meta, Name: meta.StringPool.Get("register"), Pkg: meta.StringPool.Get("app"), RecvType: -1},
		ParamArgMap: map[string]metadata.CallArgument{paramName: *arg},
	}
	childEdge := &metadata.CallGraphEdge{
		Caller: metadata.Call{Meta: meta, Name: meta.StringPool.Get("register"), Pkg: meta.StringPool.Get("app"), RecvType: -1},
		Callee: metadata.Call{Meta: meta, Name: meta.StringPool.Get("Post"), Pkg: meta.StringPool.Get("chi"), RecvType: -1},
	}
	return &pathNode{edge: childEdge, parent: &pathNode{edge: parentEdge}}
}

// TestResolveConcatenatedPathWithNode covers the operands that can only be
// resolved from the call site — the ones the fixture exercises end to end.
func TestResolveConcatenatedPathWithNode(t *testing.T) {
	meta := newTestMeta()
	b := &BasePatternMatcher{contextProvider: NewContextProvider(meta)}

	t.Run("a parameter resolves to the caller's literal", func(t *testing.T) {
		node := nodeWithCallerArg(meta, "prefix", concatLit(meta, "/v2"))
		arg := concatOf(meta, concatIdent(meta, "prefix"), concatLit(meta, "/items"))
		if path, dyn := b.resolvePathArg(arg, node); path != "/v2/items" || len(dyn) != 0 {
			t.Errorf("got (%q, %v), want (\"/v2/items\", no placeholders)", path, dyn)
		}
	})

	t.Run("a struct field the caller left unset is the zero value", func(t *testing.T) {
		// The generated shape: `HandlerWithOptions(si, ChiServerOptions{})`.
		node := nodeWithCallerArg(meta, "options", concatComposite(meta))
		arg := concatOf(meta, concatSelector(meta, "options", "BaseURL"), concatLit(meta, "/things"))
		if path, _ := b.resolvePathArg(arg, node); path != "/things" {
			t.Errorf("got %q, want \"/things\" — an unset field contributes nothing", path)
		}
	})

	t.Run("a struct field the caller set resolves to its value", func(t *testing.T) {
		lit := concatComposite(meta, concatKeyValue(meta, "BaseURL", concatLit(meta, "/v2")))
		node := nodeWithCallerArg(meta, "options", lit)
		arg := concatOf(meta, concatSelector(meta, "options", "BaseURL"), concatLit(meta, "/things"))
		if path, _ := b.resolvePathArg(arg, node); path != "/v2/things" {
			t.Errorf("got %q, want \"/v2/things\"", path)
		}
	})

	t.Run("a POSITIONAL literal is not read as all-zero fields", func(t *testing.T) {
		// `ChiServerOptions{"/api", nil}` names no fields, so "absent from the
		// keys" cannot mean the zero value. Concluding "" would silently shorten
		// the path, which is the defect this whole change exists to prevent.
		lit := concatComposite(meta, concatLit(meta, "/api"), concatIdent(meta, "nil"))
		node := nodeWithCallerArg(meta, "options", lit)
		arg := concatOf(meta, concatSelector(meta, "options", "BaseURL"), concatLit(meta, "/things"))
		path, dyn := b.resolvePathArg(arg, node)
		if path != "{BaseURL}/things" {
			t.Errorf("got %q, want \"{BaseURL}/things\" — an unnamed field must degrade to a placeholder", path)
		}
		if len(dyn) != 1 || dyn[0] != "BaseURL" {
			t.Errorf("dynamic names = %v, want [BaseURL]", dyn)
		}
	})

	t.Run("every unresolvable operand is named", func(t *testing.T) {
		// Each placeholder becomes a declared path parameter; reporting only the
		// first would leave the second undeclared in the path template.
		first := metadata.NewCallArgument(meta)
		first.SetKind(metadata.KindCall)
		first.SetName("baseOf")
		second := metadata.NewCallArgument(meta)
		second.SetKind(metadata.KindCall)
		second.SetName("versionOf")

		arg := concatOf(meta, concatOf(meta, first, second), concatLit(meta, "/things"))
		path, dyn := b.resolvePathArg(arg, nil)
		if path != "{baseOf}{versionOf}/things" {
			t.Errorf("got %q, want both placeholders kept", path)
		}
		if len(dyn) != 2 || dyn[0] != "baseOf" || dyn[1] != "versionOf" {
			t.Errorf("dynamic names = %v, want [baseOf versionOf] so both are declared", dyn)
		}
	})
}

// edgeBetween builds a caller -> callee edge.
func edgeBetween(meta *metadata.Metadata, callerPkg, callerName, calleePkg, calleeName string) *metadata.CallGraphEdge {
	sp := meta.StringPool
	return &metadata.CallGraphEdge{
		Caller: metadata.Call{Meta: meta, Name: sp.Get(callerName), Pkg: sp.Get(callerPkg), RecvType: -1},
		Callee: metadata.Call{Meta: meta, Name: sp.Get(calleeName), Pkg: sp.Get(calleePkg), RecvType: -1},
	}
}

// TestConcatPathResolutionEdges covers the paths where an operand CANNOT be
// resolved. They matter more than the happy ones: every one of them has to end
// at a placeholder, because a resolver that guesses produces a path the handler
// does not serve (golden rule #7).
func TestConcatPathResolutionEdges(t *testing.T) {
	meta := newTestMeta()
	b := &BasePatternMatcher{contextProvider: NewContextProvider(meta)}

	t.Run("a nil argument resolves to nothing", func(t *testing.T) {
		if path, dyn := b.resolvePathArg(nil, nil); path != "" || dyn != nil {
			t.Errorf("got (%q, %v), want (\"\", nil)", path, dyn)
		}
	})

	t.Run("an unnamed operand falls back to the generic name", func(t *testing.T) {
		anon := metadata.NewCallArgument(meta)
		anon.SetKind(metadata.KindCall)
		if path, _ := b.resolvePathArg(concatOf(meta, anon, concatLit(meta, "/x")), nil); path != "{path}/x" {
			t.Errorf("got %q, want \"{path}/x\"", path)
		}
	})

	t.Run("unwrapComposite looks through & and *", func(t *testing.T) {
		lit := concatComposite(meta)
		ref := metadata.NewCallArgument(meta)
		ref.SetKind(metadata.KindUnary)
		ref.X = lit
		if got := unwrapComposite(ref); got != lit {
			t.Error("&T{...} must unwrap to the literal")
		}
		if got := unwrapComposite(nil); got != nil {
			t.Error("nil must stay nil")
		}
	})

	t.Run("a selector on something that is not a composite literal is unresolved", func(t *testing.T) {
		node := nodeWithCallerArg(meta, "options", concatIdent(meta, "elsewhere"))
		arg := concatOf(meta, concatSelector(meta, "options", "BaseURL"), concatLit(meta, "/things"))
		if path, _ := b.resolvePathArg(arg, node); path != "{BaseURL}/things" {
			t.Errorf("got %q, want the field to degrade to a placeholder", path)
		}
	})

	t.Run("a field set to a non-constant is unresolved", func(t *testing.T) {
		lit := concatComposite(meta, concatKeyValue(meta, "BaseURL", concatIdent(meta, "runtimeValue")))
		node := nodeWithCallerArg(meta, "options", lit)
		arg := concatOf(meta, concatSelector(meta, "options", "BaseURL"), concatLit(meta, "/things"))
		if path, _ := b.resolvePathArg(arg, node); path != "{BaseURL}/things" {
			t.Errorf("got %q, want a placeholder: the field is set, but to a value we cannot evaluate", path)
		}
	})

	t.Run("a selector with no field name is unresolved", func(t *testing.T) {
		sel := metadata.NewCallArgument(meta)
		sel.SetKind(metadata.KindSelector)
		sel.X = concatIdent(meta, "options")
		sel.Sel = concatIdent(meta, "")
		if _, ok := b.structFieldValue(sel, nodeWithCallerArg(meta, "options", concatComposite(meta))); ok {
			t.Error("a nameless field cannot resolve")
		}
	})

	t.Run("call sites that disagree leave the value ambiguous", func(t *testing.T) {
		// Two callers passing different prefixes: neither is THE value, so the
		// honest answer is the placeholder rather than whichever comes first.
		first := edgeBetween(meta, "app", "main", "app", "register")
		first.ParamArgMap = map[string]metadata.CallArgument{"prefix": *concatLit(meta, "/a")}
		second := edgeBetween(meta, "app", "other", "app", "register")
		second.ParamArgMap = map[string]metadata.CallArgument{"prefix": *concatLit(meta, "/b")}
		meta.Callees = map[string][]*metadata.CallGraphEdge{"app.register": {first, second}}
		defer func() { meta.Callees = nil }()

		node := &pathNode{edge: edgeBetween(meta, "app", "register", "chi", "Post")}
		arg := concatOf(meta, concatIdent(meta, "prefix"), concatLit(meta, "/items"))
		if path, _ := b.resolvePathArg(arg, node); path != "{prefix}/items" {
			t.Errorf("got %q, want \"{prefix}/items\" — disagreeing callers are ambiguous", path)
		}
	})

	t.Run("a single call site resolves the parameter", func(t *testing.T) {
		only := edgeBetween(meta, "app", "main", "app", "register")
		only.ParamArgMap = map[string]metadata.CallArgument{"prefix": *concatLit(meta, "/v3")}
		meta.Callees = map[string][]*metadata.CallGraphEdge{"app.register": {only}}
		defer func() { meta.Callees = nil }()

		node := &pathNode{edge: edgeBetween(meta, "app", "register", "chi", "Post")}
		arg := concatOf(meta, concatIdent(meta, "prefix"), concatLit(meta, "/items"))
		if path, _ := b.resolvePathArg(arg, node); path != "/v3/items" {
			t.Errorf("got %q, want \"/v3/items\"", path)
		}
	})

	t.Run("a call site that does not bind the parameter is unresolved", func(t *testing.T) {
		bare := edgeBetween(meta, "app", "main", "app", "register")
		meta.Callees = map[string][]*metadata.CallGraphEdge{"app.register": {bare}}
		defer func() { meta.Callees = nil }()

		node := &pathNode{edge: edgeBetween(meta, "app", "register", "chi", "Post")}
		arg := concatOf(meta, concatIdent(meta, "prefix"), concatLit(meta, "/items"))
		if path, _ := b.resolvePathArg(arg, node); path != "{prefix}/items" {
			t.Errorf("got %q, want a placeholder", path)
		}
	})

	t.Run("no call sites at all leaves the parameter unresolved", func(t *testing.T) {
		node := &pathNode{edge: edgeBetween(meta, "app", "orphan", "chi", "Post")}
		arg := concatOf(meta, concatIdent(meta, "prefix"), concatLit(meta, "/items"))
		if path, _ := b.resolvePathArg(arg, node); path != "{prefix}/items" {
			t.Errorf("got %q, want a placeholder", path)
		}
	})

	t.Run("enclosingFuncID prefers the function that defines a closure", func(t *testing.T) {
		if got := enclosingFuncID(&pathNode{}); got != "" {
			t.Errorf("a node with no edge names no function, got %q", got)
		}
		edge := edgeBetween(meta, "app", "FuncLit:main.go:10:9", "chi", "Post")
		if got := enclosingFuncID(&pathNode{edge: edge}); got != "app.FuncLit:main.go:10:9" {
			t.Errorf("got %q, want the caller when there is no parent function", got)
		}
		edge.ParentFunction = &metadata.Call{Meta: meta, Name: meta.StringPool.Get("Register"), Pkg: meta.StringPool.Get("app"), RecvType: -1}
		if got := enclosingFuncID(&pathNode{edge: edge}); got != "app.Register" {
			t.Errorf("got %q, want the defining function", got)
		}
	})

	t.Run("paramArgOfEnclosingFunc needs a frame for that function", func(t *testing.T) {
		edge := edgeBetween(meta, "app", "FuncLit:main.go:10:9", "chi", "Post")
		edge.ParentFunction = &metadata.Call{Meta: meta, Name: meta.StringPool.Get("Register"), Pkg: meta.StringPool.Get("app"), RecvType: -1}

		// An ancestor that is not the defining function's call is not the frame.
		unrelated := &pathNode{edge: edgeBetween(meta, "app", "main", "app", "somethingElse")}
		if _, ok := paramArgOfEnclosingFunc(concatIdent(meta, "options"), &pathNode{edge: edge, parent: unrelated}); ok {
			t.Error("resolved from an unrelated frame")
		}

		// No edge, and a non-ident argument, resolve nothing.
		if _, ok := paramArgOfEnclosingFunc(concatIdent(meta, "options"), &pathNode{}); ok {
			t.Error("a node with no edge has no frame")
		}
		if _, ok := paramArgOfEnclosingFunc(concatLit(meta, "/x"), &pathNode{edge: edge}); ok {
			t.Error("only an ident names a parameter")
		}
	})
}

// stubContextProvider is a ContextProvider that is NOT the standard
// implementation, so metadata() cannot reach a call graph through it.
type stubContextProvider struct{ ContextProvider }

// TestConcatPathHelpers covers the small helpers directly, including the
// branches the resolver only reaches through code paths another test already
// exercises from the outside.
func TestConcatPathHelpers(t *testing.T) {
	meta := newTestMeta()
	b := &BasePatternMatcher{contextProvider: NewContextProvider(meta)}

	t.Run("no name means no placeholder is declared", func(t *testing.T) {
		if got := dynamicNameList(""); got != nil {
			t.Errorf("got %v, want nil: nothing was synthesized, so nothing is declared", got)
		}
		if got := dynamicNameList("base"); len(got) != 1 || got[0] != "base" {
			t.Errorf("got %v, want [base]", got)
		}
	})

	t.Run("a nil operand contributes nothing", func(t *testing.T) {
		if value, name := b.resolvePathOperand(nil, nil, 0); value != "" || name != "" {
			t.Errorf("got (%q, %q), want empties", value, name)
		}
	})

	t.Run("a bad operator anywhere abandons the whole chain", func(t *testing.T) {
		// The nested chain is walked left first, so a rejection there has to
		// propagate rather than being retried on the right.
		bad := concatOf(meta, concatLit(meta, "/a"), concatLit(meta, "/b"))
		bad.SetValue("%")
		if got := flattenConcat(concatOf(meta, bad, concatLit(meta, "/c")), nil); got != nil {
			t.Errorf("got %v, want nil", got)
		}
	})

	t.Run("a nil element in a literal is skipped, not read as a field", func(t *testing.T) {
		lit := concatComposite(meta, nil, concatKeyValue(meta, "BaseURL", concatLit(meta, "/v9")))
		node := nodeWithCallerArg(meta, "options", lit)
		if v, ok := b.structFieldValue(concatSelector(meta, "options", "BaseURL"), node); !ok || v != "/v9" {
			t.Errorf("got (%q, %v), want (\"/v9\", true)", v, ok)
		}
	})

	t.Run("without the standard provider there is no call graph to consult", func(t *testing.T) {
		stub := &BasePatternMatcher{contextProvider: &stubContextProvider{}}
		if stub.metadata() != nil {
			t.Error("a foreign provider exposes no metadata")
		}
		node := &pathNode{edge: edgeBetween(meta, "app", "register", "chi", "Post")}
		if _, ok := stub.uniqueCallerArg(concatIdent(meta, "prefix"), node); ok {
			t.Error("no metadata means no call sites to agree")
		}
	})

	t.Run("an unnamed parameter resolves nothing", func(t *testing.T) {
		node := &pathNode{edge: edgeBetween(meta, "app", "register", "chi", "Post")}
		if _, ok := b.uniqueCallerArg(concatIdent(meta, ""), node); ok {
			t.Error("a nameless ident cannot be looked up")
		}
		if _, ok := b.uniqueCallerArg(concatIdent(meta, "prefix"), nil); ok {
			t.Error("a nil node names no enclosing function")
		}
	})

	t.Run("a parameter captured by a closure resolves through the defining frame", func(t *testing.T) {
		// The positive path of paramArgOfEnclosingFunc: the ancestor whose callee
		// is the function that DEFINES the closure carries the binding.
		lit := concatComposite(meta, concatKeyValue(meta, "BaseURL", concatLit(meta, "/v4")))
		defining := edgeBetween(meta, "app", "main", "app", "Register")
		defining.ParamArgMap = map[string]metadata.CallArgument{"options": *lit}

		inner := edgeBetween(meta, "app", "FuncLit:main.go:10:9", "chi", "Post")
		inner.ParentFunction = &metadata.Call{Meta: meta, Name: meta.StringPool.Get("Register"), Pkg: meta.StringPool.Get("app"), RecvType: -1}

		node := &pathNode{edge: inner, parent: &pathNode{edge: defining}}
		arg := concatOf(meta, concatSelector(meta, "options", "BaseURL"), concatLit(meta, "/things"))
		if path, _ := b.resolvePathArg(arg, node); path != "/v4/things" {
			t.Errorf("got %q, want \"/v4/things\"", path)
		}
	})
}

// TestConcatPathRemainingBranches closes the last resolution branches: the ones
// where a frame exists but does not bind the name, and where a literal sets a
// field that is not the one being read.
func TestConcatPathRemainingBranches(t *testing.T) {
	meta := newTestMeta()
	b := &BasePatternMatcher{contextProvider: NewContextProvider(meta)}

	t.Run("the defining frame exists but binds no such parameter", func(t *testing.T) {
		defining := edgeBetween(meta, "app", "main", "app", "Register")
		defining.ParamArgMap = map[string]metadata.CallArgument{"other": *concatLit(meta, "/v5")}

		inner := edgeBetween(meta, "app", "FuncLit:main.go:10:9", "chi", "Post")
		inner.ParentFunction = &metadata.Call{Meta: meta, Name: meta.StringPool.Get("Register"), Pkg: meta.StringPool.Get("app"), RecvType: -1}

		node := &pathNode{edge: inner, parent: &pathNode{edge: defining}}
		if _, ok := paramArgOfEnclosingFunc(concatIdent(meta, "options"), node); ok {
			t.Error("a frame that does not bind the name resolves nothing")
		}
	})

	t.Run("the defining frame has no parameter bindings at all", func(t *testing.T) {
		defining := edgeBetween(meta, "app", "main", "app", "Register")

		inner := edgeBetween(meta, "app", "FuncLit:main.go:10:9", "chi", "Post")
		inner.ParentFunction = &metadata.Call{Meta: meta, Name: meta.StringPool.Get("Register"), Pkg: meta.StringPool.Get("app"), RecvType: -1}

		node := &pathNode{edge: inner, parent: &pathNode{edge: defining}}
		if _, ok := paramArgOfEnclosingFunc(concatIdent(meta, "options"), node); ok {
			t.Error("a frame with no ParamArgMap resolves nothing")
		}
	})

	t.Run("a literal that sets other fields still yields the zero value", func(t *testing.T) {
		lit := concatComposite(meta, concatKeyValue(meta, "Timeout", concatLit(meta, "30")))
		node := nodeWithCallerArg(meta, "options", lit)
		if v, ok := b.structFieldValue(concatSelector(meta, "options", "BaseURL"), node); !ok || v != "" {
			t.Errorf("got (%q, %v), want (\"\", true): the field is unset, so it is the zero value", v, ok)
		}
	})

	t.Run("a call site binding a non-constant is unresolved", func(t *testing.T) {
		// The caller passes something computed at run time, so no path is known.
		only := edgeBetween(meta, "app", "main", "app", "register")
		only.ParamArgMap = map[string]metadata.CallArgument{"prefix": *concatIdent(meta, "computed")}
		meta.Callees = map[string][]*metadata.CallGraphEdge{"app.register": {only}}
		defer func() { meta.Callees = nil }()

		node := &pathNode{edge: edgeBetween(meta, "app", "register", "chi", "Post")}
		arg := concatOf(meta, concatIdent(meta, "prefix"), concatLit(meta, "/items"))
		if path, _ := b.resolvePathArg(arg, node); path != "{prefix}/items" {
			t.Errorf("got %q, want a placeholder", path)
		}
	})

	t.Run("a parent function with no identity resolves nothing", func(t *testing.T) {
		inner := edgeBetween(meta, "app", "FuncLit:main.go:10:9", "chi", "Post")
		inner.ParentFunction = &metadata.Call{Meta: meta, Name: -1, Pkg: -1, RecvType: -1}
		node := &pathNode{edge: inner, parent: &pathNode{edge: edgeBetween(meta, "app", "main", "app", "Register")}}
		if _, ok := paramArgOfEnclosingFunc(concatIdent(meta, "options"), node); ok {
			t.Error("an unnamed parent function names no frame")
		}
	})

	t.Run("call sites that agree on the same value resolve", func(t *testing.T) {
		// Two callers passing the SAME prefix is not ambiguity — the value is
		// determined, so the path resolves rather than degrading.
		first := edgeBetween(meta, "app", "main", "app", "register")
		first.ParamArgMap = map[string]metadata.CallArgument{"prefix": *concatLit(meta, "/same")}
		second := edgeBetween(meta, "app", "other", "app", "register")
		second.ParamArgMap = map[string]metadata.CallArgument{"prefix": *concatLit(meta, "/same")}
		meta.Callees = map[string][]*metadata.CallGraphEdge{"app.register": {first, second}}
		defer func() { meta.Callees = nil }()

		node := &pathNode{edge: edgeBetween(meta, "app", "register", "chi", "Post")}
		arg := concatOf(meta, concatIdent(meta, "prefix"), concatLit(meta, "/items"))
		if path, _ := b.resolvePathArg(arg, node); path != "/same/items" {
			t.Errorf("got %q, want \"/same/items\"", path)
		}
	})
}
