package spec

import (
	"fmt"
	"go/types"
	"maps"
	"net/http"
	"os"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"

	"github.com/ehabterra/apispec/internal/metadata"
)

// Regex cache for performance optimization
var (
	mapperRegexCache = make(map[string]*regexp.Regexp)
	mapperRegexMutex sync.RWMutex
)

// getCachedMapperRegex returns a cached compiled regex or compiles and caches a new one
func getCachedMapperRegex(pattern string) *regexp.Regexp {
	mapperRegexMutex.RLock()
	if re, exists := mapperRegexCache[pattern]; exists {
		mapperRegexMutex.RUnlock()
		return re
	}
	mapperRegexMutex.RUnlock()

	mapperRegexMutex.Lock()
	defer mapperRegexMutex.Unlock()

	// Double-check after acquiring write lock
	if re, exists := mapperRegexCache[pattern]; exists {
		return re
	}

	re := regexp.MustCompile(pattern)
	mapperRegexCache[pattern] = re
	return re
}

const (
	refComponentsSchemasPrefix = "#/components/schemas/"
)

var schemaComponentNameReplacer = strings.NewReplacer("/", "_", "-->", ".", " ", "-", "[", "_", "]", "", ", ", "-")

// GeneratorConfig holds generation configuration
type GeneratorConfig struct {
	OpenAPIVersion string `yaml:"openapiVersion"`
	Title          string `yaml:"title"`
	APIVersion     string `yaml:"apiVersion"`
}

// LoadAPISpecConfig loads a APISpecConfig from a YAML file
func LoadAPISpecConfig(path string) (*APISpecConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var config APISpecConfig
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		return nil, err
	}

	return &config, nil
}

// DefaultAPISpecConfig returns a default configuration
func DefaultAPISpecConfig() *APISpecConfig {
	return &APISpecConfig{}
}

// MapMetadataToOpenAPI maps metadata to OpenAPI specification
func MapMetadataToOpenAPI(tree TrackerTreeInterface, cfg *APISpecConfig, genCfg GeneratorConfig) (*OpenAPISpec, error) {
	// Create extractor
	extractor := NewExtractor(tree, cfg)

	// Extract routes
	routes := extractor.ExtractRoutes()

	// Build paths
	paths := buildPathsFromRoutes(routes)

	// Generate component schemas
	components := generateComponentSchemas(tree.GetMetadata(), cfg, routes)

	// Use Info from config if present, else fallback to GeneratorConfig
	var info Info
	if cfg != nil && (cfg.Info.Title != "" || cfg.Info.Description != "" || cfg.Info.Version != "") {
		info = cfg.Info
		if info.Title == "" {
			info.Title = genCfg.Title
		}
		if info.Version == "" {
			info.Version = genCfg.APIVersion
		}
	} else {
		info = Info{Title: genCfg.Title, Version: genCfg.APIVersion}
	}

	// Build OpenAPI spec
	spec := &OpenAPISpec{
		OpenAPI:      genCfg.OpenAPIVersion,
		Info:         info,
		Paths:        paths,
		Components:   &components,
		Servers:      cfg.Servers,
		Security:     cfg.Security,
		Tags:         cfg.Tags,
		ExternalDocs: cfg.ExternalDocs,
	}

	// Fill securitySchemes in components if present in config
	if len(cfg.SecuritySchemes) > 0 {
		if spec.Components == nil {
			spec.Components = &Components{}
		}
		spec.Components.SecuritySchemes = cfg.SecuritySchemes
	}

	return spec, nil
}

// buildPathsFromRoutes builds OpenAPI paths from extracted routes
func buildPathsFromRoutes(routes []RouteInfo) map[string]PathItem {
	paths := make(map[string]PathItem)

	for _, route := range routes {
		// Convert path to OpenAPI format
		openAPIPath := convertPathToOpenAPI(route.Path)

		// Get or create path item
		pathItem, exists := paths[openAPIPath]
		if !exists {
			pathItem = PathItem{}
		}

		var pkg string

		if route.Package != "" {
			pkg = route.Package + "."
		}

		// Create operation
		operation := &Operation{
			OperationID: pkg + strings.Replace(strings.Replace(route.Function, TypeSep, ".", 1), pkg, "", 1),
			Summary:     route.Summary,
			Tags:        route.Tags,
		}

		// Add request body if present
		if route.Request != nil {
			operation.RequestBody = &RequestBody{
				Content: map[string]MediaType{
					route.Request.ContentType: {
						Schema: route.Request.Schema,
					},
				},
			}
		}

		// Add parameters (deduplicated and ensure all path params)
		if len(route.Params) > 0 {
			operation.Parameters = deduplicateParameters(route.Params)
		} else {
			operation.Parameters = nil
		}
		operation.Parameters = ensureAllPathParams(openAPIPath, operation.Parameters)

		// Add responses
		operation.Responses = buildResponses(route.Response)

		// Set operation on path item
		setOperationOnPathItem(&pathItem, route.Method, operation)
		paths[openAPIPath] = pathItem
	}

	return paths
}

// ensureAllPathParams ensures all path parameters in the path are present in the parameters slice
func ensureAllPathParams(openAPIPath string, params []Parameter) []Parameter {
	paramMap := make(map[string]bool)
	for _, p := range params {
		if p.In == "path" {
			paramMap[p.Name] = true
		}
	}
	// Find all {param} in the path
	re := getCachedMapperRegex(`\{([a-zA-Z_][a-zA-Z0-9_]*)\}`)
	matches := re.FindAllStringSubmatch(openAPIPath, -1)
	for _, match := range matches {
		name := match[1]
		if !paramMap[name] {
			// Add default path parameter with warning extension
			params = append(params, Parameter{
				Name:     name,
				In:       "path",
				Required: true,
				Schema:   &Schema{Type: "string"},
				Extensions: map[string]any{
					"x-warning": "This parameter is present in the path but not found in the code.",
				},
			})
		}
	}
	return params
}

// deduplicateParameters removes duplicate parameters by (name, in)
func deduplicateParameters(params []Parameter) []Parameter {
	seen := make(map[string]struct{})
	result := make([]Parameter, 0, len(params))
	for _, p := range params {
		key := p.Name + ":" + p.In
		if _, exists := seen[key]; !exists {
			seen[key] = struct{}{}
			result = append(result, p)
		}
	}
	return result
}

// buildResponses builds OpenAPI responses from response info
func buildResponses(respInfo map[string]*ResponseInfo) map[string]Response {
	responses := make(map[string]Response)

	if respInfo == nil {
		// Default response
		responses["200"] = Response{
			Description: "Success",
			Content: map[string]MediaType{
				"application/json": {
					Schema: &Schema{Type: "object"},
				},
			},
		}
		return responses
	}

	// Add success response
	for statusCode, resp := range respInfo {
		responses[statusCode] = Response{
			Description: http.StatusText(resp.StatusCode),
			Content: map[string]MediaType{
				resp.ContentType: {
					Schema: resp.Schema,
				},
			},
		}
	}

	return responses
}

// setOperationOnPathItem sets an operation on a path item based on HTTP method
func setOperationOnPathItem(item *PathItem, method string, op *Operation) {
	switch strings.ToUpper(method) {
	case "GET":
		item.Get = op
	case "POST":
		item.Post = op
	case "PUT":
		item.Put = op
	case "DELETE":
		item.Delete = op
	case "PATCH":
		item.Patch = op
	case "OPTIONS":
		item.Options = op
	case "HEAD":
		item.Head = op
	}
}

