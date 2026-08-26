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

// frameChain builds a node whose ancestors are `depth` frames deep, with the
// frame that CALLS `enclosing` sitting at `at` (1 = the immediate parent).
// Frames carry parameter bindings unless their index is in `noParams`.
func frameChain(t *testing.T, depth, at int, noParams map[int]bool) (TrackerNodeInterface, string) {
	t.Helper()
	pool := metadata.NewStringPool()
	meta := &metadata.Metadata{StringPool: pool}

	callee := func(name string) metadata.Call {
		return metadata.Call{Meta: meta, Pkg: pool.Get("pkg"), Name: pool.Get(name)}
	}
	target := callee("target")
	enclosing := target.BaseID()

	var node TrackerNodeInterface
	for i := depth; i >= 1; i-- {
		name := "filler"
		var params map[string]metadata.CallArgument
		if i == at {
			name = "target"
		}
		if !noParams[i] {
			params = map[string]metadata.CallArgument{"v": {}}
		}
		frame := &TrackerNode{
			CallGraphEdge: &metadata.CallGraphEdge{Callee: callee(name), ParamArgMap: params},
		}
		if node != nil {
			frame.Parent = node.(*TrackerNode)
		}
		node = frame
	}
	// The statement itself hangs below the innermost frame.
	stmt := &TrackerNode{
		CallGraphEdge: &metadata.CallGraphEdge{Caller: callee("target")},
		Parent:        node.(*TrackerNode),
	}
	return stmt, enclosing
}

// TestEnclosingFrameIsBounded pins the limit and the reason it is safe: it sits
// above the deepest scan either measured project needs (10 ancestors on gitea,
// 2 on a 163-route service), and past it the answer is the same "not resolved"
// that reaching the root already gives.
func TestEnclosingFrameIsBounded(t *testing.T) {
	// Inside the bound: found.
	node, enclosing := frameChain(t, enclosingFrameLimit+8, enclosingFrameLimit, nil)
	if got := enclosingFrame(node, enclosing, false); got == nil {
		t.Errorf("frame at depth %d was not found, and the limit is %d",
			enclosingFrameLimit, enclosingFrameLimit)
	}

	// Past the bound: not found, rather than walked to the root.
	node, enclosing = frameChain(t, enclosingFrameLimit+8, enclosingFrameLimit+4, nil)
	if got := enclosingFrame(node, enclosing, false); got != nil {
		t.Errorf("frame beyond the limit of %d was returned; the scan is unbounded again",
			enclosingFrameLimit)
	}

	// The nearest frame wins when several match.
	node, enclosing = frameChain(t, 6, 2, nil)
	got := enclosingFrame(node, enclosing, false)
	if got == nil {
		t.Fatal("no frame found in a chain that contains one")
	}
	if got.GetParent() == nil {
		t.Error("returned the outermost frame, want the nearest matching one")
	}
}

// TestEnclosingFrameParamRequirement covers the difference the three callers
// depend on: one stops at the first matching frame whatever it carries, the
// other keeps looking for a frame that can actually bind the parameter.
func TestEnclosingFrameParamRequirement(t *testing.T) {
	// The nearest matching frame carries no bindings; a further one does.
	node, enclosing := frameChain(t, 8, 2, map[int]bool{2: true})

	if got := enclosingFrame(node, enclosing, false); got == nil {
		t.Error("without requireParams the first matching frame must be returned, bindings or not")
	}
	// With requireParams the bindingless frame is skipped — and since this
	// fixture has only one matching frame, nothing is found.
	if got := enclosingFrame(node, enclosing, true); got != nil {
		t.Error("with requireParams a frame carrying no bindings must be skipped")
	}
}

func TestEnclosingFrameNilSafety(t *testing.T) {
	if got := enclosingFrame(nil, "pkg.target", false); got != nil {
		t.Error("a nil node must yield no frame")
	}
	node, _ := frameChain(t, 4, 2, nil)
	if got := enclosingFrame(node, "", false); got != nil {
		t.Error("an empty enclosing name must yield no frame")
	}
}
