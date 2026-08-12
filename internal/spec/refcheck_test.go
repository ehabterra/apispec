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

import (
	"sort"
	"strings"
	"testing"
	"time"
)

func timeAfterShort() <-chan time.Time { return time.After(5 * time.Second) }

func refTo(name string) *Schema { return &Schema{Ref: refComponentsSchemasPrefix + name} }

func specWithSchemas(s map[string]*Schema) *OpenAPISpec {
	return &OpenAPISpec{Paths: map[string]PathItem{}, Components: &Components{Schemas: s}}
}

// TestForEachSchemaRefVisitsEveryLocation is the important one: a location the
// walker misses is a reference the repair leaves dangling, which is the exact
// failure this feature exists to prevent. Each case puts a reference in one
// place and nowhere else.
func TestForEachSchemaRefVisitsEveryLocation(t *testing.T) {
	cases := map[string]func() *OpenAPISpec{
		"operation response body": func() *OpenAPISpec {
			return &OpenAPISpec{Paths: map[string]PathItem{"/a": {Get: &Operation{
				Responses: map[string]Response{"200": {Content: map[string]MediaType{"application/json": {Schema: refTo("T")}}}},
			}}}}
		},
		"operation request body": func() *OpenAPISpec {
			return &OpenAPISpec{Paths: map[string]PathItem{"/a": {Post: &Operation{
				RequestBody: &RequestBody{Content: map[string]MediaType{"application/json": {Schema: refTo("T")}}},
			}}}}
		},
		"operation parameter schema": func() *OpenAPISpec {
			return &OpenAPISpec{Paths: map[string]PathItem{"/a": {Get: &Operation{
				Parameters: []Parameter{{Name: "id", Schema: refTo("T")}},
			}}}}
		},
		"operation parameter $ref": func() *OpenAPISpec {
			return &OpenAPISpec{Paths: map[string]PathItem{"/a": {Get: &Operation{
				Parameters: []Parameter{{Ref: refComponentsSchemasPrefix + "T"}},
			}}}}
		},
		"path-level parameter": func() *OpenAPISpec {
			return &OpenAPISpec{Paths: map[string]PathItem{"/a": {
				Parameters: []Parameter{{Name: "id", Schema: refTo("T")}},
			}}}
		},
		"response header": func() *OpenAPISpec {
			return &OpenAPISpec{Paths: map[string]PathItem{"/a": {Get: &Operation{
				Responses: map[string]Response{"200": {Headers: map[string]Header{"X-Thing": {Schema: refTo("T")}}}},
			}}}}
		},
		"component property": func() *OpenAPISpec {
			return specWithSchemas(map[string]*Schema{"Outer": {Properties: map[string]*Schema{"f": refTo("T")}}})
		},
		"component array items": func() *OpenAPISpec {
			return specWithSchemas(map[string]*Schema{"Outer": {Type: "array", Items: refTo("T")}})
		},
		"component additionalProperties": func() *OpenAPISpec {
			return specWithSchemas(map[string]*Schema{"Outer": {Type: "object", AdditionalProperties: refTo("T")}})
		},
		"component oneOf member": func() *OpenAPISpec {
			return specWithSchemas(map[string]*Schema{"Outer": {OneOf: []*Schema{refTo("T")}}})
		},
		"component allOf member": func() *OpenAPISpec {
			return specWithSchemas(map[string]*Schema{"Outer": {AllOf: []*Schema{refTo("T")}}})
		},
		"component anyOf member": func() *OpenAPISpec {
			return specWithSchemas(map[string]*Schema{"Outer": {AnyOf: []*Schema{refTo("T")}}})
		},
		"component not": func() *OpenAPISpec {
			return specWithSchemas(map[string]*Schema{"Outer": {Not: refTo("T")}})
		},
		"components.parameters": func() *OpenAPISpec {
			return &OpenAPISpec{Paths: map[string]PathItem{}, Components: &Components{
				Parameters: map[string]*Parameter{"id": {Schema: refTo("T")}}}}
		},
		"components.requestBodies": func() *OpenAPISpec {
			return &OpenAPISpec{Paths: map[string]PathItem{}, Components: &Components{
				RequestBodies: map[string]*RequestBody{"b": {Content: map[string]MediaType{"application/json": {Schema: refTo("T")}}}}}}
		},
		"components.responses": func() *OpenAPISpec {
			return &OpenAPISpec{Paths: map[string]PathItem{}, Components: &Components{
				Responses: map[string]*Response{"r": {Content: map[string]MediaType{"application/json": {Schema: refTo("T")}}}}}}
		},
		"components.headers": func() *OpenAPISpec {
			return &OpenAPISpec{Paths: map[string]PathItem{}, Components: &Components{
				Headers: map[string]*Header{"h": {Schema: refTo("T")}}}}
		},
		"nested two deep": func() *OpenAPISpec {
			return specWithSchemas(map[string]*Schema{"Outer": {Type: "array", Items: &Schema{
				Properties: map[string]*Schema{"f": {Type: "array", Items: refTo("T")}}}}})
		},
	}

	for name, build := range cases {
		t.Run(name, func(t *testing.T) {
			var seen []string
			forEachSchemaRef(build(), func(n string) { seen = append(seen, n) })
			if len(seen) != 1 || seen[0] != "T" {
				t.Errorf("walker saw %v, want exactly [T]: a reference here would be left dangling", seen)
			}
		})
	}
}

