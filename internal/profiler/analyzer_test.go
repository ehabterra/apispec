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

package profiler

import (
	"testing"
	"time"
)

func TestDefaultThresholds(t *testing.T) {
	thresholds := DefaultThresholds()

	// Check that all expected thresholds are present
	expectedKeys := []string{
		"execution_time", "memory_usage", "memory_growth",
		"goroutine_count", "gc_frequency", "gc_pause",
	}

	for _, key := range expectedKeys {
		if _, exists := thresholds[key]; !exists {
			t.Errorf("Expected threshold key %s not found", key)
		}
	}

	// Check execution_time thresholds
	execTime := thresholds["execution_time"]
	if execTime.Warning != 10*float64(time.Second) {
		t.Errorf("Expected warning threshold 10s, got %f", execTime.Warning)
	}
	if execTime.Critical != 30*float64(time.Second) {
		t.Errorf("Expected critical threshold 30s, got %f", execTime.Critical)
	}
	if execTime.Unit != "ns" {
		t.Errorf("Expected unit 'ns', got %s", execTime.Unit)
	}

	// Check memory_usage thresholds
	memUsage := thresholds["memory_usage"]
	expectedWarning := 200 * 1024 * 1024  // 200 MB
	expectedCritical := 500 * 1024 * 1024 // 500 MB
	if memUsage.Warning != float64(expectedWarning) {
		t.Errorf("Expected warning threshold %d, got %f", expectedWarning, memUsage.Warning)
	}
	if memUsage.Critical != float64(expectedCritical) {
		t.Errorf("Expected critical threshold %d, got %f", expectedCritical, memUsage.Critical)
	}
}

func TestNewPerformanceAnalyzer(t *testing.T) {
	analyzer := NewPerformanceAnalyzer()
	if analyzer == nil {
		t.Fatal("Expected non-nil analyzer")
		return
	}
	if analyzer.thresholds == nil {
		t.Fatal("Expected non-nil thresholds")
		return
	}

	// Check that default thresholds are set
	if len(analyzer.thresholds) == 0 {
		t.Fatal("Expected non-empty thresholds")
	}
}

func TestNewPerformanceAnalyzerWithThresholds(t *testing.T) {
	customThresholds := map[string]ThresholdConfig{
		"custom_metric": {
			Warning:  10.0,
			Critical: 50.0,
			Unit:     "custom",
		},
	}

	analyzer := NewPerformanceAnalyzerWithThresholds(customThresholds)
	if analyzer == nil {
		t.Fatal("Expected non-nil analyzer")
		return
	}
	if analyzer.thresholds == nil {
		t.Fatal("Expected non-nil thresholds")
		return
	}

	// Check that custom thresholds are set
	if len(analyzer.thresholds) != 1 {
		t.Errorf("Expected 1 threshold, got %d", len(analyzer.thresholds))
	}

	custom := analyzer.thresholds["custom_metric"]
	if custom.Warning != 10.0 {
		t.Errorf("Expected warning 10.0, got %f", custom.Warning)
	}
	if custom.Critical != 50.0 {
		t.Errorf("Expected critical 50.0, got %f", custom.Critical)
	}
}

func TestAnalyzeMetrics(t *testing.T) {
	analyzer := NewPerformanceAnalyzer()

	// Test with empty metrics
	report := analyzer.AnalyzeMetrics([]Metric{})
	if report == nil {
		t.Fatal("Expected non-nil report")
		return
	}
	if report.TotalIssues != 0 {
		t.Errorf("Expected 0 issues, got %d", report.TotalIssues)
	}
	if len(report.Issues) != 0 {
		t.Errorf("Expected 0 issues, got %d", len(report.Issues))
	}
	if len(report.Summary) != 0 {
		t.Errorf("Expected empty summary, got %v", report.Summary)
	}
}

func TestAnalyzeMetricsWithTimerMetrics(t *testing.T) {
	analyzer := NewPerformanceAnalyzer()

	// Create timer metrics that exceed thresholds
	metrics := []Metric{
		{
			Name:      "slow_function",
			Type:      MetricTypeTimer,
			Value:     35 * float64(time.Second), // Exceeds critical threshold
			Unit:      "ns",
			Timestamp: time.Now(),
		},
		{
			Name:      "moderate_function",
			Type:      MetricTypeTimer,
			Value:     12 * float64(time.Second), // Exceeds warning threshold (now 10s)
			Unit:      "ns",
			Timestamp: time.Now(),
		},
		{
			Name:      "fast_function",
			Type:      MetricTypeTimer,
			Value:     1 * float64(time.Second), // Within thresholds
			Unit:      "ns",
			Timestamp: time.Now(),
		},
	}

	report := analyzer.AnalyzeMetrics(metrics)
	if report == nil {
		t.Fatal("Expected non-nil report")
		return
	}

	// Should have 2 issues (1 critical, 1 warning)
	if report.TotalIssues != 2 {
		t.Errorf("Expected 2 issues, got %d", report.TotalIssues)
	}

	// Check summary
	if report.Summary["critical"] != 1 {
		t.Errorf("Expected 1 critical issue, got %d", report.Summary["critical"])
	}
	if report.Summary["warning"] != 1 {
		t.Errorf("Expected 1 warning issue, got %d", report.Summary["warning"])
	}

	// Check that issues are sorted by severity (critical first)
	if len(report.Issues) >= 2 {
		if report.Issues[0].Severity != "critical" {
			t.Errorf("Expected first issue to be critical, got %s", report.Issues[0].Severity)
		}
		if report.Issues[1].Severity != "warning" {
			t.Errorf("Expected second issue to be warning, got %s", report.Issues[1].Severity)
		}
	}
}

