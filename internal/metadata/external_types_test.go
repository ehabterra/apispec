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

package metadata

import (
	"go/token"
	"go/types"
	"testing"
)

// External-type resolution tests. These build synthesized go/types objects
// instead of loading real source — that's enough to validate the recursive
// walker without dragging the uuid/decimal modules into the test deps.

const testCurrentModulePath = "example.com/me/proj"

// makeExternalNamed returns a Named type living in an external package, with
// the given underlying. Optionally attaches a zero-arg MarshalJSON method so
// hasCustomJSONMarshaler treats it as a custom-marshaling type.
func makeExternalNamed(name string, underlying types.Type, withMarshalJSON bool) *types.Named {
	pkg := types.NewPackage("github.com/some/ext", "ext")
	obj := types.NewTypeName(token.NoPos, pkg, name, nil)
	n := types.NewNamed(obj, underlying, nil)
	if withMarshalJSON {
		fn := types.NewFunc(token.NoPos, pkg, "MarshalJSON",
			types.NewSignatureType(nil, nil, nil, types.NewTuple(),
				types.NewTuple(
					types.NewParam(token.NoPos, pkg, "", types.NewSlice(types.Typ[types.Byte])),
					types.NewParam(token.NoPos, pkg, "", types.Universe.Lookup("error").Type()),
				), false))
		n.AddMethod(fn)
	}
	return n
}

func makeInternalNamed(name string, underlying types.Type) *types.Named {
	pkg := types.NewPackage(testCurrentModulePath+"/types", "types")
	obj := types.NewTypeName(token.NoPos, pkg, name, nil)
	return types.NewNamed(obj, underlying, nil)
}

func TestResolveExternalNamedTypes_TopLevel(t *testing.T) {
	cases := []struct {
		name     string
		in       types.Type
		wantSame bool   // resolver should return input unchanged
		wantStr  string // expected .String() when wantSame=false
	}{
		{
			name:    "external named with MarshalJSON resolves to string",
			in:      makeExternalNamed("UUID", types.NewArray(types.Typ[types.Byte], 16), true),
			wantStr: "string",
		},
		{
			name:    "external named without MarshalJSON resolves to underlying primitive",
			in:      makeExternalNamed("Duration", types.Typ[types.Int64], false),
			wantStr: "int64",
		},
		{
			// go/types prints byte (an alias of uint8) as "uint8" in
			// composite stringification — the analyzer downstream treats
			// both as primitive bytes.
			name:    "external named without MarshalJSON, byte-array underlying",
			in:      makeExternalNamed("Hash", types.NewArray(types.Typ[types.Byte], 32), false),
			wantStr: "[32]uint8",
		},
		{
			name:     "internal named is kept",
			in:       makeInternalNamed("Local", types.NewStruct(nil, nil)),
			wantSame: true,
		},
		{
			name:     "builtin string is kept",
			in:       types.Typ[types.String],
			wantSame: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := resolveExternalNamedTypes(tc.in, testCurrentModulePath)
			if tc.wantSame {
				if got != tc.in {
					t.Errorf("expected identity, got %v", got)
				}
				return
			}
			if got.String() != tc.wantStr {
				t.Errorf("want %q, got %q", tc.wantStr, got.String())
			}
		})
	}
}

func TestResolveExternalNamedTypes_Containers(t *testing.T) {
	uuid := makeExternalNamed("UUID", types.NewArray(types.Typ[types.Byte], 16), true)
	local := makeInternalNamed("Local", types.NewStruct(nil, nil))

	cases := []struct {
		name    string
		in      types.Type
		wantStr string
	}{
		{
			"pointer to external named (uuid) → *string",
			types.NewPointer(uuid),
			"*string",
		},
		{
			"slice of external named (uuid) → []string",
			types.NewSlice(uuid),
			"[]string",
		},
		{
			"array of external named (uuid) → [3]string",
			types.NewArray(uuid, 3),
			"[3]string",
		},
		{
			"map[string]uuid → map[string]string",
			types.NewMap(types.Typ[types.String], uuid),
			"map[string]string",
		},
		{
			"map[uuid]uuid → map[string]string",
			types.NewMap(uuid, uuid),
			"map[string]string",
		},
		{
			"slice of internal named is kept",
			types.NewSlice(local),
			"[]" + testCurrentModulePath + "/types.Local",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := resolveExternalNamedTypes(tc.in, testCurrentModulePath)
			if got.String() != tc.wantStr {
				t.Errorf("want %q, got %q", tc.wantStr, got.String())
			}
		})
	}
}

func TestHasCustomJSONMarshaler(t *testing.T) {
	withMJ := makeExternalNamed("X", types.NewStruct(nil, nil), true)
	withoutMJ := makeExternalNamed("Y", types.NewStruct(nil, nil), false)

	if !hasCustomJSONMarshaler(withMJ) {
		t.Errorf("expected MarshalJSON on X to be detected")
	}
	if hasCustomJSONMarshaler(withoutMJ) {
		t.Errorf("did not expect Y to advertise MarshalJSON")
	}
	if hasCustomJSONMarshaler(nil) {
		t.Errorf("nil named should return false, not panic")
	}
}

func TestIsExternalPackage(t *testing.T) {
	cases := []struct {
		pkgPath string
		want    bool
	}{
		{"strings", false}, // stdlib, no slash/dot → kept as primitive
		// Stdlib paths that contain '/' (encoding/json, database/sql) are
		// flagged external by the existing heuristic. That's fine for the
		// resolver: it walks into them just like any third-party type and
		// either honours MarshalJSON or unwraps to the underlying.
		{"encoding/json", true},
		{"database/sql", true},
		{testCurrentModulePath, false},
		{testCurrentModulePath + "/internal/x", false},
		{"github.com/google/uuid", true},
		{"github.com/me/other-project/foo", true},
	}
	for _, tc := range cases {
		got := isExternalPackage(tc.pkgPath, testCurrentModulePath)
		if got != tc.want {
			t.Errorf("isExternalPackage(%q): want %v got %v", tc.pkgPath, tc.want, got)
		}
	}
}
