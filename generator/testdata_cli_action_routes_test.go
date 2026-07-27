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

	intspec "github.com/ehabterra/apispec/internal/spec"
)

// TestTestdata_CLIActionRoutes covers issue #143: route registration reached from
// main only through a function value stored in a composite-literal field — the
// urfave/cli `Command{Action: runWeb}` shape — is now followed.
//
// This is the defect that made apispec report **zero routes** for gitea. Tracker
// roots are main functions, and no static call edge crosses the dispatcher hop:
// the function value goes into a struct field and the library calls it back as
// `c.Action()`. The registration edges were in metadata all along; nothing
// reached them.
//
// The fixture carries the three shapes that occur in the wild, and asserts all
// three resolve — including the handler bodies behind them, since a route found
// but documented empty is barely better than a route missed:
//
//   - a literal nested inside a function's own composite literal (`/users`);
//   - a package-level var holding the command, gitea's own form (`/admin/stats`);
//   - an inline closure as the Action (`/metrics`).
//
// All three commands' routes appear together, which is the intended reading: each
// is genuinely invoked for its own subcommand, so their union is the surface the
// binary serves. That is a stronger statement than picking one and is why
// several candidates for one field is not treated as ambiguity to guess at.
func TestTestdata_CLIActionRoutes(t *testing.T) {
	out := loadTestdataWithFixtureConfig(t, "cli_action_routes", nil)
	noDanglingRefs(t, out)
	noUnresolvedPlaceholders(t, out)

	// 1. The nested in-function literal: {Name: "web", Action: runWeb}.
	users, ok := out.Paths["/users"]
	if !ok {
		t.Fatalf("/users missing — the Action field hop was not followed (#143); have %v", mapPathKeys(out.Paths))
	}
	get := opFor(users, "GET")
	post := opFor(users, "POST")
	if get == nil || post == nil {
		t.Fatalf("/users: want GET and POST, got get=%v post=%v", get != nil, post != nil)
	}
	// The handler bodies have to come with the routes: reaching the registration
	// but not what it registers would leave every operation empty.
	if _, hasOK := get.Responses["200"]; !hasOK {
		t.Errorf("GET /users: 200 response missing — the handler body was not reached; have %v", keysOf(get.Responses))
	}
	if post.RequestBody == nil {
		t.Error("POST /users: requestBody missing — the handler body was not reached")
	} else if ref := contentSchemaRef(post.RequestBody.Content); !strings.HasSuffix(ref, "_User") {
		t.Errorf("POST /users requestBody = %q, want User", ref)
	}
	if created, hasCreated := post.Responses["201"]; !hasCreated {
		t.Errorf("POST /users: 201 response missing; have %v", keysOf(post.Responses))
	} else if ref := contentSchemaRef(created.Content); !strings.HasSuffix(ref, "_User") {
		t.Errorf("POST /users 201 = %q, want User", ref)
	}

	// 2. gitea's shape: the command literal in a package-level var. Its
	// initializer is not recorded as a CallArgument anywhere, so this resolves
	// through a different source than the case above and needs its own coverage.
	if stats := opFor(out.Paths["/admin/stats"], "GET"); stats == nil {
		t.Errorf("GET /admin/stats missing — a package-level command var was not followed (#143); have %v",
			mapPathKeys(out.Paths))
	} else if _, hasOK := stats.Responses["200"]; !hasOK {
		t.Errorf("GET /admin/stats: 200 response missing; have %v", keysOf(stats.Responses))
	}

	// 3. An inline closure as the Action — the other common urfave/cli form.
	if metrics := opFor(out.Paths["/metrics"], "GET"); metrics == nil {
		t.Errorf("GET /metrics missing — a closure Action was not followed (#143); have %v",
			mapPathKeys(out.Paths))
	}
}

// contentSchemaRef returns the $ref of the first content entry carrying one.
// Named apart from the identically-shaped helper in the secondary-framework test
// so the two can coexist in this package.
func contentSchemaRef(content map[string]intspec.MediaType) string {
	for _, mt := range content {
		if mt.Schema != nil && mt.Schema.Ref != "" {
			return mt.Schema.Ref
		}
	}
	return ""
}