// convertPathToOpenAPI converts a Go path to OpenAPI format
func convertPathToOpenAPI(path string) string {
	// Regular expression to match :param format
	// This matches a colon followed by one or more word characters (letters, digits, underscore)
	re := getCachedMapperRegex(`:([a-zA-Z_][a-zA-Z0-9_]*)`)

	// Replace all matches with {param} format
	result := re.ReplaceAllString(path, "{$1}")

	return result
}

// generateComponentSchemas generates component schemas from metadata
func generateComponentSchemas(meta *metadata.Metadata, cfg *APISpecConfig, routes []RouteInfo) Components {
	components := Components{
		Schemas: make(map[string]*Schema),
	}

	// Collect all types used in routes
	usedTypes := collectUsedTypesFromRoutes(routes)

	// Generate schemas for used types
	generateSchemas(usedTypes, cfg, components, meta)

	return components
}

func generateSchemas(usedTypes map[string]*Schema, cfg *APISpecConfig, components Components, meta *metadata.Metadata) {
	for typeName := range usedTypes {
		// Check external types
		if cfg != nil {
			for _, externalType := range cfg.ExternalTypes {
				if externalType.Name == strings.ReplaceAll(typeName, TypeSep, ".") {
					components.Schemas[schemaComponentNameReplacer.Replace(typeName)] = externalType.OpenAPIType
					continue
				}
			}
		}

		// Find the type in metadata
		typs := findTypesInMetadata(meta, typeName)
		if len(typs) == 0 || typs[typeName] == nil {
			continue
		}

		// Generate schema based on type kind
		for key, typ := range typs {
			var schema *Schema
			var schemas map[string]*Schema

			if typ == nil {
				keyParts := strings.Split(key, "-")
				if len(keyParts) > 1 {
					schema, schemas = mapGoTypeToOpenAPISchema(usedTypes, keyParts[1], meta, cfg, nil)
				}
			} else {
				schema, schemas = generateSchemaFromType(usedTypes, key, typ, meta, cfg, nil)
			}
			if schema != nil {
				components.Schemas[schemaComponentNameReplacer.Replace(key)] = schema
			}
			for schemaKey, newSchema := range schemas {
				components.Schemas[schemaComponentNameReplacer.Replace(schemaKey)] = newSchema
			}

		}
	}
}

// collectUsedTypesFromRoutes collects all types used in routes
func collectUsedTypesFromRoutes(routes []RouteInfo) map[string]*Schema {
	usedTypes := make(map[string]*Schema)

	for _, route := range routes {
		// Add request body types
		if route.Request != nil && route.Request.BodyType != "" {
			// addTypeAndDependenciesWithMetadata(route.Request.BodyType, usedTypes, meta, cfg)
			markUsedType(usedTypes, route.Request.BodyType, nil)
		}

		// Add response types
		for _, res := range route.Response {
			if route.Response != nil && res.BodyType != "" {
				// addTypeAndDependenciesWithMetadata(res.BodyType, usedTypes, meta, cfg)
				markUsedType(usedTypes, res.BodyType, nil)
			}
		}

		// Add parameter types
		for _, param := range route.Params {
			if param.Schema != nil && param.Schema.Ref != "" {
				// Extract type name from ref like "#/components/schemas/TypeName"
				refParts := strings.Split(param.Schema.Ref, "/")
				if len(refParts) > 0 {
					typeName := refParts[len(refParts)-1]
					// addTypeAndDependenciesWithMetadata(typeName, usedTypes, meta, cfg)
					markUsedType(usedTypes, typeName, nil)
				}
			}
		}

		for key, usedType := range route.UsedTypes {
			markUsedType(usedTypes, key, usedType)
		}
	}

	return usedTypes
}

// findTypesInMetadata finds a type in metadata
func findTypesInMetadata(meta *metadata.Metadata, typeName string) map[string]*metadata.Type {
	metaTypes := map[string]*metadata.Type{}

	// Skip primitive types - they don't need to be looked up in metadata
	if isPrimitiveType(typeName) {
		return nil
	}

	// Guard against nil metadata
	if meta == nil {
		return nil
	}

	typeParts := TypeParts(typeName)
	var pkgName string

	if !isPrimitiveType(typeParts.PkgName) && typeParts.PkgName != "" {
		pkgName = typeParts.PkgName + "."
	}

	// Generics
	if len(typeParts.GenericTypes) > 0 {
		for _, part := range typeParts.GenericTypes {
			genericType := strings.Split(part, " ")
			if isPrimitiveType(genericType[1]) {
				metaTypes[pkgName+genericType[0]+"-"+genericType[1]] = nil
			} else {
				genericTypeParts := TypeParts(genericType[0])

				if t := typeByName(genericTypeParts, meta); t != nil {
					metaTypes[pkgName+genericType[0]+"_"+genericType[1]] = t
				}
			}
		}
	}

	if typeName != "" {
		metaTypes[typeName] = typeByName(typeParts, meta)
	}

	return metaTypes
}

func typeByName(typeParts Parts, meta *metadata.Metadata) *metadata.Type {
	if meta == nil {
		return nil
	}

	if typeParts.PkgName != "" && typeParts.TypeName != "" {
		pkgName := typeParts.PkgName

		if pkg, exists := meta.Packages[pkgName]; exists {
			for _, file := range pkg.Files {
				if typ, exists := file.Types[typeParts.TypeName]; exists {
					return typ
				}
			}
		}
	}

	for _, pkg := range meta.Packages {
		for _, file := range pkg.Files {
			if typ, exists := file.Types[typeParts.TypeName]; exists {
				return typ
			}
		}
	}
	return nil
}

type Parts struct {
	PkgName      string
	TypeName     string
	GenericTypes []string
}

func TypeParts(typeName string) Parts {
	parts := Parts{}
	typeParts := strings.Split(typeName, TypeSep)

	if len(typeParts) == 1 {
		lastSep := strings.LastIndex(typeName, defaultSep)
		if lastSep > 0 {
			parts.PkgName = typeName[:lastSep]
			parts.TypeName = typeName[lastSep+1:]
		} else {
			parts.TypeName = typeName
		}
	} else if len(typeParts) > 1 {
		parts.PkgName = typeParts[0]
		parts.TypeName = typeParts[1]
		parts.GenericTypes = typeParts[2:]
	}

	if len(typeParts) == 2 && strings.Contains(typeParts[1], "[") {
		genericParts := strings.Split(typeParts[1], "[")
		if len(genericParts) > 1 {
			parts.TypeName = genericParts[0]
			parts.GenericTypes = []string{genericParts[1][:len(genericParts[1])-1]}
		}
	}

	parts.PkgName = strings.TrimPrefix(parts.PkgName, "*")
	parts.PkgName = strings.TrimPrefix(parts.PkgName, "[]")

	return parts
}

