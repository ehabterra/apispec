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

// TestTestdata_InterfaceRequestBody locks in interface-typed REQUEST body
// resolution (issue #164), the mirror of TestTestdata_InterfaceResponse: a
// handler that decodes into an interface-typed variable resolves to the
// concrete type statically assigned to it, and falls back to the interface when
// the concrete type is ambiguous.
//
// Before the fix the request path emitted a `$ref` to the bare interface, whose
// schema is an empty object — strictly worse than omitting the body, since it
// documents a payload with no fields.
func TestTestdata_InterfaceRequestBody(t *testing.T) {
	out := loadTestdataWithFixtureConfig(t, "interface_request_body", spec.DefaultHTTPConfig())
	noDanglingRefs(t, out)
	noUnresolvedPlaceholders(t, out)

	findSchema := func(suffix string) *intspec.Schema {
		for k, v := range out.Components.Schemas {
			if strings.HasSuffix(k, suffix) {
				return v
			}
		}
		return nil
	}
	// requestSchema returns the schema of a path's POST request body.
	requestSchema := func(path string) *intspec.Schema {
		item, ok := out.Paths[path]
		if !ok {
			t.Fatalf("path %q missing; have %v", path, mapPathKeys(out.Paths))
		}
		op := opFor(item, "POST")
		if op == nil {
			t.Fatalf("POST %s missing", path)
		}
		if op.RequestBody == nil {
			t.Fatalf("POST %s has no requestBody", path)
		}
		for _, mt := range op.RequestBody.Content {
			if mt.Schema != nil {
				return mt.Schema
			}
		}
		return nil
	}
	requestRef := func(path string) string {
		if s := requestSchema(path); s != nil {
			return s.Ref
		}
		return ""
	}

	// /dogs: `var a Animal = Dog{}` → concrete Dog (with breed).
	if ref := requestRef("/dogs"); !strings.HasSuffix(ref, "_Dog") {
		t.Errorf("POST /dogs requestBody = %q, want the concrete Dog (#164)", ref)
	}
	if dog := findSchema("_request_body_Dog"); dog == nil || dog.Properties["breed"] == nil {
		t.Errorf("Dog schema missing 'breed'; got %+v", dog)
	}

	// /cats: `var a Animal; a = Cat{}` → concrete Cat (with lives).
	if ref := requestRef("/cats"); !strings.HasSuffix(ref, "_Cat") {
		t.Errorf("POST /cats requestBody = %q, want the concrete Cat (#164)", ref)
	}
	if cat := findSchema("_request_body_Cat"); cat == nil || cat.Properties["lives"] == nil {
		t.Errorf("Cat schema missing 'lives'; got %+v", cat)
	}

	// /either: two concrete types assigned → the payload really is one of them,
	// so it maps to `oneOf` (issue #201) rather than to a guessed single type or
	// to the bare interface, whose schema describes nothing.
	assertOneOf(t, requestSchema("/either"), "_Cat", "_Dog")

	// /unknown: an interface with no traceable assignment stays the interface —
	// the honest answer — and its component must still be emitted. This is the
	// counterweight to the orphan check below: component marking follows what
	// the operations actually reference, rather than pruning interfaces wholesale.
	if ref := requestRef("/unknown"); !strings.HasSuffix(ref, "_Animal") {
		t.Errorf("POST /unknown requestBody = %q, want the Animal interface kept", ref)
	}
	if findSchema("_body_Animal") == nil {
		t.Error("Animal component missing though /unknown references it")
	}

	// /concrete: the pre-existing concrete path must be unaffected.
	if ref := requestRef("/concrete"); !strings.HasSuffix(ref, "_Dog") {
		t.Errorf("POST /concrete requestBody = %q, want Dog", ref)
	}

	// /via-param: the concrete is bound at the call site entering the helper
	// whose parameter is the interface.
	if ref := requestRef("/via-param"); !strings.HasSuffix(ref, "_Cat") {
		t.Errorf("POST /via-param requestBody = %q, want the bound Cat (#164)", ref)
	}

	// /pointer: `var a Animal = &Dog{}` decoded via Decode(a) — the pointer
	// shape the response fixture never exercises, since responses encode the
	// value rather than decoding into a pointer.
	if ref := requestRef("/pointer"); !strings.HasSuffix(ref, "_Dog") {
		t.Errorf("POST /pointer requestBody = %q, want the concrete Dog (#164)", ref)
	}
}
