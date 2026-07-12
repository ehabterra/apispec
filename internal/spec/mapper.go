// Copyright 2025 Ehab Terra
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
	"fmt"
	"go/ast"
	"go/types"
	"log"
	"maps"
	"net/http"
	"os"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/ehabterra/apispec/internal/metadata"
	"github.com/ehabterra/apispec/internal/typemodel"
)

const (
	refComponentsSchemasPrefix = "#/components/schemas/"
)

// schemaComponentNameReplacer sanitizes a Go-flavoured type identifier into a
// safe OpenAPI component schema name. Dots are deliberately collapsed to
// underscores: Redoc's JSON-pointer resolver splits on '.' when looking up
// `$ref` targets and rejects any component name containing one (see
// https://github.com/Redocly/redoc/issues/1816). Swagger UI and Scalar
// tolerate dots, but the underscore form works everywhere, so we normalise
// to it.
var schemaComponentNameReplacer = strings.NewReplacer(
	"/", "_",
	"-->", "_", // internal type separator → safe word break
	".", "_", // dotted package paths (github.com/...) → safe word break
	" ", "-",
	"[", "_",
	"]", "",
	", ", "-",
)

// unresolvedExternalPlaceholder returns the schema we register when a
// referenced type can't be resolved through metadata, typeMapping,
// externalTypes, or the well-known table. Without it, the $ref dangles and
// Redoc aborts with "Invalid reference token".
func unresolvedExternalPlaceholder(name string) *Schema {
	return &Schema{
		Type:        "object",
		Description: "External or unresolved type: " + name,
	}
}

// shouldPromoteToComponent reports whether an inline schema should be
// promoted into a named component and replaced with a $ref at the call
// site. Three reasons it shouldn't:
//   - The schema is already a $ref (avoid self-referencing components).
//   - The schema is primitive-shaped (uuid → {string, uuid} reads better
//     inline than as a one-line wrapper component).
//   - The key isn't ref-eligible (primitives, containers, _nested types).
func shouldPromoteToComponent(key string, s *Schema) bool {
	if s == nil || s.Ref != "" {
		return false
	}
	if !canAddRefSchemaForType(key) {
		return false
	}
	return !isPrimitiveShapedSchema(s)
}

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

	if err := config.ValidateSecurity(); err != nil {
		return nil, err
	}

	return &config, nil
}

// DefaultAPISpecConfig returns a default configuration
func DefaultAPISpecConfig() *APISpecConfig {
	return &APISpecConfig{}
}

// SecurityDiagnostics carries non-fatal findings from extraction.
type SecurityDiagnostics struct {
	// UnresolvedMiddleware lists detected auth middleware that matched no
	// SecurityMapping (deduped). The UI uses this to offer interactive mapping.
	UnresolvedMiddleware []MiddlewareRef

	// PathParamMismatches lists handlers that read a map-key path variable
	// (mux.Vars(r)["userId"]) whose key matches no route placeholder — a likely
	// typo, since the read is always empty.
	PathParamMismatches []PathParamMismatch
}

// MapMetadataToOpenAPI maps metadata to OpenAPI specification.
func MapMetadataToOpenAPI(tree TrackerTreeInterface, cfg *APISpecConfig, genCfg GeneratorConfig) (*OpenAPISpec, error) {
	spec, _, err := MapMetadataToOpenAPIWithDiagnostics(tree, cfg, genCfg)
	return spec, err
}

// MapMetadataToOpenAPIWithDiagnostics is MapMetadataToOpenAPI plus the security
// diagnostics gathered during extraction (e.g. unresolved middleware).
func MapMetadataToOpenAPIWithDiagnostics(tree TrackerTreeInterface, cfg *APISpecConfig, genCfg GeneratorConfig) (*OpenAPISpec, *SecurityDiagnostics, error) {
	// Create extractor
	extractor := NewExtractor(tree, cfg)

	// Extract routes
	routes := extractor.ExtractRoutes()

	// Warn about auth middleware that was detected but matched no
	// SecurityMapping, so the user knows what to map. apispecui surfaces the
	// same list for interactive assignment (see design doc §5). Only warn when
	// some mappings exist (a library was detected or the user configured them);
	// otherwise auth detection is effectively off and the noise is unwanted.
	if unresolved := extractor.UnresolvedSecurity(); len(unresolved) > 0 && len(cfg.SecurityMappings) > 0 {
		names := make([]string, len(unresolved))
		for i, r := range unresolved {
			names[i] = r.String()
		}
		log.Printf("[security] %d auth middleware not mapped to a security scheme "+
			"(add securityMappings to resolve): %s", len(unresolved), strings.Join(names, ", "))
	}

	// Warn about handlers that read a path variable by a key with no matching
	// path placeholder (e.g. mux.Vars(r)["userId"] on a /users/{id} route) — a
	// likely typo, since the read is always empty.
	for _, m := range extractor.PathParamMismatches() {
		log.Printf("[path-params] %s %s: handler %s reads path variable %q, "+
			"but the path declares no such parameter (did you mean a different key or path segment?)",
			m.Method, m.Path, m.Handler, m.Key)
	}

	// Build paths
	paths := buildPathsFromRoutes(routes)

	// Generate component schemas
	components := generateComponentSchemas(tree.GetMetadata(), cfg, routes)

	// Register shared component parameters for dynamic-path placeholders
	// (issue #34). Each unique placeholder name across routes becomes one
	// component, $ref'd from every operation that uses it — see
	// buildPathsFromRoutes for the per-operation wiring.
	addDynamicPathParamComponents(&components, routes)

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

	// Fill securitySchemes in components: always include user-defined schemes,
	// plus any library-preset schemes actually referenced by a resolved
	// operation (or the global security). Unused presets are omitted so the spec
	// stays lean; a referenced-but-undefined scheme is reported.
	if schemes := reconcileSecuritySchemes(cfg, routes); len(schemes) > 0 {
		if spec.Components == nil {
			spec.Components = &Components{}
		}
		spec.Components.SecuritySchemes = schemes
	}

	diag := &SecurityDiagnostics{
		UnresolvedMiddleware: extractor.UnresolvedSecurity(),
		PathParamMismatches:  extractor.PathParamMismatches(),
	}
	return spec, diag, nil
}

