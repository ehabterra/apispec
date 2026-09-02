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
