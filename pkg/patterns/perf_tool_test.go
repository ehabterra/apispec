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
	"strings"
	"testing"
	"time"
)

func TestPerfResultString(t *testing.T) {
	result := PerfResult{
		Pattern:     "*.go",
		Path:        "main.go",
		Iterations:  1000,
		TotalTime:   100 * time.Microsecond,
		AvgTime:     100 * time.Nanosecond,
		AllocsStart: 1000,
		AllocsEnd:   2000,
		TotalAllocs: 1000,
	}

	str := result.String()
	if !strings.Contains(str, "*.go") {
		t.Error("Expected string to contain pattern")
	}
	if !strings.Contains(str, "main.go") {
		t.Error("Expected string to contain path")
	}
	if !strings.Contains(str, "1000") {
		t.Error("Expected string to contain allocations")
	}
}

func TestBenchmarkPattern(t *testing.T) {
	// Test with default iterations
	result := BenchmarkPattern("*.go", "main.go", 0)
	if result.Pattern != "*.go" {
		t.Errorf("Expected pattern '*.go', got %s", result.Pattern)
	}
	if result.Path != "main.go" {
		t.Errorf("Expected path 'main.go', got %s", result.Path)
	}
	if result.Iterations != 10000 {
		t.Errorf("Expected 10000 iterations, got %d", result.Iterations)
	}
	if result.TotalTime <= 0 {
		t.Error("Expected positive total time")
	}
	if result.AvgTime <= 0 {
		t.Error("Expected positive average time")
	}

	// Test with custom iterations
	result = BenchmarkPattern("*.txt", "readme.txt", 1000)
	if result.Iterations != 1000 {
		t.Errorf("Expected 1000 iterations, got %d", result.Iterations)
	}
	if result.Pattern != "*.txt" {
		t.Errorf("Expected pattern '*.txt', got %s", result.Pattern)
	}
	if result.Path != "readme.txt" {
		t.Errorf("Expected path 'readme.txt', got %s", result.Path)
	}
}

func TestBenchmarkPatterns(t *testing.T) {
	testCases := []struct {
		Pattern, Path string
	}{
		{"*.go", "main.go"},
		{"*.txt", "readme.txt"},
		{"src/*.go", "src/main.go"},
	}

	results := BenchmarkPatterns(testCases, 100)
	if len(results) != len(testCases) {
		t.Fatalf("Expected %d results, got %d", len(testCases), len(results))
	}

	for i, result := range results {
		if result.Pattern != testCases[i].Pattern {
			t.Errorf("Expected pattern %s, got %s", testCases[i].Pattern, result.Pattern)
		}
		if result.Path != testCases[i].Path {
			t.Errorf("Expected path %s, got %s", testCases[i].Path, result.Path)
		}
		if result.Iterations != 100 {
			t.Errorf("Expected 100 iterations, got %d", result.Iterations)
		}
	}
}

func TestQuickBench(t *testing.T) {
	// Capture output to test that QuickBench runs without panicking
	// We can't easily test the output format, but we can ensure it doesn't crash
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("QuickBench panicked: %v", r)
		}
	}()

	// This should not panic
	QuickBench()
}

func TestComparePatterns(t *testing.T) {
	patterns := []string{
		"*.go",
		"**/*.go",
		"src/*.go",
	}

	// Capture output to test that ComparePatterns runs without panicking
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("ComparePatterns panicked: %v", r)
		}
	}()

	// This should not panic
	ComparePatterns(patterns, "src/main.go", 100)
}

func TestProfilePattern(t *testing.T) {
	// Capture output to test that ProfilePattern runs without panicking
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("ProfilePattern panicked: %v", r)
		}
	}()

	// This should not panic
	ProfilePattern("*.go", "main.go", 100)
}

func TestPerfResultEdgeCases(t *testing.T) {
	// Test with zero values
	result := PerfResult{}
	str := result.String()
	if str == "" {
		t.Error("Expected non-empty string representation")
	}

	// Test with very small values
	result = PerfResult{
		Pattern:     "a",
		Path:        "b",
		Iterations:  1,
		TotalTime:   1 * time.Nanosecond,
		AvgTime:     1 * time.Nanosecond,
		AllocsStart: 0,
		AllocsEnd:   0,
		TotalAllocs: 0,
	}

	str = result.String()
	if !strings.Contains(str, "a") {
		t.Error("Expected string to contain pattern")
	}
	if !strings.Contains(str, "b") {
		t.Error("Expected string to contain path")
	}
}

func TestBenchmarkPatternEdgeCases(t *testing.T) {
	// Test with negative iterations (should default to 10000)
	result := BenchmarkPattern("*.go", "main.go", -1)
	if result.Iterations != 10000 {
		t.Errorf("Expected 10000 iterations for negative input, got %d", result.Iterations)
	}

	// Test with very small iterations
	result = BenchmarkPattern("*.go", "main.go", 1)
	if result.Iterations != 1 {
		t.Errorf("Expected 1 iteration, got %d", result.Iterations)
	}

	// Test with empty pattern
	result = BenchmarkPattern("", "main.go", 100)
	if result.Pattern != "" {
		t.Errorf("Expected empty pattern, got %s", result.Pattern)
	}

	// Test with empty path
	result = BenchmarkPattern("*.go", "", 100)
	if result.Path != "" {
		t.Errorf("Expected empty path, got %s", result.Path)
	}
}

func TestBenchmarkPatternsEmpty(t *testing.T) {
	// Test with empty test cases
	results := BenchmarkPatterns([]struct{ Pattern, Path string }{}, 100)
	if len(results) != 0 {
		t.Errorf("Expected 0 results for empty test cases, got %d", len(results))
	}
}

func TestComparePatternsEmpty(t *testing.T) {
	// Test with empty patterns - this should not panic
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("ComparePatterns with empty patterns panicked: %v", r)
		}
	}()

	// ComparePatterns should handle empty patterns gracefully
	ComparePatterns([]string{}, "main.go", 100)
}

func TestProfilePatternEdgeCases(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("ProfilePattern with edge cases panicked: %v", r)
		}
	}()

	// Test with empty pattern
	ProfilePattern("", "main.go", 100)

	// Test with empty path
	ProfilePattern("*.go", "", 100)

	// Test with zero iterations
	ProfilePattern("*.go", "main.go", 0)
}