func TestForEachSchemaRefIgnoresForeignRefs(t *testing.T) {
	spec := specWithSchemas(map[string]*Schema{
		"A": {Ref: "#/components/parameters/NotASchema"},
		"B": {Ref: "https://example.com/other.yaml#/components/schemas/Remote"},
		"C": {Ref: ""},
	})
	var seen []string
	forEachSchemaRef(spec, func(n string) { seen = append(seen, n) })
	if len(seen) != 0 {
		t.Errorf("walker claimed %v; only local components/schemas references are ours to satisfy", seen)
	}
}

// TestForEachSchemaRefTerminatesOnCycles: a self-referential schema is normal
// (a tree node with children of its own type), and the walk must not hang.
func TestForEachSchemaRefTerminatesOnCycles(t *testing.T) {
	node := &Schema{Type: "object", Properties: map[string]*Schema{"child": refTo("Node")}}
	node.Properties["self"] = node

	done := make(chan int, 1)
	go func() {
		n := 0
		forEachSchemaRef(specWithSchemas(map[string]*Schema{"Node": node}), func(string) { n++ })
		done <- n
	}()
	select {
	case n := <-done:
		if n != 1 {
			t.Errorf("saw %d references, want 1", n)
		}
	case <-timeAfterShort():
		t.Fatal("walk did not terminate on a self-referential schema")
	}
}

