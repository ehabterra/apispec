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

	"github.com/ehabterra/apispec/spec"
)

// TestTestdata_ChiMethodHandle locks in chi's non-verb registration surface:
// r.Method(http.MethodGet, ...) / r.MethodFunc (verb from the first argument,
// including stdlib http.Method* constants), r.Handle with an opaque
// http.Handler value (path + defaulted GET, at minimum), and r.HandleFunc
// whose handler dispatches on r.Method (must split per verb — the defaulted
// method must not be marked explicit).
func TestTestdata_ChiMethodHandle(t *testing.T) {
	out := loadTestdataWithFixtureConfig(t, "chi_method_handle", spec.DefaultChiConfig())
	noDanglingRefs(t, out)

	want := map[string][]string{
		"/live":    {"GET"},           // plain r.Get with a func value
		"/live2":   {"GET"},           // r.Get with a method value on a struct field
		"/health":  {"GET"},           // r.Method(http.MethodGet, ..., http.Handler value)
		"/ready":   {"POST"},          // r.MethodFunc(http.MethodPost, ...)
		"/metrics": {"GET"},           // r.Handle with an opaque handler: defaulted GET
		"/items":   {"GET", "DELETE"}, // r.HandleFunc + switch r.Method: split per verb
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

	// The verb came from the constant argument, so it is explicit: /health
	// must not pick up extra verbs and must not fall back to the POST preset.
	if health, ok := out.Paths["/health"]; ok {
		if opFor(health, "POST") != nil {
			t.Errorf("/health should not carry a POST operation (verb is http.MethodGet)")
		}
		// Change detector: an opaque http.Handler *value* (r.Method(..., deps.Health))
		// names no method in the registration, so #168 cannot reach the concrete
		// ServeHTTP doc comment — resolving it needs interface-value → concrete
		// type resolution. Asserted empty on purpose; flip when issue #204 lands.
		if get := opFor(health, "GET"); get != nil && get.Summary != "" {
			t.Errorf("GET /health summary: got %q, want \"\" until handler-value doc sourcing lands (#168)", get.Summary)
		}
	}

	// #168 is framework-agnostic: it resolves off the handler declaration, not
	// the router. A chi-registered method value on a DI field (deps.Health.ServeHTTP)
	// sources its summary through the field's type, same as a net/http one.
	if live2, ok := out.Paths["/live2"]; ok {
		if get := opFor(live2, "GET"); get != nil && get.Summary != "ServeHTTP reports service health." {
			t.Errorf("GET /live2 summary: got %q (#168)", get.Summary)
		}
	}

	// The dispatch split must not leak the default onto non-served verbs.
	if items, ok := out.Paths["/items"]; ok {
		if opFor(items, "POST") != nil {
			t.Errorf("/items should not carry a POST operation (handler serves GET/DELETE)")
		}
	}
}
