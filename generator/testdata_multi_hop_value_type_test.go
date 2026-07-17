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

// TestTestdata_MultiHopValueType covers issue #180: a concrete value forwarded
// through two or more `any` helper hops must keep its concrete type. One hop was
// the control that already worked; two and three hops previously erased to a
// generic object. The request side (already multi-hop) guards symmetry.
func TestTestdata_MultiHopValueType(t *testing.T) {
	out := loadTestdataWithFixtureConfig(t, "multi_hop_value_type", spec.DefaultHTTPConfig())
	noDanglingRefs(t, out)
	noUnresolvedPlaceholders(t, out)

	for _, path := range []string{"/one-hop", "/two-hops", "/three-hops"} {
		get := opFor(out.Paths[path], "GET")
		if get == nil {
			t.Errorf("GET %s missing; have %v", path, mapPathKeys(out.Paths))
			continue
		}
		if !responseSchemaRefs(get.Responses, "User") {
			t.Errorf("GET %s: expected a User response through the helper chain, got %v",
				path, keysOf(get.Responses))
		}
	}

	post := opFor(out.Paths["/create-two-hops"], "POST")
	if post == nil {
		t.Fatalf("POST /create-two-hops missing; have %v", mapPathKeys(out.Paths))
	}
	if post.RequestBody == nil || !bodySchemaRefs(post.RequestBody, "CreateUserRequest") {
		t.Errorf("POST /create-two-hops: expected a CreateUserRequest body through the helper chain, got %v",
			post.RequestBody)
	}
}

func responseSchemaRefs(responses map[string]intspec.Response, name string) bool {
	for _, resp := range responses {
		for _, media := range resp.Content {
			if media.Schema != nil && strings.Contains(media.Schema.Ref, name) {
				return true
			}
		}
	}
	return false
}

func bodySchemaRefs(rb *intspec.RequestBody, name string) bool {
	for _, media := range rb.Content {
		if media.Schema != nil && strings.Contains(media.Schema.Ref, name) {
			return true
		}
	}
	return false
}
