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

func TestTestdata_ChiReceiverNameCollision(t *testing.T) {
	out := loadTestdataWithFixtureConfig(t, "chi_receiver_name_collision", spec.DefaultChiConfig())
	noDanglingRefs(t, out)
	noUnresolvedPlaceholders(t, out)

	for _, path := range []string{
		"/api/v1/tenant",
		"/api/v1/capabilities",
		"/api/v1/users",
	} {
		item, ok := out.Paths[path]
		if !ok {
			t.Errorf("path %q missing; have %v", path, mapPathKeys(out.Paths))
			continue
		}
		if opFor(item, "GET") == nil {
			t.Errorf("GET %s: expected operation, missing", path)
		}
	}
}
