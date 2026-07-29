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

// TestQualifyFieldType covers the package-qualification of a field type
// (issue #259): the qualifier belongs on the named leaf, so it must be pushed
// through every container constructor rather than glued in front of whatever
// follows the first one.
func TestQualifyFieldType(t *testing.T) {
	const pkg = "example.com/repro"

	for _, tc := range []struct {
		name  string
		field string
		want  string
	}{
		{"named", "Item", pkg + ".Item"},
		{"pointer", "*Item", "*" + pkg + ".Item"},
		{"slice", "[]Item", "[]" + pkg + ".Item"},
		{"slice of pointer", "[]*Item", "[]*" + pkg + ".Item"},
		// The reported shapes: every level is peeled, so the qualifier lands on
		// the leaf instead of producing "[]example.com/repro.[]Item".
		{"nested slice", "[][]Item", "[][]" + pkg + ".Item"},
		{"triple slice", "[][][]Item", "[][][]" + pkg + ".Item"},
		{"fixed array", "[3]Item", "[3]" + pkg + ".Item"},
		{"map value", "map[string]Item", "map[string]" + pkg + ".Item"},
		{"map of slices", "map[string][]Item", "map[string][]" + pkg + ".Item"},
		{"slice of maps", "[]map[string]Item", "[]map[string]" + pkg + ".Item"},
		{"generic base only", "Page[Item]", pkg + ".Page[Item]"},
		// A container of primitives has no name to qualify — it must be left
		// alone so it stays inline all the way down.
		{"nested slice of primitives", "[][]string", "[][]string"},
		{"map of nested primitive slices", "map[string][][]int", "map[string][][]int"},
		// Already-qualified leaves and empty input are untouched.
		{"already qualified", "[][]other.Item", "[][]other.Item"},
		{"stdlib qualified", "map[string]time.Duration", "map[string]time.Duration"},
		{"empty", "", ""},
		// A container with nothing at the bottom has no name to qualify.
		{"degenerate container", "[]", "[]"},
		// Anonymous struct literals keep the legacy glued-on prefix form the
		// anon-struct paths document (and ignore).
		{"anon struct", "struct{Name string}", pkg + ".struct{Name string}"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := qualifyFieldType(tc.field, pkg); got != tc.want {
				t.Errorf("qualifyFieldType(%q) = %q, want %q", tc.field, got, tc.want)
			}
		})
	}

	if got := qualifyFieldType("Item", ""); got != "Item" {
		t.Errorf("qualifyFieldType with no package = %q, want it unchanged", got)
	}
}

// TestGenerateStructSchema_NestedContainerFields is the layer-level guard for
// issue #259: nested containers nest inline, and only the named leaf becomes a
// $ref. Before the fix the inner container was handed to the $ref fast-path
// and the reference dangled.
func TestGenerateStructSchema_NestedContainerFields(t *testing.T) {
	pool := metadata.NewStringPool()
	item := &metadata.Type{
		Name: pool.Get("Item"),
		Pkg:  pool.Get("main"),
		Kind: pool.Get("struct"),
		Fields: []metadata.Field{
			{Name: pool.Get("Name"), Type: pool.Get("string")},
		},
	}
	table := &metadata.Type{
		Name: pool.Get("Table"),
		Pkg:  pool.Get("main"),
		Kind: pool.Get("struct"),
		Fields: []metadata.Field{
			{Name: pool.Get("Header"), Type: pool.Get("[]string")},
			{Name: pool.Get("Rows"), Type: pool.Get("[][]string")},
			{Name: pool.Get("Cube"), Type: pool.Get("[][][]int")},
			{Name: pool.Get("Grid"), Type: pool.Get("[][]Item")},
			{Name: pool.Get("Buckets"), Type: pool.Get("map[string][][]int")},
		},
	}
	meta := &metadata.Metadata{
		StringPool: pool,
		Packages: map[string]*metadata.Package{
			"main": {Files: map[string]*metadata.File{
				"main.go": {Types: map[string]*metadata.Type{"Item": item, "Table": table}},
			}},
		},
	}

	schema, schemas := generateStructSchema(map[string]*Schema{}, "main.Table", table, meta, DefaultAPISpecConfig(), map[string]bool{})
	if schema == nil {
		t.Fatal("expected a schema for main.Table")
	}

	// depthAndLeaf peels array levels so nesting is asserted without pinning
	// component names.
	depthAndLeaf := func(s *Schema) (int, *Schema) {
		depth := 0
		for s != nil && s.Type == "array" {
			depth++
			s = s.Items
		}
		return depth, s
	}

	if d, leaf := depthAndLeaf(schema.Properties["Header"]); d != 1 || leaf == nil || leaf.Type != "string" {
		t.Errorf("Header: depth=%d leaf=%+v, want a 1-deep array of string", d, leaf)
	}
	if d, leaf := depthAndLeaf(schema.Properties["Rows"]); d != 2 || leaf == nil || leaf.Type != "string" || leaf.Ref != "" {
		t.Errorf("Rows: depth=%d leaf=%+v, want a 2-deep array of string", d, leaf)
	}
	if d, leaf := depthAndLeaf(schema.Properties["Cube"]); d != 3 || leaf == nil || leaf.Type != "integer" {
		t.Errorf("Cube: depth=%d leaf=%+v, want a 3-deep array of integer", d, leaf)
	}

	d, leaf := depthAndLeaf(schema.Properties["Grid"])
	if d != 2 || leaf == nil || leaf.Ref != "#/components/schemas/main_Item" {
		t.Fatalf("Grid: depth=%d leaf=%+v, want a 2-deep array of $ref main_Item", d, leaf)
	}
	if schemas["main.Item"] == nil {
		t.Errorf("the referenced component main.Item was never registered; have %v", schemaKeys(schemas))
	}

	buckets := schema.Properties["Buckets"]
	if buckets == nil || buckets.AdditionalProperties == nil {
		t.Fatalf("Buckets = %+v, want an object with additionalProperties", buckets)
	}
	if d, leaf := depthAndLeaf(buckets.AdditionalProperties); d != 2 || leaf == nil || leaf.Type != "integer" {
		t.Errorf("Buckets value: depth=%d leaf=%+v, want a 2-deep array of integer", d, leaf)
	}

	// Nothing anonymous may end up as a component: a container has no name, so
	// a component keyed on one is exactly the dangling-$ref bug.
	for key := range schemas {
		if !canAddRefSchemaForType(key) {
			t.Errorf("container/unnameable type %q registered as a component", key)
		}
	}
}
