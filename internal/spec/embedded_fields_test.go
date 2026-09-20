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

// Each expectation is what encoding/json emits for the same type — the property
// set AND its order, both captured by running the encoder.
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
		{"Plain", []string{"ID", "kind", "Extra"}, "an untagged embed promotes, carrying its own tags"},
		{"Tagged", []string{"named", "Extra"}, "a tagged embed nests instead of promoting"},
		{"Pointer", []string{"Page", "Extra"}, "a pointer embed promotes exactly as a value does"},
		{"NonStruct", []string{"Ref", "Extra"}, "an embedded non-struct is one field named for the type"},
		{"Dropped", []string{"Extra"}, "`json:\"-\"` on an embed drops it whole"},
		{"Shadowed", []string{"ID", "kind"}, "the shallower field wins a name collision"},
		{"Unexported", []string{"ID", "kind", "Extra"}, "an unexported field never serialises"},
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
	if got := fieldNames(meta, "Resolved"); !equalNames(got, []string{"dup", "Dup", "Extra"}) {
		t.Errorf("Resolved = %v, want [dup Dup Extra] — a tag renames the field, so these two "+
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

// Field ORDER is part of what a struct says, and metadata keeps named fields
// and embeds in two lists — so the position each embed was declared at has to
// travel with it, or every promoted field lands after every declared one
// whatever the source said (Type.EmbedAt, issue #487).
func TestDeclarationOrderIsPreserved(t *testing.T) {
	meta := embedMeta(t, `
type Base struct {
	ID   int
	Kind string `+"`json:\"kind\"`"+`
}
type Leading struct {
	Base
	Extra string
}
type Trailing struct {
	First string
	Base
	Last string
}
type Between struct {
	A string
	Base
	B string
}
`)
	// Each is what json.Marshal emits for the same type.
	for _, tc := range []struct {
		typeName string
		want     []string
	}{
		{"Leading", []string{"ID", "kind", "Extra"}},
		{"Trailing", []string{"First", "ID", "kind", "Last"}},
		{"Between", []string{"A", "ID", "kind", "B"}},
	} {
		if got := fieldNames(meta, tc.typeName); !equalNames(got, tc.want) {
			t.Errorf("%s = %v, want %v — the embed sits where it was declared, not at the end",
				tc.typeName, got, tc.want)
		}
	}
}

// Metadata written before EmbedAt existed has no positions to read, and must
// still resolve — with the embeds last, which is the best available answer
// rather than dropping them.
func TestMissingEmbedAtFallsBackToLast(t *testing.T) {
	meta := embedMeta(t, `
type Base struct{ ID int }
type Old struct {
	Base
	Extra string
}
`)
	typ := findType(meta, "p", "Old")
	if typ == nil {
		t.Fatal("Old not found")
	}
	typ.EmbedAt = nil // as a deserialised older metadata.yaml would be

	got := fieldNames(meta, "Old")
	if !equalNames(got, []string{"Extra", "ID"}) {
		t.Errorf("Old = %v, want [Extra ID] — with no recorded position the embed goes last, "+
			"but it must not go missing", got)
	}
}

// Two embeds reaching the SAME type are two candidates at equal depth, and
// encoding/json resolves that by sending neither. A visited set shared across
// the whole walk collapsed them into one and published a property the encoder
// drops (CodeRabbit on #488).
func TestDiamondEmbedDropsTheAmbiguousField(t *testing.T) {
	meta := embedMeta(t, `
type Base struct{ ID int }
type A struct{ Base }
type B struct{ Base }
type Outer struct {
	A
	B
	Extra string
}
`)
	// json.Marshal(Outer{}) = {"Extra":""}
	if got := fieldNames(meta, "Outer"); !equalNames(got, []string{"Extra"}) {
		t.Errorf("Outer = %v, want [Extra] — A and B both embed Base, so `ID` has two "+
			"equal-depth claimants and encoding/json sends neither", got)
	}
}

// A long chain of embeds is legal Go whose leaf fields encoding/json promotes,
// so the walk must follow it however deep. A fixed cap would have dropped them
// silently; termination comes from the per-path visited set instead.
func TestDeepEmbeddingHasNoCap(t *testing.T) {
	meta := embedMeta(t, `
type L1 struct{ Deep string }
type L2 struct{ L1 }
type L3 struct{ L2 }
type L4 struct{ L3 }
type L5 struct{ L4 }
type L6 struct{ L5 }
type L7 struct{ L6 }
type Deep8 struct{ L7 }
`)
	if got := fieldNames(meta, "Deep8"); !equalNames(got, []string{"Deep"}) {
		t.Errorf("Deep8 = %v, want [Deep] — eight levels of embedding is legal Go and "+
			"encoding/json promotes the leaf", got)
	}
}

// TestEffectiveFieldsCarryPointerEmbeds pins that a field promoted through an
// embedded POINTER is marked as such, at any depth.
//
// encoding/json writes nothing at all for a nil embedded pointer, so those
// fields can never be `required` however their own tags read — which is the
// one thing the tag alone cannot tell you (issue #516).
func TestEffectiveFieldsCarryPointerEmbeds(t *testing.T) {
	meta := embedMeta(t, `
type Leaf struct {
	Deep string `+"`json:\"deep\"`"+`
}
type Mid struct {
	Leaf
	Mids string `+"`json:\"mids\"`"+`
}
type Value struct {
	Own string `+"`json:\"own\"`"+`
}
type Outer struct {
	Value
	*Mid
	Direct string `+"`json:\"direct\"`"+`
}
`)
	typ := findType(meta, "p", "Outer")
	if typ == nil {
		t.Fatal("Outer not found")
	}

	viaPointer := map[string]bool{}
	for _, ef := range effectiveJSONFields(meta, typ) {
		viaPointer[jsonFieldName(meta, ef.field)] = ef.viaPointer
	}

	for name, want := range map[string]bool{
		"direct": false, // declared here
		"own":    false, // through a VALUE embed
		"mids":   true,  // through the pointer embed
		"deep":   true,  // and one level deeper still — the flag is sticky
	} {
		got, ok := viaPointer[name]
		if !ok {
			t.Errorf("%q is missing from the effective field set", name)
			continue
		}
		if got != want {
			t.Errorf("%q viaPointer = %v, want %v", name, got, want)
		}
	}
}

// TestEffectiveFieldsTaggedPointerEmbed pins the other half of the pointer
// rule: a TAGGED embed does not promote, it is an ordinary field carrying the
// embedded object — and when that embed is a pointer, the field itself is the
// one that can go missing, not the fields inside it.
func TestEffectiveFieldsTaggedPointerEmbed(t *testing.T) {
	meta := embedMeta(t, `
type Meta struct {
	Trace string `+"`json:\"trace\"`"+`
}
type Outer struct {
	*Meta  `+"`json:\"meta\"`"+`
	Direct string `+"`json:\"direct\"`"+`
}
`)
	typ := findType(meta, "p", "Outer")
	if typ == nil {
		t.Fatal("Outer not found")
	}

	got := map[string]bool{}
	for _, ef := range effectiveJSONFields(meta, typ) {
		got[jsonFieldName(meta, ef.field)] = ef.viaPointer
	}

	// The tagged embed is one field named "meta"; "trace" is inside its schema,
	// not promoted, so it is not in this set at all.
	if _, promoted := got["trace"]; promoted {
		t.Error("a TAGGED embed promoted its fields; it carries the object instead")
	}
	for _, name := range []string{"meta", "direct"} {
		if _, ok := got[name]; !ok {
			t.Errorf("%q is missing from the effective field set", name)
		}
	}
	// Both are declared on Outer itself, so neither arrived through a pointer.
	if got["meta"] || got["direct"] {
		t.Errorf("a field declared on Outer was marked as promoted through a pointer: %v", got)
	}
}
