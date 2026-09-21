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

	"github.com/ehabterra/apispec/internal/spec"
)

// A Content-Type header write documents a streamed body only when the header is
// THIS operation's response (issue #543).
//
// The write-destination gate on the content-type pattern was inert: it placed a
// destination through the encoder-factory shape, which no header write has, so
// every `Content-Type` set anywhere in a handler's call graph became its 200 —
// an outbound token request three calls from a redirect-only handler, a literal
// `http.Header{}` nothing sends, a recorder. A handler that states 204 and
// writes nothing was documented as returning a PDF.
func TestTestdata_OutboundContentType(t *testing.T) {
	out := loadTestdata(t, "outbound_content_type", spec.DefaultHTTPConfig())
	noDanglingRefs(t, out)

	statuses := func(path string) (map[string][]string, bool) {
		item, ok := out.Paths[path]
		if !ok || item.Get == nil {
			return nil, false
		}
		got := map[string][]string{}
		for status, resp := range item.Get.Responses {
			var media []string
			for mt := range resp.Content {
				media = append(media, mt)
			}
			sort.Strings(media)
			got[status] = media
		}
		return got, true
	}

	// The header belongs to somebody else: only the status the handler states.
	for _, tc := range []struct{ path, only, why string }{
		{"/callback", "303", "the Content-Type is an OUTBOUND request's, one call deeper"},
		{"/detached", "204", "the Content-Type is on a literal http.Header{} nothing sends"},
		{"/recorded", "204", "the Content-Type is on a recorder, which is not the response"},
		{"/forwarded", "303", "a helper was handed the OUTBOUND request's header"},
		{"/proxied", "202", "the Content-Type is the INCOMING request's, rewritten before forwarding"},
	} {
		got, ok := statuses(tc.path)
		if !ok {
			t.Errorf("%s missing; have %v", tc.path, mapPathKeys(out.Paths))
			continue
		}
		if _, has := got[tc.only]; len(got) != 1 || !has {
			t.Errorf("%s responses = %v, want only %s — %s", tc.path, got, tc.only, tc.why)
		}
		if _, has := got["200"]; has {
			t.Errorf("%s documents a 200 body %v — %s", tc.path, got["200"], tc.why)
		}
	}

	// The header is the response's, reached every way the provenance walk
	// follows: directly, through a variable, a helper handed w, a wrapper built
	// around w, and a closure that captures w — a call with no argument that
	// still returns the response's own header, so a function result that IS a
	// header map is never assumed detached (review of #545).
	for _, path := range []string{"/csv", "/csv-var", "/csv-helper", "/csv-wrapped", "/csv-captured"} {
		got, ok := statuses(path)
		if !ok {
			t.Errorf("%s missing; have %v", path, mapPathKeys(out.Paths))
			continue
		}
		if media := got["200"]; len(media) != 1 || media[0] != "text/csv" {
			t.Errorf("%s 200 = %v, want text/csv — the response's own header must still be read", path, media)
		}
	}

	// Raw bytes take the media type the handler DECLARES (issue #544). A byte
	// slice names none, so `w.Write(b)` after a Content-Type declaration used to
	// win the slot as the "typed" fragment and document a PDF as a base64 string
	// under the JSON default. Reached in the handler, through a helper handed
	// w.Header(), and through a helper handed a VARIABLE holding it.
	//
	// /pdf-var and /reports hand that variable to a helper ANOTHER call site
	// also calls with w.Header() directly. That call site's binding used to
	// take the helper's `h.Set` out of the helper's body for every caller, so
	// here — where the binding fails — the declaration was on no path (#546).
	// /invoices is the same shape through a helper with no other caller, and
	// shares /reports' method name.
	for _, tc := range []struct{ path, media string }{
		{"/pdf-direct", "application/pdf"},
		{"/pdf-param", "application/pdf"},
		{"/pdf-var", "application/pdf"},
		{"/reports", "application/pdf"},
		{"/invoices", "application/zip"},
	} {
		got, ok := statuses(tc.path)
		if !ok {
			t.Errorf("%s missing; have %v", tc.path, mapPathKeys(out.Paths))
			continue
		}
		if media := got["200"]; len(media) != 1 || media[0] != tc.media {
			t.Errorf("%s 200 = %v, want only %s — the declared media type must describe the raw bytes", tc.path, media, tc.media)
			continue
		}
		schema := out.Paths[tc.path].Get.Responses["200"].Content[tc.media].Schema
		if schema == nil || schema.Type != "string" || schema.Format != "binary" {
			t.Errorf("%s 200 schema = %+v, want {type: string, format: binary} — bytes, not a base64 string", tc.path, schema)
		}
	}

	// Control.
	if got, ok := statuses("/items"); !ok || got["200"] == nil {
		t.Errorf("/items 200 missing: %v", got)
	}
}
