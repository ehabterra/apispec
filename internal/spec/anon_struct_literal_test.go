package spec

import (
	"strings"
	"testing"
)

// TestAnonStructLiteralInlined verifies that an anonymous struct type that
// reaches the spec layer as its raw go/types String() form ("struct{...}",
// possibly slice/pointer-wrapped or with a package path glued on) is inlined
// as an object schema instead of being turned into a dangling $ref with an
// invalid braced component name (Redoc "Invalid reference token").
func TestAnonStructLiteralInlined(t *testing.T) {
	const lit = `struct{AssetID string "json:\"asset_id\""; IsRequired bool "json:\"is_required\""}`

	cases := []struct {
		name   string
		goType string
		// wantArray is true when the top-level schema should be an array whose
		// items are the inlined object.
		wantArray bool
	}{
		{name: "bare", goType: lit},
		{name: "pointer", goType: "*" + lit},
		{name: "slice", goType: "[]" + lit, wantArray: true},
		{name: "pkg-prefixed slice", goType: "[]github.com/ehabterra/enigma/services/api/internal/http." + lit, wantArray: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			usedTypes := map[string]*Schema{}
			schema, schemas := mapGoTypeToOpenAPISchema(usedTypes, tc.goType, nil, &APISpecConfig{}, nil)
			if schema == nil {
				t.Fatal("nil schema")
			}

			obj := schema
			if tc.wantArray {
				if schema.Type != "array" || schema.Items == nil {
					t.Fatalf("expected array schema, got %+v", schema)
				}
				obj = schema.Items
			}

			// The object must be inlined, never a $ref.
			if obj.Ref != "" {
				t.Fatalf("anonymous struct was referenced, not inlined: %q", obj.Ref)
			}
			if obj.Type != "object" {
				t.Fatalf("expected object schema, got type %q", obj.Type)
			}
			if obj.Properties["asset_id"] == nil || obj.Properties["asset_id"].Type != "string" {
				t.Errorf("missing/incorrect asset_id property: %+v", obj.Properties)
			}
			if obj.Properties["is_required"] == nil || obj.Properties["is_required"].Type != "boolean" {
				t.Errorf("missing/incorrect is_required property: %+v", obj.Properties)
			}

			// No component schema may be registered under a braced (invalid) name.
			for name := range schemas {
				if strings.Contains(name, "struct{") {
					t.Errorf("registered a component under an invalid braced name: %q", name)
				}
			}
		})
	}
}

// TestAnonStructLiteralSkipsNonSerializedFields verifies that fields which
// encoding/json never serializes — a `json:"-"` tag and unexported fields —
// are omitted from the inlined object schema.
func TestAnonStructLiteralSkipsNonSerializedFields(t *testing.T) {
	const lit = `struct{Name string "json:\"name\""; Secret string "json:\"-\""; internal int}`

	schema, _ := mapGoTypeToOpenAPISchema(map[string]*Schema{}, lit, nil, &APISpecConfig{}, nil)
	if schema == nil || schema.Type != "object" {
		t.Fatalf("expected object schema, got %+v", schema)
	}
	if schema.Properties["name"] == nil {
		t.Errorf("serialized field 'name' is missing: %+v", schema.Properties)
	}
	if _, ok := schema.Properties["Secret"]; ok {
		t.Errorf(`json:"-" field 'Secret' must be skipped: %+v`, schema.Properties)
	}
	if _, ok := schema.Properties["internal"]; ok {
		t.Errorf("unexported field 'internal' must be skipped: %+v", schema.Properties)
	}
	if len(schema.Properties) != 1 {
		t.Errorf("expected exactly 1 property, got %d: %+v", len(schema.Properties), schema.Properties)
	}
}
