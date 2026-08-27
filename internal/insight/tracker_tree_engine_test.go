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

package insight

import (
	"testing"

	"github.com/ehabterra/apispec/internal/metadata"
	"github.com/ehabterra/apispec/internal/spec"
)

// TestCachedTrackerTreeUsesTheEngineTree pins issue #410: insight must analyse
// the same kind of tree the spec was generated from.
//
// It built an EAGER tree here while the engine has generated with the lazy one
// by default for several releases. The two do not merely differ in speed — on a
// real service the eager tree reaches 194 of 280 routes — so every metric and
// trace the panel reported was measured against a tree the spec never saw.
//
// A type assertion is the point rather than a proxy for it: there is no output
// difference to assert on a synthetic fixture (both engines agree at this size),
// and the defect was precisely that the wrong constructor was called.
func TestCachedTrackerTreeUsesTheEngineTree(t *testing.T) {
	analysisMu.Lock()
	defer analysisMu.Unlock()

	meta := &metadata.Metadata{
		StringPool: metadata.NewStringPool(),
		Callers:    map[string][]*metadata.CallGraphEdge{},
		Packages:   map[string]*metadata.Package{},
	}

	tree := cachedTrackerTreeLocked(meta, nil)
	if tree == nil {
		t.Fatal("no tree built")
	}
	if _, ok := tree.(*spec.LazyTree); !ok {
		t.Fatalf("insight built a %T; the engine generates the spec with *spec.LazyTree, "+
			"so the panel would describe a tree the spec never used (#410)", tree)
	}
}

// TestCachedTrackerTreeRekeysOnConfig covers the memo key.
//
// The config selects the entrypoint patterns and the route matcher the tree is
// rooted with, so a tree built under one framework config does not answer for
// another. Keyed on the metadata alone — which is how it was keyed while the
// config was not a parameter at all — the first caller's framework would be
// served to every later one.
func TestCachedTrackerTreeRekeysOnConfig(t *testing.T) {
	analysisMu.Lock()
	defer analysisMu.Unlock()

	meta := &metadata.Metadata{
		StringPool: metadata.NewStringPool(),
		Callers:    map[string][]*metadata.CallGraphEdge{},
		Packages:   map[string]*metadata.Package{},
	}

	gin, chi := spec.DefaultGinConfig(), spec.DefaultChiConfig()

	first := cachedTrackerTreeLocked(meta, gin)
	if again := cachedTrackerTreeLocked(meta, gin); again != first {
		t.Error("same metadata and config rebuilt the tree — the memo is not working")
	}
	if other := cachedTrackerTreeLocked(meta, chi); other == first {
		t.Error("a different framework config reused the previous tree, so it is rooted for the wrong framework")
	}
	// And back again: the key must not be a one-way latch.
	if back := cachedTrackerTreeLocked(meta, gin); back == nil {
		t.Error("no tree built after switching config back")
	}
}
