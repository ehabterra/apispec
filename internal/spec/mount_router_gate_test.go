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

// TestMountRouterArgTypeGate covers the discriminator that makes cross-framework
// mounts possible (issue #138). `mux.Handle(path, x)` registers a mount when x
// is a router and an ordinary route when x is a handler — the call name is
// identical, so only the argument type separates them. Without this gate the
// mount pattern swallowed every handler-value route.
func TestMountRouterArgTypeGate(t *testing.T) {
	meta := exSweepMeta()
	const routerRe = `^\*?(github\.com/go-chi/chi(/v\d)?\.(Mux|Router)|net/http\.ServeMux)$`

	typedIdent := func(name, typ string) *metadata.CallArgument {
		a := sweepIdent(meta, name)
		a.SetType(typ)
		return a
	}
	// stripPrefixCall models http.StripPrefix("/api", inner): its own type is
	// net/http.Handler, which says nothing — the router is the inner argument.
	stripPrefixCall := func(inner *metadata.CallArgument) *metadata.CallArgument {
		c := metadata.NewCallArgument(meta)
		c.SetKind(metadata.KindCall)
		c.SetType("net/http.Handler")
		c.Args = []*metadata.CallArgument{sweepLit(meta, `"/api"`), inner}
		return c
	}

	pattern := MountPattern{
		CallRegex:          `^Handle$`,
		PathFromArg:        true,
		RouterFromArg:      true,
		PathArgIndex:       0,
		RouterArgIndex:     1,
		IsMount:            true,
		RouterArgTypeRegex: routerRe,
	}
	m := NewMountPatternMatcher(pattern, &APISpecConfig{}, NewContextProvider(meta), nil)

	for _, tc := range []struct {
		name string
		arg  *metadata.CallArgument
		want bool
	}{
		{"chi router value is a mount", typedIdent("api", "*github.com/go-chi/chi/v5.Mux"), true},
		{"nested ServeMux is a mount", typedIdent("sub", "*net/http.ServeMux"), true},
		{"router behind StripPrefix is a mount", stripPrefixCall(typedIdent("api", "*github.com/go-chi/chi/v5.Mux")), true},
		{"plain handler value is NOT a mount", typedIdent("h", "*app.statusHandler"), false},
		{"handler behind StripPrefix is NOT a mount", stripPrefixCall(typedIdent("h", "*app.statusHandler")), false},
		{"untyped argument is NOT a mount", sweepIdent(meta, "x"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			edge := sweepEdge(meta, "main", "app", "Handle", "net/http", "*ServeMux", "", sweepLit(meta, `"/api/"`), tc.arg)
			if got := m.routerArgIsRouter(edge); got != tc.want {
				t.Errorf("routerArgIsRouter = %v, want %v", got, tc.want)
			}
		})
	}

	t.Run("missing router argument", func(t *testing.T) {
		edge := sweepEdge(meta, "main", "app", "Handle", "net/http", "*ServeMux", "", sweepLit(meta, `"/api/"`))
		if m.routerArgIsRouter(edge) {
			t.Error("an edge with no router argument must not match")
		}
	})

	t.Run("invalid regex does not match", func(t *testing.T) {
		bad := NewMountPatternMatcher(
			MountPattern{RouterArgIndex: 1, RouterArgTypeRegex: `^([`},
			&APISpecConfig{}, NewContextProvider(meta), nil)
		edge := sweepEdge(meta, "main", "app", "Handle", "net/http", "*ServeMux", "",
			sweepLit(meta, `"/api/"`), typedIdent("api", "*github.com/go-chi/chi/v5.Mux"))
		if bad.routerArgIsRouter(edge) {
			t.Error("an uncompilable regex must not match rather than match everything")
		}
	})

	// Without the gate the pattern is unchanged: every Handle looks like a
	// mount, which is the behaviour the other frameworks' patterns rely on.
	t.Run("no gate means no discrimination", func(t *testing.T) {
		ungated := NewMountPatternMatcher(
			MountPattern{CallRegex: `^Mount$`, RouterArgIndex: 1, IsMount: true},
			&APISpecConfig{}, NewContextProvider(meta), nil)
		if ungated.pattern.RouterArgTypeRegex != "" {
			t.Fatal("fixture error: expected an ungated pattern")
		}
	})
}
