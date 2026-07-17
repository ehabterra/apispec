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

	intspec "github.com/ehabterra/apispec/internal/spec"
	"github.com/ehabterra/apispec/spec"
)

// TestTestdata_FormValueParams covers issue #171: r.FormValue reads must not be
// emitted with the invalid OpenAPI parameter location `in: form`. GET form
// values resolve to `in: query`; POST form values resolve to an
// application/x-www-form-urlencoded request body. No parameter anywhere may
// carry `in: form`.
func TestTestdata_FormValueParams(t *testing.T) {
	out := loadTestdataWithFixtureConfig(t, "form_value_params", spec.DefaultHTTPConfig())
	noDanglingRefs(t, out)
	noUnresolvedPlaceholders(t, out)

	// No operation may emit the invalid `in: form` location.
	for path, item := range out.Paths {
		for _, op := range []*intspec.Operation{item.Get, item.Post, item.Put, item.Delete, item.Patch} {
			if op == nil {
				continue
			}
			for _, p := range op.Parameters {
				if p.In == "form" {
					t.Errorf("%s: parameter %q emitted with invalid location in:form", path, p.Name)
				}
			}
		}
	}

	// GET /search: form value resolves to an in:query parameter.
	search := opFor(out.Paths["/search"], "GET")
	if search == nil {
		t.Fatalf("GET /search missing; have %v", mapPathKeys(out.Paths))
	}
	var queryParam *intspec.Parameter
	for i := range search.Parameters {
		if search.Parameters[i].Name == "query" {
			queryParam = &search.Parameters[i]
		}
	}
	if queryParam == nil {
		t.Fatalf("GET /search missing 'query' parameter; have %+v", search.Parameters)
	}
	if queryParam.In != "query" {
		t.Errorf("GET /search 'query' param: in=%q, want query", queryParam.In)
	}

	// POST /submit: form values resolve to a urlencoded request body.
	submit := opFor(out.Paths["/submit"], "POST")
	if submit == nil {
		t.Fatalf("POST /submit missing; have %v", mapPathKeys(out.Paths))
	}
	if submit.RequestBody == nil {
		t.Fatal("POST /submit: expected a form-urlencoded request body, got none")
	}
	media, ok := submit.RequestBody.Content["application/x-www-form-urlencoded"]
	if !ok {
		t.Fatalf("POST /submit: missing application/x-www-form-urlencoded body; have %v",
			keysOf(submit.RequestBody.Content))
	}
	if media.Schema == nil || media.Schema.Type != "object" {
		t.Fatalf("POST /submit: form body schema should be an object, got %+v", media.Schema)
	}
	for _, field := range []string{"name", "email"} {
		if _, ok := media.Schema.Properties[field]; !ok {
			t.Errorf("POST /submit: form body missing property %q; have %v",
				field, keysOf(media.Schema.Properties))
		}
	}
	// The form fields must not also leak out as `in: form` parameters.
	for _, p := range submit.Parameters {
		if p.Name == "name" || p.Name == "email" {
			t.Errorf("POST /submit: form field %q leaked as a parameter (in=%q)", p.Name, p.In)
		}
	}
}
