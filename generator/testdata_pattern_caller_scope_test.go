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

import (
	"testing"

	intspec "github.com/ehabterra/apispec/internal/spec"
)

// callerScopedConfig admits chi registrations only from the fixture's api
// package. Nothing about the CALL distinguishes the two packages — both make the
// same `r.Get(...)` on the same chi router — so the caller's package is the only
// fact that can.
func callerScopedConfig() *intspec.APISpecConfig {
	cfg := intspec.DefaultChiConfig()
	for i := range cfg.Framework.RoutePatterns {
		cfg.Framework.RoutePatterns[i].CallerPkgPatterns = []string{`/internal/api$`}
	}
	return cfg
}

// TestTestdata_PatternCallerScope covers the four caller/callee filter fields,
// which every pattern type has declared since the config was written and which no
// matcher read: a user set `callerPkgPatterns`, got no filtering, no warning and
// no error, and had no way to tell (issue #238).
//
// The fixture registers routes from two packages through the same router, so the
// filter is the only thing that can separate them. Both directions are asserted
// against the SAME fixture — first that the unfiltered config documents
// everything, then that the filtered one drops exactly the operator routes —
// because an assertion that something is absent proves nothing on its own: the
// route has to be there to begin with.
func TestTestdata_PatternCallerScope(t *testing.T) {
	// The two runs differ in ONE field, so whatever the second one drops was
	// dropped by the filter and by nothing else.
	unfiltered := loadTestdata(t, "pattern_caller_scope", intspec.DefaultChiConfig())

	for _, path := range []string{"/users", "/debug/stats"} {
		if _, ok := unfiltered.Paths[path]; !ok {
			t.Fatalf("%s missing WITHOUT any filter; have %v — the fixture cannot prove a filter dropped it",
				path, mapPathKeys(unfiltered.Paths))
		}
	}

	filtered := loadTestdata(t, "pattern_caller_scope", callerScopedConfig())

	if _, ok := filtered.Paths["/debug/stats"]; ok {
		t.Errorf("/debug/stats is still documented; callerPkgPatterns admitted only /internal/api, so a call made from debugroutes must not match")
	}

	// The filter must narrow, not break: the admitted package keeps everything it
	// had, bodies included.
	users, ok := filtered.Paths["/users"]
	if !ok {
		t.Fatalf("/users missing under the filter; have %v — the caller filter rejected the package it names", mapPathKeys(filtered.Paths))
	}
	get, post := opFor(users, "GET"), opFor(users, "POST")
	if get == nil || post == nil {
		t.Fatalf("/users: want GET and POST, got get=%v post=%v", get != nil, post != nil)
	}
	if _, hasOK := get.Responses["200"]; !hasOK {
		t.Errorf("GET /users: 200 missing — have %v", keysOf(get.Responses))
	}
	if post.RequestBody == nil {
		t.Error("POST /users: requestBody missing — the filter must not disturb extraction of the routes it admits")
	}
}
