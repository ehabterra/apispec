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

// TestTestdata_ArrayUntypedSchemas guards issue #326: two type shapes that
// cannot be components were being registered as ones.
//
// A fixed-size array reached the mapper package-qualified (`pkg-->[2]int64`),
// which defeats a prefix test on the whole key, so it looked like a named type
// — gitea's `[][2]int64` heatmap became a component. An untyped constant is not
// a primitive by name ("untyped bool"), so `ok: true` became a component too.
// Since #330 both resolve rather than dangle; this asserts they resolve to the
// right thing.
func TestTestdata_ArrayUntypedSchemas(t *testing.T) {
	out := loadTestdataWithFixtureConfig(t, "array_untyped_schemas", spec.DefaultHTTPConfig())
	noDanglingRefs(t, out)

	// Neither shape may name a component.
	if out.Components != nil {
		for name := range out.Components.Schemas {
			if strings.HasPrefix(name, "untyped") {
				t.Errorf("component %q: an untyped constant has a default type and is not nameable", name)
			}
			if strings.Contains(name, "__2int64") {
				t.Errorf("component %q: a fixed-size array is not nameable", name)
			}
		}
	}

	t.Run("array response resolves structurally", func(t *testing.T) {
		op := opFor(pathItem(t, out, "/heatmap"), "GET")
		if op == nil {
			t.Fatal("GET /heatmap missing")
		}
		body := jsonResponseSchema(t, op)
		// [][2]int64 → array of (array of integer, bounded 2).
		if body.Type != "array" || body.Items == nil {
			t.Fatalf("response = %+v, want an array", body)
		}
		inner := body.Items
		if inner.Ref != "" {
			t.Fatalf("element is a $ref (%s); a fixed-size array has no component", inner.Ref)
		}
		if inner.Type != "array" || inner.Items == nil || inner.Items.Type != "integer" {
			t.Fatalf("element = %+v, want an array of integer", inner)
		}
		if inner.MinItems != 2 || inner.MaxItems != 2 {
			t.Errorf("element bounds = (%d, %d), want (2, 2) from [2]int64", inner.MinItems, inner.MaxItems)
		}
	})

	t.Run("untyped constants take their default type", func(t *testing.T) {
		op := opFor(pathItem(t, out, "/status"), "GET")
		if op == nil {
			t.Fatal("GET /status missing")
		}
		body := jsonResponseSchema(t, op)
		for prop, want := range map[string]string{
			"ok":    "boolean",
			"count": "integer",
			"label": "string",
		} {
			s, ok := body.Properties[prop]
			if !ok {
				t.Errorf("property %q missing", prop)
				continue
			}
			if s.Ref != "" {
				t.Errorf("%s is a $ref (%s); an untyped constant resolves inline", prop, s.Ref)
				continue
			}
			if s.Type != want {
				t.Errorf("%s = %q, want %q", prop, s.Type, want)
			}
		}
	})

	// The shapes that already worked must keep working — a struct field of
	// fixed-array type, the [][]T shape #259 fixed, and a slice of a NAMED
	// array type, which is declared and so is legitimately a component.
	t.Run("struct fields keep their shapes", func(t *testing.T) {
		series := componentEndingWith(t, out, "_Series")
		fixed, ok := series.Properties["fixed"]
		if !ok {
			t.Fatal("Series.fixed missing")
		}
		if fixed.Type != "array" || fixed.MinItems != 3 {
			t.Errorf("Series.fixed = %+v, want a 3-bounded array", fixed)
		}
		nested, ok := series.Properties["nested"]
		if !ok {
			t.Fatal("Series.nested missing")
		}
		if nested.Type != "array" || nested.Items == nil || nested.Items.Type != "array" {
			t.Errorf("Series.nested = %+v, want an array of array ([][]int)", nested)
		}
		named, ok := series.Properties["named"]
		if !ok {
			t.Fatal("Series.named missing")
		}
		if named.Items == nil || named.Items.Ref == "" {
			t.Errorf("Series.named = %+v, want a slice of $ref: Point is a declared name", named)
		}
	})
}

// jsonResponseSchema returns the operation's JSON response body schema.
func jsonResponseSchema(t *testing.T, op *intspec.Operation) *spec.Schema {
	t.Helper()
	for _, code := range []string{"200", "default"} {
		resp, ok := op.Responses[code]
		if !ok {
			continue
		}
		if mt, ok := resp.Content["application/json"]; ok && mt.Schema != nil {
			return mt.Schema
		}
	}
	t.Fatal("no application/json response schema")
	return nil
}

func pathItem(t *testing.T, out *spec.OpenAPISpec, path string) intspec.PathItem {
	t.Helper()
	item, ok := out.Paths[path]
	if !ok {
		t.Fatalf("path %q missing; have %v", path, mapPathKeys(out.Paths))
	}
	return item
}

func componentEndingWith(t *testing.T, out *spec.OpenAPISpec, suffix string) *spec.Schema {
	t.Helper()
	if out.Components == nil {
		t.Fatal("no components")
	}
	for name, s := range out.Components.Schemas {
		if strings.HasSuffix(name, suffix) {
			return s
		}
	}
	t.Fatalf("no component ending %q", suffix)
	return nil
}
