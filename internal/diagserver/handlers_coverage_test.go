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

package diagserver

import (
	"compress/gzip"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// muxFor registers the server's routes on a fresh mux with default paths.
func muxFor(s *Server) *http.ServeMux {
	mux := http.NewServeMux()
	s.RegisterRoutes(mux, RouteOptions{UIPath: "/", APIPrefix: "/api/diagram", HealthPath: "/health"})
	return mux
}

func do(mux *http.ServeMux, method, path string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(method, path, nil))
	return w
}

// TestPackageBasedDiagram exercises the by-packages data path, which needs the
// required packages parameter the smoke test omits (it only saw the 400).
func TestPackageBasedDiagram(t *testing.T) {
	s := injectedServer(t)
	mux := muxFor(s)

	// Pick a real package name from the fixture so filtering keeps nodes.
	var pkg string
	for name := range s.metadata.Packages {
		if pkg == "" || len(name) < len(pkg) {
			pkg = name // shortest name is the module root, a useful prefix
		}
	}
	if pkg == "" {
		t.Skip("fixture has no packages")
	}

	type wantPage struct {
		code     int
		hasNodes bool
	}
	cases := []struct {
		name  string
		query string
		want  wantPage
	}{
		{"missing packages param", "", wantPage{http.StatusBadRequest, false}},
		{"basic selection", "?packages=" + pkg, wantPage{http.StatusOK, true}},
		{"with depth and isolate", "?packages=" + pkg + "&depth=2&isolate=true", wantPage{http.StatusOK, true}},
		{"negative depth clamps", "?packages=" + pkg + "&depth=-3", wantPage{http.StatusOK, true}},
		{"filters that miss everything", "?packages=" + pkg + "&function=zz_no_such_fn&scope=exported", wantPage{http.StatusOK, false}},
		{"unknown package yields empty", "?packages=example.com/definitely/absent", wantPage{http.StatusOK, false}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			w := do(mux, http.MethodGet, "/api/diagram/by-packages"+c.query)
			if w.Code != c.want.code {
				t.Fatalf("code = %d, want %d (body: %s)", w.Code, c.want.code, w.Body.String()[:min(200, w.Body.Len())])
			}
			if c.want.code != http.StatusOK {
				return
			}
			var resp PaginatedResponse
			if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
				t.Fatalf("response is not JSON: %v", err)
			}
			if c.want.hasNodes && resp.TotalNodes == 0 {
				t.Error("expected nodes for selected package, got none")
			}
			if !c.want.hasNodes && resp.TotalNodes != 0 {
				t.Errorf("expected no nodes, got %d", resp.TotalNodes)
			}
		})
	}
}

// TestTrackerTreeDiagramType covers the tracker-tree branch of getAllData and
// the by-packages non-call-graph depth branch.
func TestTrackerTreeDiagramType(t *testing.T) {
	s := injectedServer(t)
	s.config.DiagramType = "tracker-tree"
	mux := muxFor(s)

	if w := do(mux, http.MethodGet, "/api/diagram"); w.Code != http.StatusOK {
		t.Errorf("tracker-tree diagram -> %d", w.Code)
	}
	var pkg string
	for name := range s.metadata.Packages {
		pkg = name
		break
	}
	if w := do(mux, http.MethodGet, "/api/diagram/by-packages?packages="+pkg); w.Code != http.StatusOK {
		t.Errorf("tracker-tree by-packages -> %d", w.Code)
	}
	// Second hit is served from the data cache.
	if w := do(mux, http.MethodGet, "/api/diagram"); w.Code != http.StatusOK {
		t.Errorf("cached tracker-tree diagram -> %d", w.Code)
	}
}

// TestMethodNotAllowed covers every handler's method guard.
func TestMethodNotAllowed(t *testing.T) {
	s := injectedServer(t)
	mux := muxFor(s)

	posts := []string{
		"/api/diagram",
		"/api/diagram/page",
		"/api/diagram/packages",
		"/api/diagram/by-packages",
		"/api/diagram/stats",
		"/api/diagram/export",
		"/health",
	}
	for _, p := range posts {
		if w := do(mux, http.MethodPost, p); w.Code != http.StatusMethodNotAllowed {
			t.Errorf("POST %s -> %d, want 405", p, w.Code)
		}
	}
	// refresh is POST-only, so GET must be rejected.
	if w := do(mux, http.MethodGet, "/api/diagram/refresh"); w.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET refresh -> %d, want 405", w.Code)
	}
}

