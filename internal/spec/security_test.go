package spec

import (
	"strings"
	"testing"

	"github.com/ehabterra/apispec/internal/metadata"
	"gopkg.in/yaml.v3"
)

// mkIdentPkg builds an ident CallArgument with a name and package.
func mkIdentPkg(meta *metadata.Metadata, name, pkg string) *metadata.CallArgument {
	a := mkIdent(meta, name, "")
	if pkg != "" {
		a.Pkg = meta.StringPool.Get(pkg)
	}
	return a
}

func TestMiddlewareRefFromArg(t *testing.T) {
	meta := newTestMeta()

	t.Run("ident", func(t *testing.T) {
		arg := mkIdentPkg(meta, "customMiddleware", "complex-chi-router")
		ref, ok := middlewareRefFromArg(arg)
		if !ok {
			t.Fatal("expected ok")
		}
		if ref.FunctionName != "customMiddleware" || ref.Pkg != "complex-chi-router" || ref.RecvType != "" {
			t.Errorf("got %+v", ref)
		}
	})

	t.Run("selector method value", func(t *testing.T) {
		// h.authMiddleware where h has type Handler in pkg .../handler.
		x := mkIdent(meta, "h", "")
		sel := mkIdentPkg(meta, "authMiddleware", "complex-chi-router/handler")
		arg := mkSelector(meta, x, sel)
		arg.ReceiverType = mkIdent(meta, "Handler", "")

		ref, ok := middlewareRefFromArg(arg)
		if !ok {
			t.Fatal("expected ok")
		}
		if ref.FunctionName != "authMiddleware" || ref.Pkg != "complex-chi-router/handler" || ref.RecvType != "Handler" {
			t.Errorf("got %+v", ref)
		}
	})

	t.Run("constructor call", func(t *testing.T) {
		// middleware.Timeout(...) -> Fun is selector(middleware, Timeout).
		x := mkIdent(meta, "middleware", "")
		fnSel := mkIdentPkg(meta, "Timeout", "github.com/go-chi/chi/v5/middleware")
		call := mkMethodCall(meta, x, fnSel)

		ref, ok := middlewareRefFromArg(call)
		if !ok {
			t.Fatal("expected ok")
		}
		if ref.FunctionName != "Timeout" || ref.Pkg != "github.com/go-chi/chi/v5/middleware" {
			t.Errorf("got %+v", ref)
		}
	})

	t.Run("nil arg", func(t *testing.T) {
		if _, ok := middlewareRefFromArg(nil); ok {
			t.Error("expected not ok for nil")
		}
	})

	t.Run("empty ident", func(t *testing.T) {
		if _, ok := middlewareRefFromArg(mkIdent(meta, "", "")); ok {
			t.Error("expected not ok for empty ident")
		}
	})
}

// TestExpandMiddlewareRefs_WrapperLookThrough verifies a custom wrapper resolves
// to the library scheme by following the call graph: middleware.Protected()
// (no direct mapping) -> jwtware.New (mapped to bearerAuth).
func TestExpandMiddlewareRefs_WrapperLookThrough(t *testing.T) {
	meta := &metadata.Metadata{StringPool: metadata.NewStringPool()}
	tree := NewTrackerTree(meta, metadata.TrackerLimits{
		MaxNodesPerTree: 1000, MaxChildrenPerNode: 100, MaxArgsPerFunction: 50,
		MaxNestedArgsDepth: 50, MaxRecursionDepth: 100,
	}, nil)

	// Synthetic call graph: app/mw.Protected's body calls jwtware.New.
	calleeNew := metadata.Call{
		Name: meta.StringPool.Get("New"),
		Pkg:  meta.StringPool.Get("github.com/gofiber/contrib/jwt"),
	}
	calleeNew.RecvType = -1
	edge := &metadata.CallGraphEdge{Callee: calleeNew}
	meta.Callers = map[string][]*metadata.CallGraphEdge{
		"app/mw.Protected": {edge},
	}

	cfg := &APISpecConfig{
		SecurityMappings: []SecurityMapping{
			{FunctionNameRegex: `^New$`, PkgRegex: `gofiber/contrib/jwt`, Schemes: []SecurityRequirement{{"bearerAuth": {}}}},
		},
	}
	e := NewExtractor(tree, cfg)

	got := e.expandMiddlewareRefs([]MiddlewareRef{{FunctionName: "Protected", Pkg: "app/mw"}})

	// The wrapper should have been replaced by the library ref that maps.
	reqs, _, unresolved := resolveSecurity(got, cfg.SecurityMappings)
	if !reqHasScheme(reqs, "bearerAuth") {
		t.Errorf("wrapper did not resolve to bearerAuth via look-through: got refs=%+v reqs=%+v", got, reqs)
	}
	if len(unresolved) != 0 {
		t.Errorf("expected no unresolved after look-through, got %+v", unresolved)
	}
}

