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
	"slices"
	"strings"
	"testing"

	"github.com/ehabterra/apispec/spec"
)

// TestTestdata_EmbeddedContextResponder covers issue #645 on the shape a real
// web framework has: a Context embedding the Base that writes, Base writing
// through a project ResponseWriter interface whose implementation forwards to
// the server's writer, and an HTML renderer held behind an interface.
//
// Every write the decorator and the renderer make is reached through interface
// dispatch. Those writes were attributed to the HANDLER's frame, so they paired
// with whatever status the handler wrote last: `ctx.HTTPError(409, msg)`
// documented `application/json` (the decorator's bytes, undeclared there) and
// `text/html` (the renderer's declaration) beside the text/plain http.Error
// sends.
func TestTestdata_EmbeddedContextResponder(t *testing.T) {
	out := loadTestdataWithFixtureConfig(t, "embedded_context_responder", spec.DefaultChiConfig())
	noDanglingRefs(t, out)
	noNullSchemas(t, out)

	const (
		plain    = "text/plain;charset=utf-8"  // what plainTextInternal declares
		plainErr = "text/plain; charset=utf-8" // what http.Error sends
		html     = "text/html; charset=utf-8"
		json     = "application/json"
	)

	// Nothing in this project sends JSON beside plain text: the JSON method
	// answers only the success, and every text/plain is an error helper.
	for path, item := range out.Paths {
		for _, method := range []string{"GET", "POST"} {
			op := opFor(item, method)
			if op == nil {
				continue
			}
			for status := range op.Responses {
				got := contentTypesOf(op, status)
				hasPlain := slices.ContainsFunc(got, func(ct string) bool { return strings.HasPrefix(ct, "text/plain") })
				if hasPlain && slices.Contains(got, json) {
					t.Errorf("%s %s %s documents application/json beside plain text: %v", method, path, status, got)
				}
			}
		}
	}

	exactly := func(method, path, status string, want ...string) {
		t.Helper()
		op := opAt(t, out, method, path)
		if op == nil {
			return
		}
		slices.Sort(want)
		if got := contentTypesOf(op, status); !slices.Equal(got, want) {
			t.Errorf("%s %s %s content = %v, want %v", method, path, status, got, want)
		}
	}
	statuses := func(method, path string, want ...string) {
		t.Helper()
		op := opAt(t, out, method, path)
		if op == nil {
			return
		}
		got := responseCodesOf(op.Responses)
		slices.Sort(got)
		slices.Sort(want)
		if !slices.Equal(got, want) {
			t.Errorf("%s %s statuses = %v, want %v", method, path, got, want)
		}
	}

	// The issue's own shape: HTTPError's 409 is http.Error's text/plain only.
	exactly("POST", "/register", "409", plainErr)
	// The success is the JSON body Base.JSON encodes on the same receiver field.
	//
	// NOT FIXED, asserted so the day it changes: the 200 also lists text/html.
	// The renderer's declaration is parked at its default 200 until the status
	// written after it moves it, and a declaration that was composed into a
	// slot as an alternate media type stays there when it moves.
	exactly("POST", "/register", "200", json, html)
	if op := opAt(t, out, "POST", "/register"); op != nil {
		if ref := schemaRefAt(op, "200"); !strings.HasSuffix(ref, "_Options") {
			t.Errorf("POST /register 200 $ref = %q, want the Options component", ref)
		}
	}
	statuses("POST", "/register", "200", "404", "409", "500")

	// PlainText → plainTextInternal: the declaration, the status and the bytes
	// all happen in plainTextInternal, the bytes one dispatch deeper.
	exactly("GET", "/plain", "400", plain)
	exactly("GET", "/plain", "200", plain)

	// NotFound answers the plain-text 404 or the 404 page. The renderer declares
	// text/html and then writes the status it was handed, so the declaration is
	// the 404's, not a 200 the handler never sends.
	exactly("GET", "/missing", "404", html, plain)
	statuses("GET", "/missing", "404", "500")
	exactly("GET", "/page", "200", html)

	// A method that YAML-encodes into its own buffer is not a responder: the
	// handler renders a page and documents no YAML.
	exactly("GET", "/dispatch", "200", html)
	// The negative's counterpart: an encoder on the response field is.
	exactly("GET", "/export", "200", "application/yaml")

	// A Content-Type set twice before the status: only the second is sent, so
	// the status carries its media type alone and the overridden one is no
	// 200 either. Every declaration on the chain used to move to the status.
	exactly("GET", "/override", "400", "text/plain")
	statuses("GET", "/override", "400")
	exactly("GET", "/accepted", "202", "text/csv")
	statuses("GET", "/accepted", "202")
	// Negative: an override on one arm leaves the first declaration in effect
	// on the other, so the 202 keeps it.
	//
	// NOT FIXED, asserted so the day it changes: the branch's text/csv does not
	// reach the 202 and stays at its default 200, because only a declaration
	// that dominates the status write moves to it.
	exactly("GET", "/branched", "202", "text/html")
	exactly("GET", "/branched", "200", "text/csv", "text/html")

	// An override written past an early exit overrides nothing on the exit's
	// path: the PDF goes out with the 200 that `stream(w)` sends, and only the
	// 500 is PNG. The PDF used to be marked overridden and its 200 removed.
	//
	// NOT FIXED, asserted so the day it changes: the 200 also lists image/png,
	// the moved declaration that was composed into the slot (as on /register).
	exactly("GET", "/download", "500", "image/png")
	exactly("GET", "/download", "200", "application/pdf", "image/png")
	statuses("GET", "/download", "200", "500")
	// Negative: an exit that answers the response itself — directly, or one
	// call down — sends its own, so the declaration still goes with the 202
	// alone and leaves no 200 behind.
	for _, path := range []string{"/report", "/report-helper"} {
		exactly("GET", path, "202", "text/csv")
		exactly("GET", path, "500", plainErr)
		statuses("GET", path, "202", "500")
	}

	// The statements one interface call runs are ordered by where they are
	// written: the implementation's early 204 does not commit the headers its
	// CSV branch sets, nor claim the bytes that branch writes. The same code
	// called directly is the control.
	for _, path := range []string{"/rows", "/rows-direct"} {
		exactly("GET", path, "200", "text/csv")
		exactly("GET", path, "204")
		statuses("GET", path, "200", "204")
	}

	// A responder writing through a local copy of its receiver's writer
	// (`dst := b.Resp`) still writes where its caller pointed it: the encoded
	// body is documented, not ruled out as a write into a buffer of its own.
	//
	// NOT FIXED, asserted so the day it changes: the body is at 200, not the
	// 201 the call writes — ctx.JSON on this Context pairs the same way.
	if op := opAt(t, out, "GET", "/options"); op != nil {
		if ref := schemaRefAt(op, "200"); !strings.HasSuffix(ref, "_Options") {
			t.Errorf("GET /options 200 $ref = %q, want the Options component", ref)
		}
	}
}
