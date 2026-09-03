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
	"path/filepath"
	"sort"
	"testing"

	"github.com/ehabterra/apispec/internal/engine"
	"github.com/ehabterra/apispec/spec"
)

// headerParamsOf returns the header parameter names an operation declares.
func headerParamsOf(t *testing.T, out *spec.OpenAPISpec, path, method string) []string {
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
	for _, p := range op.Parameters {
		if p.In == "header" {
			names = append(names, p.Name)
		}
	}
	sort.Strings(names)
	return names
}

// TestTestdata_AuthCredentialParam locks in the reconciliation from issue #412:
// the header read a security scheme was derived from is not ALSO documented as
// a parameter the client supplies itself.
//
// The value of this test is the three routes that KEEP their parameter. A name
// denylist would have passed the first assertion and broken all three, which is
// why the rule is "a scheme on this operation consumes this header" and not
// "the header is called Authorization".
func TestTestdata_AuthCredentialParam(t *testing.T) {
	// Through the ENGINE, not a single framework config: the header reads here
	// are matched by the net/http patterns that detection always layers under
	// the detected framework, and echo's own config carries none. Testing with
	// echo's config alone would assert on a spec that has no header parameters
	// at all, which would pass while proving nothing.
	cfg := engine.DefaultEngineConfig()
	cfg.InputDir = filepath.Join("..", "testdata", "auth_credential_param")
	out, err := engine.NewEngine(cfg).GenerateOpenAPI()
	if err != nil {
		t.Fatalf("GenerateOpenAPI: %v", err)
	}
	noDanglingRefs(t, out)

	cases := []struct {
		path, method string
		wantHeaders  []string
		why          string
	}{
		{
			path: "/basic/items", method: "GET", wantHeaders: nil,
			why: "an http scheme consumes Authorization, so the parameter is the scheme's job",
		},
		{
			path: "/header/items", method: "GET", wantHeaders: []string{"X-Request-Id"},
			why: "the apiKey scheme consumes X-API-Key; the unrelated header the handler reads stays",
		},
		{
			path: "/query/items", method: "GET", wantHeaders: []string{"Authorization"},
			why: "the key travels as a query parameter, so this header is a different credential",
		},
		{
			path: "/open", method: "GET", wantHeaders: []string{"Authorization"},
			why: "no security governs this operation, so the header is the handler's own",
		},
	}
	for _, c := range cases {
		got := headerParamsOf(t, out, c.path, c.method)
		if len(got) != len(c.wantHeaders) {
			t.Errorf("%s %s: header params %v, want %v — %s", c.method, c.path, got, c.wantHeaders, c.why)
			continue
		}
		for i := range got {
			if got[i] != c.wantHeaders[i] {
				t.Errorf("%s %s: header params %v, want %v — %s", c.method, c.path, got, c.wantHeaders, c.why)
				break
			}
		}
	}

	// Every protected operation still declares its security: the parameter is
	// dropped because the scheme governs the credential, not instead of it.
	for _, path := range []string{"/basic/items", "/header/items", "/query/items"} {
		item := out.Paths[path]
		op := opFor(item, "GET")
		if op == nil || op.Security == nil || len(*op.Security) == 0 {
			t.Errorf("GET %s should still require a security scheme", path)
		}
	}
}

// TestTestdata_AuthNoDuplicateCredentialParam is the issue's own reproduction,
// across the two frameworks it named: the credential must not appear as both a
// scheme and a parameter on any operation of these fixtures.
func TestTestdata_AuthNoDuplicateCredentialParam(t *testing.T) {
	for _, fixture := range []struct {
		name string
		cfg  *spec.APISpecConfig
	}{
		{"auth_gin_perroute", spec.DefaultGinConfig()},
		{"auth_nethttp_wrap", spec.DefaultHTTPConfig()},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			out := loadTestdataWithFixtureConfig(t, fixture.name, fixture.cfg)
			for path, item := range out.Paths {
				for _, method := range []string{"GET", "POST", "PUT", "PATCH", "DELETE"} {
					op := opFor(item, method)
					if op == nil || op.Security == nil || len(*op.Security) == 0 {
						continue
					}
					for _, p := range op.Parameters {
						if p.In == "header" && p.Name == "Authorization" {
							t.Errorf("%s %s declares Authorization as a parameter while a security "+
								"scheme already governs it", method, path)
						}
					}
				}
			}
		})
	}
}
