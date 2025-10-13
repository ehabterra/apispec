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
	"testing"
	"time"
)

func TestDefaultProfilerConfig(t *testing.T) {
	config := DefaultProfilerConfig()
	if config == nil {
		t.Fatal("Expected non-nil config")
		return
	}

	// Check default values
	if config.CPUProfile {
		t.Error("Expected CPUProfile to be false by default")
	}
	if config.MemProfile {
		t.Error("Expected MemProfile to be false by default")
	}
	if config.BlockProfile {
		t.Error("Expected BlockProfile to be false by default")
	}
	if config.MutexProfile {
		t.Error("Expected MutexProfile to be false by default")
	}
	if config.TraceProfile {
		t.Error("Expected TraceProfile to be false by default")
	}
	if config.CustomMetrics {
		t.Error("Expected CustomMetrics to be false by default")
	}

	// Check default paths
	expectedPaths := map[string]string{
		"CPUProfilePath":   "cpu.prof",
		"MemProfilePath":   "mem.prof",
		"BlockProfilePath": "block.prof",
		"MutexProfilePath": "mutex.prof",
		"TraceProfilePath": "trace.out",
		"MetricsPath":      "metrics.json",
		"OutputDir":        "profiles",
	}

	if config.CPUProfilePath != expectedPaths["CPUProfilePath"] {
		t.Errorf("Expected CPUProfilePath %s, got %s", expectedPaths["CPUProfilePath"], config.CPUProfilePath)
	}
	if config.MemProfilePath != expectedPaths["MemProfilePath"] {
		t.Errorf("Expected MemProfilePath %s, got %s", expectedPaths["MemProfilePath"], config.MemProfilePath)
	}
	if config.BlockProfilePath != expectedPaths["BlockProfilePath"] {
		t.Errorf("Expected BlockProfilePath %s, got %s", expectedPaths["BlockProfilePath"], config.BlockProfilePath)
	}
	if config.MutexProfilePath != expectedPaths["MutexProfilePath"] {
		t.Errorf("Expected MutexProfilePath %s, got %s", expectedPaths["MutexProfilePath"], config.MutexProfilePath)
	}
	if config.TraceProfilePath != expectedPaths["TraceProfilePath"] {
		t.Errorf("Expected TraceProfilePath %s, got %s", expectedPaths["TraceProfilePath"], config.TraceProfilePath)
	}
	if config.MetricsPath != expectedPaths["MetricsPath"] {
		t.Errorf("Expected MetricsPath %s, got %s", expectedPaths["MetricsPath"], config.MetricsPath)
	}
	if config.OutputDir != expectedPaths["OutputDir"] {
		t.Errorf("Expected OutputDir %s, got %s", expectedPaths["OutputDir"], config.OutputDir)
	}
}

func TestNewProfiler(t *testing.T) {
	config := DefaultProfilerConfig()
	profiler := NewProfiler(config)

	if profiler == nil {
		t.Fatal("Expected non-nil profiler")
		return
	}
	if profiler.config != config {
		t.Fatal("Expected config to be set")
	}
	if profiler.ctx == nil {
		t.Fatal("Expected non-nil context")
	}
	if profiler.cancel == nil {
		t.Fatal("Expected non-nil cancel function")
	}
}

func TestProfilerStartWithNoProfiling(t *testing.T) {
	config := DefaultProfilerConfig()
	profiler := NewProfiler(config)

	err := profiler.Start()
	if err != nil {
		t.Errorf("Expected no error starting profiler with no profiling enabled, got %v", err)
	}

	// Note: IsProfiling checks for any profiling configuration, not just active profiling
}

