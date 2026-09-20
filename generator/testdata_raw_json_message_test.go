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
	"strings"
	"testing"

	intspec "github.com/ehabterra/apispec/internal/spec"
)

// TestTestdata_RawJSONMessage pins that json.RawMessage documents ANY JSON
// value (issue #518).
//
// It is bytes copied into the document verbatim, so the marshaler fallback's
// "assume string" is the one answer that is almost never right: a validator
// would reject every real payload, and a generated client types the field as a
// string and casts.
func TestTestdata_RawJSONMessage(t *testing.T) {
	out := loadTestdataWithFixtureConfig(t, "raw_json_message", intspec.DefaultChiConfig())
	noDanglingRefs(t, out)
	noUnresolvedPlaceholders(t, out)

	var envelope *intspec.Schema
	for name, s := range out.Components.Schemas {
		if strings.HasSuffix(name, "_Envelope") {
			envelope = s
		}
	}
	if envelope == nil {
		t.Fatalf("no Envelope component; have %v", schemaNames(out))
	}

	// anyOf-free, type-free, member-free: the schema that permits anything.
	assertAny := func(t *testing.T, field string, s *intspec.Schema) {
		t.Helper()
		if s == nil {
			t.Fatalf("%s has no schema", field)
		}
		if s.Type != "" {
			t.Errorf("%s has type %q; json.RawMessage is any JSON value, not a %s", field, s.Type, s.Type)
		}
		if s.Ref != "" {
			t.Errorf("%s is a $ref (%s); an unconstrained schema has no content to name", field, s.Ref)
		}
		if len(s.Properties) > 0 || s.Items != nil || s.AdditionalProperties != nil {
			t.Errorf("%s constrains its members; json.RawMessage constrains nothing", field)
		}
	}

	t.Run("the named type", func(t *testing.T) {
		assertAny(t, "raw", envelope.Properties["raw"])
	})

	t.Run("through the pointer branch", func(t *testing.T) {
		assertAny(t, "optRaw", envelope.Properties["optRaw"])
	})

	t.Run("through the slice branch", func(t *testing.T) {
		many := envelope.Properties["manyRaw"]
		if many == nil || many.Type != "array" {
			t.Fatalf("manyRaw = %+v, want an array", many)
		}
		assertAny(t, "manyRaw.items", many.Items)
	})

	// The control: a marshaler whose JSON form IS knowable must stay precise,
	// so the registry entry cannot be read as "every marshaler is any".
	t.Run("a knowable marshaler stays precise", func(t *testing.T) {
		when := envelope.Properties["when"]
		if when == nil || when.Type != "string" || when.Format != "date-time" {
			t.Errorf("when = %+v, want {string, date-time}", when)
		}
	})

	// No component may be generated for it: an unconstrained schema has no
	// content, so naming one publishes an empty definition and a $ref to it.
	for name := range out.Components.Schemas {
		if strings.Contains(name, "RawMessage") {
			t.Errorf("component %q was generated for an unconstrained schema", name)
		}
	}
}
