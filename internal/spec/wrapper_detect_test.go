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
	"regexp"
	"testing"

	"github.com/ehabterra/apispec/internal/metadata"
)

// TestInnerVerbArgIndex pins which argument a pattern reads its verb from.
//
// MethodArgIndex cannot be read on its own: it defaults to 0, so for a pattern
// whose verb comes from the CALL name (chi's `r.Get(path, h)`) index 0 is the
// PATH. Reading it as the verb would derive a wrapper whose verb is its path
// parameter — every route registered through it would be documented under the
// wrong method (issue #235).
func TestInnerVerbArgIndex(t *testing.T) {
	tests := []struct {
		name    string
		pattern RoutePattern
		want    int
	}{
		{
			name:    "the verb is stated to be an argument",
			pattern: RoutePattern{MethodFromArg: true, MethodArgIndex: 1},
			want:    1,
		},
		{
			// chi's Method(verb, path, handler): no verb hint, but index 0 is
			// neither the path nor the handler, so it is the verb.
			name:    "an index that is neither path nor handler",
			pattern: RoutePattern{MethodArgIndex: 0, PathFromArg: true, PathArgIndex: 1, HandlerFromArg: true, HandlerArgIndex: 2},
			want:    0,
		},
		{
			// chi's Get(path, handler): the verb is the call name, and index 0 is
			// the path.
			name:    "the verb comes from the call name",
			pattern: RoutePattern{MethodFromCall: true, PathFromArg: true, PathArgIndex: 0, HandlerFromArg: true, HandlerArgIndex: 1},
			want:    -1,
		},
		{
			name:    "the default index collides with the path",
			pattern: RoutePattern{PathFromArg: true, PathArgIndex: 0, HandlerFromArg: true, HandlerArgIndex: 1},
			want:    -1,
		},
		{
			name:    "the verb comes from the path itself",
			pattern: RoutePattern{MethodFromPath: true, PathFromArg: true, PathArgIndex: 0},
			want:    -1,
		},
		{
			name:    "the verb comes from the handler name",
			pattern: RoutePattern{MethodFromHandler: true, HandlerFromArg: true, HandlerArgIndex: 1},
			want:    -1,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := innerVerbArgIndex(tc.pattern); got != tc.want {
				t.Errorf("innerVerbArgIndex = %d, want %d", got, tc.want)
			}
		})
	}
}

// TestRecvTypeRegexFor pins the receiver constraint a derived pattern carries: it
// has to match the receiver as metadata renders it, `pkg.*Router`, and it must be
// escaped — an unescaped module path would read its dots as wildcards and match
// unrelated packages.
func TestRecvTypeRegexFor(t *testing.T) {
	w := &wrapperMethod{pkg: "code.gitea.io/gitea/modules/web", recvType: "*Router", name: "Get"}
	re := regexp.MustCompile(recvTypeRegexFor(w))

	for _, match := range []string{
		"code.gitea.io/gitea/modules/web.*Router",
		"code.gitea.io/gitea/modules/web.Router",
	} {
		if !re.MatchString(match) {
			t.Errorf("%q should match its own type", match)
		}
	}
	for _, differ := range []string{
		"code.gitea.io/gitea/modules/web.Combo",
		"codexgitea.io/gitea/modules/web.Router", // a dot read as a wildcard
		"other/web.Router",
	} {
		if re.MatchString(differ) {
			t.Errorf("%q matched a pattern derived for a different type", differ)
		}
	}
}

// TestIdentsIn covers the expression walk the derivation rests on: it has to see
// the parameter inside the shapes a wrapper forwards through, without
// interpreting any of them.
func TestIdentsIn(t *testing.T) {
	m := wireNameMeta()
	ident := func(name string) *metadata.CallArgument {
		a := metadata.NewCallArgument(m)
		a.SetKind(metadata.KindIdent)
		a.SetName(name)
		return a
	}

	// r.getPattern(pattern) — the parameter is inside a call.
	call := metadata.NewCallArgument(m)
	call.SetKind(metadata.KindCall)
	fun := metadata.NewCallArgument(m)
	fun.SetKind(metadata.KindSelector)
	fun.X = ident("r")
	fun.Sel = ident("getPattern")
	call.Fun = fun
	call.Args = []*metadata.CallArgument{ident("pattern")}

	got := identsIn(call, wrapperExprDepth)
	if !contains(got, "pattern") {
		t.Errorf("identsIn(%v) missed the parameter inside the call", got)
	}

	// Depth is bounded, and nothing panics on an empty argument.
	if got := identsIn(call, 0); len(got) != 0 {
		t.Errorf("depth 0 returned %v, want nothing", got)
	}
	if got := identsIn(nil, wrapperExprDepth); got != nil {
		t.Errorf("nil argument returned %v", got)
	}
}