// isPrimitiveType checks if a type is a Go primitive type
func isPrimitiveType(typeName string) bool {
	// Remove pointer prefix for checking
	baseType := strings.TrimPrefix(typeName, "*")

	primitiveTypes := []string{
		"string", "int", "int8", "int16", "int32", "int64",
		"uint", "uint8", "uint16", "uint32", "uint64",
		"float32", "float64", "bool", "byte", "rune",
		"error", "interface{}", "struct{}", "any",
		"complex64", "complex128", "time.Time", "nil",
	}

	if slices.Contains(primitiveTypes, baseType) {
		return true
	}

	// Check for slice/array of primitives
	if after, ok := strings.CutPrefix(baseType, "[]"); ok {
		elementType := after
		if slices.Contains(primitiveTypes, elementType) {
			return true
		}
	}

	// Check for map with primitive key/value
	if strings.HasPrefix(baseType, "map[") {
		endIdx := strings.Index(baseType, "]")
		if endIdx > 4 {
			keyType := baseType[4:endIdx]
			valueType := strings.TrimSpace(baseType[endIdx+1:])

			// If both key and value are primitives, consider it primitive
			keyIsPrimitive := false
			valueIsPrimitive := false

			for _, primitive := range primitiveTypes {
				if keyType == primitive {
					keyIsPrimitive = true
				}
				if valueType == primitive {
					valueIsPrimitive = true
				}
			}

			if keyIsPrimitive && valueIsPrimitive {
				return true
			}
		}
	}

	return false
}

const generateSchemaFromTypeKey = "generateSchemaFromType"

// generateSchemaFromType generates an OpenAPI schema from a metadata type
func generateSchemaFromType(usedTypes map[string]*Schema, key string, typ *metadata.Type, meta *metadata.Metadata, cfg *APISpecConfig, visitedTypes map[string]bool) (*Schema, map[string]*Schema) {
	schemas := map[string]*Schema{}

	if visitedTypes == nil {
		visitedTypes = map[string]bool{}
	}

	derivedKey := strings.TrimPrefix(key, "*")
	if visitedTypes[key+generateSchemaFromTypeKey] && canAddRefSchemaForType(derivedKey) {
		return addRefSchemaForType(key), schemas
	}
	visitedTypes[key+generateSchemaFromTypeKey] = true

	if usedTypes[derivedKey] != nil && canAddRefSchemaForType(derivedKey) {
		schemas[derivedKey] = usedTypes[derivedKey]
		return addRefSchemaForType(derivedKey), schemas
	}

	// Check external types
	if cfg != nil {
		for _, externalType := range cfg.ExternalTypes {
			if externalType.Name == strings.ReplaceAll(derivedKey, TypeSep, ".") {
				markUsedType(usedTypes, derivedKey, externalType.OpenAPIType)
				return externalType.OpenAPIType, schemas
			}
		}
	}

	// Get type kind from string pool
	kind := getStringFromPool(meta, typ.Kind)

	var schema *Schema
	var newSchemas map[string]*Schema

	switch kind {
	case "struct":
		schema, newSchemas = generateStructSchema(usedTypes, key, typ, meta, cfg, visitedTypes)
	case "interface":
		schema = generateInterfaceSchema()
	case "alias":
		schema, newSchemas = generateAliasSchema(usedTypes, typ, meta, cfg, visitedTypes)
	default:
		schema = &Schema{Type: "object"}
	}

	markUsedType(usedTypes, key, schema)

	maps.Copy(schemas, newSchemas)

	return schema, schemas
}

// generateStructSchema generates a schema for a struct type
func generateStructSchema(usedTypes map[string]*Schema, key string, typ *metadata.Type, meta *metadata.Metadata, cfg *APISpecConfig, visitedTypes map[string]bool) (*Schema, map[string]*Schema) {
	schemas := map[string]*Schema{}

	keyParts := TypeParts(key)
	genericTypes := map[string]string{}

	if len(keyParts.GenericTypes) > 0 {
		for _, part := range keyParts.GenericTypes {
			genericType := strings.Split(part, " ")
			genericTypes[genericType[0]] = strings.ReplaceAll(part, " ", "-")
		}
	}

	schema := &Schema{
		Type:       "object",
		Properties: make(map[string]*Schema),
		Required:   []string{},
	}

	pkgName := getStringFromPool(meta, typ.Pkg)

	for _, field := range typ.Fields {
		fieldName := getStringFromPool(meta, field.Name)
		fieldType := getStringFromPool(meta, field.Type)

		if genericType, ok := genericTypes[fieldType]; ok {
			fieldType = genericType
		}

		// Check if fieldType is an alias/enum and resolve to underlying type
		// But don't resolve array or map types as we need the original type for enum detection
		if !strings.HasPrefix(fieldType, "[]") && !strings.Contains(fieldType, "map[") {
			if resolvedType := resolveUnderlyingType(fieldType, meta); resolvedType != "" {
				fieldType = resolvedType
			}
		}

		// Extract JSON tag if present
		jsonName := extractJSONName(getStringFromPool(meta, field.Tag))
		if jsonName != "" {
			fieldName = jsonName
		}

		// Extract validation constraints from struct tag
		validationConstraints := extractValidationConstraints(getStringFromPool(meta, field.Tag))

		// Generate schema for field type
		var fieldSchema *Schema
		var newSchemas map[string]*Schema

		if field.NestedType != nil {
			// Handle nested struct type
			fieldOriginalType := getStringFromPool(meta, field.NestedType.Name)

			fieldSchema, newSchemas = generateSchemaFromType(usedTypes, fieldOriginalType, field.NestedType, meta, cfg, visitedTypes)
			if fieldSchema == nil {
				fieldSchema = newSchemas[fieldOriginalType]
			}

			maps.Copy(schemas, newSchemas)
		} else {
			isPrimitive := isPrimitiveType(fieldType)

			if !isPrimitive && !strings.Contains(fieldType, ".") {
				re := getCachedMapperRegex(`((\[\])?\*?)(.+)$`)
				matches := re.FindStringSubmatch(fieldType)
				if len(matches) >= 4 {
					fieldType = matches[1] + pkgName + "." + matches[3]
				}
			}

			derivedFieldType := strings.TrimPrefix(fieldType, "*")
			// Check if this field type already exists in usedTypes
			if bodySchema, ok := usedTypes[derivedFieldType]; !isPrimitive && ok {
				// Create a reference to the existing schema
				fieldSchema = addRefSchemaForType(derivedFieldType)

				if bodySchema == nil {
					var newBodySchemas map[string]*Schema

					bodySchema, newBodySchemas = mapGoTypeToOpenAPISchema(usedTypes, fieldType, meta, cfg, visitedTypes)
					maps.Copy(schemas, newBodySchemas)
				}
				schemas[derivedFieldType] = bodySchema
				markUsedType(usedTypes, derivedFieldType, bodySchema)

			} else {
				fieldSchema, newSchemas = mapGoTypeToOpenAPISchema(usedTypes, derivedFieldType, meta, cfg, visitedTypes)
				if canAddRefSchemaForType(derivedFieldType) {
					schemas[derivedFieldType] = fieldSchema
					fieldSchema = addRefSchemaForType(derivedFieldType)
				}

				maps.Copy(schemas, newSchemas)
			}
		}

		// Apply validation constraints to the schema
		if validationConstraints != nil {
			applyValidationConstraints(fieldSchema, validationConstraints)

			// Add to required fields if marked as required
			if validationConstraints.Required {
				schema.Required = append(schema.Required, fieldName)
			}
		}

		// Detect and apply enum values from constants if no enum was specified in tags
		// Only apply enum detection for custom types (not built-in types)
		if fieldSchema != nil && len(fieldSchema.Enum) == 0 {
			// Use the original field type before resolution for enum detection
			originalFieldType := getStringFromPool(meta, field.Type)

			// Only detect enums for custom types, not built-in types like string, int, etc.
			if !isPrimitiveType(originalFieldType) {
				if enumValues := detectEnumFromConstants(originalFieldType, pkgName, meta); len(enumValues) > 0 {
					switch fieldSchema.Type {
					case "array":
						fieldSchema.Items.Enum = enumValues
					case "object":
						if fieldSchema.AdditionalProperties != nil {
							fieldSchema.AdditionalProperties.Enum = enumValues
						}
					default:
						fieldSchema.Enum = enumValues
					}

				}
			}
		}

		schema.Properties[fieldName] = fieldSchema
	}

	return schema, schemas
}

