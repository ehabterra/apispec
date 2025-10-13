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
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"runtime/trace"
	"sync"
)

// ProfilerConfig holds configuration for profiling
type ProfilerConfig struct {
	// CPU profiling
	CPUProfile     bool
	CPUProfilePath string

	// Memory profiling
	MemProfile     bool
	MemProfilePath string

	// Block profiling
	BlockProfile     bool
	BlockProfilePath string

	// Mutex profiling
	MutexProfile     bool
	MutexProfilePath string

	// Trace profiling
	TraceProfile     bool
	TraceProfilePath string

	// Custom metrics
	CustomMetrics bool
	MetricsPath   string

	// Output directory for all profiles
	OutputDir string
}

// DefaultProfilerConfig returns a default profiling configuration
func DefaultProfilerConfig() *ProfilerConfig {
	return &ProfilerConfig{
		CPUProfile:       false,
		CPUProfilePath:   "cpu.prof",
		MemProfile:       false,
		MemProfilePath:   "mem.prof",
		BlockProfile:     false,
		BlockProfilePath: "block.prof",
		MutexProfile:     false,
		MutexProfilePath: "mutex.prof",
		TraceProfile:     false,
		TraceProfilePath: "trace.out",
		CustomMetrics:    false,
		MetricsPath:      "metrics.json",
		OutputDir:        "profiles",
	}
}

// Profiler manages different types of profiling
type Profiler struct {
	config *ProfilerConfig
	mu     sync.Mutex

	// CPU profiling
	cpuFile *os.File

	// Trace profiling
	traceFile *os.File

	// Custom metrics
	metrics *MetricsCollector

	// Context for cancellation
	ctx    context.Context
	cancel context.CancelFunc
}

// NewProfiler creates a new profiler instance
func NewProfiler(config *ProfilerConfig) *Profiler {
	ctx, cancel := context.WithCancel(context.Background())
	return &Profiler{
		config: config,
		ctx:    ctx,
		cancel: cancel,
	}
}

// Start begins profiling based on configuration
func (p *Profiler) Start() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Create output directory if it doesn't exist
	if p.config.OutputDir != "" {
		if err := os.MkdirAll(p.config.OutputDir, 0755); err != nil {
			return fmt.Errorf("failed to create output directory: %w", err)
		}
	}

	// Start CPU profiling
	if p.config.CPUProfile {
		if err := p.startCPUProfile(); err != nil {
			return fmt.Errorf("failed to start CPU profiling: %w", err)
		}
	}

	// Start memory profiling
	if p.config.MemProfile {
		if err := p.startMemProfile(); err != nil {
			return fmt.Errorf("failed to start memory profiling: %w", err)
		}
	}

	// Start block profiling
	if p.config.BlockProfile {
		if err := p.startBlockProfile(); err != nil {
			return fmt.Errorf("failed to start block profiling: %w", err)
		}
	}

	// Start mutex profiling
	if p.config.MutexProfile {
		if err := p.startMutexProfile(); err != nil {
			return fmt.Errorf("failed to start mutex profiling: %w", err)
		}
	}

	// Start trace profiling
	if p.config.TraceProfile {
		if err := p.startTraceProfile(); err != nil {
			return fmt.Errorf("failed to start trace profiling: %w", err)
		}
	}

	// Start custom metrics collection
	if p.config.CustomMetrics {
		p.metrics = NewMetricsCollector()
		p.metrics.Start()
	}

	return nil
}

