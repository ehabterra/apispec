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

	// CHANGE DETECTOR, not an endorsement: the prefix-as-parameter forms are a
	// different gap — resolving the sub-router's position is done, resolving the
	// PREFIX through a parameter is not (resolvePathArg does not trace
	// parameters). Asserted at today's wrong behaviour so this test fails, and
	// gets updated, the moment that is fixed.
	t.Run("prefix through a parameter is still unresolved", func(t *testing.T) {
		for _, notYet := range []string{"/param/things", "/named/things"} {
			if _, ok := out.Paths[notYet]; ok {
				t.Errorf("%s now resolves — the parameter-prefix gap is fixed; "+
					"update this subtest to assert it properly and close the follow-up", notYet)
			}
		}
	})
}
