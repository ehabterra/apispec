package spec

import (
	"strconv"
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

// Map of status code names
var HTTPStatusByName = map[string]int{
	// 1xx
	"StatusContinue":           100,
	"StatusSwitchingProtocols": 101,
	"StatusProcessing":         102,
	"StatusEarlyHints":         103,

	// 2xx
	"StatusOK":                   200,
	"StatusCreated":              201,
	"StatusAccepted":             202,
	"StatusNonAuthoritativeInfo": 203,
	"StatusNoContent":            204,
	"StatusResetContent":         205,
	"StatusPartialContent":       206,
	"StatusMultiStatus":          207,
	"StatusAlreadyReported":      208,
	"StatusIMUsed":               226,

	// 3xx
	"StatusMultipleChoices":   300,
	"StatusMovedPermanently":  301,
	"StatusFound":             302,
	"StatusSeeOther":          303,
	"StatusNotModified":       304,
	"StatusUseProxy":          305,
	"StatusTemporaryRedirect": 307,
	"StatusPermanentRedirect": 308,

	// 4xx
	"StatusBadRequest":                   400,
	"StatusUnauthorized":                 401,
	"StatusPaymentRequired":              402,
	"StatusForbidden":                    403,
	"StatusNotFound":                     404,
	"StatusMethodNotAllowed":             405,
	"StatusNotAcceptable":                406,
	"StatusProxyAuthRequired":            407,
	"StatusRequestTimeout":               408,
	"StatusConflict":                     409,
	"StatusGone":                         410,
	"StatusLengthRequired":               411,
	"StatusPreconditionFailed":           412,
	"StatusRequestEntityTooLarge":        413,
	"StatusRequestURITooLong":            414,
	"StatusUnsupportedMediaType":         415,
	"StatusRequestedRangeNotSatisfiable": 416,
	"StatusExpectationFailed":            417,
	"StatusTeapot":                       418,
	"StatusMisdirectedRequest":           421,
	"StatusUnprocessableEntity":          422,
	"StatusLocked":                       423,
	"StatusFailedDependency":             424,
	"StatusTooEarly":                     425,
	"StatusUpgradeRequired":              426,
	"StatusPreconditionRequired":         428,
	"StatusTooManyRequests":              429,
	"StatusRequestHeaderFieldsTooLarge":  431,
	"StatusUnavailableForLegalReasons":   451,

	// 5xx
	"StatusInternalServerError":           500,
	"StatusNotImplemented":                501,
	"StatusBadGateway":                    502,
	"StatusServiceUnavailable":            503,
	"StatusGatewayTimeout":                504,
	"StatusHTTPVersionNotSupported":       505,
	"StatusVariantAlsoNegotiates":         506,
	"StatusInsufficientStorage":           507,
	"StatusLoopDetected":                  508,
	"StatusNotExtended":                   510,
	"StatusNetworkAuthenticationRequired": 511,
}

// MapStatusCode maps a status code string to HTTP status code
func (s *SchemaMapperImpl) MapStatusCode(statusStr string) (int, bool) {
	// Remove quotes if present
	statusStr = strings.Trim(statusStr, "\"")

	if i := strings.LastIndex(statusStr, "."); i != -1 {
		statusStr = statusStr[i+1:]
	}

	// Check for net/http status constants
	statusInt, ok := HTTPStatusByName[statusStr]
	if ok {
		return statusInt, true
	}

	// Try to parse as integer
	status, err := strconv.Atoi(statusStr)
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
