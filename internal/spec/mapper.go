package spec

import (
	"net/http"
	"os"
	"regexp"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/ehabterra/swagen/internal/metadata"
)

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

// LoadSwagenConfig loads a SwagenConfig from a YAML file
func LoadSwagenConfig(path string) (*SwagenConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var config SwagenConfig
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		return nil, err
	}

	return &config, nil
}

// DefaultSwagenConfig returns a default configuration
func DefaultSwagenConfig() *SwagenConfig {
	return &SwagenConfig{}
}

// MapMetadataToOpenAPI maps metadata to OpenAPI specification
func MapMetadataToOpenAPI(tree TrackerTreeInterface, cfg *SwagenConfig, genCfg GeneratorConfig) (*OpenAPISpec, error) {
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
	re := regexp.MustCompile(`\{([a-zA-Z_][a-zA-Z0-9_]*)\}`)
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
	re := regexp.MustCompile(`:([a-zA-Z_][a-zA-Z0-9_]*)`)

	// Replace all matches with {param} format
	result := re.ReplaceAllString(path, "{$1}")

	return result
}

// generateComponentSchemas generates component schemas from metadata
func generateComponentSchemas(meta *metadata.Metadata, cfg *SwagenConfig, routes []RouteInfo) Components {
	components := Components{
		Schemas: make(map[string]*Schema),
	}

	// Track processed types to avoid duplicates
	processed := make(map[string]bool)

	// Collect all types used in routes
	usedTypes := collectUsedTypesFromRoutes(routes, meta, cfg)

	// Generate schemas for used types
	for typeName := range usedTypes {
		if processed[typeName] {
			continue
		}
		processed[typeName] = true

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

			if typ == nil {
				keyParts := strings.Split(key, "-")
				if len(keyParts) > 1 {
					schema = mapGoTypeToOpenAPISchema(keyParts[1], meta, cfg)
				}
			} else {
				schema = generateSchemaFromType(key, typ, meta, cfg)
			}
			if schema != nil {
				components.Schemas[schemaComponentNameReplacer.Replace(key)] = schema
			}
		}
	}

	return components
}

