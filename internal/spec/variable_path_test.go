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
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/ehabterra/apispec/internal/metadata"
)

// assignTo builds one recorded assignment `name = <value>`.
func assignTo(meta *metadata.Metadata, name string, value *metadata.CallArgument) metadata.Assignment {
	return metadata.Assignment{
		VariableName: meta.StringPool.Get(name),
		Value:        *value,
	}
}

// nodeWithAssignments returns a node whose edge records the given assignments,
// which is the fallback scope pathVarAssignments reads when the metadata holds
// no enclosing function.
func nodeWithAssignments(meta *metadata.Metadata, assigns map[string][]metadata.Assignment) TrackerNodeInterface {
	return &pathNode{edge: &metadata.CallGraphEdge{
		Caller:        metadata.Call{Meta: meta, Name: meta.StringPool.Get("main"), Pkg: meta.StringPool.Get("app"), RecvType: -1},
		Callee:        metadata.Call{Meta: meta, Name: meta.StringPool.Get("HandleFunc"), Pkg: meta.StringPool.Get("net/http"), RecvType: -1},
		AssignmentMap: assigns,
	}}
}

// TestVariableValue covers reading a path out of a local variable (issue #431).
// Before it, `p := "/users"` two lines above the registration was treated as
// unreadable — reported and left out by #428, or rendered as the variable's
// TYPE, which put `/string` in the document as if it were an endpoint.
func TestVariableValue(t *testing.T) {
	meta := newTestMeta()
	b := &BasePatternMatcher{contextProvider: NewContextProvider(meta)}

	t.Run("one literal assignment resolves", func(t *testing.T) {
		node := nodeWithAssignments(meta, map[string][]metadata.Assignment{
			"p": {assignTo(meta, "p", concatLit(meta, "/users"))},
		})
		if v, ok := b.variableValue(concatIdent(meta, "p"), node, 0); !ok || v != "/users" {
			t.Errorf("got (%q, %v), want (\"/users\", true)", v, ok)
		}
	})

	t.Run("an alias chain is followed", func(t *testing.T) {
		node := nodeWithAssignments(meta, map[string][]metadata.Assignment{
			"first":  {assignTo(meta, "first", concatLit(meta, "/aliased"))},
			"second": {assignTo(meta, "second", concatIdent(meta, "first"))},
		})
		if v, ok := b.variableValue(concatIdent(meta, "second"), node, 0); !ok || v != "/aliased" {
			t.Errorf("got (%q, %v), want (\"/aliased\", true)", v, ok)
		}
	})

	t.Run("a value assembled from resolvable parts resolves", func(t *testing.T) {
		node := nodeWithAssignments(meta, map[string][]metadata.Assignment{
			"root":   {assignTo(meta, "root", concatLit(meta, "/v2"))},
			"joined": {assignTo(meta, "joined", concatOf(meta, concatIdent(meta, "root"), concatLit(meta, "/joined")))},
		})
		if v, ok := b.variableValue(concatIdent(meta, "joined"), node, 0); !ok || v != "/v2/joined" {
			t.Errorf("got (%q, %v), want (\"/v2/joined\", true)", v, ok)
		}
	})

	t.Run("two assignments that disagree resolve to nothing", func(t *testing.T) {
		// Branches assigning different paths is real ambiguity: taking the
		// latest would document one branch's route as the endpoint.
		node := nodeWithAssignments(meta, map[string][]metadata.Assignment{
			"amb": {
				assignTo(meta, "amb", concatLit(meta, "/first")),
				assignTo(meta, "amb", concatLit(meta, "/second")),
			},
		})
		if v, ok := b.variableValue(concatIdent(meta, "amb"), node, 0); ok {
			t.Errorf("want no value for an ambiguous variable, got %q", v)
		}
	})

	t.Run("two assignments that agree resolve", func(t *testing.T) {
		node := nodeWithAssignments(meta, map[string][]metadata.Assignment{
			"same": {
				assignTo(meta, "same", concatLit(meta, "/same")),
				assignTo(meta, "same", concatLit(meta, "/same")),
			},
		})
		if v, ok := b.variableValue(concatIdent(meta, "same"), node, 0); !ok || v != "/same" {
			t.Errorf("got (%q, %v), want (\"/same\", true)", v, ok)
		}
	})

	t.Run("a value assigned from a call resolves to nothing", func(t *testing.T) {
		call := metadata.NewCallArgument(meta)
		call.SetKind(metadata.KindCall)
		call.SetName("buildPath")
		node := nodeWithAssignments(meta, map[string][]metadata.Assignment{
			"built": {assignTo(meta, "built", call)},
		})
		if v, ok := b.variableValue(concatIdent(meta, "built"), node, 0); ok {
			t.Errorf("a runtime-built path must stay unresolved, got %q", v)
		}
	})

	t.Run("a partly-resolvable value resolves to nothing", func(t *testing.T) {
		// Accepting it would report the placeholder as what the variable holds.
		call := metadata.NewCallArgument(meta)
		call.SetKind(metadata.KindCall)
		call.SetName("dynamic")
		node := nodeWithAssignments(meta, map[string][]metadata.Assignment{
			"mixed": {assignTo(meta, "mixed", concatOf(meta, call, concatLit(meta, "/tail")))},
		})
		if v, ok := b.variableValue(concatIdent(meta, "mixed"), node, 0); ok {
			t.Errorf("want no value when part of it is unreadable, got %q", v)
		}
	})

	t.Run("a self-referential alias terminates", func(t *testing.T) {
		node := nodeWithAssignments(meta, map[string][]metadata.Assignment{
			"loop": {assignTo(meta, "loop", concatIdent(meta, "loop"))},
		})
		if v, ok := b.variableValue(concatIdent(meta, "loop"), node, 0); ok {
			t.Errorf("want no value for a cyclic alias, got %q", v)
		}
	})

	t.Run("guards", func(t *testing.T) {
		node := nodeWithAssignments(meta, nil)
		if _, ok := b.variableValue(nil, node, 0); ok {
			t.Error("nil argument must not resolve")
		}
		if _, ok := b.variableValue(concatIdent(meta, "p"), nil, 0); ok {
			t.Error("no node means no call site to read assignments at")
		}
		if _, ok := b.variableValue(concatLit(meta, "/x"), node, 0); ok {
			t.Error("only an identifier names a variable")
		}
		if _, ok := b.variableValue(concatIdent(meta, "unknown"), node, 0); ok {
			t.Error("a variable with no recorded assignment must not resolve")
		}
		if _, ok := b.variableValue(concatIdent(meta, "p"), node, maxPathVarDepth); ok {
			t.Error("the depth bound must stop the walk")
		}
	})
}

