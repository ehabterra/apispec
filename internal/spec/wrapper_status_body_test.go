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

// One parameter cannot be both the status and the body of a response, and a
// derived pattern that says so has followed a local back to the wrong source
// (issue #485).
func TestSameAsStatusParam(t *testing.T) {
	cases := []struct {
		name    string
		pattern *ResponsePattern
		typeIdx int
		want    bool
	}{
		{
			name:    "the body would come from the status parameter",
			pattern: &ResponsePattern{StatusFromArg: true, StatusArgIndex: 0},
			typeIdx: 0,
			want:    true,
		},
		{
			name:    "a different parameter is fine",
			pattern: &ResponsePattern{StatusFromArg: true, StatusArgIndex: 0},
			typeIdx: 1,
			want:    false,
		},
		{
			// Index 0 is the default for an unset field, so a pattern that does
			// NOT read its status from an argument must not veto a body at
			// index 0 — that is where most single-argument responders put it.
			name:    "no status argument, so nothing to collide with",
			pattern: &ResponsePattern{StatusFromArg: false, StatusArgIndex: 0},
			typeIdx: 0,
			want:    false,
		},
		{
			name:    "a nil pattern vetoes nothing",
			pattern: nil,
			typeIdx: 0,
			want:    false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sameAsStatusParam(tc.pattern, tc.typeIdx); got != tc.want {
				t.Errorf("sameAsStatusParam = %v, want %v", got, tc.want)
			}
		})
	}
}

// The collision rule has to hold whichever order the roles arrive in.
// detectValueRound walks the call graph in stored order, so a body-only edge can
// reach the merge before the status-only one — and checking the collision solely
// while ADDING the body let that order slip a pattern through with one
// parameter in both roles (CodeRabbit on #486).
func TestResponseRoleCollisionIsOrderIndependent(t *testing.T) {
	// Body first, status second: nothing to collide with when the body lands.
	bodyFirst := &DetectedWrapper{Response: &ResponsePattern{
		TypeFromArg: true, TypeArgIndex: 0,
	}}
	normaliseResponseRoles(bodyFirst)
	if !bodyFirst.Response.TypeFromArg {
		t.Fatal("a body with no status recorded yet was dropped; there is nothing for it to collide with")
	}
	bodyFirst.Response.StatusFromArg, bodyFirst.Response.StatusArgIndex = true, 0
	normaliseResponseRoles(bodyFirst)
	if bodyFirst.Response.TypeFromArg {
		t.Error("the status arrived second and the body still reads the same parameter — " +
			"one parameter cannot be both roles")
	}
	if !bodyFirst.bodyIsStatus {
		t.Error("the declined body was not recorded, so a later merge will read it as 'no body found'")
	}

	// Status first, body second: the original order.
	statusFirst := &DetectedWrapper{Response: &ResponsePattern{
		StatusFromArg: true, StatusArgIndex: 0, TypeFromArg: true, TypeArgIndex: 0,
	}}
	normaliseResponseRoles(statusFirst)
	if statusFirst.Response.TypeFromArg || !statusFirst.bodyIsStatus {
		t.Error("the collision was not resolved when the status was recorded first")
	}

	// Different parameters: an ordinary responder keeps BOTH roles.
	ok := &DetectedWrapper{Response: &ResponsePattern{
		StatusFromArg: true, StatusArgIndex: 0, TypeFromArg: true, TypeArgIndex: 1,
	}}
	normaliseResponseRoles(ok)
	if !ok.Response.TypeFromArg || ok.Response.TypeArgIndex != 1 {
		t.Error("a body on its own parameter was dropped")
	}
	if ok.bodyIsStatus {
		t.Error("a responder with distinct roles was marked as having had its body declined")
	}

	// A declined body leaves the pattern pointing at no argument, so a consumer
	// reading TypeArgIndex without checking TypeFromArg cannot land on the
	// status by accident.
	if bodyFirst.Response.TypeArgIndex != -1 {
		t.Errorf("TypeArgIndex = %d after declining, want -1", bodyFirst.Response.TypeArgIndex)
	}
	// And nothing to normalise is not a crash.
	normaliseResponseRoles(&DetectedWrapper{})
}