func TestAnalyzeMetricsWithGaugeMetrics(t *testing.T) {
	analyzer := NewPerformanceAnalyzer()

	// Create gauge metrics for memory usage
	metrics := []Metric{
		{
			Name:      "memory.alloc",
			Type:      MetricTypeGauge,
			Value:     600 * 1024 * 1024, // 600 MB - exceeds critical threshold
			Unit:      "bytes",
			Timestamp: time.Now(),
		},
		{
			Name:      "memory.heap",
			Type:      MetricTypeGauge,
			Value:     250 * 1024 * 1024, // 250 MB - exceeds warning threshold (now 200MB)
			Unit:      "bytes",
			Timestamp: time.Now(),
		},
	}

	report := analyzer.AnalyzeMetrics(metrics)
	if report == nil {
		t.Fatal("Expected non-nil report")
		return
	}

	// Should have 2 issues (1 critical, 1 warning)
	if report.TotalIssues != 2 {
		t.Errorf("Expected 2 issues, got %d", report.TotalIssues)
	}

	// Check that both issues are about memory usage
	for _, issue := range report.Issues {
		if issue.Type != "high_memory_usage" {
			t.Errorf("Expected issue type 'high_memory_usage', got %s", issue.Type)
		}
	}
}

func TestAnalyzeMetricsWithGoroutineMetrics(t *testing.T) {
	analyzer := NewPerformanceAnalyzer()

	// Create gauge metrics for goroutine count
	metrics := []Metric{
		{
			Name:      "goroutines.count",
			Type:      MetricTypeGauge,
			Value:     600, // Exceeds critical threshold
			Unit:      "count",
			Timestamp: time.Now(),
		},
		{
			Name:      "goroutines.active",
			Type:      MetricTypeGauge,
			Value:     150, // Exceeds warning threshold
			Unit:      "count",
			Timestamp: time.Now(),
		},
	}

	report := analyzer.AnalyzeMetrics(metrics)
	if report == nil {
		t.Fatal("Expected non-nil report")
		return
	}

	// Should have 2 issues (1 critical, 1 warning)
	if report.TotalIssues != 2 {
		t.Errorf("Expected 2 issues, got %d", report.TotalIssues)
	}

	// Check that both issues are about goroutine count
	for _, issue := range report.Issues {
		if issue.Type != "high_goroutine_count" {
			t.Errorf("Expected issue type 'high_goroutine_count', got %s", issue.Type)
		}
	}
}

func TestAnalyzeMetricsWithCounterMetrics(t *testing.T) {
	analyzer := NewPerformanceAnalyzer()

	// Create counter metrics
	metrics := []Metric{
		{
			Name:      "request_count",
			Type:      MetricTypeCounter,
			Value:     100,
			Unit:      "count",
			Timestamp: time.Now(),
		},
	}

	report := analyzer.AnalyzeMetrics(metrics)
	if report == nil {
		t.Fatal("Expected non-nil report")
		return
	}

	// Counter metrics don't generate issues currently
	if report.TotalIssues != 0 {
		t.Errorf("Expected 0 issues for counter metrics, got %d", report.TotalIssues)
	}
}

func TestAnalyzeMetricsWithHistogramMetrics(t *testing.T) {
	analyzer := NewPerformanceAnalyzer()

	// Create histogram metrics
	metrics := []Metric{
		{
			Name:      "response_time_histogram",
			Type:      MetricTypeHistogram,
			Value:     100,
			Unit:      "ms",
			Timestamp: time.Now(),
		},
	}

	report := analyzer.AnalyzeMetrics(metrics)
	if report == nil {
		t.Fatal("Expected non-nil report")
		return
	}

	// Histogram metrics don't generate issues currently
	if report.TotalIssues != 0 {
		t.Errorf("Expected 0 issues for histogram metrics, got %d", report.TotalIssues)
	}
}

func TestIsMemoryMetric(t *testing.T) {
	analyzer := NewPerformanceAnalyzer()

	testCases := []struct {
		name     string
		expected bool
	}{
		{"memory.alloc", true},
		{"heap.alloc", true},
		{"sys.memory", true},
		{"alloc_bytes", true},
		{"cpu.usage", false},
		{"goroutines.count", false},
		{"memory", true},
		{"alloc", true},
		{"heap", true},
		{"sys", true},
	}

	for _, tc := range testCases {
		result := analyzer.isMemoryMetric(tc.name)
		if result != tc.expected {
			t.Errorf("isMemoryMetric(%s) = %v, expected %v", tc.name, result, tc.expected)
		}
	}
}

