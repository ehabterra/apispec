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

// TestInstanceBudget covers the configurable copy cap. It is configurable because
// the default is a compromise: the scope is a group closure rather than a route,
// so a group holding more routes than the budget starves every later route of its
// response body (issue #224). Until that scoping is fixed, raising the number is
// the only way to document such a project — which a constant does not allow.
func TestInstanceBudget(t *testing.T) {
	unset := &LazyTree{}
	if got := unset.instanceBudget(); got != DefaultMaxInstancesPerKey {
		t.Errorf("unset budget = %d, want the default %d", got, DefaultMaxInstancesPerKey)
	}

	configured := &LazyTree{limits: metadata.TrackerLimits{MaxInstancesPerKey: 200}}
	if got := configured.instanceBudget(); got != 200 {
		t.Errorf("configured budget = %d, want 200", got)
	}

	// A negative is not a budget; it means the same as unset rather than "no
	// cap", so a typo cannot turn the walk loose.
	negative := &LazyTree{limits: metadata.TrackerLimits{MaxInstancesPerKey: -1}}
	if got := negative.instanceBudget(); got != DefaultMaxInstancesPerKey {
		t.Errorf("negative budget = %d, want the default", got)
	}
}
