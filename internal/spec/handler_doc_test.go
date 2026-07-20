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
	"testing"

	"github.com/ehabterra/apispec/internal/metadata"
)

// docMeta builds a package "app" holding:
//   - func Plain          — documented package-level function
//   - type Handler        — with documented method Create and undocumented Patch
//   - type Deps{H *Handler} — so a field-path receiver has something to resolve
func docMeta(t *testing.T) *metadata.Metadata {
	t.Helper()
	meta := &metadata.Metadata{StringPool: metadata.NewStringPool()}

	handler := &metadata.Type{
		Name: meta.StringPool.Get("Handler"),
		Methods: []metadata.Method{
			{
				Name:     meta.StringPool.Get("Create"),
				Receiver: meta.StringPool.Get("*Handler"),
				Comments: meta.StringPool.Get("Create makes a thing.\nAnd describes it."),
			},
			{
				Name:     meta.StringPool.Get("Patch"),
				Receiver: meta.StringPool.Get("*Handler"),
				Comments: meta.StringPool.Get(""),
			},
		},
	}
	deps := &metadata.Type{
		Name: meta.StringPool.Get("Deps"),
		Fields: []metadata.Field{
			{Name: meta.StringPool.Get("H"), Type: meta.StringPool.Get("*app.Handler")},
		},
	}
	meta.Packages = map[string]*metadata.Package{
		"app": {
			Types: map[string]*metadata.Type{"Handler": handler, "Deps": deps},
			Files: map[string]*metadata.File{
				"app.go": {
					Types: map[string]*metadata.Type{"Handler": handler, "Deps": deps},
					Functions: map[string]*metadata.Function{
						"Plain": {
							Name:     meta.StringPool.Get("Plain"),
							Comments: meta.StringPool.Get("Plain serves a thing."),
						},
					},
				},
			},
		},
	}
	return meta
}

// TestHandlerDoc covers every RouteInfo.Function shape. The method shapes are
// the regression: methods live only in the per-Type table, so the original
// findFunctionByName-only lookup returned nothing for them (issue #168).
func TestHandlerDoc(t *testing.T) {
	meta := docMeta(t)

	for _, tc := range []struct {
		name, function        string
		wantSummary, wantDesc string
	}{
		{
			name:        "package-level function",
			function:    "app.Plain",
			wantSummary: "Plain serves a thing.",
		},
		{
			name:        "method value on a variable",
			function:    "app" + TypeSep + "app.Handler.Create",
			wantSummary: "Create makes a thing.",
			wantDesc:    "And describes it.",
		},
		{
			name:        "method value on a struct field",
			function:    "app" + TypeSep + "Deps.H.Create",
			wantSummary: "Create makes a thing.",
			wantDesc:    "And describes it.",
		},
		{
			name:     "undocumented method",
			function: "app" + TypeSep + "app.Handler.Patch",
		},
		{
			// Honest-empty: the field exists but names no method, so there is
			// nothing to resolve. Matching on the method name alone would guess
			// (golden rule #7) — see issue #204.
			name:     "handler value with no method segment",
			function: "app" + TypeSep + "Deps.H",
		},
		{
			name:     "unknown receiver",
			function: "app" + TypeSep + "app.Missing.Create",
		},
		{
			name:     "func literal",
			function: "app.FuncLit:/tmp/app.go:12:3",
		},
		{
			// Regression: some render paths separate the package with a plain
			// dot instead of TypeSep. Every fixture happened to produce the
			// TypeSep form, so a TypeSep-only implementation passed the whole
			// suite while resolving nothing on real projects.
			name:        "dotted separator instead of TypeSep",
			function:    "app.Handler.Create",
			wantSummary: "Create makes a thing.",
			wantDesc:    "And describes it.",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			route := &RouteInfo{Metadata: meta, Package: "app", Function: tc.function}
			summary, desc := handlerDoc(route)
			if summary != tc.wantSummary {
				t.Errorf("summary: got %q, want %q", summary, tc.wantSummary)
			}
			if desc != tc.wantDesc {
				t.Errorf("description: got %q, want %q", desc, tc.wantDesc)
			}
		})
	}
}

// TestHandlerDocImportPathPackage repeats the shapes with a real import-path
// package name. The path itself contains dots, so the package prefix has to be
// stripped before the receiver/method split — splitting on the last dot of the
// raw string alone would work here by luck but the prefix strip is what makes it
// correct, and real projects are always this shape.
func TestHandlerDocImportPathPackage(t *testing.T) {
	const pkg = "github.com/acme/svc/internal/http"
	meta := docMeta(t)
	// Re-key the fixture package under the import path.
	meta.Packages[pkg] = meta.Packages["app"]
	delete(meta.Packages, "app")

	for _, tc := range []struct{ name, function, want string }{
		{"dotted method value", pkg + ".Handler.Create", "Create makes a thing."},
		{"TypeSep method value", pkg + TypeSep + pkg + ".Handler.Create", "Create makes a thing."},
		{"package-level func", pkg + ".Plain", "Plain serves a thing."},
	} {
		t.Run(tc.name, func(t *testing.T) {
			route := &RouteInfo{Metadata: meta, Package: pkg, Function: tc.function}
			if got, _ := handlerDoc(route); got != tc.want {
				t.Errorf("summary: got %q, want %q", got, tc.want)
			}
		})
	}
}

