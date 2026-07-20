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

// TestTestdata_DownstreamClientNotResponse covers the fix for issue #195:
// response detection is anchored on the write to w, so a downstream HTTP
// client's OUTBOUND-request marshal (json.Marshal in a method with no
// ResponseWriter) is never reached from a sink and does NOT leak as a spurious
// `default` response of the outer route. The handler's only responses are the
// success 200 (its RespondWithSuccess encode) and the error 500 (http.Error).
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
	if _, ok := get.Responses["default"]; ok {
		t.Errorf("GET /pk: spurious `default` response present — the downstream client's outbound-request marshal leaked (issue #195 regressed); have %v", keysOf(get.Responses))
	}
}
