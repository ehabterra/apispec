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

package metadata

import (
	"testing"
)

func TestFrameworkDetector_Configure(t *testing.T) {
	fd := NewFrameworkDetector()

	// Test default values
	if fd.config.IncludeExternalPackages != false {
		t.Error("Expected default IncludeExternalPackages to be false")
	}
	// MaxImportDepth has a default value in the configuration

	// Test configuration
	fd.Configure(true, 5)

	if fd.config.IncludeExternalPackages != true {
		t.Error("Expected IncludeExternalPackages to be true after Configure")
	}
	if fd.config.MaxImportDepth != 5 {
		t.Error("Expected MaxImportDepth to be 5 after Configure")
	}
}

func TestFrameworkDetector_DisableFramework(t *testing.T) {
	fd := NewFrameworkDetector()

	// Test disabling a framework
	fd.DisableFramework("http")

	if fd.config.DisabledFrameworks == nil {
		t.Error("Expected DisabledFrameworks map to be initialized")
	}

	if !fd.config.DisabledFrameworks["http"] {
		t.Error("Expected http framework to be disabled")
	}

	// Test disabling multiple frameworks
	fd.DisableFramework("chi")

	if !fd.config.DisabledFrameworks["chi"] {
		t.Error("Expected chi framework to be disabled")
	}
}

func TestFrameworkDetector_AddFrameworkPattern(t *testing.T) {
	fd := NewFrameworkDetector()

	// Test adding a framework pattern
	fd.AddFrameworkPattern("gin", []string{"github.com/gin-gonic/gin"})

	if fd.config.FrameworkPatterns == nil {
		t.Error("Expected FrameworkPatterns map to be initialized")
	}

	patterns, exists := fd.config.FrameworkPatterns["gin"]
	if !exists {
		t.Error("Expected gin framework patterns to exist")
	}

	if len(patterns) != 1 || patterns[0] != "github.com/gin-gonic/gin" {
		t.Errorf("Expected gin pattern to be 'github.com/gin-gonic/gin', got %v", patterns)
	}

	// Test adding multiple patterns for the same framework
	fd.AddFrameworkPattern("gin", []string{"github.com/gin-gonic/gin/v2"})

	patterns = fd.config.FrameworkPatterns["gin"]
	if len(patterns) != 1 || patterns[0] != "github.com/gin-gonic/gin/v2" {
		t.Errorf("Expected gin pattern to be 'github.com/gin-gonic/gin/v2', got %v", patterns)
	}
}

func TestFrameworkDetector_AddExternalPrefix(t *testing.T) {
	fd := NewFrameworkDetector()

	// Get initial count
	initialCount := len(fd.config.ExternalPrefixes)

	// Test adding external prefixes
	fd.AddExternalPrefix("github.com/")
	fd.AddExternalPrefix("golang.org/")

	if fd.config.ExternalPrefixes == nil {
		t.Error("Expected ExternalPrefixes slice to be initialized")
	}

	expectedCount := initialCount + 2
	if len(fd.config.ExternalPrefixes) != expectedCount {
		t.Errorf("Expected %d external prefixes, got %d", expectedCount, len(fd.config.ExternalPrefixes))
	}

	// Check that the new prefixes were added
	lastTwo := fd.config.ExternalPrefixes[len(fd.config.ExternalPrefixes)-2:]
	expected := []string{"github.com/", "golang.org/"}
	for i, prefix := range expected {
		if lastTwo[i] != prefix {
			t.Errorf("Expected prefix %d to be %q, got %q", i, prefix, lastTwo[i])
		}
	}
}

func TestFrameworkDetector_AddProjectPattern(t *testing.T) {
	fd := NewFrameworkDetector()

	// Get initial count
	initialCount := len(fd.config.ProjectPatterns)

	// Test adding project patterns
	fd.AddProjectPattern("myproject/")
	fd.AddProjectPattern("internal/")

	if fd.config.ProjectPatterns == nil {
		t.Error("Expected ProjectPatterns slice to be initialized")
	}

	expectedCount := initialCount + 2
	if len(fd.config.ProjectPatterns) != expectedCount {
		t.Errorf("Expected %d project patterns, got %d", expectedCount, len(fd.config.ProjectPatterns))
	}

	// Check that the new patterns were added
	lastTwo := fd.config.ProjectPatterns[len(fd.config.ProjectPatterns)-2:]
	expected := []string{"myproject/", "internal/"}
	for i, pattern := range expected {
		if lastTwo[i] != pattern {
			t.Errorf("Expected pattern %d to be %q, got %q", i, pattern, lastTwo[i])
		}
	}
}