func TestRepairDanglingRefs(t *testing.T) {
	t.Run("registers a placeholder and reports the Go type", func(t *testing.T) {
		spec := &OpenAPISpec{
			Paths: map[string]PathItem{"/a": {Get: &Operation{
				Responses: map[string]Response{"200": {Content: map[string]MediaType{
					"application/json": {Schema: refTo("github_com_golang-jwt_jwt_v5_RegisteredClaims")}}}},
			}}},
			Components: &Components{Schemas: map[string]*Schema{}},
		}
		usedTypes := map[string]*Schema{"github.com/golang-jwt/jwt/v5.RegisteredClaims": nil}

		got := repairDanglingRefs(spec, usedTypes)

		if len(got) != 1 {
			t.Fatalf("reported %d unresolved refs, want 1: %+v", len(got), got)
		}
		if got[0].GoType != "github.com/golang-jwt/jwt/v5.RegisteredClaims" {
			t.Errorf("GoType = %q; the mangled name does not tell a user which dependency to register", got[0].GoType)
		}
		if got[0].Sites != 1 {
			t.Errorf("Sites = %d, want 1", got[0].Sites)
		}

		// Repaired, not dropped: the reference still resolves.
		placeholder, ok := spec.Components.Schemas["github_com_golang-jwt_jwt_v5_RegisteredClaims"]
		if !ok {
			t.Fatal("no component was registered; the $ref still dangles")
		}
		if !strings.Contains(placeholder.Description, "github.com/golang-jwt/jwt/v5.RegisteredClaims") {
			t.Errorf("placeholder describes %q; it should name the Go type", placeholder.Description)
		}
	})

	t.Run("counts every site", func(t *testing.T) {
		spec := &OpenAPISpec{
			Paths: map[string]PathItem{
				"/a": {Get: &Operation{Responses: map[string]Response{"200": {Content: map[string]MediaType{"application/json": {Schema: refTo("T")}}}}}},
				"/b": {Get: &Operation{Responses: map[string]Response{"200": {Content: map[string]MediaType{"application/json": {Schema: refTo("T")}}}}}},
			},
			Components: &Components{Schemas: map[string]*Schema{}},
		}
		got := repairDanglingRefs(spec, nil)
		if len(got) != 1 || got[0].Sites != 2 {
			t.Errorf("got %+v, want one entry with 2 sites", got)
		}
	})

	t.Run("leaves a resolvable document alone", func(t *testing.T) {
		real := &Schema{Type: "object"}
		spec := specWithSchemas(map[string]*Schema{"T": real, "Outer": {Properties: map[string]*Schema{"f": refTo("T")}}})

		if got := repairDanglingRefs(spec, nil); len(got) != 0 {
			t.Errorf("reported %+v on a document whose refs all resolve", got)
		}
		if spec.Components.Schemas["T"] != real {
			t.Error("an existing component was overwritten")
		}
	})

	t.Run("falls back to the component name when the type is unknown", func(t *testing.T) {
		spec := specWithSchemas(map[string]*Schema{"Outer": {Properties: map[string]*Schema{"f": refTo("Mystery")}}})
		got := repairDanglingRefs(spec, nil)
		if len(got) != 1 || got[0].Component != "Mystery" {
			t.Fatalf("got %+v, want the component reported", got)
		}
		if got[0].GoType != "" {
			t.Errorf("GoType = %q, want empty when it cannot be recovered", got[0].GoType)
		}
		if !strings.Contains(spec.Components.Schemas["Mystery"].Description, "Mystery") {
			t.Error("placeholder should fall back to naming the component")
		}
	})

	t.Run("is deterministic", func(t *testing.T) {
		build := func() *OpenAPISpec {
			return specWithSchemas(map[string]*Schema{"Outer": {AllOf: []*Schema{
				refTo("Zeta"), refTo("Alpha"), refTo("Mid")}}})
		}
		var first []string
		for i := 0; i < 5; i++ {
			var names []string
			for _, r := range repairDanglingRefs(build(), nil) {
				names = append(names, r.Component)
			}
			if i == 0 {
				first = names
				if !sort.StringsAreSorted(names) {
					t.Errorf("report is not sorted: %v — map order would reach the output (golden rule #1)", names)
				}
				continue
			}
			if strings.Join(names, ",") != strings.Join(first, ",") {
				t.Errorf("run %d reported %v, first run reported %v", i, names, first)
			}
		}
	})

	t.Run("handles a spec with no components section", func(t *testing.T) {
		spec := &OpenAPISpec{Paths: map[string]PathItem{"/a": {Get: &Operation{
			Responses: map[string]Response{"200": {Content: map[string]MediaType{"application/json": {Schema: refTo("T")}}}},
		}}}}
		if got := repairDanglingRefs(spec, nil); len(got) != 1 {
			t.Fatalf("got %+v, want the dangling ref reported", got)
		}
		if spec.Components == nil || spec.Components.Schemas["T"] == nil {
			t.Error("the components section should have been created to hold the repair")
		}
	})

	t.Run("nil spec is not a crash", func(t *testing.T) {
		if got := repairDanglingRefs(nil, nil); got != nil {
			t.Errorf("got %+v, want nil", got)
		}
	})
}

func TestGoTypeByComponentName(t *testing.T) {
	got := goTypeByComponentName(map[string]*Schema{
		"github.com/x/y.Thing":  nil,
		"*github.com/x/y.Thing": nil, // pointer spelling mangles to the same name
		"main.User":             nil,
	})
	if got["github_com_x_y_Thing"] != "github.com/x/y.Thing" {
		t.Errorf("qualified type = %q, want the undecorated spelling", got["github_com_x_y_Thing"])
	}
	if got["main_User"] != "main.User" {
		t.Errorf("main.User = %q", got["main_User"])
	}
}
