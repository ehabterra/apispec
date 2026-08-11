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

package engine

import (
	"os"
	"path/filepath"
	"testing"
)

// TestModuleImportPathGrammarForms covers the go.mod forms the hand-rolled parser got
// wrong. This value decides which packages count as the project's (#282), so a
// mis-parse is not cosmetic: an empty result silently reverts classification to
// the path-shape heuristics, and a path with a trailing comment glued on
// matches no package at all, making every import look external.
func TestModuleImportPathGrammarForms(t *testing.T) {
	const want = "github.com/acme/x"

	tests := []struct {
		name    string
		content string
	}{
		{"space separated", "module github.com/acme/x\n\ngo 1.24\n"},
		{"tab separated", "module\tgithub.com/acme/x\n\ngo 1.24\n"},
		{"trailing comment", "module github.com/acme/x // the module\n\ngo 1.24\n"},
		{"quoted path", "module \"github.com/acme/x\"\n\ngo 1.24\n"},
		{"crlf line endings", "module github.com/acme/x\r\n\r\ngo 1.24\r\n"},
		{"require block after", "module github.com/acme/x\n\ngo 1.24\n\nrequire (\n\tgithub.com/other/lib v1.0.0\n)\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(tt.content), 0o600); err != nil {
				t.Fatalf("write go.mod: %v", err)
			}
			e := &Engine{config: &EngineConfig{moduleRoot: dir}}
			if got := e.moduleImportPath(); got != want {
				t.Errorf("moduleImportPath() = %q, want %q", got, want)
			}
		})
	}
}
