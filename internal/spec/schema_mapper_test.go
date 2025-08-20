package spec

import (
	"net/http"
	"testing"
)

func TestNewSchemaMapper(t *testing.T) {
	cfg := &SwagenConfig{}
	mapper := NewSchemaMapper(cfg)

	if mapper == nil {
		t.Fatal("NewSchemaMapper returned nil")
	}

	if mapper.cfg != cfg {
		t.Error("SchemaMapper config not set correctly")
	}
}

func TestSchemaMapper_TypeMappings(t *testing.T) {
	cfg := &SwagenConfig{
		TypeMapping: []TypeMapping{
			{
				GoType: "custom.Time",
				OpenAPIType: &Schema{
					Type:   "string",
					Format: "date-time",
				},
			},
			{
				GoType: "uuid.UUID",
				OpenAPIType: &Schema{
					Type:   "string",
					Format: "uuid",
				},
			},
		},
	}

	mapper := NewSchemaMapper(cfg)

	tests := []struct {
		name     string
		goType   string
		expected *Schema
	}{
		{
			name:   "custom time type",
			goType: "custom.Time",
			expected: &Schema{
				Type:   "string",
				Format: "date-time",
			},
		},
		{
			name:   "uuid type",
			goType: "uuid.UUID",
			expected: &Schema{
				Type:   "string",
				Format: "uuid",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mapper.MapGoTypeToOpenAPISchema(tt.goType)
			if !schemasEqual(result, tt.expected) {
				t.Errorf("expected %+v, got %+v", tt.expected, result)
			}
		})
	}
}

func TestSchemaMapper_PointerTypes(t *testing.T) {
	cfg := &SwagenConfig{}
	mapper := NewSchemaMapper(cfg)

	tests := []struct {
		name     string
		goType   string
		expected *Schema
	}{
		{
			name:     "pointer to string",
			goType:   "*string",
			expected: &Schema{Type: "string"},
		},
		{
			name:     "pointer to int",
			goType:   "*int",
			expected: &Schema{Type: "integer"},
		},
		{
			name:     "pointer to bool",
			goType:   "*bool",
			expected: &Schema{Type: "boolean"},
		},
		{
			name:     "pointer to custom type",
			goType:   "*User",
			expected: &Schema{Ref: "#/components/schemas/User"},
		},
		{
			name:     "pointer with spaces",
			goType:   "* User",
			expected: &Schema{Ref: "#/components/schemas/User"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mapper.MapGoTypeToOpenAPISchema(tt.goType)
			if !schemasEqual(result, tt.expected) {
				t.Errorf("expected %+v, got %+v", tt.expected, result)
			}
		})
	}
}

func TestSchemaMapper_MapTypes(t *testing.T) {
	cfg := &SwagenConfig{}
	mapper := NewSchemaMapper(cfg)

	tests := []struct {
		name     string
		goType   string
		expected *Schema
	}{
		{
			name:   "map[string]string",
			goType: "map[string]string",
			expected: &Schema{
				Type:                 "object",
				AdditionalProperties: &Schema{Type: "string"},
			},
		},
		{
			name:   "map[string]int",
			goType: "map[string]int",
			expected: &Schema{
				Type:                 "object",
				AdditionalProperties: &Schema{Type: "integer"},
			},
		},
		{
			name:   "map[string]uint",
			goType: "map[string]uint",
			expected: &Schema{
				Type:                 "object",
				AdditionalProperties: &Schema{Type: "integer", Minimum: 0},
			},
		},
		{
			name:   "map[string]float64",
			goType: "map[string]float64",
			expected: &Schema{
				Type:                 "object",
				AdditionalProperties: &Schema{Type: "number"},
			},
		},
		{
			name:   "map[string]bool",
			goType: "map[string]bool",
			expected: &Schema{
				Type:                 "object",
				AdditionalProperties: &Schema{Type: "boolean"},
			},
		},
		{
			name:   "map[string]interface{}",
			goType: "map[string]interface{}",
			expected: &Schema{
				Type:                 "object",
				AdditionalProperties: &Schema{},
			},
		},
		{
			name:   "map[string]any",
			goType: "map[string]any",
			expected: &Schema{
				Type:                 "object",
				AdditionalProperties: &Schema{},
			},
		},
		{
			name:   "map[string]User",
			goType: "map[string]User",
			expected: &Schema{
				Type:                 "object",
				AdditionalProperties: &Schema{Ref: "#/components/schemas/User"},
			},
		},
		{
			name:     "map[int]string - non-string key",
			goType:   "map[int]string",
			expected: &Schema{Type: "object"},
		},
		{
			name:     "map[float64]string - non-string key",
			goType:   "map[float64]string",
			expected: &Schema{Type: "object"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mapper.MapGoTypeToOpenAPISchema(tt.goType)
			if !schemasEqual(result, tt.expected) {
				t.Errorf("expected %+v, got %+v", tt.expected, result)
			}
		})
	}
}

