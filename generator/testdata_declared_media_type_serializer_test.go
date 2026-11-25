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

	"github.com/ehabterra/apispec/internal/spec"
)

// TestTestdata_DeclaredMediaTypeSerializer pins a Content-Type declared ahead
// of an encoded body (issue #652). net/http's json.Encoder writes no header, so
// `Header().Set(…problem+json)`, `WriteHeader(400)`, `Encode(p)` sends p as
// application/problem+json. The body used to stay under the JSON default, with
// the declaration documented beside it as a binary blob — or at a 200 the
// handler never sends.
func TestTestdata_DeclaredMediaTypeSerializer(t *testing.T) {
	out := loadTestdata(t, "declared_media_type_serializer", spec.DefaultChiConfig())
	noDanglingRefs(t, out)

	cases := []struct {
		path, status, media, ref string
	}{
		// The declaration in a function helper, in a method helper, and in the
		// handler itself.
		{"/fn-helper", "400", "application/problem+json", "Problem"},
		{"/method-helper", "404", "application/problem+json", "Problem"},
		{"/inline", "409", "application/problem+json", "Problem"},
		// A declaration written after WriteHeader commits the headers has no
		// effect, in a derived helper as inline: the body keeps the default.
		{"/late-helper", "410", "application/json", "Problem"},
		// No status write: the body keeps the implicit 200.
		{"/implicit-status", "200", "application/vnd.item+json", "Item"},
		// A declaration on one branch is not in effect after the join: the
		// body is not moved onto it (golden rule #7).
		{"/branch-declared", "422", "application/json", "Problem"},
		// The shared helper, reached without a declaration: nothing leaks from
		// the route that declares one.
		{"/plain-json", "202", "application/json", "Item"},
		// Change detector: a declaration in the CALLER of the frame that
		// writes the body is on another chain, and pairing reads declarations
		// on the body's own chain only — raw bytes behave the same. When that
		// gap closes this flips to application/problem+json.
		{"/declared-json", "403", "application/json", "Problem"},
		// chi's render.JSON sets Content-Type itself: the declaration ahead of
		// it is replaced, not adopted, and stays with the CSV bytes.
		{"/render-json", "500", "application/json", "Problem"},
	}
	for _, tc := range cases {
		op := opFor(out.Paths[tc.path], "GET")
		if op == nil {
			t.Errorf("GET %s missing; have %v", tc.path, mapPathKeys(out.Paths))
			continue
		}
		got := contentTypesOf(op, tc.status)
		if len(got) != 1 || got[0] != tc.media {
			t.Errorf("GET %s %s content = %v, want only %q", tc.path, tc.status, got, tc.media)
			continue
		}
		schema := op.Responses[tc.status].Content[tc.media].Schema
		if schema == nil || !strings.HasSuffix(schema.Ref, "_"+tc.ref) {
			t.Errorf("GET %s %s: schema %+v, want a $ref to %s", tc.path, tc.status, schema, tc.ref)
		}
	}

	// The other half of the /declared-json change detector: the caller's
	// declaration, not adopted by the helper's body, keeps a 200 of its own
	// that the handler never sends. Both halves flip when the gap closes.
	if op := opFor(out.Paths["/declared-json"], "GET"); op != nil {
		if got := contentTypesOf(op, "200"); len(got) != 1 || got[0] != "application/problem+json" {
			t.Errorf("GET /declared-json 200 content = %v, want the stranded application/problem+json declaration", got)
		}
	}

	// The CSV declaration render.JSON replaced on the error path still
	// describes the bytes the handler writes on success.
	if op := opFor(out.Paths["/render-json"], "GET"); op != nil {
		if got := contentTypesOf(op, "200"); len(got) != 1 || got[0] != "text/csv" {
			t.Errorf("GET /render-json 200 content = %v, want only text/csv", got)
		}
	}

	// A responder whose arms declare different media types: each declaration
	// governs its own arm's body, and the walk inside the responder documents
	// both arms under their own types. The call site adds no third,
	// undeclared application/json.
	if op := opFor(out.Paths["/negotiated-helper"], "GET"); op == nil {
		t.Errorf("GET /negotiated-helper missing")
	} else {
		got := contentTypesOf(op, "412")
		sort.Strings(got)
		want := []string{"application/problem+json", "application/vnd.legacy+json"}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("GET /negotiated-helper 412 content = %v, want %v", got, want)
		}
		if s := op.Responses["412"].Content["application/vnd.legacy+json"].Schema; s == nil || !strings.HasSuffix(s.Ref, "_Problem") {
			t.Errorf("GET /negotiated-helper 412 legacy schema = %+v, want a $ref to Problem", s)
		}
		// Change detector: the second arm's declaration, sent with the status
		// written after it, keeps its binary placeholder beside the body that
		// adopts it, because the first arm's body already holds the slot and
		// the two compose as alternates. When that composition lets the body
		// displace the placeholder this becomes a plain $ref to Problem.
		if s := op.Responses["412"].Content["application/problem+json"].Schema; s == nil || len(s.AnyOf) != 2 {
			t.Errorf("GET /negotiated-helper 412 problem+json schema = %+v, want the anyOf of placeholder and Problem (change detector)", s)
		}
	}

	// The declaration went with its body: it leaves no 200 behind on the
	// operations that never send one.
	for _, path := range []string{"/fn-helper", "/method-helper", "/inline", "/late-helper"} {
		op := opFor(out.Paths[path], "GET")
		if op == nil {
			continue
		}
		if got := statusKeys(op); len(got) != 1 {
			t.Errorf("GET %s statuses = %v, want only the one it sends", path, got)
		}
	}
}
