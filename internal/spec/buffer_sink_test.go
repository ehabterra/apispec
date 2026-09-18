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

// A negative index names no argument, and the bounds checks at the use site
// test only the upper end — so `Args[-1]` panics on the first call a sink
// matches. bufferSinks come from `framework.responseContext` in a hand-written
// YAML config, so the value is reachable (CodeRabbit on #510).
func TestCompileBufferSinksRejectsNegativeIndexes(t *testing.T) {
	for name, sink := range map[string]BufferSink{
		"negative writer": {CallRegex: `^WriteTo$`, BufferFromReceiver: true, WriterArgIndex: -1},
		"negative buffer": {CallRegex: `^Copy$`, WriterArgIndex: 0, BufferArgIndex: -1},
		"both negative":   {CallRegex: `^Copy$`, WriterArgIndex: -1, BufferArgIndex: -1},
	} {
		t.Run(name, func(t *testing.T) {
			if got := compileBufferSinks([]BufferSink{sink}); len(got) != 0 {
				t.Errorf("compiled %d matchers from %+v; a negative index names no argument "+
					"and would index Args[-1] at the use site", len(got), sink)
			}
		})
	}

	// A negative bufferArgIndex is irrelevant when the buffer is the receiver,
	// and must not disqualify an otherwise usable sink — 0 is also the natural
	// zero value for a field that shape never reads.
	usable := BufferSink{CallRegex: `^WriteTo$`, BufferFromReceiver: true, WriterArgIndex: 0, BufferArgIndex: -1}
	if got := compileBufferSinks([]BufferSink{usable}); len(got) != 1 {
		t.Errorf("compiled %d matchers from %+v; the buffer index is unread when the buffer "+
			"is the receiver, so it cannot make the sink unusable", len(got), usable)
	}
}

// The guard has to hold at the use site too, because a matcher can be built
// directly. Exercised through the real entry point with a call whose argument
// list is shorter than any index — the panic this prevents took a matching call
// to reach, not merely a configured one.
func TestBufferSinkGuardsAgainstOutOfRangeArguments(t *testing.T) {
	meta := &metadata.Metadata{StringPool: metadata.NewStringPool()}
	r := &responseDestResolver{
		contextProvider: NewContextProvider(meta),
		bufferSinks: []bufferSinkMatcher{{
			call:      mustCachedRegex(`^WriteTo$`),
			writerArg: 3, // beyond the call's arguments
			fromRecv:  true,
		}},
	}
	call := &metadata.CallGraphEdge{
		Callee:        metadata.Call{Name: meta.StringPool.Get("WriteTo"), Meta: meta},
		CalleeVarName: "buf",
	}
	// Must answer, not panic: the sink names this buffer but the writer index
	// points past the end.
	if r.sinkNamesBuffer(r.bufferSinks[0], call, "buf") != true {
		t.Error("the sink should recognise the buffer; the index guard belongs to the caller")
	}
}

// A sink that cannot be used must be dropped rather than half-built: an unusable
// one would either match nothing (wasting a scan) or, worse, match everything.
func TestCompileBufferSinksDropsUnusableSinks(t *testing.T) {
	for name, sink := range map[string]BufferSink{
		"no call regex":      {BufferFromReceiver: true},
		"unparsable call":    {CallRegex: `^Write(To$`, BufferFromReceiver: true},
		"unparsable package": {CallRegex: `^Copy$`, PkgRegex: `^io($`},
	} {
		t.Run(name, func(t *testing.T) {
			if got := compileBufferSinks([]BufferSink{sink}); len(got) != 0 {
				t.Errorf("compiled %d matchers from %+v, want none", len(got), sink)
			}
		})
	}
	// And a well-formed one with a package constraint keeps it.
	got := compileBufferSinks([]BufferSink{{CallRegex: `^Copy$`, PkgRegex: `^io$`, BufferArgIndex: 1}})
	if len(got) != 1 || got[0].pkg == nil {
		t.Fatalf("got %d matchers, package constraint kept: %v", len(got), len(got) == 1 && got[0].pkg != nil)
	}
}

// The package constraint has to actually exclude: `^Copy$` alone would match a
// house helper called Copy, and a buffer "flushed" by that is not the response.
func TestSinkNamesBufferHonoursThePackageConstraint(t *testing.T) {
	meta := &metadata.Metadata{StringPool: metadata.NewStringPool()}
	r := &responseDestResolver{contextProvider: NewContextProvider(meta)}
	sink := compileBufferSinks([]BufferSink{{CallRegex: `^Copy$`, PkgRegex: `^io$`, BufferArgIndex: 1}})[0]

	buf := &metadata.CallArgument{Meta: meta}
	buf.SetKind(metadata.KindIdent)
	buf.SetName("buf")
	mk := func(pkg string) *metadata.CallGraphEdge {
		return &metadata.CallGraphEdge{
			Callee: metadata.Call{Name: meta.StringPool.Get("Copy"), Pkg: meta.StringPool.Get(pkg), Meta: meta},
			Args:   []*metadata.CallArgument{nil, buf},
		}
	}
	if !r.sinkNamesBuffer(sink, mk("io"), "buf") {
		t.Error("io.Copy(w, buf) should name the buffer")
	}
	if r.sinkNamesBuffer(sink, mk("example.com/house"), "buf") {
		t.Error("house.Copy(w, buf) matched, but the sink is scoped to io")
	}
	if r.sinkNamesBuffer(sink, mk("io"), "other") {
		t.Error("the sink named a buffer it does not carry")
	}
}

// Nothing to trace must answer false rather than panic — the resolver runs on
// every encode a project makes.
func TestBufferReachesWriterHandlesNothingToTrace(t *testing.T) {
	meta := &metadata.Metadata{StringPool: metadata.NewStringPool()}
	withSinks := &responseDestResolver{
		contextProvider: NewContextProvider(meta),
		bufferSinks:     compileBufferSinks([]BufferSink{{CallRegex: `^WriteTo$`, BufferFromReceiver: true}}),
	}
	edge := &metadata.CallGraphEdge{Callee: metadata.Call{Meta: meta}}
	lit := &metadata.CallArgument{Meta: meta}
	lit.SetKind(metadata.KindLiteral)

	for name, tc := range map[string]struct {
		r    *responseDestResolver
		arg  *metadata.CallArgument
		edge *metadata.CallGraphEdge
	}{
		"no sinks configured": {&responseDestResolver{contextProvider: NewContextProvider(meta)}, lit, edge},
		"nil argument":        {withSinks, nil, edge},
		"nil edge":            {withSinks, lit, nil},
		"not a variable":      {withSinks, lit, edge},
	} {
		t.Run(name, func(t *testing.T) {
			if withSinks.bufferSinks == nil && name != "no sinks configured" {
				t.Skip()
			}
			if tc.r.bufferReachesWriter(tc.arg, tc.edge, map[string]bool{}) {
				t.Error("reported a buffer reaching a writer with nothing to trace")
			}
		})
	}
}
