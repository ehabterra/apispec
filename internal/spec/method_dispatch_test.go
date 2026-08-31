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

package spec

import (
	"sort"
	"testing"

	"github.com/ehabterra/apispec/internal/metadata"
)

func TestFileOfPosition(t *testing.T) {
	cases := map[string]string{
		"/a/b/x.go:10:5":  "/a/b/x.go",
		"x.go:1:2":        "x.go",
		`C:\a\x.go:10:20`: `C:\a\x.go`, // Windows drive colon preserved
		"nocolon":         "nocolon",
		"only:1":          "only", // one colon: strips the trailing segment
	}
	for in, want := range cases {
		if got := fileOfPosition(in); got != want {
			t.Errorf("fileOfPosition(%q) = %q, want %q", in, got, want)
		}
	}
}

// wholeFileScope is the handler scope a declaration with no recorded EndLine
// gets: every position in its file counts as inside it.
func wholeFileScope(file string) handlerScope {
	return handlerScope{file: file, start: codePos{file: file, line: 1, col: 1}}
}

// resp is a small helper to build a positioned response.
func resp(status int, file string, line int) *ResponseInfo {
	return &ResponseInfo{StatusCode: status, BodyType: "T", File: file, Line: line}
}

func TestSplitRouteByMethodBranches_Scoping(t *testing.T) {
	const handlerFile = "handler.go"
	branches := []metadata.MethodBranch{
		{Methods: []string{"GET"}, StartLine: 10, EndLine: 12},
		{Methods: []string{"POST"}, StartLine: 13, EndLine: 18},
	}
	route := &RouteInfo{
		Path:     "/users",
		Method:   "POST",
		Function: "pkg.usersHandler",
		Package:  "pkg",
		Response: map[string]*ResponseInfo{
			"200": resp(200, handlerFile, 11), // inside GET branch
			"201": resp(201, handlerFile, 15), // inside POST branch
			"405": resp(405, handlerFile, 20), // inside handler, no branch (default arm) -> dropped
			"500": resp(500, "helper.go", 3),  // other file (shared helper) -> all methods
		},
		Request: &RequestInfo{BodyType: "CreateReq", File: handlerFile, Line: 14}, // POST branch
	}

	got := splitRouteByMethodBranches(route, branches, wholeFileScope(handlerFile))
	if len(got) != 2 {
		t.Fatalf("want 2 method routes, got %d", len(got))
	}
	byMethod := map[string]*RouteInfo{}
	for _, r := range got {
		byMethod[r.Method] = r
	}

	get, post := byMethod["GET"], byMethod["POST"]
	if get == nil || post == nil {
		t.Fatalf("missing method route(s): %+v", byMethod)
	}

	statuses := func(r *RouteInfo) []string {
		var s []string
		for slot := range r.Response {
			s = append(s, slot)
		}
		sort.Strings(s)
		return s
	}

	// GET: its own 200 + the shared 500; NOT the POST 201, NOT the default 405.
	if want := []string{"200", "500"}; !equalStrs(statuses(get), want) {
		t.Errorf("GET statuses = %v, want %v", statuses(get), want)
	}
	if get.Request != nil {
		t.Errorf("GET should have no request body (Decode is in POST branch), got %+v", get.Request)
	}
	// POST: its own 201 + shared 500 + the request body; NOT GET's 200, NOT 405.
	if want := []string{"201", "500"}; !equalStrs(statuses(post), want) {
		t.Errorf("POST statuses = %v, want %v", statuses(post), want)
	}
	if post.Request == nil {
		t.Errorf("POST should carry the request body")
	}
	// operationId suffixes keep the split unique.
	if get.OperationIDSuffix != "GET" || post.OperationIDSuffix != "POST" {
		t.Errorf("operationId suffixes = %q/%q, want GET/POST", get.OperationIDSuffix, post.OperationIDSuffix)
	}
}

func TestSplitRouteByMethodBranches_MultiMethodBranch(t *testing.T) {
	branches := []metadata.MethodBranch{
		{Methods: []string{"GET", "HEAD"}, StartLine: 5, EndLine: 6},
	}
	route := &RouteInfo{
		Method:   "POST",
		Function: "pkg.ping",
		Response: map[string]*ResponseInfo{"200": resp(200, "h.go", 6)},
	}
	got := splitRouteByMethodBranches(route, branches, wholeFileScope("h.go"))
	if len(got) != 2 {
		t.Fatalf("want GET+HEAD = 2 routes, got %d", len(got))
	}
	for _, r := range got {
		if r.Method != "GET" && r.Method != "HEAD" {
			t.Errorf("unexpected method %q", r.Method)
		}
		if _, ok := r.Response["200"]; !ok {
			t.Errorf("%s missing the shared 200 response", r.Method)
		}
	}
}

