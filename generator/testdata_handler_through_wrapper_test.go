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

// A handler that reaches the registration through a wrapper parameter must be
// attributed to the concrete function, not to the rendering of the parameter —
// which is its TYPE, `net/http.HandlerFunc`, and identifies nothing (#466).
//
// The fixture alternates the two hops on purpose: `Get(http.HandlerFunc(h3))`
// is a conversion of a parameter, so resolving once and peeling once is not
// enough — peeling the conversion exposes `h3`, which must then be resolved
// again to reach `createItem`. A single pass leaves the type in place, which is
// what this test would catch.
func TestTestdata_HandlerThroughWrapper(t *testing.T) {
	out := loadTestdata(t, "handler_through_wrapper", spec.DefaultHTTPConfig())
	noDanglingRefs(t, out)

	item, ok := out.Paths["/conv"]
	if !ok {
		t.Fatalf("/conv missing; have %v", mapPathKeys(out.Paths))
	}
	op := firstOperation(&item)
	if op == nil {
		t.Fatal("/conv has no operation")
	}
	if !strings.HasSuffix(op.OperationID, "createItem") {
		t.Errorf("operationId = %q, want the concrete handler (…createItem)", op.OperationID)
	}
	// The tell of the unresolved case: the handler rendered as its type.
	if strings.Contains(op.OperationID, "HandlerFunc") {
		t.Errorf("operationId = %q — the handler was rendered as its type, not resolved", op.OperationID)
	}
}
