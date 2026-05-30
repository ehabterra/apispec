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

package main

import (
	"bytes"
	"flag"
	"os"
	"strings"
	"testing"
)

func TestDetectVersionInfo(t *testing.T) {
	originalVersion := Version
	originalCommit := Commit
	originalBuildDate := BuildDate
	originalGoVersion := GoVersion

	Version = "0.0.1"
	Commit = "unknown"
	BuildDate = "unknown"
	GoVersion = "unknown"

	Version = "1.0.0"
	detectVersionInfo()
	if Version != "1.0.0" {
		t.Errorf("Expected version to remain 1.0.0, got %s", Version)
	}

	Version = "0.0.1"
	detectVersionInfo()

	if GoVersion == "unknown" {
		t.Error("Expected GoVersion to be set from build info")
	}

	Version = originalVersion
	Commit = originalCommit
	BuildDate = originalBuildDate
	GoVersion = originalGoVersion
}

func TestPrintVersion(t *testing.T) {
	var buf bytes.Buffer
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	defer func() { os.Stdout = old }()

	printVersion()
	if err := w.Close(); err != nil {
		t.Fatalf("Failed to close stdout: %v", err)
	}
	if _, err := buf.ReadFrom(r); err != nil {
		t.Fatalf("Failed to read from stdout: %v", err)
	}

	output := buf.String()
	for _, want := range []string{"apidiag version:", "Commit:", "Build date:", "Go version:"} {
		if !strings.Contains(output, want) {
			t.Errorf("Expected output to contain %q, got %q", want, output)
		}
	}
}

func TestVersionDetectionWithBuildInfo(t *testing.T) {
	detectVersionInfo()
	if GoVersion == "unknown" {
		t.Error("Expected GoVersion to be set from build info")
	}
}

// withParsedFlags runs parseFlags() with a fresh flag set and the given args,
// restoring global flag/os.Args state afterward.
func withParsedFlags(args []string) *cliConfig {
	oldArgs := os.Args
	oldCmd := flag.CommandLine
	defer func() {
		os.Args = oldArgs
		flag.CommandLine = oldCmd
	}()
	flag.CommandLine = flag.NewFlagSet("apidiag", flag.ContinueOnError)
	os.Args = append([]string{"apidiag"}, args...)
	return parseFlags()
}

func TestParseFlags_Defaults(t *testing.T) {
	c := withParsedFlags(nil)
	if c.ShowVersion {
		t.Error("ShowVersion should default false")
	}
	if c.srv.Port != 8080 || c.srv.Host != "localhost" || c.srv.InputDir != "." {
		t.Errorf("unexpected defaults: %+v", c.srv)
	}
	if c.srv.PageSize != 100 || c.srv.MaxDepth != 3 {
		t.Errorf("unexpected pagesize/depth: %d/%d", c.srv.PageSize, c.srv.MaxDepth)
	}
	if !c.srv.EnableCORS {
		t.Error("CORS should default true")
	}
	if c.srv.DiagramType != "call-graph" {
		t.Errorf("default diagram type = %q", c.srv.DiagramType)
	}
}

func TestParseFlags_ClampingAndValidation(t *testing.T) {
	cases := []struct {
		name     string
		args     []string
		pageSize int
		maxDepth int
		diagram  string
	}{
		{"pagesize too low", []string{"--page-size", "5"}, 10, 3, "call-graph"},
		{"pagesize too high", []string{"--page-size", "5000"}, 1000, 3, "call-graph"},
		{"depth too low", []string{"--max-depth", "0"}, 100, 1, "call-graph"},
		{"depth too high", []string{"--max-depth", "99"}, 100, 10, "call-graph"},
		{"bad diagram type", []string{"--diagram-type", "bogus"}, 100, 3, "call-graph"},
		{"tracker-tree", []string{"--diagram-type", "tracker-tree"}, 100, 3, "tracker-tree"},
		{"shorthand dt", []string{"-dt", "tracker-tree"}, 100, 3, "tracker-tree"},
		{"in-range values kept", []string{"--page-size", "50", "--max-depth", "5"}, 50, 5, "call-graph"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := withParsedFlags(tc.args)
			if c.srv.PageSize != tc.pageSize {
				t.Errorf("PageSize = %d, want %d", c.srv.PageSize, tc.pageSize)
			}
			if c.srv.MaxDepth != tc.maxDepth {
				t.Errorf("MaxDepth = %d, want %d", c.srv.MaxDepth, tc.maxDepth)
			}
			if c.srv.DiagramType != tc.diagram {
				t.Errorf("DiagramType = %q, want %q", c.srv.DiagramType, tc.diagram)
			}
		})
	}
}

func TestParseFlags_ServerOptions(t *testing.T) {
	c := withParsedFlags([]string{
		"--port", "9090", "--host", "0.0.0.0", "--dir", "./proj",
		"--cors=false", "--verbose", "--afd", "--aifp", "--aet", "--aem",
	})
	if c.srv.Port != 9090 || c.srv.Host != "0.0.0.0" || c.srv.InputDir != "./proj" {
		t.Errorf("server opts not applied: %+v", c.srv)
	}
	if c.srv.EnableCORS {
		t.Error("--cors=false should disable CORS")
	}
	if !c.srv.Verbose || !c.srv.AnalyzeFrameworkDependencies || !c.srv.AutoIncludeFrameworkPackages ||
		!c.srv.AutoExcludeTests || !c.srv.AutoExcludeMocks {
		t.Errorf("bool flags not applied: %+v", c.srv)
	}
}

func TestParseFlags_VersionShorthand(t *testing.T) {
	if c := withParsedFlags([]string{"-V"}); !c.ShowVersion {
		t.Error("-V should set ShowVersion")
	}
	if c := withParsedFlags([]string{"--version"}); !c.ShowVersion {
		t.Error("--version should set ShowVersion")
	}
}
