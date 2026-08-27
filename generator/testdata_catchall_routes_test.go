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

// TestTestdata_CatchAllRoutes locks in issue #403: a catch-all route becomes a
// path template parameter rather than keeping the router's wildcard.
//
// `/assets*` was emitted verbatim, so the document offered a static path
// containing an asterisk — one no client can request and no validator accepts
// as templating. The other spellings are unit-tested in
// internal/spec/catchall_path_test.go; this pins the end-to-end shape and the
// parameter that comes with it.
func TestTestdata_CatchAllRoutes(t *testing.T) {
	out := loadTestdataWithFixtureConfig(t, "catchall_routes", spec.DefaultChiConfig())
	noDanglingRefs(t, out)
	noNullSchemas(t, out)

	for path := range out.Paths {
		if strings.ContainsAny(path, "*") {
			t.Errorf("path %q still carries a router wildcard; OpenAPI has no such syntax", path)
		}
	}

	for _, want := range []string{"/assets/{wildcard}", "/static/{wildcard}"} {
		item, ok := out.Paths[want]
		if !ok {
			t.Errorf("path %q missing; have %v", want, mapPathKeys(out.Paths))
			continue
		}
		op := opFor(item, "GET")
		if op == nil {
			t.Errorf("GET %s: operation missing", want)
			continue
		}
		var found bool
		for _, p := range op.Parameters {
			if p.Name != "wildcard" {
				continue
			}
			found = true
			if p.In != "path" || !p.Required {
				t.Errorf("GET %s: wildcard is %s/required=%v, want a required path parameter", want, p.In, p.Required)
			}
			if p.Description == "" {
				t.Errorf("GET %s: the catch-all carries no description — it is the router matching the "+
					"rest of the path, not a parameter the handler failed to read", want)
			}
			if p.Extensions["x-warning"] != nil {
				t.Errorf("GET %s: the catch-all is warned about as missing from the code: %v",
					want, p.Extensions["x-warning"])
			}
		}
		if !found {
			t.Errorf("GET %s: no wildcard parameter declared", want)
		}
	}

	// The ordinary parameter beside them is untouched: read from the code, so
	// no description invented and no warning.
	item, ok := out.Paths["/files/{id}"]
	if !ok {
		t.Fatalf("path /files/{id} missing; have %v", mapPathKeys(out.Paths))
	}
	op := opFor(item, "GET")
	if op == nil {
		t.Fatal("GET /files/{id}: operation missing")
	}
	for _, p := range op.Parameters {
		if p.Name == "id" && p.Extensions["x-warning"] != nil {
			t.Errorf("GET /files/{id}: id is read by the handler and must not be warned about")
		}
	}
}
