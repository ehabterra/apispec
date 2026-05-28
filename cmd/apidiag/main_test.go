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
