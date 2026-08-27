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

	intspec "github.com/ehabterra/apispec/internal/spec"
	"github.com/ehabterra/apispec/spec"
)

// docComponentFor is schemaBySuffix with a fatal on miss, so each assertion below
// reads as a statement about the schema rather than a nil check.
func docComponentFor(t *testing.T, out *spec.OpenAPISpec, name string) *intspec.Schema {
	t.Helper()
	if out.Components == nil {
		t.Fatal("no components")
	}
	s := schemaBySuffix(out.Components.Schemas, "_"+name)
	if s == nil {
		t.Fatalf("no component schema for %q", name)
	}
	return s
}

// TestTestdata_SchemaDocComments covers issue #366: Go doc comments on a type
// and its fields become the schema's and properties' `description`.
//
// Text is kept VERBATIM, leading identifier and all ("Item is a catalogue
// item."). Stripping it reads better in isolation, but the convention is not
// reliable enough to edit automatically and a wrong edit is worse than a
// slightly redundant sentence (golden rule #7).
func TestTestdata_SchemaDocComments(t *testing.T) {
	out := loadTestdata(t, "schema_doc_comments", spec.DefaultGinConfig())
	noDanglingRefs(t, out)
	noNullSchemas(t, out)

	item := docComponentFor(t, out, "Item")

	// The type's own comment, including the paragraph after the synopsis: a
	// schema has one description field and no summary, so nothing is dropped.
	if !strings.HasPrefix(item.Description, "Item is a catalogue item.") {
		t.Errorf("Item description = %q, want it to start with the doc comment", item.Description)
	}
	if !strings.Contains(item.Description, "second paragraph") {
		t.Errorf("Item description dropped the body paragraph: %q", item.Description)
	}

	want := map[string]string{
		"id":       "ID is the unique identifier of the item.",
		"name":     "Name is the display name.",
		"price":    "Price is in minor units.",
		"quantity": "trailing comments are collected too", // trailing, not a doc block
	}
	for prop, desc := range want {
		p, ok := item.Properties[prop]
		if !ok {
			t.Errorf("property %q missing", prop)
			continue
		}
		if p.Description != desc {
			t.Errorf("property %q description = %q, want %q", prop, p.Description, desc)
		}
	}

	// An undocumented field gets no description rather than an empty one or an
	// inherited one.
	if p, ok := item.Properties["undocked"]; !ok {
		t.Error("property `undocked` missing")
	} else if p.Description != "" {
		t.Errorf("undocumented field got description %q", p.Description)
	}

	// A comment must not resurrect a field encoding/json never serialises.
	if _, ok := item.Properties["secret"]; ok {
		t.Error("`json:\"-\"` field appeared; a doc comment must not override the tag")
	}
	if _, ok := item.Properties["Secret"]; ok {
		t.Error("`json:\"-\"` field appeared under its Go name")
	}

	// Two fields of one type each keep their OWN comment, and the shared
	// component keeps the type's — covered for both a component type
	// (owner/backup, which resolve to $ref) and an inline one (slug/barcode,
	// which resolve to a plain string schema).
	for _, tc := range []struct{ prop, wantPrefix string }{
		{"slug", "Slug is the first"},
		{"barcode", "Barcode is the second"},
		{"dimensions", "Dimensions is an anonymous nested struct"},
	} {
		p, ok := item.Properties[tc.prop]
		if !ok {
			t.Errorf("property %q missing", tc.prop)
			continue
		}
		if !strings.HasPrefix(p.Description, tc.wantPrefix) {
			t.Errorf("property %q description = %q, want it to start with %q",
				tc.prop, p.Description, tc.wantPrefix)
		}
	}
	if item.Properties["slug"].Description == item.Properties["barcode"].Description {
		t.Error("two fields of one inline type share a description — a field comment leaked onto the type")
	}

	owner, ownerOK := item.Properties["owner"]
	backup, backupOK := item.Properties["backup"]
	if !ownerOK || !backupOK {
		t.Fatal("owner/backup properties missing")
	}
	if !strings.HasPrefix(owner.Description, "Owner reuses a documented type") {
		t.Errorf("owner description = %q, want the FIELD's comment", owner.Description)
	}
	if !strings.HasPrefix(backup.Description, "Backup is a second field") {
		t.Errorf("backup description = %q, want the FIELD's comment", backup.Description)
	}
	if owner.Description == backup.Description {
		t.Error("both fields share one description — the field comment leaked onto the shared type schema")
	}
	if got := docComponentFor(t, out, "Owner").Description; !strings.HasPrefix(got, "Owner is the party responsible") {
		t.Errorf("the Owner component description = %q, want the TYPE's own comment "+
			"(a field's comment must not overwrite it)", got)
	}
}

// TestTestdata_SchemaDocCommentsOptOut covers the ExcludeTypeComments switch:
// projects that treat internal comments as private get none of it.
func TestTestdata_SchemaDocCommentsOptOut(t *testing.T) {
	cfg := spec.DefaultGinConfig()
	cfg.ExcludeTypeComments = true
	out := loadTestdata(t, "schema_doc_comments", cfg)

	item := docComponentFor(t, out, "Item")
	if item.Description != "" {
		t.Errorf("type description present with ExcludeTypeComments: %q", item.Description)
	}
	for name, p := range item.Properties {
		if p.Description != "" {
			t.Errorf("property %q kept description %q with ExcludeTypeComments", name, p.Description)
		}
	}
}
