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

	"github.com/ehabterra/apispec/internal/spec"
)

// A package that does not type-check is skipped by the loader — a real and
// routine state, since a project is often analysed mid-edit — so its types are
// not recorded. What must NOT then happen is that the lookup falls through to a
// bare-name scan and fills the component with a same-named type from somewhere
// else: the component keeps the name of the package that was asked for and gets
// the fields and doc comment of a different one, with nothing in the output
// saying so.
//
// This was found on a real 280-path service, where one package published five
// components filled from three others — 58 property and enum keys vanished and
// 5 descriptions described a different type (issue #447). The fixture makes the
// trigger explicit: `broken` genuinely fails to compile, and `twin` declares
// `Row` too.
func TestTestdata_SkippedPackageType(t *testing.T) {
	out := loadTestdata(t, "skipped_package_type", spec.DefaultHTTPConfig())
	noDanglingRefs(t, out)

	if !hasPath(out, "/rows") || !hasPath(out, "/sheets") {
		t.Fatalf("routes missing; have %v — a skipped package must not cost the routes around it",
			mapPathKeys(out.Paths))
	}

	if out.Components == nil {
		// Fataling here rather than ranging a nil pointer: a panic would replace
		// the diagnostic this test exists to print.
		t.Fatal("no components at all — expected one for broken.Row and one for twin.Row")
	}
	var brokenRow, twinRow *spec.Schema
	for name, schema := range out.Components.Schemas {
		switch {
		case strings.HasSuffix(name, "broken_Row"):
			brokenRow = schema
		case strings.HasSuffix(name, "twin_Row"):
			twinRow = schema
		}
	}

	if brokenRow == nil {
		t.Fatalf("no component for broken.Row; have %v", schemaNames(out))
	}
	// The tell is twin's field: `sheet` belongs to twin.Row and appears nowhere
	// in broken.Row, whose fields are cron and enabled.
	if _, borrowed := brokenRow.Properties["sheet"]; borrowed {
		t.Errorf("broken.Row was filled from twin.Row (it has twin's `sheet` property) — "+
			"the package that was asked for does not declare this type, and another package's "+
			"same-named type is not evidence about it; properties = %v", propertyNames(brokenRow))
	}
	if strings.Contains(brokenRow.Description, "shares the NAME") {
		t.Errorf("broken.Row carries twin.Row's doc comment: %q", brokenRow.Description)
	}

	// Refusing the substitution must not cost the type that IS declared.
	if twinRow == nil {
		t.Fatal("no component for twin.Row")
	}
	if _, ok := twinRow.Properties["sheet"]; !ok {
		t.Errorf("twin.Row lost its own property; properties = %v", propertyNames(twinRow))
	}
}

func schemaNames(out *spec.OpenAPISpec) []string {
	if out.Components == nil {
		return nil
	}
	names := make([]string, 0, len(out.Components.Schemas))
	for n := range out.Components.Schemas {
		names = append(names, n)
	}
	return names
}
