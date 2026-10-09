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

package generator

import (
	"reflect"
	"sort"
	"strings"
	"testing"

	intspec "github.com/ehabterra/apispec/internal/spec"
)

// TestTestdata_StatusLocalVar locks in issue #579: a status held in a local
// variable resolves to what reaches the call. A literal or constant argument
// always did; `code := http.StatusConflict; c.JSON(code, v)` documented
// `default`, on every framework. Each row is the full status set, so a status
// that leaks across branches or survives being overwritten fails too.
func TestTestdata_StatusLocalVar(t *testing.T) {
	type row struct {
		path     string
		statuses []string
		// bodyRef, when set, is the component each status's body must name, in
		// the same order as statuses.
		bodyRef []string
	}
	frameworks := []struct {
		name, fixture string
		cfg           *intspec.APISpecConfig
		rows          []row
	}{
		{"echo", "status_local_var_echo", intspec.DefaultEchoConfig(), []row{
			{path: "/short", statuses: []string{"409"}},
			{path: "/string", statuses: []string{"409"}},
			{path: "/var-decl", statuses: []string{"202"}},
			{path: "/literal", statuses: []string{"418"}},
			{path: "/closure", statuses: []string{"409"}},
			// Overwritten on the one path to the call: only the later value.
			{path: "/reassigned", statuses: []string{"201"}},
			// A branch that falls through: one response per value (#39).
			{path: "/branched", statuses: []string{"200", "201"}},
			// The 400 arm returns: neither value reaches the other's call.
			{path: "/early-return", statuses: []string{"200", "400"}, bodyRef: []string{"_Item", "_Problem"}},
			// Computed: not knowable, so undetermined (golden rule #7).
			{path: "/computed", statuses: []string{"default"}},
			// A call that only mentions a status constant: not its value.
			{path: "/optional-arg", statuses: []string{"default"}},
		}},
		{"gin", "status_local_var_gin", intspec.DefaultGinConfig(), []row{
			{path: "/short", statuses: []string{"409"}},
			{path: "/abort", statuses: []string{"403"}},
			// A status setter reads its variable too. The body after it is
			// gin's c.Writer.Write, not a body pattern yet (#576).
			{path: "/status", statuses: []string{"202"}},
		}},
		{"net/http", "status_local_var_nethttp", intspec.DefaultHTTPConfig(), []row{
			{path: "/write-header", statuses: []string{"409"}},
			{path: "/http-error", statuses: []string{"418"}},
		}},
	}
	for _, fw := range frameworks {
		t.Run(fw.name, func(t *testing.T) {
			out := loadTestdata(t, fw.fixture, fw.cfg)
			noDanglingRefs(t, out)
			for _, r := range fw.rows {
				op := opFor(out.Paths[r.path], "GET")
				if op == nil {
					t.Errorf("GET %s missing; have %v", r.path, mapPathKeys(out.Paths))
					continue
				}
				got := statusKeys(op)
				sort.Strings(got)
				if !reflect.DeepEqual(got, r.statuses) {
					t.Errorf("GET %s statuses = %v, want %v", r.path, got, r.statuses)
					continue
				}
				for i, ref := range r.bodyRef {
					if s := responseSchemaRef(op, r.statuses[i]); !strings.Contains(s, ref) {
						t.Errorf("GET %s %s body = %q, want %s", r.path, r.statuses[i], s, ref)
					}
				}
			}
		})
	}
}
