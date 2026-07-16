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

package callgraph

import (
	"go/token"
	"go/types"
	"testing"
)

// covcgNamed builds a *types.Named with the given bare name in a throwaway
// package, so bareRecvName's *types.Named branch can be exercised.
func covcgNamed(name string) *types.Named {
	pkg := types.NewPackage("example.com/covcg", "covcg")
	tn := types.NewTypeName(token.NoPos, pkg, name, nil)
	return types.NewNamed(tn, types.NewStruct(nil, nil), nil)
}

// TestCovcgBareRecvName covers every branch of bareRecvName: pointer receiver,
// named types (which drop package qualifier and type arguments), aliases, and
// the string-fallback path for non-named types (star/bracket trimming).
func TestCovcgBareRecvName(t *testing.T) {
	named := covcgNamed("Widget")
	pkg := types.NewPackage("example.com/covcg2", "covcg2")
	alias := types.NewAlias(types.NewTypeName(token.NoPos, pkg, "Handle", nil), types.Typ[types.Int])

	cases := []struct {
		name string
		in   types.Type
		want string
	}{
		{"named value receiver", named, "Widget"},
		{"named pointer receiver", types.NewPointer(named), "Widget"},
		{"alias receiver", alias, "Handle"},
		{"basic fallback", types.Typ[types.Int], "int"},
		// A pointer-to-pointer derefs once, leaving a leading star that the
		// fallback must trim.
		{"double pointer fallback", types.NewPointer(types.NewPointer(types.Typ[types.Int])), "int"},
		// A slice of a package-qualified named type exercises both the
		// bracket-trim branch and the qualifier closure inside TypeString.
		{"slice fallback trims bracket", types.NewSlice(named), ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := bareRecvName(tc.in); got != tc.want {
				t.Errorf("bareRecvName(%s) = %q, want %q", tc.name, got, tc.want)
			}
		})
	}
}
