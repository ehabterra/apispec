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

package engine

import (
	"bytes"
	"log"
	"strings"
	"testing"

	intspec "github.com/ehabterra/apispec/internal/spec"
)

// captureLog collects what fn writes to the standard logger.
func captureLog(t *testing.T, fn func()) string {
	t.Helper()
	var buf bytes.Buffer
	out, flags := log.Writer(), log.Flags()
	log.SetOutput(&buf)
	log.SetFlags(0)
	t.Cleanup(func() {
		log.SetOutput(out)
		log.SetFlags(flags)
	})
	fn()
	return buf.String()
}

// TestReportNoRoutesNamesTheRealCause pins what the empty-spec diagnostic says,
// which mattered because it used to argue against the actual cause (issue #524).
//
// It reported "with gin patterns in effect" from the DETECTED framework — a
// fact independent of whether those patterns survived into the config that ran.
// So at the exact moment a user's config had replaced them, it asserted they
// were in effect, and the follow-up line sent the reader looking for an
// unsupported router they did not have.
func TestReportNoRoutesNamesTheRealCause(t *testing.T) {
	cases := []struct {
		name      string
		discovery RouteDiscovery
		unres     []intspec.UnresolvedPathRoute
		want      []string
		absent    []string
	}{
		{
			name: "a config replaced the patterns",
			discovery: RouteDiscovery{
				CallEdges: 35, Packages: 1, Paths: 0,
				Frameworks: []string{"gin"}, RoutePatterns: 0, UserConfig: true,
			},
			want: []string{"supplied config carries no route patterns", "REPLACES", "gin", "--output-config"},
			// The generic advice must not follow: the cause is known.
			absent: []string{"router is unsupported"},
		},
		{
			name: "no patterns and no config to blame",
			discovery: RouteDiscovery{
				CallEdges: 35, Packages: 1, Paths: 0,
				Frameworks: []string{"gin"}, RoutePatterns: 0, UserConfig: false,
			},
			want:   []string{"no route patterns were configured at all", "gin"},
			absent: []string{"supplied config", "router is unsupported"},
		},
		{
			name: "patterns were in effect and matched nothing",
			discovery: RouteDiscovery{
				CallEdges: 35, Packages: 1, Paths: 0,
				Frameworks: []string{"gin"}, RoutePatterns: 7, UserConfig: false,
			},
			// Here the old advice is the right advice, and the count says the
			// patterns really were there.
			want:   []string{"no route registrations matched", "7 gin route pattern(s) in effect", "router is unsupported"},
			absent: []string{"supplied config"},
		},
		{
			name: "every registration builds its path at runtime",
			discovery: RouteDiscovery{
				CallEdges: 35, Packages: 1, Paths: 0,
				Frameworks: []string{"gin"}, RoutePatterns: 7,
			},
			unres:  []intspec.UnresolvedPathRoute{{}},
			want:   []string{"builds its path at runtime"},
			absent: []string{"router is unsupported", "supplied config"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := &Engine{routeDiscovery: tc.discovery, unresolvedPaths: tc.unres}
			got := captureLog(t, e.reportNoRoutes)
			for _, want := range tc.want {
				if !strings.Contains(got, want) {
					t.Errorf("report does not mention %q\ngot: %s", want, got)
				}
			}
			for _, absent := range tc.absent {
				if strings.Contains(got, absent) {
					t.Errorf("report mentions %q, which does not apply here\ngot: %s", absent, got)
				}
			}
		})
	}
}

// TestReportNoRoutesStaysQuiet: the report exists for "code was analysed and
// nothing matched". A project that documented paths, or one with no call graph
// to walk, has nothing to explain.
func TestReportNoRoutesStaysQuiet(t *testing.T) {
	for _, d := range []RouteDiscovery{
		{CallEdges: 35, Paths: 2, Frameworks: []string{"gin"}},
		{CallEdges: 0, Paths: 0, Frameworks: []string{"gin"}},
	} {
		e := &Engine{routeDiscovery: d}
		if got := captureLog(t, e.reportNoRoutes); got != "" {
			t.Errorf("reported %q for %+v", got, d)
		}
	}
}
