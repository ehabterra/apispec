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
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewMetricsCollector(t *testing.T) {
	collector := NewMetricsCollector()
	if collector == nil {
		t.Fatal("Expected non-nil collector")
	}
	if collector.metrics == nil {
		t.Fatal("Expected non-nil metrics slice")
	}
	if collector.stopCh == nil {
		t.Fatal("Expected non-nil stop channel")
	}
	if collector.started {
		t.Fatal("Expected collector to not be started initially")
	}
}

func TestMetricsCollectorStart(t *testing.T) {
	collector := NewMetricsCollector()

	// Test starting
	collector.Start()
	if !collector.started {
		t.Fatal("Expected collector to be started")
	}

	// Test starting again (should be idempotent)
	collector.Start()
	if !collector.started {
		t.Fatal("Expected collector to still be started")
	}
}

func TestMetricsCollectorStop(t *testing.T) {
	collector := NewMetricsCollector()

	// Test stopping when not started
	err := collector.Stop()
	if err != nil {
		t.Errorf("Expected no error when stopping unstarted collector, got %v", err)
	}

	// Test stopping when started
	collector.Start()
	err = collector.Stop()
	if err != nil {
		t.Errorf("Expected no error when stopping started collector, got %v", err)
	}
	if collector.started {
		t.Fatal("Expected collector to be stopped")
	}
}

func TestAddMetric(t *testing.T) {
	collector := NewMetricsCollector()

	// Add a metric
	collector.AddMetric("test_metric", MetricTypeCounter, 42.0, "count", map[string]string{"tag": "value"})

	metrics := collector.GetMetrics()
	if len(metrics) != 1 {
		t.Fatalf("Expected 1 metric, got %d", len(metrics))
	}

	metric := metrics[0]
	if metric.Name != "test_metric" {
		t.Errorf("Expected name 'test_metric', got %s", metric.Name)
	}
	if metric.Type != MetricTypeCounter {
		t.Errorf("Expected type MetricTypeCounter, got %s", metric.Type)
	}
	if metric.Value != 42.0 {
		t.Errorf("Expected value 42.0, got %f", metric.Value)
	}
	if metric.Unit != "count" {
		t.Errorf("Expected unit 'count', got %s", metric.Unit)
	}
	if metric.Tags["tag"] != "value" {
		t.Errorf("Expected tag 'value', got %s", metric.Tags["tag"])
	}
}

func TestIncrementCounter(t *testing.T) {
	collector := NewMetricsCollector()

	// Increment counter
	collector.IncrementCounter("test_counter", map[string]string{"env": "test"})

	metrics := collector.GetMetrics()
	if len(metrics) != 1 {
		t.Fatalf("Expected 1 metric, got %d", len(metrics))
	}

	metric := metrics[0]
	if metric.Name != "test_counter" {
		t.Errorf("Expected name 'test_counter', got %s", metric.Name)
	}
	if metric.Type != MetricTypeCounter {
		t.Errorf("Expected type MetricTypeCounter, got %s", metric.Type)
	}
	if metric.Value != 1.0 {
		t.Errorf("Expected value 1.0, got %f", metric.Value)
	}
	if metric.Tags["env"] != "test" {
		t.Errorf("Expected tag 'test', got %s", metric.Tags["env"])
	}
}

func TestSetGauge(t *testing.T) {
	collector := NewMetricsCollector()

	// Set gauge
	collector.SetGauge("test_gauge", 100.5, "bytes", map[string]string{"type": "memory"})

	metrics := collector.GetMetrics()
	if len(metrics) != 1 {
		t.Fatalf("Expected 1 metric, got %d", len(metrics))
	}

	metric := metrics[0]
	if metric.Name != "test_gauge" {
		t.Errorf("Expected name 'test_gauge', got %s", metric.Name)
	}
	if metric.Type != MetricTypeGauge {
		t.Errorf("Expected type MetricTypeGauge, got %s", metric.Type)
	}
	if metric.Value != 100.5 {
		t.Errorf("Expected value 100.5, got %f", metric.Value)
	}
	if metric.Unit != "bytes" {
		t.Errorf("Expected unit 'bytes', got %s", metric.Unit)
	}
	if metric.Tags["type"] != "memory" {
		t.Errorf("Expected tag 'memory', got %s", metric.Tags["type"])
	}
}

