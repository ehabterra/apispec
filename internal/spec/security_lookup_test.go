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
	"log"
	"strings"
	"testing"

	"github.com/ehabterra/apispec/internal/metadata"
)

func TestParseAPIKeyLookup(t *testing.T) {
	cases := []struct {
		lookup, in, name string
		ok               bool
	}{
		{"query:api_key", "query", "api_key", true},
		{"cookie:token", "cookie", "token", true},
		{"header:X-API-Key", "header", "X-API-Key", true},
		{" Header : X-Key ", "header", "X-Key", true}, // tolerant of spacing and case
		// OpenAPI has no apiKey location for a form field, so it is not one:
		// documenting the library default instead is a report, not a guess.
		{"form:api_key", "", "", false},
		{"body:key", "", "", false},
		{"header", "", "", false}, // no source separator
		{"query:", "", "", false}, // no name
		{"", "", "", false},
	}
	for _, c := range cases {
		in, name, ok := parseAPIKeyLookup(c.lookup)
		if ok != c.ok || in != c.in || name != c.name {
			t.Errorf("parseAPIKeyLookup(%q) = (%q, %q, %v), want (%q, %q, %v)",
				c.lookup, in, name, ok, c.in, c.name, c.ok)
		}
	}
}

func TestLookupValueFromArgs(t *testing.T) {
	meta := newTestMeta()

	// keyValue builds `Field: <value>` inside a composite literal.
	keyValue := func(field string, value *metadata.CallArgument) *metadata.CallArgument {
		kv := metadata.NewCallArgument(meta)
		kv.SetKind(metadata.KindKeyValue)
		kv.X = concatIdent(meta, field)
		kv.Fun = value
		return kv
	}
	config := func(elts ...*metadata.CallArgument) *metadata.CallArgument {
		lit := metadata.NewCallArgument(meta)
		lit.SetKind(metadata.KindCompositeLit)
		lit.Args = elts
		return lit
	}
	call := func(name string) *metadata.CallArgument {
		c := metadata.NewCallArgument(meta)
		c.SetKind(metadata.KindCall)
		c.SetName(name)
		return c
	}

	t.Run("a literal field is read", func(t *testing.T) {
		args := []*metadata.CallArgument{config(keyValue("KeyLookup", concatLit(meta, "query:api_key")))}
		if v, set := lookupValueFromArgs(args, 0, "KeyLookup"); !set || v != "query:api_key" {
			t.Errorf("got (%q, %v), want (\"query:api_key\", true)", v, set)
		}
	})

	t.Run("an absent field is not configured", func(t *testing.T) {
		// The library default applies and is correct: nothing to report.
		args := []*metadata.CallArgument{config(keyValue("Validator", concatIdent(meta, "validate")))}
		if v, set := lookupValueFromArgs(args, 0, "KeyLookup"); set {
			t.Errorf("got (%q, %v), want not configured", v, set)
		}
	})

	t.Run("a runtime value is configured but unreadable", func(t *testing.T) {
		// Distinct from absent: the default is a fallback that may be wrong, and
		// the caller reports it.
		args := []*metadata.CallArgument{config(keyValue("KeyLookup", call("lookupFromEnv")))}
		v, set := lookupValueFromArgs(args, 0, "KeyLookup")
		if !set || v != "" {
			t.Errorf("got (%q, %v), want (\"\", true)", v, set)
		}
	})

	t.Run("elements that are not keyed fields are skipped", func(t *testing.T) {
		// A positional literal (`Config{"query:k", nil}`) names no field, and a
		// nil element is defensive: neither says anything about KeyLookup, so
		// the scan continues past them rather than reading one as the value.
		args := []*metadata.CallArgument{config(
			nil,
			concatLit(meta, "query:positional"),
			keyValue("KeyLookup", concatLit(meta, "query:keyed")),
		)}
		if v, set := lookupValueFromArgs(args, 0, "KeyLookup"); !set || v != "query:keyed" {
			t.Errorf("got (%q, %v), want the keyed field's value", v, set)
		}
	})

	t.Run("guards", func(t *testing.T) {
		args := []*metadata.CallArgument{config(keyValue("KeyLookup", concatLit(meta, "query:k")))}
		if _, set := lookupValueFromArgs(args, 0, ""); set {
			t.Error("no field name means the mapping declares no lookup")
		}
		if _, set := lookupValueFromArgs(args, 1, "KeyLookup"); set {
			t.Error("an index past the arguments must not resolve")
		}
		if _, set := lookupValueFromArgs(args, -1, "KeyLookup"); set {
			t.Error("a negative index must not resolve")
		}
		if _, set := lookupValueFromArgs([]*metadata.CallArgument{concatIdent(meta, "cfg")}, 0, "KeyLookup"); set {
			t.Error("a config passed as a variable is not a literal to read")
		}
	})
}

