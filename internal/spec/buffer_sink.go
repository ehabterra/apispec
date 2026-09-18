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
	"regexp"

	"github.com/ehabterra/apispec/internal/metadata"
)

// bufferSinkMatcher is one compiled BufferSink.
type bufferSinkMatcher struct {
	call      *regexp.Regexp
	pkg       *regexp.Regexp
	writerArg int
	fromRecv  bool
	bufferArg int
}

func compileBufferSinks(sinks []BufferSink) []bufferSinkMatcher {
	out := make([]bufferSinkMatcher, 0, len(sinks))
	for _, s := range sinks {
		if s.CallRegex == "" {
			continue
		}
		call, err := cachedRegex(s.CallRegex)
		if err != nil {
			continue // an unusable pattern matches nothing, which spares nothing
		}
		m := bufferSinkMatcher{
			call:      call,
			writerArg: s.WriterArgIndex,
			fromRecv:  s.BufferFromReceiver,
			bufferArg: s.BufferArgIndex,
		}
		if s.PkgRegex != "" {
			pkg, err := cachedRegex(s.PkgRegex)
			if err != nil {
				continue
			}
			m.pkg = pkg
		}
		out = append(out, m)
	}
	return out
}

// bufferReachesWriter reports whether the buffer this encode wrote into is
// later flushed to the response writer, in the same function.
//
// The gate that calls this resolves a destination BACKWARDS — through
// assignments, parameters and struct construction — which answers "where did
// this value come from". A buffer's answer is always "bytes.NewBuffer", and
// that is true of the buffer that becomes the response and of the one that is
// thrown away. The question those two differ on is a forward one: does anything
// take this buffer's bytes to a writer?
//
//	buf := bytes.NewBufferString(xml.Header)
//	xml.NewEncoder(buf).Encode(s)   // ← destination is buf, for both shapes
//	return buf.WriteTo(w)           // ← only this one reaches the wire
//
// Scoped to the function the encode is written in, deliberately. A buffer handed
// to another function is that function's business, and following it there would
// need the same dominance reasoning a later write in THIS function already
// escapes — the point here is to recover a shape that is complete in one place,
// not to chase every buffer (issue #471).
//
// Order is not checked either. A flush textually above the encode is still the
// same buffer reaching the same writer — Go has no goto into a block, so a
// sink naming this buffer means the bytes go there on some path. Requiring
// dominance would drop the `if err != nil { return }` shape that every one of
// these handlers is written in.
func (r *responseDestResolver) bufferReachesWriter(arg *metadata.CallArgument, edge *metadata.CallGraphEdge, visited map[string]bool) bool {
	if len(r.bufferSinks) == 0 || arg == nil || edge == nil {
		return false
	}
	name := bufferVarName(arg)
	if name == "" {
		return false
	}
	meta := r.metadata()
	if meta == nil {
		return false
	}
	// Every call made by the function this encode sits in.
	for _, sibling := range meta.Callers[edge.Caller.BaseID()] {
		if sibling == nil || sibling == edge {
			continue
		}
		for _, sink := range r.bufferSinks {
			if !r.sinkNamesBuffer(sink, sibling, name) {
				continue
			}
			if sink.writerArg >= len(sibling.Args) {
				continue
			}
			// The writer side is judged by the SAME rule the gate applies to a
			// destination one level out: provenance to the response writer, or
			// a type that could still be it.
			//
			// The second half is what makes gitea's sitemap work. Its flush is
			// `buf.WriteTo(w)` inside a method whose writer is an `io.Writer`
			// PARAMETER, and an encode written straight into such a parameter is
			// already kept — a buffer flushed into one is the same claim, made
			// one hop later, and refusing it here would be the gate disagreeing
			// with itself.
			//
			// It stays honest because a resolved NON-writer still fails:
			// `buf.WriteTo(otherBuf)` is a *bytes.Buffer and `buf.WriteTo(f)` an
			// *os.File, neither compatible. That is precisely the yaml shape
			// #471 measured — every yaml encode reached from a route on that
			// project wrote to a buffer or a file, and none of them to the wire.
			w := sibling.Args[sink.writerArg]
			if r.reachesWriter(w, sibling, visited) {
				return true
			}
			if t := r.leafType(w, sibling, make(map[string]bool, 4)); t == "" || matchAny(r.compatibleREs, t) {
				return true
			}
		}
	}
	return false
}

// sinkNamesBuffer reports whether this call is the given sink, flushing the
// named buffer.
func (r *responseDestResolver) sinkNamesBuffer(sink bufferSinkMatcher, call *metadata.CallGraphEdge, buffer string) bool {
	if !sink.call.MatchString(r.contextProvider.GetString(call.Callee.Name)) {
		return false
	}
	if sink.pkg != nil && !sink.pkg.MatchString(r.contextProvider.GetString(call.Callee.Pkg)) {
		return false
	}
	if sink.fromRecv {
		// CalleeVarName, not CalleeRecvVarName. Despite the names, the RECEIVER
		// expression of `buf.WriteTo(w)` is recorded in the first — metadata
		// sets it from the selector's base identifier — while the second is the
		// variable the call's RESULT is assigned to, and is empty here because
		// these calls are written `_, _ = buf.WriteTo(w)`.
		return call.CalleeVarName == buffer
	}
	if sink.bufferArg >= len(call.Args) {
		return false
	}
	return bufferVarName(call.Args[sink.bufferArg]) == buffer
}

// bufferVarName is the variable a buffer expression names, looking through
// address-of, deref and parens so `&buf`, `*buf` and `buf` are one variable.
func bufferVarName(arg *metadata.CallArgument) string {
	for arg != nil && (arg.GetKind() == metadata.KindUnary || arg.GetKind() == metadata.KindStar || arg.GetKind() == metadata.KindParen) {
		arg = arg.X
	}
	if arg == nil || arg.GetKind() != metadata.KindIdent {
		return ""
	}
	return arg.GetName()
}
