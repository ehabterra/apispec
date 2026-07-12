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

package generator

import (
	"testing"

	intspec "github.com/ehabterra/apispec/internal/spec"
	"github.com/ehabterra/apispec/spec"
)

// opFor returns the Operation for an uppercase HTTP method on a PathItem, or
// nil if that method is not declared. Keeps the auth table below able to name
// its method as a plain string.
func opFor(item intspec.PathItem, method string) *intspec.Operation {
	switch method {
	case "GET":
		return item.Get
	case "POST":
		return item.Post
	case "PUT":
		return item.Put
	case "DELETE":
		return item.Delete
	case "PATCH":
		return item.Patch
	case "OPTIONS":
		return item.Options
	case "HEAD":
		return item.Head
	default:
		return nil
	}
}

// hasSecurityScheme reports whether reqs contains a requirement naming scheme.
func hasSecurityScheme(reqs *[]spec.SecurityRequirement, scheme string) bool {
	if reqs == nil {
		return false
	}
	for _, r := range *reqs {
		if _, ok := r[scheme]; ok {
			return true
		}
	}
	return false
}

// TestTestdata_AuthPresets locks in the built-in auth/security presets across
// every framework's untested wiring style. Each fixture imports golang-jwt, so
// the engine's import detector (ApplySecurityPresets) must attach bearerAuth to
// the guarded route(s) and leave the sibling open route untouched. The route +
// scheme expectations mirror each fixture's committed openapi-7.53.yaml snapshot
// that scripts/compare-spec.sh regenerates; this promotes those snapshots into
// automated coverage.
//
// The middleware-reach dimension differs per fixture and is the point of each:
//   - auth_chi_with     chi r.With(mw).Get(...)   — route scope (chained only)
//   - auth_echo_group   echo e.Group("/api", mw)  — subtree scope
//   - auth_fiber_group  fiber app.Group("/api",mw)— subtree scope
//   - auth_gin_perroute gin r.GET(path, mw, h)    — per-route middleware
//   - auth_mux_subrouter gorilla sub.Use(mw)      — subrouter (router) scope
//   - auth_nethttp_wrap  mux.Handle(p, auth(h))   — handler-wrapper scope
func TestTestdata_AuthPresets(t *testing.T) {
	cases := []struct {
		name      string
		cfg       func() *spec.APISpecConfig
		protected struct{ method, path string }
		open      struct{ method, path string }
	}{
		{
			name:      "auth_chi_with",
			cfg:       spec.DefaultChiConfig,
			protected: struct{ method, path string }{"GET", "/users/{id}"},
			open:      struct{ method, path string }{"GET", "/health"},
		},
		{
			name:      "auth_echo_group",
			cfg:       spec.DefaultEchoConfig,
			protected: struct{ method, path string }{"GET", "/api/me"},
			open:      struct{ method, path string }{"GET", "/health"},
		},
		{
			name:      "auth_fiber_group",
			cfg:       spec.DefaultFiberConfig,
			protected: struct{ method, path string }{"GET", "/api/me"},
			open:      struct{ method, path string }{"GET", "/health"},
		},
		{
			name:      "auth_gin_perroute",
			cfg:       spec.DefaultGinConfig,
			protected: struct{ method, path string }{"GET", "/users/{id}"},
			open:      struct{ method, path string }{"GET", "/health"},
		},
		{
			name:      "auth_mux_subrouter",
			cfg:       spec.DefaultMuxConfig,
			protected: struct{ method, path string }{"POST", "/api/me"},
			open:      struct{ method, path string }{"POST", "/health"},
		},
		{
			name:      "auth_nethttp_wrap",
			cfg:       spec.DefaultHTTPConfig,
			protected: struct{ method, path string }{"GET", "/users/{id}"},
			open:      struct{ method, path string }{"GET", "/health"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := loadTestdata(t, tc.name, tc.cfg())
			noDanglingRefs(t, out)

			// Protected route: present and carrying bearerAuth.
			pItem, ok := out.Paths[tc.protected.path]
			if !ok {
				t.Fatalf("%s %s missing; have %v", tc.protected.method, tc.protected.path, mapPathKeys(out.Paths))
			}
			pOp := opFor(pItem, tc.protected.method)
			if pOp == nil {
				t.Fatalf("%s %s: no %s operation", tc.protected.method, tc.protected.path, tc.protected.method)
			}
			if !hasSecurityScheme(pOp.Security, "bearerAuth") {
				t.Errorf("%s %s: expected bearerAuth, got security=%v",
					tc.protected.method, tc.protected.path, pOp.Security)
			}

			// Open sibling: present but with no security requirement.
			oItem, ok := out.Paths[tc.open.path]
			if !ok {
				t.Fatalf("%s %s missing; have %v", tc.open.method, tc.open.path, mapPathKeys(out.Paths))
			}
			oOp := opFor(oItem, tc.open.method)
			if oOp == nil {
				t.Fatalf("%s %s: no %s operation", tc.open.method, tc.open.path, tc.open.method)
			}
			if oOp.Security != nil {
				t.Errorf("%s %s: expected no security (sibling of guarded route), got %v",
					tc.open.method, tc.open.path, *oOp.Security)
			}

			// The bearerAuth scheme must be catalogued in components.
			if out.Components == nil || out.Components.SecuritySchemes == nil {
				t.Fatal("expected components.securitySchemes to be populated")
			}
			if _, ok := out.Components.SecuritySchemes["bearerAuth"]; !ok {
				t.Errorf("bearerAuth scheme missing from components; have %v",
					securitySchemeKeys(out))
			}
		})
	}
}

func securitySchemeKeys(out *spec.OpenAPISpec) []string {
	if out.Components == nil {
		return nil
	}
	keys := make([]string, 0, len(out.Components.SecuritySchemes))
	for k := range out.Components.SecuritySchemes {
		keys = append(keys, k)
	}
	return keys
}
