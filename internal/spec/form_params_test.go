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

package spec

import "testing"

func TestMethodTakesRequestBody(t *testing.T) {
	body := []string{"POST", "PUT", "PATCH", "post", "put", "patch"}
	for _, m := range body {
		if !methodTakesRequestBody(m) {
			t.Errorf("methodTakesRequestBody(%q) = false, want true", m)
		}
	}
	noBody := []string{"GET", "HEAD", "DELETE", "OPTIONS", "TRACE", ""}
	for _, m := range noBody {
		if methodTakesRequestBody(m) {
			t.Errorf("methodTakesRequestBody(%q) = true, want false", m)
		}
	}
}

// TestResolveFormParams pins issue #171: the invalid `in: form` sentinel must
// never survive into the output — it becomes a query param or a urlencoded body
// depending on the HTTP method.
func TestResolveFormParams(t *testing.T) {
	formParam := func(name string) Parameter {
		return Parameter{Name: name, In: "form", Schema: &Schema{Type: "string"}}
	}

	t.Run("GET form params become query", func(t *testing.T) {
		params, body := resolveFormParams("GET",
			[]Parameter{formParam("query"), {Name: "id", In: "path"}}, false, false)
		if body != nil {
			t.Fatalf("GET should not synthesize a request body, got %+v", body)
		}
		for _, p := range params {
			if p.In == "form" {
				t.Errorf("param %q still has in:form", p.Name)
			}
		}
		var q *Parameter
		for i := range params {
			if params[i].Name == "query" {
				q = &params[i]
			}
		}
		if q == nil || q.In != "query" {
			t.Errorf("query param: got %+v, want in:query", q)
		}
	})

	t.Run("POST form params become urlencoded body", func(t *testing.T) {
		params, body := resolveFormParams("POST",
			[]Parameter{formParam("name"), formParam("email")}, false, false)
		if body == nil {
			t.Fatal("POST should synthesize a request body")
		}
		media, ok := body.Content["application/x-www-form-urlencoded"]
		if !ok {
			t.Fatalf("missing urlencoded media type; have %v", body.Content)
		}
		if media.Schema.Type != "object" {
			t.Errorf("body schema type = %q, want object", media.Schema.Type)
		}
		for _, f := range []string{"name", "email"} {
			if _, ok := media.Schema.Properties[f]; !ok {
				t.Errorf("body missing property %q", f)
			}
		}
		// form params must be consumed, not left in the parameter list.
		for _, p := range params {
			if p.In == "form" {
				t.Errorf("form param %q leaked into parameters", p.Name)
			}
		}
	})

	t.Run("POST with existing body falls back to query", func(t *testing.T) {
		params, body := resolveFormParams("POST", []Parameter{formParam("name")}, true, false)
		if body != nil {
			t.Errorf("must not clobber an existing request body; got %+v", body)
		}
		if len(params) != 1 || params[0].In != "query" {
			t.Errorf("form param should fall back to query, got %+v", params)
		}
	})

	t.Run("no form params passes through untouched", func(t *testing.T) {
		in := []Parameter{{Name: "id", In: "path"}, {Name: "q", In: "query"}}
		params, body := resolveFormParams("POST", in, false, false)
		if body != nil {
			t.Errorf("no form params should not synthesize a body")
		}
		if len(params) != 2 {
			t.Errorf("params changed unexpectedly: %+v", params)
		}
	})

	fileParam := func(name string) Parameter {
		return Parameter{Name: name, In: paramInFormFile, Schema: &Schema{Type: "string", Format: "binary"}}
	}

	t.Run("multipart POST body carries file and text parts", func(t *testing.T) {
		params, body := resolveFormParams("POST",
			[]Parameter{fileParam("avatar"), formParam("title")}, false, true)
		if body == nil {
			t.Fatal("expected a multipart request body")
		}
		media, ok := body.Content[contentTypeMultipartForm]
		if !ok {
			t.Fatalf("missing multipart media type; have %v", body.Content)
		}
		if _, wrong := body.Content[contentTypeFormURLEncoded]; wrong {
			t.Error("multipart body must not also be offered as urlencoded")
		}
		if got := media.Schema.Properties["avatar"]; got == nil || got.Format != "binary" {
			t.Errorf("avatar property = %+v, want a binary string", got)
		}
		if got := media.Schema.Properties["title"]; got == nil || got.Format != "" {
			t.Errorf("title property = %+v, want a plain string", got)
		}
		for _, p := range params {
			if isFormSentinel(p.In) {
				t.Errorf("sentinel param %q leaked into parameters", p.Name)
			}
		}
	})

	t.Run("multipart marker alone still yields a multipart body", func(t *testing.T) {
		// r.ParseMultipartForm with no readable field: the shape is unknown, but
		// the media type is not — an empty object beats claiming urlencoded.
		_, body := resolveFormParams("POST", nil, false, true)
		if body == nil {
			t.Fatal("a multipart marker with no fields must still produce a body")
		}
		media, ok := body.Content[contentTypeMultipartForm]
		if !ok {
			t.Fatalf("missing multipart media type; have %v", body.Content)
		}
		if media.Schema.Type != "object" || len(media.Schema.Properties) != 0 {
			t.Errorf("schema = %+v, want a bare object", media.Schema)
		}
	})

	t.Run("file part is dropped rather than mislabelled on a non-body method", func(t *testing.T) {
		// A GET carries no body, so there is nowhere for a file part to live;
		// `in: query, format: binary` would be wrong rather than incomplete.
		params, body := resolveFormParams("GET",
			[]Parameter{fileParam("avatar"), formParam("q")}, false, true)
		if body != nil {
			t.Errorf("GET must not synthesize a request body; got %+v", body)
		}
		for _, p := range params {
			if p.Name == "avatar" {
				t.Errorf("file part survived as %+v; it has no query form", p)
			}
			if isFormSentinel(p.In) {
				t.Errorf("sentinel param %q leaked into parameters", p.Name)
			}
		}
		if len(params) != 1 || params[0].Name != "q" || params[0].In != "query" {
			t.Errorf("text form param should fall back to query, got %+v", params)
		}
	})

	t.Run("an unnamed form read cannot become a property", func(t *testing.T) {
		// The argument didn't resolve to a literal, so the wire key is unknown.
		_, body := resolveFormParams("POST",
			[]Parameter{formParam(""), formParam("real")}, false, false)
		if body == nil {
			t.Fatal("expected a request body")
		}
		schema := body.Content[contentTypeFormURLEncoded].Schema
		if _, bad := schema.Properties[""]; bad {
			t.Error(`an empty property name reached the schema`)
		}
		if _, ok := schema.Properties["real"]; !ok {
			t.Errorf("named field lost alongside the unnamed one: %v", schema.Properties)
		}
	})

	t.Run("required form field is marked required in body", func(t *testing.T) {
		req := formParam("token")
		req.Required = true
		_, body := resolveFormParams("PUT", []Parameter{req}, false, false)
		if body == nil {
			t.Fatal("expected a request body")
		}
		schema := body.Content["application/x-www-form-urlencoded"].Schema
		if len(schema.Required) != 1 || schema.Required[0] != "token" {
			t.Errorf("required = %v, want [token]", schema.Required)
		}
	})
}
