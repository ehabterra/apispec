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
	"sort"
	"testing"
)

// catalog is the scheme definitions these tests resolve requirements against.
func catalog() map[string]SecurityScheme {
	return map[string]SecurityScheme{
		"bearerAuth":  {Type: "http", Scheme: "bearer", BearerFormat: "JWT"},
		"basicAuth":   {Type: "http", Scheme: "basic"},
		"headerKey":   {Type: "apiKey", In: "header", Name: "X-API-Key"},
		"queryKey":    {Type: "apiKey", In: "query", Name: "api_key"},
		"cookieKey":   {Type: "apiKey", In: "cookie", Name: "token"},
		"oauth":       {Type: "oauth2"},
		"oidc":        {Type: "openIdConnect"},
		"weirdCasing": {Type: "APIKEY", In: "HEADER", Name: "X-Odd"},
	}
}

func TestSchemeConsumedHeaders(t *testing.T) {
	cases := []struct {
		name string
		reqs []SecurityRequirement
		want []string
	}{
		{
			// The type implies the header; OpenAPI states it nowhere.
			name: "an http scheme consumes Authorization",
			reqs: []SecurityRequirement{{"bearerAuth": {}}},
			want: []string{"authorization"},
		},
		{
			name: "oauth2 and openIdConnect carry a bearer token",
			reqs: []SecurityRequirement{{"oauth": {}}, {"oidc": {}}},
			want: []string{"authorization"},
		},
		{
			name: "an apiKey in a header consumes that header",
			reqs: []SecurityRequirement{{"headerKey": {}}},
			want: []string{"x-api-key"},
		},
		{
			// The reason a name denylist would be wrong: these consume no
			// header at all, so a header the handler reads is its own.
			name: "an apiKey elsewhere consumes no header",
			reqs: []SecurityRequirement{{"queryKey": {}, "cookieKey": {}}},
			want: nil,
		},
		{
			name: "several schemes on one operation contribute all of theirs",
			reqs: []SecurityRequirement{{"basicAuth": {}, "headerKey": {}}},
			want: []string{"authorization", "x-api-key"},
		},
		{
			// Type and location are written by hand in a config, so they are
			// compared case-insensitively.
			name: "casing does not matter",
			reqs: []SecurityRequirement{{"weirdCasing": {}}},
			want: []string{"x-odd"},
		},
		{
			// Suppressing a parameter on a guess is how a real one disappears.
			name: "a scheme with no definition contributes nothing",
			reqs: []SecurityRequirement{{"notDefinedAnywhere": {}}},
			want: nil,
		},
		{
			// The requirement list is a set of ALTERNATIVES. A client may
			// authenticate with the query key alone and never send an
			// Authorization credential, so that header is not unambiguously the
			// scheme's and the parameter must stay (CodeRabbit, PR #441).
			name: "alternatives are intersected, not unioned",
			reqs: []SecurityRequirement{{"bearerAuth": {}}, {"queryKey": {}}},
			want: nil,
		},
		{
			// Both alternatives consume it, so it is certain either way.
			name: "a header every alternative consumes is consumed",
			reqs: []SecurityRequirement{{"bearerAuth": {}}, {"basicAuth": {}}},
			want: []string{"authorization"},
		},
		{
			// Within ONE object the schemes are required together, so their
			// headers accumulate; across objects only the common ones survive.
			name: "an AND inside an alternative accumulates, the OR intersects",
			reqs: []SecurityRequirement{
				{"bearerAuth": {}, "headerKey": {}},
				{"bearerAuth": {}},
			},
			want: []string{"authorization"},
		},
		{name: "no requirements", reqs: nil, want: nil},
	}
	for _, c := range cases {
		got := schemeConsumedHeaders(c.reqs, catalog())
		var names []string
		for h := range got {
			names = append(names, h)
		}
		sort.Strings(names)
		if len(names) != len(c.want) {
			t.Errorf("%s: consumed %v, want %v", c.name, names, c.want)
			continue
		}
		for i := range names {
			if names[i] != c.want[i] {
				t.Errorf("%s: consumed %v, want %v", c.name, names, c.want)
				break
			}
		}
	}
}

