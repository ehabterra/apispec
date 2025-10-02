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

package patterns

import (
	"regexp"
	"testing"
)

func TestPatternToRegexComprehensive(t *testing.T) {
	testCases := []struct {
		pattern     string
		expected    string
		description string
	}{
		{
			pattern:     "*.go",
			expected:    `^(?:.*/)?[^/]*\.go$`,
			description: "Simple wildcard",
		},
		{
			pattern:     "**/*.go",
			expected:    `^(?:.*?/)?[^/]*\.go$`,
			description: "Double star wildcard",
		},
		{
			pattern:     "src/*.go",
			expected:    `^src/[^/]*\.go$`,
			description: "Directory with wildcard",
		},
		{
			pattern:     "test[0-9].go",
			expected:    `^(?:.*/)?test[0-9]\.go$`,
			description: "Character class",
		},
		{
			pattern:     "file?.txt",
			expected:    `^(?:.*/)?file[^/]\.txt$`,
			description: "Question mark wildcard",
		},
		{
			pattern:     "src/",
			expected:    `^(?:.*/)?src(?:|/.*)$`,
			description: "Directory with trailing slash",
		},
		{
			pattern:     "*.tmp",
			expected:    `^(?:.*/)?[^/]*\.tmp$`,
			description: "Temporary files",
		},
		{
			pattern:     "**/test/**",
			expected:    `^(?:.*?/)?test(?:/.*)?$`,
			description: "Nested directory pattern",
		},
		{
			pattern:     "github.com/org/**",
			expected:    `^github\.com/org(?:/.*)?$`,
			description: "Package path pattern",
		},
		{
			pattern:     "!*.tmp",
			expected:    `^(?:.*/)?![^/]*\.tmp$`,
			description: "Negation pattern (note: negation is handled separately)",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			result := patternToRegex(tc.pattern)
			if result != tc.expected {
				t.Errorf("Expected regex %q, got %q", tc.expected, result)
			}
		})
	}
}

func TestPatternToRegexEdgeCasesComprehensive(t *testing.T) {
	testCases := []struct {
		pattern     string
		description string
	}{
		{
			pattern:     "",
			description: "Empty pattern",
		},
		{
			pattern:     "a",
			description: "Single character",
		},
		{
			pattern:     "a/b/c",
			description: "Simple path",
		},
		{
			pattern:     "**",
			description: "Double star only",
		},
		{
			pattern:     "*",
			description: "Single star only",
		},
		{
			pattern:     "?",
			description: "Question mark only",
		},
		{
			pattern:     "[a-z]",
			description: "Character class only",
		},
		{
			pattern:     "a[",
			description: "Incomplete character class",
		},
		{
			pattern:     "a]",
			description: "Closing bracket without opening",
		},
		{
			pattern:     "a[bc",
			description: "Unclosed character class",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			// Test that patternToRegex doesn't panic
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("patternToRegex panicked for pattern %q: %v", tc.pattern, r)
				}
			}()

			result := patternToRegex(tc.pattern)
			if result == "" {
				t.Errorf("Expected non-empty regex for pattern %q", tc.pattern)
			}
		})
	}
}

func TestMatchGitignorePattern(t *testing.T) {
	testCases := []struct {
		pattern     string
		path        string
		expected    bool
		description string
	}{
		// Basic patterns
		{"*.go", "main.go", true, "Simple wildcard match"},
		{"*.go", "main.txt", false, "Simple wildcard no match"},
		{"*.go", "src/main.go", true, "Wildcard matches subdirectory"},
		{"**/*.go", "src/main.go", true, "Double star matches subdirectory"},
		{"**/*.go", "a/b/c/main.go", true, "Double star matches nested path"},
		{"src/*.go", "src/main.go", true, "Directory wildcard match"},
		{"src/*.go", "src/pkg/main.go", false, "Directory wildcard no match for nested"},
		{"src/*.go", "main.go", false, "Directory wildcard no match for root"},

		// Character classes
		{"test[0-9].go", "test1.go", true, "Character class match"},
		{"test[0-9].go", "testa.go", false, "Character class no match"},
		{"test[0-9].go", "test10.go", false, "Character class no match for double digit"},

		// Question mark
		{"file?.txt", "file1.txt", true, "Question mark match"},
		{"file?.txt", "file.txt", false, "Question mark no match for no character"},
		{"file?.txt", "file12.txt", false, "Question mark no match for multiple characters"},

		// Trailing slash
		{"src/", "src/main.go", true, "Trailing slash matches directory"},
		{"src/", "src", true, "Trailing slash matches exact name"},
		{"src/", "srcpkg/main.go", false, "Trailing slash no match for prefix"},

		// Negation (the ! is stripped before calling matchGitignorePattern)
		{"*.tmp", "file.tmp", true, "Pattern after negation stripping"},
		{"*.tmp", "file.go", false, "Pattern after negation stripping no match"},

		// Empty patterns
		{"", "main.go", false, "Empty pattern matches nothing"},
		{"*.go", "", false, "Empty path with pattern"},
		{"", "", false, "Both empty"},

		// Complex patterns
		{"github.com/org/**", "github.com/org/repo/main.go", true, "Package path match"},
		{"github.com/org/**", "github.com/other/repo/main.go", false, "Package path no match"},
		{"**/internal/**", "src/internal/service.go", true, "Nested directory match"},
		{"**/internal/**", "src/external/service.go", false, "Nested directory no match"},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			result := matchGitignorePattern(tc.pattern, tc.path)
			if result != tc.expected {
				t.Errorf("Pattern %q against path %q: expected %v, got %v",
					tc.pattern, tc.path, tc.expected, result)
			}
		})
	}
}

