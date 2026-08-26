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

// TestTestdata_InterfaceFieldSchema locks in issue #395: a struct field whose
// type has no schema mapping must not be emitted as a null property.
//
// `Err error` is the shape that surfaced it. metadata classifies `error` as
// primitive, so the mapper skipped the $ref branch, found no case for it, and
// stored the nil it got back — `properties: {err: null}`. A reader that walks
// properties dereferences that and dies ("Cannot read properties of null
// (reading '$ref')" in ReDoc 2.5.3).
func TestTestdata_InterfaceFieldSchema(t *testing.T) {
	out := loadTestdataWithFixtureConfig(t, "interface_field_schema", spec.DefaultHTTPConfig())
	noDanglingRefs(t, out)
	noNullSchemas(t, out)

	failure := componentByName(out, "PushFailure")
	if failure == nil {
		t.Fatal("PushFailure component missing")
	}
	// An error is an interface, so it is documented like `any` — the mapping
	// the same switch already gives interface{} and any.
	errProp, ok := failure.Properties["err"]
	if !ok {
		t.Fatalf("err property missing; have %v", propertyNames(failure))
	}
	if errProp == nil {
		t.Fatal("err property is null — the defect this fixture is for")
	}
	if errProp.Type != "object" {
		t.Errorf("err property = %+v, want an object like any/interface{}", errProp)
	}
	// The ordinary fields beside it are unaffected.
	if s := failure.Properties["message"]; s == nil || s.Type != "string" {
		t.Errorf("message property = %+v, want a string", s)
	}

	// A type nothing can honestly map — encoding/json cannot marshal a complex
	// number at all — gets the empty schema rather than an invented type.
	sample := componentByName(out, "Sample")
	if sample == nil {
		t.Fatal("Sample component missing")
	}
	for _, field := range []string{"ratio", "smaller"} {
		s, ok := sample.Properties[field]
		if !ok {
			t.Errorf("%s property missing; have %v", field, propertyNames(sample))
			continue
		}
		if s == nil {
			t.Errorf("%s property is null", field)
			continue
		}
		if s.Type != "" || s.Ref != "" {
			t.Errorf("%s property = %+v, want the empty schema (any)", field, s)
		}
	}
}
