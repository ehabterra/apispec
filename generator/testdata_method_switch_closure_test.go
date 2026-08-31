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

// TestTestdata_MethodSwitchClosure locks in method dispatch written in a
// CLOSURE (issue #382). A function literal has no Function record, so its
// `switch r.Method` used to be invisible from the route: every one of these
// paths was documented as a single POST carrying every arm's responses.
//
// The fixture puts the shapes side by side, and each assertion below is about a
// different way the attribution can go wrong:
//
//   - /inline, /inline-second — two closures registered from ONE function, so a
//     file-scoped attribution would mix their arms;
//   - /via-methods — the arms call methods, so the response statement is a
//     frame deeper and only the CALL CHAIN says which arm reached it;
//   - /from-factory — the dispatch is in a closure the handler function returns;
//   - /from-method — registered from a method, which has no Function record at
//     all, so the dispatch has to be recorded from a whole-file walk;
//   - /explicit — a registration that named its verb: one operation, scoped to
//     that verb's arm.
func TestTestdata_MethodSwitchClosure(t *testing.T) {
	out := loadTestdataWithFixtureConfig(t, "method_switch_closure", spec.DefaultHTTPConfig())
	noDanglingRefs(t, out)
	noUnresolvedPlaceholders(t, out)

	want := map[string][]string{
		"/inline":        {"GET", "POST"},
		"/inline-second": {"PUT", "DELETE"},
		"/via-methods":   {"GET", "POST"},
		"/from-factory":  {"GET", "POST"},
		"/from-method":   {"GET", "DELETE"},
		"/explicit":      {"GET"},
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
		// Nothing may keep the ExtractRoute POST default it used to fall to.
		if !contains(methods, "POST") && opFor(item, "POST") != nil {
			t.Errorf("%s should not carry a POST operation", path)
		}
	}

	// Per-arm bodies: the POST arm's request body and 201 belong to POST alone.
	for _, path := range []string{"/inline", "/via-methods", "/from-factory"} {
		item := out.Paths[path]
		post, get := opFor(item, "POST"), opFor(item, "GET")
		if post == nil || post.RequestBody == nil {
			t.Errorf("POST %s should carry a request body", path)
		}
		if get == nil || get.RequestBody != nil {
			t.Errorf("GET %s should not carry a request body", path)
		}
		if post != nil {
			if _, ok := post.Responses["201"]; !ok {
				t.Errorf("POST %s should carry the 201 written in its arm", path)
			}
		}
		if get != nil {
			if _, ok := get.Responses["200"]; !ok {
				t.Errorf("GET %s should carry the 200 written in its arm", path)
			}
			if _, ok := get.Responses["201"]; ok {
				t.Errorf("GET %s must not carry the POST arm's 201", path)
			}
		}
	}

	// The `default:` arm's 405 names no verb, so it belongs to no operation.
	for _, m := range []string{"GET", "POST"} {
		if op := opFor(out.Paths["/inline"], m); op != nil {
			if _, ok := op.Responses["405"]; ok {
				t.Errorf("%s /inline must not carry the default arm's 405", m)
			}
		}
	}

	// The second closure in the same function keeps its own arms: DELETE is
	// bodyless (204) and must not pick up the PUT arm's user body.
	if del := opFor(out.Paths["/inline-second"], "DELETE"); del != nil {
		if del.RequestBody != nil {
			t.Errorf("DELETE /inline-second should not carry a request body")
		}
		if _, ok := del.Responses["200"]; ok {
			t.Errorf("DELETE /inline-second must not carry the PUT arm's 200")
		}
	}

	// An explicit verb is not split, and the arms the router never sends it are
	// not documented on it either.
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
		for _, m := range []string{"GET", "POST", "PUT", "DELETE", "HEAD"} {
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

func contains(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}
