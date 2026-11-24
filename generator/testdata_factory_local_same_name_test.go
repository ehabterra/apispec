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
	"testing"

	"github.com/ehabterra/apispec/spec"
)

// assertExactGetPaths fails unless out documents exactly want, each with a GET
// operation. A route left at the root beside its prefixed copy, or documented
// at the root instead, shows up as an unexpected path.
func assertExactGetPaths(t *testing.T, out *spec.OpenAPISpec, want []string) {
	t.Helper()
	wanted := map[string]bool{}
	for _, path := range want {
		wanted[path] = true
		item, ok := out.Paths[path]
		if !ok {
			t.Errorf("path %q missing; have %v", path, mapPathKeys(out.Paths))
			continue
		}
		if item.Get == nil {
			t.Errorf("%s: no GET operation", path)
		}
	}
	for path := range out.Paths {
		if !wanted[path] {
			t.Errorf("unexpected path %q — a route outside the prefix it was mounted under?", path)
		}
	}
}

// TestTestdata_FactoryLocalSameName locks in issue #628: a router built by a
// factory and mounted by the caller keeps its mount prefix when the factory's
// local has the caller variable's own name (`same := Same()` in main, `same :=
// chi.NewRouter()` in Same). The metadata assignment relation paired the two
// `same`s by name, so the caller's variable had no routes beneath it and they
// stayed at the root.
//
// Beside it: a differently named local (the control that always worked), the
// factory called inline in the Mount, a factory in another package, and the
// shape in a helper instead of main, whose assignments are recorded
// differently.
func TestTestdata_FactoryLocalSameName(t *testing.T) {
	out := loadTestdata(t, "factory_local_same_name", spec.DefaultChiConfig())
	noDanglingRefs(t, out)
	noUnresolvedPlaceholders(t, out)

	assertExactGetPaths(t, out, []string{
		"/a/same",   // factory local named like the caller's variable
		"/b/other",  // differently named local (control)
		"/c/inline", // factory called in the Mount's argument list
		"/x/cross",  // factory in another package, same name collision
		"/h/helper", // the collision in a helper rather than main
	})
}

// TestTestdata_FactoryLocalSameNameFiber is the fiber form of issue #628:
// `same := Same(); app.Mount("/a", same)` with `same := fiber.New()` inside
// Same.
func TestTestdata_FactoryLocalSameNameFiber(t *testing.T) {
	out := loadTestdata(t, "factory_local_same_name_fiber", spec.DefaultFiberConfig())
	noDanglingRefs(t, out)
	noUnresolvedPlaceholders(t, out)

	assertExactGetPaths(t, out, []string{"/a/same", "/b/other", "/c/inline"})
}

// TestTestdata_FactoryLocalSameNameGin is the gin form of the shape: gin has no
// Mount, so the factory returns a group it made itself and the caller registers
// on it. The routes belong under the factory's Group() call whatever its local
// is called (issue #633 — the caller's routes used to hang beside that call,
// not beneath it, and were documented at the root).
func TestTestdata_FactoryLocalSameNameGin(t *testing.T) {
	out := loadTestdata(t, "factory_local_same_name_gin", spec.DefaultGinConfig())
	noDanglingRefs(t, out)
	noUnresolvedPlaceholders(t, out)

	assertExactGetPaths(t, out, []string{"/v1/same", "/v2/other"})
}
