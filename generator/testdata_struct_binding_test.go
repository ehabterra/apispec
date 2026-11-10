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

// boundParam is what one struct field must document: its location, type and
// whether the binder's validation makes it required.
type boundParam struct {
	name, in, typ string
	required      bool
}

// assertBoundParams checks one operation's parameters: every wanted one is
// present as described, and nothing else is — a struct binding that leaks a
// field the binder never reads documents a parameter no request can satisfy.
func assertBoundParams(t *testing.T, out *spec.OpenAPISpec, path, method string, want []boundParam) {
	t.Helper()
	item, ok := out.Paths[path]
	if !ok {
		t.Fatalf("no path %s", path)
	}
	op := operationFor(&item, method)
	if op == nil {
		t.Fatalf("%s has no %s operation", path, method)
	}

	got := map[string]spec.Parameter{}
	for _, p := range op.Parameters {
		if _, dup := got[p.Name]; dup {
			t.Errorf("%s %s: parameter %q emitted twice", method, path, p.Name)
		}
		got[p.Name] = p
	}
	for _, w := range want {
		p, ok := got[w.name]
		if !ok {
			t.Errorf("%s %s: no parameter %q — the bound struct names it", method, path, w.name)
			continue
		}
		delete(got, w.name)
		if p.In != w.in {
			t.Errorf("%s %s: %q is in %q, want %q", method, path, w.name, p.In, w.in)
		}
		if p.Schema == nil || p.Schema.Type != w.typ {
			t.Errorf("%s %s: %q schema = %+v, want type %q", method, path, w.name, p.Schema, w.typ)
		}
		if p.Required != w.required {
			t.Errorf("%s %s: %q required = %v, want %v", method, path, w.name, p.Required, w.required)
		}
		if _, warned := p.Extensions["x-warning"]; warned {
			t.Errorf("%s %s: %q carries %v, but the handler binds it", method, path, w.name, p.Extensions["x-warning"])
		}
	}
	for name, p := range got {
		t.Errorf("%s %s: unexpected parameter %q (in %q)", method, path, name, p.In)
	}
}

// operationFor returns the operation for an HTTP method on a path item.
func operationFor(item *spec.PathItem, method string) *spec.Operation {
	switch method {
	case "GET":
		return item.Get
	case "POST":
		return item.Post
	case "PUT":
		return item.Put
	case "DELETE":
		return item.Delete
	case "PATCH":
		return item.Patch
	}
	return nil
}

// gin's ShouldBind* family fills a struct whose `form`, `uri` and `header`
// tags name the parameters. None of them produced anything: no parameter, and
// the path variable carried a "not found in the code" warning (issue #355).
func TestTestdata_StructBindingGin(t *testing.T) {
	out := loadTestdata(t, "struct_binding_gin", spec.DefaultGinConfig())

	assertBoundParams(t, out, "/items", "GET", []boundParam{
		// Promoted from the untagged Pagination embed; `binding:"required"`
		// is gin's validator key.
		{name: "page", in: "query", typ: "integer", required: true},
		{name: "limit", in: "query", typ: "integer"},
		{name: "sort", in: "query", typ: "string"},
		{name: "tag", in: "query", typ: "array"},
		// A pointer field is optional; the parameter is the value.
		{name: "since", in: "query", typ: "integer"},
		// gin binds an untagged field under its Go name.
		{name: "Cursor", in: "query", typ: "string"},
		// Absent on purpose: `form:"-"`, the unexported field, and the
		// struct-typed Filter field.
	})
	if p := out.Paths["/items"].Get.Parameters; len(p) > 0 && p[0].Schema.Minimum != 1 {
		t.Errorf("page lost its `binding:\"min=1\"` constraint: %+v", p[0].Schema)
	}

	// `uri:"extra"` names no segment of /items/:id, so it is not a parameter.
	assertBoundParams(t, out, "/items/{id}", "GET", []boundParam{
		{name: "id", in: "path", typ: "string", required: true},
		{name: "X-Request-ID", in: "header", typ: "string", required: true},
	})

	// Bound through a house wrapper taking `any`: the type is the caller's.
	assertBoundParams(t, out, "/search", "GET", []boundParam{
		{name: "q", in: "query", typ: "string"},
	})

	// Bound into an `any`: there is no struct to read, so nothing is guessed.
	assertBoundParams(t, out, "/untyped", "GET", nil)
}

// echo's Bind fills `param` tags always and `query` tags only for GET, DELETE
// and HEAD, and leaves an untagged field alone. Its DefaultBinder's
// single-source methods take the Context first.
func TestTestdata_StructBindingEcho(t *testing.T) {
	out := loadTestdata(t, "struct_binding_echo", spec.DefaultEchoConfig())

	assertBoundParams(t, out, "/items/{id}", "GET", []boundParam{
		{name: "id", in: "path", typ: "string", required: true},
		{name: "fields", in: "query", typ: "string", required: true},
	})
	// A POST: echo's Bind reads no query string, so `dry_run` is not one.
	assertBoundParams(t, out, "/items/{id}", "POST", []boundParam{
		{name: "id", in: "path", typ: "string", required: true},
	})
	assertBoundParams(t, out, "/items", "GET", []boundParam{
		{name: "page", in: "query", typ: "integer"},
		{name: "X-Request-ID", in: "header", typ: "string"},
	})
}

// fiber's QueryParser, ParamsParser and ReqHeaderParser each bind one source,
// and read an untagged field under its Go name.
func TestTestdata_StructBindingFiber(t *testing.T) {
	out := loadTestdata(t, "struct_binding_fiber", spec.DefaultFiberConfig())

	assertBoundParams(t, out, "/items", "GET", []boundParam{
		{name: "page", in: "query", typ: "integer"},
		{name: "tag", in: "query", typ: "array"},
		{name: "Cursor", in: "query", typ: "string"},
	})
	assertBoundParams(t, out, "/items/{id}", "GET", []boundParam{
		{name: "id", in: "path", typ: "integer", required: true},
		{name: "X-Request-ID", in: "header", typ: "string"},
	})
}
