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

	"github.com/ehabterra/apispec/internal/spec"
)

// The four ways a wrapper's registration is reached, as a 2x2: one verb or
// several (the else branch or the loop over a split list), and the handler
// passed straight through or spread from a slice.
//
// It is regression coverage, not a reproduction. The bug it was built for
// (issue #498) needed a re-extraction that found File or Package unset, which
// none of these shapes produces at fixture scale — on gitea it took a wrapper
// several calls deep. TestExtractRouteFillsRatherThanReplaces pins that
// directly; this pins that all four wrapper shapes resolve at all, which is
// what no other fixture covers.
func TestTestdata_RouterWrapperVariadic(t *testing.T) {
	out := loadTestdata(t, "router_wrapper_variadic", spec.DefaultHTTPConfig())
	noDanglingRefs(t, out)

	for _, tc := range []struct {
		path string
		why  string
	}{
		{"/loop-direct", "several verbs, handler passed straight through"},
		{"/loop-spread", "several verbs, handler spread from a slice"},
		{"/single-direct", "one verb, handler passed straight through"},
		{"/single-spread", "one verb via Get, handler spread from a slice"},
	} {
		if _, ok := out.Paths[tc.path]; !ok {
			t.Errorf("path %q missing (%s); have %v", tc.path, tc.why, mapPathKeys(out.Paths))
		}
	}

	// No wrapper local leaked out as a path segment.
	for path := range out.Paths {
		if path == "{full}" || path == "/{full}" || path == "{fullPattern}" {
			t.Errorf("path %q is a wrapper local, not a route", path)
		}
	}
}
