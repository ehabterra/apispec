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

// TestTestdata_MockNamedHandlers guards issue #279: metadata used to drop any
// declaration — and every call made from it — whose identifier contained
// "mock", "fake" or "stub" as a substring. Production endpoints named after
// what they do (a fake-door A/B test, a sandbox tenant serving canned data, an
// indicative "stub" price) came out as empty operations: a documented route
// with `type: object` where its plainly-named twin resolved a $ref.
//
// Every route in the fixture returns either Widget or StubQuote, so the
// assertion is simply that the colliding names resolve the same schema as the
// control route — no name may change what a handler documents.
func TestTestdata_MockNamedHandlers(t *testing.T) {
	out := loadTestdataWithFixtureConfig(t, "mock_named_handlers", spec.DefaultHTTPConfig())
	noDanglingRefs(t, out)
	noUnresolvedPlaceholders(t, out)

	cases := []struct {
		path, component, why string
	}{
		{"/health", "Widget", "control: no colliding identifier anywhere"},
		{"/fake-door", "Widget", "colliding function name (also the calling function)"},
		{"/sandbox/data", "Widget", "colliding receiver type on a handler method"},
		{"/quote", "StubQuote", "colliding response type name"},
		{"/indicative", "StubQuote", "colliding callee name in the response helper"},
	}

	for _, tc := range cases {
		item, ok := out.Paths[tc.path]
		if !ok {
			t.Errorf("GET %s missing (%s); have %v", tc.path, tc.why, mapPathKeys(out.Paths))
			continue
		}
		op := opFor(item, "GET")
		if op == nil {
			t.Errorf("GET %s has no GET operation (%s)", tc.path, tc.why)
			continue
		}
		// The fixture's handlers write without an explicit status, so which
		// status slot the body lands in is not what this test is about.
		if !responseSchemaRefs(op.Responses, tc.component) {
			t.Errorf("GET %s does not document %s (%s); the handler body was erased",
				tc.path, tc.component, tc.why)
		}
	}
}
