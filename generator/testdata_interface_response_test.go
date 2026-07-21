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

// TestTestdata_InterfaceResponse locks in interface-typed response resolution:
// a handler that encodes an interface-typed variable resolves to the concrete
// type statically assigned to it, and falls back to the interface when the
// concrete type is ambiguous (more than one assigned).
func TestTestdata_InterfaceResponse(t *testing.T) {
	out := loadTestdataWithFixtureConfig(t, "interface_response", spec.DefaultHTTPConfig())
	noDanglingRefs(t, out)

	schemas := out.Components.Schemas
	findSchema := func(suffix string) *intspec.Schema {
		for k, v := range schemas {
			if strings.HasSuffix(k, suffix) {
				return v
			}
		}
		return nil
	}
	// responseRef returns the trailing component name a path's POST response
	// resolves to.
	// responseSchema returns the schema of a path's POST response body.
	responseSchema := func(path string) *intspec.Schema {
		item, ok := out.Paths[path]
		if !ok {
			t.Fatalf("path %q missing; have %v", path, mapPathKeys(out.Paths))
		}
		op := opFor(item, "POST")
		if op == nil {
			t.Fatalf("POST %s missing", path)
		}
		for _, resp := range op.Responses {
			for _, mt := range resp.Content {
				if mt.Schema != nil {
					return mt.Schema
				}
			}
		}
		return nil
	}
	responseRef := func(path string) string {
		if s := responseSchema(path); s != nil {
			return s.Ref
		}
		return ""
	}

	// /dog: `var a Animal = Dog{}` → concrete Dog (with breed field).
	if ref := responseRef("/dog"); !strings.HasSuffix(ref, "_Dog") {
		t.Errorf("POST /dog response = %q, want the concrete Dog", ref)
	}
	if dog := findSchema("_response_Dog"); dog == nil || dog.Properties["breed"] == nil {
		t.Errorf("Dog schema missing 'breed'; got %+v", dog)
	}

	// /cat: `var a Animal; a = Cat{}` → concrete Cat (with lives field).
	if ref := responseRef("/cat"); !strings.HasSuffix(ref, "_Cat") {
		t.Errorf("POST /cat response = %q, want the concrete Cat", ref)
	}
	if cat := findSchema("_response_Cat"); cat == nil || cat.Properties["lives"] == nil {
		t.Errorf("Cat schema missing 'lives'; got %+v", cat)
	}

	// /either: two concrete types assigned → the payload really is one of them,
	// so it maps to `oneOf` (issue #201). Narrowing to one would be a guess, but
	// the previous fallback — the bare Animal interface, an empty-object schema
	// — described nothing at all.
	assertOneOf(t, responseSchema("/either"), "_Cat", "_Dog")

	// The bare interface must not linger as a component: the operation now
	// references the members, so emitting Animal too would leave a schema
	// nothing points at. (An interface that stays unresolved DOES keep its
	// component — testdata/interface_request_body /unknown covers that side.)
	if animal := findSchema("_response_Animal"); animal != nil {
		t.Errorf("bare interface Animal emitted as an orphan component: %+v", animal)
	}

	// /made: `Encode(makeDog())` where makeDog() Animal { return Dog{} } →
	// resolves to the concrete Dog via return-value tracing.
	if ref := responseRef("/made"); !strings.HasSuffix(ref, "_Dog") {
		t.Errorf("POST /made response = %q, want the concrete Dog (return trace)", ref)
	}

	// /passed: `writeAnimal(w, Dog{})` where writeAnimal(w, v Animal) encodes v
	// → resolves to Dog via the named-interface parameter binding.
	if ref := responseRef("/passed"); !strings.HasSuffix(ref, "_Dog") {
		t.Errorf("POST /passed response = %q, want the concrete Dog (param binding)", ref)
	}
}

// assertOneOf checks that a schema is a `oneOf` whose members are exactly the
// expected component suffixes, in order. Order matters: the concrete set is
// sorted so the output cannot vary between runs (golden rule #1).
func assertOneOf(t *testing.T, schema *intspec.Schema, wantSuffixes ...string) {
	t.Helper()
	if schema == nil {
		t.Fatalf("no schema; want oneOf %v", wantSuffixes)
	}
	if len(schema.OneOf) != len(wantSuffixes) {
		t.Fatalf("oneOf has %d members, want %d: %+v", len(schema.OneOf), len(wantSuffixes), schema.OneOf)
	}
	for i, want := range wantSuffixes {
		if got := schema.OneOf[i].Ref; !strings.HasSuffix(got, want) {
			t.Errorf("oneOf[%d] = %q, want a ref ending %q", i, got, want)
		}
	}
	// A polymorphic schema must not also carry a direct $ref — that would be
	// two conflicting statements about the same payload.
	if schema.Ref != "" {
		t.Errorf("oneOf schema also carries $ref %q", schema.Ref)
	}
}
