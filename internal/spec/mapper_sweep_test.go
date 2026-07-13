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
	"go/constant"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ehabterra/apispec/internal/metadata"
	"github.com/ehabterra/apispec/internal/typemodel"
)

// sweepMeta builds a metadata fixture with a small type zoo used across the
// sweep tests: a plain struct, an alias enum, a struct-kind "enum carrier"
// (constants typed after it), and a generic declaration.
func sweepMeta(t *testing.T) (*metadata.Metadata, *metadata.StringPool) {
	t.Helper()
	pool := metadata.NewStringPool()
	meta := &metadata.Metadata{
		StringPool: pool,
		Packages: map[string]*metadata.Package{
			"main": {
				Files: map[string]*metadata.File{
					"main.go": {
						Types: map[string]*metadata.Type{
							"User": {
								Name: pool.Get("User"),
								Pkg:  pool.Get("main"),
								Kind: pool.Get("struct"),
								Fields: []metadata.Field{
									{Name: pool.Get("Name"), Type: pool.Get("string")},
								},
							},
							"Other": {
								Name: pool.Get("Other"),
								Pkg:  pool.Get("main"),
								Kind: pool.Get("struct"),
								Fields: []metadata.Field{
									{Name: pool.Get("ID"), Type: pool.Get("int")},
								},
							},
							// Alias enum: Status -> string.
							"Status": {
								Name:   pool.Get("Status"),
								Pkg:    pool.Get("main"),
								Kind:   pool.Get("alias"),
								Target: pool.Get("string"),
							},
							// Struct-kind type with same-named constants: its
							// underlying type does not resolve to a primitive, so
							// slice/array/map element handling takes the complex
							// path and enum detection still finds the constants.
							"StatusE": {
								Name: pool.Get("StatusE"),
								Pkg:  pool.Get("main"),
								Kind: pool.Get("struct"),
							},
							// Generic declaration Page[T any].
							"Page": {
								Name:       pool.Get("Page"),
								Pkg:        pool.Get("main"),
								Kind:       pool.Get("struct"),
								TypeParams: []string{"T"},
								Fields: []metadata.Field{
									{Name: pool.Get("Data"), Type: pool.Get("T")},
								},
							},
							// A type named like a declared constraint, so the
							// non-primitive-constraint branch of
							// findTypesInMetadata can resolve "T Stringer".
							"T": {
								Name: pool.Get("T"),
								Pkg:  pool.Get("main"),
								Kind: pool.Get("struct"),
							},
							"Stringer": {
								Name: pool.Get("Stringer"),
								Pkg:  pool.Get("main"),
								Kind: pool.Get("interface"),
							},
						},
						Variables: map[string]*metadata.Variable{
							"StatusEActive": {
								Name:          pool.Get("StatusEActive"),
								Tok:           pool.Get("const"),
								Type:          pool.Get("StatusE"),
								ComputedValue: "active",
								GroupIndex:    1,
							},
							"StatusEInactive": {
								Name:          pool.Get("StatusEInactive"),
								Tok:           pool.Get("const"),
								Type:          pool.Get("StatusE"),
								ComputedValue: "inactive",
								GroupIndex:    1,
							},
						},
					},
				},
			},
		},
	}
	return meta, pool
}

