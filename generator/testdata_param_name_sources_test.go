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
	"testing"

	"github.com/ehabterra/apispec/internal/spec"
)

// paramNamesAt returns the (name, in) pairs documented for a path's first
// operation.
func paramNamesAt(t *testing.T, out *spec.OpenAPISpec, path string) [][2]string {
	t.Helper()
	item, ok := out.Paths[path]
	if !ok {
		t.Fatalf("path %q missing; have %v", path, mapPathKeys(out.Paths))
	}
	op := firstOperation(&item)
	if op == nil {
		t.Fatalf("no operation on %q", path)
	}
	var got [][2]string
	for _, p := range op.Parameters {
		got = append(got, [2]string{p.Name, p.In})
	}
	return got
}

// A parameter's name is read through the same value-resolution ladder as a
// registration path, so the normal ways of writing a header name all produce
// the same parameter a literal does (issue #453).
//
// The two failure cases are the point of the fixture as much as the successes:
// a name that cannot be evaluated, and one the code rewrites on a branch, are
// LEFT OUT rather than guessed. A wrong name is worse than a missing one, and
// an empty one makes the document invalid (issue #452).
func TestTestdata_ParamNameSources(t *testing.T) {
	out := loadTestdata(t, "param_name_sources", spec.DefaultHTTPConfig())
	noDanglingRefs(t, out)

	resolved := map[string][2]string{
		"/literal":    {"X-Literal", "header"},    // control: always worked
		"/localvar":   {"X-Local", "header"},      // #453
		"/queryvar":   {"page", "query"},          // #453, and not header-specific
		"/conversion": {"X-Converted", "header"},  // #433's rung, for a name
		"/pkgconst":   {"X-Request-ID", "header"}, // the shape real code uses
		"/pkgvar":     {"X-Trace", "header"},
		"/crosspkg":   {"X-From-Package", "header"}, // const in another package
		"/viaparam":   {"X-Via-Param", "header"},    // passed in by a wrapper
	}
	for path, want := range resolved {
		got := paramNamesAt(t, out, path)
		found := false
		for _, g := range got {
			if g == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("%s: want parameter %q in %q, got %v", path, want[0], want[1], got)
		}
	}

	// Honest failure: neither may be guessed, and neither may be emitted with
	// an empty name.
	for _, path := range []string{"/unresolvable", "/ambiguous"} {
		if got := paramNamesAt(t, out, path); len(got) != 0 {
			t.Errorf("%s: want no parameter (the name cannot be resolved), got %v", path, got)
		}
	}

	// Change detector, not an endorsement: a field of a PACKAGE-LEVEL struct
	// value (`hdr.Config.Key`) is still not resolved — structFieldValue is
	// scoped to a struct the caller passed in. Tracked in #455; when that is
	// fixed this flips and the case moves up into `resolved` above.
	if got := paramNamesAt(t, out, "/structfield"); len(got) != 0 {
		t.Errorf("/structfield resolved (#455 fixed?) — move this case into the resolved table: %v", got)
	}

	// Nothing anywhere may carry an empty name (issue #452).
	for path, item := range out.Paths {
		op := firstOperation(&item)
		if op == nil {
			continue
		}
		for _, p := range op.Parameters {
			if p.Name == "" && p.Ref == "" {
				t.Errorf("%s: parameter with no name emitted: %+v", path, p)
			}
		}
	}
}
