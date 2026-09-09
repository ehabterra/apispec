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
	"testing"

	"github.com/ehabterra/apispec/internal/metadata"
)

// lookupMeta builds metadata where `agent` is present but does NOT declare the
// type, while other packages do — the state a package left in when its types
// were not recorded (issue #447).
func lookupMeta() *metadata.Metadata {
	pool := metadata.NewStringPool()
	field := func(name, typ string) metadata.Field {
		return metadata.Field{Name: pool.Get(name), Type: pool.Get(typ)}
	}
	// TypeInPackage reads a package's types out of its FILES, so that is where
	// a faithful fixture has to put them.
	pkg := func(types map[string]*metadata.Type) *metadata.Package {
		return &metadata.Package{Files: map[string]*metadata.File{"f.go": {Types: types}}}
	}
	return &metadata.Metadata{
		StringPool: pool,
		Packages: map[string]*metadata.Package{
			// Present, no types: the package whose load produced nothing.
			"example.com/m/internal/agent": pkg(map[string]*metadata.Type{}),
			"example.com/m/internal/scheduling": pkg(map[string]*metadata.Type{
				"ConfigRow": {Fields: []metadata.Field{field("cron", "string")}},
			}),
			"example.com/m/internal/worksheet": pkg(map[string]*metadata.Type{
				"Spec":  {Fields: []metadata.Field{field("cells", "int")}},
				"Sheet": {Fields: []metadata.Field{field("rows", "int")}},
			}),
			"example.com/m/internal/assets": pkg(map[string]*metadata.Type{
				"Spec": {Fields: []metadata.Field{field("bytes", "int")}},
			}),
		},
	}
}

// A type asked for BY PACKAGE that the package does not declare must not be
// answered with another package's same-named type.
//
// The component keeps the name of the package that was asked for and gets the
// fields and doc comment of a different one, and nothing in the output says
// so. On a real 280-path service that published five components under one
// package whose contents came from three other packages: 58 property/enum keys
// vanished and 5 descriptions described a different type (issue #447).
func TestTypeByNameRefusesAnotherPackagesType(t *testing.T) {
	meta := lookupMeta()

	if got := typeByName("example.com/m/internal/agent", "ConfigRow", meta); got != nil {
		t.Errorf("agent.ConfigRow resolved to %v — agent does not declare it, and scheduling's "+
			"same-named type is a different type", got.Fields)
	}
	// Unambiguous elsewhere is still not evidence about THIS package.
	if got := typeByName("example.com/m/internal/agent", "Sheet", meta); got != nil {
		t.Errorf("agent.Sheet resolved to %v — the name being unique elsewhere says nothing "+
			"about the package that was asked for", got.Fields)
	}
	// The package that does declare it still resolves.
	if got := typeByName("example.com/m/internal/scheduling", "ConfigRow", meta); got == nil {
		t.Error("scheduling.ConfigRow did not resolve; refusing a substitution must not cost the real lookup")
	}
}

// A qualifier that is a package NAME rather than an import path still has to
// resolve — metadata's own strings carry the full path, but a type recovered
// by rendering an argument carries the package's name (golden rule #3). The
// rule is the one canonicalPackageQualifier uses: exactly one package that
// both fits the name and declares the type.
func TestTypeByNameResolvesAPackageNameQualifier(t *testing.T) {
	meta := lookupMeta()

	got := typeByName("scheduling", "ConfigRow", meta)
	if got == nil {
		t.Fatal("scheduling.ConfigRow (name qualifier) did not resolve")
	}
	if len(got.Fields) != 1 {
		t.Errorf("resolved the wrong type: fields = %v", got.Fields)
	}
	// Ambiguous: two packages declare Spec, and neither is named by the
	// qualifier, so there is nothing to choose between them.
	if got := typeByName("agent", "Spec", meta); got != nil {
		t.Errorf("agent.Spec resolved to %v — no package named agent declares Spec", got.Fields)
	}
}

// With no qualifier at all the bare name is the only evidence there is, so it
// answers only when the name is unambiguous. Two packages declaring `Spec`
// used to resolve to whichever sorted first, which is deterministic but not
// right.
func TestTypeByNameBareNameMustBeUnambiguous(t *testing.T) {
	meta := lookupMeta()

	if got := typeByName("", "ConfigRow", meta); got == nil {
		t.Error("a bare name declared in exactly one package must still resolve")
	}
	if got := typeByName("", "Spec", meta); got != nil {
		t.Errorf("bare Spec resolved to %v — it is declared in two packages, so the answer is a "+
			"coin flip dressed up as determinism", got.Fields)
	}
}
