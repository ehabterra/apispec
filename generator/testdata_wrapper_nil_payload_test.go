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
// Both halves compose, and what each one composes is the point. A real payload
// names its type; a nil payload pins `data` to null, which is what the endpoint
// actually sends.
//
// The empty-schema fallback for unmappable types (#395) briefly made the nil
// case compose NOTHING useful — `data: {}`, an allOf member that constrains
// nothing — which cost 27 responses on a 163-route service their plain $ref for
// no gain. #404 replaced that with `type: "null"`, so the composition is worth
// making again. `isEmptySchema` still guards the case it was written for: an
// override that genuinely resolves to nothing is still skipped.
func TestTestdata_WrapperNilPayload(t *testing.T) {
	out := loadTestdataWithFixtureConfig(t, "wrapper_nil_payload", spec.DefaultHTTPConfig())
	noDanglingRefs(t, out)
	noNullSchemas(t, out)

	// No payload: the envelope with data pinned to null, because that is what
	// the endpoint sends. This composed to noise — `data: {}` — for as long as a
	// literal nil had no schema of its own; #404 gave it `type: "null"`, which
	// says something true and narrows the envelope's `Data any`.
	logout := responseSchemaOf(t, out, "/logout", "GET", "200")
	if len(logout.AllOf) != 2 {
		t.Fatalf("GET /logout: want the envelope composed with its null data, got %+v", logout)
	}
	if !strings.HasSuffix(logout.AllOf[0].Ref, "_Envelope") {
		t.Errorf("GET /logout: first allOf member = %+v, want the Envelope $ref", logout.AllOf[0])
	}
	nulled, ok := logout.AllOf[1].Properties["data"]
	if !ok {
		t.Fatalf("GET /logout: second allOf member has no data property: %+v", logout.AllOf[1])
	}
	if nulled.Type != "null" {
		t.Errorf("GET /logout: data = %+v, want type null — the endpoint sends `\"data\": null`", nulled)
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