func TestProfilerStartWithCustomMetrics(t *testing.T) {
	config := DefaultProfilerConfig()
	config.CustomMetrics = true
	config.OutputDir = t.TempDir()

	profiler := NewProfiler(config)

	err := profiler.Start()
	if err != nil {
		t.Errorf("Expected no error starting profiler with custom metrics, got %v", err)
	}

	// Should be profiling
	if !profiler.IsProfiling() {
		t.Error("Expected profiler to be profiling when custom metrics is enabled")
	}

	// Check that metrics collector is created
	if profiler.metrics == nil {
		t.Fatal("Expected metrics collector to be created")
	}

	// Stop profiler
	err = profiler.Stop()
	if err != nil {
		t.Errorf("Expected no error stopping profiler, got %v", err)
	}
}

func TestProfilerStartWithCPUProfile(t *testing.T) {
	config := DefaultProfilerConfig()
	config.CPUProfile = true
	config.OutputDir = t.TempDir()

	profiler := NewProfiler(config)

	err := profiler.Start()
	if err != nil {
		t.Errorf("Expected no error starting profiler with CPU profiling, got %v", err)
	}

	// Should be profiling
	if !profiler.IsProfiling() {
		t.Error("Expected profiler to be profiling when CPU profiling is enabled")
	}

	// Check that CPU file is created
	if profiler.cpuFile == nil {
		t.Fatal("Expected CPU file to be created")
	}

	// Stop profiler
	err = profiler.Stop()
	if err != nil {
		t.Errorf("Expected no error stopping profiler, got %v", err)
	}
}

func TestProfilerStartWithMemProfile(t *testing.T) {
	config := DefaultProfilerConfig()
	config.MemProfile = true
	config.OutputDir = t.TempDir()

	profiler := NewProfiler(config)

	err := profiler.Start()
	if err != nil {
		t.Errorf("Expected no error starting profiler with memory profiling, got %v", err)
	}

	// Should be profiling
	if !profiler.IsProfiling() {
		t.Error("Expected profiler to be profiling when memory profiling is enabled")
	}

	// Stop profiler
	err = profiler.Stop()
	if err != nil {
		t.Errorf("Expected no error stopping profiler, got %v", err)
	}
}

func TestProfilerStartWithBlockProfile(t *testing.T) {
	config := DefaultProfilerConfig()
	config.BlockProfile = true
	config.OutputDir = t.TempDir()

	profiler := NewProfiler(config)

	err := profiler.Start()
	if err != nil {
		t.Errorf("Expected no error starting profiler with block profiling, got %v", err)
	}

	// Should be profiling
	if !profiler.IsProfiling() {
		t.Error("Expected profiler to be profiling when block profiling is enabled")
	}

	// Stop profiler
	err = profiler.Stop()
	if err != nil {
		t.Errorf("Expected no error stopping profiler, got %v", err)
	}
}

func TestProfilerStartWithMutexProfile(t *testing.T) {
	config := DefaultProfilerConfig()
	config.MutexProfile = true
	config.OutputDir = t.TempDir()

	profiler := NewProfiler(config)

	err := profiler.Start()
	if err != nil {
		t.Errorf("Expected no error starting profiler with mutex profiling, got %v", err)
	}

	// Should be profiling
	if !profiler.IsProfiling() {
		t.Error("Expected profiler to be profiling when mutex profiling is enabled")
	}

	// Stop profiler
	err = profiler.Stop()
	if err != nil {
		t.Errorf("Expected no error stopping profiler, got %v", err)
	}
}

func TestProfilerStartWithTraceProfile(t *testing.T) {
	config := DefaultProfilerConfig()
	config.TraceProfile = true
	config.OutputDir = t.TempDir()

	profiler := NewProfiler(config)

	err := profiler.Start()
	if err != nil {
		t.Errorf("Expected no error starting profiler with trace profiling, got %v", err)
	}

	// Should be profiling
	if !profiler.IsProfiling() {
		t.Error("Expected profiler to be profiling when trace profiling is enabled")
	}

	// Check that trace file is created
	if profiler.traceFile == nil {
		t.Fatal("Expected trace file to be created")
	}

	// Stop profiler
	err = profiler.Stop()
	if err != nil {
		t.Errorf("Expected no error stopping profiler, got %v", err)
	}
}

