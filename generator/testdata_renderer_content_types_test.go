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

// contentTypesOf returns the media types documented for one status, sorted.
func contentTypesOf(op *spec.Operation, status string) []string {
	if op == nil {
		return nil
	}
	resp, ok := op.Responses[status]
	if !ok {
		return nil
	}
	out := make([]string, 0, len(resp.Content))
	for ct := range resp.Content {
		out = append(out, ct)
	}
	sort.Strings(out)
	return out
}

// A response's media type is a claim the consumer acts on: it decides which
// parser runs. Every renderer used to come out as `application/json` whatever
// the handler wrote, so an XML endpoint told the client to parse XML as JSON
// (issue #354).
//
// This fixture is the case the call name alone cannot settle: `encoding/xml`
// and `encoding/json` both spell it `Encode`, so only the receiver says which
// one writes.
func TestTestdata_RendererContentTypes(t *testing.T) {
	out := loadTestdata(t, "renderer_content_types", spec.DefaultHTTPConfig())
	noDanglingRefs(t, out)

	cases := []struct {
		path string
		want string
	}{
		{"/items/xml", "application/xml"},
		{"/items/json", "application/json"},
		// An xml encode into a buffer that never reaches the writer is not this
		// endpoint's body. The non-JSON encoder patterns drop an encode whose
		// destination cannot be shown to be the response writer, which is what
		// keeps a serializer from inventing a route's media type (issue #471).
		{"/items/away", "application/json"},
		// No Content-Type header: Go sniffs the payload and actually sends
		// text/plain; charset=utf-8, because the encoder does not write the
		// `<?xml` declaration the sniffer looks for. What is documented is what
		// the ENCODER writes, which describes the payload rather than a handler
		// that forgot its header — and saying text/plain here would be the old
		// application/json bug wearing a different label. Reading an explicit
		// header so it can win is the remaining step of #354; when that lands,
		// this case is the one that changes.
		{"/items/xml-no-header", "application/xml"},
	}
	for _, tc := range cases {
		item, ok := out.Paths[tc.path]
		if !ok {
			t.Errorf("%s missing; have %v", tc.path, mapPathKeys(out.Paths))
			continue
		}
		got := contentTypesOf(opFor(item, "GET"), "200")
		if len(got) != 1 || got[0] != tc.want {
			t.Errorf("GET %s 200 content = %v, want [%s]", tc.path, got, tc.want)
		}
	}

	// A handler that writes BOTH on one operation documents only one of them.
	// One status carries one media type, because RouteInfo.Response is keyed by
	// status alone — a structural limit, not a matching failure (issue #470).
	// Pinned so this flips when that is fixed: the assertion to write then is
	// both media types, not whichever won.
	both := contentTypesOf(opFor(out.Paths["/items/both"], "GET"), "200")
	if len(both) != 1 {
		t.Errorf("GET /items/both 200 content = %v; a status can currently hold one media type — "+
			"if this now holds both, the structural limit is fixed and this test should assert both", both)
	}
}

// The same rule through a framework's own renderers, which name the media type
// in the CALL rather than in the encoder: gin's c.XML / c.YAML / c.String /
// c.HTML were all documented as application/json (issue #354).
func TestTestdata_RendererContentTypesGin(t *testing.T) {
	out := loadTestdata(t, "renderer_content_types_gin", spec.DefaultGinConfig())
	noDanglingRefs(t, out)

	cases := []struct {
		path string
		want string
	}{
		{"/items/xml", "application/xml"},
		{"/items/yaml", "application/yaml"},
		{"/plain", "text/plain; charset=utf-8"},
		{"/page", "text/html; charset=utf-8"},
		// c.JSON stays on the config default, which is the knob a project sets
		// to say what its JSON endpoints serve (application/hal+json, a vendor
		// type). Overriding it here would silently drop that setting.
		{"/items/json", "application/json"},
	}
	for _, tc := range cases {
		item, ok := out.Paths[tc.path]
		if !ok {
			t.Errorf("%s missing; have %v", tc.path, mapPathKeys(out.Paths))
			continue
		}
		got := contentTypesOf(opFor(item, "GET"), "200")
		if len(got) != 1 || got[0] != tc.want {
			t.Errorf("GET %s 200 content = %v, want [%s]", tc.path, got, tc.want)
		}
	}
}