// generateInterfaceSchema generates a schema for an interface type
func generateInterfaceSchema() *Schema {
	// For interfaces, we'll create a generic object schema
	// In a more sophisticated implementation, you might analyze interface methods
	return &Schema{
		Type: "object",
	}
}

// generateAliasSchema generates a schema for an alias type
func generateAliasSchema(usedTypes map[string]*Schema, typ *metadata.Type, meta *metadata.Metadata, cfg *APISpecConfig, visitedTypes map[string]bool) (*Schema, map[string]*Schema) {
	underlyingType := getStringFromPool(meta, typ.Target)

	// Get the original type name for enum detection
	originalTypeName := getStringFromPool(meta, typ.Name)

	// Generate the base schema from underlying type
	schema, schemas := mapGoTypeToOpenAPISchema(usedTypes, underlyingType, meta, cfg, visitedTypes)

	// If the underlying type is a primitive (like string), try to detect enum values
	if schema != nil && isPrimitiveType(underlyingType) {
		// Extract package name for enum detection
		pkgName := ""
		if typeParts := TypeParts(originalTypeName); typeParts.PkgName != "" {
			pkgName = typeParts.PkgName
		}

		// Detect enum values for this alias type using the original type name
		if enumValues := detectEnumFromConstants(originalTypeName, pkgName, meta); len(enumValues) > 0 {
			// Apply enum values to the schema
			schema.Enum = enumValues
		}
	}

	return schema, schemas
}

// resolveUnderlyingType resolves the underlying type for alias/enum types
func resolveUnderlyingType(typeName string, meta *metadata.Metadata) string {
	if meta == nil {
		return ""
	}

	var hasArrayPrefix, hasMapPrefix, hasSlicePrefix, hasStarPrefix bool

	if after, ok := strings.CutPrefix(typeName, "[]"); ok {
		typeName = after
		hasArrayPrefix = true
	}
	if after, ok := strings.CutPrefix(typeName, "map["); ok {
		typeName = after
		hasMapPrefix = true
	}
	if after, ok := strings.CutPrefix(typeName, "[]"); ok {
		typeName = after
		hasSlicePrefix = true
	}
	if after, ok := strings.CutPrefix(typeName, "*"); ok {
		typeName = after
		hasStarPrefix = true
	}

	// Find the type in metadata
	typs := findTypesInMetadata(meta, typeName)
	if len(typs) == 0 {
		return ""
	}

	for _, typ := range typs {
		if typ == nil {
			continue
		}

		kind := getStringFromPool(meta, typ.Kind)
		if kind == "alias" {
			// Return the underlying type for alias types (like enums)
			underlyingType := getStringFromPool(meta, typ.Target)
			if hasArrayPrefix {
				return "[]" + underlyingType
			}
			if hasMapPrefix {
				return "map[" + underlyingType + "]" + underlyingType
			}
			if hasSlicePrefix {
				return "[]" + underlyingType
			}
			if hasStarPrefix {
				return "*" + underlyingType
			}
			return underlyingType
		}
	}

	return ""
}

func markUsedType(usedTypes map[string]*Schema, typeName string, markValue *Schema) bool {
	if usedTypes[typeName] != nil {
		return true
	}

	usedTypes[typeName] = markValue

	// Handle pointer types by dereferencing them
	if strings.HasPrefix(typeName, "*") {
		dereferencedType := strings.TrimSpace(typeName[1:])
		// Also add the dereferenced type to used types
		if usedTypes[dereferencedType] == nil {
			usedTypes[dereferencedType] = markValue
		}
	}
	return false
}

// getStringFromPool gets a string from the string pool
func getStringFromPool(meta *metadata.Metadata, idx int) string {
	if meta.StringPool == nil {
		return ""
	}
	return meta.StringPool.GetString(idx)
}

// extractJSONName extracts JSON name from a struct tag
func extractJSONName(tag string) string {
	if tag == "" {
		return ""
	}

	// Simple JSON tag extraction
	// In a more sophisticated implementation, you would use reflection or a proper parser
	if strings.Contains(tag, "json:") {
		parts := strings.Split(tag, "json:")
		if len(parts) > 1 {
			jsonPart := strings.Split(parts[1], " ")[0]
			jsonName := strings.Trim(jsonPart, "\"")
			// Remove ,omitempty and other options
			if idx := strings.Index(jsonName, ","); idx != -1 {
				jsonName = jsonName[:idx]
			}
			if jsonName != "" && jsonName != "-" {
				return jsonName
			}
		}
	}

	return ""
}

// ValidationConstraints represents validation constraints extracted from struct tags
type ValidationConstraints struct {
	MinLength *int
	MaxLength *int
	Min       *float64
	Max       *float64
	Format    string
	Pattern   string
	Required  bool
	Enum      []interface{}
}

