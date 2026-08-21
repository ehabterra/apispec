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

// provNode is a tracker node with exactly the two things followsControlFlow
// reads: the edge the node carries, and whether it is an argument node.
type provNode struct {
	edge *metadata.CallGraphEdge
	arg  *metadata.CallArgument
}

func (n *provNode) GetKey() string                                         { return "" }
func (n *provNode) GetChildren() []TrackerNodeInterface                    { return nil }
func (n *provNode) GetCallGraphEdge() *metadata.CallGraphEdge              { return n.edge }
func (n *provNode) GetCallArgument() *metadata.CallArgument                { return n.arg }
func (n *provNode) GetArgContext() string                                  { return "" }
func (n *provNode) GetArgIndex() int                                       { return 0 }
func (n *provNode) GetArgType() metadata.ArgumentType                      { return metadata.ArgTypeDirectCallee }
func (n *provNode) GetArgument() *metadata.CallArgument                    { return n.arg }
func (n *provNode) GetEdge() *metadata.CallGraphEdge                       { return n.edge }
func (n *provNode) GetParent() TrackerNodeInterface                        { return nil }
func (n *provNode) GetTypeParamMap() map[string]string                     { return nil }
func (n *provNode) GetRootAssignmentMap() map[string][]metadata.Assignment { return nil }

// provEdge builds an edge "callerPkg.callerName -> calleePkg.calleeName".
func provEdge(meta *metadata.Metadata, callerPkg, callerName, calleePkg, calleeName string) *metadata.CallGraphEdge {
	sp := meta.StringPool
	return &metadata.CallGraphEdge{
		Caller: metadata.Call{Meta: meta, Name: sp.Get(callerName), Pkg: sp.Get(callerPkg), RecvType: -1},
		Callee: metadata.Call{Meta: meta, Name: sp.Get(calleeName), Pkg: sp.Get(calleePkg), RecvType: -1},
	}
}

func provExtractor(meta *metadata.Metadata) *Extractor {
	return &Extractor{contextProvider: NewContextProvider(meta)}
}

