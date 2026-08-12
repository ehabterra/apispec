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
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// buildUI builds the command under test once and returns its path.
func buildUI(t *testing.T, version string) string {
	t.Helper()

	bin := filepath.Join(t.TempDir(), "apispecui")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}

	build := exec.Command("go", "build",
		"-ldflags", "-X main.Version="+version,
		"-o", bin, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	return bin
}

// TestVersionFlag runs the real binary the way Homebrew's formula does: in a
// directory with no Go project, asserting it prints the injected version and
// exits instead of starting a server.
//
// The formula's test block is `assert_match version.to_s, shell_output(
// "#{bin}/apispecui --version")`. If this path ever regressed — a server bound
// before the flag was read, a non-zero exit, a version the ldflags did not
// reach — the failure would surface as a broken `brew install` on a user's
// machine, which nothing else here would catch.
func TestVersionFlag(t *testing.T) {
	const version = "9.9.9-test"
	bin := buildUI(t, version)

	for _, flag := range []string{"--version", "-V"} {
		t.Run(flag, func(t *testing.T) {
			// An empty working directory: --version must not depend on there
			// being a project to analyse.
			cmd := exec.Command(bin, flag)
			cmd.Dir = t.TempDir()

			done := make(chan struct{})
			var out []byte
			var err error
			go func() {
				out, err = cmd.CombinedOutput()
				close(done)
			}()

			select {
			case <-done:
			case <-time.After(30 * time.Second):
				_ = cmd.Process.Kill()
				t.Fatalf("%s did not exit: it started serving instead of printing the version", flag)
			}

			if err != nil {
				t.Fatalf("%s exited with %v\n%s", flag, err, out)
			}

			got := string(out)
			if !strings.Contains(got, version) {
				t.Errorf("%s did not print the injected version %q; Homebrew's formula asserts on this.\n%s", flag, version, got)
			}
			if !strings.Contains(got, "apispecui version:") {
				t.Errorf("%s output does not identify the tool:\n%s", flag, got)
			}
			// The server prints nothing on start, so the absence of a listen
			// error is not proof by itself — but a bound port would have kept
			// the process alive and tripped the timeout above.
			if strings.Contains(got, "server failed") || strings.Contains(got, "listen") {
				t.Errorf("%s appears to have started the server:\n%s", flag, got)
			}
		})
	}
}
