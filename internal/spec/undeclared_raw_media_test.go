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
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"testing"

	"github.com/ehabterra/apispec/internal/metadata"
)

// writeArgs returns the metadata argument of every `w.Write(...)` in src, in
// source order, built the way the metadata layer builds call arguments.
func writeArgs(t *testing.T, src string) []*metadata.CallArgument {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "p.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	info := &types.Info{
		Types:      map[ast.Expr]types.TypeAndValue{},
		Defs:       map[*ast.Ident]types.Object{},
		Uses:       map[*ast.Ident]types.Object{},
		Selections: map[*ast.SelectorExpr]*types.Selection{},
	}
	if _, err := (&types.Config{Importer: importer.Default()}).Check("p", fset, []*ast.File{f}, info); err != nil {
		t.Fatal(err)
	}
	meta := &metadata.Metadata{StringPool: metadata.NewStringPool()}
	var out []*metadata.CallArgument
	ast.Inspect(f, func(n ast.Node) bool {
		c, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if s, ok := c.Fun.(*ast.SelectorExpr); ok && s.Sel.Name == "Write" {
			out = append(out, metadata.ExprToCallArgument(c.Args[0], info, "p", fset, meta))
		}
		return true
	})
	return out
}

// TestUndeclaredRawMediaType pins the media type a raw write's server sends
// when the handler declares none (issue #596): net/http sniffs a constant
// exactly as the server will, a fixed-default server sends its default, and
// bytes the analysis cannot see leave it undetermined.
func TestUndeclaredRawMediaType(t *testing.T) {
	args := writeArgs(t, "package p\n"+
		"import \"net/http\"\n"+
		"const greeting = \"hi\\n\"\n"+
		"type W struct{}\n"+
		"func (W) Write(b []byte) (int, error) { return 0, nil }\n"+
		"func h(w W, b []byte, s string) {\n"+
		"\tw.Write([]byte(\"OK\"))\n"+
		"\tw.Write([]byte(`<!DOCTYPE html><p>x`))\n"+
		"\tw.Write([]byte(\"%PDF-1.7\"))\n"+
		"\tw.Write([]byte(greeting))\n"+
		"\tw.Write([]byte(http.MethodGet))\n"+
		"\tw.Write([]byte(\"\\x00\\x01\"))\n"+
		"\tw.Write(b)\n"+
		"\tw.Write([]byte(s))\n"+
		"\tw.Write([]byte(\"a\" + s))\n"+
		"}\n")
	sniffed := []string{
		"text/plain; charset=utf-8",
		"text/html; charset=utf-8",
		"application/pdf",
		"text/plain; charset=utf-8", // a named constant
		"text/plain; charset=utf-8", // a constant from another package
		"application/octet-stream",
		"", // a []byte variable
		"", // a converted string variable
		"", // not a constant expression
	}
	if len(args) != len(sniffed) {
		t.Fatalf("found %d writes, want %d", len(args), len(sniffed))
	}

	sniff := &ResponsePatternMatcherImpl{BasePatternMatcher: &BasePatternMatcher{}}
	sniff.pattern = ResponsePattern{RawBody: true, SniffContentType: true}
	fixed := &ResponsePatternMatcherImpl{BasePatternMatcher: &BasePatternMatcher{}}
	fixed.pattern = ResponsePattern{RawBody: true, UndeclaredContentType: contentTypeText}
	neither := &ResponsePatternMatcherImpl{BasePatternMatcher: &BasePatternMatcher{}}
	neither.pattern = ResponsePattern{RawBody: true}

	for i, arg := range args {
		if got := sniff.undeclaredRawMediaType(arg); got != sniffed[i] {
			t.Errorf("write %d: sniffed %q, want %q", i, got, sniffed[i])
		}
		// fasthttp sends its default whatever the bytes are.
		if got := fixed.undeclaredRawMediaType(arg); got != contentTypeText {
			t.Errorf("write %d: fixed-default server = %q, want %q", i, got, contentTypeText)
		}
		// A writer whose server's rule is unknown claims nothing.
		if got := neither.undeclaredRawMediaType(arg); got != "" {
			t.Errorf("write %d: no rule = %q, want undetermined", i, got)
		}
	}
}

// TestDocumentUndeclaredRawBytes pins the two outcomes: bytes under the
// server's media type when it is known, and an unknown document under the
// default media type when it is not — never the byte slice's base64 rendering.
func TestDocumentUndeclaredRawBytes(t *testing.T) {
	known := &ResponseInfo{
		ContentType: "application/json", BodyType: "[]byte", RawBytes: true,
		RawMediaType: "text/plain; charset=utf-8",
		Schema:       &Schema{Type: "string", Format: "byte"},
		OneOfTypes:   []string{"[]byte"},
	}
	documentUndeclaredRawBytes(known)
	if known.ContentType != "text/plain; charset=utf-8" || !isBinarySchema(known.Schema) {
		t.Errorf("known media type: got %s %+v, want text/plain bytes", known.ContentType, known.Schema)
	}
	if known.BodyType != "" || known.OneOfTypes != nil || known.RawBytes {
		t.Errorf("known media type: body type %q / oneOf %v / raw %v left behind", known.BodyType, known.OneOfTypes, known.RawBytes)
	}

	unknown := &ResponseInfo{
		ContentType: "application/json", BodyType: "[]byte", RawBytes: true,
		Schema: &Schema{Type: "string", Format: "byte"},
	}
	documentUndeclaredRawBytes(unknown)
	if unknown.ContentType != "application/json" {
		t.Errorf("unknown media type: content type = %q, want the default kept", unknown.ContentType)
	}
	if unknown.Schema == nil || unknown.Schema.Type != "" || unknown.Schema.Format != "" {
		t.Errorf("unknown media type: schema = %+v, want the empty (unknown) schema", unknown.Schema)
	}
}

// TestRawDocumentSchema pins that bytes documented as a default-media-type
// document keep a schema their type states (json.RawMessage's "arbitrary
// JSON"), and drop only the byte slice's base64 rendering and a $ref nothing
// would register (issue #596).
func TestRawDocumentSchema(t *testing.T) {
	rawMessage := &Schema{Description: "Arbitrary JSON, copied verbatim (encoding/json.RawMessage)."}
	cases := []struct {
		name   string
		schema *Schema
		want   *Schema
	}{
		{"json.RawMessage keeps its schema", rawMessage, rawMessage},
		{"a byte slice's base64 rendering is dropped", &Schema{Type: "string", Format: "byte"}, &Schema{}},
		{"a $ref is dropped", &Schema{Ref: "#/components/schemas/Doc"}, &Schema{}},
		{"no schema", nil, &Schema{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := rawDocumentSchema(&ResponseInfo{Schema: tc.schema})
			if got == nil || got.Type != tc.want.Type || got.Format != tc.want.Format ||
				got.Ref != tc.want.Ref || got.Description != tc.want.Description {
				t.Errorf("rawDocumentSchema = %+v, want %+v", got, tc.want)
			}
		})
	}
}
