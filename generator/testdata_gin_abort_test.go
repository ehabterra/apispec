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

	intspec "github.com/ehabterra/apispec/internal/spec"
)

// gin's abort family documents the status it writes, and the JSON form its
// body — directly, and through a house helper the status reaches as a
// parameter (issue #551). Before, none of them matched a response pattern and
// every such operation documented its success alone.
func TestTestdata_GinAbort(t *testing.T) {
	out := loadTestdata(t, "gin_abort", intspec.DefaultGinConfig())
	noDanglingRefs(t, out)

	for _, tc := range []struct {
		path, status string
		body         bool
	}{
		{"/json/{id}", "404", true},
		{"/status/{id}", "403", false},
		{"/error/{id}", "422", false},
		{"/helper/{id}", "409", true},
	} {
		op := opFor(out.Paths[tc.path], "GET")
		if op == nil {
			t.Errorf("GET %s missing; have %v", tc.path, mapPathKeys(out.Paths))
			continue
		}
		if _, ok := op.Responses["200"]; !ok {
			t.Errorf("GET %s lost its 200; have %v", tc.path, statusKeys(op))
		}
		resp, ok := op.Responses[tc.status]
		if !ok {
			t.Errorf("GET %s documents no %s — the status its abort writes; have %v", tc.path, tc.status, statusKeys(op))
			continue
		}
		media, hasJSON := resp.Content["application/json"]
		if tc.body {
			if !hasJSON || media.Schema == nil || !strings.HasSuffix(media.Schema.Ref, "ErrorBody") {
				t.Errorf("GET %s %s body = %+v, want the ErrorBody AbortWithStatusJSON writes", tc.path, tc.status, resp.Content)
			}
		} else if len(resp.Content) != 0 {
			t.Errorf("GET %s %s documents a body %v — this abort writes a status alone", tc.path, tc.status, resp.Content)
		}
	}
}
