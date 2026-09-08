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

	"github.com/ehabterra/apispec/internal/spec"
)

// A path that cannot be resolved must never be RENDERED. Rendering an argument
// yields a Go symbol, never a path: `resolvePathArg` had no case for a
// selector, so `settings.Path` came out as the literal segment
// `recvfield-->Config.Path` — internal separator included — and on a real
// project 60 paths read `gitea.dev/modules/web.Combo.pattern` (issue #461).
//
// Such endpoints do not exist. A consumer would call a URL that 404s, with
// nothing in the document to warn them, which is why this is worse than a
// missing route.
func TestTestdata_ReceiverFieldPath(t *testing.T) {
	out := loadTestdata(t, "receiver_field_path", spec.DefaultHTTPConfig())
	noDanglingRefs(t, out)

	for path := range out.Paths {
		// The tells of a rendered Go value: a package/type qualifier, or the
		// internal type separator.
		if strings.Contains(path, spec.TypeSep) {
			t.Errorf("path %q carries the internal type separator — an argument was rendered as a path", path)
		}
		for _, symbol := range []string{"recvfield", "Config.Path", ".pattern"} {
			if strings.Contains(path, symbol) {
				t.Errorf("path %q carries the Go symbol %q — an argument was rendered as a path", path, symbol)
			}
		}
	}

	// The literal route is unaffected: this must not cost the paths that do
	// resolve.
	if !hasPath(out, "/plain") {
		t.Errorf("/plain missing; have %v", mapPathKeys(out.Paths))
	}

	// The builder's real path is RECOVERED, not merely left honest: the path was
	// given once to the constructor (`r.Combo("/items")`) and each verb reads it
	// back off the receiver, so it is resolved by following the chain to the call
	// that built the receiver and reading the field out of the literal it
	// returns (issue #461).
	//
	// Both verbs must appear, which is what pins the chain WALK: `.Post` links
	// to `.Get`, not to the constructor, and a verb returns its receiver rather
	// than a literal, so a single hop would recover only the first.
	item, ok := out.Paths["/items"]
	if !ok {
		t.Fatalf("/items missing — the builder's path was not recovered; have %v", mapPathKeys(out.Paths))
	}
	if item.Get == nil {
		t.Error("/items has no GET (r.Combo(\"/items\").Get)")
	}
	if item.Post == nil {
		t.Error("/items has no POST — the chained verb links to Get, not to the constructor")
	}

	// The handlers are only ever PASSED to the builder, never called, and they
	// arrive at the registration as the wrapper's parameter — so their bodies
	// were invisible to the walk and both verbs documented the framework's
	// "no response found" default. With the binding substituted, the handler's
	// own encode/WriteHeader is read (issue #466).
	if item.Get != nil {
		if _, ok := item.Get.Responses["200"]; !ok {
			t.Errorf("GET /items responses = %v, want the 200 its handler encodes", sortedStatusKeys(item.Get))
		}
	}
	if item.Post != nil {
		if _, ok := item.Post.Responses["201"]; !ok {
			t.Errorf("POST /items responses = %v, want the 201 its handler writes", sortedStatusKeys(item.Post))
		}
	}

	// What replaces a fabricated segment is a declared placeholder, not a
	// silent shortening — the route stays addressable and visibly incomplete
	// (issue #34), and #428 reports the registration.
	for path, item := range out.Paths {
		item := item
		op := firstOperation(&item)
		if op == nil {
			continue
		}
		for _, seg := range strings.Split(path, "/") {
			if !strings.HasPrefix(seg, "{") {
				continue
			}
			name := strings.Trim(seg, "{}")
			declared := false
			for _, p := range op.Parameters {
				if p.Name == name || strings.HasSuffix(p.Ref, name) || strings.HasSuffix(p.Ref, name+"Param") {
					declared = true
				}
			}
			if !declared {
				t.Errorf("%s: placeholder {%s} is not declared as a parameter — an undeclared placeholder is an invalid path template",
					path, name)
			}
		}
	}
}
