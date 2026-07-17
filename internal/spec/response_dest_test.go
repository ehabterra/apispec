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

// TestResponseDestResolver_Disabled: with no exclude patterns configured the
// resolver never rejects, preserving prior behaviour (zero-drift guarantee).
func TestResponseDestResolver_Disabled(t *testing.T) {
	meta := newTestMeta()
	cp := NewContextProvider(meta)
	r := newResponseDestResolver(&APISpecConfig{}, cp)
	if r.Enabled() {
		t.Fatal("expected resolver to be disabled with no exclude patterns")
	}
	// Even a clearly-non-writer destination is not rejected when disabled.
	if r.IsProvablyNonWriter(mkIdent(meta, "buf", "*bytes.Buffer"), &metadata.CallGraphEdge{}) {
		t.Fatal("disabled resolver must never report a non-writer")
	}
}

// TestResponseDestResolver_ProvablyNonWriter: once exclude patterns are
// configured, only a destination whose resolved type matches a known-sink
// pattern is rejected; writers, custom/unknown types, interfaces, and
// unresolvable destinations are all kept (permissive, honest over wrong).
func TestResponseDestResolver_ProvablyNonWriter(t *testing.T) {
	meta := newTestMeta()
	cp := NewContextProvider(meta)
	r := newResponseDestResolver(&APISpecConfig{
		Framework: FrameworkConfig{ResponseContext: netHTTPResponseContext},
	}, cp)
	if !r.Enabled() {
		t.Fatal("resolver should be enabled with netHTTPResponseContext")
	}

	cases := []struct {
		name    string
		arg     *metadata.CallArgument
		nonWrit bool // want IsProvablyNonWriter
	}{
		// Known sinks — rejected.
		{"bytes.Buffer", mkIdent(meta, "buf", "*bytes.Buffer"), true},
		{"strings.Builder", mkIdent(meta, "b", "*strings.Builder"), true},
		{"os.File", mkIdent(meta, "f", "*os.File"), true},
		{"hash.Hash", mkIdent(meta, "h", "hash.Hash"), true},
		// Response writer and writer-compatible/unknown — kept.
		{"response writer param w", mkIdent(meta, "w", "net/http.ResponseWriter"), false},
		{"io.Writer helper param", mkIdent(meta, "dst", "io.Writer"), false},
		{"custom writer type", mkIdent(meta, "cw", "example.com/app.LoggingWriter"), false},
		// Unresolvable — kept.
		{"untyped ident stays permissive", mkIdent(meta, "x", ""), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := r.IsProvablyNonWriter(tc.arg, &metadata.CallGraphEdge{})
			if got != tc.nonWrit {
				t.Errorf("IsProvablyNonWriter(%s) = %v, want %v", tc.name, got, tc.nonWrit)
			}
		})
	}
}

// TestResponseDestResolver_AddressOf: &buf strips to buf, so a concrete buffer
// is still recognised as a sink, while &w (a writer) is kept.
func TestResponseDestResolver_AddressOf(t *testing.T) {
	meta := newTestMeta()
	cp := NewContextProvider(meta)
	r := newResponseDestResolver(&APISpecConfig{
		Framework: FrameworkConfig{ResponseContext: netHTTPResponseContext},
	}, cp)

	addrBuf := metadata.NewCallArgument(meta)
	addrBuf.SetKind(metadata.KindUnary)
	addrBuf.X = mkIdent(meta, "buf", "bytes.Buffer")
	if !r.IsProvablyNonWriter(addrBuf, &metadata.CallGraphEdge{}) {
		t.Error("&buf should be recognised as a sink")
	}

	addrW := metadata.NewCallArgument(meta)
	addrW.SetKind(metadata.KindUnary)
	addrW.X = mkIdent(meta, "w", "net/http.ResponseWriter")
	if r.IsProvablyNonWriter(addrW, &metadata.CallGraphEdge{}) {
		t.Error("&w must not be rejected — it is the writer")
	}
}

