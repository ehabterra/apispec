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
			[]Parameter{formParam("query"), {Name: "id", In: "path"}}, false)
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
			[]Parameter{formParam("name"), formParam("email")}, false)
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
		params, body := resolveFormParams("POST", []Parameter{formParam("name")}, true)
		if body != nil {
			t.Errorf("must not clobber an existing request body; got %+v", body)
		}
		if len(params) != 1 || params[0].In != "query" {
			t.Errorf("form param should fall back to query, got %+v", params)
		}
	})

	t.Run("no form params passes through untouched", func(t *testing.T) {
		in := []Parameter{{Name: "id", In: "path"}, {Name: "q", In: "query"}}
		params, body := resolveFormParams("POST", in, false)
		if body != nil {
			t.Errorf("no form params should not synthesize a body")
		}
		if len(params) != 2 {
			t.Errorf("params changed unexpectedly: %+v", params)
		}
	})

	t.Run("required form field is marked required in body", func(t *testing.T) {
		req := formParam("token")
		req.Required = true
		_, body := resolveFormParams("PUT", []Parameter{req}, false)
		if body == nil {
			t.Fatal("expected a request body")
		}
		schema := body.Content["application/x-www-form-urlencoded"].Schema
		if len(schema.Required) != 1 || schema.Required[0] != "token" {
			t.Errorf("required = %v, want [token]", schema.Required)
		}
	})
}
