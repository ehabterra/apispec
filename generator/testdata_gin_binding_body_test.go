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
	"slices"
	"testing"

	"github.com/ehabterra/apispec/internal/spec"
)

// TestTestdata_GinBindingBody: gin validates a request body with the `binding`
// key, so a body declared the gin way documents its required fields and
// constraints, beside `validate` and with each key's `dive` kept apart
// (issue #600).
func TestTestdata_GinBindingBody(t *testing.T) {
	out := loadTestdata(t, "gin_binding_body", spec.DefaultGinConfig())
	noDanglingRefs(t, out)

	if op := opFor(out.Paths["/items"], "POST"); op == nil || op.RequestBody == nil {
		t.Fatalf("POST /items lost its request body; have %v", mapPathKeys(out.Paths))
	}

	item := schemaBySuffix(out.Components.Schemas, "_CreateItem")
	if item == nil {
		t.Fatal("no CreateItem component")
	}
	// `binding:"required"` and `validate:"required"` both count, once each and
	// in declaration order; the unvalidated note and the dive-only element
	// rules make nothing required.
	if want := []string{"name", "email", "code", "labels", "address"}; !slices.Equal(item.Required, want) {
		t.Errorf("CreateItem required = %v, want %v", item.Required, want)
	}

	prop := func(name string) *spec.Schema {
		t.Helper()
		p := item.Properties[name]
		if p == nil {
			t.Fatalf("CreateItem has no %q property", name)
		}
		return p
	}
	if p := prop("name"); p.MinLength != 3 {
		t.Errorf("name minLength = %d, want 3 from `binding:\"min=3\"`", p.MinLength)
	}
	if p := prop("email"); p.Format != "email" {
		t.Errorf("email format = %q, want email from `validate:`", p.Format)
	}
	if p := prop("code"); p.MaxLength != 8 {
		t.Errorf("code maxLength = %d, want 8 from `validate:` beside `binding:`", p.MaxLength)
	}
	if p := prop("quantity"); p.Minimum != 1 || p.Maximum != 100 {
		t.Errorf("quantity range = [%v, %v], want [1, 100]", p.Minimum, p.Maximum)
	}
	if p := prop("tags"); p.MaxItems != 5 || p.Items == nil || p.Items.MinLength != 2 {
		t.Errorf("tags = maxItems %d, items %+v; want maxItems 5 and items minLength 2", p.MaxItems, p.Items)
	}
	if p := prop("labels"); p.Items == nil || p.Items.MaxLength != 10 {
		t.Errorf("labels items = %+v, want maxLength 10 from validate's dive", p.Items)
	}

	// The nested type's own component carries its `binding` rules.
	addr := schemaBySuffix(out.Components.Schemas, "_Address")
	if addr == nil {
		t.Fatal("no Address component")
	}
	if !slices.Equal(addr.Required, []string{"city"}) {
		t.Errorf("Address required = %v, want [city]", addr.Required)
	}
	if zip := addr.Properties["zip"]; zip == nil || zip.MinLength != 5 || zip.MaxLength != 5 {
		t.Errorf("zip = %+v, want length 5 from `binding:\"len=5\"`", zip)
	}
}
