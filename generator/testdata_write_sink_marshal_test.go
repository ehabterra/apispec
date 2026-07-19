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
	"github.com/ehabterra/apispec/spec"
)

// TestTestdata_WriteSinkMarshal covers the write-sink model of issue #195:
// response detection anchored on the write to w, resolving the body by tracing
// the written []byte back to the serialization transform that produced it.
//
//   - /marshal-write (FIXED, direct case): b, _ := json.Marshal(v); w.Write(b).
//     The sink resolves the body to v's type (Payload) at the adjacent
//     WriteHeader(200) status, and no spurious `default` remains.
//   - /raw-write: w.Write([]byte("pong")) — a raw write with no transform behind
//     it stays a plain (schema-less) 200, never a spurious $ref body.
//   - /helper-write: the marshal lives one function boundary away
//     (encodeEnvelope returns json.Marshal(e)); the sink traces through the
//     helper's return to the serialized parameter and binds it to the call-site
//     value, resolving Envelope at 200.
func TestTestdata_WriteSinkMarshal(t *testing.T) {
	out := loadTestdataWithFixtureConfig(t, "write_sink_marshal", spec.DefaultHTTPConfig())
	noDanglingRefs(t, out)

	// Direct case: the payload is on 200, and there is no spurious default.
	mw := opFor(out.Paths["/marshal-write"], "GET")
	if mw == nil {
		t.Fatalf("GET /marshal-write missing; have %v", mapPathKeys(out.Paths))
	}
	if !responseRefsAt(mw.Responses, "200", "Payload") {
		t.Errorf("GET /marshal-write: expected 200 to ref Payload (sink traced b->json.Marshal(v)), got %v", keysOf(mw.Responses))
	}
	if _, ok := mw.Responses["default"]; ok {
		t.Errorf("GET /marshal-write: unexpected spurious `default` response — the write-sink model should have absorbed the marshal into 200")
	}

	// Raw write: 200 exists but carries no $ref body (it is raw bytes).
	rw := opFor(out.Paths["/raw-write"], "GET")
	if rw == nil {
		t.Fatalf("GET /raw-write missing; have %v", mapPathKeys(out.Paths))
	}
	for status, resp := range rw.Responses {
		for ct, media := range resp.Content {
			if media.Schema != nil && media.Schema.Ref != "" {
				t.Errorf("GET /raw-write [%s %s]: unexpected $ref %q — a raw w.Write([]byte) must not synthesize a body", status, ct, media.Schema.Ref)
			}
		}
	}

	// Helper hop: Envelope resolves at 200 (traced through encodeEnvelope's
	// returned marshal, bound to the call-site value), with no spurious default.
	hw := opFor(out.Paths["/helper-write"], "GET")
	if hw == nil {
		t.Fatalf("GET /helper-write missing; have %v", mapPathKeys(out.Paths))
	}
	if !responseRefsAt(hw.Responses, "200", "Envelope") {
		t.Errorf("GET /helper-write: expected 200 to ref Envelope (sink traced through helper return), got %v", keysOf(hw.Responses))
	}
	if _, ok := hw.Responses["default"]; ok {
		t.Errorf("GET /helper-write: unexpected spurious `default` — the helper-return hop should resolve Envelope at 200")
	}

	// Method handler: the sink resolves b := json.Marshal(m) from the method's
	// own scope (via the method table, since a plain method has no
	// ParentFunction), yielding Member at 200.
	mtw := opFor(out.Paths["/method-write"], "GET")
	if mtw == nil {
		t.Fatalf("GET /method-write missing; have %v", mapPathKeys(out.Paths))
	}
	if !responseRefsAt(mtw.Responses, "200", "Member") {
		t.Errorf("GET /method-write: expected 200 to ref Member (method-scope sink unwrap), got %v", keysOf(mtw.Responses))
	}
}

// responseRefsAt reports whether the response at the given status has a content
// schema whose $ref names the given type.
func responseRefsAt(responses map[string]intspec.Response, status, name string) bool {
	resp, ok := responses[status]
	if !ok {
		return false
	}
	for _, media := range resp.Content {
		if media.Schema != nil && strings.Contains(media.Schema.Ref, name) {
			return true
		}
	}
	return false
}
