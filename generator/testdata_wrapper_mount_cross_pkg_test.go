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

// TestTestdata_WrapperMountCrossPkg pins a house router's Mount methods when the
// router, the sub-router factories and the caller live in three packages
// (issue #611):
//
//   - a wrapper that mounts a fixed router on its receiver beside the forwarded
//     one is still derived, whichever of the two comes first in its body, and
//     the fixed router stays where the receiver is — the caller's prefix
//     belongs to the router it passed in;
//   - a wrapper handed two routers prefixes only the one it forwards: the
//     second, which its body mounts at a fixed path of its own, is not also
//     documented under the caller's prefix;
//   - a wrapper that adds its own text to the pattern (`path.Join("/fixed",
//     pattern)`) is not applied, so no route is documented at a path the
//     wrapper never mounts it at.
//
// The whole path set is asserted: a route without its prefix, or under one it
// does not belong to, fails here.
func TestTestdata_WrapperMountCrossPkg(t *testing.T) {
	out := loadTestdata(t, "wrapper_mount_cross_pkg", spec.DefaultChiConfig())
	noDanglingRefs(t, out)
	noUnresolvedPlaceholders(t, out)

	want := map[string]bool{
		"/api/v1/user": true, // Mount, variable from another package's factory
		"/api/h/h":     true, // MountWithHealth: the underivable mount came first
		"/api/s/s":     true, // MountWithStatus: the forwarding mount came first
		"/api/p/p":     true, // MountPair, sub-routers built inline
		"/api/pv/pv":   true, // MountPair, sub-routers held in variables
		// Mounted on the receiver inside the wrappers' bodies, not under the
		// caller's prefix.
		"/health/ping": true,
		"/status/ping": true,
		// MountFixed is left unapplied: the route keeps the context it was
		// registered in rather than "/v1/f", which drops the wrapper's "/fixed".
		"/f": true,
		// Mount, the factory's local has the caller variable's own name
		// (issue #628).
		"/api/same/same": true,
		// MountPair's second router is never under "/api/p" or "/api/pv".
		// Change detector: the body's `Mount("/extra", extra.ChiRouter)` names
		// the parameter through a field selector, and resolveArgThroughParams
		// follows a bare identifier only, so the router keeps the caller's
		// outer context. When that is fixed these become "/extra/x" and
		// "/extra/xv".
		"/x":  true,
		"/xv": true,
	}
	for path := range want {
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
		if !want[path] {
			t.Errorf("unexpected path %q — a route under a prefix it does not belong to, or without its own?", path)
		}
	}
}
