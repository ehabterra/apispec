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

package insight

import (
	"reflect"
	"testing"

	"github.com/ehabterra/apispec/internal/spec"
)

// bodySpec pairs every interesting body state with a status that makes its
// meaning clear: a resolved schema, a body whose type did not resolve, and no
// body at all — the last one both where it is correct (204) and where it is the
// symptom of a write apispec could not follow (200).
func bodySpec() *spec.OpenAPISpec {
	return &spec.OpenAPISpec{
		Paths: map[string]spec.PathItem{
			"/resolved":   {Get: &spec.Operation{Responses: jsonResp("200", ref("Order"))}},
			"/list":       {Get: &spec.Operation{Responses: jsonResp("200", &spec.Schema{Type: "array", Items: ref("Order")})}},
			"/scalar":     {Get: &spec.Operation{Responses: jsonResp("200", &spec.Schema{Type: "string"})}},
			"/unresolved": {Get: &spec.Operation{Responses: jsonResp("200", ref("Placeholder"))}},
			"/dangling":   {Get: &spec.Operation{Responses: jsonResp("200", ref("Missing"))}},
			"/opaque":     {Get: &spec.Operation{Responses: jsonResp("200", &spec.Schema{Type: "object"})}},
			// A map[string]any response: no declared fields, but the values are
			// constrained — the type resolved, the API is just free-form.
			"/freeform": {Get: &spec.Operation{Responses: jsonResp("200", &spec.Schema{Type: "object", AdditionalProperties: &spec.Schema{Type: "string"}})}},
			"/nobody":   {Get: &spec.Operation{Responses: map[string]spec.Response{"200": {Description: "OK"}}}},
			"/deleted":  {Delete: &spec.Operation{Responses: map[string]spec.Response{"204": {Description: "No Content"}}}},
			"/failing": {Get: &spec.Operation{Responses: map[string]spec.Response{
				"500":     {Content: map[string]spec.MediaType{"application/json": {Schema: ref("Order")}}},
				"default": {Description: "error"},
			}}},
		},
		Components: &spec.Components{Schemas: map[string]*spec.Schema{
			"Order":       {Type: "object", Properties: map[string]*spec.Schema{"id": {Type: "string"}}},
			"Placeholder": {Type: "object", Description: "External or unresolved type: foo.Bar"},
		}},
	}
}

// TestStatusBodies pins the per-status split the dashboard reports. The buckets
// are different diagnoses, so a response must land in the right one: `empty` at
// 200 means the body was never found, `unresolved` means it was found and the type
// could not be mapped, `freeForm` means the type resolved and genuinely has no
// declared fields.
func TestStatusBodies(t *testing.T) {
	rep := BuildOverview(bodySpec(), nil)

	want := []StatusBody{
		// 200: Order ref, array of Order, plain string resolve; placeholder,
		// dangling ref and bare `type: object` do not; one documents no body.
		{Status: "200", Total: 8, WithSchema: 3, FreeForm: 1, Empty: 1, Unresolved: 3},
		{Status: "204", Total: 1, Empty: 1},
		{Status: "500", Total: 1, WithSchema: 1},
		// `default` last: it is not a status code but the residue of one that
		// could not be pinned down.
		{Status: "default", Total: 1, Empty: 1},
	}
	if !reflect.DeepEqual(rep.StatusBodies, want) {
		t.Errorf("statusBodies =\n  %+v\nwant\n  %+v", rep.StatusBodies, want)
	}

	// Every response is counted exactly once, and the totals agree with the
	// plain status histogram the same walk produces.
	for _, sb := range rep.StatusBodies {
		if sb.WithSchema+sb.FreeForm+sb.Empty+sb.Unresolved != sb.Total {
			t.Errorf("status %s: buckets %d+%d+%d+%d != total %d", sb.Status, sb.WithSchema, sb.FreeForm, sb.Empty, sb.Unresolved, sb.Total)
		}
		if got := countOf(rep.ByStatus, sb.Status); got != sb.Total {
			t.Errorf("status %s: byStatus=%d, statusBodies total=%d", sb.Status, got, sb.Total)
		}
	}
}

// TestBodyResolution covers the classifier directly, and above all the line
// between "apispec could not read this type" and "the type has no fields to
// read" — the two render alike and mean opposite things.
func TestBodyResolution(t *testing.T) {
	comps := map[string]*spec.Schema{
		"Order":       {Type: "object", Properties: map[string]*spec.Schema{"id": {Type: "string"}}},
		"Empty":       {Type: "object"}, // an empty struct is a real, documented shape
		"Placeholder": {Type: "object", Description: "External or unresolved type: foo.Bar"},
	}

	cases := []struct {
		name string
		sc   *spec.Schema
		want string
	}{
		{"named component", ref("Order"), bodySchema},
		{"named empty struct", ref("Empty"), bodySchema},
		{"scalar", &spec.Schema{Type: "string"}, bodySchema},
		{"list of a named type", &spec.Schema{Type: "array", Items: ref("Order")}, bodySchema},
		{"inline object with fields", &spec.Schema{Type: "object", Properties: map[string]*spec.Schema{"id": {Type: "string"}}}, bodySchema},
		{"composition", &spec.Schema{OneOf: []*spec.Schema{ref("Order")}}, bodySchema},
		{"enum", &spec.Schema{Type: "string", Enum: []interface{}{"a"}}, bodySchema},

		// A Go map: the values are constrained, the keys are open. Resolved, but
		// it tells a client nothing about fields.
		{"map with a value type", &spec.Schema{Type: "object", AdditionalProperties: &spec.Schema{Type: "string"}}, bodyFreeForm},
		{"map of anything", &spec.Schema{Type: "object", AdditionalProperties: &spec.Schema{}}, bodyFreeForm},
		{"list of maps", &spec.Schema{Type: "array", Items: &spec.Schema{Type: "object", AdditionalProperties: &spec.Schema{}}}, bodyFreeForm},

		{"nil", nil, bodyUnresolved},
		{"dangling ref", ref("Missing"), bodyUnresolved},
		{"placeholder component", ref("Placeholder"), bodyUnresolved},
		{"bare object", &spec.Schema{Type: "object"}, bodyUnresolved},
		{"nothing at all", &spec.Schema{}, bodyUnresolved},
		{"list of what?", &spec.Schema{Type: "array"}, bodyUnresolved},
		{"list of an unknown component", &spec.Schema{Type: "array", Items: ref("Missing")}, bodyUnresolved},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := bodyResolution(tc.sc, comps); got != tc.want {
				t.Errorf("bodyResolution = %q, want %q", got, tc.want)
			}
		})
	}
}
