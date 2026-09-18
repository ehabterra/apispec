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

// An encode whose destination is a BUFFER (issue #471).
//
// The write-destination gate resolves a destination backwards, which answers
// "where did this value come from" — and a buffer's answer is always "a
// buffer", for the one that becomes the response and the one that is thrown
// away alike. The question they differ on is forward: does anything take these
// bytes to a writer?
//
// Both directions are asserted, because keeping both shapes documents a
// response the endpoint never sends and dropping both loses a real one.
func TestTestdata_EncodeBufferSink(t *testing.T) {
	out := loadTestdata(t, "encode_buffer_sink", spec.DefaultHTTPConfig())
	noDanglingRefs(t, out)

	xmlBody := func(t *testing.T, path string) bool {
		t.Helper()
		item, ok := out.Paths[path]
		if !ok {
			t.Fatalf("path %q missing; have %v", path, mapPathKeys(out.Paths))
		}
		op := firstOperation(&item)
		if op == nil {
			t.Fatalf("no operation on %q", path)
		}
		for _, resp := range op.Responses {
			if _, ok := resp.Content["application/xml"]; ok {
				return true
			}
		}
		return false
	}

	// The buffer reaches the writer: the body IS the response.
	for _, path := range []string{
		"/sitemap",     // buf.WriteTo(w)
		"/copied",      // io.Copy(w, buf)
		"/var-encoder", // the encoder assigned to a variable first
		"/via-func",    // the encode and the flush inside a helper FUNCTION
	} {
		if !xmlBody(t, path) {
			t.Errorf("%s: no application/xml response — the buffer this encode wrote into "+
				"is flushed to the response writer, so its body is the response", path)
		}
	}

	// The buffer is never written anywhere. Documenting it would claim a body
	// the endpoint does not send, which is the half that makes the gate worth
	// having.
	if xmlBody(t, "/discarded") {
		t.Error("/discarded: an application/xml response was documented, but that buffer " +
			"never reaches a writer — it is encoded and dropped")
	}

	// NOT FIXED, asserted so the day it changes: the encode inside a METHOD on
	// the payload type is never reached by the walk, so the gate never sees it.
	// That is gitea's sitemap exactly — `func (s *Sitemap) WriteTo(w io.Writer)`
	// — and is why this change recovers the shape without yet recovering those
	// 18 endpoints. A plain function helper (/via-func above) works, so the gap
	// is the method hop and not the extra frame.
	if xmlBody(t, "/via-helper") {
		t.Error("/via-helper now resolves: the encode inside a method on the payload type " +
			"is reached. Issue #471's remaining half is fixed — move this path up into the " +
			"list above and update the issue")
	}
}
