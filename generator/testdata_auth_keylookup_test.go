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

// schemeOf returns the security scheme documented under name.
func schemeOf(t *testing.T, out *spec.OpenAPISpec, name string) intspec.SecurityScheme {
	t.Helper()
	if out.Components == nil {
		t.Fatal("no components")
	}
	scheme, ok := out.Components.SecuritySchemes[name]
	if !ok {
		have := make([]string, 0, len(out.Components.SecuritySchemes))
		for k := range out.Components.SecuritySchemes {
			have = append(have, k)
		}
		t.Fatalf("security scheme %q missing; have %v", name, have)
	}
	return scheme
}

// requirementNames returns the scheme names an operation requires.
func requirementNames(t *testing.T, out *spec.OpenAPISpec, path, method string) []string {
	t.Helper()
	item, ok := out.Paths[path]
	if !ok {
		t.Fatalf("path %q missing; have %v", path, mapPathKeys(out.Paths))
	}
	op := opFor(item, method)
	if op == nil {
		t.Fatalf("%s %s missing", method, path)
	}
	var names []string
	if op.Security == nil {
		return names
	}
	for _, req := range *op.Security {
		for name := range req {
			names = append(names, name)
		}
	}
	return names
}

// TestTestdata_AuthEchoKeyLookup locks in that an apiKey scheme documents where
// the credential actually travels (issue #370). Echo's KeyAuth defaults to
// `header:Authorization`; a group that configures otherwise used to be
// documented at that default, so a generated client sent the key to a place the
// server does not read.
//
// The fixture's three groups must stay three schemes: two configured
// differently, and one left at the default that must not inherit either.
func TestTestdata_AuthEchoKeyLookup(t *testing.T) {
	out := loadTestdataWithFixtureConfig(t, "auth_echo_keylookup", spec.DefaultEchoConfig())
	noDanglingRefs(t, out)

	cases := []struct {
		path, method, scheme, in, name string
	}{
		{"/api/items", "GET", "apiKeyAuthQueryApiKey", "query", "api_key"},
		{"/session/me", "GET", "apiKeyAuthCookieToken", "cookie", "token"},
		// Unconfigured: the library default is the right answer, and it must not
		// be redefined by the groups that configured one.
		{"/default/plain", "GET", "apiKeyAuth", "header", "Authorization"},
	}
	for _, c := range cases {
		names := requirementNames(t, out, c.path, c.method)
		if len(names) != 1 || names[0] != c.scheme {
			t.Errorf("%s %s requires %v, want [%s]", c.method, c.path, names, c.scheme)
			continue
		}
		scheme := schemeOf(t, out, c.scheme)
		if scheme.Type != "apiKey" || scheme.In != c.in || scheme.Name != c.name {
			t.Errorf("scheme %s = {type:%s in:%s name:%s}, want {apiKey %s %s}",
				c.scheme, scheme.Type, scheme.In, scheme.Name, c.in, c.name)
		}
	}

	// The open route stays open.
	if item, ok := out.Paths["/health"]; ok {
		if op := opFor(item, "GET"); op != nil && op.Security != nil && len(*op.Security) > 0 {
			t.Errorf("GET /health should carry no security, got %v", *op.Security)
		}
	}
}

// TestTestdata_AuthFiberKeyLookup is the same for fiber's keyauth, which shares
// the KeyLookup grammar: a custom header name is documented, not the default.
// One configured shape and nothing at the default, so the plain scheme name
// takes it rather than being split.
func TestTestdata_AuthFiberKeyLookup(t *testing.T) {
	out := loadTestdataWithFixtureConfig(t, "auth_fiber_keylookup", spec.DefaultFiberConfig())
	noDanglingRefs(t, out)

	names := requirementNames(t, out, "/api/items", "GET")
	if len(names) != 1 || names[0] != "apiKeyAuth" {
		t.Fatalf("GET /api/items requires %v, want [apiKeyAuth]", names)
	}
	scheme := schemeOf(t, out, "apiKeyAuth")
	if scheme.Type != "apiKey" || scheme.In != "header" || scheme.Name != "X-API-Key" {
		t.Errorf("scheme apiKeyAuth = {type:%s in:%s name:%s}, want {apiKey header X-API-Key}",
			scheme.Type, scheme.In, scheme.Name)
	}
}
