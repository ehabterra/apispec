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

// wireNameMeta builds metadata declaring one string constant in each of two
// packages, plus an external package that is NOT analysed.
func wireNameMeta() *metadata.Metadata {
	m := &metadata.Metadata{StringPool: metadata.NewStringPool()}
	constFile := &metadata.File{Variables: map[string]*metadata.Variable{
		"CSRFHeader": {Value: m.StringPool.Get(`"X-CSRF-Token"`)},
	}}
	// Two files in one package, the constant in the later one, to prove the
	// lookup does not depend on map order.
	m.Packages = map[string]*metadata.Package{
		"example.com/pkg/csrf": {Files: map[string]*metadata.File{
			"aaa.go": {Variables: map[string]*metadata.Variable{}},
			"zzz.go": constFile,
		}},
	}
	return m
}

// TestConstantValue covers the resolution a parameter's wire name depends on:
// what the client actually sends, or nothing.
func TestConstantValue(t *testing.T) {
	m := wireNameMeta()
	cp := NewContextProvider(m)

	lit := metadata.NewCallArgument(m)
	lit.SetKind(metadata.KindLiteral)
	lit.SetValue(`"page"`)
	if got, ok := cp.ConstantValue(lit); !ok || got != "page" {
		t.Errorf("literal = (%q, %v), want (page, true)", got, ok)
	}

	// A constant this project declares resolves to its VALUE — the header a
	// client must send, not the Go identifier.
	sel := metadata.NewCallArgument(m)
	sel.SetKind(metadata.KindSelector)
	base := metadata.NewCallArgument(m)
	base.SetKind(metadata.KindIdent)
	base.SetName("csrf")
	base.SetPkg("example.com/pkg/csrf")
	name := metadata.NewCallArgument(m)
	name.SetKind(metadata.KindIdent)
	name.SetName("CSRFHeader")
	name.SetPkg("example.com/pkg/csrf")
	sel.X = base
	sel.Sel = name
	if got, ok := cp.ConstantValue(sel); !ok || got != "X-CSRF-Token" {
		t.Errorf("project constant = (%q, %v), want (X-CSRF-Token, true)", got, ok)
	}

	// A constant from a package outside the analysed set has no recorded value.
	// Rendering it produced `name: github.com/labstack/echo.HeaderXRequestID`,
	// a header no request can carry — so it must resolve to nothing instead.
	external := metadata.NewCallArgument(m)
	external.SetKind(metadata.KindSelector)
	extBase := metadata.NewCallArgument(m)
	extBase.SetKind(metadata.KindIdent)
	extBase.SetName("echo")
	extBase.SetPkg("github.com/labstack/echo/v4")
	extName := metadata.NewCallArgument(m)
	extName.SetKind(metadata.KindIdent)
	extName.SetName("HeaderXRequestID")
	extName.SetPkg("github.com/labstack/echo/v4")
	external.X = extBase
	external.Sel = extName
	if got, ok := cp.ConstantValue(external); ok {
		t.Errorf("external constant resolved to %q; it is not knowable", got)
	}

	// A plain variable is not a constant, and neither is a nil argument.
	varArg := metadata.NewCallArgument(m)
	varArg.SetKind(metadata.KindIdent)
	varArg.SetName("field")
	varArg.SetPkg("example.com/pkg/csrf")
	if got, ok := cp.ConstantValue(varArg); ok {
		t.Errorf("undeclared ident resolved to %q", got)
	}
	if _, ok := cp.ConstantValue(nil); ok {
		t.Error("nil argument resolved")
	}
	// Kinds that cannot name anything.
	call := metadata.NewCallArgument(m)
	call.SetKind(metadata.KindCall)
	if _, ok := cp.ConstantValue(call); ok {
		t.Error("a call resolved as a constant")
	}
}

// TestParamWireNameRejectsUnknown pins the extraction rule: a parameter whose
// wire name is unknown is not emitted at all.
func TestParamWireNameRejectsUnknown(t *testing.T) {
	m := wireNameMeta()
	p := NewParamPatternMatcher(
		ParamPattern{CallRegex: "^Get$", ParamIn: "header"},
		&APISpecConfig{}, NewContextProvider(m), nil,
	)

	lit := metadata.NewCallArgument(m)
	lit.SetKind(metadata.KindLiteral)
	lit.SetValue(`"X-Tenant"`)
	if got, ok := p.paramWireName(lit, nil); !ok || got != "X-Tenant" {
		t.Errorf("literal = (%q, %v), want (X-Tenant, true)", got, ok)
	}

	// The shape that produced the bogus header: an external constant, with no
	// node to trace a parameter through.
	external := metadata.NewCallArgument(m)
	external.SetKind(metadata.KindSelector)
	base := metadata.NewCallArgument(m)
	base.SetKind(metadata.KindIdent)
	base.SetName("echo")
	base.SetPkg("github.com/labstack/echo/v4")
	sel := metadata.NewCallArgument(m)
	sel.SetKind(metadata.KindIdent)
	sel.SetName("HeaderXRequestID")
	external.X = base
	external.Sel = sel
	if got, ok := p.paramWireName(external, nil); ok {
		t.Errorf("external constant produced the name %q; want no name", got)
	}

	if _, ok := p.paramWireName(nil, nil); ok {
		t.Error("nil argument produced a name")
	}
}
