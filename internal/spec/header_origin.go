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

// headerOrigin is what a header map was shown to belong to.
type headerOrigin int

const (
	// headerOriginUnknown could not be placed, so it is kept: the resolver
	// drops only what it can prove is not the response (golden rule #7).
	headerOriginUnknown headerOrigin = iota
	// headerOriginResponse traces to the handler's response writer.
	headerOriginResponse
	// headerOriginDetached was built where the handler's writer never reached
	// it — an outbound request, a literal header map, a recorder.
	headerOriginDetached
)

// maxHeaderOriginHops bounds the walk back through chains, assignments,
// selectors and parameter bindings; it is also what ends a cycle.
const maxHeaderOriginHops = 16

// HeaderWriteDetached reports whether a Content-Type header write is PROVABLY
// made on something other than the response, so it declares nothing about this
// operation's body (issue #543).
//
// The generic write-destination gate cannot answer this. It resolves a
// destination through resolveReceiverSource, which is built for the factory
// shape — `json.NewEncoder(w).Encode(v)`, the writer being the factory's first
// argument — and a header write is not one: `w.Header().Set` hands `Header()`
// no argument, `req.Header.Set` is made on a field, and `h.Set` on a variable
// holding a literal. It returned nothing for all three, the pattern does not
// drop an unresolved destination, and so the gate admitted every header write
// in a handler's call graph: an outbound request's
// `req.Header.Set("Content-Type", "application/x-www-form-urlencoded")` three
// calls from a redirect-only handler was documented as its 200 body.
//
// What decides a header write is where the header MAP came from, followed back
// through what each call is made on:
//
//   - a method's result is its receiver's (`w.Header()` → `w`);
//   - a variable is its assignment's, and a parameter its caller's argument;
//   - a map read as a FIELD belongs to the field's owner, and an owner that
//     carries a REQUEST and is no writer (`r.Header`, `c.Request.Header`) is
//     the request's header — a proxy rewriting what it forwards, not a
//     response;
//   - a composite literal or `make` is detached, and so is an owner a plain
//     function builds whose type is neither the writer nor the header map
//     (`httptest.NewRecorder()`, `http.NewRequest(…)`).
//
// Only detached is dropped. Every rule above is read from configuration —
// request types, writer types, the types a Content-Type write is made on — so
// a framework context that both carries the request and writes the response
// (gin's `c.Header(k, v)`, echo's `c.Response().Header()`) is never mistaken for
// either one: it is only ever the RECEIVER of the write, which is not a
// position any of those rules reads.
func (r *responseDestResolver) HeaderWriteDetached(node TrackerNodeInterface) bool {
	if r == nil || !r.Enabled() || node == nil || node.GetEdge() == nil {
		return false
	}
	return r.callOrigin(node.GetEdge(), node, 0) == headerOriginDetached
}

// callOrigin is the origin of what a call is made on: the root of its chain,
// then that root's recorded receiver. The write call's own receiver IS the
// header map, unless the call was chained — then the map is a method result,
// and the root is its owner.
func (r *responseDestResolver) callOrigin(call *metadata.CallGraphEdge, node TrackerNodeInterface, hops int) headerOrigin {
	isMap := true
	for call != nil && call.ChainParent != nil && hops < maxHeaderOriginHops {
		call = call.ChainParent
		isMap = false
		hops++
	}
	if call == nil || call.Receiver == nil || hops >= maxHeaderOriginHops {
		return headerOriginUnknown
	}
	return r.valueOrigin(call.Receiver, node, isMap, hops+1)
}

