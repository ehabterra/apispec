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

import "testing"

// The host set is DECLARED, so the cases that must not split are as much the
// point as the ones that do: a path reaching this function may have lost its
// leading slash, and no shape test can tell "v1/users" from a host.
func TestSplitHostFromPath(t *testing.T) {
	hosts := []string{"api.example.com", "localhost:8080", "API.EXAMPLE.COM:8443"}
	cases := []struct {
		in, host, path string
	}{
		// Named.
		{"api.example.com/items", "api.example.com", "/items"},
		{"api.example.com/", "api.example.com", "/"},
		{"localhost:8080/debug", "localhost:8080", "/debug"},
		{"API.example.com/items", "API.example.com", "/items"}, // host names are case-insensitive
		{"api.example.com:8443/x", "api.example.com:8443", "/x"},

		// Not named: left exactly as they came, however host-like they look.
		{"admin.example.com/items", "", "admin.example.com/items"},
		{"api.example.com:9999/x", "", "api.example.com:9999/x"}, // a different port is a different host
		{"/items", "", "/items"},
		{"/", "", "/"},
		{"", "", ""},
		{"v1/users", "", "v1/users"},
		{"{tenant}/items", "", "{tenant}/items"},
		{"api.example.com", "", "api.example.com"}, // no path component
	}
	for _, tc := range cases {
		host, path := splitHostFromPath(tc.in, hosts)
		if host != tc.host || path != tc.path {
			t.Errorf("splitHostFromPath(%q) = (%q, %q), want (%q, %q)", tc.in, host, path, tc.host, tc.path)
		}
	}

	// With nothing declared, nothing is split — the default is today's
	// behaviour, not a guess.
	if host, path := splitHostFromPath("api.example.com/items", nil); host != "" || path != "api.example.com/items" {
		t.Errorf("with no configured hosts: got (%q, %q), want (\"\", unchanged)", host, path)
	}
}

// A project that declares its servers has already said what its hosts are, so
// naming them twice is not required.
func TestKnownHostsIncludesServerHosts(t *testing.T) {
	cfg := &APISpecConfig{
		Hosts: []string{"declared.example.com"},
		Servers: []Server{
			{URL: "https://api.example.com/v1"},
			{URL: "http://localhost:8080"},
			{URL: "/relative"},   // OpenAPI allows a relative server URL: no host
			{URL: "://nonsense"}, // unparseable: contributes nothing
		},
	}
	got := cfg.knownHosts()
	want := map[string]bool{"declared.example.com": true, "api.example.com": true, "localhost:8080": true}
	if len(got) != len(want) {
		t.Errorf("knownHosts() = %v, want the %d hosts in %v", got, len(want), want)
	}
	for _, h := range got {
		if !want[h] {
			t.Errorf("knownHosts() returned %q, which no config field names", h)
		}
	}
	if (&APISpecConfig{}).knownHosts() != nil {
		t.Error("an empty config must declare no hosts")
	}
	var nilCfg *APISpecConfig
	if nilCfg.knownHosts() != nil {
		t.Error("a nil config must declare no hosts")
	}
}
