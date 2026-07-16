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

	"github.com/ehabterra/apispec/spec"
)

// TestTestdata_RequestBodyVarDecoder covers a request body decoded through a
// helper that assigns the decoder to a local variable (dec := json.NewDecoder(
// r.Body); dec.Decode(dst)). The intermediate variable re-homes dec.Decode
// under the json.NewDecoder producer, so the wrapper's dst parameter could not
// be traced to the caller's concrete request type — the body collapsed to a
// generic object. It must resolve to a $ref instead.
func TestTestdata_RequestBodyVarDecoder(t *testing.T) {
	out := loadTestdataWithFixtureConfig(t, "request_body_var_decoder", spec.DefaultHTTPConfig())
	noDanglingRefs(t, out)
	noUnresolvedPlaceholders(t, out)

	cases := []struct {
		path, method, wantSuffix string
	}{
		{"/users", "POST", "_CreateUserRequest"},
		{"/users/{id}", "PUT", "_UpdateUserRequest"},
	}
	for _, c := range cases {
		item, ok := out.Paths[c.path]
		if !ok {
			t.Errorf("path %q missing; have %v", c.path, mapPathKeys(out.Paths))
			continue
		}
		op := opFor(item, c.method)
		if op == nil {
			t.Errorf("%s %s missing", c.method, c.path)
			continue
		}
		if op.RequestBody == nil {
			t.Errorf("%s %s: no requestBody", c.method, c.path)
			continue
		}
		mt, ok := op.RequestBody.Content["application/json"]
		if !ok || mt.Schema == nil {
			t.Errorf("%s %s: no application/json schema", c.method, c.path)
			continue
		}
		if !strings.HasSuffix(mt.Schema.Ref, c.wantSuffix) {
			t.Errorf("%s %s: request body should $ref a schema ending %q, got ref=%q type=%q",
				c.method, c.path, c.wantSuffix, mt.Schema.Ref, mt.Schema.Type)
		}
	}
}
