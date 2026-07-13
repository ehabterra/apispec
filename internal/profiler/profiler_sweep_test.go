// Copyright 2026 Ehab Terra
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
	"runtime"
	"sync"
	"testing"
	"time"
)

// TestStartProfileCreateErrors drives the os.Create failure branches of the
// CPU and trace start paths (and the Start error-wrapping around them) by
// pointing the profile file at a subdirectory that does not exist. Start's
// MkdirAll creates OutputDir but not the extra path segment.
func TestStartProfileCreateErrors(t *testing.T) {
	t.Run("cpu", func(t *testing.T) {
		cfg := DefaultProfilerConfig()
		cfg.OutputDir = t.TempDir()
		cfg.CPUProfile = true
		cfg.CPUProfilePath = filepath.Join("nonexistent", "cpu.prof")
		if err := NewProfiler(cfg).Start(); err == nil {
			t.Fatal("expected Start to fail when the CPU profile path is unwritable")
		}
	})

	t.Run("trace", func(t *testing.T) {
		cfg := DefaultProfilerConfig()
		cfg.OutputDir = t.TempDir()
		cfg.TraceProfile = true
		cfg.TraceProfilePath = filepath.Join("nonexistent", "trace.out")
		p := NewProfiler(cfg)
		if err := p.Start(); err == nil {
			_ = p.Stop()
			t.Fatal("expected Start to fail when the trace profile path is unwritable")
		}
	})
}

// TestStopProfileCreateErrors reaches the os.Create failure branches of
// stopMemProfile/stopBlockProfile/stopMutexProfile and Stop's combined-error
// return. Stop runs these whenever their configured path is non-empty, so no
// Start is required.
func TestStopProfileCreateErrors(t *testing.T) {
	cfg := DefaultProfilerConfig()
	cfg.OutputDir = t.TempDir()
	cfg.MemProfilePath = filepath.Join("nonexistent", "mem.prof")
	cfg.BlockProfilePath = filepath.Join("nonexistent", "block.prof")
	cfg.MutexProfilePath = filepath.Join("nonexistent", "mutex.prof")

	if err := NewProfiler(cfg).Stop(); err == nil {
		t.Fatal("expected Stop to report the profile-write failures")
	}
}

// TestWriteToFileMkdirError covers the MkdirAll failure branch of WriteToFile
// by placing the target directory underneath a regular file.
func TestWriteToFileMkdirError(t *testing.T) {
	fileAsDir := filepath.Join(t.TempDir(), "not-a-dir")
	f, err := os.Create(fileAsDir)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("setup close: %v", err)
	}

	mc := NewMetricsCollector()
	mc.AddMetric("x", MetricTypeCounter, 1, "count", nil)
	target := filepath.Join(fileAsDir, "sub", "metrics.json")
	if err := mc.WriteToFile(target); err == nil {
		t.Fatal("expected WriteToFile to fail when the parent path is a file")
	}
}

// TestCollectSystemMetricsTick lets the 1s system-metrics ticker fire so the
// periodic collection body (memory + goroutine sampling and the goroutine-leak
// detection guard) runs. A burst of blocked goroutines between two ticks trips
// the leak threshold branch.
func TestCollectSystemMetricsTick(t *testing.T) {
	mc := NewMetricsCollector()
	mc.Start()

	// First tick establishes the goroutine baseline.
	time.Sleep(1200 * time.Millisecond)

	// Spawn well over the 100-goroutine leak threshold and hold them across the
	// next tick so currentGoroutines exceeds baseline+100.
	release := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 150; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-release
		}()
	}
	time.Sleep(1200 * time.Millisecond)

	close(release)
	wg.Wait()
	if err := mc.Stop(); err != nil {
		t.Fatalf("stop: %v", err)
	}

	// The periodic collection should have recorded system gauges.
	sawMemory := false
	for _, m := range mc.GetMetrics() {
		if m.Name == "memory.alloc" {
			sawMemory = true
			break
		}
	}
	if !sawMemory {
		t.Error("expected the system-metrics ticker to record memory.alloc")
	}
	// Guard against goroutine leakage from the collector itself.
	runtime.GC()
}
