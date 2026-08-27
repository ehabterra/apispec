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
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
)

// TestClassifySkip covers the reduction of a package's load errors to one kind
// and one reason (issue #237).
//
// The multi-error case is the reason this function exists rather than reading
// errs[0]: a package that fails to parse reports the go command's `# pkg` blob
// AND the parser's own message, and neither one alone is a usable report — the
// blob leads with a line that only repeats the package name, and the parser's
// message carries no position.
func TestClassifySkip(t *testing.T) {
	cases := []struct {
		name       string
		errs       []packages.Error
		wantKind   string
		wantReason string
	}{
		{
			name: "parse error, as go/packages really reports it",
			errs: []packages.Error{
				{Kind: packages.ListError, Msg: "# example.com/x/broken\nbroken/broken.go:3:1: syntax error: non-declaration statement outside function body"},
				{Kind: packages.ParseError, Msg: "expected declaration, found ','"},
			},
			wantKind: skipParse,
			// The positioned message, with the driver's header line dropped.
			wantReason: "broken/broken.go:3:1: syntax error: non-declaration statement outside function body",
		},
		{
			name:       "type error",
			errs:       []packages.Error{{Kind: packages.TypeError, Msg: "undefined: broken.Helper"}},
			wantKind:   skipType,
			wantReason: "undefined: broken.Helper",
		},
		{
			// A parse error anywhere outranks a type error: the type errors are
			// its consequence, and reporting them sends the user after the wrong
			// file.
			name: "parse outranks type regardless of order",
			errs: []packages.Error{
				{Kind: packages.TypeError, Msg: "undefined: Foo"},
				{Kind: packages.ParseError, Msg: "a.go:2:1: expected declaration"},
			},
			wantKind:   skipParse,
			wantReason: "a.go:2:1: expected declaration",
		},
		{
			name:       "list error only",
			errs:       []packages.Error{{Kind: packages.ListError, Msg: "no Go files in /x"}},
			wantKind:   skipLoad,
			wantReason: "no Go files in /x",
		},
		{
			// Nothing names a file, so the first error stands rather than the
			// report having no reason at all.
			name: "no positioned message falls back to the first",
			errs: []packages.Error{
				{Kind: packages.TypeError, Msg: "could not import x"},
				{Kind: packages.TypeError, Msg: "undefined: y"},
			},
			wantKind:   skipType,
			wantReason: "could not import x",
		},
		{name: "no errors", errs: nil, wantKind: skipLoad, wantReason: ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			kind, reason := classifySkip(tc.errs)
			if kind != tc.wantKind {
				t.Errorf("kind = %q, want %q", kind, tc.wantKind)
			}
			if reason != tc.wantReason {
				t.Errorf("reason  = %q\nwant    = %q", reason, tc.wantReason)
			}
		})
	}
}

// TestSkippedPackagesReportsParseError is the acceptance case for #237: a
// project with a syntax error must say so.
//
// It builds the module in a temp dir rather than under testdata/ on purpose —
// an unparseable .go file checked into the repo would break gofmt, vet and the
// fixture suites, which is exactly why this shape had no coverage.
//
// The importer is asserted alongside the broken package because that is what
// made the original report expensive: gitea's router package was two hops from
// the syntax error, so the whole route tree vanished with nothing naming a
// cause.
func TestSkippedPackagesReportsParseError(t *testing.T) {
	dir := t.TempDir()
	write := func(rel, content string) {
		t.Helper()
		path := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}

	write("go.mod", "module example.com/parsebug\n\ngo 1.21\n")
	// A stray comma outside a declaration: the smallest thing the parser
	// rejects while the file still looks like Go.
	write("broken/broken.go", "package broken\n\n,\n\nfunc Helper() string { return \"x\" }\n")
	write("router/router.go", "package router\n\nimport \"example.com/parsebug/broken\"\n\nfunc Name() string { return broken.Helper() }\n")
	write("main.go", `package main

import (
	"net/http"

	"example.com/parsebug/router"
)

func main() {
	http.HandleFunc("/users", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(router.Name()))
	})
	_ = http.ListenAndServe(":8080", nil)
}
`)

	e := NewEngine(&EngineConfig{InputDir: dir})
	if _, err := e.GenerateOpenAPI(); err != nil {
		// The run must still SUCCEED — the point is that it reports, not that
		// it refuses to generate what it could read.
		t.Fatalf("generation failed: %v", err)
	}

	skipped := e.SkippedPackages()
	if len(skipped) == 0 {
		t.Fatal("no packages reported as skipped — a project that does not build generated a thin spec silently (#237)")
	}

	byPkg := make(map[string]SkippedPackage, len(skipped))
	for _, s := range skipped {
		byPkg[s.Package] = s
	}

	broken, ok := byPkg["example.com/parsebug/broken"]
	if !ok {
		t.Fatalf("the unparseable package is not reported; got %v", skipped)
	}
	if broken.Kind != skipParse {
		t.Errorf("kind = %q, want %q — a syntax error must not be reported as a type error", broken.Kind, skipParse)
	}
	if !strings.Contains(broken.Reason, "broken.go:") {
		t.Errorf("reason %q does not name the file and line the parser rejected", broken.Reason)
	}

	// The cascade: a package is unusable because of a syntax error two hops
	// away, and must not disappear without a word.
	if imp, ok := byPkg["example.com/parsebug/router"]; !ok {
		t.Errorf("the importer of the broken package is not reported; got %v", skipped)
	} else if imp.Kind != skipType {
		t.Errorf("importer kind = %q, want %q", imp.Kind, skipType)
	}
}
