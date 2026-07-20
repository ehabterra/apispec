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

// TestTestdata_HandlerDocComments covers issue #168 across every handler shape.
// The original implementation resolved the doc comment only through
// file.Functions, which by construction holds no methods — so a method handler
// (`h.CreateAccount`, the dominant shape in real projects) silently produced no
// summary. Metadata now records Comments on methods and the mapper resolves the
// method shape of RouteInfo.Function through the per-Type methods table.
func TestTestdata_HandlerDocComments(t *testing.T) {
	out := loadTestdataWithFixtureConfig(t, "handler_doc_comments", spec.DefaultHTTPConfig())
	noDanglingRefs(t, out)

	path := out.Paths["/accounts"]

	for _, tc := range []struct {
		method, summary, description, shape string
		path                                string // defaults to /accounts
	}{
		{
			// swaggo annotations win over the prose, and the non-@Summary /
			// @Description directives (@Tags, @Router, …) must not leak in.
			method:      "GET",
			path:        "/accounts/search",
			shape:       "swaggo annotations",
			summary:     "Search accounts",
			description: "Filters accounts by query string.\nReturns an empty list when nothing matches.",
		},
		{
			method:      "POST",
			shape:       "pointer-receiver method",
			summary:     "CreateAccount registers a new account.",
			description: "It validates the payload and returns the created account.",
		},
		{
			method:  "DELETE",
			shape:   "value-receiver method",
			summary: "DeleteAccount removes an account.",
		},
		{
			method:      "GET",
			shape:       "package-level func",
			summary:     "listAccounts returns every account.",
			description: "The remaining lines become the operation description.",
		},
		{
			method:      "HEAD",
			shape:       "method value on a struct field",
			summary:     "CreateAccount registers a new account.",
			description: "It validates the payload and returns the created account.",
		},
		{
			method: "PATCH",
			shape:  "undocumented method",
		},
		{
			method: "PUT",
			shape:  "func literal",
		},
		{
			// #204: the handler value names no method, so the framework's
			// handler interface (ServeHTTP) supplies it. Also guards that the
			// traced origin type never leaks in as a summary ("*pkg-->Handler").
			method:      "OPTIONS",
			shape:       "handler value",
			summary:     "ServeHTTP serves the account resource directly.",
			description: "A route registered with the handler *value* (mux.Handle(\"...\", h)) names no\nmethod, so the framework's handler interface supplies it (issue #204).",
		},
	} {
		p := path
		if tc.path != "" {
			p = out.Paths[tc.path]
		}
		op := opFor(p, tc.method)
		if op == nil {
			t.Errorf("%s /accounts (%s) missing; have %v", tc.method, tc.shape, mapPathKeys(out.Paths))
			continue
		}
		if op.Summary != tc.summary {
			t.Errorf("%s /accounts (%s) summary: got %q, want %q (#168)", tc.method, tc.shape, op.Summary, tc.summary)
		}
		if op.Description != tc.description {
			t.Errorf("%s /accounts (%s) description: got %q, want %q (#168)", tc.method, tc.shape, op.Description, tc.description)
		}
	}
}
