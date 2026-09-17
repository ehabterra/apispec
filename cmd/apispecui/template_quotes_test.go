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
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestUIScriptsHaveNoEscapedDoubleQuotes guards the whole Configure tab against
// one character.
//
// The UI's markup is htm tagged template literals, and inside a template literal
// `\"` is NOT an escape — JavaScript resolves it to a plain `"`. So this, in a
// help string:
//
//	help="… mux.HandleFunc(\"GET api.example.com/items\", h) serves /items …"
//
// reaches htm as an attribute value that ENDS at the inner quote. The rest of
// the sentence is then parsed as markup, where every bare `/` (`/items`,
// `/api.example.com/items`) hits htm's self-closing branch and pops the element
// stack. The damage surfaces far away, at the template's final `<//>`, as
//
//	Uncaught TypeError: u.push is not a function
//
// and the ENTIRE Configure tab renders nothing — one character in one help
// string, no mention of that string in the error, and the section it broke is
// the last place anyone would look.
//
// The rule is a blanket one because no file needs the sequence: prose in these
// templates quotes code with single quotes (`mux.Handle('/x', h)`), and a
// genuine double quote can be written `&quot;`. A blanket check also costs
// nothing to understand, which matters more here than precision — the failure it
// prevents is invisible to every Go test in the repo.
func TestUIScriptsHaveNoEscapedDoubleQuotes(t *testing.T) {
	roots := []string{"assets/js", filepath.Join("assets", "js", "components")}
	checked := 0
	for _, dir := range roots {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("read %s: %v", dir, err)
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".js") {
				continue
			}
			path := filepath.Join(dir, e.Name())
			b, err := os.ReadFile(filepath.Clean(path))
			if err != nil {
				t.Fatalf("read %s: %v", path, err)
			}
			checked++
			for i, line := range strings.Split(string(b), "\n") {
				if strings.Contains(line, `\"`) {
					t.Errorf(`%s:%d contains \" — inside a template literal that is a plain "`+
						" and ends the attribute value early, which breaks htm's parse of the whole"+
						" template. Use single quotes, or &quot;.\n\t%s", path, i+1, trimForError(line))
				}
			}
		}
	}
	// A path typo would make this test pass by reading nothing.
	if checked == 0 {
		t.Fatal("scanned no .js files; the asset layout moved")
	}
}

func trimForError(line string) string {
	line = strings.TrimSpace(line)
	if len(line) > 160 {
		return line[:160] + "…"
	}
	return line
}
