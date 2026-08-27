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

	"github.com/ehabterra/apispec/spec"
)

// TestTestdata_FiberConstraints locks in the correctness half of issue #357:
// fiber writes a route constraint in angle brackets after the parameter name,
// and it is not part of the URL.
//
//	app.Get("/items/:id<int>", getItem)
//
// The tail used to survive into the path key — `/items/{id}<int>` — which is not
// a legal OpenAPI path template and matches no request a client can make, so the
// operation was keyed under a path that could never be routed.
//
// The parameter SCHEMAS are deliberately not asserted beyond `type: string`:
// turning `<int>` into `type: integer` is the second half of #357 and is not
// done here. Honest-over-wrong (golden rule #7) — an unrecognised constraint
// strips cleanly and leaves the general type rather than guessing one.
func TestTestdata_FiberConstraints(t *testing.T) {
	out := loadTestdata(t, "fiber_constraints", spec.DefaultFiberConfig())
	noDanglingRefs(t, out)
	noNullSchemas(t, out)

	want := []string{
		"/items/{id}",   // <int>
		"/users/{uid}",  // <guid>
		"/events/{day}", // <datetime(2006-01-02)> — parens hold arbitrary text
		"/pages/{n}",    // <min(1);max(500)> — a ';'-chained list
		"/plain/{name}", // unconstrained, must be untouched
	}
	for _, p := range want {
		if _, ok := out.Paths[p]; !ok {
			t.Errorf("path %q missing; have %v", p, mapPathKeys(out.Paths))
		}
	}
	if len(out.Paths) != len(want) {
		t.Errorf("got %d paths, want %d: %v", len(out.Paths), len(want), mapPathKeys(out.Paths))
	}

	// The defect, stated directly: no constraint syntax may survive anywhere in
	// a path key.
	for p := range out.Paths {
		if strings.ContainsAny(p, "<>") {
			t.Errorf("path %q still carries a route constraint — not a valid OpenAPI template", p)
		}
	}

	// The parameter must still be declared and named, so stripping the tail did
	// not take the parameter with it.
	item, ok := out.Paths["/items/{id}"]
	if !ok {
		t.Fatal("/items/{id} missing")
	}
	op := opFor(item, "GET")
	if op == nil {
		t.Fatal("/items/{id}: no GET operation")
	}
	var found bool
	for _, p := range op.Parameters {
		if p.Name == "id" && p.In == "path" {
			found = true
		}
	}
	if !found {
		t.Errorf("/items/{id}: no `id` path parameter; have %+v", op.Parameters)
	}
}
