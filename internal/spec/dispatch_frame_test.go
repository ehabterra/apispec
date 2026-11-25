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

// TestFrameChainKeyDispatch pins where a call reached through interface
// dispatch is attributed (issue #645): to the frame that made the dispatching
// call, at that call's position — not to the handler frame, which is where it
// went whenever its own function was on no frame of the chain.
func TestFrameChainKeyDispatch(t *testing.T) {
	meta := &metadata.Metadata{StringPool: metadata.NewStringPool()}
	sp := meta.StringPool
	call := func(recv, name, pos string) metadata.Call {
		return metadata.Call{Meta: meta, Pkg: sp.Get("app"), RecvType: sp.Get(recv), Name: sp.Get(name), Position: sp.Get(pos)}
	}
	edge := func(caller, callee metadata.Call) *metadata.CallGraphEdge {
		return &metadata.CallGraphEdge{Caller: caller, Callee: callee}
	}

	// Handler → ctx.HTML → (interface) Renderer.HTML → pageRenderer.HTML's body.
	handler := call("", "Handler", "")
	ctxHTML := call("Context", "HTML", "main.go:10:2")
	ifaceHTML := call("Renderer", "HTML", "ctx.go:22:9")
	impl := call("pageRenderer", "HTML", "")
	header := call("ResponseWriter", "Header", "render.go:21:4")

	ctxNode := &TrackerNode{CallGraphEdge: edge(handler, ctxHTML)}
	ifaceNode := &TrackerNode{CallGraphEdge: edge(call("Context", "HTML", ""), ifaceHTML), Parent: ctxNode}
	// The value the dispatching call was handed: calls on it hang here.
	argNode := &TrackerNode{CallArgument: &metadata.CallArgument{Meta: meta}, Parent: ifaceNode}
	setNode := &TrackerNode{CallGraphEdge: edge(impl, call("Header", "Set", "render.go:21:4")), Parent: argNode}
	writeHeader := &TrackerNode{CallGraphEdge: edge(impl, call("ResponseWriter", "WriteHeader", "render.go:23:3")), Parent: ifaceNode}

	ctxID, ifaceID := ctxHTML.ID(), ifaceHTML.ID()
	headerID := header.ID()

	t.Run("dispatched call joins the dispatching frame at its call site", func(t *testing.T) {
		key, at := frameChainKey([]string{ctxID, ifaceID}, writeHeader)
		if key != ctxID || at != ifaceID {
			t.Errorf("got (%q, %q), want (%q, %q)", key, at, ctxID, ifaceID)
		}
	})

	t.Run("the dispatching frame need not be the last one pushed", func(t *testing.T) {
		// `respWriter.Header().Set(…)`: the walk enters Header() as a frame,
		// while the tree hangs Set under the argument node.
		key, at := frameChainKey([]string{ctxID, ifaceID, headerID}, setNode)
		if key != ctxID || at != ifaceID {
			t.Errorf("got (%q, %q), want (%q, %q)", key, at, ctxID, ifaceID)
		}
	})

	t.Run("a statement of a frame on the chain keeps its own position", func(t *testing.T) {
		own := &TrackerNode{CallGraphEdge: edge(call("Context", "HTML", ""), call("Base", "PlainText", "ctx.go:30:3")), Parent: ctxNode}
		key, at := frameChainKey([]string{ctxID}, own)
		if key != ctxID || at != "" {
			t.Errorf("got (%q, %q), want (%q, \"\")", key, at, ctxID)
		}
	})

	t.Run("no frame on the chain dispatched it: the handler frame", func(t *testing.T) {
		orphan := &TrackerNode{CallGraphEdge: edge(impl, call("ResponseWriter", "Write", "render.go:24:3"))}
		key, at := frameChainKey([]string{ctxID}, orphan)
		if key != "" || at != "" {
			t.Errorf("got (%q, %q), want the handler frame", key, at)
		}
		if key, at := frameChainKey(nil, writeHeader); key != "" || at != "" {
			t.Errorf("empty chain: got (%q, %q), want the handler frame", key, at)
		}
	})
}

// TestBuiltLocally pins which write destinations a derived wrapper may be
// judged by (issue #645): only values the method built. One it was handed is
// its caller's to decide.
func TestBuiltLocally(t *testing.T) {
	meta := &metadata.Metadata{StringPool: metadata.NewStringPool()}
	sp := meta.StringPool
	ident := func(name string) *metadata.CallArgument {
		a := metadata.NewCallArgument(meta)
		a.SetKind(metadata.KindIdent)
		a.SetName(name)
		return a
	}
	addr := func(x *metadata.CallArgument) *metadata.CallArgument {
		a := metadata.NewCallArgument(meta)
		a.SetKind(metadata.KindUnary)
		a.X = x
		return a
	}
	kind := func(k string) *metadata.CallArgument {
		a := metadata.NewCallArgument(meta)
		a.SetKind(k)
		return a
	}
	call := func(fun *metadata.CallArgument) *metadata.CallArgument {
		a := kind(metadata.KindCall)
		a.Fun = fun
		return a
	}
	selector := func(x *metadata.CallArgument, sel string, method bool) *metadata.CallArgument {
		a := kind(metadata.KindSelector)
		a.X, a.Sel = x, ident(sel)
		if method {
			a.ReceiverType = ident("Base")
		}
		return a
	}
	edge := &metadata.CallGraphEdge{Caller: metadata.Call{Meta: meta, Pkg: sp.Get("app"), Name: sp.Get("SetJob"), RecvType: sp.Get("Workflow")}}
	isParam := func(name string) bool { return name == "w" }
	cp := NewContextProvider(meta)

	cases := []struct {
		name string
		arg  *metadata.CallArgument
		want bool
	}{
		{"address of a declared local (`var buf bytes.Buffer`)", addr(ident("buf")), true},
		{"a composite literal", kind(metadata.KindCompositeLit), true},
		{"a constructor call", kind(metadata.KindCall), true},
		{"a package function's result (`bytes.NewBuffer(nil)`)", call(selector(ident("bytes"), "NewBuffer", false)), true},
		{"a method of the receiver (`b.Writer()`)", call(selector(ident("b"), "Writer", true)), false},
		{"a method of a parameter (`w.Body()`)", call(selector(ident("w"), "Body", true)), false},
		{"a method of a local's address (`(&buf).Writer()`)", call(selector(addr(ident("buf")), "Writer", true)), true},
		{"address of a parameter", addr(ident("w")), false},
		{"a bare unassigned ident (may be the receiver)", ident("b"), false},
		{"a receiver field", kind(metadata.KindSelector), false},
		{"nothing", nil, false},
	}
	for _, tc := range cases {
		if got := builtLocally(cp, tc.arg, edge, isParam); got != tc.want {
			t.Errorf("%s: builtLocally = %v, want %v", tc.name, got, tc.want)
		}
	}
}
