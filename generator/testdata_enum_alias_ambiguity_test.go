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
	"reflect"
	"strings"
	"testing"

	intspec "github.com/ehabterra/apispec/internal/spec"
)

// TestTestdata_EnumAliasAmbiguity pins which constants an enum may be built from.
//
// Every named type in this fixture has `string` underneath, and that used to be
// enough to make one type's constants a candidate for another's: `Status` was
// documented with a *different* type's four values and never with its own three.
// Which wrong answer appeared depended on map order, so the same code produced a
// different spec per run (issue #229).
//
// The rule is type identity: constants belong to the type they are declared with.
func TestTestdata_EnumAliasAmbiguity(t *testing.T) {
	out := loadTestdataWithFixtureConfig(t, "enum_alias_ambiguity", nil)
	noDanglingRefs(t, out)
	noUnresolvedPlaceholders(t, out)

	var model *intspec.Schema
	for name, sc := range out.Components.Schemas {
		if strings.HasSuffix(name, "Model") {
			model = sc
		}
	}
	if model == nil {
		t.Fatalf("Model schema missing; have %v", enumComponentNames(out))
	}

	enumOf := func(prop string) []interface{} {
		p := model.Properties[prop]
		if p == nil {
			t.Fatalf("property %q missing; have %v", prop, propNames(model))
		}
		return p.Enum
	}

	// Declared `Status`, a distinct type: its own values, and only those.
	if got, want := enumOf("status"), []interface{}{"active", "paused", "retired"}; !reflect.DeepEqual(got, want) {
		t.Errorf("status enum = %v, want %v — a type is documented with the constants declared for IT", got, want)
	}

	// Declared `ModelType`: the constants written as ModelType, not the equally
	// large ApiFormat group that also has `string` underneath.
	if got, want := enumOf("type"), []interface{}{"caption", "face", "labels", "nsfw"}; !reflect.DeepEqual(got, want) {
		t.Errorf("type enum = %v, want %v — the other group of four is a different type", got, want)
	}

	// A plain `string` field names no type, so no constant group can claim it.
	if got := enumOf("name"); len(got) != 0 {
		t.Errorf("name enum = %v, want none — a plain string field is not an enum", got)
	}
}

// enumComponentNames lists the generated component schema names, for failure output.
func enumComponentNames(out *intspec.OpenAPISpec) []string {
	if out.Components == nil {
		return nil
	}
	names := make([]string, 0, len(out.Components.Schemas))
	for name := range out.Components.Schemas {
		names = append(names, name)
	}
	return names
}
