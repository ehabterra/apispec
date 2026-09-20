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

// TestMatchesCall pins the accessor matcher, including the case that decides
// whether the whole classifier is safe: an accessor constraining NOTHING must
// match nothing, because it would otherwise make every call a credential read
// and report every middleware as auth — the bug inverted.
func TestMatchesCall(t *testing.T) {
	cases := []struct {
		name            string
		acc             CredentialAccessor
		call, pkg, recv string
		want            bool
	}{
		{
			name: "call, package and receiver all match",
			acc:  CredentialAccessor{CallRegex: `^BasicAuth$`, PkgRegex: `^net/http$`, RecvTypeRegex: `^\*?(net/http\.)?Request$`},
			call: "BasicAuth", pkg: "net/http", recv: "*Request", want: true,
		},
		{
			name: "the qualified receiver spelling",
			// Metadata renders a receiver either way depending on the call;
			// pinning one made r.BasicAuth() invisible.
			acc:  CredentialAccessor{CallRegex: `^BasicAuth$`, PkgRegex: `^net/http$`, RecvTypeRegex: `^\*?(net/http\.)?Request$`},
			call: "BasicAuth", pkg: "net/http", recv: "*net/http.Request", want: true,
		},
		{
			name: "right name, wrong package",
			acc:  CredentialAccessor{CallRegex: `^BasicAuth$`, PkgRegex: `^net/http$`},
			call: "BasicAuth", pkg: "example.com/x", recv: "", want: false,
		},
		{
			name: "right name, wrong receiver",
			acc:  CredentialAccessor{CallRegex: `^Cookie$`, RecvTypeRegex: `^\*?(net/http\.)?Request$`},
			call: "Cookie", pkg: "net/http", recv: "*Response", want: false,
		},
		{
			name: "an accessor that constrains nothing matches nothing",
			acc:  CredentialAccessor{},
			call: "Anything", pkg: "any/pkg", recv: "*Any", want: false,
		},
		{
			name: "an unparseable regex does not match",
			acc:  CredentialAccessor{CallRegex: `^(unclosed`},
			call: "anything", want: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := matchesCall(tc.acc, tc.call, tc.pkg, tc.recv); got != tc.want {
				t.Errorf("matchesCall = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestCredentialReadConfigEmpty pins the switch that turns the classifier off:
// with no credential surface configured there is nothing to judge by, so the
// classifier must not answer "reads none" for everything — the caller treats
// an empty config as "cannot classify".
func TestCredentialReadConfigEmpty(t *testing.T) {
	if !(CredentialReadConfig{}).empty() {
		t.Error("a zero CredentialReadConfig is not reported empty")
	}
	if (CredentialReadConfig{NameRegexes: []string{"x"}}).empty() {
		t.Error("a config with names is reported empty")
	}
	if (CredentialReadConfig{Accessors: []CredentialAccessor{{CallRegex: "x"}}}).empty() {
		t.Error("a config with accessors is reported empty")
	}
}

// TestStdlibCredentialNamesAreWholeNames guards the one thing a name table must
// not do: match a header that merely looks credential-adjacent. X-Request-Id
// and X-Api-Version are on every service, and matching either would report its
// logging middleware as auth — the noise this issue is about.
func TestStdlibCredentialNamesAreWholeNames(t *testing.T) {
	cred := stdlibCredentialReads()
	match := func(s string) bool {
		for _, re := range cred.NameRegexes {
			if compiled, err := cachedRegex(re); err == nil && compiled.MatchString(s) {
				return true
			}
		}
		return false
	}

	for _, credential := range []string{
		"Authorization", "authorization", "Proxy-Authorization",
		"X-Api-Key", "x-api-key", "X-API-KEY", "X-Auth-Token", "X-Access-Token",
		"Api-Key", "apikey", "auth_token",
	} {
		if !match(credential) {
			t.Errorf("%q is a credential name and was not recognised", credential)
		}
	}

	for _, ordinary := range []string{
		"X-Request-Id", "X-Api-Version", "Content-Type", "Accept",
		"X-Forwarded-For", "User-Agent", "X-Trace-Id", "If-None-Match",
		// Close enough to matter: these are not credentials.
		"X-Api-Deprecation", "Authorization-Info", "X-Keyboard",
	} {
		if match(ordinary) {
			t.Errorf("%q is an ordinary header and was read as a credential", ordinary)
		}
	}
}
