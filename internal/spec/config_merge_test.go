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

	t.Run("nil secondaries are ignored", func(t *testing.T) {
		primary := DefaultMuxConfig()
		before := len(primary.Framework.RoutePatterns)
		MergeFrameworkConfigs(primary, nil)
		if len(primary.Framework.RoutePatterns) != before {
			t.Errorf("nil secondary changed the primary")
		}
	})
}
