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

	"github.com/ehabterra/apispec/internal/spec"
)

// TestTestdata_RawBytesSerializerMedia pins raw bytes under a declared media
// type that a serializer COULD produce (issue #570). The declaration used to be
// dropped for application/yaml, application/xml and text/plain on the grounds
// that a serializer would describe the body, even when none ran, so a service
// serving its embedded openapi.yaml documented it as application/json with a
// base64 string.
//
// The serializer cases must be untouched: a real encoder's typed body wins over
// the declaration, and raw bytes under the default application/json keep the
// behaviour they had.
func TestTestdata_RawBytesSerializerMedia(t *testing.T) {
	out := loadTestdata(t, "raw_bytes_serializer_media", spec.DefaultChiConfig())
	noDanglingRefs(t, out)

	cases := []struct {
		path, media, format, ref string
	}{
		{path: "/yaml-charset", media: "application/yaml; charset=utf-8", format: "binary"},
		{path: "/yaml", media: "application/yaml", format: "binary"},
		{path: "/xml", media: "application/xml", format: "binary"},
		{path: "/text", media: "text/plain; charset=utf-8", format: "binary"},
		{path: "/pdf", media: "application/pdf", format: "binary"},
		// Unchanged: the default media type keeps raw bytes typed.
		{path: "/json-raw", media: "application/json", format: "byte"},
		// Unchanged: a serializer's typed body wins over the declaration.
		{path: "/xml-encoded", media: "application/xml", ref: "Item"},
		{path: "/yaml-encoded", media: "application/yaml", ref: "Item"},
		{path: "/json-encoded", media: "application/json", ref: "Item"},
	}
	for _, tc := range cases {
		op := opFor(out.Paths[tc.path], "GET")
		if op == nil {
			t.Errorf("GET %s missing; have %v", tc.path, mapPathKeys(out.Paths))
			continue
		}
		got := contentTypesOf(op, "200")
		if len(got) != 1 || got[0] != tc.media {
			t.Errorf("GET %s 200 content = %v, want only %q", tc.path, got, tc.media)
			continue
		}
		schema := op.Responses["200"].Content[tc.media].Schema
		switch {
		case schema == nil:
			t.Errorf("GET %s: no schema", tc.path)
		case tc.ref != "":
			if !strings.HasSuffix(schema.Ref, "_"+tc.ref) {
				t.Errorf("GET %s: schema %+v, want a $ref to %s", tc.path, schema, tc.ref)
			}
		case schema.Type != "string" || schema.Format != tc.format:
			t.Errorf("GET %s: schema %s/%s, want string/%s", tc.path, schema.Type, schema.Format, tc.format)
		}
	}
}

// TestTestdata_RawBytesDeclaredUnderWrittenStatus pins a declaration written
// BEFORE a status write: `Header().Set(...)`, `WriteHeader(201)`, `Write(b)`.
// The header write carries the pattern's default 200 but sends no status, so
// the declaration belongs to the 201 its bytes went out under. It used to stay
// at 200 — documenting a status the handler never sends, or for a serializer's
// media type nothing at all — and leave the 201 bytes under the JSON default
// (review of #577).
func TestTestdata_RawBytesDeclaredUnderWrittenStatus(t *testing.T) {
	out := loadTestdata(t, "raw_bytes_serializer_media", spec.DefaultChiConfig())

	for _, tc := range []struct{ path, media string }{
		{"/yaml-created", "application/yaml"},
		{"/pdf-created", "application/pdf"},
	} {
		op := opFor(out.Paths[tc.path], "POST")
		if op == nil {
			t.Errorf("POST %s missing; have %v", tc.path, mapPathKeys(out.Paths))
			continue
		}
		if got := statusKeys(op); len(got) != 1 || got[0] != "201" {
			t.Errorf("POST %s statuses = %v, want only 201", tc.path, got)
			continue
		}
		got := contentTypesOf(op, "201")
		if len(got) != 1 || got[0] != tc.media {
			t.Errorf("POST %s 201 content = %v, want only %q", tc.path, got, tc.media)
			continue
		}
		if schema := op.Responses["201"].Content[tc.media].Schema; schema == nil || schema.Format != "binary" {
			t.Errorf("POST %s 201: schema %+v, want string/binary", tc.path, schema)
		}
	}
}

// TestTestdata_RawBytesLateDeclarationIgnored pins a declaration written AFTER
// the status: `WriteHeader(201)` commits the headers, so a later
// `Header().Set("Content-Type", …)` never reaches the client. It used to stand
// in as a pending 200 and take the bytes from the 201 the handler actually
// sends (review of #577).
func TestTestdata_RawBytesLateDeclarationIgnored(t *testing.T) {
	out := loadTestdata(t, "raw_bytes_serializer_media", spec.DefaultChiConfig())

	for _, path := range []string{"/yaml-late", "/pdf-late"} {
		op := opFor(out.Paths[path], "POST")
		if op == nil {
			t.Errorf("POST %s missing; have %v", path, mapPathKeys(out.Paths))
			continue
		}
		if got := statusKeys(op); len(got) != 1 || got[0] != "201" {
			t.Errorf("POST %s statuses = %v, want only 201", path, got)
			continue
		}
		if got := contentTypesOf(op, "201"); len(got) != 1 || got[0] != "application/json" {
			t.Errorf("POST %s 201 content = %v, want the undeclared default application/json", path, got)
		}
	}
}
