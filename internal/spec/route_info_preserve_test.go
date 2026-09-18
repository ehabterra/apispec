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
	"testing"

	"github.com/ehabterra/apispec/internal/metadata"
)

// TestExtractRouteFillsRatherThanReplaces pins that a re-extraction ADDS to a
// RouteInfo instead of wiping it.
//
// handleRouteNode re-extracts into the SAME RouteInfo from a route node's
// children — that is how a chain-style route gets its path, and how a router
// wrapper's inner registration is reached. ExtractRoute used to open with
//
//	if routeInfo == nil || routeInfo.File == "" || routeInfo.Package == "" {
//		*routeInfo = RouteInfo{ ... }
//	}
//
// which conflates "not initialised yet" with "initialised but short one field".
// A registration that had already resolved `/captcha/*` off its own call site
// lost it the moment the nested read found File or Package unset, and the nested
// value — a placeholder for the wrapper's local — was then written into what now
// looked like a blank route. It walked straight past the guard added for #494,
// because that guard only defends a path it can still see.
//
// On gitea this decided endpoints by tuning limit: at MaxInstancesPerKey 25 the
// walk never went deep enough to re-extract, at 100 it did, and the two settings
// documented different route sets (issue #498).
func TestExtractRouteFillsRatherThanReplaces(t *testing.T) {
	meta := &metadata.Metadata{StringPool: metadata.NewStringPool()}
	cp := NewContextProvider(meta)

	callee := metadata.Call{
		Name: meta.StringPool.Get("Method"),
		Pkg:  meta.StringPool.Get("example.com/router"),
		Meta: meta,
	}
	edge := &metadata.CallGraphEdge{Callee: callee, Position: meta.StringPool.Get("router.go:162:3")}
	node := &TrackerNode{key: "nested-registration", CallGraphEdge: edge}

	matcher := NewRoutePatternMatcher(RoutePattern{CallRegex: `^Method$`}, &APISpecConfig{}, cp)

	// What the ENCLOSING registration already worked out, with File and Package
	// still unset — exactly the state the nested call used to discard.
	route := &RouteInfo{
		Path:           "/captcha/*",
		Method:         "GET",
		MethodExplicit: true,
		Response:       map[string]*ResponseInfo{},
		UsedTypes:      map[string]*Schema{},
	}
	matcher.ExtractRoute(node, route)

	if route.Path != "/captcha/*" {
		t.Errorf("re-extraction discarded the resolved path: got %q, want %q", route.Path, "/captcha/*")
	}
	if route.Method != "GET" {
		t.Errorf("re-extraction reset the method: got %q, want GET", route.Method)
	}
	if !route.MethodExplicit {
		t.Error("re-extraction cleared MethodExplicit, so the route would be re-inferred")
	}
	// The fields it SHOULD supply are still filled in.
	if route.Package == "" {
		t.Error("Package was left empty; filling is the reason this branch exists")
	}
	if route.File == "" {
		t.Error("File was left empty; filling is the reason this branch exists")
	}
	// And a genuinely blank RouteInfo must still be initialised.
	blank := &RouteInfo{}
	matcher.ExtractRoute(node, blank)
	if blank.Method == "" || blank.Package == "" || blank.Response == nil || blank.UsedTypes == nil {
		t.Errorf("a blank RouteInfo was not initialised: %+v", blank)
	}
}
