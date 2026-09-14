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

	"github.com/ehabterra/apispec/internal/spec"
)

// wantParam is one row of the accessor matrix: the parameter an accessor must
// produce, and what it must say about it.
type wantParam struct {
	name, in string
	array    bool
	def      string
}

// assertParams checks every operation's parameters against the matrix, by name.
func assertParams(t *testing.T, out *spec.OpenAPISpec, want []wantParam) {
	t.Helper()

	got := map[string]spec.Parameter{}
	for path, item := range out.Paths {
		item := item
		op := firstOperation(&item)
		if op == nil {
			t.Errorf("%s has no operation", path)
			continue
		}
		for _, p := range op.Parameters {
			if prev, dup := got[p.Name]; dup {
				// The same parameter reached through two accessors is one
				// parameter. Emitting it twice is invalid OpenAPI.
				t.Errorf("parameter %q emitted twice (in %q and %q)", p.Name, prev.In, p.In)
			}
			got[p.Name] = p
		}
	}

	for _, w := range want {
		p, ok := got[w.name]
		if !ok {
			names := make([]string, 0, len(got))
			for n := range got {
				names = append(names, n)
			}
			t.Errorf("no parameter %q — the accessor that names it produced nothing; have %v",
				w.name, names)
			continue
		}
		if p.In != w.in {
			t.Errorf("parameter %q is in %q, want %q", w.name, p.In, w.in)
		}
		if p.Schema == nil {
			t.Errorf("parameter %q has no schema", w.name)
			continue
		}
		if w.array {
			// A repeatable key is the same parameter, sent more than once, so
			// the schema is an array of what one occurrence holds.
			if p.Schema.Type != "array" || p.Schema.Items == nil {
				t.Errorf("parameter %q is %+v, want an array — the accessor returns many values",
					w.name, p.Schema)
			}
		} else if p.Schema.Type == "array" {
			t.Errorf("parameter %q came out as an array; its accessor returns one value", w.name)
		}
		if w.def != "" {
			if p.Schema.Default != w.def {
				t.Errorf("parameter %q default = %v, want %q — the handler supplies a fallback",
					w.name, p.Schema.Default, w.def)
			}
			if p.Required {
				t.Errorf("parameter %q is required despite having a default; the handler has said "+
					"what it does when the client omits it", w.name)
			}
		}
		if w.in == "path" && !p.Required {
			t.Errorf("path parameter %q is not required", w.name)
		}
	}
}

// gin's accessor list had drifted furthest: no cookie pattern, no comma-ok or
// multi-value variants, and no `Params.ByName` — whose receiver is the Params
// slice rather than the Context, so every Context-scoped pattern missed it.
// Four of ten accessors produced a parameter (issue #365).
func TestTestdata_ParamAccessorsGin(t *testing.T) {
	out := loadTestdata(t, "param_accessors_gin", spec.DefaultGinConfig())
	assertParams(t, out, []wantParam{
		{name: "id", in: "path"},
		{name: "altid", in: "path"},             // c.Params.ByName
		{name: "session", in: "cookie"},         // c.Cookie
		{name: "X-Tenant", in: "header"},        // c.GetHeader
		{name: "q", in: "query"},                // c.GetQuery — the comma-ok twin
		{name: "tag", in: "query", array: true}, // c.QueryArray
		{name: "page", in: "query", def: "1"},   // c.DefaultQuery
		// PostForm on a GET resolves to query, which is the form/query
		// disambiguation of #171 and not this issue's concern.
		{name: "field", in: "query"},
		{name: "optfield", in: "query"},
		{name: "multifield", in: "query", array: true},
	})
}

// fiber had NO header pattern at all: it spells the request-header read `Get`,
// so a required tenant or API-version header was invisible to every client
// generated from the document.
func TestTestdata_ParamAccessorsFiber(t *testing.T) {
	out := loadTestdata(t, "param_accessors_fiber", spec.DefaultFiberConfig())
	assertParams(t, out, []wantParam{
		{name: "id", in: "path"},
		{name: "X-Tenant", in: "header"}, // c.Get — the gap
		{name: "q", in: "query"},
		{name: "session", in: "cookie"},
		{name: "field", in: "query"},
	})
}

// net/http was the most complete already; what it lacked was the repeatable
// twin of Header.Get.
func TestTestdata_ParamAccessorsHTTP(t *testing.T) {
	out := loadTestdata(t, "param_accessors_http", spec.DefaultHTTPConfig())
	assertParams(t, out, []wantParam{
		{name: "id", in: "path"},
		{name: "q", in: "query"},
		{name: "X-Tenant", in: "header"},
		{name: "X-Multi", in: "header", array: true}, // r.Header.Values
		{name: "session", in: "cookie"},
		{name: "field", in: "query"},
	})
}
