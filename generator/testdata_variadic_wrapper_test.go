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

// A house router that forwards its handler chain variadically —
//
//	func (r *Router) Get(path string, handlers ...gin.HandlerFunc) {
//		r.engine.GET(path, handlers...)
//	}
//
// derives its pattern from an inner call written ONCE, `GET(path, handlers...)`,
// which has two arguments whatever the caller passes. That fixed the handler at
// index 1, so a later `r.Get("/users", auth, endpoint)` read the MIDDLEWARE and
// built the whole operation from it — auth's doc comment as the summary, its
// parameter reads as the operation's, and endpoint's responses missing. The same
// symptom #386 fixed for direct registrations, reached through the wrapper
// wiring style that golden rule #5 requires be covered alongside it (issue #416).
//
// The derived pattern now carries HandlerArgFromEnd when the parameter it mapped
// to is variadic. Issue #416 filed that as blocked — `...T` collapses to `[]T`
// in the type model, surviving only in the rendered signature string, which
// golden rule #2 forbids parsing. True of TypeRef, and not the whole picture:
// the parameter's own CallArgument in the recorded signature keeps
// KindEllipsis, where a `[]T` parameter gets KindArrayType. The fact was already
// recorded, on the parameter rather than on the type.
func TestTestdata_VariadicWrapper(t *testing.T) {
	out := loadTestdata(t, "variadic_wrapper", spec.DefaultGinConfig())
	noDanglingRefs(t, out)

	// The unconstrained route is correct either way: one handler, so the fixed
	// index and the last argument are the same argument.
	health, ok := out.Paths["/health"]
	if !ok {
		t.Fatalf("/health missing; have %v", mapPathKeys(out.Paths))
	}
	if op := opFor(health, "GET"); op == nil {
		t.Error("/health: no GET operation")
	} else if !strings.HasSuffix(op.OperationID, ".plain") {
		t.Errorf("/health operationId = %q, want the handler `plain`", op.OperationID)
	}

	users, ok := out.Paths["/users"]
	if !ok {
		t.Fatalf("/users missing; have %v", mapPathKeys(out.Paths))
	}
	op := opFor(users, "GET")
	if op == nil {
		t.Fatal("/users: no GET operation")
	}
	if !strings.HasSuffix(op.OperationID, ".endpoint") {
		t.Errorf("/users operationId = %q, want the endpoint handler — a variadic chain puts the "+
			"handler LAST, and everything about the operation hangs off which argument is read",
			op.OperationID)
	}

	// The whole operation, not just its name: reading the middleware instead
	// took the summary and the response with it. Asserted unconditionally —
	// both handlers carry a doc comment precisely so this can tell them apart,
	// and an empty summary is a failure rather than a pass.
	if !strings.Contains(op.Summary, "endpoint") {
		t.Errorf("/users summary = %q, want the endpoint handler's doc comment — the summary "+
			"follows whichever handler was attributed", op.Summary)
	}
	if strings.Contains(op.Summary, "guards the route") {
		t.Errorf("/users summary = %q — that is the MIDDLEWARE's doc comment", op.Summary)
	}
	if _, ok := op.Responses["200"]; !ok {
		t.Errorf("/users documents %v, but the endpoint handler writes a 200 — the responses "+
			"follow whichever handler was attributed", sortedStatusKeys(op))
	}
}
