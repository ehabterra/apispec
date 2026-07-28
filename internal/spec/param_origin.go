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
	"github.com/ehabterra/apispec/internal/metadata"
)

// receiverOriginDepth bounds the chain/assignment walk. Header reads are short
// expressions (`c.Response().Header().Get(x)` is depth 2); the cap only exists
// so a cyclic assignment cannot spin.
const receiverOriginDepth = 8

// receiverOriginTypes returns the types the call's receiver came FROM, nearest
// first: the receiver types of the calls it is chained onto, and — when the
// receiver is a variable — the types on the right-hand side of that variable's
// assignment.
//
// This exists because a type alone cannot say which side of an exchange a value
// belongs to. `net/http.Header` is the header map of both the request and the
// response, so `Get` on it is a request parameter only by provenance:
// `r.Header.Get(k)` reads what a client sent, `w.Header().Get(k)` reads back
// what the server is about to send. The origin is what distinguishes them.
//
// Order is the walk order (chain parents outward, then assignment right-hand
// side) — no map is iterated, so the result is deterministic.
func receiverOriginTypes(cp ContextProvider, edge *metadata.CallGraphEdge) []string {
	if cp == nil || edge == nil {
		return nil
	}

	var types []string

	// The calls this one is chained onto. Each parent contributes the type it was
	// invoked on: for `c.Response().Header().Get(k)` those are `*echo.Response`
	// (Header's receiver) and `echo.Context` (Response's receiver).
	for parent, depth := edge.ChainParent, 0; parent != nil && depth < receiverOriginDepth; parent, depth = parent.ChainParent, depth+1 {
		if fq := fqCalleeRecvType(cp, parent); fq != "" {
			types = append(types, fq)
		}
	}

	// A variable receiver (`wh := w.Header(); wh.Get(k)`) has no chain parent —
	// its origin is the assignment.
	if edge.CalleeVarName != "" {
		assigns := assignmentsAt(cp, edge, edge.CalleeVarName)
		if len(assigns) > 0 {
			// The latest assignment is the one in effect at the call, matching how
			// every other receiver lookup reads assignments.
			value := assigns[len(assigns)-1].Value
			types = append(types, argOriginTypes(&value)...)
		}
	}

	return types
}

// fqCalleeRecvType renders an edge's callee receiver the way the pattern
// matchers do (`pkg` + `.` + `RecvType`), so one regex form works for both.
func fqCalleeRecvType(cp ContextProvider, edge *metadata.CallGraphEdge) string {
	recvType := cp.GetString(edge.Callee.RecvType)
	if recvType == "" {
		return ""
	}
	if pkg := cp.GetString(edge.Callee.Pkg); pkg != "" {
		return pkg + "." + recvType
	}
	return recvType
}

// argOriginTypes collects the types down an expression's spine, nearest first:
// `w.Header()` yields the header type then the type of `w`, `r.Header` the header
// type then the type of `r`. A call contributes its RESULT type, which is what
// makes an intermediate step legible (`c.Response()` is `*echo.Response`) — the
// selector in between only carries the method's signature, so it is stepped over.
//
// The types read here come from go/types via the string pool and are
// package-qualified; the bare receiver name recorded on a method selector is not,
// which is why it is not used.
func argOriginTypes(arg *metadata.CallArgument) []string {
	var types []string
	for depth := 0; arg != nil && depth < receiverOriginDepth; depth++ {
		if t := arg.GetType(); t != "" {
			types = append(types, t)
		}
		switch arg.GetKind() {
		case metadata.KindCall:
			// Step over the method selector to whatever the method was called on.
			if arg.Fun != nil && arg.Fun.GetKind() == metadata.KindSelector {
				arg = arg.Fun.X
				continue
			}
			arg = arg.Fun
		case metadata.KindSelector:
			arg = arg.X
		default:
			arg = nil
		}
	}
	return types
}
