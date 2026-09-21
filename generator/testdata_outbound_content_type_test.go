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
	// follows: directly, through a variable, a helper handed w, and a wrapper
	// built around w.
	for _, path := range []string{"/csv", "/csv-var", "/csv-helper", "/csv-wrapped"} {
		got, ok := statuses(path)
		if !ok {
			t.Errorf("%s missing; have %v", path, mapPathKeys(out.Paths))
			continue
		}
		if media := got["200"]; len(media) != 1 || media[0] != "text/csv" {
			t.Errorf("%s 200 = %v, want text/csv — the response's own header must still be read", path, media)
		}
	}

	// NOT DOCUMENTED, and asserted so the day it changes (#544): a header
	// declared by a helper that takes http.Header, handed the RESPONSE's
	// header, is never claimed — on main before #543 too. When it is fixed,
	// /pdf-param gains `200 application/pdf` and this block says so;
	// /forwarded above is its negative twin and must stay undocumented.
	if got, ok := statuses("/pdf-param"); ok {
		for _, media := range got {
			for _, mt := range media {
				if mt == "application/pdf" {
					t.Errorf("/pdf-param documents application/pdf — #544 is fixed: assert it as present and drop this block")
				}
			}
		}
	} else {
		t.Errorf("/pdf-param missing; have %v", mapPathKeys(out.Paths))
	}

	// Control.
	if got, ok := statuses("/items"); !ok || got["200"] == nil {
		t.Errorf("/items 200 missing: %v", got)
	}
}
