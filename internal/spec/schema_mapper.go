package spec

import (
	"fmt"
	"net/http"
	"strings"
)

// SchemaMapperImpl implements SchemaMapper
type SchemaMapperImpl struct {
	cfg *APISpecConfig
}

// NewSchemaMapper creates a new schema mapper
func NewSchemaMapper(cfg *APISpecConfig) *SchemaMapperImpl {
	return &SchemaMapperImpl{
		cfg: cfg,
	}
}

// MapGoTypeToOpenAPISchema maps Go types to OpenAPI schemas
func (s *SchemaMapperImpl) MapGoTypeToOpenAPISchema(goType string) *Schema {
	// Check type mappings first
	for _, mapping := range s.cfg.TypeMapping {
		if mapping.GoType == goType {
			return mapping.OpenAPIType
		}
	}

	// Handle pointer types
	if strings.HasPrefix(goType, "*") {
		underlyingType := strings.TrimSpace(goType[1:])
		// For pointer types, we generate the same schema as the underlying type
		return s.MapGoTypeToOpenAPISchema(underlyingType)
	}

	// Handle map types
	if strings.HasPrefix(goType, "map[") {
		endIdx := strings.Index(goType, "]")
		if endIdx > 4 {
			keyType := goType[4:endIdx]
			valueType := strings.TrimSpace(goType[endIdx+1:])
			if keyType == "string" {
				// Handle specific value types for string-keyed maps
				switch valueType {
				case "string":
					return &Schema{
						Type:                 "object",
						AdditionalProperties: &Schema{Type: "string"},
					}
				case "interface{}", "any":
					// For interface{}, allow any type
					return &Schema{
						Type:                 "object",
						AdditionalProperties: &Schema{}, // Empty schema allows any type
					}
				case "int", "int8", "int16", "int32", "int64":
					return &Schema{
						Type:                 "object",
						AdditionalProperties: &Schema{Type: "integer"},
					}
				case "uint", "uint8", "uint16", "uint32", "uint64", "byte":
					return &Schema{
						Type:                 "object",
						AdditionalProperties: &Schema{Type: "integer", Minimum: 0},
					}
				case "float32", "float64":
					return &Schema{
						Type:                 "object",
						AdditionalProperties: &Schema{Type: "number"},
					}
				case "bool":
					return &Schema{
						Type:                 "object",
						AdditionalProperties: &Schema{Type: "boolean"},
					}
				default:
					// For custom types, recursively map the value type
					return &Schema{
						Type:                 "object",
						AdditionalProperties: s.MapGoTypeToOpenAPISchema(valueType),
					}
				}
			}
			// Non-string keys are not supported in OpenAPI, fallback to generic object
			return &Schema{Type: "object"}
		}
	}

	// Handle slice/array types
	if strings.HasPrefix(goType, "[]") {
		elemType := strings.TrimSpace(goType[2:])
		// For basic types, create inline array schema
		switch elemType {
		case "string":
			return &Schema{Type: "array", Items: &Schema{Type: "string"}}
		case "int", "int8", "int16", "int32", "int64":
			return &Schema{Type: "array", Items: &Schema{Type: "integer"}}
		case "uint", "uint8", "uint16", "uint32", "uint64", "byte":
			return &Schema{Type: "array", Items: &Schema{Type: "integer", Minimum: 0}}
		case "float32", "float64":
			return &Schema{Type: "array", Items: &Schema{Type: "number"}}
		case "bool":
			return &Schema{Type: "array", Items: &Schema{Type: "boolean"}}
		default:
			// For custom types, create a reference
			return &Schema{
				Type: "array",
				Items: &Schema{
					Ref: "#/components/schemas/" + schemaComponentNameReplacer.Replace(elemType),
				},
			}
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
	case "[]byte":
		return &Schema{Type: "string", Format: "byte"}
	case "[]string":
		return &Schema{Type: "array", Items: &Schema{Type: "string"}}
	case "[]int":
		return &Schema{Type: "array", Items: &Schema{Type: "integer"}}
	case "interface{}", "any", "struct{}", "nil", "error":
		// For standalone interface{}, allow any type
		return &Schema{
			Type: "object",
		}
	default:
		if goType != "" {
			// For custom types, create a reference
			return addRefSchemaForType(goType)
		}

		return nil
	}
}

// MapStatusCode maps a status code string to HTTP status code
func (s *SchemaMapperImpl) MapStatusCode(statusStr string) (int, bool) {
	// Remove quotes if present
	statusStr = strings.Trim(statusStr, "\"")

	// TODO: This is a temporary solution till having an external const value
	// resolution for status codes for the different frameworks.
	statusStr = strings.TrimPrefix(statusStr, "net/http.")
	statusStr = strings.TrimPrefix(statusStr, "github.com/gofiber/fiber.")

	// Check for net/http status constants
	switch statusStr {
	case "StatusOK":
		return http.StatusOK, true
	case "StatusCreated":
		return http.StatusCreated, true
	case "StatusAccepted":
		return http.StatusAccepted, true
	case "StatusNoContent":
		return http.StatusNoContent, true
	case "StatusBadRequest":
		return http.StatusBadRequest, true
	case "StatusUnauthorized":
		return http.StatusUnauthorized, true
	case "StatusForbidden":
		return http.StatusForbidden, true
	case "StatusNotFound":
		return http.StatusNotFound, true
	case "StatusConflict":
		return http.StatusConflict, true
	case "StatusInternalServerError":
		return http.StatusInternalServerError, true
	case "StatusNotImplemented":
		return http.StatusNotImplemented, true
	case "StatusBadGateway":
		return http.StatusBadGateway, true
	case "StatusServiceUnavailable":
		return http.StatusServiceUnavailable, true
	}

	// Try to parse as integer
	var status int
	_, err := fmt.Sscanf(statusStr, "%d", &status)
	if err != nil {
		return 0, false
	}

	return status, true
}

// MapMethodFromFunctionName extracts HTTP method from function name
func (s *SchemaMapperImpl) MapMethodFromFunctionName(funcName string) string {
	methods := []string{"GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS", "HEAD"}
	for _, method := range methods {
		if strings.Contains(strings.ToUpper(funcName), method) {
			return method
		}
	}
	return ""
}
