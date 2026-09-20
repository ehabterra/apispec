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

	intspec "github.com/ehabterra/apispec/internal/spec"
)

// TestTestdata_StreamedBody pins that a handler which STREAMS its body
// documents a success response, with the media type it declares (issue #517).
//
// Response detection recognises a VALUE being encoded, so these handlers had
// nothing for it to see: the CSV export documented only its 500, claiming an
// endpoint that can exclusively fail.
//
// The media type is read from the handler's own `Content-Type` rather than
// deduced from which writer it used — which is why `application/pdf` is exact
// here. Enumerating writers could only have guessed `application/octet-stream`,
// and would still have missed excelize, archive/zip and any house streamer.
func TestTestdata_StreamedBody(t *testing.T) {
	out := loadTestdata(t, "streamed_body", intspec.DefaultChiConfig())
	noDanglingRefs(t, out)

	cases := []struct {
		name, path, mediaType string
		why                   string
	}{
		{
			name: "a CSV writer built on the response writer",
			path: "/export.csv", mediaType: "text/csv",
			why: "csv.NewWriter(w) streams; nothing encodes a value",
		},
		{
			name: "io.Copy from a file",
			path: "/file.pdf", mediaType: "application/pdf",
			why: "the exact type the handler declares, not a per-writer guess",
		},
		{
			// What real code does: media types live in one place, not inlined
			// at each handler. Reading only a bare literal missed all of them.
			name: "the media type named by a constant",
			path: "/const.csv", mediaType: "text/csv",
			why: "a constant is as much a declaration as an inline string",
		},
		{
			name: "declared inside a shared header helper",
			path: "/helper.csv", mediaType: "text/csv",
			why: "the declaration is often beside the other download headers",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			op := opFor(out.Paths[tc.path], "GET")
			if op == nil {
				t.Fatalf("GET %s missing; have %v", tc.path, mapPathKeys(out.Paths))
			}
			resp, ok := op.Responses["200"]
			if !ok {
				t.Fatalf("GET %s documents no success — it streams, so %s; have %v",
					tc.path, tc.why, statusKeys(op))
			}
			if _, ok := resp.Content[tc.mediaType]; !ok {
				got := make([]string, 0, len(resp.Content))
				for k := range resp.Content {
					got = append(got, k)
				}
				t.Errorf("GET %s 200 content = %v, want %q — %s", tc.path, got, tc.mediaType, tc.why)
			}
		})
	}

	// A writer built over a FILE is not the response. The destination gate is
	// what keeps an internal CSV dump from being documented as a body.
	t.Run("a writer that never reaches the wire", func(t *testing.T) {
		op := opFor(out.Paths["/internal"], "GET")
		if op == nil {
			t.Fatal("GET /internal missing")
		}
		if resp, ok := op.Responses["200"]; ok && len(resp.Content) > 0 {
			t.Errorf("GET /internal documents a body; its csv.Writer writes to a file, not to w")
		}
		if _, ok := op.Responses["204"]; !ok {
			t.Errorf("GET /internal lost its own 204; have %v", statusKeys(op))
		}
	})
}
