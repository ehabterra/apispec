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
	"reflect"
	"strings"
	"testing"

	"github.com/ehabterra/apispec/internal/metadata"
)

// entrypointGraph builds metadata for: main -> reachableCmd, plus two functions
// nothing calls (webCmd registers a route, toolCmd does not).
func entrypointGraph(t *testing.T) *metadata.Metadata {
	t.Helper()
	meta := &metadata.Metadata{StringPool: metadata.NewStringPool()}
	pkg := "example.com/app"

	// Meta must be set on every Call: BaseID() builds its identifier from the
	// string pool through it, and returns "" without it — which silently made
	// every key mismatch in the first version of this test.
	call := func(callerName, calleeName, calleeRecv, calleePkg string) metadata.CallGraphEdge {
		caller := metadata.Call{Name: meta.StringPool.Get(callerName), Pkg: meta.StringPool.Get(pkg), Meta: meta}
		callee := metadata.Call{Name: meta.StringPool.Get(calleeName), Pkg: meta.StringPool.Get(calleePkg), Meta: meta}
		if calleeRecv != "" {
			callee.RecvType = meta.StringPool.Get(calleeRecv)
		}
		return metadata.CallGraphEdge{Caller: caller, Callee: callee}
	}

	meta.CallGraph = []metadata.CallGraphEdge{
		call("main", "reachableCmd", "", pkg),
		// The router registration the gate looks for, on the framework receiver.
		call("webCmd", "Get", "*Mux", "github.com/go-chi/chi/v5"),
		// A same-named call on something that is NOT a router: the gate must not
		// mistake it for a registration.
		call("toolCmd", "Get", "*Cache", "example.com/app/cache"),
		call("reachableCmd", "Get", "*Mux", "github.com/go-chi/chi/v5"),
	}
	meta.Callers = map[string][]*metadata.CallGraphEdge{}
	for i := range meta.CallGraph {
		e := &meta.CallGraph[i]
		meta.Callers[e.Caller.BaseID()] = append(meta.Callers[e.Caller.BaseID()], e)
	}
	return meta
}

// TestEntrypointRoots covers both gates that decide whether a declared entrypoint
// becomes a tree root (issue #220).
func TestEntrypointRoots(t *testing.T) {
	meta := entrypointGraph(t)
	pkg := "example.com/app"
	cfg := &APISpecConfig{Framework: FrameworkConfig{RoutePatterns: []RoutePattern{{
		CallRegex:     `^(?i)(GET|POST)$`,
		RecvTypeRegex: `^github\.com/go-chi/chi(/v\d)?\.\*?(Router|Mux)$`,
	}}}}
	match := RouteRegistrationMatcher(cfg, meta)

	// Keys come from the same place the real candidate set does — meta.Callers,
	// keyed by Call.BaseID — rather than being composed by hand, which is how the
	// first version of this test silently compared two different key formats.
	key := func(fn string) string {
		for k := range meta.Callers {
			if strings.HasSuffix(k, "."+fn) {
				return k
			}
		}
		t.Fatalf("no caller key for %s; have %v", fn, meta.Callers)
		return ""
	}
	candidates := []string{
		key("webCmd"),       // unreachable + registers        -> root
		key("toolCmd"),      // unreachable + registers nothing -> skipped
		key("reachableCmd"), // registers but main reaches it   -> skipped
		pkg + ".missingCmd", // not in the graph at all         -> skipped
	}
	got, stats := entrypointRoots(meta, candidates, match, nil)
	if want := []string{key("webCmd")}; !reflect.DeepEqual(got, want) {
		t.Errorf("entrypointRoots = %v, want %v", got, want)
	}
	// The same numbers the UI reports: every candidate is accounted for in
	// exactly one bucket, so "0 rooted" can be read as a reason.
	want := EntrypointStats{Declared: 4, Rooted: 1, AlreadyReachable: 1, NoRoutes: 2}
	if stats != want {
		t.Errorf("stats = %+v, want %+v", stats, want)
	}

	t.Run("no candidates, no metadata, no matcher", func(t *testing.T) {
		if got, stats := entrypointRoots(meta, nil, match, nil); got != nil || stats != (EntrypointStats{}) {
			t.Errorf("no candidates = (%v, %+v), want (nil, zero)", got, stats)
		}
		if got, stats := entrypointRoots(nil, candidates, match, nil); got != nil || stats != (EntrypointStats{}) {
			t.Errorf("nil metadata = (%v, %+v), want (nil, zero)", got, stats)
		}
	})
}

