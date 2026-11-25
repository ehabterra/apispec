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

	intspec "github.com/ehabterra/apispec/internal/spec"
	"github.com/ehabterra/apispec/spec"
)

// TestTestdata_PromotedResponder covers the response side of issue #643: a
// request context that embeds the type that writes the response.
//
//	type Context struct{ *Base }
//	ctx.Status(http.StatusNoContent)   // Base.Status, promoted
//
// The call graph recorded the WRITTEN receiver, Context, which declares no
// Status, so the call led nowhere and the status Base writes was never seen.
// The same call spelled `ctx.Base.Status(…)` resolved, so two handlers running
// identical code documented different responses.
//
// The negative is a type that declares its own Status: that call is the type's
// own method, which writes nothing, and its argument is not a status.
func TestTestdata_PromotedResponder(t *testing.T) {
	out := loadTestdataWithFixtureConfig(t, "promoted_responder", spec.DefaultChiConfig())
	noDanglingRefs(t, out)
	noNullSchemas(t, out)

	cases := []struct {
		method, path, why string
		statuses          []string
		plainText         []string
	}{
		{"DELETE", "/unpin", "a promoted Status and PlainText", []string{"204", "500"}, []string{"500"}},
		{"DELETE", "/unpin-explicit", "Status and PlainText through the field by name", []string{"204", "500"}, []string{"500"}},
		{"GET", "/item", "an UNEXPORTED promoted method", []string{"200", "404"}, []string{"404"}},
		{"POST", "/internal", "methods promoted through two embedded fields", []string{"202", "503"}, []string{"503"}},
	}
	for _, tc := range cases {
		op := opAt(t, out, tc.method, tc.path)
		if op == nil {
			continue
		}
		for _, status := range tc.statuses {
			if _, ok := op.Responses[status]; !ok {
				t.Errorf("%s: %s %s response %s missing; have %v", tc.why, tc.method, tc.path, status, responseCodesOf(op.Responses))
			}
		}
		for _, status := range tc.plainText {
			if !hasContentType(op, status, "text/plain;charset=utf-8") {
				t.Errorf("%s: %s %s %s lost the plain-text body Base writes; content = %v",
					tc.why, tc.method, tc.path, status, contentTypesOf(op, status))
			}
			// Base declares text/plain before it writes, and that is all it
			// sends. The derived PlainText responder used to add the body
			// again under the JSON default (issue #645).
			if got := contentTypesOf(op, status); len(got) != 1 {
				t.Errorf("%s: %s %s %s content = %v, want text/plain;charset=utf-8 only",
					tc.why, tc.method, tc.path, status, got)
			}
		}
	}

	if op := opAt(t, out, "GET", "/item"); op != nil {
		if ref := schemaRefAt(op, "200"); !strings.HasSuffix(ref, "_Item") {
			t.Errorf("GET /item 200 body = %q, want a $ref to Item", ref)
		}
	}

	// The promoted spelling and the explicit one run the same code.
	promoted, explicit := opAt(t, out, "DELETE", "/unpin"), opAt(t, out, "DELETE", "/unpin-explicit")
	if promoted != nil && explicit != nil {
		for _, status := range responseCodesOf(explicit.Responses) {
			if got, want := contentTypesOf(promoted, status), contentTypesOf(explicit, status); !slices.Equal(got, want) {
				t.Errorf("DELETE /unpin %s content = %v, the explicit ctx.Base form documents %v", status, got, want)
			}
		}
		if got, want := responseCodesOf(promoted.Responses), responseCodesOf(explicit.Responses); !slices.Equal(got, want) {
			t.Errorf("DELETE /unpin statuses = %v, the explicit ctx.Base form documents %v", got, want)
		}
	}

	// Shadow.Status is Shadow's own method and writes nothing.
	if op := opAt(t, out, "POST", "/teapot"); op != nil {
		if _, ok := op.Responses["418"]; ok {
			t.Errorf("POST /teapot documents 418: the embedding type's own Status was matched as Base's")
		}
		if ref := schemaRefAt(op, "201"); !strings.HasSuffix(ref, "_Item") {
			t.Errorf("POST /teapot 201 body = %q, want a $ref to Item (Base.JSON, promoted past the shadowing Status)", ref)
		}
	}
}

// opAt returns the operation at method and path, reporting a missing one.
func opAt(t *testing.T, out *spec.OpenAPISpec, method, path string) *intspec.Operation {
	t.Helper()
	item, ok := out.Paths[path]
	if !ok {
		t.Errorf("path %q missing; have %v", path, mapPathKeys(out.Paths))
		return nil
	}
	op := opFor(item, method)
	if op == nil {
		t.Errorf("%s %s missing", method, path)
	}
	return op
}

// hasContentType reports whether the response at status documents ct.
func hasContentType(op *intspec.Operation, status, ct string) bool {
	return slices.Contains(contentTypesOf(op, status), ct)
}