// TestExpandMiddlewareRefs_NoMatchKeepsOriginal verifies a wrapper that calls no
// known library is left intact (and thus reported unresolved).
func TestExpandMiddlewareRefs_NoMatchKeepsOriginal(t *testing.T) {
	meta := &metadata.Metadata{StringPool: metadata.NewStringPool()}
	tree := NewTrackerTree(meta, metadata.TrackerLimits{
		MaxNodesPerTree: 1000, MaxChildrenPerNode: 100, MaxArgsPerFunction: 50,
		MaxNestedArgsDepth: 50, MaxRecursionDepth: 100,
	}, nil)
	calleeLog := metadata.Call{Name: meta.StringPool.Get("Println"), Pkg: meta.StringPool.Get("log")}
	calleeLog.RecvType = -1
	meta.Callers = map[string][]*metadata.CallGraphEdge{
		"app/mw.CustomAuth": {{Callee: calleeLog}},
	}
	cfg := &APISpecConfig{
		SecurityMappings: []SecurityMapping{
			{FunctionNameRegex: `^New$`, PkgRegex: `gofiber/contrib/jwt`, Schemes: []SecurityRequirement{{"bearerAuth": {}}}},
		},
	}
	e := NewExtractor(tree, cfg)

	in := []MiddlewareRef{{FunctionName: "CustomAuth", Pkg: "app/mw"}}
	got := e.expandMiddlewareRefs(in)
	if len(got) != 1 || got[0].FunctionName != "CustomAuth" {
		t.Errorf("expected original ref kept when nothing maps, got %+v", got)
	}
}

// TestCollectChainSecurity verifies chi-style With-chain resolution: the
// middleware on r.With(mw).Get(...) is found via the route edge's ChainParent
// and guards only that route.
func TestCollectChainSecurity(t *testing.T) {
	meta := &metadata.Metadata{StringPool: metadata.NewStringPool()}
	tree := NewTrackerTree(meta, metadata.TrackerLimits{
		MaxNodesPerTree: 1000, MaxChildrenPerNode: 100, MaxArgsPerFunction: 50,
		MaxNestedArgsDepth: 50, MaxRecursionDepth: 100,
	}, nil)

	// With(authmw) edge: callee With on *R, middleware arg = ident authmw.
	withEdge := &metadata.CallGraphEdge{
		Callee: metadata.Call{
			Name:     meta.StringPool.Get("With"),
			Pkg:      meta.StringPool.Get("app"),
			RecvType: meta.StringPool.Get("R"),
		},
		Args: []*metadata.CallArgument{mkIdentPkg(meta, "authmw", "app")},
	}
	// The route (Get) edge chains off the With edge.
	routeEdge := &metadata.CallGraphEdge{ChainParent: withEdge}

	cfg := &APISpecConfig{
		Framework: FrameworkConfig{SecurityPatterns: []SecurityPattern{
			{CallRegex: `^With$`, Scope: SecurityScopeRoute, MiddlewareArgIndex: 0, MiddlewareVariadic: true, RecvTypeRegex: `R$`},
		}},
	}
	e := NewExtractor(tree, cfg)

	t.Run("chained route picks up With middleware", func(t *testing.T) {
		got := e.collectChainSecurity(&fakeNode{edge: routeEdge})
		if len(got) != 1 || got[0].FunctionName != "authmw" {
			t.Fatalf("expected [authmw] from chain parent, got %+v", got)
		}
	})

	t.Run("sibling route with no chain parent gets nothing", func(t *testing.T) {
		got := e.collectChainSecurity(&fakeNode{edge: &metadata.CallGraphEdge{}})
		if len(got) != 0 {
			t.Errorf("expected no middleware for non-chained route, got %+v", got)
		}
	})
}