// TestRouteRegistrationMatcher pins the gate predicate, including the receiver
// check that a name-only test cannot express.
func TestRouteRegistrationMatcher(t *testing.T) {
	meta := entrypointGraph(t)
	edgeByCaller := func(name string) *metadata.CallGraphEdge {
		for i := range meta.CallGraph {
			if getString(meta, meta.CallGraph[i].Caller.Name) == name {
				return &meta.CallGraph[i]
			}
		}
		t.Fatalf("no edge from %s", name)
		return nil
	}

	t.Run("receiver-scoped pattern rejects a same-named non-router call", func(t *testing.T) {
		cfg := &APISpecConfig{Framework: FrameworkConfig{RoutePatterns: []RoutePattern{{
			CallRegex:     `^(?i)(GET|POST)$`,
			RecvTypeRegex: `^github\.com/go-chi/chi(/v\d)?\.\*?(Router|Mux)$`,
		}}}}
		match := RouteRegistrationMatcher(cfg, meta)
		if !match(edgeByCaller("webCmd")) {
			t.Error("chi router Get should match")
		}
		if match(edgeByCaller("toolCmd")) {
			t.Error("cache.Get must NOT count as a route registration")
		}
	})

	t.Run("exact receiver and mount patterns", func(t *testing.T) {
		cfg := &APISpecConfig{Framework: FrameworkConfig{
			RoutePatterns: []RoutePattern{{CallRegex: `^Get$`, RecvType: "example.com/app/cache.*Cache"}},
			MountPatterns: []MountPattern{{CallRegex: `^Mount$`}},
		}}
		match := RouteRegistrationMatcher(cfg, meta)
		if !match(edgeByCaller("toolCmd")) {
			t.Error("an exact RecvType match should count")
		}
	})

	t.Run("unscoped pattern falls back to name-only", func(t *testing.T) {
		cfg := &APISpecConfig{Framework: FrameworkConfig{
			RoutePatterns: []RoutePattern{{CallRegex: `^(?i)GET$`}},
		}}
		match := RouteRegistrationMatcher(cfg, meta)
		if !match(edgeByCaller("toolCmd")) {
			t.Error("with no receiver scope the name alone must admit the edge")
		}
	})

	t.Run("no patterns, nil inputs", func(t *testing.T) {
		if RouteRegistrationMatcher(&APISpecConfig{}, meta)(edgeByCaller("webCmd")) {
			t.Error("a config with no route patterns must match nothing")
		}
		if RouteRegistrationMatcher(nil, meta)(edgeByCaller("webCmd")) {
			t.Error("nil config must match nothing")
		}
		if RouteRegistrationMatcher(&APISpecConfig{}, nil)(edgeByCaller("webCmd")) {
			t.Error("nil metadata must match nothing")
		}
	})
}

