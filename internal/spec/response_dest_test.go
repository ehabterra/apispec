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

func newRespDestResolver(meta *metadata.Metadata) *responseDestResolver {
	return newResponseDestResolver(&APISpecConfig{
		Framework: FrameworkConfig{ResponseContext: netHTTPResponseContext},
	}, NewContextProvider(meta))
}

// TestResponseDestResolver_Disabled: with no writer types configured the
// resolver never drops, preserving prior behaviour (zero-drift guarantee).
func TestResponseDestResolver_Disabled(t *testing.T) {
	meta := newTestMeta()
	r := newResponseDestResolver(&APISpecConfig{}, NewContextProvider(meta))
	if r.Enabled() {
		t.Fatal("expected resolver to be disabled with no writer types")
	}
	if r.ShouldDrop(mkIdent(meta, "buf", "*bytes.Buffer"), &metadata.CallGraphEdge{}) {
		t.Fatal("disabled resolver must never drop")
	}
}

// TestResponseDestResolver_DirectTypes: a bare destination whose type is a
// writer / writer-compatible interface is kept; a concrete non-writer is
// dropped; an unresolved destination is kept.
func TestResponseDestResolver_DirectTypes(t *testing.T) {
	meta := newTestMeta()
	r := newRespDestResolver(meta)
	if !r.Enabled() {
		t.Fatal("resolver should be enabled with netHTTPResponseContext")
	}

	cases := []struct {
		name string
		arg  *metadata.CallArgument
		drop bool
	}{
		{"response writer w (handler param)", mkIdent(meta, "w", "net/http.ResponseWriter"), false},
		// A recorder type alone is NOT provenance — a locally-built recorder is
		// writer-shaped but is not the handler's w, so it must drop.
		{"locally-typed recorder is not the handler writer", mkIdent(meta, "rec", "*net/http/httptest.ResponseRecorder"), true},
		{"io.Writer helper param stays permissive", mkIdent(meta, "dst", "io.Writer"), false},
		{"untyped ident stays permissive", mkIdent(meta, "x", ""), false},
		{"bytes.Buffer is a sink", mkIdent(meta, "buf", "*bytes.Buffer"), true},
		{"os.File is a sink", mkIdent(meta, "f", "*os.File"), true},
		{"hash.Hash is a sink", mkIdent(meta, "h", "hash.Hash"), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := r.ShouldDrop(tc.arg, &metadata.CallGraphEdge{}); got != tc.drop {
				t.Errorf("ShouldDrop(%s) = %v, want %v", tc.name, got, tc.drop)
			}
		})
	}
}

// TestResponseDestResolver_AddressOf: &buf / &w strip to the underlying value.
func TestResponseDestResolver_AddressOf(t *testing.T) {
	meta := newTestMeta()
	r := newRespDestResolver(meta)

	addr := func(x *metadata.CallArgument) *metadata.CallArgument {
		a := metadata.NewCallArgument(meta)
		a.SetKind(metadata.KindUnary)
		a.X = x
		return a
	}
	if !r.ShouldDrop(addr(mkIdent(meta, "buf", "bytes.Buffer")), &metadata.CallGraphEdge{}) {
		t.Error("&buf should be a sink")
	}
	if r.ShouldDrop(addr(mkIdent(meta, "w", "net/http.ResponseWriter")), &metadata.CallGraphEdge{}) {
		t.Error("&w must not be dropped")
	}
}