// collectUsedTypesFromRoutes collects all types used in routes
func collectUsedTypesFromRoutes(routes []RouteInfo, meta *metadata.Metadata, cfg *SwagenConfig) map[string]bool {
	usedTypes := make(map[string]bool)

	for _, route := range routes {
		// Add request body types
		if route.Request != nil && route.Request.BodyType != "" {
			addTypeAndDependenciesWithMetadata(route.Request.BodyType, usedTypes, meta, cfg)
		}

		// Add response types
		for _, res := range route.Response {
			if route.Response != nil && res.BodyType != "" {
				addTypeAndDependenciesWithMetadata(res.BodyType, usedTypes, meta, cfg)
			}
		}

		// Add parameter types
		for _, param := range route.Params {
			if param.Schema != nil && param.Schema.Ref != "" {
				// Extract type name from ref like "#/components/schemas/TypeName"
				refParts := strings.Split(param.Schema.Ref, "/")
				if len(refParts) > 0 {
					typeName := refParts[len(refParts)-1]
					addTypeAndDependenciesWithMetadata(typeName, usedTypes, meta, cfg)
				}
			}
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

	// Generics
	if len(typeParts) > 2 {
		for _, part := range typeParts[2:] {
			genericType := strings.Split(part, " ")
			if isPrimitiveType(genericType[1]) {
				metaTypes[genericType[0]+"-"+genericType[1]] = nil
			} else {
				genericTypeParts := TypeParts(genericType[0])

				if t := typeByName(genericTypeParts, meta, genericType[0]); t != nil {
					metaTypes[genericType[0]+"_"+genericType[1]] = t
				}
			}
		}
	}

	metaTypes[typeName] = typeByName(typeParts, meta, typeName)

	return metaTypes
}

func typeByName(typeParts []string, meta *metadata.Metadata, typeName string) *metadata.Type {
	if meta == nil {
		return nil
	}
	if len(typeParts) > 1 {
		pkgName := typeParts[0]

		if pkg, exists := meta.Packages[pkgName]; exists {
			for _, file := range pkg.Files {
				if typ, exists := file.Types[typeParts[1]]; exists {
					return typ
				}
			}
		}
	}

	for _, pkg := range meta.Packages {
		for _, file := range pkg.Files {
			if typ, exists := file.Types[typeName]; exists {
				return typ
			}
		}
	}
	return nil
}

func TypeParts(typeName string) []string {
	typeParts := strings.Split(typeName, TypeSep)

	if len(typeParts) == 1 {
		lastSep := strings.LastIndex(typeName, defaultSep)
		if lastSep > 0 {
			typeParts = []string{typeName[:lastSep], typeName[lastSep+1:]}
		}
	}

	if len(typeParts) == 2 && strings.Contains(typeParts[1], "[") {
		typeParts = append(typeParts[:1], strings.Split(typeParts[1], "[")...)
		typeParts[2] = typeParts[2][:len(typeParts[2])-1]
	}

	return typeParts
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
		"complex64", "complex128",
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

// generateSchemaFromType generates an OpenAPI schema from a metadata type
func generateSchemaFromType(key string, typ *metadata.Type, meta *metadata.Metadata, cfg *SwagenConfig) *Schema {
	// Get type kind from string pool
	kind := getStringFromPool(meta, typ.Kind)

	switch kind {
	case "struct":
		return generateStructSchema(key, typ, meta, cfg)
	case "interface":
		return generateInterfaceSchema()
	case "alias":
		return generateAliasSchema(typ, meta, cfg)
	default:
		return &Schema{Type: "object"}
	}
}

// generateStructSchema generates a schema for a struct type
func generateStructSchema(key string, typ *metadata.Type, meta *metadata.Metadata, cfg *SwagenConfig) *Schema {
	keyParts := TypeParts(key)
	genericTypes := map[string]string{}

	if len(keyParts) > 2 {
		for _, part := range keyParts[2:] {
			genericType := strings.Split(part, " ")
			genericTypes[genericType[0]] = strings.ReplaceAll(part, " ", "-")
		}
	}

	schema := &Schema{
		Type:       "object",
		Properties: make(map[string]*Schema),
	}

	for _, field := range typ.Fields {
		fieldName := getStringFromPool(meta, field.Name)
		fieldType := getStringFromPool(meta, field.Type)

		if genericType, ok := genericTypes[fieldType]; ok {
			fieldType = genericType
		}

		// Extract JSON tag if present
		jsonName := extractJSONName(getStringFromPool(meta, field.Tag))
		if jsonName != "" {
			fieldName = jsonName
		}

		// Generate schema for field type
		fieldSchema := mapGoTypeToOpenAPISchema(fieldType, meta, cfg)
		schema.Properties[fieldName] = fieldSchema
	}

	return schema
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
func generateAliasSchema(typ *metadata.Type, meta *metadata.Metadata, cfg *SwagenConfig) *Schema {
	underlyingType := getStringFromPool(meta, typ.Target)
	return mapGoTypeToOpenAPISchema(underlyingType, meta, cfg)
}

// addTypeAndDependenciesWithMetadata adds a type and its dependencies to the used types set
func addTypeAndDependenciesWithMetadata(typeName string, usedTypes map[string]bool, meta *metadata.Metadata, cfg *SwagenConfig) {
	if usedTypes[typeName] {
		return // Already processed
	}

	usedTypes[typeName] = true

	// Handle pointer types by dereferencing them
	dereferencedType := typeName
	if strings.HasPrefix(typeName, "*") {
		dereferencedType = strings.TrimSpace(typeName[1:])
		// Also add the dereferenced type to used types
		if !usedTypes[dereferencedType] {
			usedTypes[dereferencedType] = true
		}
	}

	// Find the type in metadata and add its field types
	typs := findTypesInMetadata(meta, dereferencedType)
	for _, typ := range typs {
		if typ != nil {
			kind := getStringFromPool(meta, typ.Kind)
			switch kind {
			case "struct":
				// Add all field types as dependencies
				for _, field := range typ.Fields {
					fieldType := getStringFromPool(meta, field.Type)
					if fieldType != "" && fieldType != typeName { // Avoid self-reference
						addTypeAndDependenciesWithMetadata(fieldType, usedTypes, meta, cfg)
					}
				}
			case "alias":
				// Add the underlying type
				underlyingType := getStringFromPool(meta, typ.Target)
				if underlyingType != "" && underlyingType != typeName { // Avoid self-reference
					addTypeAndDependenciesWithMetadata(underlyingType, usedTypes, meta, cfg)
				}
			}
		}
	}
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

// mapGoTypeToOpenAPISchema maps Go types to OpenAPI schemas
func mapGoTypeToOpenAPISchema(goType string, meta *metadata.Metadata, cfg *SwagenConfig) *Schema {
	// Check type mappings first
	for _, mapping := range cfg.TypeMapping {
		if mapping.GoType == goType {
			return mapping.OpenAPIType
		}
	}

	// Check external types
	if cfg != nil {
		for _, externalType := range cfg.ExternalTypes {
			if externalType.Name == goType {
				return addRefSchemaForType(goType)
			}
		}
	}

	// Handle pointer types
	if strings.HasPrefix(goType, "*") {
		underlyingType := strings.TrimSpace(goType[1:])
		// For pointer types, we generate the same schema as the underlying type
		// but we could add nullable: true if needed for OpenAPI 3.0+
		return mapGoTypeToOpenAPISchema(underlyingType, meta, cfg)
	}

	// Handle map types
	if strings.HasPrefix(goType, "map[") {
		endIdx := strings.Index(goType, "]")
		if endIdx > 4 {
			keyType := goType[4:endIdx]
			valueType := strings.TrimSpace(goType[endIdx+1:])
			if keyType == "string" {
				return &Schema{
					Type:                 "object",
					AdditionalProperties: mapGoTypeToOpenAPISchema(valueType, meta, cfg),
				}
			}
			// Non-string keys are not supported in OpenAPI, fallback to generic object
			return &Schema{Type: "object"}
		}
	}

	// Handle slice types
	if strings.HasPrefix(goType, "[]") {
		elementType := strings.TrimSpace(goType[2:])
		return &Schema{
			Type:  "array",
			Items: mapGoTypeToOpenAPISchema(elementType, meta, cfg),
		}
	}

	// Default mappings
	switch goType {
	case "string":
		return &Schema{Type: "string"}
	case "int", "int8", "int16", "int32", "int64":
		return &Schema{Type: "integer"}
	case "uint", "uint8", "uint16", "uint32", "uint64", "byte":
		return &Schema{Type: "integer", Minimum: 0}
	case "float32", "float64":
		return &Schema{Type: "number"}
	case "bool":
		return &Schema{Type: "boolean"}
	case "time.Time":
		return &Schema{
			Type:   "string",
			Format: "date-time",
		}
	case "[]byte":
		return &Schema{Type: "string", Format: "byte"}
	case "[]string":
		return &Schema{Type: "array", Items: &Schema{Type: "string"}}
	case "[]time.Time":
		return &Schema{Type: "array", Items: &Schema{Type: "string", Format: "date-time"}}
	case "[]int":
		return &Schema{Type: "array", Items: &Schema{Type: "integer"}}
	case "interface{}", "struct{}", "any":
		return &Schema{Type: "object"}
	default:
		// For custom types, check if it's a struct in metadata
		if meta != nil {
			// Try to find the type in metadata
			typs := findTypesInMetadata(meta, goType)
			for key, typ := range typs {
				if typ != nil {
					// Generate inline schema for the type
					return generateSchemaFromType(key, typ, meta, cfg)
				}
			}
		}

		if goType != "" {
			return addRefSchemaForType(goType)
		}

		return nil
	}
}

func addRefSchemaForType(goType string) *Schema {
	// For custom types not found in metadata, create a reference
	return &Schema{Ref: refComponentsSchemasPrefix + schemaComponentNameReplacer.Replace(goType)}

}