func TestAPIKeySchemeKey(t *testing.T) {
	cases := []struct{ in, name, want string }{
		{"query", "api_key", "apiKeyAuthQueryApiKey"},
		{"header", "X-API-Key", "apiKeyAuthHeaderXApiKey"},
		{"cookie", "token", "apiKeyAuthCookieToken"},
	}
	for _, c := range cases {
		if got := apiKeySchemeKey("apiKeyAuth", c.in, c.name); got != c.want {
			t.Errorf("apiKeySchemeKey(apiKeyAuth, %q, %q) = %q, want %q", c.in, c.name, got, c.want)
		}
	}
}

func TestSpecializeAPIKeySchemes(t *testing.T) {
	base := map[string]SecurityScheme{"apiKeyAuth": {Type: "apiKey", In: "header", Name: "Authorization"}}
	shaped := func(path string, shape apiKeyShape) *RouteInfo {
		return &RouteInfo{
			Path:                 path,
			Security:             []SecurityRequirement{{"apiKeyAuth": {}}},
			SecuritySchemeShapes: map[string]apiKeyShape{"apiKeyAuth": shape},
		}
	}
	atDefault := func(path string) *RouteInfo {
		return &RouteInfo{Path: path, Security: []SecurityRequirement{{"apiKeyAuth": {}}}}
	}
	schemeNameOf := func(r *RouteInfo) string {
		for _, req := range r.Security {
			for name := range req {
				return name
			}
		}
		return ""
	}

	t.Run("one shape keeps the declared name", func(t *testing.T) {
		route := shaped("/a", apiKeyShape{In: "query", Name: "api_key"})
		defs := specializeAPIKeySchemes([]*RouteInfo{route}, base, nil)
		if got := schemeNameOf(route); got != "apiKeyAuth" {
			t.Errorf("route requires %q, want the declared name", got)
		}
		if def := defs["apiKeyAuth"]; def.In != "query" || def.Name != "api_key" || def.Type != "apiKey" {
			t.Errorf("definition = %+v, want the configured query lookup", def)
		}
	})

	t.Run("several shapes split, none keeping the plain name", func(t *testing.T) {
		q := shaped("/a", apiKeyShape{In: "query", Name: "api_key"})
		c := shaped("/b", apiKeyShape{In: "cookie", Name: "token"})
		defs := specializeAPIKeySchemes([]*RouteInfo{q, c}, base, nil)
		if got := schemeNameOf(q); got != "apiKeyAuthQueryApiKey" {
			t.Errorf("query route requires %q", got)
		}
		if got := schemeNameOf(c); got != "apiKeyAuthCookieToken" {
			t.Errorf("cookie route requires %q", got)
		}
		if _, ok := defs["apiKeyAuth"]; ok {
			t.Error("no shape may keep the plain name when several exist")
		}
		if defs["apiKeyAuthQueryApiKey"].In != "query" || defs["apiKeyAuthCookieToken"].In != "cookie" {
			t.Errorf("definitions = %+v", defs)
		}
	})

	t.Run("a route at the library default is one of the shapes", func(t *testing.T) {
		// Measured while building this: without counting the unshaped
		// reference, the configured shape redefined the shared name and the
		// default route's credential was documented in the wrong place.
		q := shaped("/a", apiKeyShape{In: "query", Name: "api_key"})
		d := atDefault("/b")
		defs := specializeAPIKeySchemes([]*RouteInfo{q, d}, base, nil)
		if got := schemeNameOf(q); got != "apiKeyAuthQueryApiKey" {
			t.Errorf("configured route requires %q, want its own scheme", got)
		}
		if got := schemeNameOf(d); got != "apiKeyAuth" {
			t.Errorf("default route requires %q, want the declared name", got)
		}
		if def, ok := defs["apiKeyAuth"]; ok && (def.In != "header" || def.Name != "Authorization") {
			t.Errorf("the declared name must keep the library default, got %+v", def)
		}
	})

	t.Run("a scheme the user defined is left alone", func(t *testing.T) {
		// An explicit definition is a statement of intent: inference may not
		// rewrite it, and — since the requirement keeps the declared name — must
		// not route around it with a derived one either (CodeRabbit, PR #439).
		explicit := map[string]SecurityScheme{
			"apiKeyAuth": {Type: "apiKey", In: "header", Name: "X-User-Chosen"},
		}
		q := shaped("/a", apiKeyShape{In: "query", Name: "api_key"})
		c := shaped("/b", apiKeyShape{In: "cookie", Name: "token"})
		defs := specializeAPIKeySchemes([]*RouteInfo{q, c}, explicit, explicit)
		if len(defs) != 0 {
			t.Errorf("want no discovered definitions for a user-defined scheme, got %+v", defs)
		}
		for _, r := range []*RouteInfo{q, c} {
			if got := schemeNameOf(r); got != "apiKeyAuth" {
				t.Errorf("route %s requires %q, want the user's own scheme name", r.Path, got)
			}
		}
	})

	t.Run("shapes differing only in punctuation get distinct keys", func(t *testing.T) {
		// `api_key` and `api-key` render to one CamelCase key; without a
		// collision suffix one definition overwrote the other and both routes
		// pointed at it (CodeRabbit, PR #439).
		a := shaped("/a", apiKeyShape{In: "query", Name: "api_key"})
		b := shaped("/b", apiKeyShape{In: "query", Name: "api-key"})
		defs := specializeAPIKeySchemes([]*RouteInfo{a, b}, base, nil)
		if len(defs) != 2 {
			t.Fatalf("want a definition per shape, got %+v", defs)
		}
		if schemeNameOf(a) == schemeNameOf(b) {
			t.Fatalf("both routes reference %q", schemeNameOf(a))
		}
		for _, r := range []*RouteInfo{a, b} {
			want := r.SecuritySchemeShapes["apiKeyAuth"]
			got := defs[schemeNameOf(r)]
			if got.In != want.In || got.Name != want.Name {
				t.Errorf("route %s -> %s = {%s %s}, want {%s %s}",
					r.Path, schemeNameOf(r), got.In, got.Name, want.In, want.Name)
			}
		}
	})

	t.Run("key assignment is stable across runs", func(t *testing.T) {
		// The suffix makes assignment order-dependent, so it is taken in sorted
		// shape order rather than map order (golden rule #1).
		names := func() string {
			a := shaped("/a", apiKeyShape{In: "query", Name: "api_key"})
			b := shaped("/b", apiKeyShape{In: "query", Name: "api-key"})
			specializeAPIKeySchemes([]*RouteInfo{a, b}, base, nil)
			return schemeNameOf(a) + "|" + schemeNameOf(b)
		}
		first := names()
		for i := 0; i < 20; i++ {
			if got := names(); got != first {
				t.Fatalf("run %d assigned %q, first run assigned %q", i, got, first)
			}
		}
	})

	t.Run("a requirement naming another scheme is left alone", func(t *testing.T) {
		// A route can require several schemes; only the one a shape was resolved
		// for is rewritten.
		route := &RouteInfo{
			Path: "/a",
			Security: []SecurityRequirement{{
				"apiKeyAuth": {},
				"bearerAuth": {},
			}},
			SecuritySchemeShapes: map[string]apiKeyShape{"apiKeyAuth": {In: "query", Name: "api_key"}},
		}
		other := shaped("/b", apiKeyShape{In: "cookie", Name: "token"})
		specializeAPIKeySchemes([]*RouteInfo{route, other}, base, nil)
		names := route.Security[0]
		if _, ok := names["bearerAuth"]; !ok {
			t.Errorf("bearerAuth must survive untouched, got %v", names)
		}
		if _, ok := names["apiKeyAuthQueryApiKey"]; !ok {
			t.Errorf("the shaped scheme must be renamed, got %v", names)
		}
	})

	t.Run("nothing shaped changes nothing", func(t *testing.T) {
		d := atDefault("/b")
		if defs := specializeAPIKeySchemes([]*RouteInfo{d, nil}, base, nil); defs != nil {
			t.Errorf("want no definitions, got %+v", defs)
		}
		if got := schemeNameOf(d); got != "apiKeyAuth" {
			t.Errorf("route requires %q, want it untouched", got)
		}
	})
}

