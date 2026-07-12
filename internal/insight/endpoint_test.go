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
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ehabterra/apispec/internal/spec"
)

func TestBuildEndpoint_SpecOnly(t *testing.T) {
	s := &spec.OpenAPISpec{
		Paths: map[string]spec.PathItem{
			"/orders/{id}": {
				Parameters: []spec.Parameter{{Name: "id", In: "path", Required: true, Schema: &spec.Schema{Type: "string"}}},
				Post: &spec.Operation{
					OperationID: "app.Handler.Create",
					Tags:        []string{"orders"},
					RequestBody: &spec.RequestBody{Required: true, Content: map[string]spec.MediaType{
						"application/json": {Schema: ref("CreateOrderReq")},
					}},
					Responses: map[string]spec.Response{
						"201": {Content: map[string]spec.MediaType{"application/json": {Schema: ref("Order")}}},
						"400": {Content: map[string]spec.MediaType{"application/json": {Schema: ref("Missing")}}},
					},
				},
			},
		},
		Components: &spec.Components{Schemas: map[string]*spec.Schema{
			"CreateOrderReq": {Type: "object"},
			"Order":          {Type: "object"},
		}},
	}

	rep := BuildEndpoint(s, nil, "post", "/orders/{id}")
	if !rep.Found {
		t.Fatal("operation should be Found")
	}
	if rep.HandlerFound {
		t.Error("handler should not be found with nil metadata")
	}
	if rep.Request == nil || rep.Request.Schema != "CreateOrderReq" || !rep.Request.Required {
		t.Errorf("request = %+v", rep.Request)
	}
	if len(rep.Responses) != 2 || rep.Responses[0].Status != "201" || rep.Responses[0].Schema != "Order" {
		t.Errorf("responses = %+v", rep.Responses)
	}
	if len(rep.Params) != 1 || rep.Params[0].Name != "id" || rep.Params[0].In != "path" {
		t.Errorf("params = %+v", rep.Params)
	}
	// /400 references a missing component → a warn issue
	if !hasKind(rep.Issues, "dangling-ref") {
		t.Errorf("expected dangling-ref issue, got %+v", rep.Issues)
	}
	// slices must be non-nil for JSON
	if rep.Responses == nil || rep.Params == nil || rep.Issues == nil || rep.Trace.Nodes == nil {
		t.Error("slices must be non-nil")
	}
}

func TestBuildEndpoint_NotFound(t *testing.T) {
	s := &spec.OpenAPISpec{Paths: map[string]spec.PathItem{"/a": {Get: &spec.Operation{}}}}
	if BuildEndpoint(s, nil, "GET", "/missing").Found {
		t.Error("missing path should not be Found")
	}
	if BuildEndpoint(s, nil, "DELETE", "/a").Found {
		t.Error("missing method should not be Found")
	}
}

func TestCountPaths(t *testing.T) {
	// diamond: A→{B,C}, B→D, C→D  ⇒ 2 paths
	adj := map[string][]string{"A": {"B", "C"}, "B": {"D"}, "C": {"D"}}
	if n, tr := countPaths(adj, "A"); n != 2 || tr {
		t.Errorf("diamond paths = %d trunc=%v, want 2 false", n, tr)
	}
	// linear ⇒ 1
	if n, _ := countPaths(map[string][]string{"A": {"B"}, "B": {"C"}}, "A"); n != 1 {
		t.Errorf("linear = %d, want 1", n)
	}
	// cycle must terminate and not return 0
	if n, _ := countPaths(map[string][]string{"A": {"B"}, "B": {"A"}}, "A"); n < 1 {
		t.Errorf("cycle = %d, want >=1", n)
	}
}

func TestGrade(t *testing.T) {
	if g, _ := grade(Metrics{MaxDepth: 2, CallPaths: 3, FanoutMax: 2}); g != "A" {
		t.Errorf("simple should be A, got %s", g)
	}
	if g, _ := grade(Metrics{MaxDepth: 12, CallPaths: 900}); g != "D" {
		t.Errorf("deep should be D, got %s", g)
	}
	// truncation floors the grade at C and marks it a lower bound
	g, lb := grade(Metrics{MaxDepth: 2, CallPaths: 5, FanoutMax: 2, CallPathsTruncated: true})
	if g != "C" || !lb {
		t.Errorf("truncated = %s lb=%v, want C true", g, lb)
	}
}

func TestSchemaSummary(t *testing.T) {
	cases := []struct {
		in   *spec.Schema
		want string
	}{
		{ref("github_com_x_dtos_Order"), "Order"},
		{&spec.Schema{Type: "string", Format: "uuid"}, "string (uuid)"},
		{&spec.Schema{Type: "array", Items: ref("Item")}, "[]Item"},
		{&spec.Schema{AllOf: []*spec.Schema{ref("Env"), {Type: "object", Properties: map[string]*spec.Schema{"data": ref("Order")}}}}, "allOf[Env + {data}]"},
		{nil, ""},
	}
	for _, c := range cases {
		if got := schemaSummary(c.in); got != c.want {
			t.Errorf("schemaSummary = %q, want %q", got, c.want)
		}
	}
}

func TestParsePosAndSourceWindow(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "h.go")
	if err := os.WriteFile(f, []byte("line1\nline2\nline3\nline4\nline5\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if file, line := parsePos(f + ":3:10"); file != f || line != 3 {
		t.Errorf("parsePos col form = %q,%d", file, line)
	}
	if file, line := parsePos(f + ":2"); file != f || line != 2 {
		t.Errorf("parsePos no-col = %q,%d", file, line)
	}
	if _, line := parsePos("nope"); line != 0 {
		t.Errorf("parsePos garbage should be 0, got %d", line)
	}

	win := readSourceWindow(f+":3:1", 1, 1) // lines 2..4
	if !strings.Contains(win, "line2") || !strings.Contains(win, "line4") || strings.Contains(win, "line5") {
		t.Errorf("source window = %q", win)
	}
	if readSourceWindow("/no/such/file.go:1:1", 0, 5) != "" {
		t.Error("missing file should yield empty window")
	}
}
