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
	"strings"
	"testing"
	"time"

	intspec "github.com/ehabterra/apispec/internal/spec"
)

// TestReportUnresolvedRefs covers the message a user actually sees when a type
// resolved to nothing useful (issue #327). What matters is that it names the GO
// TYPE: the mangled component name does not tell anyone which dependency to
// register under externalTypes.
func TestReportUnresolvedRefs(t *testing.T) {
	capture := func(refs []intspec.UnresolvedRef) string {
		var got string
		e := NewEngine(&EngineConfig{OnPhase: func(phase string, _ time.Duration) { got = phase }})
		e.unresolvedRefs = refs
		e.reportUnresolvedRefs()
		return got
	}

	t.Run("says nothing when everything resolved", func(t *testing.T) {
		if msg := capture(nil); msg != "" {
			t.Errorf("reported %q on a clean run", msg)
		}
	})

	t.Run("names the Go type and counts sites", func(t *testing.T) {
		msg := capture([]intspec.UnresolvedRef{
			{Component: "github_com_golang-jwt_jwt_v5_RegisteredClaims", GoType: "github.com/golang-jwt/jwt/v5.RegisteredClaims", Sites: 3},
			{Component: "xorm_io_xorm_convert_Conversion", GoType: "xorm.io/xorm/convert.Conversion", Sites: 1},
		})

		for _, want := range []string{
			"2 type(s)",
			"4 reference(s)",
			"github.com/golang-jwt/jwt/v5.RegisteredClaims",
			"(3 sites)",
			"xorm.io/xorm/convert.Conversion",
		} {
			if !strings.Contains(msg, want) {
				t.Errorf("message does not mention %q:\n%s", want, msg)
			}
		}
		// A single site needs no count — it would be noise on every entry.
		if strings.Contains(msg, "(1 sites)") {
			t.Errorf("single-site entry should carry no count:\n%s", msg)
		}
		// The mangled name is not what a user can act on.
		if strings.Contains(msg, "github_com_golang-jwt") {
			t.Errorf("message leaks the mangled component name:\n%s", msg)
		}
	})

	t.Run("falls back to the component when the type is unknown", func(t *testing.T) {
		msg := capture([]intspec.UnresolvedRef{{Component: "Mystery", Sites: 1}})
		if !strings.Contains(msg, "Mystery") {
			t.Errorf("message should name the component when the Go type is unknown:\n%s", msg)
		}
	})
}

// TestGetUnresolvedRefs covers the machine-readable half: a CI gate or the UI
// reads this rather than grepping the warning text.
func TestGetUnresolvedRefs(t *testing.T) {
	e := NewEngine(&EngineConfig{})
	if got := e.GetUnresolvedRefs(); len(got) != 0 {
		t.Errorf("fresh engine reports %+v", got)
	}

	want := []intspec.UnresolvedRef{{Component: "T", GoType: "pkg.T", Sites: 2}}
	e.unresolvedRefs = want
	got := e.GetUnresolvedRefs()
	if len(got) != 1 || got[0].GoType != "pkg.T" || got[0].Sites != 2 {
		t.Errorf("GetUnresolvedRefs() = %+v, want %+v", got, want)
	}
}
