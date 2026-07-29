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
	"maps"
	"slices"
	"strings"
	"testing"

	intspec "github.com/ehabterra/apispec/internal/spec"
	"github.com/ehabterra/apispec/spec"
)

// TestTestdata_NestedSliceField locks in issue #259: a struct field typed
// [][]T used to be emitted as an array whose items were a $ref to a component
// nobody defined (the package qualifier was glued onto the inner container,
// which then looked nameable to the $ref fast-paths). Every container level
// must nest inline; only the named leaf may become a $ref.
func TestTestdata_NestedSliceField(t *testing.T) {
	out := loadTestdataWithFixtureConfig(t, "nested_slice_field", spec.DefaultChiConfig())
	noDanglingRefs(t, out)
	noUnresolvedPlaceholders(t, out)

	for _, path := range []string{"/table", "/containers"} {
		item, ok := out.Paths[path]
		if !ok {
			t.Fatalf("path %q missing; have %v", path, mapPathKeys(out.Paths))
		}
		if opFor(item, "GET") == nil {
			t.Errorf("GET %s missing", path)
		}
	}

	schemaBySuffix := func(suffix string) *intspec.Schema {
		t.Helper()
		for k, v := range out.Components.Schemas {
			if strings.HasSuffix(k, suffix) {
				return v
			}
		}
		t.Fatalf("no component ending %q; have %v", suffix, slices.Sorted(maps.Keys(out.Components.Schemas)))
		return nil
	}

	// arrayDepth peels array levels and returns the depth plus the schema at
	// the bottom, so nesting can be asserted without pinning component names.
	arrayDepth := func(s *intspec.Schema) (int, *intspec.Schema) {
		depth := 0
		for s != nil && s.Type == "array" {
			depth++
			s = s.Items
		}
		return depth, s
	}

	table := schemaBySuffix("_Table")

	// []string: the single-level case that always worked — the regression guard
	// for the level the fix must not change.
	if depth, leaf := arrayDepth(table.Properties["header"]); depth != 1 || leaf == nil || leaf.Type != "string" {
		t.Errorf("header: depth=%d leaf=%+v, want a 1-deep array of string", depth, leaf)
	}

	// [][]string: the reported shape. Both levels inline, string at the bottom,
	// no $ref anywhere along the way.
	if depth, leaf := arrayDepth(table.Properties["rows"]); depth != 2 || leaf == nil || leaf.Type != "string" || leaf.Ref != "" {
		t.Errorf("rows: depth=%d leaf=%+v, want a 2-deep array of string", depth, leaf)
	}

	// [][][]int: arbitrary nesting depth, not just two levels.
	if depth, leaf := arrayDepth(table.Properties["cube"]); depth != 3 || leaf == nil || leaf.Type != "integer" {
		t.Errorf("cube: depth=%d leaf=%+v, want a 3-deep array of integer", depth, leaf)
	}

	// [][]Item: the named leaf — and only the named leaf — becomes a $ref, so
	// the nesting survives instead of collapsing a level.
	depth, leaf := arrayDepth(table.Properties["grid"])
	if depth != 2 || leaf == nil || !strings.HasSuffix(leaf.Ref, "_Item") {
		t.Fatalf("grid: depth=%d leaf=%+v, want a 2-deep array of $ref …_Item", depth, leaf)
	}
	if item := schemaBySuffix("_Item"); item.Properties["name"] == nil {
		t.Errorf("Item component missing 'name': %+v", item)
	}

	containers := schemaBySuffix("_Containers")

	// []map[string]T and map[string][]T: the neighbouring composite shapes
	// named in the issue. The map value is the only $ref site.
	sliceOfMaps := containers.Properties["sliceOfMaps"]
	if sliceOfMaps == nil || sliceOfMaps.Type != "array" || sliceOfMaps.Items == nil ||
		sliceOfMaps.Items.AdditionalProperties == nil || sliceOfMaps.Items.AdditionalProperties.Type != "string" {
		t.Errorf("sliceOfMaps = %+v, want an array of map[string]string", sliceOfMaps)
	}

	mapOfSlices := containers.Properties["mapOfSlices"]
	if mapOfSlices == nil || mapOfSlices.AdditionalProperties == nil {
		t.Fatalf("mapOfSlices = %+v, want an object with additionalProperties", mapOfSlices)
	}
	if d, l := arrayDepth(mapOfSlices.AdditionalProperties); d != 1 || l == nil || l.Type != "string" {
		t.Errorf("mapOfSlices value: depth=%d leaf=%+v, want a 1-deep array of string", d, l)
	}

	mapOfNested := containers.Properties["mapOfNested"]
	if mapOfNested == nil || mapOfNested.AdditionalProperties == nil {
		t.Fatalf("mapOfNested = %+v, want an object with additionalProperties", mapOfNested)
	}
	if d, l := arrayDepth(mapOfNested.AdditionalProperties); d != 2 || l == nil || l.Type != "integer" {
		t.Errorf("mapOfNested value: depth=%d leaf=%+v, want a 2-deep array of integer", d, l)
	}

	sliceOfItems := containers.Properties["sliceOfItems"]
	if sliceOfItems == nil || sliceOfItems.Type != "array" || sliceOfItems.Items == nil ||
		sliceOfItems.Items.AdditionalProperties == nil ||
		!strings.HasSuffix(sliceOfItems.Items.AdditionalProperties.Ref, "_Item") {
		t.Errorf("sliceOfItems = %+v, want an array of map[string]$ref(…_Item)", sliceOfItems)
	}
}
