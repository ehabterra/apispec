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

	"github.com/ehabterra/apispec/internal/spec"
)

// A struct that embeds another documented NONE of the embedded fields — the
// mapper walked Type.Fields and an embed is recorded in Type.Embeds. Embedding
// is how Go composes response types, so any response embedding a shared Base,
// Meta or Envelope was under-documented by exactly what the embed carries, with
// nothing saying so (issue #487).
//
// Every expectation below is what `json.Marshal` actually emits for the same
// type, captured by running it — not what embedding looks like it should do.
func TestTestdata_EmbeddedStructs(t *testing.T) {
	out := loadTestdata(t, "embedded_structs", spec.DefaultHTTPConfig())
	noDanglingRefs(t, out)

	props := func(typeName string) []string {
		t.Helper()
		if out.Components == nil {
			t.Fatal("no components")
		}
		for name, schema := range out.Components.Schemas {
			if !strings.HasSuffix(name, "_"+typeName) {
				continue
			}
			names := make([]string, 0, len(schema.Properties))
			for p := range schema.Properties {
				names = append(names, p)
			}
			sort.Strings(names)
			return names
		}
		t.Fatalf("no component for %s", typeName)
		return nil
	}

	// json.Marshal(Item{Meta: &Meta{}}) =
	//   {"ID":0,"kind":"","named":{"Label":""},"Page":0,"Ref":"","Extra":""}
	//
	// Base promotes ID and kind (its own tag, so `kind` not `Kind`), Named nests
	// under its tag rather than promoting, *Meta promotes through the pointer,
	// Ref contributes one field named for the type, and the `json:"-"` embed
	// contributes nothing at all.
	want := []string{"Extra", "ID", "Page", "Ref", "kind", "named"}
	if got := props("Item"); !slices.Equal(got, want) {
		t.Errorf("Item properties = %v, want %v — these are the keys encoding/json emits", got, want)
	}

	// The tagged embed is an OBJECT, not a promotion: its own field has to be
	// inside it.
	for name, schema := range out.Components.Schemas {
		if !strings.HasSuffix(name, "_Item") {
			continue
		}
		named := schema.Properties["named"]
		if named == nil {
			t.Fatal("Item has no `named` property")
		}
		if named.Ref == "" && named.Properties["Label"] == nil {
			t.Errorf("`named` = %+v, want the embedded object — a tagged embed nests rather "+
				"than promoting", named)
		}
		if _, promoted := schema.Properties["Label"]; promoted {
			t.Error("`Label` was promoted to Item; its embed is tagged, so it nests under `named`")
		}
		if _, leaked := schema.Properties["Secret"]; leaked {
			t.Error("`Secret` reached Item; its embed carries `json:\"-\"`")
		}
	}

	// json.Marshal(Shadow{}) = {"ID":0,"kind":""} — the OUTER Kind wins, because
	// both are named `kind` and the outer one is shallower. Getting this
	// backwards would publish the embedded type's field under a name the outer
	// struct has taken.
	if got := props("Shadow"); !slices.Equal(got, []string{"ID", "kind"}) {
		t.Errorf("Shadow properties = %v, want [ID kind]", got)
	}

	// Promotion is transitive: Deep embeds Item, which embeds Base.
	deepWant := append(append([]string{}, want...), "Own")
	sort.Strings(deepWant)
	if got := props("Deep"); !slices.Equal(got, deepWant) {
		t.Errorf("Deep properties = %v, want %v — promotion carries through two levels",
			got, deepWant)
	}
}
