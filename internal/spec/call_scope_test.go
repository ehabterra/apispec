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

// scopeEdge builds a call from a method on api.Registrar in package
// example.com/app/internal/api to a method on chi.Router.
func scopeEdge(meta *metadata.Metadata, callerPkg, callerRecv string) *metadata.CallGraphEdge {
	return &metadata.CallGraphEdge{
		Caller:   sweepCall(meta, "Register", callerPkg, callerRecv, ""),
		Callee:   sweepCall(meta, "Get", "github.com/go-chi/chi/v5", "*Mux", ""),
		Position: meta.StringPool.Get(""),
	}
}

// TestMatchCallScope covers the four caller/callee filters, which every pattern
// type declared and no matcher read — a user set one and got no filtering, no
// warning and no way to tell (issue #238).
func TestMatchCallScope(t *testing.T) {
	meta := exSweepMeta()
	base := NewBasePatternMatcher(&APISpecConfig{}, NewContextProvider(meta), nil)
	edge := scopeEdge(meta, "example.com/app/internal/api", "*Registrar")

	tests := []struct {
		name  string
		scope callScope
		want  bool
		why   string
	}{
		{
			name:  "no filters admit everything",
			scope: callScope{},
			want:  true,
			why:   "an unset filter must not narrow anything — every built-in config leaves all four empty",
		},
		{
			name:  "caller package admitted",
			scope: callScope{callerPkg: []string{`/internal/api$`}},
			want:  true,
		},
		{
			name:  "caller package rejected",
			scope: callScope{callerPkg: []string{`/internal/debugroutes$`}},
			want:  false,
			why:   "the same call made from another package is what these filters exist to separate",
		},
		{
			name:  "caller receiver type admitted",
			scope: callScope{callerRecvType: []string{`^example\.com/app/internal/api\.\*Registrar$`}},
			want:  true,
		},
		{
			name:  "caller receiver type rejected",
			scope: callScope{callerRecvType: []string{`\.\*Router$`}},
			want:  false,
		},
		{
			name:  "callee package admitted",
			scope: callScope{calleePkg: []string{`^github\.com/go-chi/chi`}},
			want:  true,
		},
		{
			name:  "callee package rejected",
			scope: callScope{calleePkg: []string{`^github\.com/gin-gonic/gin$`}},
			want:  false,
		},
		{
			name:  "callee receiver type admitted",
			scope: callScope{calleeRecvType: []string{`^github\.com/go-chi/chi/v5\.\*Mux$`}},
			want:  true,
		},
		{
			name:  "callee receiver type rejected",
			scope: callScope{calleeRecvType: []string{`\.\*Engine$`}},
			want:  false,
		},
		{
			name:  "alternatives are ORed",
			scope: callScope{callerPkg: []string{`/nowhere$`, `/internal/api$`}},
			want:  true,
			why:   "a list is a set of alternatives; requiring all of them would make two entries unsatisfiable",
		},
		{
			name:  "lists are ANDed across the four fields",
			scope: callScope{callerPkg: []string{`/internal/api$`}, calleePkg: []string{`gin`}},
			want:  false,
			why:   "each field that IS set must hold, or a caller filter could be undone by a callee filter",
		},
		{
			name:  "an uncompilable regex matches nothing",
			scope: callScope{callerPkg: []string{`([`}},
			want:  false,
			why:   "a typo must narrow a filter to nothing visible rather than silently widen it to everything",
		},
		{
			name:  "an empty string in a list is not a wildcard",
			scope: callScope{callerPkg: []string{""}},
			want:  false,
			why:   "an empty entry is a mistake; treating it as a match would admit everything the list means to exclude",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := base.matchCallScope(edge, tt.scope); got != tt.want {
				t.Errorf("matchCallScope() = %v, want %v — %s", got, tt.want, tt.why)
			}
		})
	}

	t.Run("a filtered pattern cannot be satisfied without a call", func(t *testing.T) {
		if base.matchCallScope(nil, callScope{callerPkg: []string{`.*`}}) {
			t.Error("a node with no edge matched a caller filter; there is no call to ask where it was made")
		}
		if !base.matchCallScope(nil, callScope{}) {
			t.Error("a node with no edge was rejected by an EMPTY scope, which constrains nothing")
		}
	})

	t.Run("a receiver-less function is addressed by its package", func(t *testing.T) {
		// Same convention as recvTypeRegex, which is why chi's render patterns can
		// scope a package-level responder at all.
		plain := &metadata.CallGraphEdge{
			Caller:   sweepCall(meta, "Register", "example.com/app/routes", "", ""),
			Callee:   sweepCall(meta, "JSON", "github.com/go-chi/render", "", ""),
			Position: meta.StringPool.Get(""),
		}
		if !base.matchCallScope(plain, callScope{calleeRecvType: []string{`^github\.com/go-chi/render$`}}) {
			t.Error("a package-level function did not match its package as an owner")
		}
		if !base.matchCallScope(plain, callScope{callerRecvType: []string{`^example\.com/app/routes$`}}) {
			t.Error("a plain caller function did not match its package as an owner")
		}
	})
}

// TestCallScopeIsAppliedByEveryMatcher pins the wiring rather than the rule: the
// filters are declared on six pattern types, and one matcher left unwired is
// exactly the silent no-op this issue is about.
func TestCallScopeIsAppliedByEveryMatcher(t *testing.T) {
	meta := exSweepMeta()
	cp := NewContextProvider(meta)
	cfg := &APISpecConfig{}
	edge := scopeEdge(meta, "example.com/app/internal/api", "*Registrar")
	node := sweepNode(edge)

	// Matches the caller, so each matcher must accept; then a filter naming a
	// different package, so each must reject.
	const admits, rejects = `/internal/api$`, `/internal/debugroutes$`

	matchers := map[string]func(filter string) bool{
		"route": func(f string) bool {
			return NewRoutePatternMatcher(RoutePattern{CallerPkgPatterns: []string{f}}, cfg, cp, nil).MatchNode(node)
		},
		"requestBody": func(f string) bool {
			return NewRequestPatternMatcher(RequestBodyPattern{CallerPkgPatterns: []string{f}}, cfg, cp, nil).MatchNode(node)
		},
		"response": func(f string) bool {
			return NewResponsePatternMatcher(ResponsePattern{CallerPkgPatterns: []string{f}}, cfg, cp, nil).MatchNode(node)
		},
		"param": func(f string) bool {
			return NewParamPatternMatcher(ParamPattern{CallerPkgPatterns: []string{f}}, cfg, cp, nil).MatchNode(node)
		},
		"mount": func(f string) bool {
			// IsMount is what a mount pattern returns when everything else passes.
			return NewMountPatternMatcher(MountPattern{CallerPkgPatterns: []string{f}, IsMount: true}, cfg, cp, nil).MatchNode(node)
		},
		"security": func(f string) bool {
			return NewSecurityPatternMatcher(SecurityPattern{CallerPkgPatterns: []string{f}}, cfg, cp, nil).MatchNode(node)
		},
	}

	for kind, match := range matchers {
		t.Run(kind, func(t *testing.T) {
			if !match(admits) {
				t.Errorf("%sPatterns: the filter names the caller's own package and still did not match", kind)
			}
			if match(rejects) {
				t.Errorf("%sPatterns: callerPkgPatterns is not read — the filter names another package and the call matched anyway", kind)
			}
		})
	}
}
