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

import "testing"

// TestValidateTagValue covers the struct-tag extractor: present, absent, and a
// malformed (unterminated) validate tag.
func TestValidateTagValue(t *testing.T) {
	cases := map[string]string{
		`json:"x" validate:"required,min=3"`: "required,min=3",
		`json:"x"`:                           "",
		`validate:"gtefield=Min" json:"y"`:   "gtefield=Min",
		`validate:"unterminated`:             "",
	}
	for tag, want := range cases {
		if got := validateTagValue(tag); got != want {
			t.Errorf("validateTagValue(%q) = %q, want %q", tag, got, want)
		}
	}
}

// TestStructLevelValidationNote covers #166's note builder, including the
// empty/"-"/absent residues.
func TestStructLevelValidationNote(t *testing.T) {
	cases := map[string]string{
		`validate:"gtefield=Min"`: "Struct-level validation: gtefield=Min",
		`validate:"-"`:            "",
		`validate:""`:             "",
		`json:"x"`:                "",
	}
	for tag, want := range cases {
		if got := structLevelValidationNote(tag); got != want {
			t.Errorf("structLevelValidationNote(%q) = %q, want %q", tag, got, want)
		}
	}
}

// TestAppendConstraintNote covers both the empty-description and
// existing-description branches.
func TestAppendConstraintNote(t *testing.T) {
	if got := appendConstraintNote("", "note"); got != "note" {
		t.Errorf("append to empty: got %q", got)
	}
	if got := appendConstraintNote("desc", "note"); got != "desc\nnote" {
		t.Errorf("append to existing: got %q", got)
	}
}

// TestApplyValidationConstraints_ByType pins that min/max route by schema type:
// length for strings, item-count for arrays (with post-dive on items), and
// value for numbers.
func TestApplyValidationConstraints_ByType(t *testing.T) {
	f := func(v float64) *float64 { return &v }

	str := &Schema{Type: "string"}
	applyValidationConstraints(str, &ValidationConstraints{Min: f(3), Max: f(50)})
	if str.MinLength != 3 || str.MaxLength != 50 {
		t.Errorf("string: got minLength=%d maxLength=%d, want 3/50", str.MinLength, str.MaxLength)
	}

	num := &Schema{Type: "integer"}
	applyValidationConstraints(num, &ValidationConstraints{Min: f(18), Max: f(120)})
	if num.Minimum != 18 || num.Maximum != 120 {
		t.Errorf("integer: got minimum=%v maximum=%v, want 18/120", num.Minimum, num.Maximum)
	}

	arr := &Schema{Type: "array", Items: &Schema{Type: "integer"}}
	applyValidationConstraints(arr, &ValidationConstraints{
		Min: f(1), Max: f(10),
		Dive: &ValidationConstraints{Min: f(5), Max: f(100)},
	})
	if arr.MinItems != 1 || arr.MaxItems != 10 {
		t.Errorf("array: got minItems=%d maxItems=%d, want 1/10", arr.MinItems, arr.MaxItems)
	}
	if arr.Items.Minimum != 5 || arr.Items.Maximum != 100 {
		t.Errorf("array items: got minimum=%v maximum=%v, want 5/100", arr.Items.Minimum, arr.Items.Maximum)
	}
}
