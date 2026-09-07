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
	"strings"
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
						"multipart/form-data": {Encoding: map[string]Encoding{
							"part": {Headers: map[string]Header{"X-Part": {Schema: ref("old")}}},
						}},
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

// A component this pass skips keeps its key, so a short name must not be
// allowed to land on it: the components map would keep one schema and the
// other's $refs would silently point at the wrong type.
func TestShortSchemaNamesDoesNotClaimASkippedKey(t *testing.T) {
	usedTypes := map[string]*Schema{
		"github.com/acme/svc/internal/api.Reference": {Type: "object"},
	}
	components := &Components{Schemas: map[string]*Schema{
		schemaComponentNameReplacer.Replace("github.com/acme/svc/internal/api.Reference"): {Type: "object"},
		// Already unqualified, so shortSchemaNames skips it — and "Reference"
		// is exactly what the qualified type above would shorten to.
		"Reference": {Type: "string"},
	}}

	renames := shortSchemaNames(components, usedTypes)

	spec := &OpenAPISpec{Paths: map[string]PathItem{}, Components: components}
	applySchemaRenames(spec, renames)

	if len(spec.Components.Schemas) != 2 {
		t.Fatalf("a schema was dropped: %v", mapKeys(spec.Components.Schemas))
	}
	if got := spec.Components.Schemas["Reference"]; got == nil || got.Type != "string" {
		t.Errorf("the skipped component was overwritten: %+v", got)
	}
}

// Distinct paths can normalize to one identifier (/a-b and /a/b both read as
// "getAB"), so the numeric fallback is load-bearing rather than decorative.
func TestApplyOperationIDsNumbersNormalizedCollisions(t *testing.T) {
	op := func(id string) *Operation { return &Operation{OperationID: id} }
	spec := &OpenAPISpec{Paths: map[string]PathItem{
		"/a-b": {Get: op("pkg.first")},
		"/a/b": {Get: op("pkg.second")},
		"/aB":  {Get: op("pkg.third")},
	}}

	applyOperationIDs(spec, NamingMethodPath)

	seen := map[string]string{}
	for path, item := range spec.Paths {
		item := item
		id := item.Get.OperationID
		if prev, dup := seen[id]; dup {
			t.Errorf("operationId %q used by both %s and %s", id, prev, path)
		}
		seen[id] = path
	}
	if len(seen) != 3 {
		t.Errorf("want 3 distinct ids, got %v", seen)
	}
}

// The numeric fallback must not run out. More operations normalizing to one
// identifier than a fixed cap allows used to leave the overflow sharing an id;
// the bound is the operation count, so a name is always available.
func TestApplyOperationIDsFallbackScalesWithOperationCount(t *testing.T) {
	const n = 70 // more than the fixed cap the first version used
	paths := map[string]PathItem{}
	for i := 0; i < n; i++ {
		// Each path differs only in characters methodPathID drops, so all n
		// normalize to the same identifier.
		paths["/a"+strings.Repeat("-", i+1)+"b"] = PathItem{Get: &Operation{OperationID: "pkg.shared"}}
	}
	spec := &OpenAPISpec{Paths: paths}

	applyOperationIDs(spec, NamingMethodPath)

	seen := map[string]string{}
	for path, item := range spec.Paths {
		item := item
		id := item.Get.OperationID
		if prev, dup := seen[id]; dup {
			t.Errorf("operationId %q shared by %s and %s", id, prev, path)
		}
		seen[id] = path
	}
	if len(seen) != n {
		t.Errorf("got %d distinct ids for %d operations", len(seen), n)
	}
}

// "invalid type" is go/types rendering Typ[Invalid] — a handler whose type did
// not check. It reached 153 operationIds on a real project (issue #459). It is
// prose, not a name, so it is replaced even when it is not duplicated.
func TestRepairOperationIDsReplacesUnusableIDs(t *testing.T) {
	spec := &OpenAPISpec{Paths: map[string]PathItem{
		"/a": {Get: &Operation{OperationID: "invalid type"}},
		"/b": {Get: &Operation{OperationID: ""}},
		"/c": {Get: &Operation{OperationID: "pkg.realHandler"}},
	}}

	repairOperationIDs(spec)

	if got := spec.Paths["/a"].Get.OperationID; got != "getA" {
		t.Errorf("/a: operationId %q, want %q", got, "getA")
	}
	if got := spec.Paths["/b"].Get.OperationID; got != "getB" {
		t.Errorf("/b: operationId %q, want %q", got, "getB")
	}
	// Unique and usable: left exactly as it was.
	if got := spec.Paths["/c"].Get.OperationID; got != "pkg.realHandler" {
		t.Errorf("/c: operationId %q, want it untouched", got)
	}
}

// Every holder of a duplicated id is replaced, not the runners-up: keeping it
// for whichever route sorted first would be arbitrary in a way a reader cannot
// see.
func TestRepairOperationIDsReplacesEveryHolderOfADuplicate(t *testing.T) {
	spec := &OpenAPISpec{Paths: map[string]PathItem{
		"/x": {Get: &Operation{OperationID: "pkg.shared"}},
		"/y": {Get: &Operation{OperationID: "pkg.shared"}},
		"/z": {Get: &Operation{OperationID: "pkg.unique"}},
	}}

	repairOperationIDs(spec)

	for path, want := range map[string]string{"/x": "getX", "/y": "getY", "/z": "pkg.unique"} {
		if got := spec.Paths[path].Get.OperationID; got != want {
			t.Errorf("%s: operationId %q, want %q", path, got, want)
		}
	}
}

// A replacement never lands on an id that a kept operation already holds.
func TestRepairOperationIDsAvoidsKeptIDs(t *testing.T) {
	spec := &OpenAPISpec{Paths: map[string]PathItem{
		// Both duplicates normalize to "getA"...
		"/a-": {Get: &Operation{OperationID: "pkg.shared"}},
		"/a":  {Get: &Operation{OperationID: "pkg.shared"}},
		// ...and this operation already owns "getA" outright.
		"/A": {Get: &Operation{OperationID: "getA"}},
	}}

	repairOperationIDs(spec)

	seen := map[string]string{}
	for path, item := range spec.Paths {
		id := item.Get.OperationID
		if prev, dup := seen[id]; dup {
			t.Errorf("operationId %q shared by %s and %s", id, prev, path)
		}
		seen[id] = path
	}
	if got := spec.Paths["/A"].Get.OperationID; got != "getA" {
		t.Errorf("/A: operationId %q, want it untouched", got)
	}
}
