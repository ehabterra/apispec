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
	"strings"
	"testing"

	"github.com/ehabterra/apispec/internal/metadata"
)

// --- shared builders -------------------------------------------------------

// exSweepMeta returns a fresh metadata carrier whose string pool has index 0
// burned by a sentinel. Several production checks treat pool index 0 as
// "unset" (e.g. Assignment.ConcreteType != 0), so no real string used by
// these tests may land on index 0.
func exSweepMeta() *metadata.Metadata {
	pool := metadata.NewStringPool()
	pool.Get("\x00sweep-sentinel")
	return &metadata.Metadata{StringPool: pool}
}

// sweepCall builds a metadata.Call with every pooled field set explicitly:
// an untouched zero field would resolve to pool index 0 (the sentinel).
func sweepCall(meta *metadata.Metadata, name, pkg, recvType, position string) metadata.Call {
	return metadata.Call{
		Meta:     meta,
		Name:     meta.StringPool.Get(name),
		Pkg:      meta.StringPool.Get(pkg),
		RecvType: meta.StringPool.Get(recvType),
		Position: meta.StringPool.Get(position),
	}
}

// sweepEdge builds a call edge caller -> callee (positions on the callee so
// Callee.ID() carries an "@file:line:col" suffix when calleePos != "").
func sweepEdge(meta *metadata.Metadata, callerName, callerPkg, calleeName, calleePkg, calleeRecv, calleePos string, args ...*metadata.CallArgument) *metadata.CallGraphEdge {
	return &metadata.CallGraphEdge{
		Caller:   sweepCall(meta, callerName, callerPkg, "", ""),
		Callee:   sweepCall(meta, calleeName, calleePkg, calleeRecv, calleePos),
		Position: meta.StringPool.Get(""),
		Args:     args,
	}
}

func sweepIdent(meta *metadata.Metadata, name string) *metadata.CallArgument {
	a := metadata.NewCallArgument(meta)
	a.SetKind(metadata.KindIdent)
	a.SetName(name)
	return a
}

func sweepLit(meta *metadata.Metadata, value string) *metadata.CallArgument {
	a := metadata.NewCallArgument(meta)
	a.SetKind(metadata.KindLiteral)
	a.SetValue(value)
	return a
}

func sweepNode(edge *metadata.CallGraphEdge) *TrackerNode {
	return &TrackerNode{CallGraphEdge: edge}
}

// sweepInterfaceMeta returns metadata declaring interface app.Animal plus
// concrete structs app.Dog / app.Cat, for the interface-narrowing helpers.
func sweepInterfaceMeta() *metadata.Metadata {
	meta := exSweepMeta()
	pool := meta.StringPool
	meta.Packages = map[string]*metadata.Package{
		"app": {
			Files: map[string]*metadata.File{
				"app/main.go": {
					Types: map[string]*metadata.Type{
						"Animal": {Name: pool.Get("Animal"), Pkg: pool.Get("app"), Kind: pool.Get("interface")},
						"Dog":    {Name: pool.Get("Dog"), Pkg: pool.Get("app"), Kind: pool.Get("struct")},
						"Cat":    {Name: pool.Get("Cat"), Pkg: pool.Get("app"), Kind: pool.Get("struct")},
					},
					Functions: map[string]*metadata.Function{},
				},
			},
		},
	}
	return meta
}

// --- pattern_matchers.go ----------------------------------------------------

func TestSweepResolvePathArg(t *testing.T) {
	meta := exSweepMeta()
	b := NewBasePatternMatcher(&APISpecConfig{}, NewContextProvider(meta), nil)

	callNamed := metadata.NewCallArgument(meta)
	callNamed.SetKind(metadata.KindCall)
	callNamed.SetName("mountPoint")

	callFunOnly := metadata.NewCallArgument(meta)
	callFunOnly.SetKind(metadata.KindCall)
	callFunOnly.Fun = sweepIdent(meta, "makePath")

	callAnon := metadata.NewCallArgument(meta)
	callAnon.SetKind(metadata.KindCall)

	tests := []struct {
		name     string
		arg      *metadata.CallArgument
		wantPath string
		wantDyn  string
	}{
		{"nil arg", nil, "", ""},
		{"named call", callNamed, "{mountPoint}", "mountPoint"},
		{"call named via Fun", callFunOnly, "{makePath}", "makePath"},
		{"anonymous call falls back to path", callAnon, "{path}", "path"},
		{"literal passes through", sweepLit(meta, `"/users"`), "/users", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, dyn := b.resolvePathArg(tt.arg)
			if path != tt.wantPath || dyn != tt.wantDyn {
				t.Errorf("resolvePathArg() = (%q, %q), want (%q, %q)", path, dyn, tt.wantPath, tt.wantDyn)
			}
		})
	}
}

