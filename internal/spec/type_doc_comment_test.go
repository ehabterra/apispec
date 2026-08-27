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

// TestGenerateSchemaFromTypeDescribesEveryKind covers issue #366 across the
// type-kind switch, not just structs.
//
// A struct is the kind a fixture reaches easily; interfaces, aliases and named
// container types are mapped through different branches and mostly resolve
// INLINE rather than as components, so a generated-spec fixture cannot reliably
// exercise them. The switch is the layer that changed, so it is asserted here.
func TestGenerateSchemaFromTypeDescribesEveryKind(t *testing.T) {
	const doc = "Thing is documented."

	cases := []struct {
		name   string
		kind   string
		target string
	}{
		{"struct", "struct", ""},
		{"interface", "interface", ""},
		{"alias of a primitive", "alias", "string"},
		{"named container falls through to the default branch", "", "[]string"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pool := metadata.NewStringPool()
			meta := &metadata.Metadata{StringPool: pool}
			typ := &metadata.Type{
				Name:     pool.Get("Thing"),
				Pkg:      pool.Get("example.com/app"),
				Kind:     pool.Get(tc.kind),
				Comments: pool.Get(doc),
			}
			if tc.target != "" {
				typ.Target = pool.Get(tc.target)
			}

			schema, _ := generateSchemaFromType(map[string]*Schema{}, "example.com/app.Thing",
				typ, meta, &APISpecConfig{}, map[string]bool{})
			if schema == nil {
				t.Fatal("no schema generated")
			}
			if schema.Description != doc {
				t.Errorf("description = %q, want %q — the doc comment did not reach a %q schema",
					schema.Description, doc, tc.kind)
			}
		})
	}
}

// TestGenerateSchemaFromTypeRespectsOptOut checks the switch honours
// ExcludeTypeComments for the non-struct kinds too.
func TestGenerateSchemaFromTypeRespectsOptOut(t *testing.T) {
	pool := metadata.NewStringPool()
	meta := &metadata.Metadata{StringPool: pool}
	typ := &metadata.Type{
		Name:     pool.Get("Thing"),
		Pkg:      pool.Get("example.com/app"),
		Kind:     pool.Get("interface"),
		Comments: pool.Get("Thing is documented."),
	}

	schema, _ := generateSchemaFromType(map[string]*Schema{}, "example.com/app.Thing",
		typ, meta, &APISpecConfig{ExcludeTypeComments: true}, map[string]bool{})
	if schema != nil && schema.Description != "" {
		t.Errorf("description %q present with ExcludeTypeComments", schema.Description)
	}
}

// TestWithDescriptionDoesNotCopyWhenThereIsNothingToAdd pins the fast path: the
// type-kind switch calls this for EVERY type, and most have no doc comment, so
// an empty description must not allocate a copy of every schema in the spec.
func TestWithDescriptionDoesNotCopyWhenThereIsNothingToAdd(t *testing.T) {
	s := &Schema{Type: "object"}
	if got := withDescription(s, ""); got != s {
		t.Error("an empty description copied the schema; it should return the original untouched")
	}

	// An existing description is more specific (a `_` marker's validation note,
	// or a type mapping) and wins.
	existing := &Schema{Type: "object", Description: "from a validation note"}
	if got := withDescription(existing, "Thing is documented."); got != existing {
		t.Error("an existing description was replaced")
	}

	// And when there IS something to add, the original is left alone.
	base := &Schema{Type: "string"}
	out := withDescription(base, "Thing is documented.")
	if out == base {
		t.Error("withDescription mutated the schema it was given instead of copying")
	}
	if base.Description != "" {
		t.Errorf("the original gained a description: %q", base.Description)
	}
	if out.Description != "Thing is documented." || out.Type != "string" {
		t.Errorf("copy = %+v, want the original's fields plus the description", out)
	}
}
