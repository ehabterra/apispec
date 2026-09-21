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

// A handler closure's local variable that no call produced must not have the
// route REGISTRATION as its producer (issue #550). It did: the variable fell
// back to the invocation of the function it is written in, `registerAPI(mux)`,
// so the helper argument reading it expanded every route registerAPI registers
// — and /items documented its sibling's redirect. Measured on a real project,
// the same mechanism put a static route's redirect, the rate limiter's 429 and
// an OAuth callback's query parameters on a passcode endpoint.
func TestTestdata_ClosureLocalProducer(t *testing.T) {
	out := loadTestdata(t, "closure_local_producer", spec.DefaultHTTPConfig())
	noDanglingRefs(t, out)

	op := opFor(out.Paths["/items"], "POST")
	if op == nil {
		t.Fatalf("POST /items missing; have %v", mapPathKeys(out.Paths))
	}
	if _, ok := op.Responses["200"]; !ok {
		t.Errorf("POST /items lost its own 200; have %v", statusKeys(op))
	}
	if _, ok := op.Responses["307"]; ok {
		t.Error("POST /items documents 307 — its sibling's redirect, reached through the registration expanded as a variable's producer")
	}

	// The sibling keeps its own redirect.
	if redirect := opFor(out.Paths["/login-redirect"], "GET"); redirect == nil {
		t.Errorf("GET /login-redirect missing; have %v", mapPathKeys(out.Paths))
	} else if _, ok := redirect.Responses["307"]; !ok {
		t.Errorf("GET /login-redirect lost its 307; have %v", statusKeys(redirect))
	}
}
