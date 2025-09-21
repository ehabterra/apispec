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
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

// MetricType represents the type of metric
type MetricType string

const (
	MetricTypeCounter   MetricType = "counter"
	MetricTypeGauge     MetricType = "gauge"
	MetricTypeHistogram MetricType = "histogram"
	MetricTypeTimer     MetricType = "timer"
)

// Metric represents a single metric measurement
type Metric struct {
	Name      string            `json:"name"`
	Type      MetricType        `json:"type"`
	Value     float64           `json:"value"`
	Unit      string            `json:"unit,omitempty"`
	Timestamp time.Time         `json:"timestamp"`
	Tags      map[string]string `json:"tags,omitempty"`
}

// Timer represents a timing measurement
type Timer struct {
	name      string
	startTime time.Time
	tags      map[string]string
}

// MetricsCollector collects and stores custom metrics
type MetricsCollector struct {
	mu      sync.RWMutex
	metrics []Metric
	started bool
	stopCh  chan struct{}
}

// NewMetricsCollector creates a new metrics collector
func NewMetricsCollector() *MetricsCollector {
	return &MetricsCollector{
		metrics: make([]Metric, 0),
		stopCh:  make(chan struct{}),
	}
}

// Start begins metrics collection
func (mc *MetricsCollector) Start() {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	if mc.started {
		return
	}

	mc.started = true
	go mc.collectSystemMetrics()
}

// Stop stops metrics collection
func (mc *MetricsCollector) Stop() error {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	if !mc.started {
		return nil
	}

	close(mc.stopCh)
	mc.started = false
	return nil
}

// AddMetric adds a custom metric
func (mc *MetricsCollector) AddMetric(name string, metricType MetricType, value float64, unit string, tags map[string]string) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	metric := Metric{
		Name:      name,
		Type:      metricType,
		Value:     value,
		Unit:      unit,
		Timestamp: time.Now(),
		Tags:      tags,
	}

	mc.metrics = append(mc.metrics, metric)
}

// IncrementCounter increments a counter metric
func (mc *MetricsCollector) IncrementCounter(name string, tags map[string]string) {
	mc.AddMetric(name, MetricTypeCounter, 1, "", tags)
}

// SetGauge sets a gauge metric value
func (mc *MetricsCollector) SetGauge(name string, value float64, unit string, tags map[string]string) {
	mc.AddMetric(name, MetricTypeGauge, value, unit, tags)
}

// RecordHistogram records a histogram value
func (mc *MetricsCollector) RecordHistogram(name string, value float64, unit string, tags map[string]string) {
	mc.AddMetric(name, MetricTypeHistogram, value, unit, tags)
}

// StartTimer starts a timer
func (mc *MetricsCollector) StartTimer(name string, tags map[string]string) *Timer {
	return &Timer{
		name:      name,
		startTime: time.Now(),
		tags:      tags,
	}
}

// Stop stops the timer and records the duration
func (t *Timer) Stop(mc *MetricsCollector) {
	duration := time.Since(t.startTime)
	mc.RecordTimer(t.name, duration, t.tags)
}

// RecordTimer records a timer metric
func (mc *MetricsCollector) RecordTimer(name string, duration time.Duration, tags map[string]string) {
	mc.AddMetric(name, MetricTypeTimer, float64(duration.Nanoseconds()), "ns", tags)
}

// GetMetrics returns all collected metrics
func (mc *MetricsCollector) GetMetrics() []Metric {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	// Return a copy to avoid race conditions
	metrics := make([]Metric, len(mc.metrics))
	copy(metrics, mc.metrics)
	return metrics
}

// WriteToFile writes metrics to a JSON file
func (mc *MetricsCollector) WriteToFile(filePath string) error {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	// Create directory if it doesn't exist
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Write metrics to file
	file, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("failed to create metrics file: %w", err)
	}
	defer func() {
		err = file.Close()
		if err != nil {
			log.Printf("Failed to close file: %v", err)
		}
	}()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")

	if err := encoder.Encode(mc.metrics); err != nil {
		return fmt.Errorf("failed to encode metrics: %w", err)
	}

	fmt.Printf("Metrics written to: %s\n", filePath)
	return nil
}

// collectSystemMetrics collects system-level metrics periodically
func (mc *MetricsCollector) collectSystemMetrics() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			mc.collectMemoryMetrics()
			mc.collectGoroutineMetrics()
		case <-mc.stopCh:
			return
		}
	}
}

// collectMemoryMetrics collects memory-related metrics
func (mc *MetricsCollector) collectMemoryMetrics() {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	tags := map[string]string{"type": "system"}

	// Memory usage in bytes
	mc.SetGauge("memory.alloc", float64(m.Alloc), "bytes", tags)
	mc.SetGauge("memory.total_alloc", float64(m.TotalAlloc), "bytes", tags)
	mc.SetGauge("memory.sys", float64(m.Sys), "bytes", tags)
	mc.SetGauge("memory.lookups", float64(m.Lookups), "count", tags)
	mc.SetGauge("memory.mallocs", float64(m.Mallocs), "count", tags)
	mc.SetGauge("memory.frees", float64(m.Frees), "count", tags)

	// GC metrics
	mc.SetGauge("gc.num_gc", float64(m.NumGC), "count", tags)
	mc.SetGauge("gc.pause_total", float64(m.PauseTotalNs), "ns", tags)
	if m.NumGC > 0 {
		mc.SetGauge("gc.pause_avg", float64(m.PauseTotalNs)/float64(m.NumGC), "ns", tags)
	}
}

// collectGoroutineMetrics collects goroutine-related metrics
func (mc *MetricsCollector) collectGoroutineMetrics() {
	numGoroutines := runtime.NumGoroutine()
	tags := map[string]string{"type": "system"}

	mc.SetGauge("goroutines.count", float64(numGoroutines), "count", tags)
}

// GetSummary returns a summary of collected metrics
func (mc *MetricsCollector) GetSummary() map[string]interface{} {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	summary := map[string]interface{}{
		"total_metrics":   len(mc.metrics),
		"collection_time": time.Now(),
	}

	// Group metrics by type
	byType := make(map[MetricType]int)
	for _, metric := range mc.metrics {
		byType[metric.Type]++
	}
	summary["by_type"] = byType

	// Get time range
	if len(mc.metrics) > 0 {
		first := mc.metrics[0].Timestamp
		last := mc.metrics[len(mc.metrics)-1].Timestamp
		summary["time_range"] = map[string]interface{}{
			"start":    first,
			"end":      last,
			"duration": last.Sub(first),
		}
	}

	return summary
}
