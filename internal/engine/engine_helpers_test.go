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
	"strings"
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

// TestApplyDetectedWrappersIsIdempotent pins that a second generation produces the
// same config as the first.
//
// The config may be the CALLER's — the UI hands the same one to every run — and
// derived patterns are prepended into it. Without a guard, every regeneration grew
// the user's config another copy of everything derived (CodeRabbit, PR #236).
func TestApplyDetectedWrappersIsIdempotent(t *testing.T) {
	wrappers := []spec.DetectedWrapper{{
		RecvType: "example.com/app.*Router",
		Methods:  []string{"Get"},
		Complete: true,
		Pattern: spec.RoutePattern{
			CallRegex:     "^(Get)$",
			RecvTypeRegex: `^example\.com/app\.\*?Router$`,
			PathFromArg:   true,
		},
	}}

	cfg := &spec.APISpecConfig{}
	if applied := applyDetectedWrappers(cfg, wrappers); applied != 1 {
		t.Fatalf("first run applied %d, want 1", applied)
	}
	if applied := applyDetectedWrappers(cfg, wrappers); applied != 0 {
		t.Errorf("second run applied %d more, want 0", applied)
	}
	if got := len(cfg.Framework.RoutePatterns); got != 1 {
		t.Errorf("config holds %d route patterns after two runs, want 1", got)
	}
}

// TestApplyDetectedWrappersAppliesEveryRole pins that a method carrying several
// roles contributes all of them. Roles accumulate per method, so a context method
// that writes a status and reads a parameter is one entry with two patterns — and
// applying only the first would silently drop the other.
func TestApplyDetectedWrappersAppliesEveryRole(t *testing.T) {
	cfg := &spec.APISpecConfig{}
	applied := applyDetectedWrappers(cfg, []spec.DetectedWrapper{{
		RecvType: "example.com/app.*Ctx",
		Methods:  []string{"Answer"},
		Complete: true,
		Response: &spec.ResponsePattern{CallRegex: "^(Answer)$", RecvTypeRegex: "^app.Ctx$", TypeFromArg: true},
		Param:    &spec.ParamPattern{CallRegex: "^(Answer)$", RecvTypeRegex: "^app.Ctx$", ParamIn: "query"},
	}})

	if applied != 2 {
		t.Errorf("applied %d patterns, want 2 (a response and a parameter)", applied)
	}
	if len(cfg.Framework.ResponsePatterns) != 1 || len(cfg.Framework.ParamPatterns) != 1 {
		t.Errorf("responses=%d params=%d, want one of each",
			len(cfg.Framework.ResponsePatterns), len(cfg.Framework.ParamPatterns))
	}

	// Every kind reaches its own list — a mount and a request body alongside the
	// two above, so no role is silently dropped on the way to the engine.
	all := &spec.APISpecConfig{}
	applyDetectedWrappers(all, []spec.DetectedWrapper{
		{Complete: true, Mount: &spec.MountPattern{CallRegex: "^(Group)$", RecvTypeRegex: "^app.Router$", IsMount: true}},
		{Complete: true, Request: &spec.RequestBodyPattern{CallRegex: "^(Bind)$", RecvTypeRegex: "^app.Ctx$", TypeFromArg: true}},
		// Incomplete derivations are reported, never applied.
		{Complete: false, Pattern: spec.RoutePattern{CallRegex: "^(Combo)$"}},
	})
	if len(all.Framework.MountPatterns) != 1 || len(all.Framework.RequestBodyPatterns) != 1 {
		t.Errorf("mounts=%d requests=%d, want one of each",
			len(all.Framework.MountPatterns), len(all.Framework.RequestBodyPatterns))
	}
	if len(all.Framework.RoutePatterns) != 0 {
		t.Errorf("an incomplete derivation was applied: %+v", all.Framework.RoutePatterns)
	}

	// A response derivation that resolved neither role describes nothing, so it is
	// not applied at all.
	empty := &spec.APISpecConfig{}
	applyDetectedWrappers(empty, []spec.DetectedWrapper{{
		Complete: true,
		Response: &spec.ResponsePattern{CallRegex: "^(Nothing)$"},
	}})
	if len(empty.Framework.ResponsePatterns) != 0 {
		t.Errorf("a response pattern with no status and no body was applied: %+v", empty.Framework.ResponsePatterns)
	}
}

// TestWrapperSummary covers the line a run prints about what it derived. It is
// the only place a user sees that a wrapper was found — including one that was
// found but NOT applied, which is the case that explains a thin spec.
func TestWrapperSummary(t *testing.T) {
	got := wrapperSummary([]spec.DetectedWrapper{
		{RecvType: "app.*Router", Methods: []string{"Get", "Post"}, Via: "chi.Method", Complete: true},
		{RecvType: "app.*Router", Methods: []string{"Group"}, Via: "prefix", Complete: true, Mount: &spec.MountPattern{}},
		{RecvType: "app.*Ctx", Methods: []string{"JSON"}, Via: "http.WriteHeader", Complete: true, Response: &spec.ResponsePattern{}},
		{RecvType: "app.*Ctx", Methods: []string{"Bind"}, Via: "json.Decode", Complete: true, Request: &spec.RequestBodyPattern{}},
		{RecvType: "app.*Ctx", Methods: []string{"Query"}, Via: "url.Get", Complete: true, Param: &spec.ParamPattern{}},
		{RecvType: "app.*Combo", Methods: []string{"Get"}, Via: "app.Get", Complete: false},
	})

	for _, want := range []string{
		"app.*Router [Get Post] route via chi.Method (applied)",
		"app.*Router [Group] mount via prefix (applied)",
		"app.*Ctx [JSON] response via http.WriteHeader (applied)",
		"app.*Ctx [Bind] request via json.Decode (applied)",
		"app.*Ctx [Query] param via url.Get (applied)",
		"app.*Combo [Get] route via app.Get (incomplete, not applied)",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("summary is missing %q:\n%s", want, got)
		}
	}

	if got := wrapperSummary(nil); got != "" {
		t.Errorf("nothing derived rendered %q, want empty", got)
	}
}
