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

import "testing"

// TestSortedFileScansSkipNilEntries covers the resolution lookups that answer
// from the first file declaring a name (issue #340 made the order sorted rather
// than map-random). They now index Files by sorted name, so a nil file entry —
// which a deserialised metadata.yaml may carry — has to be stepped over instead
// of dereferenced.
func TestSortedFileScansSkipNilEntries(t *testing.T) {
	pool := NewStringPool()
	const pkg = "example.com/app"

	meta := &Metadata{
		StringPool: pool,
		Packages: map[string]*Package{
			pkg: {
				Files: map[string]*File{
					// Sorts first, and is nil: every scan starts here.
					"a_generated.go": nil,
					"b_real.go": {
						Variables: map[string]*Variable{
							"Registry": {Tok: pool.Get("var"), Type: pool.Get("*Store")},
						},
						Functions: map[string]*Function{
							"Serve": {Name: pool.Get("Serve")},
						},
					},
				},
			},
		},
	}

	// traceVariableOriginHelper: the package-level variable is declared in the
	// file after the nil one.
	if _, _, typ, _ := TraceVariableOrigin("Registry", "Serve", pkg, meta); typ == nil {
		t.Error("TraceVariableOrigin skipped the real file after the nil entry")
	}

	// resolveIdentReturnType: same scan, reading the variable's type.
	ret := NewCallArgument(meta)
	ret.SetKind(KindIdent)
	ret.SetName("Registry")
	if got := meta.resolveIdentReturnType(ret, pkg, "Serve"); got != "*Store" {
		t.Errorf("resolveIdentReturnType = %q, want *Store", got)
	}

	// The iterators themselves: an unknown package and a nil file yield
	// nothing, and a caller that stops early stops the walk.
	for range meta.SortedFiles("no/such/pkg") {
		t.Error("SortedFiles yielded a file for an unknown package")
	}
	for range meta.SortedTypes("no/such/pkg", "b_real.go") {
		t.Error("SortedTypes yielded a type for an unknown package")
	}
	for range meta.SortedTypes(pkg, "a_generated.go") {
		t.Error("SortedTypes yielded a type for a nil file")
	}
	files := 0
	for range meta.SortedFiles(pkg) {
		files++
		break
	}
	if files != 1 {
		t.Errorf("SortedFiles kept walking after the caller stopped (%d files)", files)
	}
}

// TestSortedTypesStopsEarly covers the iterator's stop path over a file that
// really has types, which the nil-file cases above cannot reach.
func TestSortedTypesStopsEarly(t *testing.T) {
	pool := NewStringPool()
	const pkg = "example.com/app"
	meta := &Metadata{
		StringPool: pool,
		Packages: map[string]*Package{
			pkg: {Files: map[string]*File{"a.go": {Types: map[string]*Type{
				"Alpha": {}, "Beta": {}, "Gamma": nil,
			}}}},
		},
	}
	var seen []string
	for name := range meta.SortedTypes(pkg, "a.go") {
		seen = append(seen, name)
		if name == "Alpha" {
			break
		}
	}
	if len(seen) != 1 || seen[0] != "Alpha" {
		t.Errorf("SortedTypes yielded %v, want [Alpha] (sorted first, then stopped)", seen)
	}
	// Without stopping, the nil entry is skipped rather than yielded.
	seen = nil
	for name := range meta.SortedTypes(pkg, "a.go") {
		seen = append(seen, name)
	}
	if len(seen) != 2 || seen[0] != "Alpha" || seen[1] != "Beta" {
		t.Errorf("SortedTypes yielded %v, want [Alpha Beta] (Gamma is nil)", seen)
	}
}
