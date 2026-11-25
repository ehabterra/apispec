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
	"encoding/json"
	"strings"
	"testing"

	"github.com/ehabterra/apispec/internal/spec"
)

// A responder whose body argument is BUILT from a parameter must not be read as
// forwarding that parameter (issue #659):
//
//	func (rt *R) normalize(w http.ResponseWriter, req *http.Request) {
//		p := req.URL.Path
//		http.Error(w, "bad "+p, http.StatusBadRequest)
//	}
//
// The body mentions `req`, through `p`, but it is a string the method made.
// Mapping the body role back to `req` documented *http.Request as a response
// body of every caller. Only a value the caller passes — the parameter itself,
// an alias of it, its address — is a body the call site can supply.
func TestTestdata_DerivedBodyArg(t *testing.T) {
	out := loadTestdata(t, "derived_body_arg", spec.DefaultHTTPConfig())
	noDanglingRefs(t, out)

	opOf := func(method, path string) *spec.Operation {
		t.Helper()
		item, ok := out.Paths[path]
		if !ok {
			t.Fatalf("%s missing; have %v", path, mapPathKeys(out.Paths))
		}
		op := opFor(item, method)
		if op == nil {
			t.Fatalf("%s: no %s operation", path, method)
		}
		return op
	}
	opAt := func(path string) *spec.Operation { t.Helper(); return opOf("GET", path) }
	// No response anywhere may carry the request type the message was built from.
	noRequestBody := func(path string, op *spec.Operation) {
		t.Helper()
		raw, err := json.Marshal(op.Responses)
		if err != nil {
			t.Fatalf("%s: marshal responses: %v", path, err)
		}
		if strings.Contains(string(raw), "net_http_Request") {
			t.Errorf("%s documents *http.Request as a response body — the body is a message "+
				"built from the request, not the request: %s", path, raw)
		}
	}

	// The issue's shape, a method: 400 survives, the request does not appear,
	// and the pass-through responder beside it keeps the caller's type.
	normalized := opAt("/normalized")
	noRequestBody("/normalized", normalized)
	if _, ok := normalized.Responses["400"]; !ok {
		t.Errorf("/normalized lost its 400; have %v", sortedStatusKeys(normalized))
	}
	if ref := schemaRefAt(normalized, "200"); !strings.HasSuffix(ref, "_Item") {
		t.Errorf("/normalized 200 = %q, want the Item the caller hands send(w, v)", ref)
	}

	// The function-shaped helper: no receiver, the selector written inline.
	rejected := opAt("/rejected")
	noRequestBody("/rejected", rejected)
	if _, ok := rejected.Responses["400"]; !ok {
		t.Errorf("/rejected lost its 400; have %v", sortedStatusKeys(rejected))
	}

	// Value-preserving forwarding still maps: `out := v; Encode(&out)` is the
	// caller's Widget, and a status parameter forwarded unchanged is still the
	// caller's status.
	widgets := opAt("/widgets")
	noRequestBody("/widgets", widgets)
	if ref := schemaRefAt(widgets, "200"); !strings.HasSuffix(ref, "_Widget") {
		t.Errorf("/widgets 200 = %q, want the Widget forwarded through a local and its address", ref)
	}
	if _, ok := widgets.Responses["404"]; !ok {
		t.Errorf("/widgets lost the 404 fail(w, msg, code) forwards; have %v", sortedStatusKeys(widgets))
	}

	// `http.StatusText(code)` as the body is derived from the status parameter:
	// the status maps, the body is not the integer it was computed from.
	resp, ok := widgets.Responses["409"]
	if !ok {
		t.Fatalf("/widgets lost the 409 failCode(w, code) forwards; have %v", sortedStatusKeys(widgets))
	}
	for ct, mt := range resp.Content {
		if ct == "application/json" {
			t.Errorf("409 documents an %s body — http.Error writes text/plain", ct)
		}
		if mt.Schema != nil && mt.Schema.Type == "integer" {
			t.Errorf("409 documents an integer body (%s) — that is the status code itself", ct)
		}
	}

	// A decode destination is a value role too. `Decode(dst)` fills the
	// caller's value; `Decode(&Envelope{Data: dst})` fills an envelope the
	// method builds, and reading the caller's bare Order as the request body put
	// an Order component in the document that the envelope then displaced.
	if ref := requestRefOf(opOf("POST", "/widgets")); !strings.HasSuffix(ref, "_Widget") {
		t.Errorf("POST /widgets request = %q, want the Widget handed to bind(r, dst)", ref)
	}
	if ref := requestRefOf(opOf("POST", "/orders")); !strings.HasSuffix(ref, "_Envelope") {
		t.Errorf("POST /orders request = %q, want the Envelope bindEnvelope decodes into", ref)
	}
	for name := range out.Components.Schemas {
		if strings.HasSuffix(name, "_Order") {
			t.Errorf("component %s registered — the caller's bare value was read as the request "+
				"body of a decoder that wraps it in its own envelope", name)
		}
	}

	// A decoder method that also rejects with a message built from the request:
	// the rejection's body is declined and it has no status parameter, so it
	// adds nothing — but the decoder role it shares the method with must still
	// apply, whichever of the two calls the derivation meets first.
	for path, want := range map[string]string{"/gadgets": "_Gadget", "/parts": "_Part"} {
		op := opOf("POST", path)
		noRequestBody(path, op)
		if ref := requestRefOf(op); !strings.HasSuffix(ref, want) {
			t.Errorf("POST %s request = %q, want the %s handed to the decoder method", path, ref, want[1:])
		}
	}

	// Two layers: deny forwards to reject, which builds its message from the
	// request and forwards its status. The 403 is the caller's and nothing the
	// caller passes is the body, at either layer. The derived deny pattern
	// applies only since its declined body carries over from reject
	// (TestDeclinedBodyCarriesAcrossWrapperLayers); the walk into deny already
	// found the 403, so this guards the output while the unit test guards the
	// derivation.
	denied := opAt("/denied")
	noRequestBody("/denied", denied)
	if _, ok := denied.Responses["403"]; !ok {
		t.Errorf("/denied lost the 403 deny(w, r, code) forwards; have %v", sortedStatusKeys(denied))
	}
	if ref := schemaRefAt(denied, "200"); !strings.HasSuffix(ref, "_Item") {
		t.Errorf("/denied 200 = %q, want the Item the caller hands send(w, v)", ref)
	}

	// A parameter the method reassigns before encoding is not the caller's
	// value: `v = map[string]any{"data": v}` sends the map, never a bare Receipt,
	// and the derived sendWrapped pattern now declines (the unit cases in
	// TestParamIndexValueIndexOf pin that). CHANGE-DETECTOR: the walk into
	// sendWrapped still binds `v` to the caller's argument without reading the
	// method's own assignment to it — the response-body side of the gap #638
	// files for router parameters — so Receipt is documented anyway. When that
	// binding learns to see the reassignment, this flips: assert the Receipt is
	// gone instead.
	receipts := opAt("/receipts")
	if ref := schemaRefAt(receipts, "200"); !strings.HasSuffix(ref, "_Receipt") {
		t.Errorf("/receipts 200 = %q — the walk no longer reads the caller's Receipt through a "+
			"reassigned parameter; flip this to assert the Receipt is absent", ref)
	}
}
