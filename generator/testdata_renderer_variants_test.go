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
)

// rendererRow is what one renderer call must document: a single status, and
// under it either no body (media == "") or exactly one media type whose schema
// is a $ref to Item (body == "Item"), a string (body == "string"), raw bytes
// (body == "binary") or a byte slice's rendering (body == "byte").
type rendererRow struct {
	path, status, media, body string
}

// TestTestdata_RendererVariants locks in issue #578: every renderer variant a
// framework offers documents what it writes. The renderer patterns used to
// match a closed list of names, so the *Pretty, *Blob, Indented/Secure/Ascii/
// Pure, JSONP, TOML and Render variants and the file senders documented no
// response at all, and Redirect — in the `(status, body)` catch-all — had its
// URL documented as a JSON body. fiber's SendStatus pattern never read its
// argument.
//
// The plain renderers that already worked (JSON, XML, HTML, String, Blob, Data,
// NoContent) are asserted alongside: the new patterns sit beside theirs, and a
// collision would move them.
func TestTestdata_RendererVariants(t *testing.T) {
	const (
		jsonp = "application/javascript; charset=utf-8"
		// fiber's JSONP names the older text/javascript (review of #584).
		jsonpFiber = "text/javascript; charset=utf-8"
		html       = "text/html; charset=utf-8"
		text       = "text/plain; charset=utf-8"
		xml        = "application/xml"
		json       = "application/json"
		anyMT      = "*/*"
	)
	frameworks := []struct {
		name, fixture string
		cfg           *intspec.APISpecConfig
		rows          []rendererRow
	}{
		{"echo", "renderer_variants_echo", intspec.DefaultEchoConfig(), []rendererRow{
			{"/json", "200", json, "Item"},
			{"/json-pretty", "200", json, "Item"},
			{"/jsonp", "200", jsonp, "Item"},
			{"/xml", "200", xml, "Item"},
			{"/xml-pretty", "200", xml, "Item"},
			// JSONBlob is under the default media type: its bytes stay typed,
			// as a raw write declared application/json does (#570).
			{"/json-blob", "200", json, "byte"},
			{"/jsonp-blob", "200", jsonp, "binary"},
			{"/xml-blob", "200", xml, "binary"},
			{"/html-blob", "200", html, "binary"},
			{"/blob", "200", "application/pdf", "binary"},
			{"/html", "200", html, "string"},
			{"/string", "200", text, "string"},
			{"/render", "200", html, "string"},
			{"/file", "200", anyMT, "binary"},
			{"/attachment", "200", anyMT, "binary"},
			{"/inline", "200", anyMT, "binary"},
			{"/redirect", "302", "", ""},
			{"/no-content", "204", "", ""},
		}},
		{"gin", "renderer_variants_gin", intspec.DefaultGinConfig(), []rendererRow{
			{"/json", "200", json, "Item"},
			{"/indented-json", "200", json, "Item"},
			{"/secure-json", "200", json, "Item"},
			{"/ascii-json", "200", json, "Item"},
			{"/pure-json", "200", json, "Item"},
			{"/jsonp", "200", jsonp, "Item"},
			{"/xml", "200", xml, "Item"},
			{"/yaml", "200", "application/yaml", "Item"},
			{"/toml", "200", "application/toml", "Item"},
			{"/string", "200", text, "string"},
			{"/html", "200", html, "string"},
			{"/data", "200", "application/pdf", "binary"},
			{"/file", "200", anyMT, "binary"},
			{"/file-attachment", "200", anyMT, "binary"},
			{"/file-from-fs", "200", anyMT, "binary"},
			{"/redirect", "302", "", ""},
			{"/status", "204", "", ""},
		}},
		{"fiber", "renderer_variants_fiber", intspec.DefaultFiberConfig(), []rendererRow{
			{"/json", "200", json, "Item"},
			{"/jsonp", "200", jsonpFiber, "Item"},
			{"/xml", "200", xml, "Item"},
			{"/string", "200", text, "string"},
			{"/render", "200", html, "string"},
			{"/send-file", "200", anyMT, "binary"},
			// SendFile restores a status the handler set first.
			{"/send-file-created", "201", anyMT, "binary"},
			// With no body written, SendStatus sends the status message.
			{"/not-found", "404", text, "string"},
			{"/download", "200", anyMT, "binary"},
			{"/redirect", "302", "", ""},
			// Redirect's status is optional; fiber sends 302 without it.
			{"/redirect-default", "302", "", ""},
			{"/send-status", "204", "", ""},
		}},
		{"net/http", "renderer_variants_nethttp", intspec.DefaultHTTPConfig(), []rendererRow{
			{"/serve-file", "200", anyMT, "binary"},
			{"/serve-file-fs", "200", anyMT, "binary"},
			{"/serve-content", "200", anyMT, "binary"},
			// The first WriteHeader wins over ServeFile's own 200.
			{"/serve-file-created", "201", anyMT, "binary"},
			{"/redirect", "302", "", ""},
		}},
	}
	for _, fw := range frameworks {
		t.Run(fw.name, func(t *testing.T) {
			out := loadTestdata(t, fw.fixture, fw.cfg)
			noDanglingRefs(t, out)
			for _, row := range fw.rows {
				checkRendererRow(t, out, row)
			}
		})
	}
}

