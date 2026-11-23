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
// group nested inside the helper, one helper called twice from one parent (one
// group, both routes) and from two parents (each caller's route under its own
// parent only), and a parameter forwarded through one and two helpers before it
// is returned, or grouped on through a local (each call resolves to its own
// argument). The negatives: a fresh engine that nothing mounts stays
// unprefixed, a helper returning its parameter keeps the caller's prefix once,
// not twice, a helper whose returned local is reassigned gets no prefix, a
// recursive helper keeps its route, a helper that reassigns the parameter it
// returns, or groups on, gets no prefix, and two calls whose returned groups
// collapse into one node, in either statement order, or are moved copies that
// read their suffix from no one call, or whose local holds a parameter their
// callers disagree on, get no prefix rather than each other's. A helper-local
// group handed back through another helper stays per invocation.
func TestTestdata_ReturnedGroup(t *testing.T) {
	out := loadTestdata(t, "returned_group", spec.DefaultGinConfig())
	noDanglingRefs(t, out)
	noUnresolvedPlaceholders(t, out)

	assertExactGetPaths(t, out, []string{
		"/ident/x",
		"/ident/again",
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
		// The returned local holds /first or /second depending on the path:
		// the same ambiguity as /pick, written as a reassignment.
		"/reassigned",
		// Rec returns its own call. Following it bound the route beneath a
		// recursive copy of Rec that expansion never builds, dropping it.
		"/rec",
		"/wpub/w1",
		"/wadm/w2",
		"/wtwo/w3",
		"/vx/via/one",
		// Suffixed returns parent.Group(suffix). Both calls pass /vp, so the
		// parameter binding moves the Group under /vp once for the two of
		// them, and that node carries neither "/a" nor "/b". Binding the
		// routes there documented each under both suffixes; no invocation
		// owns the node, so they stay where they were before #633.
		"/sa",
		"/sb",
		// The same shape with the first returned group passed on to RegO
		// before the second Suffixed call. RegO's claim was resolved while
		// only one call site had moved the Group, so it took the node as
		// unshared and documented /mk/{suffix}/o.
		"/o",
		"/p",
		// Swap and SwapRet assign `parent = other` on one path before
		// returning it (through Identity, or directly): the returned value is
		// the caller's argument only on the other path, so /sp is not the
		// prefix. Both were documented under /sp.
		"/swap",
		"/swapret",
		// ViaTwice groups on `g := Identity(parent)`. The claim on g is
		// resolved once for the function, and its two callers pass /ty and
		// /tz: tracing parent followed the first caller and put both routes
		// under /ty. With no one producer they stay unprefixed.
		"/t1",
		"/t2",
		// LocalWrap and LocalVersion return a helper's result for a group of
		// their own local. Each invocation builds its own copy of that local's
		// Group, so each caller's route belongs beneath its own copy only;
		// resolved without the invocation, both copies took both routes.
		"/lpub/lv/l1",
		"/ladm/lv/l2",
		"/lpub/lw/v/m1",
		"/ladm/lw/v/m2",
		// SwapGroup assigns `parent = other` on one path before grouping on
		// it: the Group runs on /sp or on /so, so neither is the prefix. It
		// was documented under /sp/leaf.
		"/swapgroup",
		// Branch's Group is moved under /bp by both calls, once named by bp
		// and once by Identity(bp): one node carrying neither suffix, which
		// documented /a/ba, /b/ba, /b/bb and /bp/{suffix}/ba.
		"/ba",
		"/bb",
		// Area's calls move the Group under distinct parents, but each moved
		// copy reads the suffix from neither call: /ap/{suffix}/aa beside
		// /a/aa. No invocation's place is known, so no prefix.
		"/aa",
		"/ab",
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