// TestResponseDestResolver_TypeTracing covers identType's fallbacks: a variable
// whose concrete type is recorded on a local assignment resolves through, a
// selector/call destination uses its recorded result type, and an unsupported
// kind stays permissive.
func TestResponseDestResolver_TypeTracing(t *testing.T) {
	meta := newTestMeta()
	cp := NewContextProvider(meta)
	r := newResponseDestResolver(&APISpecConfig{
		Framework: FrameworkConfig{ResponseContext: netHTTPResponseContext},
	}, cp)

	t.Run("sink type recovered from assignment ConcreteType", func(t *testing.T) {
		edge := &metadata.CallGraphEdge{
			AssignmentMap: map[string][]metadata.Assignment{
				"buf": {{ConcreteType: meta.StringPool.Get("bytes.Buffer")}},
			},
		}
		if !r.IsProvablyNonWriter(mkIdent(meta, "buf", ""), edge) {
			t.Error("buf whose assignment ConcreteType is bytes.Buffer should be a sink")
		}
	})

	t.Run("writer type from assignment keeps the response", func(t *testing.T) {
		edge := &metadata.CallGraphEdge{
			AssignmentMap: map[string][]metadata.Assignment{
				"rw": {{ConcreteType: meta.StringPool.Get("net/http.ResponseWriter")}},
			},
		}
		if r.IsProvablyNonWriter(mkIdent(meta, "rw", ""), edge) {
			t.Error("rw whose ConcreteType is the writer must be kept")
		}
	})

	t.Run("selector destination uses its resolved type", func(t *testing.T) {
		sel := mkSelector(meta, mkIdent(meta, "s", "app.Server"), mkIdent(meta, "buf", ""))
		sel.ResolvedType = meta.StringPool.Get("bytes.Buffer")
		if !r.IsProvablyNonWriter(sel, &metadata.CallGraphEdge{}) {
			t.Error("a selector resolving to bytes.Buffer should be a sink")
		}
	})

	t.Run("unsupported kind and nil stay permissive", func(t *testing.T) {
		lit := metadata.NewCallArgument(meta)
		lit.SetKind(metadata.KindLiteral)
		if r.IsProvablyNonWriter(lit, &metadata.CallGraphEdge{}) {
			t.Error("a literal destination is not provably a sink")
		}
		if r.IsProvablyNonWriter(nil, &metadata.CallGraphEdge{}) {
			t.Error("a nil destination is not provably a sink")
		}
	})
}

// TestNewResponseDestResolver_EdgeCases covers the constructor's nil-config and
// invalid-regex handling.
func TestNewResponseDestResolver_EdgeCases(t *testing.T) {
	cp := NewContextProvider(newTestMeta())

	if newResponseDestResolver(nil, cp).Enabled() {
		t.Error("nil cfg should yield a disabled resolver")
	}

	bad := &APISpecConfig{Framework: FrameworkConfig{ResponseContext: ResponseContextConfig{
		WriterExcludeTypeRegexes: []string{"("}, // invalid regex, skipped
	}}}
	if newResponseDestResolver(bad, cp).Enabled() {
		t.Error("an invalid regex should be skipped, leaving the resolver disabled")
	}

	mix := &APISpecConfig{Framework: FrameworkConfig{ResponseContext: ResponseContextConfig{
		WriterExcludeTypeRegexes: []string{"(", `^\*?bytes\.Buffer$`}, // one bad, one good
	}}}
	r := newResponseDestResolver(mix, cp)
	if !r.Enabled() {
		t.Error("a valid exclude regex should enable the resolver despite an invalid sibling")
	}
	m := newTestMeta()
	if r2 := newResponseDestResolver(mix, NewContextProvider(m)); !r2.IsProvablyNonWriter(mkIdent(m, "buf", "*bytes.Buffer"), &metadata.CallGraphEdge{}) {
		t.Error("the valid exclude regex should still reject bytes.Buffer")
	}
}

// TestResponsePatternMatcher_Destination covers ResponsePatternMatcherImpl.
// destination across its branches: nil node, DestFromReceiver off, and the
// receiver-chain resolution that yields the encoder factory's first argument.
func TestResponsePatternMatcher_Destination(t *testing.T) {
	meta := newTestMeta()
	cp := NewContextProvider(meta)
	cfg := &APISpecConfig{Framework: FrameworkConfig{ResponseContext: netHTTPResponseContext}}

	off := NewResponsePatternMatcher(ResponsePattern{DestFromReceiver: false}, cfg, cp, nil)
	if got := off.destination(&metadata.CallGraphEdge{}); got != nil {
		t.Errorf("DestFromReceiver=false should yield nil destination, got %v", got)
	}
	if got := off.destination(nil); got != nil {
		t.Errorf("destination(nil) should be nil, got %v", got)
	}

	on := NewResponsePatternMatcher(ResponsePattern{DestFromReceiver: true}, cfg, cp, nil)
	w := mkIdent(meta, "w", "net/http.ResponseWriter")
	edge := &metadata.CallGraphEdge{
		ChainParent: &metadata.CallGraphEdge{Args: []*metadata.CallArgument{w}},
	}
	if got := on.destination(edge); got != w {
		t.Errorf("destination should resolve to the factory's first arg, got %v", got)
	}
}
