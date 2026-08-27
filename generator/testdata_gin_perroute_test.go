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
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/ehabterra/apispec/spec"
)

// TestTestdata_GinPerRouteHandlerAttribution locks in issue #386: gin takes its
// handler chain variadically, so the endpoint handler is the LAST argument.
//
//	r.POST("/users", jwtAuth(), rateLimit(), createUser)
//
// Reading the configured handler index instead yielded the first MIDDLEWARE, so
// the operation's identity became the middleware's: jwtAuth's name as the
// operationId and its doc comment as the summary.
//
// The auth suite pins the operationId on the single-middleware route; this adds
// the chain of TWO, which a fixed index cannot get right by accident.
//
// The body and response assertions are deliberately NOT proof of the bug —
// measured against the unfixed code they already passed, because those travel
// with the route's subtree rather than with the named handler. They are here as
// the other half of the trade: re-pointing attribution to the last argument
// must not cost the operation the payload it already documented.
func TestTestdata_GinPerRouteHandlerAttribution(t *testing.T) {
	out := loadTestdata(t, "auth_gin_perroute", spec.DefaultGinConfig())
	noDanglingRefs(t, out)
	noNullSchemas(t, out)

	item, ok := out.Paths["/users"]
	if !ok {
		t.Fatalf("POST /users missing; have %v", mapPathKeys(out.Paths))
	}
	op := opFor(item, "POST")
	if op == nil {
		t.Fatal("POST /users: no POST operation")
	}

	// Attribution: past two middlewares, not one.
	if !strings.HasSuffix(op.OperationID, ".createUser") {
		t.Errorf("operationId %q should name the handler, not a middleware of the chain", op.OperationID)
	}
	if !strings.HasPrefix(op.Summary, "createUser ") {
		t.Errorf("summary %q is not the handler's doc comment", op.Summary)
	}

	// The payload must survive the re-attribution (see the doc comment).
	if op.RequestBody == nil {
		t.Fatal("POST /users: no request body — ShouldBindJSON in the handler no longer reaches the operation")
	}
	if got := contentSchemaRef(op.RequestBody.Content); !strings.HasSuffix(got, "CreateUserRequest") {
		t.Errorf("request body schema = %q, want the handler's CreateUserRequest", got)
	}

	// The handler's own status and response type, likewise.
	resp, ok := op.Responses["201"]
	if !ok {
		t.Fatalf(`POST /users: no "201" response; have %v`, slices.Sorted(maps.Keys(op.Responses)))
	}
	if got := contentSchemaRef(resp.Content); !strings.HasSuffix(got, "_User") {
		t.Errorf("201 schema = %q, want the handler's User", got)
	}
}