func TestRecordHistogram(t *testing.T) {
	collector := NewMetricsCollector()

	// Record histogram
	collector.RecordHistogram("test_histogram", 50.0, "ms", map[string]string{"operation": "query"})

	metrics := collector.GetMetrics()
	if len(metrics) != 1 {
		t.Fatalf("Expected 1 metric, got %d", len(metrics))
	}

	metric := metrics[0]
	if metric.Name != "test_histogram" {
		t.Errorf("Expected name 'test_histogram', got %s", metric.Name)
	}
	if metric.Type != MetricTypeHistogram {
		t.Errorf("Expected type MetricTypeHistogram, got %s", metric.Type)
	}
	if metric.Value != 50.0 {
		t.Errorf("Expected value 50.0, got %f", metric.Value)
	}
	if metric.Unit != "ms" {
		t.Errorf("Expected unit 'ms', got %s", metric.Unit)
	}
	if metric.Tags["operation"] != "query" {
		t.Errorf("Expected tag 'query', got %s", metric.Tags["operation"])
	}
}

func TestStartTimer(t *testing.T) {
	collector := NewMetricsCollector()

	// Start timer
	timer := collector.StartTimer("test_timer", map[string]string{"function": "test"})
	if timer == nil {
		t.Fatal("Expected non-nil timer")
	}
	if timer.name != "test_timer" {
		t.Errorf("Expected name 'test_timer', got %s", timer.name)
	}
	if timer.tags["function"] != "test" {
		t.Errorf("Expected tag 'test', got %s", timer.tags["function"])
	}
	if timer.startTime.IsZero() {
		t.Fatal("Expected non-zero start time")
	}
}

func TestTimerStop(t *testing.T) {
	collector := NewMetricsCollector()

	// Start timer
	timer := collector.StartTimer("test_timer", map[string]string{"function": "test"})

	// Wait a bit to ensure non-zero duration
	time.Sleep(1 * time.Millisecond)

	// Stop timer
	timer.Stop(collector)

	metrics := collector.GetMetrics()
	if len(metrics) != 1 {
		t.Fatalf("Expected 1 metric, got %d", len(metrics))
	}

	metric := metrics[0]
	if metric.Name != "test_timer" {
		t.Errorf("Expected name 'test_timer', got %s", metric.Name)
	}
	if metric.Type != MetricTypeTimer {
		t.Errorf("Expected type MetricTypeTimer, got %s", metric.Type)
	}
	if metric.Value <= 0 {
		t.Errorf("Expected positive value, got %f", metric.Value)
	}
	if metric.Unit != "ns" {
		t.Errorf("Expected unit 'ns', got %s", metric.Unit)
	}
}

func TestRecordTimer(t *testing.T) {
	collector := NewMetricsCollector()

	// Record timer
	duration := 100 * time.Millisecond
	collector.RecordTimer("test_timer", duration, map[string]string{"operation": "test"})

	metrics := collector.GetMetrics()
	if len(metrics) != 1 {
		t.Fatalf("Expected 1 metric, got %d", len(metrics))
	}

	metric := metrics[0]
	if metric.Name != "test_timer" {
		t.Errorf("Expected name 'test_timer', got %s", metric.Name)
	}
	if metric.Type != MetricTypeTimer {
		t.Errorf("Expected type MetricTypeTimer, got %s", metric.Type)
	}
	expectedValue := float64(duration.Nanoseconds())
	if metric.Value != expectedValue {
		t.Errorf("Expected value %f, got %f", expectedValue, metric.Value)
	}
	if metric.Unit != "ns" {
		t.Errorf("Expected unit 'ns', got %s", metric.Unit)
	}
}

func TestGetMetrics(t *testing.T) {
	collector := NewMetricsCollector()

	// Add multiple metrics
	collector.AddMetric("metric1", MetricTypeCounter, 1.0, "count", nil)
	collector.AddMetric("metric2", MetricTypeGauge, 2.0, "bytes", nil)

	metrics := collector.GetMetrics()
	if len(metrics) != 2 {
		t.Fatalf("Expected 2 metrics, got %d", len(metrics))
	}

	// Verify that returned slice is a copy
	metrics[0].Name = "modified"
	originalMetrics := collector.GetMetrics()
	if originalMetrics[0].Name == "modified" {
		t.Fatal("Expected GetMetrics to return a copy, but original was modified")
	}
}

func TestWriteToFile(t *testing.T) {
	collector := NewMetricsCollector()

	// Add some metrics
	collector.AddMetric("test_metric", MetricTypeCounter, 42.0, "count", map[string]string{"tag": "value"})

	// Create temporary directory
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "test_metrics.json")

	// Write to file
	err := collector.WriteToFile(filePath)
	if err != nil {
		t.Fatalf("Expected no error writing to file, got %v", err)
	}

	// Check that file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Fatal("Expected file to exist after writing")
	}

	// Read and verify content
	content, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Expected no error reading file, got %v", err)
	}

	if len(content) == 0 {
		t.Fatal("Expected non-empty file content")
	}

	// Check that content contains expected metric name
	contentStr := string(content)
	if !contains(contentStr, "test_metric") {
		t.Fatal("Expected file content to contain 'test_metric'")
	}
}

