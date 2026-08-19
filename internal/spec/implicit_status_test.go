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
	"net/http"
	"sort"
	"testing"
)

// TestImplicitStatusIsDeclaredWhereItApplies pins which frameworks imply a
// status (issue #369). net/http states it outright — the first Write sends 200 —
// and fiber's c.JSON(v) sends the context's status, which is 200 unless
// c.Status() set one. gin and echo declare none because their renderers always
// carry the status (c.JSON(200, v)), so there is no absent case to fill.
func TestImplicitStatusIsDeclaredWhereItApplies(t *testing.T) {
	for _, tc := range []struct {
		name string
		cfg  *APISpecConfig
		want int
	}{
		{"net/http", DefaultHTTPConfig(), http.StatusOK},
		{"chi", DefaultChiConfig(), http.StatusOK},
		{"mux", DefaultMuxConfig(), http.StatusOK},
		{"fiber", DefaultFiberConfig(), http.StatusOK},
		{"gin", DefaultGinConfig(), 0},
		{"echo", DefaultEchoConfig(), 0},
	} {
		if got := tc.cfg.Framework.ResponseContext.ImplicitStatus; got != tc.want {
			t.Errorf("%s: ImplicitStatus = %d, want %d", tc.name, got, tc.want)
		}
	}
}

// TestImplicitStatusSurvivesBeingSecondary keeps issue #212's rule: which
// framework the detector happens to meet first must not change the spec. The
// implicit status is a property of HTTP, so a chi handler still writes 200 when
// gin is primary — but the writer TYPE regexes must not travel with it, since
// they gate which encodes count as responses and one framework's writer types
// would drop the other's.
func TestImplicitStatusSurvivesBeingSecondary(t *testing.T) {
	secondary := SecondaryView(DefaultChiConfig())
	if got := secondary.Framework.ResponseContext.ImplicitStatus; got != http.StatusOK {
		t.Errorf("SecondaryView dropped the implicit status: got %d, want %d", got, http.StatusOK)
	}
	if regexes := secondary.Framework.ResponseContext.WriterTypeRegexes; len(regexes) != 0 {
		t.Errorf("SecondaryView carried writer type regexes %v; they gate another framework's encodes", regexes)
	}

	// A primary without one adopts the secondary's...
	primary := MergeFrameworkConfigs(DefaultGinConfig(), secondary)
	if got := primary.Framework.ResponseContext.ImplicitStatus; got != http.StatusOK {
		t.Errorf("merged config: ImplicitStatus = %d, want %d", got, http.StatusOK)
	}

	// ...and one that declares its own keeps it.
	own := &APISpecConfig{Framework: FrameworkConfig{
		ResponseContext: ResponseContextConfig{ImplicitStatus: http.StatusCreated},
	}}
	merged := MergeFrameworkConfigs(own, secondary)
	if got := merged.Framework.ResponseContext.ImplicitStatus; got != http.StatusCreated {
		t.Errorf("a secondary overrode the primary's implicit status: got %d, want %d", got, http.StatusCreated)
	}
}

// TestHTTPSecondaryConfigCarriesTheImplicitStatus covers the other merge path:
// the stdlib surface is layered under every non-stdlib framework, so a plain
// ServeMux handler in a gin project implies 200 too.
func TestHTTPSecondaryConfigCarriesTheImplicitStatus(t *testing.T) {
	if got := HTTPSecondaryConfig().Framework.ResponseContext.ImplicitStatus; got != http.StatusOK {
		t.Errorf("HTTPSecondaryConfig: ImplicitStatus = %d, want %d", got, http.StatusOK)
	}
}

// stubResponseMatcher returns pre-programmed responses per node, so the pairing
// logic can be exercised without a tracker tree or real patterns.
type stubResponseMatcher struct {
	byNode map[TrackerNodeInterface][]*ResponseInfo
}

