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

package metadata

import "testing"

// TestIsEntryFunc pins which functions run with no caller (issue #580): main,
// and a package init — but not a METHOD named init, which is called like any
// other function and gets its assignments recorded through that call.
func TestIsEntryFunc(t *testing.T) {
	cases := []struct {
		name, recv string
		want       bool
	}{
		{"main", "", true},
		{"init", "", true},
		{"init", "*Server", false},
		{"setup", "", false},
		{"Init", "", false},
	}
	for _, tc := range cases {
		if got := isEntryFunc(tc.name, tc.recv); got != tc.want {
			t.Errorf("isEntryFunc(%q, %q) = %v, want %v", tc.name, tc.recv, got, tc.want)
		}
	}
}