// reconcileSecuritySchemes returns the securityScheme catalog to emit: all
// user-defined schemes, plus preset schemes referenced by an operation or the
// global security. Referenced names defined in neither are logged as warnings.
func reconcileSecuritySchemes(cfg *APISpecConfig, routes []*RouteInfo) map[string]SecurityScheme {
	out := make(map[string]SecurityScheme, len(cfg.SecuritySchemes))
	for name, scheme := range cfg.SecuritySchemes {
		out[name] = scheme
	}

	// Collect referenced scheme names from per-operation and global security.
	referenced := make(map[string]struct{})
	collect := func(reqs []SecurityRequirement) {
		for _, req := range reqs {
			for name := range req {
				referenced[name] = struct{}{}
			}
		}
	}
	collect(cfg.Security)
	for _, r := range routes {
		collect(r.Security)
	}

	var dangling []string
	for name := range referenced {
		if _, ok := out[name]; ok {
			continue
		}
		if def, ok := cfg.presetSchemes[name]; ok {
			out[name] = def
			continue
		}
		dangling = append(dangling, name)
	}
	if len(dangling) > 0 {
		sort.Strings(dangling)
		log.Printf("[security] %d security scheme(s) referenced but not defined "+
			"(add them to securitySchemes): %s", len(dangling), strings.Join(dangling, ", "))
	}

	return out
}