func TestDropSchemeConsumedParams(t *testing.T) {
	params := func(names ...string) []Parameter {
		out := make([]Parameter, 0, len(names)+1)
		out = append(out, Parameter{Name: "id", In: "path"})
		for _, n := range names {
			out = append(out, Parameter{Name: n, In: "header"})
		}
		return out
	}
	headerNames := func(r *RouteInfo) []string {
		var out []string
		for _, p := range r.Params {
			if p.In == "header" {
				out = append(out, p.Name)
			}
		}
		return out
	}

	t.Run("the governed header goes, the rest stay", func(t *testing.T) {
		route := &RouteInfo{
			Security: []SecurityRequirement{{"headerKey": {}}},
			Params:   params("X-API-Key", "X-Request-Id"),
		}
		dropSchemeConsumedParams([]*RouteInfo{route}, nil, catalog())
		if got := headerNames(route); len(got) != 1 || got[0] != "X-Request-Id" {
			t.Errorf("headers = %v, want [X-Request-Id]", got)
		}
		if len(route.Params) != 2 { // the path parameter is untouched
			t.Errorf("params = %+v, want the path parameter kept", route.Params)
		}
	})

	t.Run("header names are matched case-insensitively", func(t *testing.T) {
		// The middleware and the handler are written by different hands, and
		// HTTP header names do not distinguish case.
		route := &RouteInfo{
			Security: []SecurityRequirement{{"bearerAuth": {}}},
			Params:   params("authorization"),
		}
		dropSchemeConsumedParams([]*RouteInfo{route}, nil, catalog())
		if got := headerNames(route); len(got) != 0 {
			t.Errorf("headers = %v, want none", got)
		}
	})

	t.Run("an operation with no security keeps its headers", func(t *testing.T) {
		route := &RouteInfo{Params: params("Authorization")}
		dropSchemeConsumedParams([]*RouteInfo{route}, nil, catalog())
		if got := headerNames(route); len(got) != 1 {
			t.Errorf("headers = %v, want the handler's own header kept", got)
		}
	})

	t.Run("document-level security governs an inheriting operation", func(t *testing.T) {
		route := &RouteInfo{Params: params("Authorization")} // nil Security: inherits
		dropSchemeConsumedParams([]*RouteInfo{route}, []SecurityRequirement{{"bearerAuth": {}}}, catalog())
		if got := headerNames(route); len(got) != 0 {
			t.Errorf("headers = %v, want none", got)
		}
	})

	t.Run("an explicitly public operation keeps its headers", func(t *testing.T) {
		// A non-nil empty requirement list overrides the document's security,
		// so nothing is consumed on this operation's behalf.
		route := &RouteInfo{Security: []SecurityRequirement{}, Params: params("Authorization")}
		dropSchemeConsumedParams([]*RouteInfo{route}, []SecurityRequirement{{"bearerAuth": {}}}, catalog())
		if got := headerNames(route); len(got) != 1 {
			t.Errorf("headers = %v, want the header kept on a public operation", got)
		}
	})

	t.Run("a verb split does not drop its siblings' parameters", func(t *testing.T) {
		// A dispatch split copies the RouteInfo, so the two operations share one
		// parameter slice; filtering in place would edit both.
		shared := params("Authorization")
		get := &RouteInfo{Method: "GET", Params: shared}
		post := &RouteInfo{Method: "POST", Params: shared, Security: []SecurityRequirement{{"bearerAuth": {}}}}
		dropSchemeConsumedParams([]*RouteInfo{get, post}, nil, catalog())
		if got := headerNames(get); len(got) != 1 {
			t.Errorf("the unprotected sibling lost its header: %v", got)
		}
		if got := headerNames(post); len(got) != 0 {
			t.Errorf("the protected operation kept the credential: %v", got)
		}
	})

	t.Run("an alternative that consumes nothing keeps every header", func(t *testing.T) {
		route := &RouteInfo{
			Security: []SecurityRequirement{{"bearerAuth": {}}, {"queryKey": {}}},
			Params:   params("Authorization"),
		}
		dropSchemeConsumedParams([]*RouteInfo{route}, nil, catalog())
		if got := headerNames(route); len(got) != 1 {
			t.Errorf("headers = %v, want the header kept when one alternative does not consume it", got)
		}
	})

	t.Run("guards", func(t *testing.T) {
		route := &RouteInfo{Security: []SecurityRequirement{{"bearerAuth": {}}}, Params: params("Authorization")}
		dropSchemeConsumedParams([]*RouteInfo{route, nil}, nil, nil) // no catalog: nothing to resolve
		if got := headerNames(route); len(got) != 1 {
			t.Errorf("without a scheme catalog nothing may be dropped, got %v", got)
		}
		dropSchemeConsumedParams(nil, nil, catalog())
	})
}

// TestEffectiveSchemesPrecedence pins that the catalog the parameter filter
// reads is the one the document publishes.
//
// They are computed in two places — this filter runs before the paths are
// built, reconcileSecuritySchemes emits after — and if their precedence
// disagreed, a header parameter could be dropped because of a definition the
// document never contains (CodeRabbit, PR #441).
func TestEffectiveSchemesPrecedence(t *testing.T) {
	user := SecurityScheme{Type: "apiKey", In: "query", Name: "api_key"}
	inferred := SecurityScheme{Type: "apiKey", In: "header", Name: "X-API-Key"}
	preset := SecurityScheme{Type: "apiKey", In: "header", Name: "Authorization"}

	cfg := &APISpecConfig{
		SecuritySchemes: map[string]SecurityScheme{"shared": user},
		presetSchemes:   map[string]SecurityScheme{"shared": preset, "presetOnly": preset},
	}
	discovered := map[string]SecurityScheme{"shared": inferred, "discoveredOnly": inferred}

	filtering := effectiveSchemes(cfg, discovered)
	routes := []*RouteInfo{{Security: []SecurityRequirement{
		{"shared": {}}, {"presetOnly": {}}, {"discoveredOnly": {}},
	}}}
	emitted := reconcileSecuritySchemes(cfg, routes, discovered)

	for _, name := range []string{"shared", "presetOnly", "discoveredOnly"} {
		if filtering[name] != emitted[name] {
			t.Errorf("%s: the filter reads %+v while the document emits %+v",
				name, filtering[name], emitted[name])
		}
	}
	// And the precedence itself: the user's definition wins over both.
	if filtering["shared"] != user {
		t.Errorf("shared = %+v, want the user's own definition", filtering["shared"])
	}
}
