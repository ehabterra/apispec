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

// frameChain builds a statement whose ancestors are `depth` frames deep, with a
// frame that calls `enclosing` at each depth in `targets` (1 = the immediate
// parent). A depth listed in `noParams` carries no parameter bindings.
//
// It returns the frames by depth so a test can assert WHICH one was chosen:
// with a single matching frame, "the nearest wins" and "a bindingless frame is
// skipped" are both unfalsifiable.
func frameChain(t *testing.T, depth int, targets map[int]bool, noParams map[int]bool) (TrackerNodeInterface, string, []*TrackerNode) {
	t.Helper()
	pool := metadata.NewStringPool()
	meta := &metadata.Metadata{StringPool: pool}

	callee := func(name string) metadata.Call {
		return metadata.Call{Meta: meta, Pkg: pool.Get("pkg"), Name: pool.Get(name)}
	}
	target := callee("target")
	enclosing := target.BaseID()

	byDepth := make([]*TrackerNode, depth+1) // 1-based: byDepth[1] is the parent
	var node *TrackerNode
	for i := depth; i >= 1; i-- {
		name := "filler"
		if targets[i] {
			name = "target"
		}
		var params map[string]metadata.CallArgument
		if !noParams[i] {
			params = map[string]metadata.CallArgument{"v": {}}
		}
		frame := &TrackerNode{
			CallGraphEdge: &metadata.CallGraphEdge{Callee: callee(name), ParamArgMap: params},
			Parent:        node,
		}
		byDepth[i] = frame
		node = frame
	}
	stmt := &TrackerNode{
		CallGraphEdge: &metadata.CallGraphEdge{Caller: callee("target")},
		Parent:        node,
	}
	return stmt, enclosing, byDepth
}

// TestEnclosingFrameIsBounded pins the limit and the reason it is safe: it sits
// above the deepest scan either measured project needs (10 ancestors on gitea,
// 2 on a 163-route service), and past it the answer is the same "not resolved"
// that reaching the root already gives.
func TestEnclosingFrameIsBounded(t *testing.T) {
	// Inside the bound: found.
	node, enclosing, frames := frameChain(t, enclosingFrameLimit+8, map[int]bool{enclosingFrameLimit: true}, nil)
	if got := enclosingFrame(node, enclosing, false); got != frames[enclosingFrameLimit] {
		t.Errorf("frame at the limit (%d) was not the one returned: got %v",
			enclosingFrameLimit, got)
	}

	// Past the bound: not found, rather than walked to the root.
	node, enclosing, _ = frameChain(t, enclosingFrameLimit+8, map[int]bool{enclosingFrameLimit + 4: true}, nil)
	if got := enclosingFrame(node, enclosing, false); got != nil {
		t.Errorf("frame beyond the limit of %d was returned; the scan is unbounded again",
			enclosingFrameLimit)
	}

	// The NEAREST frame wins when several match — asserted against a chain that
	// contains two, so returning the farther one fails.
	node, enclosing, frames = frameChain(t, 8, map[int]bool{2: true, 5: true}, nil)
	if got := enclosingFrame(node, enclosing, false); got != frames[2] {
		t.Errorf("with matching frames at depths 2 and 5, got %v, want the one at 2", got)
	}
}

// TestEnclosingFrameParamRequirement covers the difference the three callers
// depend on: one stops at the first matching frame whatever it carries, the
// other keeps looking for a frame that can actually bind the parameter.
func TestEnclosingFrameParamRequirement(t *testing.T) {
	// The nearest matching frame carries no bindings; a farther one does. Both
	// are needed: with only the near one, "skips it" and "stops at it" look the
	// same.
	node, enclosing, frames := frameChain(t, 8, map[int]bool{2: true, 5: true}, map[int]bool{2: true})

	if got := enclosingFrame(node, enclosing, false); got != frames[2] {
		t.Errorf("without requireParams the nearest matching frame must be returned whatever it carries: got %v", got)
	}
	if got := enclosingFrame(node, enclosing, true); got != frames[5] {
		t.Errorf("with requireParams the bindingless frame at 2 must be skipped for the bound one at 5: got %v", got)
	}
}

func TestEnclosingFrameNilSafety(t *testing.T) {
	if got := enclosingFrame(nil, "pkg.target", false); got != nil {
		t.Error("a nil node must yield no frame")
	}
	node, _, _ := frameChain(t, 4, map[int]bool{2: true}, nil)
	if got := enclosingFrame(node, "", false); got != nil {
		t.Error("an empty enclosing name must yield no frame")
	}
}
