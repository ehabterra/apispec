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

	"github.com/ehabterra/apispec/internal/spec"
)

// Regression: include/exclude patterns set on the APISpecConfig (e.g. via the
// UI) must reach the package filter. Previously only CLI-flag patterns on the
// EngineConfig were honored, so UI filters were silently ignored.
func TestApplyConfigFilters_HonorsAPISpecConfig(t *testing.T) {
	e := NewEngine(&EngineConfig{
		APISpecConfig: &spec.APISpecConfig{
			Include: spec.IncludeExclude{Packages: []string{"example.com/app/keep"}},
			Exclude: spec.IncludeExclude{Packages: []string{"example.com/app/skip"}},
		},
	})
	// NewEngine may merge defaults; ensure our config survived.
	if e.config.APISpecConfig == nil {
		e.config.APISpecConfig = &spec.APISpecConfig{
			Include: spec.IncludeExclude{Packages: []string{"example.com/app/keep"}},
			Exclude: spec.IncludeExclude{Packages: []string{"example.com/app/skip"}},
		}
	}

	e.applyConfigFilters()

	if len(e.config.IncludePackages) != 1 || e.config.IncludePackages[0] != "example.com/app/keep" {
		t.Fatalf("include not synced into EngineConfig: %v", e.config.IncludePackages)
	}
	if len(e.config.ExcludePackages) != 1 || e.config.ExcludePackages[0] != "example.com/app/skip" {
		t.Fatalf("exclude not synced into EngineConfig: %v", e.config.ExcludePackages)
	}

	if e.shouldIncludePackage("example.com/app/skip") {
		t.Error("excluded package should not be included")
	}
	if !e.shouldIncludePackage("example.com/app/keep") {
		t.Error("included package should be included")
	}
	if e.shouldIncludePackage("example.com/app/other") {
		t.Error("package not in the include allowlist should be excluded")
	}

	// Idempotent: a second call must not duplicate entries.
	e.applyConfigFilters()
	if len(e.config.IncludePackages) != 1 || len(e.config.ExcludePackages) != 1 {
		t.Errorf("applyConfigFilters not idempotent: inc=%v exc=%v", e.config.IncludePackages, e.config.ExcludePackages)
	}
}

func TestUnionStrings(t *testing.T) {
	got := unionStrings([]string{"a", "b"}, []string{"b", "c"})
	if len(got) != 3 || got[0] != "a" || got[1] != "b" || got[2] != "c" {
		t.Errorf("unionStrings = %v, want [a b c]", got)
	}
	if got := unionStrings([]string{"a"}, nil); len(got) != 1 {
		t.Errorf("unionStrings nil extras = %v", got)
	}
	if got := unionStrings(nil, []string{"x", "x"}); len(got) != 1 {
		t.Errorf("unionStrings dedup extras = %v", got)
	}
}
