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

	"github.com/ehabterra/apispec/internal/spec"
)

// A generic handler adapter — `HandleJSON[Req, Res any](fn) http.HandlerFunc` —
// is the shape several Go HTTP toolkits ship and many projects hand-roll once
// and reuse everywhere. Its response type is a TYPE PARAMETER, and the
// parameter's own name used to reach the schema mapper: every operation built
// through the adapter `$ref`ed one component named `..._Res`, described as
// "unresolved type", so endpoints with entirely different response types all
// claimed the same schema (issue #367).
//
// That is worse than documenting nothing. A consumer generating clients from
// the document gets one wrong Go type for all of them, with nothing saying so.
func TestTestdata_GenericHandlerAdapter(t *testing.T) {
	out := loadTestdata(t, "generic_handler_adapter", spec.DefaultHTTPConfig())
	noDanglingRefs(t, out)

	// Each operation documents ITS OWN instantiation. Two of these go through
	// the same adapter, which is the point: the binding is per registration.
	cases := []struct {
		path, method, want string
	}{
		{"/users", "POST", "User"},        // HandleJSON[CreateUserRequest, User]
		{"/users", "GET", "ListResponse"}, // HandleJSONResponse[ListResponse]
		{"/health", "GET", "Health"},      // HandleJSONResponse[Health]
	}
	for _, tc := range cases {
		item, ok := out.Paths[tc.path]
		if !ok {
			t.Errorf("%s missing; have %v", tc.path, mapPathKeys(out.Paths))
			continue
		}
		op := opFor(item, tc.method)
		if op == nil {
			t.Errorf("%s %s missing", tc.method, tc.path)
			continue
		}
		resp, ok := op.Responses["200"]
		if !ok {
			t.Errorf("%s %s: no 200; have %v", tc.method, tc.path, sortedStatusKeys(op))
			continue
		}
		ref := ""
		for _, mt := range resp.Content {
			if mt.Schema != nil {
				ref = mt.Schema.Ref
			}
		}
		if !strings.HasSuffix(ref, "_"+tc.want) {
			t.Errorf("%s %s 200 schema = %q, want the instantiated %s — the response type is the "+
				"adapter's type PARAMETER, resolved from this registration's instantiation",
				tc.method, tc.path, ref, tc.want)
		}
	}

	// The request side, which already worked, must keep working: the fix reads
	// the same bindings.
	if post := opFor(out.Paths["/users"], "POST"); post != nil && post.RequestBody != nil {
		ref := ""
		for _, mt := range post.RequestBody.Content {
			if mt.Schema != nil {
				ref = mt.Schema.Ref
			}
		}
		if !strings.HasSuffix(ref, "_CreateUserRequest") {
			t.Errorf("POST /users request body = %q, want CreateUserRequest", ref)
		}
	}

	// No component may be named after a type parameter. This is the assertion
	// that fails loudest on the unfixed code, and it is phrased on the SHAPE
	// rather than on the one name, so an adapter spelled `[In, Out]` is caught
	// too.
	if out.Components != nil {
		for name := range out.Components.Schemas {
			for _, param := range []string{"_Res", "_Req", "_In", "_Out", "_T"} {
				if strings.HasSuffix(name, param) {
					t.Errorf("component %q is named after a type parameter — it describes nothing, "+
						"and every operation through the adapter would reference it", name)
				}
			}
		}
		// And the real types are present, since the whole point is that they
		// reach the document.
		for _, want := range []string{"User", "ListResponse", "Health", "CreateUserRequest"} {
			found := false
			for name := range out.Components.Schemas {
				if strings.HasSuffix(name, "_"+want) {
					found = true
				}
			}
			if !found {
				t.Errorf("%s is not in components; the instantiated type never reached the document", want)
			}
		}
	}
}