// valueOrigin places one value, evaluated in node's frame. isMap says the value
// IS the header map rather than something that produced it, which is the one
// position where a field read says whose header it is.
func (r *responseDestResolver) valueOrigin(arg *metadata.CallArgument, node TrackerNodeInterface, isMap bool, hops int) headerOrigin {
	for arg != nil && (arg.GetKind() == metadata.KindUnary || arg.GetKind() == metadata.KindStar || arg.GetKind() == metadata.KindParen) {
		arg = arg.X
	}
	if arg == nil || node == nil || hops >= maxHeaderOriginHops {
		return headerOriginUnknown
	}
	edge := node.GetEdge()
	if edge == nil {
		return headerOriginUnknown
	}
	// Provenance to the writer wins over any construction on the way: a
	// wrapper built around w (`&loggingWriter{w}`) IS the response.
	if r.reachesWriter(arg, edge, make(map[string]bool, 4)) {
		return headerOriginResponse
	}
	switch arg.GetKind() {
	case metadata.KindIdent:
		if rhs := latestAssignment(r.contextProvider, edge, arg.GetName()); rhs != nil {
			return r.valueOrigin(rhs, node, isMap, hops+1)
		}
		// No assignment in scope: a parameter, whose value is the caller's
		// argument for it — `setType(req.Header)` reaching `h.Set(…)`.
		if resolved, at := resolveArgThroughParams(arg, node); resolved != nil && resolved != arg && at != nil {
			return r.valueOrigin(resolved, at, isMap, hops+1)
		}
		return headerOriginUnknown
	case metadata.KindSelector:
		if arg.X == nil {
			return headerOriginUnknown
		}
		// The map is a FIELD, so it is its owner's. An owner that carries the
		// request and is no writer makes it the request's header. Not read for
		// a selector that merely leads to the map (`c.Writer` in
		// `c.Writer.Header()`): there the owner exposes a writer, and gin's
		// context carries the request too.
		if isMap {
			owner := r.declaredType(arg.X, edge)
			if owner != "" && matchAny(r.requestTypeREs, owner) && !matchAny(r.writerTypeREs, owner) {
				return headerOriginDetached
			}
		}
		return r.valueOrigin(arg.X, node, false, hops+1)
	case metadata.KindCompositeLit:
		return headerOriginDetached
	case metadata.KindCall:
		fun := arg.Fun
		// A METHOD's result belongs to its receiver — unless the result is
		// itself the writer (`resp := c.Response()`), which answers outright.
		// handleSelector records the receiver type only when the selected
		// object is a method, which is what tells `w.Header()` from the
		// package-qualified `http.NewRequest(…)`.
		if fun != nil && fun.GetKind() == metadata.KindSelector && fun.ReceiverType != nil {
			if matchAny(r.writerTypeREs, arg.GetType()) {
				return headerOriginResponse
			}
			return r.valueOrigin(fun.X, node, false, hops+1)
		}
		return r.functionResultOrigin(arg)
	}
	return headerOriginUnknown
}

// functionResultOrigin places the result of a plain function call none of
// whose arguments reaches the writer, by what it returns.
//
// A result that IS a header map — or anything else a Content-Type write is made
// on — can be anyone's: `hdr := func() http.Header { return w.Header() }`
// returns the response's through a capture no argument shows, so it stays
// unknown (review of #545). An OWNER the function builds is judged the way the
// encoder gate judges a destination (ShouldDrop): the writer is kept, and a
// concrete type with no provenance to it — a recorder, a request — is not the
// response. `make`/`new` construct in place and are detached outright.
func (r *responseDestResolver) functionResultOrigin(call *metadata.CallArgument) headerOrigin {
	if fun := call.Fun; fun != nil && fun.GetKind() == metadata.KindIdent && fun.GetPkg() == "" {
		switch fun.GetName() {
		case "make", "new":
			return headerOriginDetached
		}
	}
	t := call.GetType()
	switch {
	case t == "":
		return headerOriginUnknown
	case matchAny(r.writerTypeREs, t):
		return headerOriginResponse
	case matchAny(r.headerRecvREs, t), matchAny(r.compatibleREs, t):
		return headerOriginUnknown
	}
	return headerOriginDetached
}

// declaredType is a value's static type: the one it was declared with when
// metadata recorded it, else the one leafType reaches. Declared first, because
// the question is what the owner IS — `req, err := http.NewRequest(…)` assigns
// from a tuple, which records no type on the call, and leafType would then
// render the call expression rather than name a type.
func (r *responseDestResolver) declaredType(arg *metadata.CallArgument, edge *metadata.CallGraphEdge) string {
	for arg != nil && (arg.GetKind() == metadata.KindUnary || arg.GetKind() == metadata.KindStar || arg.GetKind() == metadata.KindParen) {
		arg = arg.X
	}
	if arg == nil {
		return ""
	}
	if t := arg.GetResolvedType(); t != "" {
		return t
	}
	if t := arg.GetType(); t != "" {
		return t
	}
	return r.leafType(arg, edge, make(map[string]bool, 4))
}
