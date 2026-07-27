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

import "testing"

// TestTestdata_CLIActionCrossPackage is the multi-package half of issue #143, and
// the shape real projects actually have: the command type, the dispatcher, main,
// and the route registration all live in different packages.
//
// This is deliberately separate from the single-package fixture because every
// step of the resolution changes when a package boundary is crossed, and the
// single-package form hides all of it (golden rule #10):
//
//   - the literal names its type through a selector (`clipkg.Command{…}`), so the
//     type's owner is not the package writing the literal;
//   - an *elided* element literal (`{Name: "admin", Action: srv.RunAdmin}` inside
//     `[]*clipkg.Command{…}`) states no type at all and has to inherit both the
//     name and the package from the slice;
//   - the Action values are a cross-package function (`web.RunWeb`, reached
//     through the file's import table since a rendered value only carries the
//     source-level qualifier) and a method value (`srv.RunAdmin`, whose key needs
//     the receiver type as well as the package).
//
// Each of those was a separate zero-route failure while this was built.
func TestTestdata_CLIActionCrossPackage(t *testing.T) {
	out := loadTestdataWithFixtureConfig(t, "cli_action_cross_package", nil)
	noDanglingRefs(t, out)
	noUnresolvedPlaceholders(t, out)

	// Cross-package function value, reached from a package-level command var.
	users, ok := out.Paths["/users"]
	if !ok {
		t.Fatalf("/users missing — a cross-package Action value was not resolved (#143); have %v",
			mapPathKeys(out.Paths))
	}
	get := opFor(users, "GET")
	post := opFor(users, "POST")
	if get == nil || post == nil {
		t.Fatalf("/users: want GET and POST, got get=%v post=%v", get != nil, post != nil)
	}
	if _, hasOK := get.Responses["200"]; !hasOK {
		t.Errorf("GET /users: 200 response missing — the handler body was not reached; have %v",
			keysOf(get.Responses))
	}
	if post.RequestBody == nil {
		t.Error("POST /users: requestBody missing — the handler body was not reached")
	}

	// Method value on a receiver, through an elided element literal.
	report := opFor(out.Paths["/admin/report"], "GET")
	if report == nil {
		t.Fatalf("GET /admin/report missing — a method-value Action was not resolved (#143); have %v",
			mapPathKeys(out.Paths))
	}
	if _, hasOK := report.Responses["200"]; !hasOK {
		t.Errorf("GET /admin/report: 200 response missing; have %v", keysOf(report.Responses))
	}
}
