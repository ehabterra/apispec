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

import "testing"

// TestCandidateKey carries over what the chain interner's tests asserted, since
// the key it produces replaced the interned handle as the response-candidate
// dedupe identity (issue #319). What matters is unchanged: equal (chain, callee)
// means one candidate, and anything else means separate candidates.
func TestCandidateKey(t *testing.T) {
	t.Run("value equality dedupes", func(t *testing.T) {
		// Two separate walks reaching the same call through the same frames
		// must agree, which is what makes this usable as a dedupe key.
		first, second := []string{"a@1"}, []string{"a@1"}
		if got, want := candidateKey(second, "b@2"), candidateKey(first, "b@2"); got != want {
			t.Errorf("the same chain and callee produced %q and %q", got, want)
		}
		if got, want := candidateKey(nil, "a@1"), candidateKey([]string{}, "a@1"); got != want {
			t.Errorf("nil chain gave %q, empty chain gave %q — they are the same chain", got, want)
		}
	})

	t.Run("siblings and depth stay distinct", func(t *testing.T) {
		distinct := map[string]string{
			"root sibling a": candidateKey(nil, "a@1"),
			"root sibling b": candidateKey(nil, "b@2"),
			"nested under a": candidateKey([]string{"a@1"}, "b@2"),
			"nested under b": candidateKey([]string{"b@2"}, "a@1"),
			"two deep":       candidateKey([]string{"a@1", "b@2"}, "c@3"),
		}
		seen := map[string]string{}
		for name, key := range distinct {
			if other, dup := seen[key]; dup {
				t.Errorf("%q and %q produced the same key %q", name, other, key)
			}
			seen[key] = name
		}
	})

	// The case the interner test called "diamond", and the one #269 was about:
	// route -> f -> shared and route -> g -> shared reach the SAME call site
	// through different frames. Collapsing them would let two routes resolve to
	// each other's response body.
	t.Run("same callee through different frames stays distinct", func(t *testing.T) {
		viaF := candidateKey([]string{"f@1"}, "shared@3")
		viaG := candidateKey([]string{"g@2"}, "shared@3")
		if viaF == viaG {
			t.Fatalf("diamond collapsed: both chains produced %q", viaF)
		}
	})

	// Frame boundaries must be unambiguous whatever an instance ID contains.
	// The interned handle this replaced compared (parent, callee) structurally,
	// so it could not confuse a two-frame chain with a one-frame chain whose
	// frame happens to contain a separator; a plain join would. Length
	// prefixing keeps that property.
	t.Run("chain boundaries are not ambiguous", func(t *testing.T) {
		for _, sep := range []string{chainSep, ":", "", "\x00"} {
			if candidateKey([]string{"a", "b"}, "c") == candidateKey([]string{"a"}, "b"+sep+"c") {
				t.Errorf("boundary ambiguous when a frame contains %q", sep)
			}
			if candidateKey([]string{"a" + sep + "b"}, "c") == candidateKey([]string{"a"}, sep+"b"+chainSep+"c") {
				t.Errorf("boundary ambiguous when a frame contains %q (leading)", sep)
			}
		}
	})
}

// TestExtractionChainBufferIsRestored covers the walk's push/pop discipline:
// one buffer is reused for a whole route walk, so a subtree that returns must
// leave the chain exactly as it found it. A missed pop would leak frames into
// every later sibling's candidate keys.
func TestExtractionChainBufferIsRestored(t *testing.T) {
	chain := make([]string, 0, 4)

	var descend func(depth int)
	descend = func(depth int) {
		if depth == 0 {
			return
		}
		before := len(chain)
		chain = append(chain, "frame")
		descend(depth - 1)
		chain = chain[:len(chain)-1]
		if len(chain) != before {
			t.Fatalf("depth %d: chain length %d after unwinding, want %d", depth, len(chain), before)
		}
	}
	descend(10)

	if len(chain) != 0 {
		t.Errorf("chain length %d after the walk, want 0", len(chain))
	}
}
