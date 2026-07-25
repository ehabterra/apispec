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

import "testing"

func TestMergeFrameworkConfigs(t *testing.T) {
	t.Run("secondary patterns append, identical surfaces dedupe to primary's", func(t *testing.T) {
		primary := DefaultChiConfig()
		routesBefore := len(primary.Framework.RoutePatterns)

		// The primary's variant of a shared pattern must win: give chi's Get
		// verb pattern an http-secondary doppelganger with different hints.
		sec := HTTPSecondaryConfig()
		sec.Framework.RoutePatterns = append(sec.Framework.RoutePatterns, RoutePattern{
			CallRegex:       `(?i)(GET|POST|PUT|DELETE|PATCH|OPTIONS|HEAD)$`,
			RecvTypeRegex:   "^github.com/go-chi/chi(/v\\d)?\\.\\*?(Router|Mux)$",
			PathArgIndex:    7, // conflicting hint that must NOT survive
			HandlerArgIndex: 8,
		})

		merged := MergeFrameworkConfigs(primary, sec)
		if merged != primary {
			t.Fatalf("merge must mutate and return the primary")
		}

		gotHTTP := 0
		for _, p := range merged.Framework.RoutePatterns {
			if p.RecvTypeRegex == "^net/http(\\.\\*ServeMux)?$" {
				gotHTTP++
			}
			if p.CallRegex == `(?i)(GET|POST|PUT|DELETE|PATCH|OPTIONS|HEAD)$` && p.PathArgIndex == 7 {
				t.Errorf("secondary's conflicting duplicate replaced the primary's pattern")
			}
		}
		if gotHTTP != 2 {
			t.Errorf("expected 2 net/http route patterns appended, got %d", gotHTTP)
		}
		if len(merged.Framework.RoutePatterns) != routesBefore+2 {
			t.Errorf("route patterns: got %d, want %d (dedupe must drop the doppelganger)",
				len(merged.Framework.RoutePatterns), routesBefore+2)
		}
	})

	t.Run("request context accumulates unique regexes", func(t *testing.T) {
		// Synthetic primary without the net/http regex (the shipped framework
		// configs all carry it already, which is itself part of the design).
		primary := &APISpecConfig{Framework: FrameworkConfig{
			RequestContext: RequestContextConfig{TypeRegexes: []string{`^myfw\.Ctx$`}},
		}}
		MergeFrameworkConfigs(primary, HTTPSecondaryConfig())
		after := primary.Framework.RequestContext.TypeRegexes
		if len(after) != 2 {
			t.Fatalf("expected exactly the net/http request regex appended, got %v", after)
		}
		// Merging twice must not duplicate.
		MergeFrameworkConfigs(primary, HTTPSecondaryConfig())
		if len(primary.Framework.RequestContext.TypeRegexes) != 2 {
			t.Errorf("second merge duplicated request-context regexes: %v",
				primary.Framework.RequestContext.TypeRegexes)
		}

		// Shipped configs already carry the regex — merge must add nothing.
		gin := DefaultGinConfig()
		before := len(gin.Framework.RequestContext.TypeRegexes)
		MergeFrameworkConfigs(gin, HTTPSecondaryConfig())
		if got := len(gin.Framework.RequestContext.TypeRegexes); got != before {
			t.Errorf("gin request-context regexes grew %d -> %d; net/http regex must dedupe", before, got)
		}
	})

	t.Run("chi already shares net/http response patterns — merge adds none", func(t *testing.T) {
		primary := DefaultChiConfig()
		before := len(primary.Framework.ResponsePatterns)
		MergeFrameworkConfigs(primary, HTTPSecondaryConfig())
		if got := len(primary.Framework.ResponsePatterns); got != before {
			t.Errorf("response patterns grew %d -> %d; netHTTPResponsePatterns must dedupe", before, got)
		}
	})

	t.Run("SecondaryView keeps only receiver-scoped patterns", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			cfg  *APISpecConfig
		}{
			{"gin", DefaultGinConfig()},
			{"chi", DefaultChiConfig()},
			{"echo", DefaultEchoConfig()},
			{"fiber", DefaultFiberConfig()},
			{"mux", DefaultMuxConfig()},
			{"http", DefaultHTTPConfig()},
		} {
			view := SecondaryView(tc.cfg)
			for _, p := range view.Framework.RoutePatterns {
				if p.RecvType == "" && p.RecvTypeRegex == "" {
					t.Errorf("%s: unscoped route pattern %q survived", tc.name, p.CallRegex)
				}
			}
			for _, p := range view.Framework.ResponsePatterns {
				if p.RecvType == "" && p.RecvTypeRegex == "" {
					t.Errorf("%s: unscoped response pattern %q survived", tc.name, p.CallRegex)
				}
			}
			// Every framework's verb route pattern is scoped, so a view must
			// never come back route-less — that would mean the secondary
			// framework's registrations silently stop being traced.
			if len(view.Framework.RoutePatterns) == 0 {
				t.Errorf("%s: SecondaryView dropped every route pattern", tc.name)
			}
		}
		if SecondaryView(nil) != nil {
			t.Errorf("SecondaryView(nil) must be nil")
		}

		// The known-dangerous unscoped patterns must be filtered: net/http's
		// JSON/status catch-all (misreads fiber's status-less c.JSON) and the
		// unscoped Handle route pattern.
		httpView := SecondaryView(DefaultHTTPConfig())
		for _, p := range httpView.Framework.ResponsePatterns {
			if p.CallRegex == `^(?i)(JSON|String|XML|YAML|ProtoBuf|Data|File|Redirect)$` {
				t.Errorf("http catch-all response pattern survived SecondaryView")
			}
		}
	})

	t.Run("net/http FormValue and Cookie reach a non-http primary", func(t *testing.T) {
		// The two patterns HTTPSecondaryConfig gained with this scoping. They are
		// only useful through the merge — the net/http surface is layered under
		// every other framework — so all three steps are checked: the secondary
		// config declares them scoped, SecondaryView keeps them (an unscoped
		// pattern would be filtered), and they land in a primary that has neither.
		const requestRecv = "net/http.*Request"
		want := map[string]string{"^FormValue$": "form", "^Cookie$": "cookie"}

		found := map[string]bool{}
		for _, p := range HTTPSecondaryConfig().Framework.ParamPatterns {
			if in, ok := want[p.CallRegex]; ok && p.ParamIn == in {
				found[p.CallRegex] = true
				if p.RecvType != requestRecv {
					t.Errorf("%s pattern recvType = %q, want %q — an unscoped pattern is dropped from a secondary view",
						p.CallRegex, p.RecvType, requestRecv)
				}
			}
		}
		for regex, in := range want {
			if !found[regex] {
				t.Errorf("HTTPSecondaryConfig is missing the %s -> in:%s pattern", regex, in)
			}
		}

		// Both must survive the view and land in a primary. The primary here is
		// empty on purpose: a framework preset that happens to declare its own
		// form or cookie read would mask the merged pattern, and which presets do
		// that changes as configs grow — so isolation comes from the primary
		// carrying nothing, not from picking today's framework that happens to.
		merged := MergeFrameworkConfigs(&APISpecConfig{}, SecondaryView(HTTPSecondaryConfig()))
		got := map[string]string{}
		for _, p := range merged.Framework.ParamPatterns {
			if p.ParamIn == "form" || p.ParamIn == "cookie" {
				got[p.CallRegex] = p.ParamIn
			}
		}
		for regex, in := range want {
			if got[regex] != in {
				t.Errorf("after merge into an empty primary: %s -> in:%q, want %q; have %v", regex, got[regex], in, got)
			}
		}

		// And through the composition the engine actually performs: a gin
		// primary with the net/http surface layered under it. gin declares no
		// cookie read of its own, so a cookie pattern here came from the merge.
		ginMerged := MergeFrameworkConfigs(DefaultGinConfig(), SecondaryView(HTTPSecondaryConfig()))
		var cookieFromMerge bool
		for _, p := range ginMerged.Framework.ParamPatterns {
			if p.CallRegex == "^Cookie$" && p.ParamIn == "cookie" && p.RecvType == requestRecv {
				cookieFromMerge = true
			}
		}
		if !cookieFromMerge {
			t.Error("gin primary + net/http secondary: the *http.Request Cookie pattern did not survive the merge")
		}
	})

	t.Run("nil secondaries are ignored", func(t *testing.T) {
		primary := DefaultMuxConfig()
		before := len(primary.Framework.RoutePatterns)
		MergeFrameworkConfigs(primary, nil)
		if len(primary.Framework.RoutePatterns) != before {
			t.Errorf("nil secondary changed the primary")
		}
	})

	t.Run("nil primary returns nil instead of panicking", func(t *testing.T) {
		if got := MergeFrameworkConfigs(nil, HTTPSecondaryConfig()); got != nil {
			t.Errorf("MergeFrameworkConfigs(nil, ...) = %v, want nil", got)
		}
	})
}
