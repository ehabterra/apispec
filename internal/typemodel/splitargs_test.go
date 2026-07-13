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

package typemodel

import (
	"reflect"
	"testing"
)

// TestSplitArgs pins top-level comma splitting.
func TestSplitArgs(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"", nil},
		{"K,V", []string{"K", "V"}},
		{"T any, U comparable", []string{"T any", "U comparable"}},
		{"Page[User], Box[Pair[K, V]]", []string{"Page[User]", "Box[Pair[K, V]]"}},
		// A function-type argument keeps its parameter commas.
		{"func(int, string), User", []string{"func(int, string)", "User"}},
		{"  User  ", []string{"User"}},
	}
	for _, tt := range tests {
		if got := splitArgs(tt.input); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("splitArgs(%q) = %#v, want %#v", tt.input, got, tt.want)
		}
	}
}
