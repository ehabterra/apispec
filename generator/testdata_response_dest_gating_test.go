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

// TestTestdata_ResponseDestGating covers issue #170: a value encoded to some
// other io.Writer (a bytes.Buffer, a hash) must not be attributed as the
// operation response — only a value whose encoder destination traces to the
// HTTP response writer counts. User (encoded to w, directly and through a
// wrapper) must appear; Secret/Audit (encoded to a buffer/hash) must not appear
// anywhere in the document.
func TestTestdata_ResponseDestGating(t *testing.T) {
	out := loadTestdataWithFixtureConfig(t, "response_dest_gating", spec.DefaultHTTPConfig())
	noDanglingRefs(t, out)
	noUnresolvedPlaceholders(t, out)

	// The whole document must never mention the leaked types.
	blob, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("marshal spec: %v", err)
	}
	doc := string(blob)
	for _, leaked := range []string{"Secret", "Audit"} {
		if strings.Contains(doc, leaked) {
			t.Errorf("leaked type %q reached the spec — write-destination gating failed", leaked)
		}
	}
	if !strings.Contains(doc, "User") {
		t.Error("expected the real response type User to be present")
	}

	// The two writer-destination routes carry a User response ref.
	for _, path := range []string{"/user", "/user-helper"} {
		get := opFor(out.Paths[path], "GET")
		if get == nil {
			t.Errorf("GET %s missing; have %v", path, mapPathKeys(out.Paths))
			continue
		}
		if !responseRefsUser(get.Responses) {
			t.Errorf("GET %s: expected a User response, got %v", path, keysOf(get.Responses))
		}
	}

	// The leak routes must NOT carry any object/$ref response — their only
	// write is the plain w.Write, so a struct body would be the false positive.
	for _, path := range []string{"/leak-buffer", "/leak-hash"} {
		post := opFor(out.Paths[path], "POST")
		if post == nil {
			t.Errorf("POST %s missing; have %v", path, mapPathKeys(out.Paths))
			continue
		}
		for status, resp := range post.Responses {
			for ct, media := range resp.Content {
				if media.Schema != nil && media.Schema.Ref != "" {
					t.Errorf("POST %s [%s %s]: unexpected $ref response %q — a non-writer encode leaked",
						path, status, ct, media.Schema.Ref)
				}
			}
		}
	}
}

func responseRefsUser(responses map[string]intspec.Response) bool {
	for _, resp := range responses {
		for _, media := range resp.Content {
			if media.Schema != nil && strings.Contains(media.Schema.Ref, "User") {
				return true
			}
		}
	}
	return false
}
