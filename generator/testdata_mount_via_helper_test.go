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
	"path/filepath"
	"testing"

	"github.com/ehabterra/apispec/internal/engine"
)

// TestTestdata_MountViaHelper pins that a Mount written inside a helper still
// applies its prefix (issue #275).
//
// A mount prefix reaches nested routes by tree containment. With the Mount one
// function deeper the sub-router was built at the CALL SITE, so its routes hang
// under the caller's argument while the Mount node sits in the helper's body —
// siblings, not ancestor and descendant. The routes were documented at the wrong
// paths, with nothing to indicate the path was incomplete.
func TestTestdata_MountViaHelper(t *testing.T) {
	cfg := engine.DefaultEngineConfig()
	cfg.InputDir = filepath.Join("..", "testdata", "mount_via_helper")

	out, err := engine.NewEngine(cfg).GenerateOpenAPI()
	if err != nil {
		t.Fatalf("GenerateOpenAPI: %v", err)
	}

	noDanglingRefs(t, out)
	noUnresolvedPlaceholders(t, out)

	for _, want := range []string{
		"/direct/things", // control: mounted at the call site
		"/api/things",    // literal prefix inside a helper — the fix
	} {
		if _, ok := out.Paths[want]; !ok {
			got := make([]string, 0, len(out.Paths))
			for p := range out.Paths {
				got = append(got, p)
			}
			t.Errorf("missing %s; documented %v — a Mount in a helper must still apply its prefix", want, got)
		}
	}

	// The un-prefixed form must NOT survive alongside the prefixed one: the
	// sub-router's subtree is reached both directly and through the mount, and
	// dropSubsumedMountPrefixes is what keeps only the most-mounted form.
	if _, ok := out.Paths["/things"]; ok {
		t.Error("/things is documented as well as its mounted forms — the un-prefixed " +
			"duplicate must be dropped, or every helper-mounted route appears twice")
	}

	// The prefix-as-parameter form resolves since #431: a path argument that is
	// a bare identifier now goes through the same ladder as one operand of a
	// concatenation, so `r.Mount(prefix, server)` reads the caller's "/param".
	t.Run("prefix through a parameter resolves", func(t *testing.T) {
		if _, ok := out.Paths["/param/things"]; !ok {
			t.Errorf("missing /param/things; documented %v — a prefix passed as a "+
				"parameter must be read from the caller's argument", mapPathKeys(out.Paths))
		}
	})

	// CHANGE DETECTOR, not an endorsement: a prefix wrapped in a CONVERSION at
	// the Mount call (`r.Mount(string(prefix), server)`) still does not resolve.
	// Worse, it renders as the conversion's TYPE, so the fixture's mountNamed
	// route is documented at /string/things — a path that looks literal and
	// exists nowhere (issue #433). Asserted at today's wrong behaviour so this
	// fails, and gets updated, the moment that is fixed.
	t.Run("prefix through a conversion is still unresolved", func(t *testing.T) {
		if _, ok := out.Paths["/named/things"]; ok {
			t.Error("/named/things now resolves — the conversion gap is fixed; " +
				"assert it properly here and close #433")
		}
		if _, ok := out.Paths["/string/things"]; !ok {
			t.Error("/string/things is gone — the conversion no longer renders as its " +
				"type; update this subtest and close #433")
		}
	})
}
