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
// chained call or through a variable. Every cell resolves, and so does the
// composition of the two axes.
//
// Both halves were separate bugs. The bare-selector column was net/http's
// `^Handle$` route pattern carrying no receiver constraint, unlike its
// `^HandleFunc$` sibling: it matched on the call NAME alone, and `Handle` is
// what a house router calls its own registration method, so argument 0 of
// `Combo.Handle(listDirectBare)` — the HANDLER — was read as the path and the
// real registration inside the method was never reached. The variable row was
// the resolving rung walking ChainParent, which a call on a variable does not
// have; the receiver comes from an assignment instead, and that is where its
// constructor's argument is now read from.
func TestTestdata_BuilderChainVarPath(t *testing.T) {
	out, gen := generateWithReports(t, "builder_chain_var_path")
	noDanglingRefs(t, out)

	for _, path := range []string{
		"/direct-concat", "/direct-bare",
		"/var-concat", "/var-bare", "/var-chained",
		"/reused-first",
		"/plain",
	} {
		if _, ok := out.Paths[path]; !ok {
			t.Errorf("path %q missing; have %v", path, mapPathKeys(out.Paths))
		}
	}

	// The two chains registering through `Combo.Get` must stay two routes. They
	// share one call site — the `HandleFunc` inside Get — which is the shape
	// that used to collapse onto a single route (#465/#505).
	for _, path := range []string{"/direct-concat", "/var-concat"} {
		if item, ok := out.Paths[path]; ok && item.Get == nil {
			t.Errorf("%s has no GET, so the two chains through Combo.Get collapsed", path)
		}
	}

	// Continuing a chain from a variable reaches the constructor one hop past
	// the verb the variable holds, so BOTH verbs land on the same path.
	if item, ok := out.Paths["/var-chained"]; ok {
		if item.Get == nil {
			t.Error("/var-chained has no GET (r.Combo(…).Get, whose call the variable holds)")
		}
		if item.Post == nil {
			t.Error("/var-chained has no POST — the walk stopped at the verb instead of reaching the constructor")
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

	// NOT RESOLVED, and asserted so the day it changes. One variable reused for
	// two builders resolves at the FIRST registration and not at the second:
	// both writes are above that call, so both are candidates for the value and
	// they disagree, which the agreement rule answers by declining.
	//
	// That is #436's known limit, shared with every path held in a variable —
	// the scope is keyed by NAME, and a superseded write cannot simply be
	// dropped because a loop can carry one back. The route is reported rather
	// than silently missing, which is the honest answer (golden rule #7), and
	// this block flips when the limit is lifted.
	if _, ok := out.Paths["/reused-second"]; ok {
		t.Error("/reused-second is documented — a reused builder variable now takes the value in " +
			"effect at each registration (#436): assert it as present and drop this block")
	}
	reports := gen.UnresolvedPaths()
	if len(reports) != 1 {
		t.Errorf("want the second registration on the reused variable reported, got %d: %+v",
			len(reports), reports)
	}
	for _, r := range reports {
		if r.Position == "" {
			t.Errorf("report %+v names no registration site, so a reader cannot find it", r)
		}
	}
}
