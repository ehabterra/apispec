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

import (
	"testing"
)

func TestMethodPathID(t *testing.T) {
	for _, tc := range []struct {
		method, path, want string
	}{
		{"GET", "/users", "getUsers"},
		{"GET", "/users/{id}", "getUsersById"},
		{"GET", "/users/{id}/items", "getUsersByIdItems"},
		{"POST", "/", "post"},
		{"DELETE", "/repos/{owner}/{repo}/issues/{index}", "deleteReposByOwnerByRepoIssuesByIndex"},
		// Word boundaries at non-alphanumerics, interior case preserved: a
		// camelCase segment survives, unlike security_lookup's camelSegment.
		{"GET", "/api/v1/estimateLine", "getApiV1EstimateLine"},
		{"GET", "/-/admin/user-settings", "getAdminUserSettings"},
	} {
		if got := methodPathID(tc.method, tc.path); got != tc.want {
			t.Errorf("methodPathID(%q, %q) = %q, want %q", tc.method, tc.path, got, tc.want)
		}
	}
}

func TestReceiverMethodID(t *testing.T) {
	for _, tc := range []struct{ full, want string }{
		{"github.com/acme/api/internal/httpapi.estimateHandler.updateLine", "estimateHandler.updateLine"},
		{"github.com/acme/api/internal/httpapi.ListUsers", "ListUsers"},
		{"main.handler", "handler"},
		// A closure's tail is a source position. Dropping the module path is
		// still an improvement; method-path suits such a codebase better.
		{"github.com/acme/api/internal/http.FuncLit:router.go:998:21", "FuncLit:router.go:998:21"},
		// A generic handler is qualified inside its type argument too, so the
		// base and the argument are unqualified separately. Trimming the whole
		// string at its last "/" cut inside the brackets and left
		// "InstallForm]" in a real spec.
		{"gitea.dev/modules/web.Bind[gitea.dev/services/forms.InstallForm]", "Bind[InstallForm]"},
		{"acme.dev/web.Bind[acme.dev/forms.A, acme.dev/forms.B]", "Bind[A, B]"},
		{"", ""},
	} {
		if got := receiverMethodID(tc.full); got != tc.want {
			t.Errorf("receiverMethodID(%q) = %q, want %q", tc.full, got, tc.want)
		}
	}
}

// A colliding bare name qualifies the whole GROUP: letting one win would make
// the winner depend on nothing a reader can see (issue #298).
func TestShortSchemaNamesQualifiesCollisionsAsAGroup(t *testing.T) {
	usedTypes := map[string]*Schema{
		"github.com/acme/svc/internal/billing.Components":  {},
		"github.com/acme/svc/internal/estimate.Components": {},
		"github.com/acme/svc/internal/estimate.LineInput":  {},
	}
	components := &Components{Schemas: map[string]*Schema{}}
	for name := range usedTypes {
		components.Schemas[schemaComponentNameReplacer.Replace(name)] = &Schema{}
	}

	renames := shortSchemaNames(components, usedTypes)

	want := map[string]string{
		schemaComponentNameReplacer.Replace("github.com/acme/svc/internal/billing.Components"):  "billing_Components",
		schemaComponentNameReplacer.Replace("github.com/acme/svc/internal/estimate.Components"): "estimate_Components",
		schemaComponentNameReplacer.Replace("github.com/acme/svc/internal/estimate.LineInput"):  "LineInput",
	}
	for key, expected := range want {
		if got := renames[key]; got != expected {
			t.Errorf("rename[%q] = %q, want %q", key, got, expected)
		}
	}
	if len(renames) != len(want) {
		t.Errorf("got %d renames, want %d: %v", len(renames), len(want), renames)
	}
}

