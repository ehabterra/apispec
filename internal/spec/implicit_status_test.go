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
