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

import "testing"

// TestUntypedConstantDefault pins the mapping to the Go spec's default types
// for untyped constants. These are defined by the language, so each case is
// exact rather than a heuristic.
func TestUntypedConstantDefault(t *testing.T) {
	for in, want := range map[string]string{
		"untyped bool": "bool",
		"untyped int":  "int",
		// rune, not int32: rune IS the default type the Go spec names, and it
		// is an alias for int32, so the mapper maps it to an integer.
		"untyped rune":    "rune",
		"untyped float":   "float64",
		"untyped complex": "complex128",
		"untyped string":  "string",
	} {
		got, ok := untypedConstantDefault(in)
		if !ok {
			t.Errorf("untypedConstantDefault(%q) not recognised", in)
			continue
		}
		if got != want {
			t.Errorf("untypedConstantDefault(%q) = %q, want %q", in, got, want)
		}
	}

	// "untyped nil" has no default type, and ordinary names are not untyped.
	for _, in := range []string{"untyped nil", "bool", "User", "", "untypedbool"} {
		if got, ok := untypedConstantDefault(in); ok {
			t.Errorf("untypedConstantDefault(%q) = %q, want not recognised", in, got)
		}
	}
}

// TestUntypedConstantsMapToSchemas is the reason the mapping exists: each
// default type must reach a real schema. "untyped rune" defaulting to a name
// the mapper does not handle would leave the property with no type at all.
func TestUntypedConstantsMapToSchemas(t *testing.T) {
	for in, want := range map[string]string{
		"untyped bool":   "boolean",
		"untyped int":    "integer",
		"untyped rune":   "integer",
		"untyped float":  "number",
		"untyped string": "string",
	} {
		schema, _ := mapGoTypeToOpenAPISchema(map[string]*Schema{}, in, nil, DefaultAPISpecConfig(), nil)
		if schema == nil {
			t.Errorf("%q produced no schema", in)
			continue
		}
		if schema.Ref != "" {
			t.Errorf("%q produced a $ref (%s); an untyped constant resolves inline", in, schema.Ref)
			continue
		}
		if schema.Type != want {
			t.Errorf("%q = %q, want %q", in, schema.Type, want)
		}
	}

	// rune on its own maps too — it is an alias for int32, and it previously
	// fell through to no schema at all.
	if schema, _ := mapGoTypeToOpenAPISchema(map[string]*Schema{}, "rune", nil, DefaultAPISpecConfig(), nil); schema == nil || schema.Type != "integer" {
		t.Errorf("rune = %+v, want an integer schema", schema)
	}
}

func TestUnqualifyContainer(t *testing.T) {
	pkg := "github.com/x/y"
	for in, want := range map[string]string{
		// A container carries no package, so a qualified one is mis-qualified.
		pkg + TypeSep + "[2]int64":       "[2]int64",
		pkg + TypeSep + "map[string]int": "map[string]int",
		pkg + TypeSep + "[]string":       "[]string",
		// A declared name keeps its package, and unqualified types are untouched.
		pkg + TypeSep + "User": pkg + TypeSep + "User",
		"[2]int64":             "[2]int64",
		"User":                 "User",
		"":                     "",
	} {
		if got := unqualifyContainer(in); got != want {
			t.Errorf("unqualifyContainer(%q) = %q, want %q", in, got, want)
		}
	}
}
