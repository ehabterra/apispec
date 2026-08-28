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
	"unsafe"
)

// maxLazyNodeSize is the Go size class LazyNode must stay inside.
//
// A real service materialises millions of these — 15.7M on gitea — out of a
// slab, so the struct's size class, not its field count, sets the floor on tree
// memory. 72 sits in the 80-byte class's neighbour below it; one more pointer
// field crosses back and costs 8 bytes on every node.
const maxLazyNodeSize = 72

// TestLazyNodeStaysInItsSizeClass fails when a field is added without measuring.
//
// Deliberately an equality check rather than "<=": a node that got SMALLER is
// also worth knowing about, because it means a size class was freed up and the
// bound should come down with it rather than silently leaving headroom.
func TestLazyNodeStaysInItsSizeClass(t *testing.T) {
	got := unsafe.Sizeof(LazyNode{})
	if got != maxLazyNodeSize {
		t.Errorf("sizeof(LazyNode) = %d, pinned at %d.\n"+
			"Tree memory is millions of these: measure on a large real project "+
			"(not testdata/) before changing this, and move the bound deliberately.",
			got, maxLazyNodeSize)
	}
}

// TestInternKeyRoundTrips covers the handle table the node key was traded for.
func TestInternKeyRoundTrips(t *testing.T) {
	tree := &LazyTree{}

	a := tree.internKey("pkg.Fn@a.go:1:1")
	b := tree.internKey("pkg.Other@b.go:2:2")
	if a == b {
		t.Fatal("two distinct keys share a handle")
	}
	// Interning is idempotent: the same key must not consume a second slot, or
	// onPath and the per-scope counters would stop recognising the same call.
	if again := tree.internKey("pkg.Fn@a.go:1:1"); again != a {
		t.Errorf("re-interning gave %d, want %d", again, a)
	}
	if got := tree.keyString(a); got != "pkg.Fn@a.go:1:1" {
		t.Errorf("keyString(%d) = %q", a, got)
	}
	if got := tree.keyString(b); got != "pkg.Other@b.go:2:2" {
		t.Errorf("keyString(%d) = %q", b, got)
	}

	// The first handle is 0, so a zero key is a REAL key rather than "unset".
	// instanceScope therefore reports the wiring level as -1, not 0, and an
	// out-of-range handle resolves to "" rather than panicking on a reporting
	// path.
	if a != 0 {
		t.Errorf("first handle = %d, want 0 (the -1 wiring sentinel depends on it)", a)
	}
	if got := tree.keyString(-1); got != "" {
		t.Errorf("keyString(-1) = %q, want the empty string", got)
	}
	if got := tree.keyString(int32(len(tree.keyStrings))); got != "" {
		t.Errorf("keyString(out of range) = %q, want the empty string", got)
	}
}

// TestInstanceScopeReportsWiringLevelDistinctly pins the sentinel: with handles
// starting at 0, a node with no argument ancestor cannot report 0 without
// colliding with whatever key happens to be interned first.
func TestInstanceScopeReportsWiringLevelDistinctly(t *testing.T) {
	tree := &LazyTree{}
	first := tree.internKey("the-first-key")

	wiring := &LazyNode{tree: tree, key: first}
	if got := wiring.instanceScope(); got != -1 {
		t.Errorf("a node with no argument ancestor reported scope %d, want -1", got)
	}

	arg := &LazyNode{tree: tree, key: first, isArgument: true}
	child := &LazyNode{tree: tree, key: tree.internKey("child"), parent: arg}
	if got := child.instanceScope(); got != first {
		t.Errorf("scope = %d, want the argument ancestor's key %d", got, first)
	}
}
