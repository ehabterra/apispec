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
	"regexp"
	"sort"

	"github.com/ehabterra/apispec/internal/metadata"
)

// detectForwardingMounts derives mount patterns for a project router's own
// Mount-style methods: an in-module method whose call to a framework mount
// forwards its own path parameter and its own sub-router parameter (issue #611).
//
//	func (r *Router) Mount(pattern string, sub *Router) {
//		r.chiRouter.Mount(r.getPattern(pattern), sub.chiRouter)
//	}
//	r.Mount("/api/v1", apiv1.Routes())
//
// The framework mount inside the wrapper can never be documented correctly on
// its own. Its path is whatever the wrapper made of the parameter — here a
// method call on the receiver, which renders as `/{path}` — and the router it
// mounts is a field of a parameter, which has no producer in the wrapper's body.
// Both are only known at the CALL SITE, and there the sub-router's routes hang
// under the router argument, so the prefix reaches them by tree containment the
// way it does for a framework mount written in place. This is the mount half of
// what DetectRouterWrappers does for registrations (#235).
//
// The router role is accepted only when the argument IS the parameter or a field
// reached from it (`sub`, `sub.chiRouter`, `&sub`): then the mounted router is
// part of the value the caller passed, and lives in that value's producer
// subtree. A router BUILT from a parameter (`buildRouter(cfg)`) is a new value
// the caller never produced, and mounting the caller's argument would prefix the
// wrong routes, so such a method is reported incomplete and left unapplied.
//
// Derivation is transitive, like the route half: a wrapper may forward into
// another wrapper's Mount before reaching the framework.
func detectForwardingMounts(meta *metadata.Metadata, cp ContextProvider, params *paramIndex, cfg *APISpecConfig) []DetectedWrapper {
	seeds := make([]MountPattern, 0, len(cfg.Framework.MountPatterns))
	for _, p := range cfg.Framework.MountPatterns {
		// Only a pattern that names a router argument mounts a sub-router;
		// a prefix-only grouping call has nothing to forward.
		if p.IsMount && p.RouterArgIndex >= 0 && (p.RouterFromArg || p.RouterArgIndex > 0) {
			seeds = append(seeds, p)
		}
	}

	found := map[string]DetectedWrapper{}
	for round := 0; round < wrapperDetectRounds && len(seeds) > 0; round++ {
		matchers := make([]*MountPatternMatcherImpl, 0, len(seeds))
		for _, p := range seeds {
			matchers = append(matchers, NewMountPatternMatcher(p, &APISpecConfig{}, cp))
		}
		var derived []MountPattern
		for i := range meta.CallGraph {
			edge := &meta.CallGraph[i]
			caller := wrapperMethodOf(meta, cp, edge)
			if caller == nil {
				continue
			}
			node := newEdgeNode(edge)
			for j, matcher := range matchers {
				if !matcher.MatchNode(node) {
					continue
				}
				w := deriveForwardingMount(cp, params, caller, seeds[j], edge)
				key := w.RecvType + "." + w.Methods[0]
				if _, seen := found[key]; !seen {
					found[key] = w
					if w.Complete {
						derived = append(derived, *w.Mount)
					}
				}
				break // one pattern per edge: the first match is the framework's own
			}
		}
		seeds = derived
	}

	out := make([]DetectedWrapper, 0, len(found))
	for _, w := range found {
		out = append(out, w)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].RecvType != out[j].RecvType {
			return out[i].RecvType < out[j].RecvType
		}
		return out[i].Methods[0] < out[j].Methods[0]
	})
	return out
}

// deriveForwardingMount expresses one framework mount call's path and router
// roles in terms of the enclosing method's own parameters.
func deriveForwardingMount(cp ContextProvider, params *paramIndex, w *wrapperMethod, inner MountPattern, edge *metadata.CallGraphEdge) DetectedWrapper {
	mount := &MountPattern{
		CallRegex:      "^" + regexp.QuoteMeta(w.name) + "$",
		RecvTypeRegex:  recvTypeRegexFor(w),
		IsMount:        true,
		RouterFromArg:  true,
		RouterArgIndex: -1,
	}
	out := DetectedWrapper{
		RecvType: w.fqRecv(),
		Methods:  []string{w.name},
		Via:      cp.GetString(edge.Callee.Pkg) + "." + cp.GetString(edge.Callee.Name),
		Mount:    mount,
	}
	arg := func(idx int) *metadata.CallArgument {
		if idx < 0 || idx >= len(edge.Args) {
			return nil
		}
		return edge.Args[idx]
	}

	pathOK := !inner.PathFromArg
	if inner.PathFromArg {
		if i, ok := params.indexOf(w, arg(inner.PathArgIndex)); ok {
			mount.PathFromArg, mount.PathArgIndex = true, i
			pathOK = true
		}
	}
	routerOK := false
	if name := valueRootIdent(arg(inner.RouterArgIndex)); name != "" {
		for i, param := range params.paramsOf(w) {
			if param == name {
				mount.RouterArgIndex = i
				routerOK = true
				break
			}
		}
	}
	// A path-only wrapper is a group, which detectGroupMounts owns; without the
	// router there is no sub-router to place under the prefix.
	out.Complete = pathOK && routerOK
	return out
}

// valueRootIdent returns the identifier an expression is a part of — the
// variable itself, or the variable a field selector chain, address-of or
// dereference starts from — and "" for anything that computes a new value.
func valueRootIdent(arg *metadata.CallArgument) string {
	for depth := 0; arg != nil && depth < wrapperExprDepth; depth++ {
		switch arg.GetKind() {
		case metadata.KindIdent:
			return arg.GetName()
		case metadata.KindSelector, metadata.KindUnary, metadata.KindStar, metadata.KindParen:
			arg = arg.X
		default:
			return ""
		}
	}
	return ""
}
