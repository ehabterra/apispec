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

	"github.com/ehabterra/apispec/internal/metadata"
)

// boundFields is the whole of what a binder reads off a struct, so each of its
// naming rules is pinned on its own (issue #355).
func TestBoundFields(t *testing.T) {
	meta := embedMeta(t, "type Base struct {\n"+
		"	Page int `form:\"page\"`\n"+
		"}\n"+
		"type Tagged struct {\n"+
		"	Inner string `form:\"inner\"`\n"+
		"}\n"+
		"type Q struct {\n"+
		"	Base\n"+
		"	Tagged `form:\"tagged\"`\n"+
		"	Sort    string `form:\"sort,omitempty\"`\n"+
		"	Named   string `form:\",default=5\"`\n"+
		"	Plain   string\n"+
		"	Dash    string `form:\"-\"`\n"+
		"	Other   string `query:\"other\"`\n"+
		"	private string `form:\"private\"`\n"+
		"	Page    int    `form:\"page\"`\n"+
		"}\n")
	typ := findType(meta, "p", "Q")
	if typ == nil {
		t.Fatal("Q not found")
	}

	names := func(b StructBinding) []string {
		var out []string
		for _, f := range boundFields(meta, typ, b) {
			out = append(out, f.name)
		}
		return out
	}

	byName := names(StructBinding{Tag: "form", UntaggedByName: true})
	// The outer `page` shadows the one the untagged embed promotes, so it is
	// listed once, where the OUTER field is declared. A tagged embed is one
	// value, not a promotion, so neither `tagged` nor `inner` is a scalar
	// parameter. `Other` has no form tag and binds under its Go name.
	want := []string{"sort", "Named", "Plain", "Other", "page"}
	if !slices.Equal(byName, want) {
		t.Errorf("UntaggedByName: got %v, want %v", byName, want)
	}

	tagOnly := names(StructBinding{Tag: "form"})
	want = []string{"sort", "page"}
	if !slices.Equal(tagOnly, want) {
		t.Errorf("tag only: got %v, want %v", tagOnly, want)
	}
}

// A parameter documented from a declaration replaces one read through an
// accessor, in the accessor's position, whichever came first. Two accessors or
// two declarations keep the first, as before.
func TestDeduplicateParametersPrefersDeclared(t *testing.T) {
	accessor := Parameter{Name: "page", In: "query", Schema: &Schema{Type: "string"}}
	declared := Parameter{Name: "page", In: "query", Required: true, Schema: &Schema{Type: "integer"}, declared: true}
	other := Parameter{Name: "sort", In: "query", Schema: &Schema{Type: "string"}}

	for _, order := range [][]Parameter{
		{accessor, other, declared},
		{declared, other, accessor},
	} {
		got := deduplicateParameters(order)
		if len(got) != 2 || got[1].Name != "sort" {
			t.Fatalf("got %+v, want page then sort", got)
		}
		if got[0].Schema.Type != "integer" || !got[0].Required {
			t.Errorf("order %s,%s: page = %+v, want the declared integer",
				order[0].Schema.Type, order[2].Schema.Type, got[0])
		}
	}

	second := Parameter{Name: "page", In: "query", Schema: &Schema{Type: "number"}}
	if got := deduplicateParameters([]Parameter{accessor, second}); got[0].Schema.Type != "string" {
		t.Errorf("two accessors: kept %q, want the first", got[0].Schema.Type)
	}
}