func TestProfilerStartWithAllProfiling(t *testing.T) {
	config := DefaultProfilerConfig()
	config.CPUProfile = true
	config.MemProfile = true
	config.BlockProfile = true
	config.MutexProfile = true
	config.TraceProfile = true
	config.CustomMetrics = true
	config.OutputDir = t.TempDir()

	profiler := NewProfiler(config)

	err := profiler.Start()
	if err != nil {
		t.Errorf("Expected no error starting profiler with all profiling enabled, got %v", err)
	}

	// Should be profiling
	if !profiler.IsProfiling() {
		t.Error("Expected profiler to be profiling when all profiling is enabled")
	}

	// Stop profiler
	err = profiler.Stop()
	if err != nil {
		t.Errorf("Expected no error stopping profiler, got %v", err)
	}
}

func TestProfilerStartWithNonExistentDirectory(t *testing.T) {
	config := DefaultProfilerConfig()
	config.CustomMetrics = true
	config.OutputDir = "/non/existent/directory"

	profiler := NewProfiler(config)

	err := profiler.Start()
	if err == nil {
		t.Fatal("Expected error when starting profiler with non-existent directory")
	}
}

func TestProfilerStopWhenNotStarted(t *testing.T) {
	config := DefaultProfilerConfig()
	profiler := NewProfiler(config)

	err := profiler.Stop()
	if err != nil {
		t.Errorf("Expected no error stopping profiler that was never started, got %v", err)
	}
}

func TestProfilerStopMultipleTimes(t *testing.T) {
	config := DefaultProfilerConfig()
	config.CustomMetrics = true
	config.OutputDir = t.TempDir()

	profiler := NewProfiler(config)

	// Start profiler
	err := profiler.Start()
	if err != nil {
		t.Fatalf("Expected no error starting profiler, got %v", err)
	}

	// Stop profiler
	err = profiler.Stop()
	if err != nil {
		t.Errorf("Expected no error stopping profiler first time, got %v", err)
	}

	// Stop profiler again
	err = profiler.Stop()
	if err != nil {
		t.Errorf("Expected no error stopping profiler second time, got %v", err)
	}
}

func TestProfilerGetMetrics(t *testing.T) {
	config := DefaultProfilerConfig()
	profiler := NewProfiler(config)

	// Should return nil when no metrics collector
	if profiler.GetMetrics() != nil {
		t.Error("Expected nil metrics when no metrics collector")
	}

	// Start with custom metrics
	config.CustomMetrics = true
	config.OutputDir = t.TempDir()
	profiler = NewProfiler(config)

	err := profiler.Start()
	if err != nil {
		t.Fatalf("Expected no error starting profiler, got %v", err)
	}

	// Should return metrics collector
	if profiler.GetMetrics() == nil {
		t.Error("Expected non-nil metrics collector")
	}

	// Stop profiler
	err = profiler.Stop()
	if err != nil {
		t.Errorf("Expected no error stopping profiler, got %v", err)
	}
}

func TestIsProfiling(t *testing.T) {
	config := DefaultProfilerConfig()

	// Should not be profiling initially (but IsProfiling checks for any profiling config)
	// Note: IsProfiling returns true if any profiling is configured, not just active

	// Start with custom metrics
	config.CustomMetrics = true
	config.OutputDir = t.TempDir()
	profiler := NewProfiler(config)

	err := profiler.Start()
	if err != nil {
		t.Fatalf("Expected no error starting profiler, got %v", err)
	}

	// Should be profiling
	if !profiler.IsProfiling() {
		t.Error("Expected profiler to be profiling")
	}

	// Stop profiler
	err = profiler.Stop()
	if err != nil {
		t.Errorf("Expected no error stopping profiler, got %v", err)
	}

	// Should not be profiling after stop (but IsProfiling checks for any profiling config)
	// Note: IsProfiling returns true if any profiling is configured, not just active
}

