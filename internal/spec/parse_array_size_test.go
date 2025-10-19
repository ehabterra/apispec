package spec

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseArraySize(t *testing.T) {
	tests := []struct {
		name     string
		sizeStr  string
		expected *int
	}{
		{"empty string", "", nil},
		{"variable length", "...", nil},
		{"zero", "0", intPtr(0)},
		{"positive number", "5", intPtr(5)},
		{"large number", "100", intPtr(100)},
		{"single digit", "1", intPtr(1)},
		{"double digit", "16", intPtr(16)},
		{"triple digit", "256", intPtr(256)},
		{"invalid string", "abc", nil},
		{"mixed string", "5abc", nil},
		{"negative number", "-5", intPtr(-5)},
		{"float", "5.5", nil},
		{"whitespace", " 5 ", nil},
		{"zero with prefix", "05", intPtr(5)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseArraySize(tt.sizeStr)
			if tt.expected == nil {
				assert.Nil(t, result, "parseArraySize(%q) should return nil", tt.sizeStr)
			} else {
				assert.NotNil(t, result, "parseArraySize(%q) should not return nil", tt.sizeStr)
				assert.Equal(t, *tt.expected, *result, "parseArraySize(%q) = %v, want %v", tt.sizeStr, *result, *tt.expected)
			}
		})
	}
}

// Helper function to create int pointer
func intPtr(i int) *int {
	return &i
}