func TestIsGoroutineMetric(t *testing.T) {
	analyzer := NewPerformanceAnalyzer()

	testCases := []struct {
		name     string
		expected bool
	}{
		{"goroutines.count", true},
		{"goroutine.active", true},
		{"goroutines", true},
		{"goroutine", true},
		{"memory.alloc", false},
		{"cpu.usage", false},
	}

	for _, tc := range testCases {
		result := analyzer.isGoroutineMetric(tc.name)
		if result != tc.expected {
			t.Errorf("isGoroutineMetric(%s) = %v, expected %v", tc.name, result, tc.expected)
		}
	}
}

func TestCalculateStats(t *testing.T) {
	analyzer := NewPerformanceAnalyzer()

	// Test with empty slice
	avg, max, min := analyzer.calculateStats([]float64{})
	if avg != 0 || max != 0 || min != 0 {
		t.Errorf("Expected (0,0,0) for empty slice, got (%f,%f,%f)", avg, max, min)
	}

	// Test with single value
	avg, max, min = analyzer.calculateStats([]float64{5.0})
	if avg != 5.0 || max != 5.0 || min != 5.0 {
		t.Errorf("Expected (5.0,5.0,5.0) for single value, got (%f,%f,%f)", avg, max, min)
	}

	// Test with multiple values
	values := []float64{1.0, 5.0, 3.0, 9.0, 2.0}
	avg, max, min = analyzer.calculateStats(values)
	expectedAvg := 4.0
	expectedMax := 9.0
	expectedMin := 1.0

	if avg != expectedAvg {
		t.Errorf("Expected avg %f, got %f", expectedAvg, avg)
	}
	if max != expectedMax {
		t.Errorf("Expected max %f, got %f", expectedMax, max)
	}
	if min != expectedMin {
		t.Errorf("Expected min %f, got %f", expectedMin, min)
	}
}

func TestGenerateRecommendations(t *testing.T) {
	analyzer := NewPerformanceAnalyzer()

	// Test with empty issues
	recommendations := analyzer.generateRecommendations([]PerformanceIssue{})
	if len(recommendations) == 0 {
		t.Error("Expected some general recommendations even with no issues")
	}

	// Test with issues
	issues := []PerformanceIssue{
		{
			Type:     "slow_execution",
			Severity: "critical",
			Suggestions: []string{
				"Optimize the function implementation",
				"Consider caching or memoization",
			},
		},
		{
			Type:     "high_memory_usage",
			Severity: "warning",
			Suggestions: []string{
				"Check for memory leaks",
				"Optimize data structures",
			},
		},
	}

	recommendations = analyzer.generateRecommendations(issues)
	if len(recommendations) == 0 {
		t.Error("Expected recommendations from issues")
	}

	// Check that duplicate suggestions are removed
	seen := make(map[string]bool)
	for _, rec := range recommendations {
		if seen[rec] {
			t.Errorf("Duplicate recommendation found: %s", rec)
		}
		seen[rec] = true
	}
}

func TestContains(t *testing.T) {
	testCases := []struct {
		s        string
		substr   string
		expected bool
	}{
		{"hello", "hello", true},
		{"hello", "hell", true},
		{"hello", "ello", true},
		{"hello", "lo", true},
		{"hello", "h", true},
		{"hello", "o", true},
		{"hello", "world", false},
		{"hello", "", true},
		{"", "hello", false},
		{"", "", true},
		{"test", "test", true},
		{"testing", "test", true},
		{"testing", "ing", true},
		{"testing", "tin", true},
	}

	for _, tc := range testCases {
		result := contains(tc.s, tc.substr)
		if result != tc.expected {
			t.Errorf("contains(%q, %q) = %v, expected %v", tc.s, tc.substr, result, tc.expected)
		}
	}
}

func TestContainsSubstring(t *testing.T) {
	testCases := []struct {
		s        string
		substr   string
		expected bool
	}{
		{"hello", "hello", true},
		{"hello", "hell", true},
		{"hello", "ello", true},
		{"hello", "lo", true},
		{"hello", "h", true},
		{"hello", "o", true},
		{"hello", "world", false},
		{"hello", "", true},
		{"", "hello", false},
		{"", "", true},
		{"test", "test", true},
		{"testing", "test", true},
		{"testing", "ing", true},
		{"testing", "tin", true},
		{"abc", "b", true},
		{"abc", "ac", false},
	}

	for _, tc := range testCases {
		result := containsSubstring(tc.s, tc.substr)
		if result != tc.expected {
			t.Errorf("containsSubstring(%q, %q) = %v, expected %v", tc.s, tc.substr, result, tc.expected)
		}
	}
}
