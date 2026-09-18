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

package spec

import "testing"

// A response whose schema says nothing is what a starved response looks like in
// the document: the media type is there because the handler clearly writes
// JSON, and what it writes was never resolved. It is the number issue #296
// asked for, and the only way to read a refused-copy count — 25 million drops
// on gitea cost nothing, and a handful elsewhere can delete a body.
func TestThinOperationsFindsResponsesThatSayNothing(t *testing.T) {
	spec := &OpenAPISpec{Paths: map[string]PathItem{
		"/empty": {Get: &Operation{Responses: map[string]Response{
			"200": {Content: map[string]MediaType{"application/json": {Schema: &Schema{}}}},
		}}},
		"/nil-schema": {Post: &Operation{Responses: map[string]Response{
			"201": {Content: map[string]MediaType{"application/json": {}}},
		}}},
		"/typed": {Get: &Operation{Responses: map[string]Response{
			"200": {Content: map[string]MediaType{"application/json": {Schema: &Schema{Type: "object"}}}},
		}}},
		"/ref": {Get: &Operation{Responses: map[string]Response{
			"200": {Content: map[string]MediaType{"application/json": {Schema: &Schema{Ref: "#/components/schemas/Item"}}}},
		}}},
		"/array": {Get: &Operation{Responses: map[string]Response{
			"200": {Content: map[string]MediaType{"application/json": {Schema: &Schema{Items: &Schema{Type: "string"}}}}},
		}}},
		"/no-content": {Get: &Operation{Responses: map[string]Response{"204": {}}}},
	}}

	got := thinOperations(spec)
	want := map[string]bool{"/empty": true, "/nil-schema": true}
	if len(got) != len(want) {
		t.Fatalf("got %d thin operations %v, want %d", len(got), got, len(want))
	}
	for _, op := range got {
		if !want[op.Path] {
			t.Errorf("%s %s (%s) reported as thin, but its schema says something",
				op.Method, op.Path, op.Status)
		}
	}

	// Sorted, because this reaches stderr and the diagnostics (golden rule #1).
	for i := 1; i < len(got); i++ {
		if got[i-1].Path > got[i].Path {
			t.Errorf("not sorted by path: %q before %q", got[i-1].Path, got[i].Path)
		}
	}
}

// A response that is genuinely absent is NOT thin — there is nothing to say
// about a 204, and reporting it would make the count unreadable in exactly the
// way the raw copy count already was.
func TestThinOperationsIgnoresResponsesWithNoContent(t *testing.T) {
	spec := &OpenAPISpec{Paths: map[string]PathItem{
		"/deleted": {Delete: &Operation{Responses: map[string]Response{"204": {}}}},
	}}
	if got := thinOperations(spec); len(got) != 0 {
		t.Errorf("got %v, want none: a 204 with no content is not a lost schema", got)
	}
}

// truncatedOperations joins a truncated scope to the operations it produced, by
// node KEY. The unattributed case is deliberately kept and deliberately worded:
// for a router wrapper the matched node can be the registration inside the
// wrapper body, shared by every route it registers, so a scope can have no route
// whose matched node is it even though routes came from it. Claiming "no
// operation emitted" there would be wrong (issues #296, #503).
func TestTruncatedOperationsNamesOperationsAndAdmitsWhenItCannot(t *testing.T) {
	keys := map[int32]string{1: "pkg.Router.Get@a.go:10:2", 2: "pkg.Router.Get@b.go:20:2"}
	keyOf := func(id int32) string { return keys[id] }

	routes := []*RouteInfo{
		{Method: "GET", Path: "/things", Node: &TrackerNode{key: "pkg.Router.Get@a.go:10:2"}},
		{Method: "POST", Path: "/things", Node: &TrackerNode{key: "pkg.Router.Get@a.go:10:2"}},
	}

	got := truncatedOperations(routes, []int32{1, 2}, keyOf)
	if len(got) != 3 {
		t.Fatalf("got %d entries %+v, want 3 (two operations for scope 1, one unattributed scope)", len(got), got)
	}

	var attributed, unattributed int
	for _, op := range got {
		if op.Limit != TruncatedByRouteBudget {
			t.Errorf("limit = %q, want %q", op.Limit, TruncatedByRouteBudget)
		}
		if op.Path != "" {
			attributed++
			if op.Registration == "" {
				t.Error("an attributed operation lost its registration; the reader needs both")
			}
		} else {
			unattributed++
			if op.Registration != "pkg.Router.Get@b.go:20:2" {
				t.Errorf("unattributed entry names %q, want the scope that produced nothing", op.Registration)
			}
		}
	}
	if attributed != 2 || unattributed != 1 {
		t.Errorf("attributed=%d unattributed=%d, want 2 and 1", attributed, unattributed)
	}
}

// Nothing truncated must produce nothing at all — the report is silence when
// there is nothing to say.
func TestTruncatedOperationsIsEmptyWhenNothingTruncated(t *testing.T) {
	if got := truncatedOperations(nil, nil, func(int32) string { return "" }); got != nil {
		t.Errorf("got %v, want nil", got)
	}
}

// A nil document has nothing to report, and must not panic on the way to
// saying so.
func TestThinOperationsHandlesNilAndEmpty(t *testing.T) {
	if got := thinOperations(nil); got != nil {
		t.Errorf("nil spec gave %v, want nil", got)
	}
	if got := thinOperations(&OpenAPISpec{}); got != nil {
		t.Errorf("empty spec gave %v, want nil", got)
	}
	// A path item whose verbs are all absent contributes nothing.
	if got := thinOperations(&OpenAPISpec{Paths: map[string]PathItem{"/x": {}}}); got != nil {
		t.Errorf("path with no operations gave %v, want nil", got)
	}
}

// Every verb is inspected, not just GET/POST — a starved response on a PATCH is
// exactly as invisible.
func TestThinOperationsCoversEveryVerb(t *testing.T) {
	empty := map[string]Response{"200": {Content: map[string]MediaType{"application/json": {Schema: &Schema{}}}}}
	spec := &OpenAPISpec{Paths: map[string]PathItem{"/x": {
		Get: &Operation{Responses: empty}, Post: &Operation{Responses: empty},
		Put: &Operation{Responses: empty}, Delete: &Operation{Responses: empty},
		Patch: &Operation{Responses: empty}, Options: &Operation{Responses: empty},
		Head: &Operation{Responses: empty},
	}}}
	got := thinOperations(spec)
	if len(got) != 7 {
		t.Fatalf("got %d, want one per verb", len(got))
	}
	seen := map[string]bool{}
	for _, op := range got {
		seen[op.Method] = true
	}
	for _, m := range []string{"GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS", "HEAD"} {
		if !seen[m] {
			t.Errorf("%s was not inspected", m)
		}
	}
}
