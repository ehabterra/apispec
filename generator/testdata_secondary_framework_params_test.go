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

// TestTestdata_SecondaryFrameworkParams covers issue #211: a framework that is
// not the primary one keeps its own extraction.
//
// Frameworks other than the primary are reduced to their receiver-scoped subset
// by SecondaryView. Every gin-specific param, bind and response pattern used to
// be unscoped, so all of them were dropped: a secondary gin still had its routes
// discovered but documented them with no parameters, no request body and no
// response schema.
//
// The fixture is deliberately ordered so gin is NOT primary — the chi admin file
// sorts first, and detection makes the first framework it sees the primary one
// (issue #212). That ordering is the whole point: renaming the files would have
// masked the bug.
func TestTestdata_SecondaryFrameworkParams(t *testing.T) {
	out := loadTestdataWithFixtureConfig(t, "secondary_framework_params", nil)
	noDanglingRefs(t, out)
	noUnresolvedPlaceholders(t, out)

	// The primary framework's own route must still be there — scoping the
	// secondary's patterns must not cost the primary anything.
	if _, ok := out.Paths["/admin/health"]; !ok {
		t.Errorf("chi (primary) route /admin/health missing; have %v", mapPathKeys(out.Paths))
	}

	post := opFor(out.Paths["/items"], "POST")
	if post == nil {
		t.Fatalf("POST /items missing; have %v", mapPathKeys(out.Paths))
	}

	// Params: gin's Query and GetHeader, both gin-Context-scoped.
	paramIn := map[string]string{}
	for _, p := range post.Parameters {
		paramIn[p.Name] = p.In
	}
	for name, want := range map[string]string{"filter": "query", "X-Tenant": "header"} {
		if got := paramIn[name]; got != want {
			t.Errorf("POST /items: parameter %q in=%q, want %q (#211); have %v", name, got, want, paramIn)
		}
	}

	// Request body: gin's ShouldBindJSON.
	if post.RequestBody == nil {
		t.Fatal("POST /items: requestBody missing — gin's bind pattern was dropped (#211)")
	}
	if ref := bodyRef(post.RequestBody.Content); !strings.HasSuffix(ref, "_CreateItemRequest") {
		t.Errorf("POST /items requestBody = %q, want CreateItemRequest (#211)", ref)
	}

	// Response: gin's status-carrying c.JSON(201, ...).
	created, ok := post.Responses["201"]
	if !ok {
		t.Fatalf("POST /items: 201 response missing — gin's response pattern was dropped (#211); have %v", responseCodes(post))
	}
	if ref := bodyRef(created.Content); !strings.HasSuffix(ref, "_Item") {
		t.Errorf("POST /items 201 = %q, want Item (#211)", ref)
	}

	// gin's Param, on the second operation.
	get := opFor(out.Paths["/items/{id}"], "GET")
	if get == nil {
		t.Fatalf("GET /items/{id} missing; have %v", mapPathKeys(out.Paths))
	}
	var foundID bool
	for _, p := range get.Parameters {
		if p.Name == "id" && p.In == "path" {
			foundID = true
		}
	}
	if !foundID {
		t.Errorf("GET /items/{id}: path parameter 'id' missing — gin's Param pattern was dropped (#211)")
	}
}

// bodyRef returns the $ref of the first content entry carrying one.
func bodyRef(content map[string]intspec.MediaType) string {
	for _, mt := range content {
		if mt.Schema != nil && mt.Schema.Ref != "" {
			return mt.Schema.Ref
		}
	}
	return ""
}

// responseCodes lists an operation's status codes, for failure messages.
func responseCodes(op *intspec.Operation) []string {
	out := make([]string, 0, len(op.Responses))
	for code := range op.Responses {
		out = append(out, code)
	}
	return out
}