// extractValidationConstraints extracts validation constraints from struct tags
func extractValidationConstraints(tag string) *ValidationConstraints {
	if tag == "" {
		return nil
	}

	constraints := &ValidationConstraints{}

	// Parse validate tag (common validation libraries like go-playground/validator)
	if strings.Contains(tag, "validate:") {
		parts := strings.Split(tag, "validate:")
		if len(parts) > 1 {
			validateTag := strings.Trim(parts[1], "\"")

			// Parse common validation rules - improved regex to handle various formats
			// Matches: required, email, min=5, max=10, len=8, regexp=^[a-z]{2,3}$, oneof=val1 val2, etc.
			// This regex captures validation rules more accurately:
			// - Simple rules: required, email, url, etc.
			// - Rules with values: min=5, max=10, len=8
			// - Rules with complex values: regexp=^[a-z]{2,3}$, oneof=val1 val2 val3
			rules := getCachedMapperRegex(`([a-zA-Z_][a-zA-Z0-9_]*(?:=(?:[^,{}]|{[^}]*})*)?)`).FindAllStringSubmatch(validateTag, -1)
			for _, ruleSet := range rules {
				rule := strings.TrimSpace(ruleSet[1])
				if rule == "required" {
					constraints.Required = true
				} else if strings.HasPrefix(rule, "min=") {
					if val, err := strconv.Atoi(strings.TrimPrefix(rule, "min=")); err == nil {
						// For numeric validation, use Min instead of MinLength
						constraints.Min = &[]float64{float64(val)}[0]
					}
				} else if strings.HasPrefix(rule, "max=") {
					if val, err := strconv.Atoi(strings.TrimPrefix(rule, "max=")); err == nil {
						// For numeric validation, use Max instead of MaxLength
						constraints.Max = &[]float64{float64(val)}[0]
					}
				} else if strings.HasPrefix(rule, "len=") {
					// Length validation for strings, arrays, slices
					if val, err := strconv.Atoi(strings.TrimPrefix(rule, "len=")); err == nil {
						constraints.MinLength = &val
						constraints.MaxLength = &val
					}
				} else if strings.HasPrefix(rule, "minlen=") {
					// Minimum length for strings, arrays, slices
					if val, err := strconv.Atoi(strings.TrimPrefix(rule, "minlen=")); err == nil {
						constraints.MinLength = &val
					}
				} else if strings.HasPrefix(rule, "maxlen=") {
					// Maximum length for strings, arrays, slices
					if val, err := strconv.Atoi(strings.TrimPrefix(rule, "maxlen=")); err == nil {
						constraints.MaxLength = &val
					}
				} else if strings.HasPrefix(rule, "regexp=") {
					constraints.Pattern = strings.TrimPrefix(rule, "regexp=")
				} else if strings.HasPrefix(rule, "oneof=") {
					// One of validation - creates enum values
					enumPart := strings.TrimPrefix(rule, "oneof=")
					enumValues := strings.Split(enumPart, " ")
					for _, val := range enumValues {
						constraints.Enum = append(constraints.Enum, strings.TrimSpace(val))
					}
				} else if rule == "email" {
					// Email validation - set pattern
					constraints.Format = `email`
				} else if rule == "url" {
					// URL validation - set pattern
					constraints.Format = `uri`
				} else if rule == "uuid" {
					// UUID validation - set pattern
					constraints.Format = `uuid`
				} else if rule == "alpha" {
					// Alphabetic characters only
					constraints.Pattern = `^[a-zA-Z]+$`
				} else if rule == "alphanum" {
					// Alphanumeric characters only
					constraints.Pattern = `^[a-zA-Z0-9]+$`
				} else if rule == "numeric" {
					// Numeric characters only
					constraints.Pattern = `^[0-9]+$`
				} else if rule == "alphaunicode" {
					// Unicode alphabetic characters only
					constraints.Pattern = `^\p{L}+$`
				} else if rule == "alphanumunicode" {
					// Unicode alphanumeric characters only
					constraints.Pattern = `^[\p{L}\p{N}]+$`
				} else if rule == "hexadecimal" {
					// Hexadecimal characters only
					constraints.Pattern = `^[0-9a-fA-F]+$`
				} else if rule == "hexcolor" {
					// Hex color validation
					constraints.Pattern = `^#([0-9a-fA-F]{3}|[0-9a-fA-F]{6})$`
				} else if rule == "rgb" {
					// RGB color validation
					constraints.Pattern = `^rgb\(\s*([0-9]{1,3})\s*,\s*([0-9]{1,3})\s*,\s*([0-9]{1,3})\s*\)$`
				} else if rule == "rgba" {
					// RGBA color validation
					constraints.Pattern = `^rgba\(\s*([0-9]{1,3})\s*,\s*([0-9]{1,3})\s*,\s*([0-9]{1,3})\s*,\s*([0-9]*(?:\.[0-9]+)?)\s*\)$`
				} else if rule == "hsl" {
					// HSL color validation
					constraints.Pattern = `^hsl\(\s*([0-9]{1,3})\s*,\s*([0-9]{1,3})%\s*,\s*([0-9]{1,3})%\s*\)$`
				} else if rule == "hsla" {
					// HSLA color validation
					constraints.Pattern = `^hsla\(\s*([0-9]{1,3})\s*,\s*([0-9]{1,3})%\s*,\s*([0-9]{1,3})%\s*,\s*([0-9]*(?:\.[0-9]+)?)\s*\)$`
				} else if rule == "json" {
					// JSON validation - basic pattern
					constraints.Pattern = `^[\s\S]*$` // JSON is complex, this is a basic check
				} else if rule == "base64" {
					// Base64 validation
					constraints.Pattern = `^[A-Za-z0-9+/]*={0,2}$`
				} else if rule == "base64url" {
					// Base64URL validation
					constraints.Pattern = `^[A-Za-z0-9_-]*$`
				} else if rule == "datetime" {
					// DateTime validation (RFC3339)
					constraints.Pattern = `^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?(Z|[+-]\d{2}:\d{2})$`
				} else if rule == "date" {
					// Date validation (YYYY-MM-DD)
					constraints.Pattern = `^\d{4}-\d{2}-\d{2}$`
				} else if rule == "time" {
					// Time validation (HH:MM:SS)
					constraints.Pattern = `^\d{2}:\d{2}:\d{2}$`
				} else if rule == "ip" {
					// IP address validation
					constraints.Pattern = `^(?:(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.){3}(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)$`
				} else if rule == "ipv4" {
					// IPv4 address validation
					constraints.Pattern = `^(?:(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.){3}(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)$`
				} else if rule == "ipv6" {
					// IPv6 address validation
					constraints.Pattern = `^(?:[0-9a-fA-F]{1,4}:){7}[0-9a-fA-F]{1,4}$`
				} else if rule == "cidr" {
					// CIDR validation
					constraints.Pattern = `^(?:(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.){3}(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\/(?:[0-9]|[1-2][0-9]|3[0-2])$`
				} else if rule == "cidrv4" {
					// CIDRv4 validation
					constraints.Pattern = `^(?:(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.){3}(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\/(?:[0-9]|[1-2][0-9]|3[0-2])$`
				} else if rule == "cidrv6" {
					// CIDRv6 validation
					constraints.Pattern = `^(?:[0-9a-fA-F]{1,4}:){7}[0-9a-fA-F]{1,4}\/(?:[0-9]|[1-9][0-9]|1[0-2][0-8])$`
				} else if rule == "tcp_addr" {
					// TCP address validation
					constraints.Pattern = `^[a-zA-Z0-9.-]+:\d+$`
				} else if rule == "tcp4_addr" {
					// TCP4 address validation
					constraints.Pattern = `^(?:(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.){3}(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?):\d+$`
				} else if rule == "tcp6_addr" {
					// TCP6 address validation
					constraints.Pattern = `^\[(?:[0-9a-fA-F]{1,4}:){7}[0-9a-fA-F]{1,4}\]:\d+$`
				} else if rule == "udp_addr" {
					// UDP address validation
					constraints.Pattern = `^[a-zA-Z0-9.-]+:\d+$`
				} else if rule == "udp4_addr" {
					// UDP4 address validation
					constraints.Pattern = `^(?:(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.){3}(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?):\d+$`
				} else if rule == "udp6_addr" {
					// UDP6 address validation
					constraints.Pattern = `^\[(?:[0-9a-fA-F]{1,4}:){7}[0-9a-fA-F]{1,4}\]:\d+$`
				} else if rule == "unix_addr" {
					// Unix address validation
					constraints.Pattern = `^[a-zA-Z0-9._/-]+$`
				} else if rule == "mac" {
					// MAC address validation
					constraints.Pattern = `^([0-9A-Fa-f]{2}[:-]){5}([0-9A-Fa-f]{2})$`
				} else if rule == "hostname" {
					// Hostname validation
					constraints.Pattern = `^[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$`
				} else if rule == "fqdn" {
					// FQDN validation
					constraints.Pattern = `^[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*\.$`
				} else if rule == "isbn" {
					// ISBN validation
					constraints.Pattern = `^(?:ISBN(?:-1[03])?:? )?(?=[0-9X]{10}$|(?=(?:[0-9]+[- ]){3})[- 0-9X]{13}$|97[89][0-9]{10}$|(?=(?:[0-9]+[- ]){4})[- 0-9]{17}$)(?:97[89][- ]?)?[0-9]{1,5}[- ]?[0-9]+[- ]?[0-9]+[- ]?[0-9X]$`
				} else if rule == "isbn10" {
					// ISBN-10 validation
					constraints.Pattern = `^(?:ISBN(?:-10)?:? )?(?=[0-9X]{10}$|(?=(?:[0-9]+[- ]){3})[- 0-9X]{13}$)[0-9]{1,5}[- ]?[0-9]+[- ]?[0-9]+[- ]?[0-9X]$`
				} else if rule == "isbn13" {
					// ISBN-13 validation
					constraints.Pattern = `^(?:ISBN(?:-13)?:? )?(?=[0-9]{13}$|(?=(?:[0-9]+[- ]){4})[- 0-9]{17}$)97[89][- ]?[0-9]{1,5}[- ]?[0-9]+[- ]?[0-9]+[- ]?[0-9]$`
				} else if rule == "issn" {
					// ISSN validation
					constraints.Pattern = `^[0-9]{4}-[0-9]{3}[0-9X]$`
				} else if rule == "uuid3" {
					// UUID v3 validation
					constraints.Pattern = `^[0-9a-f]{8}-[0-9a-f]{4}-3[0-9a-f]{3}-[0-9a-f]{4}-[0-9a-f]{12}$`
				} else if rule == "uuid4" {
					// UUID v4 validation
					constraints.Pattern = `^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`
				} else if rule == "uuid5" {
					// UUID v5 validation
					constraints.Pattern = `^[0-9a-f]{8}-[0-9a-f]{4}-5[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`
				} else if rule == "ulid" {
					// ULID validation
					constraints.Pattern = `^[0-9A-HJKMNP-TV-Z]{26}$`
				} else if rule == "ascii" {
					// ASCII validation
					constraints.Pattern = `^[\x00-\x7F]*$`
				} else if rule == "printascii" {
					// Printable ASCII validation
					constraints.Pattern = `^[\x20-\x7E]*$`
				} else if rule == "multibyte" {
					// Multibyte validation
					constraints.Pattern = `^[\x00-\x7F]*$`
				} else if rule == "datauri" {
					// Data URI validation
					constraints.Pattern = `^data:([a-z]+\/[a-z0-9\-\+]+(;[a-z0-9\-\+]+\=[a-z0-9\-\+]+)?)?(;base64)?,([a-z0-9\!\$\&\'\(\)\*\+\,\;\=\-\.\_\~\:\@\/\?\%\s]*)$`
				} else if rule == "latitude" {
					// Latitude validation
					constraints.Pattern = `^[-+]?([1-8]?\d(\.\d+)?|90(\.0+)?)$`
				} else if rule == "longitude" {
					// Longitude validation
					constraints.Pattern = `^[-+]?(180(\.0+)?|((1[0-7]\d)|([1-9]?\d))(\.\d+)?)$`
				} else if rule == "ssn" {
					// SSN validation
					constraints.Pattern = `^\d{3}-?\d{2}-?\d{4}$`
				} else if rule == "credit_card" {
					// Credit card validation
					constraints.Pattern = `^(?:4[0-9]{12}(?:[0-9]{3})?|5[1-5][0-9]{14}|3[47][0-9]{13}|3[0-9]{13}|6(?:011|5[0-9]{2})[0-9]{12})$`
				} else if rule == "mongodb" {
					// MongoDB ObjectID validation
					constraints.Pattern = `^[0-9a-fA-F]{24}$`
				} else if rule == "cron" {
					// Cron expression validation
					constraints.Pattern = `^(\*|([0-5]?\d)) (\*|([01]?\d|2[0-3])) (\*|([012]?\d|3[01])) (\*|([0]?\d|1[0-2])) (\*|([0-6]))$`
				}
			}
		}
	}

	// Parse custom validation tags
	if strings.Contains(tag, "min:") {
		parts := strings.Split(tag, "min:")
		if len(parts) > 1 {
			minPart := strings.Split(parts[1], " ")[0]
			if val, err := strconv.ParseFloat(strings.Trim(minPart, "\""), 64); err == nil {
				constraints.Min = &val
			}
		}
	}

	if strings.Contains(tag, "max:") {
		parts := strings.Split(tag, "max:")
		if len(parts) > 1 {
			maxPart := strings.Split(parts[1], " ")[0]
			if val, err := strconv.ParseFloat(strings.Trim(maxPart, "\""), 64); err == nil {
				constraints.Max = &val
			}
		}
	}

	if strings.Contains(tag, "regexp:") {
		parts := strings.Split(tag, "regexp:")
		if len(parts) > 1 {
			patternPart := strings.Split(parts[1], " ")[0]
			constraints.Pattern = strings.Trim(patternPart, "\"")
		}
	}

	if strings.Contains(tag, "enum:") {
		parts := strings.Split(tag, "enum:")
		if len(parts) > 1 {
			enumPart := strings.Split(parts[1], " ")[0]
			enumValues := strings.Split(strings.Trim(enumPart, "\""), ",")
			for _, val := range enumValues {
				constraints.Enum = append(constraints.Enum, strings.TrimSpace(val))
			}
		}
	}

	// Check if any constraints were found
	if constraints.MinLength == nil && constraints.MaxLength == nil &&
		constraints.Min == nil && constraints.Max == nil &&
		constraints.Pattern == "" && !constraints.Required && len(constraints.Enum) == 0 {
		return nil
	}

	return constraints
}

