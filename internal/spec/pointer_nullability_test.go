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

	"github.com/ehabterra/apispec/internal/metadata"
)

// TestNullableIfPointer covers the answers that are "leave it alone", each for
// a different reason — the widening is a claim about the wire, so it must be
// made only where encoding/json really writes a null.
func TestNullableIfPointer(t *testing.T) {
	on := &APISpecConfig{Schema: SchemaConfig{NullableFromPointers: true}}
	str := func() *Schema { return &Schema{Type: "string"} }

	t.Run("widens a pointer with no omitempty", func(t *testing.T) {
		got := nullableIfPointer(on, str(), "*string", `json:"p"`)
		if len(got.AnyOf) != 2 || got.AnyOf[1].Type != "null" {
			t.Fatalf("got %+v, want a union admitting null", got)
		}
		if got.AnyOf[0].Type != "string" {
			t.Errorf("the non-null branch = %+v", got.AnyOf[0])
		}
	})

	for _, tc := range []struct {
		name, fieldType, tag string
		cfg                  *APISpecConfig
		why                  string
	}{
		{"off by default", "*string", `json:"p"`, &APISpecConfig{}, "the option is opt-in"},
		{"nil config", "*string", `json:"p"`, nil, "nothing to read the option from"},
		{"not a pointer", "string", `json:"p"`, on, "a value field is never written as null"},
		{"pointer with omitempty", "*string", `json:"p,omitempty"`, on, "absent when nil, never null"},
		{"pointer with omitzero", "*string", `json:"p,omitzero"`, on, "absent when zero, never null"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := nullableIfPointer(tc.cfg, str(), tc.fieldType, tc.tag)
			if len(got.AnyOf) != 0 {
				t.Errorf("got %+v, want it left alone — %s", got, tc.why)
			}
		})
	}

	t.Run("a schema that already admits null", func(t *testing.T) {
		// Wrapping twice would nest a union inside a union and say nothing new.
		already := &Schema{AnyOf: []*Schema{{Type: "string"}, {Type: "null"}}}
		got := nullableIfPointer(on, already, "*string", `json:"p"`)
		if len(got.AnyOf) != 2 {
			t.Errorf("got %+v, want the existing union untouched", got)
		}
	})

	t.Run("a nil schema", func(t *testing.T) {
		if got := nullableIfPointer(on, nil, "*string", `json:"p"`); got != nil {
			t.Errorf("got %+v, want nil", got)
		}
	})

	t.Run("the description moves to the field", func(t *testing.T) {
		// A reader looks at the property for it, not inside a branch.
		got := nullableIfPointer(on, &Schema{Type: "string", Description: "why"}, "*string", `json:"p"`)
		if got.Description != "why" {
			t.Errorf("field description = %q, want it kept", got.Description)
		}
		if got.AnyOf[0].Description != "" {
			t.Errorf("the branch kept the description too: %q", got.AnyOf[0].Description)
		}
	})
}

// TestTypeMarshalsItself covers the guard that keeps tag-derived statements off
// a type whose declared fields are not its wire shape.
func TestTypeMarshalsItself(t *testing.T) {
	meta := newTestMeta()
	sp := meta.StringPool

	withMethods := func(names ...string) *metadata.Type {
		typ := &metadata.Type{Name: sp.Get("T")}
		for _, n := range names {
			typ.Methods = append(typ.Methods, metadata.Method{Name: sp.Get(n)})
		}
		return typ
	}

	if !typeMarshalsItself(meta, withMethods("String", "MarshalJSON")) {
		t.Error("a type declaring MarshalJSON was not recognised")
	}
	if typeMarshalsItself(meta, withMethods("String", "UnmarshalJSON")) {
		t.Error("UnmarshalJSON is about decoding and says nothing about the wire shape sent")
	}
	if typeMarshalsItself(meta, withMethods()) {
		t.Error("a type with no methods marshals itself")
	}
	if typeMarshalsItself(nil, withMethods("MarshalJSON")) {
		t.Error("nil metadata answered true")
	}
	if typeMarshalsItself(meta, nil) {
		t.Error("a nil type answered true")
	}
}
