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

// TestTestdata_MapperFieldStatus covers issue #187: the status handed to
// http.Error is a struct field (api.Status) whose value is set across the return
// branches of an error mapper (api := MapError(err)). It must resolve to the
// concrete branch statuses {400, 404, 500} — recursing through the per-status
// helper mappers and reading positional struct literals — not a bare default.
func TestTestdata_MapperFieldStatus(t *testing.T) {
	out := loadTestdataWithFixtureConfig(t, "mapper_field_status", spec.DefaultHTTPConfig())
	noDanglingRefs(t, out)
	noUnresolvedPlaceholders(t, out)

	get := opFor(out.Paths["/thing"], "GET")
	if get == nil {
		t.Fatalf("GET /thing missing; have %v", mapPathKeys(out.Paths))
	}
	got := keysOf(get.Responses)
	sort.Strings(got)
	// The mapper resolves to exactly {400, 404, 500}; any extra status (or a
	// spurious success code) is a regression.
	if len(got) != 3 {
		t.Fatalf("GET /thing responses = %v, want exactly [400 404 500]", got)
	}
	for _, want := range []string{"400", "404", "500"} {
		if _, ok := get.Responses[want]; !ok {
			t.Errorf("GET /thing missing status %s; have %v", want, got)
		}
	}
	if _, ok := get.Responses["default"]; ok {
		t.Errorf("GET /thing still has an unresolved default; the mapper statuses should be concrete; have %v", got)
	}
}