// buildPathsFromRoutes builds OpenAPI paths from extracted routes
func buildPathsFromRoutes(routes []*RouteInfo) map[string]PathItem {
	paths := make(map[string]PathItem)

	for _, route := range routes {
		// Convert path to OpenAPI format
		rawPath := joinPaths(route.MountPath, route.Path)
		openAPIPath := convertPathToOpenAPI(rawPath)

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
		operationID := pkg + strings.Replace(strings.Replace(route.Function, TypeSep, ".", 1), pkg, "", 1)
		if route.OperationIDSuffix != "" {
			operationID += "_" + route.OperationIDSuffix
		}
		operation := &Operation{
			OperationID: operationID,
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
		// Emit $refs for dynamic-path placeholders so each operation
		// reuses the shared component parameter rather than inlining a
		// fresh declaration. The matching {name} in the path is then
		// considered "covered" by ensureAllPathParams below.
		operation.Parameters = appendDynamicParamRefs(operation.Parameters, route.DynamicParams)
		operation.Parameters = ensureAllPathParams(openAPIPath, operation.Parameters, pathParamPatterns(rawPath))

		// Add responses
		operation.Responses = buildResponses(route.Response)

		// Per-operation security resolved from detected auth middleware.
		// route.Security: nil => inherit the document-level security (field
		// omitted); non-nil empty => explicitly public (`security: []`);
		// non-empty => the operation's requirements. The pointer field preserves
		// that distinction (a non-nil pointer is never dropped by omitempty).
		if route.Security != nil {
			sec := route.Security
			operation.Security = &sec
		}

		// Set operation on path item
		setOperationOnPathItem(&pathItem, route.Method, operation)
		paths[openAPIPath] = pathItem
	}

	return paths
}

// ensureAllPathParams ensures all path parameters in the path are present in
// the parameters slice. openAPIPath is already normalised (regex constraints
// stripped); patterns carries any `{name:pattern}` constraints recovered from
// the raw path so synthesized params still surface them as a schema pattern.
func ensureAllPathParams(openAPIPath string, params []Parameter, patterns map[string]string) []Parameter {
	paramMap := make(map[string]bool)
	for _, p := range params {
		if p.In == "path" {
			paramMap[p.Name] = true
		}
		// Treat a $ref parameter as covering its target name so we don't
		// re-inline an auto-declared duplicate (issue #34).
		if p.Ref != "" {
			if name := refTargetName(p.Ref); name != "" {
				paramMap[name] = true
			}
		}
	}
	// Find all {param} in the path
	re := mustCachedRegex(`\{([a-zA-Z_][a-zA-Z0-9_]*)\}`)
	matches := re.FindAllStringSubmatch(openAPIPath, -1)
	for _, match := range matches {
		name := match[1]
		if !paramMap[name] {
			// Add default path parameter with warning extension
			schema := &Schema{Type: "string"}
			if pat := patterns[name]; pat != "" {
				schema.Pattern = pat
			}
			params = append(params, Parameter{
				Name:     name,
				In:       "path",
				Required: true,
				Schema:   schema,
				Extensions: map[string]any{
					"x-warning": "This parameter is present in the path but not found in the code.",
				},
			})
		}
	}
	return params
}

// appendDynamicParamRefs adds one $ref entry per dynamic placeholder name,
// pointing at the shared component parameter. Duplicates (a name already
// covered by an inline parameter or another $ref) are skipped.
func appendDynamicParamRefs(params []Parameter, dynamic []string) []Parameter {
	if len(dynamic) == 0 {
		return params
	}
	covered := make(map[string]struct{}, len(params))
	for _, p := range params {
		if p.In == "path" && p.Name != "" {
			covered[p.Name] = struct{}{}
		}
		if p.Ref != "" {
			if name := refTargetName(p.Ref); name != "" {
				covered[name] = struct{}{}
			}
		}
	}
	for _, name := range dynamic {
		if _, ok := covered[name]; ok {
			continue
		}
		covered[name] = struct{}{}
		params = append(params, Parameter{Ref: dynamicParamRef(name)})
	}
	return params
}

// addDynamicPathParamComponents registers one component parameter per
// unique dynamic placeholder name found across all routes. Once registered,
// each operation references it via $ref (see appendDynamicParamRefs).
func addDynamicPathParamComponents(components *Components, routes []*RouteInfo) {
	if components == nil {
		return
	}
	for _, route := range routes {
		for _, name := range route.DynamicParams {
			if components.Parameters == nil {
				components.Parameters = map[string]*Parameter{}
			}
			key := dynamicParamComponentName(name)
			if _, exists := components.Parameters[key]; exists {
				continue
			}
			components.Parameters[key] = &Parameter{
				Name:        name,
				In:          "path",
				Required:    true,
				Description: "Auto-declared from an unresolved path expression (e.g. a function call evaluated at runtime). APISpec could not statically determine the path segment — see issue #34.",
				Schema:      &Schema{Type: "string"},
				Extensions: map[string]any{
					"x-warning": "This parameter was synthesized from an unresolved path expression and may not represent a real per-request parameter.",
				},
			}
		}
	}
}

// dynamicParamComponentName returns the PascalCase + "Param" suffix used as
// the key under components.parameters for a synthesized placeholder.
func dynamicParamComponentName(name string) string {
	if name == "" {
		return "PathParam"
	}
	first := strings.ToUpper(name[:1])
	return first + name[1:] + "Param"
}

// dynamicParamRef returns the $ref string pointing at the component
// parameter for the given synthesized placeholder name.
func dynamicParamRef(name string) string {
	return "#/components/parameters/" + dynamicParamComponentName(name)
}

// refTargetName extracts the trailing segment of a #/components/parameters/<X>
// ref and reverses the PascalCase + "Param" mangling so callers can match
// against placeholder names found in a path.
func refTargetName(ref string) string {
	const prefix = "#/components/parameters/"
	if !strings.HasPrefix(ref, prefix) {
		return ""
	}
	key := strings.TrimPrefix(ref, prefix)
	key = strings.TrimSuffix(key, "Param")
	if key == "" {
		return ""
	}
	return strings.ToLower(key[:1]) + key[1:]
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
// uninformativeDefault reports whether the "could not determine status"
// response `def` adds no response-body information beyond the resolved-status
// responses already in `chosen`: either its body duplicates a resolved one, or
// it is the bare generic `object` fallback.
func uninformativeDefault(def *ResponseInfo, chosen map[string]*ResponseInfo) bool {
	if isGenericObjectResponse(def) {
		return true
	}
	for status, r := range chosen {
		if status != "default" && sameRenderedBody(r.Schema, def.Schema) {
			return true
		}
	}
	return false
}

// sameRenderedBody compares the actual schemas the spec will render, not the Go
// BodyType label: two responses can share an (often empty) BodyType yet render
// different bodies, so matching on BodyType could prune a distinct fallback.
func sameRenderedBody(a, b *Schema) bool {
	if a == nil || b == nil {
		return a == b
	}
	ab, _ := yaml.Marshal(a)
	bb, _ := yaml.Marshal(b)
	return string(ab) == string(bb)
}

// isGenericObjectResponse reports whether a response body is the featureless
// `{type: object}` fallback (no $ref, properties, composition, or items) — the
// least-informative possible body.
func isGenericObjectResponse(r *ResponseInfo) bool {
	s := r.Schema
	if s == nil {
		return true
	}
	if s.Ref != "" || len(s.Properties) > 0 || len(s.AllOf) > 0 || len(s.OneOf) > 0 ||
		len(s.AnyOf) > 0 || s.Items != nil || s.AdditionalProperties != nil {
		return false
	}
	return s.Type == "" || s.Type == "object"
}

func buildResponses(respInfo map[string]*ResponseInfo) map[string]Response {
	responses := make(map[string]Response)

	// Handle nil case - return default response indicating no response was found
	if len(respInfo) == 0 {
		responses["default"] = Response{
			Description: "Default response (no response found)",
			Content: map[string]MediaType{
				"application/json": {
					Schema: &Schema{Type: "object"},
				},
			},
		}
		return responses
	}

	// Choose one ResponseInfo per OpenAPI status key. Iterate sorted so the
	// outcome never depends on map order. Several unresolved-status bodies
	// (StatusCode < 0) collapse onto "default" — a handler returning its
	// success type plus a framework error map; reconcile those with
	// preferResponseInfo (concrete beats generic, success beats error, stable
	// tie-break) so the winner is deterministic. Resolved statuses keep
	// last-in-sorted-order to preserve prior behaviour (no concrete-preference
	// that could let a mis-paired body displace an intentional one).
	keys := make([]string, 0, len(respInfo))
	for k := range respInfo {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	chosen := make(map[string]*ResponseInfo)
	for _, k := range keys {
		resp := respInfo[k]
		statusCode := k
		// if status less than 0, use "default" to indicate unknown/invalid status
		// OpenAPI only accepts status codes 100-599, "default", or vendor extensions
		if resp.StatusCode < 0 {
			statusCode = "default"
			chosen[statusCode] = preferResponseInfo(chosen[statusCode], resp)
			continue
		}
		chosen[statusCode] = resp
	}

	// "default" here means "status could not be determined" — a fallback, not an
	// OpenAPI catch-all. Once concrete statuses are resolved, a default adds
	// nothing if it carries no new response-body information: either (a) its
	// body is already emitted under a resolved status (the status WAS
	// determined for that body via another call path), or (b) its body is the
	// bare generic `object` fallback (e.g. an `any` parameter that couldn't be
	// traced through an indirection helper). Drop it in those cases; keep a
	// default whose body is concrete and distinct — that's a real response
	// whose status genuinely couldn't be resolved.
	if def := chosen["default"]; def != nil && len(chosen) > 1 && uninformativeDefault(def, chosen) {
		delete(chosen, "default")
	}

	for statusCode, resp := range chosen {
		description := http.StatusText(resp.StatusCode)
		if resp.StatusCode < 0 || description == "" {
			description = "Status code could not be determined"
		}

		responses[statusCode] = Response{
			Description: description,
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
	// Strip regex constraints from `{name:pattern}` placeholders (gorilla/mux
	// and chi allow them, e.g. `/users/{id:[0-9]+}`). OpenAPI path templates
	// cannot carry a regex, so the constraint is removed here and surfaced
	// separately as a schema `pattern` on the parameter.
	path = stripParamPatterns(path)

	// Convert :param (gin/echo) -> {param}.
	// This matches a colon followed by one or more word characters (letters, digits, underscore)
	re := mustCachedRegex(`:([a-zA-Z_][a-zA-Z0-9_]*)`)

	// Replace all matches with {param} format
	result := re.ReplaceAllString(path, "{$1}")

	return result
}

// forEachPathParam scans a URL path and invokes fn once per `{...}` placeholder
// with its name and optional regex pattern (the substring after the first ':').
// Braces are matched with depth counting so patterns that themselves contain
// braces — mux's `{id:[0-9]{3}}` — parse correctly. A malformed (unbalanced)
// placeholder stops the scan.
func forEachPathParam(path string, fn func(name, pattern string)) {
	for i := 0; i < len(path); i++ {
		if path[i] != '{' {
			continue
		}
		depth, j := 1, i+1
		for ; j < len(path) && depth > 0; j++ {
			switch path[j] {
			case '{':
				depth++
			case '}':
				depth--
			}
		}
		if depth != 0 {
			return // unbalanced — give up rather than emit garbage
		}
		inner := path[i+1 : j-1]
		name, pattern := inner, ""
		if c := strings.IndexByte(inner, ':'); c >= 0 {
			name, pattern = inner[:c], inner[c+1:]
		}
		fn(name, pattern)
		i = j - 1
	}
}

// stripParamPatterns rewrites `{name:pattern}` placeholders to `{name}`, leaving
// plain `{name}` and ordinary text untouched.
func stripParamPatterns(path string) string {
	if !strings.ContainsRune(path, '{') {
		return path
	}
	var b strings.Builder
	b.Grow(len(path))
	for i := 0; i < len(path); i++ {
		if path[i] != '{' {
			b.WriteByte(path[i])
			continue
		}
		depth, j := 1, i+1
		for ; j < len(path) && depth > 0; j++ {
			switch path[j] {
			case '{':
				depth++
			case '}':
				depth--
			}
		}
		if depth != 0 {
			b.WriteString(path[i:]) // unbalanced — copy the remainder verbatim
			return b.String()
		}
		inner := path[i+1 : j-1]
		name := inner
		if c := strings.IndexByte(inner, ':'); c >= 0 {
			name = inner[:c]
		}
		b.WriteByte('{')
		b.WriteString(name)
		b.WriteByte('}')
		i = j - 1
	}
	return b.String()
}

// pathParamPatterns returns the name->regex-pattern map for the constrained
// `{name:pattern}` placeholders in a path (unconstrained `{name}` are omitted).
func pathParamPatterns(path string) map[string]string {
	var out map[string]string
	forEachPathParam(path, func(name, pattern string) {
		if name == "" || pattern == "" {
			return
		}
		if out == nil {
			out = map[string]string{}
		}
		out[name] = pattern
	})
	return out
}

// generateComponentSchemas generates component schemas from metadata
func generateComponentSchemas(meta *metadata.Metadata, cfg *APISpecConfig, routes []*RouteInfo) Components {
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
	// Iterate in sorted order: generateSchemaFromType's recursion guard turns
	// already-visited types into $refs, so map-range order would decide
	// inline-vs-$ref per run.
	for _, typeName := range slices.Sorted(maps.Keys(usedTypes)) {
		// Synthetic anonymous-struct types (see metadata.AnonStructKey)
		// are emitted inline at the use site, so they have no name to
		// register under components/schemas.
		if metadata.IsAnonStructTypeName(typeName) {
			continue
		}

		// Check external types
		if cfg != nil {
			for _, externalType := range cfg.ExternalTypes {
				if externalType.Name == strings.ReplaceAll(typeName, TypeSep, ".") {
					components.Schemas[schemaComponentNameReplacer.Replace(typeName)] = externalType.OpenAPIType
					continue
				}
			}
		}

		// Known external types (uuid.UUID, decimal.Decimal, sql.Null*, …) are
		// resolved by the spec-layer registry/facts and inlined at their use
		// sites. They have no metadata type entry, so without this they'd be
		// mistaken for unresolved and get a bogus object placeholder.
		if s, _, ok := resolveExternalType(typeName, cfg, meta, usedTypes, map[string]bool{}); ok {
			if s != nil && !isPrimitiveShapedSchema(s) {
				// Non-primitive resolution (rare): emit it as a real component.
				components.Schemas[schemaComponentNameReplacer.Replace(typeName)] = s
			}
			// Primitive-shaped (the common case): inlined; emit no component.
			continue
		}

		// Find the type in metadata
		typs := findTypesInMetadata(meta, typeName)
		if len(typs) == 0 || typs[typeName] == nil {
			// Belt-and-suspenders: even when the type isn't resolvable,
			// any $ref produced earlier still needs a target. Skip the
			// placeholder for primitives and container types — those are
			// emitted inline and never reach a $ref site.
			if canAddRefSchemaForType(typeName) {
				key := schemaComponentNameReplacer.Replace(typeName)
				if _, exists := components.Schemas[key]; !exists {
					components.Schemas[key] = unresolvedExternalPlaceholder(typeName)
				}
			}
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
func collectUsedTypesFromRoutes(routes []*RouteInfo) map[string]*Schema {
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
	if metadata.IsPrimitiveType(typeName) {
		return nil
	}

	// Guard against nil metadata
	if meta == nil {
		return nil
	}

	core := typemodel.Parse(typeName).Core()
	if core == nil {
		return metaTypes
	}

	var pkgName string
	if !metadata.IsPrimitiveType(core.Pkg) && core.Pkg != "" {
		pkgName = core.Pkg + "."
	}

	// Generics
	for _, arg := range core.Args {
		if arg.Constraint == "" {
			// Concrete instantiation argument (e.g. "User" in Page[User]).
			// Don't register it here: this map has one entry per schema to
			// emit, and callers that want a single type for goType pick the
			// first non-nil entry — a second one would non-deterministically
			// shadow the parametric type itself. The concrete argument is
			// emitted as its own component through the parametric struct's
			// field resolution instead.
			continue
		}
		// Declaration form "T constraint" (e.g. "T any").
		if metadata.IsPrimitiveType(arg.Constraint) {
			metaTypes[pkgName+arg.Name+"-"+arg.Constraint] = nil
		} else if t := typeByName("", arg.Name, meta); t != nil {
			metaTypes[pkgName+arg.Name+"_"+arg.Constraint] = t
		}
	}

	if typeName != "" {
		metaTypes[typeName] = typeByName(core.Pkg, core.Name, meta)
	}

	return metaTypes
}

// typeByName looks a type up in metadata by its parsed core: first in the
// named package, then across all packages. Metadata keys a type by its bare
// declared name (tspec.Name.Name — a generic declaration is stored as "Page",
// its parameters in Type.TypeParams), so callers pass the structured core
// name (typemodel.Parse(...).Core().Name), never a bracketed form.
func typeByName(pkgName, typeName string, meta *metadata.Metadata) *metadata.Type {
	if meta == nil {
		return nil
	}

	if pkgName != "" && typeName != "" {
		if pkg, exists := meta.Packages[pkgName]; exists {
			if typ := typeInPackage(pkg, typeName); typ != nil {
				return typ
			}
		}
	}

	// Fallback: the bare type name may exist in several packages. Iterate in a
	// stable (cached) sorted order so the chosen type is deterministic across
	// runs — otherwise map iteration would pick a different package's type and
	// flip the schema between runs.
	for _, pkg := range meta.SortedPackageNames() {
		if typ := typeInPackage(meta.Packages[pkg], typeName); typ != nil {
			return typ
		}
	}
	return nil
}

// typeInPackage returns the named type from a package, scanning files in stable
// order (Files is a map).
func typeInPackage(pkg *metadata.Package, typeName string) *metadata.Type {
	if pkg == nil {
		return nil
	}
	// A type lives in exactly one file, but iterate deterministically anyway.
	var fileNames []string
	for fileName := range pkg.Files {
		fileNames = append(fileNames, fileName)
	}
	sort.Strings(fileNames)
	for _, fileName := range fileNames {
		if typ, exists := pkg.Files[fileName].Types[typeName]; exists {
			return typ
		}
	}
	return nil
}

// Parts is the flat package/type/arguments view of a string-encoded type
// name. It lives in the typemodel package now (the structured type model);
// this alias keeps the spec API stable during the migration.
type Parts = typemodel.Parts

// TypeParts splits a string-encoded type name into its package, type, and
// generic-argument parts. Transitional: delegates to typemodel; new code
// should use typemodel.Parse and consume the structured TypeRef.
func TypeParts(typeName string) Parts {
	return typemodel.ParseParts(typeName)
}

// normalizeGenericInstanceName rewrites a generic instantiation rendered in
// the go/types form (pkg.Type[pkg.Arg]) into the internal pkg-->Type[Arg]
// form with simple argument names, via the structured type model — which,
// unlike the legacy string view, also handles instantiations wrapped in
// pointer/slice constructors ([]pkg.Page[pkg.User]).
func normalizeGenericInstanceName(s string) string {
	return typemodel.Canonicalize(s)
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

// allConcreteGenericArgs reports whether every generic part is a concrete type
// argument (a single token) rather than a declaration entry ("T any"), which
// carries its constraint after a space.
func allConcreteGenericArgs(args []*typemodel.TypeRef) bool {
	for _, a := range args {
		if a == nil || a.Constraint != "" || genericArgText(a) == "" {
			return false
		}
	}
	return len(args) > 0
}

// genericArgText renders one generic argument for the substitution map,
// preferring the exact text it was parsed from (what the legacy split
// produced) and falling back to the simple rendering for programmatically
// built refs.
func genericArgText(a *typemodel.TypeRef) string {
	if a == nil {
		return ""
	}
	if r := a.Raw(); r != "" {
		return r
	}
	return a.Simple()
}

// substituteTypeParams replaces a parametric field type with its concrete
// argument, preserving leading slice/pointer markers so `Items []T` becomes
// `[]User` and `Data T` becomes `User`. Field types that don't reduce to a
// declared type parameter are returned unchanged.
func substituteTypeParams(fieldType string, genericTypes map[string]string) string {
	if len(genericTypes) == 0 {
		return fieldType
	}
	prefix := ""
	rest := fieldType
	for {
		if strings.HasPrefix(rest, "[]") {
			prefix += "[]"
			rest = rest[2:]
			continue
		}
		if strings.HasPrefix(rest, "*") {
			prefix += "*"
			rest = rest[1:]
			continue
		}
		break
	}
	if concrete, ok := genericTypes[rest]; ok {
		return prefix + concrete
	}
	return fieldType
}

// generateStructSchema generates a schema for a struct type
func generateStructSchema(usedTypes map[string]*Schema, key string, typ *metadata.Type, meta *metadata.Metadata, cfg *APISpecConfig, visitedTypes map[string]bool) (*Schema, map[string]*Schema) {
	schemas := map[string]*Schema{}

	keyCore := typemodel.Parse(key).Core()
	genericTypes := map[string]string{}

	// concreteGenerics reports whether the key carries concrete type arguments
	// (Page[User]) rather than the bare declaration (Page[T any]) — a
	// declaration argument carries its constraint ("T any").
	concreteGenerics := keyCore != nil && len(keyCore.Args) > 0 && len(typ.TypeParams) > 0 &&
		allConcreteGenericArgs(keyCore.Args)

	switch {
	case concreteGenerics:
		// Zip declared type-parameter names (typ.TypeParams) positionally with
		// the concrete arguments, so a parametric field (Data T / Items []T)
		// substitutes to the concrete type.
		for i, param := range typ.TypeParams {
			if i < len(keyCore.Args) {
				genericTypes[param] = genericArgText(keyCore.Args[i])
			}
		}
	case keyCore != nil && len(keyCore.Args) > 0:
		// Declaration form: keep the legacy placeholder behavior (field T maps
		// to the "T-constraint" stand-in).
		for _, arg := range keyCore.Args {
			genericTypes[arg.Name] = strings.ReplaceAll(genericArgText(arg), " ", "-")
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

		// Skip fields that encoding/json never serializes: a `json:"-"` tag,
		// or an unexported field. Mirrors the anonymous-struct path so both
		// stay consistent.
		if jsonFieldOmitted(getStringFromPool(meta, field.Tag)) || !ast.IsExported(fieldName) {
			continue
		}

		if concreteGenerics {
			fieldType = substituteTypeParams(fieldType, genericTypes)
		} else if genericType, ok := genericTypes[fieldType]; ok {
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
			isPrimitive := metadata.IsPrimitiveType(fieldType)

			if !isPrimitive && !strings.Contains(fieldType, ".") {
				re := mustCachedRegex(`((\[\])?\*?)(.+)$`)
				matches := re.FindStringSubmatch(fieldType)
				if len(matches) >= 4 {
					fieldType = matches[1] + pkgName + "." + matches[3]
				}
			}

			derivedFieldType := strings.TrimPrefix(fieldType, "*")
			// Check if this field type already exists in usedTypes. Inline
			// external types (uuid, decimal, …) are excluded: they resolve to
			// a primitive-shaped schema with no component, so a $ref to them
			// would dangle — let them fall through to inline resolution.
			if bodySchema, ok := usedTypes[derivedFieldType]; !isPrimitive && ok &&
				!isInlineExternalType(derivedFieldType, cfg, meta) {
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
				// Promote to component only for complex schemas — keep
				// primitive-shaped values (like uuid.UUID → {string,
				// format: uuid}) inline so the field renders as the
				// actual primitive, not as a $ref to a wrapper.
				if shouldPromoteToComponent(derivedFieldType, fieldSchema) {
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
			if !metadata.IsPrimitiveType(originalFieldType) {
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
	if schema != nil && metadata.IsPrimitiveType(underlyingType) {
		// Extract package name for enum detection
		pkgName := ""
		if core := typemodel.Parse(originalTypeName).Core(); core != nil && core.Pkg != "" {
			pkgName = core.Pkg
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
// jsonFieldOmitted reports whether a struct field with the given tag is
// excluded from JSON serialization entirely via a `json:"-"` tag. It uses an
// exact key lookup (reflect.StructTag) so an unrelated key like `myjson:"-"`
// is not mistaken for the json tag. Note the `json:"-,"` form names a field
// literally "-" and is NOT omitted.
func jsonFieldOmitted(tag string) bool {
	v, ok := reflect.StructTag(tag).Lookup("json")
	return ok && v == "-"
}

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
			rules := mustCachedRegex(`([a-zA-Z_][a-zA-Z0-9_]*(?:=(?:[^,{}]|{[^}]*})*)?)`).FindAllStringSubmatch(validateTag, -1)
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

	if core := typemodel.Parse(goType).Core(); core != nil && core.Pkg != "" {
		goTypePkgName = core.Pkg
		goType = core.Name
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

	isPrimitive := metadata.IsPrimitiveType(goType)

	derivedGoType := strings.TrimPrefix(goType, "*")

	// Inline external types (uuid, decimal, …) are leaves with no component,
	// so they are never a real cycle and a $ref to them would dangle. Exclude
	// them from the cycle/recursion guards below so every occurrence inlines.
	inlineExternal := isInlineExternalType(derivedGoType, cfg, meta)

	// Check for cycles using both the original type and the derived type
	if (visitedTypes[goType+mapGoTypeToOpenAPISchemaKey] || visitedTypes[derivedGoType+mapGoTypeToOpenAPISchemaKey]) && canAddRefSchemaForType(derivedGoType) && !inlineExternal {
		return addRefSchemaForType(goType), schemas
	}
	visitedTypes[goType+mapGoTypeToOpenAPISchemaKey] = true

	// Add recursion guard - if we're already processing this type, return a reference
	if schema, exists := usedTypes[derivedGoType]; exists && schema != nil && canAddRefSchemaForType(derivedGoType) && !inlineExternal {
		return addRefSchemaForType(derivedGoType), schemas
	}

	// Check user typeMapping first, matched by both the full import path and
	// the short pkg-qualified name so a config entry for "uuid.UUID" matches a
	// field typed "github.com/google/uuid.UUID". Primitive-shaped mappings are
	// inlined at every use site and have no component, so they must NOT be
	// marked used — otherwise a second occurrence hits the usedTypes guard and
	// emits a $ref to a component generateSchemas never produces.
	if s := lookupConfigSchema(cfg, goType); s != nil {
		if !isPrimitiveShapedSchema(s) {
			markUsedType(usedTypes, goType, s)
		}
		return s, schemas
	}

	// Check external types (emitted as named components by generateSchemas).
	if cfg != nil {
		for _, externalType := range cfg.ExternalTypes {
			if externalType.Name == goType {
				schemas[goType] = externalType.OpenAPIType
			}
		}
	}

	// Resolve well-known / marshaler-based external types (uuid.UUID,
	// decimal.Decimal, sql.Null*, …) to a precise schema via the spec-layer
	// registry + metadata facts. Wrapped forms ([]T, *T, map[K]T) are not
	// matched here; they fall through to the pointer/array/map branches below
	// and re-enter this function on the element type, which is matched then.
	if s, extra, ok := resolveExternalType(goType, cfg, meta, usedTypes, visitedTypes); ok {
		maps.Copy(schemas, extra)
		// Only register non-primitive resolutions as used types. Primitive-
		// shaped results (uuid → {string,uuid}, …) are inlined at every use
		// site; marking them would make a second occurrence hit the recursion
		// guard and emit a $ref to a component that is never generated.
		if s != nil && !isPrimitiveShapedSchema(s) {
			markUsedType(usedTypes, goType, s)
		}
		return s, schemas
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

	// Handle array types (e.g., [16]byte, [N]string)
	if strings.HasPrefix(goType, "[") {
		// Find the closing bracket
		endIdx := strings.Index(goType, "]")
		if endIdx > 1 {
			elementType := strings.TrimSpace(goType[endIdx+1:])
			arraySize := strings.TrimSpace(goType[1:endIdx])

			var resolvedType string
			if resolvedType = resolveUnderlyingType(elementType, meta); resolvedType == "" {
				resolvedType = elementType
			}
			isPrimitiveElement := metadata.IsPrimitiveType(resolvedType)

			// Special handling for byte arrays - convert to string with maxLength
			if elementType == "byte" || resolvedType == "byte" {
				schema = &Schema{
					Type:   "string",
					Format: "byte",
				}
				if size := parseArraySize(arraySize); size != nil {
					schema.MaxLength = *size
				}
				return schema, schemas
			}

			// For other primitive types, create array schema
			if isPrimitiveElement {
				items, newSchemas := mapGoTypeToOpenAPISchema(usedTypes, resolvedType, meta, cfg, visitedTypes)
				maps.Copy(schemas, newSchemas)

				schema = &Schema{
					Type:  "array",
					Items: items,
				}
				if size := parseArraySize(arraySize); size != nil {
					schema.MaxItems = *size
					schema.MinItems = *size // Fixed size array
				}
				return schema, schemas
			}

			// For complex types, check if already exists in usedTypes.
			// Inline external elements are excluded ($ref would dangle).
			if bodySchema, ok := usedTypes[elementType]; ok && !isInlineExternalType(elementType, cfg, meta) {
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
				if size := parseArraySize(arraySize); size != nil {
					schema.MaxItems = *size
					schema.MinItems = *size // Fixed size array
				}
				return schema, schemas
			}

			items, newSchemas := mapGoTypeToOpenAPISchema(usedTypes, resolvedType, meta, cfg, visitedTypes)
			maps.Copy(schemas, newSchemas)

			// Use reference for complex element types in arrays.
			// Skip when items is already a $ref (avoid self-references —
			// see TestSliceOfUnknownExternalType_NoSelfRef) or when items
			// is primitive-shaped (uuid.UUID → {string, format: uuid}
			// shouldn't be promoted to its own component).
			if shouldPromoteToComponent(resolvedType, items) {
				schemas[resolvedType] = items
				items = addRefSchemaForType(resolvedType)
			}

			// Apply enum detection for array elements if the element type is not primitive
			if !metadata.IsPrimitiveType(elementType) && items != nil && len(items.Enum) == 0 {
				// Extract package name for enum detection
				pkgName := ""
				if core := typemodel.Parse(elementType).Core(); core != nil && core.Pkg != "" {
					pkgName = core.Pkg
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
			if size := parseArraySize(arraySize); size != nil {
				schema.MaxItems = *size
				schema.MinItems = *size // Fixed size array
			}
			return schema, schemas
		}
	}

	// Handle map types
	if strings.Contains(goType, "map[") {
		startIdx := strings.Index(goType, "map[")
		endIdx := strings.Index(goType, "]")
		if endIdx > startIdx+4 {
			keyType := goType[startIdx+4 : endIdx]
			valueType := strings.TrimSpace(goType[endIdx+1:])

			// add package name to value type — but only when the value
			// is a project-local named type that needs qualification.
			// Builtins (interface{}, any, string, …) must stay bare:
			// otherwise the recursive call sees e.g.
			// "pkg.interface{}", falls into the unresolved-external
			// branch, and emits a $ref to a component nothing
			// populates (the Redoc "Invalid reference token" error).
			if startIdx > 0 && !metadata.IsPrimitiveType(valueType) {
				valueType = goType[:startIdx] + "." + valueType
			}

			if keyType == "string" {
				var resolvedType string
				if resolvedType = resolveUnderlyingType(valueType, meta); resolvedType == "" {
					resolvedType = valueType
				}

				additionalProperties, newSchemas := mapGoTypeToOpenAPISchema(usedTypes, resolvedType, meta, cfg, visitedTypes)
				maps.Copy(schemas, newSchemas)

				// Use reference for complex value types in maps. Skip when
				// inner returned a $ref (avoid self-references) or when
				// the schema is primitive-shaped (keep inline).
				if !metadata.IsPrimitiveType(resolvedType) && shouldPromoteToComponent(resolvedType, additionalProperties) {
					schemas[resolvedType] = additionalProperties
					additionalProperties = addRefSchemaForType(resolvedType)
				}

				// Apply enum detection for map values if the value type is not primitive
				if !metadata.IsPrimitiveType(valueType) && additionalProperties != nil && len(additionalProperties.Enum) == 0 {
					// Extract package name for enum detection
					pkgName := ""
					if core := typemodel.Parse(valueType).Core(); core != nil && core.Pkg != "" {
						pkgName = core.Pkg
					}

					// Detect enum values for this value type
					if enumValues := detectEnumFromConstants(valueType, pkgName, meta); len(enumValues) > 0 {
						// Apply enum values to the stored schema if it exists
						if storedSchema, exists := usedTypes[resolvedType]; exists && storedSchema != nil {
							storedSchema.Enum = enumValues
						} else if storedSchema, exists := schemas[resolvedType]; exists {
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
		isPrimitiveElement := metadata.IsPrimitiveType(resolvedType)

		// Check if the element type already exists in usedTypes. Inline
		// external elements (uuid, decimal, …) are excluded: a $ref to them
		// would dangle since they have no component — fall through to inline.
		if bodySchema, ok := usedTypes[elementType]; !isPrimitiveElement && ok &&
			!isInlineExternalType(elementType, cfg, meta) {
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

		// Use reference for complex element types in arrays. Skip when
		// inner returned a $ref (avoid self-references) or when the
		// schema is primitive-shaped (keep inline).
		if !isPrimitiveElement && shouldPromoteToComponent(resolvedType, items) {
			schemas[resolvedType] = items
			items = addRefSchemaForType(resolvedType)
		}

		// Apply enum detection for array elements if the element type is not primitive
		if !metadata.IsPrimitiveType(elementType) && items != nil && len(items.Enum) == 0 {
			// Extract package name for enum detection
			pkgName := ""
			if core := typemodel.Parse(elementType).Core(); core != nil && core.Pkg != "" {
				pkgName = core.Pkg
			}

			// Detect enum values for this element type
			if enumValues := detectEnumFromConstants(elementType, pkgName, meta); len(enumValues) > 0 {
				// Apply enum values to the stored schema if it exists
				if storedSchema, exists := usedTypes[resolvedType]; exists && storedSchema != nil {
					storedSchema.Enum = enumValues
				} else if storedSchema, exists := schemas[resolvedType]; exists {
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

	// Anonymous struct literal (a "struct{...}" form, possibly with a
	// package path glued on by generateStructSchema). It has no name, so it
	// must be inlined — turning it into a $ref dangles with an invalid
	// braced component name (Redoc "Invalid reference token"). Pointer,
	// slice, array and map wrappers were already peeled above, so what
	// reaches here is the scalar element.
	if isAnonStructLiteral(goType) {
		if s, extra := schemaFromAnonStructLiteral(usedTypes, goType, meta, cfg, visitedTypes); s != nil {
			maps.Copy(schemas, extra)
			return s, schemas
		}
		return &Schema{Type: "object"}, schemas
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
		return &Schema{Type: "string", Format: "date-time"}, schemas
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
						if canAddRefSchemaForType(key) {
							schemas[key] = schema
							schema = addRefSchemaForType(key)
						}

						maps.Copy(schemas, newSchemas)
						markUsedType(usedTypes, goType, schema)

						return schema, schemas
					}
				}
			}
		}

		if !isPrimitive && goType != "" {
			// Register a placeholder under the referenced name so we never
			// emit a $ref to a component nothing else will populate.
			// Reached when a type isn't in metadata, isn't in the
			// wellKnownExternalTypes table, isn't in externalTypes, and
			// has no primitive mapping — typical of opaque types from
			// packages that weren't part of the analyzed module.
			derivedKey := strings.TrimPrefix(goType, "*")
			if _, exists := schemas[derivedKey]; !exists {
				schemas[derivedKey] = unresolvedExternalPlaceholder(derivedKey)
			}
			return addRefSchemaForType(goType), schemas
		}

		return schema, schemas
	}
}

func canAddRefSchemaForType(key string) bool {
	if metadata.IsPrimitiveType(key) || strings.HasPrefix(key, "[]") || strings.Contains(key, "map[") {
		return false
	}

	// Exclude _nested types from reference schema generation
	if strings.HasSuffix(key, "_nested") {
		return false
	}

	// Anonymous (inline) structs are registered as synthetic *Type
	// entries with a fixed key prefix (see metadata.AnonStructKey).
	// They have no name in Go and so should never become a $ref
	// target — they are inlined at the use site.
	if metadata.IsAnonStructTypeName(key) {
		return false
	}

	// Some anonymous structs reach the spec layer as their raw go/types
	// String() form (e.g. a "[]struct{...}" field type, optionally with a
	// package path glued on). They are nameless too — a $ref would dangle
	// with an invalid braced component name (Redoc "Invalid reference
	// token"). Inline them rather than referencing them.
	if isAnonStructLiteral(key) {
		return false
	}

	// Allow reference schemas for custom types
	return true
}

// isAnonStructLiteral reports whether a Go type string carries an inline
// anonymous-struct literal ("struct{...}"). Such literals arrive as their
// go/types String() form, sometimes with a leading package path glued on by
// generateStructSchema (e.g. "pkg.struct{Field string ...}").
func isAnonStructLiteral(goType string) bool {
	return strings.Contains(goType, "struct{")
}

// schemaFromAnonStructLiteral converts a Go anonymous-struct string into an
// inline object schema, honoring json tags. The body is parsed from the
// "struct{" token (any leading package path is ignored). It does NOT use
// go/parser: go/types renders field types with full import paths (e.g.
// "github.com/google/uuid.UUID"), which are not valid Go expressions, so
// parser.ParseExpr would reject the whole struct and drop every field. Instead
// it splits fields itself and hands each field-type string straight to
// mapGoTypeToOpenAPISchema, which already resolves import-qualified names,
// slices, maps, pointers, and nested anonymous structs.
func schemaFromAnonStructLiteral(usedTypes map[string]*Schema, goType string, meta *metadata.Metadata, cfg *APISpecConfig, visitedTypes map[string]bool) (*Schema, map[string]*Schema) {
	schemas := map[string]*Schema{}

	body, ok := anonStructBody(goType)
	if !ok {
		return nil, schemas
	}

	schema := &Schema{Type: "object"}
	for _, raw := range splitTopLevel(body, ';') {
		name, fieldType, tag, ok := parseAnonField(raw)
		if !ok {
			// Embedded field (no name) — nothing to key a property on, as the
			// named-struct path also skips unnamed members.
			continue
		}
		// Mirror encoding/json: a `json:"-"` tag or an unexported field is
		// never serialized, so it must not appear as a property.
		if jsonFieldOmitted(tag) || !ast.IsExported(name) {
			continue
		}

		fieldSchema, newSchemas := mapGoTypeToOpenAPISchema(usedTypes, fieldType, meta, cfg, visitedTypes)
		maps.Copy(schemas, newSchemas)

		propName := name
		if jsonName := extractJSONName(tag); jsonName != "" {
			propName = jsonName
		}
		if schema.Properties == nil {
			schema.Properties = map[string]*Schema{}
		}
		schema.Properties[propName] = fieldSchema
	}
	return schema, schemas
}

// anonStructBody returns the field list inside the first "struct{...}" found in
// s (the text between the braces), tracking brace depth so nested structs don't
// terminate the scan early. ok is false when there is no balanced struct body.
func anonStructBody(s string) (string, bool) {
	idx := strings.Index(s, "struct{")
	if idx < 0 {
		return "", false
	}
	open := idx + len("struct{") - 1 // position of '{'
	depth, inQuote, escaped := 0, false, false
	for i := open; i < len(s); i++ {
		c := s[i]
		if inQuote {
			switch {
			case escaped:
				escaped = false
			case c == '\\':
				escaped = true
			case c == '"':
				inQuote = false
			}
			continue
		}
		switch c {
		case '"':
			inQuote = true
		case '{', '[', '(':
			depth++
		case '}', ']', ')':
			depth--
			if c == '}' && depth == 0 {
				return s[open+1 : i], true
			}
		}
	}
	return "", false
}

// splitTopLevel splits s on sep, but only at bracket depth 0 and never inside a
// double-quoted string (struct tags). This keeps nested "struct{a int; b int}"
// field types and "func(a, b int)" signatures intact.
func splitTopLevel(s string, sep byte) []string {
	var parts []string
	depth, inQuote, escaped, start := 0, false, false, 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inQuote {
			switch {
			case escaped:
				escaped = false
			case c == '\\':
				escaped = true
			case c == '"':
				inQuote = false
			}
			continue
		}
		switch c {
		case '"':
			inQuote = true
		case '{', '[', '(':
			depth++
		case '}', ']', ')':
			depth--
		case sep:
			if depth == 0 {
				parts = append(parts, strings.TrimSpace(s[start:i]))
				start = i + 1
			}
		}
	}
	parts = append(parts, strings.TrimSpace(s[start:]))
	return parts
}

// parseAnonField splits one go/types-rendered struct field ("Name Type" or
// "Name Type \"tag\"", or a bare "Type"/"Type \"tag\"" for embedded fields)
// into its name, type string, and unquoted tag. ok is false for embedded
// fields (no field name).
//
// The struct tag, when present, is a quoted string at brace depth 0 — but the
// field TYPE may itself contain quoted strings (a nested struct's own tags) at
// depth > 0, so the tag is the first quote seen at depth 0, not the first quote
// overall. Once the tag is stripped, the name is the leading identifier up to
// its following space (always at depth 0, since names carry no brackets).
func parseAnonField(field string) (name, fieldType, tag string, ok bool) {
	field = strings.TrimSpace(field)
	if field == "" {
		return "", "", "", false
	}

	depth := 0
	for i := 0; i < len(field); i++ {
		c := field[i]
		switch c {
		case '{', '[', '(':
			depth++
		case '}', ']', ')':
			depth--
		case '"':
			if depth == 0 {
				if unq, err := strconv.Unquote(field[i:]); err == nil {
					tag = unq
				}
				field = strings.TrimSpace(field[:i])
				i = len(field) // done scanning
				continue
			}
			// A quoted string nested inside the type (e.g. a nested struct's
			// tag). Skip to its closing quote so its spaces/braces are ignored.
			for i++; i < len(field); i++ {
				if field[i] == '\\' {
					i++
					continue
				}
				if field[i] == '"' {
					break
				}
			}
		}
	}

	sp := strings.IndexByte(field, ' ')
	if sp < 0 {
		return "", field, tag, false
	}
	return field[:sp], strings.TrimSpace(field[sp+1:]), tag, true
}

// isPrimitiveShapedSchema reports whether a schema carries only scalar/array
// fields (Type/Format/Enum/Min/Max) with no structural members. Used by
// shouldPromoteToComponent to keep simple shapes inline.
func isPrimitiveShapedSchema(s *Schema) bool {
	if s == nil || s.Ref != "" {
		return false
	}
	if len(s.Properties) > 0 || len(s.AllOf) > 0 || len(s.OneOf) > 0 || len(s.AnyOf) > 0 {
		return false
	}
	if s.AdditionalProperties != nil {
		return false
	}
	return s.Type != "" && s.Type != "object"
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

// parseArraySize parses the array size from Go array syntax
// Returns the size as an integer, or nil if parsing fails or no size constraint
func parseArraySize(sizeStr string) *int {
	if sizeStr == "" {
		return nil
	}

	// Handle "..." (variable length array)
	if sizeStr == "..." {
		return nil
	}

	// Try to parse as integer
	if size, err := strconv.Atoi(sizeStr); err == nil {
		return &size
	}

	// If it's not a number, return nil (no size constraint)
	return nil
}
