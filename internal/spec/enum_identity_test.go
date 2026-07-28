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