// Stop stops all profiling and writes results to files
func (p *Profiler) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	var errors []error

	// Stop CPU profiling
	if p.cpuFile != nil {
		pprof.StopCPUProfile()
		if err := p.cpuFile.Close(); err != nil {
			errors = append(errors, fmt.Errorf("failed to close CPU profile file: %w", err))
		}
		p.cpuFile = nil
	}

	// Stop memory profiling
	if p.config.MemProfilePath != "" {
		if err := p.stopMemProfile(); err != nil {
			errors = append(errors, fmt.Errorf("failed to stop memory profiling: %w", err))
		}
	}

	// Stop block profiling
	if p.config.BlockProfilePath != "" {
		if err := p.stopBlockProfile(); err != nil {
			errors = append(errors, fmt.Errorf("failed to stop block profiling: %w", err))
		}
	}

	// Stop mutex profiling
	if p.config.MutexProfilePath != "" {
		if err := p.stopMutexProfile(); err != nil {
			errors = append(errors, fmt.Errorf("failed to stop mutex profiling: %w", err))
		}
	}

	// Stop trace profiling
	if p.traceFile != nil {
		trace.Stop()
		if err := p.traceFile.Close(); err != nil {
			errors = append(errors, fmt.Errorf("failed to close trace file: %w", err))
		}
		p.traceFile = nil
	}

	// Stop custom metrics collection
	if p.metrics != nil {
		if err := p.metrics.Stop(); err != nil {
			errors = append(errors, fmt.Errorf("failed to stop metrics collection: %w", err))
		}
	}

	// Cancel context
	p.cancel()

	// Return combined errors
	if len(errors) > 0 {
		return fmt.Errorf("profiling stop errors: %v", errors)
	}

	return nil
}

// startCPUProfile starts CPU profiling
func (p *Profiler) startCPUProfile() error {
	filePath := filepath.Join(p.config.OutputDir, p.config.CPUProfilePath)
	file, err := os.Create(filePath)
	if err != nil {
		return err
	}
	p.cpuFile = file

	if err := pprof.StartCPUProfile(file); err != nil {
		err = file.Close()
		if err != nil {
			log.Printf("Failed to close file: %v", err)
		}
		return err
	}

	fmt.Printf("CPU profiling started: %s\n", filePath)
	return nil
}

// startMemProfile starts memory profiling
func (p *Profiler) startMemProfile() error {
	// Memory profiling is done at stop time
	fmt.Printf("Memory profiling enabled: %s\n", filepath.Join(p.config.OutputDir, p.config.MemProfilePath))
	return nil
}

// stopMemProfile stops memory profiling and writes to file
func (p *Profiler) stopMemProfile() error {
	filePath := filepath.Join(p.config.OutputDir, p.config.MemProfilePath)
	file, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			log.Printf("Failed to close memory profile file: %v", closeErr)
		}
	}()

	runtime.GC() // Force garbage collection before profiling
	if err := pprof.WriteHeapProfile(file); err != nil {
		return err
	}

	fmt.Printf("Memory profile written: %s\n", filePath)
	return nil
}

// startBlockProfile starts block profiling
func (p *Profiler) startBlockProfile() error {
	runtime.SetBlockProfileRate(1) // Profile every block event
	fmt.Printf("Block profiling enabled: %s\n", filepath.Join(p.config.OutputDir, p.config.BlockProfilePath))
	return nil
}

// stopBlockProfile stops block profiling and writes to file
func (p *Profiler) stopBlockProfile() error {
	filePath := filepath.Join(p.config.OutputDir, p.config.BlockProfilePath)
	file, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			log.Printf("Failed to close block profile file: %v", closeErr)
		}
	}()

	if err := pprof.Lookup("block").WriteTo(file, 0); err != nil {
		return err
	}

	runtime.SetBlockProfileRate(0) // Disable block profiling
	fmt.Printf("Block profile written: %s\n", filePath)
	return nil
}

// startMutexProfile starts mutex profiling
func (p *Profiler) startMutexProfile() error {
	runtime.SetMutexProfileFraction(1) // Profile every mutex event
	fmt.Printf("Mutex profiling enabled: %s\n", filepath.Join(p.config.OutputDir, p.config.MutexProfilePath))
	return nil
}

// stopMutexProfile stops mutex profiling and writes to file
func (p *Profiler) stopMutexProfile() error {
	filePath := filepath.Join(p.config.OutputDir, p.config.MutexProfilePath)
	file, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			log.Printf("Failed to close mutex profile file: %v", closeErr)
		}
	}()

	if err := pprof.Lookup("mutex").WriteTo(file, 0); err != nil {
		return err
	}

	runtime.SetMutexProfileFraction(0) // Disable mutex profiling
	fmt.Printf("Mutex profile written: %s\n", filePath)
	return nil
}

