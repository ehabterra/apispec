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
