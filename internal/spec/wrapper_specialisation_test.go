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

// Field-binding recovery in isolation: build a CallArgument tree
// mirroring `return &T{Message: m, Data: d, Code: c}` and confirm
// the binding extractor identifies (Data ← d) as well as the
// concrete-typed siblings (which the spec layer filters later).
func TestFieldParamBindingsFromReturnVar(t *testing.T) {
	meta := &metadata.Metadata{StringPool: metadata.NewStringPool()}

	// Constructor signature: `NewEnvelope(m string, d any, c int) *T`.
	// Only the parameter-name set matters here — the spec layer
	// later filters bindings by the wrapper field's declared type.
	mkParam := func(name string) *metadata.CallArgument {
		p := metadata.NewCallArgument(meta)
		p.SetKind(metadata.KindIdent)
		p.SetName(name)
		return p
	}
	ctor := &metadata.Function{
		Signature: metadata.CallArgument{
			Args: []*metadata.CallArgument{mkParam("m"), mkParam("d"), mkParam("c")},
		},
	}

	mkKV := func(key, val string) *metadata.CallArgument {
		k := metadata.NewCallArgument(meta)
		k.SetKind(metadata.KindIdent)
		k.SetName(key)

		v := metadata.NewCallArgument(meta)
		v.SetKind(metadata.KindIdent)
		v.SetName(val)

		kv := metadata.NewCallArgument(meta)
		kv.SetKind(metadata.KindKeyValue)
		kv.X = k
		kv.Fun = v
		return kv
	}

	cl := metadata.NewCallArgument(meta)
	cl.SetKind(metadata.KindCompositeLit)
	cl.Args = []*metadata.CallArgument{
		mkKV("Message", "m"),
		mkKV("Data", "d"),
		mkKV("Code", "c"),
	}

	// Address-of wrapper: `&T{...}`.
	ret := metadata.NewCallArgument(meta)
	ret.SetKind(metadata.KindUnary)
	ret.X = cl

	got := fieldParamBindingsFromReturnVar(ret, ctor)
	want := map[string]string{"Message": "m", "Data": "d", "Code": "c"}
	if len(got) != len(want) {
		t.Fatalf("len mismatch: got %d (%v), want %d (%v)", len(got), got, len(want), want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("binding[%q] = %q, want %q", k, got[k], v)
		}
	}
}

// A field whose value isn't a parameter ident (literal, selector,
// nested struct lit, etc.) must NOT show up as a binding.
func TestFieldParamBindingsFromReturnVar_NonParamValueIgnored(t *testing.T) {
	meta := &metadata.Metadata{StringPool: metadata.NewStringPool()}

	p := metadata.NewCallArgument(meta)
	p.SetKind(metadata.KindIdent)
	p.SetName("m")
	ctor := &metadata.Function{
		Signature: metadata.CallArgument{Args: []*metadata.CallArgument{p}},
	}

	mkKV := func(key string, value *metadata.CallArgument) *metadata.CallArgument {
		k := metadata.NewCallArgument(meta)
		k.SetKind(metadata.KindIdent)
		k.SetName(key)
		kv := metadata.NewCallArgument(meta)
		kv.SetKind(metadata.KindKeyValue)
		kv.X = k
		kv.Fun = value
		return kv
	}

	literal := metadata.NewCallArgument(meta)
	literal.SetKind(metadata.KindLiteral)
	literal.SetValue("\"hello\"")

	stray := metadata.NewCallArgument(meta)
	stray.SetKind(metadata.KindIdent)
	stray.SetName("someLocal") // not in the param set

	cl := metadata.NewCallArgument(meta)
	cl.SetKind(metadata.KindCompositeLit)
	cl.Args = []*metadata.CallArgument{
		mkKV("Message", literal),
		mkKV("Note", stray),
	}

	got := fieldParamBindingsFromReturnVar(cl, ctor)
	if len(got) != 0 {
		t.Fatalf("expected no bindings, got %v", got)
	}
}

