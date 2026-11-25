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
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/ehabterra/apispec/spec"
)

// TestTestdata_MethodTextWriter covers issue #645: a responder written as a
// METHOD documents exactly what the same body written by a plain FUNCTION does.
//
//	func (b *Base) PlainText(status int, text string) {
//		b.Resp.Header().Set("Content-Type", "text/plain;charset=utf-8")
//		b.Resp.WriteHeader(status)
//		b.Resp.Write([]byte(text))
//	}
//
// A method forwarding its parameters into a response write is derived as the
// project's own responder, and the derived pattern lifted the body to the call
// site without the media type the write gives it — so every plain-text answer
// was documented as application/json too. A function is never derived, which
// is why the same code written as one was right.
func TestTestdata_MethodTextWriter(t *testing.T) {
	out := loadTestdataWithFixtureConfig(t, "method_text_writer", spec.DefaultChiConfig())
	noDanglingRefs(t, out)
	noNullSchemas(t, out)

	// Each method/function pair writes identical bodies, so it documents
	// identical content maps: the plain-text declaration (PlainText), the
	// server's sniff of undeclared bytes (Bytes — whatever that rule gives, it
	// gives both), and http.Error's own text/plain (Fail).
	pairs := []struct {
		name   string
		status string
		want   []string
	}{
		{"text", "500", []string{"text/plain;charset=utf-8"}},
		{"bytes", "400", nil},
		{"fail", "409", []string{"text/plain; charset=utf-8"}},
	}
	for _, p := range pairs {
		method, function := opAt(t, out, "GET", "/method/"+p.name), opAt(t, out, "GET", "/func/"+p.name)
		if method == nil || function == nil {
			continue
		}
		if got, want := responseCodesOf(method.Responses), responseCodesOf(function.Responses); !slices.Equal(got, want) {
			t.Errorf("%s: the method writer documents statuses %v, the function writer %v", p.name, got, want)
		}
		mr, fr := method.Responses[p.status], function.Responses[p.status]
		if len(fr.Content) == 0 {
			t.Errorf("%s: the function writer's %s has no content; the comparison proves nothing", p.name, p.status)
		}
		if !reflect.DeepEqual(mr.Content, fr.Content) {
			t.Errorf("%s %s: the method writer documents %v, the function writer %v",
				p.name, p.status, contentTypesOf(method, p.status), contentTypesOf(function, p.status))
		}
		if p.want != nil && !slices.Equal(contentTypesOf(method, p.status), p.want) {
			t.Errorf("%s %s: content = %v, want %v", p.name, p.status, contentTypesOf(method, p.status), p.want)
		}
	}

	// The negative: a method that serializes the value it is handed still
	// documents that value's type, at each call site, under the JSON it sends.
	// What changed is only raw bytes, whose media type the method decides.
	if op := opAt(t, out, "GET", "/method/json"); op != nil {
		if ref := schemaRefAt(op, "200"); !strings.HasSuffix(ref, "_Item") {
			t.Errorf("GET /method/json 200 body = %q, want a $ref to Item", ref)
		}
		if got := contentTypesOf(op, "200"); !slices.Equal(got, []string{"application/json"}) {
			t.Errorf("GET /method/json 200 content = %v, want application/json only", got)
		}
		if got := contentTypesOf(op, "404"); !slices.Equal(got, []string{"text/plain;charset=utf-8"}) {
			t.Errorf("GET /method/json 404 content = %v, want text/plain;charset=utf-8 only", got)
		}
	}

	// Bytes the CALLER serialized are that serializer's document wherever the
	// method writing them is called: `bs := json.Marshal(item); b.Blob(201, bs)`
	// documents Item. Whether a forwarded byte slice is a document is decided
	// per call site, so declining raw bytes in the method must not decline these.
	if op := opAt(t, out, "GET", "/method/blob"); op != nil {
		if ref := schemaRefAt(op, "201"); !strings.HasSuffix(ref, "_Item") {
			t.Errorf("GET /method/blob 201 body = %q, want a $ref to Item", ref)
		}
		if got := contentTypesOf(op, "201"); !slices.Equal(got, []string{"application/json"}) {
			t.Errorf("GET /method/blob 201 content = %v, want application/json only", got)
		}
	}
	// Change-detector: the function writer cannot yet follow its forwarded
	// []byte parameter back to the caller's json.Marshal, so it documents the
	// declared media type with no schema. When the walk learns that hop, this
	// flips to the method's $ref and the pair above becomes a parity check.
	if op := opAt(t, out, "GET", "/func/blob"); op != nil {
		if got := contentTypesOf(op, "201"); !slices.Equal(got, []string{"application/json"}) {
			t.Errorf("GET /func/blob 201 content = %v, want application/json only", got)
		}
		if ref := schemaRefAt(op, "201"); ref != "" {
			t.Errorf("GET /func/blob 201 body = %q; the function writer now resolves the caller's serializer — make it a parity assertion", ref)
		}
	}

	// A wrapper of a wrapper is derived from the first wrapper's derived
	// pattern, and must inherit the same rule: Fail500's text is raw bytes,
	// documented as text/plain by the walk, with no JSON body and no stray
	// 200 beside it.
	if op := opAt(t, out, "GET", "/method/fail500"); op != nil {
		if got := responseCodesOf(op.Responses); !slices.Equal(got, []string{"500"}) {
			t.Errorf("GET /method/fail500 statuses = %v, want [500]", got)
		}
		if got := contentTypesOf(op, "500"); !slices.Equal(got, []string{"text/plain;charset=utf-8"}) {
			t.Errorf("GET /method/fail500 500 content = %v, want text/plain;charset=utf-8 only", got)
		}
	}
}

// TestTestdata_MethodTextWriterGin is the same parity on gin, whose handlers
// write through the context's `c.Writer` field: the fix is in how a responder
// is derived, not in any one framework's patterns (golden rule #5).
func TestTestdata_MethodTextWriterGin(t *testing.T) {
	out := loadTestdataWithFixtureConfig(t, "method_text_writer_gin", spec.DefaultGinConfig())
	noDanglingRefs(t, out)
	noNullSchemas(t, out)

	method, function := opAt(t, out, "GET", "/method/text"), opAt(t, out, "GET", "/func/text")
	if method == nil || function == nil {
		return
	}
	if got := contentTypesOf(method, "500"); !slices.Equal(got, []string{"text/plain;charset=utf-8"}) {
		t.Errorf("GET /method/text 500 content = %v, want text/plain;charset=utf-8 only", got)
	}
	if !reflect.DeepEqual(method.Responses["500"].Content, function.Responses["500"].Content) {
		t.Errorf("the method writer documents %v, the function writer %v",
			contentTypesOf(method, "500"), contentTypesOf(function, "500"))
	}
}
