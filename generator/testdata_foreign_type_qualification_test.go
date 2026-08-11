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

// TestTestdata_ForeignTypeQualification guards issue #329. The argument
// renderer re-attaches a package to a type that carries none. It used to decide
// "carries none" by testing the string for a slash, which is wrong for a SHORT
// qualified name: time.Time is already qualified but has no slash, so the
// package the argument appears in was glued on, giving
// `<this package>-->time.Time`. That name matches no metadata entry and no
// well-known external type, so a timestamp documented as an empty object.
func TestTestdata_ForeignTypeQualification(t *testing.T) {
	out := loadTestdataWithFixtureConfig(t, "foreign_type_qualification", spec.DefaultHTTPConfig())

	// The map-literal response is where the mis-qualification surfaced: each
	// value's type is resolved separately, so a time.Time variable goes through
	// the argument renderer. On the broken predicate this produced a $ref to
	// `<this package>-->time.Time`, which no component ever satisfies.
	status, ok := out.Paths["/status"]
	if !ok {
		t.Fatalf("path /status missing; have %v", mapPathKeys(out.Paths))
	}
	statusOp := opFor(status, "GET")
	if statusOp == nil {
		t.Fatal("GET /status missing")
	}
	checkedAt := responsePropertySchema(t, statusOp, "checked_at")
	if checkedAt.Ref != "" {
		t.Errorf("checked_at is a $ref (%s); time.Time should resolve inline as a date-time string", checkedAt.Ref)
	}
	if checkedAt.Type != "string" || checkedAt.Format != "date-time" {
		t.Errorf("checked_at = {type:%q, format:%q}, want {string, date-time}", checkedAt.Type, checkedAt.Format)
	}

	// The same claim stated over the whole document: no reference anywhere may
	// name this package glued to a foreign type. Deliberately not
	// noDanglingRefs — this fixture still carries unrelated dangling refs from
	// issues #326 (untyped constants) and time.Duration, and this test is not
	// the place to assert those.
	forEachRef(out, func(ref string) {
		if strings.Contains(ref, "foreign_type_qualification_time_") {
			t.Errorf("$ref %q qualifies a foreign type with the package that uses it", ref)
		}
	})

	item, ok := out.Paths["/events/{id}"]
	if !ok {
		t.Fatalf("path /events/{id} missing; have %v", mapPathKeys(out.Paths))
	}
	if op := opFor(item, "GET"); op == nil {
		t.Fatal("GET /events/{id} missing")
	}
	event := componentFor(t, out, "_Event")

	// A type declared in this package must still be qualified with it — the fix
	// narrows re-qualification, it must not stop it.
	if local, ok := event.Properties["local"]; !ok {
		t.Errorf("Event.local missing; properties: %v", schemaPropertyNames(event))
	} else if local.Ref == "" {
		t.Errorf("Event.local should $ref the locally declared Local type, got %+v", local)
	} else if !strings.Contains(local.Ref, "foreign_type_qualification_Local") {
		t.Errorf("Event.local $refs %q, want the component qualified with this package", local.Ref)
	}
}

// responsePropertySchema returns a named property of the operation's first
// JSON response body schema.
func responsePropertySchema(t *testing.T, op *intspec.Operation, prop string) *spec.Schema {
	t.Helper()
	for _, code := range []string{"200", "default"} {
		resp, ok := op.Responses[code]
		if !ok {
			continue
		}
		mt, ok := resp.Content["application/json"]
		if !ok || mt.Schema == nil {
			continue
		}
		if s, ok := mt.Schema.Properties[prop]; ok {
			return s
		}
	}
	t.Fatalf("property %q not found in any JSON response of the operation", prop)
	return nil
}

// forEachRef visits every $ref in the document's paths and components.
func forEachRef(out *spec.OpenAPISpec, fn func(string)) {
	var walk func(s *spec.Schema)
	seen := map[*spec.Schema]bool{}
	walk = func(s *spec.Schema) {
		if s == nil || seen[s] {
			return
		}
		seen[s] = true
		if s.Ref != "" {
			fn(s.Ref)
		}
		for _, p := range s.Properties {
			walk(p)
		}
		walk(s.Items)
		walk(s.AdditionalProperties)
		for _, a := range s.AllOf {
			walk(a)
		}
		for _, a := range s.OneOf {
			walk(a)
		}
		for _, a := range s.AnyOf {
			walk(a)
		}
	}
	if out.Components != nil {
		for _, s := range out.Components.Schemas {
			walk(s)
		}
	}
	for _, item := range out.Paths {
		for _, op := range []*intspec.Operation{item.Get, item.Post, item.Put, item.Delete, item.Patch} {
			if op == nil {
				continue
			}
			if op.RequestBody != nil {
				for _, mt := range op.RequestBody.Content {
					walk(mt.Schema)
				}
			}
			for _, resp := range op.Responses {
				for _, mt := range resp.Content {
					walk(mt.Schema)
				}
			}
			for _, p := range op.Parameters {
				walk(p.Schema)
			}
		}
	}
}

// componentFor returns the single component whose name ends with suffix.
func componentFor(t *testing.T, out *spec.OpenAPISpec, suffix string) *spec.Schema {
	t.Helper()
	if out.Components == nil || out.Components.Schemas == nil {
		t.Fatal("no components generated")
	}
	for name, s := range out.Components.Schemas {
		if strings.HasSuffix(name, suffix) {
			return s
		}
	}
	t.Fatalf("no component ending %q; have %v", suffix, componentNames(out))
	return nil
}

func componentNames(out *spec.OpenAPISpec) []string {
	var names []string
	for n := range out.Components.Schemas {
		names = append(names, n)
	}
	return names
}

func schemaPropertyNames(s *spec.Schema) []string {
	var out []string
	for k := range s.Properties {
		out = append(out, k)
	}
	return out
}
