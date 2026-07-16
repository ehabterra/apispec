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

	"github.com/ehabterra/apispec/spec"
)

// This file pins KNOWN GAPS: each test asserts today's (incomplete) output
// so the gap is documented, reproducible, and fails LOUD the day the
// capability lands — at which point the assertions must flip to the
// commented expectations and the tracking issue closes.

// TestTestdata_CLIActionRoutes pins issue #143: route registration reached
// from main only through a function value stored in a composite-literal
// field (`Command{Action: runWeb}`, the urfave/cli shape used by gitea) is
// invisible — tracker roots are main functions and no static edge crosses
// the dispatcher hop, even though the registration edges exist in metadata.
//
// When #143 lands, this fixture must document /users GET+POST and this test
// must assert exactly that.
func TestTestdata_CLIActionRoutes(t *testing.T) {
	out := loadTestdataWithFixtureConfig(t, "cli_action_routes", nil)

	if len(out.Paths) != 0 {
		t.Errorf("cli_action_routes now documents %d paths (%v) — the #143 gap "+
			"seems fixed: flip this test to assert /users GET+POST and close the issue",
			len(out.Paths), mapPathKeys(out.Paths))
	}
}

// TestTestdata_StatusViaConstructor covers issue #144 (now fixed): a status
// carried through a constructor struct field (`e := NewAPIError(msg, 401);
// RespondWithError(w, e)` → `w.WriteHeader(err.Code)`) resolves to the real
// status. statusFromConstructorField follows the selector's base variable
// through the wrapper parameter to its constructor assignment, matches the
// return composite-literal field (Code ← the parameter `code`), and reads that
// parameter's actual argument at the constructor call site.
func TestTestdata_StatusViaConstructor(t *testing.T) {
	out := loadTestdataWithFixtureConfig(t, "status_via_constructor", spec.DefaultHTTPConfig())
	noDanglingRefs(t, out)

	item, ok := out.Paths["/profile"]
	if !ok {
		t.Fatalf("path /profile missing; have %v", mapPathKeys(out.Paths))
	}
	get := opFor(item, "GET")
	if get == nil {
		t.Fatal("GET /profile missing")
	}

	// The 401 now resolves and carries the APIError schema.
	resp401, ok := get.Responses["401"]
	if !ok {
		t.Fatalf("GET /profile should resolve the 401 (via the constructor field); have %v", keysOf(get.Responses))
	}
	ref := ""
	if mt, ok := resp401.Content["application/json"]; ok && mt.Schema != nil {
		ref = mt.Schema.Ref
	}
	if !strings.HasSuffix(ref, "_APIError") {
		t.Errorf("401 response should carry the APIError schema, got %q", ref)
	}

	// The success body still resolves, and the error write no longer falls
	// into the unresolved-status "default" bucket.
	if _, ok := get.Responses["200"]; !ok {
		t.Errorf("GET /profile lost its 200 response: %v", keysOf(get.Responses))
	}
	if _, ok := get.Responses["default"]; ok {
		t.Errorf("GET /profile still has a default response — the error write should now be the 401: %v", keysOf(get.Responses))
	}
}

func keysOf[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
