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
	"testing"

	"github.com/ehabterra/apispec/internal/spec"
)

// A builder registers every route it makes from ONE call site, so several
// chains reach the same node with the same key and the same empty mount path.
// Both gates that dedupe the route walk keyed on exactly that, and dropped
// every chain after the first — silently: no placeholder, no warning, just a
// document that stated one path confidently and omitted the others (#465).
//
// The handler matters as much as the path here. A surviving route used to carry
// the wrapper's parameter as its operationId, so even the one route that came
// through said nothing about which handler served it.
func TestTestdata_BuilderChainRoutes(t *testing.T) {
	out := loadTestdata(t, "builder_chain_routes", spec.DefaultHTTPConfig())
	noDanglingRefs(t, out)

	want := map[string]struct{ method, handler string }{
		"/alpha": {"GET", "builderchainroutes.listAlpha"},
		"/beta":  {"GET", "builderchainroutes.listBeta"},
		"/gamma": {"GET", "builderchainroutes.listGamma"},
		"/plain": {"GET", "builderchainroutes.listPlain"},
	}
	for path, exp := range want {
		item, ok := out.Paths[path]
		if !ok {
			t.Errorf("path %q missing — a chain after the first was dropped; have %v",
				path, mapPathKeys(out.Paths))
			continue
		}
		op := firstOperation(&item)
		if op == nil {
			t.Errorf("no operation on %q", path)
			continue
		}
		// Each chain must keep its OWN handler: one operationId across several
		// paths would mean the chains collapsed a different way.
		if op.OperationID != exp.handler {
			t.Errorf("%s operationId = %q, want %q — the chain kept another chain's handler",
				path, op.OperationID, exp.handler)
		}
	}

	// The multi-verb chain registers from two call sites, each ALSO shared with
	// the single-verb chains above, so the same node is reached by four chains.
	items, ok := out.Paths["/items"]
	if !ok {
		t.Fatalf("path /items missing; have %v", mapPathKeys(out.Paths))
	}
	if items.Get == nil || items.Get.OperationID != "builderchainroutes.listItems" {
		t.Errorf("GET /items = %+v, want the listItems handler", items.Get)
	}
	if items.Post == nil || items.Post.OperationID != "builderchainroutes.createItem" {
		t.Errorf("POST /items = %+v, want the createItem handler", items.Post)
	}
}
