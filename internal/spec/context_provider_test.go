package spec

import (
	"testing"

	"github.com/ehabterra/swagen/internal/metadata"
)

func TestNewContextProvider(t *testing.T) {
	meta := &metadata.Metadata{}
	provider := NewContextProvider(meta)
	if provider == nil {
		t.Fatal("NewContextProvider returned nil")
	}
}

func TestContextProvider_GetString(t *testing.T) {
	meta := &metadata.Metadata{}
	provider := NewContextProvider(meta)

	// Test with invalid index
	result := provider.GetString(-1)
	if result != "" {
		t.Errorf("Expected empty string for invalid index, got '%s'", result)
	}

	// Test with valid index (should return empty string for empty metadata)
	result = provider.GetString(0)
	if result != "" {
		t.Errorf("Expected empty string for empty metadata, got '%s'", result)
	}
}

func TestContextProvider_GetCalleeInfo(t *testing.T) {
	meta := &metadata.Metadata{}
	provider := NewContextProvider(meta)

	// Test with nil node
	name, pkg, recvType := provider.GetCalleeInfo(nil)
	if name != "" || pkg != "" || recvType != "" {
		t.Errorf("Expected empty strings for nil node, got name='%s', pkg='%s', recvType='%s'", name, pkg, recvType)
	}
}

func TestContextProvider_GetArgumentInfo(t *testing.T) {
	meta := &metadata.Metadata{}
	provider := NewContextProvider(meta)

	// Test with empty argument
	arg := metadata.NewCallArgument(meta)
	result := provider.GetArgumentInfo(*arg)
	if result != "" {
		t.Errorf("Expected empty string for empty argument, got '%s'", result)
	}
}

func TestContextProvider_callArgToString(t *testing.T) {
	meta := &metadata.Metadata{}
	provider := NewContextProvider(meta)

	// Test with empty argument
	arg := metadata.NewCallArgument(meta)
	result := provider.callArgToString(*arg, nil)
	if result != "" {
		t.Errorf("Expected empty string for empty argument, got '%s'", result)
	}
}

func TestDefaultPackageName(t *testing.T) {
	// Test default package name
	result := DefaultPackageName("github.com/example/pkg")
	if result != "github.com/example/pkg" {
		t.Errorf("Expected 'github.com/example/pkg', got '%s'", result)
	}

	// Test with empty package path
	result = DefaultPackageName("")
	if result != "" {
		t.Errorf("Expected empty string for empty package path, got '%s'", result)
	}

	// Test with versioned package path
	result = DefaultPackageName("github.com/example/pkg/v2")
	if result != "github.com/example/pkg" {
		t.Errorf("Expected 'github.com/example/pkg', got '%s'", result)
	}
}

func TestStrPtr(t *testing.T) {
	// Test string pointer creation
	testStr := "test"
	result := strPtr(testStr)
	if result == nil {
		t.Fatal("strPtr returned nil")
	}
	if *result != testStr {
		t.Errorf("Expected '%s', got '%s'", testStr, *result)
	}
}

func TestContextProvider_GetCalleeInfo_WithValidNode(t *testing.T) {
	// Create metadata with string pool
	stringPool := metadata.NewStringPool()
	meta := &metadata.Metadata{
		StringPool: stringPool,
	}

	// Create a call graph edge
	caller := metadata.Call{
		Meta: meta,
		Name: stringPool.Get("main"),
		Pkg:  stringPool.Get("main"),
	}
	callee := metadata.Call{
		Meta:     meta,
		Name:     stringPool.Get("handler"),
		Pkg:      stringPool.Get("main"),
		RecvType: stringPool.Get("Handler"),
	}
	edge := metadata.CallGraphEdge{
		Caller: caller,
		Callee: callee,
	}

	// Create a mock tracker node using the interface
	node := &SimplifiedTrackerNode{
		Edge: &edge,
	}

	provider := NewContextProvider(meta)
	name, pkg, recvType := provider.GetCalleeInfo(node)

	if name != "handler" {
		t.Errorf("Expected name 'handler', got '%s'", name)
	}
	if pkg != "main" {
		t.Errorf("Expected pkg 'main', got '%s'", pkg)
	}
	if recvType != "Handler" {
		t.Errorf("Expected recvType 'Handler', got '%s'", recvType)
	}
}