// TestPathVarAssignmentsPrefersFunctionScope pins why this lookup exists rather
// than reusing assignmentsAt: the edge's map answers with the assignment in
// EFFECT at the call, so a variable reassigned in a branch looks unambiguous
// there. Measured on `amb := "/one"; if … { amb = "/two" }` — the edge map
// yielded "/two" alone, and the route was documented at it.
func TestPathVarAssignmentsPrefersFunctionScope(t *testing.T) {
	meta := newTestMeta()
	both := []metadata.Assignment{
		assignTo(meta, "amb", concatLit(meta, "/one")),
		assignTo(meta, "amb", concatLit(meta, "/two")),
	}
	meta.Packages = map[string]*metadata.Package{
		"app": {Files: map[string]*metadata.File{
			"main.go": {Functions: map[string]*metadata.Function{
				"main": {AssignmentMap: map[string][]metadata.Assignment{"amb": both}},
			}},
		}},
	}
	edge := &metadata.CallGraphEdge{
		Caller: metadata.Call{Meta: meta, Name: meta.StringPool.Get("main"), Pkg: meta.StringPool.Get("app"), RecvType: -1},
		Callee: metadata.Call{Meta: meta, Name: meta.StringPool.Get("HandleFunc"), Pkg: meta.StringPool.Get("net/http"), RecvType: -1},
		// The fast path an ordinary resolver would take: the latest only.
		AssignmentMap: map[string][]metadata.Assignment{"amb": {both[1]}},
	}

	cp := NewContextProvider(meta)
	if got := len(pathVarAssignments(cp, edge, "amb")); got != 2 {
		t.Fatalf("want both assignments from the function scope, got %d", got)
	}
	if got := len(assignmentsAt(cp, edge, "amb")); got != 1 {
		t.Fatalf("assignmentsAt is expected to answer with the effective one; got %d — "+
			"if this changed, pathVarAssignments may no longer be needed", got)
	}

	b := &BasePatternMatcher{contextProvider: cp}
	if v, ok := b.variableValue(concatIdent(meta, "amb"), &pathNode{edge: edge}, 0); ok {
		t.Errorf("the ambiguity must survive the lookup, got %q", v)
	}
}

