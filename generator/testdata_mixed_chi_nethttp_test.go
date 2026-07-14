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

// TestTestdata_MixedChiNetHTTP locks in multi-framework config merging: a
// binary serving a chi API alongside plain net/http ops endpoints (default
// ServeMux, the expvar/pprof wiring style) must document BOTH surfaces.
// net/http never appears in go.mod, so this rides on the engine layering the
// receiver-scoped HTTPSecondaryConfig under the detected framework — the
// fallback config passed here is nil on purpose so the engine's own
// detection + merge path is what runs.
func TestTestdata_MixedChiNetHTTP(t *testing.T) {
	out := loadTestdataWithFixtureConfig(t, "mixed_chi_nethttp", nil)
	noDanglingRefs(t, out)
	noUnresolvedPlaceholders(t, out)

	want := map[string][]string{
		"/api/users":   {"GET", "POST"},   // chi verb calls
		"/ops/version": {"GET"},           // Go 1.22 "GET /ops/version" ServeMux pattern
		"/ops/status":  {"GET", "DELETE"}, // plain HandleFunc + switch r.Method
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

	// The explicit pattern verb must not leak extra operations, and the
	// dispatch split must not leave the POST preset behind.
	if version, ok := out.Paths["/ops/version"]; ok {
		if opFor(version, "POST") != nil {
			t.Errorf("/ops/version should not carry a POST operation")
		}
	}
	if status, ok := out.Paths["/ops/status"]; ok {
		if opFor(status, "POST") != nil {
			t.Errorf("/ops/status should not carry a POST operation")
		}
	}

	// POST /api/users carries the chi-side request body.
	if users, ok := out.Paths["/api/users"]; ok {
		if post := opFor(users, "POST"); post == nil || post.RequestBody == nil {
			t.Errorf("POST /api/users should carry a request body (User)")
		}
		// A raw r.Header.Get inside the chi handler is documented via the
		// merged stdlib param patterns.
		if get := opFor(users, "GET"); get != nil {
			var found bool
			for _, p := range get.Parameters {
				if p.Name == "X-Request-ID" && p.In == "header" {
					found = true
				}
			}
			if !found {
				t.Errorf("GET /api/users should document the X-Request-ID header parameter")
			}
		}
	}
}
