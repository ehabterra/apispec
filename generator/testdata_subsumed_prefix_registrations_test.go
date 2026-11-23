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
	"github.com/ehabterra/apispec/spec"
)

// TestTestdata_SubsumedPrefixRegistrations locks in issue #614: a handler
// registered by two separate calls at prefixes that extend each other keeps
// both endpoints.
//
//	repo.GET("/attachments/:uuid", getAttachment)
//	repo.Group("/issues/:index").GET("/attachments/:uuid", getAttachment)
//
// The partial-mount dedup judged a pair by mount segments alone, and the
// shorter prefix is a subsequence of the longer, so the first endpoint was read
// as an echo of the second and dropped. Only one registration reached through
// several contexts is an echo; the dedup is now scoped to the call site.
func TestTestdata_SubsumedPrefixRegistrations(t *testing.T) {
	out := loadTestdataWithFixtureConfig(t, "subsumed_prefix_registrations", spec.DefaultGinConfig())
	noDanglingRefs(t, out)
	noUnresolvedPlaceholders(t, out)

	// Two registration calls: both are real, and each carries the handler's
	// own params and body — neither is a stub left behind by the other.
	for _, path := range []string{
		"/{owner}/{repo}/attachments/{uuid}",
		"/{owner}/{repo}/issues/{index}/attachments/{uuid}",
	} {
		item, ok := out.Paths[path]
		if !ok {
			t.Errorf("missing %s; documented %v — a second registration at an extending "+
				"prefix must not erase the first", path, mapPathKeys(out.Paths))
			continue
		}
		op := opFor(item, "GET")
		if op == nil {
			t.Errorf("GET %s: operation missing", path)
			continue
		}
		if !hasPathParam(op.Parameters, "uuid") {
			t.Errorf("GET %s: the handler's uuid parameter is missing", path)
		}
		resp, ok := op.Responses["200"]
		if !ok || resp.Content["application/json"].Schema == nil {
			t.Errorf("GET %s: the handler's 200 body is missing", path)
		}
	}

	// One registration in a helper handed a nested group: the traversal also
	// reaches it under partial mount contexts, and those echoes must still go.
	if _, ok := out.Paths["/api/users/{id}"]; !ok {
		t.Errorf("missing /api/users/{id}; documented %v", mapPathKeys(out.Paths))
	}
	for _, echo := range []string{"/users/{id}", "/{id}", "/attachments/{uuid}", "/items", "/b/items"} {
		if _, ok := out.Paths[echo]; ok {
			t.Errorf("%s is a partial-mount echo and must be dropped", echo)
		}
	}

	// The probe: ONE registration call in a helper invoked with /a and with
	// /a/b. Both prefixes are genuine, but the two routes share the call site,
	// so the call-site scope cannot tell /a/items from an echo of /a/b/items and
	// it is dropped as one. Pinned at that current (wrong) behaviour so the test
	// flips when the echo is told apart by something finer than the site.
	if _, ok := out.Paths["/a/b/items"]; !ok {
		t.Errorf("missing /a/b/items; documented %v", mapPathKeys(out.Paths))
	}
	if _, ok := out.Paths["/a/items"]; ok {
		t.Error("/a/items is now documented — the same-site extending-prefix gap is fixed; update this assertion")
	}
}

func hasPathParam(params []intspec.Parameter, name string) bool {
	for _, p := range params {
		if p.Name == name && p.In == "path" {
			return true
		}
	}
	return false
}
