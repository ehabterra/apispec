package spec

import (
	"testing"

	"github.com/ehabterra/apispec/internal/metadata"
)

// newFactMeta builds a minimal Metadata carrying only external-type facts, which
// is all resolveExternalType consults from metadata.
func newFactMeta(facts map[string]metadata.ExternalTypeFact) *metadata.Metadata {
	return &metadata.Metadata{ExternalTypes: facts}
}

func TestResolveExternalType_Registry(t *testing.T) {
	cases := []struct {
		name       string
		goType     string
		wantType   string
		wantFormat string
	}{
		{"uuid full path", "github.com/google/uuid.UUID", "string", "uuid"},
		{"uuid short", "uuid.UUID", "string", "uuid"},
		{"decimal", "github.com/shopspring/decimal.Decimal", "string", "decimal"},
		{"sql.NullInt64", "database/sql.NullInt64", "integer", "int64"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, _, ok := resolveExternalType(tc.goType, nil, nil, map[string]*Schema{}, map[string]bool{})
			if !ok {
				t.Fatalf("expected %s to be handled", tc.goType)
			}
			if s.Type != tc.wantType || s.Format != tc.wantFormat {
				t.Fatalf("got {%s,%s}, want {%s,%s}", s.Type, s.Format, tc.wantType, tc.wantFormat)
			}
		})
	}
}

func TestResolveExternalType_FactsRule(t *testing.T) {
	t.Run("TextMarshaler is exact string", func(t *testing.T) {
		meta := newFactMeta(map[string]metadata.ExternalTypeFact{
			"x.Email": {Marshaler: metadata.MarshalerText, Underlying: "string"},
		})
		s, _, ok := resolveExternalType("x.Email", nil, meta, map[string]*Schema{}, map[string]bool{})
		if !ok || s.Type != "string" || s.Description != "" {
			t.Fatalf("got %+v, ok=%v", s, ok)
		}
	})

	t.Run("JSONMarshaler is low-confidence string with note", func(t *testing.T) {
		meta := newFactMeta(map[string]metadata.ExternalTypeFact{
			"x.Money": {Marshaler: metadata.MarshalerJSON, Underlying: "struct{...}"},
		})
		s, _, ok := resolveExternalType("x.Money", nil, meta, map[string]*Schema{}, map[string]bool{})
		if !ok || s.Type != "string" || s.Description != lowConfidenceExternalNote {
			t.Fatalf("got %+v, ok=%v", s, ok)
		}
	})

	t.Run("no marshaler + primitive underlying recurses", func(t *testing.T) {
		meta := newFactMeta(map[string]metadata.ExternalTypeFact{
			"x.ID": {Marshaler: metadata.MarshalerNone, Underlying: "string"},
		})
		s, _, ok := resolveExternalType("x.ID", nil, meta, map[string]*Schema{}, map[string]bool{})
		if !ok || s.Type != "string" {
			t.Fatalf("expected primitive underlying to resolve to string, got %+v ok=%v", s, ok)
		}
	})

	t.Run("no marshaler + map underlying recurses to object", func(t *testing.T) {
		// External `type Headers map[string]string` → free-form object is the
		// right resolution (the map's key/value are primitive).
		meta := newFactMeta(map[string]metadata.ExternalTypeFact{
			"x.Headers": {Marshaler: metadata.MarshalerNone, Underlying: "map[string]string"},
		})
		s, _, ok := resolveExternalType("x.Headers", nil, meta, map[string]*Schema{}, map[string]bool{})
		if !ok || s == nil || s.Type != "object" {
			t.Fatalf("map underlying should resolve to object, got %+v ok=%v", s, ok)
		}
	})

	t.Run("no marshaler + struct underlying is left for existing machinery", func(t *testing.T) {
		// gin.Context-style opaque struct: must NOT be inlined as a giant object.
		meta := newFactMeta(map[string]metadata.ExternalTypeFact{
			"gin.Context": {Marshaler: metadata.MarshalerNone, Underlying: "struct{abort bool}"},
		})
		_, _, ok := resolveExternalType("gin.Context", nil, meta, map[string]*Schema{}, map[string]bool{})
		if ok {
			t.Fatalf("expected opaque struct underlying to be left unhandled")
		}
	})

	t.Run("unknown type is not handled", func(t *testing.T) {
		_, _, ok := resolveExternalType("my/pkg.Local", nil, nil, map[string]*Schema{}, map[string]bool{})
		if ok {
			t.Fatalf("expected unknown type to be unhandled")
		}
	})
}

func TestResolveExternalType_ConfigWins(t *testing.T) {
	// A user externalTypes entry must stop the registry from pre-empting it.
	cfg := &APISpecConfig{
		ExternalTypes: []ExternalType{{Name: "uuid.UUID", OpenAPIType: &Schema{Type: "string"}}},
	}
	_, _, ok := resolveExternalType("github.com/google/uuid.UUID", cfg, nil, map[string]*Schema{}, map[string]bool{})
	if ok {
		t.Fatalf("user externalTypes entry should defer registry resolution")
	}
}

func TestLookupConfigSchema_ShortAndFullName(t *testing.T) {
	cfg := &APISpecConfig{
		TypeMapping: []TypeMapping{{GoType: "uuid.UUID", OpenAPIType: &Schema{Type: "string", Format: "uuid"}}},
	}
	// Config written short must match a field typed by full path.
	if s := lookupConfigSchema(cfg, "github.com/google/uuid.UUID"); s == nil || s.Format != "uuid" {
		t.Fatalf("short config entry should match full-path type, got %+v", s)
	}
}

// TestMapGoType_WrappedExternalComposition proves []T / *T compose: the wrapper
// branches recurse into the element, which the registry then resolves.
func TestMapGoType_WrappedExternalComposition(t *testing.T) {
	used := map[string]*Schema{}
	t.Run("slice of uuid", func(t *testing.T) {
		s, _ := mapGoTypeToOpenAPISchema(used, "[]github.com/google/uuid.UUID", nil, nil, map[string]bool{})
		if s == nil || s.Type != "array" || s.Items == nil || s.Items.Format != "uuid" {
			t.Fatalf("[]uuid.UUID should be array of {string,uuid}, got %+v", s)
		}
	})
	t.Run("pointer to uuid", func(t *testing.T) {
		s, _ := mapGoTypeToOpenAPISchema(used, "*github.com/google/uuid.UUID", nil, nil, map[string]bool{})
		if s == nil || s.Type != "string" || s.Format != "uuid" {
			t.Fatalf("*uuid.UUID should be {string,uuid}, got %+v", s)
		}
	})
}
