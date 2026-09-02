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

package generator

import (
	"strings"
	"testing"
)

// TestTestdata_VariablePath locks in reading a registration path out of a local
// variable (issue #431).
//
// Before it, `p := "/users"` two lines above the registration was treated as
// unreadable: reported and left out by #428 when it was one operand of a
// concatenation, and — when the whole path was the variable — rendered as the
// variable's TYPE, so `/string` appeared in the document as an endpoint.
//
// The fixture puts the resolvable and the genuinely ambiguous side by side,
// because the value of this change is that they end up different.
func TestTestdata_VariablePath(t *testing.T) {
	out, gen := generateWithReports(t, "variable_path")
	noDanglingRefs(t, out)
	noUnresolvedPlaceholders(t, out)

	for _, want := range []string{
		"/users",     // the whole path in one variable
		"/bare",      // the same, passed as the bare argument
		"/aliased",   // an alias chain
		"/api/items", // a variable prefix with a literal tail
		"/v2/joined", // a variable assembled from parts that all resolve
	} {
		if _, ok := out.Paths[want]; !ok {
			t.Errorf("missing %s; documented %v", want, mapPathKeys(out.Paths))
		}
	}

	// Nothing may be documented under a variable's TYPE: that is the wrong
	// answer this replaces.
	for path := range out.Paths {
		if path == "/string" || strings.HasPrefix(path, "/string/") {
			t.Errorf("path %q is the variable's TYPE, not its value", path)
		}
	}

	// CHANGE DETECTOR, not an endorsement (issue #436): assignments are matched
	// by NAME, so a shadowed binding and a write after the registration each
	// look like two disagreeing values and keep a placeholder. Both paths ARE
	// knowable. Asserted at today's conservative behaviour — approximate rather
	// than wrong — so this fails, and gets updated, when #436 lands.
	t.Run("a shadowed binding and a later write are not read yet", func(t *testing.T) {
		for _, notYet := range []string{"/outer/a", "/inner/b", "/first/c"} {
			if _, ok := out.Paths[notYet]; ok {
				t.Errorf("%s now resolves — assignments are matched per binding; "+
					"assert it properly here and close #436", notYet)
			}
		}
		for _, stillApprox := range []string{"/{shadowed}/a", "/{shadowed}/b", "/{later}/c"} {
			if _, ok := out.Paths[stillApprox]; !ok {
				t.Errorf("%s is gone — the route must stay documented (approximately) "+
					"while its prefix is unreadable; have %v", stillApprox, mapPathKeys(out.Paths))
			}
		}
	})

	// The two that cannot be read are reported rather than guessed at: one
	// assigned in two branches, one assigned from a call.
	reports := gen.UnresolvedPaths()
	if len(reports) != 2 {
		t.Fatalf("want the ambiguous and the runtime-built registrations reported, got %d: %+v",
			len(reports), reports)
	}
	handlers := map[string]bool{}
	for _, r := range reports {
		handlers[r.Handler[strings.LastIndexByte(r.Handler, '.')+1:]] = true
		if r.Position == "" {
			t.Errorf("report %+v should locate the registration", r)
		}
	}
	for _, want := range []string{"ambiguous", "fromCall"} {
		if !handlers[want] {
			t.Errorf("handler %s should be reported; got %v", want, handlers)
		}
	}
	// Neither may be documented at whichever branch the walk saw last.
	for _, notWanted := range []string{"/first", "/second", "/built"} {
		if _, ok := out.Paths[notWanted]; ok {
			t.Errorf("%s is documented, but the path is not knowable — picking one is a guess", notWanted)
		}
	}
}
