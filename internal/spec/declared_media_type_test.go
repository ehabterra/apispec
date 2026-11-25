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
	"regexp"
	"testing"
)

// TestPairingAdoptsDeclaredMediaType pins the adopt/no-adopt decision for a
// serializer's body beside a Content-Type declaration (issue #652). The body
// takes the declared media type only when the declaration is in effect where
// it is written — on its own chain — and the call leaves the header to the
// handler. Everywhere else the body keeps the media type it had.
func TestPairingAdoptsDeclaredMediaType(t *testing.T) {
	type frag struct {
		chain string
		resp  *ResponseInfo
	}
	run := func(frags ...frag) *RouteInfo {
		meta := exSweepMeta()
		stub := &stubResponseMatcher{byNode: map[TrackerNodeInterface][]*ResponseInfo{}}
		candidates := make([]responseCandidate, 0, len(frags))
		for i, f := range frags {
			pos := "h.go:" + string(rune('1'+i)) + ":1"
			node := &fakeNode{edge: sweepEdge(meta, "handler", "app", "write", "app", "", pos)}
			stub.byNode[node] = []*ResponseInfo{f.resp}
			candidates = append(candidates, responseCandidate{node: node, chain: f.chain})
		}
		e := &Extractor{responseMatchers: []ResponsePatternMatcher{stub}, cfg: DefaultHTTPConfig()}
		route := &RouteInfo{Metadata: meta, Response: map[string]*ResponseInfo{}}
		e.pairAndFillResponses(route, candidates)
		return route
	}
	decl := func(mediaType string, tentative bool) *ResponseInfo {
		return &ResponseInfo{
			StatusCode: 200, ContentType: mediaType, MediaTypeDeclared: true,
			HeaderDeclaration: true, DeclarationOnly: tentative,
			Schema: &Schema{Type: "string", Format: "binary"},
		}
	}
	status := func(code int) *ResponseInfo {
		return &ResponseInfo{StatusCode: code, ContentType: "application/json", StatusInFrame: true}
	}
	body := func(fromHeader bool) *ResponseInfo {
		return &ResponseInfo{
			StatusCode: -1, ContentType: "application/json", BodyType: "Problem",
			Schema: &Schema{Ref: "#/components/schemas/Problem"}, ImplicitStatus: 200,
			MediaTypeFromHeader: fromHeader,
		}
	}
	only := func(t *testing.T, route *RouteInfo, slot, mediaType string) {
		t.Helper()
		if got := statusKeysOf(route); len(got) != 1 || got[0] != slot {
			t.Fatalf("statuses = %v, want only %s", got, slot)
		}
		resp := route.Response[slot]
		if resp.ContentType != mediaType || resp.BodyType != "Problem" || len(resp.Alternates) != 0 {
			t.Errorf("%s = %s %s (alternates %d), want Problem under %s only",
				slot, resp.ContentType, resp.BodyType, len(resp.Alternates), mediaType)
		}
	}

	t.Run("a declaration ahead of the status and body is the body's media type", func(t *testing.T) {
		route := run(
			frag{"c", decl("application/problem+json", false)},
			frag{"c", status(400)},
			frag{"c", body(true)},
		)
		only(t, route, "400", "application/problem+json")
	})

	t.Run("with no status write the body keeps the implicit one", func(t *testing.T) {
		route := run(
			frag{"c", decl("application/vnd.item+json", false)},
			frag{"c", body(true)},
		)
		only(t, route, "200", "application/vnd.item+json")
	})

	t.Run("a declaration on another chain is not in effect", func(t *testing.T) {
		route := run(
			frag{"other", decl("application/problem+json", false)},
			frag{"c", status(400)},
			frag{"c", body(true)},
		)
		if resp := route.Response["400"]; resp == nil || resp.ContentType != "application/json" || resp.BodyType != "Problem" {
			t.Errorf("400 = %+v, want Problem under the undeclared default", resp)
		}
	})

	t.Run("a tentative declaration leaves the body describing itself", func(t *testing.T) {
		route := run(
			frag{"c", decl("application/xml", true)},
			frag{"c", status(400)},
			frag{"c", body(true)},
		)
		only(t, route, "400", "application/json")
	})

	t.Run("a call that sets the header itself is not adopted", func(t *testing.T) {
		route := run(
			frag{"c", decl("application/problem+json", false)},
			frag{"c", status(400)},
			frag{"c", body(false)},
		)
		// The declaration keeps its own slot beside the body, as before: the
		// server replaces it, so it is not the body's media type.
		resp := route.Response["400"]
		if resp == nil {
			t.Fatalf("400 missing, have %v", statusKeysOf(route))
		}
		if resp.ContentType != "application/json" && !hasAlternate(resp, "application/json") {
			t.Errorf("400 lost the body's own application/json: %s", resp.ContentType)
		}
	})
}

func hasAlternate(resp *ResponseInfo, mediaType string) bool {
	_, ok := resp.Alternates[mediaType]
	return ok
}

// TestTakesHeaderMediaType pins which calls leave their body's media type to
// the Content-Type header (issue #652): net/http's encoder and gin's and echo's
// JSON renderers do; fiber's renderers, chi's render.JSON and every call that
// names its media type do not.
func TestTakesHeaderMediaType(t *testing.T) {
	find := func(cfg *APISpecConfig, call, recv string) *ResponsePattern {
		t.Helper()
		for i := range cfg.Framework.ResponsePatterns {
			p := &cfg.Framework.ResponsePatterns[i]
			if p.CallRegex == "" || !regexp.MustCompile(p.CallRegex).MatchString(call) {
				continue
			}
			if p.RecvTypeRegex != "" && !regexp.MustCompile(p.RecvTypeRegex).MatchString(recv) {
				continue
			}
			return p
		}
		t.Fatalf("no pattern matches %s on %s", call, recv)
		return nil
	}
	cases := []struct {
		name string
		p    *ResponsePattern
		want bool
	}{
		{"net/http json encoder", find(DefaultHTTPConfig(), "Encode", "encoding/json.*Encoder"), true},
		{"gin c.JSON", find(DefaultGinConfig(), "JSON", "github.com/gin-gonic/gin.*Context"), true},
		{"echo c.JSON", find(DefaultEchoConfig(), "JSON", "github.com/labstack/echo/v4.Context"), true},
		{"fiber c.JSON", find(DefaultFiberConfig(), "JSON", "github.com/gofiber/fiber/v2.*Ctx"), false},
		{"chi render.JSON", find(DefaultChiConfig(), "JSON", "github.com/go-chi/render"), false},
		{"xml encoder", find(DefaultHTTPConfig(), "Encode", "encoding/xml.*Encoder"), false},
		{"gin c.XML", find(DefaultGinConfig(), "XML", "github.com/gin-gonic/gin.*Context"), false},
		{"derived responder declaring its type", &ResponsePattern{TypeFromArg: true, DeclaredContentType: "application/problem+json"}, false},
	}
	for _, tc := range cases {
		if got := takesHeaderMediaType(tc.p); got != tc.want {
			t.Errorf("%s: takesHeaderMediaType = %v, want %v (pattern %+v)", tc.name, got, tc.want, *tc.p)
		}
	}
}
