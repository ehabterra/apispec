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

// TestTestdata_MountFieldProducer locks in the mount prefix of a router mounted
// from a struct field:
//
//	app := NewApp(WithOpt(OptAPI()))   // the option's closure stores r in a.opt
//	app.SetSet(SetAPI())               // the setter stores r in a.set
//	root.Mount("/opt", a.opt)
//
// The fix for #550 stopped linking a variable assigned in a callee's body to
// the call that invokes it, and with it the one case that link is right for:
// the value stored IS a parameter that call bound. The routes stayed found and
// were documented at the root — every one of them, silently, in a real service
// that wires its modules with functional options. That is why the whole path
// set is asserted here: a count of routes cannot notice a prefix going missing.
func TestTestdata_MountFieldProducer(t *testing.T) {
	out := loadTestdata(t, "mount_field_producer", spec.DefaultChiConfig())
	noDanglingRefs(t, out)

	want := map[string]bool{
		"/opt/api/items": true, // functional option
		"/set/api/users": true, // setter method
		// NOT PREFIXED, and asserted so the day it changes: a router placed in
		// a struct LITERAL (`&App{lit: LitAPI()}`) was never traced to its
		// field, before #550 or since. Assert /lit/api/orders and drop this
		// line when it is.
		"/api/orders": true,
	}
	for path := range want {
		if _, ok := out.Paths[path]; !ok {
			t.Errorf("path %q missing; have %v", path, mapPathKeys(out.Paths))
		}
	}
	for path := range out.Paths {
		if !want[path] {
			t.Errorf("unexpected path %q — a route documented without the prefix it was mounted under?", path)
		}
	}
}
