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
	"testing"

	"github.com/ehabterra/apispec/internal/metadata"
)

// mountWrapperGraph builds a house router whose Mount forwards a FIELD of its
// sub-router parameter to the framework mount (issue #611), a second type that
// forwards into that Mount (the transitive shape), and the two shapes that must
// not be derived.
func mountWrapperGraph(t *testing.T) *metadata.Metadata {
	t.Helper()
	pool := metadata.NewStringPool()
	m := &metadata.Metadata{StringPool: pool, CurrentModulePath: "example.com/app"}

	newArg := func(kind string) *metadata.CallArgument {
		a := metadata.NewCallArgument(m)
		a.SetKind(kind)
		return a
	}
	ident := func(name, typ string) *metadata.CallArgument {
		a := newArg(metadata.KindIdent)
		a.SetName(name)
		if typ != "" {
			a.SetType(typ)
		}
		return a
	}
	selector := func(x *metadata.CallArgument, sel string) *metadata.CallArgument {
		a := newArg(metadata.KindSelector)
		a.X = x
		a.Sel = ident(sel, "")
		return a
	}
	callOf := func(fun *metadata.CallArgument, args ...*metadata.CallArgument) *metadata.CallArgument {
		a := newArg(metadata.KindCall)
		a.Fun = fun
		a.Args = args
		return a
	}
	literal := func(value string) *metadata.CallArgument {
		a := newArg(metadata.KindLiteral)
		a.SetValue(value)
		return a
	}
	sig := func(params ...*metadata.CallArgument) metadata.CallArgument {
		s := newArg(metadata.KindFuncType)
		s.Args = params
		return *s
	}
	call := func(name, pkg, recv string) metadata.Call {
		c := metadata.Call{Meta: m, Name: pool.Get(name), Pkg: pool.Get(pkg), Position: -1, Scope: -1, SignatureStr: -1, RecvType: -1}
		if recv != "" {
			c.RecvType = pool.Get(recv)
		}
		return c
	}

	m.Packages = map[string]*metadata.Package{
		"example.com/app": {Files: map[string]*metadata.File{
			"router.go": {Types: map[string]*metadata.Type{
				"Router": {Name: pool.Get("Router"), Methods: []metadata.Method{
					{Name: pool.Get("Mount"), Signature: sig(ident("pattern", "string"), ident("sub", "*example.com/app.Router"))},
					{Name: pool.Get("MountBuilt"), Signature: sig(ident("pattern", "string"), ident("cfg", "Config"))},
					{Name: pool.Get("MountAPI"), Signature: sig(ident("sub", "*example.com/app.Router"))},
				}},
				"Outer": {Name: pool.Get("Outer"), Methods: []metadata.Method{
					{Name: pool.Get("Attach"), Signature: sig(ident("p", "string"), ident("s", "*example.com/app.Router"))},
				}},
			}},
		}},
	}

	m.CallGraph = []metadata.CallGraphEdge{
		// r.chiRouter.Mount(r.getPattern(pattern), sub.chiRouter): the path is
		// built from the parameter, the router is a field OF the parameter.
		{
			Caller: call("Mount", "example.com/app", "*Router"),
			Callee: call("Mount", "github.com/go-chi/chi/v5", "*Mux"),
			Args: []*metadata.CallArgument{
				callOf(selector(ident("r", ""), "getPattern"), ident("pattern", "string")),
				selector(ident("sub", "*example.com/app.Router"), "chiRouter"),
			},
		},
		// r.chiRouter.Mount(pattern, buildRouter(cfg)): the router is a NEW
		// value computed from a parameter, not part of what the caller passed.
		{
			Caller: call("MountBuilt", "example.com/app", "*Router"),
			Callee: call("Mount", "github.com/go-chi/chi/v5", "*Mux"),
			Args: []*metadata.CallArgument{
				ident("pattern", "string"),
				callOf(ident("buildRouter", ""), ident("cfg", "Config")),
			},
		},
		// r.chiRouter.Mount("/api", sub.chiRouter): a fixed prefix is a mount
		// performed once, not a way of mounting.
		{
			Caller: call("MountAPI", "example.com/app", "*Router"),
			Callee: call("Mount", "github.com/go-chi/chi/v5", "*Mux"),
			Args: []*metadata.CallArgument{
				literal(`"/api"`),
				selector(ident("sub", "*example.com/app.Router"), "chiRouter"),
			},
		},
		// o.inner.Mount(p, s): a wrapper of the wrapper, derivable only once
		// Router.Mount is known.
		{
			Caller: call("Attach", "example.com/app", "*Outer"),
			Callee: call("Mount", "example.com/app", "*Router"),
			Args:   []*metadata.CallArgument{ident("p", "string"), ident("s", "*example.com/app.Router")},
		},
		// A plain function forwarding both: there is no type to scope it to,
		// and #275's helper binding already covers it.
		{
			Caller: call("mountHelper", "example.com/app", ""),
			Callee: call("Mount", "github.com/go-chi/chi/v5", "*Mux"),
			Args:   []*metadata.CallArgument{ident("pattern", "string"), ident("sub", "")},
		},
	}
	m.BuildCallGraphMaps()
	return m
}

