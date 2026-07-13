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

package engine

import (
	"testing"
	"time"

	spec "github.com/ehabterra/apispec/internal/spec"
)

// TestVerboseLogger covers both gates of every logging method.
func TestVerboseLogger(t *testing.T) {
	for _, verbose := range []bool{true, false} {
		vl := NewVerboseLogger(verbose)
		vl.Printf("format %d\n", 1)
		vl.Println("line")
		vl.Print("raw")
		vl.Warnf("warn %s\n", "always")
	}
}

func TestReportPhase(t *testing.T) {
	// nil engine is a no-op.
	var nilEngine *Engine
	nilEngine.reportPhase("noop", time.Millisecond)

	// OnPhase callback fires with the phase name.
	var gotPhase string
	e := NewEngine(&EngineConfig{OnPhase: func(phase string, elapsed time.Duration) {
		gotPhase = phase
	}})
	e.reportPhase("load", 5*time.Millisecond)
	if gotPhase != "load" {
		t.Errorf("OnPhase got %q, want load", gotPhase)
	}

	// A panicking callback must not crash the engine.
	p := NewEngine(&EngineConfig{OnPhase: func(string, time.Duration) { panic("boom") }})
	p.reportPhase("load", time.Millisecond)

	// No callback configured.
	NewEngine(&EngineConfig{}).reportPhase("load", time.Millisecond)
}

func TestEngineAccessors(t *testing.T) {
	e := NewEngine(&EngineConfig{})
	if got := e.GetUnresolvedSecurity(); len(got) != 0 {
		t.Errorf("fresh engine unresolved security = %v", got)
	}
	if got := e.SkippedPackages(); len(got) != 0 {
		t.Errorf("fresh engine skipped packages = %v", got)
	}
	if got := e.ModuleRoot(); got != "" {
		t.Errorf("fresh engine module root = %q", got)
	}
}

func TestMergeIncludeExcludePatterns(t *testing.T) {
	e := NewEngine(&EngineConfig{
		IncludeFiles:     []string{"a.go"},
		IncludePackages:  []string{"pkgA"},
		IncludeFunctions: []string{"FnA"},
		IncludeTypes:     []string{"TypeA"},
		ExcludeFiles:     []string{"b.go"},
		ExcludePackages:  []string{"pkgB"},
		ExcludeFunctions: []string{"FnB"},
		ExcludeTypes:     []string{"TypeB"},
	})
	cfg := &spec.APISpecConfig{}
	e.mergeIncludeExcludePatterns(cfg)

	if len(cfg.Include.Files) == 0 || cfg.Include.Files[len(cfg.Include.Files)-1] != "a.go" {
		t.Errorf("include files not merged: %v", cfg.Include.Files)
	}
	if len(cfg.Include.Packages) == 0 || len(cfg.Include.Functions) == 0 || len(cfg.Include.Types) == 0 {
		t.Error("include patterns not merged")
	}
	if len(cfg.Exclude.Files) == 0 || cfg.Exclude.Files[len(cfg.Exclude.Files)-1] != "b.go" {
		t.Errorf("exclude files not merged: %v", cfg.Exclude.Files)
	}
	if len(cfg.Exclude.Packages) == 0 || len(cfg.Exclude.Functions) == 0 || len(cfg.Exclude.Types) == 0 {
		t.Error("exclude patterns not merged")
	}
}

func TestShouldIncludeFile(t *testing.T) {
	cases := []struct {
		name string
		cfg  EngineConfig
		file string
		want bool
	}{
		{"plain file no patterns", EngineConfig{}, "handler.go", true},
		{"auto-exclude test suffix", EngineConfig{AutoExcludeTests: true}, "handler_test.go", false},
		{"auto-exclude tests dir", EngineConfig{AutoExcludeTests: true}, "pkg/tests/x.go", false},
		{"tests off keeps test file", EngineConfig{}, "handler_test.go", true},
		{"auto-exclude mock suffix", EngineConfig{AutoExcludeMocks: true}, "store_mock.go", false},
		{"auto-exclude fakes", EngineConfig{AutoExcludeMocks: true}, "db_fakes.go", false},
		{"mocks off keeps mock", EngineConfig{}, "store_mock.go", true},
		{"exclude pattern wins", EngineConfig{ExcludeFiles: []string{"*.gen.go"}}, "api.gen.go", false},
		{"include pattern matches", EngineConfig{IncludeFiles: []string{"api*.go"}}, "api_routes.go", true},
		{"include pattern misses", EngineConfig{IncludeFiles: []string{"api*.go"}}, "internal.go", false},
		{"exclude beats include", EngineConfig{IncludeFiles: []string{"*.go"}, ExcludeFiles: []string{"skip.go"}}, "skip.go", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cfg := c.cfg
			e := NewEngine(&cfg)
			if got := e.shouldIncludeFile(c.file); got != c.want {
				t.Errorf("shouldIncludeFile(%q) = %v, want %v", c.file, got, c.want)
			}
		})
	}
}
