package spec

import (
	"strings"

	"github.com/ehabterra/apispec/internal/metadata"
)

// This file owns the *policy* for mapping external (third-party) named types to
// OpenAPI schemas. The metadata layer reports only facts (is-external,
// marshaler kind, underlying type — see metadata.ExternalTypeFact); the
// decision of what schema to emit is made here so it can consult user config
// and a built-in registry that the metadata layer must not know about.
//
// Resolution order (resolveExternalType): user config is applied earlier in
// mapGoTypeToOpenAPISchema (so it always wins); then this registry of
// well-known types; then a structural rule driven by the metadata facts.

// wellKnownExternalSchemas seeds the common ecosystem types whose JSON form is
// well established, so they render with a precise OpenAPI format instead of a
// featureless string. This is *data*: adding a library is a one-line change,
// identical in shape to a user typeMapping entry. Keyed by both the full
// import path and the short pkg-qualified name; resolveExternalType also falls
// back to the short form, so listing both is belt-and-suspenders.
var wellKnownExternalSchemas = map[string]*Schema{
	// UUID / ULID libraries — RFC-4122-style strings.
	"github.com/google/uuid.UUID":    {Type: "string", Format: "uuid"},
	"github.com/gofrs/uuid.UUID":     {Type: "string", Format: "uuid"},
	"github.com/satori/go.uuid.UUID": {Type: "string", Format: "uuid"},
	"github.com/oklog/ulid.ULID":     {Type: "string", Format: "uuid"},
	"github.com/oklog/ulid/v2.ULID":  {Type: "string", Format: "uuid"},
	"uuid.UUID":                      {Type: "string", Format: "uuid"},
	"ulid.ULID":                      {Type: "string", Format: "uuid"},

	// Decimal / big-number libraries — string-encoded to preserve precision.
	"github.com/shopspring/decimal.Decimal": {Type: "string", Format: "decimal"},
	"decimal.Decimal":                       {Type: "string", Format: "decimal"},

	// database/sql nullable scalars marshal as the bare value or null.
	"database/sql.NullString":  {Type: "string"},
	"sql.NullString":           {Type: "string"},
	"database/sql.NullBool":    {Type: "boolean"},
	"sql.NullBool":             {Type: "boolean"},
	"database/sql.NullInt32":   {Type: "integer", Format: "int32"},
	"sql.NullInt32":            {Type: "integer", Format: "int32"},
	"database/sql.NullInt64":   {Type: "integer", Format: "int64"},
	"sql.NullInt64":            {Type: "integer", Format: "int64"},
	"database/sql.NullFloat64": {Type: "number", Format: "double"},
	"sql.NullFloat64":          {Type: "number", Format: "double"},
	"database/sql.NullTime":    {Type: "string", Format: "date-time"},
	"sql.NullTime":             {Type: "string", Format: "date-time"},
}

// shortTypeName reduces a full import-path-qualified name
// ("github.com/google/uuid.UUID") to its short pkg-qualified form
// ("uuid.UUID"). Names without a slash are returned unchanged.
func shortTypeName(s string) string {
	if i := strings.LastIndex(s, "/"); i >= 0 {
		return s[i+1:]
	}
	return s
}

// lookupConfigSchema returns the user-configured typeMapping schema for goType,
// matching both the full import path and the short pkg-qualified name on either
// side so a config entry written as "uuid.UUID" still matches a field typed as
// "github.com/google/uuid.UUID" and vice-versa. Returns nil when no entry
// matches. (externalTypes is handled separately — it produces a named
// component + $ref rather than an inline schema — see configHasExternalType.)
func lookupConfigSchema(cfg *APISpecConfig, goType string) *Schema {
	if cfg == nil {
		return nil
	}
	want := shortTypeName(goType)
	for _, m := range cfg.TypeMapping {
		if m.GoType == goType || shortTypeName(m.GoType) == want {
			return m.OpenAPIType
		}
	}
	return nil
}

// configHasExternalType reports whether goType matches a user externalTypes
// entry (by full or short name). Such types are emitted as named components by
// the existing externalTypes path, so the built-in registry must not pre-empt
// them.
func configHasExternalType(cfg *APISpecConfig, goType string) bool {
	if cfg == nil {
		return false
	}
	want := shortTypeName(goType)
	for _, e := range cfg.ExternalTypes {
		if e.Name == goType || shortTypeName(e.Name) == want {
			return true
		}
	}
	return false
}

