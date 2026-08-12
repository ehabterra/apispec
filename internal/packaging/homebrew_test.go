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

const packagingDir = "../../packaging/homebrew"

// formulae is every tool the tap publishes. Adding a binary to the release
// without adding it here means nobody can `brew install` it, which is a
// silence rather than a failure — hence the list, and hence
// TestEveryTemplateIsCovered below reading the directory rather than trusting
// it.
var formulae = []struct {
	tool  string
	asset string // release asset prefix, e.g. apispec-darwin-arm64
}{
	{tool: "apispec", asset: "apispec"},
	{tool: "apispecui", asset: "apispecui"},
}

func templatePathFor(tool string) string { return filepath.Join(packagingDir, tool+".rb.tmpl") }
func formulaPathFor(tool string) string  { return filepath.Join(packagingDir, tool+".rb") }

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
	for _, f := range formulae {
		t.Run(f.tool, func(t *testing.T) {
			// A rendered .rb only exists once a release has published that
			// tool's artifacts — its digests cannot be invented, and a wrong
			// one is a failed `brew install`. Until then the template is the
			// whole source of truth and there is nothing to drift against.
			if _, err := os.Stat(formulaPathFor(f.tool)); os.IsNotExist(err) {
				t.Skipf("no rendered %s.rb yet: the first release publishing %s artifacts creates it", f.tool, f.asset)
			}
			assertFormulaMatchesTemplate(t, f.tool)
		})
	}
}

func assertFormulaMatchesTemplate(t *testing.T, tool string) {
	t.Helper()
	formula := readFile(t, formulaPathFor(tool))
	tmpl := readFile(t, templatePathFor(tool))

	// Render the template the way release.yml does: substitute placeholders with
	// whatever the formula currently carries. What must match is everything else.
	//
	// The version is read out of a URL because the formula deliberately has no
	// `version` stanza: Homebrew scans it from the download path, and declaring
	// it as well is a `brew audit --strict` failure.
	version := captureOne(t, formula, `releases/download/v([^/]+)/`)
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
		t.Errorf("packaging/homebrew/"+tool+".rb is not what "+tool+".rb.tmpl renders.\n"+
			"Edit the TEMPLATE, then regenerate the .rb — the workflow ships the template, "+
			"so a change made only in the .rb never reaches anyone.\n\n--- rendered\n%s\n--- committed\n%s",
			rendered, formula)
	}
}

// TestHomebrewTemplateHasEveryPlaceholder keeps the workflow's substitutions and
// the template in agreement. A placeholder dropped from the template does not
// fail the release — it silently ships the PREVIOUS version's URL or digest.
func TestHomebrewTemplateHasEveryPlaceholder(t *testing.T) {
	for _, f := range formulae {
		t.Run(f.tool, func(t *testing.T) { assertTemplatePlaceholders(t, f.tool) })
	}
}

func assertTemplatePlaceholders(t *testing.T, tool string) {
	t.Helper()
	tmpl := readFile(t, templatePathFor(tool))
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
	for _, f := range formulae {
		t.Run(f.tool, func(t *testing.T) {
			// Checked on the TEMPLATE: it is what the release ships, and it
			// exists for a tool that has not been published yet.
			assertTargetsPublishedAssets(t, templatePathFor(f.tool), f.asset)
		})
	}
}

func assertTargetsPublishedAssets(t *testing.T, path, assetPrefix string) {
	t.Helper()
	formula := readFile(t, path)
	for _, asset := range []string{
		assetPrefix + "-darwin-arm64", assetPrefix + "-darwin-amd64",
		assetPrefix + "-linux-arm64", assetPrefix + "-linux-amd64",
	} {
		if !strings.Contains(formula, asset) {
			t.Errorf("formula never references %s, which the release publishes", asset)
		}
	}
	if strings.Contains(formula, "windows") {
		t.Error("formula references a windows asset; Homebrew does not install those")
	}

	// `brew audit --strict` rejects an explicit version when the URL already
	// carries one, and every URL here does.
	if regexp.MustCompile(`(?m)^\s*version "`).MatchString(formula) {
		t.Error("formula declares a version stanza; it is redundant with the version " +
			"scanned from the download URL and fails brew audit --strict")
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

// tapToolList returns the tools the release workflow's tap step iterates,
// padded with spaces so a lookup cannot match a prefix ("apispec" inside
// "apispecui").
func tapToolList(t *testing.T, workflow string) string {
	t.Helper()
	m := regexp.MustCompile(`for tool in ([^;]+);`).FindStringSubmatch(workflow)
	if m == nil {
		t.Fatal("release.yml has no `for tool in ...;` loop in the tap step; the rendering shape changed")
	}
	if !strings.Contains(workflow, `packaging/homebrew/$tool.rb.tmpl`) {
		t.Error("release.yml's tap loop does not render packaging/homebrew/$tool.rb.tmpl")
	}
	return " " + strings.TrimSpace(m[1]) + " "
}

// TestEveryTemplateIsCovered reads the packaging directory rather than trusting
// the list above. A template added without a formulae entry would be tested by
// nothing, and a formulae entry with no template would be a release step that
// cannot run — both fail here rather than at release time.
func TestEveryTemplateIsCovered(t *testing.T) {
	entries, err := os.ReadDir(packagingDir)
	if err != nil {
		t.Fatalf("read %s: %v", packagingDir, err)
	}

	onDisk := map[string]bool{}
	for _, e := range entries {
		if name, ok := strings.CutSuffix(e.Name(), ".rb.tmpl"); ok {
			onDisk[name] = true
		}
	}

	listed := map[string]bool{}
	for _, f := range formulae {
		listed[f.tool] = true
		if !onDisk[f.tool] {
			t.Errorf("formulae lists %q but %s does not exist; the release step would fail", f.tool, templatePathFor(f.tool))
		}
	}
	for tool := range onDisk {
		if !listed[tool] {
			t.Errorf("%s exists but %q is not in formulae, so nothing checks it", templatePathFor(tool), tool)
		}
	}
}

// TestReleaseWorkflowPublishesEveryFormulaAsset ties the formulae to the
// workflow that has to produce their downloads. A formula whose asset the
// release never uploads installs as a 404 on a user's machine, and nothing
// before that point notices.
func TestReleaseWorkflowPublishesEveryFormulaAsset(t *testing.T) {
	workflow := readFile(t, "../../.github/workflows/release.yml")

	for _, f := range formulae {
		t.Run(f.tool, func(t *testing.T) {
			// The four Homebrew platforms; windows is published but never
			// referenced by a formula.
			for _, suffix := range []string{"darwin-arm64", "darwin-amd64", "linux-arm64", "linux-amd64"} {
				asset := f.asset + "-" + suffix
				if !strings.Contains(workflow, asset) {
					t.Errorf("release.yml never publishes %s, which %s.rb.tmpl downloads", asset, f.tool)
				}
			}
			// The tap step renders every template in a loop, so what must
			// name the tool is the loop's list, not a literal path.
			if !strings.Contains(tapToolList(t, workflow), " "+f.tool+" ") {
				t.Errorf("release.yml's tap loop does not cover %q, so the tap keeps the previous version of it", f.tool)
			}
		})
	}
}
