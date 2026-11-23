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

// TestTestdata_ReturnedGroup locks in issue #633: a group a helper creates and
// returns keeps its prefix when the caller registers on it. The caller's routes
// used to hang under the helper's call, beside the Group() call that carries
// the prefix rather than beneath it, so they were documented at the root.
//
// Covered side by side: an ident return, a call return, a two-hop helper, a
// group nested inside the helper, and one helper called from two parents (each
// caller's route under its own parent only). The negatives: a fresh engine that
// nothing mounts stays unprefixed, and a helper returning its parameter keeps
// the caller's prefix once, not twice.
func TestTestdata_ReturnedGroup(t *testing.T) {
	out := loadTestdata(t, "returned_group", spec.DefaultGinConfig())
	noDanglingRefs(t, out)
	noUnresolvedPlaceholders(t, out)

	assertExactGetPaths(t, out, []string{
		"/ident/x",
		"/direct/x",
		"/hop/x",
		"/api/nested/x",
		"/public/v/a",
		"/admin/v/b",
		"/passed/reg",
		"/witherr/x",
		"/fresh",
		"/kept/same",
		// Pick returns /alt on one path and /main on the other. Neither is the
		// prefix, so none is applied (golden rule #7): the route stays where it
		// was before #633 rather than under a guessed group.
		"/pick",
	})
}

// TestTestdata_ReturnedGroupEcho is the echo form of issue #633.
func TestTestdata_ReturnedGroupEcho(t *testing.T) {
	out := loadTestdata(t, "returned_group_echo", spec.DefaultEchoConfig())
	noDanglingRefs(t, out)
	noUnresolvedPlaceholders(t, out)

	assertExactGetPaths(t, out, []string{
		"/ident/x",
		"/direct/x",
		"/public/v/a",
		"/admin/v/b",
		"/kept/same",
	})
}

// TestTestdata_ReturnedGroupMux is the gorilla/mux form of issue #633: a
// PathPrefix(...).Subrouter() built in a helper and returned, both directly
// and through a local.
func TestTestdata_ReturnedGroupMux(t *testing.T) {
	out := loadTestdata(t, "returned_group_mux", spec.DefaultMuxConfig())
	noDanglingRefs(t, out)
	noUnresolvedPlaceholders(t, out)

	assertExactGetPaths(t, out, []string{"/api/items", "/admin/users"})
}

// TestTestdata_ReturnedGroupChi covers the chi form of issue #633, where the
// returned value is a router mounted by the caller: routes the caller registers
// on it keep the mount prefix beside the ones the factory registered itself.
//
// Change detector: a helper that mounts the router it returns (`Sub` mounts
// `s` under "/sub" and returns it) does not yet carry that Mount to the
// caller's registrations, so `s.Get("/x")` is documented at "/x" instead of
// "/sub/x". The Mount consumes the router through its argument, which a
// returned-value binding does not reach; when it does, this becomes "/sub/x".
func TestTestdata_ReturnedGroupChi(t *testing.T) {
	out := loadTestdata(t, "returned_group_chi", spec.DefaultChiConfig())
	noDanglingRefs(t, out)
	noUnresolvedPlaceholders(t, out)

	assertExactGetPaths(t, out, []string{"/api/inside", "/api/outside", "/x"})
}
