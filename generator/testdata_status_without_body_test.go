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
	"testing"

	intspec "github.com/ehabterra/apispec/internal/spec"
	"github.com/ehabterra/apispec/spec"
)

// responseCodesOf lists an operation's status codes, sorted, for failure
// messages that have to say what WAS emitted.
func responseCodesOf(responses map[string]intspec.Response) []string {
	codes := make([]string, 0, len(responses))
	for code := range responses {
		codes = append(codes, code)
	}
	sort.Strings(codes)
	return codes
}

// TestTestdata_StatusWithoutBody locks in issue #393: a status the handler
// writes without a body carries no `content` block.
//
// It used to be emitted as `content: {application/json: {}}`, which is not "no
// body" — an empty media-type object says there IS a body of that type whose
// shape could not be described. On gitea that was half of all responses.
//
// The statuses here are ordinary ones (403, 418), deliberately not the RFC
// bodyless codes #169 already handles: the rule is about the handler writing no
// body, not about the code forbidding one.
func TestTestdata_StatusWithoutBody(t *testing.T) {
	out := loadTestdataWithFixtureConfig(t, "status_without_body", spec.DefaultHTTPConfig())
	noDanglingRefs(t, out)

	for path, status := range map[string]string{
		"/forbidden": "403",
		"/teapot":    "418",
	} {
		item, ok := out.Paths[path]
		if !ok {
			t.Errorf("path %q missing; have %v", path, mapPathKeys(out.Paths))
			continue
		}
		op := opFor(item, "GET")
		if op == nil {
			t.Errorf("GET %s: expected operation, missing", path)
			continue
		}
		resp, ok := op.Responses[status]
		if !ok {
			t.Errorf("GET %s: response %s missing; have %v", path, status, responseCodesOf(op.Responses))
			continue
		}
		if len(resp.Content) != 0 {
			t.Errorf("GET %s response %s: has content %v, want none — the handler writes no body",
				path, status, resp.Content)
		}
		if resp.Description == "" {
			t.Errorf("GET %s response %s: lost its description", path, status)
		}
	}

	// The control: a handler that does write a body keeps its schema, so the
	// rule cannot be satisfied by dropping every content block.
	item, ok := out.Paths["/item"]
	if !ok {
		t.Fatalf("path /item missing; have %v", mapPathKeys(out.Paths))
	}
	op := opFor(item, "GET")
	if op == nil {
		t.Fatal("GET /item: expected operation, missing")
	}
	resp, ok := op.Responses["200"]
	if !ok {
		t.Fatalf("GET /item: response 200 missing; have %v", responseCodesOf(op.Responses))
	}
	media, ok := resp.Content["application/json"]
	if !ok {
		t.Fatalf("GET /item 200: expected a JSON body, got content %v", resp.Content)
	}
	if media.Schema == nil || media.Schema.Ref == "" {
		t.Errorf("GET /item 200: want a $ref to the Item schema, got %+v", media.Schema)
	}
}