// TestValidateSecurityLookupFields covers the config checks for a lookup: a
// mapping that shapes a scheme has to declare one, and an argument index means
// nothing without the field naming what to read there.
func TestValidateSecurityLookupFields(t *testing.T) {
	mapping := func(m SecurityMapping) *APISpecConfig {
		m.FunctionNameRegex = "^New$"
		return &APISpecConfig{SecurityMappings: []SecurityMapping{m}}
	}
	cases := []struct {
		name    string
		cfg     *APISpecConfig
		wantErr bool
	}{
		{
			name: "a lookup with a scheme to shape",
			cfg: mapping(SecurityMapping{
				Schemes:     []SecurityRequirement{{"apiKeyAuth": {}}},
				LookupField: "KeyLookup",
			}),
		},
		{
			name: "a lookup on a public mapping shapes nothing",
			cfg: mapping(SecurityMapping{
				Public: true, LookupField: "KeyLookup",
			}),
			wantErr: true,
		},
		{
			name: "a negative argument index",
			cfg: mapping(SecurityMapping{
				Schemes:        []SecurityRequirement{{"apiKeyAuth": {}}},
				LookupField:    "KeyLookup",
				LookupArgIndex: -1,
			}),
			wantErr: true,
		},
		{
			name: "an index without a field",
			cfg: mapping(SecurityMapping{
				Schemes:        []SecurityRequirement{{"apiKeyAuth": {}}},
				LookupArgIndex: 1,
			}),
			wantErr: true,
		},
	}
	for _, c := range cases {
		err := c.cfg.ValidateSecurity()
		if (err != nil) != c.wantErr {
			t.Errorf("%s: ValidateSecurity() error = %v, wantErr %v", c.name, err, c.wantErr)
		}
	}
}

