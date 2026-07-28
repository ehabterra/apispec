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

package callgraph

import (
	"reflect"
	"strings"
	"testing"
)

// TestSiteKey pins the join key both graphs have to agree on.
func TestSiteKey(t *testing.T) {
	tests := []struct {
		name     string
		position string
		calleeID string
		want     string
		why      string
	}{
		{
			name:     "column is dropped",
			position: "/src/app/context.go:25:21",
			calleeID: "encoding/json.NewEncoder",
			want:     "/src/app/context.go:25#NewEncoder",
			why:      "the two graphs never agree on the column — metadata records the statement start, SSA the opening parenthesis",
		},
		{
			name:     "a differently-qualified callee keys the same",
			position: "/src/app/media.go:40:9",
			calleeID: "example.com/app/storage.s3driver.List",
			want:     "/src/app/media.go:40#List",
			why:      "an interface call metadata records as Storage.List must reach the concrete target VTA resolved, or the disagreement is invisible",
		},
		{
			name:     "a Windows drive letter is not mistaken for a column",
			position: `C:\src\app\main.go:12:3`,
			calleeID: "net/http.ListenAndServe",
			want:     `C:\src\app\main.go:12#ListenAndServe`,
			why:      "the colon after the drive letter is inside the filename",
		},
		{
			name:     "a position with no column is already a line",
			position: "/src/app/main.go:12",
			calleeID: "pkg.Fn",
			want:     "/src/app/main.go:12#Fn",
		},
		{
			name:     "no position, no key",
			position: "",
			calleeID: "pkg.Fn",
			want:     "",
			why:      "a call that is nowhere cannot be joined to",
		},
		{
			name:     "no callee, no key",
			position: "/src/app/main.go:12:3",
			calleeID: "",
			want:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SiteKey(tt.position, tt.calleeID); got != tt.want {
				t.Errorf("SiteKey(%q, %q) = %q, want %q — %s", tt.position, tt.calleeID, got, tt.want, tt.why)
			}
		})
	}
}

// TestBareName covers the discriminator that separates several calls written on
// one line.
func TestBareName(t *testing.T) {
	for id, want := range map[string]string{
		"encoding/json.Encoder.Encode": "Encode",
		"example.com/app.Handler":      "Handler",
		"Encode":                       "Encode",
		"":                             "",
	} {
		if got := BareName(id); got != want {
			t.Errorf("BareName(%q) = %q, want %q", id, got, want)
		}
	}
}

// TestCallSitesOnNilGraph pins that the accessor is safe before a build has run —
// it is reachable whenever the resolved graph is off.
func TestCallSitesOnNilGraph(t *testing.T) {
	var r *Resolved
	if sites := r.CallSites(); sites != nil {
		t.Errorf("CallSites on a nil Resolved returned %d sites", len(sites))
	}
	if index := (&Resolved{}).CalleesAt(); len(index) != 0 {
		t.Errorf("CalleesAt on an unbuilt Resolved returned %d entries", len(index))
	}
}

// TestCallSitesOverRealGraph pins the accessor against a built graph: every
// resolved call must carry a position in a real file, the order must be stable,
// and CalleesAt must be able to answer for a call the fixture actually makes.
func TestCallSitesOverRealGraph(t *testing.T) {
	r := Build(loadFixture(t, muxFixture))

	sites := r.CallSites()
	if len(sites) == 0 {
		t.Fatal("no call sites in a fixture that plainly makes calls")
	}

	for _, site := range sites {
		if !site.Position.IsValid() {
			t.Errorf("site %s -> %s has no position; it could never be joined to", site.CallerID, site.CalleeID)
			break
		}
		if site.CallerID == "" || site.CalleeID == "" {
			t.Errorf("site at %s has an empty end: caller=%q callee=%q", site.Position, site.CallerID, site.CalleeID)
			break
		}
	}

	// Determinism: the join is built from this, and anything feeding output must
	// be ordered (golden rule #1).
	again := r.CallSites()
	if !reflect.DeepEqual(sites, again) {
		t.Error("CallSites is not deterministic across calls on the same graph")
	}

	// mux.Vars is the call this fixture exists for; it must be findable by key.
	index := r.CalleesAt()
	found := false
	for key, targets := range index {
		if strings.HasSuffix(key, "#Vars") {
			for _, target := range targets {
				if strings.Contains(target, "mux") {
					found = true
				}
			}
		}
	}
	if !found {
		t.Errorf("no `#Vars` site resolving into gorilla/mux; %d keys indexed", len(index))
	}
}
