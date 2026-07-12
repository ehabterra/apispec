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
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ehabterra/apispec/internal/metadata"
	"github.com/ehabterra/apispec/internal/spec"
)

func emptyNode() spec.CytoscapeNode {
	return spec.CytoscapeNode{}
}

func newTestServer() *Server {
	return New(&Config{
		Host:        "localhost",
		Port:        8080,
		DiagramType: "call-graph",
	})
}

func TestNew(t *testing.T) {
	cfg := &Config{Host: "localhost", Port: 8080}
	server := New(cfg)
	if server == nil {
		t.Fatal("Expected non-nil server")
	}
	if server.config != cfg {
		t.Fatal("Expected config to be set")
	}
}

func TestRegisterRoutes(t *testing.T) {
	server := newTestServer()
	mux := http.NewServeMux()
	server.RegisterRoutes(mux, RouteOptions{})

	// Hit /health on the mux to confirm registration.
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("Expected /health to return 200, got %d", w.Code)
	}
}

func TestRegisterRoutesCustomPrefix(t *testing.T) {
	server := newTestServer()
	server.metadata = &metadata.Metadata{
		Packages:  make(map[string]*metadata.Package),
		CallGraph: []metadata.CallGraphEdge{},
	}

	mux := http.NewServeMux()
	server.RegisterRoutes(mux, RouteOptions{
		UIPath:     "/diagram",
		APIPrefix:  "/api/diagram",
		HealthPath: "/api/diagram/health",
	})

	req := httptest.NewRequest(http.MethodGet, "/api/diagram/health", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("Expected custom health to return 200, got %d", w.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/diagram/stats", nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("Expected stats to return 200, got %d", w.Code)
	}
}

func TestSetInputDirInvalidatesCache(t *testing.T) {
	server := newTestServer()
	server.metadata = &metadata.Metadata{Packages: map[string]*metadata.Package{}}
	server.SetInputDir("/tmp/something")
	if server.metadata != nil {
		t.Error("Expected metadata to be cleared after SetInputDir")
	}
	if len(server.cache) != 0 {
		t.Error("Expected paginated cache to be cleared")
	}
	if len(server.dataCache) != 0 {
		t.Error("Expected data cache to be cleared")
	}
}

func TestHandleIndex(t *testing.T) {
	server := newTestServer()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	server.handleIndex(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, "<!DOCTYPE html>") {
		t.Error("Expected HTML content")
	}
}

func TestHandleHealth(t *testing.T) {
	server := newTestServer()

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	server.handleHealth(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Expected valid JSON, got %v", err)
	}

	if response["status"] != "healthy" {
		t.Errorf("Expected status 'healthy', got %v", response["status"])
	}
}

func TestHandleDiagram(t *testing.T) {
	server := newTestServer()
	server.metadata = &metadata.Metadata{
		Packages:  make(map[string]*metadata.Package),
		CallGraph: []metadata.CallGraphEdge{},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/diagram", nil)
	w := httptest.NewRecorder()

	server.handleDiagram(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

func TestHandlePaginatedDiagram(t *testing.T) {
	server := newTestServer()
	server.metadata = &metadata.Metadata{
		Packages:  make(map[string]*metadata.Package),
		CallGraph: []metadata.CallGraphEdge{},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/diagram/page", nil)
	w := httptest.NewRecorder()

	server.handlePaginatedDiagram(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

func TestHandleStats(t *testing.T) {
	server := newTestServer()
	server.metadata = &metadata.Metadata{
		Packages:  make(map[string]*metadata.Package),
		CallGraph: []metadata.CallGraphEdge{},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/diagram/stats", nil)
	w := httptest.NewRecorder()

	server.handleStats(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Expected valid JSON, got %v", err)
	}

	if response["total_nodes"] == nil {
		t.Error("Expected total_nodes in response")
	}
}

func TestHandleExportInvalidFormat(t *testing.T) {
	server := newTestServer()
	server.metadata = &metadata.Metadata{
		Packages:  make(map[string]*metadata.Package),
		CallGraph: []metadata.CallGraphEdge{},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/diagram/export?format=xml", nil)
	w := httptest.NewRecorder()

	server.handleExport(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400 for invalid format, got %d", w.Code)
	}
}

func TestGeneratePaginatedData(t *testing.T) {
	server := newTestServer()
	server.metadata = &metadata.Metadata{
		Packages:  make(map[string]*metadata.Package),
		CallGraph: []metadata.CallGraphEdge{},
	}

	data := server.generatePaginatedData(1, 10, 3, nil, nil, nil, nil, nil, nil, "")
	if data == nil {
		t.Fatal("Expected non-nil data with metadata")
	}
}

func TestWriteJSON(t *testing.T) {
	server := newTestServer()

	w := httptest.NewRecorder()
	server.writeJSON(w, map[string]string{"test": "value"})

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	if w.Header().Get("Content-Type") != "application/json" {
		t.Errorf("Expected Content-Type application/json, got %s", w.Header().Get("Content-Type"))
	}

	var response map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Expected valid JSON, got %v", err)
	}
	if response["test"] != "value" {
		t.Errorf("Expected test value, got %v", response["test"])
	}
}

func TestWriteResponse(t *testing.T) {
	server := newTestServer()

	w := httptest.NewRecorder()
	server.writeResponse(w, "Test message", "application/json")

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
	if w.Body.String() != "Test message" {
		t.Errorf("Expected body 'Test message', got %q", w.Body.String())
	}
}

func TestWriteError(t *testing.T) {
	server := newTestServer()

	w := httptest.NewRecorder()
	server.writeError(w, "Test error", http.StatusInternalServerError)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("Expected status 500, got %d", w.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Expected valid JSON, got %v", err)
	}
	if response["message"] != "Test error" {
		t.Errorf("Expected message 'Test error', got %v", response["message"])
	}
}

func TestNodeMatchesFiltersScope(t *testing.T) {
	// Using empty node — scope filter should fall through.
	cases := []struct {
		scope string
		want  bool
	}{
		{"", true},
		{"all", true},
		{"exported", false},   // node.Scope is "" → not exported
		{"unexported", false}, // node.Scope is "" → not unexported
	}
	for _, tc := range cases {
		got := nodeMatchesFilters(emptyNode(), nil, nil, nil, nil, nil, tc.scope)
		if got != tc.want {
			t.Errorf("scope=%q: want %v got %v", tc.scope, tc.want, got)
		}
	}
}

func TestGzipMiddlewareCompresses(t *testing.T) {
	server := newTestServer()
	server.metadata = &metadata.Metadata{
		Packages:  make(map[string]*metadata.Package),
		CallGraph: []metadata.CallGraphEdge{},
	}

	mux := http.NewServeMux()
	server.RegisterRoutes(mux, RouteOptions{})

	req := httptest.NewRequest(http.MethodGet, "/api/diagram/stats", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200, got %d", w.Code)
	}
	if w.Header().Get("Content-Encoding") != "gzip" {
		t.Fatalf("Expected Content-Encoding: gzip, got %q", w.Header().Get("Content-Encoding"))
	}
	gr, err := gzip.NewReader(w.Body)
	if err != nil {
		t.Fatalf("gzip.NewReader: %v", err)
	}
	defer func() { _ = gr.Close() }()
	body, err := io.ReadAll(gr)
	if err != nil {
		t.Fatalf("read decompressed body: %v", err)
	}
	if !strings.Contains(string(body), "total_nodes") {
		t.Errorf("Decompressed body missing expected field: %s", body)
	}
}

func TestGzipMiddlewarePassThrough(t *testing.T) {
	server := newTestServer()
	server.metadata = &metadata.Metadata{
		Packages:  make(map[string]*metadata.Package),
		CallGraph: []metadata.CallGraphEdge{},
	}

	mux := http.NewServeMux()
	server.RegisterRoutes(mux, RouteOptions{})

	// No Accept-Encoding: should not compress.
	req := httptest.NewRequest(http.MethodGet, "/api/diagram/stats", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Header().Get("Content-Encoding") == "gzip" {
		t.Errorf("Should not gzip when client did not advertise support")
	}
	if !strings.Contains(w.Body.String(), "total_nodes") {
		t.Errorf("Body missing expected field: %s", w.Body.String())
	}
}

func TestSplitCSV(t *testing.T) {
	cases := map[string]int{
		"":      0,
		"a":     1,
		"a,b":   2,
		"a,b,c": 3,
	}
	for in, want := range cases {
		got := splitCSV(in)
		if len(got) != want {
			t.Errorf("splitCSV(%q): want %d parts, got %d (%v)", in, want, len(got), got)
		}
	}
}
