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
	"slices"
	"testing"
)

// boundFields is the whole of what a binder reads off a struct, so each of its
// naming rules is pinned on its own (issue #355).
func TestBoundFields(t *testing.T) {
	meta := embedMeta(t, "type Base struct {\n"+
		"	Page int `form:\"page\"`\n"+
		"}\n"+
		"type Tagged struct {\n"+
		"	Inner string `form:\"inner\"`\n"+
		"}\n"+
		"type Q struct {\n"+
		"	Base\n"+
		"	Tagged `form:\"tagged\"`\n"+
		"	Sort    string `form:\"sort,omitempty\"`\n"+
		"	Named   string `form:\",default=5\"`\n"+
		"	Plain   string\n"+
		"	Dash    string `form:\"-\"`\n"+
		"	Other   string `query:\"other\"`\n"+
		"	private string `form:\"private\"`\n"+
		"	Page    int    `form:\"page\"`\n"+
		"}\n")
	typ := findType(meta, "p", "Q")
	if typ == nil {
		t.Fatal("Q not found")
	}

	names := func(b StructBinding) []string {
		var out []string
		for _, f := range boundFields(meta, typ, b) {
			out = append(out, f.name)
		}
		return out
	}

	byName := names(StructBinding{Tag: "form", UntaggedByName: true})
	// The outer `page` shadows the one the untagged embed promotes, so it is
	// listed once, where the OUTER field is declared. A tagged embed is one
	// value, not a promotion, so neither `tagged` nor `inner` is a scalar
	// parameter. `Other` has no form tag and binds under its Go name.
	want := []string{"sort", "Named", "Plain", "Other", "page"}
	if !slices.Equal(byName, want) {
		t.Errorf("UntaggedByName: got %v, want %v", byName, want)
	}

	tagOnly := names(StructBinding{Tag: "form"})
	want = []string{"sort", "page"}
	if !slices.Equal(tagOnly, want) {
		t.Errorf("tag only: got %v, want %v", tagOnly, want)
	}
}

// A parameter documented from a declaration replaces one read through an
// accessor, in the accessor's position, whichever came first. Two accessors or
// two declarations keep the first, as before.
func TestDeduplicateParametersPrefersDeclared(t *testing.T) {
	accessor := Parameter{Name: "page", In: "query", Schema: &Schema{Type: "string"}}
	declared := Parameter{Name: "page", In: "query", Required: true, Schema: &Schema{Type: "integer"}, declared: true}
	other := Parameter{Name: "sort", In: "query", Schema: &Schema{Type: "string"}}

	for _, order := range [][]Parameter{
		{accessor, other, declared},
		{declared, other, accessor},
	} {
		got := deduplicateParameters(order)
		if len(got) != 2 || got[1].Name != "sort" {
			t.Fatalf("got %+v, want page then sort", got)
		}
		if got[0].Schema.Type != "integer" || !got[0].Required {
			t.Errorf("order %s,%s: page = %+v, want the declared integer",
				order[0].Schema.Type, order[2].Schema.Type, got[0])
		}
	}

	second := Parameter{Name: "page", In: "query", Schema: &Schema{Type: "number"}}
	if got := deduplicateParameters([]Parameter{accessor, second}); got[0].Schema.Type != "string" {
		t.Errorf("two accessors: kept %q, want the first", got[0].Schema.Type)
	}
}