// TestConversionOperand covers reading through a type conversion (issue #433).
// A conversion changes the TYPE and never the string inside it, but rendering
// one yields its TARGET TYPE — right for a response body, whose type is the
// question, and wrong for a path, whose value is. That is how a phantom
// `/string/…` reached the document.
func TestConversionOperand(t *testing.T) {
	meta := newTestMeta()

	conv := func(typeName string, inner *metadata.CallArgument) *metadata.CallArgument {
		a := metadata.NewCallArgument(meta)
		a.SetKind(metadata.KindTypeConversion)
		a.Fun = concatIdent(meta, typeName)
		if inner != nil {
			a.Args = []*metadata.CallArgument{inner}
		}
		return a
	}

	t.Run("the wrapped value is returned", func(t *testing.T) {
		inner, ok := conversionOperand(conv("string", concatLit(meta, "/named")))
		if !ok || inner == nil || inner.GetValue() != `"/named"` {
			t.Errorf("got (%+v, %v), want the wrapped literal", inner, ok)
		}
	})

	t.Run("only a conversion with exactly one operand", func(t *testing.T) {
		if _, ok := conversionOperand(nil); ok {
			t.Error("nil is not a conversion")
		}
		if _, ok := conversionOperand(concatIdent(meta, "prefix")); ok {
			t.Error("an identifier is not a conversion")
		}
		if _, ok := conversionOperand(conv("string", nil)); ok {
			t.Error("a conversion with no operand resolves to nothing")
		}
	})

	b := &BasePatternMatcher{contextProvider: NewContextProvider(meta)}

	t.Run("a converted literal resolves as the literal", func(t *testing.T) {
		if path, dyn := b.resolvePathArg(conv("RoutePrefix", concatLit(meta, "/named")), nil); path != "/named" || len(dyn) != 0 {
			t.Errorf("got (%q, %v), want (\"/named\", no placeholders)", path, dyn)
		}
	})

	t.Run("nested conversions resolve", func(t *testing.T) {
		arg := conv("string", conv("RoutePrefix", concatLit(meta, "/deep")))
		if path, _ := b.resolvePathArg(arg, nil); path != "/deep" {
			t.Errorf("got %q, want \"/deep\"", path)
		}
	})

	t.Run("a converted variable resolves through its assignments", func(t *testing.T) {
		node := nodeWithAssignments(meta, map[string][]metadata.Assignment{
			"p": {assignTo(meta, "p", concatLit(meta, "/converted"))},
		})
		if path, _ := b.resolvePathArg(conv("string", concatIdent(meta, "p")), node); path != "/converted" {
			t.Errorf("got %q, want \"/converted\"", path)
		}
	})

	t.Run("an assignment through a conversion resolves", func(t *testing.T) {
		// `p := RoutePrefix("/x")` holds the string, not the type.
		node := nodeWithAssignments(meta, map[string][]metadata.Assignment{
			"p": {assignTo(meta, "p", conv("RoutePrefix", concatLit(meta, "/assigned")))},
		})
		if v, ok := b.variableValue(concatIdent(meta, "p"), node, 0); !ok || v != "/assigned" {
			t.Errorf("got (%q, %v), want (\"/assigned\", true)", v, ok)
		}
	})

	t.Run("an unreadable conversion becomes a placeholder named after its value", func(t *testing.T) {
		// Never named after the TYPE: that is the phantom this replaces.
		call := metadata.NewCallArgument(meta)
		call.SetKind(metadata.KindCall)
		call.SetName("buildPath")
		path, dyn := b.resolvePathArg(conv("string", call), nil)
		if path != "{buildPath}" || len(dyn) != 1 || dyn[0] != "buildPath" {
			t.Errorf("got (%q, %v), want a placeholder named after the wrapped call", path, dyn)
		}
	})
}

