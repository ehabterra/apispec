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
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// strictModule is a module with a call graph and no route registration, so a
// run over it has exactly one strict finding: nothing matched (category paths).
func strictModule(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"go.mod":  "module example.com/strictui\n\ngo 1.22\n",
		"main.go": "package main\n\nimport \"fmt\"\n\nfunc main() { greet() }\n\nfunc greet() { fmt.Println(\"hi\") }\n",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func postGenerate(t *testing.T, s *UIServer, req GenerateRequest) (int, GenerateResponse, string) {
	t.Helper()
	body, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	s.handleGenerate(rec, httptest.NewRequest(http.MethodPost, "/api/generate", bytes.NewReader(body)))
	var resp GenerateResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	return rec.Code, resp, rec.Body.String()
}

// The UI's --strict: findings are reported on every run, and a run is marked
// failed only when a finding falls in a category the request gated on. It was
// unreachable from the UI before — the CLI was the only way to see findings.
func TestGenerateReportsStrictFindings(t *testing.T) {
	dir := strictModule(t)

	for _, tc := range []struct {
		name       string
		strict     []string
		wantFailed bool
	}{
		{"no gate: findings reported, run not failed", nil, false},
		{"gated on the finding's category", []string{"paths"}, true},
		{"gated on another category", []string{"security"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &UIServer{cfg: &ServerConfig{}, inputDir: dir}
			code, resp, raw := postGenerate(t, s, GenerateRequest{Dir: dir, Strict: tc.strict})
			if code != http.StatusOK {
				t.Fatalf("status %d: %s", code, raw)
			}
			found := false
			for _, f := range resp.StrictFindings {
				if f.Category == "paths" {
					found = true
				}
			}
			if !found {
				t.Errorf("no paths finding for a module with no routes: %+v", resp.StrictFindings)
			}
			if resp.StrictFailed != tc.wantFailed {
				t.Errorf("strictFailed = %v, want %v", resp.StrictFailed, tc.wantFailed)
			}
		})
	}
}

// A misspelt category is refused, as the CLI refuses it: a gate that ignored an
// unknown category would pass runs it was meant to fail.
func TestGenerateRejectsUnknownStrictCategory(t *testing.T) {
	dir := strictModule(t)
	s := &UIServer{cfg: &ServerConfig{}, inputDir: dir}
	code, _, raw := postGenerate(t, s, GenerateRequest{Dir: dir, Strict: []string{"secruity"}})
	if code != http.StatusBadRequest || !strings.Contains(raw, "strict") {
		t.Errorf("got %d %s, want 400 naming strict", code, raw)
	}
}

// An unset analysis switch keeps the value the UI always ran with (on); only
// an explicit false turns one off.
func TestAnalysisSwitchDefaultsOn(t *testing.T) {
	f := false
	tr := true
	if !on(nil) || !on(&tr) || on(&f) {
		t.Error("on(nil) and on(&true) must be true, on(&false) false")
	}
}