// applyValidationConstraints applies validation constraints to an OpenAPI schema
func applyValidationConstraints(schema *Schema, constraints *ValidationConstraints) {
	if schema == nil || constraints == nil {
		return
	}

	// Apply string length constraints (only for string types)
	if schema.Type == "string" {
		if constraints.MinLength != nil {
			schema.MinLength = *constraints.MinLength
		}
		if constraints.MaxLength != nil {
			schema.MaxLength = *constraints.MaxLength
		}
	}

	// Apply numeric constraints (for integer and number types)
	if schema.Type == "integer" || schema.Type == "number" {
		if constraints.Min != nil {
			schema.Minimum = *constraints.Min
		}
		if constraints.Max != nil {
			schema.Maximum = *constraints.Max
		}
		// Also check min/max from validate tags for numeric types
		if constraints.MinLength != nil && schema.Type == "integer" {
			schema.Minimum = float64(*constraints.MinLength)
		}
		if constraints.MaxLength != nil && schema.Type == "integer" {
			schema.Maximum = float64(*constraints.MaxLength)
		}
	}

	// Apply pattern constraint
	if constraints.Pattern != "" {
		schema.Pattern = constraints.Pattern
	}

	// Apply format constraint
	if constraints.Format != "" {
		schema.Format = constraints.Format
	}

	// Apply enum constraint
	if len(constraints.Enum) > 0 {
		switch schema.Type {
		case "array":
			schema.Items.Enum = constraints.Enum
		case "object":
			if schema.AdditionalProperties != nil {
				schema.AdditionalProperties.Enum = constraints.Enum
			}
		default:
			schema.Enum = constraints.Enum
		}
	}
}