// TestAPIKeyShapeOf covers the decision a matched mapping makes about the
// middleware's configuration: read it, or keep the library default and say so.
//
// The two "say so" branches — a lookup built at runtime, and a source OpenAPI
// has no apiKey location for — are the honest fallback this change rests on,
// and no fixture exercises them (both need code a real project would be odd to
// write).
func TestAPIKeyShapeOf(t *testing.T) {
	meta := newTestMeta()

	keyValue := func(field string, value *metadata.CallArgument) *metadata.CallArgument {
		kv := metadata.NewCallArgument(meta)
		kv.SetKind(metadata.KindKeyValue)
		kv.X = concatIdent(meta, field)
		kv.Fun = value
		return kv
	}
	// refWith returns a ref whose constructor was called with this config.
	refWith := func(elts ...*metadata.CallArgument) MiddlewareRef {
		lit := metadata.NewCallArgument(meta)
		lit.SetKind(metadata.KindCompositeLit)
		lit.Args = elts
		call := metadata.NewCallArgument(meta)
		call.SetKind(metadata.KindCall)
		call.Args = []*metadata.CallArgument{lit}
		return MiddlewareRef{FunctionName: "KeyAuthWithConfig", Pkg: "echo/middleware", ConfigCall: call}
	}
	runtimeValue := func() *metadata.CallArgument {
		c := metadata.NewCallArgument(meta)
		c.SetKind(metadata.KindCall)
		c.SetName("lookupFromEnv")
		return c
	}

	withLookup := SecurityMapping{
		FunctionNameRegex: "^KeyAuthWithConfig$",
		Schemes:           []SecurityRequirement{{"apiKeyAuth": {}}},
		LookupField:       "KeyLookup",
	}
	noLookup := SecurityMapping{
		FunctionNameRegex: "^KeyAuthWithConfig$",
		Schemes:           []SecurityRequirement{{"apiKeyAuth": {}}},
	}

	cases := []struct {
		name    string
		mapping SecurityMapping
		ref     MiddlewareRef
		want    apiKeyShape
		ok      bool
	}{
		{
			name:    "a configured lookup is read",
			mapping: withLookup,
			ref:     refWith(keyValue("KeyLookup", concatLit(meta, "query:api_key"))),
			want:    apiKeyShape{In: "query", Name: "api_key"},
			ok:      true,
		},
		{
			// The library default is the right answer, so there is nothing to
			// report either.
			name:    "an unconfigured middleware keeps the default",
			mapping: withLookup,
			ref:     refWith(keyValue("Validator", concatIdent(meta, "validate"))),
		},
		{
			name:    "a mapping that declares no lookup reads nothing",
			mapping: noLookup,
			ref:     refWith(keyValue("KeyLookup", concatLit(meta, "query:api_key"))),
		},
		{
			// Configured and unknowable: the default is a fallback that may be
			// wrong, which is what the warning is for.
			name:    "a runtime lookup keeps the default",
			mapping: withLookup,
			ref:     refWith(keyValue("KeyLookup", runtimeValue())),
		},
		{
			// OpenAPI has no apiKey location for a form field, so inventing
			// `query` would be the confident wrong answer this change removes.
			name:    "a form source keeps the default",
			mapping: withLookup,
			ref:     refWith(keyValue("KeyLookup", concatLit(meta, "form:api_key"))),
		},
		{
			name:    "middleware that is not a constructor call has no config",
			mapping: withLookup,
			ref:     MiddlewareRef{FunctionName: "authMiddleware", Pkg: "app"},
		},
	}
	for _, c := range cases {
		got, ok := apiKeyShapeOf(c.mapping, c.ref)
		if ok != c.ok || got != c.want {
			t.Errorf("%s: apiKeyShapeOf() = (%+v, %v), want (%+v, %v)", c.name, got, ok, c.want, c.ok)
		}
	}
}

