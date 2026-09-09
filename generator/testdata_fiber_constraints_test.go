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
// The constraint also STATES something about the value, and that half is now
// read: `<int>` is `type: integer`, `<guid>` is a uuid-formatted string, and
// `<min(1);max(500)>` carries its bounds. An unrecognised constraint still
// strips cleanly and leaves the general type rather than guessing one
// (golden rule #7).
func TestTestdata_FiberConstraints(t *testing.T) {
	out := loadTestdata(t, "fiber_constraints", spec.DefaultFiberConfig())
	noDanglingRefs(t, out)
	noNullSchemas(t, out)

	want := []string{
		"/items/{id}",    // <int>
		"/users/{uid}",   // <guid>
		"/events/{day}",  // <datetime(2006-01-02)> — parens hold arbitrary text
		"/pages/{n}",     // <min(1);max(500)> — a ';'-chained list
		"/plain/{name}",  // unconstrained, must be untouched
		"/unread/{code}", // <int>, and NOT read in the handler
		"/odd/{x}",       // a constraint the table does not know
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

	// What each constraint states about the value.
	type schemaWant struct {
		typ, format string
		min, max    float64
	}
	cases := map[string]schemaWant{
		"/items/{id}":    {typ: "integer"},                   // <int>
		"/users/{uid}":   {typ: "string", format: "uuid"},    // <guid>
		"/pages/{n}":     {typ: "integer", min: 1, max: 500}, // <min(1);max(500)>
		"/unread/{code}": {typ: "integer"},                   // <int>, unread by the handler
		"/plain/{name}":  {typ: "string"},                    // unconstrained
		// The layout in <datetime(2006-01-02)> decides whether the format is
		// `date` or `date-time`, and reading it is a parse rather than a table
		// row, so nothing is claimed (golden rule #7).
		"/events/{day}": {typ: "string"},
		// An unrecognised constraint states nothing at all.
		"/odd/{x}": {typ: "string"},
	}
	for path, w := range cases {
		item, ok := out.Paths[path]
		if !ok {
			continue // the presence check above already reported it
		}
		op := opFor(item, "GET")
		if op == nil || len(op.Parameters) == 0 {
			t.Errorf("%s: no parameters", path)
			continue
		}
		p := op.Parameters[0]
		if p.Schema == nil {
			t.Errorf("%s: parameter %q has no schema", path, p.Name)
			continue
		}
		if p.Schema.Type != w.typ || p.Schema.Format != w.format {
			t.Errorf("%s: schema = {type:%q format:%q}, want {type:%q format:%q}",
				path, p.Schema.Type, p.Schema.Format, w.typ, w.format)
		}
		if p.Schema.Minimum != w.min || p.Schema.Maximum != w.max {
			t.Errorf("%s: bounds = [%v,%v], want [%v,%v]",
				path, p.Schema.Minimum, p.Schema.Maximum, w.min, w.max)
		}
	}

	// A parameter the ROUTE typed is better attested than one a handler happens
	// to read, so warning that it was "not found in the code" reads as a defect
	// where there is none. An unrecognised constraint has told us nothing, so
	// its warning stays.
	unread := opFor(out.Paths["/unread/{code}"], "GET")
	if unread != nil && len(unread.Parameters) > 0 {
		if _, warned := unread.Parameters[0].Extensions["x-warning"]; warned {
			t.Error("/unread/{code}: the route pattern types this parameter, so it must not be " +
				"reported as not found in the code")
		}
	}
	odd := opFor(out.Paths["/odd/{x}"], "GET")
	if odd != nil && len(odd.Parameters) > 0 {
		if _, warned := odd.Parameters[0].Extensions["x-warning"]; !warned {
			t.Error("/odd/{x}: an unrecognised constraint states nothing, so the parameter is still " +
				"only attested by the path")
		}
	}
}
