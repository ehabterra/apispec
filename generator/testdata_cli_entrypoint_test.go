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
)

// TestTestdata_CLIEntrypointRoutes covers issue #220: routes reachable only
// through a command library's dispatcher.
//
// The fixture depends on the **real** github.com/urfave/cli/v3, which is the
// whole point. The call that invokes `Action` lives inside that module, and
// apispec never analyses external packages, so there is no call edge from main to
// `runWeb` — the mechanism from #143 (follow a func-typed field to its value)
// finds nothing to follow. Only treating the field as an *entrypoint*, and rooting
// the function it holds, makes the subtree exist.
//
// This is the gap the single-package `cli_action_routes` fixture could not
// express: that one has an in-module dispatcher, so its Action call edge exists
// and it passed while gitea and photoprism still documented zero routes.
//
// The fixture is run with a nil config so the urfave/cli entrypoint preset arrives
// through import detection, exactly as it does for a real project.
func TestTestdata_CLIEntrypointRoutes(t *testing.T) {
	out := loadTestdataWithFixtureConfig(t, "cli_entrypoint_routes", nil)
	noDanglingRefs(t, out)
	noUnresolvedPlaceholders(t, out)

	users, ok := out.Paths["/users"]
	if !ok {
		t.Fatalf("/users missing — the cli.Command Action entrypoint was not rooted (#220); have %v",
			mapPathKeys(out.Paths))
	}
	get := opFor(users, "GET")
	post := opFor(users, "POST")
	if get == nil || post == nil {
		t.Fatalf("/users: want GET and POST, got get=%v post=%v", get != nil, post != nil)
	}
	// The handler bodies must come with the routes: rooting the entrypoint is
	// only useful if the whole subtree below it resolves.
	if _, hasOK := get.Responses["200"]; !hasOK {
		t.Errorf("GET /users: 200 missing — the handler body was not reached; have %v", keysOf(get.Responses))
	}
	if post.RequestBody == nil {
		t.Error("POST /users: requestBody missing — the handler body was not reached")
	} else if ref := contentSchemaRef(post.RequestBody.Content); !strings.HasSuffix(ref, "_User") {
		t.Errorf("POST /users requestBody = %q, want User", ref)
	}

	// The fixture's second command (`index`) registers nothing. Rooting it would
	// be harmless for output but not for cost — every subcommand of every CLI
	// would drag its subtree into the walk — so the gate must exclude it. What is
	// observable here is that its work produced no paths of its own.
	if len(out.Paths) != 1 {
		t.Errorf("expected only /users, have %v — a route-less entrypoint may have been expanded", mapPathKeys(out.Paths))
	}
}