// TestDetectForwardingMounts pins the mount half of wrapper derivation: a
// method whose framework mount forwards its own prefix and its own sub-router
// (or a field of it) becomes a mount pattern at its call sites, transitively,
// while a router built from a parameter or a fixed prefix is never applied.
func TestDetectForwardingMounts(t *testing.T) {
	meta := mountWrapperGraph(t)
	cfg := &APISpecConfig{Framework: FrameworkConfig{
		RoutePatterns: []RoutePattern{{CallRegex: `^Get$`}},
		MountPatterns: []MountPattern{{
			CallRegex:      `^Mount$`,
			RecvTypeRegex:  `^github\.com/go-chi/chi/v5\.\*?Mux$`,
			PathFromArg:    true,
			RouterFromArg:  true,
			PathArgIndex:   0,
			RouterArgIndex: 1,
			IsMount:        true,
		}},
	}}

	byMethod := map[string]DetectedWrapper{}
	for _, w := range DetectRouterWrappers(meta, cfg) {
		if w.Mount == nil {
			continue
		}
		byMethod[w.RecvType+"."+w.Methods[0]] = w
	}

	want := MountPattern{
		CallRegex:      "^Mount$",
		RecvTypeRegex:  recvTypeRegexFor(&wrapperMethod{pkg: "example.com/app", recvType: "*Router"}),
		PathFromArg:    true,
		PathArgIndex:   0,
		RouterFromArg:  true,
		RouterArgIndex: 1,
		IsMount:        true,
	}
	mount, ok := byMethod["example.com/app.*Router.Mount"]
	if !ok || !mount.Complete {
		t.Fatalf("Router.Mount not derived as an applicable mount: %+v", byMethod)
	}
	if !reflect.DeepEqual(*mount.Mount, want) {
		t.Errorf("Router.Mount derived %+v, want %+v", *mount.Mount, want)
	}

	attach, ok := byMethod["example.com/app.*Outer.Attach"]
	if !ok || !attach.Complete {
		t.Fatalf("Outer.Attach (a wrapper of the wrapper) not derived: %+v", byMethod)
	}
	if attach.Mount.PathArgIndex != 0 || attach.Mount.RouterArgIndex != 1 {
		t.Errorf("Outer.Attach roles = (%d, %d), want (0, 1)", attach.Mount.PathArgIndex, attach.Mount.RouterArgIndex)
	}

	for _, key := range []string{"example.com/app.*Router.MountBuilt", "example.com/app.*Router.MountAPI"} {
		if w, ok := byMethod[key]; ok && w.Complete {
			t.Errorf("%s was derived as applicable; mounting the caller's argument there would prefix routes it never produced", key)
		}
	}
	for key := range byMethod {
		if key == "example.com/app.mountHelper" {
			t.Errorf("a plain function was derived as a wrapper")
		}
	}

	// A config with no mount that names a router derives nothing.
	noRouter := &APISpecConfig{Framework: FrameworkConfig{
		RoutePatterns: cfg.Framework.RoutePatterns,
		MountPatterns: []MountPattern{{CallRegex: `^Group$`, PathFromArg: true, IsMount: true}},
	}}
	if got := detectForwardingMounts(meta, NewContextProvider(meta), newParamIndex(meta), noRouter); len(got) != 0 {
		t.Errorf("a path-only mount pattern derived %+v", got)
	}
}

// TestValueRootIdent pins which router arguments are PART of a parameter's
// value — and so live in the caller argument's producer subtree — and which
// compute a new value.
func TestValueRootIdent(t *testing.T) {
	m := &metadata.Metadata{StringPool: metadata.NewStringPool()}
	newArg := func(kind, name string, x *metadata.CallArgument) *metadata.CallArgument {
		a := metadata.NewCallArgument(m)
		a.SetKind(kind)
		if name != "" {
			a.SetName(name)
		}
		a.X = x
		return a
	}
	sub := func() *metadata.CallArgument { return newArg(metadata.KindIdent, "sub", nil) }
	field := func(x *metadata.CallArgument, name string) *metadata.CallArgument {
		a := newArg(metadata.KindSelector, "", x)
		a.Sel = newArg(metadata.KindIdent, name, nil)
		return a
	}
	call := newArg(metadata.KindCall, "", nil)
	call.Fun = newArg(metadata.KindIdent, "build", nil)
	call.Args = []*metadata.CallArgument{sub()}

	cases := []struct {
		name string
		arg  *metadata.CallArgument
		want string
	}{
		{"the parameter itself", sub(), "sub"},
		{"a field of it", field(sub(), "chiRouter"), "sub"},
		{"a nested field", field(field(sub(), "inner"), "mux"), "sub"},
		{"its address", newArg(metadata.KindUnary, "", sub()), "sub"},
		{"a dereference", newArg(metadata.KindStar, "", sub()), "sub"},
		{"parenthesised", newArg(metadata.KindParen, "", field(sub(), "mux")), "sub"},
		{"a call built from it", call, ""},
		{"a field of a call", field(call, "mux"), ""},
		{"nil", nil, ""},
	}
	for _, tc := range cases {
		if got := valueRootIdent(tc.arg); got != tc.want {
			t.Errorf("%s: valueRootIdent = %q, want %q", tc.name, got, tc.want)
		}
	}
}
