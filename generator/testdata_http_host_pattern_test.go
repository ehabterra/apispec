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
func TestTestdata_HTTPHostPattern(t *testing.T) {
	out := loadTestdata(t, "http_host_pattern", spec.DefaultHTTPConfig())
	noDanglingRefs(t, out)

	for _, p := range []string{"/items", "/items/new", "/debug", "/health", "/status"} {
		if !hasPath(out, p) {
			t.Errorf("path %q missing; have %v", p, mapPathKeys(out.Paths))
		}
	}

	// The defect, stated directly: no path may carry a host.
	for p := range out.Paths {
		for _, host := range []string{"api.example.com", "admin.example.com", "localhost"} {
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
