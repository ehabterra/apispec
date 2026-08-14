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
)

// TestTestdata_BarrenSubtree guards the barren-subtree prune (issue #318): the
// expansion skips subtrees no matcher family can accept, which is 97.6% of the
// nodes on a real service.
//
// The fixture wraps two ordinary routes around a dense, mutually recursive
// utility layer that matches nothing. What must survive the prune is everything
// the routes actually declare — the paths, the request body, the status-coded
// response and the array response — because each of those hangs off a node the
// prune has to prove reachable. A prune that is too aggressive does not fail
// loudly; it drops one of these quietly, which is what this asserts against.
func TestTestdata_BarrenSubtree(t *testing.T) {
	out := loadTestdata(t, "barren_subtree", nil)

	item, ok := out.Paths["/orders"]
	if !ok {
		t.Fatalf("/orders missing; have %v", mapPathKeys(out.Paths))
	}
	if item.Post == nil {
		t.Fatal("POST /orders missing — the route is behind the barren helper layer")
	}
	if item.Get == nil {
		t.Fatal("GET /orders missing — it shares the barren helpers with POST")
	}

	// Request body: reached through the handler, which also calls into the
	// barren layer.
	if item.Post.RequestBody == nil {
		t.Fatal("POST /orders: request body dropped")
	}
	body, ok := item.Post.RequestBody.Content["application/json"]
	if !ok || body.Schema == nil {
		t.Fatalf("POST /orders: want an application/json body schema, got %+v", item.Post.RequestBody.Content)
	}
	if !strings.Contains(body.Schema.Ref, "CreateOrderRequest") {
		t.Errorf("POST /orders: want a $ref to CreateOrderRequest, got ref=%q type=%q", body.Schema.Ref, body.Schema.Type)
	}

	// Response: the status comes from WriteHeader and the schema from the
	// Encode argument, both below the same handler.
	created, ok := item.Post.Responses["201"]
	if !ok {
		codes := make([]string, 0, len(item.Post.Responses))
		for code := range item.Post.Responses {
			codes = append(codes, code)
		}
		sort.Strings(codes)
		t.Fatalf("POST /orders: want a 201 response, have %v", codes)
	}
	resp, ok := created.Content["application/json"]
	if !ok || resp.Schema == nil {
		t.Fatalf("POST /orders 201: want an application/json schema, got %+v", created.Content)
	}
	if !strings.Contains(resp.Schema.Ref, "Order") {
		t.Errorf("POST /orders 201: want a $ref to Order, got ref=%q type=%q", resp.Schema.Ref, resp.Schema.Type)
	}

	// The array response on the sibling route, which reaches the same helpers
	// along different paths — the diamond the unfolding duplicates.
	var listSchemaOK bool
	for _, r := range item.Get.Responses {
		c, ok := r.Content["application/json"]
		if !ok || c.Schema == nil {
			continue
		}
		if c.Schema.Type == "array" && c.Schema.Items != nil && strings.Contains(c.Schema.Items.Ref, "Order") {
			listSchemaOK = true
		}
	}
	if !listSchemaOK {
		t.Error("GET /orders: want an array-of-Order response; the sibling route's schema was dropped")
	}
}
