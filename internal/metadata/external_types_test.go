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

// External-type fact tests. These build synthesized go/types objects
// instead of loading real source — that's enough to validate the marshaler
// classification without dragging the uuid/decimal modules into the test deps.

const testCurrentModulePath = "example.com/me/proj"

// makeExternalNamed returns a Named type living in an external package, with
// the given underlying. Optionally attaches a zero-arg MarshalJSON method so
// marshalerKind classifies it as controlling its own wire format.
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

// TestMarshalerKind pins the live classification recordExternalTypeFacts
// relies on: MarshalJSON wins over nothing, and a plain named type has no
// marshaler.
func TestMarshalerKind(t *testing.T) {
	withJSON := makeExternalNamed("UUID", types.NewArray(types.Typ[types.Byte], 16), true)
	if got := marshalerKind(withJSON); got != MarshalerJSON {
		t.Errorf("marshalerKind(with MarshalJSON) = %v, want MarshalerJSON", got)
	}
	plain := makeExternalNamed("Duration", types.Typ[types.Int64], false)
	if got := marshalerKind(plain); got != MarshalerNone {
		t.Errorf("marshalerKind(plain) = %v, want MarshalerNone", got)
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