// TestParamValueFromCallSites covers the last rung of the path ladder: a
// parameter read from the CALL SITES of the function the registration is
// written in (issue #433).
//
// The tree answers this better when it can, and for the shape that prompted
// this it cannot — the walk reaches the registration under a different
// helper's frame, so there is no binding for the parameter there. Reading the
// call graph is frame-blind, so it may only answer when every caller agrees.
func TestParamValueFromCallSites(t *testing.T) {
	// helperNode returns a node whose edge is a call written INSIDE `helper`,
	// with metadata recording the given calls to helper.
	helperNode := func(meta *metadata.Metadata, calls ...map[string]metadata.CallArgument) TrackerNodeInterface {
		helper := metadata.Call{Meta: meta, Name: meta.StringPool.Get("helper"), Pkg: meta.StringPool.Get("app"), RecvType: -1}
		meta.Callees = map[string][]*metadata.CallGraphEdge{}
		for _, bound := range calls {
			meta.Callees[helper.BaseID()] = append(meta.Callees[helper.BaseID()], &metadata.CallGraphEdge{
				Caller:      metadata.Call{Meta: meta, Name: meta.StringPool.Get("main"), Pkg: meta.StringPool.Get("app"), RecvType: -1},
				Callee:      helper,
				ParamArgMap: bound,
			})
		}
		return &pathNode{edge: &metadata.CallGraphEdge{
			Caller: helper,
			Callee: metadata.Call{Meta: meta, Name: meta.StringPool.Get("Mount"), Pkg: meta.StringPool.Get("chi"), RecvType: -1},
		}}
	}

	t.Run("one caller's literal resolves", func(t *testing.T) {
		meta := newTestMeta()
		b := &BasePatternMatcher{contextProvider: NewContextProvider(meta)}
		node := helperNode(meta, map[string]metadata.CallArgument{"prefix": *concatLit(meta, "/named")})
		if v, ok := b.paramValueFromCallSites(concatIdent(meta, "prefix"), node, 0); !ok || v != "/named" {
			t.Errorf("got (%q, %v), want (\"/named\", true)", v, ok)
		}
	})

	t.Run("a converted literal at the call site resolves", func(t *testing.T) {
		meta := newTestMeta()
		b := &BasePatternMatcher{contextProvider: NewContextProvider(meta)}
		conv := metadata.NewCallArgument(meta)
		conv.SetKind(metadata.KindTypeConversion)
		conv.Fun = concatIdent(meta, "RoutePrefix")
		conv.Args = []*metadata.CallArgument{concatLit(meta, "/named")}
		node := helperNode(meta, map[string]metadata.CallArgument{"prefix": *conv})
		if v, ok := b.paramValueFromCallSites(concatIdent(meta, "prefix"), node, 0); !ok || v != "/named" {
			t.Errorf("got (%q, %v), want (\"/named\", true)", v, ok)
		}
	})

	t.Run("callers that agree resolve", func(t *testing.T) {
		meta := newTestMeta()
		b := &BasePatternMatcher{contextProvider: NewContextProvider(meta)}
		node := helperNode(meta,
			map[string]metadata.CallArgument{"prefix": *concatLit(meta, "/same")},
			map[string]metadata.CallArgument{"prefix": *concatLit(meta, "/same")},
		)
		if v, ok := b.paramValueFromCallSites(concatIdent(meta, "prefix"), node, 0); !ok || v != "/same" {
			t.Errorf("got (%q, %v), want (\"/same\", true)", v, ok)
		}
	})

	t.Run("callers that disagree resolve to nothing", func(t *testing.T) {
		// Two helpers mounted at different prefixes: adopting one would document
		// the other's routes at the wrong path.
		meta := newTestMeta()
		b := &BasePatternMatcher{contextProvider: NewContextProvider(meta)}
		node := helperNode(meta,
			map[string]metadata.CallArgument{"prefix": *concatLit(meta, "/one")},
			map[string]metadata.CallArgument{"prefix": *concatLit(meta, "/two")},
		)
		if v, ok := b.paramValueFromCallSites(concatIdent(meta, "prefix"), node, 0); ok {
			t.Errorf("want no value when callers disagree, got %q", v)
		}
	})

	t.Run("a caller that does not bind the name resolves to nothing", func(t *testing.T) {
		meta := newTestMeta()
		b := &BasePatternMatcher{contextProvider: NewContextProvider(meta)}
		node := helperNode(meta,
			map[string]metadata.CallArgument{"prefix": *concatLit(meta, "/one")},
			map[string]metadata.CallArgument{"other": *concatLit(meta, "/two")},
		)
		if v, ok := b.paramValueFromCallSites(concatIdent(meta, "prefix"), node, 0); ok {
			t.Errorf("want no value when a caller binds nothing for it, got %q", v)
		}
	})

	t.Run("a value whose meaning needs a frame resolves to nothing", func(t *testing.T) {
		// Another parameter, a local, or a call: this rung has no frame to
		// evaluate them in, so it declines rather than guessing.
		meta := newTestMeta()
		b := &BasePatternMatcher{contextProvider: NewContextProvider(meta)}
		node := helperNode(meta, map[string]metadata.CallArgument{"prefix": *concatIdent(meta, "outer")})
		if v, ok := b.paramValueFromCallSites(concatIdent(meta, "prefix"), node, 0); ok {
			t.Errorf("want no value for a frame-dependent argument, got %q", v)
		}
	})

	t.Run("guards", func(t *testing.T) {
		meta := newTestMeta()
		b := &BasePatternMatcher{contextProvider: NewContextProvider(meta)}
		node := helperNode(meta, map[string]metadata.CallArgument{"prefix": *concatLit(meta, "/x")})
		if _, ok := b.paramValueFromCallSites(nil, node, 0); ok {
			t.Error("nil argument must not resolve")
		}
		if _, ok := b.paramValueFromCallSites(concatIdent(meta, "prefix"), nil, 0); ok {
			t.Error("no node means no enclosing function to read call sites of")
		}
		if _, ok := b.paramValueFromCallSites(concatIdent(meta, "prefix"), node, maxPathVarDepth); ok {
			t.Error("the depth bound must stop the walk")
		}
		if _, ok := b.paramValueFromCallSites(concatIdent(meta, "unknown"), node, 0); ok {
			t.Error("a name no caller binds must not resolve")
		}
	})
}