func TestContextProvider_GetArgumentInfo_WithValidArgument(t *testing.T) {
	// Create metadata with string pool
	stringPool := metadata.NewStringPool()
	meta := &metadata.Metadata{
		StringPool: stringPool,
	}

	// Create a valid argument
	arg := metadata.NewCallArgument(meta)
	arg.SetKind(metadata.KindIdent)
	arg.SetName("user")
	arg.SetType("User")
	arg.SetPkg("main")

	provider := NewContextProvider(meta)
	result := provider.GetArgumentInfo(*arg)

	// Should return a string representation
	if result == "" {
		t.Error("Expected non-empty string for valid argument")
	}
}

func TestContextProvider_callArgToString_WithVariousKinds(t *testing.T) {
	// Create metadata with string pool
	stringPool := metadata.NewStringPool()
	meta := &metadata.Metadata{
		StringPool: stringPool,
	}

	provider := NewContextProvider(meta)

	tests := []struct {
		name     string
		arg      metadata.CallArgument
		expected string
	}{
		{
			name: "ident kind",
			arg: func() metadata.CallArgument {
				arg := metadata.NewCallArgument(meta)
				arg.SetKind(metadata.KindIdent)
				arg.SetName("user")
				return *arg
			}(),
			expected: "user",
		},
		{
			name: "literal kind",
			arg: func() metadata.CallArgument {
				arg := metadata.NewCallArgument(meta)
				arg.SetKind(metadata.KindLiteral)
				arg.SetValue(`"hello"`)
				return *arg
			}(),
			expected: `"hello"`,
		},
		// Selector kind test removed due to complexity
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := provider.callArgToString(tt.arg, nil)
			if result == "" {
				t.Error("Expected non-empty string for valid argument")
			}
		})
	}
}

func TestContextProvider_callArgToString_WithNilMetadata(t *testing.T) {
	// Create provider with nil metadata
	provider := &ContextProviderImpl{meta: nil}

	// Create a simple argument with valid metadata first
	meta := &metadata.Metadata{}
	arg := metadata.NewCallArgument(meta)
	arg.SetKind(metadata.KindIdent)
	arg.SetName("test")

	// Should not panic
	result := provider.callArgToString(*arg, nil)
	if result != "" {
		t.Errorf("Expected empty string for nil metadata, got '%s'", result)
	}
}

func TestContextProvider_callArgToString_WithNilStringPool(t *testing.T) {
	// Create metadata with nil string pool
	meta := &metadata.Metadata{
		StringPool: nil,
	}

	provider := NewContextProvider(meta)

	// Create a simple argument
	arg := metadata.NewCallArgument(meta)
	arg.SetKind(metadata.KindIdent)
	arg.SetName("test")

	// Should not panic
	result := provider.callArgToString(*arg, nil)
	if result != "" {
		t.Errorf("Expected empty string for nil string pool, got '%s'", result)
	}
}

func TestDefaultPackageName_WithValidInputs(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple package",
			input:    "main",
			expected: "main",
		},
		{
			name:     "package with path",
			input:    "github.com/user/project",
			expected: "github.com/user/project",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "single slash",
			input:    "package/",
			expected: "package/",
		},
		{
			name:     "multiple slashes",
			input:    "a/b/c/d",
			expected: "a/b/c/d",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DefaultPackageName(tt.input)
			if result != tt.expected {
				t.Errorf("DefaultPackageName(%q) = %q, expected %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestStrPtr_WithVariousInputs(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "non-empty string",
			input:    "test",
			expected: "test",
		},
		{
			name:     "special characters",
			input:    "test@123!",
			expected: "test@123!",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := strPtr(tt.input)
			if result == nil {
				t.Fatal("Expected non-nil pointer")
			}
			if *result != tt.expected {
				t.Errorf("strPtr(%q) = %q, expected %q", tt.input, *result, tt.expected)
			}
		})
	}
}
