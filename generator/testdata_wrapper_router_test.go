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

	intspec "github.com/ehabterra/apispec/internal/spec"
)

// wrapperRouterConfig describes the fixture's house router the way a project
// would: its own patterns ON TOP of chi's defaults, which stay active because the
// project still uses chi directly elsewhere.
//
// Keeping chi's patterns is the whole point of the test. They also match the
// delegate call inside the wrapper — where the path and handler are the wrapper's
// own parameters — and that used to win, producing one junk route (`/{path}` with
// `operationId: net/http.HandlerFunc`) and none of the real ones (issue #221).
func wrapperRouterConfig() *intspec.APISpecConfig {
	const recv = `^github\.com/ehabterra/apispec/testdata/wrapper_router\.\*?Router$`

	cfg := intspec.DefaultChiConfig()
	cfg.Framework.RoutePatterns = append([]intspec.RoutePattern{
		{
			// r.Get("/users", listUsers) — the verb is the method name.
			CallRegex:       `^(?i)(Get|Post|Put|Delete)$`,
			RecvTypeRegex:   recv,
			MethodFromCall:  true,
			PathFromArg:     true,
			PathArgIndex:    0,
			HandlerFromArg:  true,
			HandlerArgIndex: 1,
		},
		{
			// r.Methods("GET,POST", "/search", h) — the verb is an argument, and
			// may name several.
			CallRegex:       `^Methods$`,
			RecvTypeRegex:   recv,
			MethodFromArg:   true,
			MethodArgIndex:  0,
			PathFromArg:     true,
			PathArgIndex:    1,
			HandlerFromArg:  true,
			HandlerArgIndex: 2,
		},
	}, cfg.Framework.RoutePatterns...)

	// The group prefix lives in a field the wrapper applies itself, so the Group
	// call is what states it.
	cfg.Framework.MountPatterns = append([]intspec.MountPattern{
		{
			CallRegex:     `^Group$`,
			RecvTypeRegex: recv,
			PathFromArg:   true,
			PathArgIndex:  0,
			IsMount:       true,
		},
	}, cfg.Framework.MountPatterns...)

	return cfg
}

// TestTestdata_WrapperRouter documents a project that puts its own router type in
// front of chi — gitea's shape — and pins the three things that were wrong:
//
//  1. the framework's own patterns matched the DELEGATE inside the wrapper, and
//     the inner call's unresolved parameters overwrote the outer route's path and
//     handler;
//  2. a verb travelling as an argument (`Methods("GET,POST", …)`) could not be
//     configured, so such a registration fell back to the POST default;
//  3. a handler behind a type conversion (`http.HandlerFunc(fn)`) was taken as the
//     handler, losing its body.
func TestTestdata_WrapperRouter(t *testing.T) {
	out := loadTestdataWithFixtureConfig(t, "wrapper_router", wrapperRouterConfig())
	noDanglingRefs(t, out)
	noUnresolvedPlaceholders(t, out)

	// No route may be named after an unresolved parameter. `/{path}` is what the
	// delegate produced when it won: the wrapper's `pattern` parameter, rendered
	// as a placeholder.
	for path := range out.Paths {
		if path == "/{path}" || path == "/string" {
			t.Errorf("path %q comes from the wrapper's own parameters, not from a registration; have %v", path, mapPathKeys(out.Paths))
		}
	}

	cases := []struct {
		method, path, handler string
	}{
		// Registered through the wrapper's verb methods.
		{"GET", "/users", "listUsers"},
		{"POST", "/users", "createUser"},
		// Inside a Group, whose prefix the wrapper holds in a field.
		{"GET", "/api/items", "getItem"},
		// One registration naming two verbs.
		{"GET", "/api/items/search", "searchItems"},
		{"POST", "/api/items/search", "searchItems"},
		// The handler arrives behind http.HandlerFunc(...).
		{"GET", "/api/items/count", "countItems"},
	}
	for _, tc := range cases {
		op := opFor(out.Paths[tc.path], tc.method)
		if op == nil {
			t.Errorf("%s %s missing; have %v", tc.method, tc.path, mapPathKeys(out.Paths))
			continue
		}
		if !strings.Contains(op.OperationID, tc.handler) {
			t.Errorf("%s %s: operationId %q does not name the handler %q — the registration resolved to something else",
				tc.method, tc.path, op.OperationID, tc.handler)
		}
	}

	// The handler bodies come through, which is what the junk route lost: a
	// conversion or a `[]any` names a type, and a type has no request or response.
	if op := opFor(out.Paths["/users"], "POST"); op != nil {
		if op.RequestBody == nil {
			t.Error("POST /users: the handler decodes a User, so a request body is expected")
		}
		if _, ok := op.Responses["201"]; !ok {
			t.Errorf("POST /users: expected the handler's 201, have %v", statusKeys(op))
		}
	}

	// A helper handed the framework router directly must keep working: its inner
	// call is the only registration there is, so the fix must not reject it for
	// being inside another function.
	//
	// The helper builds its path by concatenation — `m.Delete(base+"/{id}", h)`
	// with `base` a parameter — and the caller passes "/users". That used to
	// resolve to nothing at all, documenting the route at "/", which said the
	// handler answers at the root. Concatenated paths now fold and the parameter
	// is followed to the caller's literal, so the route is documented where it
	// actually is (issue #274).
	if op := opFor(out.Paths["/users/{id}"], "DELETE"); op == nil {
		t.Errorf("the helper-registrar route is gone; have %v", mapPathKeys(out.Paths))
	} else if !strings.Contains(op.OperationID, "deleteUser") {
		t.Errorf("helper-registrar operationId = %q, want it to name deleteUser", op.OperationID)
	}
}

