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
	"slices"
	"strings"
	"testing"

	intspec "github.com/ehabterra/apispec/internal/spec"
)

func nullableConfig(on bool) *intspec.APISpecConfig {
	cfg := intspec.DefaultChiConfig()
	cfg.Schema.RequiredFromJSONTags = true
	cfg.Schema.NullableFromPointers = on
	return cfg
}

// admitsNull reports whether a schema's union carries a null branch, and
// returns the non-null branch beside it.
func admitsNull(s *intspec.Schema) (*intspec.Schema, bool) {
	if s == nil || len(s.AnyOf) != 2 {
		return nil, false
	}
	for i, alt := range s.AnyOf {
		if alt != nil && alt.Type == "null" {
			return s.AnyOf[1-i], true
		}
	}
	return nil, false
}

// TestTestdata_PointerNullability pins that a field encoding/json writes as
// `null` says so (issue #368).
//
// A `*T` with no `omitempty` is always written, and written as `null` when the
// pointer is nil — so `type: string` claims a shape the API does not guarantee,
// and a client validating against it rejects a response the server legitimately
// sends.
func TestTestdata_PointerNullability(t *testing.T) {
	out := loadTestdata(t, "required_from_tags", nullableConfig(true))
	noDanglingRefs(t, out)
	item := itemSchema(t, out)

	t.Run("a pointer to a primitive", func(t *testing.T) {
		inner, ok := admitsNull(item.Properties["ptr"])
		if !ok {
			t.Fatalf("ptr = %+v, want a union admitting null", item.Properties["ptr"])
		}
		if inner.Type != "string" {
			t.Errorf("the non-null branch = %q, want string", inner.Type)
		}
	})

	t.Run("a pointer to a named type", func(t *testing.T) {
		// A $ref may carry no sibling keywords, which is why the union is the
		// only encoding available and why both cases use the same one.
		inner, ok := admitsNull(item.Properties["owner"])
		if !ok {
			t.Fatalf("owner = %+v, want a union admitting null", item.Properties["owner"])
		}
		if inner.Ref == "" {
			t.Errorf("the non-null branch = %+v, want a $ref", inner)
		}
	})

	t.Run("a pointer to a slice", func(t *testing.T) {
		// The union wraps the array; the ITEMS are not nullable.
		inner, ok := admitsNull(item.Properties["tags"])
		if !ok {
			t.Fatalf("tags = %+v, want a union admitting null", item.Properties["tags"])
		}
		if inner.Type != "array" {
			t.Errorf("the non-null branch = %q, want array", inner.Type)
		}
		if _, itemsNull := admitsNull(inner.Items); itemsNull {
			t.Error("the array's items admit null; only the field does")
		}
	})

	t.Run("a pointer WITH omitempty", func(t *testing.T) {
		// Absent when nil rather than null, so nothing widens.
		if _, ok := admitsNull(item.Properties["ptrOpt"]); ok {
			t.Errorf("ptrOpt = %+v; with omitempty the field is absent, never null",
				item.Properties["ptrOpt"])
		}
	})

	t.Run("a non-pointer", func(t *testing.T) {
		if _, ok := admitsNull(item.Properties["name"]); ok {
			t.Error("a non-pointer field was made nullable")
		}
	})

	// The two halves compose: always PRESENT and sometimes NULL are different
	// statements, and this field makes both.
	t.Run("required and nullable together", func(t *testing.T) {
		if !slices.Contains(item.Required, "ptr") {
			t.Errorf("ptr is always written, so it is required; required = %v", item.Required)
		}
	})

	// The description belongs to the field, not to the non-null branch, or a
	// reader looking at the property does not find it.
	t.Run("the field keeps its description", func(t *testing.T) {
		if !strings.Contains(item.Properties["ptr"].Description, "Always present") {
			t.Errorf("ptr lost its doc comment: %+v", item.Properties["ptr"])
		}
	})
}

// TestTestdata_PointerNullabilityIsOptIn pins that the option is off by
// default, so no existing document moves.
func TestTestdata_PointerNullabilityIsOptIn(t *testing.T) {
	out := loadTestdata(t, "required_from_tags", nullableConfig(false))
	item := itemSchema(t, out)

	for _, field := range []string{"ptr", "owner", "tags"} {
		if _, ok := admitsNull(item.Properties[field]); ok {
			t.Errorf("%q admits null with the option off", field)
		}
	}
}
