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
	"slices"
	"testing"
)

// TestConvertPathCatchAll covers every catch-all spelling apispec's routers
// use. Only the chi ones reach a fixture; the rest are here because a whole
// project per router spelling would be a lot of scaffolding for one string.
func TestConvertPathCatchAll(t *testing.T) {
	cases := []struct {
		name     string
		in       string
		want     string
		wantName string
	}{
		{"chi trailing star", "/assets*", "/assets/{wildcard}", "wildcard"},
		{"chi and echo slash star", "/static/*", "/static/{wildcard}", "wildcard"},
		{"gin names the catch-all", "/files/*filepath", "/files/{filepath}", "filepath"},
		{"root catch-all", "/*", "/{wildcard}", "wildcard"},
		{"catch-all after a parameter", "/users/:id/files/*path", "/users/{id}/files/{path}", "path"},
		// gorilla/mux writes it as a pattern, which the constraint stripper
		// already turns into a plain placeholder — nothing to convert.
		{"mux regex catch-all is already a template", "/static/{rest:.*}", "/static/{rest}", ""},
		{"no catch-all", "/users/{id}", "/users/{id}", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, names := convertPathToOpenAPI(tc.in)
			if got != tc.want {
				t.Errorf("convertPathToOpenAPI(%q) = %q, want %q", tc.in, got, tc.want)
			}
			if tc.wantName == "" {
				if len(names) != 0 {
					t.Errorf("reported catch-all names %v for a path that has none", names)
				}
				return
			}
			if !slices.Contains(names, tc.wantName) {
				t.Errorf("catch-all names = %v, want to contain %q — the name is what tells the "+
					"parameter declaration to describe it rather than warn about it", names, tc.wantName)
			}
		})
	}
}

// TestConvertPathCatchAllNameCollision pins that a generated name never shadows
// a placeholder the route already declares: two parameters with one name would
// leave the real one undocumented.
func TestConvertPathCatchAllNameCollision(t *testing.T) {
	got, names := convertPathToOpenAPI("/files/{wildcard}/blobs/*")
	if got != "/files/{wildcard}/blobs/{wildcard2}" {
		t.Errorf("path = %q, want the catch-all named around the existing {wildcard}", got)
	}
	if !slices.Contains(names, "wildcard2") {
		t.Errorf("catch-all names = %v, want to contain wildcard2", names)
	}
}
