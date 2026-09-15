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

package spec

import (
	"slices"
	"testing"
)

func mediaResp(ct, bodyType string, schema *Schema) *ResponseInfo {
	return &ResponseInfo{ContentType: ct, BodyType: bodyType, Schema: schema, StatusCode: 200}
}

// otherMediaType decides whether two fragments of one status are different
// REPRESENTATIONS — which compose — or competing descriptions of the same one,
// which must keep displacing each other as before (issue #470).
func TestOtherMediaType(t *testing.T) {
	json := mediaResp("application/json", "pkg.Item", &Schema{Ref: "#/x"})
	xml := mediaResp("application/xml", "pkg.Item", &Schema{Ref: "#/x"})

	if !otherMediaType(json, xml) {
		t.Error("two media types for one status are different representations and must compose")
	}
	if otherMediaType(json, mediaResp("application/json", "pkg.Other", &Schema{Ref: "#/y"})) {
		t.Error("the same media type twice is not a second representation — that is the anyOf case")
	}
	// A fragment with no body is not a representation: admitting it would
	// advertise a media type the handler never writes.
	if otherMediaType(json, mediaResp("application/xml", "", nil)) {
		t.Error("a bodyless fragment was treated as a representation")
	}
	// Nor is an UNTYPED CONSTANT. No handler encodes a literal as its payload;
	// where one turns up it is an argument read as the body — on a large real
	// project every second representation this rule excludes was `untyped int`,
	// the status argument of an error helper (issue #470).
	if otherMediaType(json, mediaResp("application/xml", "untyped int", &Schema{Type: "integer"})) {
		t.Error("an untyped constant was treated as a serialised body")
	}
	if otherMediaType(mediaResp("application/json", "untyped string", &Schema{Type: "string"}), xml) {
		t.Error("an untyped constant was treated as a serialised body")
	}
	if otherMediaType(mediaResp("", "pkg.Item", nil), xml) {
		t.Error("a fragment with no media type was treated as a representation")
	}
	if otherMediaType(nil, xml) || otherMediaType(json, nil) {
		t.Error("a nil fragment was treated as a representation")
	}
}

// The primary pair is never displaced, and a third representation accumulates
// rather than replacing the second.
func TestAddAlternateMediaType(t *testing.T) {
	json := mediaResp("application/json", "pkg.Item", &Schema{Ref: "#/json"})
	xml := mediaResp("application/xml", "pkg.Item", &Schema{Ref: "#/xml"})
	yaml := mediaResp("application/yaml", "pkg.Item", &Schema{Ref: "#/yaml"})

	got := addAlternateMediaType(json, xml)
	if got.ContentType != "application/json" || got.Schema.Ref != "#/json" {
		t.Errorf("the primary pair moved: %q %+v", got.ContentType, got.Schema)
	}
	if s := got.Alternates["application/xml"]; s == nil || s.Ref != "#/xml" {
		t.Errorf("alternate not recorded: %+v", got.Alternates)
	}

	// Three representations arrive as two merges.
	got = addAlternateMediaType(got, yaml)
	var cts []string
	for ct := range got.Alternates {
		cts = append(cts, ct)
	}
	slices.Sort(cts)
	if !slices.Equal(cts, []string{"application/xml", "application/yaml"}) {
		t.Errorf("alternates = %v, want both the xml and yaml representations", cts)
	}

	// The input is not mutated: fragments are shared, and a merge that wrote
	// through would leak one response's representations into another.
	if json.Alternates != nil {
		t.Error("addAlternateMediaType mutated its input")
	}

	// A fragment whose media type IS the primary one adds nothing, so a
	// re-merge cannot shadow the primary schema with a copy.
	same := addAlternateMediaType(json, mediaResp("application/json", "pkg.Item", &Schema{Ref: "#/other"}))
	if _, dup := same.Alternates["application/json"]; dup {
		t.Error("the primary media type was also recorded as an alternate")
	}
}

// buildResponses emits one content entry per representation, in sorted order so
// the document cannot depend on map iteration (golden rule #1).
func TestBuildResponsesEmitsEveryMediaType(t *testing.T) {
	r := mediaResp("application/json", "pkg.Item", &Schema{Ref: "#/json"})
	r.Alternates = map[string]*Schema{
		"application/xml":  {Ref: "#/xml"},
		"application/yaml": {Ref: "#/yaml"},
	}
	out := buildResponses(map[string]*ResponseInfo{"200": r})

	got := out["200"].Content
	if len(got) != 3 {
		t.Fatalf("content has %d entries, want 3: %+v", len(got), got)
	}
	for ct, want := range map[string]string{
		"application/json": "#/json", "application/xml": "#/xml", "application/yaml": "#/yaml",
	} {
		if mt, ok := got[ct]; !ok || mt.Schema == nil || mt.Schema.Ref != want {
			t.Errorf("content[%q] = %+v, want the %s schema", ct, mt.Schema, want)
		}
	}

	// A response with no alternates is untouched — the ordinary case.
	plain := buildResponses(map[string]*ResponseInfo{"200": mediaResp("application/json", "pkg.Item", &Schema{Ref: "#/j"})})
	if len(plain["200"].Content) != 1 {
		t.Errorf("a single-representation response gained entries: %+v", plain["200"].Content)
	}
}