// TestAssignmentsReaching covers which writes can be the value at a
// registration (issue #436).
//
// The rule is not source order: a loop runs the body again, so a write BELOW
// the call is live on the next iteration. What decides is whether the write and
// the call share an enclosing region.
func TestAssignmentsReaching(t *testing.T) {
	const file = "main.go"
	// blocks: a loop body spanning lines 20-24.
	meta := newTestMeta()
	meta.Packages = map[string]*metadata.Package{
		"app": {Files: map[string]*metadata.File{
			file: {Functions: map[string]*metadata.Function{
				"main": {
					Position: meta.StringPool.Get(file + ":1:1"),
					EndLine:  40,
					Blocks: []metadata.Block{{
						Kind:      metadata.BlockLoop,
						StartLine: 20, StartCol: 2,
						EndLine: 24, EndCol: 3,
					}},
				},
			}},
		}},
	}
	cp := NewContextProvider(meta)
	b := &BasePatternMatcher{contextProvider: cp}

	// nodeAt returns a node whose call site is at this position.
	nodeAt := func(line, col int) TrackerNodeInterface {
		callee := metadata.Call{Meta: meta, Name: meta.StringPool.Get("HandleFunc"),
			Pkg: meta.StringPool.Get("net/http"), RecvType: -1,
			Position: meta.StringPool.Get(fmt.Sprintf("%s:%d:%d", file, line, col))}
		return &pathNode{edge: &metadata.CallGraphEdge{
			Caller: metadata.Call{Meta: meta, Name: meta.StringPool.Get("main"), Pkg: meta.StringPool.Get("app"), RecvType: -1},
			Callee: callee,
		}}
	}
	assignAt := func(line, col int, value string) metadata.Assignment {
		a := assignTo(meta, "p", concatLit(meta, value))
		a.Position = meta.StringPool.Get(fmt.Sprintf("%s:%d:%d", file, line, col))
		return a
	}
	values := func(assigns []metadata.Assignment) []string {
		out := make([]string, 0, len(assigns))
		for i := range assigns {
			out = append(out, strings.Trim(assigns[i].Value.GetValue(), `"`))
		}
		sort.Strings(out)
		return out
	}

	t.Run("a straight-line write below the call is dropped", func(t *testing.T) {
		assigns := []metadata.Assignment{assignAt(10, 2, "/first"), assignAt(30, 2, "/second")}
		got := values(b.assignmentsReaching(assigns, nodeAt(15, 2)))
		if len(got) != 1 || got[0] != "/first" {
			t.Errorf("reaching = %v, want [/first]", got)
		}
	})

	t.Run("a write below the call inside the same loop is kept", func(t *testing.T) {
		// `for { register(p); p = next(p) }` — the write is live on every
		// iteration after the first, so the path stays ambiguous.
		assigns := []metadata.Assignment{assignAt(10, 2, "/first"), assignAt(23, 3, "/next")}
		got := values(b.assignmentsReaching(assigns, nodeAt(21, 3)))
		if len(got) != 2 {
			t.Errorf("reaching = %v, want both writes kept", got)
		}
	})

	t.Run("writes at or before the call are all kept", func(t *testing.T) {
		assigns := []metadata.Assignment{assignAt(10, 2, "/one"), assignAt(12, 2, "/two")}
		got := values(b.assignmentsReaching(assigns, nodeAt(15, 2)))
		if len(got) != 2 {
			t.Errorf("reaching = %v, want both writes kept (genuine ambiguity)", got)
		}
	})

	t.Run("missing facts keep every write", func(t *testing.T) {
		// An unplaceable write, a node with no position, another file: each
		// keeps the conservative answer rather than guessing.
		unplaceable := []metadata.Assignment{assignAt(10, 2, "/first"), assignTo(meta, "p", concatLit(meta, "/nopos"))}
		if got := values(b.assignmentsReaching(unplaceable, nodeAt(15, 2))); len(got) != 2 {
			t.Errorf("reaching = %v, want both kept when one has no position", got)
		}
		twoBefore := []metadata.Assignment{assignAt(10, 2, "/one"), assignAt(30, 2, "/two")}
		if got := values(b.assignmentsReaching(twoBefore, nil)); len(got) != 2 {
			t.Errorf("reaching = %v, want both kept without a node", got)
		}
		single := []metadata.Assignment{assignAt(30, 2, "/only")}
		if got := values(b.assignmentsReaching(single, nodeAt(15, 2))); len(got) != 1 {
			t.Errorf("reaching = %v, want the single write kept even below the call", got)
		}
	})

	t.Run("dropping everything falls back to keeping everything", func(t *testing.T) {
		// Two writes, both below the call, neither sharing a region: the filter
		// would empty the set, which says nothing — so the old answer stands.
		assigns := []metadata.Assignment{assignAt(30, 2, "/one"), assignAt(32, 2, "/two")}
		if got := values(b.assignmentsReaching(assigns, nodeAt(15, 2))); len(got) != 2 {
			t.Errorf("reaching = %v, want both kept when nothing reaches", got)
		}
	})
}
