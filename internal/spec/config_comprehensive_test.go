package spec

import (
	"testing"
)

func TestIncludeExclude_ShouldIncludeFile(t *testing.T) {
	tests := []struct {
		name     string
		patterns []string
		filePath string
		expected bool
	}{
		{
			name:     "no patterns - include everything",
			patterns: []string{},
			filePath: "any/file.go",
			expected: true,
		},
		{
			name:     "exact match",
			patterns: []string{"main.go"},
			filePath: "main.go",
			expected: true,
		},
		{
			name:     "wildcard match",
			patterns: []string{"*.go"},
			filePath: "main.go",
			expected: true,
		},
		{
			name:     "directory wildcard",
			patterns: []string{"cmd/**/*.go"},
			filePath: "cmd/apispec/main.go",
			expected: true,
		},
		{
			name:     "no match",
			patterns: []string{"*.go"},
			filePath: "README.md",
			expected: false,
		},
		{
			name:     "multiple patterns - first matches",
			patterns: []string{"*.go", "*.md"},
			filePath: "main.go",
			expected: true,
		},
		{
			name:     "multiple patterns - second matches",
			patterns: []string{"*.go", "*.md"},
			filePath: "README.md",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ie := &IncludeExclude{Files: tt.patterns}
			result := ie.ShouldIncludeFile(tt.filePath)
			if result != tt.expected {
				t.Errorf("ShouldIncludeFile() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestIncludeExclude_ShouldIncludePackage(t *testing.T) {
	tests := []struct {
		name     string
		patterns []string
		pkgPath  string
		expected bool
	}{
		{
			name:     "no patterns - include everything",
			patterns: []string{},
			pkgPath:  "any/package",
			expected: true,
		},
		{
			name:     "exact match",
			patterns: []string{"main"},
			pkgPath:  "main",
			expected: true,
		},
		{
			name:     "wildcard match",
			patterns: []string{"cmd/*"},
			pkgPath:  "cmd/apispec",
			expected: true,
		},
		{
			name:     "nested package",
			patterns: []string{"internal/**"},
			pkgPath:  "internal/spec",
			expected: true,
		},
		{
			name:     "no match",
			patterns: []string{"cmd/*"},
			pkgPath:  "pkg/utils",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ie := &IncludeExclude{Packages: tt.patterns}
			result := ie.ShouldIncludePackage(tt.pkgPath)
			if result != tt.expected {
				t.Errorf("ShouldIncludePackage() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestIncludeExclude_ShouldIncludeFunction(t *testing.T) {
	tests := []struct {
		name     string
		patterns []string
		funcName string
		expected bool
	}{
		{
			name:     "no patterns - include everything",
			patterns: []string{},
			funcName: "anyFunction",
			expected: true,
		},
		{
			name:     "exact match",
			patterns: []string{"main"},
			funcName: "main",
			expected: true,
		},
		{
			name:     "wildcard match",
			patterns: []string{"*Handler"},
			funcName: "userHandler",
			expected: true,
		},
		{
			name:     "prefix match",
			patterns: []string{"get*"},
			funcName: "getUser",
			expected: true,
		},
		{
			name:     "no match",
			patterns: []string{"*Handler"},
			funcName: "processData",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ie := &IncludeExclude{Functions: tt.patterns}
			result := ie.ShouldIncludeFunction(tt.funcName)
			if result != tt.expected {
				t.Errorf("ShouldIncludeFunction() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestIncludeExclude_ShouldIncludeType(t *testing.T) {
	tests := []struct {
		name     string
		patterns []string
		typeName string
		expected bool
	}{
		{
			name:     "no patterns - include everything",
			patterns: []string{},
			typeName: "anyType",
			expected: true,
		},
		{
			name:     "exact match",
			patterns: []string{"User"},
			typeName: "User",
			expected: true,
		},
		{
			name:     "wildcard match",
			typeName: "UserHandler",
			patterns: []string{"*Handler"},
			expected: true,
		},
		{
			name:     "suffix match",
			patterns: []string{"*Request"},
			typeName: "CreateUserRequest",
			expected: true,
		},
		{
			name:     "no match",
			patterns: []string{"*Handler"},
			typeName: "UserService",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ie := &IncludeExclude{Types: tt.patterns}
			result := ie.ShouldIncludeType(tt.typeName)
			if result != tt.expected {
				t.Errorf("ShouldIncludeType() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestIncludeExclude_ShouldExcludeFile(t *testing.T) {
	tests := []struct {
		name     string
		patterns []string
		filePath string
		expected bool
	}{
		{
			name:     "no patterns - exclude nothing",
			patterns: []string{},
			filePath: "any/file.go",
			expected: false,
		},
		{
			name:     "exact match - exclude",
			patterns: []string{"test.go"},
			filePath: "test.go",
			expected: true,
		},
		{
			name:     "wildcard match - exclude",
			patterns: []string{"*_test.go"},
			filePath: "main_test.go",
			expected: true,
		},
		{
			name:     "no match - don't exclude",
			patterns: []string{"*_test.go"},
			filePath: "main.go",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ie := &IncludeExclude{Files: tt.patterns}
			result := ie.ShouldExcludeFile(tt.filePath)
			if result != tt.expected {
				t.Errorf("ShouldExcludeFile() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestIncludeExclude_ShouldExcludePackage(t *testing.T) {
	tests := []struct {
		name     string
		patterns []string
		pkgPath  string
		expected bool
	}{
		{
			name:     "no patterns - exclude nothing",
			patterns: []string{},
			pkgPath:  "any/package",
			expected: false,
		},
		{
			name:     "exact match - exclude",
			patterns: []string{"vendor"},
			pkgPath:  "vendor",
			expected: true,
		},
		{
			name:     "wildcard match - exclude",
			patterns: []string{"vendor/*"},
			pkgPath:  "vendor/github.com",
			expected: true,
		},
		{
			name:     "no match - don't exclude",
			patterns: []string{"vendor/*"},
			pkgPath:  "internal/spec",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ie := &IncludeExclude{Packages: tt.patterns}
			result := ie.ShouldExcludePackage(tt.pkgPath)
			if result != tt.expected {
				t.Errorf("ShouldExcludePackage() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestIncludeExclude_ShouldExcludeFunction(t *testing.T) {
	tests := []struct {
		name     string
		patterns []string
		funcName string
		expected bool
	}{
		{
			name:     "no patterns - exclude nothing",
			patterns: []string{},
			funcName: "anyFunction",
			expected: false,
		},
		{
			name:     "exact match - exclude",
			patterns: []string{"init"},
			funcName: "init",
			expected: true,
		},
		{
			name:     "wildcard match - exclude",
			patterns: []string{"*Test"},
			funcName: "userTest",
			expected: true,
		},
		{
			name:     "no match - don't exclude",
			patterns: []string{"*Test"},
			funcName: "getUser",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ie := &IncludeExclude{Functions: tt.patterns}
			result := ie.ShouldExcludeFunction(tt.funcName)
			if result != tt.expected {
				t.Errorf("ShouldExcludeFunction() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestIncludeExclude_ShouldExcludeType(t *testing.T) {
	tests := []struct {
		name     string
		patterns []string
		typeName string
		expected bool
	}{
		{
			name:     "no patterns - exclude nothing",
			patterns: []string{},
			typeName: "anyType",
			expected: false,
		},
		{
			name:     "exact match - exclude",
			patterns: []string{"Test"},
			typeName: "Test",
			expected: true,
		},
		{
			name:     "wildcard match - exclude",
			patterns: []string{"*Test"},
			typeName: "UserTest",
			expected: true,
		},
		{
			name:     "no match - don't exclude",
			patterns: []string{"*Test"},
			typeName: "User",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ie := &IncludeExclude{Types: tt.patterns}
			result := ie.ShouldExcludeType(tt.typeName)
			if result != tt.expected {
				t.Errorf("ShouldExcludeType() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestAPISpecConfig_ShouldIncludeFile(t *testing.T) {
	config := &APISpecConfig{
		Include: IncludeExclude{
			Files: []string{"*.go"},
		},
		Exclude: IncludeExclude{
			Files: []string{"*_test.go"},
		},
	}

	tests := []struct {
		name     string
		filePath string
		expected bool
	}{
		{
			name:     "include pattern matches",
			filePath: "main.go",
			expected: true,
		},
		{
			name:     "exclude pattern matches",
			filePath: "main_test.go",
			expected: false,
		},
		{
			name:     "neither pattern matches",
			filePath: "README.md",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := config.ShouldIncludeFile(tt.filePath)
			if result != tt.expected {
				t.Errorf("ShouldIncludeFile() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestAPISpecConfig_ShouldIncludePackage(t *testing.T) {
	config := &APISpecConfig{
		Include: IncludeExclude{
			Packages: []string{"internal/*"},
		},
		Exclude: IncludeExclude{
			Packages: []string{"internal/vendor"},
		},
	}

	tests := []struct {
		name     string
		pkgPath  string
		expected bool
	}{
		{
			name:     "include pattern matches",
			pkgPath:  "internal/spec",
			expected: true,
		},
		{
			name:     "exclude pattern matches",
			pkgPath:  "internal/vendor",
			expected: false,
		},
		{
			name:     "neither pattern matches",
			pkgPath:  "cmd/apispec",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := config.ShouldIncludePackage(tt.pkgPath)
			if result != tt.expected {
				t.Errorf("ShouldIncludePackage() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestAPISpecConfig_ShouldIncludeFunction(t *testing.T) {
	config := &APISpecConfig{
		Include: IncludeExclude{
			Functions: []string{"get*"},
		},
		Exclude: IncludeExclude{
			Functions: []string{"*Test"},
		},
	}

	tests := []struct {
		name     string
		funcName string
		expected bool
	}{
		{
			name:     "include pattern matches",
			funcName: "getUser",
			expected: true,
		},
		{
			name:     "exclude pattern matches",
			funcName: "userTest",
			expected: false,
		},
		{
			name:     "neither pattern matches",
			funcName: "processData",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := config.ShouldIncludeFunction(tt.funcName)
			if result != tt.expected {
				t.Errorf("ShouldIncludeFunction() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestAPISpecConfig_ShouldIncludeType(t *testing.T) {
	config := &APISpecConfig{
		Include: IncludeExclude{
			Types: []string{"*Handler"},
		},
		Exclude: IncludeExclude{
			Types: []string{"*Test"},
		},
	}

	tests := []struct {
		name     string
		typeName string
		expected bool
	}{
		{
			name:     "include pattern matches",
			typeName: "UserHandler",
			expected: true,
		},
		{
			name:     "exclude pattern matches",
			typeName: "UserTest",
			expected: false,
		},
		{
			name:     "neither pattern matches",
			typeName: "UserService",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := config.ShouldIncludeType(tt.typeName)
			if result != tt.expected {
				t.Errorf("ShouldIncludeType() = %v, want %v", result, tt.expected)
			}
		})
	}
}
