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

import "github.com/ehabterra/apispec/internal/metadata"

// callScope is the caller/callee filter set every pattern type declares, and
// which nothing used to read: four lists of regexes narrowing a pattern by where
// a call is MADE, not only by what is called (issue #238).
//
// The callee side overlaps with RecvTypeRegex on purpose — it is the list form of
// the same question, so several unrelated owners can be admitted without writing
// one alternation. The caller side is the capability that had no other
// expression: `callRegex: ^Get$` scoped to callers in `.../internal/api` documents
// the routes that package registers and leaves an identical registration
// elsewhere alone.
//
// Semantics, uniform across all four:
//
//   - An empty list is no constraint.
//   - A non-empty list admits the call when ANY of its regexes matches, so the
//     lists are alternatives rather than a conjunction.
//   - They are include filters. There is no way to say "everything except", and
//     RE2 has no negative lookahead, so a pattern that must avoid one caller is
//     expressed by admitting the ones it wants.
//   - Regexes are unanchored, like every other regex in the config; anchor with
//     ^…$ when a prefix match would be wrong.
type callScope struct {
	callerPkg      []string
	callerRecvType []string
	calleePkg      []string
	calleeRecvType []string
}

// empty reports whether the scope constrains nothing, which is the overwhelmingly
// common case — every built-in framework config leaves all four unset.
func (s callScope) empty() bool {
	return len(s.callerPkg) == 0 && len(s.callerRecvType) == 0 &&
		len(s.calleePkg) == 0 && len(s.calleeRecvType) == 0
}

// matchCallScope reports whether an edge satisfies a pattern's caller/callee
// filters.
//
// A pattern that declares a filter cannot be satisfied by a node with no edge:
// there is no call to ask where it was made, and admitting it would make the
// filter mean "sometimes".
func (b *BasePatternMatcher) matchCallScope(edge *metadata.CallGraphEdge, s callScope) bool {
	if s.empty() {
		return true
	}
	if edge == nil {
		return false
	}
	cp := b.contextProvider
	return matchesAny(s.callerPkg, cp.GetString(edge.Caller.Pkg)) &&
		matchesAny(s.callerRecvType, fqOwner(cp, edge.Caller.Pkg, edge.Caller.RecvType)) &&
		matchesAny(s.calleePkg, cp.GetString(edge.Callee.Pkg)) &&
		matchesAny(s.calleeRecvType, fqOwner(cp, edge.Callee.Pkg, edge.Callee.RecvType))
}

// matchesAny reports whether value matches at least one of the patterns, and
// reports true for an empty list (no constraint).
//
// A pattern that fails to compile matches nothing rather than everything: a typo
// should narrow a filter to nothing visible, not silently widen it.
func matchesAny(patterns []string, value string) bool {
	if len(patterns) == 0 {
		return true
	}
	for _, pattern := range patterns {
		if pattern == "" {
			continue
		}
		if re, err := cachedRegex(pattern); err == nil && re.MatchString(value) {
			return true
		}
	}
	return false
}

// fqOwner renders what a function belongs to, the way RecvTypeRegex has always
// been matched: `pkg.*Type` for a method, and the package path alone for a plain
// function, so a package-level responder (`render.JSON`) is addressable by the
// same field as a method on a router.
func fqOwner(cp ContextProvider, pkgIdx, recvTypeIdx int) string {
	pkg := cp.GetString(pkgIdx)
	recvType := cp.GetString(recvTypeIdx)
	switch {
	case pkg != "" && recvType != "":
		return pkg + "." + recvType
	case recvType != "":
		return recvType
	default:
		return pkg
	}
}

// scope returns the caller/callee filters declared on a route pattern.
func (p *RoutePattern) scope() callScope {
	return callScope{p.CallerPkgPatterns, p.CallerRecvTypePatterns, p.CalleePkgPatterns, p.CalleeRecvTypePatterns}
}

// scope returns the caller/callee filters declared on a request-body pattern.
func (p *RequestBodyPattern) scope() callScope {
	return callScope{p.CallerPkgPatterns, p.CallerRecvTypePatterns, p.CalleePkgPatterns, p.CalleeRecvTypePatterns}
}

// scope returns the caller/callee filters declared on a response pattern.
func (p *ResponsePattern) scope() callScope {
	return callScope{p.CallerPkgPatterns, p.CallerRecvTypePatterns, p.CalleePkgPatterns, p.CalleeRecvTypePatterns}
}

// scope returns the caller/callee filters declared on a parameter pattern.
func (p *ParamPattern) scope() callScope {
	return callScope{p.CallerPkgPatterns, p.CallerRecvTypePatterns, p.CalleePkgPatterns, p.CalleeRecvTypePatterns}
}

// scope returns the caller/callee filters declared on a mount pattern.
func (p *MountPattern) scope() callScope {
	return callScope{p.CallerPkgPatterns, p.CallerRecvTypePatterns, p.CalleePkgPatterns, p.CalleeRecvTypePatterns}
}

// scope returns the caller/callee filters declared on a security pattern.
func (p *SecurityPattern) scope() callScope {
	return callScope{p.CallerPkgPatterns, p.CallerRecvTypePatterns, p.CalleePkgPatterns, p.CalleeRecvTypePatterns}
}