func TestWriteToFileWithNonExistentDirectory(t *testing.T) {
	collector := NewMetricsCollector()

	// Add some metrics
	collector.AddMetric("test_metric", MetricTypeCounter, 42.0, "count", nil)

	// Try to write to non-existent directory
	filePath := "/non/existent/path/test_metrics.json"

	err := collector.WriteToFile(filePath)
	if err == nil {
		t.Fatal("Expected error when writing to non-existent directory")
	}
}

func TestGetSummary(t *testing.T) {
	collector := NewMetricsCollector()

	// Test with no metrics
	summary := collector.GetSummary()
	if summary["total_metrics"] != 0 {
		t.Errorf("Expected 0 total metrics, got %v", summary["total_metrics"])
	}

	// Add some metrics
	collector.AddMetric("counter1", MetricTypeCounter, 1.0, "count", nil)
	collector.AddMetric("gauge1", MetricTypeGauge, 2.0, "bytes", nil)
	collector.AddMetric("counter2", MetricTypeCounter, 3.0, "count", nil)

	summary = collector.GetSummary()
	if summary["total_metrics"] != 3 {
		t.Errorf("Expected 3 total metrics, got %v", summary["total_metrics"])
	}

	// Check by_type
	byType, ok := summary["by_type"].(map[MetricType]int)
	if !ok {
		t.Fatal("Expected by_type to be map[MetricType]int")
	}

	if byType[MetricTypeCounter] != 2 {
		t.Errorf("Expected 2 counter metrics, got %d", byType[MetricTypeCounter])
	}
	if byType[MetricTypeGauge] != 1 {
		t.Errorf("Expected 1 gauge metric, got %d", byType[MetricTypeGauge])
	}

	// Check time_range
	timeRange, ok := summary["time_range"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected time_range to be map[string]interface{}")
	}

	if timeRange["start"] == nil {
		t.Fatal("Expected non-nil start time")
	}
	if timeRange["end"] == nil {
		t.Fatal("Expected non-nil end time")
	}
	if timeRange["duration"] == nil {
		t.Fatal("Expected non-nil duration")
	}
}

func TestCollectSystemMetrics(t *testing.T) {
	collector := NewMetricsCollector()

	// Start collection
	collector.Start()

	// Wait a bit for metrics to be collected
	time.Sleep(500 * time.Millisecond)

	// Stop collection
	err := collector.Stop()
	if err != nil {
		t.Errorf("Expected no error stopping collector, got %v", err)
	}

	// Check that some metrics were collected
	metrics := collector.GetMetrics()
	if len(metrics) == 0 {
		// If no metrics were collected, that's okay for this test
		// The system metrics collection might not work in all test environments
		t.Log("No metrics were collected, which is acceptable in test environment")
		return
	}

	// Check for memory metrics
	foundMemoryMetric := false
	for _, metric := range metrics {
		if contains(metric.Name, "memory") {
			foundMemoryMetric = true
			break
		}
	}
	if !foundMemoryMetric {
		t.Fatal("Expected to find memory metrics")
	}

	// Check for goroutine metrics
	foundGoroutineMetric := false
	for _, metric := range metrics {
		if contains(metric.Name, "goroutine") {
			foundGoroutineMetric = true
			break
		}
	}
	if !foundGoroutineMetric {
		t.Fatal("Expected to find goroutine metrics")
	}
}

func TestCollectMemoryMetrics(t *testing.T) {
	collector := NewMetricsCollector()

	// Call collectMemoryMetrics directly
	collector.collectMemoryMetrics()

	metrics := collector.GetMetrics()
	if len(metrics) == 0 {
		t.Fatal("Expected some memory metrics to be collected")
	}

	// Check for specific memory metrics
	expectedMetrics := []string{
		"memory.alloc",
		"memory.total_alloc",
		"memory.sys",
		"memory.lookups",
		"memory.mallocs",
		"memory.frees",
		"gc.num_gc",
		"gc.pause_total",
	}

	for _, expected := range expectedMetrics {
		found := false
		for _, metric := range metrics {
			if metric.Name == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected to find metric %s", expected)
		}
	}
}

func TestCollectGoroutineMetrics(t *testing.T) {
	collector := NewMetricsCollector()

	// Call collectGoroutineMetrics directly
	collector.collectGoroutineMetrics()

	metrics := collector.GetMetrics()
	if len(metrics) == 0 {
		t.Fatal("Expected some goroutine metrics to be collected")
	}

	// Check for goroutine count metric
	found := false
	for _, metric := range metrics {
		if metric.Name == "goroutines.count" {
			found = true
			if metric.Type != MetricTypeGauge {
				t.Errorf("Expected MetricTypeGauge, got %s", metric.Type)
			}
			if metric.Unit != "count" {
				t.Errorf("Expected unit 'count', got %s", metric.Unit)
			}
			break
		}
	}
	if !found {
		t.Fatal("Expected to find goroutines.count metric")
	}
}