// TestTestdata_RendererVariantsStatusShapes covers the review of #584 beyond
// one-status rows: a status argument that is passed but unresolved, and a
// status set before a file is sent, which the frameworks treat differently.
func TestTestdata_RendererVariantsStatusShapes(t *testing.T) {
	t.Run("fiber", func(t *testing.T) {
		out := loadTestdata(t, "renderer_variants_fiber", intspec.DefaultFiberConfig())
		// Fiber's 302 stands for an OMITTED status; a computed one is not it.
		op := opFor(out.Paths["/redirect-computed"], "GET")
		if op == nil {
			t.Fatalf("GET /redirect-computed missing; have %v", mapPathKeys(out.Paths))
		}
		if _, ok := op.Responses["302"]; ok {
			t.Errorf("GET /redirect-computed documents 302; its status is passed and unresolved, not omitted")
		}
	})
	t.Run("gin", func(t *testing.T) {
		out := loadTestdata(t, "renderer_variants_gin", intspec.DefaultGinConfig())
		op := opFor(out.Paths["/file-created"], "GET")
		if op == nil {
			t.Fatalf("GET /file-created missing; have %v", mapPathKeys(out.Paths))
		}
		// gin's writer lets the file server's WriteHeader(200) replace the
		// 201, so the file is a 200 — not claimed by the earlier status.
		if got := contentTypesOf(op, "200"); len(got) != 1 || got[0] != "*/*" {
			t.Errorf("GET /file-created 200 content = %v, want the file under */*", got)
		}
		// Change detector for #585: c.Status(201) is still documented as a bodyless
		// 201, which gin never sends here — a status write a later one
		// supersedes is not dropped. Flip when it is.
		if _, ok := op.Responses["201"]; !ok {
			t.Errorf("GET /file-created no longer documents the superseded 201 — assert its absence instead")
		}
	})
	t.Run("echo", func(t *testing.T) {
		out := loadTestdata(t, "renderer_variants_echo", intspec.DefaultEchoConfig())
		op := opFor(out.Paths["/file-created"], "GET")
		if op == nil {
			t.Fatalf("GET /file-created missing; have %v", mapPathKeys(out.Paths))
		}
		// Change detector for #576's family: c.Response().WriteHeader(201) is
		// not a status-write pattern on echo, so the file keeps its implicit
		// 200. Echo commits the first WriteHeader, so it should be 201.
		if got := statusKeys(op); len(got) != 1 || got[0] != "200" {
			t.Errorf("GET /file-created statuses = %v — echo's c.Response().WriteHeader may now be read; expect 201", got)
		}
	})
}

func checkRendererRow(t *testing.T, out *intspec.OpenAPISpec, row rendererRow) {
	t.Helper()
	op := opFor(out.Paths[row.path], "GET")
	if op == nil {
		t.Errorf("GET %s missing; have %v", row.path, mapPathKeys(out.Paths))
		return
	}
	if got := statusKeys(op); len(got) != 1 || got[0] != row.status {
		t.Errorf("GET %s statuses = %v, want only %s", row.path, got, row.status)
		return
	}
	got := contentTypesOf(op, row.status)
	if row.media == "" {
		if len(got) != 0 {
			t.Errorf("GET %s %s documents a body %v, want none", row.path, row.status, got)
		}
		return
	}
	if len(got) != 1 || got[0] != row.media {
		t.Errorf("GET %s %s content = %v, want only %q", row.path, row.status, got, row.media)
		return
	}
	schema := op.Responses[row.status].Content[row.media].Schema
	if schema == nil {
		t.Errorf("GET %s %s: no schema", row.path, row.status)
		return
	}
	switch row.body {
	case "Item":
		if !strings.HasSuffix(schema.Ref, "_Item") {
			t.Errorf("GET %s %s: schema %+v, want a $ref to Item", row.path, row.status, schema)
		}
	case "string":
		if schema.Type != "string" || schema.Format != "" {
			t.Errorf("GET %s %s: schema %s/%s, want a plain string", row.path, row.status, schema.Type, schema.Format)
		}
	default: // binary, byte
		if schema.Type != "string" || schema.Format != row.body {
			t.Errorf("GET %s %s: schema %s/%s, want string/%s", row.path, row.status, schema.Type, schema.Format, row.body)
		}
	}
}
