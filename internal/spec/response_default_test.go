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

func TestBuildResponses_UninformativeDefaultDropped(t *testing.T) {
	ref := func(name string) *Schema { return &Schema{Ref: "#/components/schemas/" + name} }

	t.Run("redundant default (same body as resolved) is dropped", func(t *testing.T) {
		r := buildResponses(map[string]*ResponseInfo{
			"201": {StatusCode: 201, ContentType: "application/json", BodyType: "User", Schema: ref("User")},
			"-1":  {StatusCode: -1, ContentType: "application/json", BodyType: "User", Schema: ref("User")},
		})
		if _, ok := r["default"]; ok {
			t.Error("default with a body already at a resolved status should be dropped")
		}
		if _, ok := r["201"]; !ok {
			t.Error("the resolved 201 must remain")
		}
	})

	t.Run("generic-object default is dropped when resolved statuses exist", func(t *testing.T) {
		r := buildResponses(map[string]*ResponseInfo{
			"200": {StatusCode: 200, ContentType: "application/json", BodyType: "[]Item", Schema: &Schema{Type: "array", Items: ref("Item")}},
			"-1":  {StatusCode: -1, ContentType: "application/json", BodyType: "any", Schema: &Schema{Type: "object"}},
		})
		if _, ok := r["default"]; ok {
			t.Error("bare generic-object default should be dropped alongside resolved statuses")
		}
	})

	t.Run("distinct concrete default is kept", func(t *testing.T) {
		r := buildResponses(map[string]*ResponseInfo{
			"400": {StatusCode: 400, ContentType: "application/json", BodyType: "ErrorResponse", Schema: ref("ErrorResponse")},
			"-1":  {StatusCode: -1, ContentType: "application/json", BodyType: "User", Schema: ref("User")},
		})
		if _, ok := r["default"]; !ok {
			t.Error("a default carrying a distinct concrete body must be kept")
		}
	})

	t.Run("default kept when it is the only response", func(t *testing.T) {
		r := buildResponses(map[string]*ResponseInfo{
			"-1": {StatusCode: -1, ContentType: "application/json", BodyType: "User", Schema: ref("User")},
		})
		if _, ok := r["default"]; !ok {
			t.Error("a sole default must be kept")
		}
	})

	t.Run("distinct bodies sharing an empty BodyType are not conflated", func(t *testing.T) {
		// Both have empty BodyType but different rendered schemas; matching on
		// BodyType alone would wrongly drop the distinct default.
		r := buildResponses(map[string]*ResponseInfo{
			"200": {StatusCode: 200, ContentType: "application/json", BodyType: "", Schema: ref("User")},
			"-1":  {StatusCode: -1, ContentType: "application/json", BodyType: "", Schema: ref("Account")},
		})
		if _, ok := r["default"]; !ok {
			t.Error("default with a distinct body must be kept even when BodyType matches")
		}
	})
}
