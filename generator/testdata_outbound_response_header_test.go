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

// A header is a parameter of the operation only when the http.Header it is
// read off came from the request (issue #569), the header counterpart of
// #552's query rule. On a real service one outbound client's Retry-After
// handling put a request header on every operation that reached it.
func TestTestdata_OutboundResponseHeader(t *testing.T) {
	cases := []struct {
		fixture string
		cfg     *spec.APISpecConfig
		want    []string
		reject  []string
	}{
		{
			fixture: "outbound_response_header",
			cfg:     spec.DefaultHTTPConfig(),
			// X-Ctx-Out is the change-detector for #574: read back off an
			// outbound request built with r.Context(), it is still documented.
			// Move it to reject when #574 is fixed.
			want:   []string{"X-Request-Id", "X-Trace", "Accept-Language", "X-Tenant", "X-Ctx-Out"},
			reject: []string{"X-Rate-Remaining", "Retry-After", "X-Upstream", "X-Out", "X-Written", "X-Ctx-Upstream"},
		},
		{
			// The request reaches a gin handler through c.Request, not as a
			// parameter of its own. The header patterns are net/http's, merged
			// under gin as the CLI composes them.
			fixture: "outbound_response_header_gin",
			cfg:     spec.MergeFrameworkConfigs(spec.DefaultGinConfig(), spec.HTTPSecondaryConfig()),
			want:    []string{"X-Request-Id", "X-Trace"},
			reject:  []string{"X-Rate-Remaining", "Retry-After"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.fixture, func(t *testing.T) {
			out := loadTestdata(t, tc.fixture, tc.cfg)
			noDanglingRefs(t, out)

			op := opFor(out.Paths["/items"], "GET")
			if op == nil {
				t.Fatalf("GET /items missing; have %v", mapPathKeys(out.Paths))
			}
			got := map[string]bool{}
			for _, p := range op.Parameters {
				if p.In == "header" {
					got[p.Name] = true
				}
			}
			for _, name := range tc.want {
				if !got[name] {
					t.Errorf("header %q missing — it is read off the request's own headers; have %v", name, got)
				}
			}
			for _, name := range tc.reject {
				if got[name] {
					t.Errorf("header %q documented — it is read off a header map the client never sends", name)
				}
			}
		})
	}
}
