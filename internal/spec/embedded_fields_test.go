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
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	"github.com/ehabterra/apispec/internal/metadata"
)

// embedMeta builds metadata from Go source, so the cases are written as the Go
// they describe rather than as hand-assembled records.
func embedMeta(t *testing.T, src string) *metadata.Metadata {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "p.go", "package p\n"+src, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return metadata.GenerateMetadata(
		map[string]map[string]*ast.File{"p": {"p.go": file}},
		nil, map[string]string{"p": "p"}, fset,
	)
}

func fieldNames(meta *metadata.Metadata, typeName string) []string {
	typ := findType(meta, "p", typeName)
	if typ == nil {
		return nil
	}
	var out []string
	for _, ef := range effectiveJSONFields(meta, typ) {
		out = append(out, jsonFieldName(meta, ef.field))
	}
	return out
}

func equalNames(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

// Each expectation is the property SET encoding/json emits for the same type,
// checked by running the encoder.
//
// The ORDER here is fields-then-embeds, which is not always Go's declaration
// order — `Tagged{Named; Extra}` serialises `named` first while this yields
// `Extra` first. Metadata records Fields and Embeds as two lists with no record
// of how they interleaved, so true declaration order is not recoverable without
// another metadata fact. It reaches the document only through the `required`
// array, whose order carries no meaning, and `properties` is a map — which is
// why all 123 fixtures are byte-identical.
func TestEffectiveJSONFields(t *testing.T) {
	meta := embedMeta(t, `
type Base struct {
	ID   int
	Kind string `+"`json:\"kind\"`"+`
}
type Named struct{ Label string }
type Meta struct{ Page int }
type Ref string
type Hidden struct{ Secret string }

type Plain struct {
	Base
	Extra string
}
type Tagged struct {
	Named `+"`json:\"named\"`"+`
	Extra string
}
type Pointer struct {
	*Meta
	Extra string
}
type NonStruct struct {
	Ref
	Extra string
}
type Dropped struct {
	Hidden `+"`json:\"-\"`"+`
	Extra  string
}
type Shadowed struct {
	Base
	Kind string `+"`json:\"kind\"`"+`
}
type Unexported struct {
	Base
	hidden string
	Extra  string
}
`)

	cases := []struct {
		typeName string
		want     []string
		why      string
	}{
		{"Plain", []string{"Extra", "ID", "kind"}, "an untagged embed promotes, carrying its own tags"},
		{"Tagged", []string{"Extra", "named"}, "a tagged embed nests instead of promoting"},
		{"Pointer", []string{"Extra", "Page"}, "a pointer embed promotes exactly as a value does"},
		{"NonStruct", []string{"Extra", "Ref"}, "an embedded non-struct is one field named for the type"},
		{"Dropped", []string{"Extra"}, "`json:\"-\"` on an embed drops it whole"},
		{"Shadowed", []string{"kind", "ID"}, "the shallower field wins a name collision"},
		{"Unexported", []string{"Extra", "ID", "kind"}, "an unexported field never serialises"},
	}
	for _, tc := range cases {
		t.Run(tc.typeName, func(t *testing.T) {
			got := fieldNames(meta, tc.typeName)
			if !equalNames(got, tc.want) {
				t.Errorf("%s = %v, want %v — %s", tc.typeName, got, tc.want, tc.why)
			}
		})
	}
}

// Go drops a name entirely when two fields at the SAME depth claim it and
// neither is tagged, rather than picking one. Choosing would document a field
// the encoder does not send.
func TestEqualDepthCollision(t *testing.T) {
	meta := embedMeta(t, `
type Left struct{ Dup string }
type Right struct{ Dup string }
type Ambiguous struct {
	Left
	Right
	Extra string
}

type TaggedLeft struct {
	Dup string `+"`json:\"dup\"`"+`
}
type PlainRight struct{ Dup string }
type Resolved struct {
	TaggedLeft
	PlainRight
	Extra string
}
`)

	got := fieldNames(meta, "Ambiguous")
	for _, name := range got {
		if name == "Dup" {
			t.Errorf("Ambiguous = %v — two untagged fields at the same depth claim `Dup`, "+
				"so encoding/json sends neither", got)
		}
	}
	if !equalNames(got, []string{"Extra"}) {
		t.Errorf("Ambiguous = %v, want only [Extra]", got)
	}

	// A tag RENAMES rather than competing: `dup` and `Dup` are different names,
	// so there is no collision and encoding/json sends both —
	// {"dup":"","Dup":"","Extra":""}, checked against the encoder.
	if got := fieldNames(meta, "Resolved"); !equalNames(got, []string{"Extra", "dup", "Dup"}) {
		t.Errorf("Resolved = %v, want [Extra dup Dup] — a tag renames the field, so these two "+
			"never collide", got)
	}
}

// A struct with no embeds resolves to its own fields, unchanged: this path runs
// for every type in every project, so it must not disturb the ordinary case.
func TestNoEmbedsIsUnchanged(t *testing.T) {
	meta := embedMeta(t, `
type Plain struct {
	A string
	B string `+"`json:\"b\"`"+`
	c string
	D string `+"`json:\"-\"`"+`
}
`)
	if got := fieldNames(meta, "Plain"); !equalNames(got, []string{"A", "b"}) {
		t.Errorf("Plain = %v, want [A b]", got)
	}

	// Degenerate inputs do not panic.
	if got := effectiveJSONFields(nil, nil); got != nil {
		t.Errorf("nil metadata produced %v", got)
	}
	if got := effectiveJSONFields(meta, nil); got != nil {
		t.Errorf("a nil type produced %v", got)
	}
}

// An embed whose declaration is not visible — an external type — contributes
// NOTHING, because encoding/json promotes its exported fields and we cannot see
// them. Inventing a field named for the type would be wrong for the common
// case: `url.Userinfo` has no exported fields at all (golden rule #7).
func TestInvisibleEmbedContributesNothing(t *testing.T) {
	meta := embedMeta(t, `
type Outer struct {
	somepkg.External
	Extra string
}
`)
	got := fieldNames(meta, "Outer")
	for _, name := range got {
		if name == "External" {
			t.Errorf("Outer = %v — an embed we cannot see through must not become a property "+
				"named for its type", got)
		}
	}
}

func TestShallowestFieldEdges(t *testing.T) {
	if _, ok := shallowestField(nil); ok {
		t.Error("no candidates resolved to a winner")
	}
	one := effectiveField{depth: 3}
	if got, ok := shallowestField([]effectiveField{one}); !ok || got.depth != 3 {
		t.Error("a single candidate at any depth is the winner")
	}
	// Two tagged at equal depth is still ambiguous: a tag breaks a tie only
	// when exactly one field has it.
	both := []effectiveField{{depth: 0, tagged: true}, {depth: 0, tagged: true}}
	if _, ok := shallowestField(both); ok {
		t.Error("two tagged fields at the same depth are ambiguous, not resolved")
	}
}

func TestEmbeddedFieldNameAndType(t *testing.T) {
	for in, want := range map[string]string{
		"Base":           "Base",
		"*Meta":          "Meta",
		"pkg.External":   "External",
		"a/b/pkg.Nested": "Nested",
		"":               "",
	} {
		if got := embeddedFieldName(in); got != want {
			t.Errorf("embeddedFieldName(%q) = %q, want %q", in, got, want)
		}
	}

	meta := embedMeta(t, "type T struct{ A string }\ntype S string\n")
	if embeddedType(meta, "", "p") != nil {
		t.Error("an empty embed name resolved to a type")
	}
	if embeddedType(meta, "T", "p") == nil {
		t.Error("an unqualified embed did not resolve against the embedding package")
	}
	if !isStructType(meta, findType(meta, "p", "T")) {
		t.Error("T is a struct")
	}
	if isStructType(meta, findType(meta, "p", "S")) {
		t.Error("S is a string, not a struct")
	}
	if isStructType(meta, nil) {
		t.Error("a nil type is not a struct")
	}
}