// startTraceProfile starts trace profiling
func (p *Profiler) startTraceProfile() error {
	filePath := filepath.Join(p.config.OutputDir, p.config.TraceProfilePath)
	file, err := os.Create(filePath)
	if err != nil {
		return err
	}
	p.traceFile = file

	// trace.Start() writes its own header, so we don't need to write one manually
	if err := trace.Start(file); err != nil {
		if closeErr := file.Close(); closeErr != nil {
			log.Printf("Failed to close trace profile file: %v", closeErr)
		}
		return err
	}

	fmt.Printf("Trace profiling started: %s\n", filePath)
	return nil
}

// GetMetrics returns the metrics collector if available
func (p *Profiler) GetMetrics() *MetricsCollector {
	return p.metrics
}

// IsProfiling returns true if any profiling is active
func (p *Profiler) IsProfiling() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.cpuFile != nil || p.traceFile != nil || p.metrics != nil ||
		p.config.MemProfilePath != "" || p.config.BlockProfilePath != "" || p.config.MutexProfilePath != ""
}

// ProfileFunc profiles a function execution
func ProfileFunc(mc *MetricsCollector, name string, fn func() error, tags map[string]string) error {
	if mc == nil {
		return fn()
	}

	timer := mc.StartTimer(name, tags)
	defer timer.Stop(mc)

	return fn()
}

// ProfileFuncWithResult profiles a function execution and returns the result
func ProfileFuncWithResult[T any](mc *MetricsCollector, name string, fn func() (T, error), tags map[string]string) (T, error) {
	if mc == nil {
		return fn()
	}

	timer := mc.StartTimer(name, tags)
	defer timer.Stop(mc)

	return fn()
}

// MeasureMemoryUsage measures memory usage before and after a function call
func MeasureMemoryUsage(mc *MetricsCollector, name string, fn func() error, tags map[string]string) error {
	if mc == nil {
		return fn()
	}

	// Force garbage collection before measurement
	runtime.GC()

	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)

	err := fn()

	// Force GC after measurement for accurate memory tracking
	runtime.GC()
	runtime.ReadMemStats(&after)

	// Record memory metrics
	memTags := make(map[string]string)
	for k, v := range tags {
		memTags[k] = v
	}
	memTags["measurement"] = "memory"

	mc.SetGauge(fmt.Sprintf("%s.memory.alloc_before", name), float64(before.Alloc), "bytes", memTags)
	mc.SetGauge(fmt.Sprintf("%s.memory.alloc_after", name), float64(after.Alloc), "bytes", memTags)
	mc.SetGauge(fmt.Sprintf("%s.memory.alloc_delta", name), float64(after.Alloc-before.Alloc), "bytes", memTags)
	mc.SetGauge(fmt.Sprintf("%s.memory.total_alloc_delta", name), float64(after.TotalAlloc-before.TotalAlloc), "bytes", memTags)

	return err
}

// MeasureGoroutines measures goroutine count before and after a function call
func MeasureGoroutines(mc *MetricsCollector, name string, fn func() error, tags map[string]string) error {
	if mc == nil {
		return fn()
	}

	before := runtime.NumGoroutine()
	err := fn()
	after := runtime.NumGoroutine()

	// Record goroutine metrics
	goroutineTags := make(map[string]string)
	for k, v := range tags {
		goroutineTags[k] = v
	}
	goroutineTags["measurement"] = "goroutines"

	mc.SetGauge(fmt.Sprintf("%s.goroutines.before", name), float64(before), "count", goroutineTags)
	mc.SetGauge(fmt.Sprintf("%s.goroutines.after", name), float64(after), "count", goroutineTags)
	mc.SetGauge(fmt.Sprintf("%s.goroutines.delta", name), float64(after-before), "count", goroutineTags)

	return err
}