func TestSweepRouteMatcherMatchNode(t *testing.T) {
	meta := exSweepMeta()
	cp := NewContextProvider(meta)
	cfg := &APISpecConfig{}

	tests := []struct {
		name    string
		pattern RoutePattern
		edge    *metadata.CallGraphEdge
		want    bool
	}{
		{
			name:    "call regex mismatch",
			pattern: RoutePattern{CallRegex: "^GET$"},
			edge:    sweepEdge(meta, "main", "app", "POST", "app", "", ""),
			want:    false,
		},
		{
			name:    "recv-only fq type matches exact RecvType",
			pattern: RoutePattern{RecvType: "Router"},
			edge:    sweepEdge(meta, "main", "app", "GET", "", "Router", ""),
			want:    true,
		},
		{
			name:    "function name regex mismatch",
			pattern: RoutePattern{CallRegex: "^GET$", FunctionNameRegex: "^setup"},
			edge:    sweepEdge(meta, "main", "app", "GET", "app", "", ""),
			want:    false,
		},
		{
			name:    "recv type regex mismatch",
			pattern: RoutePattern{RecvTypeRegex: `^chi\.Mux$`},
			edge:    sweepEdge(meta, "main", "app", "GET", "other", "Router", ""),
			want:    false,
		},
		{
			name:    "exact recv type mismatch",
			pattern: RoutePattern{RecvType: "chi.Mux"},
			edge:    sweepEdge(meta, "main", "app", "GET", "other", "Router", ""),
			want:    false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewRoutePatternMatcher(tt.pattern, cfg, cp, nil)
			if got := m.MatchNode(sweepNode(tt.edge)); got != tt.want {
				t.Errorf("MatchNode() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSweepRouteMatcherExtractRoute(t *testing.T) {
	meta := exSweepMeta()
	cp := NewContextProvider(meta)
	cfg := &APISpecConfig{}

	t.Run("file falls back to argument position", func(t *testing.T) {
		m := NewRoutePatternMatcher(RoutePattern{MethodArgIndex: -1}, cfg, cp, nil)
		edge := sweepEdge(meta, "main", "app", "Get", "app", "", "")
		arg := sweepIdent(meta, "h")
		arg.SetPosition("app/main.go:5:2")
		node := &TrackerNode{CallGraphEdge: edge, CallArgument: arg}
		route := NewRouteInfo()
		m.ExtractRoute(node, route)
		if route.File != "app/main.go:5:2" {
			t.Errorf("File = %q, want argument position", route.File)
		}
	})

	t.Run("nil edge takes metadata from argument node", func(t *testing.T) {
		m := NewRoutePatternMatcher(RoutePattern{MethodArgIndex: -1}, cfg, cp, nil)
		arg := sweepIdent(meta, "h")
		node := &TrackerNode{CallArgument: arg}
		route := &RouteInfo{
			File: "f.go", Package: "app",
			Response: map[string]*ResponseInfo{}, UsedTypes: map[string]*Schema{},
		}
		m.ExtractRoute(node, route)
		if route.Metadata != meta {
			t.Errorf("expected metadata to come from the argument node")
		}
	})

	t.Run("handler ident is traced to its origin package", func(t *testing.T) {
		m := NewRoutePatternMatcher(RoutePattern{MethodArgIndex: -1, HandlerFromArg: true, HandlerArgIndex: 0}, cfg, cp, nil)
		edge := sweepEdge(meta, "setupRoutes", "app", "Handle", "app", "", "", sweepIdent(meta, "myHandler"))
		route := NewRouteInfo()
		found := m.ExtractRoute(sweepNode(edge), route)
		if !found {
			t.Fatal("expected handler extraction to report found")
		}
		if route.Handler == "" {
			t.Error("expected a traced handler name")
		}
		if route.Package == "" {
			t.Error("expected the traced origin package to be recorded")
		}
	})
}

func TestSweepExtractRouteDetails(t *testing.T) {
	cfg := &APISpecConfig{}

	t.Run("method from handler name", func(t *testing.T) {
		meta := exSweepMeta()
		cp := NewContextProvider(meta)
		m := NewRoutePatternMatcher(RoutePattern{
			MethodFromHandler: true, HandlerFromArg: true, HandlerArgIndex: 0, MethodArgIndex: -1,
		}, cfg, cp, nil)
		edge := sweepEdge(meta, "main", "app", "Handle", "app", "", "", sweepIdent(meta, "deleteWidget"))
		route := NewRouteInfo()
		if !m.extractRouteDetails(sweepNode(edge), route) {
			t.Fatal("expected details to be found")
		}
		if route.Method != "DELETE" || !route.MethodExplicit {
			t.Errorf("Method = %q (explicit=%v), want DELETE explicit", route.Method, route.MethodExplicit)
		}
	})

	t.Run("method arg falls back to const-resolved argument info", func(t *testing.T) {
		meta := exSweepMeta()
		pool := meta.StringPool
		// A const declared in metadata: GetArgumentInfo resolves the ident to
		// its literal value "DELETE" while GetValue stays the raw expression.
		meta.Packages = map[string]*metadata.Package{
			"net/http": {Files: map[string]*metadata.File{
				"h.go": {Variables: map[string]*metadata.Variable{
					"MethodDelete": {Tok: pool.Get("const"), Value: pool.Get(`"DELETE"`)},
				}},
			}},
		}
		cp := NewContextProvider(meta)
		m := NewRoutePatternMatcher(RoutePattern{MethodArgIndex: 0}, cfg, cp, nil)
		arg := sweepIdent(meta, "MethodDelete")
		arg.SetPkg("net/http")
		arg.SetValue("http.MethodDelete") // raw value: not a valid method by itself
		edge := sweepEdge(meta, "main", "app", "Handle", "app", "", "", arg)
		route := NewRouteInfo()
		if !m.extractRouteDetails(sweepNode(edge), route) {
			t.Fatal("expected details to be found")
		}
		if route.Method != "DELETE" {
			t.Errorf("Method = %q, want DELETE via argument info", route.Method)
		}
	})

	t.Run("unresolvable method infers from context", func(t *testing.T) {
		meta := exSweepMeta()
		cp := NewContextProvider(meta)
		m := NewRoutePatternMatcher(RoutePattern{
			MethodArgIndex: 0, MethodExtraction: DefaultMethodExtractionConfig(),
		}, cfg, cp, nil)
		arg := sweepIdent(meta, "someVerb")
		arg.SetValue("someVerb")
		edge := sweepEdge(meta, "listThings", "app", "Handle", "app", "", "", arg)
		route := NewRouteInfo()
		if !m.extractRouteDetails(sweepNode(edge), route) {
			t.Fatal("expected details to be found")
		}
		if route.Method != "GET" {
			t.Errorf("Method = %q, want GET inferred from caller name", route.Method)
		}
	})

	t.Run("dynamic path arg records a placeholder param", func(t *testing.T) {
		meta := exSweepMeta()
		cp := NewContextProvider(meta)
		m := NewRoutePatternMatcher(RoutePattern{PathFromArg: true, PathArgIndex: 0, MethodArgIndex: -1}, cfg, cp, nil)
		pathCall := metadata.NewCallArgument(meta)
		pathCall.SetKind(metadata.KindCall)
		pathCall.SetName("mountPoint")
		edge := sweepEdge(meta, "main", "app", "Mount", "app", "", "", pathCall)
		route := NewRouteInfo()
		m.extractRouteDetails(sweepNode(edge), route)
		if route.Path != "{mountPoint}" {
			t.Errorf("Path = %q, want {mountPoint}", route.Path)
		}
		if len(route.DynamicParams) != 1 || route.DynamicParams[0] != "mountPoint" {
			t.Errorf("DynamicParams = %v, want [mountPoint]", route.DynamicParams)
		}
	})
}

func TestSweepInferMethodFromContext(t *testing.T) {
	cfg := &APISpecConfig{}

	t.Run("sibling Methods call resolved via argument info", func(t *testing.T) {
		meta := exSweepMeta()
		pool := meta.StringPool
		meta.Packages = map[string]*metadata.Package{
			"net/http": {Files: map[string]*metadata.File{
				"h.go": {Variables: map[string]*metadata.Variable{
					"MethodPut": {Tok: pool.Get("const"), Value: pool.Get(`"PUT"`)},
				}},
			}},
		}
		cp := NewContextProvider(meta)
		m := NewRoutePatternMatcher(RoutePattern{MethodExtraction: DefaultMethodExtractionConfig()}, cfg, cp, nil)

		methodArg := sweepIdent(meta, "MethodPut")
		methodArg.SetPkg("net/http")
		methodArg.SetValue("http.MethodPut")
		siblingEdge := sweepEdge(meta, "main", "app", "Methods", "mux", "", "", methodArg)

		parent := &TrackerNode{}
		self := &TrackerNode{Parent: parent}
		sibling := &TrackerNode{CallGraphEdge: siblingEdge, Parent: parent}
		parent.Children = []*TrackerNode{self, sibling}

		edge := sweepEdge(meta, "main", "app", "HandleFunc", "mux", "", "")
		if got := m.inferMethodFromContext(self, edge); got != "PUT" {
			t.Errorf("inferMethodFromContext() = %q, want PUT", got)
		}
	})

	t.Run("caller name yields the method", func(t *testing.T) {
		meta := exSweepMeta()
		cp := NewContextProvider(meta)
		m := NewRoutePatternMatcher(RoutePattern{MethodExtraction: DefaultMethodExtractionConfig()}, cfg, cp, nil)
		// "deleteWidget" pins the word-boundary fix: the old substring
		// matcher spotted "get" inside "widget" and returned GET.
		edge := sweepEdge(meta, "deleteWidget", "app", "HandleFunc", "mux", "", "")
		if got := m.inferMethodFromContext(&TrackerNode{}, edge); got != "DELETE" {
			t.Errorf("inferMethodFromContext() = %q, want DELETE", got)
		}
	})

	t.Run("handler argument yields the method when caller maps to POST", func(t *testing.T) {
		meta := exSweepMeta()
		cp := NewContextProvider(meta)
		m := NewRoutePatternMatcher(RoutePattern{MethodExtraction: DefaultMethodExtractionConfig()}, cfg, cp, nil)
		// "updateWidget" pins the word-boundary fix on the handler-arg path:
		// the old substring matcher resolved "widget" to GET before the
		// "update" prefix could map to PUT.
		edge := sweepEdge(meta, "createThing", "app", "HandleFunc", "mux", "", "",
			sweepLit(meta, `"/x"`), sweepIdent(meta, "updateWidget"))
		if got := m.inferMethodFromContext(&TrackerNode{}, edge); got != "PUT" {
			t.Errorf("inferMethodFromContext() = %q, want PUT from handler arg", got)
		}
	})
}

func TestSweepMountMatcherMatchNode(t *testing.T) {
	meta := exSweepMeta()
	cp := NewContextProvider(meta)
	cfg := &APISpecConfig{}

	tests := []struct {
		name    string
		pattern MountPattern
		edge    *metadata.CallGraphEdge
		want    bool
	}{
		{
			name:    "call regex mismatch",
			pattern: MountPattern{CallRegex: "^Mount$", IsMount: true},
			edge:    sweepEdge(meta, "main", "app", "Route", "chi", "", ""),
			want:    false,
		},
		{
			name:    "function name regex mismatch",
			pattern: MountPattern{CallRegex: "^Mount$", FunctionNameRegex: "^setup", IsMount: true},
			edge:    sweepEdge(meta, "main", "app", "Mount", "chi", "", ""),
			want:    false,
		},
		{
			name:    "recv type regex mismatch",
			pattern: MountPattern{RecvTypeRegex: `^chi\.Mux$`, IsMount: true},
			edge:    sweepEdge(meta, "main", "app", "Mount", "other", "Router", ""),
			want:    false,
		},
		{
			name:    "exact recv type mismatch",
			pattern: MountPattern{RecvType: "chi.Mux", IsMount: true},
			edge:    sweepEdge(meta, "main", "app", "Mount", "other", "Router", ""),
			want:    false,
		},
		{
			name:    "full match returns IsMount",
			pattern: MountPattern{CallRegex: "^Mount$", RecvType: "chi.Mux", IsMount: true},
			edge:    sweepEdge(meta, "main", "app", "Mount", "chi", "Mux", ""),
			want:    true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewMountPatternMatcher(tt.pattern, cfg, cp, nil)
			if got := m.MatchNode(sweepNode(tt.edge)); got != tt.want {
				t.Errorf("MatchNode() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSweepSecurityMatcher(t *testing.T) {
	meta := exSweepMeta()
	cp := NewContextProvider(meta)
	cfg := &APISpecConfig{}

	t.Run("nil node and nil edge", func(t *testing.T) {
		m := NewSecurityPatternMatcher(SecurityPattern{CallRegex: "^Use$"}, cfg, cp, nil)
		if m.MatchNode(nil) {
			t.Error("MatchNode(nil) = true, want false")
		}
		if m.ExtractMiddleware(nil) != nil {
			t.Error("ExtractMiddleware(nil) != nil")
		}
		if m.ExtractMiddlewareFromEdge(nil) != nil {
			t.Error("ExtractMiddlewareFromEdge(nil) != nil")
		}
	})

	t.Run("match edge rejections", func(t *testing.T) {
		tests := []struct {
			name    string
			pattern SecurityPattern
			edge    *metadata.CallGraphEdge
		}{
			{"call regex", SecurityPattern{CallRegex: "^Use$"}, sweepEdge(meta, "main", "app", "Get", "chi", "", "")},
			{"function name regex", SecurityPattern{CallRegex: "^Use$", FunctionNameRegex: "^setup"}, sweepEdge(meta, "main", "app", "Use", "chi", "", "")},
			{"exact recv type", SecurityPattern{RecvType: "chi.Mux"}, sweepEdge(meta, "main", "app", "Use", "other", "Router", "")},
		}
		for _, tt := range tests {
			m := NewSecurityPatternMatcher(tt.pattern, cfg, cp, nil)
			if m.MatchEdge(tt.edge) {
				t.Errorf("%s: MatchEdge() = true, want false", tt.name)
			}
		}
	})

	t.Run("priority counts function name regex", func(t *testing.T) {
		m := NewSecurityPatternMatcher(SecurityPattern{CallRegex: "x", FunctionNameRegex: "y", RecvType: "z"}, cfg, cp, nil)
		if got := m.GetPriority(); got != 18 {
			t.Errorf("GetPriority() = %d, want 18", got)
		}
	})

	t.Run("negative middleware arg index clamps to zero", func(t *testing.T) {
		m := NewSecurityPatternMatcher(SecurityPattern{Scope: SecurityScopeRouter, MiddlewareArgIndex: -1}, cfg, cp, nil)
		mw := sweepIdent(meta, "authMW")
		mw.SetPkg("app")
		edge := sweepEdge(meta, "main", "app", "Use", "chi", "Mux", "", mw)
		refs := m.ExtractMiddlewareFromEdge(edge)
		if len(refs) != 1 || refs[0].FunctionName != "authMW" {
			t.Errorf("refs = %+v, want single authMW ref", refs)
		}
	})
}

func TestSweepRequestMatcherMatchNodeAndPriority(t *testing.T) {
	meta := exSweepMeta()
	cp := NewContextProvider(meta)
	cfg := &APISpecConfig{}

	t.Run("nil node", func(t *testing.T) {
		m := NewRequestPatternMatcher(RequestBodyPattern{CallRegex: "^Bind$"}, cfg, cp, nil)
		if m.MatchNode(nil) {
			t.Error("MatchNode(nil) = true, want false")
		}
	})

	rejections := []struct {
		name    string
		pattern RequestBodyPattern
		edge    *metadata.CallGraphEdge
	}{
		{"call regex", RequestBodyPattern{CallRegex: "^Bind$"}, sweepEdge(meta, "h", "app", "JSON", "gin", "", "")},
		{"function name regex", RequestBodyPattern{CallRegex: "^Bind$", FunctionNameRegex: "^handle"}, sweepEdge(meta, "main", "app", "Bind", "gin", "", "")},
		{"recv type regex", RequestBodyPattern{RecvTypeRegex: `^gin\.Context$`}, sweepEdge(meta, "h", "app", "Bind", "other", "Ctx", "")},
		{"exact recv type", RequestBodyPattern{RecvType: "gin.Context"}, sweepEdge(meta, "h", "app", "Bind", "other", "Ctx", "")},
	}
	for _, tt := range rejections {
		t.Run(tt.name, func(t *testing.T) {
			m := NewRequestPatternMatcher(tt.pattern, cfg, cp, nil)
			if m.MatchNode(sweepNode(tt.edge)) {
				t.Errorf("MatchNode() = true, want false")
			}
		})
	}

	t.Run("priority branches", func(t *testing.T) {
		m := NewRequestPatternMatcher(RequestBodyPattern{FunctionNameRegex: "y", RecvTypeRegex: "z"}, cfg, cp, nil)
		if got := m.GetPriority(); got != 8 {
			t.Errorf("GetPriority() = %d, want 8", got)
		}
	})
}

func TestSweepRequestBodySource(t *testing.T) {
	meta := exSweepMeta()
	cp := NewContextProvider(meta)
	cfg := &APISpecConfig{}

	t.Run("nil edge", func(t *testing.T) {
		m := NewRequestPatternMatcher(RequestBodyPattern{}, cfg, cp, nil)
		if m.bodySource(nil) != nil {
			t.Error("bodySource(nil) != nil")
		}
	})

	t.Run("body from receiver resolves via chain parent", func(t *testing.T) {
		m := NewRequestPatternMatcher(RequestBodyPattern{BodyFromReceiver: true}, cfg, cp, nil)
		src := sweepIdent(meta, "r")
		edge := sweepEdge(meta, "h", "app", "Decode", "json", "Decoder", "")
		edge.ChainParent = sweepEdge(meta, "h", "app", "NewDecoder", "json", "", "", src)
		got := m.bodySource(edge)
		if got != src {
			t.Errorf("bodySource() = %v, want the chain-parent factory arg", got)
		}
	})
}

func TestSweepRequestExtractRequest(t *testing.T) {
	cfg := &APISpecConfig{Defaults: Defaults{RequestContentType: "application/json"}}

	t.Run("call arg uses its return type", func(t *testing.T) {
		meta := exSweepMeta()
		cp := NewContextProvider(meta)
		m := NewRequestPatternMatcher(RequestBodyPattern{TypeFromArg: true, TypeArgIndex: 0}, cfg, cp, nil)
		arg := metadata.NewCallArgument(meta)
		arg.SetKind(metadata.KindCall)
		arg.Fun = sweepIdent(meta, "readUser")
		arg.SetType("app.User")
		edge := sweepEdge(meta, "h", "app", "Decode", "json", "", "", arg)
		req := m.ExtractRequest(sweepNode(edge), NewRouteInfo())
		if req == nil || req.BodyType != "app.User" {
			t.Fatalf("req = %+v, want BodyType app.User", req)
		}
	})

	t.Run("resolved type wins", func(t *testing.T) {
		meta := exSweepMeta()
		cp := NewContextProvider(meta)
		m := NewRequestPatternMatcher(RequestBodyPattern{TypeFromArg: true, TypeArgIndex: 0}, cfg, cp, nil)
		arg := sweepIdent(meta, "v")
		arg.SetResolvedType("app.Widget")
		edge := sweepEdge(meta, "h", "app", "Decode", "json", "", "", arg)
		req := m.ExtractRequest(sweepNode(edge), NewRouteInfo())
		if req == nil || req.BodyType != "app.Widget" {
			t.Fatalf("req = %+v, want BodyType app.Widget", req)
		}
	})

	t.Run("generic arg resolves through type param map", func(t *testing.T) {
		meta := exSweepMeta()
		cp := NewContextProvider(meta)
		m := NewRequestPatternMatcher(RequestBodyPattern{TypeFromArg: true, TypeArgIndex: 0}, cfg, cp, nil)
		arg := sweepIdent(meta, "v")
		arg.IsGenericType = true
		arg.GenericTypeName = meta.StringPool.Get("T")
		edge := sweepEdge(meta, "h", "app", "Decode", "json", "", "", arg)
		// Seed via the edge: TrackerNode.TypeParams() rebuilds its map from the
		// edge/arg/parent on every read, so a directly-set node.typeParamMap is
		// discarded.
		edge.TypeParamMap = map[string]string{"T": "app.Item"}
		node := sweepNode(edge)
		req := m.ExtractRequest(node, NewRouteInfo())
		if req == nil || req.BodyType != "app.Item" {
			t.Fatalf("req = %+v, want BodyType app.Item", req)
		}
	})

	t.Run("deref strips pointer and param rebinds through wrapper", func(t *testing.T) {
		meta := exSweepMeta()
		cp := NewContextProvider(meta)
		m := NewRequestPatternMatcher(RequestBodyPattern{TypeFromArg: true, TypeArgIndex: 0, Deref: true}, cfg, cp, nil)

		concrete := metadata.NewCallArgument(meta)
		concrete.SetKind(metadata.KindIdent)
		concrete.SetName("u")
		concrete.SetResolvedType("*app.User")

		parentEdge := sweepEdge(meta, "handler", "app", "readRequest", "app", "", "")
		parentEdge.ParamArgMap = map[string]metadata.CallArgument{"v": *concrete}
		parent := sweepNode(parentEdge)

		param := sweepIdent(meta, "v")
		edge := sweepEdge(meta, "readRequest", "app", "Decode", "json", "", "", param)
		node := sweepNode(edge)
		node.Parent = parent

		req := m.ExtractRequest(node, NewRouteInfo())
		if req == nil || req.BodyType != "app.User" {
			t.Fatalf("req = %+v, want BodyType app.User via wrapper rebinding + deref", req)
		}
	})
}

func TestSweepRequestResolveTypeOrigin(t *testing.T) {
	meta := exSweepMeta()
	cp := NewContextProvider(meta)
	m := NewRequestPatternMatcher(RequestBodyPattern{}, &APISpecConfig{}, cp, nil)

	t.Run("selector fast path uses recorded type", func(t *testing.T) {
		sel := metadata.NewCallArgument(meta)
		sel.SetKind(metadata.KindSelector)
		sel.SetType("string")
		node := sweepNode(sweepEdge(meta, "h", "app", "Bind", "gin", "", ""))
		if got := m.resolveTypeOrigin(sel, node, "orig"); got != "string" {
			t.Errorf("resolveTypeOrigin(selector) = %q, want string", got)
		}
	})

	t.Run("ident resolves through edge assignments", func(t *testing.T) {
		arg := sweepIdent(meta, "u")
		edge := sweepEdge(meta, "h", "app", "Bind", "gin", "", "")
		edge.AssignmentMap = map[string][]metadata.Assignment{
			"u": {{ConcreteType: meta.StringPool.Get("app.User")}},
		}
		if got := m.resolveTypeOrigin(arg, sweepNode(edge), "any"); got != "app.User" {
			t.Errorf("resolveTypeOrigin(ident) = %q, want app.User", got)
		}
	})

	t.Run("fallthrough keeps original type", func(t *testing.T) {
		lit := sweepLit(meta, "42")
		node := sweepNode(sweepEdge(meta, "h", "app", "Bind", "gin", "", ""))
		if got := m.resolveTypeOrigin(lit, node, "int"); got != "int" {
			t.Errorf("resolveTypeOrigin(literal) = %q, want int", got)
		}
	})
}

func TestSweepBaseMatcherGuards(t *testing.T) {
	t.Run("empty pattern never matches", func(t *testing.T) {
		b := NewBasePatternMatcher(&APISpecConfig{}, NewContextProvider(exSweepMeta()), nil)
		if b.matchPattern("", "anything") {
			t.Error("matchPattern(\"\") = true, want false")
		}
	})

	t.Run("trace and assignment lookups degrade without metadata", func(t *testing.T) {
		b := NewBasePatternMatcher(&APISpecConfig{}, NewContextProvider(nil), nil)
		v, p, typ, fn := b.traceVariable("x", "f", "pkg")
		if v != "x" || p != "pkg" || typ != nil || fn != "" {
			t.Errorf("traceVariable = (%q,%q,%v,%q), want passthrough", v, p, typ, fn)
		}
		arg := metadata.NewCallArgument(exSweepMeta())
		if got := b.findAssignmentFunction(arg); got != nil {
			t.Errorf("findAssignmentFunction = %v, want nil", got)
		}
	})
}

func TestSweepExtractMethodFromFunctionNameWithConfig(t *testing.T) {
	b := NewBasePatternMatcher(&APISpecConfig{}, NewContextProvider(exSweepMeta()), nil)

	if got := b.extractMethodFromFunctionNameWithConfig("", nil); got != "" {
		t.Errorf("empty func name: got %q, want empty", got)
	}

	// Mappings deliberately listed in ascending priority so the sort has to
	// reorder them: the higher-priority longer prefix must win.
	cfg := &MethodExtractionConfig{
		MethodMappings: []MethodMapping{
			{Patterns: []string{"x"}, Method: "GET", Priority: 1},
			{Patterns: []string{"xy"}, Method: "PUT", Priority: 5},
		},
		UsePrefix:     true,
		DefaultMethod: "HEAD",
	}
	if got := b.extractMethodFromFunctionNameWithConfig("xy", cfg); got != "PUT" {
		t.Errorf("priority sort: got %q, want PUT", got)
	}
	if got := b.extractMethodFromFunctionNameWithConfig("zzz", cfg); got != "HEAD" {
		t.Errorf("default method: got %q, want HEAD", got)
	}

	// The matched report separates evidence from fallback: a mapping hit is
	// explicit; the DefaultMethod is not (so verb-less registrations stay
	// open to method-dispatch splitting).
	if m, matched := b.methodFromFunctionName("xy", cfg); m != "PUT" || !matched {
		t.Errorf("mapping hit: got (%q,%v), want (PUT,true)", m, matched)
	}
	if m, matched := b.methodFromFunctionName("zzz", cfg); m != "HEAD" || matched {
		t.Errorf("default fallback: got (%q,%v), want (HEAD,false)", m, matched)
	}
	if m, matched := b.methodFromFunctionName("", cfg); m != "" || matched {
		t.Errorf("empty name: got (%q,%v), want (\"\",false)", m, matched)
	}
}

// --- extractor.go ------------------------------------------------------------

func TestSweepSmallHelpers(t *testing.T) {
	t.Run("appendUniqueStrings", func(t *testing.T) {
		base := []string{"a"}
		if got := appendUniqueStrings(base); len(got) != 1 || &got[0] != &base[0] {
			t.Errorf("no extras must return base unchanged, got %v", got)
		}
		if got := appendUniqueStrings([]string{"a", "a"}, "a", "b"); len(got) != 2 || got[0] != "a" || got[1] != "b" {
			t.Errorf("got %v, want [a b]", got)
		}
	})

	t.Run("frameChainKey without edge joins the chain", func(t *testing.T) {
		got := frameChainKey([]string{"x", "y"}, &TrackerNode{})
		if got != "x"+chainSep+"y" {
			t.Errorf("frameChainKey = %q, want joined chain", got)
		}
	})

	t.Run("preferRequestInfo", func(t *testing.T) {
		concrete := &RequestInfo{Schema: &Schema{Ref: "#/x"}}
		generic := &RequestInfo{Schema: &Schema{Type: "object"}}
		if got := preferRequestInfo(nil, generic); got != generic {
			t.Error("nil cur must yield next")
		}
		if got := preferRequestInfo(generic, nil); got != generic {
			t.Error("nil next must keep cur")
		}
		if got := preferRequestInfo(generic, concrete); got != concrete {
			t.Error("concrete next must win")
		}
		if got := preferRequestInfo(concrete, generic); got != concrete {
			t.Error("concrete cur must win")
		}
		genericB := &RequestInfo{Schema: &Schema{Type: "object"}}
		if got := preferRequestInfo(generic, genericB); got != genericB {
			t.Error("generic tie must keep the newer request (last-write-wins)")
		}
		concreteB := &RequestInfo{Schema: &Schema{Ref: "#/y"}}
		if got := preferRequestInfo(concrete, concreteB); got != concreteB {
			t.Error("concrete tie must keep the newer request")
		}
	})

	t.Run("requestIsConcrete guards", func(t *testing.T) {
		if requestIsConcrete(nil) || requestIsConcrete(&RequestInfo{}) {
			t.Error("nil/schema-less request must not be concrete")
		}
	})

	t.Run("preferResponseInfo", func(t *testing.T) {
		cur := &ResponseInfo{BodyType: "app.User", Schema: &Schema{Ref: "#/u"}}
		errResp := &ResponseInfo{BodyType: "app.ErrorDTO", Schema: &Schema{Ref: "#/e"}}
		if got := preferResponseInfo(cur, nil); got != cur {
			t.Error("nil next must keep cur")
		}
		if got := preferResponseInfo(cur, errResp); got != cur {
			t.Error("error-named next must lose")
		}
		if got := preferResponseInfo(errResp, cur); got != cur {
			t.Error("error-named cur must lose")
		}
		a := &ResponseInfo{BodyType: "app.A", Schema: &Schema{Ref: "#/a"}}
		b := &ResponseInfo{BodyType: "app.B", Schema: &Schema{Ref: "#/b"}}
		if got := preferResponseInfo(b, a); got != a {
			t.Error("lexicographic tie-break must pick the smaller BodyType")
		}
		if got := preferResponseInfo(a, b); got != a {
			t.Error("lexicographic tie-break must keep the smaller BodyType")
		}
	})

	t.Run("determineLiteralType uint", func(t *testing.T) {
		if got := determineLiteralType("18446744073709551615"); got != "uint" {
			t.Errorf("got %q, want uint", got)
		}
	})

	t.Run("preprocessingBodyType", func(t *testing.T) {
		for in, want := range map[string]string{"*User": "User", "&User": "User", "[]User": "User", "*": "*"} {
			if got := preprocessingBodyType(in); got != want {
				t.Errorf("preprocessingBodyType(%q) = %q, want %q", in, got, want)
			}
		}
	})

	t.Run("isInterfaceTypeName", func(t *testing.T) {
		meta := sweepInterfaceMeta()
		if isInterfaceTypeName("", meta) {
			t.Error("empty name must not be an interface")
		}
		if isInterfaceTypeName("app.Animal", nil) {
			t.Error("nil meta must not resolve")
		}
		if !isInterfaceTypeName("app.Animal", meta) {
			t.Error("app.Animal must resolve to an interface")
		}
		if isInterfaceTypeName("app.Dog", meta) {
			t.Error("app.Dog is a struct")
		}
		if isInterfaceTypeName("map[string]int", meta) {
			t.Error("map types are never interfaces")
		}
	})
}

func TestSweepCalleePosition(t *testing.T) {
	meta := exSweepMeta()
	tests := []struct {
		name     string
		pos      string
		wantFile string
		wantLine int
		wantCol  int
	}{
		{"no position", "", "app.fn", 0, 0},
		{"no colon", "main.go", "main.go", 0, 0},
		{"one colon", "main.go:10", "main.go", 0, 10},
		{"full position", "main.go:10:5", "main.go", 10, 5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			edge := sweepEdge(meta, "h", "app", "fn", "app", "", tt.pos)
			file, line, col := calleePosition(sweepNode(edge))
			if file != tt.wantFile || line != tt.wantLine || col != tt.wantCol {
				t.Errorf("calleePosition() = (%q,%d,%d), want (%q,%d,%d)", file, line, col, tt.wantFile, tt.wantLine, tt.wantCol)
			}
		})
	}
	t.Run("nil edge", func(t *testing.T) {
		file, line, col := calleePosition(&TrackerNode{})
		if file != "" || line != 0 || col != 0 {
			t.Errorf("calleePosition(no edge) = (%q,%d,%d), want zero values", file, line, col)
		}
	})
}

func TestSweepExtractorGuards(t *testing.T) {
	meta := exSweepMeta()
	tree := NewMockTrackerTree(meta, metadata.TrackerLimits{})
	cfg := &APISpecConfig{
		Framework: FrameworkConfig{
			SecurityPatterns: []SecurityPattern{{CallRegex: "^With$", Scope: SecurityScopeRoute, MiddlewareArgIndex: 0}},
		},
	}
	ext := NewExtractor(tree, cfg)

	t.Run("traverse tolerates nil node", func(t *testing.T) {
		var routes []*RouteInfo
		ext.traverseForRoutesWithVisited(nil, "", nil, nil, nil, &routes, map[string]bool{})
		if len(routes) != 0 {
			t.Errorf("routes = %v, want none", routes)
		}
	})

	t.Run("collectChainSecurity guards", func(t *testing.T) {
		if got := ext.collectChainSecurity(nil); got != nil {
			t.Errorf("nil node: got %v", got)
		}
		if got := ext.collectChainSecurity(&TrackerNode{}); got != nil {
			t.Errorf("nil edge: got %v", got)
		}
	})

	t.Run("collectChainSecurity walks chain parents", func(t *testing.T) {
		mw := sweepIdent(meta, "authMW")
		mw.SetPkg("app")
		routeEdge := sweepEdge(meta, "main", "app", "Get", "chi", "Mux", "")
		routeEdge.ChainParent = sweepEdge(meta, "main", "app", "With", "chi", "Mux", "", mw)
		refs := ext.collectChainSecurity(sweepNode(routeEdge))
		if len(refs) != 1 || refs[0].FunctionName != "authMW" {
			t.Errorf("refs = %+v, want single authMW", refs)
		}
	})

	t.Run("responseMatcherIndex without edge", func(t *testing.T) {
		if got := ext.responseMatcherIndex(&TrackerNode{}); got != -1 {
			t.Errorf("got %d, want -1", got)
		}
	})

	t.Run("extractResponsesMatched without matchers", func(t *testing.T) {
		node := sweepNode(sweepEdge(meta, "h", "app", "JSON", "gin", "", ""))
		if got := ext.extractResponsesMatched(node, NewRouteInfo()); got != nil {
			t.Errorf("got %v, want nil", got)
		}
	})

	t.Run("handlerCallDepths guards and cache", func(t *testing.T) {
		empty := ext.handlerCallDepths(&RouteInfo{})
		if len(empty) != 0 {
			t.Errorf("empty function: got %v", empty)
		}
		route := &RouteInfo{Function: "app.h", Metadata: meta}
		first := ext.handlerCallDepths(route)
		second := ext.handlerCallDepths(route)
		if len(first) != 1 || first["app.h"] != 0 {
			t.Errorf("depths = %v, want {app.h:0}", first)
		}
		// The memo must return the identical map: a mutation through one
		// reference is visible through a fresh fetch.
		if len(second) != len(first) {
			t.Errorf("cached call returned different depths: %v vs %v", second, first)
		}
		first["zz_probe"] = 99
		if third := ext.handlerCallDepths(route); third["zz_probe"] != 99 {
			t.Error("handlerCallDepths did not return the cached map")
		}
		delete(first, "zz_probe")
	})

	t.Run("calleeMiddlewareRef nil edge", func(t *testing.T) {
		if got := ext.calleeMiddlewareRef(nil); got != (MiddlewareRef{}) {
			t.Errorf("got %+v, want zero", got)
		}
	})
}

func TestSweepExpandMiddlewareRefs(t *testing.T) {
	t.Run("nil metadata keeps refs", func(t *testing.T) {
		tree := NewMockTrackerTree(nil, metadata.TrackerLimits{})
		cfg := &APISpecConfig{SecurityMappings: []SecurityMapping{{FunctionNameRegex: "^jwt$"}}}
		ext := NewExtractor(tree, cfg)
		refs := []MiddlewareRef{{FunctionName: "custom", Pkg: "app"}}
		got := ext.expandMiddlewareRefs(refs)
		if len(got) != 1 || got[0].FunctionName != "custom" {
			t.Errorf("got %+v, want refs unchanged", got)
		}
	})

	t.Run("identity-less ref survives look-through", func(t *testing.T) {
		meta := exSweepMeta()
		meta.BuildCallGraphMaps()
		tree := NewMockTrackerTree(meta, metadata.TrackerLimits{})
		cfg := &APISpecConfig{SecurityMappings: []SecurityMapping{{FunctionNameRegex: "^jwt$"}}}
		ext := NewExtractor(tree, cfg)
		refs := []MiddlewareRef{{Position: "x.go:1:1"}}
		got := ext.expandMiddlewareRefs(refs)
		if len(got) != 1 {
			t.Errorf("got %+v, want the empty-identity ref kept", got)
		}
	})
}

func TestSweepHandleMountNodeWithAssignment(t *testing.T) {
	meta := exSweepMeta()
	tree := NewMockTrackerTree(meta, metadata.TrackerLimits{})
	ext := NewExtractor(tree, &APISpecConfig{})

	assignment := sweepIdent(meta, "sub")
	assignment.SetPkg("app")

	child := sweepNode(sweepEdge(meta, "routes", "app", "Get", "chi", "Mux", ""))
	target := &TrackerNode{key: assignment.ID(), Children: []*TrackerNode{child}}
	tree.AddRoot(target)

	var routes []*RouteInfo
	visited := map[string]bool{}
	node := &TrackerNode{}
	// Path empty on purpose: handleRouterAssignment must inherit mountTags
	// (the mountPath == "" branch), not synthesize tags from the path.
	ext.handleMountNode(node, MountInfo{Assignment: assignment}, "", []string{"inherited"}, nil, nil, &routes, visited)
	if len(routes) != 0 {
		t.Errorf("no route patterns configured: routes = %v, want none", routes)
	}
	if !visited[child.GetKey()+"@"] {
		t.Errorf("expected the assigned router's child to be traversed; visited = %v", visited)
	}
}

func TestSweepApplyOverrides(t *testing.T) {
	cfg := &APISpecConfig{Overrides: []Override{{
		FunctionName:   "app.h",
		Summary:        "overridden",
		ResponseStatus: 200,
		ResponseType:   "*app.User",
		Tags:           []string{"users"},
	}}}
	applier := NewOverrideApplier(cfg)
	route := &RouteInfo{
		Function: "app.h",
		Response: map[string]*ResponseInfo{"200": {StatusCode: 0, BodyType: "old"}},
	}
	applier.ApplyOverrides(route)
	if route.Summary != "overridden" {
		t.Errorf("Summary = %q", route.Summary)
	}
	if route.Response["200"].StatusCode != 200 {
		t.Errorf("StatusCode = %d, want 200", route.Response["200"].StatusCode)
	}
	if route.Response["200"].BodyType != "app.User" {
		t.Errorf("BodyType = %q, want app.User", route.Response["200"].BodyType)
	}
	if len(route.Tags) != 1 || route.Tags[0] != "users" {
		t.Errorf("Tags = %v", route.Tags)
	}
	if !applier.HasOverride("app.h") || applier.HasOverride("app.other") {
		t.Error("HasOverride mismatch")
	}
}

func TestSweepResponseMatcherMatchNodeAndPriority(t *testing.T) {
	meta := exSweepMeta()
	cp := NewContextProvider(meta)
	cfg := &APISpecConfig{}

	t.Run("nil node", func(t *testing.T) {
		m := NewResponsePatternMatcher(ResponsePattern{CallRegex: "^JSON$"}, cfg, cp, nil)
		if m.MatchNode(nil) {
			t.Error("MatchNode(nil) = true")
		}
	})

	rejections := []struct {
		name    string
		pattern ResponsePattern
		edge    *metadata.CallGraphEdge
	}{
		{"call regex", ResponsePattern{CallRegex: "^JSON$"}, sweepEdge(meta, "h", "app", "Bind", "gin", "", "")},
		{"function name regex", ResponsePattern{CallRegex: "^JSON$", FunctionNameRegex: "^handle"}, sweepEdge(meta, "main", "app", "JSON", "gin", "", "")},
		{"exact recv type", ResponsePattern{RecvType: "gin.Context"}, sweepEdge(meta, "h", "app", "JSON", "other", "Ctx", "")},
	}
	for _, tt := range rejections {
		t.Run(tt.name, func(t *testing.T) {
			m := NewResponsePatternMatcher(tt.pattern, cfg, cp, nil)
			if m.MatchNode(sweepNode(tt.edge)) {
				t.Errorf("MatchNode() = true, want false")
			}
		})
	}

	t.Run("priority branches", func(t *testing.T) {
		m := NewResponsePatternMatcher(ResponsePattern{FunctionNameRegex: "y", RecvType: "z"}, cfg, cp, nil)
		if got := m.GetPriority(); got != 8 {
			t.Errorf("GetPriority() = %d, want 8", got)
		}
	})
}

func TestSweepExtractResponse(t *testing.T) {
	cfg := &APISpecConfig{Defaults: Defaults{ResponseContentType: "application/json"}}

	t.Run("unresolved status steps below the least existing", func(t *testing.T) {
		meta := exSweepMeta()
		m := NewResponsePatternMatcher(ResponsePattern{}, cfg, NewContextProvider(meta), nil)
		route := NewRouteInfo()
		route.Response["-1"] = &ResponseInfo{StatusCode: -1}
		node := sweepNode(sweepEdge(meta, "h", "app", "JSON", "gin", "", ""))
		if got := m.ExtractResponse(node, route); got != nil {
			t.Errorf("nothing resolved: got %+v, want nil", got)
		}
	})

	t.Run("body adopts the lowest bodyless status", func(t *testing.T) {
		meta := exSweepMeta()
		m := NewResponsePatternMatcher(ResponsePattern{TypeFromArg: true, TypeArgIndex: 0}, cfg, NewContextProvider(meta), nil)
		route := NewRouteInfo()
		route.Response["400"] = &ResponseInfo{StatusCode: 400}
		route.Response["200"] = &ResponseInfo{StatusCode: 200}
		arg := sweepIdent(meta, "u")
		arg.SetResolvedType("app.User")
		node := sweepNode(sweepEdge(meta, "h", "app", "JSON", "gin", "", "", arg))
		got := m.ExtractResponse(node, route)
		if len(got) != 1 || got[0].StatusCode != 200 || got[0].BodyType != "app.User" {
			t.Fatalf("got %+v, want one 200/app.User response", got)
		}
	})

	t.Run("no bodyless status leaves the unknown slot", func(t *testing.T) {
		meta := exSweepMeta()
		m := NewResponsePatternMatcher(ResponsePattern{TypeFromArg: true, TypeArgIndex: 0}, cfg, NewContextProvider(meta), nil)
		route := NewRouteInfo()
		route.Response["200"] = &ResponseInfo{StatusCode: 200, BodyType: "app.Existing"}
		arg := sweepIdent(meta, "u")
		arg.SetResolvedType("app.User")
		node := sweepNode(sweepEdge(meta, "h", "app", "JSON", "gin", "", "", arg))
		got := m.ExtractResponse(node, route)
		if len(got) != 1 || got[0].StatusCode >= 100 {
			t.Fatalf("got %+v, want an unknown-status response", got)
		}
	})
}

func TestSweepExpandStatusesFromIdent(t *testing.T) {
	cfg := &APISpecConfig{}

	t.Run("nil metadata", func(t *testing.T) {
		m := NewResponsePatternMatcher(ResponsePattern{}, cfg, NewContextProvider(nil), nil)
		meta := exSweepMeta()
		edge := sweepEdge(meta, "h", "app", "JSON", "gin", "", "")
		if got := m.expandStatusesFromIdent(sweepIdent(meta, "code"), edge); got != nil {
			t.Errorf("got %v, want nil", got)
		}
	})

	t.Run("caller function not found", func(t *testing.T) {
		meta := exSweepMeta()
		m := NewResponsePatternMatcher(ResponsePattern{}, cfg, NewContextProvider(meta), nil)
		edge := sweepEdge(meta, "h", "app", "JSON", "gin", "", "")
		if got := m.expandStatusesFromIdent(sweepIdent(meta, "code"), edge); got != nil {
			t.Errorf("got %v, want nil", got)
		}
	})

	t.Run("branched assignments expand to distinct statuses", func(t *testing.T) {
		meta := exSweepMeta()
		mkCallAssign := func(args ...*metadata.CallArgument) metadata.Assignment {
			call := metadata.NewCallArgument(meta)
			call.SetKind(metadata.KindCall)
			call.Fun = sweepIdent(meta, "respond")
			call.Args = args
			return metadata.Assignment{Value: *call}
		}
		nonCall := metadata.Assignment{Value: *sweepIdent(meta, "other")}
		meta.Packages = map[string]*metadata.Package{
			"app": {Files: map[string]*metadata.File{
				"h.go": {Functions: map[string]*metadata.Function{
					"h": {AssignmentMap: map[string][]metadata.Assignment{
						"code": {
							nonCall,
							mkCallAssign(nil, sweepLit(meta, "400")),
							mkCallAssign(sweepLit(meta, "400")),
							mkCallAssign(sweepLit(meta, "500")),
						},
					}},
				}},
			}},
		}
		m := NewResponsePatternMatcher(ResponsePattern{}, cfg, NewContextProvider(meta), nil)
		edge := sweepEdge(meta, "h", "app", "JSON", "gin", "", "")
		got := m.expandStatusesFromIdent(sweepIdent(meta, "code"), edge)
		if len(got) != 2 || got[0] != 400 || got[1] != 500 {
			t.Errorf("got %v, want [400 500]", got)
		}
	})
}

func TestSweepResponseResolveTypeOrigin(t *testing.T) {
	meta := exSweepMeta()
	m := NewResponsePatternMatcher(ResponsePattern{}, &APISpecConfig{}, NewContextProvider(meta), nil)

	t.Run("generic concrete resolution", func(t *testing.T) {
		arg := sweepIdent(meta, "v")
		arg.IsGenericType = true
		arg.GenericTypeName = meta.StringPool.Get("T")
		edge := sweepEdge(meta, "h", "app", "JSON", "gin", "", "")
		// Seed via the edge: TrackerNode.TypeParams() rebuilds its map on every
		// read, so a directly-set node.typeParamMap would be discarded.
		edge.TypeParamMap = map[string]string{"T": "app.Item"}
		node := sweepNode(edge)
		if got := m.resolveTypeOrigin(arg, node, "T"); got != "app.Item" {
			t.Errorf("got %q, want app.Item", got)
		}
	})

	t.Run("selector fast path", func(t *testing.T) {
		sel := metadata.NewCallArgument(meta)
		sel.SetKind(metadata.KindSelector)
		sel.SetType("string")
		node := sweepNode(sweepEdge(meta, "h", "app", "JSON", "gin", "", ""))
		if got := m.resolveTypeOrigin(sel, node, "orig"); got != "string" {
			t.Errorf("got %q, want string", got)
		}
	})

	t.Run("ident assignment resolution", func(t *testing.T) {
		arg := sweepIdent(meta, "v")
		edge := sweepEdge(meta, "h", "app", "JSON", "gin", "", "")
		edge.AssignmentMap = map[string][]metadata.Assignment{
			"v": {{ConcreteType: meta.StringPool.Get("app.User")}},
		}
		if got := m.resolveTypeOrigin(arg, sweepNode(edge), "any"); got != "app.User" {
			t.Errorf("got %q, want app.User", got)
		}
	})
}

func TestSweepConcreteFromEnclosingFunc(t *testing.T) {
	meta := sweepInterfaceMeta()
	pool := meta.StringPool
	appFile := meta.Packages["app"].Files["app/main.go"]
	appFile.Functions["handler"] = &metadata.Function{
		AssignmentMap: map[string][]metadata.Assignment{
			"a": {
				{ConcreteType: 0},                      // unset: skipped
				{ConcreteType: pool.Get("app.Animal")}, // interface: skipped
				{ConcreteType: pool.Get("app.Dog")},    // the single concrete
				{ConcreteType: pool.Get("app.Dog")},    // duplicate concrete: not ambiguous
			},
		},
	}
	appFile.Functions["ambiguous"] = &metadata.Function{
		AssignmentMap: map[string][]metadata.Assignment{
			"a": {
				{ConcreteType: pool.Get("app.Dog")},
				{ConcreteType: pool.Get("app.Cat")},
			},
		},
	}
	m := NewResponsePatternMatcher(ResponsePattern{}, &APISpecConfig{}, NewContextProvider(meta), nil)
	arg := sweepIdent(meta, "a")

	t.Run("nil edge", func(t *testing.T) {
		if got := m.concreteFromEnclosingFunc(arg, nil, "app.Animal"); got != "" {
			t.Errorf("got %q, want empty", got)
		}
	})
	t.Run("empty caller name", func(t *testing.T) {
		edge := sweepEdge(meta, "", "app", "Encode", "json", "", "")
		if got := m.concreteFromEnclosingFunc(arg, edge, "app.Animal"); got != "" {
			t.Errorf("got %q, want empty", got)
		}
	})
	t.Run("caller function not found", func(t *testing.T) {
		edge := sweepEdge(meta, "nosuch", "app", "Encode", "json", "", "")
		if got := m.concreteFromEnclosingFunc(arg, edge, "app.Animal"); got != "" {
			t.Errorf("got %q, want empty", got)
		}
	})
	t.Run("single concrete wins", func(t *testing.T) {
		edge := sweepEdge(meta, "handler", "app", "Encode", "json", "", "")
		if got := m.concreteFromEnclosingFunc(arg, edge, "app.Animal"); got != "app.Dog" {
			t.Errorf("got %q, want app.Dog", got)
		}
	})
	t.Run("ambiguous keeps the interface", func(t *testing.T) {
		edge := sweepEdge(meta, "ambiguous", "app", "Encode", "json", "", "")
		if got := m.concreteFromEnclosingFunc(arg, edge, "app.Animal"); got != "" {
			t.Errorf("got %q, want empty (honest over wrong)", got)
		}
	})
}

func TestSweepConcreteFromParamBinding(t *testing.T) {
	meta := sweepInterfaceMeta()
	m := NewResponsePatternMatcher(ResponsePattern{}, &APISpecConfig{}, NewContextProvider(meta), nil)
	arg := sweepIdent(meta, "v")

	newChain := func(paramArg *metadata.CallArgument, mapName string) *TrackerNode {
		edge := sweepEdge(meta, "writeAnimal", "app", "Encode", "json", "", "", arg)
		parentEdge := sweepEdge(meta, "handler", "app", "writeAnimal", "app", "", "")
		if paramArg != nil {
			parentEdge.ParamArgMap = map[string]metadata.CallArgument{mapName: *paramArg}
		} else {
			parentEdge.ParamArgMap = map[string]metadata.CallArgument{}
		}
		node := sweepNode(edge)
		node.Parent = sweepNode(parentEdge)
		return node
	}

	t.Run("nil edge", func(t *testing.T) {
		if got := m.concreteFromParamBinding(arg, &TrackerNode{}, "app.Animal"); got != "" {
			t.Errorf("got %q, want empty", got)
		}
	})
	t.Run("param not bound at the call site", func(t *testing.T) {
		if got := m.concreteFromParamBinding(arg, newChain(nil, ""), "app.Animal"); got != "" {
			t.Errorf("got %q, want empty", got)
		}
	})
	t.Run("interface-typed caller arg keeps the interface", func(t *testing.T) {
		ifaceArg := sweepIdent(meta, "a")
		ifaceArg.SetPkg("app")
		ifaceArg.SetType("app.Animal")
		if got := m.concreteFromParamBinding(arg, newChain(ifaceArg, "v"), "app.Animal"); got != "" {
			t.Errorf("got %q, want empty", got)
		}
	})
	t.Run("concrete caller arg resolves", func(t *testing.T) {
		dogArg := sweepIdent(meta, "d")
		dogArg.SetPkg("app")
		dogArg.SetType("app.Dog")
		got := m.concreteFromParamBinding(arg, newChain(dogArg, "v"), "app.Animal")
		if got == "" || !strings.Contains(got, "Dog") {
			t.Errorf("got %q, want the concrete Dog type", got)
		}
	})
}

func TestSweepConcreteFromCalleeReturn(t *testing.T) {
	meta := sweepInterfaceMeta()
	appFile := meta.Packages["app"].Files["app/main.go"]

	returnVar := func(typ string) metadata.CallArgument {
		a := metadata.NewCallArgument(meta)
		a.SetKind(metadata.KindIdent)
		a.SetName("r")
		a.SetPkg("app")
		a.SetType(typ)
		return *a
	}
	appFile.Functions["makeAnimal"] = &metadata.Function{
		ReturnVars: []metadata.CallArgument{returnVar("app.Animal"), returnVar("app.Dog")},
	}
	appFile.Functions["makeEither"] = &metadata.Function{
		ReturnVars: []metadata.CallArgument{returnVar("app.Dog"), returnVar("app.Cat")},
	}

	m := NewResponsePatternMatcher(ResponsePattern{}, &APISpecConfig{}, NewContextProvider(meta), nil)
	edge := sweepEdge(meta, "handler", "app", "Encode", "json", "", "")

	newCallArg := func(selName string) *metadata.CallArgument {
		call := metadata.NewCallArgument(meta)
		call.SetKind(metadata.KindCall)
		fun := metadata.NewCallArgument(meta)
		fun.SetKind(metadata.KindSelector)
		fun.SetPkg("app")
		fun.Sel = sweepIdent(meta, selName)
		call.Fun = fun
		return call
	}

	t.Run("nil Fun", func(t *testing.T) {
		bare := metadata.NewCallArgument(meta)
		bare.SetKind(metadata.KindCall)
		if got := m.concreteFromCalleeReturn(bare, edge, "app.Animal"); got != "" {
			t.Errorf("got %q, want empty", got)
		}
	})
	t.Run("callee not found", func(t *testing.T) {
		if got := m.concreteFromCalleeReturn(newCallArg("nosuch"), edge, "app.Animal"); got != "" {
			t.Errorf("got %q, want empty", got)
		}
	})
	t.Run("single concrete return resolves via selector name", func(t *testing.T) {
		got := m.concreteFromCalleeReturn(newCallArg("makeAnimal"), edge, "app.Animal")
		if got == "" || !strings.Contains(got, "Dog") {
			t.Errorf("got %q, want the concrete Dog type", got)
		}
	})
	t.Run("ambiguous returns keep the interface", func(t *testing.T) {
		if got := m.concreteFromCalleeReturn(newCallArg("makeEither"), edge, "app.Animal"); got != "" {
			t.Errorf("got %q, want empty (honest over wrong)", got)
		}
	})
}

func TestSweepParamMatcher(t *testing.T) {
	meta := exSweepMeta()
	cp := NewContextProvider(meta)
	cfg := &APISpecConfig{}

	t.Run("match node rejections", func(t *testing.T) {
		tests := []struct {
			name    string
			pattern ParamPattern
			node    TrackerNodeInterface
		}{
			{"nil node", ParamPattern{CallRegex: "^Param$"}, nil},
			{"call regex", ParamPattern{CallRegex: "^Param$"}, sweepNode(sweepEdge(meta, "h", "app", "Query", "gin", "", ""))},
			{"function name regex", ParamPattern{CallRegex: "^Param$", FunctionNameRegex: "^handle"}, sweepNode(sweepEdge(meta, "main", "app", "Param", "gin", "", ""))},
			{"recv type regex", ParamPattern{RecvTypeRegex: `^gin\.Context$`}, sweepNode(sweepEdge(meta, "h", "app", "Param", "other", "Ctx", ""))},
			{"exact recv type", ParamPattern{RecvType: "gin.Context"}, sweepNode(sweepEdge(meta, "h", "app", "Param", "other", "Ctx", ""))},
		}
		for _, tt := range tests {
			m := NewParamPatternMatcher(tt.pattern, cfg, cp, nil)
			if m.MatchNode(tt.node) {
				t.Errorf("%s: MatchNode() = true, want false", tt.name)
			}
		}
	})

	t.Run("priority branches", func(t *testing.T) {
		m := NewParamPatternMatcher(ParamPattern{CallRegex: "x", FunctionNameRegex: "y", RecvTypeRegex: "z"}, cfg, cp, nil)
		if got := m.GetPriority(); got != 18 {
			t.Errorf("GetPriority() = %d, want 18", got)
		}
	})

	t.Run("extract param with literal type arg", func(t *testing.T) {
		m := NewParamPatternMatcher(ParamPattern{ParamIn: "path", ParamArgIndex: 0, TypeFromArg: true, TypeArgIndex: 1}, cfg, cp, nil)
		edge := sweepEdge(meta, "h", "app", "Param", "gin", "", "", sweepLit(meta, `"id"`), sweepLit(meta, "42"))
		param := m.ExtractParam(sweepNode(edge), NewRouteInfo())
		if param == nil || param.Name != "id" || !param.Required {
			t.Fatalf("param = %+v, want required path param id", param)
		}
		if param.Schema == nil || param.Schema.Type != "integer" {
			t.Errorf("Schema = %+v, want integer from the literal", param.Schema)
		}
	})

	t.Run("extract param derefs resolved pointer type", func(t *testing.T) {
		m := NewParamPatternMatcher(ParamPattern{ParamIn: "query", ParamArgIndex: 0, TypeFromArg: true, TypeArgIndex: 1, Deref: true}, cfg, cp, nil)
		typed := sweepIdent(meta, "v")
		typed.SetResolvedType("*string")
		edge := sweepEdge(meta, "h", "app", "Query", "gin", "", "", sweepLit(meta, `"q"`), typed)
		param := m.ExtractParam(sweepNode(edge), NewRouteInfo())
		if param == nil || param.Schema == nil || param.Schema.Type != "string" {
			t.Fatalf("param = %+v, want string schema after deref", param)
		}
		if param.Required {
			t.Error("query params must not be forced required")
		}
	})
}

func TestSweepParamResolveTypeOrigin(t *testing.T) {
	meta := exSweepMeta()
	m := NewParamPatternMatcher(ParamPattern{}, &APISpecConfig{}, NewContextProvider(meta), nil)
	node := sweepNode(sweepEdge(meta, "h", "app", "Param", "gin", "", ""))

	t.Run("resolved type fast path", func(t *testing.T) {
		arg := sweepIdent(meta, "v")
		arg.SetResolvedType("int")
		if got := m.resolveTypeOrigin(arg, node, "orig"); got != "int" {
			t.Errorf("got %q, want int", got)
		}
	})
	t.Run("generic resolution", func(t *testing.T) {
		arg := sweepIdent(meta, "v")
		arg.IsGenericType = true
		arg.GenericTypeName = meta.StringPool.Get("T")
		edge := sweepEdge(meta, "h", "app", "Param", "gin", "", "")
		// Seed via the edge: TrackerNode.TypeParams() rebuilds its map on every
		// read, so a directly-set node.typeParamMap would be discarded.
		edge.TypeParamMap = map[string]string{"T": "int"}
		n := sweepNode(edge)
		if got := m.resolveTypeOrigin(arg, n, "T"); got != "int" {
			t.Errorf("got %q, want int", got)
		}
	})
	t.Run("selector fast path", func(t *testing.T) {
		sel := metadata.NewCallArgument(meta)
		sel.SetKind(metadata.KindSelector)
		sel.SetType("string")
		if got := m.resolveTypeOrigin(sel, node, "orig"); got != "string" {
			t.Errorf("got %q, want string", got)
		}
	})
	t.Run("ident assignment resolution", func(t *testing.T) {
		arg := sweepIdent(meta, "v")
		edge := sweepEdge(meta, "h", "app", "Param", "gin", "", "")
		edge.AssignmentMap = map[string][]metadata.Assignment{
			"v": {{ConcreteType: meta.StringPool.Get("uuid.UUID")}},
		}
		if got := m.resolveTypeOrigin(arg, sweepNode(edge), "string"); got != "uuid.UUID" {
			t.Errorf("got %q, want uuid.UUID", got)
		}
	})
	t.Run("fallthrough keeps original", func(t *testing.T) {
		if got := m.resolveTypeOrigin(sweepLit(meta, "1"), node, "int"); got != "int" {
			t.Errorf("got %q, want int", got)
		}
	})
}

func TestSweepPairAndFillResponses(t *testing.T) {
	meta := exSweepMeta()
	// handler -> helper edge gives the two callers distinct BFS depths.
	meta.CallGraph = []metadata.CallGraphEdge{
		*sweepEdge(meta, "handler", "app", "helper", "app", "", ""),
	}
	meta.BuildCallGraphMaps()

	cfg := &APISpecConfig{
		Framework: FrameworkConfig{
			ResponsePatterns: []ResponsePattern{{
				CallRegex: "^JSON$", StatusFromArg: true, StatusArgIndex: 0, TypeFromArg: true, TypeArgIndex: 1,
			}},
		},
		Defaults: Defaults{ResponseContentType: "application/json"},
	}
	tree := NewMockTrackerTree(meta, metadata.TrackerLimits{})
	ext := NewExtractor(tree, cfg)

	userArg := func() *metadata.CallArgument {
		a := sweepIdent(meta, "u")
		a.SetResolvedType("app.User")
		return a
	}
	errArg := func() *metadata.CallArgument {
		a := sweepIdent(meta, "e")
		a.SetResolvedType("app.Payload")
		return a
	}

	// Known status with body (line 20).
	n1 := sweepNode(sweepEdge(meta, "handler", "app", "JSON", "fiber", "Ctx", "h.go:20:3", sweepLit(meta, "200"), userArg()))
	// Sub-100 status and no body: nothing usable, must be dropped (line 15).
	n2 := sweepNode(sweepEdge(meta, "handler", "app", "JSON", "fiber", "Ctx", "h.go:15:2", sweepLit(meta, "42")))
	// Unknown status + body in the handler frame (depth 0): default slot.
	n3 := sweepNode(sweepEdge(meta, "handler", "app", "JSON", "fiber", "Ctx", "h.go:5:1", sweepIdent(meta, "code"), userArg()))
	// Unknown status + body two hops down (depth 1): outbound payload, dropped.
	n4 := sweepNode(sweepEdge(meta, "helper", "app", "JSON", "fiber", "Ctx", "z.go:2:1", sweepIdent(meta, "code"), errArg()))

	route := &RouteInfo{
		Function:  "app.handler",
		Metadata:  meta,
		Response:  map[string]*ResponseInfo{},
		UsedTypes: map[string]*Schema{},
	}
	candidates := []responseCandidate{
		{node: n1, chain: ""},
		{node: n2, chain: ""},
		{node: n3, chain: ""},
		{node: n4, chain: ""},
	}
	ext.pairAndFillResponses(route, candidates)

	if len(route.Response) != 2 {
		t.Fatalf("Response = %+v, want exactly the 200 and default slots", route.Response)
	}
	if got := route.Response["200"]; got == nil || got.BodyType != "app.User" {
		t.Errorf("200 slot = %+v, want app.User", got)
	}
	if got := route.Response["-1"]; got == nil || got.BodyType != "app.User" {
		t.Errorf("default slot = %+v, want the shallow app.User body", got)
	}
	for slot := range route.Response {
		if slot == "42" {
			t.Error("sub-100 bodyless fragment must be dropped")
		}
	}
}

func TestSweepAccessorKeyRecovery(t *testing.T) {
	newMuxExtractor := func(meta *metadata.Metadata) *Extractor {
		cfg := &APISpecConfig{Framework: FrameworkConfig{ParamPatterns: []ParamPattern{{
			CallRegex:      "^Vars$",
			RecvTypeRegex:  `github\.com/gorilla/mux`,
			NameFromMapKey: true,
			ParamIn:        "path",
		}}}}
		return NewExtractor(NewMockTrackerTree(meta, metadata.TrackerLimits{}), cfg)
	}

	t.Run("nil-guards", func(t *testing.T) {
		meta := exSweepMeta()
		ext := newMuxExtractor(meta)
		ext.recordPathVarKeyMismatches(nil)
		ext.recordPathVarKeyMismatches(&RouteInfo{Metadata: meta}) // no Function
		ext.completeMapKeyPathParams(nil)
		if len(ext.PathParamMismatches()) != 0 {
			t.Errorf("mismatches = %v, want none", ext.PathParamMismatches())
		}
	})

	t.Run("invalid accessor regex recovers nothing", func(t *testing.T) {
		meta := exSweepMeta()
		meta.Packages = map[string]*metadata.Package{
			"app": {Files: map[string]*metadata.File{"h.go": {Functions: map[string]*metadata.Function{"h": {}}}}},
		}
		ext := newMuxExtractor(meta)
		route := &RouteInfo{Function: "app.h", Package: "app", Metadata: meta}
		keys := ext.recoverAccessorKeys(route, ParamPattern{CallRegex: "("})
		if keys != nil {
			t.Errorf("keys = %v, want nil for invalid regex", keys)
		}
	})

	t.Run("mismatched key recorded once", func(t *testing.T) {
		meta := exSweepMeta()
		indexExpr := metadata.NewCallArgument(meta)
		indexExpr.SetKind(metadata.KindIndex)
		indexExpr.X = sweepIdent(meta, "vars")
		lit := metadata.NewCallArgument(meta)
		lit.SetKind(metadata.KindLiteral)
		lit.SetValue(`"userId"`)
		indexExpr.Fun = lit

		fn := &metadata.Function{AssignmentMap: map[string][]metadata.Assignment{
			"vars": {{CalleeFunc: "Vars", CalleePkg: "github.com/gorilla/mux", Value: *sweepIdent(meta, "r")}},
			"id":   {{Value: *indexExpr}},
		}}
		meta.Packages = map[string]*metadata.Package{
			"app": {Files: map[string]*metadata.File{"h.go": {Functions: map[string]*metadata.Function{"h": fn}}}},
		}
		ext := newMuxExtractor(meta)
		route := &RouteInfo{Function: "app.h", Package: "app", Metadata: meta, Method: "GET", Path: "/users/{id}"}
		ext.recordPathVarKeyMismatches(route)
		ext.recordPathVarKeyMismatches(route) // second pass must dedupe
		mismatches := ext.PathParamMismatches()
		if len(mismatches) != 1 || mismatches[0].Key != "userId" {
			t.Fatalf("mismatches = %+v, want exactly one userId entry", mismatches)
		}
	})
}

func TestSweepIsAccessorCall(t *testing.T) {
	meta := exSweepMeta()
	callRe, _ := cachedRegex("^Vars$")
	recvRe, _ := cachedRegex("mux")

	newCall := func(kind string, fun *metadata.CallArgument) *metadata.CallArgument {
		a := metadata.NewCallArgument(meta)
		a.SetKind(kind)
		a.Fun = fun
		return a
	}
	selFun := func(pkg, name string) *metadata.CallArgument {
		f := metadata.NewCallArgument(meta)
		f.SetKind(metadata.KindSelector)
		f.Sel = sweepIdent(meta, name)
		f.Sel.SetPkg(pkg)
		return f
	}

	tests := []struct {
		name string
		x    *metadata.CallArgument
		want bool
	}{
		{"nil", nil, false},
		{"not a call", sweepIdent(meta, "vars"), false},
		{"selector accessor matches", newCall(metadata.KindCall, selFun("github.com/gorilla/mux", "Vars")), true},
		{"call name mismatch", newCall(metadata.KindCall, selFun("github.com/gorilla/mux", "Other")), false},
		{"pkg mismatch", newCall(metadata.KindCall, selFun("net/http", "Vars")), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isAccessorCall(tt.x, callRe, recvRe); got != tt.want {
				t.Errorf("isAccessorCall() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSweepFindFunctionByName(t *testing.T) {
	meta := exSweepMeta()
	fn := &metadata.Function{}
	meta.Packages = map[string]*metadata.Package{
		"a":   {Files: map[string]*metadata.File{"a.go": {Functions: map[string]*metadata.Function{}}}},
		"lib": {Files: map[string]*metadata.File{"l.go": {Functions: map[string]*metadata.Function{"helper": fn}}}},
	}

	if findFunctionByName(nil, "a", "x") != nil {
		t.Error("nil meta must yield nil")
	}
	if findFunctionByName(meta, "a", "") != nil {
		t.Error("empty name must yield nil")
	}
	if got := findFunctionByName(meta, "lib", "helper"); got != fn {
		t.Error("direct package lookup failed")
	}
	// Fallback: the named package doesn't declare it, another one does.
	if got := findFunctionByName(meta, "a", "helper"); got != fn {
		t.Error("fallback lookup across packages failed")
	}
	if findFunctionByName(meta, "a", "nosuch") != nil {
		t.Error("unknown name must yield nil")
	}
}

func TestSweepHandlerReachesAccessorAndEdgeMatch(t *testing.T) {
	pattern := ParamPattern{CallRegex: "^Vars$", RecvTypeRegex: "mux"}

	t.Run("guards", func(t *testing.T) {
		meta := exSweepMeta()
		ext := NewExtractor(NewMockTrackerTree(meta, metadata.TrackerLimits{}), &APISpecConfig{})
		if ext.handlerReachesAccessor(&RouteInfo{Function: "app.h"}, pattern) {
			t.Error("nil metadata must not reach")
		}
		if ext.handlerReachesAccessor(&RouteInfo{Metadata: meta}, pattern) {
			t.Error("empty function must not reach")
		}
	})

	t.Run("package mismatch does not reach", func(t *testing.T) {
		meta := exSweepMeta()
		meta.CallGraph = []metadata.CallGraphEdge{
			// Same bare name, wrong package: must be skipped.
			*sweepEdge(meta, "h", "other", "Vars", "github.com/gorilla/mux", "", ""),
		}
		meta.BuildCallGraphMaps()
		ext := NewExtractor(NewMockTrackerTree(meta, metadata.TrackerLimits{}), &APISpecConfig{})
		route := &RouteInfo{Function: "app.h", Package: "app", Metadata: meta}
		if ext.handlerReachesAccessor(route, pattern) {
			t.Error("caller in another package must not satisfy reachability")
		}
	})

	t.Run("edgeMatchesAccessor recv-only and mismatch", func(t *testing.T) {
		meta := exSweepMeta()
		// RecvType set with empty pkg exercises the fq = recvType branch.
		edge := sweepEdge(meta, "h", "app", "Vars", "", "muxRouter", "")
		if !edgeMatchesAccessor(meta, edge, pattern) {
			t.Error("recv-only fq type should match the mux regex")
		}
		other := sweepEdge(meta, "h", "app", "Vars", "", "chiRouter", "")
		if edgeMatchesAccessor(meta, other, pattern) {
			t.Error("non-mux receiver must not match")
		}
	})
}
