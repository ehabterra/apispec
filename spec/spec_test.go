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

func TestLoadAPISpecConfig(t *testing.T) {
	// Test loading config from non-existent file
	_, err := LoadAPISpecConfig("/non/existent/config.yaml")
	if err == nil {
		t.Error("Expected error when loading non-existent config file")
	}
}
