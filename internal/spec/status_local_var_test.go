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
	"reflect"
	"testing"

	"github.com/ehabterra/apispec/internal/metadata"
)

// TestStatusFromLocalVarSentinel pins the review of #586: a value outside the
// status range is not a status, neither a resolved one nor "unset".
//
// `code := 0; if … { code = 201 }` used to resolve to 201: MapStatusCode parses
// any integer, and the single-value path read 0 as "no status yet". The spec
// output cannot show the difference — the mapper folds the zero arm's residue
// `default` into the 201 that has the same body — so it is pinned here, where
// the variable is read.
func TestStatusFromLocalVarSentinel(t *testing.T) {
	setup := func(values ...string) (*ResponsePatternMatcherImpl, *TrackerNode) {
		meta := exSweepMeta()
		assigns := make([]metadata.Assignment, 0, len(values))
		for _, v := range values {
			assigns = append(assigns, metadata.Assignment{Value: *sweepLit(meta, v)})
		}
		meta.Packages = map[string]*metadata.Package{
			"app": {Files: map[string]*metadata.File{
				"h.go": {Functions: map[string]*metadata.Function{
					"h": {AssignmentMap: map[string][]metadata.Assignment{"code": assigns}},
				}},
			}},
		}
		m := NewResponsePatternMatcher(ResponsePattern{}, &APISpecConfig{}, NewContextProvider(meta))
		return m, sweepNode(sweepEdge(meta, "h", "app", "JSON", "echo", "", ""))
	}

	cases := []struct {
		name    string
		values  []string
		single  int // 0: must not resolve to one status
		fanOut  []int
		residue bool
	}{
		{name: "one status", values: []string{"409"}, single: 409},
		{name: "a sentinel alone", values: []string{"0"}},
		{name: "a sentinel beside a status", values: []string{"0", "201"}, fanOut: []int{201}, residue: true},
		{name: "two statuses", values: []string{"200", "201"}, fanOut: []int{200, 201}},
		{name: "out of range", values: []string{"7", "201"}, fanOut: []int{201}, residue: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m, node := setup(tc.values...)
			arg := sweepIdent(node.GetEdge().Caller.Meta, "code")
			got, ok := m.statusFromLocalVar(arg, node)
			if tc.single != 0 {
				if !ok || got != tc.single {
					t.Errorf("statusFromLocalVar = %d, %v; want %d", got, ok, tc.single)
				}
			} else if ok {
				t.Errorf("statusFromLocalVar = %d; want unresolved", got)
			}
			codes, residue := m.expandStatusesFromIdent(arg, node.GetEdge())
			if !reflect.DeepEqual(codes, tc.fanOut) || residue != tc.residue {
				t.Errorf("fan-out = %v, residue %v; want %v, %v", codes, residue, tc.fanOut, tc.residue)
			}
		})
	}
}
