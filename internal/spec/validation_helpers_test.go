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

// TestValidatorRules covers how a field's validator rules are gathered from
// `validate` and the framework's own keys (issue #600): each read as a whole
// key, and each key's `dive` kept apart so one key's element rules cannot
// absorb another key's container rules.
func TestValidatorRules(t *testing.T) {
	cases := []struct {
		name, tag string
		keys      []string
		want      string
		found     bool
	}{
		{name: "validate", tag: `json:"x" validate:"required,min=3"`, want: "required,min=3", found: true},
		{name: "absent", tag: `json:"x"`},
		{name: "unterminated", tag: `validate:"unterminated`},
		// Without the framework's key, gin's tag is not a validator tag.
		{name: "binding unconfigured", tag: `binding:"required"`},
		{name: "binding", tag: `binding:"required,min=3"`, keys: []string{"binding"}, want: "required,min=3", found: true},
		// A whole key, never a substring of another.
		{name: "key inside another", tag: `xvalidate:"required"`},
		{name: "both keys", tag: `validate:"email" binding:"required"`, keys: []string{"binding"}, want: "email,required", found: true},
		// `validate` listed again is not read twice.
		{name: "repeated key", tag: `validate:"required"`, keys: []string{"validate", ""}, want: "required", found: true},
		{
			name: "dive per key", tag: `validate:"dive,max=10" binding:"required,min=1,dive,min=2"`,
			keys: []string{"binding"}, want: "required,min=1,dive,max=10,min=2", found: true,
		},
		{name: "empty value", tag: `validate:""`, found: true},
	}
	for _, c := range cases {
		got, found := validatorRules(c.tag, c.keys)
		if got != c.want || found != c.found {
			t.Errorf("%s: validatorRules(%q, %v) = %q, %v; want %q, %v", c.name, c.tag, c.keys, got, found, c.want, c.found)
		}
	}
}

// TestExtractValidationConstraintsFrameworkKey: gin's `binding` rules set
// `required` and constraints only when the framework names the key, and a
// `dive` under one key leaves the other key's container rules on the container.
func TestExtractValidationConstraintsFrameworkKey(t *testing.T) {
	if c := extractValidationConstraints(`binding:"required,min=3"`); c != nil {
		t.Errorf("binding read with no framework key: %+v", c)
	}
	c := extractValidationConstraints(`binding:"required,min=3"`, "binding")
	if c == nil || !c.Required || c.Min == nil || *c.Min != 3 {
		t.Fatalf("binding:\"required,min=3\" = %+v, want required with min 3", c)
	}
	c = extractValidationConstraints(`validate:"dive,max=10" binding:"required"`, "binding")
	if c == nil || !c.Required {
		t.Fatalf("binding's `required` was lost behind validate's dive: %+v", c)
	}
	if c.Dive == nil || c.Dive.Max == nil || *c.Dive.Max != 10 || c.Dive.Required {
		t.Errorf("element rules = %+v, want max 10 and not required", c.Dive)
	}
	if c.Max != nil {
		t.Errorf("an element rule reached the container: max = %v", *c.Max)
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
		// gin's key, read because the framework names it (issue #600).
		`binding:"gtefield=Min"`: "Struct-level validation: gtefield=Min",
	}
	for tag, want := range cases {
		if got := structLevelValidationNote(tag, "binding"); got != want {
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

// The validate key's value ends at its closing quote. Reading on to the end of
// the tag took every later key and value as rules, so a field whose json NAME
// was "date", "uuid" or "url" gained that rule's pattern or format (CodeRabbit
// on #601).
func TestExtractValidationConstraints_StopsAtOwnValue(t *testing.T) {
	for _, tag := range []string{
		`form:"q" validate:"required" json:"date"`,
		`validate:"required" json:"uuid" example:"url"`,
	} {
		c := extractValidationConstraints(tag)
		if c == nil || !c.Required {
			t.Errorf("%s: lost its own `required` rule: %+v", tag, c)
			continue
		}
		if c.Pattern != "" || c.Format != "" {
			t.Errorf("%s: a later key's text became a rule: pattern=%q format=%q", tag, c.Pattern, c.Format)
		}
	}

	// A tag with no validate key states no validate rules, whatever its other
	// values spell.
	if c := extractValidationConstraints(`json:"email"`); c != nil && (c.Format != "" || c.Required) {
		t.Errorf("no validate key, yet got %+v", c)
	}
}

// The custom constraint keys are keys, not substrings: another tag's VALUE
// that happens to spell `min:5` is not a constraint, and a key's value is read
// whole, spaces included (CodeRabbit on #601).
func TestExtractValidationConstraints_CustomKeysAreKeys(t *testing.T) {
	if c := extractValidationConstraints(`json:"n" example:"min:5 max:9 regexp:x enum:a,b"`); c != nil {
		t.Errorf("another tag's value became constraints: %+v", c)
	}

	c := extractValidationConstraints(`json:"n" min:"1" max:"10" regexp:"^a b$" enum:"x, y"`)
	if c == nil {
		t.Fatal("own keys produced nothing")
	}
	if c.Min == nil || *c.Min != 1 || c.Max == nil || *c.Max != 10 {
		t.Errorf("min/max = %v/%v, want 1/10", c.Min, c.Max)
	}
	if c.Pattern != "^a b$" {
		t.Errorf("pattern = %q, want the whole value %q", c.Pattern, "^a b$")
	}
	if len(c.Enum) != 2 || c.Enum[0] != "x" || c.Enum[1] != "y" {
		t.Errorf("enum = %v, want [x y]", c.Enum)
	}
}
