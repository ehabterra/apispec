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
	"path/filepath"
	"strings"
	"testing"

	"github.com/ehabterra/apispec/spec"
)

// generateWithReports runs a fixture through its own Generator so the test can
// read what the generation reported, not only what it emitted.
func generateWithReports(t *testing.T, name string) (*spec.OpenAPISpec, *Generator) {
	t.Helper()
	dir := filepath.Join("..", "testdata", name)
	gen := NewGenerator(spec.DefaultHTTPConfig())
	out, err := gen.GenerateFromDirectory(dir)
	if err != nil {
		t.Fatalf("GenerateFromDirectory(%s): %v", dir, err)
	}
	if out == nil || out.Paths == nil {
		t.Fatalf("nil spec or paths for %s", name)
	}
	return out, gen
}

// assertNoPlaceholderOnlyPath fails when the document contains a path that is
// nothing but placeholders — the shape issue #428 is about. Written as a
// property over the whole document rather than as a check for the two specific
// keys, so any other expression that starts resolving to a bare placeholder
// fails here too.
func assertNoPlaceholderOnlyPath(t *testing.T, out *spec.OpenAPISpec) {
	t.Helper()
	for path := range out.Paths {
		stripped := path
		for {
			open := strings.IndexByte(stripped, '{')
			if open < 0 {
				break
			}
			close := strings.IndexByte(stripped[open:], '}')
			if close < 0 {
				break
			}
			stripped = stripped[:open] + stripped[open+close+1:]
		}
		if strings.Trim(stripped, "/ ") == "" {
			t.Errorf("path %q is nothing but placeholders: an unresolvable registration must be reported, not emitted", path)
		}
	}
}

// TestTestdata_RouteTable locks in the reporting of a registration read from a
// route table (issue #428): `mux.HandleFunc(rt.Method+" "+rt.Path, rt.Handler)`
// used to be documented at `/{Method} {Path}`, an operation no request can
// match. The literal and partly-resolved registrations beside it must survive —
// dropping those would trade one wrong answer for a missing API.
func TestTestdata_RouteTable(t *testing.T) {
	out, gen := generateWithReports(t, "route_table")
	noDanglingRefs(t, out)
	assertNoPlaceholderOnlyPath(t, out)

	if _, ok := out.Paths["/{Method} {Path}"]; ok {
		t.Error("the table registration must not be documented under its placeholder path")
	}
	// The routes the table registers are not in the document at all: their paths
	// exist only at runtime.
	for _, path := range []string{"/users", "/users/{id}"} {
		if _, ok := out.Paths[path]; ok {
			t.Errorf("path %q cannot be recovered from a table; documenting it would be a guess", path)
		}
	}

	// What IS readable is still documented.
	if _, ok := out.Paths["/health"]; !ok {
		t.Errorf("the literal registration must still be documented; have %v", mapPathKeys(out.Paths))
	}
	// A prefix that could not be read leaves a real endpoint with a flagged
	// placeholder — kept, not dropped (#274/#34).
	if _, ok := out.Paths["/{prefix}/partly"]; !ok {
		t.Errorf("a partly-resolved path must still be documented; have %v", mapPathKeys(out.Paths))
	}

	// The drop is reported, with the registration site.
	reports := gen.UnresolvedPaths()
	if len(reports) != 1 {
		t.Fatalf("want exactly the table registration reported, got %d: %+v", len(reports), reports)
	}
	r := reports[0]
	if !strings.Contains(r.Path, "{") {
		t.Errorf("report should name the placeholder path it would have emitted, got %q", r.Path)
	}
	if r.Reason == "" {
		t.Error("report should say which way the path failed")
	}
	if !strings.Contains(r.Position, "main.go:") {
		t.Errorf("report should locate the registration, got %q", r.Position)
	}
	if r.Handler == "" {
		t.Error("report should name the handler")
	}
}

// TestTestdata_ChainedWrapper locks in the same for a house router that carries
// the pattern on a returned object: wrapper derivation already declined it as
// "incomplete, not applied", and the framework call inside the wrapper is no
// longer documented at `/{pattern}` behind its back (issue #428).
func TestTestdata_ChainedWrapper(t *testing.T) {
	out, gen := generateWithReports(t, "chained_wrapper")
	noDanglingRefs(t, out)
	assertNoPlaceholderOnlyPath(t, out)

	if _, ok := out.Paths["/{pattern}"]; ok {
		t.Error("the chained registration must not be documented under its placeholder path")
	}
	if _, ok := out.Paths["/items"]; ok {
		t.Error("the chained pattern is not readable at the framework call; documenting /items would be a guess")
	}

	// The ordinary method on the same router still resolves: what is unsupported
	// is the chained wiring, not this project's router.
	item, ok := out.Paths["/health"]
	if !ok {
		t.Fatalf("the forwarded-parameter registration must still be documented; have %v", mapPathKeys(out.Paths))
	}
	if opFor(item, "GET") == nil {
		t.Error("GET /health missing")
	}

	// Both chained calls are reported.
	if reports := gen.UnresolvedPaths(); len(reports) != 2 {
		t.Errorf("want the chained Get and Post reported, got %d: %+v", len(reports), reports)
	}
}