func TestFrameworkDetector_AddTestMockPattern(t *testing.T) {
	fd := NewFrameworkDetector()

	// Get initial count
	initialCount := len(fd.config.TestMockPatterns)

	// Test adding test/mock patterns
	fd.AddTestMockPattern("test")
	fd.AddTestMockPattern("mock")
	fd.AddTestMockPattern("fake")

	if fd.config.TestMockPatterns == nil {
		t.Error("Expected TestMockPatterns slice to be initialized")
	}

	expectedCount := initialCount + 3
	if len(fd.config.TestMockPatterns) != expectedCount {
		t.Errorf("Expected %d test/mock patterns, got %d", expectedCount, len(fd.config.TestMockPatterns))
	}

	// Check that the new patterns were added
	lastThree := fd.config.TestMockPatterns[len(fd.config.TestMockPatterns)-3:]
	expected := []string{"test", "mock", "fake"}
	for i, pattern := range expected {
		if lastThree[i] != pattern {
			t.Errorf("Expected pattern %d to be %q, got %q", i, pattern, lastThree[i])
		}
	}
}

func TestFrameworkDetector_GetConfig(t *testing.T) {
	fd := NewFrameworkDetector()

	config := fd.GetConfig()

	// Test that it returns a valid config
	if config.IncludeExternalPackages != false {
		t.Error("Expected default IncludeExternalPackages to be false")
	}
	// MaxImportDepth has a default value in the configuration, so we just check it's set
	if config.MaxImportDepth < 0 {
		t.Error("Expected MaxImportDepth to be non-negative")
	}
}

func TestNewFrameworkDetector(t *testing.T) {
	fd := NewFrameworkDetector()

	if fd == nil {
		t.Error("Expected non-nil FrameworkDetector")
		return
	}

	if fd.packages == nil {
		t.Error("Expected packages map to be initialized")
		return
	}

	if fd.dependencyGraph == nil {
		t.Error("Expected dependencyGraph map to be initialized")
		return
	}

	if fd.reverseDependencyGraph == nil {
		t.Error("Expected reverseDependencyGraph map to be initialized")
		return
	}
}

func TestNewFrameworkDetectorWithConfig(t *testing.T) {
	config := FrameworkDetectorConfig{
		IncludeExternalPackages: true,
		MaxImportDepth:          5,
	}

	fd := NewFrameworkDetectorWithConfig(config)

	if fd == nil {
		t.Error("Expected non-nil FrameworkDetector")
		return
	}

	if fd.config.IncludeExternalPackages != true {
		t.Error("Expected IncludeExternalPackages to be true")
	}

	if fd.config.MaxImportDepth != 5 {
		t.Error("Expected MaxImportDepth to be 5")
	}
}

func TestDefaultFrameworkDetectorConfig(t *testing.T) {
	config := DefaultFrameworkDetectorConfig()

	if config.IncludeExternalPackages != false {
		t.Error("Expected default IncludeExternalPackages to be false")
	}

	// MaxImportDepth has a default value in the configuration
	if config.MaxImportDepth < 0 {
		t.Error("Expected default MaxImportDepth to be non-negative")
	}

	if config.FrameworkPatterns == nil {
		t.Error("Expected FrameworkPatterns to be initialized")
	}

	if config.ExternalPrefixes == nil {
		t.Error("Expected ExternalPrefixes to be initialized")
	}

	if config.ProjectPatterns == nil {
		t.Error("Expected ProjectPatterns to be initialized")
	}

	if config.TestMockPatterns == nil {
		t.Error("Expected TestMockPatterns to be initialized")
	}

	if config.DisabledFrameworks == nil {
		t.Error("Expected DisabledFrameworks to be initialized")
	}
}
