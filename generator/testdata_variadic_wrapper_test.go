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

// TestTestdata_VariadicWrapper is a CHANGE DETECTOR for a known gap, not a
// statement that the current output is right.
//
// A house router that forwards the chain variadically —
//
//	func (r *Router) Get(path string, handlers ...gin.HandlerFunc) {
//		r.engine.GET(path, handlers...)
//	}
//
// derives a wrapper pattern with a FIXED handler index, because the inner call
// passes one spread argument. A later `r.Get("/users", auth, endpoint)` then
// reads that fixed index and attributes the operation to `auth`: the same
// symptom #386 fixed for direct registrations, reached through the wrapper.
//
// It is not fixed here because it cannot be fixed honestly yet. Deriving
// HandlerArgFromEnd for the wrapper needs to know that the mapped parameter is
// variadic, and the type model erases that: `typemodel.FromExpr` maps
// `*ast.Ellipsis` to `KindSlice`, and metadata records the parameter's type as
// `[]gin.HandlerFunc`. The `...` survives only inside the rendered
// SignatureStr, so recovering it would mean parsing a type string — golden rule
// #2. Filed as the metadata gap instead (golden rule #9).
//
// So this asserts what happens TODAY. When the gap is closed the assertion
// flips to `endpoint`, and this test failing is the signal that it worked.
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
	// KNOWN WRONG — pinned deliberately. Flip to ".endpoint" when the variadic
	// gap is closed.
	if !strings.HasSuffix(op.OperationID, ".auth") {
		t.Errorf("/users operationId = %q; this fixture pins the CURRENT (wrong) "+
			"attribution to the middleware `auth`. If it now names `endpoint`, the "+
			"variadic-wrapper gap is fixed — flip this assertion and delete the note.",
			op.OperationID)
	}
}
