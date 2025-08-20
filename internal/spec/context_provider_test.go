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

func TestContextProvider_GetCallerInfo(t *testing.T) {
	meta := &metadata.Metadata{}
	provider := NewContextProvider(meta)

	// Test with nil node
	name, pkg := provider.GetCallerInfo(nil)
	if name != "" || pkg != "" {
		t.Errorf("Expected empty strings for nil node, got name='%s', pkg='%s'", name, pkg)
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
