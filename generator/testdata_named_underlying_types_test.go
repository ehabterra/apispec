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

	"github.com/ehabterra/apispec/spec"
)

// TestTestdata_NamedUnderlyingTypes guards issue #333: a named type whose
// underlying type is a container documented as an empty object.
//
// The cause was in metadata, not the mapper. processTypeKind recorded a target
// for `type Count int` (an *ast.Ident) but not for `type IDs []string`, so the
// spec layer had nothing to build from and defaulted to {type: object}. Named
// primitives worked, which is why it went unnoticed.
func TestTestdata_NamedUnderlyingTypes(t *testing.T) {
	out := loadTestdataWithFixtureConfig(t, "named_underlying_types", spec.DefaultHTTPConfig())
	noDanglingRefs(t, out)

	comp := func(suffix string) *spec.Schema {
		t.Helper()
		if out.Components == nil {
			t.Fatal("no components")
		}
		for name, s := range out.Components.Schemas {
			if strings.HasSuffix(name, suffix) {
				return s
			}
		}
		t.Fatalf("no component ending %q; have %v", suffix, componentNamesOf(out))
		return nil
	}

	t.Run("named array carries the array shape", func(t *testing.T) {
		point := comp("_Point")
		if point.Type != "array" || point.Items == nil || point.Items.Type != "integer" {
			t.Fatalf("Point = %+v, want an array of integer ([2]int64)", point)
		}
		if point.MinItems != 2 || point.MaxItems != 2 {
			t.Errorf("Point bounds = (%d, %d), want (2, 2)", point.MinItems, point.MaxItems)
		}
	})

	t.Run("named slice carries the slice shape", func(t *testing.T) {
		ids := comp("_IDs")
		if ids.Type != "array" || ids.Items == nil || ids.Items.Type != "string" {
			t.Errorf("IDs = %+v, want an array of string ([]string)", ids)
		}
	})

	t.Run("named map carries the map shape", func(t *testing.T) {
		lookup := comp("_Lookup")
		if lookup.Type != "object" || lookup.AdditionalProperties == nil {
			t.Fatalf("Lookup = %+v, want an object with additionalProperties (map[string]int)", lookup)
		}
		if lookup.AdditionalProperties.Type != "integer" {
			t.Errorf("Lookup values = %q, want integer", lookup.AdditionalProperties.Type)
		}
	})

	// The element must resolve to the ONE component for Point, qualified with
	// its package. The target is recorded as written ("[]Point"), so without
	// qualification it produces a second, duplicate component under the bare
	// name — which is what happened on the first attempt at this fix.
	t.Run("named slice of a named type refs the qualified component", func(t *testing.T) {
		nested := comp("_Nested")
		if nested.Type != "array" || nested.Items == nil {
			t.Fatalf("Nested = %+v, want an array", nested)
		}
		if nested.Items.Ref == "" {
			t.Fatalf("Nested items = %+v, want a $ref to Point", nested.Items)
		}
		if !strings.Contains(nested.Items.Ref, "named_underlying_types_Point") {
			t.Errorf("Nested items $ref = %q, want the package-qualified Point", nested.Items.Ref)
		}
		for name := range out.Components.Schemas {
			if name == "Point" {
				t.Error(`a bare "Point" component exists alongside the qualified one: the target was not qualified`)
			}
		}
	})

	// The case that already worked, kept as a control: a named primitive.
	t.Run("named primitive still resolves", func(t *testing.T) {
		bundle := comp("_Bundle")
		count, ok := bundle.Properties["count"]
		if !ok {
			t.Fatal("Bundle.count missing")
		}
		if count.Ref != "" || count.Type != "integer" {
			t.Errorf("Bundle.count = %+v, want an inline integer (type Count int)", count)
		}
	})
}

func componentNamesOf(out *spec.OpenAPISpec) []string {
	var names []string
	for n := range out.Components.Schemas {
		names = append(names, n)
	}
	return names
}