// TestResponseDestResolver_Assignment: a destination is traced through its
// latest local assignment — dst := w keeps, dst := &bytes.Buffer{} drops, and
// an interface var reassigned to a concrete sink drops by that sink.
func TestResponseDestResolver_Assignment(t *testing.T) {
	meta := newTestMeta()
	r := newRespDestResolver(meta)

	t.Run("dst := w traces to the writer", func(t *testing.T) {
		w := mkIdent(meta, "w", "net/http.ResponseWriter")
		edge := &metadata.CallGraphEdge{AssignmentMap: map[string][]metadata.Assignment{
			"dst": {{Value: *w}},
		}}
		if r.ShouldDrop(mkIdent(meta, "dst", ""), edge) {
			t.Error("dst := w must be kept")
		}
	})

	t.Run("latest assignment wins: writer then buffer drops", func(t *testing.T) {
		w := mkIdent(meta, "w", "net/http.ResponseWriter")
		buf := mkIdent(meta, "buf", "bytes.Buffer")
		edge := &metadata.CallGraphEdge{AssignmentMap: map[string][]metadata.Assignment{
			"d": {{Value: *w}, {Value: *buf}}, // reassigned to a buffer
		}}
		if !r.ShouldDrop(mkIdent(meta, "d", "io.Writer"), edge) {
			t.Error("an interface var reassigned to a buffer must drop by the buffer")
		}
	})

	t.Run("latest assignment wins: buffer then writer keeps", func(t *testing.T) {
		w := mkIdent(meta, "w", "net/http.ResponseWriter")
		buf := mkIdent(meta, "buf", "bytes.Buffer")
		edge := &metadata.CallGraphEdge{AssignmentMap: map[string][]metadata.Assignment{
			"d": {{Value: *buf}, {Value: *w}}, // reassigned to the writer
		}}
		if r.ShouldDrop(mkIdent(meta, "d", "io.Writer"), edge) {
			t.Error("an interface var reassigned to the writer must be kept")
		}
	})
}

// TestResponseDestResolver_Wrapper: a struct constructed around the writer
// (&loggingWriter{w} / {ResponseWriter: w}) reaches the writer through the
// composite literal.
func TestResponseDestResolver_Wrapper(t *testing.T) {
	meta := newTestMeta()
	r := newRespDestResolver(meta)

	w := mkIdent(meta, "w", "net/http.ResponseWriter")
	comp := metadata.NewCallArgument(meta)
	comp.SetKind(metadata.KindCompositeLit)
	comp.Args = []*metadata.CallArgument{w}
	addr := metadata.NewCallArgument(meta)
	addr.SetKind(metadata.KindUnary)
	addr.X = comp
	edge := &metadata.CallGraphEdge{AssignmentMap: map[string][]metadata.Assignment{
		"lw": {{Value: *addr}},
	}}
	if r.ShouldDrop(mkIdent(meta, "lw", "*app.loggingWriter"), edge) {
		t.Error("a wrapper constructed around w must reach the writer and be kept")
	}

	// A wrapper built around a non-writer must NOT be rescued.
	buf := mkIdent(meta, "buf", "bytes.Buffer")
	comp2 := metadata.NewCallArgument(meta)
	comp2.SetKind(metadata.KindCompositeLit)
	comp2.Args = []*metadata.CallArgument{buf}
	edge2 := &metadata.CallGraphEdge{AssignmentMap: map[string][]metadata.Assignment{
		"x": {{Value: *comp2}},
	}}
	// The wrapper's own type is unknown here (empty) so it stays permissive, but
	// it must not be *reported as reaching the writer*.
	if r.reachesWriter(mkIdent(meta, "x", ""), edge2, map[string]bool{}) {
		t.Error("a wrapper around a non-writer must not reach the writer")
	}
}

// TestResponseDestResolver_ConstructorWrapper covers the constructor-function
// wrapper: ww := newWrapped(w) — the writer flows through the call argument, so
// the result reaches the writer; a sink constructor with no writer argument
// does not.
func TestResponseDestResolver_ConstructorWrapper(t *testing.T) {
	meta := newTestMeta()
	r := newRespDestResolver(meta)

	// ww := newWrapped(w) — a call whose arg is the writer.
	w := mkIdent(meta, "w", "net/http.ResponseWriter")
	ctorCall := metadata.NewCallArgument(meta)
	ctorCall.SetKind(metadata.KindCall)
	ctorCall.Fun = mkIdent(meta, "newWrapped", "")
	ctorCall.Args = []*metadata.CallArgument{w}
	edge := &metadata.CallGraphEdge{AssignmentMap: map[string][]metadata.Assignment{
		"ww": {{Value: *ctorCall}},
	}}
	if r.ShouldDrop(mkIdent(meta, "ww", "*app.wrappedWriter"), edge) {
		t.Error("a constructor wrapper receiving w must reach the writer and be kept")
	}

	// buf := bytes.NewBufferString("{}") — a call with no writer argument.
	lit := metadata.NewCallArgument(meta)
	lit.SetKind(metadata.KindLiteral)
	lit.SetValue(`"{}"`)
	sinkCall := metadata.NewCallArgument(meta)
	sinkCall.SetKind(metadata.KindCall)
	sinkCall.Fun = mkIdent(meta, "NewBufferString", "")
	sinkCall.Args = []*metadata.CallArgument{lit}
	if r.reachesWriter(sinkCall, &metadata.CallGraphEdge{}, map[string]bool{}) {
		t.Error("a constructor with no writer argument must not reach the writer")
	}
}