func (s *stubResponseMatcher) MatchNode(node TrackerNodeInterface) bool {
	_, ok := s.byNode[node]
	return ok
}
func (s *stubResponseMatcher) GetPattern() interface{} { return nil }
func (s *stubResponseMatcher) GetPriority() int        { return 0 }
func (s *stubResponseMatcher) ExtractResponse(node TrackerNodeInterface, _ *RouteInfo) []*ResponseInfo {
	return s.byNode[node]
}

// TestPairAndFillResponsesImplicitStatus covers the pairing states the implicit
// status introduces (issue #369), at the layer that decides them: the body is
// unclaimed, the body is claimed by a stated status, and the body follows a
// status write whose value could not be read.
func TestPairAndFillResponsesImplicitStatus(t *testing.T) {
	// One fragment per call site, positioned so source order is unambiguous —
	// pairing walks fragments in that order.
	build := func(resps ...[]*ResponseInfo) (*Extractor, []responseCandidate, *RouteInfo) {
		meta := exSweepMeta()
		stub := &stubResponseMatcher{byNode: map[TrackerNodeInterface][]*ResponseInfo{}}
		candidates := make([]responseCandidate, 0, len(resps))
		for i, r := range resps {
			pos := "h.go:" + string(rune('1'+i)) + ":1"
			node := &fakeNode{edge: sweepEdge(meta, "handler", "app", "write", "app", "", pos)}
			stub.byNode[node] = r
			candidates = append(candidates, responseCandidate{node: node, chain: "chain"})
		}
		e := &Extractor{responseMatchers: []ResponsePatternMatcher{stub}}
		return e, candidates, &RouteInfo{Metadata: meta, Response: map[string]*ResponseInfo{}}
	}
	body := func(implicit int) *ResponseInfo {
		return &ResponseInfo{StatusCode: -1, ContentType: "application/json", BodyType: "Item", ImplicitStatus: implicit}
	}

	t.Run("an unclaimed body takes the implicit status", func(t *testing.T) {
		e, candidates, route := build([]*ResponseInfo{body(200)})
		e.pairAndFillResponses(route, candidates)
		if resp, ok := route.Response["200"]; !ok || resp.BodyType != "Item" {
			t.Errorf("body should be documented as 200, got %v", statusKeysOf(route))
		}
	})

	t.Run("a stated status claims the body instead", func(t *testing.T) {
		e, candidates, route := build(
			[]*ResponseInfo{{StatusCode: 201, ContentType: "application/json"}},
			[]*ResponseInfo{body(200)},
		)
		e.pairAndFillResponses(route, candidates)
		if resp, ok := route.Response["201"]; !ok || resp.BodyType != "Item" {
			t.Errorf("the stated 201 should claim the body, got %v", statusKeysOf(route))
		}
		if _, ok := route.Response["200"]; ok {
			t.Errorf("the implicit status must not also be emitted, got %v", statusKeysOf(route))
		}
	})

	t.Run("a body after an unreadable status write stays undetermined", func(t *testing.T) {
		e, candidates, route := build(
			[]*ResponseInfo{{StatusCode: -1, ContentType: "application/json", StatusUnresolved: true}},
			[]*ResponseInfo{body(200)},
		)
		e.pairAndFillResponses(route, candidates)
		if _, ok := route.Response["200"]; ok {
			t.Errorf("the handler states a status we cannot read; 200 would be a guess, got %v", statusKeysOf(route))
		}
		if resp, ok := route.Response["-1"]; !ok || resp.BodyType != "Item" {
			t.Errorf("the body should keep an undetermined status, got %v", statusKeysOf(route))
		}
	})

	t.Run("a body with no implicit status stays undetermined", func(t *testing.T) {
		e, candidates, route := build([]*ResponseInfo{body(0)})
		e.pairAndFillResponses(route, candidates)
		if _, ok := route.Response["-1"]; !ok {
			t.Errorf("a framework declaring no implicit status leaves the body undetermined, got %v", statusKeysOf(route))
		}
	})
}

func statusKeysOf(route *RouteInfo) []string {
	keys := make([]string, 0, len(route.Response))
	for k := range route.Response {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