// The two composition paths are different answers to different questions, and
// the boundary between them is what #470 added: two bodies under ONE media type
// are alternatives the status may carry (anyOf), while two MEDIA TYPES are
// separate representations of the response (separate content entries). Getting
// this backwards would either bury a representation inside an anyOf or
// advertise an error body as a second format.
func TestMergePathsAreDistinct(t *testing.T) {
	item := mediaResp("application/json", "pkg.Item", &Schema{Ref: "#/item"})
	failure := mediaResp("application/json", "pkg.Error", &Schema{Ref: "#/err"})

	// Same media type, different bodies: alternatives under one representation.
	if otherMediaType(item, failure) {
		t.Fatal("two bodies of one media type are not separate representations")
	}
	if !alternativeBodies(item, failure) {
		t.Fatal("two concrete, differently-shaped bodies are alternatives")
	}
	merged := mergeResponseAlternatives(item, failure)
	if merged.Schema == nil || len(merged.Schema.AnyOf) != 2 {
		t.Errorf("merged schema = %+v, want an anyOf of both bodies", merged.Schema)
	}
	if merged.ContentType != "application/json" || len(merged.Alternates) != 0 {
		t.Errorf("an anyOf merge invented a representation: %q %+v",
			merged.ContentType, merged.Alternates)
	}

	// A merge that adds nothing returns the original: an empty schema matches
	// anything, so an anyOf containing it says LESS than its other members.
	if got := mergeResponseAlternatives(item, mediaResp("application/json", "pkg.X", &Schema{})); got != item {
		t.Errorf("an empty alternative was merged in: %+v", got.Schema)
	}
	if got := mergeResponseAlternatives(item, nil); got != item {
		t.Errorf("a nil alternative was merged in: %+v", got)
	}
	if got := mergeResponseAlternatives(nil, item); got != item {
		t.Error("merging into nothing lost the fragment")
	}

	// The same body twice is not an alternative at all.
	if alternativeBodies(item, mediaResp("application/json", "pkg.Item", &Schema{Ref: "#/item"})) {
		t.Error("one body was treated as two alternatives")
	}
}

// serialisedBody is the rule that keeps composition from surfacing a
// mis-resolved argument as a second representation.
func TestSerialisedBody(t *testing.T) {
	cases := []struct {
		bodyType string
		want     bool
	}{
		{"pkg.Item", true},
		{"string", true}, // what http.Error writes: a real value, just a primitive
		{"int", true},    // a typed int is a value; only the UNTYPED form is a literal
		{"untyped int", false},
		{"untyped string", false},
		{"untyped bool", false},
		{"", false},
	}
	for _, tc := range cases {
		got := serialisedBody(&ResponseInfo{BodyType: tc.bodyType})
		if got != tc.want {
			t.Errorf("serialisedBody(%q) = %v, want %v", tc.bodyType, got, tc.want)
		}
	}
	if serialisedBody(nil) {
		t.Error("a nil fragment has no body")
	}
}

// Three fragments, two of them sharing a media type. Before, the third
// overwrote the second and the XML item was documented as the XML error —
// a representation silently deleted (CodeRabbit on #470).
func TestAlternateSchemasCompose(t *testing.T) {
	jsonItem := mediaResp("application/json", "pkg.Item", &Schema{Ref: "#/item"})
	xmlItem := mediaResp("application/xml", "pkg.Item", &Schema{Ref: "#/item"})
	xmlErr := mediaResp("application/xml", "pkg.Error", &Schema{Ref: "#/err"})

	got := addAlternateMediaType(addAlternateMediaType(jsonItem, xmlItem), xmlErr)

	if got.ContentType != "application/json" || got.Schema.Ref != "#/item" {
		t.Errorf("the primary pair moved: %q %+v", got.ContentType, got.Schema)
	}
	xml := got.Alternates["application/xml"]
	if xml == nil {
		t.Fatal("the xml representation is gone")
	}
	if len(xml.AnyOf) != 2 {
		t.Fatalf("xml schema = %+v, want an anyOf of BOTH bodies it can carry", xml)
	}
	refs := []string{xml.AnyOf[0].Ref, xml.AnyOf[1].Ref}
	slices.Sort(refs)
	if !slices.Equal(refs, []string{"#/err", "#/item"}) {
		t.Errorf("xml anyOf = %v, want both the item and the error", refs)
	}

	// Every type is registered for component collection, which reads
	// OneOfTypes — not the alternate schemas — so a $ref appearing only under a
	// second media type would otherwise dangle.
	types := append([]string(nil), got.OneOfTypes...)
	slices.Sort(types)
	if !slices.Equal(types, []string{"pkg.Error", "pkg.Item"}) {
		t.Errorf("OneOfTypes = %v, want every type any representation names", types)
	}
}
