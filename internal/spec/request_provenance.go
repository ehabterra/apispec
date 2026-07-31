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

// handlerArgDepth is the depth at which a route's handler argument sits: the
// registration node is depth 0, its argument children depth 1.
const handlerArgDepth = 1

// Request-body provenance (issue #269).
//
// A route's tracker subtree is not only the code that route runs. Several
// relations attach nodes LATERALLY — the argument-producer link answers "who
// produced this value" with every function that writes it, the interface
// fan-out expands every implementer — and those answers can be another route's
// handler. Once a sibling handler's body hangs under this route, its request
// decode is a perfectly well-formed body candidate for the wrong route, and
// preferRequestInfo's last-wins tie-break lets it overwrite the right one.
//
// Observed on a real service: a route documented a SIBLING route's DTO, reached
// through its own RESPONSE converter — the converter reads a shared domain
// field, producer resolution walks back to the handler that writes that field,
// and the walk arrives inside that sibling's closure. For one route the walk
// produced 112 candidates carrying its own type and 80 carrying the sibling's,
// and whichever came last won. Editing an unrelated statement in one handler
// moved the wrong schema onto a third route.
//
// The gate: a body candidate counts only if EVERY step from the registration
// down to it followed the program's control flow. Losing a body when the walk
// can only reach it laterally is the honest outcome — a route documented as
// LESS detailed, never as DIFFERENTLY detailed (golden rule #7).
//
// This is deliberately narrow. It gates the request body only; responses and
// parameters keep the full subtree, since their own resolution models
// (pairAndFillResponses' chain pairing) already discriminate differently, and
// widening the change would be a behaviour change beyond this defect.

// followsControlFlow reports whether child is reached from parent by executing
// it: the parent called it, it is the body of what the parent called (a
// returned closure, an interface implementer), it is a link in the same fluent
// chain, or it is a value inside the current frame.
//
// A child whose caller is some unrelated function is a lateral attachment — the
// tracker can see that code from here, but this route does not run it.
func (e *Extractor) followsControlFlow(parent, child TrackerNodeInterface, depth int) bool {
	if parent == nil || child == nil {
		return true
	}
	// The registration's handler ARGUMENT is how every route enters its own
	// code, so that argument's children are the handler body. Only at the
	// registration, though: deeper down, an argument node is an ordinary value,
	// and the producer relation hangs every writer of that value underneath it —
	// which is the lateral jump this gate exists to stop.
	if parent.GetArgument() != nil {
		return depth <= handlerArgDepth
	}
	// Arguments belong to the frame that is already in scope.
	if child.GetArgument() != nil {
		return true
	}

	parentEdge, childEdge := parent.GetEdge(), child.GetEdge()
	if parentEdge == nil || childEdge == nil {
		return true
	}
	// A fluent chain is one statement written across several calls.
	if childEdge.ChainParent != nil {
		return true
	}

	callerBase := childEdge.Caller.BaseID()
	// Inside the function the parent just called — the ordinary descent.
	if callerBase == parentEdge.Callee.BaseID() {
		return true
	}
	// A sibling call in the same function: chain/receiver children are listed
	// under this node but written in the caller's own frame.
	if callerBase == parentEdge.Caller.BaseID() {
		return true
	}

	meta := e.metadata()
	if meta == nil {
		return true
	}

	// The body of what the parent called, when that body is a function literal
	// the callee defines or returns (a handler factory's closure). This is the
	// ParentFunctions attachment the lazy tree makes in expandKey; following it
	// IS control flow — the closure is what the call produces.
	calleeBase := parentEdge.Callee.BaseID()
	for _, edge := range meta.ParentFunctions[calleeBase] {
		if edge == childEdge {
			return true
		}
	}
	if stripped := metadata.StripToBase(parentEdge.Callee.ID()); stripped != calleeBase {
		for _, edge := range meta.ParentFunctions[stripped] {
			if edge == childEdge {
				return true
			}
		}
	}

	// An implementer of the interface method the parent called: same method,
	// different receiver. The fan-out is how a call on an interface value
	// reaches the code that actually runs.
	cp := e.contextProvider
	if name := cp.GetString(childEdge.Caller.Name); name != "" &&
		name == cp.GetString(parentEdge.Callee.Name) {
		return true
	}

	return false
}

// metadata returns the metadata behind the context provider, or nil when the
// provider is not the standard implementation (mocks in tests).
func (e *Extractor) metadata() *metadata.Metadata {
	if ctxImpl, ok := e.contextProvider.(*ContextProviderImpl); ok {
		return ctxImpl.meta
	}
	return nil
}