// TestSplitSynopsis pins the sentence split. Splitting on the first *line*
// truncates mid-sentence, because real doc comments wrap — that produced
// summaries like "…origin publisher (admin-only). PUT" on a real project.
func TestSplitSynopsis(t *testing.T) {
	for _, tc := range []struct{ name, text, wantSum, wantDesc string }{
		{
			name:     "sentence wraps across lines",
			text:     "setSource records an asset's origin publisher (admin-only). PUT\nbecause it replaces the record.",
			wantSum:  "setSource records an asset's origin publisher (admin-only).",
			wantDesc: "PUT\nbecause it replaces the record.",
		},
		{
			name:    "single sentence spanning two lines has no remainder",
			text:    "usage returns the reference graph — collections that assemble it and\nlessons that use it.",
			wantSum: "usage returns the reference graph — collections that assemble it and lessons that use it.",
		},
		{
			name:     "sentence per line",
			text:     "Create makes a thing.\nAnd describes it.",
			wantSum:  "Create makes a thing.",
			wantDesc: "And describes it.",
		},
		{
			name:    "no terminator",
			text:    "listAccounts returns every account",
			wantSum: "listAccounts returns every account",
		},
		{name: "empty", text: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sum, desc := splitSynopsis(tc.text)
			if sum != tc.wantSum {
				t.Errorf("summary: got %q, want %q", sum, tc.wantSum)
			}
			if desc != tc.wantDesc {
				t.Errorf("description: got %q, want %q", desc, tc.wantDesc)
			}
		})
	}
}

// TestSwaggoDoc pins swaggo/swag annotation handling: @Summary/@Description are
// consumed, every other directive is dropped rather than swept into the prose,
// and a comment with no annotations is left to the sentence split.
func TestSwaggoDoc(t *testing.T) {
	for _, tc := range []struct {
		name, text, wantSum, wantDesc string
		wantOK                        bool
	}{
		{
			name:     "full annotation block",
			text:     "CreateAccount godoc\n@Summary      Create an account\n@Description  Registers a new account.\n@Tags         accounts\n@Router       /accounts [post]",
			wantSum:  "Create an account",
			wantDesc: "Registers a new account.",
			wantOK:   true,
		},
		{
			name:     "multi-line description",
			text:     "@Summary Search\n@Description  Filters by query.\n@Description  Empty list when nothing matches.",
			wantSum:  "Search",
			wantDesc: "Filters by query.\nEmpty list when nothing matches.",
			wantOK:   true,
		},
		{
			name:     "continuation line belongs to the annotation above",
			text:     "@Summary Create an account\n@Description Registers a new account\nand returns the created record.",
			wantSum:  "Create an account",
			wantDesc: "Registers a new account\nand returns the created record.",
			wantOK:   true,
		},
		{
			name:    "no @Summary falls back to the prose above the annotations",
			text:    "CreateAccount registers a new account.\n@Tags accounts\n@Router /accounts [post]",
			wantSum: "CreateAccount registers a new account.",
			wantOK:  true,
		},
		{
			name:     "only @Description",
			text:     "@Description Registers a new account.",
			wantDesc: "Registers a new account.",
			wantOK:   true,
		},
		{
			name: "plain doc comment is not swaggo",
			text: "Create makes a thing.\nAnd describes it.",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sum, desc, ok := swaggoDoc(tc.text)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if sum != tc.wantSum {
				t.Errorf("summary: got %q, want %q", sum, tc.wantSum)
			}
			if desc != tc.wantDesc {
				t.Errorf("description: got %q, want %q", desc, tc.wantDesc)
			}
		})
	}
}

// TestHandlerDocGuards covers the nil/empty short-circuits.
func TestHandlerDocGuards(t *testing.T) {
	for _, tc := range []struct {
		name  string
		route *RouteInfo
	}{
		{"nil route", nil},
		{"nil metadata", &RouteInfo{Function: "app.Plain"}},
		{"empty function", &RouteInfo{Metadata: docMeta(t)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if s, d := handlerDoc(tc.route); s != "" || d != "" {
				t.Errorf("got (%q, %q), want empty", s, d)
			}
		})
	}
}

// TestReceiverTypeName pins the receiver-segment resolution: a bare type name
// passes through, a field path resolves through the field's type (pointer
// unwrapped to its named core), and an unresolvable path falls back to the last
// segment rather than matching broadly.
func TestReceiverTypeName(t *testing.T) {
	meta := docMeta(t)

	for _, tc := range []struct{ name, recv, want string }{
		{"bare type name", "Handler", "Handler"},
		{"field path resolves to the field's type", "Deps.H", "Handler"},
		{"unknown owner falls back to the last segment", "Nope.Handler", "Handler"},
		{"unknown field falls back to the last segment", "Deps.Missing", "Missing"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := receiverTypeName(meta, "app", tc.recv); got != tc.want {
				t.Errorf("receiverTypeName(%q) = %q, want %q", tc.recv, got, tc.want)
			}
		})
	}
}

// TestFindMethodByName covers the per-Type method lookup, including the pointer
// receiver trim and the unknown-package guard.
func TestFindMethodByName(t *testing.T) {
	meta := docMeta(t)

	if m := findMethodByName(meta, "app", "Handler", "Create"); m == nil {
		t.Error("value receiver should match the *Handler record (leading * trimmed)")
	}
	if m := findMethodByName(meta, "app", "", "Create"); m == nil {
		t.Error("empty receiver should match on the method name alone")
	}
	if m := findMethodByName(meta, "app", "Other", "Create"); m != nil {
		t.Error("a non-matching receiver must not resolve")
	}
	if m := findMethodByName(meta, "nosuch", "Handler", "Create"); m != nil {
		t.Error("unknown package must not resolve")
	}
	if m := findMethodByName(nil, "app", "Handler", "Create"); m != nil {
		t.Error("nil metadata must not resolve")
	}
}
