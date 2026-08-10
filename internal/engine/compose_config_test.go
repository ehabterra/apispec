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
	"path/filepath"
	"strings"
	"testing"
)

// TestComposeFrameworkConfig pins the composition every consumer must share.
//
// The UI used to build its config from the detected *primary alone*. That is
// broken for any project whose primary is decided by file-walk order (issue
// #212): photoprism's primary detects as `mux` because one dummy file imports
// gorilla/mux and sorts first, so a mux-only config recognises none of its gin
// routes — the CLI documented 107 paths while the UI documented zero.
func TestComposeFrameworkConfig(t *testing.T) {
	dir := filepath.Join("..", "..", "testdata", "mixed_gin_mux")

	cfg, frameworks, err := ComposeFrameworkConfig(dir)
	if err != nil {
		t.Fatalf("ComposeFrameworkConfig: %v", err)
	}
	if len(frameworks) < 2 {
		t.Fatalf("expected both frameworks detected, got %v", frameworks)
	}

	recvs := make([]string, 0, len(cfg.Framework.RoutePatterns))
	for _, p := range cfg.Framework.RoutePatterns {
		recvs = append(recvs, p.RecvType+p.RecvTypeRegex)
	}
	joined := strings.Join(recvs, "\n")

	// Every detected framework must be recognisable, not just the one that
	// happened to sort first.
	for _, want := range []string{"gin-gonic/gin", "gorilla/mux"} {
		if !strings.Contains(joined, want) {
			t.Errorf("composed config cannot recognise %s registrations; receivers:\n%s", want, joined)
		}
	}
	// The stdlib surface is layered underneath for mixed projects.
	if !strings.Contains(joined, "net/http") {
		t.Errorf("composed config is missing the net/http surface; receivers:\n%s", joined)
	}
}

// TestComposeFrameworkConfigWithPrimary covers the UI's framework selector: the
// user picks which framework leads, and the others must still merge in — picking
// one must not silently discard the rest, which is the bug this replaces.
func TestComposeFrameworkConfigWithPrimary(t *testing.T) {
	dir := filepath.Join("..", "..", "testdata", "mixed_gin_mux")

	for _, primary := range []string{"gin", "mux", "chi"} {
		cfg, frameworks, err := ComposeFrameworkConfigWithPrimary(dir, primary)
		if err != nil {
			t.Fatalf("primary %s: %v", primary, err)
		}
		if frameworks[0] != primary {
			t.Errorf("primary %s: leading framework = %s", primary, frameworks[0])
		}

		var joined strings.Builder
		for _, p := range cfg.Framework.RoutePatterns {
			joined.WriteString(p.RecvType + p.RecvTypeRegex + "\n")
		}
		// Both detected frameworks survive whichever one the user selected —
		// including "chi", which this project does not use at all: an explicit
		// choice leads, the detected ones still merge under it.
		for _, want := range []string{"gin-gonic/gin", "gorilla/mux"} {
			if !strings.Contains(joined.String(), want) {
				t.Errorf("primary %s: %s registrations became unrecognisable:\n%s", primary, want, joined.String())
			}
		}
	}
}

// TestComposeFrameworkConfigSingleFramework checks the ordinary case is
// unchanged: one framework, plus the net/http layer.
func TestComposeFrameworkConfigSingleFramework(t *testing.T) {
	cfg, frameworks, err := ComposeFrameworkConfig(filepath.Join("..", "..", "testdata", "gin"))
	if err != nil {
		t.Fatalf("ComposeFrameworkConfig: %v", err)
	}
	if len(frameworks) != 1 || frameworks[0] != "gin" {
		t.Fatalf("frameworks = %v, want [gin]", frameworks)
	}
	if len(cfg.Framework.RoutePatterns) == 0 {
		t.Fatal("no route patterns composed")
	}
}

// TestComposeFrameworkConfigNormalisesPrimaryCase covers user-supplied primary
// names. The UI posts req.Framework straight into
// ComposeFrameworkConfigWithPrimary without validating it against the registry,
// so "Gin" is reachable. The config lookup is case-insensitive, but the dedup
// and stdlib comparisons are ==, so an unnormalised name used to return the
// framework twice ([Gin gin]) and merge gin's own patterns back over itself.
func TestComposeFrameworkConfigNormalisesPrimaryCase(t *testing.T) {
	for _, primary := range []string{"GIN", "Gin", "gIn"} {
		cfg, frameworks := ComposeFrameworkConfigFrom([]string{"gin"}, primary)

		if len(frameworks) != 1 || frameworks[0] != "gin" {
			t.Errorf("primary %q: frameworks = %v, want [gin] — the name was not canonicalised before dedup", primary, frameworks)
		}
		base, _ := ComposeFrameworkConfigFrom([]string{"gin"}, "gin")
		if len(cfg.Framework.RoutePatterns) != len(base.Framework.RoutePatterns) {
			t.Errorf("primary %q: composed %d route patterns, canonical %q composed %d — the secondary merge ran against itself",
				primary, len(cfg.Framework.RoutePatterns), "gin", len(base.Framework.RoutePatterns))
		}
	}

	// The stdlib name carries a slash and is compared against separately.
	if _, frameworks := ComposeFrameworkConfigFrom([]string{"net/http"}, "NET/HTTP"); len(frameworks) != 1 {
		t.Errorf("frameworks = %v, want one entry", frameworks)
	}

	// An unknown explicit choice is still carried through, not rejected.
	_, frameworks := ComposeFrameworkConfigFrom([]string{"gin"}, "house-router")
	if len(frameworks) != 2 || frameworks[0] != "house-router" {
		t.Errorf("frameworks = %v, want [house-router gin]", frameworks)
	}
}

// TestComposeFrameworkConfigFromDoesNotMutateInput guards the canonicalisation
// pass: the caller's slice is shared (the engine reuses the detected list).
func TestComposeFrameworkConfigFromDoesNotMutateInput(t *testing.T) {
	input := []string{"GIN", "MUX"}
	ComposeFrameworkConfigFrom(input, "")
	if input[0] != "GIN" || input[1] != "MUX" {
		t.Errorf("input slice was rewritten in place: %v", input)
	}
}
