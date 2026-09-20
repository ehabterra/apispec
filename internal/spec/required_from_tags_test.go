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

// TestJSONTagOmitsEmpty pins the tag reading the `required` rule turns on.
// Getting it wrong in the permissive direction claims a field is always sent
// when the encoder may drop it, which is the one failure that makes a client
// trust something false.
func TestJSONTagOmitsEmpty(t *testing.T) {
	cases := []struct {
		name, tag string
		want      bool
	}{
		{"no tag at all", "", false},
		{"a name only", `json:"id"`, false},
		{"omitempty", `json:"id,omitempty"`, true},
		{"omitzero", `json:"id,omitzero"`, true},
		{"both", `json:"id,omitempty,omitzero"`, true},
		{"an unrelated option", `json:"id,string"`, false},
		{"omitempty after another option", `json:"id,string,omitempty"`, true},
		{"no name, just the option", `json:",omitempty"`, true},
		// A field of another tag must not be read as json's.
		{"omitempty in a different tag", `yaml:"id,omitempty" json:"id"`, false},
		{"a validate tag alongside", `json:"id" validate:"required"`, false},
		// Not an option list at all.
		{"a bare non-json tag", `xml:"id"`, false},
		// A key ENDING in json is not the json key. encoding/json ignores it
		// and writes the field, so reading it as omitempty left an
		// always-present field out of `required`.
		{"a key ending in json", `myjson:"id,omitempty"`, false},
		{"that key beside a real one", `myjson:"id,omitempty" json:"id"`, false},
		{"a real one beside that key", `myjson:"id" json:"id,omitempty"`, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := jsonTagOmitsEmpty(tc.tag); got != tc.want {
				t.Errorf("jsonTagOmitsEmpty(%q) = %v, want %v", tc.tag, got, tc.want)
			}
		})
	}
}

// TestRequiredFromTagsGuards covers the three answers that are "no" for a
// reason other than the tag.
func TestRequiredFromTagsGuards(t *testing.T) {
	on := &APISpecConfig{Schema: SchemaConfig{RequiredFromJSONTags: true}}

	t.Run("off by default", func(t *testing.T) {
		if requiredFromTags(&APISpecConfig{}, effectiveField{}, `json:"id"`) {
			t.Error("a field was required with the option off")
		}
		if requiredFromTags(nil, effectiveField{}, `json:"id"`) {
			t.Error("a nil config required a field")
		}
	})

	t.Run("promoted through an embedded pointer", func(t *testing.T) {
		// Gone entirely when the pointer is nil, whatever its own tag says.
		if requiredFromTags(on, effectiveField{viaPointer: true}, `json:"trace"`) {
			t.Error("a field promoted through an embedded pointer was required")
		}
		if !requiredFromTags(on, effectiveField{}, `json:"trace"`) {
			t.Error("the same field NOT behind a pointer should be required")
		}
	})
}

// TestJSONTagOptions covers the shapes that carry no option list, so the
// reader above cannot mistake a malformed tag for one that omits.
func TestJSONTagOptions(t *testing.T) {
	for _, tc := range []struct {
		name, tag string
		want      int
	}{
		{"no json key", `yaml:"id,omitempty"`, 0},
		{"a name with no comma", `json:"id"`, 0},
		{"the key with nothing after it", `json:`, 0},
		{"one option", `json:"id,omitempty"`, 1},
		{"two options", `json:"id,omitempty,string"`, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := len(jsonTagOptions(tc.tag)); got != tc.want {
				t.Errorf("jsonTagOptions(%q) = %d options, want %d", tc.tag, got, tc.want)
			}
		})
	}
}