// TestMatchParam pins the parameter lookup, including the case that keeps an
// ordinary registration helper from being read as a wrapper: a value the method
// made up itself matches nothing.
func TestMatchParam(t *testing.T) {
	params := []string{"methods", "pattern", "h"}

	if i, ok := matchParam(params, []string{"r", "getPattern", "pattern"}); !ok || i != 1 {
		t.Errorf("matchParam = (%d, %v), want (1, true)", i, ok)
	}
	if _, ok := matchParam(params, []string{"strings", "TrimSpace", "literal"}); ok {
		t.Error("a value the method built itself was read as a parameter")
	}
	if _, ok := matchParam(params, nil); ok {
		t.Error("no names matched a parameter")
	}
	if _, ok := matchParam(nil, []string{"pattern"}); ok {
		t.Error("a method with no parameters matched one")
	}
}

// TestGroupWrappersMergesMethods pins the grouping: a router's verb methods share
// one derived pattern rather than producing one line each, and the result is
// ordered — derived patterns reach the config, and config order reaches the
// output.
func TestGroupWrappersMergesMethods(t *testing.T) {
	roles := RoutePattern{PathFromArg: true, PathArgIndex: 0, HandlerFromArg: true, HandlerArgIndex: 1, MethodFromCall: true}
	other := RoutePattern{PathFromArg: true, PathArgIndex: 1, HandlerFromArg: true, HandlerArgIndex: 2, MethodFromArg: true}

	found := map[string]DetectedWrapper{
		"app.*Router.Get":     {RecvType: "app.*Router", Methods: []string{"Get"}, Pattern: roles, Complete: true},
		"app.*Router.Post":    {RecvType: "app.*Router", Methods: []string{"Post"}, Pattern: roles, Complete: true},
		"app.*Router.Methods": {RecvType: "app.*Router", Methods: []string{"Methods"}, Pattern: other, Complete: true},
	}

	got := groupWrappers(found)
	if len(got) != 2 {
		t.Fatalf("grouped into %d patterns, want 2 (the verb methods share roles)", len(got))
	}
	var verbs, registrar DetectedWrapper
	for _, w := range got {
		if w.Pattern.MethodFromCall {
			verbs = w
		} else {
			registrar = w
		}
	}
	if want := []string{"Get", "Post"}; !reflect.DeepEqual(verbs.Methods, want) {
		t.Errorf("verb methods = %v, want %v (sorted, merged)", verbs.Methods, want)
	}
	if verbs.Pattern.CallRegex != "^(Get|Post)$" {
		t.Errorf("call regex = %q, want ^(Get|Post)$", verbs.Pattern.CallRegex)
	}
	if registrar.Pattern.CallRegex != "^(Methods)$" {
		t.Errorf("registrar regex = %q, want ^(Methods)$", registrar.Pattern.CallRegex)
	}
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

// wrapperGraph builds one in-module router type whose method forwards its own
// parameters into a framework registration — the shape the whole derivation turns
// on — plus the two shapes that must NOT be derived.
func wrapperGraph(t *testing.T) *metadata.Metadata {
	t.Helper()
	pool := metadata.NewStringPool()
	m := &metadata.Metadata{StringPool: pool, CurrentModulePath: "example.com/app"}

	ident := func(name, typ string) metadata.CallArgument {
		a := metadata.NewCallArgument(m)
		a.SetKind(metadata.KindIdent)
		a.SetName(name)
		if typ != "" {
			a.SetType(typ)
		}
		return *a
	}
	arg := func(name string) *metadata.CallArgument {
		a := ident(name, "")
		return &a
	}
	literal := func(value string) *metadata.CallArgument {
		a := metadata.NewCallArgument(m)
		a.SetKind(metadata.KindLiteral)
		a.SetValue(value)
		return a
	}
	sig := func(params ...metadata.CallArgument) metadata.CallArgument {
		s := metadata.NewCallArgument(m)
		s.SetKind(metadata.KindFuncType)
		for i := range params {
			s.Args = append(s.Args, &params[i])
		}
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
					{Name: pool.Get("Get"), Signature: sig(ident("pattern", "string"), ident("h", ""))},
					{Name: pool.Get("Health"), Signature: sig()},
				}},
			}},
		}},
	}

	m.CallGraph = []metadata.CallGraphEdge{
		// The wrapper: both roles are its own parameters.
		{
			Caller: call("Get", "example.com/app", "*Router"),
			Callee: call("HandleFunc", "net/http", "*ServeMux"),
			Args:   []*metadata.CallArgument{arg("pattern"), arg("h")},
		},
		// A method that registers ONE route with literals: a registrar, not a way
		// of registering, so deriving a pattern from it would invent routes.
		{
			Caller: call("Health", "example.com/app", "*Router"),
			Callee: call("HandleFunc", "net/http", "*ServeMux"),
			Args:   []*metadata.CallArgument{literal(`"/health"`), arg("healthHandler")},
		},
		// A plain function: there is no type to scope a pattern to.
		{
			Caller: call("registerCRUD", "example.com/app", ""),
			Callee: call("HandleFunc", "net/http", "*ServeMux"),
			Args:   []*metadata.CallArgument{arg("base"), arg("h")},
		},
		// A dependency's method: describing it would not document this project.
		{
			Caller: call("Route", "github.com/vendor/kit", "*Kit"),
			Callee: call("HandleFunc", "net/http", "*ServeMux"),
			Args:   []*metadata.CallArgument{arg("pattern"), arg("h")},
		},
	}
	m.BuildCallGraphMaps()
	return m
}