// TestTestdata_WrapperRouterIsDetected is the same fixture with NO wrapper
// patterns configured: chi's defaults only, exactly what a user gets by pointing
// apispec at the project.
//
// The wrapper is derived from the parameter flow — a method of the project's own
// type that forwards its own parameters into a framework registration is a
// registrar, and the framework call it delegates to says which parameter plays
// which role (issue #235). Before that, this run documented one junk route.
func TestTestdata_WrapperRouterIsDetected(t *testing.T) {
	out := loadTestdataWithFixtureConfig(t, "wrapper_router", intspec.DefaultChiConfig())
	noDanglingRefs(t, out)
	noUnresolvedPlaceholders(t, out)

	// The same set the hand-written config produces, including the group prefix
	// (held in a field, so only the Group call states it) and both verbs of the
	// one registration that names two.
	want := []struct{ method, path, handler string }{
		{"GET", "/users", "listUsers"},
		{"POST", "/users", "createUser"},
		{"GET", "/api/items", "getItem"},
		{"GET", "/api/items/search", "searchItems"},
		{"POST", "/api/items/search", "searchItems"},
		{"GET", "/api/items/count", "countItems"},
	}
	for _, tc := range want {
		op := opFor(out.Paths[tc.path], tc.method)
		if op == nil {
			t.Errorf("%s %s missing with detection only; have %v", tc.method, tc.path, mapPathKeys(out.Paths))
			continue
		}
		if !strings.Contains(op.OperationID, tc.handler) {
			t.Errorf("%s %s: operationId %q does not name %q", tc.method, tc.path, op.OperationID, tc.handler)
		}
	}

	// A responder declared on one context type but reached through the type that
	// EMBEDS it: Go promotes the method, and the call is recorded against the
	// embedding type, so a pattern scoped to the declaring type alone matches
	// nothing at the call sites. That is what left gitea with 894 routes and 8
	// components (issue #235).
	if op := opFor(out.Paths["/api-items"], "GET"); op == nil {
		t.Errorf("GET /api-items missing; have %v", mapPathKeys(out.Paths))
	} else if _, ok := op.Responses["200"]; !ok {
		t.Errorf("GET /api-items: expected the status its responder sets, have %v — the layered context was not recognised", statusKeys(op))
	}

	// The delegate must still not be documented as a route of its own.
	for path := range out.Paths {
		if path == "/{path}" || path == "/string" {
			t.Errorf("path %q is the wrapper's own parameter, not a route; have %v", path, mapPathKeys(out.Paths))
		}
	}

	// A plain function that registers on the framework router directly is NOT a
	// wrapper — there is no type to scope a pattern to, and its inner call is the
	// only registration. It must keep working, and must not gain a derived pattern
	// that would fire elsewhere.
	//
	// Its concatenated path (`base+"/{id}"`, with the caller passing "/users")
	// resolves to /users/{id}; it was documented at "/" before #274.
	if op := opFor(out.Paths["/users/{id}"], "DELETE"); op == nil {
		t.Errorf("the helper-registrar route is gone; have %v", mapPathKeys(out.Paths))
	}
}

// statusKeys lists an operation's documented statuses, for failure output.
func statusKeys(op *intspec.Operation) []string {
	out := make([]string, 0, len(op.Responses))
	for status := range op.Responses {
		out = append(out, status)
	}
	return out
}
