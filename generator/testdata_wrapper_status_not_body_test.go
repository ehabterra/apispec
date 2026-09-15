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

// An error helper that derives its message FROM its status makes the wrapper
// derivation read a status constant as the payload:
//
//	func (b *Base) HTTPError(status int, contents ...string) {
//		v := http.StatusText(status)   // v traces back to `status`
//		if len(contents) > 0 {
//			v = contents[0]
//		}
//		http.Error(b.Resp, v, status)
//	}
//
// The inner http.Error names its body at argument 1 and its status at 2, both
// correct. Mapped back onto HTTPError's parameters the status lands on `status`
// — right — and the body follows `v` through the first assignment it finds and
// lands on `status` too. The derived pattern then documents a status CONSTANT as
// an application/json body, which on a large real project was eighteen error
// responses claiming `{type: integer}` bodies they never send (issue #485).
//
// One parameter cannot be both the status and the body, so the derivation now
// declines the body rather than guessing which branch of that assignment was
// meant.
func TestTestdata_WrapperStatusNotBody(t *testing.T) {
	out := loadTestdata(t, "wrapper_status_not_body", spec.DefaultHTTPConfig())
	noDanglingRefs(t, out)

	items, ok := out.Paths["/items"]
	if !ok {
		t.Fatalf("/items missing; have %v", mapPathKeys(out.Paths))
	}
	op := opFor(items, "GET")
	if op == nil {
		t.Fatal("/items: no GET operation")
	}

	// The error response is what http.Error writes: a plain-text message. The
	// status constant is not a second representation of it.
	resp, ok := op.Responses["422"]
	if !ok {
		t.Fatalf("no 422; have %v", sortedStatusKeys(op))
	}
	for ct, mt := range resp.Content {
		if ct == "application/json" {
			t.Errorf("422 documents an %s body — the only value at that status is the message "+
				"http.Error writes as text/plain; a JSON body here is the status constant "+
				"read as the payload", ct)
		}
		if mt.Schema != nil && mt.Schema.Type == "integer" {
			t.Errorf("422 documents an integer body (%s) — that is the status code itself", ct)
		}
	}
	if _, ok := resp.Content["text/plain; charset=utf-8"]; !ok {
		t.Errorf("422 lost its real body; content = %v — declining the mis-resolved one must "+
			"not cost the message the handler does send", contentTypesOf(op, "422"))
	}

	// The success body is untouched: this rejects one derived role, not the
	// responder.
	if got := contentTypesOf(op, "200"); len(got) != 1 || got[0] != "application/json" {
		t.Errorf("200 content = %v, want the JSON item body", got)
	}
}