// lowConfidenceExternalNote is attached as a schema Description when an
// external type's JSON shape had to be guessed, so the guess is visible in the
// emitted spec (the same self-documenting approach as
// unresolvedExternalPlaceholder) and users know they can refine it.
const lowConfidenceExternalNote = "External type with a custom JSON marshaler; " +
	"assumed string — add a typeMapping entry to set a precise schema."

// resolveExternalType decides the schema for an external named type using the
// registry and the metadata facts (user config is handled by the caller before
// this point). Returns handled=false when goType is not a recognised external
// type, so the caller continues with its normal logic. extra carries any
// component schemas produced while resolving an underlying type.
func resolveExternalType(goType string, cfg *APISpecConfig, meta *metadata.Metadata,
	usedTypes map[string]*Schema, visitedTypes map[string]bool) (schema *Schema, extra map[string]*Schema, handled bool) {

	// Only bare named types are resolved here. Wrapped forms ([]T, *T,
	// map[K]V) must fall through to the dedicated wrapper branches in
	// mapGoTypeToOpenAPISchema, which recurse into the element type and
	// re-enter this function on the unwrapped name. (shortTypeName would
	// otherwise strip a leading "[]" together with the package path and
	// mistake "[]pkg.UUID" for "pkg.UUID".)
	if strings.HasPrefix(goType, "[") || strings.HasPrefix(goType, "*") ||
		strings.HasPrefix(goType, "map[") {
		return nil, nil, false
	}

	// A user externalTypes entry owns this type — it is emitted as a named
	// component elsewhere, so the registry must not pre-empt it.
	if configHasExternalType(cfg, goType) {
		return nil, nil, false
	}

	// 1. Built-in registry (data). Try full name then short form.
	if s, ok := wellKnownExternalSchemas[goType]; ok {
		return cloneSchema(s), nil, true
	}
	if s, ok := wellKnownExternalSchemas[shortTypeName(goType)]; ok {
		return cloneSchema(s), nil, true
	}

	// 2. Structural rule driven by metadata facts.
	if meta != nil {
		fact, ok := meta.ExternalTypes[goType]
		if !ok {
			fact, ok = meta.ExternalTypes[shortTypeName(goType)]
		}
		if ok {
			switch fact.Marshaler {
			case metadata.MarshalerText:
				// encoding/json always emits a string for TextMarshaler: exact.
				return &Schema{Type: "string"}, nil, true
			case metadata.MarshalerJSON:
				// JSON kind is not statically knowable; string is the modal
				// guess. Record the guess in the schema so it is visible.
				return &Schema{Type: "string", Description: lowConfidenceExternalNote}, nil, true
			default:
				// No custom marshaler. Derive from the underlying type only
				// when it is primitive (e.g. an external `type ID string`).
				// Non-primitive underlyings (maps like gin.H, opaque framework
				// structs like gin.Context) are left for the existing
				// component/$ref machinery, matching prior behaviour and
				// avoiding huge or meaningless inlined objects.
				u := fact.Underlying
				if u != "" && u != goType && u != shortTypeName(goType) && metadata.IsPrimitiveType(u) {
					s, newSchemas := mapGoTypeToOpenAPISchema(usedTypes, u, meta, cfg, visitedTypes)
					return s, newSchemas, true
				}
			}
		}
	}

	return nil, nil, false
}

// isInlineExternalType reports whether goType is an external type that resolves
// to an inlined, primitive-shaped schema (uuid → {string,uuid}, decimal,
// sql.Null*, an external alias of a primitive, …). Such types have no component
// and must never be referenced via $ref. Used by the mapper's field/element
// fast-paths, which otherwise treat any non-primitive *name* as referenceable —
// a hazard now that external types keep their name instead of being flattened.
func isInlineExternalType(goType string, cfg *APISpecConfig, meta *metadata.Metadata) bool {
	s, _, ok := resolveExternalType(goType, cfg, meta, map[string]*Schema{}, map[string]bool{})
	return ok && isPrimitiveShapedSchema(s)
}

// cloneSchema returns a shallow copy so callers that decorate a registry schema
// (e.g. applying validation constraints to a field) never mutate the shared
// registry entry. Registry schemas are primitive-shaped (no nested maps), so a
// shallow copy is sufficient.
func cloneSchema(s *Schema) *Schema {
	if s == nil {
		return nil
	}
	c := *s
	return &c
}
