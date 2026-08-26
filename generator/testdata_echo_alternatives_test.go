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

// TestTestdata_EchoBadRequestAlternatives pins #391's composition on a
// framework fixture written the way handlers actually are.
//
// echo's CreateUser answers 400 three different ways — an inline map with
// `error` and `details`, an ErrorResponse, and an inline map with `error` —
// each from its own guard. The response is the alternation of all three. Before
// #391 the spec kept whichever the walk saw last and silently dropped the other
// two, so an endpoint with three documented failure shapes advertised one.
//
// This is also what the fixture's committed snapshots record, which is how the
// change was noticed: the 400's `$ref` did not disappear, it moved into an
// anyOf member.
func TestTestdata_EchoBadRequestAlternatives(t *testing.T) {
	out := loadTestdataWithFixtureConfig(t, "echo", spec.DefaultEchoConfig())
	noDanglingRefs(t, out)
	noNullSchemas(t, out)

	cases := []struct {
		path, method string
		want         []string // the shapes the status may carry, sorted
	}{
		// c.Bind failure -> {error, details}; validation -> ErrorResponse;
		// range check -> {error}.
		{"/v1/users/", "POST", []string{"ErrorResponse", "object{details,error}", "object{error}"}},
		// bad id -> {error}; c.Bind failure -> ErrorResponse.
		{"/v1/users/{id}", "PUT", []string{"ErrorResponse", "object{error}"}},
	}

	for _, tc := range cases {
		item, ok := out.Paths[tc.path]
		if !ok {
			t.Errorf("path %q missing; have %v", tc.path, mapPathKeys(out.Paths))
			continue
		}
		op := opFor(item, tc.method)
		if op == nil {
			t.Errorf("%s %s: operation missing", tc.method, tc.path)
			continue
		}
		resp, ok := op.Responses["400"]
		if !ok {
			t.Errorf("%s %s: response 400 missing; have %v", tc.method, tc.path, responseCodesOf(op.Responses))
			continue
		}
		media, ok := resp.Content["application/json"]
		if !ok || media.Schema == nil {
			t.Errorf("%s %s 400: no JSON schema, got %v", tc.method, tc.path, resp.Content)
			continue
		}
		if len(media.Schema.AnyOf) != len(tc.want) {
			t.Errorf("%s %s 400: %d alternatives, want %d — the handler answers 400 in %d shapes",
				tc.method, tc.path, len(media.Schema.AnyOf), len(tc.want), len(tc.want))
			continue
		}
		got := make([]string, 0, len(tc.want))
		for _, member := range media.Schema.AnyOf {
			got = append(got, shapeOf(member))
		}
		sort.Strings(got)
		if !equalStrings(got, tc.want) {
			t.Errorf("%s %s 400: alternatives %v, want %v", tc.method, tc.path, got, tc.want)
		}
	}
}

// shapeOf names a schema for comparison: the bare component for a $ref, and the
// sorted property names for an inline object, since the inline 400 bodies here
// are map literals that differ only in their keys.
func shapeOf(s *intspec.Schema) string {
	if s == nil {
		return "<nil>"
	}
	if s.Ref != "" {
		name := s.Ref[strings.LastIndexByte(s.Ref, '/')+1:]
		return name[strings.LastIndexByte(name, '_')+1:]
	}
	if len(s.Properties) > 0 {
		props := make([]string, 0, len(s.Properties))
		for name := range s.Properties {
			props = append(props, name)
		}
		sort.Strings(props)
		return "object{" + strings.Join(props, ",") + "}"
	}
	return s.Type
}
