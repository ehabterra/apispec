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

// One parameter cannot be both the status and the body of a response, and a
// derived pattern that says so has followed a local back to the wrong source
// (issue #485).
func TestSameAsStatusParam(t *testing.T) {
	cases := []struct {
		name    string
		pattern *ResponsePattern
		typeIdx int
		want    bool
	}{
		{
			name:    "the body would come from the status parameter",
			pattern: &ResponsePattern{StatusFromArg: true, StatusArgIndex: 0},
			typeIdx: 0,
			want:    true,
		},
		{
			name:    "a different parameter is fine",
			pattern: &ResponsePattern{StatusFromArg: true, StatusArgIndex: 0},
			typeIdx: 1,
			want:    false,
		},
		{
			// Index 0 is the default for an unset field, so a pattern that does
			// NOT read its status from an argument must not veto a body at
			// index 0 — that is where most single-argument responders put it.
			name:    "no status argument, so nothing to collide with",
			pattern: &ResponsePattern{StatusFromArg: false, StatusArgIndex: 0},
			typeIdx: 0,
			want:    false,
		},
		{
			name:    "a nil pattern vetoes nothing",
			pattern: nil,
			typeIdx: 0,
			want:    false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sameAsStatusParam(tc.pattern, tc.typeIdx); got != tc.want {
				t.Errorf("sameAsStatusParam = %v, want %v", got, tc.want)
			}
		})
	}
}