func TestLoadAPISpecConfig_ErrorPaths(t *testing.T) {
	dir := t.TempDir()

	tests := []struct {
		name    string
		content string
		missing bool
	}{
		{name: "missing file", missing: true},
		{name: "invalid yaml", content: "info: [unclosed"},
		{
			// A mapping with no identity matcher fails ValidateSecurity.
			name:    "invalid security config",
			content: "securityMappings:\n  - schemes:\n      - bearerAuth: []\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(dir, strings.ReplaceAll(tt.name, " ", "_")+".yaml")
			if !tt.missing {
				if err := os.WriteFile(path, []byte(tt.content), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			cfg, err := LoadAPISpecConfig(path)
			if err == nil {
				t.Fatalf("expected error, got config %+v", cfg)
			}
		})
	}
}

func TestMapMetadataToOpenAPIWithDiagnostics_InfoFallbacks(t *testing.T) {
	meta, _ := sweepMeta(t)
	tree := NewMockTrackerTree(meta, metadata.TrackerLimits{})

	// Description set means cfg.Info wins, but the empty Title/Version must
	// fall back to the GeneratorConfig values.
	cfg := DefaultAPISpecConfig()
	cfg.Info = Info{Description: "configured description"}

	genCfg := GeneratorConfig{OpenAPIVersion: "3.1.1", Title: "Gen Title", APIVersion: "9.9.9"}
	spec, diag, err := MapMetadataToOpenAPIWithDiagnostics(tree, cfg, genCfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if diag == nil {
		t.Fatal("expected non-nil diagnostics")
	}
	if spec.Info.Title != "Gen Title" {
		t.Errorf("Title = %q, want fallback %q", spec.Info.Title, "Gen Title")
	}
	if spec.Info.Version != "9.9.9" {
		t.Errorf("Version = %q, want fallback %q", spec.Info.Version, "9.9.9")
	}
	if spec.Info.Description != "configured description" {
		t.Errorf("Description = %q, want configured value", spec.Info.Description)
	}
}

func TestForEachPathParam_NestedAndUnbalanced(t *testing.T) {
	t.Run("nested braces in pattern", func(t *testing.T) {
		var names, patterns []string
		forEachPathParam("/users/{id:[0-9]{3}}/posts/{slug}", func(name, pattern string) {
			names = append(names, name)
			patterns = append(patterns, pattern)
		})
		if !reflect.DeepEqual(names, []string{"id", "slug"}) {
			t.Errorf("names = %v", names)
		}
		if !reflect.DeepEqual(patterns, []string{"[0-9]{3}", ""}) {
			t.Errorf("patterns = %v", patterns)
		}
	})

	t.Run("unbalanced placeholder stops scan", func(t *testing.T) {
		var names []string
		forEachPathParam("/users/{id", func(name, _ string) { names = append(names, name) })
		if len(names) != 0 {
			t.Errorf("expected no params from unbalanced path, got %v", names)
		}
	})
}

func TestStripParamPatterns_NestedAndUnbalanced(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"nested braces", "/users/{id:[0-9]{3}}/x", "/users/{id}/x"},
		{"unbalanced copies remainder", "/users/{id:[0-9]", "/users/{id:[0-9]"},
		{"plain placeholder", "/a/{b}", "/a/{b}"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := stripParamPatterns(tt.in); got != tt.want {
				t.Errorf("stripParamPatterns(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestAppendDynamicParamRefs_SkipsCoveredNames(t *testing.T) {
	params := []Parameter{
		{Name: "inline", In: "path"},
		{Ref: dynamicParamRef("id")}, // covers "id" via $ref target
	}
	got := appendDynamicParamRefs(params, []string{"id", "inline", "fresh"})
	if len(got) != 3 {
		t.Fatalf("expected 3 params (2 existing + 1 new ref), got %d: %+v", len(got), got)
	}
	if got[2].Ref != dynamicParamRef("fresh") {
		t.Errorf("appended ref = %q, want %q", got[2].Ref, dynamicParamRef("fresh"))
	}
}

func TestAddDynamicPathParamComponents_NilComponents(t *testing.T) {
	// Must be a no-op, not a panic.
	addDynamicPathParamComponents(nil, []*RouteInfo{{DynamicParams: []string{"id"}}})
}

func TestDynamicParamComponentName_Empty(t *testing.T) {
	if got := dynamicParamComponentName(""); got != "PathParam" {
		t.Errorf("dynamicParamComponentName(\"\") = %q, want PathParam", got)
	}
}

func TestRefTargetName(t *testing.T) {
	tests := []struct {
		name string
		ref  string
		want string
	}{
		{"non-parameter ref", "#/components/schemas/User", ""},
		{"bare Param suffix leaves empty key", "#/components/parameters/Param", ""},
		{"round trip", dynamicParamRef("userId"), "userId"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := refTargetName(tt.ref); got != tt.want {
				t.Errorf("refTargetName(%q) = %q, want %q", tt.ref, got, tt.want)
			}
		})
	}
}

func TestIsGenericObjectResponse_NilSchema(t *testing.T) {
	if !isGenericObjectResponse(&ResponseInfo{Schema: nil}) {
		t.Error("nil schema must count as the generic object fallback")
	}
}

func TestFindTypesInMetadata_EdgeCases(t *testing.T) {
	meta, _ := sweepMeta(t)

	t.Run("nil metadata", func(t *testing.T) {
		if got := findTypesInMetadata(nil, "main.User"); got != nil {
			t.Errorf("expected nil for nil metadata, got %v", got)
		}
	})

	t.Run("generic declaration with primitive constraint", func(t *testing.T) {
		got := findTypesInMetadata(meta, "main.Page[T any]")
		if typ, ok := got["main.T-any"]; !ok || typ != nil {
			t.Errorf("expected nil placeholder entry for main.T-any, got %v", got)
		}
		if got["main.Page[T any]"] == nil {
			t.Errorf("expected Page type entry, got %v", got)
		}
	})

	t.Run("generic declaration with named constraint", func(t *testing.T) {
		got := findTypesInMetadata(meta, "main.Page[T Stringer]")
		if typ, ok := got["main.T_Stringer"]; !ok || typ == nil {
			t.Errorf("expected resolved entry for main.T_Stringer, got %v", got)
		}
	})
}

func TestTypeByName_NilMetadata(t *testing.T) {
	if got := typeByName("main", "User", nil); got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

func TestTypeInPackage_NilPackage(t *testing.T) {
	if got := typeInPackage(nil, "User"); got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

func TestGenerateSchemaFromType_VisitedReturnsRef(t *testing.T) {
	meta, pool := sweepMeta(t)
	typ := meta.Packages["main"].Files["main.go"].Types["User"]
	_ = pool

	visited := map[string]bool{"main.User" + generateSchemaFromTypeKey: true}
	schema, _ := generateSchemaFromType(map[string]*Schema{}, "main.User", typ, meta, DefaultAPISpecConfig(), visited)
	if schema == nil || schema.Ref == "" {
		t.Fatalf("expected $ref for already-visited type, got %+v", schema)
	}
}

func TestAllConcreteGenericArgs_NilAndConstraint(t *testing.T) {
	if allConcreteGenericArgs([]*typemodel.TypeRef{nil}) {
		t.Error("nil arg must not count as concrete")
	}
	if allConcreteGenericArgs(nil) {
		t.Error("empty args must not count as concrete")
	}
	constrained := &typemodel.TypeRef{Kind: typemodel.KindNamed, Name: "T", Constraint: "any"}
	if allConcreteGenericArgs([]*typemodel.TypeRef{constrained}) {
		t.Error("declaration-form parameter (with constraint) must not count as concrete")
	}
	concrete := typemodel.Parse("main.User")
	if !allConcreteGenericArgs([]*typemodel.TypeRef{concrete}) {
		t.Error("plain named argument must count as concrete")
	}
	if allConcreteGenericArgs([]*typemodel.TypeRef{concrete, constrained}) {
		t.Error("one constrained parameter poisons the whole list")
	}
}

func TestSubstituteTypeParams(t *testing.T) {
	tests := []struct {
		name      string
		fieldType string
		generics  map[string]string
		want      string
	}{
		{"no generics", "T", nil, "T"},
		{"plain param", "T", map[string]string{"T": "User"}, "User"},
		{"pointer param", "*T", map[string]string{"T": "User"}, "*User"},
		{"slice of pointer param", "[]*T", map[string]string{"T": "User"}, "[]*User"},
		{"unrelated type unchanged", "*Other", map[string]string{"T": "User"}, "*Other"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := substituteTypeParams(tt.fieldType, tt.generics); got != tt.want {
				t.Errorf("substituteTypeParams(%q) = %q, want %q", tt.fieldType, got, tt.want)
			}
		})
	}
}

func TestGenerateStructSchema_DeclarationFormGenerics(t *testing.T) {
	meta, _ := sweepMeta(t)
	page := meta.Packages["main"].Files["main.go"].Types["Page"]

	schema, _ := generateStructSchema(map[string]*Schema{}, "main.Page[T any]", page, meta, DefaultAPISpecConfig(), map[string]bool{})
	if schema == nil || schema.Type != "object" {
		t.Fatalf("expected object schema, got %+v", schema)
	}
	// The parametric field maps through the "T-any" placeholder.
	if schema.Properties["Data"] == nil {
		t.Fatalf("expected Data property, got %+v", schema.Properties)
	}
}

func TestGenerateStructSchema_FieldBranches(t *testing.T) {
	meta, pool := sweepMeta(t)

	typ := &metadata.Type{
		Name: pool.Get("Wrapper"),
		Pkg:  pool.Get("main"),
		Kind: pool.Get("struct"),
		Fields: []metadata.Field{
			// json:"-" fields and unexported fields are skipped.
			{Name: pool.Get("Hidden"), Type: pool.Get("string"), Tag: pool.Get(`json:"-"`)},
			{Name: pool.Get("secret"), Type: pool.Get("string")},
			// Field whose type is pre-marked in usedTypes with a nil schema:
			// forces the $ref + deferred body resolution branch.
			{Name: pool.Get("Other"), Type: pool.Get("Other")},
			// Field resolved via user typeMapping to a complex (object) schema:
			// must be promoted to a component and replaced by a $ref.
			{Name: pool.Get("Meta"), Type: pool.Get("Meta")},
		},
	}

	cfg := DefaultAPISpecConfig()
	cfg.TypeMapping = []TypeMapping{
		{
			GoType: "main.Meta",
			OpenAPIType: &Schema{
				Type:       "object",
				Properties: map[string]*Schema{"k": {Type: "string"}},
			},
		},
	}

	usedTypes := map[string]*Schema{"main.Other": nil}
	schema, schemas := generateStructSchema(usedTypes, "main.Wrapper", typ, meta, cfg, map[string]bool{})
	if schema == nil {
		t.Fatal("expected schema")
	}
	if _, ok := schema.Properties["Hidden"]; ok {
		t.Error(`json:"-" field must be skipped`)
	}
	if _, ok := schema.Properties["secret"]; ok {
		t.Error("unexported field must be skipped")
	}
	other := schema.Properties["Other"]
	if other == nil || other.Ref == "" {
		t.Errorf("expected $ref for pre-marked type, got %+v", other)
	}
	if schemas["main.Other"] == nil {
		t.Error("expected deferred body schema for main.Other to be resolved")
	}
	metaProp := schema.Properties["Meta"]
	if metaProp == nil || metaProp.Ref == "" {
		t.Errorf("expected promoted $ref for typeMapping object schema, got %+v", metaProp)
	}
	if s := schemas["main.Meta"]; s == nil || s.Type != "object" || s.Properties["k"] == nil {
		t.Errorf("expected promoted component for main.Meta, got %+v", s)
	}
}

func TestGenerateStructSchema_NestedTypeNilSchemaFallback(t *testing.T) {
	meta, pool := sweepMeta(t)

	// An externalTypes entry with a nil OpenAPIType makes
	// generateSchemaFromType return nil, exercising the fallback lookup in the
	// nested-type branch.
	cfg := DefaultAPISpecConfig()
	cfg.ExternalTypes = []ExternalType{{Name: "main.Nested", OpenAPIType: nil}}

	typ := &metadata.Type{
		Name: pool.Get("Holder"),
		Pkg:  pool.Get("main"),
		Kind: pool.Get("struct"),
		Fields: []metadata.Field{
			{
				Name: pool.Get("Nested"),
				Type: pool.Get("main.Nested"),
				NestedType: &metadata.Type{
					Name: pool.Get("main.Nested"),
					Kind: pool.Get("struct"),
				},
			},
		},
	}

	schema, _ := generateStructSchema(map[string]*Schema{}, "main.Holder", typ, meta, cfg, map[string]bool{})
	if schema == nil {
		t.Fatal("expected schema")
	}
	if _, ok := schema.Properties["Nested"]; !ok {
		t.Error("expected Nested property to be present (even with nil schema)")
	}
}

func TestGenerateStructSchema_EnumOnArrayAndMapFields(t *testing.T) {
	meta, pool := sweepMeta(t)

	// Both fields carry the enum-backed type name "main.StatusE" as their
	// declared type, with a NestedType alias shaping the schema as an array
	// (Items) or a map (AdditionalProperties). detectEnumFromConstants then
	// routes the values through the array/object switch arms.
	typ := &metadata.Type{
		Name: pool.Get("EnumHolder"),
		Pkg:  pool.Get("main"),
		Kind: pool.Get("struct"),
		Fields: []metadata.Field{
			{
				Name: pool.Get("List"),
				Type: pool.Get("main.StatusE"),
				NestedType: &metadata.Type{
					Name:   pool.Get("main.StatusEList"),
					Kind:   pool.Get("alias"),
					Target: pool.Get("[]string"),
				},
			},
			{
				Name: pool.Get("ByKey"),
				Type: pool.Get("main.StatusE"),
				NestedType: &metadata.Type{
					Name:   pool.Get("main.StatusEMap"),
					Kind:   pool.Get("alias"),
					Target: pool.Get("map[string]string"),
				},
			},
		},
	}

	schema, _ := generateStructSchema(map[string]*Schema{}, "main.EnumHolder", typ, meta, DefaultAPISpecConfig(), map[string]bool{})
	if schema == nil {
		t.Fatal("expected schema")
	}
	list := schema.Properties["List"]
	if list == nil || list.Type != "array" || list.Items == nil {
		t.Fatalf("expected array schema for List, got %+v", list)
	}
	if len(list.Items.Enum) != 2 {
		t.Errorf("expected enum on array items, got %+v", list.Items)
	}
	byKey := schema.Properties["ByKey"]
	if byKey == nil || byKey.Type != "object" || byKey.AdditionalProperties == nil {
		t.Fatalf("expected map schema for ByKey, got %+v", byKey)
	}
	if len(byKey.AdditionalProperties.Enum) != 2 {
		t.Errorf("expected enum on additionalProperties, got %+v", byKey.AdditionalProperties)
	}
}

func TestGenerateSchemas_Branches(t *testing.T) {
	meta, _ := sweepMeta(t)
	cfg := DefaultAPISpecConfig()

	usedTypes := map[string]*Schema{
		// Well-known inline external type: resolves primitive-shaped, no component.
		"uuid.UUID": nil,
		// Unknown non-primitive type: gets the unresolved placeholder.
		"foo.Unknown": nil,
		// Generic declaration: yields a nil-typed "main.T-any" entry whose
		// schema comes from the key's constraint suffix.
		"main.Page[T any]": nil,
	}

	components := Components{Schemas: map[string]*Schema{}}
	generateSchemas(usedTypes, cfg, components, meta)

	if _, ok := components.Schemas["uuid_UUID"]; ok {
		t.Error("primitive-shaped external type must not become a component")
	}
	placeholder := components.Schemas["foo_Unknown"]
	if placeholder == nil || placeholder.Type != "object" || !strings.Contains(placeholder.Description, "foo.Unknown") {
		t.Errorf("expected unresolved placeholder for foo.Unknown, got %+v", placeholder)
	}
	if components.Schemas["main_T-any"] == nil {
		t.Errorf("expected schema for generic constraint placeholder, got keys %v", schemaKeys(components.Schemas))
	}
	if components.Schemas["main_Page_T-any"] == nil {
		t.Errorf("expected schema for Page declaration, got keys %v", schemaKeys(components.Schemas))
	}
}

func schemaKeys(m map[string]*Schema) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func TestResolveUnderlyingType_WrappedPrefixes(t *testing.T) {
	meta, _ := sweepMeta(t)

	tests := []struct {
		name string
		in   string
		want string
	}{
		// The map[ prefix is cut and re-applied around the alias target.
		{"map prefix", "map[Status", "map[string]string"},
		// A slice nested behind the map prefix is consumed too; the map form
		// still wins on reconstruction.
		{"map then slice prefix", "map[[]Status", "map[string]string"},
		{"star prefix", "*Status", "*string"},
		{"not an alias", "User", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveUnderlyingType(tt.in, meta); got != tt.want {
				t.Errorf("resolveUnderlyingType(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestExtractValidationConstraints_PatternAndFormatRules(t *testing.T) {
	// Every rule here maps to a non-empty Pattern.
	patternRules := []string{
		"alpha", "alphanum", "numeric", "alphaunicode", "alphanumunicode",
		"hexadecimal", "hexcolor", "rgb", "rgba", "hsl", "hsla", "json",
		"base64", "base64url", "datetime", "date", "time", "ip", "ipv4",
		"ipv6", "cidr", "cidrv4", "cidrv6", "tcp_addr", "tcp4_addr",
		"tcp6_addr", "udp_addr", "udp4_addr", "udp6_addr", "unix_addr",
		"mac", "hostname", "fqdn", "isbn", "isbn10", "isbn13", "issn",
		"uuid3", "uuid4", "uuid5", "ulid", "ascii", "printascii",
		"multibyte", "datauri", "latitude", "longitude", "ssn",
		"credit_card", "mongodb", "cron",
	}
	for _, rule := range patternRules {
		t.Run(rule, func(t *testing.T) {
			c := extractValidationConstraints(`validate:"` + rule + `"`)
			if c == nil || c.Pattern == "" {
				t.Errorf("rule %q: expected a pattern constraint, got %+v", rule, c)
			}
		})
	}

	t.Run("custom regexp tag", func(t *testing.T) {
		c := extractValidationConstraints(`regexp:"^[a-z]+$"`)
		if c == nil || c.Pattern != "^[a-z]+$" {
			t.Errorf("expected pattern from regexp: tag, got %+v", c)
		}
	})
}

func TestApplyValidationConstraints_Branches(t *testing.T) {
	minLen, maxLen := 2, 8

	t.Run("nil schema is a no-op", func(t *testing.T) {
		applyValidationConstraints(nil, &ValidationConstraints{Required: true})
	})

	t.Run("string length bounds", func(t *testing.T) {
		s := &Schema{Type: "string"}
		applyValidationConstraints(s, &ValidationConstraints{MinLength: &minLen, MaxLength: &maxLen})
		if s.MinLength != 2 || s.MaxLength != 8 {
			t.Errorf("got min/max length %d/%d, want 2/8", s.MinLength, s.MaxLength)
		}
	})

	t.Run("integer length bounds become min/max", func(t *testing.T) {
		s := &Schema{Type: "integer"}
		applyValidationConstraints(s, &ValidationConstraints{MinLength: &minLen, MaxLength: &maxLen})
		if s.Minimum != 2 || s.Maximum != 8 {
			t.Errorf("got minimum/maximum %v/%v, want 2/8", s.Minimum, s.Maximum)
		}
	})
}

func TestExtractEnumValues_TypesConst(t *testing.T) {
	mk := func(name, val string) EnumConstant {
		return EnumConstant{
			Name:  name,
			Value: types.NewConst(token.NoPos, nil, name, types.Typ[types.String], constant.MakeString(val)),
		}
	}
	got := extractEnumValues([]EnumConstant{mk("B", "beta"), mk("A", "alpha")})
	want := []interface{}{"alpha", "beta"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("extractEnumValues = %v, want %v", got, want)
	}
}

func TestTypeMatches_AliasResolvesToTarget(t *testing.T) {
	meta, _ := sweepMeta(t)
	if !typeMatches("Status", "string", meta) {
		t.Error("alias Status should match its underlying type string")
	}
}

func TestMapGoTypeToOpenAPISchema_FixedArrays(t *testing.T) {
	meta, _ := sweepMeta(t)
	cfg := DefaultAPISpecConfig()

	t.Run("element already in usedTypes", func(t *testing.T) {
		usedTypes := map[string]*Schema{"main.User": {Type: "object"}}
		schema, _ := mapGoTypeToOpenAPISchema(usedTypes, "[2]main.User", meta, cfg, nil)
		if schema == nil || schema.Type != "array" || schema.Items == nil || schema.Items.Ref == "" {
			t.Fatalf("expected array of $ref, got %+v", schema)
		}
		if schema.MaxItems != 2 || schema.MinItems != 2 {
			t.Errorf("expected fixed size 2, got max=%d min=%d", schema.MaxItems, schema.MinItems)
		}
	})

	t.Run("element in usedTypes with nil body", func(t *testing.T) {
		usedTypes := map[string]*Schema{"main.User": nil}
		schema, _ := mapGoTypeToOpenAPISchema(usedTypes, "[2]main.User", meta, cfg, nil)
		if schema == nil || schema.Type != "array" || schema.Items == nil || schema.Items.Ref == "" {
			t.Fatalf("expected array of $ref, got %+v", schema)
		}
		if usedTypes["main.User"] == nil {
			t.Error("expected nil body to be resolved and re-marked")
		}
	})

	t.Run("complex element not in usedTypes", func(t *testing.T) {
		schema, schemas := mapGoTypeToOpenAPISchema(map[string]*Schema{}, "[3]main.Other", meta, cfg, nil)
		if schema == nil || schema.Type != "array" || schema.Items == nil {
			t.Fatalf("expected array schema, got %+v", schema)
		}
		if schema.MaxItems != 3 || schema.MinItems != 3 {
			t.Errorf("expected fixed size 3, got max=%d min=%d", schema.MaxItems, schema.MinItems)
		}
		if schemas["main.Other"] == nil {
			t.Errorf("expected component for main.Other, got %v", schemaKeys(schemas))
		}
	})

	t.Run("typeMapping element promoted to component", func(t *testing.T) {
		mappedCfg := DefaultAPISpecConfig()
		mappedCfg.TypeMapping = []TypeMapping{{
			GoType:      "main.Meta",
			OpenAPIType: &Schema{Type: "object", Properties: map[string]*Schema{"k": {Type: "string"}}},
		}}
		schema, schemas := mapGoTypeToOpenAPISchema(map[string]*Schema{}, "[2]main.Meta", meta, mappedCfg, nil)
		if schema == nil || schema.Items == nil || schema.Items.Ref == "" {
			t.Fatalf("expected promoted $ref items, got %+v", schema)
		}
		if schemas["main.Meta"] == nil {
			t.Error("expected promoted component for main.Meta")
		}
	})

	t.Run("array element enum applied to stored component", func(t *testing.T) {
		_, schemas := mapGoTypeToOpenAPISchema(map[string]*Schema{}, "[2]main.StatusE", meta, cfg, nil)
		stored := schemas["main.StatusE"]
		if stored == nil {
			t.Fatalf("expected component for main.StatusE, got %v", schemaKeys(schemas))
		}
		if len(stored.Enum) != 2 {
			t.Errorf("expected 2 enum values on stored component, got %v", stored.Enum)
		}
	})

	t.Run("array enum falls back onto items when no component stored", func(t *testing.T) {
		visited := map[string]bool{"main.StatusE" + mapGoTypeToOpenAPISchemaKey: true}
		schema, _ := mapGoTypeToOpenAPISchema(map[string]*Schema{}, "[2]main.StatusE", meta, cfg, visited)
		if schema == nil || schema.Items == nil {
			t.Fatalf("expected array schema, got %+v", schema)
		}
		if len(schema.Items.Enum) != 2 {
			t.Errorf("expected enum on items, got %+v", schema.Items)
		}
	})
}

func TestMapGoTypeToOpenAPISchema_SliceBranches(t *testing.T) {
	meta, _ := sweepMeta(t)
	cfg := DefaultAPISpecConfig()

	t.Run("element in usedTypes with nil body", func(t *testing.T) {
		usedTypes := map[string]*Schema{"main.Other": nil}
		schema, _ := mapGoTypeToOpenAPISchema(usedTypes, "[]main.Other", meta, cfg, nil)
		if schema == nil || schema.Type != "array" || schema.Items == nil || schema.Items.Ref == "" {
			t.Fatalf("expected array of $ref, got %+v", schema)
		}
		if usedTypes["main.Other"] == nil {
			t.Error("expected nil body to be resolved and re-marked")
		}
	})

	t.Run("typeMapping element promoted to component", func(t *testing.T) {
		mappedCfg := DefaultAPISpecConfig()
		mappedCfg.TypeMapping = []TypeMapping{{
			GoType:      "main.Meta",
			OpenAPIType: &Schema{Type: "object", Properties: map[string]*Schema{"k": {Type: "string"}}},
		}}
		schema, schemas := mapGoTypeToOpenAPISchema(map[string]*Schema{}, "[]main.Meta", meta, mappedCfg, nil)
		if schema == nil || schema.Items == nil || schema.Items.Ref == "" {
			t.Fatalf("expected promoted $ref items, got %+v", schema)
		}
		if schemas["main.Meta"] == nil {
			t.Error("expected promoted component for main.Meta")
		}
	})

	t.Run("enum falls back onto items when no component stored", func(t *testing.T) {
		// Pre-visiting the element makes the recursion return a bare $ref
		// without registering a component, so the detected enum lands on the
		// items schema itself.
		visited := map[string]bool{"main.StatusE" + mapGoTypeToOpenAPISchemaKey: true}
		schema, _ := mapGoTypeToOpenAPISchema(map[string]*Schema{}, "[]main.StatusE", meta, cfg, visited)
		if schema == nil || schema.Items == nil {
			t.Fatalf("expected array schema, got %+v", schema)
		}
		if len(schema.Items.Enum) != 2 {
			t.Errorf("expected enum on items, got %+v", schema.Items)
		}
	})
}

func TestMapGoTypeToOpenAPISchema_MapBranches(t *testing.T) {
	meta, _ := sweepMeta(t)
	cfg := DefaultAPISpecConfig()

	t.Run("typeMapping value promoted to component", func(t *testing.T) {
		mappedCfg := DefaultAPISpecConfig()
		mappedCfg.TypeMapping = []TypeMapping{{
			GoType:      "main.Meta",
			OpenAPIType: &Schema{Type: "object", Properties: map[string]*Schema{"k": {Type: "string"}}},
		}}
		schema, schemas := mapGoTypeToOpenAPISchema(map[string]*Schema{}, "map[string]main.Meta", meta, mappedCfg, nil)
		if schema == nil || schema.Type != "object" || schema.AdditionalProperties == nil || schema.AdditionalProperties.Ref == "" {
			t.Fatalf("expected map with promoted $ref values, got %+v", schema)
		}
		if schemas["main.Meta"] == nil {
			t.Error("expected promoted component for main.Meta")
		}
	})

	t.Run("enum falls back onto additionalProperties", func(t *testing.T) {
		visited := map[string]bool{"main.StatusE" + mapGoTypeToOpenAPISchemaKey: true}
		schema, _ := mapGoTypeToOpenAPISchema(map[string]*Schema{}, "map[string]main.StatusE", meta, cfg, visited)
		if schema == nil || schema.AdditionalProperties == nil {
			t.Fatalf("expected map schema, got %+v", schema)
		}
		if len(schema.AdditionalProperties.Enum) != 2 {
			t.Errorf("expected enum on additionalProperties, got %+v", schema.AdditionalProperties)
		}
	})
}

func TestMapGoTypeToOpenAPISchema_MalformedAnonStructFallback(t *testing.T) {
	meta, _ := sweepMeta(t)
	// Contains "struct{" but has no balanced body: must fall back to a plain
	// object rather than a dangling $ref.
	schema, _ := mapGoTypeToOpenAPISchema(map[string]*Schema{}, "main.struct{", meta, DefaultAPISpecConfig(), nil)
	if schema == nil || schema.Type != "object" || schema.Ref != "" {
		t.Fatalf("expected plain object fallback, got %+v", schema)
	}
}

func TestAnonStructHelpers_EdgeCases(t *testing.T) {
	t.Run("anonStructBody without struct token", func(t *testing.T) {
		if _, ok := anonStructBody("main.User"); ok {
			t.Error("expected ok=false without struct{")
		}
	})

	t.Run("anonStructBody unbalanced", func(t *testing.T) {
		if _, ok := anonStructBody("struct{Name string"); ok {
			t.Error("expected ok=false for unbalanced struct body")
		}
	})

	t.Run("schemaFromAnonStructLiteral unbalanced", func(t *testing.T) {
		meta, _ := sweepMeta(t)
		s, _ := schemaFromAnonStructLiteral(map[string]*Schema{}, "struct{Name string", meta, DefaultAPISpecConfig(), nil)
		if s != nil {
			t.Errorf("expected nil schema for unbalanced literal, got %+v", s)
		}
	})

	t.Run("parseAnonField embedded field", func(t *testing.T) {
		name, fieldType, _, ok := parseAnonField("User")
		if ok || name != "" || fieldType != "User" {
			t.Errorf("expected embedded (ok=false) with type User, got name=%q type=%q ok=%v", name, fieldType, ok)
		}
	})
}
