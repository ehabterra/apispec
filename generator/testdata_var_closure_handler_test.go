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
	"github.com/ehabterra/apispec/spec"
)

// TestTestdata_VarClosureHandler locks in issue #610: a handler closure held in
// a local variable documents the same operation as the literal written inline.
//
//	handler := func(c *gin.Context) { … }
//	router.GET("/geo", handler)
//
// The route used to be found with nothing under it. The ident's key has no
// calls — the literal's does — so the tracker walked nothing, and the handler
// argument rendered as the variable's name or its declared type
// (`net/http.HandlerFunc`) rather than resolving to the literal.
//
// Three neighbours are asserted with it:
//   - the inline literal, the control the fix has to match;
//   - a closure that reaches the registration through a helper's parameter,
//     where the parameter hop and the closure hop have to chain;
//   - a variable assigned two different closures, which must stay unresolved:
//     which one serves the route is runtime state (golden rule #7);
//   - a handler factory whose returned closure reads the factory's parameters,
//     which the binding guard below must keep resolving;
//   - a closure in an inner block shadowing a package-level function or a
//     parameter, in a function and in a method: the call outside the block
//     must keep naming the outer binding;
//   - a conversion at the registration, `http.HandlerFunc(c)`.
//
// The helper case also guards the binding leak the fix exposed: routes
// registered in main hang under the helper's call (the router's producer), and
// main's own `h` was read through that call's parameter `h` by name — so /geo
// was attributed to the closure registered at /wrapped.
func TestTestdata_VarClosureHandler(t *testing.T) {
	// A factory route's schema comes from the argument the factory was given.
	type factoryWant struct {
		method, status, response, request string
	}
	type want struct {
		query    string // the query parameter the handler reads, if any
		literal  string // "main.go:<line>" of the closure the route must name, if unique
		response string // the 200 body's type, when which handler serves it is the point
	}
	cases := []struct {
		fixture    string
		cfg        *spec.APISpecConfig
		resolved   map[string]want
		factory    map[string]factoryWant
		unresolved []string
	}{
		{
			fixture: "var_closure_handler_gin",
			cfg:     spec.DefaultGinConfig(),
			resolved: map[string]want{
				// One closure at two routes: its id is shared, so the
				// operationId falls back to method+path (#459) — no literal.
				"/api/geo":          {query: "count"},
				"/api/geo/{format}": {query: "count"},
				"/api/once":         {query: "q", literal: "main.go:41:"},
				"/api/inline":       {query: "count", literal: "main.go:50:"},
			},
			factory: map[string]factoryWant{
				"/api/factory/geo":   {method: "GET", status: "200", response: "Geo"},
				"/api/factory/place": {method: "POST", request: "NewPlace"},
			},
			unresolved: []string{"/api/ambiguous"},
		},
		{
			fixture: "var_closure_handler_http",
			cfg:     spec.DefaultHTTPConfig(),
			resolved: map[string]want{
				"/geo": {query: "count", literal: "main.go:89:"},
				// var health http.HandlerFunc = func… — also bound to
				// shadowParam's `p`, so it serves /shadowp/outer too and its
				// operationId falls back to method+path (#459): no literal.
				"/health":    {response: "Status"},
				"/wrapped":   {query: "page", literal: "main.go:100:"},
				"/converted": {query: "q", literal: "main.go:107:"}, // http.HandlerFunc(c)
				"/inline":    {query: "count", literal: "main.go:121:"},
				// An inner block's closure resolves for the call inside it…
				"/shadow/inner":  {literal: "main.go:35:", response: "Inner"},
				"/shadowp/inner": {literal: "main.go:44:", response: "Inner"},
				// …and NOT for the call outside it, which names the
				// package-level function the local shadows, or the parameter:
				// `p` is bound to `health` at shadowParam(mux, health) — the
				// registration sits in that call's frame (issue #615).
				"/shadow/outer":  {response: "Pkg"},
				"/shadowp/outer": {response: "Status"},
				// A closure local in a METHOD resolves like one in a function:
				// methods record their control-flow regions (issue #617).
				"/method": {literal: "main.go:56:", response: "Geo"},
				// The shadowing shape in a METHOD: the outer call names the
				// parameter, bound here to pkgHandler — never the inner
				// block's closure.
				"/mshadow/outer": {response: "Pkg"},
				// The inner call names the inner block's closure: a closure
				// the name provably holds at the call outranks the parameter
				// hop, in the tracker and in the handler's identity alike
				// (issue #615).
				"/mshadow/inner": {literal: "main.go:64:", response: "Inner"},
				// A package-level function shadowed in a METHOD: only the
				// method's recorded regions tell the two `pkgHandler`s apart.
				"/mshadowpkg/inner": {literal: "main.go:74:", response: "Inner"},
				"/mshadowpkg/outer": {response: "Pkg"},
				// A closure declared in an if/switch/type-switch/for
				// initializer (issue #618) is the handler inside the
				// statement — else arm included — and NOT after it, where
				// the name is the package-level function again.
				"/ifinit/inner":  {response: "Inner"}, // one closure at two routes: no literal in the id
				"/ifinit/else":   {response: "Inner"},
				"/ifinit/outer":  {response: "Pkg"},
				"/swinit/inner":  {literal: "main.go:150:", response: "Inner"},
				"/swinit/outer":  {response: "Pkg"},
				"/tsinit/inner":  {literal: "main.go:159:", response: "Inner"},
				"/tsinit/outer":  {response: "Pkg"},
				"/forinit/inner": {literal: "main.go:167:", response: "Inner"},
				"/forinit/outer": {response: "Pkg"},
				"/minit/inner":   {literal: "main.go:176:", response: "Inner"},
				"/minit/outer":   {response: "Pkg"},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.fixture, func(t *testing.T) {
			out := loadTestdataWithFixtureConfig(t, tc.fixture, tc.cfg)
			noDanglingRefs(t, out)
			noNullSchemas(t, out)

			// The package-level function, not a literal, names the outer routes.
			for _, path := range []string{"/ifinit/outer", "/swinit/outer", "/tsinit/outer", "/forinit/outer", "/minit/outer"} {
				if item, ok := out.Paths[path]; tc.fixture == "var_closure_handler_http" && ok {
					if op := opFor(item, "GET"); op != nil && strings.Contains(op.OperationID, "FuncLit") {
						t.Errorf("GET %s: operationId %q names an init-scoped closure", path, op.OperationID)
					}
				}
			}

			for path, w := range tc.resolved {
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
				if _, ok := op.Responses["200"]; !ok {
					t.Errorf("GET %s: no 200 response — the closure body did not reach the route; responses=%v", path, op.Responses)
				}
				if w.query != "" && paramNamed(op, w.query, "query") == nil {
					t.Errorf("GET %s: query parameter %q missing", path, w.query)
				}
				if w.response != "" && !strings.HasSuffix(schemaRefAt(op, "200"), "_"+w.response) {
					t.Errorf("GET %s: 200 body is %q, want %s — attributed to the wrong handler", path, schemaRefAt(op, "200"), w.response)
				}
				if w.literal != "" && !strings.Contains(op.OperationID, "FuncLit:"+w.literal) {
					t.Errorf("GET %s: operationId %q does not name the closure at %s", path, op.OperationID, w.literal)
				}
			}

			for path, f := range tc.factory {
				item, ok := out.Paths[path]
				if !ok {
					t.Errorf("path %q missing; have %v", path, mapPathKeys(out.Paths))
					continue
				}
				op := opFor(item, f.method)
				if op == nil {
					t.Errorf("%s %s: operation missing", f.method, path)
					continue
				}
				if f.response != "" && !strings.HasSuffix(schemaRefAt(op, f.status), "_"+f.response) {
					t.Errorf("%s %s: %s response is %q, want the factory argument %s", f.method, path, f.status, schemaRefAt(op, f.status), f.response)
				}
				if f.request != "" && !strings.HasSuffix(requestRefOf(op), "_"+f.request) {
					t.Errorf("%s %s: request body is %q, want the factory argument %s", f.method, path, requestRefOf(op), f.request)
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
				if strings.Contains(op.OperationID, "FuncLit:") {
					t.Errorf("GET %s: attributed to %q, but nothing proves the variable holds that closure here", path, op.OperationID)
				}
				if _, ok := op.Responses["200"]; ok {
					t.Errorf("GET %s: documents a 200 body from a closure that may not be the handler", path)
				}
			}
		})
	}
}

// schemaRefAt returns the $ref of an operation's JSON response at a status, or
// "" when there is none.
func schemaRefAt(op *intspec.Operation, status string) string {
	resp, ok := op.Responses[status]
	if !ok {
		return ""
	}
	if mt, ok := resp.Content["application/json"]; ok && mt.Schema != nil {
		return mt.Schema.Ref
	}
	return ""
}

// requestRefOf returns the $ref of an operation's JSON request body, or "".
func requestRefOf(op *intspec.Operation) string {
	if op.RequestBody == nil {
		return ""
	}
	if mt, ok := op.RequestBody.Content["application/json"]; ok && mt.Schema != nil {
		return mt.Schema.Ref
	}
	return ""
}
