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
	"sort"
	"testing"
)

// TestFuncFieldEngineParity runs the func-field dispatch fixtures (#143) through
// BOTH tracker engines and requires the same routes, with the same operations and
// response statuses, from each.
//
// The two engines add children in different places — the lazy tree expands by
// key, the eager tree materializes nodes — so a capability landing in one is
// silently absent from the other. That is not hypothetical here: the eager side
// initially found the routes and documented them *empty*, because a node builds
// its own callees while its ARGUMENTS are built by the parent loop, and the new
// attachment point had to do that itself. Comparing statuses per operation is
// what catches that class of divergence; comparing path names alone would not.
func TestFuncFieldEngineParity(t *testing.T) {
	for _, fixture := range []string{"cli_action_routes", "cli_action_cross_package"} {
		t.Run(fixture, func(t *testing.T) {
			// nil config on purpose: these fixtures rely on framework detection
			// and the multi-framework merge, which an explicit config bypasses.
			lazy := generateWithTracker(t, fixture, nil, true)
			eager := generateWithTracker(t, fixture, nil, false)

			lazyPaths, eagerPaths := mapPathKeys(lazy.Paths), mapPathKeys(eager.Paths)
			sort.Strings(lazyPaths)
			sort.Strings(eagerPaths)
			if len(lazyPaths) == 0 {
				t.Fatalf("lazy documented no paths at all — the fixture stopped exercising #143")
			}
			if len(lazyPaths) != len(eagerPaths) {
				t.Fatalf("paths differ between engines:\n lazy:  %v\n eager: %v", lazyPaths, eagerPaths)
			}
			for i := range lazyPaths {
				if lazyPaths[i] != eagerPaths[i] {
					t.Fatalf("paths differ between engines:\n lazy:  %v\n eager: %v", lazyPaths, eagerPaths)
				}
			}

			for _, path := range lazyPaths {
				for _, method := range []string{"GET", "POST", "PUT", "PATCH", "DELETE"} {
					lazyOp := opFor(lazy.Paths[path], method)
					eagerOp := opFor(eager.Paths[path], method)
					if (lazyOp == nil) != (eagerOp == nil) {
						t.Errorf("%s %s: lazy=%v eager=%v", method, path, lazyOp != nil, eagerOp != nil)
						continue
					}
					if lazyOp == nil {
						continue
					}
					// Statuses, not schemas: the question is whether the handler
					// body was reached at all, which is what diverged.
					if len(lazyOp.Responses) != len(eagerOp.Responses) {
						t.Errorf("%s %s: response count lazy=%d eager=%d",
							method, path, len(lazyOp.Responses), len(eagerOp.Responses))
					}
					for status := range lazyOp.Responses {
						if _, ok := eagerOp.Responses[status]; !ok {
							t.Errorf("%s %s: status %q in lazy but not eager", method, path, status)
						}
					}
					if (lazyOp.RequestBody == nil) != (eagerOp.RequestBody == nil) {
						t.Errorf("%s %s: requestBody lazy=%v eager=%v",
							method, path, lazyOp.RequestBody != nil, eagerOp.RequestBody != nil)
					}
				}
			}
		})
	}
}
