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
	"sort"
	"testing"

	"github.com/ehabterra/apispec/spec"
)

// TestTestdata_StatusViaHelperChain covers status codes threaded through nested
// response helpers: respondError(w, http.StatusX, ...) -> respondJSON(w, status,
// ...) -> w.WriteHeader(status). The status is a parameter across two (and, via
// the shared writeError mapper, three) hops, so a single-hop parent lookup left
// every error at "default"; multi-hop parameter resolution recovers the real
// codes. Every branch's status must appear and none may fall into "default".
func TestTestdata_StatusViaHelperChain(t *testing.T) {
	out := loadTestdataWithFixtureConfig(t, "status_via_helper_chain", spec.DefaultHTTPConfig())
	noDanglingRefs(t, out)

	item, ok := out.Paths["/widget"]
	if !ok {
		t.Fatalf("path /widget missing; have %v", mapPathKeys(out.Paths))
	}
	get := opFor(item, "GET")
	if get == nil {
		t.Fatal("GET /widget missing")
	}

	got := keysOf(get.Responses)
	sort.Strings(got)
	// 401/400 resolve through two helper hops; 404/403/500 through the shared
	// writeError mapper (three hops); 200 through the single respondJSON hop.
	for _, want := range []string{"200", "400", "401", "403", "404", "500"} {
		if _, ok := get.Responses[want]; !ok {
			t.Errorf("GET /widget missing status %s; have %v", want, got)
		}
	}
	if _, ok := get.Responses["default"]; ok {
		t.Errorf("GET /widget still has an unresolved default response; have %v", got)
	}
}
