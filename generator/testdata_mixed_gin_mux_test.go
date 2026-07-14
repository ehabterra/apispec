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

package generator

import "testing"

// TestTestdata_MixedGinMux locks in cross-framework merging: a binary serving
// a gin API alongside a gorilla/mux admin router must document both surfaces.
// gin is detected first (primary); mux merges in as a receiver-scoped
// SecondaryView, so its HandleFunc/Methods chains and {id} path params are
// traced without its unscoped helpers bleeding into gin's routes. The
// fallback config is nil on purpose so the engine's own DetectAll + merge
// path is what runs.
func TestTestdata_MixedGinMux(t *testing.T) {
	out := loadTestdataWithFixtureConfig(t, "mixed_gin_mux", nil)
	noDanglingRefs(t, out)

	want := map[string][]string{
		"/products":         {"GET", "POST"}, // gin verb calls (primary)
		"/admin/report":     {"GET"},         // mux HandleFunc().Methods("GET")
		"/admin/users/{id}": {"DELETE"},      // mux path template + Methods("DELETE")
	}
	for path, methods := range want {
		item, ok := out.Paths[path]
		if !ok {
			t.Errorf("path %q missing; have %v", path, mapPathKeys(out.Paths))
			continue
		}
		for _, m := range methods {
			if opFor(item, m) == nil {
				t.Errorf("%s %s: expected operation, missing", m, path)
			}
		}
	}

	// The mux verb chain must win over any default: no POST on admin routes.
	for _, path := range []string{"/admin/report", "/admin/users/{id}"} {
		if item, ok := out.Paths[path]; ok {
			if opFor(item, "POST") != nil {
				t.Errorf("%s should not carry a POST operation", path)
			}
		}
	}

	// POST /products carries the gin-side request body.
	if products, ok := out.Paths["/products"]; ok {
		if post := opFor(products, "POST"); post == nil || post.RequestBody == nil {
			t.Errorf("POST /products should carry a request body (Product)")
		}
	}
}
