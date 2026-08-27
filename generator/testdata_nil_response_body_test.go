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

// TestTestdata_NilResponseBody locks in issue #404: a handler that answers with
// a literal nil sends `null`, and the spec says `type: "null"` rather than the
// empty schema.
//
// `{}` is not a vaguer version of the same answer — it says the response may be
// an object, an array or a string, which is the one thing this endpoint never
// sends. The value is stated at the call site and was being thrown away.
func TestTestdata_NilResponseBody(t *testing.T) {
	out := loadTestdataWithFixtureConfig(t, "nil_response_body", spec.DefaultHTTPConfig())
	noDanglingRefs(t, out)
	noNullSchemas(t, out)

	// The error branch answers nil…
	if s := responseSchemaOf(t, out, "/items/{id}", "GET", "404"); s.Type != "null" {
		t.Errorf("GET /items/{id} 404: schema = %+v, want type null", s)
	}
	// …and so does a success path, which must read the same way.
	if s := responseSchemaOf(t, out, "/items/{id}", "DELETE", "200"); s.Type != "null" {
		t.Errorf("DELETE /items/{id} 200: schema = %+v, want type null", s)
	}
	// The real body beside them is untouched.
	if s := responseSchemaOf(t, out, "/items/{id}", "GET", "200"); s.Ref == "" {
		t.Errorf("GET /items/{id} 200: schema = %+v, want a $ref to Item", s)
	}
}
