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
	"strings"
	"testing"

	"github.com/ehabterra/apispec/spec"
)

// TestTestdata_WrapperMountField pins the prefix of a sub-router mounted through
// a house router's own Mount method (issue #611):
//
//	func (r *Router) Mount(pattern string, sub *Router) {
//		r.chiRouter.Mount(r.getPattern(pattern), sub.chiRouter)
//	}
//
// Every route was found before the fix, at the root: the framework mount inside
// the wrapper sees a field of a parameter and a path built by a method call, and
// neither says where the sub-router came from. So the whole path set is asserted
// — a route documented without its prefix, or under another mount's prefix,
// fails here, and a count of routes could not notice either.
//
// Each wrapper is exercised with an inline call argument AND a variable,
// because the variable form needs its own producer link (golden rule 11).
func TestTestdata_WrapperMountField(t *testing.T) {
	out := loadTestdata(t, "wrapper_mount_field", spec.DefaultChiConfig())
	noDanglingRefs(t, out)
	noUnresolvedPlaceholders(t, out)

	want := map[string]string{
		"/api/v1/user":    "User", // Mount, prefixed pattern, variable argument
		"/api/v2/user2":   "User", // Mount, prefixed pattern, inline call argument
		"/api/plain/repo": "Repo", // MountPlain, variable argument
		"/api/pv/repo2":   "Repo", // MountPlain, inline call argument
		"/api/mux/mux":    "User", // MountMux (*chi.Mux), inline — the control
		"/api/mx/mux2":    "User", // MountMux (*chi.Mux), variable
		// Negative: registered on a wrapper router that is never mounted, so it
		// stays at the root — no prefix is invented for it.
		"/orphan": "Repo",
	}
	for path, body := range want {
		item, ok := out.Paths[path]
		if !ok {
			t.Errorf("path %q missing; have %v", path, mapPathKeys(out.Paths))
			continue
		}
		if item.Get == nil {
			t.Errorf("%s: no GET operation", path)
			continue
		}
		resp, ok := item.Get.Responses["200"]
		if !ok {
			t.Errorf("%s: no 200 response", path)
			continue
		}
		media, ok := resp.Content["application/json"]
		if !ok || media.Schema == nil || !strings.HasSuffix(media.Schema.Ref, "_"+body) {
			t.Errorf("%s: 200 body is not a $ref to %s: %+v", path, body, media.Schema)
		}
	}

	// Negative: no un-prefixed copy survives (the sub-router's subtree is also
	// reached directly, without the mount), and no route leaks under another
	// sub-router's prefix.
	for path := range out.Paths {
		if _, ok := want[path]; !ok {
			t.Errorf("unexpected path %q — a mounted route documented without its own prefix, or under another's?", path)
		}
	}
}
