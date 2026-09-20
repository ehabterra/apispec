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
	"testing"

	"github.com/ehabterra/apispec/internal/metadata"
)

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

// TestStatusArgValue pins reading a status out of an argument in the spellings
// a handler writes one. A status that does not resolve is not a refusal, so a
// miss here costs a silent unreported auth middleware.
func TestStatusArgValue(t *testing.T) {
	meta := newTestMeta()

	t.Run("a bare number", func(t *testing.T) {
		arg := metadata.NewCallArgument(meta)
		arg.SetKind(metadata.KindLiteral)
		arg.SetValue("401")
		if code, ok := statusArgValue(arg); !ok || code != 401 {
			t.Errorf("statusArgValue = (%d, %v), want (401, true)", code, ok)
		}
	})

	t.Run("net/http's constant", func(t *testing.T) {
		// `http.StatusForbidden` — matched on the trailing identifier, so an
		// aliased import resolves the same way.
		arg := mkSelector(meta, mkIdent(meta, "http", ""), mkIdent(meta, "StatusForbidden", ""))
		if code, ok := statusArgValue(arg); !ok || code != 403 {
			t.Errorf("statusArgValue = (%d, %v), want (403, true)", code, ok)
		}
	})

	t.Run("a dot-imported constant", func(t *testing.T) {
		if code, ok := statusArgValue(mkIdent(meta, "StatusUnauthorized", "")); !ok || code != 401 {
			t.Errorf("statusArgValue = (%d, %v), want (401, true)", code, ok)
		}
	})

	t.Run("not a status", func(t *testing.T) {
		lit := metadata.NewCallArgument(meta)
		lit.SetKind(metadata.KindLiteral)
		lit.SetValue(`"unauthorized"`)
		if _, ok := statusArgValue(lit); ok {
			t.Error("a message string was read as a status")
		}
		if _, ok := statusArgValue(mkIdent(meta, "someLocal", "")); ok {
			t.Error("an unrelated identifier was read as a status")
		}
	})
}

// TestRefusalStatusesAreAuthOnly guards the status table against the failure
// that would undo the fix: a refusing middleware is not an authenticating one.
// A rate limiter (429), a size limiter (413) and a timeout (504) all refuse.
func TestRefusalStatusesAreAuthOnly(t *testing.T) {
	cred := stdlibCredentialReads()

	for _, auth := range []int{401, 403} {
		if !cred.refuses(auth) {
			t.Errorf("%d means the request was not authenticated/authorised and is not counted", auth)
		}
	}
	for _, refusal := range []int{400, 404, 409, 413, 415, 422, 429, 500, 503, 504} {
		if cred.refuses(refusal) {
			t.Errorf("%d is a refusal but not an auth one; counting it reports every guard as auth", refusal)
		}
	}
}
