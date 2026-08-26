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
	"sort"
	"strings"
	"testing"

	intspec "github.com/ehabterra/apispec/internal/spec"
	"github.com/ehabterra/apispec/spec"
)

// TestTestdata_BranchStatusScope locks in that a status write only claims a
// body the write DOMINATES — same function, written first, and inside no region
// the body escaped.
//
// Before the rule, `if bad { WriteHeader(400); return }` above a success body
// gave that body the 400: the endpoint documented its error status carrying its
// success schema, and had no success response at all. Measured on this fixture,
// /guard was `400(Item)` and became `200(Item) 400(no content)`.
//
// Statuses are asserted together with whether each carries a body, because the
// defect was never a missing status — it was the wrong status carrying the
// schema.
func TestTestdata_BranchStatusScope(t *testing.T) {
	out := loadTestdataWithFixtureConfig(t, "branch_status_scope", spec.DefaultHTTPConfig())
	noDanglingRefs(t, out)

	// status -> the schema name it carries, "" for a bodyless response.
	want := map[string]map[string]string{
		// The guard's 400 is bodyless and the success body is an implicit 200.
		"/guard/{id}": {"400": "", "200": "Item"},
		// Control: the body written INSIDE the arm that set the status does
		// carry it, and the success body below is unaffected.
		"/paired/{id}": {"400": "APIError", "200": "Item"},
		// Nesting is not escaping: a body deeper inside the same arm is still
		// dominated by the status write.
		"/nested/{id}": {"400": "APIError", "200": "Item"},
		// A loop may run zero times, so the status does not dominate what
		// follows it.
		"/loop": {"202": "", "200": "array"},
		// Neither arm's status dominates the body below the branch, so the
		// honest answer is that it carries neither (golden rule #7). Before the
		// rule the body took whichever arm the walk saw last.
		"/arms": {"202": "", "206": "", "200": "Item"},
		// The 201 dominates BOTH bodies and is carried by both (#391). They
		// are the same type, so the response is one schema — nothing to
		// compose. This used to also document a spurious 200, because a
		// pending status was consumed by the first body that claimed it.
		"/before": {"201": "Item"},
	}

	// One status over two different types: the endpoint can answer 202 with
	// either, so the response alternates between them rather than keeping
	// whichever the walk saw last (#391). Asserted apart from the table above,
	// which describes responses carrying a single schema.
	alternatives := out.Paths["/alternatives"]
	if op := opFor(alternatives, "GET"); op == nil {
		t.Error("GET /alternatives: expected operation, missing")
	} else if resp, ok := op.Responses["202"]; !ok {
		t.Errorf("GET /alternatives: response 202 missing; have %v", responseCodesOf(op.Responses))
	} else {
		media, ok := resp.Content["application/json"]
		switch {
		case !ok || media.Schema == nil:
			t.Errorf("GET /alternatives 202: no JSON schema, got %v", resp.Content)
		case len(media.Schema.AnyOf) != 2:
			t.Errorf("GET /alternatives 202: schema = %+v, want an anyOf of both bodies", media.Schema)
		default:
			got := []string{schemaRefName(media.Schema.AnyOf[0]), schemaRefName(media.Schema.AnyOf[1])}
			sort.Strings(got)
			if !equalStrings(got, []string{"APIError", "Item"}) {
				t.Errorf("GET /alternatives 202: anyOf members %v, want [APIError Item]", got)
			}
		}
	}

	for path, wantResponses := range want {
		item, ok := out.Paths[path]
		if !ok {
			t.Errorf("path %q missing; have %v", path, mapPathKeys(out.Paths))
			continue
		}
		op := opFor(item, "GET")
		if op == nil {
			t.Errorf("GET %s: expected operation, missing", path)
			continue
		}

		gotStatuses := make([]string, 0, len(op.Responses))
		for status := range op.Responses {
			gotStatuses = append(gotStatuses, status)
		}
		wantStatuses := make([]string, 0, len(wantResponses))
		for status := range wantResponses {
			wantStatuses = append(wantStatuses, status)
		}
		sort.Strings(gotStatuses)
		sort.Strings(wantStatuses)
		if !equalStrings(gotStatuses, wantStatuses) {
			t.Errorf("GET %s: responses %v, want %v", path, gotStatuses, wantStatuses)
			continue
		}

		for status, wantSchema := range wantResponses {
			gotSchema := schemaNameOf(op.Responses[status])
			if gotSchema != wantSchema {
				what := "a body of " + wantSchema
				if wantSchema == "" {
					what = "no body"
				}
				t.Errorf("GET %s response %s: carries %q, want %s",
					path, status, gotSchema, what)
			}
		}
	}
}

// schemaRefName is the bare component name a schema references.
func schemaRefName(s *intspec.Schema) string {
	if s == nil || s.Ref == "" {
		return ""
	}
	name := s.Ref[strings.LastIndexByte(s.Ref, '/')+1:]
	return name[strings.LastIndexByte(name, '_')+1:]
}

// schemaNameOf names the schema a response carries: the bare component name for
// a $ref, the type for an inline schema, and "" when there is no content at all.
// Media types are visited in sorted order so a response with more than one
// cannot make the assertion depend on map order.
func schemaNameOf(resp intspec.Response) string {
	types := make([]string, 0, len(resp.Content))
	for mediaType := range resp.Content {
		types = append(types, mediaType)
	}
	sort.Strings(types)
	for _, mediaType := range types {
		schema := resp.Content[mediaType].Schema
		if schema == nil {
			continue
		}
		if ref := schema.Ref; ref != "" {
			name := ref[strings.LastIndexByte(ref, '/')+1:]
			// Components are named after the fully-qualified type; the fixture
			// only needs the type itself.
			return name[strings.LastIndexByte(name, '_')+1:]
		}
		return schema.Type
	}
	return ""
}