// TestFollowsControlFlow_ProvenanceGate pins the rule that keeps a route from
// documenting a SIBLING route's request body (issue #269).
//
// A route's tracker subtree contains more than the code the route runs: the
// argument-producer relation answers "who produced this value" with every
// function that WRITES it, which for a shared domain field is another route's
// handler. Once that handler hangs under this route, its own decode call is a
// well-formed body candidate for the wrong route, and preferRequestInfo's
// last-wins tie-break lets it overwrite the right one. On a real service that
// documented three endpoints with a neighbour's DTO, and the winner moved when
// an unrelated statement in a third handler changed.
func TestFollowsControlFlow_ProvenanceGate(t *testing.T) {
	meta := newTestMeta()
	meta.ParentFunctions = map[string][]*metadata.CallGraphEdge{}
	e := provExtractor(meta)

	t.Run("ordinary descent into what the parent called", func(t *testing.T) {
		parent := &provNode{edge: provEdge(meta, "app", "handler", "app", "decode")}
		child := &provNode{edge: provEdge(meta, "app", "decode", "json", "Decode")}
		if !e.followsControlFlow(parent, child, 4) {
			t.Error("a call inside the function the parent called is control flow; it must be allowed")
		}
	})

	t.Run("sibling call written in the same function", func(t *testing.T) {
		parent := &provNode{edge: provEdge(meta, "app", "handler", "app", "first")}
		child := &provNode{edge: provEdge(meta, "app", "handler", "app", "second")}
		if !e.followsControlFlow(parent, child, 4) {
			t.Error("a chain/receiver child written in the caller's own frame must be allowed")
		}
	})

	t.Run("registration handler argument opens the handler body", func(t *testing.T) {
		// The handler argument sits at depth 1; its children are the handler's
		// own body, whose caller is the closure rather than the registration.
		parent := &provNode{
			edge: provEdge(meta, "app", "routes", "chi", "Post"),
			arg:  metadata.NewCallArgument(meta),
		}
		child := &provNode{edge: provEdge(meta, "app", "FuncLit:h.go:10:9", "httpx", "DecodeJSON")}
		if !e.followsControlFlow(parent, child, handlerArgDepth) {
			t.Error("the registration's handler argument must open the route's own body")
		}
	})

	t.Run("a wrapper hop beyond the handler argument stays on the route's own path", func(t *testing.T) {
		// `mux.Handle(p, withLogging(http.HandlerFunc(h)))` puts the handler two
		// argument hops down — a call, then a conversion — and refusing those
		// hops lost the handler's request body (issue #364).
		for _, kind := range []string{metadata.KindCall, metadata.KindTypeConversion} {
			arg := metadata.NewCallArgument(meta)
			arg.SetKind(kind)
			parent := &provNode{edge: provEdge(meta, "app", "routes", "net/http", "Handle"), arg: arg}
			child := &provNode{edge: provEdge(meta, "app", "getItem", "json", "Decode")}
			if !e.followsControlFlow(parent, child, handlerArgDepth+2) {
				t.Errorf("a %s argument is the handler itself; its body must stay reachable", kind)
			}
		}
	})

	t.Run("a value hop beyond the handler argument does not", func(t *testing.T) {
		// The distinction that keeps #269 closed: the lateral producer relation
		// hangs every writer of a value under VARIABLE and SELECTOR arguments,
		// never under a call or a conversion.
		for _, kind := range []string{metadata.KindIdent, metadata.KindSelector} {
			arg := metadata.NewCallArgument(meta)
			arg.SetKind(kind)
			parent := &provNode{edge: provEdge(meta, "app", "ToView", "app", "estimateView"), arg: arg}
			child := &provNode{edge: provEdge(meta, "app", "FuncLit:sibling.go:28:9", "httpx", "DecodeJSON")}
			if e.followsControlFlow(parent, child, handlerArgDepth+2) {
				t.Errorf("a %s argument deep in the body must not open a sibling handler", kind)
			}
		}
	})

	t.Run("value argument deep in the body does NOT open a sibling handler", func(t *testing.T) {
		// This is #269: a response converter passes a shared domain field,
		// producer resolution answers with the sibling handler that writes that
		// field, and the sibling's decode arrives as a candidate for THIS route.
		parent := &provNode{
			edge: provEdge(meta, "app", "ToView", "app", "estimateView"),
			arg:  metadata.NewCallArgument(meta),
		}
		child := &provNode{edge: provEdge(meta, "app", "FuncLit:sibling.go:28:9", "httpx", "DecodeJSON")}
		if e.followsControlFlow(parent, child, handlerArgDepth+3) {
			t.Error("a sibling handler reached through a produced value is not this route's control flow")
		}
	})

	t.Run("closure the callee returns", func(t *testing.T) {
		// A handler factory has no direct calls; the tree reaches its returned
		// closure through ParentFunctions, and that IS control flow.
		parent := &provNode{edge: provEdge(meta, "app", "routes", "app", "MakeHandler")}
		child := &provNode{edge: provEdge(meta, "app", "FuncLit:h.go:12:9", "httpx", "DecodeJSON")}
		meta.ParentFunctions[parent.edge.Callee.BaseID()] = []*metadata.CallGraphEdge{child.edge}
		if !e.followsControlFlow(parent, child, 3) {
			t.Error("a factory's returned closure is what the call produces; it must be allowed")
		}
	})

	t.Run("interface implementer of the called method", func(t *testing.T) {
		parent := &provNode{edge: provEdge(meta, "app", "handler", "svc", "Store")}
		child := &provNode{edge: provEdge(meta, "impl", "Store", "db", "Insert")}
		if !e.followsControlFlow(parent, child, 3) {
			t.Error("an implementer of the interface method the parent called must be allowed")
		}
	})

	t.Run("missing edges are never blocked", func(t *testing.T) {
		if !e.followsControlFlow(&provNode{}, &provNode{}, 3) {
			t.Error("a node with no edge carries no evidence of a lateral jump; it must not be blocked")
		}
		if !e.followsControlFlow(nil, nil, 0) {
			t.Error("nil nodes must not be blocked")
		}
	})
}
