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
	"reflect"
	"slices"
	"testing"

	"github.com/ehabterra/apispec/internal/metadata"
)

// enumMeta builds one package declaring several string-underlying types, each with
// its own constants — the shape that made every type a candidate for every other
// (issue #229).
func enumMeta() *metadata.Metadata {
	sp := metadata.NewStringPool()
	constant := func(name, declaredType, value string, group int) *metadata.Variable {
		return &metadata.Variable{
			Name:          sp.Get(name),
			Type:          sp.Get(declaredType),
			ResolvedType:  sp.Get("string"),
			Tok:           sp.Get("const"),
			Value:         sp.Get(value),
			ComputedValue: value,
			GroupIndex:    group,
		}
	}
	return &metadata.Metadata{
		StringPool: sp,
		Packages: map[string]*metadata.Package{
			"app": {Files: map[string]*metadata.File{
				// Two files, so the walk cannot rely on one map's order.
				"status.go": {
					Types: map[string]*metadata.Type{"Status": {Name: sp.Get("Status"), Kind: sp.Get("string")}},
					Variables: map[string]*metadata.Variable{
						"StatusActive":  constant("StatusActive", "Status", "active", 1),
						"StatusRetired": constant("StatusRetired", "Status", "retired", 1),
					},
				},
				"format.go": {
					Types: map[string]*metadata.Type{"ApiFormat": {Name: sp.Get("ApiFormat"), Kind: sp.Get("string")}},
					Variables: map[string]*metadata.Variable{
						// A larger group, which is what used to win.
						"FormatUrl":    constant("FormatUrl", "ApiFormat", "url", 2),
						"FormatImages": constant("FormatImages", "ApiFormat", "images", 2),
						"FormatVision": constant("FormatVision", "ApiFormat", "vision", 2),
					},
				},
			}},
		},
	}
}

// TestDetectEnumUsesTypeIdentity pins the rule: constants belong to the type they
// are declared with. `Status` and `ApiFormat` are both `string` underneath, and
// that used to be enough for the bigger group to be documented as Status's enum —
// so a type's own values never appeared and the answer changed with map order.
func TestDetectEnumUsesTypeIdentity(t *testing.T) {
	meta := enumMeta()

	if got, want := detectEnumFromConstants("Status", "app", meta), []interface{}{"active", "retired"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Status enum = %v, want %v — its own constants, not the larger group's", got, want)
	}
	if got, want := detectEnumFromConstants("ApiFormat", "app", meta), []interface{}{"images", "url", "vision"}; !reflect.DeepEqual(got, want) {
		t.Errorf("ApiFormat enum = %v, want %v", got, want)
	}
	// A type with no constants gets no enum, rather than the nearest group's.
	if got := detectEnumFromConstants("Role", "app", meta); len(got) != 0 {
		t.Errorf("Role enum = %v, want none", got)
	}

	// Repeated calls agree: the choice must not depend on map iteration order.
	first := detectEnumFromConstants("Status", "app", meta)
	for i := 0; i < 20; i++ {
		if got := detectEnumFromConstants("Status", "app", meta); !reflect.DeepEqual(got, first) {
			t.Fatalf("call %d returned %v, want %v — the answer depends on map order", i, got, first)
		}
	}
}

// TestTypeMatchesRejectsSharedUnderlyingType covers the predicate change on its
// own: an alias NAME for the target still matches (a constant declared `Status`
// belongs to a field whose type resolves to `Status`), while a merely shared
// underlying type does not.
func TestTypeMatchesRejectsSharedUnderlyingType(t *testing.T) {
	meta := enumMeta()

	if typeMatches("Status", "ApiFormat", meta) {
		t.Error("two distinct types sharing `string` matched — their constants are not interchangeable")
	}
	if typeMatches("ApiFormat", "Status", meta) {
		t.Error("the same mistake in the other direction")
	}
	if !typeMatches("Status", "Status", meta) {
		t.Error("a type no longer matches itself")
	}
}