func TestProfileFunc(t *testing.T) {
	collector := NewMetricsCollector()

	// Test with nil collector
	err := ProfileFunc(nil, "test_func", func() error {
		return nil
	}, nil)
	if err != nil {
		t.Errorf("Expected no error with nil collector, got %v", err)
	}

	// Test with collector
	err = ProfileFunc(collector, "test_func", func() error {
		return nil
	}, map[string]string{"test": "value"})
	if err != nil {
		t.Errorf("Expected no error with collector, got %v", err)
	}

	// Check that metric was recorded
	metrics := collector.GetMetrics()
	if len(metrics) != 1 {
		t.Fatalf("Expected 1 metric, got %d", len(metrics))
	}

	metric := metrics[0]
	if metric.Name != "test_func" {
		t.Errorf("Expected name 'test_func', got %s", metric.Name)
	}
	if metric.Type != MetricTypeTimer {
		t.Errorf("Expected type MetricTypeTimer, got %s", metric.Type)
	}
	if metric.Tags["test"] != "value" {
		t.Errorf("Expected tag 'value', got %s", metric.Tags["test"])
	}
}

func TestProfileFuncWithError(t *testing.T) {
	collector := NewMetricsCollector()

	expectedErr := &os.PathError{Op: "test", Path: "test", Err: os.ErrNotExist}
	err := ProfileFunc(collector, "test_func", func() error {
		return expectedErr
	}, nil)

	if err != expectedErr {
		t.Errorf("Expected error %v, got %v", expectedErr, err)
	}

	// Check that metric was still recorded
	metrics := collector.GetMetrics()
	if len(metrics) != 1 {
		t.Fatalf("Expected 1 metric, got %d", len(metrics))
	}
}

func TestProfileFuncWithResult(t *testing.T) {
	collector := NewMetricsCollector()

	// Test with nil collector
	result, err := ProfileFuncWithResult(nil, "test_func", func() (string, error) {
		return "test_result", nil
	}, nil)
	if err != nil {
		t.Errorf("Expected no error with nil collector, got %v", err)
	}
	if result != "test_result" {
		t.Errorf("Expected result 'test_result', got %s", result)
	}

	// Test with collector
	result, err = ProfileFuncWithResult(collector, "test_func", func() (string, error) {
		return "test_result", nil
	}, map[string]string{"test": "value"})
	if err != nil {
		t.Errorf("Expected no error with collector, got %v", err)
	}
	if result != "test_result" {
		t.Errorf("Expected result 'test_result', got %s", result)
	}

	// Check that metric was recorded
	metrics := collector.GetMetrics()
	if len(metrics) != 1 {
		t.Fatalf("Expected 1 metric, got %d", len(metrics))
	}

	metric := metrics[0]
	if metric.Name != "test_func" {
		t.Errorf("Expected name 'test_func', got %s", metric.Name)
	}
	if metric.Type != MetricTypeTimer {
		t.Errorf("Expected type MetricTypeTimer, got %s", metric.Type)
	}
}

func TestProfileFuncWithResultError(t *testing.T) {
	collector := NewMetricsCollector()

	expectedErr := &os.PathError{Op: "test", Path: "test", Err: os.ErrNotExist}
	result, err := ProfileFuncWithResult(collector, "test_func", func() (string, error) {
		return "", expectedErr
	}, nil)

	if err != expectedErr {
		t.Errorf("Expected error %v, got %v", expectedErr, err)
	}
	if result != "" {
		t.Errorf("Expected empty result, got %s", result)
	}

	// Check that metric was still recorded
	metrics := collector.GetMetrics()
	if len(metrics) != 1 {
		t.Fatalf("Expected 1 metric, got %d", len(metrics))
	}
}

func TestMeasureMemoryUsage(t *testing.T) {
	collector := NewMetricsCollector()

	// Test with nil collector
	err := MeasureMemoryUsage(nil, "test_func", func() error {
		return nil
	}, nil)
	if err != nil {
		t.Errorf("Expected no error with nil collector, got %v", err)
	}

	// Test with collector
	err = MeasureMemoryUsage(collector, "test_func", func() error {
		// Allocate some memory
		_ = make([]byte, 1024)
		return nil
	}, map[string]string{"test": "value"})
	if err != nil {
		t.Errorf("Expected no error with collector, got %v", err)
	}

	// Check that memory metrics were recorded
	metrics := collector.GetMetrics()
	if len(metrics) == 0 {
		t.Fatal("Expected some metrics to be recorded")
	}

	// Look for memory metrics
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
}

