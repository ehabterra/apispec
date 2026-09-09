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

// The rule is deliberately narrower than Go's own grammar, and the cases that
// must NOT be split are the point: a path reaching this function has already
// been through resolution and can have lost its leading slash, so treating any
// dotless prefix as a host (which is what Go itself does) would delete a real
// segment.
func TestSplitHostFromPath(t *testing.T) {
	cases := []struct {
		in, host, path string
	}{
		// Hosts.
		{"api.example.com/items", "api.example.com", "/items"},
		{"api.example.com/", "api.example.com", "/"},
		{"localhost:8080/debug", "localhost:8080", "/debug"},
		{"api.example.com:8443/items/{id}", "api.example.com:8443", "/items/{id}"},
		{"sub.api.example.com/x", "sub.api.example.com", "/x"},
		{"api-1.example.com/x", "api-1.example.com", "/x"},

		// Not hosts.
		{"/items", "", "/items"},                   // ordinary path
		{"/", "", "/"},                             // root
		{"", "", ""},                               // nothing
		{"v1/users", "", "v1/users"},               // a path that lost its leading slash
		{"items", "", "items"},                     // no slash at all: not a pattern
		{"{tenant}/items", "", "{tenant}/items"},   // an unresolved placeholder, not a host
		{"api.example.com", "", "api.example.com"}, // host with no path: no "/" to split on
	}
	for _, tc := range cases {
		host, path := splitHostFromPath(tc.in)
		if host != tc.host || path != tc.path {
			t.Errorf("splitHostFromPath(%q) = (%q, %q), want (%q, %q)", tc.in, host, path, tc.host, tc.path)
		}
	}
}
