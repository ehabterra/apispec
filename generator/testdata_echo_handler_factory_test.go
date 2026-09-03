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
	"sort"
	"strings"
	"testing"

	"github.com/ehabterra/apispec/spec"
)

// TestTestdata_EchoHandlerFactory covers the handler-factory pattern: a route
// registered as a CALL that returns the framework's handler type
// (`g.POST("/users", h.Create())`), with the method dispatched through an
// interface implemented in another package.
//
// This lived in internal/spec as two tests that built a tracker tree from the
// fixture's checked-in metadata.yaml. That input is a lossy reconstruction —
// enough for the eager tree, not enough for the lazy one — so with the eager
// engine removed (issue #425) the assertions moved here, where the same
// behaviour is checked through the pipeline that actually ships.
func TestTestdata_EchoHandlerFactory(t *testing.T) {
	out := loadTestdataWithFixtureConfig(t, "echo_handler_factory", spec.DefaultEchoConfig())
	noDanglingRefs(t, out)
	noUnresolvedPlaceholders(t, out)

	// The closure the factory returns is descended into: the body it binds and
	// the response it writes both reach the operation.
	users, ok := out.Paths["/api/v1/users"]
	if !ok {
		t.Fatalf("POST /api/v1/users missing; have %v", mapPathKeys(out.Paths))
	}
	post := opFor(users, "POST")
	if post == nil {
		t.Fatal("POST /api/v1/users missing")
	}
	if post.RequestBody == nil {
		t.Error("POST /api/v1/users has no request body — the c.Bind inside the returned closure was not traced")
	}
	if _, ok := post.Responses["201"]; !ok {
		got := make([]string, 0, len(post.Responses))
		for status := range post.Responses {
			got = append(got, status)
		}
		sort.Strings(got)
		t.Errorf("POST /api/v1/users responses = %v, want the 201 the closure writes", got)
	}

	// A function-local request type on a second route, through the same shape.
	login, ok := out.Paths["/api/v1/login"]
	if !ok {
		t.Fatalf("POST /api/v1/login missing; have %v", mapPathKeys(out.Paths))
	}
	if op := opFor(login, "POST"); op == nil || op.RequestBody == nil {
		t.Error("POST /api/v1/login has no request body")
	}

	// And the schemas those bodies name are real components, not placeholders.
	var haveUser bool
	for name := range out.Components.Schemas {
		if strings.HasSuffix(name, "_User") {
			haveUser = true
		}
	}
	if !haveUser {
		names := make([]string, 0, len(out.Components.Schemas))
		for name := range out.Components.Schemas {
			names = append(names, name)
		}
		sort.Strings(names)
		t.Errorf("components.schemas has no User; got %v", names)
	}
}
