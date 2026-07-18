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

	"github.com/ehabterra/apispec/spec"
)

// TestTestdata_CrossPackageConstructorStatus covers the cross-package error
// constructor: a branch-assigned status handed to `common.NewAPIError(...)` (a
// selector call, whose callee name lives in .Sel, not the selector itself). This
// previously collapsed to a bare `default`; the resolver must resolve the callee
// name from the selector and fan the branch statuses to {400, 404, 500}.
func TestTestdata_CrossPackageConstructorStatus(t *testing.T) {
	out := loadTestdataWithFixtureConfig(t, "cross_package_constructor_status", spec.DefaultHTTPConfig())
	noDanglingRefs(t, out)
	noUnresolvedPlaceholders(t, out)

	post := opFor(out.Paths["/reserve"], "POST")
	if post == nil {
		t.Fatalf("POST /reserve missing; have %v", mapPathKeys(out.Paths))
	}
	got := keysOf(post.Responses)
	sort.Strings(got)
	for _, want := range []string{"400", "404", "500"} {
		if _, ok := post.Responses[want]; !ok {
			t.Errorf("POST /reserve missing status %s; have %v", want, got)
		}
	}
	if _, ok := post.Responses["default"]; ok {
		t.Errorf("POST /reserve still has an unresolved default; cross-package constructor statuses should be concrete; have %v", got)
	}
}
