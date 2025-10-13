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
	"fmt"
	"sort"
	"time"
)

// PerformanceIssue represents a detected performance issue
type PerformanceIssue struct {
	Type        string                 `json:"type"`
	Severity    string                 `json:"severity"`
	Description string                 `json:"description"`
	Location    string                 `json:"location,omitempty"`
	Value       float64                `json:"value"`
	Unit        string                 `json:"unit"`
	Threshold   float64                `json:"threshold"`
	Suggestions []string               `json:"suggestions"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// PerformanceReport contains the analysis results
type PerformanceReport struct {
	Timestamp       time.Time          `json:"timestamp"`
	TotalIssues     int                `json:"total_issues"`
	Issues          []PerformanceIssue `json:"issues"`
	Summary         map[string]int     `json:"summary"`
	Recommendations []string           `json:"recommendations"`
}

// PerformanceAnalyzer analyzes performance metrics and identifies issues
type PerformanceAnalyzer struct {
	thresholds map[string]ThresholdConfig
}

// ThresholdConfig defines thresholds for different metrics
type ThresholdConfig struct {
	Warning  float64 `json:"warning"`
	Critical float64 `json:"critical"`
	Unit     string  `json:"unit"`
}

// DefaultThresholds returns default threshold configurations
func DefaultThresholds() map[string]ThresholdConfig {
	return map[string]ThresholdConfig{
		"execution_time": {
			Warning:  10 * float64(time.Second), // 10 seconds in nanoseconds (adjusted for large projects)
			Critical: 30 * float64(time.Second), // 30 seconds in nanoseconds
			Unit:     "ns",
		},
		"memory_usage": {
			Warning:  200 * 1024 * 1024, // 200 MB (adjusted for large projects)
			Critical: 500 * 1024 * 1024, // 500 MB
			Unit:     "bytes",
		},
		"memory_growth": {
			Warning:  100 * 1024 * 1024, // 100 MB (adjusted for large projects)
			Critical: 500 * 1024 * 1024, // 500 MB (adjusted for large projects)
			Unit:     "bytes",
		},
		"goroutine_count": {
			Warning:  100,
			Critical: 500,
			Unit:     "count",
		},
		"gc_frequency": {
			Warning:  10, // 10 GCs per second
			Critical: 50, // 50 GCs per second
			Unit:     "per_second",
		},
		"gc_pause": {
			Warning:  5 * 1000 * 1000,  // 5ms (adjusted for large projects)
			Critical: 20 * 1000 * 1000, // 20ms (adjusted for large projects)
			Unit:     "ns",
		},
	}
}

// NewPerformanceAnalyzer creates a new performance analyzer
func NewPerformanceAnalyzer() *PerformanceAnalyzer {
	return &PerformanceAnalyzer{
		thresholds: DefaultThresholds(),
	}
}

// NewPerformanceAnalyzerWithThresholds creates a new analyzer with custom thresholds
func NewPerformanceAnalyzerWithThresholds(thresholds map[string]ThresholdConfig) *PerformanceAnalyzer {
	return &PerformanceAnalyzer{
		thresholds: thresholds,
	}
}

// AnalyzeMetrics analyzes collected metrics and identifies performance issues
func (pa *PerformanceAnalyzer) AnalyzeMetrics(metrics []Metric) *PerformanceReport {
	report := &PerformanceReport{
		Timestamp: time.Now(),
		Issues:    make([]PerformanceIssue, 0),
		Summary:   make(map[string]int),
	}

	// Group metrics by name for analysis
	metricGroups := pa.groupMetricsByName(metrics)

	// Analyze each metric group
	for name, group := range metricGroups {
		issues := pa.analyzeMetricGroup(name, group)
		report.Issues = append(report.Issues, issues...)
	}

	// Count issues by severity
	for _, issue := range report.Issues {
		report.Summary[issue.Severity]++
	}
	report.TotalIssues = len(report.Issues)

	// Generate recommendations
	report.Recommendations = pa.generateRecommendations(report.Issues)

	// Sort issues by severity (critical first)
	sort.Slice(report.Issues, func(i, j int) bool {
		severityOrder := map[string]int{"critical": 0, "warning": 1, "info": 2}
		return severityOrder[report.Issues[i].Severity] < severityOrder[report.Issues[j].Severity]
	})

	return report
}

// groupMetricsByName groups metrics by their name
func (pa *PerformanceAnalyzer) groupMetricsByName(metrics []Metric) map[string][]Metric {
	groups := make(map[string][]Metric)
	for _, metric := range metrics {
		groups[metric.Name] = append(groups[metric.Name], metric)
	}
	return groups
}

// analyzeMetricGroup analyzes a group of metrics with the same name
func (pa *PerformanceAnalyzer) analyzeMetricGroup(name string, metrics []Metric) []PerformanceIssue {
	issues := make([]PerformanceIssue, 0)

	// Analyze based on metric type
	switch metrics[0].Type {
	case MetricTypeTimer:
		issues = append(issues, pa.analyzeTimerMetrics(name, metrics)...)
	case MetricTypeGauge:
		issues = append(issues, pa.analyzeGaugeMetrics(name, metrics)...)
	case MetricTypeCounter:
		issues = append(issues, pa.analyzeCounterMetrics(name, metrics)...)
	case MetricTypeHistogram:
		issues = append(issues, pa.analyzeHistogramMetrics(name, metrics)...)
	}

	return issues
}

// analyzeTimerMetrics analyzes timer metrics for performance issues
func (pa *PerformanceAnalyzer) analyzeTimerMetrics(name string, metrics []Metric) []PerformanceIssue {
	issues := make([]PerformanceIssue, 0)

	// Calculate statistics
	values := make([]float64, len(metrics))
	for i, metric := range metrics {
		values[i] = metric.Value
	}

	avg, max, min := pa.calculateStats(values)

	// Check against thresholds
	if threshold, exists := pa.thresholds["execution_time"]; exists {
		if max > threshold.Critical {
			issues = append(issues, PerformanceIssue{
				Type:        "slow_execution",
				Severity:    "critical",
				Description: fmt.Sprintf("Function %s took %f ms (max), exceeding critical threshold of %f ms", name, max/1000000, threshold.Critical/1000000),
				Value:       max,
				Unit:        "ns",
				Threshold:   threshold.Critical,
				Suggestions: []string{
					"Optimize the function implementation",
					"Consider caching or memoization",
					"Profile the function to identify bottlenecks",
					"Consider breaking down into smaller functions",
				},
				Metadata: map[string]interface{}{
					"average": avg,
					"min":     min,
					"max":     max,
					"count":   len(metrics),
				},
			})
		} else if avg > threshold.Warning {
			issues = append(issues, PerformanceIssue{
				Type:        "slow_execution",
				Severity:    "warning",
				Description: fmt.Sprintf("Function %s averaged %f ms, exceeding warning threshold of %f ms", name, avg/1000000, threshold.Warning/1000000),
				Value:       avg,
				Unit:        "ns",
				Threshold:   threshold.Warning,
				Suggestions: []string{
					"Monitor the function performance",
					"Consider optimization if performance degrades further",
				},
				Metadata: map[string]interface{}{
					"average": avg,
					"min":     min,
					"max":     max,
					"count":   len(metrics),
				},
			})
		}
	}

	return issues
}

// analyzeGaugeMetrics analyzes gauge metrics for performance issues
func (pa *PerformanceAnalyzer) analyzeGaugeMetrics(name string, metrics []Metric) []PerformanceIssue {
	issues := make([]PerformanceIssue, 0)

	// Get the latest value
	latest := metrics[len(metrics)-1]

	// Check memory usage
	if pa.isMemoryMetric(name) {
		if threshold, exists := pa.thresholds["memory_usage"]; exists {
			if latest.Value > threshold.Critical {
				issues = append(issues, PerformanceIssue{
					Type:        "high_memory_usage",
					Severity:    "critical",
					Description: fmt.Sprintf("Memory usage is %f MB, exceeding critical threshold of %f MB", latest.Value/1024/1024, threshold.Critical/1024/1024),
					Value:       latest.Value,
					Unit:        "bytes",
					Threshold:   threshold.Critical,
					Suggestions: []string{
						"Check for memory leaks",
						"Optimize data structures",
						"Consider garbage collection tuning",
						"Profile memory allocation patterns",
					},
				})
			} else if latest.Value > threshold.Warning {
				issues = append(issues, PerformanceIssue{
					Type:        "high_memory_usage",
					Severity:    "warning",
					Description: fmt.Sprintf("Memory usage is %f MB, exceeding warning threshold of %f MB", latest.Value/1024/1024, threshold.Warning/1024/1024),
					Value:       latest.Value,
					Unit:        "bytes",
					Threshold:   threshold.Warning,
					Suggestions: []string{
						"Monitor memory usage trends",
						"Consider memory optimization",
					},
				})
			}
		}
	}

	// Check goroutine count
	if pa.isGoroutineMetric(name) {
		if threshold, exists := pa.thresholds["goroutine_count"]; exists {
			if latest.Value > threshold.Critical {
				issues = append(issues, PerformanceIssue{
					Type:        "high_goroutine_count",
					Severity:    "critical",
					Description: fmt.Sprintf("Goroutine count is %f, exceeding critical threshold of %f", latest.Value, threshold.Critical),
					Value:       latest.Value,
					Unit:        "count",
					Threshold:   threshold.Critical,
					Suggestions: []string{
						"Check for goroutine leaks",
						"Optimize concurrency patterns",
						"Consider using worker pools",
						"Review channel usage",
					},
				})
			} else if latest.Value > threshold.Warning {
				issues = append(issues, PerformanceIssue{
					Type:        "high_goroutine_count",
					Severity:    "warning",
					Description: fmt.Sprintf("Goroutine count is %f, exceeding warning threshold of %f", latest.Value, threshold.Warning),
					Value:       latest.Value,
					Unit:        "count",
					Threshold:   threshold.Warning,
					Suggestions: []string{
						"Monitor goroutine count trends",
						"Review concurrency patterns",
					},
				})
			}
		}
	}

	return issues
}

// analyzeCounterMetrics analyzes counter metrics
func (pa *PerformanceAnalyzer) analyzeCounterMetrics(_ string, _ []Metric) []PerformanceIssue {
	// Counters are typically used for counting events
	// Analysis would depend on the specific counter
	return []PerformanceIssue{}
}

// analyzeHistogramMetrics analyzes histogram metrics
func (pa *PerformanceAnalyzer) analyzeHistogramMetrics(_ string, _ []Metric) []PerformanceIssue {
	// Histograms provide distribution information
	// Analysis would depend on the specific histogram
	return []PerformanceIssue{}
}

// isMemoryMetric checks if a metric name indicates memory usage
func (pa *PerformanceAnalyzer) isMemoryMetric(name string) bool {
	memoryKeywords := []string{"memory", "alloc", "heap", "sys"}
	for _, keyword := range memoryKeywords {
		if contains(name, keyword) {
			return true
		}
	}
	return false
}

// isGoroutineMetric checks if a metric name indicates goroutine count
func (pa *PerformanceAnalyzer) isGoroutineMetric(name string) bool {
	goroutineKeywords := []string{"goroutine", "goroutines"}
	for _, keyword := range goroutineKeywords {
		if contains(name, keyword) {
			return true
		}
	}
	return false
}

// calculateStats calculates basic statistics for a slice of values
func (pa *PerformanceAnalyzer) calculateStats(values []float64) (avg, max, min float64) {
	if len(values) == 0 {
		return 0, 0, 0
	}

	sum := 0.0
	max = values[0]
	min = values[0]

	for _, v := range values {
		sum += v
		if v > max {
			max = v
		}
		if v < min {
			min = v
		}
	}

	avg = sum / float64(len(values))
	return avg, max, min
}

// generateRecommendations generates general recommendations based on issues
func (pa *PerformanceAnalyzer) generateRecommendations(issues []PerformanceIssue) []string {
	recommendations := make([]string, 0)
	seen := make(map[string]bool)

	for _, issue := range issues {
		for _, suggestion := range issue.Suggestions {
			if !seen[suggestion] {
				recommendations = append(recommendations, suggestion)
				seen[suggestion] = true
			}
		}
	}

	// Add general recommendations
	generalRecommendations := []string{
		"Run 'go tool pprof' on the generated profile files for detailed analysis",
		"Consider using 'go test -bench' for benchmarking specific functions",
		"Use 'go test -race' to detect race conditions",
		"Profile with different workloads to identify patterns",
	}

	for _, rec := range generalRecommendations {
		if !seen[rec] {
			recommendations = append(recommendations, rec)
		}
	}

	return recommendations
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > len(substr) && (s[:len(substr)] == substr ||
			s[len(s)-len(substr):] == substr ||
			containsSubstring(s, substr))))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
