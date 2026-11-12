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
	if page := findParam(out.Paths["/items"].Get, "page"); page == nil || page.Schema == nil ||
		page.Schema.Minimum != 1 {
		t.Errorf("page lost its `binding:\"min=1\"` constraint: %+v", page)
	}

	// A query bind is parameters, never a body: ShouldBindQuery reads no
	// request body, and documenting one would tell clients to send JSON on a
	// GET (issue #355's acceptance).
	if rb := out.Paths["/items"].Get.RequestBody; rb != nil {
		t.Errorf("GET /items binds only the query, yet documents a request body: %+v", rb)
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

	// A handler reading the query both ways documents each parameter once.
	// `page` is read by `c.Query` first and then bound; the struct field's
	// type and `binding:"required"` win, because an accessor returns a string
	// whatever the value is.
	assertBoundParams(t, out, "/mixed", "GET", []boundParam{
		{name: "page", in: "query", typ: "integer", required: true},
		{name: "sort", in: "query", typ: "string"},
		{name: "X-Tenant", in: "header", typ: "string"},
	})
}

// findParam returns the operation's parameter with this name, or nil.
func findParam(op *spec.Operation, name string) *spec.Parameter {
	if op == nil {
		return nil
	}
	for i := range op.Parameters {
		if op.Parameters[i].Name == name {
			return &op.Parameters[i]
		}
	}
	return nil
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
	if rb := out.Paths["/items"].Get.RequestBody; rb != nil {
		t.Errorf("GET /items binds only the query, yet documents a request body: %+v", rb)
	}
	assertBoundParams(t, out, "/items/{id}", "GET", []boundParam{
		{name: "id", in: "path", typ: "integer", required: true},
		{name: "X-Request-ID", in: "header", typ: "string"},
	})
}

// gorilla/schema's Decode reads whatever map it is handed, so the location is
// the source's: the request's query is query parameters, its parsed form a form
// body, and the query of a URL built from configuration is no request read at
// all (issue #602).
func TestTestdata_StructBindingHTTP(t *testing.T) {
	out := loadTestdata(t, "struct_binding_http", spec.DefaultHTTPConfig())

	// `,required` and `,default:20` are gorilla/schema's own options; a
	// default also means the client may omit the parameter.
	assertBoundParams(t, out, "/items", "GET", []boundParam{
		{name: "page", in: "query", typ: "integer", required: true},
		{name: "limit", in: "query", typ: "integer"},
		{name: "sort", in: "query", typ: "string"},
		{name: "Cursor", in: "query", typ: "string"},
	})
	if limit := findParam(out.Paths["/items"].Get, "limit"); limit == nil || limit.Schema == nil ||
		limit.Schema.Default != int64(20) {
		t.Errorf("limit default = %+v, want the integer 20 — the binder converts it to the field's type", limit)
	}
	if rb := out.Paths["/items"].Get.RequestBody; rb != nil {
		t.Errorf("GET /items decodes only the query, yet documents a request body: %+v", rb)
	}

	// Through a local variable, and through a wrapper taking the source and
	// the destination as parameters.
	assertBoundParams(t, out, "/search", "GET", []boundParam{
		{name: "q", in: "query", typ: "string", required: true},
	})
	assertBoundParams(t, out, "/tagged", "GET", []boundParam{
		{name: "tag", in: "query", typ: "array"},
	})

	// The parsed form on a POST is the body, not parameters.
	assertBoundParams(t, out, "/items", "POST", nil)
	rb := out.Paths["/items"].Post.RequestBody
	if rb == nil {
		t.Fatal("POST /items decodes r.PostForm, yet documents no request body")
	}
	form, ok := rb.Content["application/x-www-form-urlencoded"]
	if !ok || form.Schema == nil {
		t.Fatalf("POST /items body is %+v, want application/x-www-form-urlencoded", rb.Content)
	}
	for _, name := range []string{"name", "email"} {
		if _, ok := form.Schema.Properties[name]; !ok {
			t.Errorf("form body has no %q property", name)
		}
	}
	// One entry per field, however many times the walk met the read.
	if got := form.Schema.Required; len(got) != 1 || got[0] != "name" {
		t.Errorf("form body required = %v, want [name]", got)
	}

	// A URL parsed from configuration has a Query() too; decoding it reads no
	// request parameter.
	assertBoundParams(t, out, "/upstream", "GET", nil)
}

// gorilla/schema is the decoder of every net/http-based router, so each of
// their configs carries it — not only the one the fixture runs under.
func TestGorillaSchemaInEveryNetHTTPConfig(t *testing.T) {
	for name, cfg := range map[string]*spec.APISpecConfig{
		"net/http": spec.DefaultHTTPConfig(),
		"chi":      spec.DefaultChiConfig(),
		"mux":      spec.DefaultMuxConfig(),
	} {
		found := false
		for _, p := range cfg.Framework.ParamPatterns {
			if p.CallRegex == "^Decode$" && p.RecvType == "github.com/gorilla/schema.*Decoder" {
				found = true
			}
		}
		if !found {
			t.Errorf("%s config has no gorilla/schema struct binding", name)
		}
	}
}
