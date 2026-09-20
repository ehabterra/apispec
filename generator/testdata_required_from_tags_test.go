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
	"sort"
	"strings"
	"testing"

	intspec "github.com/ehabterra/apispec/internal/spec"
)

func requiredFromTagsConfig(on bool) *intspec.APISpecConfig {
	cfg := intspec.DefaultChiConfig()
	cfg.Schema.RequiredFromJSONTags = on
	return cfg
}

func itemSchema(t *testing.T, out *intspec.OpenAPISpec) *intspec.Schema {
	t.Helper()
	for name, s := range out.Components.Schemas {
		if strings.HasSuffix(name, "_Item") {
			return s
		}
	}
	t.Fatalf("no Item component; have %v", schemaNames(out))
	return nil
}

// TestTestdata_RequiredFromTags pins `required` derived from what
// encoding/json does (issue #516).
//
// The encoder already states which fields are always on the wire — one with no
// `omitempty` and no `omitzero` is written on every encode — and none of it
// reached the document, so a generated client null-checked every field.
//
// Presence only: whether a value may be null is a separate statement (#368).
// `ptr` is always PRESENT, written as `null`, so it is required here.
func TestTestdata_RequiredFromTags(t *testing.T) {
	out := loadTestdata(t, "required_from_tags", requiredFromTagsConfig(true))
	noDanglingRefs(t, out)
	item := itemSchema(t, out)

	want := []string{"id", "name", "ptr", "validated", "when"}
	got := append([]string(nil), item.Required...)
	sort.Strings(got)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("required = %v, want %v", got, want)
	}

	for _, tc := range []struct{ field, why string }{
		{"note", "omitempty on an embedded VALUE's field still means absent"},
		{"optional", "omitempty"},
		{"zeroable", "omitzero (Go 1.24)"},
		{"ptrOpt", "a pointer WITH omitempty is absent when nil"},
		{"trace", "promoted through an embedded POINTER: gone entirely when Meta is nil"},
	} {
		if slices.Contains(item.Required, tc.field) {
			t.Errorf("%q is required, but %s", tc.field, tc.why)
		}
		if _, ok := item.Properties[tc.field]; !ok {
			t.Errorf("%q is not a property at all; the field set must be unchanged", tc.field)
		}
	}

	t.Run("a validation tag is merged, not duplicated", func(t *testing.T) {
		n := 0
		for _, r := range item.Required {
			if r == "validated" {
				n++
			}
		}
		if n != 1 {
			t.Errorf("validated appears %d times in required; it is stated by BOTH tags", n)
		}
	})

	t.Run("sorted", func(t *testing.T) {
		if !sort.StringsAreSorted(item.Required) {
			t.Errorf("required = %v, which is not sorted — the list is fed from two tag sources, "+
				"so its order must not depend on which arrived first", item.Required)
		}
	})
}

// TestTestdata_RequiredFromTagsIsOptIn pins that the option is off by default,
// so no existing document moves: it states what the SERVER SENDS, which is an
// over-claim for a request body, and that is a trade a project takes on
// knowingly rather than one inferred for it.
func TestTestdata_RequiredFromTagsIsOptIn(t *testing.T) {
	out := loadTestdata(t, "required_from_tags", requiredFromTagsConfig(false))
	item := itemSchema(t, out)

	if strings.Join(item.Required, ",") != "validated" {
		t.Errorf("required = %v with the option off, want only the validate:\"required\" field",
			item.Required)
	}
}
