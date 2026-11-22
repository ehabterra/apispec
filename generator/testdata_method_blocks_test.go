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
	"github.com/ehabterra/apispec/spec"
)

// TestTestdata_MethodBlocks locks in issue #617: a METHOD handler documents the
// same responses as the identical FUNCTION handler.
//
// Control-flow regions were recorded for declared functions only, so inside a
// method every status write dominated everything below it and no two bodies
// were ever alternatives. Measured on these fixtures before the fix:
//
//	/m/guard         400(Item)            — the function: 200(Item) 400(none)
//	/m/alternatives  200(Err) 202(Item)   — the function: 202(anyOf Err|Item)
//	/m/branchstatus  202 only             — the 200 read as overwritten by an
//	                                        assignment inside the `if`
//
// Every shape is written as /fn/<shape> and /m/<shape>. The pair must agree
// exactly, and each also has to match the expectations below, so the test
// cannot pass by both being wrong the same way.
func TestTestdata_MethodBlocks(t *testing.T) {
	type shape struct {
		method string
		// status -> schema the response carries ("" none, "anyOf[A,B]" an
		// alternation). Statuses not listed are covered only by fn/m agreement.
		want map[string]string
		// statuses that must be present, whatever they carry.
		present []string
	}
	cases := []struct {
		fixture string
		cfg     *spec.APISpecConfig
		shapes  map[string]shape
	}{
		{
			fixture: "method_blocks_http",
			cfg:     spec.DefaultHTTPConfig(),
			shapes: map[string]shape{
				// The early-return error: each status carries its own body.
				"create": {method: "POST", want: map[string]string{"201": "Item", "400": "Err"}},
				// A bodyless guard must not hand its 400 to the success body.
				"guard": {method: "GET", want: map[string]string{"200": "Item", "400": ""}},
				// One status over two exclusive arms is their alternation.
				"alternatives": {method: "GET", want: map[string]string{"202": "anyOf[Err,Item]"}},
				// Negative: two bodies written one after the other are not
				// alternatives, so the 201 carries the first alone.
				"sequential": {method: "GET", want: map[string]string{"201": "Item"}},
				// The 200 assigned above the `if` still reaches the write.
				"branchstatus": {method: "GET", present: []string{"200", "202"}},
				// The 400 is assigned in a returning arm of a function
				// literal: that return ends the literal, not the handler, so
				// the 400 still reaches the write below the call.
				"closurestatus": {method: "GET", present: []string{"200", "400"}},
			},
		},
		{
			fixture: "method_blocks_gin",
			cfg:     spec.DefaultGinConfig(),
			shapes: map[string]shape{
				"create":       {method: "POST", want: map[string]string{"201": "Item", "400": "Err"}},
				"guard":        {method: "GET", want: map[string]string{"200": "Item", "400": ""}},
				"alternatives": {method: "GET", want: map[string]string{"202": "anyOf[Err,Item]"}},
				// Two sends in sequence both run, so they are not
				// alternatives — yet a status-carrying send (c.JSON(201, …))
				// is merged with every other send of its status without an
				// exclusivity check, functions included and before this
				// change. Change-detector: should become "Item" (or both, but
				// never an anyOf) once that merge consults exclusive().
				"sequential":   {method: "GET", want: map[string]string{"201": "anyOf[Err,Item]"}},
				"branchstatus": {method: "GET", want: map[string]string{"200": "Item", "202": "Item"}},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.fixture, func(t *testing.T) {
			out := loadTestdataWithFixtureConfig(t, tc.fixture, tc.cfg)
			noDanglingRefs(t, out)
			noNullSchemas(t, out)

			for name, sh := range tc.shapes {
				fn := responseSummary(t, out, sh.method, "/fn/"+name)
				m := responseSummary(t, out, sh.method, "/m/"+name)
				if fn == nil || m == nil {
					continue
				}
				if !equalSummaries(fn, m) {
					t.Errorf("%s: method handler documents %v, the identical function %v", name, m, fn)
				}
				for _, got := range []map[string]string{fn, m} {
					for status, want := range sh.want {
						if carried, ok := got[status]; !ok {
							t.Errorf("%s: response %s missing; have %v", name, status, got)
						} else if carried != want {
							t.Errorf("%s: response %s carries %q, want %q", name, status, carried, want)
						}
					}
					for _, status := range sh.present {
						if _, ok := got[status]; !ok {
							t.Errorf("%s: response %s missing; have %v", name, status, got)
						}
					}
				}
			}
		})
	}
}

// responseSummary maps each status of an operation to the schema it carries:
// the bare component name, "anyOf[A,B]" (members sorted) for an alternation,
// or "" for a bodyless response.
func responseSummary(t *testing.T, out *spec.OpenAPISpec, method, path string) map[string]string {
	t.Helper()
	item, ok := out.Paths[path]
	if !ok {
		t.Errorf("path %q missing; have %v", path, mapPathKeys(out.Paths))
		return nil
	}
	op := opFor(item, method)
	if op == nil {
		t.Errorf("%s %s: operation missing", method, path)
		return nil
	}
	got := make(map[string]string, len(op.Responses))
	for status, resp := range op.Responses {
		got[status] = carriedSchema(resp)
	}
	return got
}

func carriedSchema(resp intspec.Response) string {
	media, ok := resp.Content["application/json"]
	if !ok || media.Schema == nil {
		return schemaNameOf(resp)
	}
	if len(media.Schema.AnyOf) == 0 {
		return schemaNameOf(resp)
	}
	members := make([]string, 0, len(media.Schema.AnyOf))
	for _, s := range media.Schema.AnyOf {
		members = append(members, schemaRefName(s))
	}
	sort.Strings(members)
	return "anyOf[" + strings.Join(members, ",") + "]"
}

func equalSummaries(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if w, ok := b[k]; !ok || w != v {
			return false
		}
	}
	return true
}