// A user definition that AGREES with the middleware's configuration is not a
// disagreement, so it is not reported: the warning exists to surface a genuine
// contradiction, not to comment on every configured scheme.
func TestReportOverriddenShapesAgreement(t *testing.T) {
	var logged strings.Builder
	restore := log.Writer()
	log.SetOutput(&logged)
	log.SetFlags(0)
	defer func() { log.SetOutput(restore); log.SetFlags(log.LstdFlags) }()

	shape := apiKeyShape{In: "query", Name: "api_key"}
	route := &RouteInfo{
		Security:             []SecurityRequirement{{"apiKeyAuth": {}}},
		SecuritySchemeShapes: map[string]apiKeyShape{"apiKeyAuth": shape},
	}

	agrees := map[string]SecurityScheme{"apiKeyAuth": {Type: "apiKey", In: "query", Name: "api_key"}}
	reportOverriddenShapes([]*RouteInfo{route, nil}, agrees)
	if logged.Len() != 0 {
		t.Errorf("an agreeing definition must not be reported, logged: %q", logged.String())
	}

	differs := map[string]SecurityScheme{"apiKeyAuth": {Type: "apiKey", In: "header", Name: "X-Chosen"}}
	reportOverriddenShapes([]*RouteInfo{route}, differs)
	if !strings.Contains(logged.String(), "keeping your definition") {
		t.Errorf("a contradicted definition must be reported once, logged: %q", logged.String())
	}

	// Nothing user-defined: nothing to compare against.
	logged.Reset()
	reportOverriddenShapes([]*RouteInfo{route}, nil)
	if logged.Len() != 0 {
		t.Errorf("no user definitions means nothing to report, logged: %q", logged.String())
	}
}
