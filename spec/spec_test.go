package spec

import (
	"testing"
)

func TestDefaultConfigs(t *testing.T) {
	// Test that all default configs can be created without errors
	ginConfig := DefaultGinConfig()
	if ginConfig == nil {
		t.Error("DefaultGinConfig returned nil")
	}

	chiConfig := DefaultChiConfig()
	if chiConfig == nil {
		t.Error("DefaultChiConfig returned nil")
	}

	echoConfig := DefaultEchoConfig()
	if echoConfig == nil {
		t.Error("DefaultEchoConfig returned nil")
	}

	fiberConfig := DefaultFiberConfig()
	if fiberConfig == nil {
		t.Error("DefaultFiberConfig returned nil")
	}

	httpConfig := DefaultHTTPConfig()
	if httpConfig == nil {
		t.Error("DefaultHTTPConfig returned nil")
	}
}

func TestLoadSwagenConfig(t *testing.T) {
	// Test loading config from non-existent file
	_, err := LoadSwagenConfig("/non/existent/config.yaml")
	if err == nil {
		t.Error("Expected error when loading non-existent config file")
	}
}
