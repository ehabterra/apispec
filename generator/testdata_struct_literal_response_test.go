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
)

// TestTestdata_StructLiteralResponse pins that an INLINE anonymous-struct
// response documents its fields, exactly as the same value assigned to a
// variable first always did (issue #515).
//
// The assertion that matters is the AGREEMENT: `/inline` and `/assigned` are
// the identical value written two ways, so comparing them to each other — not
// to a hand-written expectation — is what makes this a change detector rather
// than a schema snapshot. It was the single miss among 575 statically-typed
// responses in a downstream consumer's review.
func TestTestdata_StructLiteralResponse(t *testing.T) {
	out := loadTestdata(t, "struct_literal_response", intspec.DefaultChiConfig())
	noDanglingRefs(t, out)

	schemaOf := func(t *testing.T, path, status, mediaType string) *intspec.Schema {
		t.Helper()
		op := opFor(out.Paths[path], "GET")
		if op == nil {
			t.Fatalf("GET %s missing; have %v", path, mapPathKeys(out.Paths))
		}
		resp, ok := op.Responses[status]
		if !ok {
			t.Fatalf("GET %s has no %s; have %v", path, status, statusKeys(op))
		}
		media, ok := resp.Content[mediaType]
		if !ok || media.Schema == nil {
			got := make([]string, 0, len(resp.Content))
			for k := range resp.Content {
				got = append(got, k)
			}
			t.Fatalf("GET %s %s documents no %s body; have %v — an inline struct literal is a body like any other",
				path, status, mediaType, got)
		}
		return media.Schema
	}
	schemaAt := func(t *testing.T, path, status string) *intspec.Schema {
		t.Helper()
		return schemaOf(t, path, status, "application/json")
	}

	propertyNames := func(s *intspec.Schema) []string {
		names := make([]string, 0, len(s.Properties))
		for k := range s.Properties {
			names = append(names, k)
		}
		return names
	}

	// The control: assigning first has always worked, so it defines what the
	// inline spelling has to produce.
	assigned := schemaAt(t, "/assigned", "200")
	if len(assigned.Properties) != 2 {
		t.Fatalf("/assigned lost its own fields, so it cannot be the control: %v", propertyNames(assigned))
	}

	t.Run("inline through a helper matches the assigned form", func(t *testing.T) {
		inline := schemaAt(t, "/inline", "200")
		for _, field := range []string{"values", "truncated"} {
			if _, ok := inline.Properties[field]; !ok {
				t.Errorf("/inline is missing %q; the identical value assigned first has %v",
					field, propertyNames(assigned))
			}
		}
	})

	t.Run("inline encoded directly", func(t *testing.T) {
		// Under 200, not `default`: the implicit-200 rule needs a body to apply
		// to, so the missing body took the status with it.
		direct := schemaAt(t, "/direct", "200")
		if _, ok := direct.Properties["values"]; !ok {
			t.Errorf("/direct is missing %q; have %v", "values", propertyNames(direct))
		}
	})

	t.Run("a map literal still resolves", func(t *testing.T) {
		// The control in the other direction: a map IS described by its type
		// expression, which is why this shape worked all along and must keep
		// working — the fix reads the literal's own type FIRST.
		m := schemaAt(t, "/map", "200")
		if _, ok := m.Properties["values"]; !ok {
			t.Errorf("/map is missing %q; have %v", "values", propertyNames(m))
		}
	})

	// One inline error envelope reached from two routes with different success
	// bodies. This is the shape a real service writes, and the risk the fix
	// carries: a body that was invisible cannot be misattributed, and making it
	// visible must not put it on the wrong status or over the right body.
	t.Run("a shared inline error envelope lands on each route's own status", func(t *testing.T) {
		for _, tc := range []struct{ path, status string }{
			{"/shared-meta", "404"},
			{"/shared-download.pdf", "500"},
		} {
			errBody := schemaAt(t, tc.path, tc.status)
			if _, ok := errBody.Properties["errors"]; !ok {
				t.Errorf("GET %s %s does not carry the shared envelope; have %v",
					tc.path, tc.status, propertyNames(errBody))
			}
		}

		// And neither success body was displaced by it.
		if meta := schemaAt(t, "/shared-meta", "200"); meta.Ref == "" {
			t.Errorf("/shared-meta 200 lost its Meta $ref to the error envelope: %+v", meta)
		}
		download := schemaOf(t, "/shared-download.pdf", "200", "application/pdf")
		if download.Format != "binary" {
			t.Errorf("/shared-download.pdf 200 is %q/%q, want the streamed body — the error envelope displaced it",
				download.Type, download.Format)
		}
	})
}