// Same bare name AND same last package segment: qualification extends one
// segment at a time until the group is distinct.
func TestShortSchemaNamesExtendsUntilDistinct(t *testing.T) {
	usedTypes := map[string]*Schema{
		"github.com/acme/svc/alpha/config.Row": {},
		"github.com/acme/svc/zeta/config.Row":  {},
	}
	components := &Components{Schemas: map[string]*Schema{}}
	for name := range usedTypes {
		components.Schemas[schemaComponentNameReplacer.Replace(name)] = &Schema{}
	}

	renames := shortSchemaNames(components, usedTypes)
	want := map[string]string{
		schemaComponentNameReplacer.Replace("github.com/acme/svc/alpha/config.Row"): "alpha_config_Row",
		schemaComponentNameReplacer.Replace("github.com/acme/svc/zeta/config.Row"):  "zeta_config_Row",
	}
	for key, expected := range want {
		if got := renames[key]; got != expected {
			t.Errorf("rename[%q] = %q, want %q", key, got, expected)
		}
	}
}

// A wrapper-typed key (*T) is left alone: shortening it would render "*T",
// sanitize to a leading underscore and collide with T's own component.
func TestShortSchemaNamesSkipsWrapperKeys(t *testing.T) {
	usedTypes := map[string]*Schema{
		"*github.com/acme/svc/internal/api.Reference": {},
		"github.com/acme/svc/internal/api.Reference":  {},
	}
	components := &Components{Schemas: map[string]*Schema{}}
	for name := range usedTypes {
		components.Schemas[schemaComponentNameReplacer.Replace(name)] = &Schema{}
	}

	renames := shortSchemaNames(components, usedTypes)
	if got, ok := renames[schemaComponentNameReplacer.Replace("*github.com/acme/svc/internal/api.Reference")]; ok {
		t.Errorf("pointer-typed key was renamed to %q; it should be left as it is", got)
	}
	if got := renames[schemaComponentNameReplacer.Replace("github.com/acme/svc/internal/api.Reference")]; got != "Reference" {
		t.Errorf("named key = %q, want %q", got, "Reference")
	}
}

// The rename must reach every $ref site, or the spec ships a dangling
// reference. This drives the real traversal rather than a hand-rolled one.
func TestApplySchemaRenamesRewritesEveryRefSite(t *testing.T) {
	ref := func(name string) *Schema { return &Schema{Ref: refComponentsSchemasPrefix + name} }
	spec := &OpenAPISpec{
		Paths: map[string]PathItem{
			"/x": {
				Get: &Operation{
					Parameters: []Parameter{{Name: "p", In: "query", Schema: ref("old")}},
					RequestBody: &RequestBody{Content: map[string]MediaType{
						"application/json": {Schema: &Schema{Properties: map[string]*Schema{"nested": ref("old")}}},
					}},
					Responses: map[string]Response{
						"200": {
							Content: map[string]MediaType{"application/json": {Schema: &Schema{Items: ref("old")}}},
							Headers: map[string]Header{"X-Thing": {Schema: ref("old")}},
						},
					},
				},
			},
		},
		Components: &Components{
			Schemas: map[string]*Schema{
				"old":     {Type: "object"},
				"wrapper": {AllOf: []*Schema{ref("old")}, AdditionalProperties: ref("old")},
			},
			Parameters:    map[string]*Parameter{"P": {Schema: ref("old")}},
			RequestBodies: map[string]*RequestBody{"B": {Content: map[string]MediaType{"application/json": {Schema: ref("old")}}}},
			Responses:     map[string]*Response{"R": {Content: map[string]MediaType{"application/json": {Schema: ref("old")}}}},
		},
	}

	applySchemaRenames(spec, map[string]string{"old": "New"})

	if _, ok := spec.Components.Schemas["New"]; !ok {
		t.Fatalf("component was not renamed: %v", spec.Components.Schemas)
	}
	if _, ok := spec.Components.Schemas["old"]; ok {
		t.Error("the old component key is still present")
	}
	var stale []string
	forEachSchemaRef(spec, func(name string) {
		if name == "old" {
			stale = append(stale, name)
		}
	})
	if len(stale) != 0 {
		t.Errorf("%d $ref sites still point at the old name", len(stale))
	}
}
