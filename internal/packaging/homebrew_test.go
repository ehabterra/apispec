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

// Package packaging holds tests for the distribution artifacts. There is no
// package code — the formula is data, and these are the only checks that can
// catch it going wrong before a release does.
package packaging

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const (
	formulaPath  = "../../packaging/homebrew/apispec.rb"
	templatePath = "../../packaging/homebrew/apispec.rb.tmpl"
)

// TestHomebrewFormulaMatchesTemplate pins that the readable formula is exactly
// what the template renders.
//
// The release workflow ships the TEMPLATE, filled with checksums of the
// artifacts it just built; the .rb is the copy a human reads and runs
// `brew audit` against. If an edit lands in one and not the other, the file
// people review stops describing the file people install — and the failure is
// invisible until someone's `brew install` fetches a binary whose digest does
// not match.
func TestHomebrewFormulaMatchesTemplate(t *testing.T) {
	formula := readFile(t, formulaPath)
	tmpl := readFile(t, templatePath)

	// Render the template the way release.yml does: substitute placeholders with
	// whatever the formula currently carries. What must match is everything else.
	version := captureOne(t, formula, `version "([^"]+)"`)
	rendered := strings.ReplaceAll(tmpl, "__VERSION__", version)

	shas := regexp.MustCompile(`sha256 "([0-9a-f]{64})"`).FindAllStringSubmatch(formula, -1)
	if len(shas) != 4 {
		t.Fatalf("formula declares %d sha256 values, want 4 (darwin/linux × arm64/amd64)", len(shas))
	}
	for i, ph := range []string{
		"__SHA_DARWIN_ARM64__", "__SHA_DARWIN_AMD64__",
		"__SHA_LINUX_ARM64__", "__SHA_LINUX_AMD64__",
	} {
		rendered = strings.ReplaceAll(rendered, ph, shas[i][1])
	}

	if rendered != formula {
		t.Errorf("packaging/homebrew/apispec.rb is not what apispec.rb.tmpl renders.\n"+
			"Edit the TEMPLATE, then regenerate the .rb — the workflow ships the template, "+
			"so a change made only in the .rb never reaches anyone.\n\n--- rendered\n%s\n--- committed\n%s",
			rendered, formula)
	}
}

// TestHomebrewTemplateHasEveryPlaceholder keeps the workflow's substitutions and
// the template in agreement. A placeholder dropped from the template does not
// fail the release — it silently ships the PREVIOUS version's URL or digest.
func TestHomebrewTemplateHasEveryPlaceholder(t *testing.T) {
	tmpl := readFile(t, templatePath)
	for _, ph := range []string{
		"__VERSION__",
		"__SHA_DARWIN_ARM64__", "__SHA_DARWIN_AMD64__",
		"__SHA_LINUX_ARM64__", "__SHA_LINUX_AMD64__",
	} {
		if !strings.Contains(tmpl, ph) {
			t.Errorf("template is missing %s; the release would ship a stale value there", ph)
		}
	}
	// A literal digest left in the template would survive substitution and be
	// served for a version it was never built from.
	if m := regexp.MustCompile(`sha256 "[0-9a-f]{64}"`).FindString(tmpl); m != "" {
		t.Errorf("template carries a hard-coded %s — it must be a placeholder", m)
	}
}

// TestHomebrewFormulaTargetsThePublishedAssets pins the formula against the
// names the release actually publishes. Homebrew reports a wrong URL only as a
// 404 at install time, on the user's machine.
func TestHomebrewFormulaTargetsThePublishedAssets(t *testing.T) {
	formula := readFile(t, formulaPath)
	for _, asset := range []string{
		"apispec-darwin-arm64", "apispec-darwin-amd64",
		"apispec-linux-arm64", "apispec-linux-amd64",
	} {
		if !strings.Contains(formula, asset) {
			t.Errorf("formula never references %s, which the release publishes", asset)
		}
	}
	if strings.Contains(formula, "windows") {
		t.Error("formula references a windows asset; Homebrew does not install those")
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

func captureOne(t *testing.T, s, pattern string) string {
	t.Helper()
	m := regexp.MustCompile(pattern).FindStringSubmatch(s)
	if len(m) < 2 {
		t.Fatalf("pattern %q did not match", pattern)
	}
	return m[1]
}
