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
	"strings"
	"testing"
)

// TestDropForeignPathParams pins the rule at the layer it lives in: an
// operation may declare only the path parameters its own template binds
// (issue #514).
//
// Framework-agnostic by construction — it reads the resolved parameter list,
// not any framework's accessor — so one table covers every router.
func TestDropForeignPathParams(t *testing.T) {
	cases := []struct {
		name string
		path string
		in   []Parameter
		want []string
		why  string
	}{
		{
			name: "a path name belonging to a sibling route",
			path: "/codes/{codeId}/thing",
			in: []Parameter{
				{Name: "id", In: "path"},
				{Name: "codeId", In: "path"},
			},
			want: []string{"codeId"},
			why:  "the template has nowhere to bind id",
		},
		{
			name: "every name declared",
			path: "/groups/{id}/codes/{codeId}/thing",
			in: []Parameter{
				{Name: "id", In: "path"},
				{Name: "codeId", In: "path"},
			},
			want: []string{"id", "codeId"},
			why:  "both bind here",
		},
		{
			name: "query, header and cookie are never template-bound",
			path: "/reports",
			in: []Parameter{
				{Name: "orgId", In: "path"},
				{Name: "page", In: "query"},
				{Name: "X-Trace", In: "header"},
				{Name: "session", In: "cookie"},
			},
			want: []string{"page", "X-Trace", "session"},
			why:  "only the path parameter is bound to the template",
		},
		{
			name: "a $ref parameter is left alone",
			path: "/things/{id}",
			in: []Parameter{
				{Ref: "#/components/parameters/IdParam"},
				{Name: "id", In: "path"},
			},
			want: []string{"", "id"},
			why:  "a $ref carries no location here; the dynamic-placeholder path owns those",
		},
		{
			name: "a path with no placeholders",
			path: "/health",
			in: []Parameter{
				{Name: "id", In: "path"},
				{Name: "verbose", In: "query"},
			},
			want: []string{"verbose"},
			why:  "nothing can bind, so no path parameter belongs",
		},
		{
			name: "no parameters at all",
			path: "/things/{id}",
			in:   nil,
			want: nil,
			why:  "the empty case must not allocate or panic",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := dropForeignPathParams(tc.in, tc.path)
			names := make([]string, 0, len(got))
			for _, p := range got {
				names = append(names, p.Name)
			}
			if strings.Join(names, ",") != strings.Join(tc.want, ",") {
				t.Errorf("dropForeignPathParams(%s) = %v, want %v — %s", tc.path, names, tc.want, tc.why)
			}
		})
	}
}

// TestDropForeignPathParamsDoesNotAliasInput guards the filter against writing
// through the caller's backing array: routes share parameter slices, and
// filtering in place would corrupt the sibling route that legitimately declares
// the dropped name — which is the very route the fix relies on to document it.
func TestDropForeignPathParamsDoesNotAliasInput(t *testing.T) {
	shared := []Parameter{
		{Name: "id", In: "path"},
		{Name: "codeId", In: "path"},
	}
	short := dropForeignPathParams(shared, "/codes/{codeId}/thing")
	long := dropForeignPathParams(shared, "/groups/{id}/codes/{codeId}/thing")

	if len(short) != 1 || short[0].Name != "codeId" {
		t.Fatalf("short route = %v, want [codeId]", short)
	}
	if len(long) != 2 || long[0].Name != "id" || long[1].Name != "codeId" {
		t.Errorf("long route = %v, want [id codeId] — the first call wrote through the shared array", long)
	}
	if shared[0].Name != "id" || shared[1].Name != "codeId" {
		t.Errorf("input mutated: %v", shared)
	}
}

// TestHandlerPlaceholderUnion pins how a legitimate multi-route read is told
// apart from a typo: the union over the routes a handler actually serves.
func TestHandlerPlaceholderUnion(t *testing.T) {
	routes := []*RouteInfo{
		{Function: "thing", Method: "GET", Path: "/codes/{codeId}/thing"},
		{Function: "thing", Method: "GET", Path: "/groups/{id}/codes/{codeId}/thing"},
		{Function: "misspelled", Method: "GET", Path: "/teams/{teamId}"},
		// A mounted route: the prefix carries placeholders too.
		{Function: "nested", Method: "GET", MountPath: "/orgs/{orgId}", Path: "/members/{memberId}"},
		// No handler name — must not create an entry keyed by "".
		{Function: "", Method: "GET", Path: "/anon/{x}"},
	}

	union := handlerPlaceholderUnion(routes)

	for _, tc := range []struct {
		handler, name string
		want          bool
	}{
		{"thing", "codeId", true},
		{"thing", "id", true},      // declared by the sibling — a legitimate read
		{"thing", "teamId", false}, // another handler's name
		{"misspelled", "teamId", true},
		{"misspelled", "teamID", false}, // the typo
		{"nested", "orgId", true},       // from the mount prefix
		{"nested", "memberId", true},
	} {
		if got := union[tc.handler][tc.name]; got != tc.want {
			t.Errorf("union[%s][%s] = %v, want %v", tc.handler, tc.name, got, tc.want)
		}
	}
	if _, ok := union[""]; ok {
		t.Error(`union has an entry keyed by "" — a route with no handler must not contribute`)
	}
}

// TestUnboundPathParamsCoversEveryFramework pins that the diagnostic reports a
// read no template binds regardless of how the name was recovered, and reports
// it once.
//
// The recorded list comes from the map-key recovery, which only gorilla/mux
// needs. Every other framework resolves the name into a parameter instead, and
// before #514 that path was reported by nothing — so a misspelled
// `chi.URLParam(r, "teamID")` was emitted as a real path parameter in silence.
func TestUnboundPathParamsCoversEveryFramework(t *testing.T) {
	routes := []*RouteInfo{
		// Resolved as a parameter (chi/gin/echo shape).
		{Function: "misspelled", Method: "GET", Path: "/teams/{teamId}", Params: []Parameter{
			{Name: "teamID", In: "path"},
			{Name: "teamId", In: "path"},
		}},
		// A query parameter the template does not bind is not a mismatch.
		{Function: "withQuery", Method: "GET", Path: "/reports", Params: []Parameter{
			{Name: "page", In: "query"},
		}},
	}
	// The same name also recorded by the map-key recovery (mux shape), which
	// must not produce a second line.
	recorded := []PathParamMismatch{
		{Method: "GET", Path: "/teams/{teamId}", Handler: "misspelled", Key: "teamID"},
	}

	got := unboundPathParams(routes, recorded)
	if len(got) != 1 {
		t.Fatalf("unboundPathParams = %d entries, want 1 (deduped): %+v", len(got), got)
	}
	if got[0].Key != "teamID" || got[0].Handler != "misspelled" {
		t.Errorf("got %+v, want the teamID read on misspelled", got[0])
	}

	// Without the recorded entry, the resolved parameter alone must still be
	// reported — that is the half no framework but mux had.
	if got := unboundPathParams(routes, nil); len(got) != 1 || got[0].Key != "teamID" {
		t.Errorf("unboundPathParams(no recorded) = %+v, want the teamID read", got)
	}
}
