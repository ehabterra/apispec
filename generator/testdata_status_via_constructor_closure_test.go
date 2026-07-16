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

// TestTestdata_StatusViaConstructorClosure covers the constructor-field status
// shape (#144) when the handler is a closure returned by a METHOD (the
// handler-factory pattern). The `e := NewAPIError(msg, 401)` assignment lives
// in the closure but is recorded on the enclosing method's AssignmentMap, and
// methods live in Type.Methods, not file.Functions — so resolving the status
// requires walking the edge's ParentFunction into the method table. The
// plain-function variant (status_via_constructor) already worked; this pins the
// method-closure variant, which previously collapsed the 401 to "default".
func TestTestdata_StatusViaConstructorClosure(t *testing.T) {
	out := loadTestdataWithFixtureConfig(t, "status_via_constructor_closure", spec.DefaultHTTPConfig())
	noDanglingRefs(t, out)

	item, ok := out.Paths["/profile"]
	if !ok {
		t.Fatalf("path /profile missing; have %v", mapPathKeys(out.Paths))
	}
	get := opFor(item, "GET")
	if get == nil {
		t.Fatal("GET /profile missing")
	}

	// The 401 resolves through the constructor field even though the handler is
	// a method-returned closure, and carries the APIError schema.
	resp401, ok := get.Responses["401"]
	if !ok {
		t.Fatalf("GET /profile should resolve the 401 through the method-closure constructor field; have %v", keysOf(get.Responses))
	}
	ref := ""
	if mt, ok := resp401.Content["application/json"]; ok && mt.Schema != nil {
		ref = mt.Schema.Ref
	}
	if !strings.HasSuffix(ref, "_APIError") {
		t.Errorf("401 response should carry the APIError schema, got %q", ref)
	}

	if _, ok := get.Responses["200"]; !ok {
		t.Errorf("GET /profile lost its 200 response: %v", keysOf(get.Responses))
	}
	if _, ok := get.Responses["default"]; ok {
		t.Errorf("GET /profile still has an unresolved default response: %v", keysOf(get.Responses))
	}
}
