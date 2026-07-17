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

// TestTestdata_BodylessStatus covers issue #169 (GAP §7.3): bodyless status
// codes — 1xx, 204, and 304 — must be emitted with NO `content` block, since
// the OpenAPI spec forbids a body for these codes. Any body the handler appears
// to write (deleteWidget writes a spurious one alongside its 204) must be
// stripped. Non-bodyless responses (200) must keep their content untouched.
func TestTestdata_BodylessStatus(t *testing.T) {
	out := loadTestdataWithFixtureConfig(t, "bodyless_status", spec.DefaultHTTPConfig())
	noDanglingRefs(t, out)

	// (path, method, status) -> whether a content block is expected.
	cases := []struct {
		path, method, status string
		wantContent          bool
	}{
		{"/widget/{id}", "DELETE", "204", false}, // No Content — spurious body dropped
		{"/widget/{id}", "HEAD", "304", false},   // Not Modified
		{"/upload", "POST", "100", false},        // 1xx informational
		{"/widget/{id}", "GET", "200", true},     // normal body — unaffected
	}

	for _, tc := range cases {
		item, ok := out.Paths[tc.path]
		if !ok {
			t.Errorf("path %s missing; have %v", tc.path, mapPathKeys(out.Paths))
			continue
		}
		op := opFor(item, tc.method)
		if op == nil {
			t.Errorf("%s %s missing", tc.method, tc.path)
			continue
		}
		resp, ok := op.Responses[tc.status]
		if !ok {
			t.Errorf("%s %s: status %s missing; have %v",
				tc.method, tc.path, tc.status, keysOf(op.Responses))
			continue
		}
		hasContent := len(resp.Content) > 0
		if hasContent != tc.wantContent {
			t.Errorf("%s %s status %s: content present=%v, want %v (content=%v)",
				tc.method, tc.path, tc.status, hasContent, tc.wantContent, resp.Content)
		}
	}
}
