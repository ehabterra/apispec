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

	"github.com/ehabterra/apispec/internal/spec"
)

// A query key is a parameter of the operation only when the url.Values it is
// read off came from the request (issue #552). On a real project the pattern,
// matching the type alone, documented keys read off a Redis connection URI as
// query parameters of eight operations.
func TestTestdata_QueryParamOrigin(t *testing.T) {
	out := loadTestdata(t, "query_param_origin", spec.DefaultHTTPConfig())
	noDanglingRefs(t, out)

	op := opFor(out.Paths["/items"], "GET")
	if op == nil {
		t.Fatalf("GET /items missing; have %v", mapPathKeys(out.Paths))
	}
	got := map[string]bool{}
	for _, p := range op.Parameters {
		if p.In == "query" {
			got[p.Name] = true
		}
	}
	for _, name := range []string{"page", "sort", "limit", "filter", "cursor", "mq"} {
		if !got[name] {
			t.Errorf("query parameter %q missing — it is read off the request's own query; have %v", name, got)
		}
	}
	for _, name := range []string{"clientname", "region", "token", "skipverify", "outq", "sig"} {
		if got[name] {
			t.Errorf("query parameter %q documented — it is read off url.Values the client never sends", name)
		}
	}
}
