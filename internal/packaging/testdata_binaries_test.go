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

package packaging

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// maxTrackedTestdataSize is generous next to a fixture's source and goldens,
// and tiny next to a Go binary (the ones this test was written for were 8–9MB
// each). The largest legitimate tracked fixture file is a metadata golden of
// roughly 390KB.
const maxTrackedTestdataSize = 2 << 20 // 2MB

// TestNoCompiledBinariesUnderTestdata fails when a compiled binary is tracked
// under testdata/.
//
// Building a fixture to check it compiles — `cd testdata/x && go build ./...` —
// drops a binary named after the module beside main.go, and `git add -A` sweeps
// it in. No gitignore pattern can express "a file named like its parent
// directory", so .gitignore carries a hand-maintained list; it had been added
// to five times and still let five binaries totalling ~44MB into the
// repository, because the failure is silent at every step: the build says
// nothing, `git add` says nothing, and the diff just looks large.
//
// This is the check that is not silent. Build fixtures with
// `go build -o /dev/null ./...` and nothing is dropped in the first place.
func TestNoCompiledBinariesUnderTestdata(t *testing.T) {
	root := repoRoot(t)

	out, err := exec.Command("git", "-C", root, "ls-files", "-z", "testdata").Output()
	if err != nil {
		t.Skipf("git ls-files unavailable (%v); this check needs a git checkout", err)
	}

	for _, rel := range strings.Split(strings.TrimRight(string(out), "\x00"), "\x00") {
		if rel == "" {
			continue
		}
		path := filepath.Join(root, rel)
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() {
			continue // deleted in the working tree, or not a plain file
		}

		if isExecutableImage(t, path) {
			t.Errorf("%s is a compiled binary and is tracked in git.\n"+
				"Delete it, add it to .gitignore, and build fixtures with `go build -o /dev/null ./...`.", rel)
			continue
		}
		if info.Size() > maxTrackedTestdataSize {
			t.Errorf("%s is %dKB, over the %dKB limit for a tracked fixture file.\n"+
				"If it is genuinely needed, raise maxTrackedTestdataSize and say why.",
				rel, info.Size()/1024, maxTrackedTestdataSize/1024)
		}
	}
}

// isExecutableImage reports whether the file starts with an ELF, Mach-O or PE
// magic number — the three a `go build` on a supported platform produces.
func isExecutableImage(t *testing.T, path string) bool {
	t.Helper()

	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()

	var head [4]byte
	n, _ := f.Read(head[:])
	if n < 4 {
		return false
	}

	switch {
	case bytes.Equal(head[:], []byte{0x7f, 'E', 'L', 'F'}): // ELF (linux)
		return true
	case bytes.Equal(head[:], []byte{0xcf, 0xfa, 0xed, 0xfe}), // Mach-O 64, little-endian
		bytes.Equal(head[:], []byte{0xfe, 0xed, 0xfa, 0xcf}), // Mach-O 64, big-endian
		bytes.Equal(head[:], []byte{0xca, 0xfe, 0xba, 0xbe}): // Mach-O universal
		return true
	case head[0] == 'M' && head[1] == 'Z': // PE (windows)
		return true
	}
	return false
}

func repoRoot(t *testing.T) string {
	t.Helper()
	abs, err := filepath.Abs("../..")
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	return abs
}
