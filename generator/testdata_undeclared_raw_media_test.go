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

	intspec "github.com/ehabterra/apispec/internal/spec"
)

// TestTestdata_UndeclaredRawWriteMedia pins what bytes written verbatim under no
// declared media type are documented as, on every framework (issue #596). They
// were `application/json` with `format: byte` — a base64 string, which nothing
// on the wire is. Each now carries the media type its server actually sends:
// net/http (and gin and echo, which write through it) sniffs a constant
// exactly as the server will, and fasthttp sends text/plain whatever the
// bytes are. Bytes the analysis cannot see keep the default media type with
// an unknown schema.
func TestTestdata_UndeclaredRawWriteMedia(t *testing.T) {
	cases := []struct {
		name, fixture, path, status, media string
		cfg                                *intspec.APISpecConfig
		unknown                            bool // the empty schema rather than bytes
	}{
		{name: "net/http", fixture: "mux", path: "/api/v1/health", status: "200",
			media: "text/plain; charset=utf-8", cfg: intspec.DefaultMuxConfig()},
		{name: "gin", fixture: "status_local_var_gin", path: "/status", status: "202",
			media: "text/plain; charset=utf-8", cfg: intspec.DefaultGinConfig()},
		{name: "echo", fixture: "streamed_body_echo", path: "/undeclared", status: "200",
			media: "text/html; charset=utf-8", cfg: intspec.DefaultEchoConfig()},
		// fasthttp does not sniff: JSON-looking bytes still go out as text.
		{name: "fiber", fixture: "streamed_body_fiber", path: "/undeclared", status: "200",
			media: "text/plain; charset=utf-8", cfg: intspec.DefaultFiberConfig()},
		// A helper's bytes are not a constant here: undetermined.
		{name: "non-constant", fixture: "write_sink_marshal", path: "/rawhelper-write", status: "200",
			media: "application/json", cfg: intspec.DefaultHTTPConfig(), unknown: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := loadTestdata(t, tc.fixture, tc.cfg)
			op := opFor(out.Paths[tc.path], "GET")
			if op == nil {
				t.Fatalf("GET %s missing; have %v", tc.path, mapPathKeys(out.Paths))
			}
			if got := contentTypesOf(op, tc.status); len(got) != 1 || got[0] != tc.media {
				t.Fatalf("GET %s %s content = %v, want only %q", tc.path, tc.status, got, tc.media)
			}
			schema := op.Responses[tc.status].Content[tc.media].Schema
			switch {
			case schema == nil:
				t.Errorf("GET %s: no schema", tc.path)
			case tc.unknown:
				if schema.Type != "" || schema.Format != "" || schema.Ref != "" {
					t.Errorf("GET %s: schema %+v, want the empty schema", tc.path, schema)
				}
			case schema.Type != "string" || schema.Format != "binary":
				t.Errorf("GET %s: schema %s/%s, want string/binary", tc.path, schema.Type, schema.Format)
			}
		})
	}
}

// TestTestdata_DeclaredJSONKeepsRawMessageSchema pins that bytes written under
// a declared application/json keep the schema their type states. Pairing the
// declaration with them must not trade json.RawMessage's "arbitrary JSON" for
// the bare empty schema (issue #596).
func TestTestdata_DeclaredJSONKeepsRawMessageSchema(t *testing.T) {
	out := loadTestdata(t, "raw_bytes_serializer_media", intspec.DefaultChiConfig())
	op := opFor(out.Paths["/raw-message"], "GET")
	if op == nil {
		t.Fatalf("GET /raw-message missing; have %v", mapPathKeys(out.Paths))
	}
	if got := contentTypesOf(op, "200"); len(got) != 1 || got[0] != "application/json" {
		t.Fatalf("GET /raw-message 200 content = %v, want only application/json", got)
	}
	schema := op.Responses["200"].Content["application/json"].Schema
	if schema == nil || !strings.Contains(schema.Description, "json.RawMessage") {
		t.Errorf("GET /raw-message schema = %+v, want json.RawMessage's own schema", schema)
	}
}

// TestTestdata_DeclarationDoesNotLendItsArm pins that a tentative Content-Type
// declaration a body displaces lends that body none of its entry sites. A
// declared application/json in POST's handler sat on the 200 slot until GET's
// body, from the same dispatched route, displaced it and inherited POST's
// site, so the method split kept GET's User on POST as a 200 it never sends
// (found while fixing #596).
func TestTestdata_DeclarationDoesNotLendItsArm(t *testing.T) {
	out := loadTestdata(t, "enum_validation", intspec.DefaultHTTPConfig())
	op := opFor(out.Paths["/users"], "POST")
	if op == nil {
		t.Fatalf("POST /users missing; have %v", mapPathKeys(out.Paths))
	}
	got := statusKeys(op)
	sort.Strings(got)
	if len(got) != 2 || got[0] != "201" || got[1] != "400" {
		t.Errorf("POST /users statuses = %v, want [201 400]", got)
	}
}