func TestMatchGitignorePatternEdgeCases(t *testing.T) {
	// Test with special regex characters
	testCases := []struct {
		pattern     string
		path        string
		description string
	}{
		{"file.txt", "file.txt", "Exact match"},
		{"file.txt", "file.tx", "Partial match"},
		{"file.txt", "file.txxt", "Over match"},
		{"a.b", "a.b", "Dot in pattern"},
		{"a+b", "a+b", "Plus in pattern"},
		{"a*b", "a*b", "Asterisk in pattern"},
		{"a?b", "a?b", "Question in pattern"},
		{"a^b", "a^b", "Caret in pattern"},
		{"a$b", "a$b", "Dollar in pattern"},
		{"a|b", "a|b", "Pipe in pattern"},
		{"a(b", "a(b", "Parenthesis in pattern"},
		{"a)b", "a)b", "Closing parenthesis in pattern"},
		{"a{b", "a{b", "Brace in pattern"},
		{"a}b", "a}b", "Closing brace in pattern"},
		{"a[b", "a[b", "Bracket in pattern"},
		{"a]b", "a]b", "Closing bracket in pattern"},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			// Test that matchGitignorePattern doesn't panic
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("matchGitignorePattern panicked for pattern %q, path %q: %v",
						tc.pattern, tc.path, r)
				}
			}()

			result := matchGitignorePattern(tc.pattern, tc.path)
			// We don't care about the result, just that it doesn't panic
			_ = result
		})
	}
}

func TestMatchGitignorePatternRegexErrors(t *testing.T) {
	// Test patterns that might cause regex compilation errors
	problematicPatterns := []string{
		"[",    // Unclosed character class
		"]",    // Closing bracket without opening
		"[a-",  // Incomplete character class
		"[a-z", // Unclosed character class
		"\\",   // Backslash (might cause issues)
		"(",    // Unclosed group
		")",    // Closing parenthesis without opening
		"{",    // Unclosed brace
		"}",    // Closing brace without opening
	}

	for _, pattern := range problematicPatterns {
		t.Run("pattern_"+pattern, func(t *testing.T) {
			// Test that matchGitignorePattern handles regex errors gracefully
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("matchGitignorePattern panicked for problematic pattern %q: %v", pattern, r)
				}
			}()

			result := matchGitignorePattern(pattern, "test")
			// Should fall back to simple string matching
			expected := pattern == "test"
			if result != expected {
				t.Errorf("Expected fallback behavior for pattern %q: expected %v, got %v",
					pattern, expected, result)
			}
		})
	}
}

func TestMatchGitignorePatternNormalization(t *testing.T) {
	// Test path normalization
	testCases := []struct {
		pattern     string
		path        string
		expected    bool
		description string
	}{
		{"/src/*.go", "src/main.go", true, "Leading slash in pattern"},
		{"src/*.go", "/src/main.go", true, "Leading slash in path"},
		{"/src/*.go", "/src/main.go", true, "Both with leading slashes"},
		{"src/*.go", "src/main.go", true, "Neither with leading slashes"},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			result := matchGitignorePattern(tc.pattern, tc.path)
			if result != tc.expected {
				t.Errorf("Pattern %q against path %q: expected %v, got %v",
					tc.pattern, tc.path, tc.expected, result)
			}
		})
	}
}

func TestMatchGitignorePatternNegation(t *testing.T) {
	// Test negation patterns
	testCases := []struct {
		pattern     string
		path        string
		expected    bool
		description string
	}{
		{"*.tmp", "file.tmp", true, "Pattern after negation stripping"},
		{"*.tmp", "file.go", false, "Pattern after negation stripping no match"},
		{"src/", "src/main.go", true, "Directory pattern after negation stripping"},
		{"src/", "main.go", false, "Directory pattern after negation stripping no match"},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			result := matchGitignorePattern(tc.pattern, tc.path)
			if result != tc.expected {
				t.Errorf("Pattern %q against path %q: expected %v, got %v",
					tc.pattern, tc.path, tc.expected, result)
			}
		})
	}
}

func TestRegexCompilation(t *testing.T) {
	// Test that generated regexes can be compiled
	testPatterns := []string{
		"*.go",
		"**/*.go",
		"src/*.go",
		"test[0-9].go",
		"file?.txt",
		"src/",
		"*.tmp",
		"**/test/**",
		"github.com/org/**",
		"!*.tmp",
	}

	for _, pattern := range testPatterns {
		t.Run("regex_"+pattern, func(t *testing.T) {
			regexStr := patternToRegex(pattern)

			// Test that the regex can be compiled
			_, err := regexp.Compile(regexStr)
			if err != nil {
				t.Errorf("Failed to compile regex %q for pattern %q: %v", regexStr, pattern, err)
			}
		})
	}
}
