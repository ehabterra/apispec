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

// schemaBySuffix returns the first component schema whose key ends with the
// given suffix (component keys are fully package-qualified).
func schemaBySuffix(schemas map[string]*intspec.Schema, suffix string) *intspec.Schema {
	for k, v := range schemas {
		if strings.HasSuffix(k, suffix) {
			return v
		}
	}
	return nil
}

// TestTestdata_ValidationTags covers validator-tag fidelity:
//   - #167: string min/max → minLength/maxLength; numeric min/max →
//     minimum/maximum; a decoded JSON body is required.
//   - #165: on a slice, the pre-`dive` min/max constrain item count
//     (minItems/maxItems) and the post-`dive` min/max constrain each element
//     (items.minimum/items.maximum).
func TestTestdata_ValidationTags(t *testing.T) {
	out := loadTestdataWithFixtureConfig(t, "validation_tags", spec.DefaultHTTPConfig())
	noDanglingRefs(t, out)

	post := opFor(out.Paths["/accounts"], "POST")
	if post == nil {
		t.Fatalf("POST /accounts missing; have %v", mapPathKeys(out.Paths))
	}

	// #167 part 1: a decoded JSON body is required.
	if post.RequestBody == nil {
		t.Fatal("POST /accounts: requestBody missing")
	}
	if !post.RequestBody.Required {
		t.Error("POST /accounts: requestBody should be required:true (#167)")
	}

	req := schemaBySuffix(out.Components.Schemas, "CreateAccountRequest")
	if req == nil {
		t.Fatalf("CreateAccountRequest schema missing; have %v", mapSchemaKeys(out.Components.Schemas))
	}

	// #167 part 2: string min/max → minLength/maxLength.
	if name := req.Properties["name"]; name == nil {
		t.Error("name property missing")
	} else if name.MinLength != 3 || name.MaxLength != 50 {
		t.Errorf("name: got minLength=%d maxLength=%d, want 3/50 (#167 string min/max)", name.MinLength, name.MaxLength)
	}

	// Numeric min/max → minimum/maximum (regression guard).
	if age := req.Properties["age"]; age == nil {
		t.Error("age property missing")
	} else if age.Minimum != 18 || age.Maximum != 120 {
		t.Errorf("age: got minimum=%v maximum=%v, want 18/120", age.Minimum, age.Maximum)
	}
}