// TestApplyEntrypointPresets covers the import-keyed presets, including cobra,
// which has no fixture because the dependency is not vendored — the preset itself
// is still pinned here.
func TestApplyEntrypointPresets(t *testing.T) {
	metaWithImport := func(path string) *metadata.Metadata {
		meta := &metadata.Metadata{StringPool: metadata.NewStringPool()}
		meta.Packages = map[string]*metadata.Package{"p": {Files: map[string]*metadata.File{
			"main.go": {Imports: map[int]int{
				meta.StringPool.Get(path): meta.StringPool.Get(path),
			}},
		}}}
		return meta
	}

	for _, tc := range []struct {
		imp       string
		ownerType string
		field     string
		wantMatch bool
		wantAtAll bool
	}{
		{"github.com/urfave/cli/v3", "github.com/urfave/cli/v3.Command", "Action", true, true},
		{"github.com/urfave/cli/v2", "github.com/urfave/cli/v2.App", "Action", true, true},
		{"github.com/urfave/cli", "github.com/urfave/cli.Command", "Before", true, true},
		{"github.com/spf13/cobra", "github.com/spf13/cobra.Command", "RunE", true, true},
		{"github.com/spf13/cobra", "github.com/spf13/cobra.Command", "SilenceUsage", false, true},
		{"github.com/peterbourgon/ff/v3/ffcli", "github.com/peterbourgon/ff/v3/ffcli.Command", "Exec", true, true},
		// A project that imports none of them gets no patterns at all.
		{"github.com/gin-gonic/gin", "github.com/urfave/cli/v3.Command", "Action", false, false},
	} {
		cfg := &APISpecConfig{}
		ApplyEntrypointPresets(cfg, metaWithImport(tc.imp))
		if got := len(cfg.Framework.EntrypointPatterns) > 0; got != tc.wantAtAll {
			t.Errorf("import %s: patterns present = %v, want %v", tc.imp, got, tc.wantAtAll)
		}
		if got := matchesEntrypoint(cfg.Framework.EntrypointPatterns, tc.ownerType, tc.field); got != tc.wantMatch {
			t.Errorf("import %s: matchesEntrypoint(%s, %s) = %v, want %v",
				tc.imp, tc.ownerType, tc.field, got, tc.wantMatch)
		}
	}

	t.Run("idempotent, and user patterns keep precedence", func(t *testing.T) {
		cfg := &APISpecConfig{Framework: FrameworkConfig{EntrypointPatterns: []EntrypointPattern{{
			FieldRegex: `^Handle$`, RecvTypeRegex: `^example\.com/house\.Cmd$`,
		}}}}
		meta := metaWithImport("github.com/urfave/cli/v3")
		ApplyEntrypointPresets(cfg, meta)
		first := len(cfg.Framework.EntrypointPatterns)
		ApplyEntrypointPresets(cfg, meta)
		if got := len(cfg.Framework.EntrypointPatterns); got != first {
			t.Errorf("applying twice grew the list %d -> %d", first, got)
		}
		if cfg.Framework.EntrypointPatterns[0].FieldRegex != `^Handle$` {
			t.Error("the user's pattern must stay first")
		}
		if !matchesEntrypoint(cfg.Framework.EntrypointPatterns, "example.com/house.Cmd", "Handle") {
			t.Error("the user's own house dispatcher must still match")
		}
	})

	t.Run("nil config and metadata without imports", func(t *testing.T) {
		ApplyEntrypointPresets(nil, metaWithImport("github.com/urfave/cli/v3")) // must not panic
		cfg := &APISpecConfig{}
		ApplyEntrypointPresets(cfg, &metadata.Metadata{StringPool: metadata.NewStringPool()})
		if len(cfg.Framework.EntrypointPatterns) != 0 {
			t.Errorf("no imports should yield no patterns, got %v", cfg.Framework.EntrypointPatterns)
		}
	})
}

// TestMatchesEntrypoint covers the guard that an unconstrained pattern is a
// misconfiguration rather than a wildcard.
func TestMatchesEntrypoint(t *testing.T) {
	patterns := []EntrypointPattern{{FieldRegex: `^Action$`}} // no owner constraint
	if matchesEntrypoint(patterns, "anything.Command", "Action") {
		t.Error("a pattern with no owner constraint must not match anything")
	}
	for _, tc := range []struct{ owner, field string }{
		{"", "Action"},
		{"x.Command", ""},
	} {
		if matchesEntrypoint([]EntrypointPattern{{FieldRegex: `^Action$`, RecvType: "x.Command"}}, tc.owner, tc.field) {
			t.Errorf("empty owner/field must not match (%q, %q)", tc.owner, tc.field)
		}
	}
	if matchesEntrypoint(nil, "x.Command", "Action") {
		t.Error("no patterns must match nothing")
	}
}
