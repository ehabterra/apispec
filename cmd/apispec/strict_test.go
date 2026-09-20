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

package main

import (
	"bytes"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ehabterra/apispec/internal/engine"
)

func TestStrictFlagParsing(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		enabled bool
		want    []engine.StrictCategory
		wantErr bool
	}{
		{name: "absent", args: []string{}, enabled: false},
		{name: "bare flag gates on everything", args: []string{"--strict"}, enabled: true, want: engine.StrictCategories()},
		{name: "explicit true", args: []string{"--strict=true"}, enabled: true, want: engine.StrictCategories()},
		// So a wrapper can neutralise a --strict baked into a Makefile.
		{name: "explicit false", args: []string{"--strict=false"}, enabled: false},
		{
			name:    "a category list",
			args:    []string{"--strict=security,truncation"},
			enabled: true,
			want:    []engine.StrictCategory{engine.StrictSecurity, engine.StrictTruncation},
		},
		{name: "an unknown category is a usage error", args: []string{"--strict=nonsense"}, wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			config, err := parseFlags(tc.args)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected an error")
				}
				return
			}
			if err != nil {
				t.Fatalf("parseFlags(%v): %v", tc.args, err)
			}
			if config.Strict.enabled != tc.enabled {
				t.Fatalf("enabled = %v, want %v", config.Strict.enabled, tc.enabled)
			}
			if len(config.Strict.categories) != len(tc.want) {
				t.Fatalf("categories = %v, want %v", config.Strict.categories, tc.want)
			}
			for i := range tc.want {
				if config.Strict.categories[i] != tc.want[i] {
					t.Fatalf("categories = %v, want %v", config.Strict.categories, tc.want)
				}
			}
		})
	}
}

// TestStrictFlagDoesNotEatTheDirectory is the reason strictFlag implements
// IsBoolFlag. Without it the flag package takes the NEXT argument as the
// value, so `apispec --strict ./api` would parse as --strict="./api" — which
// fails as an unknown category if you are lucky, and silently analyses the
// default directory if you are not.
func TestStrictFlagDoesNotEatTheDirectory(t *testing.T) {
	config, err := parseFlags([]string{"--strict", "./api"})
	if err != nil {
		t.Fatalf("parseFlags: %v", err)
	}
	if !config.Strict.enabled {
		t.Error("--strict was not enabled")
	}
	if config.InputDir != "./api" {
		t.Errorf("InputDir = %q, want %q — the positional argument was consumed by --strict", config.InputDir, "./api")
	}
}

func TestStrictExit(t *testing.T) {
	dirty := &engine.Engine{}
	// An engine with nothing recorded has nothing to report, which is what a
	// passing run looks like.
	if code := strictExit(&CLIConfig{}, dirty); code != 0 {
		t.Errorf("a run without --strict exited %d, want 0", code)
	}
	if code := strictExit(&CLIConfig{Strict: strictFlag{enabled: true, categories: engine.StrictCategories()}}, dirty); code != 0 {
		t.Errorf("a clean strict run exited %d, want 0", code)
	}
}

// TestStrictExitOnARealRun walks the whole path the flag exists for: generate
// over a module that has code to walk and no routes in it, and check that the
// gate fails, says why, and is scoped to the category that covers it.
//
// End to end rather than against a hand-built engine, because the value of
// --strict is entirely in whether the conditions the engine records reach the
// exit code — a unit test of strictExit alone would pass with the wiring cut.
func TestStrictExitOnARealRun(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", "module example.com/norouter\n\ngo 1.24\n")
	write("main.go", `package main

import "fmt"

func greet(name string) string { return fmt.Sprintf("hello %s", name) }

func main() { fmt.Println(greet("world")) }
`)

	_, genEngine, err := runGeneration(&CLIConfig{
		InputDir:       dir,
		OutputFile:     filepath.Join(dir, "openapi.json"),
		OpenAPIVersion: "3.1.1",
	})
	if err != nil {
		t.Fatalf("generation failed: %v", err)
	}

	// Without --strict this run exits 0, which is the whole complaint: the
	// document is empty and nothing says so in the exit code.
	if code := strictExit(&CLIConfig{}, genEngine); code != 0 {
		t.Errorf("a run without --strict exited %d, want 0", code)
	}

	config := &CLIConfig{Strict: strictFlag{enabled: true, categories: engine.StrictCategories()}}
	var buf bytes.Buffer
	out, flags := log.Writer(), log.Flags()
	log.SetOutput(&buf)
	log.SetFlags(0)
	t.Cleanup(func() {
		log.SetOutput(out)
		log.SetFlags(flags)
	})

	if code := strictExit(config, genEngine); code != strictExitCode {
		t.Fatalf("exit code = %d, want %d\nlog: %s", code, strictExitCode, buf.String())
	}
	got := buf.String()
	for _, want := range []string{"[strict]", "quality gate(s) failed", "[paths]", "0 paths documented"} {
		if !strings.Contains(got, want) {
			t.Errorf("report does not mention %q\ngot: %s", want, got)
		}
	}

	// And the same run passes a gate that does not cover it — the reason the
	// categories exist at all.
	config.Strict.categories = []engine.StrictCategory{engine.StrictSecurity}
	if code := strictExit(config, genEngine); code != 0 {
		t.Errorf("--strict=security failed on a paths finding (exit %d)", code)
	}
}