func TestSchemaMapper_SliceTypes(t *testing.T) {
	cfg := &SwagenConfig{}
	mapper := NewSchemaMapper(cfg)

	tests := []struct {
		name     string
		goType   string
		expected *Schema
	}{
		{
			name:   "[]string",
			goType: "[]string",
			expected: &Schema{
				Type:  "array",
				Items: &Schema{Type: "string"},
			},
		},
		{
			name:   "[]int",
			goType: "[]int",
			expected: &Schema{
				Type:  "array",
				Items: &Schema{Type: "integer"},
			},
		},
		{
			name:   "[]uint",
			goType: "[]uint",
			expected: &Schema{
				Type:  "array",
				Items: &Schema{Type: "integer", Minimum: 0},
			},
		},
		{
			name:   "[]float64",
			goType: "[]float64",
			expected: &Schema{
				Type:  "array",
				Items: &Schema{Type: "number"},
			},
		},
		{
			name:   "[]bool",
			goType: "[]bool",
			expected: &Schema{
				Type:  "array",
				Items: &Schema{Type: "boolean"},
			},
		},
		{
			name:   "[]User",
			goType: "[]User",
			expected: &Schema{
				Type: "array",
				Items: &Schema{
					Ref: "#/components/schemas/User",
				},
			},
		},
		{
			name:   "[] with spaces",
			goType: "[] User",
			expected: &Schema{
				Type: "array",
				Items: &Schema{
					Ref: "#/components/schemas/User",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mapper.MapGoTypeToOpenAPISchema(tt.goType)
			if !schemasEqual(result, tt.expected) {
				t.Errorf("expected %+v, got %+v", tt.expected, result)
			}
		})
	}
}

func TestSchemaMapper_BasicTypes(t *testing.T) {
	cfg := &SwagenConfig{}
	mapper := NewSchemaMapper(cfg)

	tests := []struct {
		name     string
		goType   string
		expected *Schema
	}{
		{
			name:     "string",
			goType:   "string",
			expected: &Schema{Type: "string"},
		},
		{
			name:     "int",
			goType:   "int",
			expected: &Schema{Type: "integer"},
		},
		{
			name:     "int8",
			goType:   "int8",
			expected: &Schema{Type: "integer"},
		},
		{
			name:     "int16",
			goType:   "int16",
			expected: &Schema{Type: "integer"},
		},
		{
			name:     "int32",
			goType:   "int32",
			expected: &Schema{Type: "integer"},
		},
		{
			name:     "int64",
			goType:   "int64",
			expected: &Schema{Type: "integer"},
		},
		{
			name:     "uint",
			goType:   "uint",
			expected: &Schema{Type: "integer", Minimum: 0},
		},
		{
			name:     "uint8",
			goType:   "uint8",
			expected: &Schema{Type: "integer", Minimum: 0},
		},
		{
			name:     "uint16",
			goType:   "uint16",
			expected: &Schema{Type: "integer", Minimum: 0},
		},
		{
			name:     "uint32",
			goType:   "uint32",
			expected: &Schema{Type: "integer", Minimum: 0},
		},
		{
			name:     "uint64",
			goType:   "uint64",
			expected: &Schema{Type: "integer", Minimum: 0},
		},
		{
			name:     "byte",
			goType:   "byte",
			expected: &Schema{Type: "integer", Minimum: 0},
		},
		{
			name:     "float32",
			goType:   "float32",
			expected: &Schema{Type: "number"},
		},
		{
			name:     "float64",
			goType:   "float64",
			expected: &Schema{Type: "number"},
		},
		{
			name:     "bool",
			goType:   "bool",
			expected: &Schema{Type: "boolean"},
		},
		{
			name:     "[]byte",
			goType:   "[]byte",
			expected: &Schema{Type: "array", Items: &Schema{Type: "integer", Minimum: 0}},
		},
		{
			name:     "[]string",
			goType:   "[]string",
			expected: &Schema{Type: "array", Items: &Schema{Type: "string"}},
		},
		{
			name:     "[]int",
			goType:   "[]int",
			expected: &Schema{Type: "array", Items: &Schema{Type: "integer"}},
		},
		{
			name:     "interface{}",
			goType:   "interface{}",
			expected: &Schema{Type: "object"},
		},
		{
			name:     "any",
			goType:   "any",
			expected: &Schema{Type: "object"},
		},
		{
			name:     "struct{}",
			goType:   "struct{}",
			expected: &Schema{Type: "object"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mapper.MapGoTypeToOpenAPISchema(tt.goType)
			if !schemasEqual(result, tt.expected) {
				t.Errorf("expected %+v, got %+v", tt.expected, result)
			}
		})
	}
}

func TestSchemaMapper_CustomTypes(t *testing.T) {
	cfg := &SwagenConfig{}
	mapper := NewSchemaMapper(cfg)

	tests := []struct {
		name     string
		goType   string
		expected *Schema
	}{
		{
			name:     "User",
			goType:   "User",
			expected: &Schema{Ref: "#/components/schemas/User"},
		},
		{
			name:     "models.Product",
			goType:   "models.Product",
			expected: &Schema{Ref: "#/components/schemas/models.Product"},
		},
		{
			name:     "github.com/user/project/types.Response",
			goType:   "github.com/user/project/types.Response",
			expected: &Schema{Ref: "#/components/schemas/github.com_user_project_types.Response"},
		},
		{
			name:     "empty string",
			goType:   "",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mapper.MapGoTypeToOpenAPISchema(tt.goType)
			if !schemasEqual(result, tt.expected) {
				t.Errorf("expected %+v, got %+v", tt.expected, result)
			}
		})
	}
}