func TestSecurityMappingMatches(t *testing.T) {
	ref := MiddlewareRef{FunctionName: "authMiddleware", Pkg: "app/handler", RecvType: "Handler"}

	tests := []struct {
		name string
		m    SecurityMapping
		want bool
	}{
		{"function match", SecurityMapping{FunctionNameRegex: "^authMiddleware$"}, true},
		{"function mismatch", SecurityMapping{FunctionNameRegex: "^other$"}, false},
		{"pkg match", SecurityMapping{PkgRegex: "app/.*"}, true},
		{"pkg mismatch", SecurityMapping{PkgRegex: "vendor/.*"}, false},
		{"recv match", SecurityMapping{RecvTypeRegex: "Handler"}, true},
		{"recv mismatch", SecurityMapping{RecvTypeRegex: "Service"}, false},
		{"all three match", SecurityMapping{FunctionNameRegex: "auth", PkgRegex: "app", RecvTypeRegex: "Handler"}, true},
		{"one of three fails", SecurityMapping{FunctionNameRegex: "auth", PkgRegex: "app", RecvTypeRegex: "Service"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.m.matches(ref); got != tt.want {
				t.Errorf("matches() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestResolveSecurity(t *testing.T) {
	bearer := SecurityMapping{FunctionNameRegex: "^authMiddleware$", Schemes: []SecurityRequirement{{"bearerAuth": {}}}}
	apiKey := SecurityMapping{FunctionNameRegex: "^apiKeyAuth$", Schemes: []SecurityRequirement{{"apiKeyAuth": {}}}}
	public := SecurityMapping{FunctionNameRegex: "^AllowPublic$", Public: true}
	orMap := SecurityMapping{
		FunctionNameRegex: "^eitherAuth$",
		SchemesAnyOf:      [][]SecurityRequirement{{{"bearerAuth": {}}}, {{"apiKeyAuth": {}}}},
	}
	mappings := []SecurityMapping{bearer, apiKey, public, orMap}

	t.Run("single scheme", func(t *testing.T) {
		reqs, pub, unresolved := resolveSecurity([]MiddlewareRef{{FunctionName: "authMiddleware"}}, mappings)
		if pub || len(unresolved) != 0 {
			t.Fatalf("pub=%v unresolved=%v", pub, unresolved)
		}
		if len(reqs) != 1 || !reqHasScheme(reqs, "bearerAuth") {
			t.Fatalf("got %+v", reqs)
		}
	})

	t.Run("AND merge across two middleware", func(t *testing.T) {
		reqs, _, _ := resolveSecurity([]MiddlewareRef{{FunctionName: "authMiddleware"}, {FunctionName: "apiKeyAuth"}}, mappings)
		if len(reqs) != 1 {
			t.Fatalf("expected one merged requirement object, got %+v", reqs)
		}
		if _, ok := reqs[0]["bearerAuth"]; !ok {
			t.Errorf("missing bearerAuth in %+v", reqs[0])
		}
		if _, ok := reqs[0]["apiKeyAuth"]; !ok {
			t.Errorf("missing apiKeyAuth in %+v", reqs[0])
		}
	})

	t.Run("OR alternatives", func(t *testing.T) {
		reqs, _, _ := resolveSecurity([]MiddlewareRef{{FunctionName: "eitherAuth"}}, mappings)
		if len(reqs) != 2 {
			t.Fatalf("expected two alternative requirement objects, got %+v", reqs)
		}
	})

	t.Run("public", func(t *testing.T) {
		_, pub, _ := resolveSecurity([]MiddlewareRef{{FunctionName: "AllowPublic"}}, mappings)
		if !pub {
			t.Error("expected public")
		}
	})

	t.Run("unresolved", func(t *testing.T) {
		reqs, _, unresolved := resolveSecurity([]MiddlewareRef{{FunctionName: "mysteryMW", Pkg: "app"}}, mappings)
		if len(reqs) != 0 {
			t.Errorf("expected no reqs, got %+v", reqs)
		}
		if len(unresolved) != 1 || unresolved[0].FunctionName != "mysteryMW" {
			t.Fatalf("expected one unresolved, got %+v", unresolved)
		}
	})

	t.Run("dedup identical requirements", func(t *testing.T) {
		reqs, _, _ := resolveSecurity([]MiddlewareRef{{FunctionName: "authMiddleware"}, {FunctionName: "authMiddleware"}}, mappings)
		if len(reqs) != 1 {
			t.Errorf("expected dedup to one, got %+v", reqs)
		}
	})

	t.Run("skip is resolved without scheme or unresolved", func(t *testing.T) {
		skip := SecurityMapping{FunctionNameRegex: "^Logger$", Skip: true}
		ms := []SecurityMapping{bearer, skip}
		reqs, pub, unresolved := resolveSecurity(
			[]MiddlewareRef{{FunctionName: "Logger"}, {FunctionName: "authMiddleware"}}, ms)
		if pub {
			t.Error("skip must not make the scope public")
		}
		if len(unresolved) != 0 {
			t.Fatalf("skip must not be reported unresolved, got %+v", unresolved)
		}
		// Only the bearer middleware contributes a scheme; Logger emits nothing.
		if len(reqs) != 1 || !reqHasScheme(reqs, "bearerAuth") {
			t.Fatalf("got %+v", reqs)
		}
		if reqHasScheme(reqs, "Logger") {
			t.Error("skip middleware must not emit a scheme")
		}
	})
}

// TestOperationSecurityRendering pins the three render states of per-operation
// security: nil omits the field (inherit global), a non-nil empty slice renders
// `security: []` (explicit public), and a non-empty slice renders the list.
func TestOperationSecurityRendering(t *testing.T) {
	routes := []*RouteInfo{
		{Path: "/inherit", Method: "GET", Function: "a", Security: nil},
		{Path: "/public", Method: "GET", Function: "b", Security: []SecurityRequirement{}},
		{Path: "/protected", Method: "GET", Function: "c", Security: []SecurityRequirement{{"bearerAuth": {}}}},
	}
	paths := buildPathsFromRoutes(routes)
	out, err := yaml.Marshal(paths)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := string(out)

	// /inherit: no security key under its GET.
	if i := strings.Index(got, "/inherit"); i >= 0 {
		seg := got[i:]
		if j := strings.Index(seg, "/p"); j >= 0 { // up to the next path
			seg = seg[:j]
		}
		if strings.Contains(seg, "security:") {
			t.Errorf("/inherit should omit security; got:\n%s", seg)
		}
	}
	// /public: explicit empty array.
	if !strings.Contains(got, "security: []") {
		t.Errorf("/public should render `security: []`; got:\n%s", got)
	}
	// /protected: the scheme listed.
	if !strings.Contains(got, "bearerAuth: []") {
		t.Errorf("/protected should list bearerAuth; got:\n%s", got)
	}
}