// TestMetadataLoadFailure covers the ensureMetadata/LoadMetadata error path:
// no injected metadata and an input dir with no Go packages.
func TestMetadataLoadFailure(t *testing.T) {
	s := New(&Config{Host: "localhost", Port: 8080, DiagramType: "call-graph", InputDir: t.TempDir()})
	mux := muxFor(s)

	for _, p := range []string{"/api/diagram", "/api/diagram/stats", "/api/diagram/by-packages?packages=x"} {
		if w := do(mux, http.MethodGet, p); w.Code != http.StatusInternalServerError {
			t.Errorf("GET %s without loadable metadata -> %d, want 500", p, w.Code)
		}
	}
	// refresh reloads via the same engine path and must surface the failure.
	if w := do(mux, http.MethodPost, "/api/diagram/refresh"); w.Code != http.StatusInternalServerError {
		t.Errorf("POST refresh with bad input dir -> %d, want 500", w.Code)
	}
}

// TestExportFormats covers the JSON export payload and the client-side-format
// rejection branch, plus pagination parameter handling.
func TestExportFormats(t *testing.T) {
	s := injectedServer(t)
	mux := muxFor(s)

	w := do(mux, http.MethodGet, "/api/diagram/export?format=json&page=1&size=5&depth=2")
	if w.Code != http.StatusOK {
		t.Fatalf("json export -> %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("content type = %q", ct)
	}
	if cd := w.Header().Get("Content-Disposition"); cd != `attachment; filename="diagram.json"` {
		t.Errorf("content disposition = %q", cd)
	}
	if !json.Valid(w.Body.Bytes()) {
		t.Error("export body is not valid JSON")
	}

	// Default format is svg, which is handled client-side -> 400 with hint.
	if w := do(mux, http.MethodGet, "/api/diagram/export"); w.Code != http.StatusBadRequest {
		t.Errorf("default svg export -> %d, want 400", w.Code)
	}
	// Out-of-range paging params are clamped, not rejected.
	if w := do(mux, http.MethodGet, "/api/diagram/export?format=json&page=0&size=99999"); w.Code != http.StatusOK {
		t.Errorf("clamped paging export -> %d", w.Code)
	}
}

// TestCORSHeaders covers the EnableCORS branches of writeJSON, writeError and
// writeResponse.
func TestCORSHeaders(t *testing.T) {
	s := injectedServer(t)
	s.config.EnableCORS = true
	mux := muxFor(s)

	w := do(mux, http.MethodGet, "/api/diagram/stats")
	if w.Code != http.StatusOK {
		t.Fatalf("stats -> %d", w.Code)
	}
	if w.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Error("writeJSON missing CORS header")
	}

	w = do(mux, http.MethodPost, "/api/diagram/stats")
	if w.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Error("writeError missing CORS header")
	}

	w = do(mux, http.MethodGet, "/")
	if w.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Error("writeResponse missing CORS header")
	}
}

// failingWriter satisfies http.ResponseWriter but fails every write, to reach
// the write-error logging branches.
type failingWriter struct {
	header http.Header
}

func (f *failingWriter) Header() http.Header {
	if f.header == nil {
		f.header = http.Header{}
	}
	return f.header
}
func (f *failingWriter) Write([]byte) (int, error) { return 0, errors.New("sink closed") }
func (f *failingWriter) WriteHeader(int)           {}

// TestWriteHelperErrorPaths drives writeJSON/writeResponse/writeError into
// their failure branches directly.
func TestWriteHelperErrorPaths(t *testing.T) {
	s := newTestServer()

	// Unencodable value (a channel) fails json.Encode.
	s.writeJSON(httptest.NewRecorder(), map[string]any{"bad": make(chan int)})
	// Failing sink covers the write-error logging in all three helpers.
	s.writeJSON(&failingWriter{}, map[string]string{"ok": "yes"})
	s.writeResponse(&failingWriter{}, "payload", "text/plain")
	s.writeError(&failingWriter{}, "boom", http.StatusTeapot)
}

// TestGzipFlush covers the gzipResponseWriter Flush path.
func TestGzipFlush(t *testing.T) {
	rec := httptest.NewRecorder()
	gw, _ := gzip.NewWriterLevel(rec, gzip.BestSpeed)
	zrw := &gzipResponseWriter{ResponseWriter: rec, gw: gw}
	if _, err := zrw.Write([]byte("hello")); err != nil {
		t.Fatalf("write: %v", err)
	}
	zrw.Flush()
	if err := gw.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	zr, err := gzip.NewReader(rec.Body)
	if err != nil {
		t.Fatalf("body is not gzip: %v", err)
	}
	out, _ := io.ReadAll(zr)
	if string(out) != "hello" {
		t.Errorf("roundtrip = %q", out)
	}
}
