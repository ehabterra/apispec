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

// TestTestdata_MultipartUpload covers issue #207: a handler that reads a
// multipart body must document multipart/form-data, with file parts as binary
// strings.
//
// Before this, a file upload was documented twice wrong — the media type said
// application/x-www-form-urlencoded, which is not what the handler parses, and
// the file part was missing entirely, so the one field the endpoint exists for
// was undocumented.
func TestTestdata_MultipartUpload(t *testing.T) {
	out := loadTestdataWithFixtureConfig(t, "multipart_upload", nil)
	noDanglingRefs(t, out)
	noUnresolvedPlaceholders(t, out)

	for _, tc := range []struct {
		path        string
		contentType string
		// props maps a property name to its expected format ("binary" for a
		// file part, "" for a text field).
		props map[string]string
		why   string
	}{
		{
			path:        "/upload",
			contentType: "multipart/form-data",
			props:       map[string]string{"avatar": "binary", "title": ""},
			why:         "ParseMultipartForm + FormFile + FormValue",
		},
		{
			path:        "/documents",
			contentType: "multipart/form-data",
			props:       map[string]string{"document": "binary"},
			why:         "FormFile alone implies multipart",
		},
		{
			path:        "/captions",
			contentType: "multipart/form-data",
			props:       map[string]string{"caption": ""},
			why:         "ParseMultipartForm alone: no part read by name",
		},
		{
			// The #171 control: without multipart evidence the media type must
			// not drift to multipart.
			path:        "/plain",
			contentType: "application/x-www-form-urlencoded",
			props:       map[string]string{"name": ""},
			why:         "FormValue only — stays urlencoded",
		},
	} {
		op := opFor(out.Paths[tc.path], "POST")
		if op == nil {
			t.Errorf("POST %s missing; have %v", tc.path, mapPathKeys(out.Paths))
			continue
		}
		if op.RequestBody == nil {
			t.Errorf("POST %s: requestBody missing (%s)", tc.path, tc.why)
			continue
		}
		mt, ok := op.RequestBody.Content[tc.contentType]
		if !ok {
			t.Errorf("POST %s: content type %q missing (%s); have %v",
				tc.path, tc.contentType, tc.why, contentTypes(op.RequestBody))
			continue
		}
		if mt.Schema == nil {
			t.Errorf("POST %s: %s schema missing", tc.path, tc.contentType)
			continue
		}
		for name, format := range tc.props {
			prop, ok := mt.Schema.Properties[name]
			if !ok {
				t.Errorf("POST %s: property %q missing (%s); have %v",
					tc.path, name, tc.why, propNames(mt.Schema))
				continue
			}
			if prop.Type != "string" {
				t.Errorf("POST %s: property %q type = %q, want string", tc.path, name, prop.Type)
			}
			if prop.Format != format {
				t.Errorf("POST %s: property %q format = %q, want %q", tc.path, name, prop.Format, format)
			}
		}
		// A urlencoded body must never carry a binary part, and a multipart body
		// must not also be offered as urlencoded — the media type is a claim
		// about what the handler parses, not a list of possibilities.
		if len(op.RequestBody.Content) != 1 {
			t.Errorf("POST %s: expected exactly one request content type, have %v",
				tc.path, contentTypes(op.RequestBody))
		}
	}
}

// TestTestdata_MultipartUploadGin is the same fact reached through a framework
// context receiver (gin's c.FormFile / c.MultipartForm) rather than
// *http.Request, since detection has to be framework-agnostic (golden rule #5).
//
// It also covers gin's PostForm read, which had no pattern at all: gin form
// bodies documented no fields, so the text parts of a gin upload were missing
// even once the file part resolved.
func TestTestdata_MultipartUploadGin(t *testing.T) {
	out := loadTestdataWithFixtureConfig(t, "multipart_upload_gin", nil)
	noDanglingRefs(t, out)
	noUnresolvedPlaceholders(t, out)

	avatar := opFor(out.Paths["/avatar"], "POST")
	if avatar == nil || avatar.RequestBody == nil {
		t.Fatalf("POST /avatar missing or bodyless; have %v", mapPathKeys(out.Paths))
	}
	mt, ok := avatar.RequestBody.Content["multipart/form-data"]
	if !ok {
		t.Fatalf("POST /avatar: multipart/form-data missing; have %v", contentTypes(avatar.RequestBody))
	}
	if mt.Schema == nil || mt.Schema.Properties["avatar"] == nil {
		t.Fatalf("POST /avatar: file part 'avatar' missing; have %v", propNames(mt.Schema))
	}
	if got := mt.Schema.Properties["avatar"].Format; got != "binary" {
		t.Errorf("POST /avatar: avatar format = %q, want binary", got)
	}
	if mt.Schema.Properties["title"] == nil {
		t.Errorf("POST /avatar: text part 'title' missing (gin PostForm); have %v", propNames(mt.Schema))
	}

	// c.MultipartForm() with no part read by name.
	batch := opFor(out.Paths["/batch"], "POST")
	if batch == nil || batch.RequestBody == nil {
		t.Fatalf("POST /batch missing or bodyless; have %v", mapPathKeys(out.Paths))
	}
	if _, ok := batch.RequestBody.Content["multipart/form-data"]; !ok {
		t.Errorf("POST /batch: multipart/form-data missing — c.MultipartForm() is the only evidence; have %v",
			contentTypes(batch.RequestBody))
	}
}

func contentTypes(body *intspec.RequestBody) []string {
	if body == nil {
		return nil
	}
	out := make([]string, 0, len(body.Content))
	for ct := range body.Content {
		out = append(out, ct)
	}
	return out
}

func propNames(schema *intspec.Schema) []string {
	if schema == nil {
		return nil
	}
	out := make([]string, 0, len(schema.Properties))
	for name := range schema.Properties {
		out = append(out, name)
	}
	return out
}
