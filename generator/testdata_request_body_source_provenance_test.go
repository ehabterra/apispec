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

	intspec "github.com/ehabterra/apispec/internal/spec"
	"github.com/ehabterra/apispec/spec"
)

// TestTestdata_RequestBodySourceProvenance is the read-side mirror of the
// response write-destination gating: a decode is a request body only when its
// source traces to r.Body. The KEEP routes cover direct/helper/assign
// provenance to r.Body (the io.Reader-helper case fixes a false negative); the
// DROP routes decode non-request readers through the same helper and inline (no
// false positive from the shared helper node). Config must never be a body.
func TestTestdata_RequestBodySourceProvenance(t *testing.T) {
	out := loadTestdataWithFixtureConfig(t, "request_body_source_provenance", spec.DefaultHTTPConfig())
	noDanglingRefs(t, out)
	noUnresolvedPlaceholders(t, out)

	// Config is only ever decoded from non-request readers — never a body.
	for _, path := range []string{"/decode-buffer", "/decode-string"} {
		post := opFor(out.Paths[path], "POST")
		if post == nil {
			t.Errorf("POST %s missing; have %v", path, mapPathKeys(out.Paths))
			continue
		}
		if post.RequestBody != nil && requestBodyRefs(post.RequestBody, "Config") {
			t.Errorf("POST %s: Config leaked as a request body — a non-request source was accepted", path)
		}
	}

	// KEEP: source traces to r.Body → CreateUserRequest body present.
	for _, path := range []string{"/direct", "/helper", "/assign"} {
		post := opFor(out.Paths[path], "POST")
		if post == nil {
			t.Errorf("POST %s missing; have %v", path, mapPathKeys(out.Paths))
			continue
		}
		if post.RequestBody == nil || !requestBodyRefs(post.RequestBody, "CreateUserRequest") {
			t.Errorf("POST %s: expected a CreateUserRequest body (source traces to r.Body), got %v",
				path, post.RequestBody)
		}
	}
}

func requestBodyRefs(rb *intspec.RequestBody, name string) bool {
	for _, media := range rb.Content {
		if media.Schema != nil && strings.Contains(media.Schema.Ref, name) {
			return true
		}
	}
	return false
}