// detectEnumFromConstants detects if a type has associated constants that form an enum
// This is a generic implementation using enhanced metadata with types.Info
func detectEnumFromConstants(goType string, pkgName string, meta *metadata.Metadata) []interface{} {
	if meta == nil {
		return nil
	}

	var goTypePkgName string

	goTypeParts := TypeParts(goType)
	if goTypeParts.PkgName != "" {
		goTypePkgName = goTypeParts.PkgName
		goTypePkgName = strings.TrimPrefix(goTypePkgName, "*")
		goTypePkgName = strings.TrimPrefix(goTypePkgName, "[]")

		goType = goTypeParts.TypeName
	}

	// Group constants by their resolved type and group index
	constantGroups := make(map[string]map[int][]EnumConstant)

	targetPkgName := pkgName
	if goTypePkgName != "" {
		targetPkgName = goTypePkgName
	}

	// Collect all constants and group them
	if pkg, exist := meta.Packages[targetPkgName]; exist {
		for _, file := range pkg.Files {
			for _, variable := range file.Variables {
				if getStringFromPool(meta, variable.Tok) == "const" {
					varType := getStringFromPool(meta, variable.Type)
					resolvedType := getStringFromPool(meta, variable.ResolvedType)
					varName := getStringFromPool(meta, variable.Name)

					// For enum detection, we want to match against the declared type, not the underlying type
					// Use the declared type if available, otherwise fall back to resolved type
					targetType := varType
					if targetType == "" {
						targetType = resolvedType
					}

					// Check if this constant's type matches our target enum type
					// For iota constants, we also need to check if they're in the same group as a typed constant
					if typeMatches(targetType, goType, meta) ||
						(varType == "" && isInSameGroupAsTypedConstant(variable.GroupIndex, goType, file.Variables, meta)) {
						groupIndex := variable.GroupIndex

						if constantGroups[targetType] == nil {
							constantGroups[targetType] = make(map[int][]EnumConstant)
						}

						enumConst := EnumConstant{
							Name:     varName,
							Type:     varType,
							Resolved: resolvedType,
							Value:    variable.ComputedValue,
							Group:    groupIndex,
						}

						constantGroups[targetType][groupIndex] = append(
							constantGroups[targetType][groupIndex],
							enumConst,
						)
					}
				}
			}
		}
	}

	// Find the best enum group for this type
	var bestEnumValues []interface{}
	var maxGroupSize int

	for _, groups := range constantGroups {
		for _, group := range groups {
			if len(group) > maxGroupSize {
				maxGroupSize = len(group)
				bestEnumValues = extractEnumValues(group)
			}
		}
	}

	return bestEnumValues
}

// EnumConstant represents a constant that might be part of an enum
type EnumConstant struct {
	Name     string
	Type     string
	Resolved string
	Value    interface{}
	Group    int
}

// extractEnumValues extracts the actual values from enum constants
func extractEnumValues(constants []EnumConstant) []interface{} {
	var values []interface{}

	for _, constant := range constants {
		if constant.Value != nil {
			// Use the computed value from types.Info
			switch v := constant.Value.(type) {
			case *types.Const:
				// Handle types.Const values
				if v.Val() != nil {
					extracted := extractConstantValue(v.Val())
					values = append(values, extracted)
				}
			default:
				// The values are already in their proper form (string, int, etc.)
				// Just extract them using our helper function
				extracted := extractConstantValue(v)
				values = append(values, extracted)
			}
		}
	}

	// Sort the values to ensure consistent order
	sort.Slice(values, func(i, j int) bool {
		// Convert to strings for comparison
		valI := fmt.Sprintf("%v", values[i])
		valJ := fmt.Sprintf("%v", values[j])
		return valI < valJ
	})

	return values
}

// extractConstantValue extracts the actual value from a constant.Value
func extractConstantValue(val interface{}) interface{} {
	if val == nil {
		return nil
	}

	// Try to use the String() method if available to extract the value
	if stringer, ok := val.(interface{ String() string }); ok {
		str := stringer.String()

		// For string constants, remove quotes if they exist
		if len(str) >= 2 && str[0] == '"' && str[len(str)-1] == '"' {
			return str[1 : len(str)-1] // Remove surrounding quotes
		}

		// For numeric constants, try to parse
		if i, err := strconv.ParseInt(str, 10, 64); err == nil {
			return i
		}
		if f, err := strconv.ParseFloat(str, 64); err == nil {
			return f
		}
		if b, err := strconv.ParseBool(str); err == nil {
			return b
		}

		// Return the string representation as fallback
		return str
	}

	// If it's not a stringer, return as-is
	return val
}

// typeMatches checks if a constant type matches the target enum type
func typeMatches(constantType, targetType string, meta *metadata.Metadata) bool {
	// Direct match
	if constantType == targetType {
		return true
	}

	// Handle pointer types
	if strings.HasPrefix(constantType, "*") && constantType[1:] == targetType {
		return true
	}
	if strings.HasPrefix(targetType, "*") && targetType[1:] == constantType {
		return true
	}

	// Check if constantType is an alias of targetType
	if resolvedConstType := resolveUnderlyingType(constantType, meta); resolvedConstType != "" {
		if resolvedConstType == targetType {
			return true
		}
		// Also check if the resolved type matches the target's underlying type
		if resolvedTargetType := resolveUnderlyingType(targetType, meta); resolvedTargetType != "" {
			if resolvedConstType == resolvedTargetType {
				return true
			}
		}
	}

	// Handle package-qualified types - extract just the type name
	constTypeParts := strings.Split(constantType, ".")
	targetTypeParts := strings.Split(targetType, ".")

	if len(constTypeParts) > 1 && len(targetTypeParts) > 1 {
		// Both are package-qualified, compare the type names
		constTypeName := constTypeParts[len(constTypeParts)-1]
		targetTypeName := targetTypeParts[len(targetTypeParts)-1]
		return constTypeName == targetTypeName
	} else if len(constTypeParts) > 1 {
		// Constant is package-qualified, target is not
		constTypeName := constTypeParts[len(constTypeParts)-1]
		return constTypeName == targetType
	} else if len(targetTypeParts) > 1 {
		// Target is package-qualified, constant is not
		targetTypeName := targetTypeParts[len(targetTypeParts)-1]
		return constantType == targetTypeName
	}

	return false
}

const mapGoTypeToOpenAPISchemaKey = "mapGoTypeToOpenAPISchema"