// TestSplitMethodDispatchRoutes_SplitsDispatchHandler drives the full split
// path via real (hand-built) metadata: a handler whose Function carries a
// MethodDispatch is expanded into one route per method with scoped responses.
func TestSplitMethodDispatchRoutes_SplitsDispatchHandler(t *testing.T) {
	meta := &metadata.Metadata{StringPool: metadata.NewStringPool()}
	meta.Packages = map[string]*metadata.Package{
		"pkg": {Files: map[string]*metadata.File{
			"h.go": {Functions: map[string]*metadata.Function{
				"usersHandler": {
					Position: meta.StringPool.Get("h.go:1:1"),
					MethodDispatch: []metadata.MethodBranch{
						{Methods: []string{"GET"}, StartLine: 2, EndLine: 3},
						{Methods: []string{"POST"}, StartLine: 4, EndLine: 6},
					},
				},
			}},
		}},
	}
	route := &RouteInfo{
		Path: "/users", Method: "POST",
		Function: "pkg.usersHandler", Package: "pkg",
		Metadata: meta,
		Response: map[string]*ResponseInfo{
			"200": resp(200, "h.go", 2), // GET branch
			"201": resp(201, "h.go", 5), // POST branch
		},
	}
	got := splitMethodDispatchRoutes([]*RouteInfo{route})
	if len(got) != 2 {
		t.Fatalf("want 2 routes after split, got %d", len(got))
	}
	byMethod := map[string]*RouteInfo{}
	for _, r := range got {
		byMethod[r.Method] = r
	}
	if byMethod["GET"] == nil || byMethod["POST"] == nil {
		t.Fatalf("want GET and POST routes, got %v", byMethod)
	}
	if _, ok := byMethod["GET"].Response["200"]; !ok {
		t.Errorf("GET should own the 200 response")
	}
	if _, ok := byMethod["POST"].Response["201"]; !ok {
		t.Errorf("POST should own the 201 response")
	}
	if _, ok := byMethod["GET"].Response["201"]; ok {
		t.Errorf("GET should not carry the POST-branch 201")
	}
}

// TestSplitMethodDispatchRoutes_SkipsExplicitMethod verifies that a route
// registered with a concrete verb (MethodExplicit) is NOT split even when its
// handler happens to branch on r.Method — the router only routes that one verb
// to the handler.
func TestSplitMethodDispatchRoutes_SkipsExplicitMethod(t *testing.T) {
	meta := &metadata.Metadata{StringPool: metadata.NewStringPool()}
	meta.Packages = map[string]*metadata.Package{
		"pkg": {Files: map[string]*metadata.File{
			"h.go": {Functions: map[string]*metadata.Function{
				"h": {
					Position: meta.StringPool.Get("h.go:1:1"),
					MethodDispatch: []metadata.MethodBranch{
						{Methods: []string{"GET"}, StartLine: 2, EndLine: 3},
						{Methods: []string{"POST"}, StartLine: 4, EndLine: 6},
					},
				},
			}},
		}},
	}
	route := &RouteInfo{
		Path: "/x", Method: "GET", MethodExplicit: true, // e.g. router.GET("/x", h)
		Function: "pkg.h", Package: "pkg", Metadata: meta,
	}
	got := splitMethodDispatchRoutes([]*RouteInfo{route})
	if len(got) != 1 || got[0].Method != "GET" {
		t.Errorf("explicit-method route must not split; got %d routes %+v", len(got), got)
	}
}

// splitMethodDispatchRoutes must pass through routes whose handler has no
// dispatch (or no resolvable metadata) unchanged.
func TestSplitMethodDispatchRoutes_PassThrough(t *testing.T) {
	routes := []*RouteInfo{
		{Path: "/a", Method: "GET", Function: "pkg.plain"}, // nil Metadata -> not dispatch
	}
	got := splitMethodDispatchRoutes(routes)
	if len(got) != 1 || got[0] != routes[0] {
		t.Errorf("non-dispatch route should pass through unchanged, got %+v", got)
	}
}

