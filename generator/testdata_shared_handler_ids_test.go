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

// An operationId must be unique across the document, and a shared handler's
// symbol cannot be: it is genuinely the resolved handler for several routes.
// A duplicated id is replaced for EVERY holder by the operation's own
// method-and-path identity; a handler used once keeps its symbol (issue #459).
func TestTestdata_SharedHandlerOperationIDs(t *testing.T) {
	out := loadTestdata(t, "shared_handler_ids", spec.DefaultHTTPConfig())
	noDanglingRefs(t, out)

	got := map[string]string{} // "METHOD path" -> operationId
	seen := map[string]string{}
	for path, item := range out.Paths {
		item := item
		for method, op := range map[string]*spec.Operation{
			"GET": item.Get, "POST": item.Post, "PUT": item.Put, "DELETE": item.Delete,
		} {
			if op == nil {
				continue
			}
			key := method + " " + path
			got[key] = op.OperationID
			if op.OperationID == "" {
				t.Errorf("%s: empty operationId", key)
				continue
			}
			if prev, dup := seen[op.OperationID]; dup {
				t.Errorf("operationId %q shared by %s and %s — distinct operations need distinct ids",
					op.OperationID, prev, key)
			}
			seen[op.OperationID] = key
		}
	}

	// The three routes sharing serveItem take their identity from the
	// operation, since the symbol describes none of them.
	for key, want := range map[string]string{
		"GET /items/{id}":   "getItemsById",
		"GET /widgets/{id}": "getWidgetsById",
		"POST /items":       "postItems",
	} {
		if got[key] != want {
			t.Errorf("%s: operationId %q, want %q", key, got[key], want)
		}
	}

	// A handler used once still identifies its operation, so it is untouched —
	// the repair must be surgical, not a wholesale renaming.
	if id := got["GET /catalogue"]; !strings.HasSuffix(id, "listOnce") {
		t.Errorf("GET /catalogue: operationId %q, want the handler symbol (…listOnce) kept", id)
	}
}
