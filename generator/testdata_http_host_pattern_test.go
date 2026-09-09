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
	"strings"
	"testing"

	"github.com/ehabterra/apispec/internal/spec"
)

// A Go 1.22 ServeMux pattern is "[METHOD ][HOST]/[PATH]". The host selects
// which mux entry serves the request; it is not part of the URL a client asks
// for. Folded into the path it produced `/api.example.com/items` — an endpoint
// that does not exist and that no consumer can call, which is worse than a
// missing one because nothing in the document says it is wrong (issue #356).
//
// The hosts are DECLARED, not detected: a path reaching the matcher may have
// lost its leading slash, so no shape test can separate a host from a real
// segment (golden rule #7). This test therefore runs the fixture twice — once
// with the hosts configured, once without — because the default behaviour is
// part of the contract.
func TestTestdata_HTTPHostPattern(t *testing.T) {
	cfg := spec.DefaultHTTPConfig()
	cfg.Hosts = []string{"api.example.com", "admin.example.com", "localhost:8080"}
	out := loadTestdata(t, "http_host_pattern", cfg)
	noDanglingRefs(t, out)

	for _, p := range []string{"/items", "/items/new", "/debug", "/health", "/status"} {
		if !hasPath(out, p) {
			t.Errorf("path %q missing; have %v", p, mapPathKeys(out.Paths))
		}
	}

	// The defect, stated directly: no path may carry a declared host.
	for p := range out.Paths {
		for _, host := range cfg.Hosts {
			if strings.Contains(p, host) {
				t.Errorf("path %q carries the host — the pattern's host component is not part of "+
					"the requested URL", p)
			}
		}
	}

	// A pattern with no host is untouched, so the split cannot cost the paths
	// that were already right.
	if !hasPath(out, "/health") {
		t.Error("/health missing — a hostless pattern must pass through unchanged")
	}

	// Two hosts serving the same path collapse into ONE operation, because a
	// path is the whole key and the host has nowhere to go yet. Pinned rather
	// than hidden: documenting both needs `servers`, which is the part of #356
	// this change deliberately leaves open. When that lands, /status carries a
	// server per host and this expectation changes.
	status, ok := out.Paths["/status"]
	if !ok {
		t.Fatal("/status missing")
	}
	if op := opFor(status, "GET"); op == nil {
		t.Error("/status has no GET")
	}
}

// Declaring the servers is declaring the hosts: a project that already lists
// `servers` should not have to name the same hosts twice.
func TestTestdata_HTTPHostPatternFromServers(t *testing.T) {
	cfg := spec.DefaultHTTPConfig()
	cfg.Servers = []spec.Server{{URL: "https://api.example.com/"}}
	out := loadTestdata(t, "http_host_pattern", cfg)

	if !hasPath(out, "/items") {
		t.Errorf("/items missing; a host named by a server URL must be split off too. have %v",
			mapPathKeys(out.Paths))
	}
	// A host NOT named anywhere is still left alone — the point of declaring.
	if !hasPath(out, "/admin.example.com/status") {
		t.Errorf("an undeclared host must be left exactly as it came; have %v", mapPathKeys(out.Paths))
	}
}

// With nothing declared, nothing is split. The default is today's behaviour
// rather than a guess, so the invalid path stays until the project says which
// hosts are its own — pinned so the default is a decision, not an accident.
func TestTestdata_HTTPHostPatternUndeclared(t *testing.T) {
	out := loadTestdata(t, "http_host_pattern", spec.DefaultHTTPConfig())

	if !hasPath(out, "/api.example.com/items") {
		t.Errorf("with no hosts declared the pattern must pass through unchanged; have %v",
			mapPathKeys(out.Paths))
	}
	if hasPath(out, "/items") {
		t.Error("a host was split off without being declared — that is the guess this design refuses")
	}
}