func TestMapStatusCode(t *testing.T) {
	cfg := &SwagenConfig{}
	mapper := NewSchemaMapper(cfg)

	tests := []struct {
		name        string
		statusStr   string
		expected    int
		shouldMatch bool
	}{
		// net/http constants
		{
			name:        "StatusOK",
			statusStr:   "StatusOK",
			expected:    http.StatusOK,
			shouldMatch: true,
		},
		{
			name:        "StatusCreated",
			statusStr:   "StatusCreated",
			expected:    http.StatusCreated,
			shouldMatch: true,
		},
		{
			name:        "StatusAccepted",
			statusStr:   "StatusAccepted",
			expected:    http.StatusAccepted,
			shouldMatch: true,
		},
		{
			name:        "StatusNoContent",
			statusStr:   "StatusNoContent",
			expected:    http.StatusNoContent,
			shouldMatch: true,
		},
		{
			name:        "StatusBadRequest",
			statusStr:   "StatusBadRequest",
			expected:    http.StatusBadRequest,
			shouldMatch: true,
		},
		{
			name:        "StatusUnauthorized",
			statusStr:   "StatusUnauthorized",
			expected:    http.StatusUnauthorized,
			shouldMatch: true,
		},
		{
			name:        "StatusForbidden",
			statusStr:   "StatusForbidden",
			expected:    http.StatusForbidden,
			shouldMatch: true,
		},
		{
			name:        "StatusNotFound",
			statusStr:   "StatusNotFound",
			expected:    http.StatusNotFound,
			shouldMatch: true,
		},
		{
			name:        "StatusConflict",
			statusStr:   "StatusConflict",
			expected:    http.StatusConflict,
			shouldMatch: true,
		},
		{
			name:        "StatusInternalServerError",
			statusStr:   "StatusInternalServerError",
			expected:    http.StatusInternalServerError,
			shouldMatch: true,
		},
		{
			name:        "StatusNotImplemented",
			statusStr:   "StatusNotImplemented",
			expected:    http.StatusNotImplemented,
			shouldMatch: true,
		},
		{
			name:        "StatusBadGateway",
			statusStr:   "StatusBadGateway",
			expected:    http.StatusBadGateway,
			shouldMatch: true,
		},
		{
			name:        "StatusServiceUnavailable",
			statusStr:   "StatusServiceUnavailable",
			expected:    http.StatusServiceUnavailable,
			shouldMatch: true,
		},
		// net/http prefixed constants
		{
			name:        "net/http.StatusOK",
			statusStr:   "net/http.StatusOK",
			expected:    http.StatusOK,
			shouldMatch: true,
		},
		// quoted strings
		{
			name:        "quoted StatusOK",
			statusStr:   "\"StatusOK\"",
			expected:    http.StatusOK,
			shouldMatch: true,
		},
		{
			name:        "quoted net/http.StatusOK",
			statusStr:   "\"net/http.StatusOK\"",
			expected:    http.StatusOK,
			shouldMatch: true,
		},
		// numeric strings
		{
			name:        "numeric 200",
			statusStr:   "200",
			expected:    200,
			shouldMatch: true,
		},
		{
			name:        "numeric 404",
			statusStr:   "404",
			expected:    404,
			shouldMatch: true,
		},
		{
			name:        "numeric 500",
			statusStr:   "500",
			expected:    500,
			shouldMatch: true,
		},
		// invalid cases
		{
			name:        "invalid string",
			statusStr:   "InvalidStatus",
			expected:    0,
			shouldMatch: false,
		},
		{
			name:        "empty string",
			statusStr:   "",
			expected:    0,
			shouldMatch: false,
		},
		{
			name:        "non-numeric string",
			statusStr:   "abc",
			expected:    0,
			shouldMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, matched := mapper.MapStatusCode(tt.statusStr)
			if matched != tt.shouldMatch {
				t.Errorf("expected match %v, got %v", tt.shouldMatch, matched)
			}
			if matched && result != tt.expected {
				t.Errorf("expected status %d, got %d", tt.expected, result)
			}
		})
	}
}

