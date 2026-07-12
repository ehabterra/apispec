package generator

import (
	"testing"

	"github.com/ehabterra/apispec/spec"
)

// TestTestdata_MethodSwitch locks in control-flow method dispatch: a single
// net/http handler registered without a verb that branches on r.Method (via a
// switch or an if/else-if chain) must split into one operation per HTTP method,
// with each branch's request/response attributed to its own method and unique
// operationIds across the split.
func TestTestdata_MethodSwitch(t *testing.T) {
	out := loadTestdataWithFixtureConfig(t, "method_switch", spec.DefaultHTTPConfig())
	noDanglingRefs(t, out)

	// Expected method set per path (the core win: no more single default POST).
	want := map[string][]string{
		"/users": {"GET", "POST"},
		"/item":  {"GET", "DELETE"},
		"/ping":  {"GET", "HEAD"},
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
		// The default POST must be gone where the handler doesn't serve POST.
		if path == "/item" && opFor(item, "POST") != nil {
			t.Errorf("%s should not carry a POST operation", path)
		}
		if path == "/ping" && opFor(item, "POST") != nil {
			t.Errorf("%s should not carry a POST operation", path)
		}
	}

	// Per-branch attribution: POST /users carries the request body; GET does not.
	users := out.Paths["/users"]
	if post := opFor(users, "POST"); post == nil || post.RequestBody == nil {
		t.Errorf("POST /users should carry a request body (CreateUserRequest)")
	}
	if get := opFor(users, "GET"); get == nil || get.RequestBody != nil {
		t.Errorf("GET /users should not carry a request body, got %+v", get)
	}

	// DELETE /item is bodyless (204): it must not carry a GET-style user body.
	item := out.Paths["/item"]
	if del := opFor(item, "DELETE"); del != nil {
		for status := range del.Responses {
			if status == "200" {
				t.Errorf("DELETE /item should not carry a 200 body response")
			}
		}
	}

	// operationIds are unique across the split (method-suffixed).
	seen := map[string]bool{}
	for _, path := range []string{"/users", "/item", "/ping"} {
		for _, m := range []string{"GET", "POST", "PUT", "DELETE", "HEAD"} {
			if op := opFor(out.Paths[path], m); op != nil {
				id := op.OperationID
				if id == "" {
					t.Errorf("%s %s: empty operationId", m, path)
				}
				if seen[id] {
					t.Errorf("duplicate operationId %q across the method split", id)
				}
				seen[id] = true
			}
		}
	}
}
