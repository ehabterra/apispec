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
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ehabterra/apispec/internal/metadata"
	"github.com/ehabterra/apispec/internal/spec"
)

func injectedServer(t *testing.T) *Server {
	t.Helper()
	meta, err := metadata.LoadMetadata("../../testdata/echo/metadata.yaml")
	if err != nil {
		t.Skipf("fixture unavailable: %v", err)
	}
	meta.BuildCallGraphMaps()
	s := New(&Config{Host: "localhost", Port: 8080, DiagramType: "call-graph", PageSize: 50, MaxDepth: 3})
	s.metadata = meta
	s.cache = map[string]*spec.PaginatedCytoscapeData{}
	s.dataCache = map[string]*spec.CytoscapeData{}
	return s
}

func TestDiagServerHandlers_WithMetadata(t *testing.T) {
	s := injectedServer(t)
	mux := http.NewServeMux()
	s.RegisterRoutes(mux, RouteOptions{UIPath: "/", APIPrefix: "/api/diagram", HealthPath: "/health"})

	get := func(path string) int {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		return w.Code
	}
	paths := []string{
		"/api/diagram",
		"/api/diagram/page?page=1&size=20&depth=2",
		"/api/diagram/page?package=echo&function=Handler&scope=exported&receiver=Handler&signature=func",
		"/api/diagram/packages",
		"/api/diagram/by-packages",
		"/api/diagram/stats",
		"/api/diagram/export?format=json",
		"/health",
	}
	for _, p := range paths {
		if code := get(p); code >= 500 {
			t.Errorf("GET %s -> %d", p, code)
		}
	}
	// Invalid export format → 400.
	if code := get("/api/diagram/export?format=bogus"); code != http.StatusBadRequest {
		t.Errorf("bogus export format -> %d, want 400", code)
	}
}

func TestDiagServerHelpers(t *testing.T) {
	if got := splitCSV(""); len(got) != 0 {
		t.Errorf("splitCSV(empty) = %v", got)
	}
	if got := splitCSVTrim(" a , b ,c "); len(got) != 3 || got[0] != "a" || got[1] != "b" || got[2] != "c" {
		t.Errorf("splitCSVTrim = %v", got)
	}
	if got := splitCSVTrim(""); len(got) != 0 {
		t.Errorf("splitCSVTrim(empty) = %v", got)
	}
	if !matchesFunctionName("pkg.Handler.Create", "create") {
		t.Error("matchesFunctionName should match case-insensitively")
	}
	if matchesFunctionName("pkg.Handler.Create", "") {
		t.Error("empty search term should not match")
	}

	node := spec.CytoscapeNode{Data: spec.CytoscapeNodeData{
		Label:        "pkg.Handler.Create",
		Position:     "handler.go:10",
		ReceiverType: "Handler",
		SignatureStr: "func(ctx) error",
		Generics:     map[string]string{"T": "int"},
		Scope:        "exported",
	}}
	cases := []struct {
		name               string
		fn, fl, rc, sg, gn []string
		scope              string
		want               bool
	}{
		{"no filters", nil, nil, nil, nil, nil, "", true},
		{"func match", []string{"create"}, nil, nil, nil, nil, "", true},
		{"func miss", []string{"zzz"}, nil, nil, nil, nil, "", false},
		{"file match", nil, []string{"handler.go"}, nil, nil, nil, "", true},
		{"file miss", nil, []string{"other.go"}, nil, nil, nil, "", false},
		{"receiver match", nil, nil, []string{"handler"}, nil, nil, "", true},
		{"receiver miss", nil, nil, []string{"xxx"}, nil, nil, "", false},
		{"sig match", nil, nil, nil, []string{"func"}, nil, "", true},
		{"generic match", nil, nil, nil, nil, []string{"int"}, "", true},
		{"scope exported ok", nil, nil, nil, nil, nil, "exported", true},
		{"scope unexported no", nil, nil, nil, nil, nil, "unexported", false},
		{"scope all", nil, nil, nil, nil, nil, "all", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := nodeMatchesFilters(node, c.fn, c.fl, c.rc, c.sg, c.gn, c.scope); got != c.want {
				t.Errorf("nodeMatchesFilters = %v, want %v", got, c.want)
			}
		})
	}
}

func TestDiagServerLoadAndRefresh(t *testing.T) {
	s := New(&Config{
		Host: "localhost", Port: 8080, DiagramType: "call-graph",
		InputDir: "../../testdata/echo", PageSize: 50, MaxDepth: 3,
	})
	if err := s.LoadMetadata(); err != nil {
		t.Skipf("engine generate unavailable: %v", err)
	}
	mux := http.NewServeMux()
	s.RegisterRoutes(mux, RouteOptions{UIPath: "/", APIPrefix: "/api/diagram", HealthPath: "/health"})

	// stats with real metadata
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/diagram/stats", nil))
	if w.Code != http.StatusOK {
		t.Errorf("stats -> %d", w.Code)
	}
	// refresh reloads metadata from InputDir
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/diagram/refresh", nil))
	if w.Code >= 500 {
		t.Errorf("refresh -> %d", w.Code)
	}
}
