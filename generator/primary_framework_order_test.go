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

package generator

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestPrimaryFrameworkOrderInvariance covers issue #212: the generated spec must
// not depend on which framework the detector happens to see first.
//
// Detection walks files in lexical order and treats the first framework it finds
// as the primary one, so the choice is a property of the *filenames*, not of the
// API. That is only acceptable if being secondary costs a framework nothing —
// which is the property under test here, and which was false twice over: unscoped
// patterns were dropped (#211) and ExternalTypes were dropped (#212), so a
// `git mv` could quietly rewrite the documentation.
//
// The test generates the multi-framework fixture twice from two copies whose
// framework files are renamed to reverse the sort order, and compares the two
// specs in full. Not a snapshot: it asserts A == B, so schema evolution moves
// both sides and only an asymmetry fails.
func TestPrimaryFrameworkOrderInvariance(t *testing.T) {
	const fixture = "secondary_framework_params"
	src := filepath.Join("..", "testdata", fixture)

	// The fixture's own names put chi first (gin secondary); swapping the
	// prefixes puts gin first (chi secondary). Same bytes either way — only the
	// order the detector meets them in changes.
	chiPrimary := copyFixture(t, src, map[string]string{
		"a_admin_chi.go": "a_admin_chi.go",
		"z_api_gin.go":   "z_api_gin.go",
	})
	ginPrimary := copyFixture(t, src, map[string]string{
		"a_admin_chi.go": "z_admin_chi.go",
		"z_api_gin.go":   "a_api_gin.go",
	})

	chiFirst, err := NewGenerator(nil).GenerateFromDirectory(chiPrimary)
	if err != nil {
		t.Fatalf("GenerateFromDirectory(chi first): %v", err)
	}
	ginFirst, err := NewGenerator(nil).GenerateFromDirectory(ginPrimary)
	if err != nil {
		t.Fatalf("GenerateFromDirectory(gin first): %v", err)
	}

	// Sanity: the run really did exercise both orderings, i.e. the fixture still
	// has one file per framework and both frameworks still route. Without this,
	// a fixture that stopped importing chi would make the comparison vacuous.
	for _, want := range []string{"/admin/health", "/items", "/items/status"} {
		if _, ok := chiFirst.Paths[want]; !ok {
			t.Fatalf("fixture no longer documents %s; have %v", want, mapPathKeys(chiFirst.Paths))
		}
	}

	a, err := yaml.Marshal(chiFirst)
	if err != nil {
		t.Fatalf("marshal chi-first spec: %v", err)
	}
	b, err := yaml.Marshal(ginFirst)
	if err != nil {
		t.Fatalf("marshal gin-first spec: %v", err)
	}
	if normalizeFuncLitIDs(string(a)) != normalizeFuncLitIDs(string(b)) {
		t.Errorf("spec depends on which framework sorts first (#212):\n%s",
			firstDiffLines(normalizeFuncLitIDs(string(a)), normalizeFuncLitIDs(string(b))))
	}
}

// normalizeFuncLitIDs strips the file path out of a closure handler's
// operationId (`…FuncLit:/abs/path/file.go:20:25` -> `…FuncLit:<pos>`).
//
// That path is a separate, real defect — the operationId of any closure handler
// carries the *absolute* source path, so the same code documents differently on
// two machines — filed as #216 rather than absorbed here. It has to be
// normalized for this test because renaming a file is exactly what the test
// does, so the id would differ for a reason that has nothing to do with which
// framework is primary. Delete this when #216 is fixed.
func normalizeFuncLitIDs(spec string) string {
	return funcLitPathRE.ReplaceAllString(spec, "FuncLit:<pos>")
}

var funcLitPathRE = regexp.MustCompile(`FuncLit:\S+`)

// copyFixture copies a fixture project into a temp dir, renaming files per the
// given map (unlisted files keep their name). Test writes stay inside
// t.TempDir() — a `go test` run must never dirty the working tree.
func copyFixture(t *testing.T, src string, rename map[string]string) string {
	t.Helper()
	dst := t.TempDir()

	entries, err := os.ReadDir(src)
	if err != nil {
		t.Fatalf("ReadDir(%s): %v", src, err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		// Only the project's own sources and module files; openapi*.yaml and
		// used-config.yaml are gitignored compare artifacts that would change
		// what is being generated.
		if !strings.HasSuffix(name, ".go") && name != "go.mod" && name != "go.sum" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(src, name))
		if err != nil {
			t.Fatalf("ReadFile(%s): %v", name, err)
		}
		out := name
		if renamed, ok := rename[name]; ok {
			out = renamed
		}
		if err := os.WriteFile(filepath.Join(dst, out), data, 0o600); err != nil {
			t.Fatalf("WriteFile(%s): %v", out, err)
		}
	}
	// A rename map naming a file the fixture no longer has means the test is
	// pointing at something stale, and the "invariance" it proves is empty.
	for from := range rename {
		if _, err := os.Stat(filepath.Join(src, from)); err != nil {
			t.Fatalf("rename source %q missing from fixture %s", from, src)
		}
	}
	return dst
}

// firstDiffLines reports the first differing line with a little context, so a
// failure names the drifting field instead of printing two whole specs.
func firstDiffLines(a, b string) string {
	al, bl := strings.Split(a, "\n"), strings.Split(b, "\n")
	for i := 0; i < len(al) || i < len(bl); i++ {
		x, y := "", ""
		if i < len(al) {
			x = al[i]
		}
		if i < len(bl) {
			y = bl[i]
		}
		if x != y {
			var ctx strings.Builder
			for j := max(0, i-3); j < i; j++ {
				ctx.WriteString("  " + al[j] + "\n")
			}
			ctx.WriteString("- chi first: " + x + "\n")
			ctx.WriteString("+ gin first: " + y + "\n")
			return ctx.String()
		}
	}
	return "(no line-level difference)"
}