// mapGoTypeToOpenAPISchema maps Go types to OpenAPI schemas
func mapGoTypeToOpenAPISchema(usedTypes map[string]*Schema, goType string, meta *metadata.Metadata, cfg *APISpecConfig, visitedTypes map[string]bool) (*Schema, map[string]*Schema) {
	schemas := map[string]*Schema{}
	var schema *Schema

	if visitedTypes == nil {
		visitedTypes = map[string]bool{}
	}

	isPrimitive := isPrimitiveType(goType)

	derivedGoType := strings.TrimPrefix(goType, "*")

	// Check for cycles using both the original type and the derived type
	if (visitedTypes[goType+mapGoTypeToOpenAPISchemaKey] || visitedTypes[derivedGoType+mapGoTypeToOpenAPISchemaKey]) && canAddRefSchemaForType(derivedGoType) {
		return addRefSchemaForType(goType), schemas
	}
	visitedTypes[goType+mapGoTypeToOpenAPISchemaKey] = true

	// Add recursion guard - if we're already processing this type, return a reference
	if schema, exists := usedTypes[derivedGoType]; exists && schema != nil && canAddRefSchemaForType(derivedGoType) {
		return addRefSchemaForType(derivedGoType), schemas
	}

	// Check type mappings first
	for _, mapping := range cfg.TypeMapping {
		if mapping.GoType == goType {
			schema = mapping.OpenAPIType
			markUsedType(usedTypes, goType, schema)

			return schema, schemas
		}
	}

	// Check external types
	if cfg != nil {
		for _, externalType := range cfg.ExternalTypes {
			if externalType.Name == goType {
				schemas[goType] = externalType.OpenAPIType
			}
		}
	}

	// Handle pointer types
	if strings.HasPrefix(goType, "*") {
		underlyingType := strings.TrimSpace(goType[1:])
		// For pointer types, we generate the same schema as the underlying type
		// but we could add nullable: true if needed for OpenAPI 3.0+
		schema, newSchemas := mapGoTypeToOpenAPISchema(usedTypes, underlyingType, meta, cfg, visitedTypes)
		maps.Copy(schemas, newSchemas)
		return schema, schemas
	}

	// Handle map types
	if strings.Contains(goType, "map[") {
		startIdx := strings.Index(goType, "map[")
		endIdx := strings.Index(goType, "]")
		if endIdx > startIdx+4 {
			keyType := goType[startIdx+4 : endIdx]
			valueType := strings.TrimSpace(goType[endIdx+1:])

			// add package name to value type
			if startIdx > 0 {
				valueType = goType[:startIdx] + "." + valueType
			}

			if keyType == "string" {
				var resolvedType string
				if resolvedType = resolveUnderlyingType(valueType, meta); resolvedType == "" {
					resolvedType = valueType
				}

				additionalProperties, newSchemas := mapGoTypeToOpenAPISchema(usedTypes, resolvedType, meta, cfg, visitedTypes)
				maps.Copy(schemas, newSchemas)

				// Use reference for complex value types in maps
				if !isPrimitiveType(resolvedType) && canAddRefSchemaForType(resolvedType) {
					schemas[resolvedType] = additionalProperties
					additionalProperties = addRefSchemaForType(resolvedType)
				}

				// Apply enum detection for map values if the value type is not primitive
				if !isPrimitiveType(valueType) && additionalProperties != nil && len(additionalProperties.Enum) == 0 {
					// Extract package name for enum detection
					pkgName := ""
					if typeParts := TypeParts(valueType); typeParts.PkgName != "" {
						pkgName = typeParts.PkgName
					}

					// Detect enum values for this value type
					if enumValues := detectEnumFromConstants(valueType, pkgName, meta); len(enumValues) > 0 {
						// Apply enum values to the stored schema if it exists
						if storedSchema, exists := schemas[resolvedType]; exists {
							storedSchema.Enum = enumValues
						} else {
							additionalProperties.Enum = enumValues
						}
					}
				}

				schema = &Schema{
					Type:                 "object",
					AdditionalProperties: additionalProperties,
				}

				return schema, schemas
			}
			// Non-string keys are not supported in OpenAPI, fallback to generic object
			schema = &Schema{Type: "object"}

			return schema, schemas
		}
	}

	// Handle slice types
	if strings.HasPrefix(goType, "[]") {
		elementType := strings.TrimSpace(goType[2:])

		var resolvedType string
		if resolvedType = resolveUnderlyingType(elementType, meta); resolvedType == "" {
			resolvedType = elementType
		}
		isPrimitiveElement := isPrimitiveType(resolvedType)

		// Check if the element type already exists in usedTypes
		if bodySchema, ok := usedTypes[elementType]; !isPrimitiveElement && ok {
			if bodySchema == nil {
				var newBodySchemas map[string]*Schema

				bodySchema, newBodySchemas = mapGoTypeToOpenAPISchema(usedTypes, resolvedType, meta, cfg, visitedTypes)
				maps.Copy(schemas, newBodySchemas)
			}
			markUsedType(usedTypes, resolvedType, bodySchema)

			// Create a reference to the existing schema
			schema = &Schema{
				Type:  "array",
				Items: addRefSchemaForType(resolvedType),
			}

			return schema, schemas
		}

		items, newSchemas := mapGoTypeToOpenAPISchema(usedTypes, resolvedType, meta, cfg, visitedTypes)
		maps.Copy(schemas, newSchemas)

		// Use reference for complex element types in arrays
		if !isPrimitiveElement && canAddRefSchemaForType(resolvedType) && items != nil {
			schemas[resolvedType] = items
			items = addRefSchemaForType(resolvedType)
		}

		// Apply enum detection for array elements if the element type is not primitive
		if !isPrimitiveType(elementType) && items != nil && len(items.Enum) == 0 {
			// Extract package name for enum detection
			pkgName := ""
			if typeParts := TypeParts(elementType); typeParts.PkgName != "" {
				pkgName = typeParts.PkgName
			}

			// Detect enum values for this element type
			if enumValues := detectEnumFromConstants(elementType, pkgName, meta); len(enumValues) > 0 {
				// Apply enum values to the stored schema if it exists
				if storedSchema, exists := schemas[resolvedType]; exists {
					storedSchema.Enum = enumValues
				} else {
					items.Enum = enumValues
				}
			}
		}

		schema = &Schema{
			Type:  "array",
			Items: items,
		}

		return schema, schemas
	}

	// Default mappings
	switch goType {
	case "string":
		return &Schema{Type: "string"}, schemas
	case "int", "int8", "int16", "int32", "int64":
		return &Schema{Type: "integer"}, schemas
	case "uint", "uint8", "uint16", "uint32", "uint64", "byte":
		return &Schema{Type: "integer", Minimum: 0}, schemas
	case "float32", "float64":
		return &Schema{Type: "number"}, schemas
	case "bool":
		return &Schema{Type: "boolean"}, schemas
	case "time.Time":
		return &Schema{
			Type:   "string",
			Format: "date-time",
		}, schemas
	case "[]byte":
		return &Schema{Type: "string", Format: "byte"}, schemas
	case "[]string":
		return &Schema{Type: "array", Items: &Schema{Type: "string"}}, schemas
	case "[]time.Time":
		return &Schema{Type: "array", Items: &Schema{Type: "string", Format: "date-time"}}, schemas
	case "[]int":
		return &Schema{Type: "array", Items: &Schema{Type: "integer"}}, schemas
	case "interface{}", "struct{}", "any":
		return &Schema{Type: "object"}, schemas
	default:
		// For custom types, check if it's a struct in metadata
		if meta != nil {
			// Try to find the type in metadata
			typs := findTypesInMetadata(meta, goType)
			for key, typ := range typs {
				if typ != nil {
					// Generate inline schema for the type
					schema, newSchemas := generateSchemaFromType(usedTypes, key, typ, meta, cfg, visitedTypes)
					if schema != nil {
						maps.Copy(schemas, newSchemas)
						markUsedType(usedTypes, goType, schema)

						return schema, schemas
					}
				}
			}
		}

		if !isPrimitive && goType != "" {
			return addRefSchemaForType(goType), schemas
		}

		return schema, schemas
	}
}

func canAddRefSchemaForType(key string) bool {
	if isPrimitiveType(key) || strings.HasPrefix(key, "[]") || strings.Contains(key, "map[") {
		return false
	}

	// Exclude _nested types from reference schema generation
	if strings.HasSuffix(key, "_nested") {
		return false
	}

	// Allow reference schemas for custom types
	return true
}

func addRefSchemaForType(goType string) *Schema {
	// For custom types not found in metadata, create a reference
	goType = strings.TrimPrefix(goType, "*")
	return &Schema{Ref: refComponentsSchemasPrefix + schemaComponentNameReplacer.Replace(goType)}
}

// isInSameGroupAsTypedConstant checks if a constant is in the same group as a typed constant
func isInSameGroupAsTypedConstant(groupIndex int, targetType string, variables map[string]*metadata.Variable, meta *metadata.Metadata) bool {
	for _, variable := range variables {
		if getStringFromPool(meta, variable.Tok) == "const" &&
			variable.GroupIndex == groupIndex {
			varType := getStringFromPool(meta, variable.Type)
			if typeMatches(varType, targetType, meta) {
				return true
			}
		}
	}
	return false
}
