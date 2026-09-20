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
	"strings"
	"testing"

	"github.com/ehabterra/apispec/internal/metadata"
)

// TestResultVarNamesFindsTheMethodScope pins that the forward trace can see a
// serializer's result when the call is written in a METHOD.
//
// Methods live in Type.Methods rather than the file's function table, so a
// lookup that reads only the function table finds nothing and the trace reports
// "does not reach the writer" — which, for a gate that DROPS on false, silently
// removes a real response.
func TestResultVarNamesFindsTheMethodScope(t *testing.T) {
	meta := newTestMeta()
	sp := meta.StringPool

	call := metadata.NewCallArgument(meta)
	call.SetKind(metadata.KindCall)
	method := metadata.Method{
		Name:     sp.Get("render"),
		Receiver: sp.Get("*W"),
		AssignmentMap: map[string][]metadata.Assignment{
			"b": {{Value: *call, CalleeFunc: "Marshal", CalleePkg: "encoding/json"}},
		},
	}
	typ := &metadata.Type{Name: sp.Get("W"), Methods: []metadata.Method{method}}
	meta.Packages = map[string]*metadata.Package{
		"p": {Files: map[string]*metadata.File{"p.go": {Types: map[string]*metadata.Type{"W": typ}}}},
	}

	am := methodAssignmentsOf(meta, "p", "*W", "render")
	if am == nil {
		t.Fatal("methodAssignmentsOf returned nil; a method's assignments are invisible to the forward trace")
	}
	if len(am["b"]) == 0 {
		t.Errorf("the method's `b` assignment is missing; have %v", assignedNames(am))
	}

	// And the trace reaches it through the enclosing method, not just the edge.
	m := NewResponsePatternMatcher(ResponsePattern{TypeFromArg: true}, DefaultChiConfig(), NewContextProvider(meta))
	edge := &metadata.CallGraphEdge{
		Caller: metadata.Call{Meta: meta, Name: sp.Get("render"), Pkg: sp.Get("p"), RecvType: sp.Get("*W")},
		Callee: metadata.Call{Meta: meta, Name: sp.Get("Marshal"), Pkg: sp.Get("encoding/json")},
	}
	if got := strings.Join(m.resultVarNames(edge), ","); got != "b" {
		t.Errorf("resultVarNames = %q, want \"b\" — the method scope was not consulted", got)
	}

	// A receiver that does not match must not answer for another type.
	if got := methodAssignmentsOf(meta, "p", "*Other", "render"); got != nil {
		t.Error("a mismatched receiver returned assignments")
	}
	if got := methodAssignmentsOf(meta, "p", "*W", "nosuch"); got != nil {
		t.Error("an unknown method returned assignments")
	}
	if got := methodAssignmentsOf(nil, "p", "*W", "render"); got != nil {
		t.Error("nil metadata returned assignments")
	}
	if got := methodAssignmentsOf(meta, "p", "*W", ""); got != nil {
		t.Error("an empty method name returned assignments")
	}
}

func assignedNames(am map[string][]metadata.Assignment) []string {
	out := make([]string, 0, len(am))
	for n := range am {
		out = append(out, n)
	}
	return out
}

// TestCallNamesAnyVar covers the argument scan that decides whether a sibling
// call consumes the serializer's bytes.
func TestCallNamesAnyVar(t *testing.T) {
	meta := newTestMeta()
	call := &metadata.CallGraphEdge{
		Args: []*metadata.CallArgument{mkIdent(meta, "b", "[]byte")},
	}
	if !callNamesAnyVar(call, []string{"b"}) {
		t.Error("the call names b and was not matched")
	}
	if callNamesAnyVar(call, []string{"other"}) {
		t.Error("matched a variable the call does not name")
	}
	if callNamesAnyVar(call, nil) {
		t.Error("matched with no names to look for")
	}
	if callNamesAnyVar(&metadata.CallGraphEdge{}, []string{"b"}) {
		t.Error("matched a call with no arguments")
	}
}