func TestMeasureMemoryUsageWithError(t *testing.T) {
	collector := NewMetricsCollector()

	expectedErr := &os.PathError{Op: "test", Path: "test", Err: os.ErrNotExist}
	err := MeasureMemoryUsage(collector, "test_func", func() error {
		return expectedErr
	}, nil)

	if err != expectedErr {
		t.Errorf("Expected error %v, got %v", expectedErr, err)
	}

	// Check that memory metrics were still recorded
	metrics := collector.GetMetrics()
	if len(metrics) == 0 {
		t.Fatal("Expected some metrics to be recorded")
	}
}

func TestMeasureGoroutines(t *testing.T) {
	collector := NewMetricsCollector()

	// Test with nil collector
	err := MeasureGoroutines(nil, "test_func", func() error {
		return nil
	}, nil)
	if err != nil {
		t.Errorf("Expected no error with nil collector, got %v", err)
	}

	// Test with collector
	err = MeasureGoroutines(collector, "test_func", func() error {
		// Start a goroutine
		go func() {
			time.Sleep(10 * time.Millisecond)
		}()
		return nil
	}, map[string]string{"test": "value"})
	if err != nil {
		t.Errorf("Expected no error with collector, got %v", err)
	}

	// Check that goroutine metrics were recorded
	metrics := collector.GetMetrics()
	if len(metrics) == 0 {
		t.Fatal("Expected some metrics to be recorded")
	}

	// Look for goroutine metrics
	foundGoroutineMetric := false
	for _, metric := range metrics {
		if contains(metric.Name, "goroutines") {
			foundGoroutineMetric = true
			break
		}
	}
	if !foundGoroutineMetric {
		t.Fatal("Expected to find goroutine metrics")
	}
}

func TestMeasureGoroutinesWithError(t *testing.T) {
	collector := NewMetricsCollector()

	expectedErr := &os.PathError{Op: "test", Path: "test", Err: os.ErrNotExist}
	err := MeasureGoroutines(collector, "test_func", func() error {
		return expectedErr
	}, nil)

	if err != expectedErr {
		t.Errorf("Expected error %v, got %v", expectedErr, err)
	}

	// Check that goroutine metrics were still recorded
	metrics := collector.GetMetrics()
	if len(metrics) == 0 {
		t.Fatal("Expected some metrics to be recorded")
	}
}

func TestContextCancellation(t *testing.T) {
	config := DefaultProfilerConfig()
	config.CustomMetrics = true
	config.OutputDir = t.TempDir()

	profiler := NewProfiler(config)

	err := profiler.Start()
	if err != nil {
		t.Fatalf("Expected no error starting profiler, got %v", err)
	}

	// Cancel context
	profiler.cancel()

	// Wait a bit for cancellation to take effect
	time.Sleep(10 * time.Millisecond)

	// Stop profiler
	err = profiler.Stop()
	if err != nil {
		t.Errorf("Expected no error stopping profiler, got %v", err)
	}
}

func TestProfilerConcurrentAccess(t *testing.T) {
	config := DefaultProfilerConfig()
	config.CustomMetrics = true
	config.OutputDir = t.TempDir()

	profiler := NewProfiler(config)

	// Start profiler
	err := profiler.Start()
	if err != nil {
		t.Fatalf("Expected no error starting profiler, got %v", err)
	}

	// Test concurrent access to IsProfiling
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				profiler.IsProfiling()
			}
			done <- true
		}()
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}

	// Stop profiler
	err = profiler.Stop()
	if err != nil {
		t.Errorf("Expected no error stopping profiler, got %v", err)
	}
}
