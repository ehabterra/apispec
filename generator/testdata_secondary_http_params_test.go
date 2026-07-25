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

import "testing"

// TestTestdata_SecondaryHTTPFormAndCookie covers the two patterns
// HTTPSecondaryConfig gained with the #211 scoping: the *http.Request FormValue
// and Cookie reads, which its comment had deferred "until they gain receiver
// scoping".
//
// The net/http surface is layered under every non-net/http framework, so these
// patterns reach real code only through that merge. The cookie read is what
// isolates it: chi (the fixture's primary) has a FormValue pattern of its own but
// **no Cookie pattern at all**, so a resolved `session` cookie parameter can only
// have come from the merged net/http config. FormValue is asserted alongside it
// for the end-to-end shape; which of the two configs supplied it is settled at
// the merge layer instead (TestMergeFrameworkConfigs).
func TestTestdata_SecondaryHTTPFormAndCookie(t *testing.T) {
	out := loadTestdataWithFixtureConfig(t, "secondary_framework_params", nil)
	noDanglingRefs(t, out)
	noUnresolvedPlaceholders(t, out)

	get := opFor(out.Paths["/admin/search"], "GET")
	if get == nil {
		t.Fatalf("GET /admin/search missing; have %v", mapPathKeys(out.Paths))
	}

	paramIn := map[string]string{}
	for _, p := range get.Parameters {
		paramIn[p.Name] = p.In
	}
	// FormValue on a GET resolves to a query parameter — Go's FormValue reads
	// the URL query for body-less methods (issue #171).
	if got := paramIn["q"]; got != "query" {
		t.Errorf("GET /admin/search: parameter %q in=%q, want query; have %v", "q", got, paramIn)
	}
	if got := paramIn["session"]; got != "cookie" {
		t.Errorf("GET /admin/search: parameter %q in=%q, want cookie — the net/http Cookie pattern "+
			"was dropped from the merged secondary config (#211); have %v", "session", got, paramIn)
	}
}