func equalStrs(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestSplitMethodDispatchRoutes_ClosureHandler drives the split for a handler
// that is a function literal. Its dispatch is recorded under the closure's
// identity rather than on a Function, because a literal has no Function record
// and the enclosing declaration's MethodDispatch mixes in every other literal's
// arms (issue #382).
func TestSplitMethodDispatchRoutes_ClosureHandler(t *testing.T) {
	const file = "main.go"
	const key = "pkg.FuncLit:main.go:10:30"
	meta := &metadata.Metadata{StringPool: metadata.NewStringPool()}
	meta.LitDispatch = map[string]metadata.LitDispatch{
		key: {
			File: meta.StringPool.Get(file),
			Body: metadata.Block{
				Kind: metadata.BlockFuncLit,
				// The literal's body opens on the SAME line as the
				// registration call that carries it.
				StartLine: 10, StartCol: 70,
				EndLine: 20, EndCol: 3,
			},
			Branches: []metadata.MethodBranch{
				{Methods: []string{"GET"}, StartLine: 12, EndLine: 13},
				{Methods: []string{"POST"}, StartLine: 14, EndLine: 16},
			},
		},
	}
	route := &RouteInfo{
		Path: "/users", Method: "POST",
		Function: key, Package: "pkg", Metadata: meta,
		Response: map[string]*ResponseInfo{
			"200": {StatusCode: 200, BodyType: "User", File: file, Line: 12},
			"201": {StatusCode: 201, File: file, Line: 15},
			// The default arm's 405: inside the literal, in no arm.
			"405": {StatusCode: 405, File: file, Line: 18},
			// A second closure further down the same file: outside THIS
			// literal, so it is not one of its arms' bodies either.
			"418": {StatusCode: 418, BodyType: "Teapot", File: file, Line: 30},
		},
	}

	got := splitMethodDispatchRoutes([]*RouteInfo{route})
	byMethod := map[string]*RouteInfo{}
	for _, r := range got {
		byMethod[r.Method] = r
	}
	if len(got) != 2 || byMethod["GET"] == nil || byMethod["POST"] == nil {
		t.Fatalf("want a GET and a POST route, got %d: %v", len(got), byMethod)
	}
	if _, ok := byMethod["GET"].Response["200"]; !ok {
		t.Error("GET should own the 200 written in its arm")
	}
	if _, ok := byMethod["GET"].Response["201"]; ok {
		t.Error("GET must not carry the POST arm's 201")
	}
	for _, m := range []string{"GET", "POST"} {
		if _, ok := byMethod[m].Response["405"]; ok {
			t.Errorf("%s must not carry the default arm's 405", m)
		}
		// Outside the literal with no chain to place it: shared, as for any
		// body whose arm cannot be determined.
		if _, ok := byMethod[m].Response["418"]; !ok {
			t.Errorf("%s should keep the body written outside the literal", m)
		}
	}
}

// A response written in a CALLEE is attributed by the call site in the arm that
// reached it — the statement's own position is in another function and says
// nothing about the verb (issue #382).
func TestSplitRouteByMethodBranches_AttributesByChain(t *testing.T) {
	const file = "main.go"
	scope := handlerScope{
		file:  file,
		start: codePos{file: file, line: 10, col: 70},
		end:   codePos{file: file, line: 20, col: 3},
	}
	branches := []metadata.MethodBranch{
		{Methods: []string{"GET"}, StartLine: 12, EndLine: 13},
		{Methods: []string{"POST"}, StartLine: 14, EndLine: 16},
	}
	// Both bodies are written in handlers.go, one frame down; the arm is only
	// visible in EntrySites.
	route := &RouteInfo{
		Path: "/users", Method: "POST",
		Response: map[string]*ResponseInfo{
			"200": {StatusCode: 200, BodyType: "User", File: "handlers.go", Line: 5,
				EntrySites: []codePos{{file: file, line: 13, col: 4}, {file: "handlers.go", line: 5, col: 2}}},
			"201": {StatusCode: 201, File: "handlers.go", Line: 12,
				EntrySites: []codePos{{file: file, line: 15, col: 4}, {file: "handlers.go", line: 12, col: 2}}},
		},
		Request: &RequestInfo{BodyType: "CreateReq", File: "handlers.go", Line: 10,
			EntrySites: []codePos{{file: file, line: 15, col: 4}, {file: "handlers.go", line: 10, col: 2}}},
	}
	got := splitRouteByMethodBranches(route, branches, scope)
	byMethod := map[string]*RouteInfo{}
	for _, r := range got {
		byMethod[r.Method] = r
	}
	get, post := byMethod["GET"], byMethod["POST"]
	if get == nil || post == nil {
		t.Fatalf("want GET and POST, got %v", byMethod)
	}
	if _, ok := get.Response["200"]; !ok {
		t.Error("the 200 was reached from the GET arm, so GET should own it")
	}
	if _, ok := get.Response["201"]; ok {
		t.Error("the 201 was reached from the POST arm; GET must not carry it")
	}
	if get.Request != nil {
		t.Error("the request body was decoded under the POST arm; GET must not carry it")
	}
	if post.Request == nil {
		t.Error("POST should carry the request body decoded in its arm")
	}
}

// A registration that named its verb is not split — the router sends it that
// verb only — but the other arms' bodies are dead for it and must not be
// documented on it.
func TestSplitMethodDispatchRoutes_ExplicitMethodScopedToArm(t *testing.T) {
	const file = "h.go"
	meta := &metadata.Metadata{StringPool: metadata.NewStringPool()}
	meta.Packages = map[string]*metadata.Package{
		"pkg": {Files: map[string]*metadata.File{
			file: {Functions: map[string]*metadata.Function{
				"h": {
					Position: meta.StringPool.Get(file + ":1:1"),
					EndLine:  10,
					MethodDispatch: []metadata.MethodBranch{
						{Methods: []string{"GET"}, StartLine: 2, EndLine: 3},
						{Methods: []string{"POST"}, StartLine: 4, EndLine: 6},
					},
				},
			}},
		}},
	}
	route := &RouteInfo{
		Path: "/x", Method: "GET", MethodExplicit: true, // e.g. router.GET("/x", h)
		Function: "pkg.h", Package: "pkg", Metadata: meta,
		Response: map[string]*ResponseInfo{
			"200": {StatusCode: 200, BodyType: "User", File: file, Line: 2},
			"201": {StatusCode: 201, File: file, Line: 5},
		},
		Request: &RequestInfo{BodyType: "CreateReq", File: file, Line: 5},
	}
	got := splitMethodDispatchRoutes([]*RouteInfo{route})
	if len(got) != 1 || got[0].Method != "GET" {
		t.Fatalf("explicit-method route must not split; got %d routes %+v", len(got), got)
	}
	if got[0].OperationIDSuffix != "" {
		t.Errorf("a route that did not split must keep its operationId, got suffix %q", got[0].OperationIDSuffix)
	}
	if _, ok := got[0].Response["200"]; !ok {
		t.Error("GET should keep the 200 written in the GET arm")
	}
	if _, ok := got[0].Response["201"]; ok {
		t.Error("GET must not document the POST arm's 201")
	}
	if got[0].Request != nil {
		t.Error("GET must not document the POST arm's request body")
	}

	// A verb the switch does not name says nothing about this route: nothing is
	// scoped away rather than everything (golden rule #7).
	route.Method = "PATCH"
	got = splitMethodDispatchRoutes([]*RouteInfo{route})
	if len(got) != 1 || got[0] != route {
		t.Fatalf("a verb with no arm must pass the route through untouched, got %+v", got)
	}
}

func TestHandlerScopeContains(t *testing.T) {
	scope := handlerScope{
		file:  "main.go",
		start: codePos{file: "main.go", line: 10, col: 70},
		end:   codePos{file: "main.go", line: 20, col: 3},
	}
	cases := []struct {
		name string
		pos  codePos
		want bool
	}{
		// The registration call shares the literal's opening line but is
		// written before it — a line-only test would call this inside.
		{"registration call on the opening line", codePos{file: "main.go", line: 10, col: 2}, false},
		{"inside the body", codePos{file: "main.go", line: 15, col: 4}, true},
		{"on the closing line, before the brace", codePos{file: "main.go", line: 20, col: 2}, true},
		{"after the closing brace", codePos{file: "main.go", line: 20, col: 40}, false},
		{"below the literal", codePos{file: "main.go", line: 30, col: 2}, false},
		{"another file", codePos{file: "other.go", line: 15, col: 4}, false},
		{"no position", codePos{}, false},
	}
	for _, c := range cases {
		if got := scope.contains(c.pos); got != c.want {
			t.Errorf("%s: contains(%+v) = %v, want %v", c.name, c.pos, got, c.want)
		}
	}
	// A declaration with no recorded EndLine falls back to its whole file, the
	// behaviour before ranges existed.
	whole := wholeFileScope("main.go")
	if !whole.contains(codePos{file: "main.go", line: 900, col: 1}) {
		t.Error("a scope with no end line must cover its whole file")
	}
}

func TestParsePosition(t *testing.T) {
	cases := []struct {
		in   string
		file string
		line int
		col  int
	}{
		{"/a/b/x.go:10:5", "/a/b/x.go", 10, 5},
		{`C:\a\x.go:10:20`, `C:\a\x.go`, 10, 20},
		{"nocolon", "nocolon", 0, 0},
	}
	for _, c := range cases {
		file, line, col := parsePosition(c.in)
		if file != c.file || line != c.line || col != c.col {
			t.Errorf("parsePosition(%q) = %q,%d,%d; want %q,%d,%d",
				c.in, file, line, col, c.file, c.line, c.col)
		}
	}
}
