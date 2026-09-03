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
	"encoding/json"
	"go/build"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// sourceGuardModule lays out a module to serve source from, a file inside it,
// and a secret outside it that an in-module symlink points at.
func sourceGuardModule(t *testing.T) (dir, secret string) {
	t.Helper()
	root := t.TempDir()

	dir = filepath.Join(root, "module")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module guarded\n\ngo 1.26\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	secret = filepath.Join(root, "secret", "id_rsa")
	if err := os.MkdirAll(filepath.Dir(secret), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(secret, []byte("-----BEGIN PRIVATE KEY-----\nSTOLEN\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir, secret
}

func getInsightSource(t *testing.T, s *UIServer, pos string) (int, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	s.handleInsightSource(rec, httptest.NewRequest(http.MethodGet, "/api/insight/source?pos="+pos, nil))
	return rec.Code, rec.Body.String()
}

// A symlink planted inside the analyzed module must not serve what it points
// at. The guard used to compare paths lexically, so `<module>/leak.go` for a
// link to ~/.ssh/id_rsa produced rel == "leak.go" — no leading "..", allowed —
// and the endpoint returned the key's contents while reporting the in-module
// path as `file` (#424).
func TestInsightSourceRejectsSymlinkEscape(t *testing.T) {
	dir, secret := sourceGuardModule(t)
	link := filepath.Join(dir, "leak.go")
	if err := os.Symlink(secret, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	s := &UIServer{inputDir: dir}

	code, body := getInsightSource(t, s, link+":1")
	if code == http.StatusOK {
		t.Errorf("symlink escape served with 200; body: %s", body)
	}
	if strings.Contains(body, "STOLEN") {
		t.Errorf("response leaked the linked file's contents: %s", body)
	}
	if code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", code, http.StatusForbidden)
	}
}

// A real file inside the module still works — the guard must not be so tight
// that the endpoint stops doing its job.
//
// This also covers the module sitting under a SYMLINKED path, which is the
// regression resolving the roots prevents: on macOS t.TempDir() lives below
// /var, itself a link to /private/var, so the requested file resolves to
// /private/var/... while the root reads /var/... . Comparing those without
// resolving the root puts ".." in the relative path and refuses the module its
// own source. Verified by removing the root resolution: this test fails 403.
func TestInsightSourceServesInModuleFile(t *testing.T) {
	dir, _ := sourceGuardModule(t)
	s := &UIServer{inputDir: dir}

	code, body := getInsightSource(t, s, filepath.Join(dir, "main.go")+":1")
	if code != http.StatusOK {
		t.Fatalf("in-module file rejected: status %d, body %s", code, body)
	}
	var got struct {
		File string `json:"file"`
		Code string `json:"code"`
	}
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatalf("bad JSON: %v (%s)", err, body)
	}
	if !strings.Contains(got.Code, "package main") {
		t.Errorf("code = %q, want the file's source", got.Code)
	}
}

// A path outside the module is still refused outright.
func TestInsightSourceRejectsOutsidePath(t *testing.T) {
	dir, secret := sourceGuardModule(t)
	s := &UIServer{inputDir: dir}

	code, body := getInsightSource(t, s, secret+":1")
	if code != http.StatusForbidden {
		t.Errorf("status = %d, want %d (body %s)", code, http.StatusForbidden, body)
	}
	if strings.Contains(body, "STOLEN") {
		t.Errorf("response leaked contents: %s", body)
	}
}

// A symlink is not automatically suspect: one pointing at a file inside the
// module resolves to a path still under the module, and must be served. The
// reported path is the resolved one, since that is what was read.
func TestInsightSourceAllowsSymlinkWithinModule(t *testing.T) {
	dir, _ := sourceGuardModule(t)
	link := filepath.Join(dir, "alias.go")
	if err := os.Symlink(filepath.Join(dir, "main.go"), link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	s := &UIServer{inputDir: dir}

	code, body := getInsightSource(t, s, link+":1")
	if code != http.StatusOK {
		t.Fatalf("in-module symlink rejected: status %d, body %s", code, body)
	}
	var got struct {
		File string `json:"file"`
		Code string `json:"code"`
	}
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatalf("bad JSON: %v (%s)", err, body)
	}
	if !strings.Contains(got.Code, "package main") {
		t.Errorf("code = %q, want the target's source", got.Code)
	}
	if strings.HasSuffix(got.File, "alias.go") {
		t.Errorf("file = %q, want the resolved target rather than the link", got.File)
	}
}

// A relative escape is refused: filepath.Abs cleans the "..", so this lands
// outside the module and never reaches the read.
func TestInsightSourceRejectsDotDotEscape(t *testing.T) {
	dir, secret := sourceGuardModule(t)
	s := &UIServer{inputDir: dir}

	pos := filepath.Join(dir, "..", "secret", "id_rsa") + ":1"
	code, body := getInsightSource(t, s, pos)
	if code != http.StatusForbidden {
		t.Errorf("status = %d, want %d (body %s)", code, http.StatusForbidden, body)
	}
	if strings.Contains(body, "STOLEN") {
		t.Errorf("response leaked contents: %s", body)
	}
	_ = secret
}

// A path that does not exist fails closed, and says so rather than claiming the
// file lives outside the module: EvalSymlinks cannot resolve it, and the old
// code's lexical fallback is exactly what made the escape possible.
func TestInsightSourceMissingFileFailsClosed(t *testing.T) {
	dir, _ := sourceGuardModule(t)
	s := &UIServer{inputDir: dir}

	code, body := getInsightSource(t, s, filepath.Join(dir, "absent.go")+":1")
	if code != http.StatusNotFound {
		t.Errorf("status = %d, want %d (body %s)", code, http.StatusNotFound, body)
	}
}

// Stdlib source is still served: GOROOT is one of the allowed roots, and it is
// commonly reached through a symlinked toolchain path.
func TestInsightSourceServesGOROOT(t *testing.T) {
	stdlib := filepath.Join(build.Default.GOROOT, "src", "net", "http", "server.go")
	if _, err := os.Stat(stdlib); err != nil {
		t.Skipf("stdlib source not present: %v", err)
	}
	dir, _ := sourceGuardModule(t)
	s := &UIServer{inputDir: dir}

	code, body := getInsightSource(t, s, stdlib+":1")
	if code != http.StatusOK {
		t.Fatalf("stdlib source rejected: status %d, body %s", code, body)
	}
}

// A position with no line is a bad request, not a read.
func TestInsightSourceRejectsPositionWithoutLine(t *testing.T) {
	dir, _ := sourceGuardModule(t)
	s := &UIServer{inputDir: dir}

	for _, pos := range []string{"", filepath.Join(dir, "main.go"), filepath.Join(dir, "main.go") + ":0"} {
		if code, body := getInsightSource(t, s, pos); code != http.StatusBadRequest {
			t.Errorf("pos %q: status = %d, want %d (body %s)", pos, code, http.StatusBadRequest, body)
		}
	}
}
