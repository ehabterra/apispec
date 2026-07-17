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

func TestIsBodylessStatus(t *testing.T) {
	bodyless := []int{100, 101, 102, 199, 204, 205, 304}
	for _, code := range bodyless {
		if !isBodylessStatus(code) {
			t.Errorf("isBodylessStatus(%d) = false, want true", code)
		}
	}
	withBody := []int{200, 201, 202, 203, 206, 300, 301, 400, 404, 500}
	for _, code := range withBody {
		if isBodylessStatus(code) {
			t.Errorf("isBodylessStatus(%d) = true, want false", code)
		}
	}
}

// TestBuildResponses_BodylessOmitsContent pins issue #169: 1xx/204/304 must be
// emitted with no `content` block even when a body was resolved for them, while
// ordinary statuses keep their content.
func TestBuildResponses_BodylessOmitsContent(t *testing.T) {
	ref := func(name string) *Schema { return &Schema{Ref: "#/components/schemas/" + name} }

	r := buildResponses(map[string]*ResponseInfo{
		// A spurious body is attached to each bodyless status to prove it is
		// dropped regardless.
		"100": {StatusCode: 100, ContentType: "application/json", BodyType: "Widget", Schema: ref("Widget")},
		"204": {StatusCode: 204, ContentType: "application/json", BodyType: "Widget", Schema: ref("Widget")},
		"304": {StatusCode: 304, ContentType: "application/json", BodyType: "Widget", Schema: ref("Widget")},
		"200": {StatusCode: 200, ContentType: "application/json", BodyType: "Widget", Schema: ref("Widget")},
	})

	for _, status := range []string{"100", "204", "304"} {
		resp, ok := r[status]
		if !ok {
			t.Errorf("status %s missing from responses", status)
			continue
		}
		if len(resp.Content) != 0 {
			t.Errorf("status %s must have no content, got %v", status, resp.Content)
		}
		if resp.Description == "" {
			t.Errorf("status %s should retain a description", status)
		}
	}

	if resp, ok := r["200"]; !ok || len(resp.Content) == 0 {
		t.Error("status 200 must keep its content block")
	}
}
