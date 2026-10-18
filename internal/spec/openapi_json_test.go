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
	"encoding/json"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestParameterExtensionsInlineInJSON pins that a parameter's extensions are
// sibling keys in JSON, as they already are in YAML, and never a nested
// "Extensions" object — encoding/json has no `,inline` (issue #589).
func TestParameterExtensionsInlineInJSON(t *testing.T) {
	cases := []struct {
		name  string
		param Parameter
		want  string
	}{
		{
			name:  "no extensions",
			param: Parameter{Name: "id", In: "path", Required: true},
			want:  `{"name":"id","in":"path","required":true}`,
		},
		{
			name: "extensions follow the fields, sorted",
			param: Parameter{
				Name: "id", In: "path", Required: true,
				Extensions: map[string]any{"x-warning": "w", "x-a": 1},
			},
			want: `{"name":"id","in":"path","required":true,"x-a":1,"x-warning":"w"}`,
		},
		{
			name:  "extensions alone",
			param: Parameter{Extensions: map[string]any{"x-warning": "w"}},
			want:  `{"x-warning":"w"}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := json.Marshal(tc.param)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tc.want {
				t.Errorf("json = %s\nwant   %s", got, tc.want)
			}

			// Parity with YAML: both encoders must yield the same object.
			y, err := yaml.Marshal(tc.param)
			if err != nil {
				t.Fatal(err)
			}
			var fromYAML, fromJSON map[string]any
			if err := yaml.Unmarshal(y, &fromYAML); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(got, &fromJSON); err != nil {
				t.Fatal(err)
			}
			if len(fromYAML) != len(fromJSON) {
				t.Errorf("yaml keys %v != json keys %v", fromYAML, fromJSON)
			}
			for k := range fromYAML {
				if _, ok := fromJSON[k]; !ok {
					t.Errorf("yaml has %q, json does not", k)
				}
			}
		})
	}

	// Through a pointer and inside a slice, as operations hold them.
	got, err := json.Marshal([]*Parameter{{Name: "q", Extensions: map[string]any{"x-warning": "w"}}})
	if err != nil {
		t.Fatal(err)
	}
	if want := `[{"name":"q","x-warning":"w"}]`; string(got) != want {
		t.Errorf("json = %s, want %s", got, want)
	}
}
