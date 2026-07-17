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

// TestTestdata_BranchedStatusConstructor covers issue #155: a status assigned
// across switch/if branches then handed to an error constructor must resolve to
// the concrete branch codes, not a single `default`.
func TestTestdata_BranchedStatusConstructor(t *testing.T) {
	out := loadTestdataWithFixtureConfig(t, "branched_status_constructor", spec.DefaultHTTPConfig())
	noDanglingRefs(t, out)
	noUnresolvedPlaceholders(t, out)

	// GET /thing reports errors through writeError (inline constructor); its
	// switch sets {404, 400, 500}.
	get := opFor(out.Paths["/thing"], "GET")
	if get == nil {
		t.Fatalf("GET /thing missing; have %v", mapPathKeys(out.Paths))
	}
	got := keysOf(get.Responses)
	sort.Strings(got)
	for _, want := range []string{"400", "404", "500"} {
		if _, ok := get.Responses[want]; !ok {
			t.Errorf("GET /thing missing status %s; have %v", want, got)
		}
	}
	if _, ok := get.Responses["default"]; ok {
		t.Errorf("GET /thing still has an unresolved default; the branch statuses should be concrete; have %v", got)
	}

	// GET /other uses the variable form (e := NewAPIError(...)); its if sets
	// {404, 500}.
	other := opFor(out.Paths["/other"], "GET")
	if other == nil {
		t.Fatalf("GET /other missing; have %v", mapPathKeys(out.Paths))
	}
	for _, want := range []string{"404", "500"} {
		if _, ok := other.Responses[want]; !ok {
			t.Errorf("GET /other missing status %s; have %v", want, keysOf(other.Responses))
		}
	}
}
