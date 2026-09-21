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
// What decides a header write is where the header MAP came from, which is
// followed back through what each call is made on:
//
//   - a method's result is its receiver's (`w.Header()` → `w`);
//   - a field is its owner's (`req.Header` → `req`);
//   - a variable is its assignment's, and a parameter its caller's argument;
//   - a composite literal, or a function call no argument of which reaches the
//     writer (`http.NewRequest(…)`, `make(http.Header)`), is detached.
//
// Only detached is dropped. A context that merely exposes the writer (gin's
// `c.Header(k, v)`) never reaches a construction, so it stays unknown and is
// kept — which is how every framework's own shorthand keeps working.
func (r *responseDestResolver) HeaderWriteDetached(node TrackerNodeInterface) bool {
	if r == nil || !r.Enabled() || node == nil || node.GetEdge() == nil {
		return false
	}
	return r.callOrigin(node.GetEdge(), node, 0) == headerOriginDetached
}

// callOrigin is the origin of what a call is made on: the root of its chain,
// then that root's recorded receiver.
func (r *responseDestResolver) callOrigin(call *metadata.CallGraphEdge, node TrackerNodeInterface, hops int) headerOrigin {
	for call != nil && call.ChainParent != nil && hops < maxHeaderOriginHops {
		call = call.ChainParent
		hops++
	}
	if call == nil || call.Receiver == nil || hops >= maxHeaderOriginHops {
		return headerOriginUnknown
	}
	return r.valueOrigin(call.Receiver, node, hops+1)
}

// valueOrigin places one value, evaluated in node's frame.
func (r *responseDestResolver) valueOrigin(arg *metadata.CallArgument, node TrackerNodeInterface, hops int) headerOrigin {
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
			return r.valueOrigin(rhs, node, hops+1)
		}
		// No assignment in scope: a parameter, whose value is the caller's
		// argument for it — `setType(req.Header)` reaching `h.Set(…)`.
		if resolved, at := resolveArgThroughParams(arg, node); resolved != nil && resolved != arg && at != nil {
			return r.valueOrigin(resolved, at, hops+1)
		}
		return headerOriginUnknown
	case metadata.KindSelector:
		return r.valueOrigin(arg.X, node, hops+1)
	case metadata.KindCompositeLit:
		return headerOriginDetached
	case metadata.KindCall:
		// A METHOD's result belongs to its receiver. handleSelector records the
		// receiver type only when the selected object is a method, which is what
		// tells `w.Header()` from the package-qualified `http.NewRequest(…)`.
		if fun := arg.Fun; fun != nil && fun.GetKind() == metadata.KindSelector && fun.ReceiverType != nil {
			return r.valueOrigin(fun.X, node, hops+1)
		}
		// A function no argument of which reaches the writer builds a value the
		// writer never touched (reachesWriter above already checked the args).
		return headerOriginDetached
	}
	return headerOriginUnknown
}