// A binder's own tag options, read by gorilla/schema's grammar and converted
// by the field's Go kind, as its converters do (issue #602, CodeRabbit on #603).
func TestApplyTagOptions(t *testing.T) {
	meta := embedMeta(t, "type Level uint8\ntype Mode string\ntype Box struct{ N int }\n")
	b := StructBinding{Tag: "schema", OptionRequired: "required", OptionDefault: "default:"}
	cases := []struct {
		name, value, goType string
		required            bool
		def                 any
	}{
		{name: "no options", value: "page", goType: "int"},
		{name: "required", value: "page,required", goType: "int", required: true},
		{name: "default clears required", value: "page,default:20", goType: "int", def: int64(20)},
		// gorilla/schema rejects both on every decode, so no default is ever
		// applied and the field stays required.
		{name: "required and default", value: "page,required,default:20", goType: "int", required: true},
		// Options are compared as written: a spaced option is not one.
		{name: "spaced required", value: "id, required", goType: "string"},
		{name: "spaced default", value: "n, default:5", goType: "int"},
		{name: "bool on", value: "on,default:on", goType: "bool", def: true},
		{name: "bool", value: "on,default:false", goType: "bool", def: false},
		{name: "float", value: "ratio,default:0.5", goType: "float64", def: 0.5},
		{name: "string", value: "sort,default:name", goType: "string", def: "name"},
		{name: "cut at colon", value: "sort,default:a:b", goType: "string", def: "a"},
		{name: "unsigned", value: "n,default:7", goType: "uint8", def: uint64(7)},
		// Out of the field's range or sign: the binder stores nothing, though
		// the parameter is still optional.
		{name: "negative uint8", value: "n,default:-1", goType: "uint8"},
		{name: "int8 overflow", value: "n,default:200", goType: "int8"},
		{name: "unparseable", value: "page,default:many", goType: "int"},
		// JSON cannot spell these.
		{name: "NaN", value: "x,default:NaN", goType: "float64"},
		{name: "Inf", value: "x,default:+Inf", goType: "float64"},
		// A declared type converts through its underlying kind.
		{name: "declared uint8", value: "lvl,default:3", goType: "p.Level", def: uint64(3)},
		{name: "declared uint8 range", value: "lvl,default:300", goType: "p.Level"},
		{name: "declared string", value: "m,default:fast", goType: "p.Mode", def: "fast"},
		// No builtin kind, no default.
		{name: "struct", value: "b,default:1", goType: "p.Box"},
		{name: "omitempty only", value: "page,omitempty", goType: "int"},
	}
	for _, c := range cases {
		p := &Parameter{Schema: &Schema{Type: "x"}}
		applyTagOptions(p, c.value, b, defaultConverter(c.goType, meta))
		if p.Required != c.required {
			t.Errorf("%s (%q): required = %v, want %v", c.name, c.value, p.Required, c.required)
		}
		if p.Schema.Default != c.def {
			t.Errorf("%s (%q): default = %#v, want %#v", c.name, c.value, p.Schema.Default, c.def)
		}
	}

	// A slice's default lists elements separated by `|`, each converted by the
	// element's kind; one bad element means no default at all.
	for value, want := range map[string][]any{
		"ids,default:1|2": {int64(1), int64(2)},
		"ids,default:1|x": nil,
	} {
		p := &Parameter{Schema: &Schema{Type: "array"}}
		applyTagOptions(p, value, b, defaultConverter("[]int", meta))
		got, _ := p.Schema.Default.([]any)
		if len(got) != len(want) || (len(want) == 2 && (got[0] != want[0] || got[1] != want[1])) {
			t.Errorf("%q: slice default = %#v, want %#v", value, p.Schema.Default, want)
		}
	}
	if conv := defaultConverter("[]p.Box", meta); conv != nil {
		t.Error("a slice of structs has a default converter; the binder rejects it")
	}

	// A $ref schema is shared with every use of the type: never mutated.
	shared := &Parameter{Schema: &Schema{Ref: "#/components/schemas/Mode"}}
	applyTagOptions(shared, "mode,default:fast", b, defaultConverter("p.Mode", meta))
	if shared.Schema.Default != nil {
		t.Errorf("a $ref schema gained a default: %#v", shared.Schema.Default)
	}
}

// decodesFrom's own decisions, on hand-built call sites: the source must sit at
// a real argument, the pattern must compile, and the producer is read through
// parentheses. With no request types configured there is nothing to prove a
// source foreign against, so a matching producer is accepted.
func TestDecodesFrom(t *testing.T) {
	meta := embedMeta(t, "")
	cfg := &APISpecConfig{}
	p := NewParamPatternMatcher(ParamPattern{}, cfg, NewContextProvider(meta))

	call := func(name string) *metadata.CallArgument {
		fun := metadata.NewCallArgument(meta)
		fun.SetKind(metadata.KindIdent)
		fun.SetName(name)
		a := metadata.NewCallArgument(meta)
		a.SetKind(metadata.KindCall)
		a.Fun = fun
		return a
	}
	paren := func(x *metadata.CallArgument) *metadata.CallArgument {
		a := metadata.NewCallArgument(meta)
		a.SetKind(metadata.KindParen)
		a.X = x
		return a
	}
	lit := metadata.NewCallArgument(meta)
	lit.SetKind(metadata.KindLiteral)
	lit.SetValue(`"x"`)

	site := func(src *metadata.CallArgument) TrackerNodeInterface {
		return callSiteFrame{edge: &metadata.CallGraphEdge{Args: []*metadata.CallArgument{lit, src}}}
	}
	query := StructBinding{SourceRegex: "^Query$", SourceArgIndex: 1}

	cases := []struct {
		name    string
		node    TrackerNodeInterface
		binding StructBinding
		want    bool
	}{
		{"matching producer", site(call("Query")), query, true},
		{"through parentheses", site(paren(call("Query"))), query, true},
		{"other producer", site(call("Header")), query, false},
		{"no producer name", site(lit), query, false},
		{"source index past the arguments", site(call("Query")), StructBinding{SourceRegex: "^Query$", SourceArgIndex: 5}, false},
		{"negative source index", site(call("Query")), StructBinding{SourceRegex: "^Query$", SourceArgIndex: -1}, false},
		{"pattern that does not compile", site(call("Query")), StructBinding{SourceRegex: "(", SourceArgIndex: 1}, false},
		{"no call site", callSiteFrame{}, query, false},
	}
	for _, c := range cases {
		if got := p.decodesFrom(c.node, c.binding); got != c.want {
			t.Errorf("%s: decodesFrom = %v, want %v", c.name, got, c.want)
		}
	}

	// ExtractStructParams has nothing to bind without a route or a bound
	// argument.
	sp := NewParamPatternMatcher(ParamPattern{StructBindings: []StructBinding{{Tag: "form", In: "query"}}, TypeArgIndex: 3}, cfg, NewContextProvider(meta))
	if got := sp.ExtractStructParams(site(lit), nil); got != nil {
		t.Errorf("no route: got %v", got)
	}
	if got := sp.ExtractStructParams(site(lit), &RouteInfo{Metadata: meta}); got != nil {
		t.Errorf("no bound argument: got %v", got)
	}
}