func TestMapMethodFromFunctionName(t *testing.T) {
	cfg := &SwagenConfig{}
	mapper := NewSchemaMapper(cfg)

	tests := []struct {
		name     string
		funcName string
		expected string
	}{
		{
			name:     "getUsers",
			funcName: "getUsers",
			expected: "GET",
		},
		{
			name:     "createUser",
			funcName: "createUser",
			expected: "",
		},
		{
			name:     "updateUser",
			funcName: "updateUser",
			expected: "",
		},
		{
			name:     "deleteUser",
			funcName: "deleteUser",
			expected: "DELETE",
		},
		{
			name:     "patchUser",
			funcName: "patchUser",
			expected: "PATCH",
		},
		{
			name:     "optionsHandler",
			funcName: "optionsHandler",
			expected: "OPTIONS",
		},
		{
			name:     "headRequest",
			funcName: "headRequest",
			expected: "HEAD",
		},
		{
			name:     "mixed case GetUser",
			funcName: "GetUser",
			expected: "GET",
		},
		{
			name:     "mixed case POSTHandler",
			funcName: "POSTHandler",
			expected: "POST",
		},
		{
			name:     "no method in name",
			funcName: "handler",
			expected: "",
		},
		{
			name:     "empty function name",
			funcName: "",
			expected: "",
		},
		{
			name:     "multiple methods - first wins",
			funcName: "getPostUser",
			expected: "GET",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mapper.MapMethodFromFunctionName(tt.funcName)
			if result != tt.expected {
				t.Errorf("expected method %s, got %s", tt.expected, result)
			}
		})
	}
}

// Helper function to compare schemas
func schemasEqual(a, b *Schema) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}

	if a.Type != b.Type {
		return false
	}
	if a.Format != b.Format {
		return false
	}
	if a.Ref != b.Ref {
		return false
	}
	if a.Minimum != b.Minimum {
		return false
	}
	if a.Maximum != b.Maximum {
		return false
	}
	if a.Items != nil && b.Items != nil {
		return schemasEqual(a.Items, b.Items)
	}
	if a.Items != nil || b.Items != nil {
		return false
	}
	if a.AdditionalProperties != nil && b.AdditionalProperties != nil {
		return schemasEqual(a.AdditionalProperties, b.AdditionalProperties)
	}
	if a.AdditionalProperties != nil || b.AdditionalProperties != nil {
		return false
	}

	return true
}
