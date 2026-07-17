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
	"encoding/json"
	"strings"
	"testing"

	intspec "github.com/ehabterra/apispec/internal/spec"
	"github.com/ehabterra/apispec/spec"
)

// TestTestdata_ResponseWriterProvenance covers issue #170: an encode is the
// operation response only when its destination traces (through parameters,
// assignments, and struct construction) to the handler's response-writer
// parameter w. The KEEP routes cover direct/helper/assign/wrapper provenance to
// w; the DROP routes cover buffer/hash/discard/constructor sinks. Secret must
// never reach the spec.
func TestTestdata_ResponseWriterProvenance(t *testing.T) {
	out := loadTestdataWithFixtureConfig(t, "response_writer_provenance", spec.DefaultHTTPConfig())
	noDanglingRefs(t, out)
	noUnresolvedPlaceholders(t, out)

	// Secret is only ever encoded to non-writer sinks — it must not appear.
	if blob, err := json.Marshal(out); err == nil && strings.Contains(string(blob), "Secret") {
		t.Error("Secret leaked into the spec — a non-writer destination was treated as the response")
	}

	// KEEP: destination traces to w → User response present.
	for _, path := range []string{"/direct", "/helper", "/assign", "/wrapper"} {
		get := opFor(out.Paths[path], "GET")
		if get == nil {
			t.Errorf("GET %s missing; have %v", path, mapPathKeys(out.Paths))
			continue
		}
		if !responseRefsType(get.Responses, "User") {
			t.Errorf("GET %s: expected a User response (destination traces to w), got %v",
				path, keysOf(get.Responses))
		}
	}

	// DROP: destination is a sink → no struct/$ref response.
	for _, path := range []string{"/leak-buffer", "/leak-hash", "/leak-discard", "/leak-constructed", "/leak-recorder"} {
		post := opFor(out.Paths[path], "POST")
		if post == nil {
			t.Errorf("POST %s missing; have %v", path, mapPathKeys(out.Paths))
			continue
		}
		for status, resp := range post.Responses {
			for ct, media := range resp.Content {
				if media.Schema != nil && media.Schema.Ref != "" {
					t.Errorf("POST %s [%s %s]: unexpected $ref response %q — a non-writer destination leaked",
						path, status, ct, media.Schema.Ref)
				}
			}
		}
	}
}

func responseRefsType(responses map[string]intspec.Response, name string) bool {
	for _, resp := range responses {
		for _, media := range resp.Content {
			if media.Schema != nil && strings.Contains(media.Schema.Ref, name) {
				return true
			}
		}
	}
	return false
}
