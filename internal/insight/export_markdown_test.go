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

package insight

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	spec "github.com/ehabterra/apispec/internal/spec"
)

// writeNumberedSource writes a Go-ish file with n numbered lines and returns
// its path, for exercising the position-based source readers.
func writeNumberedSource(t *testing.T, n int) string {
	t.Helper()
	var b strings.Builder
	for i := 1; i <= n; i++ {
		fmt.Fprintf(&b, "// line %d\n", i)
	}
	p := filepath.Join(t.TempDir(), "handler.go")
	if err := os.WriteFile(p, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestBuildEndpointExportMarkdown(t *testing.T) {
	src := writeNumberedSource(t, 40)
	const module = "github.com/me/proj"

	rep := &EndpointReport{
		Method:     "POST",
		Path:       "/users",
		Handler:    "createUser",
		HandlerPos: src + ":10",
		Request:    &ReqInfo{ContentType: "application/json", Schema: "github_com_me_proj_User"},
		Responses: []RespInfo{
			{Status: "201", ContentType: "application/json", Schema: "github_com_me_proj_User"},
		},
		Params: []ParamInfo{
			{Name: "id", In: "path", Type: "string", Required: true},
			{Name: "verbose", In: "query", Type: "boolean", Required: false},
		},
		Issues: []Issue{
			{Severity: "warn", Kind: "unresolved-type", Detail: "placeholder schema", Ref: "github_com_me_proj_User"},
			{Severity: "warn", Kind: "unresolved-type", Detail: "external placeholder", Ref: "uuid_UUID"},
			{Severity: "info", Kind: "no-responses", Detail: "informational only"},
		},
		Trace: TraceGraph{
			Nodes: []TraceNode{
				{ID: "n1", Label: "handler"},
				{ID: "n2", Label: "svc.Create", Resolved: true},
			},
			Edges: []TraceEdge{{Source: "n1", Target: "n2"}},
		},
	}
	opts := ExportOptions{ConfigYAML: "framework: gin\n", ModulePath: module}

	out := BuildEndpointExportMarkdown(rep, opts)

	for _, want := range []string{
		"**POST /users**",
		"Handler: `createUser`",
		"- Request: application/json `github_com_me_proj_User`",
		"- Response 201:",
		"- Param `id` in path: string (required)",
		"- Param `verbose` in query: boolean\n", // no (required) suffix
		"## Problem(s)",
		"_in-module type; should auto-resolve, prefer a code/analysis fix_",
		"_external type; prefer a config entry_",
		"## Resolution trace",
		"⟐ impl (resolved from interface)",
		"## Handler source",
		"// line 10",
		"## Current apispec config",
		"framework: gin",
		"## What I need",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("markdown missing %q", want)
		}
	}
	if strings.Contains(out, "informational only") {
		t.Error("info-severity issue must not appear in Problem(s)")
	}
}

func TestBuildEndpointExportMarkdown_RedactsAndTruncates(t *testing.T) {
	const module = "github.com/me/proj"
	// 45 edges exceeds the 40-edge cap.
	var edges []TraceEdge
	for i := 0; i < 45; i++ {
		edges = append(edges, TraceEdge{Source: "a", Target: "b"})
	}
	rep := &EndpointReport{
		Method: "GET",
		Path:   "/" + module + "/thing",
		Trace:  TraceGraph{Edges: edges},
	}
	out := BuildEndpointExportMarkdown(rep, ExportOptions{ModulePath: module, Redact: true})

	if strings.Contains(out, module) {
		t.Error("module path must be redacted")
	}
	if !strings.Contains(out, "example.com/app") {
		t.Error("redaction placeholder missing")
	}
	if !strings.Contains(out, "…\n") {
		t.Error("trace should be truncated with ellipsis at 40 edges")
	}
}

func TestPosFileAndSourceSnippet(t *testing.T) {
	src := writeNumberedSource(t, 30)

	if got := PosFile(src + ":12:4"); got != src {
		t.Errorf("PosFile with col = %q, want %q", got, src)
	}
	if got := PosFile("no-position-here"); got != "" {
		t.Errorf("PosFile(unparseable) = %q, want empty", got)
	}
	if got := PosFile(""); got != "" {
		t.Errorf("PosFile(empty) = %q, want empty", got)
	}

	code, start, target := SourceSnippet(src+":12", 2, 3)
	if start != 10 || target != 12 {
		t.Errorf("window = (start %d, target %d), want (10, 12)", start, target)
	}
	if !strings.Contains(code, "// line 10") || !strings.Contains(code, "// line 15") {
		t.Errorf("window content wrong:\n%s", code)
	}
	if strings.Contains(code, "// line 9") || strings.Contains(code, "// line 16") {
		t.Errorf("window leaked outside bounds:\n%s", code)
	}

	// Window near the top of file clamps to line 1.
	_, start, _ = SourceSnippet(src+":2", 5, 2)
	if start != 1 {
		t.Errorf("clamped start = %d, want 1", start)
	}

	// Failure modes: missing file, bad position.
	if code, s, tl := SourceSnippet(filepath.Join(t.TempDir(), "absent.go")+":3", 1, 1); code != "" || s != 0 || tl != 0 {
		// readSourceWindow is best-effort: code empty; SourceSnippet still
		// reports the parsed window; only fully unparseable input zeroes out.
		if code != "" {
			t.Errorf("missing file should yield empty code, got %q", code)
		}
	}
	if code, s, tl := SourceSnippet("garbage", 1, 1); code != "" || s != 0 || tl != 0 {
		t.Errorf("unparseable pos = (%q,%d,%d), want zeros", code, s, tl)
	}
}

func TestOverviewSmallHelpers(t *testing.T) {
	if got := lastSegment("github.com/me/proj/api"); got != "api" {
		t.Errorf("lastSegment = %q", got)
	}
	if got := lastSegment("api"); got != "api" {
		t.Errorf("lastSegment(no slash) = %q", got)
	}

	counts := []Count{{Name: "a", Count: 3}, {Name: "b", Count: 2}, {Name: "c", Count: 1}}
	if got := topN(counts, 2); len(got) != 2 || got[1].Name != "b" {
		t.Errorf("topN(2) = %v", got)
	}
	if got := topN(counts, 10); len(got) != 3 {
		t.Errorf("topN(10) = %v", got)
	}
}

func TestOperationFor(t *testing.T) {
	ops := map[string]*spec.Operation{
		"GET": {}, "POST": {}, "PUT": {}, "DELETE": {}, "PATCH": {}, "OPTIONS": {}, "HEAD": {},
	}
	pi := spec.PathItem{
		Get: ops["GET"], Post: ops["POST"], Put: ops["PUT"], Delete: ops["DELETE"],
		Patch: ops["PATCH"], Options: ops["OPTIONS"], Head: ops["HEAD"],
	}
	for method, want := range ops {
		if got := operationFor(pi, strings.ToLower(method)); got != want {
			t.Errorf("operationFor(%q) returned the wrong operation", method)
		}
	}
	if got := operationFor(pi, "TRACE"); got != nil {
		t.Errorf("operationFor(TRACE) = %v, want nil", got)
	}
}

func TestFirstContent(t *testing.T) {
	jsonMT := spec.MediaType{Schema: &spec.Schema{Type: "object"}}
	xmlMT := spec.MediaType{Schema: &spec.Schema{Type: "string"}}

	ct, mt := firstContent(map[string]spec.MediaType{"application/xml": xmlMT, "application/json": jsonMT})
	if ct != "application/json" || mt.Schema != jsonMT.Schema {
		t.Errorf("firstContent should prefer application/json, got %q", ct)
	}
	ct, mt = firstContent(map[string]spec.MediaType{"application/xml": xmlMT})
	if ct != "application/xml" || mt.Schema != xmlMT.Schema {
		t.Errorf("firstContent fallback = %q", ct)
	}
	if ct, _ := firstContent(nil); ct != "" {
		t.Errorf("firstContent(nil) = %q, want empty", ct)
	}
}
