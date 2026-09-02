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

	// Nothing may be documented under a variable's name or type: those are the
	// two wrong answers this replaces.
	for path := range out.Paths {
		if strings.Contains(path, "{") {
			t.Errorf("path %q keeps a placeholder — every variable here resolves", path)
		}
		if path == "/string" || strings.HasPrefix(path, "/string/") {
			t.Errorf("path %q is the variable's TYPE, not its value", path)
		}
	}

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
