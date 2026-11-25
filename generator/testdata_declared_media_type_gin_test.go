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
	"strings"
	"testing"

	"github.com/ehabterra/apispec/internal/spec"
)

// TestTestdata_DeclaredMediaTypeGin pins a Content-Type declaration ahead of a
// gin renderer whose status the branches assign (issue #652). The status
// fan-out copies the body into one response per status; each copy is the same
// write, so each goes out under the declaration. A copy that dropped the
// renderer's header-media-type mark stayed under application/json and left
// the declaration at a 200 the handler never sends.
func TestTestdata_DeclaredMediaTypeGin(t *testing.T) {
	out := loadTestdata(t, "declared_media_type_gin", spec.DefaultGinConfig())
	noDanglingRefs(t, out)

	cases := []struct {
		path     string
		statuses []string
		media    string
	}{
		{"/branch-status", []string{"400", "422"}, "application/problem+json"},
		// An unreadable branch fans out to a default that is pruned beside the
		// concrete 400 (see testdata_branched_status_constructor_test.go); the
		// 400 still takes the declaration, which strands no 200.
		{"/residue-status", []string{"400"}, "application/problem+json"},
		// Negative: nothing declared, nothing adopted.
		{"/undeclared", []string{"400", "422"}, "application/json"},
	}
	for _, tc := range cases {
		op := opFor(out.Paths[tc.path], "GET")
		if op == nil {
			t.Errorf("GET %s missing; have %v", tc.path, mapPathKeys(out.Paths))
			continue
		}
		got := statusKeys(op)
		sort.Strings(got)
		if strings.Join(got, ",") != strings.Join(tc.statuses, ",") {
			t.Errorf("GET %s statuses = %v, want %v", tc.path, got, tc.statuses)
		}
		for _, status := range tc.statuses {
			got := contentTypesOf(op, status)
			if len(got) != 1 || got[0] != tc.media {
				t.Errorf("GET %s %s content = %v, want only %q", tc.path, status, got, tc.media)
				continue
			}
			schema := op.Responses[status].Content[tc.media].Schema
			if schema == nil || !strings.HasSuffix(schema.Ref, "_Problem") {
				t.Errorf("GET %s %s: schema %+v, want a $ref to Problem", tc.path, status, schema)
			}
		}
	}
}
