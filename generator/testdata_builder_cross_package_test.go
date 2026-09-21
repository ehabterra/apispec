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

// A builder declared in ANOTHER package — the ordinary shape on a real project,
// and the one a single-package fixture cannot produce (issue #506, golden rule
// #10). Two facts only exist here:
//
//   - the constructor is reached by a selector call (`r.NewCombo`) and the
//     literal it returns states its type as a selector too (`&web.Combo{…}`),
//     so anything reading a name off the ident alone gets "" for both;
//   - the receiver's type lives in a package that is not the registration's, so
//     the package half of the identity check compares two different packages
//     rather than one with itself.
func TestTestdata_BuilderCrossPackage(t *testing.T) {
	out := loadTestdata(t, "builder_cross_package", spec.DefaultHTTPConfig())
	noDanglingRefs(t, out)

	for _, path := range []string{"/direct", "/assigned", "/plain"} {
		if _, ok := out.Paths[path]; !ok {
			t.Errorf("path %q missing; have %v", path, mapPathKeys(out.Paths))
		}
	}

	// Both verbs on the variable-assigned chain, which is what pins that the
	// receiver is resolved per invocation rather than once.
	if item, ok := out.Paths["/assigned"]; ok {
		if item.Get == nil {
			t.Error("/assigned has no GET")
		}
		if item.Post == nil {
			t.Error("/assigned has no POST")
		}
	}

	// The receiver field must never be rendered as a path segment: across a
	// package boundary that yields the fully qualified Go symbol, which is what
	// 60 paths on a real project used to read (issue #461).
	for path := range out.Paths {
		for _, symbol := range []string{"buildercrosspackage", "pattern", "Combo"} {
			if strings.Contains(path, symbol) {
				t.Errorf("path %q carries the Go symbol %q — an argument was rendered as a path", path, symbol)
			}
		}
	}
}
