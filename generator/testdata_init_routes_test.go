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
	"strings"
	"testing"

	"github.com/ehabterra/apispec/spec"
)

// TestTestdata_InitRoutesNetHTTP locks in issue #580: routes registered from a
// package init function are documented. Nothing calls init, so the tree, rooted
// only at main, never reached them — including the plugin shape, an init in a
// package main only blank-imports.
//
// Two inits in one package share an identity in metadata; both are asserted so
// that a fix keying on one declaration cannot quietly drop the other.
func TestTestdata_InitRoutesNetHTTP(t *testing.T) {
	out := loadTestdata(t, "init_routes_nethttp", spec.DefaultHTTPConfig())
	noDanglingRefs(t, out)

	for path, ref := range map[string]string{
		"/from-main":        "_A",
		"/from-init":        "_B",
		"/from-second-init": "_C",
		"/plugins":          "_Plugin",
	} {
		// A verbless HandleFunc takes the POST default (golden rule #8), the
		// same wherever it is registered from.
		op := opFor(out.Paths[path], "POST")
		if op == nil {
			t.Errorf("POST %s missing; have %v", path, mapPathKeys(out.Paths))
			continue
		}
		if !strings.Contains(responseSchemaRef(op, "200"), ref) {
			t.Errorf("POST %s 200 = %q, want a $ref ending %s", path, responseSchemaRef(op, "200"), ref)
		}
	}

	// Change detector for #581: a call in a package-level var initializer has
	// no call-graph edge, so `var _ = register("/from-var", d)` registers
	// nothing. Flip this when that is fixed.
	if _, ok := out.Paths["/from-var"]; ok {
		t.Errorf("/from-var is now documented — #581 may be fixed; assert it here instead")
	}
}

// TestTestdata_InitRoutesGin covers init-registered routes on a router held in
// a package-level var (issue #580). The group created inside init must keep its
// prefix: main is never anyone's callee, so its own assignments were recorded
// specially, and init — never called either — needed the same, or
// `v1 := r.Group("/v1")` documented /health.
func TestTestdata_InitRoutesGin(t *testing.T) {
	out := loadTestdata(t, "init_routes_gin", spec.DefaultGinConfig())
	noDanglingRefs(t, out)

	for _, path := range []string{"/from-main", "/items/{id}", "/v1/health"} {
		if opFor(out.Paths[path], "GET") == nil {
			t.Errorf("GET %s missing; have %v", path, mapPathKeys(out.Paths))
		}
	}
	if _, ok := out.Paths["/health"]; ok {
		t.Errorf("/health documented without its /v1 group prefix")
	}

	// Change detector for #582: a group stored in a package-level var by one
	// function loses its prefix where another registers on it — init or not.
	// The route is found; only its prefix is missing. Flip to /api/plugins.
	if opFor(out.Paths["/plugins"], "GET") == nil {
		t.Errorf("GET /plugins missing (expected unprefixed until #582); have %v", mapPathKeys(out.Paths))
	}
}
