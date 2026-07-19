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
	"testing"

	"github.com/ehabterra/apispec/spec"
)

// TestTestdata_DownstreamClientNotResponse pins the KNOWN over-detection bug of
// issue #195: json.Marshal is matched as a response with no write-sink check, so
// a downstream HTTP client's OUTBOUND-request marshal (in a method with no
// ResponseWriter) leaks as a spurious `default` response of the outer route.
// The handler's real responses are the success 200 and the error 500.
//
// This is a CHANGE-DETECTOR for the unsolved bug: it asserts the spurious
// `default` is still present. When #195's root-cause fix lands (anchor response
// detection on the write to w and trace the written value's type back), the
// spurious default disappears — this test then fails, prompting the update to
// require exactly {200, 500}.
func TestTestdata_DownstreamClientNotResponse(t *testing.T) {
	out := loadTestdataWithFixtureConfig(t, "downstream_client_not_response", spec.DefaultHTTPConfig())
	noDanglingRefs(t, out)

	get := opFor(out.Paths["/pk"], "GET")
	if get == nil {
		t.Fatalf("GET /pk missing; have %v", mapPathKeys(out.Paths))
	}
	for _, want := range []string{"200", "500"} {
		if _, ok := get.Responses[want]; !ok {
			t.Errorf("GET /pk missing status %s; have %v", want, keysOf(get.Responses))
		}
	}
	if _, ok := get.Responses["default"]; !ok {
		t.Errorf("issue #195 appears FIXED (no spurious `default` on GET /pk) — update this test to require exactly {200,500}")
	}
}
