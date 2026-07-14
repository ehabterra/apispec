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
	"reflect"
	"testing"
)

func TestChainInterner(t *testing.T) {
	t.Run("empty chain is handle 0 and reconstructs to nil", func(t *testing.T) {
		ci := &chainInterner{}
		if got := ci.strings(0); got != nil {
			t.Errorf("strings(0) = %v, want nil", got)
		}
	})

	t.Run("handles are 1-based, stable, and value-interned", func(t *testing.T) {
		ci := &chainInterner{}
		a := ci.push(0, "a@1")
		if a != 1 {
			t.Fatalf("first handle = %d, want 1", a)
		}
		// Same (parent, callee) must reuse the handle — this is what makes
		// interning equivalent to string-key equality for dedupe.
		if again := ci.push(0, "a@1"); again != a {
			t.Errorf("re-push returned %d, want %d", again, a)
		}
		// A sibling with the same parent but different callee is distinct.
		b := ci.push(0, "b@2")
		if b == a {
			t.Errorf("sibling collided with %d", a)
		}
		// The same callee under a different parent is distinct too.
		ab := ci.push(a, "b@2")
		if ab == b || ab == a {
			t.Errorf("nested handle %d collided (a=%d, b=%d)", ab, a, b)
		}
		if len(ci.steps) != 3 {
			t.Errorf("interned %d steps, want 3", len(ci.steps))
		}
	})

	t.Run("strings reconstructs root-to-leaf", func(t *testing.T) {
		ci := &chainInterner{}
		a := ci.push(0, "root@1")
		b := ci.push(a, "mid@2")
		c := ci.push(b, "leaf@3")

		cases := []struct {
			handle int
			want   []string
		}{
			{a, []string{"root@1"}},
			{b, []string{"root@1", "mid@2"}},
			{c, []string{"root@1", "mid@2", "leaf@3"}},
		}
		for _, tc := range cases {
			if got := ci.strings(tc.handle); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("strings(%d) = %v, want %v", tc.handle, got, tc.want)
			}
		}

		// Reconstruction must not disturb interning state: pushing the same
		// steps again still reuses the existing handles.
		if ci.push(a, "mid@2") != b || ci.push(b, "leaf@3") != c {
			t.Errorf("handles not stable after reconstruction")
		}
	})

	t.Run("diamond: two paths to the same callee stay distinct", func(t *testing.T) {
		// route -> f -> shared and route -> g -> shared: the SAME call-site
		// reached through different frames must produce different chains —
		// that distinction is what keeps two invocations of one helper apart
		// in response pairing.
		ci := &chainInterner{}
		f := ci.push(0, "f@1")
		g := ci.push(0, "g@2")
		viaF := ci.push(f, "shared@3")
		viaG := ci.push(g, "shared@3")
		if viaF == viaG {
			t.Fatalf("diamond collapsed: %d == %d", viaF, viaG)
		}
		if got := ci.strings(viaF); !reflect.DeepEqual(got, []string{"f@1", "shared@3"}) {
			t.Errorf("via f = %v", got)
		}
		if got := ci.strings(viaG); !reflect.DeepEqual(got, []string{"g@2", "shared@3"}) {
			t.Errorf("via g = %v", got)
		}
	})
}