// TestDetectRouterWrappers pins what is and is not a wrapper.
//
// The rule is one fact — a method of an in-module type that FORWARDS ITS OWN
// PARAMETERS into a framework registration — and each of the three rejected
// shapes here would, if accepted, invent routes rather than find them (issue
// #235).
func TestDetectRouterWrappers(t *testing.T) {
	meta := wrapperGraph(t)
	cfg := &APISpecConfig{Framework: FrameworkConfig{RoutePatterns: []RoutePattern{{
		CallRegex:       `^HandleFunc$`,
		RecvTypeRegex:   `^net/http\.\*?ServeMux$`,
		PathFromArg:     true,
		PathArgIndex:    0,
		HandlerFromArg:  true,
		HandlerArgIndex: 1,
	}}}}

	got := DetectRouterWrappers(meta, cfg)

	var applied []DetectedWrapper
	for _, w := range got {
		if w.Complete {
			applied = append(applied, w)
		}
	}
	if len(applied) != 1 {
		t.Fatalf("derived %d applicable wrappers, want 1: %+v", len(applied), got)
	}

	w := applied[0]
	if w.RecvType != "example.com/app.*Router" || !reflect.DeepEqual(w.Methods, []string{"Get"}) {
		t.Errorf("derived %s %v, want example.com/app.*Router [Get]", w.RecvType, w.Methods)
	}
	if !w.Pattern.PathFromArg || w.Pattern.PathArgIndex != 0 {
		t.Errorf("path role = (%v, %d), want the first parameter", w.Pattern.PathFromArg, w.Pattern.PathArgIndex)
	}
	if !w.Pattern.HandlerFromArg || w.Pattern.HandlerArgIndex != 1 {
		t.Errorf("handler role = (%v, %d), want the second parameter", w.Pattern.HandlerFromArg, w.Pattern.HandlerArgIndex)
	}
	// The method is named for its verb, so the verb is fixed by the call.
	if !w.Pattern.MethodFromCall {
		t.Error("a method named Get should take its verb from the call name")
	}

	// Nothing was derived for the rejected shapes.
	for _, w := range got {
		if w.Complete && (contains(w.Methods, "Health") || contains(w.Methods, "registerCRUD") || contains(w.Methods, "Route")) {
			t.Errorf("%s %v was derived; it registers a route rather than describing how to register", w.RecvType, w.Methods)
		}
	}

	// Guards.
	if got := DetectRouterWrappers(nil, cfg); got != nil {
		t.Errorf("nil metadata derived %v", got)
	}
	if got := DetectRouterWrappers(meta, &APISpecConfig{}); got != nil {
		t.Errorf("a config with no patterns derived %v", got)
	}
}

// TestSplitFQRecv covers the receiver split, whose failure mode would be a
// pattern scoped to the wrong package.
func TestSplitFQRecv(t *testing.T) {
	pkg, recv, ok := splitFQRecv("code.gitea.io/gitea/modules/web.*Router")
	if !ok || pkg != "code.gitea.io/gitea/modules/web" || recv != "*Router" {
		t.Errorf("splitFQRecv = (%q, %q, %v)", pkg, recv, ok)
	}
	for _, bad := range []string{"", "noDot", "trailing."} {
		if _, _, ok := splitFQRecv(bad); ok {
			t.Errorf("%q split into a package and receiver", bad)
		}
	}
}

