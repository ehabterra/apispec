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

// The 2x2 of how a builder's path reaches its registration (issue #506): the
// receiver field concatenated with a literal or passed whole, reached by a
// chained call or through a variable.
//
// Half of it is fixed here and half is not, so the unfixed half is asserted as
// it currently BEHAVES — with the reason — rather than left unmentioned. When
// the variable shape is resolved this test fails and says what to change.
func TestTestdata_BuilderChainVarPath(t *testing.T) {
	out, gen := generateWithReports(t, "builder_chain_var_path")
	noDanglingRefs(t, out)

	// FIXED: a route registered through a house method named `Handle`.
	//
	// net/http's `^Handle$` route pattern carried no receiver constraint, unlike
	// its `^HandleFunc$` sibling, so it matched on the NAME alone — and `Handle`
	// is what a house router calls its own registration method. Argument 0 of
	// `Combo.Handle(listDirectBare)` is the HANDLER, and it was read as the path.
	for _, path := range []string{"/direct-concat", "/direct-bare", "/plain"} {
		if _, ok := out.Paths[path]; !ok {
			t.Errorf("path %q missing; have %v", path, mapPathKeys(out.Paths))
		}
	}

	// No path may be built out of a handler's name. `{listDirectBare}` is what
	// the unscoped pattern produced, and it is not an endpoint any client can
	// call.
	for path := range out.Paths {
		for _, handler := range []string{"listDirectBare", "listVarBare", "listItems"} {
			if strings.Contains(path, handler) {
				t.Errorf("path %q is built from a handler name, not a path", path)
			}
		}
	}

	// NOT FIXED, and asserted so the day it changes: a chain assigned to a
	// variable first loses the receiver's value, because the resolving rung
	// walks ChainParent and a call on a variable has none.
	//
	// The important half is that they are REPORTED. Before the `Handle` scoping
	// above, the two bare shapes produced no path, no placeholder and no
	// diagnostic — the document simply omitted them and read as finished.
	for _, path := range []string{"/var-concat", "/var-bare"} {
		if _, ok := out.Paths[path]; ok {
			t.Errorf("path %q is documented — the variable-assigned builder chain now resolves, "+
				"so issue #506's second half is fixed: assert these as present and drop this block",
				path)
		}
	}
	reports := gen.UnresolvedPaths()
	if len(reports) != 2 {
		t.Errorf("want both variable-assigned registrations reported as runtime paths, got %d: %+v",
			len(reports), reports)
	}
	for _, r := range reports {
		if r.Position == "" {
			t.Errorf("report %+v names no registration site, so a reader cannot find it", r)
		}
	}
}
