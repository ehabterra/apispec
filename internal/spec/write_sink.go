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

import "github.com/ehabterra/apispec/internal/metadata"

// unwrapWriteSink resolves a write-sink byte argument back to the payload of the
// serialization transform that produced it. The write sink w.Write(b) sees only
// b's []byte type; when b came from `b, _ := json.Marshal(v)` the response body
// is v's type, one transform hop back. Returns the payload argument (v) when the
// sink's argument traces to a configured BodyTransform, or nil when it does not —
// a raw write (w.Write([]byte("ok"))) has no transform behind it and is kept
// as-is (honest over wrong).
//
// This is the mechanism that lets response detection be anchored on the write
// sink rather than on the marshal call in isolation (issue #195): a json.Marshal
// whose result is never written to a response writer (a downstream client's
// outbound request) is simply never reached from a sink, so it never becomes a
// response.
func (r *ResponsePatternMatcherImpl) unwrapWriteSink(arg *metadata.CallArgument, edge *metadata.CallGraphEdge) *metadata.CallArgument {
	if arg == nil || edge == nil || len(r.cfg.Framework.ResponseContext.BodyTransforms) == 0 {
		return nil
	}
	// Strip address-of/deref/paren so &b and *b trace the same as b.
	for arg != nil && (arg.GetKind() == metadata.KindUnary || arg.GetKind() == metadata.KindStar || arg.GetKind() == metadata.KindParen) {
		arg = arg.X
	}
	if arg == nil {
		return nil
	}

	// Direct local assignment: b, _ := json.Marshal(v); w.Write(b). The
	// assignment records the transform's callee (func + pkg) and its RHS call,
	// whose payload argument is the response value.
	if arg.GetKind() == metadata.KindIdent {
		assigns := assignmentsAt(r.contextProvider, edge, arg.GetName())
		if len(assigns) > 0 {
			a := assigns[len(assigns)-1]
			if idx, ok := r.matchBodyTransform(a.CalleeFunc, a.CalleePkg); ok {
				if a.Value.GetKind() == metadata.KindCall && len(a.Value.Args) > idx {
					return a.Value.Args[idx]
				}
			}
		}
	}

	return nil
}

// matchBodyTransform reports whether a call to (calleeFunc, calleePkg) is a
// configured serialization transform, returning the index of its payload
// argument. An empty PkgRegex matches any package.
func (r *ResponsePatternMatcherImpl) matchBodyTransform(calleeFunc, calleePkg string) (int, bool) {
	if calleeFunc == "" {
		return 0, false
	}
	for _, t := range r.cfg.Framework.ResponseContext.BodyTransforms {
		if t.CallRegex != "" {
			re, err := cachedRegex(t.CallRegex)
			if err != nil || !re.MatchString(calleeFunc) {
				continue
			}
		}
		if t.PkgRegex != "" {
			re, err := cachedRegex(t.PkgRegex)
			if err != nil || !re.MatchString(calleePkg) {
				continue
			}
		}
		return t.ArgIndex, true
	}
	return 0, false
}
