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

// TestTestdata_WrapperNilPayload guards the pair that has to stay apart when a
// response envelope is specialised per call site.
//
// A payload the caller did not supply — `respondJSON(w, nil)`, the shape of
// every logout and delete endpoint — is nothing to specialise, so the response
// is the envelope itself. A real payload still composes an allOf naming the
// concrete type.
//
// The empty-schema fallback for unmappable types (#395) briefly broke the first
// half: `nil` stopped resolving to a nil schema, so the override fired with a
// schema that constrains nothing and wrapped the base $ref in an allOf whose
// second member said nothing. It cost 27 responses on a 163-route service
// before it was caught by a snapshot comparison.
func TestTestdata_WrapperNilPayload(t *testing.T) {
	out := loadTestdataWithFixtureConfig(t, "wrapper_nil_payload", spec.DefaultHTTPConfig())
	noDanglingRefs(t, out)
	noNullSchemas(t, out)

	// No payload: the envelope, plainly. An allOf here is the defect.
	logout := responseSchemaOf(t, out, "/logout", "GET", "200")
	if len(logout.AllOf) != 0 {
		t.Errorf("GET /logout: composed an allOf for a nil payload: %+v", logout.AllOf)
	}
	if !strings.HasSuffix(logout.Ref, "_Envelope") {
		t.Errorf("GET /logout: schema = %+v, want a $ref to Envelope", logout)
	}

	// A real payload: still specialised, naming the concrete type.
	profile := responseSchemaOf(t, out, "/profile", "GET", "200")
	if len(profile.AllOf) != 2 {
		t.Fatalf("GET /profile: want an allOf of the envelope and its data, got %+v", profile)
	}
	if !strings.HasSuffix(profile.AllOf[0].Ref, "_Envelope") {
		t.Errorf("GET /profile: first allOf member = %+v, want the Envelope $ref", profile.AllOf[0])
	}
	data, ok := profile.AllOf[1].Properties["data"]
	if !ok {
		t.Fatalf("GET /profile: second allOf member has no data property: %+v", profile.AllOf[1])
	}
	if !strings.HasSuffix(data.Ref, "_User") {
		t.Errorf("GET /profile: data = %+v, want a $ref to User", data)
	}
}

// responseSchemaOf returns the JSON schema of one response, failing the test
// when any step of the path is missing.
func responseSchemaOf(t *testing.T, out *spec.OpenAPISpec, path, method, status string) *intspec.Schema {
	t.Helper()
	item, ok := out.Paths[path]
	if !ok {
		t.Fatalf("path %q missing; have %v", path, mapPathKeys(out.Paths))
	}
	op := opFor(item, method)
	if op == nil {
		t.Fatalf("%s %s: operation missing", method, path)
	}
	resp, ok := op.Responses[status]
	if !ok {
		t.Fatalf("%s %s: response %s missing; have %v", method, path, status, responseCodesOf(op.Responses))
	}
	media, ok := resp.Content["application/json"]
	if !ok {
		t.Fatalf("%s %s response %s: no JSON content", method, path, status)
	}
	if media.Schema == nil {
		t.Fatalf("%s %s response %s: null schema", method, path, status)
	}
	return media.Schema
}
