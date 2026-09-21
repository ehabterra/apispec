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
// from a struct field, for every way the field gets its value:
//
//	app := NewApp(WithOpt(OptAPI()))   // an option's closure stores r in a.opt
//	app.SetSet(SetAPI())               // a setter stores r in a.set
//	app := &App{lit: LitAPI()}         // a struct literal sets a.lit (#565)
//	root.Mount("/opt", a.opt)
//
// The routes are found whether or not the field is traced to the call that
// built its router; only the prefix goes missing, and a count of routes cannot
// notice that. So the whole path set is asserted: a route documented at the
// root instead of under its mount fails here.
//
// History: the fix for #550 dropped the option and setter shapes (a callee-body
// store of a bound parameter lost its producer); the struct-literal shapes were
// never traced until #565.
func TestTestdata_MountFieldProducer(t *testing.T) {
	out := loadTestdata(t, "mount_field_producer", spec.DefaultChiConfig())
	noDanglingRefs(t, out)

	want := map[string]bool{
		"/opt/api/items":       true, // functional option
		"/set/api/users":       true, // setter method
		"/lit/api/orders":      true, // pointer literal assigned to a variable
		"/direct/api/direct":   true, // pointer literal returned directly
		"/value/api/value":     true, // value literal, value receiver
		"/local/api/local":     true, // literal built in main itself
		"/convlit/api/convlit": true, // literal element through a type conversion
		"/convset/api/convset": true, // explicit store through a type conversion
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
