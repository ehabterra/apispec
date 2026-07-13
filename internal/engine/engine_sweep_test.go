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
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestEngineCtx(t *testing.T) {
	if NewEngine(&EngineConfig{}).ctx() == nil {
		t.Error("default ctx must not be nil")
	}
	type key struct{}
	custom := context.WithValue(context.Background(), key{}, "v")
	e := NewEngine(&EngineConfig{Context: custom})
	if e.ctx().Value(key{}) != "v" {
		t.Error("configured context not returned")
	}
}

func TestModuleImportPath(t *testing.T) {
	e := NewEngine(&EngineConfig{})

	// No module root resolved yet.
	if got := e.moduleImportPath(); got != "" {
		t.Errorf("empty root = %q", got)
	}

	// go.mod present: the module line wins.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("// comment\nmodule example.com/mymod\n\ngo 1.26\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	e.config.moduleRoot = dir
	if got := e.moduleImportPath(); got != "example.com/mymod" {
		t.Errorf("module path = %q, want example.com/mymod", got)
	}

	// go.mod without a module line.
	dir2 := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir2, "go.mod"), []byte("go 1.26\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	e.config.moduleRoot = dir2
	if got := e.moduleImportPath(); got != "" {
		t.Errorf("missing module line = %q, want empty", got)
	}

	// go.mod unreadable/missing.
	e.config.moduleRoot = filepath.Join(dir2, "nope")
	if got := e.moduleImportPath(); got != "" {
		t.Errorf("missing go.mod = %q, want empty", got)
	}
}

func TestShouldIncludePackage(t *testing.T) {
	cases := []struct {
		name string
		cfg  EngineConfig
		pkg  string
		want bool
	}{
		{"plain package", EngineConfig{}, "example.com/app/api", true},
		{"cgo sqlite skipped", EngineConfig{SkipCGOPackages: true}, "github.com/mattn/go-sqlite3", false},
		{"cgo graft-tensorflow skipped", EngineConfig{SkipCGOPackages: true}, "github.com/x/graft/tensorflow", false},
		{"cgo off keeps sqlite", EngineConfig{}, "github.com/mattn/go-sqlite3", true},
		{"auto-exclude _test pkg", EngineConfig{AutoExcludeTests: true}, "example.com/app/foo_test", false},
		{"auto-exclude mocks pkg", EngineConfig{AutoExcludeMocks: true}, "example.com/app/mocks", false},
		{"auto-exclude stubs pkg", EngineConfig{AutoExcludeMocks: true}, "example.com/app/stubs", false},
		{"exclude pattern", EngineConfig{ExcludePackages: []string{"internal"}}, "example.com/app/internal", false},
		{"exclude last-segment match", EngineConfig{ExcludePackages: []string{"vendor*"}}, "example.com/app/vendorx", false},
		{"include pattern hit", EngineConfig{IncludePackages: []string{"*api*"}}, "example.com/app/api", true},
		{"include last-segment hit", EngineConfig{IncludePackages: []string{"api"}}, "example.com/app/api", true},
		{"include pattern miss", EngineConfig{IncludePackages: []string{"*api*"}}, "example.com/app/db", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cfg := c.cfg
			e := NewEngine(&cfg)
			if got := e.shouldIncludePackage(c.pkg); got != c.want {
				t.Errorf("shouldIncludePackage(%q) = %v, want %v", c.pkg, got, c.want)
			}
		})
	}
}
