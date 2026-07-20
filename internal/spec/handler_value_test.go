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
	"slices"
	"testing"

	"github.com/ehabterra/apispec/internal/metadata"
)

func TestSplitTypeKey(t *testing.T) {
	for _, tc := range []struct {
		in, pkg, name string
		ok            bool
	}{
		// Package paths contain dots, so the split is on the LAST dot.
		{"net/http.Handler", "net/http", "Handler", true},
		{"github.com/acme/svc/internal/api.Handler", "github.com/acme/svc/internal/api", "Handler", true},
		{"app.H", "app", "H", true},
		{"Handler", "", "", false},
		{"", "", "", false},
	} {
		t.Run(tc.in, func(t *testing.T) {
			pkg, name, ok := splitTypeKey(tc.in)
			if pkg != tc.pkg || name != tc.name || ok != tc.ok {
				t.Errorf("splitTypeKey(%q) = (%q, %q, %v), want (%q, %q, %v)",
					tc.in, pkg, name, ok, tc.pkg, tc.name, tc.ok)
			}
		})
	}
}

func TestHandlerValueTypeOf(t *testing.T) {
	meta := &metadata.Metadata{StringPool: metadata.NewStringPool()}

	named := metadata.NewCallArgument(meta)
	named.SetKind(metadata.KindIdent)
	named.SetName("h")
	named.SetType("*app.H")

	// A method value's type is a signature, not a named type: those resolve via
	// the method-value paths, so this must decline them.
	fn := metadata.NewCallArgument(meta)
	fn.SetKind(metadata.KindSelector)
	fn.SetType("func(w net/http.ResponseWriter, r *net/http.Request)")

	untyped := metadata.NewCallArgument(meta)
	untyped.SetKind(metadata.KindIdent)
	untyped.SetName("x")

	for _, tc := range []struct {
		name       string
		arg        *metadata.CallArgument
		pkg, tname string
	}{
		{"named pointer type", named, "app", "H"},
		{"func signature declines", fn, "", ""},
		{"untyped declines", untyped, "", ""},
		{"nil declines", nil, "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pkg, name := handlerValueTypeOf(tc.arg)
			if pkg != tc.pkg || name != tc.tname {
				t.Errorf("got (%q, %q), want (%q, %q)", pkg, name, tc.pkg, tc.tname)
			}
		})
	}
}

// TestHandlerMethodKeys covers the shared resolution both tracker engines use,
// including the guards that keep it honest: a type contributes a key only for a
// handler method it actually declares (golden rules #7/#9).
func TestHandlerMethodKeys(t *testing.T) {
	meta := &metadata.Metadata{StringPool: metadata.NewStringPool()}
	h := &metadata.Type{
		Name: meta.StringPool.Get("H"),
		Methods: []metadata.Method{
			{Name: meta.StringPool.Get("ServeHTTP"), Receiver: meta.StringPool.Get("*H")},
			{Name: meta.StringPool.Get("Other"), Receiver: meta.StringPool.Get("*H")},
		},
	}
	// Declares no handler method at all.
	plain := &metadata.Type{
		Name:    meta.StringPool.Get("Plain"),
		Methods: []metadata.Method{{Name: meta.StringPool.Get("Nope")}},
	}
	// A LOCAL interface, which carries ImplementedBy directly.
	local := &metadata.Type{
		Name:          meta.StringPool.Get("Local"),
		Kind:          meta.StringPool.Get("interface"),
		ImplementedBy: []int{meta.StringPool.Get("app.H")},
	}
	types := map[string]*metadata.Type{"H": h, "Plain": plain, "Local": local}
	meta.Packages = map[string]*metadata.Package{
		"app": {Types: types, Files: map[string]*metadata.File{"app.go": {Types: types}}},
	}

	methods := []string{"ServeHTTP"}
	for _, tc := range []struct {
		name, pkg, tname string
		methods          []string
		want             []string
	}{
		{"concrete type declaring the method", "app", "H", methods, []string{"app.H.ServeHTTP"}},
		{"type declaring no handler method", "app", "Plain", methods, nil},
		{"local interface fans out to implementers", "app", "Local", methods, []string{"app.H.ServeHTTP"}},
		{"no configured methods", "app", "H", nil, nil},
		{"unknown type", "app", "Missing", methods, nil},
		{"empty package", "", "H", methods, nil},
		{"empty name", "app", "", methods, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := handlerMethodKeys(meta, tc.methods, tc.pkg, tc.tname)
			if !slices.Equal(got, tc.want) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}

	if got := handlerMethodKeys(nil, methods, "app", "H"); got != nil {
		t.Errorf("nil metadata: got %v, want nil", got)
	}
}