// TestDetectValueWrappers covers the read/write twins of a router wrapper: the
// project's own context, through which handlers answer, decode and read
// parameters. A house router is rarely alone, and gitea documented 856 routes
// with zero components until these were derived (issue #235).
func TestDetectValueWrappers(t *testing.T) {
	pool := metadata.NewStringPool()
	m := &metadata.Metadata{StringPool: pool, CurrentModulePath: "example.com/app"}

	ident := func(name, typ string) metadata.CallArgument {
		a := metadata.NewCallArgument(m)
		a.SetKind(metadata.KindIdent)
		a.SetName(name)
		if typ != "" {
			a.SetType(typ)
		}
		return *a
	}
	arg := func(name string) *metadata.CallArgument { a := ident(name, ""); return &a }
	sig := func(params ...metadata.CallArgument) metadata.CallArgument {
		s := metadata.NewCallArgument(m)
		s.SetKind(metadata.KindFuncType)
		for i := range params {
			s.Args = append(s.Args, &params[i])
		}
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
			"ctx.go": {Types: map[string]*metadata.Type{
				"Ctx": {Name: pool.Get("Ctx"), Methods: []metadata.Method{
					{Name: pool.Get("JSON"), Signature: sig(ident("status", "int"), ident("body", "any"))},
					{Name: pool.Get("Bind"), Signature: sig(ident("dst", "any"))},
					{Name: pool.Get("Query"), Signature: sig(ident("name", "string"))},
				}},
			}},
		}},
	}
	m.CallGraph = []metadata.CallGraphEdge{
		// One responder, two roles: the status through one call, the body through
		// another. Both have to end up on the same derived pattern.
		{Caller: call("JSON", "example.com/app", "*Ctx"), Callee: call("WriteHeader", "net/http", "ResponseWriter"), Args: []*metadata.CallArgument{arg("status")}},
		{Caller: call("JSON", "example.com/app", "*Ctx"), Callee: call("Encode", "encoding/json", "*Encoder"), Args: []*metadata.CallArgument{arg("body")}},
		{Caller: call("Bind", "example.com/app", "*Ctx"), Callee: call("Decode", "encoding/json", "*Decoder"), Args: []*metadata.CallArgument{arg("dst")}},
		{Caller: call("Query", "example.com/app", "*Ctx"), Callee: call("Get", "net/url", "Values"), Args: []*metadata.CallArgument{arg("name")}},
	}
	m.BuildCallGraphMaps()

	cfg := &APISpecConfig{Framework: FrameworkConfig{
		ResponsePatterns: []ResponsePattern{
			{CallRegex: `^WriteHeader$`, StatusFromArg: true, StatusArgIndex: 0, TypeArgIndex: -1},
			{CallRegex: `^Encode$`, TypeFromArg: true, TypeArgIndex: 0},
		},
		RequestBodyPatterns: []RequestBodyPattern{{CallRegex: `^Decode$`, TypeFromArg: true, TypeArgIndex: 0, Deref: true}},
		ParamPatterns:       []ParamPattern{{CallRegex: `^Get$`, RecvType: "net/url.Values", ParamIn: "query", ParamArgIndex: 0}},
	}}

	byMethod := map[string]DetectedWrapper{}
	for _, w := range DetectRouterWrappers(m, cfg) {
		byMethod[w.Methods[0]] = w
	}

	resp, ok := byMethod["JSON"]
	if !ok || resp.Response == nil {
		t.Fatalf("no responder derived; have %v", byMethod)
	}
	if !resp.Response.StatusFromArg || resp.Response.StatusArgIndex != 0 {
		t.Errorf("status role = (%v, %d), want the first parameter", resp.Response.StatusFromArg, resp.Response.StatusArgIndex)
	}
	if !resp.Response.TypeFromArg || resp.Response.TypeArgIndex != 1 {
		t.Errorf("body role = (%v, %d), want the second parameter — merged from the other call",
			resp.Response.TypeFromArg, resp.Response.TypeArgIndex)
	}
	if !resp.Complete {
		t.Error("a responder with a body should be applicable")
	}

	if req, ok := byMethod["Bind"]; !ok || req.Request == nil || req.Request.TypeArgIndex != 0 {
		t.Errorf("decoder not derived from its own parameter: %+v", req)
	}
	if par, ok := byMethod["Query"]; !ok || par.Param == nil || par.Param.ParamIn != "query" || par.Param.ParamArgIndex != 0 {
		t.Errorf("parameter reader not derived, or lost its location: %+v", par)
	}
}