// TestResponseDestResolver_CyclicAssignment: a := b; b := a terminates.
func TestResponseDestResolver_CyclicAssignment(t *testing.T) {
	meta := newTestMeta()
	r := newRespDestResolver(meta)
	a := mkIdent(meta, "a", "")
	b := mkIdent(meta, "b", "")
	edge := &metadata.CallGraphEdge{AssignmentMap: map[string][]metadata.Assignment{
		"a": {{Value: *b}},
		"b": {{Value: *a}},
	}}
	// Must terminate; neither reaches a writer nor resolves to a concrete sink.
	if r.reachesWriter(mkIdent(meta, "a", ""), edge, map[string]bool{}) {
		t.Error("cyclic non-writer assignment must not report reaching the writer")
	}
}

// TestNewResponseDestResolver_EdgeCases covers nil-config and invalid-regex.
func TestNewResponseDestResolver_EdgeCases(t *testing.T) {
	cp := NewContextProvider(newTestMeta())
	if newResponseDestResolver(nil, cp).Enabled() {
		t.Error("nil cfg should yield a disabled resolver")
	}
	bad := &APISpecConfig{Framework: FrameworkConfig{ResponseContext: ResponseContextConfig{
		WriterTypeRegexes: []string{"("}, // invalid, skipped
	}}}
	if newResponseDestResolver(bad, cp).Enabled() {
		t.Error("an invalid writer regex should leave the resolver disabled")
	}
	mix := &APISpecConfig{Framework: FrameworkConfig{ResponseContext: ResponseContextConfig{
		WriterTypeRegexes:           []string{"(", `^net/http\.ResponseWriter$`},
		WriterCompatibleTypeRegexes: []string{"[", `^io\.Writer$`},
	}}}
	if !newResponseDestResolver(mix, cp).Enabled() {
		t.Error("a valid writer regex should enable the resolver despite an invalid sibling")
	}
}

// TestResponsePatternMatcher_Destination covers the receiver-based destination
// resolution and its guards.
func TestResponsePatternMatcher_Destination(t *testing.T) {
	meta := newTestMeta()
	cp := NewContextProvider(meta)
	cfg := &APISpecConfig{Framework: FrameworkConfig{ResponseContext: netHTTPResponseContext}}

	off := NewResponsePatternMatcher(ResponsePattern{DestFromReceiver: false}, cfg, cp, nil)
	if got, _ := off.destination(&fakeNode{edge: &metadata.CallGraphEdge{}}); got != nil {
		t.Errorf("DestFromReceiver=false should yield nil, got %v", got)
	}
	if got, _ := off.destination(nil); got != nil {
		t.Errorf("destination(nil) should be nil, got %v", got)
	}

	on := NewResponsePatternMatcher(ResponsePattern{DestFromReceiver: true}, cfg, cp, nil)
	w := mkIdent(meta, "w", "net/http.ResponseWriter")
	edge := &metadata.CallGraphEdge{
		ChainParent: &metadata.CallGraphEdge{Args: []*metadata.CallArgument{w}},
	}
	if got, _ := on.destination(&fakeNode{edge: edge}); got != w {
		t.Errorf("destination should resolve to the factory's first arg, got %v", got)
	}
}
