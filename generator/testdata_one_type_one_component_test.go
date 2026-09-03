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

// One Go type must produce ONE component, however its name reached the mapper.
//
// A response type recovered from a call carries the package's NAME
// ("api.Issue"); metadata's own strings carry the import PATH
// ("twoname/api.Issue"). Both used to become separate components, and the
// short-keyed one resolved to a DIFFERENT type entirely — a migration's
// function-local `type Issue struct`, found by a bare-name scan over sorted
// packages — so an endpoint documented one field instead of four (issue #457).
func TestTestdata_OneTypeOneComponent(t *testing.T) {
	out := loadTestdata(t, "one_type_one_component", spec.DefaultHTTPConfig())
	noDanglingRefs(t, out)
	noUnresolvedPlaceholders(t, out)

	// Exactly one component for Issue, and it is the fully-qualified one.
	var issueComps []string
	for name := range out.Components.Schemas {
		if strings.HasSuffix(name, "Issue") {
			issueComps = append(issueComps, name)
		}
	}
	if len(issueComps) != 1 {
		t.Fatalf("want exactly one Issue component, got %v", issueComps)
	}
	comp := issueComps[0]
	if !strings.Contains(comp, "twoname_api") {
		t.Errorf("Issue component is %q, want the import-path-qualified name", comp)
	}

	// The real type has four fields; the migration's throwaway struct has one,
	// so this is what catches a resolution that picked the wrong declaration.
	schema := out.Components.Schemas[comp]
	if schema == nil {
		t.Fatalf("component %q missing", comp)
	}
	if got := len(schema.Properties); got != 4 {
		t.Errorf("%s has %d properties, want 4 (%v) — a wrong-package resolution looks exactly like this",
			comp, got, mapSchemaPropKeys(schema.Properties))
	}
	if _, ok := schema.Properties["OnlyColumn"]; ok {
		t.Errorf("%s carries the migration struct's field: resolved the wrong Issue", comp)
	}

	// A package whose declared name differs from its path's last segment (a
	// module major version, `thing` under thing/v2) must resolve to the real
	// type. Reading the name off the path answers "v2", so this route used to
	// document aaamig's one-field struct instead.
	var thingComps []string
	for name := range out.Components.Schemas {
		if strings.HasSuffix(name, "Thing") {
			thingComps = append(thingComps, name)
		}
	}
	if len(thingComps) != 1 {
		t.Fatalf("want exactly one Thing component, got %v", thingComps)
	}
	thingSchema := out.Components.Schemas[thingComps[0]]
	if thingSchema == nil || len(thingSchema.Properties) != 3 {
		t.Errorf("%s has %d properties, want 3 — the version-suffixed package resolved to the wrong type",
			thingComps[0], len(thingSchema.Properties))
	}
	if _, bad := thingSchema.Properties["OnlyColumn"]; bad {
		t.Errorf("%s carries the migration struct's field", thingComps[0])
	}

	// Every route that can resolve a body points at that one component.
	for _, path := range []string{"/direct", "/viaconcrete", "/viapointer", "/viavar", "/viawrapper"} {
		item, ok := out.Paths[path]
		if !ok {
			t.Errorf("missing path %q; have %v", path, mapPathKeys(out.Paths))
			continue
		}
		op := firstOperation(&item)
		if op == nil {
			t.Errorf("%s: no operation", path)
			continue
		}
		found := ""
		for _, resp := range op.Responses {
			for _, media := range resp.Content {
				if media.Schema != nil && media.Schema.Ref != "" {
					found = media.Schema.Ref
				}
			}
		}
		if !strings.HasSuffix(found, comp) {
			t.Errorf("%s response schema is %q, want a $ref to %s", path, found, comp)
		}
	}

	// /viaany stays unresolved on purpose: the concrete type is not recoverable
	// from an `any` return, and guessing one is worse than documenting none.
	//
	// Required, not conditional: if the route or its response disappears, this
	// fixture silently stops testing the behavior it exists for. And ANY $ref
	// fails it, not just one to the Issue component — resolving `any` to some
	// other component would be the same mistake wearing a different name.
	item, ok := out.Paths["/viaany"]
	if !ok {
		t.Fatalf("/viaany missing; have %v", mapPathKeys(out.Paths))
	}
	op := firstOperation(&item)
	if op == nil {
		t.Fatal("/viaany has no operation")
	}
	if len(op.Responses) == 0 {
		t.Fatal("/viaany has no response, so the unresolved-any guard checked nothing")
	}
	for status, resp := range op.Responses {
		for ct, media := range resp.Content {
			if media.Schema != nil && media.Schema.Ref != "" {
				t.Errorf("/viaany %s %s resolved to %q — an `any` return must not be guessed",
					status, ct, media.Schema.Ref)
			}
		}
	}
}

// mapSchemaPropKeys lists a schema's property names for a failure message.
func mapSchemaPropKeys(props map[string]*spec.Schema) []string {
	out := make([]string, 0, len(props))
	for k := range props {
		out = append(out, k)
	}
	return out
}
