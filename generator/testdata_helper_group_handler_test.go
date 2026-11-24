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
	"sort"
	"strings"
	"testing"

	intspec "github.com/ehabterra/apispec/internal/spec"
	"github.com/ehabterra/apispec/spec"
)

// TestTestdata_HelperGroupHandler locks in issue #615: a helper that receives
// both the router group and the handler documents each route once, under the
// group's prefix, with the handler its call site passed.
//
//	func reg(r *gin.RouterGroup, h gin.HandlerFunc) { r.GET("/x", h) }
//	api := r.Group("/api"); reg(api, named)
//
// It used to document two operations: `/api/x` with the parameter rendered as
// the handler (`pkg.h`, `gin.HandlerFunc`) and no body, and `/x` with the real
// handler and no prefix. The group's prefix reached the registration by
// containment under the Group call, the handler's binding only inside the
// helper's call, and no single tree position had both. A helper taking the path
// too collapsed its call sites into one `/api/{path}`.
//
// Each fixture asserts the routes once, prefixed and resolved, and the
// unprefixed copies gone. Alongside:
//   - a handler parameter bound to nothing concrete stays unresolved — no
//     handler borrowed from another call site (golden rule #7);
//   - a helper building a sub-group from its parameter, called twice on one
//     group (issue #637), resolves each call's suffix;
//   - a helper forwarding its group to another helper, fed two different groups
//     (issue #636), is a change-detector: still wrong, not made worse;
//   - net/http, where the same helper registers on the root mux and on a
//     sub-mux mounted under a prefix, and a helper reassigning its handler
//     parameter to a wrapped copy of itself still names the caller's handler.
func TestTestdata_HelperGroupHandler(t *testing.T) {
	cases := []struct {
		fixture string
		cfg     *spec.APISpecConfig
		// path → the handler the operationId must end with.
		resolved map[string]string
		// routes whose handler parameter is never bound.
		unresolved []string
		// paths that must not be documented at all.
		absent []string
		// extra paths documented as they are today (change-detectors and
		// handler-less routes), so the full path set is pinned.
		other []string
	}{
		{
			fixture: "gin_helper_group_handler",
			cfg:     spec.DefaultGinConfig(),
			resolved: map[string]string{
				"/api/namedtype": ".named",
				"/api/functype":  ".named2",
				"/api/closure":   ".FuncLit:main.go:50:13",
				"/api/a":         ".handA",
				"/api/b":         ".handB",
			},
			unresolved: []string{"/api/unbound"},
			absent: []string{
				"/namedtype", "/functype", "/closure", "/a", "/b", "/unbound",
				"/api/{path}", "/vr/{suffix}/in", "/c/in", "/d/in",
			},
			other: []string{
				"/vr/c/in", "/vr/d/in", // issue #637: one frame per call site
				// Change-detector for issue #636, NOT the desired answer:
				// the forwarded parameter binds to the first caller only.
				// Flips to /ra/one and /rb/two when that is fixed.
				"/one", "/ra/{p}",
			},
		},
		{
			fixture: "echo_helper_group_handler",
			cfg:     spec.DefaultEchoConfig(),
			resolved: map[string]string{
				"/api/namedtype": ".named",
				"/api/functype":  ".named2",
				"/api/closure":   ".FuncLit:main.go:30:13",
				"/api/a":         ".handA",
				"/api/b":         ".handB",
			},
			unresolved: []string{"/api/unbound"},
			absent: []string{
				"/namedtype", "/functype", "/closure", "/a", "/b", "/unbound", "/api/{path}",
			},
		},
		{
			fixture: "http_helper_group_handler",
			cfg:     spec.DefaultHTTPConfig(),
			resolved: map[string]string{
				"/named": ".named",
				// `h = withLogging(h)` in the helper: the parameter hop still
				// names the caller's handler — only a closure the name
				// provably holds outranks it.
				"/logged": ".logged",
				"/api/a":  ".handA",
				"/api/b":  ".handB",
			},
			absent: []string{"/a", "/b", "/api/{pattern}"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.fixture, func(t *testing.T) {
			out := loadTestdataWithFixtureConfig(t, tc.fixture, tc.cfg)
			noDanglingRefs(t, out)
			noNullSchemas(t, out)

			for path, handler := range tc.resolved {
				item, ok := out.Paths[path]
				if !ok {
					t.Errorf("path %q missing; have %v", path, mapPathKeys(out.Paths))
					continue
				}
				op := opFor(item, "GET")
				if op == nil {
					t.Errorf("GET %s: operation missing", path)
					continue
				}
				if !strings.HasSuffix(op.OperationID, handler) {
					t.Errorf("GET %s: operationId %q, want the handler the call site passed (%s)", path, op.OperationID, handler)
				}
				if paramNamed(op, "page", "query") == nil {
					t.Errorf("GET %s: query parameter page missing — the handler body did not reach the route", path)
				}
				if !strings.HasSuffix(schemaRefAt(op, "200"), "_Place") {
					t.Errorf("GET %s: 200 body is %q, want Place", path, schemaRefAt(op, "200"))
				}
			}

			for _, path := range tc.unresolved {
				item, ok := out.Paths[path]
				if !ok {
					t.Errorf("path %q missing; have %v", path, mapPathKeys(out.Paths))
					continue
				}
				op := opFor(item, "GET")
				if op == nil {
					t.Errorf("GET %s: operation missing", path)
					continue
				}
				for _, h := range tc.resolved {
					if strings.HasSuffix(op.OperationID, h) {
						t.Errorf("GET %s: attributed to %q, a handler bound at another call site", path, op.OperationID)
					}
				}
				if _, ok := op.Responses["200"]; ok {
					t.Errorf("GET %s: documents a 200 body for a handler nothing binds", path)
				}
				// The parameter renders as its type (`gin.HandlerFunc`), which
				// names no operation: the id is the method-and-path one.
				if want := methodPathOperationID("get", path); op.OperationID != want {
					t.Errorf("GET %s: operationId %q, want %q — an unbound handler parameter names no operation", path, op.OperationID, want)
				}
			}

			for _, path := range tc.absent {
				if _, ok := out.Paths[path]; ok {
					t.Errorf("path %q documented; want it absent (the route belongs under its group, once)", path)
				}
			}

			// The whole path set, so a duplicate under any other name fails.
			want := append(append(append([]string{}, tc.unresolved...), tc.other...), mapKeys(tc.resolved)...)
			sort.Strings(want)
			got := mapPathKeys(out.Paths)
			sort.Strings(got)
			if strings.Join(got, " ") != strings.Join(want, " ") {
				t.Errorf("paths = %v, want %v", got, want)
			}
		})
	}
}

// methodPathOperationID is the id an operation falls back to for these
// fixtures' plain paths: the method, then each segment capitalised.
func methodPathOperationID(method, path string) string {
	id := method
	for _, seg := range strings.Split(path, "/") {
		if seg != "" {
			id += strings.ToUpper(seg[:1]) + seg[1:]
		}
	}
	return id
}

func mapKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// TestTestdata_HelperFrameResponse guards the other side of issue #615: a
// helper's binding argument hangs under the PRODUCER of the value it passes, so
// it is reached wherever that producer is — including routes that never make
// the helper call. Read as the call it belongs to, `c.Fail(err)` documented
// Fail's error body on every route that reached the call producing `err`: a
// JSON alternative beside a plain-text 200 (gitea gained it on 193 operations).
// An argument is not a call, and only calls are read for responses.
func TestTestdata_HelperFrameResponse(t *testing.T) {
	out := loadTestdataWithFixtureConfig(t, "http_helper_frame_response", spec.DefaultHTTPConfig())
	noDanglingRefs(t, out)
	noNullSchemas(t, out)

	list := opFor(out.Paths["/items"], "GET")
	if list == nil {
		t.Fatalf("GET /items missing; have %v", mapPathKeys(out.Paths))
	}
	if got := strings.Join(contentTypesOf(list, "200"), " "); got != "text/plain; charset=utf-8" {
		t.Errorf("GET /items 200 media types = %q, want only text/plain — Fail is never called on this route", got)
	}
	for status := range list.Responses {
		if status != "200" {
			t.Errorf("GET /items documents %s; its only response is the 200 it writes", status)
		}
	}

	// The route that does call Fail keeps every response Fail sends.
	get := opFor(out.Paths["/items/{id}"], "GET")
	if get == nil {
		t.Fatalf("GET /items/{id} missing; have %v", mapPathKeys(out.Paths))
	}
	for status, want := range map[string]string{"200": "_Item", "404": "_ErrBody", "500": "_ErrBody"} {
		if got := schemaRefAt(get, status); !strings.HasSuffix(got, want) {
			t.Errorf("GET /items/{id} %s body = %q, want %s", status, got, want)
		}
	}
	if got := strings.Join(contentTypesOf(get, "200"), " "); got != "application/json" {
		t.Errorf("GET /items/{id} 200 media types = %q, want application/json alone", got)
	}
}

// TestTestdata_HelperFrameParam guards the parameter side of the same frame
// (issue #615): a configured accessor whose binding argument hangs under a
// shared producer must not document its parameter on a route that only
// reaches the producer. The walk reads that frame node twice — as a child of
// the producer, and again in its own recursion's tail — and both must hold the
// gate. The default config is the negative case: readFlag is then no accessor
// and nothing documents `verbose` at all. GET /self is the other side of the
// gate: a header read nested as another call's argument is its own statement.
func TestTestdata_HelperFrameParam(t *testing.T) {
	accessor := intspec.DefaultHTTPConfig()
	accessor.Framework.ParamPatterns = append(accessor.Framework.ParamPatterns, intspec.ParamPattern{
		CallRegex:     "^readFlag$",
		ParamIn:       "query",
		ParamArgIndex: 1,
	})
	for _, tc := range []struct {
		name    string
		cfg     *intspec.APISpecConfig
		getWant string
	}{
		{"accessor", accessor, "path:id query:verbose"},
		{"default", intspec.DefaultHTTPConfig(), "path:id"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := loadTestdataWithFixtureConfig(t, "http_helper_frame_param", tc.cfg)
			noDanglingRefs(t, out)

			list := opFor(out.Paths["/items"], "GET")
			if list == nil {
				t.Fatalf("GET /items missing; have %v", mapPathKeys(out.Paths))
			}
			if got := paramKeys(list); len(got) != 0 {
				t.Errorf("GET /items parameters = %v, want none — readFlag is never called on this route", got)
			}

			get := opFor(out.Paths["/items/{id}"], "GET")
			if get == nil {
				t.Fatalf("GET /items/{id} missing; have %v", mapPathKeys(out.Paths))
			}
			if got := strings.Join(paramKeys(get), " "); got != tc.getWant {
				t.Errorf("GET /items/{id} parameters = %q, want %q", got, tc.getWant)
			}

			// A header read nested as another call's argument is a statement
			// of the route that makes the call. Here a direct copy reaches it
			// too; TestReadsAsStatement pins the gate for the argument copy,
			// which on a large service is often the only one.
			self := opFor(out.Paths["/self"], "GET")
			if self == nil {
				t.Fatalf("GET /self missing; have %v", mapPathKeys(out.Paths))
			}
			if got := strings.Join(paramKeys(self), " "); got != "header:X-Forwarded-Proto" {
				t.Errorf("GET /self parameters = %q, want header:X-Forwarded-Proto", got)
			}
		})
	}
}

// paramKeys lists an operation's parameters as sorted "in:name" keys.
func paramKeys(op *intspec.Operation) []string {
	var keys []string
	for _, p := range op.Parameters {
		keys = append(keys, p.In+":"+p.Name)
	}
	sort.Strings(keys)
	return keys
}
