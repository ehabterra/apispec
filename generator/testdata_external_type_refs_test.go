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
	"path/filepath"
	"strings"
	"testing"

	"github.com/ehabterra/apispec/spec"
)

// TestTestdata_ExternalTypeRefs guards issue #325: a field whose type is
// declared outside the analysed module has no metadata entry, so no component
// can be generated for it — but a $ref was emitted anyway and the spec failed
// to resolve in any consumer.
//
// The standard library stands in for a dependency here: it is not part of the
// analysed package set either, which is the only property that matters.
func TestTestdata_ExternalTypeRefs(t *testing.T) {
	out := loadTestdataWithFixtureConfig(t, "external_type_refs", spec.DefaultHTTPConfig())

	// The check this issue is about. noDanglingRefs walks every $ref in the
	// document and asserts a component exists for it.
	noDanglingRefs(t, out)

	// Method as well as path: the fixture registers each handler under one
	// verb, and a route whose method was lost still produces a path entry.
	for _, tc := range []struct{ path, method string }{
		{"/accounts/{id}", "GET"},
		{"/embedded", "POST"},
	} {
		item, ok := out.Paths[tc.path]
		if !ok {
			t.Errorf("path %q missing; have %v", tc.path, mapPathKeys(out.Paths))
			continue
		}
		if opFor(item, tc.method) == nil {
			t.Errorf("%s %s: expected operation, missing", tc.method, tc.path)
		}
	}

	if out.Components == nil || out.Components.Schemas == nil {
		t.Fatal("no components generated")
	}

	// A type declared in the fixture must still resolve to a real schema —
	// the fix must not turn everything into a placeholder.
	var inner *spec.Schema
	for name, s := range out.Components.Schemas {
		if strings.HasSuffix(name, "_Inner") {
			inner = s
		}
	}
	if inner == nil {
		t.Fatal("Inner is declared in the fixture and must have a component")
	}
	if len(inner.Properties) == 0 {
		t.Error("Inner resolved to an empty schema; it should carry its declared fields")
	}
	if _, ok := inner.Properties["label"]; !ok {
		t.Errorf("Inner.label missing; properties: %v", schemaPropNames(inner))
	}

	// The external ones resolve to the honest placeholder rather than a broken
	// pointer. Naming the Go type in the description is what tells a user which
	// dependency to register in externalTypes.
	for _, want := range []string{"net_url_Values", "regexp_Regexp"} {
		s, ok := out.Components.Schemas[want]
		if !ok {
			t.Errorf("component %q missing", want)
			continue
		}
		if !strings.Contains(s.Description, "External or unresolved type") {
			t.Errorf("%s: expected the unresolved-external placeholder, got description %q", want, s.Description)
		}
	}
}

func schemaPropNames(s *spec.Schema) []string {
	var out []string
	for k := range s.Properties {
		out = append(out, k)
	}
	return out
}

// TestTestdata_UnresolvedRefsAreReported covers the end of the chain for issue
// #327: a type with no schema is repaired so the document still loads, AND the
// fact is reported so a user knows their spec has a placeholder in it rather
// than a type.
//
// external_type_refs is the right fixture: its fields are standard-library
// types, which are outside the analysed module exactly like a dependency, so
// they genuinely cannot resolve.
func TestTestdata_UnresolvedRefsAreReported(t *testing.T) {
	dir := filepath.Join("..", "testdata", "external_type_refs")
	g := NewGenerator(spec.DefaultHTTPConfig())
	out, err := g.GenerateFromDirectory(dir)
	if err != nil {
		t.Fatalf("GenerateFromDirectory: %v", err)
	}

	// The document must load regardless — that is what the repair is for.
	noDanglingRefs(t, out)

	// This fixture must report NOTHING, and that is a real assertion rather
	// than a vacuous loop: since #325 the generation pass registers a
	// placeholder for a type it cannot resolve, so nothing should reach the
	// final repair. A non-empty report here means type resolution regressed
	// far enough that the safety net had to catch it — which is exactly the
	// signal worth failing on.
	//
	// The repair itself is exercised directly by unit tests in internal/spec
	// (one per document location, plus its repair and reporting behaviour).
	// Reproducing it end to end would need a fixture that encodes a bug which
	// does not exist, and that would stop reproducing the moment the bug was
	// fixed elsewhere.
	refs := g.UnresolvedRefs()
	if len(refs) != 0 {
		t.Errorf("%d reference(s) needed repairing in a fixture whose types all resolve — "+
			"type resolution regressed: %+v", len(refs), refs)
	}

	// Whatever is reported, the report must describe what was repaired: every
	// component it names now exists. Kept so a future non-empty report is
	// checked rather than only counted.
	for _, r := range refs {
		if r.Component == "" {
			t.Error("an unresolved ref reports no component name")
		}
		if r.Sites < 1 {
			t.Errorf("%s reports %d sites, want at least 1", r.Component, r.Sites)
		}
		if out.Components == nil || out.Components.Schemas[r.Component] == nil {
			t.Errorf("%s was reported unresolved but no placeholder was registered for it", r.Component)
		}
	}
}
