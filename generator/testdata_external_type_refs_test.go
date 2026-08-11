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

	for _, path := range []string{"/accounts/{id}", "/embedded"} {
		if _, ok := out.Paths[path]; !ok {
			t.Errorf("path %q missing; have %v", path, mapPathKeys(out.Paths))
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
