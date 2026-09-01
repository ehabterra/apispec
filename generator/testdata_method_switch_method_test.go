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

	"github.com/ehabterra/apispec/spec"
)

// TestTestdata_MethodSwitchMethod locks in method dispatch written in a METHOD
// (issue #427) — the last handler shape that did not split, after plain
// functions and closures (#382).
//
// Each path is a different way the resolution can fail:
//
//   - /users — a pointer receiver, the ordinary shape;
//   - /things — a value receiver, whose identity renders with the metadata type
//     separator rather than a dot;
//   - /items — a method in another package, where the handler identity carries
//     the SHORT package name while the route's package is the import path, so
//     the two cannot be string-compared;
//   - /user — arms that delegate to other methods, so the body is a frame deeper
//     and only the call chain says which arm reached it;
//   - /explicit — a registration that named its verb: one operation, scoped to
//     that verb's arm.
func TestTestdata_MethodSwitchMethod(t *testing.T) {
	out := loadTestdataWithFixtureConfig(t, "method_switch_method", spec.DefaultHTTPConfig())
	noDanglingRefs(t, out)
	noUnresolvedPlaceholders(t, out)

	want := map[string][]string{
		"/users":    {"GET", "POST"},
		"/user":     {"GET", "PUT"},
		"/things":   {"GET", "PATCH"},
		"/items":    {"GET", "DELETE"},
		"/explicit": {"GET"},
	}
	for path, methods := range want {
		item, ok := out.Paths[path]
		if !ok {
			t.Errorf("path %q missing; have %v", path, mapPathKeys(out.Paths))
			continue
		}
		for _, m := range methods {
			if opFor(item, m) == nil {
				t.Errorf("%s %s: expected operation, missing", m, path)
			}
		}
		// None of these may keep the ExtractRoute POST default they fell to.
		if !contains(methods, "POST") && opFor(item, "POST") != nil {
			t.Errorf("%s should not carry a POST operation", path)
		}
	}

	// Per-arm bodies on the inline shape.
	users := out.Paths["/users"]
	post, get := opFor(users, "POST"), opFor(users, "GET")
	if post == nil || post.RequestBody == nil {
		t.Error("POST /users should carry the request body decoded in its arm")
	}
	if get == nil || get.RequestBody != nil {
		t.Error("GET /users should not carry a request body")
	}
	if get != nil {
		if _, ok := get.Responses["201"]; ok {
			t.Error("GET /users must not carry the POST arm's 201")
		}
	}
	// The `default:` arm names no verb, so its 405 belongs to no operation.
	for _, m := range []string{"GET", "POST"} {
		if op := opFor(users, m); op != nil {
			if _, ok := op.Responses["405"]; ok {
				t.Errorf("%s /users must not carry the default arm's 405", m)
			}
		}
	}

	// Delegating arms: the bodies live in other methods, and each still lands on
	// the verb whose arm called it.
	user := out.Paths["/user"]
	if put := opFor(user, "PUT"); put == nil || put.RequestBody == nil {
		t.Error("PUT /user should carry the body its delegate decodes")
	}
	if g := opFor(user, "GET"); g == nil || g.RequestBody != nil {
		t.Error("GET /user should not carry the PUT delegate's request body")
	}

	// An explicit verb is not split, and the arm the router never sends it is
	// not documented on it.
	explicit := opFor(out.Paths["/explicit"], "GET")
	if explicit == nil {
		t.Fatal("GET /explicit missing")
	}
	if _, ok := explicit.Responses["201"]; ok {
		t.Errorf("GET /explicit must not carry the POST arm's 201: %v", explicit.Responses)
	}

	// operationIds stay unique across every split.
	seen := map[string]bool{}
	for path := range want {
		for _, m := range []string{"GET", "POST", "PUT", "PATCH", "DELETE"} {
			op := opFor(out.Paths[path], m)
			if op == nil {
				continue
			}
			if op.OperationID == "" {
				t.Errorf("%s %s: empty operationId", m, path)
			}
			if seen[op.OperationID] {
				t.Errorf("duplicate operationId %q across the method split", op.OperationID)
			}
			seen[op.OperationID] = true
		}
	}
}
