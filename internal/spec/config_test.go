package spec

import (
	"testing"
)

func TestDefaultConfigs(t *testing.T) {
	tests := []struct {
		name string
		fn   func() *APISpecConfig
	}{
		{"Chi", DefaultChiConfig},
		{"Echo", DefaultEchoConfig},
		{"Fiber", DefaultFiberConfig},
		{"Gin", DefaultGinConfig},
		{"HTTP", DefaultHTTPConfig},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := tt.fn()
			if config == nil {
				t.Fatal("Config should not be nil")
			}

			// Test basic structure
			if config.Framework.RoutePatterns == nil {
				t.Error("RoutePatterns should not be nil")
			}

			if len(config.Framework.RoutePatterns) == 0 {
				t.Error("RoutePatterns should not be empty")
			}

			// Test that at least one route pattern exists
			foundRoutePattern := false
			for _, pattern := range config.Framework.RoutePatterns {
				if pattern.CallRegex != "" {
					foundRoutePattern = true
					break
				}
			}
			if !foundRoutePattern {
				t.Error("Should have at least one route pattern with CallRegex")
			}
		})
	}
}

func TestDefaultChiConfig(t *testing.T) {
	config := DefaultChiConfig()

	// Test Chi-specific patterns
	// Chi uses a regex pattern that includes all HTTP methods
	expectedRegex := `(?i)(GET|POST|PUT|DELETE|PATCH|OPTIONS|HEAD)$`
	found := false

	for _, pattern := range config.Framework.RoutePatterns {
		if pattern.CallRegex == expectedRegex {
			found = true
			break
		}
	}

	if !found {
		t.Errorf("Expected Chi route pattern with regex: %s", expectedRegex)
	}

	// Test that Chi has the expected configuration
	if !config.Framework.RoutePatterns[0].MethodFromCall {
		t.Error("Chi should have MethodFromCall enabled")
	}

	if !config.Framework.RoutePatterns[0].PathFromArg {
		t.Error("Chi should have PathFromArg enabled")
	}

	if !config.Framework.RoutePatterns[0].HandlerFromArg {
		t.Error("Chi should have HandlerFromArg enabled")
	}
}

func TestDefaultEchoConfig(t *testing.T) {
	config := DefaultEchoConfig()

	// Test Echo-specific patterns
	// Echo should have route patterns
	if len(config.Framework.RoutePatterns) == 0 {
		t.Error("Echo config should have route patterns")
	}

	// Test that Echo patterns are configured
	for _, pattern := range config.Framework.RoutePatterns {
		if pattern.CallRegex != "" {
			// Found at least one pattern
			return
		}
	}
	t.Error("Echo config should have at least one route pattern")
}

func TestDefaultFiberConfig(t *testing.T) {
	config := DefaultFiberConfig()

	// Test Fiber-specific patterns
	// Fiber should have route patterns
	if len(config.Framework.RoutePatterns) == 0 {
		t.Error("Fiber config should have route patterns")
	}

	// Test that Fiber patterns are configured
	for _, pattern := range config.Framework.RoutePatterns {
		if pattern.CallRegex != "" {
			// Found at least one pattern
			return
		}
	}
	t.Error("Fiber config should have at least one route pattern")
}

func TestDefaultGinConfig(t *testing.T) {
	config := DefaultGinConfig()

	// Test Gin-specific patterns
	// Gin should have route patterns
	if len(config.Framework.RoutePatterns) == 0 {
		t.Error("Gin config should have route patterns")
	}

	// Test that Gin patterns are configured
	for _, pattern := range config.Framework.RoutePatterns {
		if pattern.CallRegex != "" {
			// Found at least one pattern
			return
		}
	}
	t.Error("Gin config should have at least one route pattern")
}

func TestDefaultHTTPConfig(t *testing.T) {
	config := DefaultHTTPConfig()

	// Test HTTP-specific patterns
	// HTTP should have route patterns
	if len(config.Framework.RoutePatterns) == 0 {
		t.Error("HTTP config should have route patterns")
	}

	// Test that HTTP patterns are configured
	for _, pattern := range config.Framework.RoutePatterns {
		if pattern.CallRegex != "" {
			// Found at least one pattern
			return
		}
	}
	t.Error("HTTP config should have at least one route pattern")
}

func TestConfigStructure(t *testing.T) {
	config := DefaultGinConfig()

	// Test that config has all required sections
	if config.Framework.RoutePatterns == nil {
		t.Fatal("RoutePatterns should exist")
	}

	if config.Framework.RequestBodyPatterns == nil {
		t.Fatal("RequestBodyPatterns should exist")
	}

	if config.Framework.ResponsePatterns == nil {
		t.Fatal("ResponsePatterns should exist")
	}

	if config.Framework.ParamPatterns == nil {
		t.Fatal("ParamPatterns should exist")
	}

	if config.Framework.MountPatterns == nil {
		t.Fatal("MountPatterns should exist")
	}

	// Test that each section has at least some patterns
	if len(config.Framework.RoutePatterns) == 0 {
		t.Error("RoutePatterns should not be empty")
	}

	if len(config.Framework.RequestBodyPatterns) == 0 {
		t.Error("RequestBodyPatterns should not be empty")
	}

	if len(config.Framework.ResponsePatterns) == 0 {
		t.Error("ResponsePatterns should not be empty")
	}

	if len(config.Framework.ParamPatterns) == 0 {
		t.Error("ParamPatterns should not be empty")
	}

	if len(config.Framework.MountPatterns) == 0 {
		t.Error("MountPatterns should not be empty")
	}
}

func TestConfigPatternValidation(t *testing.T) {
	config := DefaultGinConfig()

	// Test route patterns have required fields
	for i, pattern := range config.Framework.RoutePatterns {
		if pattern.CallRegex == "" {
			t.Errorf("RoutePattern[%d] should have CallRegex", i)
		}
	}

	// Test request body patterns have required fields
	for i, pattern := range config.Framework.RequestBodyPatterns {
		if pattern.CallRegex == "" {
			t.Errorf("RequestBodyPattern[%d] should have CallRegex", i)
		}
	}

	// Test response patterns have required fields
	for i, pattern := range config.Framework.ResponsePatterns {
		if pattern.CallRegex == "" {
			t.Errorf("ResponsePattern[%d] should have CallRegex", i)
		}
	}

	// Test param patterns have required fields
	for i, pattern := range config.Framework.ParamPatterns {
		if pattern.CallRegex == "" {
			t.Errorf("ParamPattern[%d] should have CallRegex", i)
		}
		if pattern.ParamIn == "" {
			t.Errorf("ParamPattern[%d] should have ParamIn", i)
		}
	}

	// Test mount patterns have required fields
	for i, pattern := range config.Framework.MountPatterns {
		if pattern.CallRegex == "" {
			t.Errorf("MountPattern[%d] should have CallRegex", i)
		}
		if !pattern.IsMount {
			t.Errorf("MountPattern[%d] should have IsMount=true", i)
		}
	}
}