// splitBlockMeta declares ONE type whose constants sit in three `const (...)`
// blocks of sizes 3, 2 and 3 — the ordinary way an enum with more than a handful
// of members is written, grouped by meaning under its own comment.
//
// Two blocks are tied for largest on purpose: that is the state a real 32-value
// type was in when two constants were appended to the tied block, which silently
// replaced the whole documented enum with the other block's values.
func splitBlockMeta() *metadata.Metadata {
	sp := metadata.NewStringPool()
	constant := func(name, declaredType, value string, group int) *metadata.Variable {
		return &metadata.Variable{
			Name:          sp.Get(name),
			Type:          sp.Get(declaredType),
			ResolvedType:  sp.Get("string"),
			Tok:           sp.Get("const"),
			Value:         sp.Get(value),
			ComputedValue: value,
			GroupIndex:    group,
		}
	}
	return &metadata.Metadata{
		StringPool: sp,
		Packages: map[string]*metadata.Package{
			"app": {Files: map[string]*metadata.File{
				"reason.go": {
					Types: map[string]*metadata.Type{"Reason": {Name: sp.Get("Reason"), Kind: sp.Get("string")}},
					Variables: map[string]*metadata.Variable{
						// Block 1 — 3 values.
						"ReasonLessonCompleted": constant("ReasonLessonCompleted", "Reason", "lesson_completed", 1),
						"ReasonHWCompleted":     constant("ReasonHWCompleted", "Reason", "hw_completed", 1),
						"ReasonQuizAnnounced":   constant("ReasonQuizAnnounced", "Reason", "quiz_announced", 1),
						// Block 2 — 2 values, the smallest.
						"ReasonWithinTarget": constant("ReasonWithinTarget", "Reason", "within_target", 2),
						"ReasonOverMaximum":  constant("ReasonOverMaximum", "Reason", "over_maximum", 2),
						// Block 3 — 3 values, tied with block 1.
						"ReasonDeadlinePassed": constant("ReasonDeadlinePassed", "Reason", "deadline_passed", 3),
						"ReasonQuizBoundary":   constant("ReasonQuizBoundary", "Reason", "quiz_boundary", 3),
						"ReasonUnenrolled":     constant("ReasonUnenrolled", "Reason", "unenrolled", 3),
					},
				},
			}},
		},
	}
}

// TestDetectEnumUnionsEveryConstBlock pins that a `const (...)` block is source
// arrangement, not membership.
//
// Detection used to document the largest block and silently drop the others, so
// a type's enum was a subset chosen by how its source happened to be laid out.
// Every constant here declares Reason, which is a fact about each one; nothing
// in the source says a block is authoritative, because none of them is.
func TestDetectEnumUnionsEveryConstBlock(t *testing.T) {
	meta := splitBlockMeta()

	want := []interface{}{
		"deadline_passed", "hw_completed", "lesson_completed", "over_maximum",
		"quiz_announced", "quiz_boundary", "unenrolled", "within_target",
	}
	got := detectEnumFromConstants("Reason", "app", meta)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Reason enum =\n  %v\nwant all 8 constants\n  %v", got, want)
	}
}

// TestDetectEnumIsStableWhenABlockGrows is the regression guard for how this was
// FOUND: as unexplained spec drift.
//
// With the largest block winning and ties going to the earliest, appending a
// constant to a tied block did not add a value — it swapped the entire enum for
// a different block's. The values already documented must never depend on the
// relative sizes of the blocks.
func TestDetectEnumIsStableWhenABlockGrows(t *testing.T) {
	meta := splitBlockMeta()
	before := detectEnumFromConstants("Reason", "app", meta)

	// Append two constants to block 3, breaking the tie it had with block 1.
	sp := meta.StringPool
	file := meta.Packages["app"].Files["reason.go"]
	for name, value := range map[string]string{
		"ReasonReenrolled":      "reenrolled",
		"ReasonMidtermBoundary": "midterm_boundary",
	} {
		file.Variables[name] = &metadata.Variable{
			Name: sp.Get(name), Type: sp.Get("Reason"), ResolvedType: sp.Get("string"),
			Tok: sp.Get("const"), Value: sp.Get(value), ComputedValue: value, GroupIndex: 3,
		}
	}

	after := detectEnumFromConstants("Reason", "app", meta)
	if len(after) != len(before)+2 {
		t.Fatalf("after adding 2 constants the enum went from %d to %d values:\n  %v\n"+
			"growing one block must ADD to the enum, never replace it", len(before), len(after), after)
	}
	for _, v := range before {
		if !slices.Contains(after, v) {
			t.Errorf("%v was documented before a constant was added elsewhere and is gone now", v)
		}
	}
}

// TestExtractEnumValuesDeduplicates keeps the union from producing an invalid
// schema: OpenAPI requires enum members to be unique, and two constants of one
// type may legitimately share a value (a renamed member kept as a deprecated
// alias) — which unioning the blocks now brings together.
func TestExtractEnumValuesDeduplicates(t *testing.T) {
	got := extractEnumValues([]EnumConstant{
		{Name: "A", Value: "active"},
		{Name: "ALegacy", Value: "active"},
		{Name: "R", Value: "retired"},
	})
	if want := []interface{}{"active", "retired"}; !reflect.DeepEqual(got, want) {
		t.Errorf("enum values = %v, want %v — duplicates make the schema invalid", got, want)
	}
}
