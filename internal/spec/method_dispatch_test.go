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

	got := splitRouteByMethodBranches(route, branches, handlerFile)
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
	got := splitRouteByMethodBranches(route, branches, "h.go")
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