// TestReceiverIdent covers recovering the value a method call was made on, in
// both shapes, and the case where there is nothing to recover — an interface
// method call records no receiver variable, which is why the serializer is
// recognised from the BodyTransforms table rather than by excluding calls made
// on a writer.
func TestReceiverIdent(t *testing.T) {
	meta := newTestMeta()

	t.Run("a variable receiver", func(t *testing.T) {
		got := receiverIdent(meta, &metadata.CallGraphEdge{CalleeVarName: "w"})
		if got == nil || got.GetName() != "w" {
			t.Fatalf("receiverIdent = %v, want an ident named w", got)
		}
		if got.GetKind() != metadata.KindIdent {
			t.Errorf("kind = %v, want ident", got.GetKind())
		}
	})

	t.Run("an inline chain", func(t *testing.T) {
		parent := &metadata.CallGraphEdge{Args: []*metadata.CallArgument{mkIdent(meta, "buf", "*bytes.Buffer")}}
		got := receiverIdent(meta, &metadata.CallGraphEdge{ChainParent: parent})
		if got == nil || got.GetName() != "buf" {
			t.Fatalf("receiverIdent = %v, want the chain parent's first argument", got)
		}
	})

	t.Run("nothing to recover", func(t *testing.T) {
		if got := receiverIdent(meta, &metadata.CallGraphEdge{}); got != nil {
			t.Errorf("receiverIdent = %v, want nil for a call with no receiver", got)
		}
		if got := receiverIdent(nil, &metadata.CallGraphEdge{CalleeVarName: "w"}); got != nil {
			t.Errorf("receiverIdent = %v, want nil without metadata", got)
		}
	})
}

// TestSerializerGateOffWithoutWriterTypes pins that the gate is inert when the
// project's config declares no response writer — the same convention every
// other resolver here follows, so a config carrying only patterns keeps working.
func TestSerializerGateOffWithoutWriterTypes(t *testing.T) {
	meta := newTestMeta()
	cfg := &APISpecConfig{} // no ResponseContext at all
	m := NewResponsePatternMatcher(ResponsePattern{TypeFromArg: true}, cfg, NewContextProvider(meta))

	if m.isSerializerCall(&metadata.CallGraphEdge{}) {
		t.Error("isSerializerCall = true with no writer types configured; the gate must be off")
	}
	if !m.resultReachesWriter(&metadata.CallGraphEdge{}) {
		t.Error("resultReachesWriter = false with the gate off; nothing may be dropped")
	}
	if !m.resultReachesWriter(nil) {
		t.Error("a nil edge must not be treated as proof the bytes go nowhere")
	}
}

// TestIsSerializerCallNeedsABodyClaim pins the two pattern shapes that are
// never gated: one that names a writer, and one that reads no type at all.
func TestIsSerializerCallNeedsABodyClaim(t *testing.T) {
	meta := newTestMeta()
	cfg := DefaultChiConfig()
	edge := &metadata.CallGraphEdge{}

	for _, tc := range []struct {
		name    string
		pattern ResponsePattern
	}{
		{"reads no type from an argument", ResponsePattern{TypeArgIndex: -1}},
		{"names a writer receiver", ResponsePattern{TypeFromArg: true, DestFromReceiver: true}},
		{"names a writer argument", ResponsePattern{TypeFromArg: true, DestFromAnyArg: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := NewResponsePatternMatcher(tc.pattern, cfg, NewContextProvider(meta))
			if m.isSerializerCall(edge) {
				t.Error("isSerializerCall = true; this pattern is gated the existing way")
			}
		})
	}
}

// TestResultVarNamesMatchesTheCallee pins that the variable is tied to the call
// it came from: an assignment from a DIFFERENT function must not be read as
// this serializer's result, or the forward trace would follow the wrong bytes.
func TestResultVarNamesMatchesTheCallee(t *testing.T) {
	meta := newTestMeta()
	cfg := DefaultChiConfig()
	m := NewResponsePatternMatcher(ResponsePattern{TypeFromArg: true}, cfg, NewContextProvider(meta))

	call := metadata.NewCallArgument(meta)
	call.SetKind(metadata.KindCall)
	edge := &metadata.CallGraphEdge{
		Callee: metadata.Call{Name: meta.StringPool.Get("Marshal"), Pkg: meta.StringPool.Get("encoding/json")},
		AssignmentMap: map[string][]metadata.Assignment{
			"ours":   {{Value: *call, CalleeFunc: "Marshal", CalleePkg: "encoding/json"}},
			"theirs": {{Value: *call, CalleeFunc: "Marshal", CalleePkg: "gopkg.in/yaml.v3"}},
			"other":  {{Value: *call, CalleeFunc: "Sprintf", CalleePkg: "fmt"}},
		},
	}

	got := strings.Join(m.resultVarNames(edge), ",")
	if got != "ours" {
		t.Errorf("resultVarNames = %q, want %q — the variable must come from THIS call", got, "ours")
	}

	// A callee with no name gives nothing rather than matching everything.
	if names := m.resultVarNames(&metadata.CallGraphEdge{}); len(names) != 0 {
		t.Errorf("resultVarNames = %v for a nameless callee, want none", names)
	}
}