// cleanOverrideType is the safety net that keeps the wrapper
// specialiser from emitting $refs to non-existent components when the
// caller-side argument resolves to something that isn't a real Go
// type. Pinning the rejected shapes here.
func TestCleanOverrideType(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		// Stripped, kept.
		{"*github.com/foo/bar.Baz", "github.com/foo/bar.Baz"},
		{"&pkg.T", "pkg.T"},
		{"[]pkg.T", "[]pkg.T"},
		{"pkg.T", "pkg.T"},
		{"string", "string"},
		{"int", "int"},

		// Rejected: no information vs. wrapper's declared field.
		{"", ""},
		{"interface{}", ""},
		{"any", ""},
		{"*interface{}", ""},

		// Rejected: untyped constants.
		{"untyped bool", ""},
		{"untyped int", ""},

		// Rejected: bare identifiers that aren't real types — almost
		// always a function name that leaked through.
		{"mapToGeneric", ""},
		{"ToCartDTOFromDomainCart", ""},
	}
	for _, c := range cases {
		if got := cleanOverrideType(c.in); got != c.want {
			t.Errorf("cleanOverrideType(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// specialiseWrapperSchema must register the payload type it references
// from the `data` override so the $ref never dangles. This pins the fix
// for a real-world wrapped-payload specialisation bug: when the payload type
// isn't present in the analysed metadata (an external/vendored type),
// mapGoTypeToOpenAPISchema returns the $ref plus a placeholder in its
// second return value — which the specialiser must fold into usedTypes
// rather than discard, otherwise generateSchemas emits no component and
// Redoc rejects the spec as "Invalid reference token".
func TestSpecialiseWrapperSchema_RegistersPayloadComponent(t *testing.T) {
	pool := metadata.NewStringPool()
	meta := &metadata.Metadata{StringPool: pool}

	// A wrapper type `Envelope { data interface{} `json:"data"` }`
	// reachable via typeByName(parts, meta).
	wrapper := &metadata.Type{
		Name: pool.Get("Envelope"),
		Fields: []metadata.Field{
			{Name: pool.Get("Data"), Type: pool.Get("interface{}"), Tag: pool.Get(`json:"data"`)},
		},
	}
	meta.Packages = map[string]*metadata.Package{
		"envpkg": {Files: map[string]*metadata.File{
			"envpkg.go": {Types: map[string]*metadata.Type{"Envelope": wrapper}},
		}},
	}

	// Payload type that is NOT defined anywhere in metadata — the
	// external/unresolved case that used to dangle.
	const payload = "github.com/ext/foo.Bar"

	usedTypes := map[string]*Schema{}
	base := &Schema{Ref: refComponentsSchemasPrefix + "envpkg_Envelope"}
	overrides := []wrapperFieldOverride{{StructFieldName: "Data", GoType: payload}}

	out := specialiseWrapperSchema(base, overrides, "envpkg.Envelope", usedTypes, meta, &APISpecConfig{})

	// The result must specialise `data` with a $ref to the payload.
	if len(out.AllOf) != 2 {
		t.Fatalf("expected allOf[base, override], got %+v", out)
	}
	data := out.AllOf[1].Properties["data"]
	if data == nil || data.Ref == "" {
		t.Fatalf("expected data $ref override, got %+v", out.AllOf[1])
	}
	wantRef := refComponentsSchemasPrefix + schemaComponentNameReplacer.Replace(payload)
	if data.Ref != wantRef {
		t.Errorf("data $ref = %q, want %q", data.Ref, wantRef)
	}

	// The payload type's name must be registered in usedTypes so
	// generateSchemas later emits a component for it (the regression):
	// without registration the $ref above would dangle.
	if _, ok := usedTypes[payload]; !ok {
		t.Errorf("payload type %q not registered in usedTypes; have %v",
			payload, mapKeys(usedTypes))
	}
}

// wrapperFieldIsGeneric must only return true for interface{} / any
// fields — concrete-typed fields are not eligible for per-route
// override (otherwise call-site literals leak as $refs).
func TestWrapperFieldIsGeneric(t *testing.T) {
	pool := metadata.NewStringPool()
	meta := &metadata.Metadata{StringPool: pool}

	wrapper := &metadata.Type{
		Fields: []metadata.Field{
			{Name: pool.Get("Message"), Type: pool.Get("string")},
			{Name: pool.Get("Data"), Type: pool.Get("interface{}")},
			{Name: pool.Get("Any"), Type: pool.Get("any")},
			{Name: pool.Get("Code"), Type: pool.Get("int")},
			{Name: pool.Get("Ptr"), Type: pool.Get("*interface{}")},
		},
	}

	cases := []struct {
		name string
		want bool
	}{
		{"Message", false},
		{"Data", true},
		{"Any", true},
		{"Code", false},
		{"Ptr", true},
		{"Missing", false},
	}
	for _, c := range cases {
		if got := wrapperFieldIsGeneric(meta, wrapper, c.name); got != c.want {
			t.Errorf("wrapperFieldIsGeneric(%q) = %v, want %v", c.name, got, c.want)
		}
	}
}
